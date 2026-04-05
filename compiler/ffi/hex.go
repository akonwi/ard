package ffi

import (
	"encoding/hex"
)

// HexEncode encodes the input bytes (as a Str) to a lowercase hexadecimal string.
func HexEncode(input string) string {
	return hex.EncodeToString([]byte(input))
}

// HexDecode decodes a hexadecimal string into the original bytes (as a Str).
// Returns an error if the input is not valid hex.
func HexDecode(input string) (string, error) {
	decoded, err := hex.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
