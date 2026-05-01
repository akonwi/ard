package air

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestLowerTinyProgram(t *testing.T) {
	program := lowerSource(t, `
		fn add(a: Int, b: Int) Int {
			a + b
		}

		add(1, 2)
	`)

	add := findFunction(t, program, "add")
	if got := len(add.Signature.Params); got != 2 {
		t.Fatalf("add param count = %d, want 2", got)
	}
	if kind := typeKind(t, program, add.Signature.Return); kind != TypeInt {
		t.Fatalf("add return kind = %v, want TypeInt", kind)
	}
	if add.Body.Result == nil || add.Body.Result.Kind != ExprIntAdd {
		t.Fatalf("add result = %#v, want ExprIntAdd", add.Body.Result)
	}
	if add.Body.Result.Left.Kind != ExprLoadLocal || add.Body.Result.Left.Local != 0 {
		t.Fatalf("add left = %#v, want local 0", add.Body.Result.Left)
	}
	if add.Body.Result.Right.Kind != ExprLoadLocal || add.Body.Result.Right.Local != 1 {
		t.Fatalf("add right = %#v, want local 1", add.Body.Result.Right)
	}

	main := program.Functions[program.Entry]
	if main.Body.Result == nil || main.Body.Result.Kind != ExprCall {
		t.Fatalf("main result = %#v, want ExprCall", main.Body.Result)
	}
	if main.Body.Result.Function != add.ID {
		t.Fatalf("main calls function %d, want add %d", main.Body.Result.Function, add.ID)
	}
	if got := len(main.Body.Result.Args); got != 2 {
		t.Fatalf("main call arg count = %d, want 2", got)
	}
}

func TestLowerStructLayoutAndFieldAccess(t *testing.T) {
	program := lowerSource(t, `
		struct User {
			name: Str,
			age: Int,
		}

		fn next_age(user: User) Int {
			user.age + 1
		}
	`)

	userType := findType(t, program, "User")
	if userType.Kind != TypeStruct {
		t.Fatalf("User kind = %v, want TypeStruct", userType.Kind)
	}
	if len(userType.Fields) != 2 {
		t.Fatalf("User field count = %d, want 2", len(userType.Fields))
	}
	if userType.Fields[0].Name != "age" || userType.Fields[0].Index != 0 {
		t.Fatalf("first field = %#v, want age at index 0", userType.Fields[0])
	}

	nextAge := findFunction(t, program, "next_age")
	if nextAge.Body.Result == nil || nextAge.Body.Result.Kind != ExprIntAdd {
		t.Fatalf("next_age result = %#v, want ExprIntAdd", nextAge.Body.Result)
	}
	field := nextAge.Body.Result.Left
	if field.Kind != ExprGetField {
		t.Fatalf("next_age left = %#v, want ExprGetField", field)
	}
	if field.Field != 0 {
		t.Fatalf("field index = %d, want age index 0", field.Field)
	}
}

func TestLowerIfExpression(t *testing.T) {
	program := lowerSource(t, `
		fn choose(flag: Bool) Int {
			if flag {
				1
			} else {
				2
			}
		}
	`)

	choose := findFunction(t, program, "choose")
	if choose.Body.Result == nil || choose.Body.Result.Kind != ExprIf {
		t.Fatalf("choose result = %#v, want ExprIf", choose.Body.Result)
	}
	if choose.Body.Result.Condition == nil || choose.Body.Result.Condition.Kind != ExprLoadLocal {
		t.Fatalf("condition = %#v, want local load", choose.Body.Result.Condition)
	}
	if choose.Body.Result.Then.Result == nil || choose.Body.Result.Then.Result.Int != 1 {
		t.Fatalf("then block = %#v, want 1", choose.Body.Result.Then.Result)
	}
	if choose.Body.Result.Else.Result == nil || choose.Body.Result.Else.Result.Int != 2 {
		t.Fatalf("else block = %#v, want 2", choose.Body.Result.Else.Result)
	}
}

func TestLowerResultConstructors(t *testing.T) {
	program := lowerSource(t, `
		fn pass() Void!Str {
			Result::ok(())
		}

		fn fail() Void!Str {
			Result::err("boom")
		}
	`)

	pass := findFunction(t, program, "pass")
	if pass.Body.Result == nil || pass.Body.Result.Kind != ExprMakeResultOk {
		t.Fatalf("pass result = %#v, want ExprMakeResultOk", pass.Body.Result)
	}
	if pass.Body.Result.Target == nil || pass.Body.Result.Target.Kind != ExprConstVoid {
		t.Fatalf("pass value = %#v, want void", pass.Body.Result.Target)
	}

	fail := findFunction(t, program, "fail")
	if fail.Body.Result == nil || fail.Body.Result.Kind != ExprMakeResultErr {
		t.Fatalf("fail result = %#v, want ExprMakeResultErr", fail.Body.Result)
	}
	if fail.Body.Result.Target == nil || fail.Body.Result.Target.Str != "boom" {
		t.Fatalf("fail value = %#v, want boom", fail.Body.Result.Target)
	}
	if len(program.Externs) != 0 {
		t.Fatalf("extern count = %d, want result constructors as AIR built-ins", len(program.Externs))
	}
}

func TestLowerTestsManifest(t *testing.T) {
	program := lowerSource(t, `
		use ard/testing

		test fn adds() Void!Str {
			testing::pass()
		}
	`)

	if len(program.Tests) != 1 {
		t.Fatalf("test count = %d, want 1", len(program.Tests))
	}
	if program.Tests[0].Name != "adds" {
		t.Fatalf("test name = %q, want adds", program.Tests[0].Name)
	}
}

func TestValidateRejectsBadTypeReference(t *testing.T) {
	program := &Program{
		Types: []TypeInfo{{ID: 1, Kind: TypeList, Name: "[Missing]", Elem: 99}},
		Entry: NoFunction,
	}
	if err := Validate(program); err == nil {
		t.Fatalf("Validate succeeded, want invalid type error")
	}
}

func lowerSource(t *testing.T, input string) *Program {
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
	program, err := Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	return program
}

func findFunction(t *testing.T, program *Program, name string) Function {
	t.Helper()
	for _, fn := range program.Functions {
		if fn.Name == name {
			return fn
		}
	}
	t.Fatalf("function %s not found", name)
	return Function{}
}

func findType(t *testing.T, program *Program, name string) TypeInfo {
	t.Helper()
	for _, typ := range program.Types {
		if typ.Name == name {
			return typ
		}
	}
	t.Fatalf("type %s not found", name)
	return TypeInfo{}
}

func typeKind(t *testing.T, program *Program, id TypeID) TypeKind {
	t.Helper()
	for _, typ := range program.Types {
		if typ.ID == id {
			return typ.Kind
		}
	}
	t.Fatalf("type id %d not found", id)
	return 0
}
