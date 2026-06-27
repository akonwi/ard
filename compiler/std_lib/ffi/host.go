package ffi

import (
	"bufio"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/text/unicode/norm"
)

const (
	defaultScryptN       = 16384
	defaultScryptR       = 16
	defaultScryptP       = 1
	defaultScryptDKLen   = 64
	defaultScryptSaltLen = 16
)

// Runner is the shared query surface implemented by both *sql.DB and *sql.Tx.
// Ard's sql module references it directly (ffi::Runner) instead of a Db|Tx
// union, so a connection or transaction handle can be passed without a runtime
// type switch.
type Runner interface {
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

func OsArgs() []string {
	return append([]string(nil), os.Args...)
}

func Print(str string) {
	fmt.Println(str)
}

var (
	stdinReaderMu sync.Mutex
	stdinReader   *bufio.Reader
	stdinSource   *os.File
)

func ReadLine() (string, error) {
	stdinReaderMu.Lock()
	defer stdinReaderMu.Unlock()

	if stdinReader == nil || stdinSource != os.Stdin {
		stdinSource = os.Stdin
		stdinReader = bufio.NewReader(os.Stdin)
	}

	line, err := stdinReader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return strings.TrimRight(line, "\r\n"), nil
		}
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func ByteFromInt(value int) (byte, bool) {
	if value < 0 || value > 255 {
		return 0, false
	}
	return byte(value), true
}

func RuneFromInt(value int) (rune, bool) {
	r := rune(value)
	if !utf8.ValidRune(r) {
		return 0, false
	}
	return r, true
}

func StrFromUtf8(bytes []byte) (string, error) {
	if !utf8.Valid(bytes) {
		return "", errors.New("invalid UTF-8")
	}
	return string(bytes), nil
}

func StrFromRunes(runes []rune) (string, error) {
	for _, r := range runes {
		if !utf8.ValidRune(r) {
			return "", errors.New("invalid Unicode scalar value")
		}
	}
	return string(runes), nil
}

func CryptoMd5(input []byte) []byte {
	sum := md5.Sum(input)
	return sum[:]
}

func CryptoSha256(input []byte) []byte {
	sum := sha256.Sum256(input)
	return sum[:]
}

func CryptoSha512(input []byte) []byte {
	sum := sha512.Sum512(input)
	return sum[:]
}

func CryptoHashPassword(password string, cost *int) (string, error) {
	hashCost := bcrypt.DefaultCost
	if cost != nil {
		hashCost = *cost
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), hashCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func CryptoVerifyPassword(password, hashed string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, nil
	}
	return false, err
}

func CryptoScryptHash(password string, saltHex *string, n *int, r *int, p *int, dkLen *int) (string, error) {
	password = norm.NFKC.String(password)
	nVal := ptrOr(n, defaultScryptN)
	rVal := ptrOr(r, defaultScryptR)
	pVal := ptrOr(p, defaultScryptP)
	dkLenVal := ptrOr(dkLen, defaultScryptDKLen)

	if err := validateScryptParams(nVal, rVal, pVal, dkLenVal); err != nil {
		return "", fmt.Errorf("scrypt_runtime: %s", err.Error())
	}

	var saltHexValue string
	if saltHex != nil {
		saltHexValue = strings.TrimSpace(*saltHex)
		decoded, err := hex.DecodeString(saltHexValue)
		if err != nil {
			return "", fmt.Errorf("scrypt_runtime: invalid salt hex: %s", err.Error())
		}
		if len(decoded) == 0 {
			return "", errors.New("scrypt_runtime: invalid salt hex: empty salt")
		}
	} else {
		saltBytes := make([]byte, defaultScryptSaltLen)
		if _, err := rand.Read(saltBytes); err != nil {
			return "", fmt.Errorf("scrypt_runtime: failed to generate salt: %s", err.Error())
		}
		saltHexValue = hex.EncodeToString(saltBytes)
	}

	derived, err := scrypt.Key([]byte(password), []byte(saltHexValue), nVal, rVal, pVal, dkLenVal)
	if err != nil {
		return "", fmt.Errorf("scrypt_runtime: %s", err.Error())
	}

	return fmt.Sprintf("%s:%s", saltHexValue, hex.EncodeToString(derived)), nil
}

func CryptoScryptVerify(password, hash string, n *int, r *int, p *int, dkLen *int) (bool, error) {
	password = norm.NFKC.String(password)
	hash = strings.TrimSpace(hash)
	nVal := ptrOr(n, defaultScryptN)
	rVal := ptrOr(r, defaultScryptR)
	pVal := ptrOr(p, defaultScryptP)
	dkLenVal := ptrOr(dkLen, defaultScryptDKLen)

	if err := validateScryptParams(nVal, rVal, pVal, dkLenVal); err != nil {
		return false, fmt.Errorf("scrypt_runtime: %s", err.Error())
	}

	parts := strings.Split(hash, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false, errors.New("scrypt_malformed_hash: expected format <salt_hex>:<derived_key_hex>")
	}

	saltHex := parts[0]
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return false, fmt.Errorf("scrypt_malformed_hash: invalid salt hex: %s", err.Error())
	}
	if len(salt) == 0 {
		return false, errors.New("scrypt_malformed_hash: invalid salt hex: empty salt")
	}

	storedKey, err := hex.DecodeString(parts[1])
	if err != nil {
		return false, fmt.Errorf("scrypt_malformed_hash: invalid derived key hex: %s", err.Error())
	}
	if len(storedKey) != dkLenVal {
		return false, fmt.Errorf("scrypt_malformed_hash: derived key length mismatch: expected %d bytes, got %d", dkLenVal, len(storedKey))
	}

	derived, err := scrypt.Key([]byte(password), []byte(saltHex), nVal, rVal, pVal, dkLenVal)
	if err != nil {
		return false, fmt.Errorf("scrypt_runtime: %s", err.Error())
	}

	return subtle.ConstantTimeCompare(derived, storedKey) == 1, nil
}

