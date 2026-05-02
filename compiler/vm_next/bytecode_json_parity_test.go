package vm_next

import "testing"

func TestVMNextBytecodeParityEncodeJSONPrimitives(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "encoding Str",
			input: `
				use ard/encode
				encode::json("hello").expect("encode failed")
			`,
			want: `"hello"`,
		},
		{
			name: "encoding Int",
			input: `
				use ard/encode
				encode::json(200).expect("encode failed")
			`,
			want: `200`,
		},
		{
			name: "encoding Float",
			input: `
				use ard/encode
				encode::json(98.6).expect("encode failed")
			`,
			want: `98.6`,
		},
		{
			name: "encoding Bool",
			input: `
				use ard/encode
				encode::json(true).expect("encode failed")
			`,
			want: `true`,
		},
	})
}
