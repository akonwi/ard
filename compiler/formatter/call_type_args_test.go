package formatter

import "testing"

// Issue #639: call type arguments that are themselves generic print back
// unchanged, and the comparisons and shifts they resemble keep their meaning.
func TestFormatNestedCallTypeArguments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "nested", input: "let c = same<Box<Int>>(b)\n"},
		{name: "map", input: "let c = same<Map<Str, Int>>(b)\n"},
		{name: "two levels", input: "let c = same<Box<Box<Int>>>(b)\n"},
		{name: "three levels", input: "let c = same<Box<Box<Box<Int>>>>(b)\n"},
		{name: "several", input: "let c = pick<Box<Int>, Str?>(b, s)\n"},
		{name: "nullable and result", input: "let c = same<Box<Int>?>(b)\nlet d = same<Box<Int>!Str>(r)\n"},
		{name: "static", input: "let m = Maybe::new<Box<Int>>()\n"},
		{name: "module", input: "let c = mod::same<Box<Int>>(b)\n"},
		{name: "method", input: "let c = value.map<Box<Int>>(wrap)\n"},
		{name: "compared", input: "let ok = 1 < same<Box<Int>>(b).v < 3\n"},
		{name: "spaced comparisons", input: "let ok = a < b < c >> d\nlet ok2 = a < b > c\nlet ok3 = x < y and z > w\n"},
		{name: "shifts", input: "let s = a << b\nlet t = a >> b\n"},
		{
			name:  "unspaced comparison with shift",
			input: "let ok = a<b<c>>d\n",
			want:  "let ok = a < b < c >> d\n",
		},
		{
			name:  "unspaced comparison against nested shift call",
			input: "let ok = a<b<1>>(d)\n",
			want:  "let ok = a < b < 1 >> d\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if want == "" {
				want = tt.input
			}
			formatted, err := Format([]byte(tt.input), "test.ard")
			if err != nil {
				t.Fatalf("format: %v", err)
			}
			if string(formatted) != want {
				t.Fatalf("formatted = %q, want %q", string(formatted), want)
			}
			again, err := Format(formatted, "test.ard")
			if err != nil {
				t.Fatalf("second format: %v", err)
			}
			if string(again) != want {
				t.Fatalf("formatter is not idempotent: %q", string(again))
			}
		})
	}
}
