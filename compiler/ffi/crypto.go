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

func CryptoMd5(input string) string {
	sum := md5.Sum([]byte(input))
	return hex.EncodeToString(sum[:])
}

func CryptoSha256(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func CryptoSha512(input string) string {
	sum := sha512.Sum512([]byte(input))
	return hex.EncodeToString(sum[:])
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

func CryptoScryptHash(password string, saltHex *string, n, r, p, dkLen *int) (string, error) {
	password = norm.NFKC.String(password)

	nVal := defaultScryptN
	rVal := defaultScryptR
	pVal := defaultScryptP
	dkLenVal := defaultScryptDKLen

	if n != nil {
		nVal = *n
	}
	if r != nil {
		rVal = *r
	}
	if p != nil {
		pVal = *p
	}
	if dkLen != nil {
		dkLenVal = *dkLen
	}

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

func CryptoScryptVerify(password, hash string, n, r, p, dkLen *int) (bool, error) {
	password = norm.NFKC.String(password)
	hash = strings.TrimSpace(hash)

	nVal := defaultScryptN
	rVal := defaultScryptR
	pVal := defaultScryptP
	dkLenVal := defaultScryptDKLen

	if n != nil {
		nVal = *n
	}
	if r != nil {
		rVal = *r
	}
	if p != nil {
		pVal = *p
	}
	if dkLen != nil {
		dkLenVal = *dkLen
	}

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

func CryptoUUID() string {
	u := make([]byte, 16)
	if _, err := rand.Read(u); err != nil {
		panic(fmt.Errorf("CryptoUUID failed: %w", err))
	}

	// RFC 4122 version 4, variant 10xx
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}
