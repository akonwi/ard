package checker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestRepeatedVariadicCallSignatureDoesNotCopyDeclarationBody(t *testing.T) {
	body := &Block{}
	declaration := &FunctionDef{
		Name:       "print",
		Parameters: []Parameter{{Name: "value", Type: Any, Variadic: true}},
		ReturnType: Void,
		Body:       body,
	}
	signature := expandFunctionDefForRepeatedVariadic(declaration, 3)
	if signature == declaration {
		t.Fatal("expanded signature aliases declaration")
	}
	if signature.Body != nil {
		t.Fatal("expanded call signature retained declaration body")
	}
	if declaration.Body != body {
		t.Fatal("expanding signature changed declaration body")
	}
}

func TestGenericFunctionCallKeepsCanonicalDeclarationSeparateFromSignature(t *testing.T) {
	result := parse.Parse([]byte(`
		fn invoke_identity() Int {
			identity<Int>(42)
		}

		fn identity(value: $T) $T {
			value
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checker := New("test.ard", result.Program, nil)
	checker.Check()
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	var invokeIdentity, identity *FunctionDef
	for _, statement := range checker.program.Statements {
		function, ok := statement.Expr.(*FunctionDef)
		if !ok {
			continue
		}
		switch function.Name {
		case "invoke_identity":
			invokeIdentity = function
		case "identity":
			identity = function
		}
	}
	if invokeIdentity == nil || identity == nil {
		t.Fatalf("functions = invoke_identity:%p identity:%p, want both declarations", invokeIdentity, identity)
	}
	call, ok := invokeIdentity.Body.Stmts[0].Expr.(*FunctionCall)
	if !ok {
		t.Fatalf("invoke_identity expression = %T, want FunctionCall", invokeIdentity.Body.Stmts[0].Expr)
	}
	assertSeparatedResolvedCall(t, call, identity)
}

func TestFunctionValueCallHasNoSourceDeclaration(t *testing.T) {
	result := parse.Parse([]byte(`
		fn invoke(callback: fn(Int) Int) Int {
			callback(42)
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checker := New("test.ard", result.Program, nil)
	checker.Check()
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	var invoke *FunctionDef
	for _, statement := range checker.program.Statements {
		if function, ok := statement.Expr.(*FunctionDef); ok && function.Name == "invoke" {
			invoke = function
		}
	}
	if invoke == nil {
		t.Fatal("invoke declaration not found")
	}
	call, ok := invoke.Body.Stmts[0].Expr.(*FunctionCall)
	if !ok {
		t.Fatalf("invoke expression = %T, want FunctionCall", invoke.Body.Stmts[0].Expr)
	}
	if declaration := call.Declaration(); declaration != nil {
		t.Fatalf("function-value call declaration = %p, want nil", declaration)
	}
	if call.Signature() == nil {
		t.Fatal("function-value call has no signature")
	}
}

func TestClosureValueCallHasNoSourceDeclaration(t *testing.T) {
	result := parse.Parse([]byte(`
		fn main() Int {
			let callback = fn(value: Int) Int { value }
			callback(42)
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checker := New("test.ard", result.Program, nil)
	checker.Check()
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	var main *FunctionDef
	for _, statement := range checker.program.Statements {
		if function, ok := statement.Expr.(*FunctionDef); ok && function.Name == "main" {
			main = function
		}
	}
	if main == nil {
		t.Fatal("main declaration not found")
	}
	call, ok := main.Body.Stmts[1].Expr.(*FunctionCall)
	if !ok {
		t.Fatalf("main expression = %T, want FunctionCall", main.Body.Stmts[1].Expr)
	}
	if declaration := call.Declaration(); declaration != nil {
		t.Fatalf("closure-value call declaration = %p, want nil", declaration)
	}
	if call.Signature() == nil {
		t.Fatal("closure-value call has no signature")
	}
}

func TestImportedFunctionValueCallHasNoSourceDeclaration(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "feature.ard"), []byte(`
		let callback = fn(value: Int) Int { value }
	`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	result := parse.Parse([]byte(`
		use app/feature

		fn main() Int {
			feature::callback(42)
		}
	`), mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	resolver, err := NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	checker := New(mainPath, result.Program, resolver)
	checker.Check()
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	var main *FunctionDef
	for _, statement := range checker.program.Statements {
		if function, ok := statement.Expr.(*FunctionDef); ok && function.Name == "main" {
			main = function
		}
	}
	if main == nil {
		t.Fatal("main declaration not found")
	}
	moduleCall, ok := main.Body.Stmts[0].Expr.(*ModuleFunctionCall)
	if !ok {
		t.Fatalf("main expression = %T, want ModuleFunctionCall", main.Body.Stmts[0].Expr)
	}
	if declaration := moduleCall.Call.Declaration(); declaration != nil {
		t.Fatalf("imported function-value call declaration = %p, want nil", declaration)
	}
	if moduleCall.Call.Signature() == nil {
		t.Fatal("imported function-value call has no signature")
	}
}

func TestImportedGenericCallKeepsCanonicalDeclarationSeparateFromSignature(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "feature.ard"), []byte(`
		fn identity(value: $T) $T {
			value
		}
	`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	result := parse.Parse([]byte(`
		use app/feature

		fn main() Int {
			feature::identity<Int>(42)
		}
	`), mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	resolver, err := NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	checker := New(mainPath, result.Program, resolver)
	checker.Check()
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	var main *FunctionDef
	for _, statement := range checker.program.Statements {
		if function, ok := statement.Expr.(*FunctionDef); ok && function.Name == "main" {
			main = function
		}
	}
	if main == nil {
		t.Fatal("main declaration not found")
	}
	moduleCall, ok := main.Body.Stmts[0].Expr.(*ModuleFunctionCall)
	if !ok {
		t.Fatalf("main expression = %T, want ModuleFunctionCall", main.Body.Stmts[0].Expr)
	}
	var identity *FunctionDef
	for _, imported := range checker.program.Imports {
		if imported.Path() != "app/feature" || imported.Program() == nil {
			continue
		}
		for _, statement := range imported.Program().Statements {
			if function, ok := statement.Expr.(*FunctionDef); ok && function.Name == "identity" {
				identity = function
			}
		}
	}
	if identity == nil {
		t.Fatal("imported identity declaration not found")
	}
	assertSeparatedResolvedCall(t, moduleCall.Call, identity)
}

func TestSourceFunctionReferencesKeepCanonicalDeclarations(t *testing.T) {
	result := parse.Parse([]byte(`
		fn answer() Int { 40 }

		struct Box {}
		fn Box::answer() Int { 2 }

		fn main() {
			let top_level = answer
			let static = Box::answer
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checker := New("test.ard", result.Program, nil)
	checker.Check()
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	declarations := map[string]*FunctionDef{}
	var main *FunctionDef
	for _, statement := range checker.program.Statements {
		if function, ok := statement.Expr.(*FunctionDef); ok {
			declarations[function.Name] = function
			if function.Name == "main" {
				main = function
			}
		}
	}
	if main == nil || declarations["answer"] == nil || declarations["Box::answer"] == nil {
		t.Fatalf("declarations = %#v, want answer, Box::answer, and main", declarations)
	}
	for index, name := range []string{"answer", "Box::answer"} {
		binding, ok := main.Body.Stmts[index].Stmt.(*VariableDef)
		if !ok {
			t.Fatalf("main statement %d = %T, want VariableDef", index, main.Body.Stmts[index].Stmt)
		}
		reference, ok := binding.Value.(*Variable)
		if !ok {
			t.Fatalf("%s initializer = %T, want Variable", binding.Name, binding.Value)
		}
		if got := reference.Declaration(); got != declarations[name] {
			t.Fatalf("%s declaration = %p, want %p", binding.Name, got, declarations[name])
		}
	}
}

func TestGenericInterpolationMethodKeepsCanonicalDeclaration(t *testing.T) {
	result := parse.Parse([]byte(`
		struct Label<$T> { value: $T }

		impl Label {
			fn to_str() Str { "label" }

			fn render() Str {
				"{self}"
			}
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checker := New("test.ard", result.Program, nil)
	checker.Check()
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	var toStr, render *FunctionDef
	for _, methods := range checker.program.InherentMethods {
		if method := methods["to_str"]; method != nil {
			toStr = method
		}
		if method := methods["render"]; method != nil {
			render = method
		}
	}
	if toStr == nil || render == nil {
		t.Fatalf("methods = to_str:%p render:%p", toStr, render)
	}
	call := interpolationMethodCall(t, render.Body.Stmts[0].Expr)
	assertSeparatedResolvedCall(t, call.Method, toStr)
}

func TestGenericTraitInterpolationKeepsConcreteImplementationDeclaration(t *testing.T) {
	result := parse.Parse([]byte(`
		trait Renderable {
			fn to_str() Str
		}

		struct Label<$T> { value: $T }

		impl Renderable for Label {
			fn to_str() Str { "label" }
		}

		fn render(value: Label<Int>) Str {
			"{value}"
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checker := New("test.ard", result.Program, nil)
	checker.Check()
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	traitSymbol, ok := checker.scope.get("Renderable")
	if !ok {
		t.Fatal("Renderable trait not found")
	}
	trait := traitSymbol.Type.(*Trait)
	var toStr, render *FunctionDef
	for _, methods := range checker.program.TraitMethods {
		if method := methods["to_str"]; method != nil {
			toStr = method
		}
	}
	for _, statement := range checker.program.Statements {
		if function, ok := statement.Expr.(*FunctionDef); ok && function.Name == "render" {
			render = function
		}
	}
	if toStr == nil || render == nil {
		t.Fatalf("functions = to_str:%p render:%p", toStr, render)
	}
	call := interpolationMethodCall(t, render.Body.Stmts[0].Expr)
	assertSeparatedResolvedCall(t, call.Method, toStr)
	if call.DispatchTrait != trait || !call.HasTraitMethodSlot || call.TraitMethodSlot != 0 {
		t.Fatalf("generic trait interpolation dispatch = trait:%p slot:%d resolved:%v", call.DispatchTrait, call.TraitMethodSlot, call.HasTraitMethodSlot)
	}
}

func TestErrorInterpolationKeepsConcreteImplementationDeclaration(t *testing.T) {
	result := parse.Parse([]byte(`
		struct AppError { message: Str }

		impl Error for AppError {
			fn error() Str { self.message }
		}

		fn render(value: AppError) Str {
			"{value}"
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checker := New("test.ard", result.Program, nil)
	checker.Check()
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	var errorImpl, render *FunctionDef
	for _, methods := range checker.program.TraitMethods {
		if method := methods["error"]; method != nil {
			errorImpl = method
		}
	}
	for _, statement := range checker.program.Statements {
		if function, ok := statement.Expr.(*FunctionDef); ok && function.Name == "render" {
			render = function
		}
	}
	if errorImpl == nil || render == nil {
		t.Fatalf("functions = error:%p render:%p", errorImpl, render)
	}
	call := interpolationMethodCall(t, render.Body.Stmts[0].Expr)
	if call.Method.Declaration() != errorImpl {
		t.Fatalf("error interpolation declaration = %p, want concrete implementation %p", call.Method.Declaration(), errorImpl)
	}
	if call.DispatchTrait != BuiltinError || !call.HasTraitMethodSlot || call.TraitMethodSlot != 0 {
		t.Fatalf("error interpolation dispatch = trait:%p slot:%d resolved:%v", call.DispatchTrait, call.TraitMethodSlot, call.HasTraitMethodSlot)
	}
}

func interpolationMethodCall(t *testing.T, expression Expression) *InstanceMethod {
	t.Helper()
	template, ok := expression.(*TemplateStr)
	if !ok {
		t.Fatalf("expression = %T, want TemplateStr", expression)
	}
	for _, chunk := range template.Chunks {
		if method, ok := chunk.(*InstanceMethod); ok {
			return method
		}
	}
	t.Fatalf("template chunks = %#v, want InstanceMethod", template.Chunks)
	return nil
}

func TestTraitCallsKeepCanonicalDeclarationAndMethodSlot(t *testing.T) {
	result := parse.Parse([]byte(`
		trait Pair {
			fn first() Int
			fn second() Int
		}

		struct Box<$T> { value: $T }

		impl Pair for Box {
			fn first() Int {
				self.second()
			}

			fn second() Int {
				42
			}
		}

		fn call_pair(pair: Pair) Int {
			pair.second()
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checker := New("test.ard", result.Program, nil)
	checker.Check()
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	traitSymbol, ok := checker.scope.get("Pair")
	if !ok {
		t.Fatal("Pair trait not found")
	}
	trait, ok := traitSymbol.Type.(*Trait)
	if !ok || len(trait.methods) != 2 {
		t.Fatalf("Pair = %T %#v, want two-method trait", traitSymbol.Type, traitSymbol.Type)
	}

	var firstImpl, secondImpl, callPair *FunctionDef
	for _, methods := range checker.program.TraitMethods {
		if method := methods["first"]; method != nil {
			firstImpl = method
		}
		if method := methods["second"]; method != nil {
			secondImpl = method
		}
	}
	for _, statement := range checker.program.Statements {
		if function, ok := statement.Expr.(*FunctionDef); ok && function.Name == "call_pair" {
			callPair = function
		}
	}
	if firstImpl == nil || secondImpl == nil || callPair == nil {
		t.Fatalf("functions = first:%p second:%p call_pair:%p", firstImpl, secondImpl, callPair)
	}

	concreteCall, ok := firstImpl.Body.Stmts[0].Expr.(*InstanceMethod)
	if !ok {
		t.Fatalf("first expression = %T, want InstanceMethod", firstImpl.Body.Stmts[0].Expr)
	}
	if concreteCall.Method.Declaration() != secondImpl {
		t.Fatalf("concrete trait call declaration = %p, want %p", concreteCall.Method.Declaration(), secondImpl)
	}
	if concreteCall.DispatchTrait != trait {
		t.Fatalf("concrete trait dispatch = %p, want Pair %p", concreteCall.DispatchTrait, trait)
	}
	if !concreteCall.HasTraitMethodSlot || concreteCall.TraitMethodSlot != 1 {
		t.Fatalf("concrete trait method slot = %d (resolved %v), want 1", concreteCall.TraitMethodSlot, concreteCall.HasTraitMethodSlot)
	}

	dynamicCall, ok := callPair.Body.Stmts[0].Expr.(*InstanceMethod)
	if !ok {
		t.Fatalf("call_pair expression = %T, want InstanceMethod", callPair.Body.Stmts[0].Expr)
	}
	if dynamicCall.Method.Declaration() != &trait.methods[1] {
		t.Fatalf("trait-object declaration = %p, want canonical slot %p", dynamicCall.Method.Declaration(), &trait.methods[1])
	}
	if !dynamicCall.HasTraitMethodSlot || dynamicCall.TraitMethodSlot != 1 {
		t.Fatalf("trait-object method slot = %d (resolved %v), want 1", dynamicCall.TraitMethodSlot, dynamicCall.HasTraitMethodSlot)
	}
}

func TestGenericMethodCallKeepsCanonicalDeclarationSeparateFromSignature(t *testing.T) {
	result := parse.Parse([]byte(`
		struct Holder<$T> { items: [$T] }

		impl Holder {
			fn mut reset() {
				self.clear()
			}

			private fn mut clear() {
				self.items = []
			}
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checker := New("test.ard", result.Program, nil)
	checker.Check()
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	var reset, clear *FunctionDef
	for _, methods := range checker.program.InherentMethods {
		if method := methods["reset"]; method != nil {
			reset = method
		}
		if method := methods["clear"]; method != nil {
			clear = method
		}
	}
	if reset == nil || clear == nil {
		t.Fatalf("methods = reset:%p clear:%p, want both declarations", reset, clear)
	}
	if reset.Body == nil || len(reset.Body.Stmts) != 1 {
		t.Fatalf("reset body = %#v, want one call", reset.Body)
	}
	call, ok := reset.Body.Stmts[0].Expr.(*InstanceMethod)
	if !ok {
		t.Fatalf("reset expression = %T, want InstanceMethod", reset.Body.Stmts[0].Expr)
	}
	assertSeparatedResolvedCall(t, call.Method, clear)
}

func assertSeparatedResolvedCall(t *testing.T, call *FunctionCall, declaration *FunctionDef) {
	t.Helper()
	if got := call.Declaration(); got != declaration {
		t.Fatalf("call declaration = %p, want canonical declaration %p", got, declaration)
	}
	if declaration.Body == nil {
		t.Fatal("canonical declaration has no checked body")
	}
	if signature := call.Signature(); signature == nil {
		t.Fatal("call has no specialized signature")
	} else {
		if signature == declaration {
			t.Fatal("generic call signature aliases its canonical declaration")
		}
		if signature.Body != nil {
			t.Fatal("call-local signature retained a declaration body")
		}
	}
}