func ptrOr(value *int, fallback int) int {
	if value != nil {
		return *value
	}
	return fallback
}

func validateScryptParams(n, r, p, dkLen int) error {
	if n <= 1 || n&(n-1) != 0 {
		return fmt.Errorf("invalid N parameter: must be a power of two greater than 1")
	}
	if r <= 0 {
		return fmt.Errorf("invalid r parameter: must be greater than 0")
	}
	if p <= 0 {
		return fmt.Errorf("invalid p parameter: must be greater than 0")
	}
	if dkLen <= 0 {
		return fmt.Errorf("invalid dk_len parameter: must be greater than 0")
	}
	return nil
}

func FloatFromInt(value int) float64 {
	return float64(value)
}

func FSExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func FSIsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func FSIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func FSListDir(path string) (map[string]bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(entries))
	for _, entry := range entries {
		out[entry.Name()] = !entry.IsDir()
	}
	return out, nil
}

func CryptoUUID() string {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		panic(fmt.Errorf("CryptoUUID failed: %w", err))
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4],
		uuid[4:6],
		uuid[6:8],
		uuid[8:10],
		uuid[10:16],
	)
}

func SqlCreateConnection(driver string, connectionString string) (*sql.DB, error) {
	db, err := sql.Open(driver, connectionString)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return db, nil
}

func normalizeSQLPlaceholders(sqlStr string, driver string) string {
	if driver != "pgx" {
		return sqlStr
	}

	var out strings.Builder
	out.Grow(len(sqlStr) + 16)
	index := 1
	for i := 0; i < len(sqlStr); i++ {
		ch := sqlStr[i]
		if ch == '@' {
			j := i + 1
			for j < len(sqlStr) {
				c := sqlStr[j]
				isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
				isDigit := c >= '0' && c <= '9'
				if isAlpha || isDigit || c == '_' {
					j++
					continue
				}
				break
			}
			if j > i+1 {
				out.WriteString(fmt.Sprintf("$%d", index))
				index++
				i = j - 1
				continue
			}
		}
		if ch == '?' {
			out.WriteString(fmt.Sprintf("$%d", index))
			index++
			continue
		}
		out.WriteByte(ch)
	}
	return out.String()
}

