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

// Base64EncodeURL encodes the input using base64url (URL-safe alphabet).
// By default the output includes '=' padding. Pass noPad=true to opt out of
// padding (required for JWTs and PKCE code challenges).
func Base64EncodeURL(input string, noPad *bool) string {
	if noPad != nil && *noPad {
		return base64.RawURLEncoding.EncodeToString([]byte(input))
	}
	return base64.URLEncoding.EncodeToString([]byte(input))
}

// Base64DecodeURL decodes a base64url-encoded string. By default it expects
// '=' padding; pass noPad=true to decode input that has no padding.
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
