package gotarget

import "testing"

// #283: Str::from([Byte]) and Str::from([Rune]) build a Str, mirroring Go's
// string([]byte) / string([]rune). The byte form is unchecked.
func TestGoTargetStrFrom(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "from bytes round-trips",
			input: `fn main() Bool {
  Str::from("hé".bytes()) == "hé"
}`,
			want: "true",
		},
		{
			name: "from runes round-trips",
			input: `fn main() Bool {
  Str::from("hé".runes()) == "hé"
}`,
			want: "true",
		},
		{
			name: "empty bytes build an empty string",
			input: `fn main() Bool {
  let empty: [Byte] = []
  Str::from(empty) == ""
}`,
			want: "true",
		},
		{
			name: "invalid utf-8 bytes are carried through unchecked",
			input: `fn main() Int {
  mut partial: [Byte] = []
  partial.push("é".bytes().at(0).expect("first byte"))
  Str::from(partial).size()
}`,
			want: "1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := lowerParitySource(t, tc.input)
			if got := runGoTargetParityJSON(t, program); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