// ExecuteSQLQuery runs a statement on the given runner (a *sql.DB or *sql.Tx)
// and returns the result rows as []any. Statements that produce no result set
// (inserts, updates, DDL) simply yield no rows.
func ExecuteSQLQuery(conn Runner, driver string, sqlStr string, values []any) ([]any, error) {
	if conn == nil {
		return nil, fmt.Errorf("SQL Error: invalid connection object")
	}
	sqlStr = normalizeSQLPlaceholders(sqlStr, driver)
	rows, err := conn.Query(sqlStr, values...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get column names: %w", err)
	}

	var results []any
	for rows.Next() {
		scanValues := make([]any, len(columns))
		scanTargets := make([]any, len(columns))
		for i := range scanValues {
			scanTargets[i] = &scanValues[i]
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		rowMap := make(map[string]any, len(columns))
		for i, columnName := range columns {
			rowMap[columnName] = normalizeSQLDynamicValue(scanValues[i])
		}
		results = append(results, rowMap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}
	return results, nil
}

func normalizeSQLDynamicValue(value any) any {
	switch value := value.(type) {
	case []byte:
		return string(value)
	default:
		return value
	}
}

func BytesToDynamic(value []byte) any {
	return value
}

func VoidToDynamic() any {
	return nil
}

func ListToDynamic(list []any) any {
	return list
}

func MapToDynamic(from map[string]any) any {
	return from
}

func IsNil(data any) bool {
	if data == nil {
		return true
	}
	value := reflect.ValueOf(data)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return value.IsNil()
	default:
		return false
	}
}

func JsonToDynamic(input string) (any, error) {
	var out any
	if err := jsonv2.Unmarshal([]byte(input), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// JsonEncode marshals v to a JSON string. Marshal accepts any value, so Ard's
// json::encode is an ordinary function that calls this directly via use go:;
// only json::parse needs intrinsic lowering for its typed target.
func JsonEncode(v any) (string, error) {
	b, err := jsonv2.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// The Decode* functions report failure with an error whose message is the
// formatted "found" value. The Ard decode module supplies the "expected" type
// name and decode path, building its structured decode error in Ard (ADR 0031).
func DecodeString(data any) (string, error) {
	if value, ok := data.(string); ok {
		return value, nil
	}
	return "", errors.New(formatDynamicValueForError(data))
}

func DecodeInt(data any) (int, error) {
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	maxUintInt := uint64(^uint(0) >> 1)
	switch value := data.(type) {
	case int:
		return value, nil
	case int8:
		return int(value), nil
	case int16:
		return int(value), nil
	case int32:
		return int(value), nil
	case int64:
		if value >= minInt && value <= maxInt {
			return int(value), nil
		}
	case uint:
		if uint64(value) <= maxUintInt {
			return int(value), nil
		}
	case uint8:
		return int(value), nil
	case uint16:
		return int(value), nil
	case uint32:
		if uint64(value) <= maxUintInt {
			return int(value), nil
		}
	case uint64:
		if value <= maxUintInt {
			return int(value), nil
		}
	case float64:
		parsed := int(value)
		if value == float64(parsed) {
			return parsed, nil
		}
	case json.Number:
		if parsed, err := value.Int64(); err == nil && parsed >= minInt && parsed <= maxInt {
			return int(parsed), nil
		}
	}
	return 0, errors.New(formatDynamicValueForError(data))
}

func DecodeByte(data any) (byte, error) {
	value, err := DecodeInt(data)
	if err != nil || value < 0 || value > 255 {
		return 0, errors.New(formatDynamicValueForError(data))
	}
	return byte(value), nil
}

func DecodeRune(data any) (rune, error) {
	value, err := DecodeInt(data)
	if err != nil {
		return 0, errors.New(formatDynamicValueForError(data))
	}
	r := rune(value)
	if !utf8.ValidRune(r) {
		return 0, errors.New(formatDynamicValueForError(data))
	}
	return r, nil
}

func DecodeFloat(data any) (float64, error) {
	switch value := data.(type) {
	case float64:
		return value, nil
	case int:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case json.Number:
		if parsed, err := value.Float64(); err == nil {
			return parsed, nil
		}
	}
	return 0, errors.New(formatDynamicValueForError(data))
}

func DecodeBool(data any) (bool, error) {
	if value, ok := data.(bool); ok {
		return value, nil
	}
	return false, errors.New(formatDynamicValueForError(data))
}

func DynamicToList(data any) ([]any, error) {
	if data == nil {
		return nil, fmt.Errorf("Void")
	}
	if values, ok := data.([]any); ok {
		return values, nil
	}
	return nil, fmt.Errorf("%s", formatDynamicValueForError(data))
}

func DynamicToMap(data any) (map[string]any, error) {
	if data == nil {
		return nil, fmt.Errorf("Void")
	}
	if values, ok := data.(map[string]any); ok {
		return values, nil
	}
	if values, ok := data.(map[any]any); ok {
		out := make(map[string]any, len(values))
		for key, value := range values {
			keyString, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("%s", formatDynamicValueForError(data))
			}
			out[keyString] = value
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s", formatDynamicValueForError(data))
}

func ExtractField(data any, name string) (any, error) {
	if values, ok := data.(map[string]any); ok {
		value, ok := values[name]
		if !ok {
			return nil, fmt.Errorf("Missing field %q", name)
		}
		return value, nil
	}
	if values, ok := data.(map[any]any); ok {
		value, ok := values[name]
		if !ok {
			return nil, fmt.Errorf("Missing field %q", name)
		}
		return value, nil
	}
	return nil, fmt.Errorf("%s", formatDynamicValueForError(data))
}

func formatDynamicValueForError(data any) string {
	switch value := data.(type) {
	case nil:
		return "null"
	case string:
		if len(value) > 50 {
			return fmt.Sprintf("%q", value[:47]+"...")
		}
		return fmt.Sprintf("%q", value)
	case bool:
		return strconv.FormatBool(value)
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case json.Number:
		return value.String()
	case []any:
		if len(value) == 0 {
			return "[]"
		}
		if len(value) <= 3 {
			parts := make([]string, len(value))
			for i, item := range value {
				parts[i] = formatDynamicValueForError(item)
			}
			return "[" + strings.Join(parts, ", ") + "]"
		}
		return fmt.Sprintf("[array with %d elements]", len(value))
	case map[string]any:
		return formatStringAnyMapForError(value)
	case map[any]any:
		return formatAnyMapForError(value)
	default:
		return fmt.Sprintf("%T", data)
	}
}

func formatStringAnyMapForError(values map[string]any) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 3 {
		return fmt.Sprintf("{object with keys: %v}", keys[:3])
	}
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = fmt.Sprintf("%s: %s", key, formatDynamicValueForError(values[key]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func formatAnyMapForError(values map[any]any) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]any, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j])
	})
	if len(keys) > 3 {
		return fmt.Sprintf("{object with keys: %v}", keys[:3])
	}
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = fmt.Sprintf("%s: %s", formatDynamicValueForError(key), formatDynamicValueForError(values[key]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
