package vm_next

import (
	"reflect"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

type bytecodeRunCase struct {
	name    string
	input   string
	want    any
	externs HostFunctionRegistry
}

func TestBytecodeRunScriptScalarSlice(t *testing.T) {
	runBytecodeScriptCases(t, []bytecodeRunCase{
		{name: "arithmetic", input: `40 + 2`, want: 42},
		{
			name: "assignment",
			input: `
				mut count = 40
				count = count + 2
				count
			`,
			want: 42,
		},
		{
			name: "direct function call",
			input: `
				fn add(a: Int, b: Int) Int { a + b }
				add(20, 22)
			`,
			want: 42,
		},
		{
			name: "string and bool operations",
			input: `
				let label = "ard" + "lang"
				(label == "ardlang") and (3 < 4)
			`,
			want: true,
		},
		{name: "if expression", input: `if 10 > 5 { 42 } else { 0 }`, want: 42},
		{
			name: "block expression",
			input: `
				let value = {
					let x = 10
					let y = 32
					x + y
				}
				value
			`,
			want: 42,
		},
		{name: "to_str methods", input: `40.to_str() + " " + true.to_str()`, want: "40 true"},
	})
}

func TestBytecodeRunScriptDataAndExternSlice(t *testing.T) {
	runBytecodeScriptCases(t, []bytecodeRunCase{
		{
			name: "struct fields and assignment",
			input: `
				struct User { name: Str, age: Int }
				mut user = User{name: "Ada", age: 41}
				user.age = user.age + 1
				user.age
			`,
			want: 42,
		},
		{
			name: "list operations",
			input: `
				mut xs = [1, 2]
				xs.push(40)
				xs.at(0) + xs.at(1) + xs.at(2) + xs.size()
			`,
			want: 46,
		},
		{
			name: "map operations",
			input: `
				mut values = ["a": 1]
				values.set("b", 2)
				if values.has("b") { values.size() } else { 0 }
			`,
			want: 2,
		},
	})
}

func TestBytecodeRunScriptExternCall(t *testing.T) {
	var printed []string
	program := lowerProgramForBytecodeTest(t, `
		use ard/io
		io::print(42)
	`)
	vm, err := NewWithBytecode(program, HostFunctionRegistry{
		"Print": func(value string) { printed = append(printed, value) },
	})
	if err != nil {
		t.Fatalf("new bytecode vm: %v", err)
	}
	got, err := vm.RunScript()
	if err != nil {
		t.Fatalf("run bytecode vm: %v", err)
	}
	if got.Kind != ValueVoid {
		t.Fatalf("got %#v, want void", got)
	}
	if !reflect.DeepEqual(printed, []string{"42"}) {
		t.Fatalf("printed = %#v, want [42]", printed)
	}
}

func TestBytecodeRunScriptMatchSlice(t *testing.T) {
	runBytecodeScriptCases(t, []bytecodeRunCase{
		{
			name: "enum match",
			input: `
				enum Status {
					Open,
					Closed,
				}
				let status = Status::Closed
				match status {
					Status::Open => 1,
					Status::Closed => 42,
				}
			`,
			want: 42,
		},
		{
			name: "int range match",
			input: `
				let x = 7
				match x {
					0 => 0,
					1..10 => 42,
					_ => 1,
				}
			`,
			want: 42,
		},
		{
			name: "maybe match",
			input: `
				use ard/maybe
				let value = maybe::some(42)
				match value {
					x => x,
					_ => 0,
				}
			`,
			want: 42,
		},
		{
			name: "result match",
			input: `
				let value: Int!Str = Result::err("nope")
				match value {
					ok => ok,
					err => err.size() + 38,
				}
			`,
			want: 42,
		},
		{
			name: "union match",
			input: `
				type Printable = Str | Int
				let value: Printable = 42
				match value {
					Str => 0,
					Int => it,
				}
			`,
			want: 42,
		},
	})
}

func TestBytecodeRunScriptHelpersAndClosureSlice(t *testing.T) {
	runBytecodeScriptCases(t, []bytecodeRunCase{
		{
			name: "maybe map",
			input: `
				use ard/maybe
				let value = maybe::some(41)
				value.map(fn(x: Int) Int { x + 1 }).expect("mapped")
			`,
			want: 42,
		},
		{
			name: "result map",
			input: `
				let value: Int!Str = Result::ok(41)
				value.map(fn(x: Int) Int { x + 1 }).expect("mapped")
			`,
			want: 42,
		},
		{
			name: "try result",
			input: `
				fn parse() Int!Str { Result::ok(42) }
				fn main_value() Int!Str {
					let value = try parse()
					Result::ok(value)
				}
				main_value().expect("ok")
			`,
			want: 42,
		},
		{
			name: "string helpers",
			input: `
				let parts = "  ard lang  ".trim().split(" ")
				parts.size() + parts.at(0).size() + parts.at(1).size()
			`,
			want: 9,
		},
	})
}

func TestBytecodeRunEntryScalarSlice(t *testing.T) {
	program := lowerProgramForBytecodeTest(t, `
		fn main() Int {
			let base = 20
			if base == 20 { base + 22 } else { 0 }
		}
	`)
	vm, err := NewWithBytecode(program, nil)
	if err != nil {
		t.Fatalf("new bytecode vm: %v", err)
	}
	got, err := vm.RunEntry()
	if err != nil {
		t.Fatalf("run bytecode vm: %v", err)
	}
	if got.GoValue() != 42 {
		t.Fatalf("got %#v, want 42", got.GoValue())
	}
}

func runBytecodeScriptCases(t *testing.T, tests []bytecodeRunCase) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := lowerProgramForBytecodeTest(t, test.input)
			vm, err := NewWithBytecode(program, test.externs)
			if err != nil {
				t.Fatalf("new bytecode vm: %v", err)
			}
			got, err := vm.RunScript()
			if err != nil {
				t.Fatalf("run bytecode vm: %v", err)
			}
			if !reflect.DeepEqual(got.GoValue(), test.want) {
				t.Fatalf("got %#v, want %#v", got.GoValue(), test.want)
			}
		})
	}
}

func lowerProgramForBytecodeTest(t *testing.T, input string) *air.Program {
	t.Helper()
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	return program
}
