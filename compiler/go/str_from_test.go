package gotarget

import "testing"

// #283: Str::from([Byte]) Str? validates UTF-8 at the boundary,
// returning some(Str) for valid bytes and none for invalid sequences.
func TestGoTargetStrFrom(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "valid bytes round-trip to some",
			input: `fn main() Bool {
  let round = Str::from("hé".bytes())
  round.is_some() and round.or("") == "hé"
}`,
			want: "true",
		},
		{
			name: "empty bytes are valid",
			input: `fn main() Bool {
  let empty: [Byte] = []
  Str::from(empty).or("x") == ""
}`,
			want: "true",
		},
		{
			name: "invalid utf-8 returns none",
			input: `fn main() Bool {
  mut partial: [Byte] = []
  partial.push("é".bytes().at(0).expect("first byte"))
  Str::from(partial).is_none()
}`,
			want: "true",
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
