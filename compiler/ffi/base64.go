package ffi

import (
	"encoding/base64"
)

// Base64Encode encodes the input using standard base64 encoding with padding.
func Base64Encode(input string) string {
	return base64.StdEncoding.EncodeToString([]byte(input))
}

// Base64Decode decodes a standard base64-encoded string (with padding).
// Returns the decoded string or an error describing the invalid input.
func Base64Decode(input string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// Base64EncodeURL encodes the input using base64url encoding with padding.
// This uses the URL-safe alphabet (- and _ instead of + and /).
func Base64EncodeURL(input string) string {
	return base64.URLEncoding.EncodeToString([]byte(input))
}

// Base64DecodeURL decodes a base64url-encoded string (with padding).
func Base64DecodeURL(input string) (string, error) {
	decoded, err := base64.URLEncoding.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// Base64EncodeURLNoPad encodes the input using base64url encoding without
// trailing `=` padding. This is the form required by PKCE (OAuth 2.1),
// JWTs, and other specifications that disallow padding.
func Base64EncodeURLNoPad(input string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(input))
}

// Base64DecodeURLNoPad decodes a base64url-encoded string without padding.
func Base64DecodeURLNoPad(input string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
