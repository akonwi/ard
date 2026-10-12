package formatter

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestFormatIsIdempotent(t *testing.T) {
	inputs := []struct {
		name  string
		input string
	}{
		{
			name:  "try catch block",
			input: "fn example() {\n  let raw = try self.raw -> _ {\n    \"\"\n  }\n  next(raw)\n}\n",
		},
		{
			name:  "test function",
			input: "test fn my_test() Void!Str {\n  try testing::assert(true, \"ok\")\n  Result::ok(())\n}\n",
		},
		{
			name:  "fixed array type",
			input: "let bytes: [Byte; 3] = [1, 2, 3]\nlet maybe: [Byte; 0]? = Maybe::new()\n",
		},
		{
			name:  "test and regular functions together",
			input: "fn helper() Int {\n  1\n}\ntest fn test_helper() Void!Str {\n  Result::ok(())\n}\n",
		},
		{
			name:  "anonymous function with inferred parameter type",
			input: "fn main() {\n  let adults = List::keep(\n    users,\n    fn(u) {\n      u.age >= 30\n    },\n  )\n}\n",
		},
		{
			name:  "go import",
			input: "use go:fmt\n\nfn main() {\n  fmt::Println(\"hello\")\n}\n",
		},
		{
			name:  "variadic callable type",
			input: "fn invoke(call: fn(Str,...Str) Str) Str {\n  call(\"a\", \"b\")\n}\n",
		},
	}

	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			first, err := Format([]byte(tt.input), "test.ard")
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
		})
	}
}

func TestFormatHugsTrailingClosures(t *testing.T) {
	boundaryArgument := strings.Repeat("x", 88)
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "route handler stays on opening line",
			input: "fn register(router: mut App) {\n  router.get(\n    \"/fixtures/:id/outlook\",\n    fn(r: Request, w: mut Response) {\n      handle(r, w)\n    },\n  )\n}\n",
			want:  "fn register(router: mut App) {\n  router.get(\"/fixtures/:id/outlook\", fn(r: Request, w: mut Response) {\n    handle(r, w)\n  })\n}\n",
		},
		{
			name:  "long opening line uses expanded call",
			input: "fn register(router: mut VeryLongApplicationRouterName) {\n  router.register_route_with_a_very_long_method_name(\n    \"/fixtures/:fixture_identifier/outlook-and-detailed-analysis\",\n    fn(request: VeryLongRequestType, response: mut VeryLongResponseType) {\n      handle(request, response)\n    },\n  )\n}\n",
			want:  "fn register(router: mut VeryLongApplicationRouterName) {\n  router.register_route_with_a_very_long_method_name(\n    \"/fixtures/:fixture_identifier/outlook-and-detailed-analysis\",\n    fn(request: VeryLongRequestType, response: mut VeryLongResponseType) {\n      handle(request, response)\n    },\n  )\n}\n",
		},
		{
			name:  "outer indentation counts toward line width",
			input: "fn main() {\n  f(\"" + boundaryArgument + "\", fn() {\n    work()\n  })\n}\n",
			want:  "fn main() {\n  f(\n    \"" + boundaryArgument + "\",\n    fn() {\n      work()\n    },\n  )\n}\n",
		},
		{
			name:  "multiline preceding argument uses expanded call",
			input: "fn configure(app: mut App) {\n  app.route(\n    match path {\n      some(value) => value,\n      _ => \"/\",\n    },\n    fn(request: Request) {\n      handle(request)\n    },\n  )\n}\n",
			want:  "fn configure(app: mut App) {\n  app.route(\n    match path {\n      some(value) => value,\n      _ => \"/\",\n    },\n    fn(request: Request) {\n      handle(request)\n    },\n  )\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Format([]byte(tt.input), "test.ard")
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("format mismatch:\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestFormatStructLiteralShorthand(t *testing.T) {
	input := "fn make(name: Str, age: Int) Person {\n  Person{ name, age, employed: employed }\n}\n"
	want := "fn make(name: Str, age: Int) Person {\n  Person{\n    name,\n    age,\n    employed: employed,\n  }\n}\n"

	got, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("format mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
	gotAgain, err := Format(got, "test.ard")
	if err != nil {
		t.Fatal(err)
	}
	if string(gotAgain) != want {
		t.Fatalf("second format mismatch:\ngot:\n%s\nwant:\n%s", gotAgain, want)
	}
}

func TestFormatGenericStructLiterals(t *testing.T) {
	inputs := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "one-line generic struct literal keeps type args",
			input: "fn main() {\n  let r = Radio<Str>{value: \"compact\"}\n}\n",
			want:  "fn main() {\n  let r = Radio<Str>{value: \"compact\"}\n}\n",
		},
		{
			name:  "empty generic struct literal keeps type args",
			input: "fn main() {\n  let b = Box<Int>{}\n}\n",
			want:  "fn main() {\n  let b = Box<Int>{}\n}\n",
		},
		{
			name:  "multi-line generic struct literal keeps type args",
			input: "fn main() {\n  let r = Radio<Str>{value: \"compact\", group: mode, label: \"Compact\", disabled: false}\n}\n",
			want:  "fn main() {\n  let r = Radio<Str>{\n    value: \"compact\",\n    group: mode,\n    label: \"Compact\",\n    disabled: false,\n  }\n}\n",
		},
		{
			name:  "static generic struct literal keeps type args",
			input: "fn main() {\n  let p = ui::Provider<ui::Theme>{value: active}\n}\n",
			want:  "fn main() {\n  let p = ui::Provider<ui::Theme>{value: active}\n}\n",
		},
	}

	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Format([]byte(tt.input), "test.ard")
			if err != nil {
				t.Fatalf("format failed: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("format mismatch:\ngot:\n%s\nwant:\n%s", got, tt.want)
			}

			again, err := Format(got, "test.ard")
			if err != nil {
				t.Fatalf("second format failed: %v", err)
			}
			if string(again) != string(got) {
				t.Fatalf("format not idempotent:\nfirst:\n%s\nsecond:\n%s", got, again)
			}
		})
	}
}

