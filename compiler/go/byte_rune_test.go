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
			name: "decode byte and rune dynamics",
			input: `use ard/decode
fn main() Bool {
  let b = Byte::from_int(7).expect("byte")
  let r = Rune::from_str("é").expect("rune")
  decode::byte(b.to_dyn()).expect("byte").to_int() == 7 and decode::rune(r.to_dyn()).expect("rune").to_str() == "é" and decode::int(b.to_dyn()).expect("int") == 7
}`,
			want: "true",
		},
		{
			name: "json encodes bytes as base64",
			input: `use ard/json
fn main() Bool {
  json::encode("hi".bytes()).expect("json") == "\"aGk=\""
}`,
			want: "true",
		},
		{
			name: "json parses byte and rune numbers",
			input: `use ard/json
fn main() Bool {
  let b = json::parse<Byte>("255").expect("byte")
  let r = json::parse<Rune>("233").expect("rune")
  b.to_int() == 255 and r.to_str() == "é" and json::parse<Byte>("256").is_err()
}`,
			want: "true",
		},
		{
			name: "json parses bytes from base64",
			input: `use ard/json
fn main() Bool {
  let bytes = json::parse<[Byte]>("\"aGk=\"").expect("json")
  Str::from_bytes(bytes).expect("utf8") == "hi"
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
