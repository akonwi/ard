package vm

import (
	"regexp"
	"testing"
)

func TestBytecodeCryptoUUID(t *testing.T) {
	out := runBytecode(t, `
		use ard/crypto
		crypto::uuid()
	`)

	uuid, ok := out.(string)
	if !ok {
		t.Fatalf("Expected UUID string, got %T", out)
	}

	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(uuid) {
		t.Fatalf("Expected valid UUID v4 format, got %q", uuid)
	}
}
