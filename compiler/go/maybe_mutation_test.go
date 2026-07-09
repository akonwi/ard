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
  let a: Maybe<Int> = Maybe::new(40)
  let b: Int? = Maybe::new<Int>()
  a.or(0) == 40 and b.is_none()
}`,
			want: "true",
		},
		{
			name: "new preserves existing maybe values",
			input: `fn main() Bool {
  let none: Int? = Maybe::new<Int>()
  let a: Int? = Maybe::new(none)
  let some: Int? = Maybe::new(7)
  let b: Int? = Maybe::new(some)
  a.is_none() and b.or(0) == 7
}`,
			want: "true",
		},
		{
			name: "set and clear mutate in place",
			input: `fn main() Bool {
  mut m = Maybe::new<Int>()
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
