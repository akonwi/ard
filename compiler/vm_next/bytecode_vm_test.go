package vm_next

import (
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestBytecodeRunScriptScalarSliceMatchesTreeWalk(t *testing.T) {
	tests := []string{
		`40 + 2`,
		`
			mut count = 40
			count = count + 2
			count
		`,
		`
			fn add(a: Int, b: Int) Int { a + b }
			add(20, 22)
		`,
		`
			let label = "ard" + "lang"
			(label == "ardlang") and (3 < 4)
		`,
		`
			if 10 > 5 { 42 } else { 0 }
		`,
		`
			let value = {
				let x = 10
				let y = 32
				x + y
			}
			value
		`,
		`40.to_str() + " " + true.to_str()`,
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			program := lowerProgramForBytecodeTest(t, input)
			wantVM, err := NewWithExterns(program, nil)
			if err != nil {
				t.Fatalf("new tree vm: %v", err)
			}
			want, err := wantVM.RunScript()
			if err != nil {
				t.Fatalf("run tree vm: %v", err)
			}

			gotVM, err := NewWithBytecode(program, nil)
			if err != nil {
				t.Fatalf("new bytecode vm: %v", err)
			}
			got, err := gotVM.RunScript()
			if err != nil {
				t.Fatalf("run bytecode vm: %v", err)
			}
			if !valuesEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
}

func TestBytecodeRunScriptDataAndExternSliceMatchesTreeWalk(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		externs HostFunctionRegistry
	}{
		{
			name: "struct fields and assignment",
			input: `
				struct User { name: Str, age: Int }
				mut user = User{name: "Ada", age: 41}
				user.age = user.age + 1
				user.age
			`,
		},
		{
			name: "list operations",
			input: `
				mut xs = [1, 2]
				xs.push(40)
				xs.at(0) + xs.at(1) + xs.at(2) + xs.size()
			`,
		},
		{
			name: "map operations",
			input: `
				mut values = ["a": 1]
				values.set("b", 2)
				if values.has("b") { values.size() } else { 0 }
			`,
		},
		{
			name: "extern call",
			input: `
				use ard/io
				io::print(42)
			`,
			externs: HostFunctionRegistry{"Print": func(value string) {}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := lowerProgramForBytecodeTest(t, test.input)
			wantVM, err := NewWithExterns(program, test.externs)
			if err != nil {
				t.Fatalf("new tree vm: %v", err)
			}
			want, err := wantVM.RunScript()
			if err != nil {
				t.Fatalf("run tree vm: %v", err)
			}
			gotVM, err := NewWithBytecode(program, test.externs)
			if err != nil {
				t.Fatalf("new bytecode vm: %v", err)
			}
			got, err := gotVM.RunScript()
			if err != nil {
				t.Fatalf("run bytecode vm: %v", err)
			}
			if !valuesEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
}

func TestBytecodeRunEntryScalarSliceMatchesTreeWalk(t *testing.T) {
	program := lowerProgramForBytecodeTest(t, `
		fn main() Int {
			let base = 20
			if base == 20 { base + 22 } else { 0 }
		}
	`)
	wantVM, err := NewWithExterns(program, nil)
	if err != nil {
		t.Fatalf("new tree vm: %v", err)
	}
	want, err := wantVM.RunEntry()
	if err != nil {
		t.Fatalf("run tree vm: %v", err)
	}
	gotVM, err := NewWithBytecode(program, nil)
	if err != nil {
		t.Fatalf("new bytecode vm: %v", err)
	}
	got, err := gotVM.RunEntry()
	if err != nil {
		t.Fatalf("run bytecode vm: %v", err)
	}
	if !valuesEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
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
