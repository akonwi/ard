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

	if program.Entry != NoFunction {
		t.Fatalf("entry = %d, want no entry for script-only program", program.Entry)
	}
	if program.Script == NoFunction {
		t.Fatal("script = NoFunction, want generated script function")
	}
	script := program.Functions[program.Script]
	if script.Body.Result == nil || script.Body.Result.Kind != ExprCall {
		t.Fatalf("script result = %#v, want ExprCall", script.Body.Result)
	}
	if script.Body.Result.Function != add.ID {
		t.Fatalf("script calls function %d, want add %d", script.Body.Result.Function, add.ID)
	}
	if got := len(script.Body.Result.Args); got != 2 {
		t.Fatalf("script call arg count = %d, want 2", got)
	}
}

func TestLowerMainEntrypoint(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			42
		}
	`)

	if program.Entry == NoFunction {
		t.Fatal("entry = NoFunction, want main function")
	}
	if program.Script != NoFunction {
		t.Fatalf("script = %d, want no generated script function", program.Script)
	}
	entry := program.Functions[program.Entry]
	if entry.Name != "main" {
		t.Fatalf("entry name = %q, want main", entry.Name)
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

func TestLowerBoolMatch(t *testing.T) {
	program := lowerSource(t, `
		fn choose(flag: Bool) Int {
			match flag {
				true => 1,
				false => 2,
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

func TestLowerWhileLoop(t *testing.T) {
	program := lowerSource(t, `
		mut count = 0
		while count < 3 {
			count = count + 1
		}
		count
	`)

	script := program.Functions[program.Script]
	if len(script.Body.Stmts) != 2 {
		t.Fatalf("script stmt count = %d, want let and while", len(script.Body.Stmts))
	}
	loop := script.Body.Stmts[1]
	if loop.Kind != StmtWhile {
		t.Fatalf("second stmt = %#v, want StmtWhile", loop)
	}
	if loop.Condition == nil || loop.Condition.Kind != ExprLt {
		t.Fatalf("while condition = %#v, want ExprLt", loop.Condition)
	}
	if len(loop.Body.Stmts) != 1 || loop.Body.Stmts[0].Kind != StmtAssign {
		t.Fatalf("while body = %#v, want assignment", loop.Body)
	}
}

func TestLowerEnums(t *testing.T) {
	program := lowerSource(t, `
		enum Direction {
			Up, Down, Left, Right
		}

		fn right() Direction {
			Direction::Right
		}

		fn name(direction: Direction) Str {
			match direction {
				Direction::Up => "North",
				Direction::Down => "South",
				Direction::Left => "West",
				Direction::Right => "East",
			}
		}
	`)

	directionType := findType(t, program, "Direction")
	if directionType.Kind != TypeEnum {
		t.Fatalf("Direction kind = %v, want TypeEnum", directionType.Kind)
	}
	if len(directionType.Variants) != 4 {
		t.Fatalf("Direction variant count = %d, want 4", len(directionType.Variants))
	}

	right := findFunction(t, program, "right")
	if right.Body.Result == nil || right.Body.Result.Kind != ExprEnumVariant {
		t.Fatalf("right result = %#v, want ExprEnumVariant", right.Body.Result)
	}
	if right.Body.Result.Variant != 3 || right.Body.Result.Discriminant != 3 {
		t.Fatalf("right variant = %#v, want index/discriminant 3", right.Body.Result)
	}

	name := findFunction(t, program, "name")
	if name.Body.Result == nil || name.Body.Result.Kind != ExprMatchEnum {
		t.Fatalf("name result = %#v, want ExprMatchEnum", name.Body.Result)
	}
	if len(name.Body.Result.EnumCases) != 4 {
		t.Fatalf("enum case count = %d, want 4", len(name.Body.Result.EnumCases))
	}
	if name.Body.Result.EnumCases[3].Discriminant != 3 {
		t.Fatalf("right case = %#v, want discriminant 3", name.Body.Result.EnumCases[3])
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

func TestLowerMaybes(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn some() Int? {
			maybe::some(42)
		}

		fn none() Int? {
			maybe::none()
		}

		fn fallback(value: Int?) Int {
			value.or(42)
		}

		fn pick(value: Int?) Int {
			match value {
				v => v,
				_ => 0,
			}
		}
	`)

	some := findFunction(t, program, "some")
	if some.Body.Result == nil || some.Body.Result.Kind != ExprMakeMaybeSome {
		t.Fatalf("some result = %#v, want ExprMakeMaybeSome", some.Body.Result)
	}
	if some.Body.Result.Target == nil || some.Body.Result.Target.Int != 42 {
		t.Fatalf("some target = %#v, want 42", some.Body.Result.Target)
	}

	none := findFunction(t, program, "none")
	if none.Body.Result == nil || none.Body.Result.Kind != ExprMakeMaybeNone {
		t.Fatalf("none result = %#v, want ExprMakeMaybeNone", none.Body.Result)
	}

	fallback := findFunction(t, program, "fallback")
	if fallback.Body.Result == nil || fallback.Body.Result.Kind != ExprMaybeOr {
		t.Fatalf("fallback result = %#v, want ExprMaybeOr", fallback.Body.Result)
	}

	pick := findFunction(t, program, "pick")
	if pick.Body.Result == nil || pick.Body.Result.Kind != ExprMatchMaybe {
		t.Fatalf("pick result = %#v, want ExprMatchMaybe", pick.Body.Result)
	}
	if pick.Body.Result.SomeLocal < LocalID(len(pick.Signature.Params)) {
		t.Fatalf("some local = %d, want local after params", pick.Body.Result.SomeLocal)
	}
}

func TestLowerResults(t *testing.T) {
	program := lowerSource(t, `
		fn value(result: Int!Str) Int {
			result.or(42)
		}

		fn pick(result: Int!Str) Int {
			match result {
				ok(value) => value,
				err(msg) => 0,
			}
		}
	`)

	value := findFunction(t, program, "value")
	if value.Body.Result == nil || value.Body.Result.Kind != ExprResultOr {
		t.Fatalf("value result = %#v, want ExprResultOr", value.Body.Result)
	}

	pick := findFunction(t, program, "pick")
	if pick.Body.Result == nil || pick.Body.Result.Kind != ExprMatchResult {
		t.Fatalf("pick result = %#v, want ExprMatchResult", pick.Body.Result)
	}
	if pick.Body.Result.OkLocal < LocalID(len(pick.Signature.Params)) {
		t.Fatalf("ok local = %d, want local after params", pick.Body.Result.OkLocal)
	}
	if pick.Body.Result.ErrLocal < LocalID(len(pick.Signature.Params)) {
		t.Fatalf("err local = %d, want local after params", pick.Body.Result.ErrLocal)
	}
}

func TestLowerTraitAndImplTables(t *testing.T) {
	program := lowerSource(t, `
		trait Speaks {
			fn speak() Str
		}

		struct Dog {
			name: Str,
		}

		impl Speaks for Dog {
			fn speak() Str {
				self.name + " says hi"
			}
		}

		fn describe(speaker: Speaks) Str {
			"speaker"
		}
	`)

	if len(program.Traits) != 1 {
		t.Fatalf("trait count = %d, want 1", len(program.Traits))
	}
	trait := program.Traits[0]
	if trait.Name != "Speaks" {
		t.Fatalf("trait name = %q, want Speaks", trait.Name)
	}
	if len(trait.Methods) != 1 || trait.Methods[0].Name != "speak" {
		t.Fatalf("trait methods = %#v, want speak", trait.Methods)
	}
	if kind := typeKind(t, program, trait.Methods[0].Signature.Return); kind != TypeStr {
		t.Fatalf("trait method return kind = %v, want TypeStr", kind)
	}

	traitObject := findType(t, program, "Speaks")
	if traitObject.Kind != TypeTraitObject {
		t.Fatalf("Speaks type kind = %v, want TypeTraitObject", traitObject.Kind)
	}
	if traitObject.Trait != trait.ID {
		t.Fatalf("Speaks trait id = %d, want %d", traitObject.Trait, trait.ID)
	}

	dogType := findType(t, program, "Dog")
	if len(program.Impls) != 1 {
		t.Fatalf("impl count = %d, want 1", len(program.Impls))
	}
	impl := program.Impls[0]
	if impl.Trait != trait.ID || impl.ForType != dogType.ID {
		t.Fatalf("impl = %#v, want trait %d for Dog type %d", impl, trait.ID, dogType.ID)
	}
	if len(impl.Methods) != 1 {
		t.Fatalf("impl method count = %d, want 1", len(impl.Methods))
	}
	method := program.Functions[impl.Methods[0]]
	if method.Name != "Dog.Speaks.speak" {
		t.Fatalf("method name = %q, want Dog.Speaks.speak", method.Name)
	}
	if len(method.Signature.Params) != 1 {
		t.Fatalf("method param count = %d, want receiver only", len(method.Signature.Params))
	}
	if method.Signature.Params[0].Name != "self" || method.Signature.Params[0].Type != dogType.ID {
		t.Fatalf("method receiver = %#v, want self Dog", method.Signature.Params[0])
	}
	if method.Body.Result == nil || method.Body.Result.Kind != ExprStrConcat {
		t.Fatalf("method result = %#v, want ExprStrConcat", method.Body.Result)
	}
	if method.Body.Result.Left == nil || method.Body.Result.Left.Kind != ExprGetField {
		t.Fatalf("method left = %#v, want field access", method.Body.Result.Left)
	}
}

func TestLowerTraitObjectDispatch(t *testing.T) {
	program := lowerSource(t, `
		trait Speaks {
			fn speak() Str
		}

		struct Dog {
			name: Str,
		}

		impl Speaks for Dog {
			fn speak() Str {
				self.name + " says hi"
			}
		}

		fn describe(speaker: Speaks) Str {
			speaker.speak()
		}

		describe(Dog{name: "Ada"})
	`)

	describe := findFunction(t, program, "describe")
	if describe.Body.Result == nil || describe.Body.Result.Kind != ExprCallTrait {
		t.Fatalf("describe result = %#v, want ExprCallTrait", describe.Body.Result)
	}
	if describe.Body.Result.Method != 0 {
		t.Fatalf("trait method index = %d, want 0", describe.Body.Result.Method)
	}

	script := program.Functions[program.Script]
	if script.Body.Result == nil || script.Body.Result.Kind != ExprCall {
		t.Fatalf("script result = %#v, want ExprCall", script.Body.Result)
	}
	if len(script.Body.Result.Args) != 1 || script.Body.Result.Args[0].Kind != ExprTraitUpcast {
		t.Fatalf("script arg = %#v, want ExprTraitUpcast", script.Body.Result.Args)
	}
	if script.Body.Result.Args[0].Impl != program.Impls[0].ID {
		t.Fatalf("upcast impl = %d, want %d", script.Body.Result.Args[0].Impl, program.Impls[0].ID)
	}
}

func TestLowerTryOps(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn result_value(result: Int!Str) Int!Str {
			let value = try result
			Result::ok(value + 1)
		}

		fn catch_result(result: Int!Str) Int {
			let value = try result -> err {
				0
			}
			value + 1
		}

		fn maybe_value(value: Int?) Int? {
			let inner = try value
			maybe::some(inner + 1)
		}
	`)

	resultValue := findFunction(t, program, "result_value")
	firstLet := resultValue.Body.Stmts[0]
	if firstLet.Value == nil || firstLet.Value.Kind != ExprTryResult {
		t.Fatalf("result try = %#v, want ExprTryResult", firstLet.Value)
	}
	if firstLet.Value.HasCatch {
		t.Fatalf("result try HasCatch = true, want false")
	}

	catchResult := findFunction(t, program, "catch_result")
	catchLet := catchResult.Body.Stmts[0]
	if catchLet.Value == nil || catchLet.Value.Kind != ExprTryResult {
		t.Fatalf("catch try = %#v, want ExprTryResult", catchLet.Value)
	}
	if !catchLet.Value.HasCatch {
		t.Fatalf("catch try HasCatch = false, want true")
	}
	if catchLet.Value.CatchLocal < LocalID(len(catchResult.Signature.Params)) {
		t.Fatalf("catch local = %d, want local after params", catchLet.Value.CatchLocal)
	}

	maybeValue := findFunction(t, program, "maybe_value")
	maybeLet := maybeValue.Body.Stmts[0]
	if maybeLet.Value == nil || maybeLet.Value.Kind != ExprTryMaybe {
		t.Fatalf("maybe try = %#v, want ExprTryMaybe", maybeLet.Value)
	}
}

func TestLowerImportedModuleFunctionCall(t *testing.T) {
	program := lowerSource(t, `
		use ard/testing

		fn check() Void!Str {
			let result = testing::assert(true, "should pass")
			result
		}
	`)

	assert := findFunction(t, program, "assert")
	check := findFunction(t, program, "check")
	if len(check.Body.Stmts) != 1 || check.Body.Stmts[0].Value == nil || check.Body.Stmts[0].Value.Kind != ExprCall {
		t.Fatalf("check function = %#v, want let ExprCall", check)
	}
	if check.Body.Stmts[0].Value.Function != assert.ID {
		t.Fatalf("check calls function %d, want assert %d", check.Body.Stmts[0].Value.Function, assert.ID)
	}
	for _, test := range program.Tests {
		if test.Function == assert.ID || test.Name == "test_assert_true_passes" {
			t.Fatalf("imported module tests should not be added to root manifest: %#v", program.Tests)
		}
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
		Types:  []TypeInfo{{ID: 1, Kind: TypeList, Name: "[Missing]", Elem: 99}},
		Entry:  NoFunction,
		Script: NoFunction,
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
