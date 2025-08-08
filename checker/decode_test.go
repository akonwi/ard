package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestDecodeError(t *testing.T) {
	run(t, []test{
		{
			name: "decode::Error implements Str::ToString",
			input: `
			use ard/decode
			use ard/io

			let err = decode::Error{expected: "Str", found: "1", path: ["dunno"] }
			io::print(err)
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}
