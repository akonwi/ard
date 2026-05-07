package vm

import "testing"

func TestVMStdlibCryptoHashes(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "md5 hashes hello",
			input: `
				use ard/crypto
				crypto::md5("hello")
			`,
			want: "5d41402abc4b2a76b9719d911017c592",
		},
		{
			name: "sha256 returns raw bytes",
			input: `
				use ard/crypto
				crypto::sha256("").size()
			`,
			want: 32,
		},
		{
			name: "sha256 can be hex encoded",
			input: `
				use ard/crypto
				use ard/hex
				hex::encode(crypto::sha256(""))
			`,
			want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name: "sha512 returns raw bytes",
			input: `
				use ard/crypto
				crypto::sha512("hello").size()
			`,
			want: 64,
		},
	})
}

func TestVMStdlibCryptoPasswordHashing(t *testing.T) {
	got := runSourceGoValue(t, `
		use ard/crypto

		fn check() Bool!Str {
			let hashed = try crypto::hash("password123", 4)
			let verified = try crypto::verify("password123", hashed)
			let wrong = try crypto::verify("wrong-password", hashed)

			Result::ok(verified and not wrong and crypto::hash("password123", 32).is_err())
		}

		check().expect("crypto password check failed")
	`)

	if got != true {
		t.Fatalf("got %#v, want password hashing checks to pass", got)
	}
}

func TestVMStdlibCryptoScrypt(t *testing.T) {
	got := runSourceGoValue(t, `
		use ard/crypto

		fn check() Bool!Str {
			let hashed = try crypto::scrypt_hash("password", "73616c74", 16, 1, 1, 16)
			let verified = try crypto::scrypt_verify("password", hashed, 16, 1, 1, 16)

			let deterministic = hashed == "73616c74:d360147c2a2db7903186e387bb385547"
			let malformed = crypto::scrypt_verify("password123", "bad-hash").is_err()

			Result::ok(deterministic and verified and malformed)
		}

		check().expect("scrypt check failed")
	`)

	if got != true {
		t.Fatalf("got %#v, want scrypt checks to pass", got)
	}
}
