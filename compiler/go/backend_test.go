package gotarget

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/frontend"
	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/version"
)

func TestTypesForModuleKeepsOwnedTypesWithOwningModule(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{
			{ID: 0, Path: "a.ard", Types: []air.TypeID{1}},
			{ID: 1, Path: "b.ard", Types: []air.TypeID{2}},
		},
		Types: []air.TypeInfo{
			{ID: 1, Kind: air.TypeStruct, Name: "A"},
			{ID: 2, Kind: air.TypeStruct, Name: "B"},
			{ID: 3, Kind: air.TypeTraitObject, Name: "Synthetic"},
		},
	}
	l := &lowerer{program: program}
	moduleA := l.typesForModule(0, 1)
	if len(moduleA) != 1 || moduleA[0].Name != "A" {
		t.Fatalf("module A types = %#v, want only A", moduleA)
	}
	moduleB := l.typesForModule(1, 1)
	if got := typeNames(moduleB); strings.Join(got, ",") != "B,Synthetic" {
		t.Fatalf("module B types = %v, want B,Synthetic", got)
	}
}

func typeNames(types []air.TypeInfo) []string {
	out := make([]string, len(types))
	for i, typ := range types {
		out[i] = typ.Name
	}
	return out
}

func lowerProgramAST(t testing.TB, program *air.Program, options Options) map[string]*ast.File {
	t.Helper()
	files, err := lowerProgram(program, options)
	if err != nil {
		t.Fatalf("lower program: %v", err)
	}
	return files
}

func astFilesHaveImport(files map[string]*ast.File, alias string, importPath string) bool {
	for _, file := range files {
		if astFileHasImport(file, alias, importPath) {
			return true
		}
	}
	return false
}

type testGoResolver struct {
	packages map[string]*checker.GoPackage
}

func (r testGoResolver) LoadPackage(importPath string) (*checker.GoPackage, error) {
	pkg, ok := r.packages[importPath]
	if !ok {
		return nil, fmt.Errorf("package %q not found", importPath)
	}
	return pkg, nil
}

func astFileHasImport(file *ast.File, alias string, importPath string) bool {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		for _, specNode := range gen.Specs {
			spec, ok := specNode.(*ast.ImportSpec)
			if !ok || spec.Path == nil || strings.Trim(spec.Path.Value, "\"") != importPath {
				continue
			}
			actualAlias := ""
			if spec.Name != nil {
				actualAlias = spec.Name.Name
			}
			if actualAlias == alias {
				return true
			}
		}
	}
	return false
}

func astFilesContain(files map[string]*ast.File, match func(ast.Node) bool) bool {
	for _, file := range files {
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			if match(node) {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func astFilesHaveSelector(files map[string]*ast.File, qualifier string, selectorName string) bool {
	for _, file := range files {
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || selector.Sel == nil || selector.Sel.Name != selectorName {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if ok && ident.Name == qualifier {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func astFilesHaveCall(files map[string]*ast.File, name string) bool {
	return astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return false
		}
		return astCallName(call) == name
	})
}

func astFilesHaveFuncWithPrefix(files map[string]*ast.File, prefix string) bool {
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Name != nil && strings.HasPrefix(fn.Name.Name, prefix) {
				return true
			}
		}
	}
	return false
}

func astFilesHaveFuncContaining(files map[string]*ast.File, part string) bool {
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Name != nil && strings.Contains(fn.Name.Name, part) {
				return true
			}
		}
	}
	return false
}

func astFilesHaveTypeSpec(files map[string]*ast.File, name string) bool {
	return astFilesContain(files, func(node ast.Node) bool {
		typ, ok := node.(*ast.TypeSpec)
		return ok && typ.Name != nil && typ.Name.Name == name
	})
}

func astFilesHaveTypeSwitchCase(files map[string]*ast.File, typeName string) bool {
	return astFilesContain(files, func(node ast.Node) bool {
		clause, ok := node.(*ast.CaseClause)
		if !ok {
			return false
		}
		for _, expr := range clause.List {
			if astExprName(expr) == typeName {
				return true
			}
		}
		return false
	})
}

func astFilesHaveValueSpec(files map[string]*ast.File, name string) bool {
	return astFilesContain(files, func(node ast.Node) bool {
		value, ok := node.(*ast.ValueSpec)
		if !ok {
			return false
		}
		for _, ident := range value.Names {
			if ident.Name == name {
				return true
			}
		}
		return false
	})
}

func astCallName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if ident, ok := fun.X.(*ast.Ident); ok {
			return ident.Name + "." + fun.Sel.Name
		}
		return fun.Sel.Name
	case *ast.IndexExpr:
		return astExprName(fun.X)
	case *ast.IndexListExpr:
		return astExprName(fun.X)
	}
	return ""
}

func astExprName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if ident, ok := e.X.(*ast.Ident); ok {
			return ident.Name + "." + e.Sel.Name
		}
		return e.Sel.Name
	case *ast.IndexExpr:
		return astExprName(e.X)
	case *ast.IndexListExpr:
		return astExprName(e.X)
	case *ast.StarExpr:
		return "*" + astExprName(e.X)
	case *ast.ArrayType:
		return "[]" + astExprName(e.Elt)
	}
	return ""
}

func astFilesFunc(files map[string]*ast.File, name string) (*ast.FuncDecl, bool) {
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Name != nil && fn.Name.Name == name {
				return fn, true
			}
		}
	}
	return nil, false
}