func TestUnusedImportRemovalKeepsTypeArgUses(t *testing.T) {
	inputs := []struct {
		name  string
		input string
	}{
		{
			name:  "module used only in struct literal type args",
			input: "use go:example.com/ui\n\nstruct Box<$T> {\n  value: $T,\n}\n\nfn f() {\n  let b = Box<ui::Theme>{}\n}\n",
		},
		{
			name:  "module used only in static call type args",
			input: "use ard/unsafe\nuse go:example.com/ui\n\nfn f(v: Any) {\n  let x = unsafe::cast<ui::Theme>(v)\n}\n",
		},
		{
			name:  "module used only in function call type args",
			input: "use go:example.com/ui\n\nfn f(v: Any) {\n  let x = identity<ui::Theme>(v)\n}\n",
		},
		{
			name:  "module used only in method call type args",
			input: "use go:example.com/ui\n\nfn f(v: Holder) {\n  let x = v.pick<ui::Theme>()\n}\n",
		},
		{
			name:  "unused import is still removed alongside type arg use",
			input: "use go:example.com/ui\nuse go:example.com/unused\n\nstruct Box<$T> {\n  value: $T,\n}\n\nfn f() {\n  let b = Box<ui::Theme>{}\n}\n",
		},
	}

	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Format([]byte(tt.input), "test.ard")
			if err != nil {
				t.Fatalf("format failed: %v", err)
			}
			if !strings.Contains(string(got), "use go:example.com/ui") {
				t.Fatalf("expected import to be preserved, got:\n%s", got)
			}
			if strings.Contains(string(got), "use go:example.com/unused") {
				t.Fatalf("expected unused import to be removed, got:\n%s", got)
			}
		})
	}
}

