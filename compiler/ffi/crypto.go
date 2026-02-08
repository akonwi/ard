package ffi

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
	"golang.org/x/crypto/bcrypt"
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
