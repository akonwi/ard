package air

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLowerTransitiveStructMethodCanReadOwnerModuleGlobal(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.1.0\""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "c.ard"), []byte(`
		let SECRET = "ok"

		struct C {}

		impl C {
			fn greet() Str {
				SECRET
			}
		}
	`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "b.ard"), []byte(`
		use test_project/c

		type C = c::C
	`), 0o644); err != nil {
		t.Fatal(err)
	}

	program := lowerProjectSource(t, tempDir, `
		use test_project/b

		fn use_c(value: b::C) Str {
			value.greet()
		}
	`)
	greet := findFunction(t, program, "C.greet")
	if greet.Body.Result == nil || greet.Body.Result.Kind != ExprLoadGlobal {
		t.Fatalf("greet result = %#v, want ExprLoadGlobal", greet.Body.Result)
	}
}

func TestLowerFunctionCanReadModuleLevelLet(t *testing.T) {
	program := lowerSource(t, `
		let refresh_event = "inbox.refresh"

		fn event_name() Str {
			refresh_event
		}
	`)

	if len(program.Globals) != 1 {
		t.Fatalf("global count = %d, want 1", len(program.Globals))
	}
	if program.Globals[0].Name != "refresh_event" {
		t.Fatalf("global name = %q, want refresh_event", program.Globals[0].Name)
	}
	eventName := findFunction(t, program, "event_name")
	if eventName.Body.Result == nil || eventName.Body.Result.Kind != ExprLoadGlobal {
		t.Fatalf("event_name result = %#v, want ExprLoadGlobal", eventName.Body.Result)
	}
	if eventName.Body.Result.Global != program.Globals[0].ID {
		t.Fatalf("event_name loads global %d, want %d", eventName.Body.Result.Global, program.Globals[0].ID)
	}
}