func TestFormatMutRefExpressions(t *testing.T) {
	input := "mut counter = 0\nlet r = mut counter\nbump(mut counter)\nlet fresh = mut Point{x: 1}\n"
	formatted, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	want := "mut counter = 0\nlet r = mut counter\nbump(mut counter)\nlet fresh = mut Point{x: 1}\n"
	if string(formatted) != want {
		t.Fatalf("formatted = %q, want %q", string(formatted), want)
	}
}

func TestFormatParenthesizesMutRefExpressionStatements(t *testing.T) {
	input := `struct Holder {
  value: Int,
}

fn make() mut Holder {
  (mut Holder{value: 1})
}
`
	formatted, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if string(formatted) != input {
		t.Fatalf("formatted:\n%s\nwant:\n%s", formatted, input)
	}
	if result := parse.Parse(formatted, "test.ard"); len(result.Errors) > 0 {
		t.Fatalf("formatted output does not parse: %v", result.Errors)
	}
	again, err := Format(formatted, "test.ard")
	if err != nil {
		t.Fatalf("second format: %v", err)
	}
	if string(again) != input {
		t.Fatalf("formatter is not idempotent:\n%s", again)
	}
}

func TestFormatStringBraceEscapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "literal braces", input: "let text = \"{{value}}\"\n", want: "let text = \"{{value}}\"\n"},
		{name: "literal braces around interpolation", input: "let text = \"{{{value}}}\"\n", want: "let text = \"{{{value}}}\"\n"},
		{name: "legacy backslash escapes", input: "let text = \"\\{value\\}\"\n", want: "let text = \"{{value}}\"\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted, err := Format([]byte(tt.input), "test.ard")
			if err != nil {
				t.Fatalf("format: %v", err)
			}
			if string(formatted) != tt.want {
				t.Fatalf("formatted = %q, want %q", string(formatted), tt.want)
			}
		})
	}
}

func TestFormatQualifiedMaybeInsideResultType(t *testing.T) {
	input := "use foo/bar\n\nprivate fn load() bar::Item?!Str {}\n"
	formatted, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if string(formatted) != input {
		t.Fatalf("formatted = %q, want unchanged %q", string(formatted), input)
	}
}

func TestFormatPreservesParenthesesAroundTryPostfixTarget(t *testing.T) {
	input := "fn lookup() Int?!Str {\n  Result::ok(Maybe::new(1))\n}\n\nfn check() Void!Str {\n  let value = (try lookup()).expect(\"value\")\n  let present = (try lookup()).is_some()\n  Result::ok(())\n}\n"
	formatted, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if string(formatted) != input {
		t.Fatalf("formatted = %q, want unchanged %q", string(formatted), input)
	}
}

// Comparison operators are non-associative: the parser folds `a op b op c`
// into a single ChainedComparison, and the checker rejects chains containing
// == or !=. The formatter must keep parentheses around comparison operands of
// comparisons so formatting never produces such a chain (#519).
func TestFormatPreservesParenthesesAroundComparisonOperands(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "relational operands of not-equal",
			input: "let x = (a > 0) != (b > 0)\n",
			want:  "let x = (a > 0) != (b > 0)\n",
		},
		{
			name:  "equality operands of not-equal",
			input: "let x = (a == 0) != (b == 0)\n",
			want:  "let x = (a == 0) != (b == 0)\n",
		},
		{
			name:  "relational left operand of equal",
			input: "let x = (a + 1 > b) == true\n",
			want:  "let x = (a + 1 > b) == true\n",
		},
		{
			name:  "chained comparison right operand of equal",
			input: "let x = ok == (0 < n < 10)\n",
			want:  "let x = ok == (0 < n < 10)\n",
		},
		{
			name:  "chained comparison left operand of equal",
			input: "let x = (0 < n < 10) == ok\n",
			want:  "let x = (0 < n < 10) == ok\n",
		},
		{
			name:  "chained comparison stays bare under and",
			input: "let x = (0 < n < 10) and ok\n",
			want:  "let x = 0 < n < 10 and ok\n",
		},
		{
			name:  "comparison operands under and stay bare",
			input: "let x = (a > 0) and (b > 0)\n",
			want:  "let x = a > 0 and b > 0\n",
		},
		{
			name:  "higher precedence operand of comparison stays bare",
			input: "let x = (a + 1) > b\n",
			want:  "let x = a + 1 > b\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted, err := Format([]byte(tt.input), "test.ard")
			if err != nil {
				t.Fatalf("format: %v", err)
			}
			if string(formatted) != tt.want {
				t.Fatalf("formatted = %q, want %q", string(formatted), tt.want)
			}
			again, err := Format(formatted, "test.ard")
			if err != nil {
				t.Fatalf("second format: %v", err)
			}
			if string(again) != tt.want {
				t.Fatalf("formatter is not idempotent: %q", string(again))
			}
		})
	}
}

