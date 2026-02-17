package ffi

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
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

func CryptoMd5(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("CryptoMd5 expects 1 argument, got %d", len(args)))
	}

	sum := md5.Sum([]byte(args[0].AsString()))
	return runtime.MakeStr(hex.EncodeToString(sum[:]))
}

func CryptoSha256(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("CryptoSha256 expects 1 argument, got %d", len(args)))
	}

	sum := sha256.Sum256([]byte(args[0].AsString()))
	return runtime.MakeStr(hex.EncodeToString(sum[:]))
}

func CryptoSha512(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("CryptoSha512 expects 1 argument, got %d", len(args)))
	}

	sum := sha512.Sum512([]byte(args[0].AsString()))
	return runtime.MakeStr(hex.EncodeToString(sum[:]))
}

func CryptoHashPassword(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 && len(args) != 2 {
		panic(fmt.Errorf("CryptoHashPassword expects 1 or 2 arguments, got %d", len(args)))
	}

	password := args[0].AsString()
	cost := bcrypt.DefaultCost

	if len(args) == 2 && !args[1].IsNone() {
		cost = args[1].AsInt()
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	return runtime.MakeOk(runtime.MakeStr(string(hashed)))
}

func CryptoVerifyPassword(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 2 {
		panic(fmt.Errorf("CryptoVerifyPassword expects 2 arguments, got %d", len(args)))
	}

	password := args[0].AsString()
	hashed := args[1].AsString()
	err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password))
	if err == nil {
		return runtime.MakeOk(runtime.MakeBool(true))
	}

	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return runtime.MakeOk(runtime.MakeBool(false))
	}

	return runtime.MakeErr(runtime.MakeStr(err.Error()))
}

func CryptoScryptHash(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) < 1 || len(args) > 6 {
		panic(fmt.Errorf("CryptoScryptHash expects 1 to 6 arguments, got %d", len(args)))
	}

	password := norm.NFKC.String(args[0].AsString())

	n := defaultScryptN
	r := defaultScryptR
	p := defaultScryptP
	dkLen := defaultScryptDKLen

	if len(args) >= 3 && !args[2].IsNone() {
		n = args[2].AsInt()
	}
	if len(args) >= 4 && !args[3].IsNone() {
		r = args[3].AsInt()
	}
	if len(args) >= 5 && !args[4].IsNone() {
		p = args[4].AsInt()
	}
	if len(args) >= 6 && !args[5].IsNone() {
		dkLen = args[5].AsInt()
	}

	if err := validateScryptParams(n, r, p, dkLen); err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("scrypt_runtime: %s", err.Error())))
	}

	var saltHex string
	if len(args) >= 2 && !args[1].IsNone() {
		saltHex = strings.TrimSpace(args[1].AsString())
		decoded, err := hex.DecodeString(saltHex)
		if err != nil {
			return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("scrypt_runtime: invalid salt hex: %s", err.Error())))
		}
		if len(decoded) == 0 {
			return runtime.MakeErr(runtime.MakeStr("scrypt_runtime: invalid salt hex: empty salt"))
		}
	} else {
		saltBytes := make([]byte, defaultScryptSaltLen)
		if _, err := rand.Read(saltBytes); err != nil {
			return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("scrypt_runtime: failed to generate salt: %s", err.Error())))
		}
		saltHex = hex.EncodeToString(saltBytes)
	}

	derived, err := scrypt.Key([]byte(password), []byte(saltHex), n, r, p, dkLen)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("scrypt_runtime: %s", err.Error())))
	}

	hash := fmt.Sprintf("%s:%s", saltHex, hex.EncodeToString(derived))
	return runtime.MakeOk(runtime.MakeStr(hash))
}

func CryptoScryptVerify(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) < 2 || len(args) > 6 {
		panic(fmt.Errorf("CryptoScryptVerify expects 2 to 6 arguments, got %d", len(args)))
	}

	password := norm.NFKC.String(args[0].AsString())
	hash := strings.TrimSpace(args[1].AsString())

	n := defaultScryptN
	r := defaultScryptR
	p := defaultScryptP
	dkLen := defaultScryptDKLen

	if len(args) >= 3 && !args[2].IsNone() {
		n = args[2].AsInt()
	}
	if len(args) >= 4 && !args[3].IsNone() {
		r = args[3].AsInt()
	}
	if len(args) >= 5 && !args[4].IsNone() {
		p = args[4].AsInt()
	}
	if len(args) >= 6 && !args[5].IsNone() {
		dkLen = args[5].AsInt()
	}

	if err := validateScryptParams(n, r, p, dkLen); err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("scrypt_runtime: %s", err.Error())))
	}

	parts := strings.Split(hash, ":")
	if len(parts) != 2 {
		return runtime.MakeErr(runtime.MakeStr("scrypt_malformed_hash: expected format <salt_hex>:<derived_key_hex>"))
	}
	if parts[0] == "" || parts[1] == "" {
		return runtime.MakeErr(runtime.MakeStr("scrypt_malformed_hash: expected format <salt_hex>:<derived_key_hex>"))
	}

	saltHex := parts[0]
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("scrypt_malformed_hash: invalid salt hex: %s", err.Error())))
	}
	if len(salt) == 0 {
		return runtime.MakeErr(runtime.MakeStr("scrypt_malformed_hash: invalid salt hex: empty salt"))
	}

	storedKey, err := hex.DecodeString(parts[1])
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("scrypt_malformed_hash: invalid derived key hex: %s", err.Error())))
	}
	if len(storedKey) != dkLen {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("scrypt_malformed_hash: derived key length mismatch: expected %d bytes, got %d", dkLen, len(storedKey))))
	}

	derived, err := scrypt.Key([]byte(password), []byte(saltHex), n, r, p, dkLen)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("scrypt_runtime: %s", err.Error())))
	}

	matched := subtle.ConstantTimeCompare(derived, storedKey) == 1
	return runtime.MakeOk(runtime.MakeBool(matched))
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

func CryptoUUID(_ []*runtime.Object, _ checker.Type) *runtime.Object {
	u := make([]byte, 16)
	if _, err := rand.Read(u); err != nil {
		panic(fmt.Errorf("CryptoUUID failed: %w", err))
	}

	// RFC 4122 version 4, variant 10xx
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80

	return runtime.MakeStr(fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16]))
}
