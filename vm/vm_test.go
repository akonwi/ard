package vm_test

import (
	"fmt"
	"os"
	"path/filepath"
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
	panic string
}

func run(t *testing.T, input string) any {
	t.Helper()
	result := ast.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}
	tree := result.Program
	module, diagnostics := checker.Check(tree, nil, "test.ard")
	if len(diagnostics) > 0 {
		t.Fatalf("Diagnostics found: %v", diagnostics)
	}
	vm := vm.NewScriptRuntime(module)
	res, err := vm.Interpret()
	if err != nil {
		t.Fatalf("VM error: %s", err.Error())
	}
	return res
}

func expectPanic(t *testing.T, substring, input string) {
	t.Helper()
	result := ast.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}
	tree := result.Program
	module, diagnostics := checker.Check(tree, nil, "test.ard")
	if len(diagnostics) > 0 {
		t.Fatalf("Diagnostics found: %v", diagnostics)
	}
	vm := vm.NewScriptRuntime(module)
	_, runErr := vm.Interpret()
	if runErr == nil {
		t.Fatalf("Did not encounter expcted panic: %s", substring)
	}
	if !strings.Contains(runErr.Error(), substring) {
		t.Fatalf("Expected a panic containing: %s\nInstead received `%s`", substring, runErr)
	}
}

func runTests(t *testing.T, tests []test) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.panic != "" {
				expectPanic(t, test.panic, test.input)
				return
			}
			if res := run(t, test.input); test.want != res {
				t.Logf("Expected \"%v\", got \"%v\"", test.want, res)
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
		{`not true`, false},
		{`not false`, true},
		{`-10`, -10},
		{`-20.1`, -20.1},
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
		{input: `3_000 + 12`, want: 3012},
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
		// Float comparisons
		{input: `3.5 > 2.1`, want: true},
		{input: `2.1 > 3.5`, want: false},
		{input: `3.5 >= 2.1`, want: true},
		{input: `3.5 >= 3.5`, want: true},
		{input: `2.1 >= 3.5`, want: false},
		{input: `3.5 < 2.1`, want: false},
		{input: `2.1 < 3.5`, want: true},
		{input: `3.5 <= 2.1`, want: false},
		{input: `3.5 <= 3.5`, want: true},
		{input: `2.1 <= 3.5`, want: true},
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
		{input: `not 30 == 30`, want: false},
		{input: `not 1 == 10`, want: true},
		{input: `true == false`, want: false},
		{input: `not true == false`, want: true},
		{input: `"hello" == "world"`, want: false},
		{input: `not "hello" == "world"`, want: true},
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

	runTests(t, tests)
}