// `%` shares precedence and left associativity with `*` and `/`, so the
// formatter only drops parentheses the parser does not need. (#530)
func TestFormatModuloPrecedence(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "modulo operand of addition stays bare",
			input: "let x = (7 % 3) + 1\n",
			want:  "let x = 7 % 3 + 1\n",
		},
		{
			name:  "addition operand of modulo keeps parentheses",
			input: "let x = 7 % (3 + 1)\n",
			want:  "let x = 7 % (3 + 1)\n",
		},
		{
			name:  "right operand of modulo keeps parentheses for same precedence",
			input: "let x = 7 % (3 * 2)\n",
			want:  "let x = 7 % (3 * 2)\n",
		},
		{
			name:  "chained modulo stays left associative",
			input: "let x = (20 % 7) % 4\n",
			want:  "let x = 20 % 7 % 4\n",
		},
		{
			name:  "wrap-around modulo",
			input: "let x = ((a + d) % c + c) % c\n",
			want:  "let x = ((a + d) % c + c) % c\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted, err := Format([]byte(tt.input), "test.ard")
			if err != nil {
				t.Fatalf("format: %v", err)
			}
			if string(formatted) != tt.want {
				t.Fatalf("formatted = %q, want %q", string(formatted), tt.want)
			}
			again, err := Format(formatted, "test.ard")
			if err != nil {
				t.Fatalf("second format: %v", err)
			}
			if string(again) != tt.want {
				t.Fatalf("formatter is not idempotent: %q", string(again))
			}
		})
	}
}

func TestFormatExpandedLiteralsIsIdempotent(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "map", input: "fn example() {\n  let value: [Str: Any] = [\"identifier\": 123456789, \"email_address\": \"person@example.com\", \"display_name\": \"Example Person\"]\n  let next = value\n}\n"},
		{name: "qualified struct", input: "fn example() {\n  let value = module::Item{first: 123456789, second: 987654321, third: 456789123, fourth: 321987654}\n  let next = value\n}\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, err := Format([]byte(tt.input), "test.ard")
			if err != nil {
				t.Fatalf("first format: %v", err)
			}
			second, err := Format(first, "test.ard")
			if err != nil {
				t.Fatalf("second format: %v", err)
			}
			if string(second) != string(first) {
				t.Fatalf("format is not idempotent: first=%q second=%q", string(first), string(second))
			}
		})
	}
}

func TestFormatDefer(t *testing.T) {
	input := "fn main() {\n  defer cleanup(  value )\n  defer {\n  cleanup()\n  log(\"done\")\n}\n}\n"
	formatted, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	want := "fn main() {\n  defer cleanup(value)\n  defer {\n    cleanup()\n    log(\"done\")\n  }\n}\n"
	if string(formatted) != want {
		t.Fatalf("formatted = %q, want %q", string(formatted), want)
	}
}

func TestFormatInlineBreakMatchArms(t *testing.T) {
	input := "fn main() {\n  for i in 1..3 {\n    match i {\n      2 => break,\n      _ => (),\n    }\n    match {\n      i == 1 => break,\n      _ => (),\n    }\n  }\n}\n"
	formatted, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if string(formatted) != input {
		t.Fatalf("formatted = %q, want unchanged %q", string(formatted), input)
	}
}

