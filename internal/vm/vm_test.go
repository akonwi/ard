package vm_test

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/akonwi/ard/internal/ast"
	"github.com/akonwi/ard/internal/checker"
	"github.com/akonwi/ard/internal/vm"
	ts_ard "github.com/akonwi/tree-sitter-ard/bindings/go"
)

type test struct {
	name  string
	input string
	want  any
}

func parse(t *testing.T, input string) ast.Program {
	t.Helper()
	ts, err := ts_ard.MakeParser()
	if err != nil {
		panic(err)
	}
	tree := ts.Parse([]byte(input), nil)
	parser := ast.NewParser([]byte(input), tree)
	program, err := parser.Parse()
	if err != nil {
		t.Fatalf("Program error: %v", err)
	}
	return program
}

func run(t *testing.T, input string) any {
	t.Helper()
	program, diagnostics := checker.Check(parse(t, input))
	if len(diagnostics) > 0 {
		t.Fatalf("Diagnostics found: %v", diagnostics)
	}
	v := vm.New(&program)
	res, err := v.Run()
	if err != nil {
		t.Fatalf("VM error: %v", err)
	}
	return res
}

func runTests(t *testing.T, tests []test) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if res := run(t, test.input); test.want != res {
				t.Logf("Expected %v, got %v", test.want, res)
				t.Fail()
			}
		})
	}
}

func TestEmptyProgram(t *testing.T) {
	res := run(t, "")
	if res != nil {
		t.Fatalf("Expected nil, got %v", res)
	}
}

func TestPrinting(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	run(t, strings.Join([]string{
		`use ard/io`,
		`io.print("Hello, World!")`,
		`io.print("Hello, {{"Ard"}}!")`,
	}, "\n"))

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	got := buf.String()

	for _, want := range []string{
		"Hello, World!",
		"Hello, Ard!",
	} {
		if strings.Contains(got, want) == false {
			t.Errorf("Expected \"%s\", got %s", want, got)
		}
	}
}

func TestBindingVariables(t *testing.T) {
	for want := range []any{
		"Alice",
		40,
		true,
	} {
		res := run(t, strings.Join([]string{
			fmt.Sprintf(`let val = %v`, want),
			`val`,
		}, "\n"))
		if res != want {
			t.Fatalf("Expected %v, got %v", want, res)
		}
	}
}

func TestReassigningVariables(t *testing.T) {
	res := run(t, strings.Join([]string{
		`mut val = 1`,
		`val = 2`,
		`val = 3`,
		`val`,
	}, "\n"))
	if res != 3 {
		t.Fatalf("Expected 3, got %v", res)
	}
}

func TestUnaryExpressions(t *testing.T) {
	for _, test := range []struct {
		input string
		want  any
	}{
		{`!true`, false},
		{`!false`, true},
		{`-10`, -10},
	} {
		res := run(t, test.input)
		if res != test.want {
			t.Fatalf("Expected %v, got %v", test.want, res)
		}
	}
}

func TestNumberOperations(t *testing.T) {
	tests := []struct {
		input string
		want  any
	}{
		{input: `30 + 12`, want: 42},
		{input: `30 - 2`, want: 28},
		{input: `30 * 2`, want: 60},
		{input: `30 / 2`, want: 15},
		{input: `30 % 2`, want: 30 % 2},
		{input: `30 > 2`, want: true},
		{input: `30 >= 2`, want: true},
		{input: `30 < 2`, want: false},
		{input: `30 <= 2`, want: false},
		{input: `30 <= 30`, want: true},
	}

	for _, test := range tests {
		if res := run(t, test.input); res != test.want {
			t.Errorf("%s = %v but got %v", test.input, test.want, res)
		}
	}
}

func TestEquality(t *testing.T) {
	tests := []struct {
		input string
		want  any
	}{
		{input: `30 == 30`, want: true},
		{input: `1 == 10`, want: false},
		{input: `30 != 30`, want: false},
		{input: `1 != 10`, want: true},
		{input: `true == false`, want: false},
		{input: `true != false`, want: true},
		{input: `"hello" == "world"`, want: false},
		{input: `"hello" != "world"`, want: true},
	}

	for _, test := range tests {
		if res := run(t, test.input); res != test.want {
			t.Errorf("%s = %v but got %v", test.input, test.want, res)
		}
	}
}

func TestBooleanOperations(t *testing.T) {
	tests := []struct {
		input string
		want  any
	}{
		{input: `true and false`, want: false},
		{input: `true or false`, want: true},
	}

	for _, test := range tests {
		if res := run(t, test.input); res != test.want {
			t.Errorf("%s = %v but got %v", test.input, test.want, res)
		}
	}
}

func TestArithmatic(t *testing.T) {
	tests := []struct {
		input string
		want  any
	}{
		{input: `(30 + 20) * 4`, want: 200},
		{input: `30 + (20 * 4)`, want: 110},
	}

	for _, test := range tests {
		if res := run(t, test.input); res != test.want {
			t.Errorf("%s = %v but got %v", test.input, test.want, res)
		}
	}
}

