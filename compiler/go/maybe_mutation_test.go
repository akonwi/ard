package gotarget

import "testing"

func TestGoTargetMaybePreludeAndMutation(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "Maybe constructors need no import",
			input: `fn main() Bool {
  let a: Maybe<Int> = Maybe::some(40)
  let b: Int? = Maybe::none<Int>()
  a.or(0) == 40 and b.is_none()
}`,
			want: "true",
		},
		{
			name: "set and clear mutate in place",
			input: `fn main() Bool {
  mut m = Maybe::none<Int>()
  m.set(42)
  let after_set = m.or(0)
  m.clear()
  after_set == 42 and m.is_none()
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