func TestFormatMutRefMatchArmStaysInline(t *testing.T) {
	// #formatter: a `=> mut <expr>` arm must not be wrapped into a block
	// (`=> { mut x }`), since `mut x` cannot be a block's final expression.
	input := "struct S {\n  n: Int,\n}\n\nfn pick(cond: Bool, a: mut S, b: mut S) mut S {\n  match cond {\n    true => mut a,\n    false => mut b,\n  }\n}\n"
	formatted, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if string(formatted) != input {
		t.Fatalf("formatted = %q, want unchanged %q", string(formatted), input)
	}
	// The formatted output must still parse cleanly.
	if res := parse.Parse(formatted, "test.ard"); len(res.Errors) > 0 {
		t.Fatalf("formatted output does not re-parse: %v", res.Errors)
	}
}

func TestFormatBlankLinesHaveNoTrailingWhitespace(t *testing.T) {
	// #497: blank lines inside indented blocks must be truly empty, not
	// indentation-only lines that `git diff --check` rejects.
	input := "fn main() {\n  let first = 1\n\n  let second = 2\n  if first == 1 {\n    let third = 3\n\n    let fourth = 4\n  }\n}\n"
	formatted, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if string(formatted) != input {
		t.Fatalf("formatted = %q, want unchanged %q", string(formatted), input)
	}
	for i, line := range strings.Split(string(formatted), "\n") {
		if strings.TrimSpace(line) == "" && line != "" {
			t.Fatalf("line %d is whitespace-only: %q", i+1, line)
		}
	}
}

// A comment on its own line before a match, conditional match, or select arm
// stays attached to that arm instead of moving above the first arm (#543).
func TestFormatKeepsCommentsWithTheirArms(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "match arms keep their leading comments",
			input: `fn describe(count: Int) Str {
  match count {
    // Nothing yet.
    0 => "none",
    1 => "one",
    // Two is a pair.
    2 => "pair",
    // Everything else.
    _ => "many",
  }
}
`,
		},
		{
			name: "bool match keeps the comment on the second arm",
			input: `fn describe(flag: Bool) Str {
  match flag {
    true => "yes",
    // Otherwise no.
    false => "no",
  }
}
`,
		},
		{
			name: "comments after the last arm stay at the end",
			input: `fn describe(count: Int) Str {
  match count {
    0 => "none",
    _ => "many",
    // More cases later.
  }
}
`,
		},
		{
			name: "block arm comments stay between arms",
			input: `fn describe(count: Int) Str {
  match count {
    0 => {
      let label = "none"
      label
    },
    // Many has a block too.
    _ => {
      let label = "many"
      label
    },
  }
}
`,
		},
		{
			name: "trailing same-line comments move below their arm",
			input: `fn describe(count: Int) Str {
  match count {
    0 => "none", // zero
    _ => "many",
  }
}
`,
			want: `fn describe(count: Int) Str {
  match count {
    0 => "none",
    // zero
    _ => "many",
  }
}
`,
		},
		{
			name: "conditional match arms keep their leading comments",
			input: `fn describe(count: Int) Str {
  match {
    count == 0 => "none",
    // Small counts.
    count < 3 => "few",
    // Everything else.
    _ => "many",
  }
}
`,
		},
		{
			name: "select arms keep their leading comments",
			input: `fn wait(first: Chan<Int>, second: Chan<Int>) Int? {
  select {
    let value = first.recv() => value,
    // The second channel wins ties.
    let value = second.recv() => value,
  }
}
`,
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
				t.Fatalf("formatted =\n%s\nwant\n%s", formatted, want)
			}
			again, err := Format(formatted, "test.ard")
			if err != nil {
				t.Fatalf("reformat: %v", err)
			}
			if string(again) != string(formatted) {
				t.Fatalf("formatting is not idempotent:\n%s", again)
			}
		})
	}
}

