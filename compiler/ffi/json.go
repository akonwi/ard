package ffi

import (
	"encoding/json/v2"
)

// JsonEncode marshals an Ard value to a JSON string.
func JsonEncode(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
