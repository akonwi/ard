package vm

import (
	"regexp"
	"testing"
)

func TestBytecodeCryptoHashes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "md5 hashes hello",
			input: `
				use ard/crypto
				crypto::md5("hello")
			`,
			want: "5d41402abc4b2a76b9719d911017c592",
		},
		{
			name: "sha256 hashes hello",
			input: `
				use ard/crypto
				crypto::sha256("hello")
			`,
			want: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		},
		{
			name: "sha512 hashes hello",
			input: `
				use ard/crypto
				crypto::sha512("hello")
			`,
			want: "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043",
		},
		{
			name: "md5 hashes empty string",
			input: `
				use ard/crypto
				crypto::md5("")
			`,
			want: "d41d8cd98f00b204e9800998ecf8427e",
		},
		{
			name: "sha256 hashes empty string",
			input: `
				use ard/crypto
				crypto::sha256("")
			`,
			want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name: "sha512 hashes empty string",
			input: `
				use ard/crypto
				crypto::sha512("")
			`,
			want: "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBytecodeCryptoPasswordHashing(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "hash and verify with default cost",
			input: `
				use ard/crypto
				let hashed = crypto::hash("password123").expect("hash failed")
				crypto::verify("password123", hashed).expect("verify failed")
			`,
			want: true,
		},
		{
			name: "verify returns false for wrong password",
			input: `
				use ard/crypto
				let hashed = crypto::hash("password123").expect("hash failed")
				crypto::verify("wrong-password", hashed).expect("verify failed")
			`,
			want: false,
		},
		{
			name: "hashes include salt",
			input: `
				use ard/crypto
				let first = crypto::hash("password123").expect("first hash failed")
				let second = crypto::hash("password123").expect("second hash failed")
				not first == second
			`,
			want: true,
		},
		{
			name: "hash supports explicit cost",
			input: `
				use ard/crypto
				let hashed = crypto::hash("password123", 4).expect("hash failed")
				crypto::verify("password123", hashed).expect("verify failed")
			`,
			want: true,
		},
		{
			name: "hash returns err for invalid cost",
			input: `
				use ard/crypto
				crypto::hash("password123", 32).is_err()
			`,
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}

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