func astFuncHasBlankAssignString(fn *ast.FuncDecl, value string) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if !ok || ident.Name != "_" || i >= len(assign.Rhs) {
				continue
			}
			lit, ok := assign.Rhs[i].(*ast.BasicLit)
			if ok && lit.Kind == token.STRING && lit.Value == value {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func astFuncHasReturnString(fn *ast.FuncDecl, value string) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		ret, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range ret.Results {
			lit, ok := result.(*ast.BasicLit)
			if ok && lit.Kind == token.STRING && lit.Value == value {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func astFilesHaveEmptyStructType(files map[string]*ast.File) bool {
	for _, file := range files {
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			structType, ok := node.(*ast.StructType)
			if ok && (structType.Fields == nil || len(structType.Fields.List) == 0) {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func TestLowerProgramKeepsCrossModuleNestedStructLiteralFields(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"nestlit\"\nard = \">= 0.13.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "inner", "types.ard"), []byte(`
struct Inner {
  a: Int,
  b: Int,
  c: Int,
  d: Int,
}

struct Outer {
  border: Int,
  padding: Inner?,
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	result := parse.Parse([]byte(`
use ard/io
use nestlit/inner/types

fn main() {
  let x = types::Outer{
    border: 1,
    padding: types::Inner{a: 0, b: 1, c: 0, d: 1},
  }
  io::print("border={x.border}")
  io::print("after 1")
  io::print("after 2")
}
`), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New(filepath.Join(tempDir, "main.ard"), result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveFuncContaining(files, "__main") {
		t.Fatal("generated AST missing main body")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		kv, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, keyOK := kv.Key.(*ast.Ident)
		call, callOK := kv.Value.(*ast.CallExpr)
		if !keyOK || key.Name != "Padding" || !callOK || astCallName(call) != "ardruntime.Some" {
			return false
		}
		indexed, ok := call.Fun.(*ast.IndexExpr)
		return ok && astExprName(indexed.Index) == "Inner"
	}) {
		t.Fatal("generated AST missing cross-module nested optional struct literal field")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(astCallName(call), "ard_io__print") || len(call.Args) == 0 {
			return false
		}
		inner, ok := call.Args[0].(*ast.CallExpr)
		if !ok || astCallName(inner) != "any" || len(inner.Args) == 0 {
			return false
		}
		lit, ok := inner.Args[0].(*ast.BasicLit)
		return ok && lit.Value == `"after 2"`
	}) {
		t.Fatal("generated AST truncated statements after nested struct literal")
	}
}

func TestLowerProgramTakesAddressOfLocalMutTraitArgs(t *testing.T) {
	program := lowerSource(t, `
		struct Counter { value: Int }

		impl Counter {
			fn mut bump() { self.value = self.value + 1 }
		}

		trait Bumpable {
			fn poke(mut c: Counter)
		}

		struct Doubler {}

		impl Bumpable for Doubler {
			fn poke(mut c: Counter) {
				c.bump()
				c.bump()
			}
		}

		fn main() {
			mut c = Counter{value: 0}
			let d: Bumpable = Doubler{}
			d.poke(c)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(astCallName(call), "poke") {
			return false
		}
		for _, arg := range call.Args {
			addr, ok := arg.(*ast.UnaryExpr)
			if !ok || addr.Op != token.AND {
				continue
			}
			ident, identOK := addr.X.(*ast.Ident)
			if identOK && ident.Name == "c_0" {
				return true
			}
		}
		return false
	}) {
		t.Fatal("generated AST missing address-of for local mutable trait dispatch arg")
	}
}

func TestLowerProgramPassesMutTraitArgsByPointer(t *testing.T) {
	program := lowerSource(t, `
		struct Counter { value: Int }

		impl Counter {
			fn mut bump() { self.value = self.value + 1 }
		}

		trait Bumpable {
			fn poke(mut c: Counter)
		}

		struct Doubler {}

		impl Bumpable for Doubler {
			fn poke(mut c: Counter) {
				c.bump()
				c.bump()
			}
		}

		fn invoke(b: Bumpable, mut c: Counter) {
			b.poke(c)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(astCallName(call), "Doubler_Bumpable_poke") || len(call.Args) < 2 {
			return false
		}
		ident, ok := call.Args[1].(*ast.Ident)
		return ok && ident.Name == "c_1"
	}) {
		t.Fatal("generated AST missing pointer trait dispatch arg")
	}
	if astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(astCallName(call), "Doubler_Bumpable_poke") || len(call.Args) < 2 {
			return false
		}
		star, ok := call.Args[1].(*ast.StarExpr)
		if !ok {
			return false
		}
		ident, identOK := star.X.(*ast.Ident)
		return identOK && ident.Name == "c_1"
	}) {
		t.Fatal("generated AST dereferences mutable trait dispatch arg")
	}
}

func TestLowerProgramDereferencesMutParamForNonMutMethodCall(t *testing.T) {
	program := lowerSource(t, `
		struct Box {
			value: Int,
		}

		impl Box {
			fn mut bump() {
				self.value = self.value + 1
			}

			fn peek() Int {
				self.value
			}
		}

		fn process(mut b: Box) Int {
			b.bump()
			b.peek()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(astCallName(call), "Box_bump") || len(call.Args) == 0 {
			return false
		}
		ident, ok := call.Args[0].(*ast.Ident)
		return ok && ident.Name == "b_0"
	}) {
		t.Fatal("generated AST missing mut method pointer call")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(astCallName(call), "Box_peek") || len(call.Args) == 0 {
			return false
		}
		star, ok := call.Args[0].(*ast.StarExpr)
		if !ok {
			return false
		}
		ident, identOK := star.X.(*ast.Ident)
		return identOK && ident.Name == "b_0"
	}) {
		t.Fatal("generated AST missing deref for non-mut method call on mut param")
	}
}

func TestGenerateSourcesFormatsSimpleProgram(t *testing.T) {
	program := lowerSource(t, `
		fn add(a: Int, b: Int) Int {
			a + b
		}

		fn main() Int {
			add(1, 2)
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source, ok := sources["test.go"]
	if !ok {
		t.Fatalf("generated sources missing test.go: %#v", mapsKeys(sources))
	}
	got := string(source)
	if !strings.Contains(got, "package main") {
		t.Fatalf("generated source missing package declaration:\n%s", got)
	}
	if !strings.Contains(got, "func test_ard__add(a_0 int, b_1 int) int") {
		t.Fatalf("generated source missing lowered add function:\n%s", got)
	}
	if !strings.Contains(got, "return a_0 + b_1") {
		t.Fatalf("generated source missing arithmetic return:\n%s", got)
	}
	if !strings.Contains(got, "func main()") {
		t.Fatalf("generated source missing Go main wrapper:\n%s", got)
	}
}

func TestLowerProgramOmitsTestsUnlessIncluded(t *testing.T) {
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
	program, err := air.LowerWithTests(c.Module())
	if err != nil {
		t.Fatalf("lower with tests: %v", err)
	}

	productionFiles := lowerProgramAST(t, program, Options{PackageName: "main"})
	if _, ok := astFilesFunc(productionFiles, "test_ard__check"); ok {
		t.Fatal("production AST includes test function")
	}

	testFiles := lowerProgramAST(t, program, Options{PackageName: "main", IncludeTests: true, SuppressMain: true})
	if _, ok := astFilesFunc(testFiles, "test_ard__check"); !ok {
		t.Fatal("test AST missing test function")
	}
}

func TestLowerProgramDiscardsFinalExprInVoidFunction(t *testing.T) {
	program := lowerSource(t, `
		fn main() {
			"Hello"
		}
	`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	fn, ok := astFilesFunc(files, "test_ard__main")
	if !ok {
		t.Fatal("generated AST missing main function")
	}
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		t.Fatalf("generated AST gives void main a return type: %#v", fn.Type.Results)
	}
	if !astFuncHasBlankAssignString(fn, `"Hello"`) {
		t.Fatalf("generated AST does not discard final expression: %#v", fn.Body)
	}
	if astFuncHasReturnString(fn, `"Hello"`) {
		t.Fatalf("generated AST returns final expression from void function: %#v", fn.Body)
	}
	if astFilesHaveEmptyStructType(files) {
		t.Fatal("generated AST still uses anonymous empty struct for Void")
	}
}

func TestLowerProgramUsesRuntimeVoidForVoidResultValues(t *testing.T) {
	program := lowerSource(t, `
		fn ok() Void!Str {
			Result::ok(())
		}

		fn main() Void {
			ok()
		}
	`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	fn, ok := astFilesFunc(files, "test_ard__ok")
	if !ok || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		t.Fatalf("generated AST missing ok return type: %#v", fn)
	}
	resultType, ok := fn.Type.Results.List[0].Type.(*ast.IndexListExpr)
	if !ok || astExprName(resultType.X) != "ardruntime.Result" || len(resultType.Indices) != 2 || astExprName(resultType.Indices[0]) != "ardruntime.Void" || astExprName(resultType.Indices[1]) != "string" {
		t.Fatalf("generated AST missing void result container return type using ardruntime.Void: %#v", fn.Type.Results.List[0].Type)
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		kv, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, keyOK := kv.Key.(*ast.Ident)
		lit, litOK := kv.Value.(*ast.CompositeLit)
		return keyOK && key.Name == "Value" && litOK && astExprName(lit.Type) == "ardruntime.Void"
	}) {
		t.Fatal("generated AST missing ardruntime.Void value")
	}
	if astFilesHaveEmptyStructType(files) {
		t.Fatal("generated AST still uses anonymous empty struct for Void")
	}
}

func TestLowerProgramMaterializesVoidGlobalInitializers(t *testing.T) {
	program := lowerSource(t, `
		fn touch() Void { () }
		let saved = touch()
		fn main() Void { saved }
	`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if astFilesContain(files, func(node ast.Node) bool {
		value, ok := node.(*ast.ValueSpec)
		if !ok {
			return false
		}
		for _, expr := range value.Values {
			call, ok := expr.(*ast.CallExpr)
			if ok && strings.Contains(astCallName(call), "test_ard__touch") {
				return true
			}
		}
		return false
	}) {
		t.Fatal("generated AST uses no-value Void call as global initializer")
	}
	if !astFilesHaveCall(files, "test_ard__touch") {
		t.Fatal("generated AST does not materialize Void global initializer call")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		ret, ok := node.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return false
		}
		lit, ok := ret.Results[0].(*ast.CompositeLit)
		return ok && astExprName(lit.Type) == "ardruntime.Void"
	}) {
		t.Fatal("generated AST does not return ardruntime.Void{} for materialized global")
	}
}

func TestRenderTestRunnerUsesRuntimeVoidForVoidResult(t *testing.T) {
	result := parse.Parse([]byte(`
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
	program, err := air.LowerWithTests(c.Module())
	if err != nil {
		t.Fatalf("lower with tests: %v", err)
	}
	runner := renderTestRunner(program, []TestCase{{Name: "check", DisplayName: "check", Function: program.Tests[0].Function}}, false)
	if !strings.Contains(runner, "func() runtime.Result[runtime.Void, string]") {
		t.Fatalf("test runner missing void result container using runtime.Void:\n%s", runner)
	}
	if strings.Contains(runner, "struct{}") || strings.Contains(runner, "struct {}") {
		t.Fatalf("test runner still uses anonymous empty struct for Void:\n%s", runner)
	}
}

func TestRunProgramExecutesSimpleMain(t *testing.T) {
	program := lowerSource(t, `
		fn main() Void {
			()
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSupportsModuleLevelLetCapturedByClosure(t *testing.T) {
	program := lowerSource(t, `
		let refresh_event = "inbox.refresh"

		fn main() {
			let event = refresh_event
			let read: fn() Str = fn() { event }
			if not read() == "inbox.refresh" {
				panic("wrong event")
			}
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSupportsTransitiveSameNamedStructsFromDifferentModules(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"models", "tui"} {
		if err := os.MkdirAll(filepath.Join(tempDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"models/inbox.ard": `
struct Store {
  item: Str,
}

fn new() Store {
  Store{item: "inbox"}
}
`,
		"models/issues.ard": `
struct Store {
  column: Str,
}

fn new() Store {
  Store{column: "issues"}
}
`,
		"tui/inbox_screen.ard": `
use app/models/inbox

struct Screen {
  store: inbox::Store,
}

fn new() Screen {
  Screen{store: inbox::new()}
}

impl Screen {
  fn item() Str { self.store.item }
}
`,
		"tui/issues_screen.ard": `
use app/models/issues

struct Screen {
  store: issues::Store,
}

fn new() Screen {
  Screen{store: issues::new()}
}

impl Screen {
  fn column() Str { self.store.column }
}
`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(tempDir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/tui/inbox_screen
use app/tui/issues_screen

fn main() {
  let inbox = inbox_screen::new()
  let issues = issues_screen::new()
  if not inbox.item() == "inbox" {
    panic("wrong inbox screen")
  }
  if not issues.column() == "issues" {
    panic("wrong issues screen")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSupportsSameNamedStructMethodsFromDifferentModules(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, module := range []struct{ name, label string }{{"left", "left"}, {"right", "right"}} {
		source := fmt.Sprintf(`
struct Store {
  label: Str,
}

fn new() Store {
  Store{label: %q}
}

impl Store {
  fn value() Str { self.label }
}
`, module.label)
		if err := os.WriteFile(filepath.Join(tempDir, module.name+".ard"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/left
use app/right

fn main() {
  if not left::new().value() == "left" {
    panic("wrong left value")
  }
  if not right::new().value() == "right" {
    panic("wrong right value")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSupportsSameNamedStructsFromDifferentModules(t *testing.T) {
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
	if err := os.WriteFile(mainPath, []byte(`
use app/models/inbox
use app/models/issues

fn main() {
  let inbox_store = inbox::new()
  let issues_store = issues::new()
  if not inbox_store.item == "inbox" {
    panic("wrong inbox store")
  }
  if not issues_store.column == "issues" {
    panic("wrong issues store")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSupportsImportedModuleLevelLetCapturedByClosure(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.ard"), []byte(`
let refresh_event = "inbox.refresh"

fn run() {
  let event = refresh_event
  let read: fn() Str = fn() { event }
  if not read() == "inbox.refresh" {
    panic("wrong event")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn main() {
  feature::run()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSupportsModuleGlobalInitializerCallingInstanceMethod(t *testing.T) {
	program := lowerSource(t, `
		struct Source {}

		impl Source {
			fn value() Str { "ok" }
		}

		let source = Source{}
		let saved = source.value()

		fn main() {
			if not saved == "ok" {
				panic("wrong saved value")
			}
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSupportsImportedTraitObjectModuleGlobal(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.ard"), []byte(`
trait Named {
  fn name() Str
}

struct Item {}

impl Named for Item {
  fn name() Str { "item" }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

let saved: feature::Named = feature::Item{}

fn main() {
  if not saved.name() == "item" {
    panic("wrong saved trait")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSupportsImportedFunctionSymbolReadingModuleLevelLet(t *testing.T) {
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
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn main() {
  let event_name: fn() Str = feature::event_name
  if not event_name() == "inbox.refresh" {
    panic("wrong event")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSupportsImportedFunctionValuedModuleLet(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.ard"), []byte(`
let handler: fn() Str = fn() { "ok" }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn main() {
  let handler: fn() Str = feature::handler
  if not handler() == "ok" {
    panic("wrong handler symbol")
  }
  if not feature::handler() == "ok" {
    panic("wrong handler call")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSupportsImportedTraitMethodReadingModuleLevelLet(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.ard"), []byte(`
let label = "imported"

trait Named {
  fn name() Str
}

struct Item {}

impl Named for Item {
  fn name() Str { label }
}

fn run() {
  let item: Named = Item{}
  if not item.name() == "imported" {
    panic("wrong trait name")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn main() { feature::run() }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSupportsImportedInstanceMethodReadingModuleLevelLet(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.ard"), []byte(`
let label = "instance"

struct Item {}

impl Item {
  fn name() Str { label }
}

fn make() Item { Item{} }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn main() {
  let item = feature::make()
  if not item.name() == "instance" {
    panic("wrong instance name")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSpecializesGenericEmptyListLocal(t *testing.T) {
	program := lowerSource(t, `
		fn drop(from: [$T], till: Int) [$T] {
			mut out: [$T] = []
			for item, idx in from {
				if idx >= till {
					out.push(item)
				}
			}
			out
		}

		fn main() Bool {
			let dropped = drop([1, 2, 3], 1)
			dropped.size() == 2 and dropped.at(0) == 2
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestBuildProgramSpecializesNestedGenericLambdasPerOuterBinding(t *testing.T) {
	workspace := t.TempDir()
	sharedDir := filepath.Join(workspace, "state-shared")
	appDir := filepath.Join(workspace, "state-app")
	for _, dir := range []string{sharedDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "ard.toml"), []byte("name = \"state-shared\"\nard = \">= 0.23.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "shared.ard"), []byte(`struct State<$T> { handle: Int }

fn _stateful<$T>(
  init: fn(Int) $T,
  build: fn(Int) Int,
) Int {
  let _ = init(0)
  build(0)
}

fn stateful<$T>(
  init: fn() $T,
  build: fn(State<$T>) Int,
) Int {
  _stateful(
    init: fn(h: Int) $T {
      init()
    },
    build: fn(h: Int) Int {
      build(State{handle: h})
    },
  )
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"state-app\"\nard = \">= 0.23.0\"\n\n[dependencies]\nstate-shared = { path = \"../state-shared\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"main.ard": `use state-app/a
use state-app/b

fn main() Void {
  a::go()
  b::go()
}
`,
		"a.ard": `use state-shared/shared

struct State { x: Int }

fn go() Void {
  let _ = shared::stateful(
    fn() State { State{x: 1} },
    fn(s: shared::State<State>) Int { 0 },
  )
}
`,
		"b.ard": `use state-shared/shared

struct State { y: Str }

fn go() Void {
  let _ = shared::stateful(
    fn() State { State{y: "hi"} },
    fn(s: shared::State<State>) Int { 0 },
  )
}
`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(appDir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mainPath := filepath.Join(appDir, "main.ard")
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestRunProgramAllowsModuleWithoutEntry(t *testing.T) {
	program := lowerSource(t, `
		fn add(a: Int, b: Int) Int {
			a + b
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestLowerProgramSupportsStructsAndEnums(t *testing.T) {
	program := lowerSource(t, `
		enum Direction {
			Up, Down
		}

		struct User {
			name: Str,
			age: Int,
		}

		fn direction() Direction {
			Direction::Down
		}

		fn next_age() Int {
			let user = User{name: "Ada", age: 41}
			user.age + 1
		}

		fn main() Int {
			next_age()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveTypeSpec(files, "Direction") {
		t.Fatal("generated AST missing enum type")
	}
	if !astFilesHaveValueSpec(files, "Direction__Down") {
		t.Fatal("generated AST missing enum constants")
	}
	if !astFilesHaveTypeSpec(files, "User") {
		t.Fatal("generated AST missing struct type")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		lit, ok := node.(*ast.CompositeLit)
		if !ok || astExprName(lit.Type) != "User" {
			return false
		}
		hasName := false
		hasAge := false
		for _, elem := range lit.Elts {
			kv, ok := elem.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, keyOK := kv.Key.(*ast.Ident)
			if !keyOK {
				continue
			}
			if key.Name == "Name" {
				value, ok := kv.Value.(*ast.BasicLit)
				hasName = ok && value.Value == `"Ada"`
			}
			if key.Name == "Age" {
				value, ok := kv.Value.(*ast.BasicLit)
				hasAge = ok && value.Value == "41"
			}
		}
		return hasName && hasAge
	}) {
		t.Fatal("generated AST missing struct literal lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		binary, ok := node.(*ast.BinaryExpr)
		if !ok || binary.Op != token.ADD {
			return false
		}
		selector, ok := binary.X.(*ast.SelectorExpr)
		return ok && selector.Sel.Name == "Age"
	}) {
		t.Fatal("generated AST missing field access lowering")
	}
}

func TestLowerProgramSupportsTryMaybeCatchAndEarlyReturn(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn missing() Int? {
			maybe::none()
		}

		fn with_default() Int {
			let value = try missing() -> _ { 42 }
			value
		}

		fn passthrough() Int? {
			let value = try missing()
			maybe::some(value)
		}

		fn main() Int {
			with_default()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		ret, ok := node.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return false
		}
		ident, ok := ret.Results[0].(*ast.Ident)
		return ok && strings.HasPrefix(ident.Name, "_tmp_")
	}) {
		t.Fatal("generated AST missing try early return lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return false
		}
		for _, rhs := range assign.Rhs {
			lit, ok := rhs.(*ast.BasicLit)
			if ok && lit.Value == "42" {
				return true
			}
		}
		return false
	}) {
		t.Fatal("generated AST missing try catch lowering")
	}
}

func TestLowerProgramPropagatesTryResultAcrossDifferentResultValueTypes(t *testing.T) {
	program := lowerSource(t, `
		fn read_text() Str!Str {
			Result::err("bad")
		}

		fn parse() Int!Str {
			let text = try read_text()
			let _ignore = text
			Result::ok(1)
		}

		fn main() Int!Str {
			parse()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		ret, ok := node.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return false
		}
		lit, ok := ret.Results[0].(*ast.CompositeLit)
		if !ok || astExprName(lit.Type) != "ardruntime.Result" {
			return false
		}
		for _, elem := range lit.Elts {
			kv, ok := elem.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, keyOK := kv.Key.(*ast.Ident)
			if !keyOK || key.Name != "Err" {
				continue
			}
			if value, ok := kv.Value.(*ast.Ident); ok && strings.HasPrefix(value.Name, "_tmp_") {
				return true
			}
			if selector, ok := kv.Value.(*ast.SelectorExpr); ok {
				if ident, ok := selector.X.(*ast.Ident); ok && strings.HasPrefix(ident.Name, "_tmp_") && selector.Sel.Name == "Err" {
					return true
				}
			}
		}
		return false
	}) {
		t.Fatal("generated AST missing result error propagation conversion")
	}
}

func TestRunProgramSupportsCommonStdlibExterns(t *testing.T) {
	program := lowerSource(t, `
		use ard/argv
		use ard/base64
		use ard/dynamic
		use ard/env
		use ard/float
		use ard/hex

		fn main() Bool {
			let encoded = base64::encode("hi".bytes(), true)
			let decoded = base64::decode(encoded, true).expect("decode")
			let hexed = hex::encode(decoded)
			let unhex = hex::decode(hexed).expect("hex")
			let unhex_text = Str::from_bytes(unhex).expect("utf8")
			let args = argv::os_args()
			let _path = env::get("PATH")
			let parsed = float::from_str("3.5").or(0.0)
			let floored = float::floor(parsed)
			let _dyn_list = dynamic::from_list([dynamic::from_str(unhex_text)])
			let _dyn_map = dynamic::object(["value": dynamic::from_int(args.size())])
			unhex_text == "hi" and floored == 3.0 and args.size() >= 0
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestLowerProgramReusesJSONGlueHelpers(t *testing.T) {
	program := lowerSource(t, `
		use ard/json

		struct Item { name: Str }
		struct Payload { items: [Item], note: Str? }

		fn main() Bool {
			let parsed = json::parse<Payload>("\{\"items\":[\{\"name\":\"one\"\}],\"note\":null\}").expect("parse")
			let encoded = json::encode(parsed).expect("encode")
			encoded.size() > 0
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	for _, name := range []string{"ardJSONDecodeMaybe", "ardJSONDecodeList", "ardJSONEncodeMaybe", "ardJSONEncodeList"} {
		if _, ok := astFilesFunc(files, name); !ok {
			t.Fatalf("generated AST missing reusable JSON glue function %q", name)
		}
		if !astFilesHaveCall(files, name) {
			t.Fatalf("generated AST missing reusable JSON glue call %q", name)
		}
	}
}

func TestBuildProgramCompilesJSONPreludeForStdlibBackedHTTPTypes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/http
use ard/json

struct App {
  routes: [Str: fn(http::Request, mut http::Response)]
}

fn main() Str {
  json::encode(Dynamic::from("ok")).or("")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(dir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestBuildProgramLowersTransitiveStdlibExternFromSubmodule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"sleep-repro\"\nard = \">= 0.13.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib.ard"), []byte(`use ard/async

fn tick() Void {
  async::sleep(1000000)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use sleep-repro/lib

fn main() Void {
  lib::tick()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(dir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestBuildProgramLowersOptionMatchArmModuleExternCall(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use ard/decode

fn nested_name(obj: Dynamic, field: Str) Str {
  let nested = decode::run(obj, decode::field(field, decode::nullable(decode::dynamic)))
    .expect("Missing nested field")
  match nested {
    n => decode::run(n, decode::field("name", decode::string)).expect("Missing nested name"),
    _ => "",
  }
}

fn main() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(dir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestLowerProgramUsesProjectNameForProjectFFI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo_app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

type Handle struct{}

func MakeHandle() Handle { return Handle{} }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`extern type Handle = "demo_app.Handle"
extern fn make_handle() Handle = "demo_app.MakeHandle"

struct Box {
  handle: Handle,
}

fn main() {
  let handle: Handle = make_handle()
  let _ = Box{handle: handle}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	files := lowerProgramAST(t, program, Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo})
	if !astFilesHaveImport(files, "demo_app", "generated/demo_app") {
		t.Fatalf("generated AST missing project-name FFI import")
	}
	if !astFilesHaveSelector(files, "demo_app", "Handle") || !astFilesHaveSelector(files, "demo_app", "MakeHandle") {
		t.Fatalf("generated AST did not qualify project FFI with project name")
	}
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeProgram(workspace, program, Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo}); err != nil {
		t.Fatalf("write program: %v", err)
	}
	if !fileExists(filepath.Join(workspace, "demo_app", "ffi.go")) {
		t.Fatalf("project FFI companion was not copied to project-named package")
	}
	if fileExists(filepath.Join(workspace, "projectffi", "ffi.go")) {
		t.Fatalf("project FFI companion was copied to legacy projectffi package")
	}
}

func TestLowerProgramRejectsUnqualifiedProjectFFIExternType(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

type Handle struct{}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`extern type Handle = "Handle"

struct Box {
  handle: Handle,
}

fn main() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	_, err = lowerProgram(program, Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo})
	if err == nil || !strings.Contains(err.Error(), `must qualify Handle with package demo`) {
		t.Fatalf("GenerateSources error = %v, want unqualified project FFI type rejection", err)
	}
}

func TestArtifactWorkspacePreservesGoModuleFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := artifactWorkspace(dir, "run")
	if err != nil {
		t.Fatalf("artifact workspace: %v", err)
	}
	goMod := []byte("module generated\n\nrequire example.com/cached v1.0.0\n")
	goSum := []byte("example.com/cached v1.0.0 h1:abc\n")
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), goMod, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "go.sum"), goSum, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "stale.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	workspace, err = artifactWorkspace(dir, "run")
	if err != nil {
		t.Fatalf("artifact workspace: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "go.mod")); err != nil || string(got) != string(goMod) {
		t.Fatalf("preserved go.mod = %q, %v", string(got), err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "go.sum")); err != nil || string(got) != string(goSum) {
		t.Fatalf("preserved go.sum = %q, %v", string(got), err)
	}
	if fileExists(filepath.Join(workspace, "stale.go")) {
		t.Fatal("artifact workspace kept stale generated file")
	}
}

func TestLowerProgramRejectsUnqualifiedProjectFFIExternFunction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

func Lookup() string { return "ok" }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`extern fn lookup() Str = "Lookup"

fn main() Str { lookup() }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	_, err = lowerProgram(program, Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo})
	if err == nil || !strings.Contains(err.Error(), `must qualify Lookup with package demo`) {
		t.Fatalf("GenerateSources error = %v, want unqualified project FFI function rejection", err)
	}
}

func TestWriteProgramCarriesProjectAndGeneratedGoModuleState(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(`module demo

go 1.26.0

require (
	example.com/direct v1.2.3
	example.com/indirect v0.1.0 // indirect
)

replace example.com/direct => ../localdep
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), []byte("example.com/direct v1.2.3 h1:project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

import _ "example.com/direct"

func Lookup() string { return "ok" }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`extern fn lookup() Str = "demo.Lookup"

fn main() Str { lookup() }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte(`module generated

go 1.26.0

require (
	example.com/direct v0.0.1
	example.com/inferred v0.9.0
)
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "go.sum"), []byte("example.com/inferred v0.9.0 h1:generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeProgram(workspace, program, Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo}); err != nil {
		t.Fatalf("write program: %v", err)
	}
	goMod, err := os.ReadFile(filepath.Join(workspace, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	goModText := string(goMod)
	for _, want := range []string{
		"example.com/direct v1.2.3",
		"example.com/indirect v0.1.0 // indirect",
		"example.com/inferred v0.9.0",
		"example.com/direct => " + filepath.Clean(filepath.Join(dir, "..", "localdep")),
	} {
		if !strings.Contains(goModText, want) {
			t.Fatalf("generated go.mod missing %q:\n%s", want, goModText)
		}
	}
	if strings.Contains(goModText, "example.com/direct v0.0.1") {
		t.Fatalf("generated go.mod kept stale project requirement version:\n%s", goModText)
	}
	goSum, err := os.ReadFile(filepath.Join(workspace, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	goSumText := string(goSum)
	for _, want := range []string{"example.com/direct v1.2.3 h1:project", "example.com/inferred v0.9.0 h1:generated"} {
		if !strings.Contains(goSumText, want) {
			t.Fatalf("generated go.sum missing %q:\n%s", want, goSumText)
		}
	}
}

func TestWriteProgramCopiesProjectQualifiedExternTypeFFI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

type Handle struct{}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`extern type Handle = "demo.Handle"

struct Box {
  handle: Handle,
}

fn main() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeProgram(workspace, program, Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo}); err != nil {
		t.Fatalf("write program: %v", err)
	}
	if !fileExists(filepath.Join(workspace, "demo", "ffi.go")) {
		t.Fatalf("project-qualified extern type did not cause project FFI companion copy")
	}
	if err := buildGeneratedProgram(workspace, filepath.Join(dir, "app")); err != nil {
		t.Fatalf("build generated program: %v", err)
	}
}

func TestBuildProgramImportsProjectFFIForExternTypesOnlyUsedAsTypes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(dir, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "host.go"), []byte(`package ffi

type Handle struct {
	Name string
}

func MakeHandle(name string) (*Handle, error) {
	return &Handle{Name: name}, nil
}

func HandleName(h *Handle) string {
	return h.Name
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib.ard"), []byte(`extern type Handle = "*demo.Handle"

extern fn make_handle_raw(name: Str) Handle!Str = "demo.MakeHandle"
extern fn handle_name(h: Handle) Str = "demo.HandleName"

struct KeyEvent { name: Str }
struct QuitEvent {}

type Event = KeyEvent | QuitEvent

fn next_event(name: Str) Event!Str {
  let h = try make_handle_raw(name)
  let ev: Event = KeyEvent{name: handle_name(h)}
  Result::ok(ev)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use ard/io
use ard/json
use demo/lib

fn main() {
  match lib::next_event("hello").expect("ev") {
    KeyEvent(k) => {
      let s = json::encode(k).expect("enc")
      io::print(s)
    },
    QuitEvent(_) => io::print("quit"),
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	builtPath, err := BuildProgram(program, filepath.Join(dir, "app"), loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramJSONEncodeDoesNotStealStdRuntimeImport(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/json

extern type Stats = "runtime.MemStats"
extern fn stats() Stats = "demo.Stats"

fn main() Bool {
	let _stats = stats()
	json::encode(1).expect("json") == "1"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

import "runtime"

func Stats() runtime.MemStats {
	return runtime.MemStats{}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	builtPath, err := BuildProgram(program, filepath.Join(dir, "app"), loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramSupportsProjectGoFFIWithTypedExternType(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
extern type Buffer = "*bytes.Buffer"

extern fn new_buffer() Buffer!Str = "demo.NewBuffer"
extern fn buffer_len(buffer: Buffer) Int = "demo.BufferLen"

fn main() Bool {
	let buffer = new_buffer().expect("buffer")
	buffer_len(buffer) == 0
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

import "bytes"

func NewBuffer() (*bytes.Buffer, error) {
	return &bytes.Buffer{}, nil
}

func BufferLen(buffer *bytes.Buffer) int {
	return buffer.Len()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	binaryPath := filepath.Join(dir, "app")
	builtPath, err := BuildProgram(program, binaryPath, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramEmitsTypeArgsForReturnOnlyGenericProjectExtern(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
extern type RawState = "demo.StateContext"
extern fn new_raw_state() RawState = "demo.NewRawState"
extern fn get_raw<$T>(state: RawState, key: Str) $T? = "demo.GetRaw"

struct State {
	raw: RawState,
}

fn state() State {
	State{raw: new_raw_state()}
}

impl State {
	fn get<$T>(key: Str) $T? {
		get_raw<$T>(self.raw, key)
	}
}

fn main() Bool {
	state().get<Int>("count").or(0) == 42
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

import ardruntime "github.com/akonwi/ard/runtime"

type StateContext struct{}

func NewRawState() StateContext {
	return StateContext{}
}

func GetRaw[T any](state StateContext, key string) ardruntime.Maybe[T] {
	if key != "count" {
		return ardruntime.None[T]()
	}
	return ardruntime.Some(any(42).(T))
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	binaryPath := filepath.Join(dir, "app")
	builtPath, err := BuildProgram(program, binaryPath, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramKeepsReturnOnlyGenericWrapperSpecializations(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
extern fn get_raw<$T>(key: Str) $T? = "demo.GetRaw"

fn id<$T>(key: Str) $T? {
	get_raw<$T>(key)
}

fn has<$T>(key: Str) Bool {
	id<$T>(key).is_some()
}

fn outer<$U>(key: Str) Bool {
	has<$U>(key)
}

fn main() Bool {
	outer<Int>("int") and outer<Str>("str")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

import ardruntime "github.com/akonwi/ard/runtime"

func GetRaw[T any](key string) ardruntime.Maybe[T] {
	switch key {
	case "int":
		return ardruntime.Some(any(1).(T))
	case "str":
		return ardruntime.Some(any("ok").(T))
	default:
		return ardruntime.None[T]()
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	binaryPath := filepath.Join(dir, "app")
	builtPath, err := BuildProgram(program, binaryPath, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramWrapsProjectFFIRawChannelReturn(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/async/channel
use ard/io

extern type RawEvent = "demo.Event"
extern fn events() channel::Channel<RawEvent> = "demo.Events"
extern fn event_value(e: RawEvent) Int = "demo.EventValue"

fn main() {
	let ch = events()
	let raw = ch.recv().expect("event")
	io::print(event_value(raw))
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

type Event struct{ Value int }

func Events() chan Event {
	ch := make(chan Event, 1)
	ch <- Event{Value: 42}
	close(ch)
	return ch
}

func EventValue(e Event) int { return e.Value }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	binaryPath := filepath.Join(dir, "app")
	builtPath, err := BuildProgram(program, binaryPath, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	out, err := exec.Command(builtPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run built binary: %v\n%s", err, out)
	}
	if got := string(out); got != "42\n" {
		t.Fatalf("stdout = %q, want 42\\n", got)
	}
}

func TestBuildProgramSupportsProjectGoFFIWithNativeChannel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/async/channel

extern fn observe(ch: channel::Chan<Int>) Int = "demo.Observe"

fn main() Bool {
	let ch = channel::new<Int>(size: 1)
	ch.send(7) and observe(ch.chan) == 7
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

func Observe(ch chan int) int {
	return <-ch
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	binaryPath := filepath.Join(dir, "app")
	builtPath, err := BuildProgram(program, binaryPath, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramSupportsProjectGoFFI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
extern fn lookup(flag: Bool) Str? = {
	go = "demo.Lookup"
}

extern fn read_value() Str!Str = {
	go = "demo.ReadValue"
}

extern fn mark() Void!Str = {
	go = "demo.Mark"
}

extern fn select(input: Str?) Str = {
	go = "demo.Select"
}

fn main() Bool {
	let found = lookup(true)
	let name = found.or("missing")
	let value = read_value().expect("read")
	mark().expect("mark")
	name == "yes" and value == "ok" and select(found) == "yes"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

import ardruntime "github.com/akonwi/ard/runtime"

func Lookup(flag bool) ardruntime.Maybe[string] {
	if !flag {
		return ardruntime.None[string]()
	}
	return ardruntime.Some("yes")
}

func ReadValue() (string, error) {
	return "ok", nil
}

func Mark() error {
	return nil
}

func Select(input ardruntime.Maybe[string]) string {
	if input.IsNone() {
		return "missing"
	}
	return input.Value()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	binaryPath := filepath.Join(dir, "app")
	builtPath, err := BuildProgram(program, binaryPath, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramCoercesProjectExternNativeTraitReturn(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
trait Draw {
	fn draw() Int
}

struct Box {
	value: Int,
}

impl Draw for Box {
	fn draw() Int {
		self.value
	}
}

extern fn identity(value: Draw) Draw = "demo.Identity"
extern fn ok_draw(value: Draw) Draw!Str = "demo.OkDraw"
extern fn fail_draw() Draw!Str = "demo.FailDraw"
extern fn some_draw(value: Draw) Draw? = "demo.SomeDraw"

fn main() {
	let d: Draw = Box{value: 5}
	let ok = ok_draw(d).expect("ok")
	let failed = fail_draw().or(d)
	let some = some_draw(d).expect("some")
	if identity(d).draw() + ok.draw() + failed.draw() + some.draw() != 20 {
		panic("bad identity")
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

import (
	"errors"

	ardruntime "github.com/akonwi/ard/runtime"
)

func Identity(value any) any { return value }
func OkDraw(value any) (any, error) { return value, nil }
func FailDraw() (any, error) { return nil, errors.New("boom") }
func SomeDraw(value any) ardruntime.Maybe[any] { return ardruntime.Some(value) }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	builtPath, err := BuildProgram(program, filepath.Join(dir, "app"), loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramKeepsAnyTraitRepresentationForNestedFFIWrappers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
trait Draw {
	fn draw() Int
}

struct Box {
	value: Int,
}

impl Draw for Box {
	fn draw() Int {
		self.value
	}
}

extern fn maybe_ok_draw(value: Draw) (Draw!Str)? = "demo.MaybeOkDraw"

fn main() {
	let d: Draw = Box{value: 9}
	let ok = maybe_ok_draw(d).expect("some").expect("ok")
	if ok.draw() != 9 {
		panic("bad nested wrapper")
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

import ardruntime "github.com/akonwi/ard/runtime"

func MaybeOkDraw(value any) ardruntime.Maybe[ardruntime.Result[any, string]] {
	return ardruntime.Some(ardruntime.Ok[any, string](value))
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	builtPath, err := BuildProgram(program, filepath.Join(dir, "app"), loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramKeepsAnyTraitRepresentationForFFIContainers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
trait Draw {
	fn draw() Int
}

struct Box {
	value: Int,
}

impl Draw for Box {
	fn draw() Int {
		self.value
	}
}

extern fn identity_list(values: [Draw]) [Draw] = "demo.IdentityList"

fn main() {
	let values: [Draw] = [Box{value: 7}]
	if identity_list(values).at(0).draw() != 7 {
		panic("bad list")
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

func IdentityList(values []any) []any { return values }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	builtPath, err := BuildProgram(program, filepath.Join(dir, "app"), loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramDoesNotRequireDependencyFFIForDirectGoCall(t *testing.T) {
	workspace := t.TempDir()
	depDir := filepath.Join(workspace, "dep")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{depDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(depDir, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "dep.ard"), []byte(`use go:strconv

fn parse(value: Str) Int!Str {
  strconv::Atoi(value)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use dep

fn main() Int!Str {
  dep::parse("42")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestBuildProgramIncludesDependencyGoModForDirectGoCall(t *testing.T) {
	workspace := t.TempDir()
	helperDir := filepath.Join(workspace, "helper")
	badHelperDir := filepath.Join(workspace, "bad-helper")
	depDir := filepath.Join(workspace, "dep")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{helperDir, badHelperDir, depDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(helperDir, "go.mod"), []byte("module example.com/helper\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperDir, "helper.go"), []byte(`package helper

type Thing struct{ Value int }

func Make() Thing { return Thing{Value: 21} }
func Value(thing Thing) int { return thing.Value }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badHelperDir, "go.mod"), []byte("module example.com/helper\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badHelperDir, "helper.go"), []byte(`package helper

type Thing struct{}

func Other() Thing { return Thing{} }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	depGoMod := fmt.Sprintf("module dep\n\ngo 1.26.0\n\nrequire example.com/helper v0.0.0\nreplace example.com/helper => %s\n", helperDir)
	if err := os.WriteFile(filepath.Join(depDir, "go.mod"), []byte(depGoMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "dep.ard"), []byte(`use go:example.com/helper as helper

fn make() helper::Thing {
  helper::Make()
}

fn value(thing: helper::Thing) Int {
  helper::Value(thing)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	appGoMod := fmt.Sprintf("module app\n\ngo 1.26.0\n\nreplace example.com/helper => %s\n", badHelperDir)
	if err := os.WriteFile(filepath.Join(appDir, "go.mod"), []byte(appGoMod), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use dep

let thing = dep::make()

fn main() Int {
  dep::value(thing)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestBuildProgramIncludesDependencyGoModForDirectGoPackageVariable(t *testing.T) {
	workspace := t.TempDir()
	helperDir := filepath.Join(workspace, "helper")
	depDir := filepath.Join(workspace, "dep")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{helperDir, depDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(helperDir, "go.mod"), []byte("module example.com/directvar\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperDir, "directvar.go"), []byte(`package directvar

var Value = 7
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	depGoMod := fmt.Sprintf("module dep\n\ngo 1.26.0\n\nrequire example.com/directvar v0.0.0\nreplace example.com/directvar => %s\n", helperDir)
	if err := os.WriteFile(filepath.Join(depDir, "go.mod"), []byte(depGoMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "dep.ard"), []byte(`use go:example.com/directvar as directvar

fn answer() Int {
  directvar::Value
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use dep

fn main() {
  if not dep::answer() == 7 {
    panic("dependency direct Go package variable failed")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestBuildProgramIncludesDependencyGoModForDirectGoEnumType(t *testing.T) {
	workspace := t.TempDir()
	statusDir := filepath.Join(workspace, "status")
	depDir := filepath.Join(workspace, "dep")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{statusDir, depDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(statusDir, "go.mod"), []byte("module example.com/status\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(statusDir, "status.go"), []byte(`package status

type State int

const (
	StateReady State = iota
	StateDone
)
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	depGoMod := fmt.Sprintf("module dep\n\ngo 1.26.0\n\nrequire example.com/status v0.0.0\nreplace example.com/status => %s\n", statusDir)
	if err := os.WriteFile(filepath.Join(depDir, "go.mod"), []byte(depGoMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "dep.ard"), []byte(`use go:example.com/status as status

fn ready() status::State {
  status::StateReady
}

fn id(state: status::State) status::State {
  state
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use dep

fn main() {
  let state = dep::id(dep::ready())
  if not state == dep::ready() {
    panic("dependency direct Go enum failed")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestBuildProgramSupportsDependencyGoFFI(t *testing.T) {
	workspace := t.TempDir()
	depDir := filepath.Join(workspace, "dep")
	appDir := filepath.Join(workspace, "app")
	if err := os.MkdirAll(filepath.Join(depDir, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "dep.ard"), []byte(`extern fn answer() Int = "Answer"
extern fn lookup(flag: Bool) Str? = "Lookup"
extern fn select(input: Str?) Str = "Select"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "ffi", "host.go"), []byte(`package ffi

import ardruntime "github.com/akonwi/ard/runtime"

func Answer() int { return 42 }

func Lookup(flag bool) ardruntime.Maybe[string] {
	if !flag {
		return ardruntime.None[string]()
	}
	return ardruntime.Some("yes")
}

func Select(input ardruntime.Maybe[string]) string {
	if input.IsNone() {
		return "missing"
	}
	return input.Value()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use dep

fn main() {
	let found = dep::lookup(true)
	if not dep::answer() == 42 or not dep::select(found) == "yes" or not dep::select(dep::lookup(false)) == "missing" {
		panic("dependency ffi failed")
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	binaryPath := filepath.Join(appDir, "app")
	builtPath, err := BuildProgram(program, binaryPath, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	cmd := exec.Command(builtPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramCopiesDependencyGoFFIForExternTypeOnly(t *testing.T) {
	workspace := t.TempDir()
	depDir := filepath.Join(workspace, "dep")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{filepath.Join(depDir, "ffi"), appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(depDir, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "dep.ard"), []byte(`extern type Callback = "func(map[string]ffi.Handle) error"
extern type Handles = "chan ffi.Handle"
struct Holder { callback: Callback, handles: Handles }
fn noop() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "ffi", "host.go"), []byte(`package ffi

type Handle struct{}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use dep

fn main() { dep::noop() }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestBuildProgramSupportsTransitiveDependencyGoFFI(t *testing.T) {
	workspace := t.TempDir()
	hostDir := filepath.Join(workspace, "host")
	depDir := filepath.Join(workspace, "dep")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{filepath.Join(hostDir, "ffi"), depDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(hostDir, "ard.toml"), []byte("name = \"host\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostDir, "host.ard"), []byte(`extern type Handle = "ffi.Handle"
extern fn handle() Handle = "HandleValue"
extern fn answer_from(handle: Handle) Int = "AnswerFrom"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostDir, "ffi", "host.go"), []byte(`package ffi

type Handle struct{ Value int }

func HandleValue() Handle { return Handle{Value: 42} }

func AnswerFrom(handle Handle) int { return handle.Value }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n\n[dependencies]\nhost = { path = \"../host\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "dep.ard"), []byte(`use host

fn answer() Int {
	let handle = host::handle()
	host::answer_from(handle)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := fmt.Sprintf(`{
  "version": 1,
  "root": "root",
  "packages": [
    {"id": "root", "name": "app", "path": ".", "dependencies": {"dep": "path:%s"}},
    {"id": "path:%s", "name": "dep", "path": "%s", "dependencies": {"host": "path:%s"}},
    {"id": "path:%s", "name": "host", "path": "%s"}
  ]
}
`, depDir, depDir, depDir, hostDir, hostDir, hostDir)
	if err := os.WriteFile(filepath.Join(appDir, "ard.lock"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use dep

fn main() {
	if not dep::answer() == 42 {
		panic("transitive dependency ffi failed")
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	builtPath, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestWriteProgramDoesNotRequireProjectFFIForStdlibExternMethods(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/sql

fn close(db: sql::Database) Void!Str {
	db.close()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	workspace := filepath.Join(dir, "workspace")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeProgram(workspace, program, Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo}); err != nil {
		t.Fatalf("write program: %v", err)
	}
}

func TestLowerProgramUsesRuntimeMaybeForRecursiveNullableFields(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		struct Node { value: Int, parent: Node? }

		fn main() Int {
			let root = Node{value: 1, parent: maybe::none()}
			let child = Node{value: 2, parent: maybe::some(root)}
			child.parent.expect("").value
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok || len(field.Names) != 1 || field.Names[0].Name != "Parent" {
			return false
		}
		indexed, ok := field.Type.(*ast.IndexExpr)
		return ok && astExprName(indexed.X) == "ardruntime.Maybe" && astExprName(indexed.Index) == "Node"
	}) {
		t.Fatal("generated AST missing runtime Maybe recursive nullable field")
	}
	if astFilesContain(files, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok || len(field.Names) != 1 || field.Names[0].Name != "Parent" {
			return false
		}
		star, ok := field.Type.(*ast.StarExpr)
		return ok && astExprName(star.X) == "Node"
	}) {
		t.Fatal("generated AST lowered recursive nullable field as pointer")
	}
}

func TestLowerProgramUsesExpectedLocalTypeForMaybeNone(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn main() Bool {
			let found: Int? = maybe::none()
			found.is_none()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || astCallName(call) != "ardruntime.None" {
			return false
		}
		indexed, ok := call.Fun.(*ast.IndexExpr)
		return ok && astExprName(indexed.Index) == "int"
	}) {
		t.Fatal("generated AST missing typed maybe none")
	}
	if astFilesHaveEmptyStructType(files) {
		t.Fatal("generated AST used untyped maybe none")
	}
}

func TestLowerProgramUsesExpectedDefaultTypeForResultOr(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn fetch() Int?!Str {
			let empty: Int? = maybe::none()
			Result::ok(empty)
		}

		fn main() Bool {
			let value = fetch().or(maybe::none())
			value.is_none()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if astFilesHaveEmptyStructType(files) {
		t.Fatal("generated AST used untyped maybe default")
	}
}

func TestLowerProgramSkipsVoidAssignmentForStatementMatchBranches(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn main() Bool {
			match maybe::some(1) {
				value => value == 1,
				_ => (),
			}
			false
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if astFilesContain(files, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return false
		}
		for _, rhs := range assign.Rhs {
			ident, ok := rhs.(*ast.Ident)
			if ok && ident.Name == "nil" {
				return true
			}
		}
		return false
	}) {
		t.Fatal("generated AST assigned nil in statement match lowering")
	}
}

func TestRunProgramSupportsVoidFiberFunctions(t *testing.T) {
	program := lowerSource(t, `
		use ard/async

		fn job() Void {
			()
		}

		fn main() Void {
			async::start(job)
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestTypeNameUsesModulePathAndUniqueFallback(t *testing.T) {
	program := &air.Program{}
	inbox := typeName(program, air.TypeInfo{ID: 1, Name: "Store", ModulePath: "app/models/inbox"})
	issues := typeName(program, air.TypeInfo{ID: 2, Name: "Store", ModulePath: "app/models/issues"})
	if inbox != "app_models_inbox__Store" || issues != "app_models_issues__Store" {
		t.Fatalf("module type names = %q, %q", inbox, issues)
	}

	left := typeName(program, air.TypeInfo{ID: 3, Name: "Request"})
	right := typeName(program, air.TypeInfo{ID: 4, Name: "Request"})
	if left == right {
		t.Fatalf("fallback type names should be unique, got %q", left)
	}
}

func TestLowerProgramSupportsResultExpectAndStringPredicates(t *testing.T) {
	program := lowerSource(t, `
		use ard/io

		fn main() Bool {
			let line = io::read_line().expect("no line")
			line.is_empty()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		indexed, ok := node.(*ast.IndexListExpr)
		return ok && astExprName(indexed.X) == "ardruntime.Result" && len(indexed.Indices) == 2 && astExprName(indexed.Indices[0]) == "string" && astExprName(indexed.Indices[1]) == "string"
	}) {
		t.Fatal("generated AST missing runtime.Result usage")
	}
	if !astFilesHaveCall(files, "stdlibffi.ReadLine") {
		t.Fatal("generated AST missing ReadLine lowering")
	}
	if astFilesHaveCall(files, "ardReadLine") {
		t.Fatal("generated AST should not use legacy ReadLine helper")
	}
	if !astFilesHaveCall(files, "panic") || !astFilesContain(files, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		return ok && lit.Kind == token.STRING && strings.Contains(lit.Value, "no line")
	}) {
		t.Fatal("generated AST missing Result.expect lowering")
	}
	if !astFilesHaveCall(files, "len") {
		t.Fatal("generated AST missing is_empty lowering")
	}
}

func TestLowerProgramUsesDirectStdlibMaybeCalls(t *testing.T) {
	program := lowerSource(t, `
		use ard/dynamic
		use ard/env
		use ard/float
		use ard/int

		fn main() Bool {
			let _a = env::get("PATH")
			let _b = float::from_str("1.5")
			let _c = int::from_str("2")
			let _d = dynamic::object(["a": dynamic::from_int(1)])
			true
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveCall(files, "os.LookupEnv") {
		t.Fatal("generated AST missing direct Go env lookup")
	}
	if !astFilesHaveCall(files, "strconv.ParseFloat") {
		t.Fatal("generated AST missing direct Go float parsing call")
	}
	if !astFilesHaveCall(files, "strconv.Atoi") {
		t.Fatal("generated AST missing direct Go int parsing call")
	}
	if astFilesHaveCall(files, "ardIntFromStr") {
		t.Fatal("generated AST should not use legacy IntFromStr helper")
	}
	if astFilesHaveCall(files, "ardMapToDynamic") {
		t.Fatal("generated AST should not use legacy MapToDynamic helper")
	}
}

func TestLowerProgramUsesDirectGoPackageConstantSelector(t *testing.T) {
	program := lowerSource(t, `
		use go:os

		fn flags() Int {
			os::O_WRONLY
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		return ok && selector.Sel != nil && selector.Sel.Name == "O_WRONLY" && astExprName(selector.X) == "os"
	}) {
		t.Fatal("generated AST missing os.O_WRONLY selector")
	}
}

func TestLowerProgramConvertsTypedDirectGoPackageConstants(t *testing.T) {
	program := lowerSource(t, `
		use go:time

		fn duration() Int {
			time::Nanosecond
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || astCallName(call) != "int" || len(call.Args) != 1 {
			return false
		}
		return astExprName(call.Args[0]) == "time.Nanosecond"
	}) {
		t.Fatal("generated AST missing int(time.Nanosecond) conversion")
	}
}

func TestLowerProgramUsesDirectGoPackageVariableSelector(t *testing.T) {
	program := lowerSource(t, `
		use go:encoding/base64 as base64

		fn encode(bytes: [Byte]) Str {
			base64::StdEncoding.EncodeToString(bytes)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel == nil || selector.Sel.Name != "EncodeToString" {
			return false
		}
		return astExprName(selector.X) == "base64.StdEncoding"
	}) {
		t.Fatal("generated AST missing base64.StdEncoding.EncodeToString selector")
	}
}

func TestLowerProgramUsesDirectGoStructFieldSelector(t *testing.T) {
	response := checker.GoValueType{Kind: checker.GoValueOther, Expr: "http.Response", Named: true, ImportPath: "example.com/http", Package: "http", Name: "Response"}
	program := lowerSourceWithCheckOptions(t, `
		use go:example.com/http as http

		fn status() Int {
			http::DefaultResponse.StatusCode
		}
	`, checker.CheckOptions{GoResolver: testGoResolver{packages: map[string]*checker.GoPackage{
		"example.com/http": {
			ImportPath: "example.com/http",
			Name:       "http",
			Variables:  map[string]checker.GoVariable{"DefaultResponse": {Name: "DefaultResponse", Type: response}},
			Types: map[string]checker.GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]checker.GoField{
				"StatusCode": {Name: "StatusCode", Type: checker.GoValueType{Kind: checker.GoValueInt, Expr: "int"}},
			}}},
		},
	}}})

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel == nil || selector.Sel.Name != "StatusCode" {
			return false
		}
		return astExprName(selector.X) == "http.DefaultResponse"
	}) {
		t.Fatal("generated AST missing http.DefaultResponse.StatusCode selector")
	}
}

func TestLowerProgramUsesDirectGoStructFieldAssignmentSelector(t *testing.T) {
	response := checker.GoValueType{Kind: checker.GoValueOther, Expr: "http.Response", Named: true, ImportPath: "example.com/http", Package: "http", Name: "Response"}
	program := lowerSourceWithCheckOptions(t, `
		use go:example.com/http as http

		fn status() Int {
			mut res = http::DefaultResponse
			res.StatusCode = 201
			res.StatusCode
		}
	`, checker.CheckOptions{GoResolver: testGoResolver{packages: map[string]*checker.GoPackage{
		"example.com/http": {
			ImportPath: "example.com/http",
			Name:       "http",
			Variables:  map[string]checker.GoVariable{"DefaultResponse": {Name: "DefaultResponse", Type: response}},
			Types: map[string]checker.GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]checker.GoField{
				"StatusCode": {Name: "StatusCode", Type: checker.GoValueType{Kind: checker.GoValueInt, Expr: "int"}},
			}}},
		},
	}}})

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return false
		}
		selector, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok || selector.Sel == nil || selector.Sel.Name != "StatusCode" {
			return false
		}
		lit, ok := assign.Rhs[0].(*ast.BasicLit)
		return ok && lit.Value == "201"
	}) {
		t.Fatal("generated AST missing direct Go StatusCode assignment")
	}
}

func TestLowerProgramRangeChecksDirectGoStructFieldAssignment(t *testing.T) {
	response := checker.GoValueType{Kind: checker.GoValueOther, Expr: "narrow.Response", Named: true, ImportPath: "example.com/narrow", Package: "narrow", Name: "Response"}
	program := lowerSourceWithCheckOptions(t, `
		use go:example.com/narrow as narrow

		fn set_code() {
			mut res = narrow::DefaultResponse
			res.Code = 7
		}
	`, checker.CheckOptions{GoResolver: testGoResolver{packages: map[string]*checker.GoPackage{
		"example.com/narrow": {
			ImportPath: "example.com/narrow",
			Name:       "narrow",
			Variables:  map[string]checker.GoVariable{"DefaultResponse": {Name: "DefaultResponse", Type: response}},
			Types: map[string]checker.GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]checker.GoField{
				"Code": {Name: "Code", Type: checker.GoValueType{Kind: checker.GoValueInt, Expr: "int8", Bits: 8}},
			}}},
		},
	}}})

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		return ok && astCallName(call) == "ardDirectGoCheckSignedIntRange"
	}) {
		t.Fatal("generated AST missing direct Go field assignment range check")
	}
}

func TestLowerProgramUsesDirectGoStructCompositeLiteral(t *testing.T) {
	program := lowerSourceWithCheckOptions(t, `
		use go:example.com/image as image

		fn point() image::Point {
			image::Point{X: 10, Y: 20}
		}
	`, checker.CheckOptions{GoResolver: testGoResolver{packages: map[string]*checker.GoPackage{
		"example.com/image": {
			ImportPath: "example.com/image",
			Name:       "image",
			Types: map[string]checker.GoType{"Point": {Name: "Point", Struct: true, Fields: map[string]checker.GoField{
				"X": {Name: "X", Type: checker.GoValueType{Kind: checker.GoValueInt, Expr: "int"}},
				"Y": {Name: "Y", Type: checker.GoValueType{Kind: checker.GoValueInt, Expr: "int"}},
			}}},
		},
	}}})

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		lit, ok := node.(*ast.CompositeLit)
		if !ok || astExprName(lit.Type) != "image.Point" {
			return false
		}
		fields := map[string]string{}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			if basic, ok := kv.Value.(*ast.BasicLit); ok {
				fields[key.Name] = basic.Value
			}
		}
		return fields["X"] == "10" && fields["Y"] == "20"
	}) {
		t.Fatal("generated AST missing image.Point keyed composite literal")
	}
}

func TestLowerProgramRangeChecksDirectGoStructConstruction(t *testing.T) {
	program := lowerSourceWithCheckOptions(t, `
		use go:example.com/narrow as narrow

		fn response() narrow::Response {
			narrow::Response{Code: 7}
		}
	`, checker.CheckOptions{GoResolver: testGoResolver{packages: map[string]*checker.GoPackage{
		"example.com/narrow": {
			ImportPath: "example.com/narrow",
			Name:       "narrow",
			Types: map[string]checker.GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]checker.GoField{
				"Code": {Name: "Code", Type: checker.GoValueType{Kind: checker.GoValueInt, Expr: "int8", Bits: 8}},
			}}},
		},
	}}})

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		return ok && astCallName(call) == "ardDirectGoCheckSignedIntRange"
	}) {
		t.Fatal("generated AST missing direct Go struct construction range check")
	}
}

func TestLowerProgramUsesPointersForMutableStructParams(t *testing.T) {
	program := lowerSource(t, `
		struct Response {
			body: Str,
		}

		fn set_body(mut res: Response) Void {
			res.body = "ok"
		}

		fn main() Void {
			mut res = Response{body: ""}
			set_body(res)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	fn, ok := astFilesFunc(files, "test_ard__set_body")
	if !ok || fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		t.Fatalf("generated AST missing set_body function")
	}
	paramType, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok || astExprName(paramType.X) != "Response" {
		t.Fatalf("generated AST missing pointer mutable param lowering: %#v", fn.Type.Params.List[0].Type)
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || astCallName(call) != "test_ard__set_body" || len(call.Args) == 0 {
			return false
		}
		addr, ok := call.Args[0].(*ast.UnaryExpr)
		if !ok || addr.Op != token.AND {
			return false
		}
		ident, ok := addr.X.(*ast.Ident)
		return ok && ident.Name == "res_0"
	}) {
		t.Fatal("generated AST missing pointer call lowering")
	}
}

func TestLowerProgramSupportsCapturedClosureSort(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			mut items = [3, 1, 2]
			let bias = 0
			items.sort(fn(a: Int, b: Int) Bool {
				a + bias < b + bias
			})
			items.at(0)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveCall(files, "sort.SliceStable") {
		t.Fatal("generated AST missing list sort lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		lit, ok := node.(*ast.FuncLit)
		return ok && lit.Type.Params != nil && len(lit.Type.Params.List) == 2 && lit.Type.Results != nil && len(lit.Type.Results.List) == 1 && astExprName(lit.Type.Results.List[0].Type) == "bool"
	}) {
		t.Fatal("generated AST missing closure literal lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		return ok && strings.HasPrefix(ident.Name, "bias")
	}) {
		t.Fatal("generated AST missing closure capture usage")
	}
	if astFilesHaveFuncContaining(files, "anon_func") {
		t.Fatal("generated AST should inline local closure body instead of emitting an anon helper")
	}
}

func TestLowerProgramInlinesNestedImmediateClosures(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn main() Int {
			let bias = 2
			let result = maybe::some(40).map(fn(value) {
				maybe::some(value).map(fn(inner) { inner + bias }).or(0)
			})
			result.or(0)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if astFilesHaveFuncContaining(files, "anon_func") {
		t.Fatal("generated AST should inline nested immediate closures instead of emitting anon helpers")
	}
	funcLits := 0
	astFilesContain(files, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			funcLits++
		}
		return false
	})
	if funcLits < 2 {
		t.Fatalf("generated AST missing nested function literals: got %d", funcLits)
	}
}

func TestLowerProgramKeepsHelperForMutableCaptureClosure(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn main() Int {
			mut total = 0
			let result = maybe::some(1).map(fn(value) {
				total = total + value
				total
			})
			result.or(0)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveFuncContaining(files, "anon_func") {
		t.Fatal("generated AST should keep helper for mutable capture closure")
	}
}

func TestLowerProgramKeepsHelperForRetainedClosure(t *testing.T) {
	program := lowerSource(t, `
		fn make_adder(offset: Int) fn(Int) Int {
			fn(value: Int) Int {
				value + offset
			}
		}

		fn main() Int {
			let add = make_adder(2)
			add(3)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveFuncContaining(files, "anon_func") {
		t.Fatal("generated AST should keep helper for retained closure")
	}
}

func TestLowerProgramEmitsGoMethodWrapperForInherentImpl(t *testing.T) {
	program := lowerSource(t, `
		struct Box {
			value: Int,
		}

		impl Box {
			fn Count() Int {
				self.value
			}
		}

		fn main() Int {
			let box = Box{value: 7}
			box.Count()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil || fn.Name.Name != "Count" || len(fn.Recv.List) != 1 {
			return false
		}
		foundCall := false
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok && strings.Contains(astCallName(call), "Box_Count") {
				foundCall = true
				return false
			}
			return true
		})
		return foundCall
	}) {
		t.Fatal("generated AST missing Go method wrapper for inherent impl")
	}
}

func TestLowerProgramEmitsGoMethodWrapperForTraitImpl(t *testing.T) {
	program := lowerSource(t, `
		trait Labeled {
			fn Label() Str
		}

		struct Button {
			text: Str,
		}

		impl Labeled for Button {
			fn Label() Str {
				self.text
			}
		}

		fn label(value: Labeled) Str {
			value.Label()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil || fn.Name.Name != "Label" || len(fn.Recv.List) != 1 {
			return false
		}
		foundCall := false
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok && strings.Contains(astCallName(call), "Button_Labeled_Label") {
				foundCall = true
				return false
			}
			return true
		})
		return foundCall
	}) {
		t.Fatal("generated AST missing Go method wrapper for trait impl")
	}
}

func TestLowerProgramEmitsGoInterfaceForTraitObject(t *testing.T) {
	program := lowerSource(t, `
		trait Renderable {
			fn render() Str
			fn area(scale: Int) Int
		}

		struct Block {
			title: Str,
		}

		impl Renderable for Block {
			fn render() Str {
				self.title
			}

			fn area(scale: Int) Int {
				scale
			}
		}

		fn draw(value: Renderable) Str {
			value.render()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name == nil || !strings.HasPrefix(typeSpec.Name.Name, "ardTrait_Renderable_") {
			return false
		}
		iface, ok := typeSpec.Type.(*ast.InterfaceType)
		if !ok || iface.Methods == nil || len(iface.Methods.List) != 2 {
			return false
		}
		methods := map[string]*ast.FuncType{}
		for _, method := range iface.Methods.List {
			if len(method.Names) != 1 {
				return false
			}
			fnType, ok := method.Type.(*ast.FuncType)
			if !ok {
				return false
			}
			methods[method.Names[0].Name] = fnType
		}
		render, ok := methods["render"]
		if !ok || render.Params == nil || len(render.Params.List) != 0 || render.Results == nil || len(render.Results.List) != 1 || astExprName(render.Results.List[0].Type) != "string" {
			return false
		}
		area, ok := methods["area"]
		return ok && area.Params != nil && len(area.Params.List) == 1 && astExprName(area.Params.List[0].Type) == "int" && area.Results != nil && len(area.Results.List) == 1 && astExprName(area.Results.List[0].Type) == "int"
	}) {
		t.Fatal("generated AST missing Go interface for Ard trait")
	}
	if !astFilesHaveTypeSpec(files, "ardMutTrait_Renderable_0") {
		t.Fatal("generated AST should keep mutable trait reference type")
	}
}

func TestLowerProgramSkipsGoMethodWrapperWhenStructFieldCollides(t *testing.T) {
	program := lowerSource(t, `
		trait Named {
			fn Name() Str
		}

		struct User {
			name: Str,
		}

		impl Named for User {
			fn Name() Str {
				self.name
			}
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		return ok && fn.Recv != nil && fn.Name != nil && fn.Name.Name == "Name"
	}) {
		t.Fatal("generated AST should not emit Go method wrapper that collides with a struct field")
	}
}

func TestLowerProgramSkipsGoMethodWrapperForReservedStructReceiverMethods(t *testing.T) {
	program := lowerSource(t, `
		struct Payload {
			value: Int,
		}

		impl Payload {
			fn MarshalJSONTo() Int {
				self.value
			}

			fn UnmarshalJSONFrom() Int {
				self.value
			}
		}

		fn main() Int {
			let payload = Payload{value: 1}
			payload.MarshalJSONTo() + payload.UnmarshalJSONFrom()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	for _, reserved := range []string{"MarshalJSONTo", "UnmarshalJSONFrom"} {
		if astFilesContain(files, func(node ast.Node) bool {
			fn, ok := node.(*ast.FuncDecl)
			return ok && fn.Recv != nil && fn.Name != nil && fn.Name.Name == reserved
		}) {
			t.Fatalf("generated AST should not emit Go method wrapper %s reserved for generated JSON helpers", reserved)
		}
	}
}

func TestLowerProgramPassesPointerReceiverForMutatingTraitImpl(t *testing.T) {
	program := lowerSource(t, `
		trait Writer {
			fn write(text: Str)
		}

		struct Buffer {
			contents: Str,
		}

		impl Writer for Buffer {
			fn mut write(text: Str) {
				self.contents = self.contents + text
			}
		}

		fn send(w: Writer) {
			w.write("hi")
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Name == nil || !strings.Contains(fn.Name.Name, "Buffer_Writer_write") || fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
			return false
		}
		if len(fn.Type.Params.List[0].Names) == 0 || fn.Type.Params.List[0].Names[0].Name != "self_0" {
			return false
		}
		_, ok = fn.Type.Params.List[0].Type.(*ast.StarExpr)
		return ok
	}) {
		t.Fatal("generated AST missing pointer receiver for mutating trait impl")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil || fn.Name.Name != "write" || len(fn.Recv.List) != 1 {
			return false
		}
		_, ok = fn.Recv.List[0].Type.(*ast.StarExpr)
		return ok
	}) {
		t.Fatal("generated AST missing pointer Go method wrapper for mutating trait impl")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(astCallName(call), "Buffer_Writer_write") || len(call.Args) < 2 {
			return false
		}
		addr, ok := call.Args[0].(*ast.UnaryExpr)
		if !ok || addr.Op != token.AND {
			return false
		}
		lit, ok := call.Args[1].(*ast.BasicLit)
		return ok && lit.Value == `"hi"`
	}) {
		t.Fatal("generated AST missing address-of for mutating trait dispatch receiver")
	}
}

func TestLowerProgramSupportsUserTraitObjectDispatch(t *testing.T) {
	program := lowerSource(t, `
		trait Renderable {
			fn render() Str
		}

		struct Block {
			title: Str,
		}

		struct Para {
			body: Str,
		}

		impl Renderable for Block {
			fn render() Str {
				"[block:" + self.title + "]"
			}
		}

		impl Renderable for Para {
			fn render() Str {
				"[para:" + self.body + "]"
			}
		}

		fn draw(r: Renderable) Str {
			r.render()
		}

		fn main() Str {
			draw(Block{title: "hi"}) + draw(Para{body: "there"})
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name == nil || !strings.HasPrefix(typeSpec.Name.Name, "ardTrait_Renderable_") {
			return false
		}
		_, ok = typeSpec.Type.(*ast.InterfaceType)
		return ok
	}) {
		t.Fatal("generated AST missing native Go interface for trait object")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return false
		}
		name := astCallName(call)
		return name == "render" || strings.HasSuffix(name, ".render")
	}) {
		t.Fatal("generated AST missing native interface trait method call")
	}
	for _, name := range []string{"Block_Renderable_render", "Para_Renderable_render"} {
		if !astFilesContain(files, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			return ok && strings.Contains(astCallName(call), name)
		}) {
			t.Fatalf("generated AST missing %s Go method wrapper call", name)
		}
	}
}

func TestLowerProgramSupportsCrossModuleTraitObjectDispatch(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"checkprobe\"\nard = \">= 0.13.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "widget.ard"), []byte(`
struct Frame { size: Int }

trait Widget {
  fn render(frame: Frame)
}

struct Text { content: Str }

impl Widget for Text {
  fn mut render(frame: Frame) { () }
}

fn plain(content: Str) Widget {
  Text{content: content}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	result := parse.Parse([]byte(`
use checkprobe/widget

fn main() {
  let f = widget::Frame{size: 10}
  let t = widget::plain("hi")
  t.render(f)
}
`), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New(filepath.Join(tempDir, "main.ard"), result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		_, ok := node.(*ast.TypeSwitchStmt)
		return ok
	}) {
		t.Fatal("generated AST missing trait dispatch")
	}
	if !astFilesHaveTypeSwitchCase(files, "Text") {
		t.Fatal("generated AST missing cross-module trait dispatch case")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		return ok && strings.Contains(astCallName(call), "checkprobe_widget__Text_Widget_render") && len(call.Args) >= 2 && astExprName(call.Args[1]) == "f_0"
	}) {
		t.Fatal("generated AST missing cross-module trait dispatch call")
	}
}

func TestLowerProgramUsesCallSiteImportsForCrossModuleTraitObjectDispatch(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"nestprobe\"\nard = \">= 0.13.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(tempDir, "commands"), filepath.Join(tempDir, "tui", "core")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"tui/core/widget.ard": `
struct Frame { size: Int }

trait Widget {
  fn render(frame: Frame)
}
`,
		"tui/core/text.ard": `
use nestprobe/tui/core/widget

struct Text { content: Str }

impl widget::Widget for Text {
  fn mut render(frame: widget::Frame) { () }
}

fn plain(content: Str) widget::Widget {
  Text{content: content}
}
`,
		"tui/core/box.ard": `
use nestprobe/tui/core/widget

struct Box { child: widget::Widget }

impl widget::Widget for Box {
  fn mut render(frame: widget::Frame) {
    self.child.render(frame)
  }
}

fn wrap(child: widget::Widget) widget::Widget {
  Box{child: child}
}
`,
		"commands/demo.ard": `
use nestprobe/tui/core/widget
use nestprobe/tui/core/text as textw
use nestprobe/tui/core/box as boxw

fn run() {
  let f = widget::Frame{size: 10}
  let demo = boxw::wrap(textw::plain("hi"))
  demo.render(f)
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tempDir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	result := parse.Parse([]byte(`
use nestprobe/commands/demo

fn main() {
  demo::run()
}
`), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New(filepath.Join(tempDir, "main.ard"), result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	generatedFiles := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(generatedFiles, func(node ast.Node) bool {
		_, ok := node.(*ast.TypeSwitchStmt)
		return ok
	}) {
		t.Fatal("generated AST missing call-site trait dispatch")
	}
	if !astFilesHaveTypeSwitchCase(generatedFiles, "Box") {
		t.Fatal("generated AST missing Box dispatch case from call-site imports")
	}
	if !astFilesHaveTypeSwitchCase(generatedFiles, "Text") {
		t.Fatal("generated AST missing Text dispatch case from call-site imports")
	}
}

func TestLowerProgramUsesAliasOriginImportsForTraitObjectDispatch(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"aliasprobe\"\nard = \">= 0.13.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(tempDir, "commands"), filepath.Join(tempDir, "widgets")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"widget.ard": `
struct Frame { size: Int }

trait Widget {
  fn render(frame: Frame)
}
`,
		"widgets/text.ard": `
use aliasprobe/widget

struct Text { content: Str }

impl widget::Widget for Text {
  fn mut render(frame: widget::Frame) { () }
}

fn new(content: Str) widget::Widget { Text{content: content} }
`,
		"widgets/box.ard": `
use aliasprobe/widget

struct Box { child: widget::Widget }

impl widget::Widget for Box {
  fn mut render(frame: widget::Frame) { self.child.render(frame) }
}

fn new(child: widget::Widget) widget::Widget { Box{child: child} }
`,
		"facade_let.ard": `
use aliasprobe/widgets/text
use aliasprobe/widgets/box

let make_text = text::new
let make_box = box::new
`,
		"commands/demo.ard": `
use aliasprobe/widget
use aliasprobe/facade_let as facade

fn run() {
  let f = widget::Frame{size: 10}
  let w = facade::make_box(facade::make_text("hi"))
  w.render(f)
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tempDir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	result := parse.Parse([]byte(`
use aliasprobe/widgets/text
use aliasprobe/widgets/box
use aliasprobe/commands/demo

fn main() { demo::run() }
`), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New(filepath.Join(tempDir, "main.ard"), result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	generatedFiles := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(generatedFiles, func(node ast.Node) bool {
		_, ok := node.(*ast.TypeSwitchStmt)
		return ok
	}) {
		t.Fatal("generated AST missing aliased-constructor trait dispatch")
	}
	if !astFilesHaveTypeSwitchCase(generatedFiles, "Box") {
		t.Fatal("generated AST missing Box dispatch case through let alias")
	}
	if !astFilesHaveTypeSwitchCase(generatedFiles, "Text") {
		t.Fatal("generated AST missing Text dispatch case through let alias")
	}
}

func TestLowerProgramSupportsVoidTraitObjectDispatch(t *testing.T) {
	program := lowerSource(t, `
		use ard/io

		trait Greet {
			fn say()
		}

		struct Cat {
			name: Str,
		}

		impl Greet for Cat {
			fn say() {
				io::print("meow from {self.name}")
			}
		}

		fn invoke(g: Greet) {
			g.say()
		}

		fn main() {
			invoke(Cat{name: "milo"})
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		_, ok := node.(*ast.TypeSwitchStmt)
		return ok
	}) {
		t.Fatal("generated AST missing void trait object dispatch lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		return ok && strings.Contains(astCallName(call), "Cat_Greet_say")
	}) {
		t.Fatal("generated AST missing void trait dispatch call")
	}
	if astFilesContain(files, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return false
		}
		for _, rhs := range assign.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if ok && strings.Contains(astCallName(call), "Cat_Greet_say") {
				return true
			}
		}
		return false
	}) {
		t.Fatal("generated AST assigns void trait dispatch result")
	}
	if !astFilesHaveCall(files, "any") {
		t.Fatal("generated AST missing trait upcast for call argument")
	}
}

func TestLowerProgramSupportsStoredTraitObjectDispatch(t *testing.T) {
	program := lowerSource(t, `
		use ard/io

		trait Drawable {
			fn draw() Str
		}

		struct Box {
			w: Int,
		}

		impl Drawable for Box {
			fn draw() Str {
				"box[{self.w}]"
			}
		}

		struct Container {
			child: Drawable,
		}

		fn show(d: Drawable) {
			io::print(d.draw())
		}

		fn main() {
			let d: Drawable = Box{w: 1}
			io::print(d.draw())

			let c = Container{child: Box{w: 2}}
			io::print(c.child.draw())

			let items: [Drawable] = [Box{w: 3}]
			show(items.at(0))
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		return ok && strings.HasPrefix(astCallName(call), "ardTrait_Drawable_")
	}) {
		t.Fatal("generated AST missing native interface trait-object conversion")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		kv, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, keyOK := kv.Key.(*ast.Ident)
		call, callOK := kv.Value.(*ast.CallExpr)
		return keyOK && key.Name == "Child" && callOK && strings.HasPrefix(astCallName(call), "ardTrait_Drawable_")
	}) {
		t.Fatal("generated AST missing struct field native trait-object conversion")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !strings.HasPrefix(astExprName(lit.Type), "[]ardTrait_Drawable_") {
			return false
		}
		for _, elem := range lit.Elts {
			call, ok := elem.(*ast.CallExpr)
			if ok && strings.HasPrefix(astCallName(call), "ardTrait_Drawable_") {
				return true
			}
		}
		return false
	}) {
		t.Fatal("generated AST missing list element native trait-object conversion")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return false
		}
		name := astCallName(call)
		return name == "draw" || strings.HasSuffix(name, ".draw")
	}) {
		t.Fatal("generated AST missing native interface trait-object dispatch")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(astCallName(call), "show") || len(call.Args) != 1 {
			return false
		}
		_, ok = call.Args[0].(*ast.IndexExpr)
		return ok
	}) {
		t.Fatal("generated AST missing list element trait-object use")
	}
}

func TestLowerProgramSupportsTraitObjectDispatch(t *testing.T) {
	program := lowerSource(t, `
		use ard/io

		struct Book {
			title: Str,
		}

		impl Str::ToString for Book {
			fn to_str() Str {
				self.title
			}
		}

		fn show(item: Str::ToString) Str {
			item.to_str()
		}

		fn main() Str {
			show(Book{title: "The Hobbit"})
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return false
		}
		name := astCallName(call)
		return name == "to_str" || strings.HasSuffix(name, ".to_str")
	}) {
		t.Fatal("generated AST missing native interface trait dispatch")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		return ok && strings.Contains(astCallName(call), "Book_ToString_to_str")
	}) {
		t.Fatal("generated AST missing concrete trait method wrapper call")
	}
}

func TestLowerProgramSupportsListSwapAndMapKeys(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			mut items = [1, 2, 3]
			items.swap(0, 2)
			let values = ["b": 2, "a": 1]
			let keys = values.keys()
			items.at(0) + keys.size()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) == 0 {
			return false
		}
		_, ok = assign.Lhs[0].(*ast.IndexExpr)
		return ok
	}) {
		t.Fatal("generated AST missing list swap lowering")
	}
	if !astFilesHaveCall(files, "ardSortedStringKeys") {
		t.Fatal("generated AST missing map keys lowering")
	}
}

func TestLowerProgramEmitsOnlyUsedImports(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			1
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	for _, importPath := range []string{"bufio", "strconv", "strings"} {
		if astFilesHaveImport(files, "", importPath) {
			t.Fatalf("generated AST included unused runtime import %q", importPath)
		}
	}
}

func TestLowerProgramSupportsFieldMutation(t *testing.T) {
	program := lowerSource(t, `
		struct Counter {
			value: Int,
		}

		fn bump(counter: Counter) Int {
			mut current = counter
			current.value = current.value + 1
			current.value
		}

		fn main() Int {
			bump(Counter{value: 1})
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return false
		}
		lhs, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok || lhs.Sel.Name != "Value" {
			return false
		}
		binary, ok := assign.Rhs[0].(*ast.BinaryExpr)
		if !ok || binary.Op != token.ADD {
			return false
		}
		rhsSelector, ok := binary.X.(*ast.SelectorExpr)
		lit, litOK := binary.Y.(*ast.BasicLit)
		return ok && rhsSelector.Sel.Name == "Value" && litOK && lit.Value == "1"
	}) {
		t.Fatal("generated AST missing field mutation lowering")
	}
}

func TestLowerProgramSupportsIfAndWhile(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			mut count = 0
			while count < 3 {
				count = count + 1
			}
			if count == 3 {
				count
			} else {
				0
			}
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		stmt, ok := node.(*ast.ForStmt)
		if !ok {
			return false
		}
		cond, ok := stmt.Cond.(*ast.BinaryExpr)
		lit, litOK := cond.Y.(*ast.BasicLit)
		return ok && cond.Op == token.LSS && litOK && lit.Value == "3"
	}) {
		t.Fatal("generated AST missing while lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		stmt, ok := node.(*ast.IfStmt)
		if !ok {
			return false
		}
		cond, ok := stmt.Cond.(*ast.BinaryExpr)
		lit, litOK := cond.Y.(*ast.BasicLit)
		return ok && cond.Op == token.EQL && litOK && lit.Value == "3"
	}) {
		t.Fatal("generated AST missing if lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		value, ok := node.(*ast.ValueSpec)
		if !ok || astExprName(value.Type) != "int" {
			return false
		}
		for _, name := range value.Names {
			if strings.HasPrefix(name.Name, "_tmp_") {
				return true
			}
		}
		return false
	}) {
		t.Fatal("generated AST missing expression temp lowering")
	}
}

func TestCollectFFIGoImportsIncludesStdlibImportsWithoutSourceCheckout(t *testing.T) {
	imports := collectGoImportsFromEmbeddedArdModule()
	if imports["sql"] != "database/sql" {
		t.Fatalf("embedded stdlib FFI imports missing sql: %#v", imports)
	}
	if imports["http"] != "net/http" {
		t.Fatalf("embedded stdlib FFI imports missing http: %#v", imports)
	}
}

func TestWriteProgramUsesEmbeddedArdModuleForReleaseVersion(t *testing.T) {
	original := version.Version
	version.Version = "v0.19.1"
	t.Cleanup(func() { version.Version = original })

	program := lowerSource(t, `
		fn main() Void {
		}
	`)
	dir := t.TempDir()
	if err := writeProgram(dir, program, Options{PackageName: "main"}); err != nil {
		t.Fatalf("writeProgram error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	goMod := string(data)
	if !strings.Contains(goMod, "require github.com/akonwi/ard v0.0.0") {
		t.Fatalf("go.mod missing Ard module requirement:\n%s", goMod)
	}
	if !strings.Contains(goMod, "replace github.com/akonwi/ard => ./.ard/ard-module") {
		t.Fatalf("release go.mod missing embedded module replace:\n%s", goMod)
	}
	if strings.Contains(goMod, "/home/runner") {
		t.Fatalf("release go.mod must not contain CI source path:\n%s", goMod)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ard", "ard-module", "runtime", "maybe.go")); err != nil {
		t.Fatalf("embedded runtime module not written: %v", err)
	}
}

func TestWriteProgramUsesLocalReplaceForDevVersion(t *testing.T) {
	original := version.Version
	version.Version = "dev"
	t.Cleanup(func() { version.Version = original })

	program := lowerSource(t, `
		fn main() Void {
		}
	`)
	dir := t.TempDir()
	if err := writeProgram(dir, program, Options{PackageName: "main"}); err != nil {
		t.Fatalf("writeProgram error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	goMod := string(data)
	if !strings.Contains(goMod, "require github.com/akonwi/ard v0.0.0") || !strings.Contains(goMod, "replace github.com/akonwi/ard =>") {
		t.Fatalf("dev go.mod missing local replace:\n%s", goMod)
	}
}

func TestBuildProgramProducesBinary(t *testing.T) {
	program := lowerSource(t, `
		fn main() Void {
			()
		}
	`)

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "ard-bin")
	builtPath, err := BuildProgram(program, outputPath)
	if err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
	if builtPath != outputPath {
		t.Fatalf("built path = %q, want %q", builtPath, outputPath)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("built binary stat error = %v", err)
	}
}

func TestRunProgramPreservesArtifactsUnderArdOut(t *testing.T) {
	program := lowerSource(t, `
		fn main() Void {
			()
		}
	`)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	projectDir := t.TempDir()
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	if err := RunProgram(program, []string{"ard", "run", "main.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(projectDir, "ard-out", "go", "run", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected generated sources under %s", filepath.Join(projectDir, "ard-out", "go", "run"))
	}
}

func TestRunBinaryNameSanitizesProjectName(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		want        string
	}{
		{name: "empty", projectName: "", want: "ard-program"},
		{name: "dot dot", projectName: "..", want: "ard-program"},
		{name: "plain", projectName: "tinear", want: "tinear"},
		{name: "hyphen", projectName: "demo-app", want: "demo-app"},
		{name: "path chars", projectName: `bad/name:with*chars?`, want: "bad_name_with_chars_"},
		{name: "only invalid chars", projectName: `/**`, want: "ard-program"},
		{name: "reserved windows name", projectName: "CON", want: "ard-CON"},
		{name: "reserved windows name with extension", projectName: "nul.txt", want: "ard-nul.txt"},
		{name: "trims spaces and dots", projectName: " team. ", want: "team"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runBinaryName(&checker.ProjectInfo{ProjectName: tt.projectName})
			if got != tt.want {
				t.Fatalf("runBinaryName(%q) = %q, want %q", tt.projectName, got, tt.want)
			}
		})
	}
	if got := runBinaryName(nil); got != "ard-program" {
		t.Fatalf("runBinaryName(nil) = %q, want ard-program", got)
	}
}

func TestRunProgramNamesBinaryAfterProject(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"tinear\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ffi.go"), []byte(`package ffi

func Lookup() int { return 1 }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`extern fn lookup() Int = "tinear.Lookup"

fn main() Int { lookup() }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}

	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}

	workspaceDir := filepath.Join(projectDir, "ard-out", "go", "run")
	ffiDirInfo, err := os.Stat(filepath.Join(workspaceDir, "tinear"))
	if err != nil || !ffiDirInfo.IsDir() {
		t.Fatalf("project FFI dir stat = %v, info = %#v", err, ffiDirInfo)
	}
	binaryInfo, err := os.Stat(filepath.Join(workspaceDir, ".bin", "tinear"))
	if err != nil || binaryInfo.IsDir() {
		t.Fatalf("project-named binary stat = %v, info = %#v", err, binaryInfo)
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "ard-program")); !os.IsNotExist(err) {
		t.Fatalf("legacy ard-program binary should not exist, stat error = %v", err)
	}
}

func TestArtifactWorkspaceUsesProjectLocalArdOut(t *testing.T) {
	projectDir := t.TempDir()
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte("fn main() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := artifactRootDir(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if root != projectDir {
		t.Fatalf("artifact root = %q, want %q", root, projectDir)
	}
	workspace, err := artifactWorkspace(mainPath, "build")
	if err != nil {
		t.Fatal(err)
	}
	if workspace != filepath.Join(projectDir, "ard-out", "go", "build") {
		t.Fatalf("workspace = %q, want %q", workspace, filepath.Join(projectDir, "ard-out", "go", "build"))
	}
}

func mapsKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func TestBuildProgramWithDirectGoCallDoesNotRequireProjectFFICompanion(t *testing.T) {
	program := lowerSource(t, `use go:strconv
fn main() Int!Str { strconv::Atoi("42") }`)
	tempDir := t.TempDir()
	if _, err := BuildProgram(program, filepath.Join(tempDir, "direct-go-call")); err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
}

func TestBuildProgramIncludesProjectGoModForDirectGoCall(t *testing.T) {
	workspace := t.TempDir()
	helperDir := filepath.Join(workspace, "helper")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{helperDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(helperDir, "go.mod"), []byte("module example.com/helper\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperDir, "helper.go"), []byte(`package helper

func Double(value int) int { return value * 2 }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf("module app\n\ngo 1.26.0\n\nrequire example.com/helper v0.0.0\nreplace example.com/helper => %s\n", helperDir)
	if err := os.WriteFile(filepath.Join(appDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:example.com/helper as helper

fn main() Int {
  helper::Double(21)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
}

func TestBuildProgramPassesArdStructToDirectGoInterface(t *testing.T) {
	workspace := t.TempDir()
	helperDir := filepath.Join(workspace, "helper")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{helperDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(helperDir, "go.mod"), []byte("module example.com/directiface\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperDir, "directiface.go"), []byte(`package directiface

type Handler interface { Handle(string) int }

func Use(handler Handler) int { return handler.Handle("hello") }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf("module app\n\ngo 1.26.0\n\nrequire example.com/directiface v0.0.0\nreplace example.com/directiface => %s\n", helperDir)
	if err := os.WriteFile(filepath.Join(appDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:example.com/directiface as directiface

struct Handler {
  offset: Int,
}

impl Handler {
  fn Handle(value: Str) Int {
    value.size() + self.offset
  }
}

fn main() {
  if not directiface::Use(Handler{offset: 2}) == 7 {
    panic("Ard struct did not satisfy Go interface")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	builtPath, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramIncludesProjectGoModForDirectGoPackageVariable(t *testing.T) {
	workspace := t.TempDir()
	helperDir := filepath.Join(workspace, "helper")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{helperDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(helperDir, "go.mod"), []byte("module example.com/directvar\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperDir, "directvar.go"), []byte(`package directvar

var Value = 7
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf("module app\n\ngo 1.26.0\n\nrequire example.com/directvar v0.0.0\nreplace example.com/directvar => %s\n", helperDir)
	if err := os.WriteFile(filepath.Join(appDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:example.com/directvar as directvar

fn answer() Int {
  directvar::Value
}

fn main() {
  if not answer() == 7 {
    panic("project direct Go package variable failed")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
}

func TestBuildProgramIncludesProjectGoModForDirectGoStructField(t *testing.T) {
	workspace := t.TempDir()
	helperDir := filepath.Join(workspace, "helper")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{helperDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(helperDir, "go.mod"), []byte("module example.com/directfield\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperDir, "directfield.go"), []byte(`package directfield

type Response struct { StatusCode int }
var DefaultResponse = Response{StatusCode: 204}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf("module app\n\ngo 1.26.0\n\nrequire example.com/directfield v0.0.0\nreplace example.com/directfield => %s\n", helperDir)
	if err := os.WriteFile(filepath.Join(appDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:example.com/directfield as directfield

fn status() Int {
  directfield::DefaultResponse.StatusCode
}

fn main() {
  if not status() == 204 {
    panic("project direct Go struct field failed")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
}

func TestBuildProgramSupportsFFIIsNilForDirectGoPointers(t *testing.T) {
	workspace := t.TempDir()
	helperDir := filepath.Join(workspace, "helper")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{helperDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(helperDir, "go.mod"), []byte("module example.com/nilcheck\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperDir, "nilcheck.go"), []byte(`package nilcheck

type Thing struct { Name string }

func Missing() *Thing { return nil }
func Present() *Thing { return &Thing{Name: "ok"} }
func MaybeThing(ok bool) (*Thing, bool) {
	if !ok {
		return nil, false
	}
	return &Thing{Name: "ok"}, true
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf("module app\n\ngo 1.26.0\n\nrequire example.com/nilcheck v0.0.0\nreplace example.com/nilcheck => %s\n", helperDir)
	if err := os.WriteFile(filepath.Join(appDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use ard/ffi
use go:example.com/nilcheck as nilcheck

fn main() {
  let missing = nilcheck::Missing()
  if not ffi::is_nil(missing) {
    panic("typed nil pointer was not nil")
  }
  let present = nilcheck::Present()
  if ffi::is_nil(present) {
    panic("non-nil pointer was nil")
  }
  let maybe_missing: (mut nilcheck::Thing)? = nilcheck::MaybeThing(false)
  if maybe_missing.is_some() {
    panic("missing maybe pointer was present")
  }
  let maybe_present: (mut nilcheck::Thing)? = nilcheck::MaybeThing(true)
  if maybe_present.is_none() {
    panic("present maybe pointer was missing")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	binaryPath, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
	cmd := exec.Command(binaryPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("generated program failed: %v\nstderr: %s", err, stderr.String())
	}
}

func TestBuildProgramCoercesUnexportedNamedScalarResult(t *testing.T) {
	workspace := t.TempDir()
	helperDir := filepath.Join(workspace, "helper")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{helperDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(helperDir, "go.mod"), []byte("module example.com/helper\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperDir, "helper.go"), []byte(`package helper

import "errors"

type hidden int64

func Hidden(ok bool) (hidden, error) {
	if !ok {
		return 0, errors.New("missing")
	}
	return 42, nil
}

func HiddenMaybe(ok bool) (hidden, bool) {
	return 42, ok
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf("module app\n\ngo 1.26.0\n\nrequire example.com/helper v0.0.0\nreplace example.com/helper => %s\n", helperDir)
	if err := os.WriteFile(filepath.Join(appDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:example.com/helper as helper

extern fn hidden(ok: Bool) Int!Str = helper::Hidden
extern fn hidden_maybe(ok: Bool) Int? = helper::HiddenMaybe

fn main() Int!Str {
  if not hidden_maybe(true).or(0) == 42 {
    panic("hidden maybe failed")
  }
  hidden(true)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
}

func TestBuildProgramIncludesProjectGoModForDirectGoEnumGlobal(t *testing.T) {
	workspace := t.TempDir()
	statusDir := filepath.Join(workspace, "status")
	appDir := filepath.Join(workspace, "app")
	for _, dir := range []string{statusDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(statusDir, "go.mod"), []byte("module example.com/status\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(statusDir, "status.go"), []byte(`package status

type State int

const (
	StateReady State = iota
	StateDone
)
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf("module app\n\ngo 1.26.0\n\nrequire example.com/status v0.0.0\nreplace example.com/status => %s\n", statusDir)
	if err := os.WriteFile(filepath.Join(appDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:example.com/status as status

let ready = status::StateReady

fn main() {
  if not ready == status::StateReady {
    panic("project direct Go enum global failed")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
}

func TestLowerDirectGoFunctionCallWithoutExtern(t *testing.T) {
	program := lowerSource(t, `use go:strconv
fn main() Int!Str { strconv::Atoi("42") }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "strconv", "strconv") {
		t.Fatal("generated AST missing strconv import")
	}
	if !astFilesHaveCall(files, "strconv.Atoi") {
		t.Fatal("generated AST missing strconv.Atoi call")
	}
	if !astFilesHaveCall(files, "fmt.Sprint") {
		t.Fatal("generated AST missing fmt.Sprint error conversion")
	}
}

func TestLowerDirectGoFunctionCallWithoutExternUsesExpectedScalarResult(t *testing.T) {
	program := lowerSource(t, `use go:strconv
fn main() Int!Str { strconv::ParseInt("42", 10, 64) }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveCall(files, "strconv.ParseInt") {
		t.Fatal("generated AST missing strconv.ParseInt call")
	}
	if !astFilesHaveCall(files, "ardDirectGoIntFromSigned") {
		t.Fatal("generated AST missing int64 return coercion")
	}
}

func TestLowerDirectGoFunctionCallWithoutExternCoercesNamedScalarReturn(t *testing.T) {
	program := lowerSource(t, `use go:time as tm
fn main() Int { tm::Since(tm::Now()) }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveCall(files, "tm.Since") {
		t.Fatal("generated AST missing tm.Since call")
	}
	if !astFilesHaveCall(files, "ardDirectGoIntFromSigned") {
		t.Fatal("generated AST missing named scalar return coercion")
	}
}

func buildProgramFromGeneratedSources(t *testing.T, program *air.Program, outputName string) {
	t.Helper()
	tempDir := t.TempDir()
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate sources: %v", err)
	}
	for name, source := range sources {
		if err := os.WriteFile(filepath.Join(tempDir, name), source, 0o644); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
	}
	goMod, err := generatedGoMod(tempDir, program, nil)
	if err != nil {
		t.Fatalf("generate go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := buildGeneratedProgram(tempDir, filepath.Join(tempDir, outputName)); err != nil {
		t.Fatalf("build generated program: %v", err)
	}
}

func TestDirectGoScalarReturnBuildsWithPredeclaredNameParameter(t *testing.T) {
	program := lowerSource(t, `use go:math/rand as rand
fn foo(int64: Int) Int { rand::Int63() }
fn main() { foo(0) }`)
	buildProgramFromGeneratedSources(t, program, "direct-go-predeclared-param")
}

func TestInlineClosureBuildsWithPredeclaredNameParameter(t *testing.T) {
	program := lowerSource(t, `use ard/maybe
fn main() {
  maybe::some(1).map(fn(int64) { int64 + 1 }).or(0)
}`)
	buildProgramFromGeneratedSources(t, program, "inline-closure-predeclared-param")
}

func TestInlineClosureBuildsWhenParamCollidesWithCaptureRewrite(t *testing.T) {
	program := lowerSource(t, `use ard/maybe
fn main() {
  let x_0 = 10
  maybe::some(1).map(fn(x) { x + x_0 }).or(0)
}`)
	buildProgramFromGeneratedSources(t, program, "inline-closure-capture-param-collision")
}

func TestFunctionBuildsWithReservedAndSuffixedParameterNames(t *testing.T) {
	program := lowerSource(t, `fn foo(int64: Int, int64_0: Int) Int { int64_0 }
fn main() { foo(0, 1) }`)
	buildProgramFromGeneratedSources(t, program, "reserved-suffixed-params")
}

func TestLowerDirectGoInstanceMethodCallWithoutExtern(t *testing.T) {
	program := lowerSource(t, `use go:time
fn main() Str { time::Now().Format("2006-01-02") }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "time", "time") {
		t.Fatal("generated AST missing time import")
	}
	if !astFilesHaveCall(files, "time.Now") {
		t.Fatal("generated AST missing time.Now call")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		return ok && selector.Sel != nil && selector.Sel.Name == "Format"
	}) {
		t.Fatal("generated AST missing Format method call")
	}
}

func TestLowerDirectGoInstanceMethodCallWithoutExternUsesExpectedScalarResult(t *testing.T) {
	program := lowerSource(t, `use go:time
fn main() Int { time::Now().UnixNano() }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveCall(files, "time.Now") {
		t.Fatal("generated AST missing time.Now call")
	}
	if !astFilesHaveCall(files, "ardDirectGoIntFromSigned") {
		t.Fatal("generated AST missing int64 method return coercion")
	}
}

func TestLowerDirectGoExternFunctionCall(t *testing.T) {
	program := lowerSource(t, `use go:math
extern fn floor(value: Float) Float = math::Floor
fn main() Float { floor(1.2) }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "math", "math") {
		t.Fatal("generated AST missing math import")
	}
	if !astFilesHaveCall(files, "math.Floor") {
		t.Fatal("generated AST missing math.Floor call")
	}
}

func TestLowererDetectsImportAliasPathConflicts(t *testing.T) {
	l := &lowerer{currentImports: map[string]string{}}
	l.registerImport("fmt", "example.com/fmt")
	l.registerImport("fmt", "fmt")
	if l.importErr == nil || !strings.Contains(l.importErr.Error(), `Go import alias "fmt" used for both`) {
		t.Fatalf("importErr = %v, want alias conflict", l.importErr)
	}
}

func TestLowerDirectGoExternUsesImportAlias(t *testing.T) {
	program := lowerSource(t, `use go:math/rand as mathrand
extern fn intn(max: Int) Int = mathrand::Intn
fn main() Int { intn(10) }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "mathrand", "math/rand") {
		t.Fatal("generated AST missing mathrand import alias")
	}
	if !astFilesHaveCall(files, "mathrand.Intn") {
		t.Fatal("generated AST missing mathrand.Intn call")
	}
}

func TestLowerDirectGoExternAliasAvoidsPredeclaredIdentifiers(t *testing.T) {
	program := lowerSource(t, `use go:math as int
extern fn floor(value: Float) Float = int::Floor
fn use_floor() Float { floor(1.2) }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "int_1", "math") {
		t.Fatal("generated AST missing collision-free int_1 import alias")
	}
	if !astFilesHaveCall(files, "int_1.Floor") {
		t.Fatal("generated AST missing int_1.Floor call")
	}
}

func TestGeneratedGoImportAliasAvoidsTempPrefix(t *testing.T) {
	l := &lowerer{directGoAliases: map[string]string{}, reservedGoIdentifiers: collectReservedGoIdentifiers(nil)}
	alias := l.generatedGoImportAlias("math", "_tmp_0")
	if alias != "ardgo" {
		t.Fatalf("alias = %q, want ardgo", alias)
	}
}

func TestDirectGoTypeExprAllocatesAliasesForSecondaryPackages(t *testing.T) {
	l := &lowerer{currentImports: map[string]string{}, directGoAliases: map[string]string{}, reservedGoIdentifiers: map[string]bool{"b": true}}
	expr, err := l.directGoTypeExpr(checker.GoValueType{Expr: "b.MyInt", ImportPath: "example.com/cross/b", Package: "b", Name: "MyInt", Named: true, Kind: checker.GoValueInt}, directGoExternBinding{ImportPath: "example.com/cross/a", Alias: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if got := astExprName(expr); got != "b_1.MyInt" {
		t.Fatalf("type expr = %q, want b_1.MyInt", got)
	}
	if got := l.currentImports["b_1"]; got != "example.com/cross/b" {
		t.Fatalf("registered import = %q, want example.com/cross/b", got)
	}
}

func TestDirectGoExternAliasAvoidsTypeSpecificJSONHelperNames(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{{ID: 2, Kind: air.TypeInt, Name: "Int"}}}
	l := &lowerer{program: program, directGoAliases: map[string]string{}, reservedGoIdentifiers: collectReservedGoIdentifiers(program)}
	alias := l.directGoBindingAlias(directGoExternBinding{ImportPath: "math", Alias: "ardJSONEncode_2"})
	if alias != "ardJSONEncode_2_1" {
		t.Fatalf("alias = %q, want ardJSONEncode_2_1", alias)
	}
}

func TestLowerDirectGoExternAliasAvoidsRuntimeHelperNames(t *testing.T) {
	program := lowerSource(t, `use go:unicode/utf8 as ardDirectGoCheckSignedIntRange
extern fn rune_len(value: Int) Int = ardDirectGoCheckSignedIntRange::RuneLen
fn main() Int { rune_len(65) }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "ardDirectGoCheckSignedIntRange_1", "unicode/utf8") {
		t.Fatal("generated AST missing collision-free runtime-helper import alias")
	}
	if !astFilesHaveCall(files, "ardDirectGoCheckSignedIntRange_1.RuneLen") {
		t.Fatal("generated AST missing suffixed RuneLen call")
	}
	if !astFilesHaveCall(files, "ardDirectGoCheckSignedIntRange") {
		t.Fatal("generated AST missing signed range helper call")
	}
}

func TestLowerDirectGoExternAliasAvoidsSortComparatorParams(t *testing.T) {
	program := lowerSource(t, `use go:strings as i
extern fn eq(a: Str, b: Str) Bool = i::EqualFold
fn main() {
  mut values = ["a", "B"]
  values.sort(fn(a: Str, b: Str) Bool { eq(a, b) })
}`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "i_1", "strings") {
		t.Fatal("generated AST missing collision-free i_1 import alias")
	}
	if !astFilesHaveCall(files, "i_1.EqualFold") {
		t.Fatal("generated AST missing i_1.EqualFold call")
	}
}

func TestLowerDirectGoExternAliasAvoidsLocalShadowing(t *testing.T) {
	program := lowerSource(t, `use go:math as m
extern fn floor(value: Float) Float = m::Floor
fn use_floor(m: Int) Float { floor(1.2) }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "m", "math") {
		t.Fatal("generated AST missing m import alias")
	}
	if !astFilesHaveCall(files, "m.Floor") {
		t.Fatal("generated AST missing m.Floor call")
	}
}

func TestLowerDirectGoTypeReference(t *testing.T) {
	program := lowerSource(t, `use go:time as tm
extern fn sleep(duration: tm::Duration) Void = tm::Sleep
fn use_duration(duration: tm::Duration) {
  sleep(duration)
}`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "tm", "time") {
		t.Fatal("generated AST missing tm import")
	}
	if !astFilesHaveSelector(files, "tm", "Duration") {
		t.Fatal("generated AST missing tm.Duration type reference")
	}
}

func TestLowerDirectGoEnumLikeConstants(t *testing.T) {
	program := lowerSource(t, `use go:time
extern fn now() time::Time = time::Now
extern fn month(value: time::Time) time::Month = time::Time::Month
fn month_number(value: time::Month) Int {
  match value {
    time::January => 1
    _ => 0
  }
}
fn main() Int { month_number(month(now())) }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "time", "time") {
		t.Fatal("generated AST missing time import")
	}
	if !astFilesHaveSelector(files, "time", "Month") {
		t.Fatal("generated AST missing time.Month type alias")
	}
	if !astFilesHaveSelector(files, "time", "January") {
		t.Fatal("generated AST missing time.January enum constant")
	}
}

func TestLowerDirectGoClosedEnumReturnValidation(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{{
		ID:            1,
		Kind:          air.TypeEnum,
		Name:          "State",
		ExternBinding: "go:example.com/status::State",
		Variants:      []air.VariantInfo{{Name: "StateReady", Discriminant: 0}, {Name: "StateDone", Discriminant: 1}},
	}}}
	l := &lowerer{program: program}
	checked, ok, err := l.validateDirectGoReturnValue(1, ast.NewIdent("value"), checker.GoValueType{Kind: checker.GoValueInt, Expr: "status.State", Named: true, ImportPath: "example.com/status", Name: "State"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected closed enum return validation")
	}
	foundPanic := false
	ast.Inspect(checked, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok && astCallName(call) == "panic" {
			foundPanic = true
			return false
		}
		return true
	})
	if !foundPanic {
		t.Fatalf("validation expression = %#v, want panic path", checked)
	}
}

func TestDirectGoEnumLikeConstantsBuild(t *testing.T) {
	program := lowerSource(t, `use go:time
extern fn now() time::Time = time::Now
extern fn month(value: time::Time) time::Month = time::Time::Month
fn main() Int {
  match month(now()) {
    time::January => 1
    _ => 0
  }
}`)
	tempDir := t.TempDir()
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate sources: %v", err)
	}
	for name, source := range sources {
		if err := os.WriteFile(filepath.Join(tempDir, name), source, 0o644); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
	}
	goMod, err := generatedGoMod(tempDir, program, nil)
	if err != nil {
		t.Fatalf("generate go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := buildGeneratedProgram(tempDir, filepath.Join(tempDir, "direct-go-enum")); err != nil {
		t.Fatalf("build generated program: %v", err)
	}
}

func TestDirectGoPointerReturnAndMethodBuilds(t *testing.T) {
	program := lowerSource(t, `use go:os
extern fn create_temp(dir: Str, pattern: Str) (mut os::File)!Str = os::CreateTemp
extern fn name(file: mut os::File) Str = os::File::Name
extern fn close(file: mut os::File) Void!Str = os::File::Close
extern fn remove(path: Str) Void!Str = os::Remove
fn main() Void!Str {
  let file = try create_temp("", "ard-direct-go-pointer-*")
  let path = name(file)
  try close(file)
  remove(path)
}`)
	tempDir := t.TempDir()
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate sources: %v", err)
	}
	for name, source := range sources {
		if err := os.WriteFile(filepath.Join(tempDir, name), source, 0o644); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
	}
	goMod, err := generatedGoMod(tempDir, program, nil)
	if err != nil {
		t.Fatalf("generate go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	binaryPath := filepath.Join(tempDir, "direct-go-pointer")
	if err := buildGeneratedProgram(tempDir, binaryPath); err != nil {
		t.Fatalf("build generated program: %v", err)
	}
	cmd := exec.Command(binaryPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("generated program failed: %v\nstderr: %s", err, stderr.String())
	}
}

func TestDirectGoInterfaceAssignabilityBuilds(t *testing.T) {
	program := lowerSource(t, `use go:io
use go:net/http as http
use go:strings

fn read_all(reader: io::Reader) [Byte]!Str {
  io::ReadAll(reader)
}

fn close_body(resp: mut http::Response) Void!Str {
  resp.Body.Close()
}

fn main() {
  let bytes = read_all(strings::NewReader("hello")).expect("read")
  if not Str::from_bytes(bytes).expect("utf8") == "hello" {
    panic("bad read")
  }
}`)
	buildProgramFromGeneratedSources(t, program, "direct-go-interface-assignability")
}

func TestLowerDirectGoExternCoercesNamedScalarArguments(t *testing.T) {
	program := lowerSource(t, `use go:time as tm
extern fn sleep(ms: Int) Void = tm::Sleep
fn main() {
  let ms = 1
  sleep(ms)
}`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "tm", "time") {
		t.Fatal("generated AST missing tm import")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || astCallName(call) != "tm.Sleep" || len(call.Args) != 1 {
			return false
		}
		conversion, ok := call.Args[0].(*ast.CallExpr)
		return ok && astCallName(conversion) == "tm.Duration"
	}) {
		t.Fatal("generated AST missing tm.Duration argument conversion")
	}
}

func TestLowerDirectGoExternCoercesNamedScalarReturn(t *testing.T) {
	program := lowerSource(t, `use go:time as tm
extern fn now() tm::Time = tm::Now
extern fn since(value: tm::Time) Int = tm::Since
fn main() Int { since(now()) }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveCall(files, "tm.Since") {
		t.Fatal("generated AST missing tm.Since call")
	}
	if !astFilesHaveCall(files, "ardDirectGoIntFromSigned") {
		t.Fatal("generated AST missing signed return-to-int helper")
	}
}

func TestLowerDirectGoExternChecksUnsignedArgumentRange(t *testing.T) {
	program := lowerSource(t, `use go:strings
extern fn index_byte(value: Str, byte: Int) Int = strings::IndexByte
fn main() Int { index_byte("abc", 98) }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveCall(files, "strings.IndexByte") {
		t.Fatal("generated AST missing strings.IndexByte call")
	}
	if !astFilesHaveCall(files, "ardDirectGoCheckUintIntRange") {
		t.Fatal("generated AST missing uint range check")
	}
}

func TestDirectGoExternRangeCheckPanicsBeforeUnsignedWrap(t *testing.T) {
	program := lowerSource(t, `use go:strings
extern fn index_byte(value: Str, byte: Int) Int = strings::IndexByte
fn main() Int { index_byte("abc", 300) }`)
	tempDir := t.TempDir()
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate sources: %v", err)
	}
	for name, source := range sources {
		if err := os.WriteFile(filepath.Join(tempDir, name), source, 0o644); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
	}
	goMod, err := generatedGoMod(tempDir, program, nil)
	if err != nil {
		t.Fatalf("generate go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	binaryPath := filepath.Join(tempDir, "range-check")
	if err := buildGeneratedProgram(tempDir, binaryPath); err != nil {
		t.Fatalf("build generated program: %v", err)
	}
	cmd := exec.Command(binaryPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("generated program succeeded; expected range check panic")
	}
	if !strings.Contains(stderr.String(), "int value out of range for byte") {
		t.Fatalf("stderr = %q, want range check panic", stderr.String())
	}
}

func TestLowerDirectGoExternChecksSignedArgumentRange(t *testing.T) {
	program := lowerSource(t, `use go:unicode/utf8 as utf8
extern fn rune_len(value: Int) Int = utf8::RuneLen
fn main() Int { rune_len(65) }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveCall(files, "utf8.RuneLen") {
		t.Fatal("generated AST missing utf8.RuneLen call")
	}
	if !astFilesHaveCall(files, "ardDirectGoCheckSignedIntRange") {
		t.Fatal("generated AST missing signed range check")
	}
}

func TestLowerDirectGoExternChecksFloat32ArgumentRange(t *testing.T) {
	l := &lowerer{program: &air.Program{Types: []air.TypeInfo{{ID: 1, Kind: air.TypeFloat}}}, runtimeHelpers: map[string]bool{}}
	checked := l.checkedDirectGoArg(1, ast.NewIdent("value"), checker.GoValueType{Kind: checker.GoValueFloat, Bits: 32, Expr: "float32"})
	call, ok := checked.(*ast.CallExpr)
	if !ok || astCallName(call) != "ardDirectGoCheckFloat32Range" {
		t.Fatalf("checked arg = %#v, want ardDirectGoCheckFloat32Range call", checked)
	}
	if !l.runtimeHelpers["direct_go_float32_range"] {
		t.Fatal("float32 range helper was not marked")
	}
}

func TestLowerDirectGoExternValidatesRuneReturn(t *testing.T) {
	program := lowerSource(t, `use go:unicode
extern fn upper(value: Rune) Rune = unicode::ToUpper
fn main() Rune { upper('a') }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveCall(files, "unicode.ToUpper") {
		t.Fatal("generated AST missing unicode.ToUpper call")
	}
	if !astFilesHaveCall(files, "ardDirectGoCheckRune") {
		t.Fatal("generated AST missing rune validation")
	}
	if !astFilesHaveImport(files, "ardutf8", "unicode/utf8") {
		t.Fatal("generated AST missing private unicode/utf8 import for rune validation")
	}
}

func TestLowerDirectGoExternAdaptsErrorReturnToResult(t *testing.T) {
	program := lowerSource(t, `use go:os
extern fn remove(path: Str) Void!Str = os::Remove
fn main() Void!Str { remove("missing") }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "os", "os") {
		t.Fatal("generated AST missing os import")
	}
	if !astFilesHaveCall(files, "os.Remove") {
		t.Fatal("generated AST missing os.Remove call")
	}
	if !astFilesHaveCall(files, "fmt.Sprint") {
		t.Fatal("generated AST missing fmt.Sprint error conversion")
	}
}

func TestLowerDirectGoExternAdaptsValueErrorReturnToResult(t *testing.T) {
	program := lowerSource(t, `use go:strconv
extern fn atoi(value: Str) Int!Str = strconv::Atoi
fn main() Int!Str { atoi("42") }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "strconv", "strconv") {
		t.Fatal("generated AST missing strconv import")
	}
	if !astFilesHaveCall(files, "strconv.Atoi") {
		t.Fatal("generated AST missing strconv.Atoi call")
	}
	if !astFilesHaveCall(files, "fmt.Sprint") {
		t.Fatal("generated AST missing fmt.Sprint error conversion")
	}
}

func TestDirectGoExternAdaptsInt64ErrorReturnToIntResultBuild(t *testing.T) {
	program := lowerSource(t, `use go:strconv
extern fn parse_int(value: Str, base: Int, bits: Int) Int!Str = strconv::ParseInt
fn main() Int!Str { parse_int("42", 10, 64) }`)
	tempDir := t.TempDir()
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate sources: %v", err)
	}
	for name, source := range sources {
		if err := os.WriteFile(filepath.Join(tempDir, name), source, 0o644); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
	}
	goMod, err := generatedGoMod(tempDir, program, nil)
	if err != nil {
		t.Fatalf("generate go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := buildGeneratedProgram(tempDir, filepath.Join(tempDir, "direct-go-int64-result")); err != nil {
		t.Fatalf("build generated program: %v", err)
	}
}

func TestLowerDirectGoExternAdaptsValueBoolReturnToMaybe(t *testing.T) {
	program := lowerSource(t, `use go:os
extern fn lookup_env(key: Str) Str? = os::LookupEnv
fn main() Str? { lookup_env("PATH") }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveImport(files, "os", "os") {
		t.Fatal("generated AST missing os import")
	}
	if !astFilesHaveCall(files, "os.LookupEnv") {
		t.Fatal("generated AST missing os.LookupEnv call")
	}
	if !astFilesHaveCall(files, "ardruntime.Some") {
		t.Fatal("generated AST missing ardruntime.Some maybe adapter")
	}
	if !astFilesHaveCall(files, "ardruntime.None") {
		t.Fatal("generated AST missing ardruntime.None maybe adapter")
	}
}

func TestLowerStructFieldsUseNaturalVisibilityNames(t *testing.T) {
	program := lowerSource(t, `struct User {
  first_name: Str
  type: Int
}

private struct internal_config {
  secret_key: Str
}

fn make_user() User { User{first_name: "Ada", type: 1} }
fn main() internal_config { internal_config{secret_key: "s"} }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	for _, field := range []string{"FirstName", "Type"} {
		if !astFilesHaveStructField(files, "User", field) {
			t.Fatalf("generated public User missing exported field %s", field)
		}
	}
	if !astFilesHaveStructField(files, "internalConfig", "secretKey") {
		t.Fatal("generated private internal_config missing unexported natural field secretKey")
	}
}

func astFilesHaveStructField(files map[string]*ast.File, typeName string, fieldName string) bool {
	return astFilesContain(files, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != typeName {
			return false
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok || structType.Fields == nil {
			return false
		}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if name.Name == fieldName {
					return true
				}
			}
		}
		return false
	})
}

func lowerSource(t *testing.T, input string) *air.Program {
	t.Helper()
	return lowerSourceWithCheckOptions(t, input, checker.CheckOptions{})
}

func lowerSourceWithCheckOptions(t *testing.T, input string, options checker.CheckOptions) *air.Program {
	t.Helper()
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil, options)
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
