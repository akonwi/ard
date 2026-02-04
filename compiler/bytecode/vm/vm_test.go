package vm

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func runBytecode(t *testing.T, input string) any {
	t.Helper()
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}
	tree := result.Program
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	resolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		t.Fatalf("Failed to init module resolver: %v", err)
	}
	c := checker.New("test.ard", tree, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Diagnostics found: %v", c.Diagnostics())
	}

	emitter := bytecode.NewEmitter()
	program, err := emitter.EmitProgram(c.Module())
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}

	vm := New(program)
	res, err := vm.Run("main")
	if err != nil {
		t.Fatalf("VM error: %v", err)
	}
	if res == nil {
		return nil
	}
	return res.GoValue()
}

func TestBytecodeEmptyProgram(t *testing.T) {
	res := runBytecode(t, "")
	if res != nil {
		t.Fatalf("Expected nil, got %v", res)
	}
}

func TestBytecodeBindingVariables(t *testing.T) {
	tests := []struct {
		input string
		want  any
	}{
		{input: `"Alice"`, want: "Alice"},
		{input: `40`, want: 40},
		{input: `true`, want: true},
	}
	for _, test := range tests {
		res := runBytecode(t, strings.Join([]string{
			fmt.Sprintf("let val = %s", test.input),
			"val",
		}, "\n"))
		if res != test.want {
			t.Fatalf("Expected %v, got %v", test.want, res)
		}
	}
}

func TestBytecodeArithmetic(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`let a = 2`,
		`let b = 3`,
		`a * b + 4`,
	}, "\n"))
	if res != 10 {
		t.Fatalf("Expected 10, got %v", res)
	}
}

func TestBytecodeStringConcat(t *testing.T) {
	res := runBytecode(t, `"hello" + " world"`)
	if res != "hello world" {
		t.Fatalf("Expected hello world, got %v", res)
	}
}

func TestBytecodeEquality(t *testing.T) {
	res := runBytecode(t, `1 == 1`)
	if res != true {
		t.Fatalf("Expected true, got %v", res)
	}
	res = runBytecode(t, `"a" == "a"`)
	if res != true {
		t.Fatalf("Expected true, got %v", res)
	}
}

func TestBytecodeIfExpression(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`let val = 3`,
		`if val > 2 { 10 } else { 20 }`,
	}, "\n"))
	if res != 10 {
		t.Fatalf("Expected 10, got %v", res)
	}
}

func TestBytecodeLogicalOps(t *testing.T) {
	res := runBytecode(t, `true and false`)
	if res != false {
		t.Fatalf("Expected false, got %v", res)
	}
	res = runBytecode(t, `true or false`)
	if res != true {
		t.Fatalf("Expected true, got %v", res)
	}
}

func TestBytecodeFunctionCall(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`fn add(a: Int, b: Int) Int { a + b }`,
		`add(2, 3)`,
	}, "\n"))
	if res != 5 {
		t.Fatalf("Expected 5, got %v", res)
	}
}

func TestBytecodeFirstClassFunction(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`let sub = fn(a: Int, b: Int) { a - b }`,
		`sub(30, 8)`,
	}, "\n"))
	if res != 22 {
		t.Fatalf("Expected 22, got %v", res)
	}
}

func TestBytecodeClosureCapture(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`fn createAdder(base: Int) fn(Int) Int {`,
		`  fn(x: Int) Int {`,
		`    base + x`,
		`  }`,
		`}`,
		`let addFive = createAdder(5)`,
		`addFive(10)`,
	}, "\n"))
	if res != 15 {
		t.Fatalf("Expected 15, got %v", res)
	}
}

