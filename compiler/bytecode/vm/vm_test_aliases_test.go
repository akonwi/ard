package vm

import "testing"

func TestBytecodeReassigningVariables(t *testing.T) {
	if got := runBytecode(t, "mut val = 1\nval = 2\nval = 3\nval"); got != 3 {
		t.Fatalf("Expected 3, got %v", got)
	}
}

func TestBytecodeUnaryExpressions(t *testing.T) {
	if got := runBytecode(t, "not true"); got != false {
		t.Fatalf("Expected false, got %v", got)
	}
}

func TestBytecodeNumberOperations(t *testing.T) {
	if got := runBytecode(t, "30 + 12"); got != 42 {
		t.Fatalf("Expected 42, got %v", got)
	}
}

func TestBytecodeEnumToIntComparisons(t *testing.T) {
	if got := runBytecode(t, "enum Status { active, inactive }\nlet status = Status::active\nstatus == 0"); got != true {
		t.Fatalf("Expected true, got %v", got)
	}
}

func TestBytecodeBooleanOperations(t *testing.T) {
	if got := runBytecode(t, "true and false"); got != false {
		t.Fatalf("Expected false, got %v", got)
	}
}

func TestBytecodeArithmatic(t *testing.T) {
	if got := runBytecode(t, "(30 + 20) * 4"); got != 200 {
		t.Fatalf("Expected 200, got %v", got)
	}
}

func TestBytecodeChainedComparisons(t *testing.T) {
	if got := runBytecode(t, "200 <= 250 <= 300"); got != true {
		t.Fatalf("Expected true, got %v", got)
	}
}

func TestBytecodeIfStatements(t *testing.T) {
	if got := runBytecode(t, "let is_on = false\nmut result = \"\"\nif is_on { result = \"then\" } else { result = \"else\" }\nresult"); got != "else" {
		t.Fatalf("Expected else, got %v", got)
	}
}

func TestBytecodeNumApi(t *testing.T) {
	if got := runBytecode(t, "Int::from_str(\"100\")"); got != 100 {
		t.Fatalf("Expected 100, got %v", got)
	}
}

func TestBytecodeFloatApi(t *testing.T) {
	if got := runBytecode(t, "Float::from_int(100)"); got != 100.0 {
		t.Fatalf("Expected 100.0, got %v", got)
	}
}

func TestBytecodeBoolApi(t *testing.T) {
	if got := runBytecode(t, "true.to_str()"); got != "true" {
		t.Fatalf("Expected true, got %v", got)
	}
}

func TestBytecodeStrApi(t *testing.T) {
	if got := runBytecode(t, "\"foobar\".size()"); got != 6 {
		t.Fatalf("Expected 6, got %v", got)
	}
}

func TestBytecodeMapApi(t *testing.T) {
	if got := runBytecode(t, "mut ages = Map::new<Int>()\nages.set(\"Alice\", 25)\nages.get(\"Alice\").or(0)"); got != 25 {
		t.Fatalf("Expected 25, got %v", got)
	}
}

func TestBytecodeEnums(t *testing.T) {
	if got := runBytecode(t, "enum Direction { Up, Down, Left, Right }\nlet dir: Direction = Direction::Right\ndir"); got != 3 {
		t.Fatalf("Expected 3, got %v", got)
	}
}

func TestBytecodeEnumValues(t *testing.T) {
	if got := runBytecode(t, "enum HttpStatus { Ok = 200, Not_Found = 404 }\nHttpStatus::Ok"); got != 200 {
		t.Fatalf("Expected 200, got %v", got)
	}
}

func TestBytecodeEnumEquality(t *testing.T) {
	if got := runBytecode(t, "enum Direction { Up, Down }\nlet a = Direction::Up\nlet b = Direction::Down\na == b"); got != false {
		t.Fatalf("Expected false, got %v", got)
	}
}

func TestBytecodeUnions(t *testing.T) {
	if got := runBytecode(t, `
		type Printable = Str | Int | Bool
		fn print(p: Printable) Str {
			match p {
				Str(s) => s,
				Int(i) => i.to_str(),
				_ => "bool"
			}
		}
		print(20)
	`); got != "20" {
		t.Fatalf("Expected 20, got %v", got)
	}
}

func TestBytecodePanic(t *testing.T) {
	expectBytecodeRuntimeError(t, "This is an error", `
		fn speak() { panic("This is an error") }
		speak()
	`)
}

func TestBytecodeUserModuleVMIntegration(t *testing.T) {
	TestBytecodeVMParityModuleIntegration(t)
}

func TestBytecodeFunctionVariableFromModule(t *testing.T) {
	TestBytecodeVMParityModuleIntegration(t)
}

func TestBytecodeFunctionVariableCallDirectly(t *testing.T) {
	TestBytecodeVMParityModuleIntegration(t)
}

func TestBytecodeFunctionVariableInLocalScope(t *testing.T) {
	if got := runBytecode(t, "let multiply = fn(a: Int, b: Int) Int { a * b }\nmultiply(3, 4)"); got != 12 {
		t.Fatalf("Expected 12, got %v", got)
	}
}

func TestBytecodeVoidLiteral(t *testing.T) {
	runBytecodeRaw(t, `
		let unit = ()
		fn void() Void!Str { Result::ok(()) }
		void()
	`)
}

func TestBytecodeInlineBlockExpressions(t *testing.T) {
	if got := runBytecode(t, `
		let value = {
			let x = 10
			let y = 32
			x + y
		}
		value
	`); got != 42 {
		t.Fatalf("Expected 42, got %v", got)
	}
}
