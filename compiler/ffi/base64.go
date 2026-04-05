package ffi

import (
	"encoding/base64"
)

// Base64Encode encodes the input using standard base64 (alphabet: A-Z a-z 0-9 + /).
// Output includes '=' padding by default. Pass noPad=true to strip padding.
func Base64Encode(input string, noPad *bool) string {
	if noPad != nil && *noPad {
		return base64.RawStdEncoding.EncodeToString([]byte(input))
	}
	return base64.StdEncoding.EncodeToString([]byte(input))
}

// Base64Decode decodes a standard base64 string. The noPad flag must match how
// the input was encoded: omit for padded input, pass true for unpadded input.
func Base64Decode(input string, noPad *bool) (string, error) {
	enc := base64.StdEncoding
	if noPad != nil && *noPad {
		enc = base64.RawStdEncoding
	}
	decoded, err := enc.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// Base64EncodeURL encodes the input using base64url (alphabet: A-Z a-z 0-9 - _).
// Output includes '=' padding by default. Pass noPad=true to strip padding
// (required by JWT headers/payloads and PKCE code challenges).
func Base64EncodeURL(input string, noPad *bool) string {
	if noPad != nil && *noPad {
		return base64.RawURLEncoding.EncodeToString([]byte(input))
	}
	return base64.URLEncoding.EncodeToString([]byte(input))
}

// Base64DecodeURL decodes a base64url string. The noPad flag must match how
// the input was encoded: omit for padded input, pass true for unpadded input.
func Base64DecodeURL(input string, noPad *bool) (string, error) {
	enc := base64.URLEncoding
	if noPad != nil && *noPad {
		enc = base64.RawURLEncoding
	}
	decoded, err := enc.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
