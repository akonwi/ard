package ffi

import (
	"bufio"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/text/unicode/norm"
)

type HostConfig struct {
	Args []string
}

var HostFunctions = NewHostFunctions(HostConfig{})

func NewHostFunctions(config HostConfig) map[string]any {
	return NewHost(config).Functions()
}

func NewHost(config HostConfig) Host {
	args := config.Args
	if args != nil {
		args = append([]string(nil), args...)
	}
	return Host{
		ByteFromInt:          ByteFromInt,
		RuneFromInt:          RuneFromInt,
		RuneFromStr:          RuneFromStr,
		StrFromUtf8:          StrFromUtf8,
		StrFromRunes:         StrFromRunes,
		Base64Decode:         Base64Decode,
		Base64DecodeURL:      Base64DecodeURL,
		Base64Encode:         Base64Encode,
		Base64EncodeURL:      Base64EncodeURL,
		BoolToDynamic:        BoolToDynamic,
		BytesToDynamic:       BytesToDynamic,
		CryptoHashPassword:   CryptoHashPassword,
		CryptoMd5:            CryptoMd5,
		CryptoScryptHash:     CryptoScryptHash,
		CryptoScryptVerify:   CryptoScryptVerify,
		CryptoSha256:         CryptoSha256,
		CryptoSha512:         CryptoSha512,
		CryptoUUID:           CryptoUUID,
		CryptoVerifyPassword: CryptoVerifyPassword,
		DecodeBool:           DecodeBool,
		DecodeByte:           DecodeByte,
		DecodeFloat:          DecodeFloat,
		DecodeInt:            DecodeInt,
		DecodeRune:           DecodeRune,
		DecodeString:         DecodeString,
		DynamicToList:        DynamicToList,
		DynamicToMap:         DynamicToMap,
		ExtractField:         ExtractField,
		FSAppendFile:         FSAppendFile,
		FSCopy:               FSCopy,
		FSCreateDir:          FSCreateDir,
		FSCreateFile:         FSCreateFile,
		FSExists:             FSExists,
		FSIsDir:              FSIsDir,
		FSIsFile:             FSIsFile,
		FSListDir:            FSListDir,
		FSReadFile:           FSReadFile,
		FSWriteFile:          FSWriteFile,
		FloatFormat:          FloatFormat,
		FloatFromInt:         FloatFromInt,
		FloatFromStr:         FloatFromStr,
		FloatToDynamic:       FloatToDynamic,
		GetPathValue:         GetPathValue,
		GetQueryParam:        GetQueryParam,
		GetReqPath:           GetReqPath,
		HTTPDo:               HTTPDo,
		HTTPResponseBody:     HTTPResponseBody,
		HTTPResponseClose:    HTTPResponseClose,
		HTTPResponseHeaders:  HTTPResponseHeaders,
		HTTPResponseStatus:   HTTPResponseStatus,
		HTTPServe:            HTTPServe,
		IntToDynamic:         IntToDynamic,
		IsNil:                IsNil,
		JsonEncode:           JsonEncode,
		JsonToDynamic:        JsonToDynamic,
		ListToDynamic:        ListToDynamic,
		MapToDynamic:         MapToDynamic,
		OsArgs:               func() []string { return hostOSArgs(args) },
		Print:                Print,
		ReadLine:             ReadLine,
		SqlBeginTx:           SqlBeginTx,
		SqlClose:             SqlClose,
		SqlCommit:            SqlCommit,
		SqlCreateConnection:  SqlCreateConnection,
		SqlExecute:           SqlExecute,
		SqlExtractParams:     SqlExtractParams,
		SqlQuery:             SqlQuery,
		SqlRollback:          SqlRollback,
		StrToDynamic:         StrToDynamic,
		VoidToDynamic:        VoidToDynamic,
	}
}

const (
	defaultScryptN       = 16384
	defaultScryptR       = 16
	defaultScryptP       = 1
	defaultScryptDKLen   = 64
	defaultScryptSaltLen = 16
)

type sqlRunner interface {
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

func OsArgs() []string {
	return hostOSArgs(nil)
}

func hostOSArgs(args []string) []string {
	if args != nil {
		return append([]string(nil), args...)
	}
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

func ChannelSend[T any](ch chan T, value T) (sent bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			sent = false
		}
	}()
	ch <- value
	return true
}

func ChannelRecv[T any](ch chan T) Maybe[T] {
	value, ok := <-ch
	if !ok {
		return None[T]()
	}
	return Some(value)
}

func ChannelClose[T any](ch chan T) (closed bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			closed = false
		}
	}()
	close(ch)
	return true
}

func ByteFromInt(value int) Maybe[byte] {
	if value < 0 || value > 255 {
		return None[byte]()
	}
	return Some(byte(value))
}

