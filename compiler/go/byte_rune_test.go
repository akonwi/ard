package gotarget

import "testing"

func TestGoTargetParityByteRunePrimitives(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "byte from int and to int",
			input: `fn main() Int {
  Byte::from_int(255).expect("byte").to_int()
}`,
			want: "255",
		},
		{
			name: "byte rejects out of range",
			input: `fn main() Bool {
  Byte::from_int(256).is_none()
}`,
			want: "true",
		},
		{
			name: "rune from int and to str",
			input: `fn main() Str {
  Rune::from_int(233).expect("rune").to_str()
}`,
			want: `"é"`,
		},
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
			name: "byte relative comparison",
			input: `fn main() Bool {
  let a = Byte::from_int(1).expect("a")
  let b = Byte::from_int(2).expect("b")
  a < b
}`,
			want: "true",
		},
		{
			name: "rune relative comparison",
			input: `fn main() Bool {
  let a = Rune::from_str("a").expect("a")
  let b = Rune::from_str("b").expect("b")
  a < b
}`,
			want: "true",
		},
		{
			name: "rune from str rejects empty and multi rune strings",
			input: `fn main() Bool {
  Rune::from_str("").is_none() and Rune::from_str("ab").is_none()
}`,
			want: "true",
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
		{
			name: "decode byte and rune anys",
			input: `use ard/decode
use ard/any as Any
fn main() Bool {
  let b = Byte::from_int(7).expect("byte")
  let r = Rune::from_str("é").expect("rune")
  decode::byte(Any::from(b)).expect("byte").to_int() == 7 and decode::rune(Any::from(r)).expect("rune").to_str() == "é" and decode::int(Any::from(b)).expect("int") == 7
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
