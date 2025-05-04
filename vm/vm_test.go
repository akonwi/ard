package vm_test

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm"
)

type test struct {
	name  string
	input string
	want  any
}

func run(t *testing.T, input string) any {
	t.Helper()
	tree, err := ast.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Error parsing program: %v", err)
	}
	program, diagnostics := checker.Check(tree)
	if len(diagnostics) > 0 {
		t.Fatalf("Diagnostics found: %v", diagnostics)
	}
	res, err := vm.Run(&program)
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
	res := run2(t, "")
	if res != nil {
		t.Fatalf("Expected nil, got %v", res)
	}
}

func TestPrinting(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	run2(t, strings.Join([]string{
		`use ard/io`,
		`io::print("Hello, World!")`,
		// `io::print("Hello, {{"Ard"}}!")`,
	}, "\n"))

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	got := buf.String()

	for _, want := range []string{
		"Hello, World!",
		// "Hello, Ard!",
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
		res := run2(t, strings.Join([]string{
			fmt.Sprintf(`let val = %v`, want),
			`val`,
		}, "\n"))
		if res != want {
			t.Fatalf("Expected %v, got %v", want, res)
		}
	}
}

func TestReassigningVariables(t *testing.T) {
	res := run2(t, strings.Join([]string{
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
		{`not true`, false},
		{`not false`, true},
		{`-10`, -10},
		{`-20.1`, -20.1},
	} {
		res := run2(t, test.input)
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
		{input: "(72.0 - 32.0) * 5.0 / 9.0", want: 22.22222222222222},
	}

	for _, test := range tests {
		if res := run2(t, test.input); res != test.want {
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
		{input: `not 30 == 30`, want: false},
		{input: `not 1 == 10`, want: true},
		{input: `true == false`, want: false},
		{input: `not true == false`, want: true},
		{input: `"hello" == "world"`, want: false},
		{input: `not "hello" == "world"`, want: true},
	}

	for _, test := range tests {
		if res := run2(t, test.input); res != test.want {
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
		if res := run2(t, test.input); res != test.want {
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
		if res := run2(t, test.input); res != test.want {
			t.Errorf("%s = %v but got %v", test.input, test.want, res)
		}
	}
}

func TestIfStatements(t *testing.T) {
	tests := []test{
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
				else if result.size() == 0 { result = "else if" }
				else { result = "else" }
				result`,
			want: "else if",
		},
	}

	runTests2(t, tests)
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
				fn add(a: Int, b: Int) { a + b }
				add(1, 2)`,
			want: 3,
		},
		{
			name: "first class functions",
			input: `
			let sub = fn(a: Int, b: Int) { a - b }
			sub(30, 8)`,
			want: 22,
		},
	}

	runTests2(t, tests)
}

func TestNumApi(t *testing.T) {
	runTests2(t, []test{
		{
			name:  "Int.to_str() returns the string representation of a number",
			input: `100.to_str()`,
			want:  "100",
		},
		{
			name:  "Int::from_str parses a string into a number",
			input: `Int::from_str("100")`,
			want:  100,
		},
	})
}

func TestFloatApi(t *testing.T) {
	runTests2(t, []test{
		{
			name:  ".to_str() returns the Str representation of a Float",
			input: `10.1.to_str()`,
			want:  "10.10",
		},
		{
			name:  "::from_str parses a Str into a Float",
			input: `Float::from_str("100")`,
			want:  100.0,
		},
		{
			name:  "::from_int turns an Int into a Float",
			input: `Float::from_int(100)`,
			want:  100.0,
		},
	})
}

func TestBoolApi(t *testing.T) {
	if res := run2(t, `true.to_str()`); res != "true" {
		t.Errorf(`Expected "true", got %v`, res)
	}
}

func TestStrApi(t *testing.T) {
	tests := []test{
		{
			name:  "Str.size()",
			input: `"foobar".size()`,
			want:  6,
		},
		{
			name:  "Str.is_empty()",
			input: `"".is_empty()`,
			want:  true,
		},
		{
			name:  "Str.contains()",
			input: `"foobar".contains("oba")`,
			want:  true,
		},
	}
	runTests2(t, tests)
}

func TestListApi(t *testing.T) {
	runTests2(t, []test{
		{
			name:  "List.size",
			input: "[1,2,3].size()",
			want:  3,
		},
		{
			name: "List::push",
			input: `
				mut list = [1,2,3]
				list.push(4)
			  list.size()`,
			want: 4,
		},
		{
			name: "List::at",
			input: `
				mut list = [1,2,3]
				list.push(4)
			  list.at(3)`,
			want: 4,
		},
		{
			name: "List::set updates the list at the specified index",
			input: `
				mut list = [1,2,3]
				list.set(1, 10)
				list.at(1)`,
			want: 10,
		},
	})
}

func TestMapApi(t *testing.T) {
	runTests2(t, []test{
		{
			name: "Map::size",
			input: `
				let ages = ["Alice":40, "Bob":30]
				let jobs: [Str:Int] = [:]
				ages.size() + jobs.size()`,
			want: 2,
		},
		{
			name: "Map::get reads entries",
			input: `
				let ages = ["Alice":40, "Bob":30]
				match ages.get("Alice") {
				  age => "Alice is {{age.to_str()}}",
					_ => "Not found"
				}`,
			want: "Alice is 40",
		},
		{
			name: "Map::set adds entries",
			input: `
				mut ages = ["Alice":40, "Bob":30]
				ages.set("Charlie", 25)
				ages.set("Joe", 1)
				ages.size()`,
			want: 4,
		},
		{
			name: "Map::set updates entries",
			input: `
				mut ages = ["Alice":40, "Bob":30]
				ages.set("Bob", 31)
				match ages.get("Bob") {
				  age => "Bob is {{age.to_str()}}",
					_ => "Not found"
				}`,
			want: "Bob is 31",
		},
		{
			name: "Map::drop removes entries",
			input: `
				mut ages = ["Alice":40, "Bob":30]
				ages.drop("Alice")
				match ages.get("Alice") {
				  age => age,
					_ => 0
				}`,
			want: 0,
		},
		{
			name: "Map::has returns whether an entry exists",
			input: `
				let ages = ["Alice":40, "Bob":30]
				let has_alice = ages.has("Alice").to_str()
				let has_charlie = ages.has("Charlie").to_str()
				"{{has_alice}},{{has_charlie}}"
				`,
			want: "true,false",
		},
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
					x: Int,
					y: Int,
				}

				impl (p: Point) {
					fn print() Str {
						"{{p.x.to_str()}},{{p.y.to_str()}}"
					}
				}

				let p = Point { x: 10, y: 20 }
				p.print()`,
			want: "10,20",
		},
		{
			name: "Reassigning struct properties",
			input: `
				struct Point {
					x: Int,
					y: Int,
				}
				mut p = Point { x: 10, y: 20 }
				p.x = 30
				p.x`,
			want: 30,
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

func TestUnions(t *testing.T) {
	runTests(t, []test{
		{
			name: "Using unions",
			input: `
				type Printable = Str | Int | Bool
				fn print(p: Printable) Str {
				  match p {
					  Str => it,
						Int => it.to_str(),
						_ => {
						  "boolean value"
						}
					}
				}
				print(true)
			`,
			want: "boolean value",
		},
	})
}