// Block-like expressions (match, select, conditional match, blocks) used as an
// operand keep their parentheses: without them the operator's operand parses
// as a struct literal or not at all (#546).
func TestFormatKeepsParenthesesAroundBlockLikeOperands(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "match on the right of or",
			input: `fn matches(target: Int, focus: Int?) Bool {
  target == 1 or (match focus {
    id => id == target,
    _ => false,
  })
}
`,
		},
		{
			name: "match on the right of and",
			input: `fn matches(target: Int, focus: Int?) Bool {
  target == 1 and (match focus {
    id => id == target,
    _ => false,
  })
}
`,
		},
		{
			name: "match on the left of or",
			input: `fn matches(flag: Bool, focus: Int?) Bool {
  (match focus {
    id => id == 1,
    _ => false,
  }) or flag
}
`,
		},
		{
			name: "match as a comparison operand",
			input: `fn same(target: Int, focus: Int?) Bool {
  target == (match focus {
    id => id,
    _ => 0,
  })
}
`,
		},
		{
			name: "conditional match as an arithmetic operand",
			input: `fn bump(count: Int) Int {
  count + (match {
    count > 1 => 1,
    _ => 0,
  })
}
`,
		},
		{
			name: "match under unary minus",
			input: `fn negate(focus: Int?) Int {
  -(match focus {
    id => id,
    _ => 0,
  })
}
`,
		},
		{
			name: "match under try",
			input: `fn pick(focus: Int?) Int!Str {
  let value = try (match focus {
    id => Result::ok(id),
    _ => Result::err("none"),
  })
  Result::ok(value)
}
`,
		},
		{
			name: "select as an operand",
			input: `fn ready(flag: Bool, signals: Chan<Bool>) Bool {
  flag or (select {
    let value = signals.recv() => value.or(false),
  })
}
`,
		},
		{
			name: "block expression as an operand",
			input: `fn total(count: Int) Int {
  count + ({
    let extra = 1
    extra
  })
}
`,
		},
		{
			name: "match as a method receiver",
			input: `fn label(focus: Int?) Str {
  (match focus {
    id => id,
    _ => 0,
  }).to_str()
}
`,
		},
		{
			name: "immediately called anonymous function",
			input: `fn answer() Int {
  (fn() Int {
    1
  })()
}
`,
		},
		{
			name: "dereferenced match keeps single parentheses",
			input: `fn read(flag: Bool, a: &Int, b: &Int) Int {
  (match flag {
    true => a,
    false => b,
  }).*
}
`,
		},
		{
			name: "not needs no parentheses around a match",
			input: `fn missing(focus: Int?) Bool {
  not (match focus {
    id => id == 1,
    _ => false,
  })
}
`,
			want: `fn missing(focus: Int?) Bool {
  not match focus {
    id => id == 1,
    _ => false,
  }
}
`,
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
				t.Fatalf("formatted =\n%s\nwant\n%s", formatted, want)
			}
			if res := parse.Parse(formatted, "test.ard"); len(res.Errors) > 0 {
				t.Fatalf("formatted output does not parse: %v", res.Errors)
			}
			again, err := Format(formatted, "test.ard")
			if err != nil {
				t.Fatalf("reformat: %v", err)
			}
			if string(again) != string(formatted) {
				t.Fatalf("formatting is not idempotent:\n%s", again)
			}
		})
	}
}

// Format never returns source that no longer parses: a printer bug becomes a
// format error instead of silently corrupting the file (#546).
func TestFormattedOutputMustParse(t *testing.T) {
	broken := []byte("fn matches(target: Int, focus: Int?) Bool {\n  target == 1 or match focus {\n    id => id == target,\n    _ => false,\n  }\n}\n")
	err := checkFormattedOutputParses(broken, "repro.ard")
	if err == nil {
		t.Fatal("unparseable output was accepted")
	}
	for _, want := range []string{"repro.ard", "would not parse", "bug"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to mention %q", err, want)
		}
	}
	if err := checkFormattedOutputParses([]byte("fn main() {}\n"), "ok.ard"); err != nil {
		t.Fatalf("valid output rejected: %v", err)
	}
}
