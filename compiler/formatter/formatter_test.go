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
			name:   "keeps self method calls with dot",
			input:  "struct Fiber {\n  result: Int\n}\nimpl Fiber {\n  fn join() {}\n  fn get() Int {\n    self.join()\n    self.result\n  }\n}\n",
			output: "struct Fiber {\n  result: Int,\n}\nimpl Fiber {\n  fn join() {}\n\n  fn get() Int {\n    self.join()\n    self.result\n  }\n}\n",
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
			name:   "keeps small struct literals on one line",
			input:  "fn main() {\n  let p = Point{ x: 1, y: 2 }\n}\n",
			output: "fn main() {\n  let p = Point{x: 1, y: 2}\n}\n",
		},
		{
			name:   "formats struct literals with 3 properties across lines",
			input:  "fn main() {\n  let u = User{ name: \"a\", age: 1, role: \"admin\" }\n}\n",
			output: "fn main() {\n  let u = User{\n    name: \"a\",\n    age: 1,\n    role: \"admin\",\n  }\n}\n",
		},
		{
			name:   "keeps impl comments attached to following method",
			input:  "impl Database {\n  fn close() {\n    close_db(self._ptr)\n  }\n\n  // simple one-off executions where the results aren't needed\n  // [note]: could be removed entirely for query.run() once optional params are sorted\n  fn exec(sql: Str) {\n    execute(self._ptr, sql)\n  }\n}\n",
			output: "impl Database {\n  fn close() {\n    close_db(self._ptr)\n  }\n\n  // simple one-off executions where the results aren't needed\n  // [note]: could be removed entirely for query.run() once optional params are sorted\n  fn exec(sql: Str) {\n    execute(self._ptr, sql)\n  }\n}\n",
		},
		{
			name:   "does not insert blank lines between top-level statements",
			input:  "fn one() {}\nfn two() {}\n",
			output: "fn one() {}\nfn two() {}\n",
		},
		{
			name:   "no synthetic blank line after multiline declaration expression",
			input:  "fn example() {\n  let raw = try self.raw -> _ {\n    let x = \"\"\n    x\n  }\n  next(raw)\n}\n",
			output: "fn example() {\n  let raw = try self.raw -> _ {\n    let x = \"\"\n    x\n  }\n  next(raw)\n}\n",
		},
		{
			name:   "formats if else chain with stable braces",
			input:  "fn main() {\n  if a{b}else if c{d}else{e}\n}\n",
			output: "fn main() {\n  if a {\n    b\n  } else if c {\n    d\n  } else {\n    e\n  }\n}\n",
		},
		{
			name:   "keeps single-expression match block inline when it fits",
			input:  "fn main() {\n  match x {\n    true => { total =+ 1 },\n    false => { total =- 1 },\n  }\n}\n",
			output: "fn main() {\n  match x {\n    true => { total =+ 1 },\n    false => { total =- 1 },\n  }\n}\n",
		},
		{
			name:   "keeps single-expression try catch block inline when it fits",
			input:  "fn main() {\n  let raw = try self.raw -> _ {\n    \"\"\n  }\n}\n",
			output: "fn main() {\n  let raw = try self.raw -> _ { \"\" }\n}\n",
		},
		{
			name:   "formats for loop header spacing",
			input:  "fn main() {\n  for mut i=0;i<10;i=+1{ i }\n}\n",
			output: "fn main() {\n  for mut i = 0; i < 10; i =+ 1 {\n    i\n  }\n}\n",
		},
		{
			name:   "preserves blank line after type declaration",
			input:  "type Value = Str | Int\n\nfn main() {}\n",
			output: "type Value = Str | Int\n\nfn main() {}\n",
		},
		{
			name:   "preserves blank line between enum and impl",
			input:  "enum Method {\n  Get\n}\n\nimpl Str::ToString for Method {\n  fn to_str() Str {\n    \"GET\"\n  }\n}\n",
			output: "enum Method {\n  Get,\n}\n\nimpl Str::ToString for Method {\n  fn to_str() Str {\n    \"GET\"\n  }\n}\n",
		},
		{
			name:   "preserves blank line before struct comment group",
			input:  "struct Request {\n  body: Dynamic?\n\n  // inbound requests have the *http.Request\n  raw: Dynamic?\n}\n",
			output: "struct Request {\n  body: Dynamic?,\n\n  // inbound requests have the *http.Request\n  raw: Dynamic?,\n}\n",
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

func TestFormatIsIdempotent(t *testing.T) {
	input := "fn example() {\n  let raw = try self.raw -> _ {\n    \"\"\n  }\n  next(raw)\n}\n"

	first, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatalf("first format failed: %v", err)
	}

	second, err := Format(first, "test.ard")
	if err != nil {
		t.Fatalf("second format failed: %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("expected idempotent formatting, first=%q second=%q", string(first), string(second))
	}
}