func TestBytecodeInferAnonymousFunctionTypes(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`fn process(f: fn(Str) Bool) Bool {`,
		`  f("hello")`,
		`}`,
		`process(fn(x) { x.size() > 0 })`,
	}, "\n"))
	if res != true {
		t.Fatalf("Expected true, got %v", res)
	}

	res = runBytecode(t, strings.Join([]string{
		`fn check(f: fn(Str) Bool) Bool {`,
		`  f("test")`,
		`}`,
		`check(fn(s) { true })`,
	}, "\n"))
	if res != true {
		t.Fatalf("Expected true, got %v", res)
	}
}

func TestBytecodeFunctionCallFeatures(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "noop function",
			input: `
				fn noop() {}
				noop()`,
			want: nil,
		},
		{
			name: "returning with no args",
			input: `
				fn get_msg() { "Hello" }
				get_msg()`,
			want: "Hello",
		},
		{
			name: "one arg",
			input: `
				fn greet(name: Str) { "Hello, {name}!" }
				greet("Alice")`,
			want: "Hello, Alice!",
		},
		{
			name: "multiple args",
			input: `
				fn add(a: Int, b: Int) { a + b }
				add(1, 2)`,
			want: 3,
		},
		{
			name: "named arguments on static function respect names",
			input: `
				fn Person::full(first: Str, last: Str) Str {
				  "{first} {last}"
				}
				Person::full(last: "Doe", first: "Jane")`,
			want: "Jane Doe",
		},
		{
			name: "named arguments on instance method respect names",
			input: `
				struct Person {
				  first: Str,
				  last: Str,
				}
				impl Person {
				  fn full(first: Str, last: Str) Str {
				    "{first} {last}"
				  }
				}
				let p = Person{first: "Ignored", last: "AlsoIgnored"}
				p.full(last: "Doe", first: "Jane")`,
			want: "Jane Doe",
		},
		{
			name: "referencing module-level fn within method of same name",
			input: `
				fn process(x: Int) Int {
				  x * 2
				}
				struct Handler { }
				impl Handler {
				  fn process(x: Int) Str {
				    let result = process(5)
				    "Result: {result}"
				  }
				}
				let h = Handler{}
				h.process(10)`,
			want: "Result: 10",
		},
		{
			name: "module-level fn and method with same name but different param types",
			input: `
				fn process(x: Str) Str {
				  "string: {x}"
				}
				struct Handler { }
				impl Handler {
				  fn process(x: Int) Str {
				    let result = process("hello")
				    "handler: {result}"
				  }
				}
				let h = Handler{}
				h.process(42)`,
			want: "handler: string: hello",
		},
		{
			name: "calling Type::function static method defined with double colon syntax",
			input: `
				struct Fixture {
				  id: Int,
				  name: Str,
				}
				fn Fixture::from_entry(data: Str) Fixture {
				  Fixture{id: 1, name: data}
				}
				let f = Fixture::from_entry("Test")
				f.name`,
			want: "Test",
		},
		{
			name: "omitting nullable parameters",
			input: `
				use ard/maybe
				fn add(a: Int, b: Int?) Int {
					a + b.or(0)
				}
				add(1)`,
			want: 1,
		},
		{
			name: "omitting nullable parameters with explicit value",
			input: `
				use ard/maybe
				fn add(a: Int, b: Int?) Int {
					a + b.or(0)
				}
				add(1, maybe::some(5))`,
			want: 6,
		},
		{
			name: "automatic wrapping of non-nullable values for nullable parameters",
			input: `
				fn add(a: Int, b: Int?) Int {
					a + b.or(0)
				}
				add(1, 5)`,
			want: 6,
		},
		{
			name: "automatic wrapping works with omitted args",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				test(42)`,
			want: "42,0,default",
		},
		{
			name: "automatic wrapping with one wrapped argument",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				test(42, 7)`,
			want: "42,7,default",
		},
		{
			name: "automatic wrapping with all arguments provided",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				test(42, 7, "hello")`,
			want: "42,7,hello",
		},
		{
			name: "automatic wrapping of list literals for nullable parameters",
			input: `
				fn process(items: [Int]?) Bool {
					match items {
						lst => true
						_ => false
					}
				}
				process([10, 20, 30])`,
			want: true,
		},
		{
			name: "automatic wrapping of map literals for nullable parameters",
			input: `
				fn process(data: [Str:Int]?) Bool {
					match data {
						m => true
						_ => false
					}
				}
				process(["count": 42])`,
			want: true,
		},
		{
			name: "automatic wrapping with labeled arguments",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				test(a: 42, b: 7, c: "hello")`,
			want: "42,7,hello",
		},
		{
			name: "automatic wrapping with labeled arguments and omitted values",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				test(a: 42, c: "world")`,
			want: "42,0,world",
		},
		{
			name: "automatic wrapping with labeled arguments in different order",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				test(c: "reorder", b: 99, a: 5)`,
			want: "5,99,reorder",
		},
	}

	for _, test := range tests {
		res := runBytecode(t, test.input)
		if test.want == nil {
			if res != nil {
				t.Fatalf("%s: expected nil, got %v", test.name, res)
			}
			continue
		}
		if res != test.want {
			t.Fatalf("%s: expected %v, got %v", test.name, test.want, res)
		}
	}
}