func TestIfStatements(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "Simple if",
			input: `
				let is_on = true
				mut result = 0
				if is_on {
					result = 1
				}
				result`,
			want: 1,
		},
		{
			name: "if-else",
			input: `
				let is_on = false
				mut result = ""
				if is_on { result = "on" }
				else { result = "off" }
				result`,
			want: "off",
		},
		{
			name: "if-(else if)-else",
			input: `
				let is_on = false
				mut result = ""
				if is_on { result = "then" }
				else if result.size > 0 { result = "else if" }
				else { result = "else" }
				result`,
			want: "else",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := run(t, test.input)
			if res != test.want {
				t.Logf("Expected %v, got %v", test.want, res)
				t.Fail()
			}
		})
	}
}

func TestForLoops(t *testing.T) {
	runTests(t, []test{
		{
			name: "loop over numeric range",
			input: `
					mut sum = 0
					for i in 1..5 {
						sum = sum + i
					}
					sum`,
			want: 15,
		},
		{
			name: "loop over a number",
			input: `
				mut sum = 0
				for i in 5 {
					sum = sum + i
				}
				sum`,
			want: 15,
		},
		{
			name: "looping over a string",
			input: `
				mut res = ""
				for c in "hello" {
					res = "{{c}}{{res}}"
				}
				res`,
			want: "olleh",
		},
		{
			name: "looping over a list",
			input: `
				mut sum = 0
				for n in [1,2,3,4,5] {
					sum = sum + n
				}
				sum
			`,
			want: 15,
		},
	})
}

func TestWhileLoops(t *testing.T) {
	input := `
		mut count = 5
		while count > 0 {
			count = count - 1
		}
		count == 0`

	if res := run(t, input); res != true {
		t.Errorf("Expected true but got %v", res)
	}
}

func TestFunctions(t *testing.T) {
	tests := []test{
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
				fn greet(name: Str) { "Hello, {{name}}!" }
				greet("Alice")`,
			want: "Hello, Alice!",
		},
		{
			name: "multiple args",
			input: `
				fn add(a: Num, b: Num) { a + b }
				add(1, 2)`,
			want: 3,
		},
		{
			name: "first class functions",
			input: `
			let sub = (a: Num, b: Num) { a - b }
			sub(30, 8)`,
			want: 22,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if res := run(t, test.input); res != test.want {
				t.Errorf("Expected %v, got %v", test.want, res)
			}
		})
	}
}

func TestNumApi(t *testing.T) {
	if res := run(t, `100.as_str`); res != "100" {
		t.Errorf(`Expected "100", got %v`, res)
	}
}

func TestBoolApi(t *testing.T) {
	if res := run(t, `true.as_str`); res != "true" {
		t.Errorf(`Expected "true", got %v`, res)
	}
}

func TestStrApi(t *testing.T) {
	res := run(t, `"foobar".size`)
	if res != 6 {
		t.Fatalf("Expected 6, got %v", res)
	}
}

func TestListApi(t *testing.T) {
	runTests(t, []test{
		{
			name:  "List::size",
			input: "[1,2,3].size",
			want:  3,
		},
		{
			name: "List::push",
			input: `
				mut list = [1,2,3]
				list.push(4)
			  list.size`,
			want: 4,
		},
		//// todo: requires Option type
		// {
		// 	name: "List::at",
		// 	input: `
		// 		mut list = [1,2,3]
		// 		list.push(4)
		// 	  list.at(3)`,
		// 	want: 4,
		// },
	})
}

func TestEnums(t *testing.T) {
	runTests(t, []test{
		{
			name: "Enum usage",
			input: `
				enum Direction {
					Up, Down, Left, Right
				}
				let dir: Direction = Direction::Right
				dir`,
			want: 3,
		},
		{
			name: "Matching on enum",
			input: `
				enum Direction {
					Up, Down, Left, Right
				}
				let dir: Direction = Direction::Right
				match dir {
					Direction::Up => "North",
					Direction::Down => {
						"South"
					},
					Direction::Left => "West",
					Direction::Right => "East"
				}`,
			want: "East",
		},
		{
			name: "Catch all",
			input: `
				enum Direction {
					Up, Down, Left, Right
				}
				let dir: Direction = Direction::Right
				match dir {
					Direction::Up => "North",
					Direction::Down => "South",
					_ => "skip"
				}`,
			want: "skip",
		},
	})
}

func TestStructs(t *testing.T) {
	runTests(t, []test{
		{
			name: "Struct usage",
			input: `
				struct Point {
					x: Num,
					y: Num,
				}
				let p = Point { x: 10, y: 20 }
				p.x`,
			want: 10,
		},
	})
}

func TestMatchingOnBooleans(t *testing.T) {
	runTests(t, []test{
		{
			name: "Matching on booleans",
			input: `
				let is_on = true
				match is_on {
					true => "on",
					false => "off"
				}`,
			want: "on",
		},
	})
}

// func TestOptions(t *testing.T) {
// 	runTests(t, []test{
// 		{
// 			name: "Option usage",
// 			input: `
// 				let maybe = option()
// 			`,
// 		},
// 	})
// }
