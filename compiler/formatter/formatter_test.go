package formatter

import "testing"

func TestFormat(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
		error  bool
	}{
		{
			name:   "empty source",
			input:  "",
			output: "",
		},
		{
			name:   "normalizes windows line endings",
			input:  "let x = 1\r\nlet y = 2\r\n",
			output: "let x = 1\nlet y = 2\n",
		},
		{
			name:   "trims trailing spaces and tabs",
			input:  "let x = 1  \nlet y = 2\t\n",
			output: "let x = 1\nlet y = 2\n",
		},
		{
			name:   "adds trailing newline",
			input:  "let x = 1",
			output: "let x = 1\n",
		},
		{
			name:   "normalizes spacing in expressions and declarations",
			input:  "let value:Int=1+2\nif value>2{value}\n",
			output: "let value: Int = 1 + 2\nif value > 2 {\n  value\n}\n",
		},
		{
			name:   "sorts imports into groups",
			input:  "use github.com/zeta/lib\nuse ard/io\nuse github.com/alpha/lib\n",
			output: "use ard/io\n\nuse github.com/alpha/lib\nuse github.com/zeta/lib\n",
		},
		{
			name:   "wraps long function parameters one per line with trailing comma",
			input:  "fn super_long_function_name(first_name: Str, second_name: Str, third_name: Str, fourth_name: Str, fifth_name: Str, sixth_name: Str) Str { first_name }\n",
			output: "fn super_long_function_name(\n  first_name: Str,\n  second_name: Str,\n  third_name: Str,\n  fourth_name: Str,\n  fifth_name: Str,\n  sixth_name: Str,\n) Str {\n  first_name\n}\n",
		},
		{
			name:   "match cases include commas and empty block becomes unit",
			input:  "fn main() {\n  match maybe_name {\n    name => io::print(name),\n    _ => {}\n  }\n}\n",
			output: "fn main() {\n  match maybe_name {\n    name => io::print(name),\n    _ => (),\n  }\n}\n",
		},
		{
			name:   "keeps self method calls without dot",
			input:  "struct Fiber {\n  result: Int\n}\nimpl Fiber {\n  fn join() {}\n  fn get() Int {\n    @join()\n    @result\n  }\n}\n",
			output: "struct Fiber {\n  result: Int,\n}\nimpl Fiber {\n  fn join() {}\n\n  fn get() Int {\n    @join()\n    @result\n  }\n}\n",
		},
		{
			name:   "preserves formal result type syntax when written formally",
			input:  "extern fn hash(password: Str, cost: Int?) Result<Str, Str> = \"CryptoHashPassword\"\n",
			output: "extern fn hash(password: Str, cost: Int?) Result<Str, Str> = \"CryptoHashPassword\"\n",
		},
		{
			name:   "preserves mut in function type parameters",
			input:  "type Handler = fn(Request, mut Response)\n",
			output: "type Handler = fn(Request, mut Response)\n",
		},
		{
			name:   "preserves increment decrement sugar",
			input:  "fn main() {\n  mut x = 1\n  x =+ 2\n  x =- 1\n}\n",
			output: "fn main() {\n  mut x = 1\n  x =+ 2\n  x =- 1\n}\n",
		},
		{
			name:   "preserves empty map literal token",
			input:  "fn main() {\n  let m = [:]\n}\n",
			output: "fn main() {\n  let m = [:]\n}\n",
		},
		{
			name:   "does not insert blank lines between top-level statements",
			input:  "fn one() {}\nfn two() {}\n",
			output: "fn one() {}\nfn two() {}\n",
		},
		{
			name:  "fails on invalid source",
			input: "fn broken( {",
			error: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Format([]byte(tt.input), "test.ard")
			if tt.error {
				if err == nil {
					t.Fatalf("expected formatting error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if string(got) != tt.output {
				t.Fatalf("expected %q, got %q", tt.output, string(got))
			}
		})
	}
}