func TestBytecodeIfInFunction(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`fn pick(n: Int) Int { if n > 2 { 10 } else { 20 } }`,
		`pick(3)`,
	}, "\n"))
	if res != 10 {
		t.Fatalf("Expected 10, got %v", res)
	}
}

func TestBytecodeFloatComparisons(t *testing.T) {
	res := runBytecode(t, `3.5 > 2.1`)
	if res != true {
		t.Fatalf("Expected true, got %v", res)
	}
	res = runBytecode(t, `3.5 <= 2.1`)
	if res != false {
		t.Fatalf("Expected false, got %v", res)
	}
}

func TestBytecodeListLiteral(t *testing.T) {
	res := runBytecode(t, `[1, 2, 3]`)
	items, ok := res.([]any)
	if !ok {
		t.Fatalf("Expected list result, got %T", res)
	}
	if len(items) != 3 || items[0] != 1 || items[1] != 2 || items[2] != 3 {
		t.Fatalf("Unexpected list result: %v", res)
	}
}

func TestBytecodeMapLiteral(t *testing.T) {
	res := runBytecode(t, `["a": 1, "b": 2]`)
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("Expected map result, got %T", res)
	}
	if m["a"] != 1 || m["b"] != 2 {
		t.Fatalf("Unexpected map result: %v", res)
	}
}

func TestBytecodeWhileLoop(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`mut count = 0`,
		`while count < 3 {`,
		`  count = count + 1`,
		`}`,
		`count`,
	}, "\n"))
	if res != 3 {
		t.Fatalf("Expected 3, got %v", res)
	}
}

func TestBytecodeForRangeLoop(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`mut sum = 0`,
		`for i in 1..3 {`,
		`  sum = sum + i`,
		`}`,
		`sum`,
	}, "\n"))
	if res != 6 {
		t.Fatalf("Expected 6, got %v", res)
	}
}

func TestBytecodeForInListLoop(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`mut sum = 0`,
		`let items = [1, 2, 3]`,
		`for item in items {`,
		`  sum = sum + item`,
		`}`,
		`sum`,
	}, "\n"))
	if res != 6 {
		t.Fatalf("Expected 6, got %v", res)
	}
}

func TestBytecodeForInMapLoop(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`mut sum = 0`,
		`let items = ["a": 1, "b": 2]`,
		`for key, val in items {`,
		`  sum = sum + val`,
		`}`,
		`sum`,
	}, "\n"))
	if res != 3 {
		t.Fatalf("Expected 3, got %v", res)
	}
}

func TestBytecodeListMethods(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`let items = [1, 2, 3]`,
		`items.size()`,
	}, "\n"))
	if res != 3 {
		t.Fatalf("Expected 3, got %v", res)
	}
	res = runBytecode(t, strings.Join([]string{
		`let items = [1, 2, 3]`,
		`items.at(1)`,
	}, "\n"))
	if res != 2 {
		t.Fatalf("Expected 2, got %v", res)
	}
}