func TestLowerOmitsTestsByDefault(t *testing.T) {
	result := parse.Parse([]byte(`
		fn main() Int { 1 }
		test fn check() Void!Str { Result::ok(()) }
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}

	production, err := Lower(c.Module())
	if err != nil {
		t.Fatalf("lower production: %v", err)
	}
	if len(production.Tests) != 0 {
		t.Fatalf("production tests = %#v, want none", production.Tests)
	}
	for _, fn := range production.Functions {
		if fn.IsTest || fn.Name == "check" {
			t.Fatalf("production function includes test: %#v", fn)
		}
	}

	withTests, err := LowerWithTests(c.Module())
	if err != nil {
		t.Fatalf("lower with tests: %v", err)
	}
	if len(withTests.Tests) != 1 || withTests.Tests[0].Name != "check" {
		t.Fatalf("withTests tests = %#v, want check", withTests.Tests)
	}
}

func TestLowerModulesWithTestsIncludesEachRootModuleTest(t *testing.T) {
	left := checkedModuleWithPath(t, "demo/left", `
		test fn same() Void!Str { Result::ok(()) }
	`)
	right := checkedModuleWithPath(t, "demo/right", `
		test fn same() Void!Str { Result::ok(()) }
	`)

	program, err := LowerModulesWithTests([]checker.Module{left, right})
	if err != nil {
		t.Fatalf("lower modules with tests: %v", err)
	}

	seen := map[string]bool{}
	for _, test := range program.Tests {
		fn := program.Functions[test.Function]
		module := program.Modules[fn.Module]
		seen[module.Path+"::"+test.Name] = true
	}
	for _, want := range []string{"demo/left::same", "demo/right::same"} {
		if !seen[want] {
			t.Fatalf("lowered tests missing %s: %#v", want, seen)
		}
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

func TestLowerTemplateString(t *testing.T) {
	program := lowerSource(t, `
		let name = "Ada"
		let age = 42
		"{name} is {age}"
	`)

	script := program.Functions[program.Script]
	if script.Body.Result == nil || script.Body.Result.Kind != ExprStrConcat {
		t.Fatalf("script result = %#v, want ExprStrConcat", script.Body.Result)
	}
	if !containsExprKind(script.Body.Result, ExprToStr) {
		t.Fatalf("script result = %#v, want ExprToStr inside concat tree", script.Body.Result)
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

func TestLowerContextualMaybeTypesInNestedExpressions(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn pick(choices: [Dynamic]) Dynamic? {
			let first_choice = match choices.size() {
				0 => maybe::none(),
				_ => maybe::some(choices.at(0)),
			}
			let choice = try first_choice -> _ { maybe::none() }
			maybe::some(choice)
		}
	`)

	pick := findFunction(t, program, "pick")
	if got := testTypeInfo(t, program, pick.Locals[1].Type).Name; got != "Dynamic?" {
		t.Fatalf("first_choice type = %q, want Dynamic?", got)
	}
	if got := testTypeInfo(t, program, pick.Locals[2].Type).Name; got != "Dynamic" {
		t.Fatalf("choice type = %q, want Dynamic", got)
	}
	if got := testTypeInfo(t, program, pick.Body.Stmts[0].Value.Type).Name; got != "Dynamic?" {
		t.Fatalf("first_choice expr type = %q, want Dynamic?", got)
	}
	if got := testTypeInfo(t, program, pick.Body.Stmts[1].Value.Type).Name; got != "Dynamic" {
		t.Fatalf("choice expr type = %q, want Dynamic", got)
	}
	if pick.Body.Result == nil || pick.Body.Result.Target == nil {
		t.Fatalf("result tree missing maybe::some target: %#v", pick.Body.Result)
	}
	if got := testTypeInfo(t, program, pick.Body.Result.Target.Type).Name; got != "Dynamic" {
		t.Fatalf("result some target type = %q, want Dynamic", got)
	}
}

func TestLowerContextualResultTypesInNestedExpressions(t *testing.T) {
	program := lowerSource(t, `
		fn pick(flag: Bool) Int!Str {
			let value = match flag {
				true => Result::ok(1),
				false => Result::err("bad"),
			}
			value
		}
	`)

	pick := findFunction(t, program, "pick")
	if got := testTypeInfo(t, program, pick.Locals[1].Type).Name; got != "Int!Str" {
		t.Fatalf("value local type = %q, want Int!Str", got)
	}
	if got := testTypeInfo(t, program, pick.Body.Stmts[0].Value.Type).Name; got != "Int!Str" {
		t.Fatalf("value expr type = %q, want Int!Str", got)
	}
	if pick.Body.Result == nil {
		t.Fatalf("result = nil")
	}
	if got := testTypeInfo(t, program, pick.Body.Result.Type).Name; got != "Int!Str" {
		t.Fatalf("result type = %q, want Int!Str", got)
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

func TestLowerKeepsSameNamedStructsFromDifferentModulesDistinct(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	modelsDir := filepath.Join(tempDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "inbox.ard"), []byte(`
struct Store {
  item: Str,
}

fn new() Store {
  Store{item: "inbox"}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "issues.ard"), []byte(`
struct Store {
  column: Str,
}

fn new() Store {
  Store{column: "issues"}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	result := parse.Parse([]byte(`
use app/models/inbox
use app/models/issues

fn main() Str {
  let inbox_store = inbox::new()
  let issues_store = issues::new()
  inbox_store.item + issues_store.column
}
`), mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(mainPath, result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}

	program, err := Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}

	var stores []TypeInfo
	for _, typ := range program.Types {
		if typ.Name == "Store" {
			stores = append(stores, typ)
		}
	}
	if len(stores) != 2 {
		t.Fatalf("Store type count = %d, want 2: %#v", len(stores), program.Types)
	}
	paths := map[string]bool{}
	for _, typ := range stores {
		paths[typ.ModulePath] = true
	}
	if !paths["app/models/inbox"] || !paths["app/models/issues"] {
		t.Fatalf("Store module paths = %#v, want inbox and issues", paths)
	}
}

func TestLowerImportedModuleFunctionCanReadModuleLevelLet(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.ard"), []byte(`
let refresh_event = "inbox.refresh"

fn event_name() Str {
  refresh_event
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	result := parse.Parse([]byte(`
use app/feature

fn main() Str {
  feature::event_name()
}
`), mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(mainPath, result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}

	program, err := Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}

	featureFn := findFunction(t, program, "event_name")
	if featureFn.Body.Result == nil || featureFn.Body.Result.Kind != ExprLoadGlobal {
		t.Fatalf("event_name result = %#v, want ExprLoadGlobal", featureFn.Body.Result)
	}
}

func TestLowerImportedGenericModuleFunctionSpecialization(t *testing.T) {
	program := lowerSource(t, `
		use ard/list

		let kept = list::keep([1, 2, 3], fn(x: Int) Bool { x > 1 })
	`)

	keep := findFunction(t, program, "keep")
	if len(keep.Signature.Params) != 2 {
		t.Fatalf("keep param count = %d, want 2", len(keep.Signature.Params))
	}
	// keep is lowered once as a generic definition, so its parameter is a list
	// of the type parameter rather than a monomorphized element type.
	paramType := testTypeInfo(t, program, keep.Signature.Params[0].Type)
	if paramType.Kind != TypeList {
		t.Fatalf("keep param kind = %v, want TypeList", paramType.Kind)
	}
	if typeKind(t, program, paramType.Elem) != TypeParam {
		t.Fatalf("keep param elem = %v, want TypeParam", typeKind(t, program, paramType.Elem))
	}
}

func TestLowerImportedGenericStdlibFunctionLowersAsGoGeneric(t *testing.T) {
	// A generic stdlib function is lowered once as a generic definition (ADR
	// 0031, Phase 2): its signature and body reference type parameters rather
	// than concrete monomorphized types.
	program := lowerSource(t, `
		use ard/list

		fn main() [Int] {
			list::keep([1, 2, 3], fn(value) { value > 1 })
		}
	`)

	keep := findFunction(t, program, "keep")
	if len(keep.Signature.Params) != 2 {
		t.Fatalf("keep param count = %d, want 2", len(keep.Signature.Params))
	}
	if len(keep.TypeParams) == 0 {
		t.Fatalf("keep should be a generic definition with type parameters")
	}
	returnType := testTypeInfo(t, program, keep.Signature.Return)
	if returnType.Kind != TypeList || typeKind(t, program, returnType.Elem) != TypeParam {
		t.Fatalf("keep return = %#v, want [TypeParam]", returnType)
	}
	// The body must reference type parameters, never lower unresolved type
	// variables to Void.
	for _, local := range keep.Locals {
		info := testTypeInfo(t, program, local.Type)
		if info.Kind == TypeList && typeKind(t, program, info.Elem) == TypeVoid {
			t.Fatalf("keep local %s has [Void], want type parameter", local.Name)
		}
	}
}

func TestLowerGenericStructMethodBodyUsesReceiverBindings(t *testing.T) {
	program := lowerSource(t, `
		struct Box {
			item: $T
		}

		impl Box {
			fn get() $T {
				self.item
			}
		}

		fn main() Int {
			let box: Box<Int> = Box{item: 42}
			box.get()
		}
	`)

	// The method is lowered once as a generic definition whose return type is
	// the struct's type parameter (ADR 0031, Phase 3).
	get := findFunction(t, program, "Box.get")
	if len(get.TypeParams) == 0 {
		t.Fatalf("Box.get should be a generic method definition")
	}
	if typeKind(t, program, get.Signature.Return) != TypeParam {
		t.Fatalf("get return kind = %v, want TypeParam", typeKind(t, program, get.Signature.Return))
	}
}

func TestLowerGenericStructMethodLowersOnceAsGeneric(t *testing.T) {
	program := lowerSource(t, `
		struct Box {
			item: $T
		}

		impl Box {
			fn get() $T {
				self.item
			}
		}

		fn get_int(box: Box<Int>) Int {
			box.get()
		}

		fn get_str(box: Box<Str>) Str {
			box.get()
		}
	`)

	intGet := findFunction(t, program, "get_int")
	strGet := findFunction(t, program, "get_str")
	if intGet.Body.Result == nil || intGet.Body.Result.Kind != ExprCall {
		t.Fatalf("get_int result = %#v, want method call", intGet.Body.Result)
	}
	if strGet.Body.Result == nil || strGet.Body.Result.Kind != ExprCall {
		t.Fatalf("get_str result = %#v, want method call", strGet.Body.Result)
	}
	// Both calls resolve to the single generic method definition (no
	// monomorphized specializations), each supplying its own type argument.
	if intGet.Body.Result.Function != strGet.Body.Result.Function {
		t.Fatalf("generic method should lower once, got functions %d and %d", intGet.Body.Result.Function, strGet.Body.Result.Function)
	}
	method := program.Functions[intGet.Body.Result.Function]
	if len(method.TypeParams) == 0 || typeKind(t, program, method.Signature.Return) != TypeParam {
		t.Fatalf("method should be generic with a TypeParam return, got %#v", method.Signature)
	}
	if len(intGet.Body.Result.TypeArgs) != 1 || typeKind(t, program, intGet.Body.Result.TypeArgs[0]) != TypeInt {
		t.Fatalf("get_int call type args = %v, want [Int]", intGet.Body.Result.TypeArgs)
	}
	if len(strGet.Body.Result.TypeArgs) != 1 || typeKind(t, program, strGet.Body.Result.TypeArgs[0]) != TypeStr {
		t.Fatalf("get_str call type args = %v, want [Str]", strGet.Body.Result.TypeArgs)
	}
}

func TestLowerGenericStructMethodOwnGenericSpecializationsDoNotCollapse(t *testing.T) {
	program := lowerSource(t, `
		struct Box {
			item: Int
		}

		impl Box {
			fn echo(value: $U) $U {
				value
			}
		}

		fn echo_int(box: Box) Int {
			box.echo(1)
		}

		fn echo_str(box: Box) Str {
			box.echo("x")
		}
	`)

	intEcho := findFunction(t, program, "echo_int")
	strEcho := findFunction(t, program, "echo_str")
	if intEcho.Body.Result == nil || intEcho.Body.Result.Kind != ExprCall {
		t.Fatalf("echo_int result = %#v, want method call", intEcho.Body.Result)
	}
	if strEcho.Body.Result == nil || strEcho.Body.Result.Kind != ExprCall {
		t.Fatalf("echo_str result = %#v, want method call", strEcho.Body.Result)
	}
	if intEcho.Body.Result.Function == strEcho.Body.Result.Function {
		t.Fatalf("method-local generic specializations collapsed to function %d", intEcho.Body.Result.Function)
	}
	if typeKind(t, program, program.Functions[intEcho.Body.Result.Function].Signature.Return) != TypeInt {
		t.Fatalf("echo_int method return kind = %v, want Int", typeKind(t, program, program.Functions[intEcho.Body.Result.Function].Signature.Return))
	}
	if typeKind(t, program, program.Functions[strEcho.Body.Result.Function].Signature.Return) != TypeStr {
		t.Fatalf("echo_str method return kind = %v, want Str", typeKind(t, program, program.Functions[strEcho.Body.Result.Function].Signature.Return))
	}
}

func TestLowerInstanceMethodKeepsDeclaredTraitParameterType(t *testing.T) {
	program := lowerSource(t, `
		trait View {
			fn render()
		}

		struct Node {
			view: View
		}

		struct Context {}

		impl Context {
			fn add_child(child: View) {
				let node = Node{view: child}
			}
		}

		struct Child {}

		impl View for Child {
			fn render() {}
		}

		fn main() {
			let ctx = Context{}
			ctx.add_child(Child{})
		}
	`)

	addChild := findFunction(t, program, "Context.add_child")
	if len(addChild.Signature.Params) != 2 {
		t.Fatalf("add_child param count = %d, want 2", len(addChild.Signature.Params))
	}
	if kind := typeKind(t, program, addChild.Signature.Params[1].Type); kind != TypeTraitObject {
		t.Fatalf("add_child child param kind = %v, want trait object", kind)
	}
}

func TestLowerSameShapeStructsWithDifferentMethodsStayDistinct(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "first.ard")
	if err := os.WriteFile(firstPath, []byte(`
		struct Item {
			value: Int
		}

		impl Item {
			fn value_plus_one() Int {
				self.value + 1
			}
		}
	`), 0o644); err != nil {
		t.Fatalf("write first module: %v", err)
	}
	secondPath := filepath.Join(root, "second.ard")
	if err := os.WriteFile(secondPath, []byte(`
		struct Item {
			value: Int
		}

		impl Item {
			fn value_text() Str {
				self.value.to_str()
			}
		}
	`), 0o644); err != nil {
		t.Fatalf("write second module: %v", err)
	}

	mainPath := filepath.Join(root, "main.ard")
	result := parse.Parse([]byte(`
		use app/first
		use app/second

		fn read_first(item: first::Item) Int {
			item.value_plus_one()
		}

		fn read_second(item: second::Item) Str {
			item.value_text()
		}
	`), mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(mainPath, result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}

	firstItem := findType(t, program, "Item")
	var secondItem TypeInfo
	for _, typ := range program.Types {
		if typ.Name == "Item" && typ.ID != firstItem.ID {
			secondItem = typ
			break
		}
	}
	if secondItem.ID == NoType {
		t.Fatal("second Item type not found")
	}
	if firstItem.ID == secondItem.ID {
		t.Fatalf("same-shape imported structs collapsed to type %d", firstItem.ID)
	}
	readFirst := findFunction(t, program, "read_first")
	readSecond := findFunction(t, program, "read_second")
	if typeKind(t, program, readFirst.Signature.Params[0].Type) != TypeStruct {
		t.Fatalf("read_first param kind = %v, want struct", typeKind(t, program, readFirst.Signature.Params[0].Type))
	}
	if typeKind(t, program, readSecond.Signature.Params[0].Type) != TypeStruct {
		t.Fatalf("read_second param kind = %v, want struct", typeKind(t, program, readSecond.Signature.Params[0].Type))
	}
	if readFirst.Signature.Params[0].Type == readSecond.Signature.Params[0].Type {
		t.Fatalf("same-shape imported struct params collapsed to type %d", readFirst.Signature.Params[0].Type)
	}
}

func TestLowerTestsManifest(t *testing.T) {
	program := lowerSourceWithTests(t, `
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

func checkedModuleWithPath(t *testing.T, modulePath string, input string) checker.Module {
	t.Helper()
	result := parse.Parse([]byte(input), modulePath+".ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New(modulePath+".ard", result.Program, nil, checker.CheckOptions{ModulePath: modulePath})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	return c.Module()
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

func lowerProjectSource(t *testing.T, root string, input string) *Program {
	t.Helper()
	result := parse.Parse([]byte(input), filepath.Join(root, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New("main.ard", result.Program, resolver)
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

func lowerSourceWithTests(t *testing.T, input string) *Program {
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
	program, err := LowerWithTests(c.Module())
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

func containsExprKind(expr *Expr, kind ExprKind) bool {
	if expr == nil {
		return false
	}
	if expr.Kind == kind {
		return true
	}
	return containsExprKind(expr.Target, kind) ||
		containsExprKind(expr.Left, kind) ||
		containsExprKind(expr.Right, kind) ||
		containsExprKind(expr.Condition, kind) ||
		containsExprKind(expr.Then.Result, kind) ||
		containsExprKind(expr.Else.Result, kind)
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
	return testTypeInfo(t, program, id).Kind
}

func testTypeInfo(t *testing.T, program *Program, id TypeID) TypeInfo {
	t.Helper()
	for _, typ := range program.Types {
		if typ.ID == id {
			return typ
		}
	}
	t.Fatalf("type id %d not found", id)
	return TypeInfo{}
}

func TestLowerRejectsUnboundReturnOnlyGenericWrapper(t *testing.T) {
	result := parse.Parse([]byte(`
		use ard/maybe
		fn raw<$T>(key: Str) $T? { maybe::none() }

		fn has<$T>(key: Str) Bool {
			raw<$T>(key).is_some()
		}

		fn main() {
			let a = has("int")
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	_, err := Lower(c.Module())
	if err == nil {
		t.Fatal("Lower succeeded; expected unbound return-only generic wrapper error")
	}
	if !strings.Contains(err.Error(), "cannot declare unspecialized generic function has") {
		t.Fatalf("Lower error = %v, want unspecialized has", err)
	}
}

func TestLowerImportedGenericStructReturnTypeWithTypeArg(t *testing.T) {
	lowerSource(t, `
use ard/list
fn run() {
  let p = list::partition([1, 2, 3], fn(x: Int) Bool { x > 1 })
}
fn main() { run() }
`)
}
func TestLowerInstanceMethodForwardedGenericTypeArg(t *testing.T) {
	_ = lowerSource(t, `
		struct Box {
			item: $T
		}

		impl Box {
			fn pick<$U>(value: $U) $T {
				self.item
			}
		}

		fn use_pick<$V>(box: Box<Int>, value: $V) Int {
			box.pick<$V>(value)
		}

		fn main() Int {
			let box = Box{item: 1}
			use_pick<Str>(box, "x")
		}
	`)
}
func TestLowerForwardedGenericUsedOnlyInCalleeBody(t *testing.T) {
	_ = lowerSource(t, `
		use ard/maybe
		fn raw<$T>(key: Str) $T? { maybe::none() }

		fn has<$T>() Bool {
			raw<$T>("x").is_some()
		}

		fn outer<$U>() Bool {
			has<$U>()
		}

		fn main() Bool {
			outer<Int>()
		}
	`)
}
func TestLowerReceiverGenericUsedOnlyInMethodBody(t *testing.T) {
	// A generic function called inside a generic method body forwards the
	// struct's type parameter abstractly; this must lower without trying to
	// monomorphize at an unbound type parameter.
	_ = lowerSource(t, `
		use ard/maybe
		fn raw<$T>(key: Str) $T? { maybe::none() }

		struct Box {
			item: $T
		}

		impl Box {
			fn has_raw() Bool {
				raw<$T>("x").is_some()
			}
		}

		fn main() Bool {
			let box = Box{item: 1}
			box.has_raw()
		}
	`)
}
func TestLowerGenericFunctionAdapterClosureCapturesSpecializedCallback(t *testing.T) {
	_ = lowerSource(t, `
		struct StateHandle { id: Int }
		struct BuildContextHandle { id: Int }
		struct BuildContext {}
		struct Widget {}

		struct State<$T> {
			handle: StateHandle,
		}

		fn stateful_raw_init_key<$T>(key: Str, init: fn(BuildContextHandle, StateHandle) $T, build: fn(BuildContextHandle, StateHandle) Widget) Widget {
			let handle = StateHandle{id: 0}
			let ctx = BuildContextHandle{id: 0}
			let _value = init(ctx, handle)
			build(ctx, handle)
		}

		fn stateful<$T>(key: Str, init: fn(BuildContext) $T, build: fn(BuildContext, State<$T>) Widget) Widget {
			stateful_raw_init_key(
				key: key,
				init: fn(_build_ctx: BuildContextHandle, _state_handle: StateHandle) $T {
					init(BuildContext{})
				},
				build: fn(_build_ctx: BuildContextHandle, state_handle: StateHandle) Widget {
					build(BuildContext{}, State{handle: state_handle})
				},
			)
		}

		struct One { n: Int }
		struct Two { s: Str }

		fn main() {
			let _ = stateful(
				key: "one",
				init: fn(_ctx: BuildContext) One { One{n: 1} },
				build: fn(_ctx: BuildContext, _state: State<One>) Widget { Widget{} },
			)
			let _ = stateful(
				key: "two",
				init: fn(_ctx: BuildContext) Two { Two{s: "two"} },
				build: fn(_ctx: BuildContext, _state: State<Two>) Widget { Widget{} },
			)
		}
	`)
}