func RuneFromInt(value int) Maybe[rune] {
	r := rune(value)
	if !utf8.ValidRune(r) {
		return None[rune]()
	}
	return Some(r)
}

func RuneFromStr(value string) Maybe[rune] {
	if !utf8.ValidString(value) {
		return None[rune]()
	}
	runes := []rune(value)
	if len(runes) != 1 || !utf8.ValidRune(runes[0]) {
		return None[rune]()
	}
	return Some(runes[0])
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

func Base64Encode(input []byte, noPad Maybe[bool]) string {
	if noPad.IsSome() && noPad.Value() {
		return base64.RawStdEncoding.EncodeToString(input)
	}
	return base64.StdEncoding.EncodeToString(input)
}

func Base64Decode(input string, noPad Maybe[bool]) ([]byte, error) {
	enc := base64.StdEncoding
	if noPad.IsSome() && noPad.Value() {
		enc = base64.RawStdEncoding
	}
	decoded, err := enc.DecodeString(input)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func Base64EncodeURL(input []byte, noPad Maybe[bool]) string {
	if noPad.IsSome() && noPad.Value() {
		return base64.RawURLEncoding.EncodeToString(input)
	}
	return base64.URLEncoding.EncodeToString(input)
}

func Base64DecodeURL(input string, noPad Maybe[bool]) ([]byte, error) {
	enc := base64.URLEncoding
	if noPad.IsSome() && noPad.Value() {
		enc = base64.RawURLEncoding
	}
	decoded, err := enc.DecodeString(input)
	if err != nil {
		return nil, err
	}
	return decoded, nil
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

func CryptoHashPassword(password string, cost Maybe[int]) (string, error) {
	hashCost := bcrypt.DefaultCost
	if cost.IsSome() {
		hashCost = cost.Value()
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

func CryptoScryptHash(password string, saltHex Maybe[string], n Maybe[int], r Maybe[int], p Maybe[int], dkLen Maybe[int]) (string, error) {
	password = norm.NFKC.String(password)
	nVal := maybeOr(n, defaultScryptN)
	rVal := maybeOr(r, defaultScryptR)
	pVal := maybeOr(p, defaultScryptP)
	dkLenVal := maybeOr(dkLen, defaultScryptDKLen)

	if err := validateScryptParams(nVal, rVal, pVal, dkLenVal); err != nil {
		return "", fmt.Errorf("scrypt_runtime: %s", err.Error())
	}

	var saltHexValue string
	if saltHex.IsSome() {
		saltHexValue = strings.TrimSpace(saltHex.Value())
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

func CryptoScryptVerify(password, hash string, n Maybe[int], r Maybe[int], p Maybe[int], dkLen Maybe[int]) (bool, error) {
	password = norm.NFKC.String(password)
	hash = strings.TrimSpace(hash)
	nVal := maybeOr(n, defaultScryptN)
	rVal := maybeOr(r, defaultScryptR)
	pVal := maybeOr(p, defaultScryptP)
	dkLenVal := maybeOr(dkLen, defaultScryptDKLen)

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

func maybeOr(value Maybe[int], fallback int) int {
	if value.IsSome() {
		return value.Value()
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

func FloatFromStr(str string) Maybe[float64] {
	value, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return None[float64]()
	}
	return Some(value)
}

func FloatFromInt(value int) float64 {
	return float64(value)
}

func FloatFloor(value float64) float64 {
	return math.Floor(value)
}

func FloatCeil(value float64) float64 {
	return math.Ceil(value)
}

func FloatRound(value float64) float64 {
	return math.Round(value)
}

func FloatFormat(value float64, decimals int) string {
	if decimals < 0 {
		decimals = 0
	}
	return strconv.FormatFloat(value, 'f', decimals, 64)
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

func FSCreateFile(path string) (bool, error) {
	file, err := os.Create(path)
	if err != nil {
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	return true, nil
}

func FSWriteFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func FSAppendFile(path string, content string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(content)
	return err
}

func FSReadFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func FSCopy(from string, to string) error {
	content, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, content, 0o644)
}

func FSCreateDir(path string) error {
	return os.MkdirAll(path, 0o755)
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

func SqlCreateConnection(connectionString string) (*sql.DB, error) {
	driver := SqlDetectDriver(connectionString)
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

func SqlClose(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("SQL Error: invalid connection object")
	}
	return db.Close()
}

func SqlBeginTx(db *sql.DB) (*sql.Tx, error) {
	if db == nil {
		return nil, fmt.Errorf("SQL Error: invalid connection object")
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return tx, nil
}

func SqlCommit(tx *sql.Tx) error {
	if tx == nil {
		return fmt.Errorf("SQL Error: invalid transaction object")
	}
	return tx.Commit()
}

func SqlRollback(tx *sql.Tx) error {
	if tx == nil {
		return fmt.Errorf("SQL Error: invalid transaction object")
	}
	return tx.Rollback()
}

func SqlExtractParams(sqlStr string) []string {
	delimiters := []string{" ", "(", ")", ",", ";", "=", "<", ">", "!", "\t", "\n", "\r"}
	tokens := splitSQLByMultipleDelimiters(sqlStr, delimiters)
	var paramNames []string
	for _, token := range tokens {
		if strings.HasPrefix(token, "@") && len(token) > 1 {
			paramName := strings.TrimLeft(token[1:], "@")
			paramName = strings.TrimRight(paramName, ".,;:!?")
			if paramName != "" {
				paramNames = append(paramNames, paramName)
			}
		}
	}
	return paramNames
}

func SqlQuery(conn any, driver string, sqlStr string, values []any) ([]any, error) {
	runner, ok := resolveSQLRunner(conn)
	if !ok {
		return nil, fmt.Errorf("SQL Error: invalid connection object")
	}
	sqlStr = normalizeSQLPlaceholders(sqlStr, driver)
	return executeSQLQuery(runner, sqlStr, values)
}

func SqlExecute(conn any, driver string, sqlStr string, values []any) error {
	runner, ok := resolveSQLRunner(conn)
	if !ok {
		return fmt.Errorf("SQL Error: invalid connection object")
	}
	sqlStr = normalizeSQLPlaceholders(sqlStr, driver)
	_, err := runner.Exec(sqlStr, values...)
	return err
}

func SqlDetectDriver(connStr string) string {
	connStr = strings.TrimSpace(connStr)
	if strings.HasPrefix(connStr, "postgres://") || strings.HasPrefix(connStr, "postgresql://") {
		return "pgx"
	}
	if strings.Contains(connStr, "@tcp(") || strings.Contains(connStr, "@unix(") {
		return "mysql"
	}
	return "sqlite3"
}

func splitSQLByMultipleDelimiters(s string, delimiters []string) []string {
	result := s
	for _, delimiter := range delimiters {
		result = strings.ReplaceAll(result, delimiter, " ")
	}
	tokens := strings.Split(result, " ")
	nonEmpty := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token != "" {
			nonEmpty = append(nonEmpty, token)
		}
	}
	return nonEmpty
}

func resolveSQLRunner(raw any) (sqlRunner, bool) {
	if db, ok := raw.(*sql.DB); ok && db != nil {
		return db, true
	}
	if tx, ok := raw.(*sql.Tx); ok && tx != nil {
		return tx, true
	}
	return nil, false
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

func executeSQLQuery(runner sqlRunner, sqlStr string, values []any) ([]any, error) {
	rows, err := runner.Query(sqlStr, values...)
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

func StrToDynamic(value string) any {
	return value
}

func IntToDynamic(value int) any {
	return value
}

func FloatToDynamic(value float64) any {
	return value
}

func BoolToDynamic(value bool) any {
	return value
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
	return data == nil
}

func JsonToDynamic(input string) (any, error) {
	var out any
	if err := jsonv2.Unmarshal([]byte(input), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func JsonEncode(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func DecodeString(data any) Result[string, Error] {
	if value, ok := data.(string); ok {
		return Ok[string, Error](value)
	}
	return Err[string](decodeError("Str", formatDynamicValueForError(data)))
}

func DecodeInt(data any) Result[int, Error] {
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	maxUintInt := uint64(^uint(0) >> 1)
	switch value := data.(type) {
	case int:
		return Ok[int, Error](value)
	case int8:
		return Ok[int, Error](int(value))
	case int16:
		return Ok[int, Error](int(value))
	case int32:
		return Ok[int, Error](int(value))
	case int64:
		if value >= minInt && value <= maxInt {
			return Ok[int, Error](int(value))
		}
	case uint:
		if uint64(value) <= maxUintInt {
			return Ok[int, Error](int(value))
		}
	case uint8:
		return Ok[int, Error](int(value))
	case uint16:
		return Ok[int, Error](int(value))
	case uint32:
		if uint64(value) <= maxUintInt {
			return Ok[int, Error](int(value))
		}
	case uint64:
		if value <= maxUintInt {
			return Ok[int, Error](int(value))
		}
	case float64:
		parsed := int(value)
		if value == float64(parsed) {
			return Ok[int, Error](parsed)
		}
	case json.Number:
		if parsed, err := value.Int64(); err == nil && parsed >= minInt && parsed <= maxInt {
			return Ok[int, Error](int(parsed))
		}
	}
	return Err[int](decodeError("Int", formatDynamicValueForError(data)))
}

func DecodeByte(data any) Result[byte, Error] {
	decoded := DecodeInt(data)
	if !decoded.Ok {
		return Err[byte](decodeError("Byte", formatDynamicValueForError(data)))
	}
	value := decoded.Value
	if value < 0 || value > 255 {
		return Err[byte](decodeError("Byte", formatDynamicValueForError(data)))
	}
	return Ok[byte, Error](byte(value))
}

func DecodeRune(data any) Result[rune, Error] {
	decoded := DecodeInt(data)
	if !decoded.Ok {
		return Err[rune](decodeError("Rune", formatDynamicValueForError(data)))
	}
	value := decoded.Value
	r := rune(value)
	if !utf8.ValidRune(r) {
		return Err[rune](decodeError("Rune", formatDynamicValueForError(data)))
	}
	return Ok[rune, Error](r)
}

func DecodeFloat(data any) Result[float64, Error] {
	switch value := data.(type) {
	case float64:
		return Ok[float64, Error](value)
	case int:
		return Ok[float64, Error](float64(value))
	case int64:
		return Ok[float64, Error](float64(value))
	case json.Number:
		if parsed, err := value.Float64(); err == nil {
			return Ok[float64, Error](parsed)
		}
	}
	return Err[float64](decodeError("Float", formatDynamicValueForError(data)))
}

func DecodeBool(data any) Result[bool, Error] {
	if value, ok := data.(bool); ok {
		return Ok[bool, Error](value)
	}
	return Err[bool](decodeError("Bool", formatDynamicValueForError(data)))
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

func DynamicToMap(data any) (map[any]any, error) {
	if data == nil {
		return nil, fmt.Errorf("Void")
	}
	if values, ok := data.(map[string]any); ok {
		out := make(map[any]any, len(values))
		for key, value := range values {
			out[key] = value
		}
		return out, nil
	}
	if values, ok := data.(map[any]any); ok {
		out := make(map[any]any, len(values))
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

func HTTPDo(method string, url string, body any, headers map[string]string, timeout Maybe[int]) (*http.Response, error) {
	var bodyReader io.Reader = strings.NewReader("")
	if body != nil {
		switch value := body.(type) {
		case string:
			bodyReader = strings.NewReader(value)
		case []byte:
			bodyReader = strings.NewReader(string(value))
		default:
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			bodyReader = strings.NewReader(string(encoded))
		}
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	if timeout.IsSome() {
		client.Timeout = time.Duration(timeout.Value()) * time.Second
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func HTTPResponseStatus(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

func HTTPResponseHeaders(resp *http.Response) map[string]string {
	if resp == nil {
		return map[string]string{}
	}
	headers := make(map[string]string, len(resp.Header))
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
}

func HTTPResponseBody(resp *http.Response) (string, error) {
	if resp == nil {
		return "", fmt.Errorf("invalid HTTP response handle")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func HTTPResponseClose(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func GetReqPath(req *http.Request) string {
	if req != nil && req.URL != nil {
		return req.URL.Path
	}
	return ""
}

func GetPathValue(req *http.Request, name string) string {
	if req != nil {
		return req.PathValue(name)
	}
	return ""
}

func GetQueryParam(req *http.Request, name string) string {
	if req != nil && req.URL != nil {
		return req.URL.Query().Get(name)
	}
	return ""
}

func HTTPServe(port int, handlers map[string]func(Request, *Response)) error {
	mux := http.NewServeMux()
	for path, handler := range handlers {
		path := path
		handler := handler
		mux.HandleFunc(convertHTTPPattern(path), func(writer http.ResponseWriter, req *http.Request) {
			ardReq := Request{
				Method:  methodFromHTTPRequest(req.Method),
				Url:     req.URL.String(),
				Headers: requestHeaders(req),
				Body:    requestBody(req),
				Raw:     Some[*http.Request](req),
			}
			ardRes := Response{
				Status:  200,
				Headers: map[string]string{},
			}
			handler(ardReq, &ardRes)
			for key, value := range ardRes.Headers {
				writer.Header().Set(key, value)
			}
			status := ardRes.Status
			if status == 0 {
				status = 200
			}
			writer.WriteHeader(status)
			_, _ = io.WriteString(writer, ardRes.Body)
		})
	}
	return http.ListenAndServe(fmt.Sprintf(":%d", port), mux)
}

func convertHTTPPattern(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

func methodFromHTTPRequest(method string) Method {
	switch method {
	case "GET":
		return Method(0)
	case "POST":
		return Method(1)
	case "PUT":
		return Method(2)
	case "DELETE":
		return Method(3)
	case "PATCH":
		return Method(4)
	case "OPTIONS":
		return Method(5)
	default:
		return Method(0)
	}
}

func requestHeaders(req *http.Request) map[string]string {
	headers := make(map[string]string, len(req.Header))
	for key, values := range req.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
}

func requestBody(req *http.Request) Maybe[any] {
	if req.Body == nil {
		return None[any]()
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil || len(body) == 0 {
		return None[any]()
	}
	return Some[any](string(body))
}

func decodeError(expected string, found string) Error {
	return Error{Expected: expected, Found: found}
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
