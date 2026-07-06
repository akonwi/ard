package gotarget

import "testing"

func TestGoTargetParityByteRunePrimitives(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "str at returns rune",
			input: `fn main() Int {
  "hé".at(1).expect("rune").to_int()
}`,
			want: "233",
		},
		{
			name: "str byte and rune views",
			input: `fn main() Bool {
  "hé".bytes().size() == 3 and "hé".runes().size() == 2
}`,
			want: "true",
		},
		{
			name: "direct str iteration yields runes",
			input: `fn main() Int {
  mut total = 0
  for r in "hé" {
    total = total + r.to_int()
  }
  total
}`,
			want: "337",
		},
		{
			name: "rune literal comparisons and match",
			input: `fn main() Bool {
  let newline: Rune = '\n'
  let matched = match '/' {
    '/' => true,
    _ => false,
  }
  mut saw_slash = false
  for ch in "a/b" {
    if ch == '/' {
      saw_slash = true
    }
  }
  newline.to_int() == 10 and matched and saw_slash
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