func TestNumApi(t *testing.T) {
	runTests(t, []test{
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
	runTests(t, []test{
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
		{
			name: "::floor() rounds down to the nearest integer",
			input: `
				if not Float::floor(10.5) == 10.0 { panic("10.5 failed") }
				if not Float::floor(9.0) == 9.0 { panic("9.0 failed") }
				if not Float::floor(7.73) == 7.0 { panic("7.73 failed") }
			`,
			want: nil,
		},
		{
			name:  ".to_int() converts exact floats",
			input: `5.0.to_int()`,
			want:  5,
		},
		{
			name:  ".to_int() truncates toward zero for positive decimals",
			input: `5.7.to_int()`,
			want:  5,
		},
		{
			name:  ".to_int() truncates toward zero for positive decimals near next integer",
			input: `5.9.to_int()`,
			want:  5,
		},
		{
			name:  ".to_int() works with negative values",
			input: `(0.0 - 10.0).to_int()`,
			want:  -10,
		},
		{
			name:  ".to_int() truncates toward zero for negative decimals",
			input: `(0.0 - 3.14).to_int()`,
			want:  -3,
		},
		{
			name:  ".to_int() truncates toward zero for negative decimals near next integer",
			input: `(0.0 - 3.9).to_int()`,
			want:  -3,
		},
		{
			name:  ".to_int() works with zero",
			input: `0.0.to_int()`,
			want:  0,
		},
	})
}

func TestBoolApi(t *testing.T) {
	if res := run(t, `true.to_str()`); res != "true" {
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
		{
			name:  "Str.split()",
			input: `"hello world".split(" ").at(1)`,
			want:  "world",
		},
		{
			name:  "Str.replace()",
			input: `"hello world".replace("world", "universe")`,
			want:  "hello universe",
		},
		{
			name:  "Str.replace_all()",
			input: `"hello world hello world".replace_all("world", "universe")`,
			want:  "hello universe hello universe",
		},
	}
	runTests(t, tests)
}

func TestListApi(t *testing.T) {
	runTests(t, []test{
		{
			name: "List::new",
			input: `mut nums = List::new<Int>()
			nums.push(1)
			nums.push(2)
			nums.size()`,
			want: 2,
		},
		{
			name:  "List.size",
			input: "[1,2,3].size()",
			want:  3,
		},
		{
			name: "List::prepend",
			input: `
				mut list = [1,2,3]
				list.prepend(4)
			  list.size()`,
			want: 4,
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
		{
			name: "List.sort()",
			input: `
				mut list = [3,7,8,5,2,9,5,4]
				list.sort(fn(a: Int, b: Int) Bool { a < b })
				list.at(0) + list.at(7) // 2 + 9 = 11
			`,
			want: 11,
		},
		{
			name: "List.swap swaps values at the given indexes",
			input: `
				mut list = [1,2,3]
				list.swap(0,2)
				list.at(0)`,
			want: 3,
		},
		{
			name: "List::concat a combined list",
			input: `
				let a = [1,2,3]
				let b = [4,5,6]
				let list = List::concat(a, b)
				list.at(3) == 4`,
			want: true,
		},
	})
}

func TestMapApi(t *testing.T) {
	runTests(t, []test{
		{
			name: "Map::size",
			input: `
				let ages = ["Alice":40, "Bob":30]
				let jobs: [Str:Int] = [:]
				ages.size() + jobs.size()`,
			want: 2,
		},
		{
			name: "Map::keys",
			input: `
						let ages = ["Alice":40, "Bob":30]
						ages.keys().size()`,
			want: 2,
		},
		{
			name: "Map::get reads entries",
			input: `
				let ages = ["Alice":40, "Bob":30]
				match ages.get("Alice") {
				  age => "Alice is {age.to_str()}",
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
				  age => "Bob is {age.to_str()}",
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
				"{has_alice},{has_charlie}"
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
			want: int8(3),
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
					  Str(str) => str,
						Int(int) => int.to_str(),
						_ => "boolean value"
					}
				}
				print(20)
			`,
			want: "20",
		},
	})
}

func TestPanic(t *testing.T) {
	expectPanic(t, "This is an error", `
		fn speak() {
		  panic("This is an error")
		}
		speak()
		1 + 1
	`)
}

func TestUserModuleVMIntegration(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "ard_vm_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create project files
	err = os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"test_project\""), 0644)
	if err != nil {
		t.Fatal(err)
	}

	mathContent := `fn add(a: Int, b: Int) Int {
    a + b
}`
	err = os.WriteFile(filepath.Join(tempDir, "math.ard"), []byte(mathContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Test content that imports and uses math::add
	mainContent := `use test_project/math
math::add(10, 20)`

	// Parse and check
	parseResult := ast.Parse([]byte(mainContent), "main.ard")
	if len(parseResult.Errors) > 0 {
		t.Fatal(parseResult.Errors[0].Message)
	}
	astTree := parseResult.Program

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	module, diagnostics := checker.Check(astTree, resolver, "main.ard")
	if len(diagnostics) > 0 {
		t.Fatalf("Unexpected diagnostics: %v", diagnostics)
	}

	// Run with VM
	vm := vm.NewScriptRuntime(module)
	result, err := vm.Interpret()
	if err != nil {
		t.Fatalf("VM error: %v", err)
	}

	// Should return 30
	t.Logf("Result type: %T, value: %v", result, result)
	if result != 30 {
		t.Errorf("Expected 30, got %v", result)
	}
}

func TestVoidLiteral(t *testing.T) {
	run(t, `
		// can assign a Void
		let unit = ()
		unit

		// can return void expresson
		fn void() Void!Str {
			if not 42 == 42 {
				Result::err("42 should equal 42")
			}
			Result::ok(())
		}
		// can do empty method calls
		void()
	`)
}

func TestTryOnMaybe(t *testing.T) {
	tests := []test{
		{
			name: "try on Maybe::some returns unwrapped value",
			input: `
				use ard/maybe

				fn get_value() Int? {
					maybe::some(42)
				}

				fn test() Int? {
					let value = try get_value()
					maybe::some(value + 1)
				}

				let result = test()
				match result {
					value => value,
					_ => -1
				}
			`,
			want: 43,
		},
		{
			name: "try on Maybe::none propagates none",
			input: `
				use ard/maybe

				fn get_value() Int? {
					maybe::none()
				}

				fn test() Int? {
					let value = try get_value()
					maybe::some(value + 1)
				}

				let result = test()
				match result {
					value => value,
					_ => -999
				}
			`,
			want: -999,
		},
		{
			name: "try on Maybe with catch block transforms none",
			input: `
				use ard/maybe

				fn get_value() Int? {
					maybe::none()
				}

				fn test() Int {
					let value = try get_value() -> _ { 42 }
					value + 1
				}

				test()
			`,
			want: 42,
		},
		{
			name: "try on Maybe with catch block - some case",
			input: `
				use ard/maybe

				fn get_value() Int? {
					maybe::some(10)
				}

				fn test() Int {
					let value = try get_value() -> _ { 42 }
					value + 1
				}

				test()
			`,
			want: 11,
		},
	}
	runTests(t, tests)
}

func TestTryOnMaybeDifferentTypes(t *testing.T) {
	tests := []test{
		{
			name: "try on Maybe with different inner types - success case",
			input: `
				use ard/maybe

				fn get_value() Int? {
					maybe::some(42)
				}

				fn test() Str? {
					let value = try get_value()  // Int? -> Int, function returns Str?
					maybe::some("success")
				}

				let result = test()
				match result {
					value => value,
					_ => "none"
				}
			`,
			want: "success",
		},
		{
			name: "try on Maybe with different inner types - none case",
			input: `
				use ard/maybe

				fn get_value() Int? {
					maybe::none()
				}

				fn test() Str? {
					let value = try get_value()  // Should early return none as Str?
					maybe::some("should not reach")
				}

				let result = test()
				match result {
					value => value,
					_ => "got none as expected"
				}
			`,
			want: "got none as expected",
		},
	}

	runTests(t, tests)
}