func TestBytecodeMapMethods(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`let items = ["a": 1, "b": 2]`,
		`items.size()`,
	}, "\n"))
	if res != 2 {
		t.Fatalf("Expected 2, got %v", res)
	}
	res = runBytecode(t, strings.Join([]string{
		`let items = ["a": 1, "b": 2]`,
		`items.has("a")`,
	}, "\n"))
	if res != true {
		t.Fatalf("Expected true, got %v", res)
	}
	res = runBytecode(t, strings.Join([]string{
		`let items = ["a": 1, "b": 2]`,
		`items.get("a").or(0)`,
	}, "\n"))
	if res != 1 {
		t.Fatalf("Expected 1, got %v", res)
	}
}

func TestBytecodeStringMethods(t *testing.T) {
	res := runBytecode(t, `"hello".size()`)
	if res != 5 {
		t.Fatalf("Expected 5, got %v", res)
	}
	res = runBytecode(t, `"hello".contains("ell")`)
	if res != true {
		t.Fatalf("Expected true, got %v", res)
	}
}

func TestBytecodeBoolMatch(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`match (1 < 2) {`,
		`  true => 1,`,
		`  false => 2`,
		`}`,
	}, "\n"))
	if res != 1 {
		t.Fatalf("Expected 1, got %v", res)
	}
}

func TestBytecodeMaybeMatch(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`use ard/maybe`,
		`match maybe::some(3) {`,
		`  n => n + 1,`,
		`  _ => 0`,
		`}`,
	}, "\n"))
	if res != 4 {
		t.Fatalf("Expected 4, got %v", res)
	}
}

func TestBytecodeResultMatch(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`use ard/result`,
		`match Result::ok(5) {`,
		`  ok(n) => n,`,
		`  err => 0`,
		`}`,
	}, "\n"))
	if res != 5 {
		t.Fatalf("Expected 5, got %v", res)
	}
}

func TestBytecodeTryResult(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`use ard/result`,
		`fn compute() Int!Str {`,
		`  let val = try Result::ok(4)`,
		`  Result::ok(val + 1)`,
		`}`,
		`match compute() {`,
		`  ok(n) => n,`,
		`  err => 0`,
		`}`,
	}, "\n"))
	if res != 5 {
		t.Fatalf("Expected 5, got %v", res)
	}
}

func TestBytecodeTryMaybe(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`use ard/maybe`,
		`fn compute() Int? {`,
		`  let val = try maybe::some(4)`,
		`  maybe::some(val + 2)`,
		`}`,
		`match compute() {`,
		`  n => n,`,
		`  _ => 0`,
		`}`,
	}, "\n"))
	if res != 6 {
		t.Fatalf("Expected 6, got %v", res)
	}
}

func TestBytecodeStructAccess(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`struct Point { x: Int, y: Int }`,
		`fn main() Int {`,
		`  let p = Point{x: 1, y: 2}`,
		`  p.x + p.y`,
		`}`,
		`main()`,
	}, "\n"))
	if res != 3 {
		t.Fatalf("Expected 3, got %v", res)
	}
}

func TestBytecodeStructMethod(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`struct Point { x: Int, y: Int }`,
		`impl Point {`,
		`  fn sum() Int { @x + @y }`,
		`}`,
		`fn main() Int {`,
		`  let p = Point{x: 2, y: 3}`,
		`  p.sum()`,
		`}`,
		`main()`,
	}, "\n"))
	if res != 5 {
		t.Fatalf("Expected 5, got %v", res)
	}
}

func TestBytecodeEnumMatch(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`enum Color { Red, Green }`,
		`match Color::Red {`,
		`  Color::Red => 1,`,
		`  _ => 0`,
		`}`,
	}, "\n"))
	if res != 1 {
		t.Fatalf("Expected 1, got %v", res)
	}
}
