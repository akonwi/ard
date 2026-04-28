package go_backend

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/akonwi/ard/checker"
	backendir "github.com/akonwi/ard/go_backend/ir"
)

func TestCompileModuleSourceViaBackendIR_EntrypointParses(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn add(a: Int, b: Int) Int {
  a + b
}

fn main() {
  let total = add(1, 2)
  add(total, 3)
}
`)

	standardOut, err := compileModuleSource(module, "main", true, "")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	irOut, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}

	assertParsesAsGo(t, standardOut)
	assertParsesAsGo(t, irOut)

	generated := string(irOut)
	for _, want := range []string{
		"package main",
		"func Add(a int, b int) int",
		"return a + b",
		"func Main()",
		"func main()",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected backend IR output to contain %q\n%s", want, generated)
		}
	}
}

func TestCompileModuleSourceViaBackendIR_EntrypointSkipsDeclarationOnlyUnionExternTypeStatements(t *testing.T) {
	// Synthetic statement-level filter verification: declaration-only forms
	// (function, extern, struct, enum, union, extern-type) must be excluded
	// from the executable entrypoint stream so the entrypoint block stays
	// declaration-safe even when a future checker pass surfaces them.
	syntheticDeclStatements := []checker.Statement{
		{Expr: &checker.FunctionDef{Name: "fn_decl"}},
		{Expr: &checker.ExternalFunctionDef{Name: "extern_fn"}},
		{Stmt: &checker.StructDef{Name: "S"}},
		{Stmt: &checker.Enum{Name: "E"}},
		{Stmt: &checker.Union{Name: "U", Types: []checker.Type{checker.Int, checker.Str}}},
		{Stmt: checker.Union{Name: "UVal", Types: []checker.Type{checker.Int, checker.Str}}},
		{Stmt: &checker.ExternType{Name_: "Handle"}},
	}
	executableStatement := checker.Statement{
		Stmt: &checker.VariableDef{Name: "x", Value: &checker.IntLiteral{Value: 1}},
	}

	allStatements := append([]checker.Statement{}, syntheticDeclStatements...)
	allStatements = append(allStatements, executableStatement)

	filtered := topLevelExecutableStatements(allStatements)
	if len(filtered) != 1 {
		t.Fatalf("expected only the executable variable statement to remain, got %d statements: %#v", len(filtered), filtered)
	}
	if vd, ok := filtered[0].Stmt.(*checker.VariableDef); !ok || vd.Name != "x" {
		t.Fatalf("expected the executable VariableDef to remain after filtering, got %#v", filtered[0])
	}

	// Integration lowering: a module containing extern-type and other
	// declaration forms must produce an entrypoint block that is free of
	// any declaration-marker calls (`union_decl_stmt`,
	// `extern_type_decl_stmt`, `struct_decl_stmt`, `enum_decl_stmt`,
	// `nonproducing_stmt`).
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Square { size: Int }
struct Circle { radius: Int }

type Shape = Square | Circle

extern type Handle

fn label(shape: Shape) Str {
  match shape {
    Square => "square",
    Circle => "circle",
  }
}

let first = Square { size: 1 }
let kind = label(first)
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	if irModule.Entrypoint == nil {
		t.Fatalf("expected entrypoint block to be lowered for entrypoint module")
	}

	declarationMarkers := []string{
		"union_decl_stmt",
		"extern_type_decl_stmt",
		"struct_decl_stmt",
		"enum_decl_stmt",
		"nonproducing_stmt",
	}
	for _, marker := range declarationMarkers {
		if containsCallNamedInBlock(irModule.Entrypoint, marker) {
			t.Fatalf("expected entrypoint block to be free of %q marker call, got: %#v", marker, irModule.Entrypoint)
		}
	}

	// Extern-type declaration form must still be preserved as a module-level
	// decl even though it is excluded from the executable entrypoint stream.
	hasExternTypeDecl := false
	for _, decl := range irModule.Decls {
		if d, ok := decl.(*backendir.ExternTypeDecl); ok && d.Name == "Handle" {
			hasExternTypeDecl = true
		}
	}
	if !hasExternTypeDecl {
		t.Fatalf("expected backend IR module to retain extern-type decl Handle, got decls: %#v", irModule.Decls)
	}

	// The entrypoint should still preserve genuinely executable top-level
	// statements (the let bindings).
	if len(irModule.Entrypoint.Stmts) == 0 {
		t.Fatalf("expected entrypoint block to retain executable statements, got empty block")
	}

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)

	generated := string(out)
	for _, unwanted := range declarationMarkers {
		if strings.Contains(generated, unwanted) {
			t.Fatalf("expected generated source to be free of marker %q\n%s", unwanted, generated)
		}
	}
	for _, want := range []string{
		"type Handle struct",
		"func main()",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected generated source to contain %q\n%s", want, generated)
		}
	}
}

func TestCompileModuleSourceViaBackendIR_ExternWrapper(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
extern fn now() Int = "Now"

fn main() {
  now()
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)

	generated := string(out)
	if !strings.Contains(generated, "CallExtern(\"Now\"") {
		t.Fatalf("expected extern wrapper to call CallExtern\n%s", generated)
	}
	if !strings.Contains(generated, "CoerceExtern[int](result)") {
		t.Fatalf("expected extern wrapper to coerce result\n%s", generated)
	}
}

func TestCompileModuleSourceViaBackendIR_ExternTraitParamSignatureNative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/string as Str

extern fn show(value: Str::ToString) Void = "Show"

fn main() {
  show("ok")
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)

	generated := string(out)
	if !strings.Contains(generated, "func Show(value ardgo.ToString)") {
		t.Fatalf("expected trait-typed extern signature to stay concrete in native emission\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_TraitSignatureAndCoercionWithoutSourceModule(t *testing.T) {
	traitType := &backendir.TraitType{Name: "ToString"}
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name: "show",
				Params: []backendir.Param{{
					Name: "value",
					Type: traitType,
				}},
				Return: backendir.Void,
				Body:   &backendir.Block{},
			},
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{Stmts: []backendir.Stmt{
					&backendir.ExprStmt{Value: &backendir.CallExpr{
						Callee: &backendir.IdentExpr{Name: "show"},
						Args: []backendir.Expr{
							&backendir.TraitCoerceExpr{
								Value: &backendir.LiteralExpr{Kind: "str", Value: "ok"},
								Type:  traitType,
							},
						},
					}},
				}},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	for _, want := range []string{
		"func Show(value ardgo.ToString)",
		"Show(ardgo.AsToString(\"ok\"))",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected generated source to contain %q\n%s", want, generated)
		}
	}
}

func TestEmitGoFileFromBackendIR_UnionAndExternTypeDecls(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.UnionDecl{
				Name:  "Value",
				Types: []backendir.Type{backendir.IntType, backendir.StrType},
			},
			&backendir.ExternTypeDecl{
				Name: "Handle",
			},
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body:   &backendir.Block{},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	for _, want := range []string{
		"type Value interface",
		"type Handle struct",
		"func Main()",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected generated source to contain %q\n%s", want, generated)
		}
	}
}

func TestEmitGoFileFromBackendIR_UnionTypedFunctionSignatureNative(t *testing.T) {
	// Union-typed function parameters and return types must emit native
	// Go signatures referencing the concrete union interface name (e.g.
	// `Shape`) instead of falling back to `any`. This relies on
	// lowerCheckerTypeToBackendIR mapping union types to NamedType so
	// the emitter can resolve them to the interface declared via
	// UnionDecl emission.
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.UnionDecl{
				Name:  "Shape",
				Types: []backendir.Type{backendir.IntType, backendir.StrType},
			},
			&backendir.FuncDecl{
				Name: "describe",
				Params: []backendir.Param{{
					Name: "shape",
					Type: &backendir.NamedType{Name: "Shape"},
				}},
				Return: &backendir.NamedType{Name: "Shape"},
				Body: &backendir.Block{Stmts: []backendir.Stmt{
					&backendir.ReturnStmt{Value: &backendir.IdentExpr{Name: "shape"}},
				}},
			},
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body:   &backendir.Block{},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	for _, want := range []string{
		"type Shape interface",
		"func Describe(shape Shape) Shape",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected generated source to contain %q\n%s", want, generated)
		}
	}
	// The native union-typed signature must not erase the parameter or
	// return type to `any`.
	for _, unwanted := range []string{
		"func Describe(shape any)",
		"shape any) any",
	} {
		if strings.Contains(generated, unwanted) {
			t.Fatalf("expected generated source to be free of erased signature %q\n%s", unwanted, generated)
		}
	}
}

func TestEmitGoFileFromBackendIR_UnionDeclFromOrphanTypeReference(t *testing.T) {
	// Verify that a union type alias referenced through a function
	// signature (rather than declared as an explicit Statement) is
	// surfaced into backend IR module declarations and emitted as a
	// native Go interface. This guarantees the orphan-union collection
	// path drives declaration emission without depending on special-case
	// declaration handling.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Square { size: Int }
struct Circle { radius: Int }

type Shape = Square | Circle

fn label(shape: Shape) Str {
  match shape {
    Square => "square",
    Circle => "circle",
  }
}

fn main() {
  let s = Square{size: 1}
  let kind = label(s)
  let _ = kind
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	hasShapeUnion := false
	for _, decl := range irModule.Decls {
		if u, ok := decl.(*backendir.UnionDecl); ok && u.Name == "Shape" {
			hasShapeUnion = true
			break
		}
	}
	if !hasShapeUnion {
		t.Fatalf("expected backend IR module to include orphan UnionDecl for Shape, got decls: %#v", irModule.Decls)
	}

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)

	generated := string(out)
	for _, want := range []string{
		"type Shape interface",
		"type Square struct",
		"type Circle struct",
		"func main()",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected generated source to contain %q\n%s", want, generated)
		}
	}
	for _, unwanted := range []string{
		"union_decl_stmt",
		"struct_decl_stmt",
		"nonproducing_stmt",
	} {
		if strings.Contains(generated, unwanted) {
			t.Fatalf("expected generated source to be free of %q marker\n%s", unwanted, generated)
		}
	}
}

func TestCompileModuleSourceViaBackendIR_FunctionCallBodyTraitWrappingNative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/io

fn show(x: Int) {
  io::print(x)
}

fn main() {
  show(1)
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)

	generated := string(out)
	if !strings.Contains(generated, "io.Print(ardgo.AsToString") {
		t.Fatalf("expected native backend IR emission to preserve trait wrapping for io::print call\n%s", generated)
	}
}

func TestCompileModuleSourceViaBackendIR_EntrypointModuleCallsNative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/io

io::print("ok")
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)

	generated := string(out)
	if !strings.Contains(generated, "io.Print(ardgo.AsToString(\"ok\"))") {
		t.Fatalf("expected native entrypoint emission to preserve trait wrapping for io::print call\n%s", generated)
	}
}

func TestCompileModuleSourceViaBackendIR_MutableFunctionParamsNative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/io

struct Box {
  value: Int,
}

fn set_box(mut box: Box) {
  box.value = 2
}

fn bump(mut value: Int) {
  value = value + 1
}

fn append_one(mut values: [Int]) {
  values.push(1)
}

fn main() {
  mut box = Box{value: 1}
  set_box(box)
  io::print(box.value)

  mut value = 1
  bump(value)
  io::print(value)

  mut values = [1]
  append_one(values)
  io::print(values.size())
  io::print(values.at(1))
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)

	generated := string(out)
	for _, want := range []string{
		"func SetBox(box *Box)",
		"func AppendOne(values *[]int)",
		"func Bump(value int)",
		"SetBox(&box)",
		"AppendOne(&values)",
		"Bump(value)",
		"(*box).Value = 2",
		"value = value + 1",
		"*values = append(*values, 1)",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected generated source to contain %q\n%s", want, generated)
		}
	}
}

func TestCompileModuleSourceViaBackendIR_MutatingMethodAssignment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Box {
  value: Int,
}

impl Box {
  fn mut set(value: Int) {
    self.value = value
  }
}

fn main() {
  mut box = Box{value: 1}
  box.set(2)
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)

	generated := string(out)
	if !strings.Contains(generated, "func (self *Box) Set(value int)") {
		t.Fatalf("expected mutating receiver method to compile with pointer receiver\n%s", generated)
	}
	if !strings.Contains(generated, "(*self).Value = value") {
		t.Fatalf("expected mutating receiver method assignment to compile natively\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_EntrypointBlockNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Entrypoint: &backendir.Block{
			Stmts: []backendir.Stmt{
				&backendir.AssignStmt{Target: "sum", Value: &backendir.CallExpr{
					Callee: &backendir.IdentExpr{Name: "int_add"},
					Args: []backendir.Expr{
						&backendir.LiteralExpr{Kind: "int", Value: "1"},
						&backendir.LiteralExpr{Kind: "int", Value: "2"},
					},
				}},
				&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "sum"}},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	for _, want := range []string{
		"sum := 1 + 2",
		"_ = sum",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected generated entrypoint source to contain %q\n%s", want, generated)
		}
	}
}

func TestEmitGoFileFromBackendIR_MemberAssignStmtNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.StructDecl{
				Name: "Box",
				Fields: []backendir.Field{
					{Name: "value", Type: backendir.IntType},
				},
			},
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.MemberAssignStmt{
							Subject: &backendir.IdentExpr{Name: "self"},
							Field:   "value",
							Value:   &backendir.LiteralExpr{Kind: "int", Value: "2"},
						},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "self.Value = 2") {
		t.Fatalf("expected generated source to contain native member assignment\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_LoopStatementsNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name: "less",
				Params: []backendir.Param{
					{Name: "a", Type: backendir.IntType},
					{Name: "b", Type: backendir.IntType},
				},
				Return: backendir.BoolType,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.ReturnStmt{
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "int_lt"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "a"},
									&backendir.IdentExpr{Name: "b"},
								},
							},
						},
					},
				},
			},
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.ForIntRangeStmt{
							Cursor: "i",
							Start:  &backendir.LiteralExpr{Kind: "int", Value: "0"},
							End:    &backendir.LiteralExpr{Kind: "int", Value: "2"},
							Body: &backendir.Block{
								Stmts: []backendir.Stmt{
									&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "i"}},
								},
							},
						},
						&backendir.AssignStmt{Target: "n", Value: &backendir.LiteralExpr{Kind: "int", Value: "0"}},
						&backendir.WhileStmt{
							Cond: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "int_lt"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "n"},
									&backendir.LiteralExpr{Kind: "int", Value: "3"},
								},
							},
							Body: &backendir.Block{
								Stmts: []backendir.Stmt{
									&backendir.AssignStmt{
										Target: "n",
										Value: &backendir.CallExpr{
											Callee: &backendir.IdentExpr{Name: "int_add"},
											Args: []backendir.Expr{
												&backendir.IdentExpr{Name: "n"},
												&backendir.LiteralExpr{Kind: "int", Value: "1"},
											},
										},
									},
									&backendir.BreakStmt{},
								},
							},
						},
						&backendir.ForLoopStmt{
							InitName:  "j",
							InitValue: &backendir.LiteralExpr{Kind: "int", Value: "0"},
							Cond: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "int_lt"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "j"},
									&backendir.LiteralExpr{Kind: "int", Value: "2"},
								},
							},
							Update: &backendir.AssignStmt{
								Target: "j",
								Value: &backendir.CallExpr{
									Callee: &backendir.IdentExpr{Name: "int_add"},
									Args: []backendir.Expr{
										&backendir.IdentExpr{Name: "j"},
										&backendir.LiteralExpr{Kind: "int", Value: "1"},
									},
								},
							},
							Body: &backendir.Block{
								Stmts: []backendir.Stmt{
									&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "j"}},
								},
							},
						},
						&backendir.ForInListStmt{
							Cursor: "value",
							Index:  "idx",
							List:   &backendir.IdentExpr{Name: "values"},
							Body: &backendir.Block{
								Stmts: []backendir.Stmt{
									&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "value"}},
									&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "idx"}},
								},
							},
						},
						&backendir.ForInStrStmt{
							Cursor: "char",
							Index:  "charIndex",
							Value:  &backendir.IdentExpr{Name: "text"},
							Body: &backendir.Block{
								Stmts: []backendir.Stmt{
									&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "char"}},
									&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "charIndex"}},
								},
							},
						},
						&backendir.ForInMapStmt{
							Key:   "key",
							Value: "value",
							Map:   &backendir.IdentExpr{Name: "items"},
							Body: &backendir.Block{
								Stmts: []backendir.Stmt{
									&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "key"}},
									&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "value"}},
								},
							},
						},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "for i := 0; i <= 2; i++") {
		t.Fatalf("expected generated source to contain native for-int-range loop\n%s", generated)
	}
	if !strings.Contains(generated, "for n < 3") {
		t.Fatalf("expected generated source to contain native while loop\n%s", generated)
	}
	if !strings.Contains(generated, "break") {
		t.Fatalf("expected generated source to contain native break statement\n%s", generated)
	}
	if !strings.Contains(generated, "for j := 0; j < 2; j = j + 1") {
		t.Fatalf("expected generated source to contain native for-loop statement\n%s", generated)
	}
	if !strings.Contains(generated, "for idx, value := range values") {
		t.Fatalf("expected generated source to contain native for-in-list statement\n%s", generated)
	}
	if !strings.Contains(generated, "for charindex, __ardRune := range []rune(text)") {
		t.Fatalf("expected generated source to contain native for-in-str statement\n%s", generated)
	}
	if !strings.Contains(generated, "for _, key := range ardgo.MapKeys(") {
		t.Fatalf("expected generated source to contain native for-in-map iteration\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_ListAndMapLiteralsNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.AssignStmt{
							Target: "values",
							Value: &backendir.ListLiteralExpr{
								Type: &backendir.ListType{Elem: backendir.IntType},
								Elements: []backendir.Expr{
									&backendir.LiteralExpr{Kind: "int", Value: "1"},
									&backendir.LiteralExpr{Kind: "int", Value: "2"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "items",
							Value: &backendir.MapLiteralExpr{
								Type: &backendir.MapType{Key: backendir.StrType, Value: backendir.IntType},
								Entries: []backendir.MapEntry{
									{
										Key:   &backendir.LiteralExpr{Kind: "str", Value: "a"},
										Value: &backendir.LiteralExpr{Kind: "int", Value: "1"},
									},
								},
							},
						},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "values"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "items"}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "values := []int{1, 2}") {
		t.Fatalf("expected generated source to contain native list literal\n%s", generated)
	}
	if !strings.Contains(generated, "items := map[string]int{\"a\": 1}") {
		t.Fatalf("expected generated source to contain native map literal\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_StructAndEnumLiteralsNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.StructDecl{
				Name: "Box",
				Fields: []backendir.Field{
					{Name: "value", Type: backendir.IntType},
				},
			},
			&backendir.EnumDecl{
				Name: "Status",
				Values: []backendir.EnumValue{
					{Name: "active", Value: 0},
					{Name: "inactive", Value: 1},
				},
			},
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.AssignStmt{
							Target: "box",
							Value: &backendir.StructLiteralExpr{
								Type: &backendir.NamedType{Name: "Box"},
								Fields: []backendir.StructFieldValue{
									{Name: "value", Value: &backendir.LiteralExpr{Kind: "int", Value: "1"}},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "status",
							Value: &backendir.EnumVariantExpr{
								Type:         &backendir.NamedType{Name: "Status"},
								Discriminant: 0,
							},
						},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "box"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "status"}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "box := Box{Value: 1}") {
		t.Fatalf("expected generated source to contain native struct literal\n%s", generated)
	}
	if !strings.Contains(generated, "status := Status{Tag: 0}") {
		t.Fatalf("expected generated source to contain native enum variant literal\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_IfExprNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.AssignStmt{
							Target: "value",
							Value: &backendir.IfExpr{
								Cond: &backendir.LiteralExpr{Kind: "bool", Value: "true"},
								Then: &backendir.Block{Stmts: []backendir.Stmt{
									&backendir.ReturnStmt{Value: &backendir.LiteralExpr{Kind: "int", Value: "1"}},
								}},
								Else: &backendir.Block{Stmts: []backendir.Stmt{
									&backendir.ReturnStmt{Value: &backendir.LiteralExpr{Kind: "int", Value: "0"}},
								}},
								Type: backendir.IntType,
							},
						},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "value"}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "var value int") || !strings.Contains(generated, "if true") {
		t.Fatalf("expected generated source to contain native if-expression statement assignment\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_PanicExprNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.AssignStmt{
							Target: "value",
							Value: &backendir.PanicExpr{
								Message: &backendir.LiteralExpr{Kind: "str", Value: "boom"},
								Type:    backendir.IntType,
							},
						},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "value"}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "value := func() int") {
		t.Fatalf("expected generated source to contain typed panic expression closure\n%s", generated)
	}
	if !strings.Contains(generated, "panic(\"boom\")") {
		t.Fatalf("expected generated source to contain panic call\n%s", generated)
	}
	if strings.Contains(generated, "callExpr(\"panic_expr\"") || strings.Contains(generated, "panic_expr(") {
		t.Fatalf("expected generated source to avoid panic_expr markers\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_PanicExprNative_StrReturn(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name:   "describe",
				Return: backendir.StrType,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.ReturnStmt{Value: &backendir.PanicExpr{
							Message: &backendir.LiteralExpr{Kind: "str", Value: "no description"},
							Type:    backendir.StrType,
						}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "func() string") {
		t.Fatalf("expected typed panic closure to preserve string return type\n%s", generated)
	}
	if !strings.Contains(generated, "panic(\"no description\")") {
		t.Fatalf("expected panic call with quoted message\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_TryExprNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name: "divide",
				Params: []backendir.Param{
					{Name: "a", Type: backendir.IntType},
					{Name: "b", Type: backendir.IntType},
				},
				Return: &backendir.ResultType{Val: backendir.IntType, Err: backendir.StrType},
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.ReturnStmt{Value: &backendir.LiteralExpr{Kind: "int", Value: "1"}},
					},
				},
			},
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.AssignStmt{
							Target: "value",
							Value: &backendir.TryExpr{
								Kind: "result",
								Subject: &backendir.CallExpr{
									Callee: &backendir.IdentExpr{Name: "divide"},
									Args: []backendir.Expr{
										&backendir.LiteralExpr{Kind: "int", Value: "4"},
										&backendir.LiteralExpr{Kind: "int", Value: "2"},
									},
								},
								CatchVar: "err",
								Catch: &backendir.Block{
									Stmts: []backendir.Stmt{
										&backendir.ReturnStmt{Value: &backendir.LiteralExpr{Kind: "int", Value: "0"}},
									},
								},
								Type: backendir.IntType,
							},
						},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "value"}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if strings.Contains(generated, "value := func() int") {
		t.Fatalf("expected generated source to lower try expression as control-flow statements, got closure form\n%s", generated)
	}
	if !strings.Contains(generated, "__ardTryValue := Divide(4, 2)") {
		t.Fatalf("expected generated source to evaluate try subject once\n%s", generated)
	}
	if !strings.Contains(generated, "if __ardTryValue.IsErr()") {
		t.Fatalf("expected generated source to contain result error guard\n%s", generated)
	}
	if !strings.Contains(generated, "err := __ardTryValue.UnwrapErr()") {
		t.Fatalf("expected generated source to bind result err catch variable\n%s", generated)
	}
	if !strings.Contains(generated, "_ = err") {
		t.Fatalf("expected generated source to discard possibly-unused catch variable binding\n%s", generated)
	}
	if !strings.Contains(generated, "(&__ardTryValue).ExpectRef(\"unreachable err in try success path\")") {
		t.Fatalf("expected generated source to contain result success unwrap\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_TryExprWithoutCatchNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name: "unwrap",
				Params: []backendir.Param{
					{
						Name: "res",
						Type: &backendir.ResultType{Val: backendir.IntType, Err: backendir.StrType},
					},
				},
				Return: &backendir.ResultType{Val: backendir.IntType, Err: backendir.StrType},
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.ExprStmt{
							Value: &backendir.TryExpr{
								Kind:    "result",
								Subject: &backendir.IdentExpr{Name: "res"},
								Catch:   nil,
								Type:    backendir.IntType,
							},
						},
						&backendir.ReturnStmt{Value: &backendir.IdentExpr{Name: "res"}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "if __ardTryValue.IsErr()") {
		t.Fatalf("expected generated source to contain result error guard\n%s", generated)
	}
	if !strings.Contains(generated, "return ardgo.Err[int, string](__ardTryValue.UnwrapErr())") {
		t.Fatalf("expected generated source to contain default result failure propagation\n%s", generated)
	}
	if !strings.Contains(generated, "(&__ardTryValue).ExpectRef(\"unreachable err in try success path\")") {
		t.Fatalf("expected generated source to contain result success unwrap\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_TryExprWithoutCatchMaybeNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name: "unwrap",
				Params: []backendir.Param{
					{
						Name: "opt",
						Type: &backendir.MaybeType{Of: backendir.IntType},
					},
				},
				Return: &backendir.MaybeType{Of: backendir.IntType},
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.ExprStmt{
							Value: &backendir.TryExpr{
								Kind:    "maybe",
								Subject: &backendir.IdentExpr{Name: "opt"},
								Catch:   nil,
								Type:    backendir.IntType,
							},
						},
						&backendir.ReturnStmt{Value: &backendir.IdentExpr{Name: "opt"}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "if __ardTryValue.IsNone()") {
		t.Fatalf("expected generated source to contain maybe none guard\n%s", generated)
	}
	if !strings.Contains(generated, "return ardgo.None[int]()") {
		t.Fatalf("expected generated source to contain typed none propagation\n%s", generated)
	}
	if !strings.Contains(generated, "__ardTryValue.Expect(\"unreachable none in try success path\")") {
		t.Fatalf("expected generated source to contain maybe success unwrap\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_TryExprReturnStmtNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name: "unwrapOrZero",
				Params: []backendir.Param{
					{
						Name: "res",
						Type: &backendir.ResultType{Val: backendir.IntType, Err: backendir.StrType},
					},
				},
				Return: backendir.IntType,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.ReturnStmt{Value: &backendir.TryExpr{
							Kind:     "result",
							Subject:  &backendir.IdentExpr{Name: "res"},
							CatchVar: "err",
							Catch: &backendir.Block{
								Stmts: []backendir.Stmt{
									&backendir.ReturnStmt{Value: &backendir.LiteralExpr{Kind: "int", Value: "0"}},
								},
							},
							Type: backendir.IntType,
						}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if strings.Contains(generated, "func() int") && strings.Contains(generated, "return func()") {
		t.Fatalf("expected generated source to lower try expression as control-flow statements, got closure form\n%s", generated)
	}
	if !strings.Contains(generated, "__ardTryValue := res") {
		t.Fatalf("expected generated source to evaluate try subject once into a temp\n%s", generated)
	}
	if !strings.Contains(generated, "if __ardTryValue.IsErr()") {
		t.Fatalf("expected generated source to contain result error guard\n%s", generated)
	}
	if !strings.Contains(generated, "err := __ardTryValue.UnwrapErr()") {
		t.Fatalf("expected generated source to bind result err catch variable\n%s", generated)
	}
	if !strings.Contains(generated, "return (&__ardTryValue).ExpectRef(\"unreachable err in try success path\")") {
		t.Fatalf("expected generated source to return success path via Expect on temp\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_TryExprExprStmtNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name: "process",
				Params: []backendir.Param{
					{
						Name: "res",
						Type: &backendir.ResultType{Val: backendir.IntType, Err: backendir.StrType},
					},
				},
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.ExprStmt{Value: &backendir.TryExpr{
							Kind:     "result",
							Subject:  &backendir.IdentExpr{Name: "res"},
							CatchVar: "err",
							Catch: &backendir.Block{
								Stmts: []backendir.Stmt{
									&backendir.ReturnStmt{Value: nil},
								},
							},
							Type: backendir.IntType,
						}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if strings.Contains(generated, "func() int") {
		t.Fatalf("expected generated source to lower try expression as control-flow statements, got closure form\n%s", generated)
	}
	if !strings.Contains(generated, "__ardTryValue := res") {
		t.Fatalf("expected generated source to evaluate try subject once into a temp\n%s", generated)
	}
	if !strings.Contains(generated, "if __ardTryValue.IsErr()") {
		t.Fatalf("expected generated source to contain result error guard\n%s", generated)
	}
	if !strings.Contains(generated, "err := __ardTryValue.UnwrapErr()") {
		t.Fatalf("expected generated source to bind result err catch variable\n%s", generated)
	}
	if !strings.Contains(generated, "_ = err") {
		t.Fatalf("expected generated source to discard possibly-unused catch variable binding\n%s", generated)
	}
	if !strings.Contains(generated, "(&__ardTryValue).ExpectRef(\"unreachable err in try success path\")") {
		t.Fatalf("expected generated source to contain result success unwrap\n%s", generated)
	}
}

// TestEmitGoFileFromBackendIR_TryExprCatchInLoopReturnsEarly verifies that a
// `try ... -> err { ... }` placed inside a for-loop body emits the catch
// branch as an early `return` from the enclosing function. This guards
// against regressing back to the prior `_ = ardgo.Err(...)` shape that
// silently fell through to subsequent loop iterations and the trailing
// success path -- matching the VM's `OpReturn`-after-catch semantics.
func TestEmitGoFileFromBackendIR_TryExprCatchInLoopReturnsEarly(t *testing.T) {
	resultIntStr := &backendir.ResultType{Val: backendir.IntType, Err: backendir.StrType}
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name: "loop_catch",
				Params: []backendir.Param{
					{
						Name: "values",
						Type: &backendir.ListType{Elem: backendir.IntType},
					},
				},
				Return: resultIntStr,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.ForInListStmt{
							Cursor: "v",
							List:   &backendir.IdentExpr{Name: "values"},
							Body: &backendir.Block{
								Stmts: []backendir.Stmt{
									&backendir.AssignStmt{
										Target: "n",
										Value: &backendir.TryExpr{
											Kind:     "result",
											Subject:  &backendir.IdentExpr{Name: "v"},
											CatchVar: "err",
											Catch: &backendir.Block{
												Stmts: []backendir.Stmt{
													&backendir.ReturnStmt{
														Value: &backendir.IdentExpr{Name: "v"},
													},
												},
											},
											Type: backendir.IntType,
										},
									},
									&backendir.AssignStmt{
										Target: "_",
										Value:  &backendir.IdentExpr{Name: "n"},
									},
								},
							},
						},
						&backendir.ReturnStmt{Value: &backendir.IdentExpr{Name: "values"}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "for _, v := range values") {
		t.Fatalf("expected generated source to lower for-in-list loop natively\n%s", generated)
	}
	if !strings.Contains(generated, "__ardTryValue := v") {
		t.Fatalf("expected generated source to evaluate try subject once into a temp\n%s", generated)
	}
	if !strings.Contains(generated, "if __ardTryValue.IsErr()") {
		t.Fatalf("expected generated source to contain result error guard inside loop\n%s", generated)
	}
	if !strings.Contains(generated, "err := __ardTryValue.UnwrapErr()") {
		t.Fatalf("expected generated source to bind catch variable from try temp\n%s", generated)
	}
	// The catch branch must early-return from the enclosing function.
	if !strings.Contains(generated, "return v") {
		t.Fatalf("expected catch branch to early-return from enclosing function\n%s", generated)
	}
	// The success path inside the loop must continue normally with the
	// unwrapped value, not panic on the unreachable-err message.
	if !strings.Contains(generated, "n := (&__ardTryValue).ExpectRef(\"unreachable err in try success path\")") {
		t.Fatalf("expected loop body to bind unwrapped success value into n\n%s", generated)
	}
	// The closure form must not be used for control-flow try.
	if strings.Contains(generated, "n := func() int") {
		t.Fatalf("expected control-flow lowering, got closure form\n%s", generated)
	}
}

// TestEmitGoFileFromBackendIR_TryExprCatchInMatchArmReturnsEarly verifies
// that a `try ... -> err { ... }` whose catch branch is reached from within
// a stmt-position union match arm emits the catch as an early `return` from
// the enclosing function rather than falling through to a trailing success
// expression. This matches the VM's `OpReturn`-after-catch semantics for
// catch arms regardless of the surrounding match-arm wrapping.
func TestEmitGoFileFromBackendIR_TryExprCatchInMatchArmReturnsEarly(t *testing.T) {
	resultIntStr := &backendir.ResultType{Val: backendir.IntType, Err: backendir.StrType}
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name: "match_catch",
				Params: []backendir.Param{
					{
						Name: "value",
						Type: backendir.IntType,
					},
				},
				Return: resultIntStr,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.IfStmt{
							Cond: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "int_lt"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "value"},
									&backendir.LiteralExpr{Kind: "int", Value: "0"},
								},
							},
							Then: &backendir.Block{
								Stmts: []backendir.Stmt{
									&backendir.ExprStmt{
										Value: &backendir.TryExpr{
											Kind:     "result",
											Subject:  &backendir.IdentExpr{Name: "value"},
											CatchVar: "err",
											Catch: &backendir.Block{
												Stmts: []backendir.Stmt{
													&backendir.ReturnStmt{
														Value: &backendir.IdentExpr{Name: "value"},
													},
												},
											},
											Type: backendir.IntType,
										},
									},
								},
							},
						},
						&backendir.ReturnStmt{Value: &backendir.IdentExpr{Name: "value"}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "if value < 0") {
		t.Fatalf("expected generated source to emit the surrounding match-arm guard natively\n%s", generated)
	}
	if !strings.Contains(generated, "__ardTryValue := value") {
		t.Fatalf("expected generated source to evaluate try subject once into a temp\n%s", generated)
	}
	if !strings.Contains(generated, "if __ardTryValue.IsErr()") {
		t.Fatalf("expected generated source to contain result error guard inside match arm\n%s", generated)
	}
	if !strings.Contains(generated, "err := __ardTryValue.UnwrapErr()") {
		t.Fatalf("expected generated source to bind catch variable from try temp\n%s", generated)
	}
	// The catch branch must early-return from the enclosing function.
	returnCount := strings.Count(generated, "return value")
	if returnCount < 2 {
		t.Fatalf("expected catch branch + trailing success to both early-return; got %d 'return value' occurrences\n%s", returnCount, generated)
	}
	// Closure form must not be used for the control-flow try.
	if strings.Contains(generated, "func() int {") && strings.Contains(generated, "}()") {
		// allow other closures, but not for the try expression itself
		// (sanity check via try temp not appearing inside a closure form).
		if !strings.Contains(generated, "if __ardTryValue.IsErr()") {
			t.Fatalf("expected try control-flow lowering, got closure form\n%s", generated)
		}
	}
}

func TestEmitGoFileFromBackendIR_TryExprWithoutCatchMaybeReturnStmtNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name: "unwrap",
				Params: []backendir.Param{
					{
						Name: "opt",
						Type: &backendir.MaybeType{Of: backendir.IntType},
					},
				},
				Return: &backendir.MaybeType{Of: backendir.IntType},
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.ReturnStmt{Value: &backendir.TryExpr{
							Kind:    "maybe",
							Subject: &backendir.IdentExpr{Name: "opt"},
							Catch:   nil,
							Type:    backendir.IntType,
						}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if strings.Contains(generated, "func() Maybe[int]") {
		t.Fatalf("expected generated source to lower try expression as control-flow statements, got closure form\n%s", generated)
	}
	if !strings.Contains(generated, "__ardTryValue := opt") {
		t.Fatalf("expected generated source to evaluate try subject once into a temp\n%s", generated)
	}
	if !strings.Contains(generated, "if __ardTryValue.IsNone()") {
		t.Fatalf("expected generated source to contain maybe none guard\n%s", generated)
	}
	if !strings.Contains(generated, "return ardgo.None[int]()") {
		t.Fatalf("expected generated source to contain typed none propagation\n%s", generated)
	}
	if !strings.Contains(generated, "return __ardTryValue.Expect(\"unreachable none in try success path\")") {
		t.Fatalf("expected generated source to return success path via Expect on temp\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_UnionMatchExprNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.AssignStmt{Target: "value", Value: &backendir.LiteralExpr{Kind: "int", Value: "1"}},
						&backendir.AssignStmt{
							Target: "out",
							Value: &backendir.UnionMatchExpr{
								Subject: &backendir.IdentExpr{Name: "value"},
								Cases: []backendir.UnionMatchCase{
									{
										Type:    backendir.IntType,
										Pattern: "num",
										Body: &backendir.Block{
											Stmts: []backendir.Stmt{
												&backendir.ReturnStmt{Value: &backendir.IdentExpr{Name: "num"}},
											},
										},
									},
									{
										Type:    backendir.StrType,
										Pattern: "text",
										Body: &backendir.Block{
											Stmts: []backendir.Stmt{
												&backendir.ReturnStmt{Value: &backendir.LiteralExpr{Kind: "int", Value: "0"}},
											},
										},
									},
								},
								CatchAll: &backendir.Block{
									Stmts: []backendir.Stmt{
										&backendir.ReturnStmt{Value: &backendir.LiteralExpr{Kind: "int", Value: "0"}},
									},
								},
								Type: backendir.IntType,
							},
						},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "out"}},
					},
				},
			},
		},
	}

	out, err := emitGoFileFromBackendIRWithImports(module, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "var out int") {
		t.Fatalf("expected generated source to contain native union-match assignment target\n%s", generated)
	}
	if !strings.Contains(generated, "switch unionValue := any(value).(type)") {
		t.Fatalf("expected generated source to contain native union-match type switch\n%s", generated)
	}
	if !strings.Contains(generated, "case int:") || !strings.Contains(generated, "case string:") {
		t.Fatalf("expected generated source to contain native union-match type cases\n%s", generated)
	}
	if !strings.Contains(generated, "num := unionValue") {
		t.Fatalf("expected case-local pattern binding for int case\n%s", generated)
	}
	if !strings.Contains(generated, "text := unionValue") {
		t.Fatalf("expected case-local pattern binding for string case\n%s", generated)
	}
	if !strings.Contains(generated, "out = num") {
		t.Fatalf("expected case body to assign bound pattern value\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_ScalarMethodOpsNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.AssignStmt{
							Target: "contains",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "str_contains"},
								Args: []backendir.Expr{
									&backendir.LiteralExpr{Kind: "str", Value: "hello"},
									&backendir.LiteralExpr{Kind: "str", Value: "ell"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "intText",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "int_to_str"},
								Args: []backendir.Expr{
									&backendir.LiteralExpr{Kind: "int", Value: "42"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "floatInt",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "float_to_int"},
								Args: []backendir.Expr{
									&backendir.LiteralExpr{Kind: "float", Value: "3.5"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "dynStr",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "str_to_dyn"},
								Args: []backendir.Expr{
									&backendir.LiteralExpr{Kind: "str", Value: "hello"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "dynInt",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "int_to_dyn"},
								Args: []backendir.Expr{
									&backendir.LiteralExpr{Kind: "int", Value: "42"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "dynFloat",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "float_to_dyn"},
								Args: []backendir.Expr{
									&backendir.LiteralExpr{Kind: "float", Value: "3.5"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "dynBool",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "bool_to_dyn"},
								Args: []backendir.Expr{
									&backendir.LiteralExpr{Kind: "bool", Value: "true"},
								},
							},
						},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "contains"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "intText"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "floatInt"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "dynStr"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "dynInt"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "dynFloat"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "dynBool"}},
					},
				},
			},
		},
	}

	imports := map[string]string{
		helperImportPath: helperImportAlias,
		"strconv":        "strconv",
		"strings":        "strings",
	}
	out, err := emitGoFileFromBackendIRWithImports(module, imports, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "strings.Contains(\"hello\", \"ell\")") {
		t.Fatalf("expected generated source to contain native str_contains emission\n%s", generated)
	}
	if !strings.Contains(generated, "strconv.Itoa(42)") {
		t.Fatalf("expected generated source to contain native int_to_str emission\n%s", generated)
	}
	if !strings.Contains(generated, "value := float64(3.5)") {
		t.Fatalf("expected generated source to contain native float_to_int emission\n%s", generated)
	}
	if !strings.Contains(generated, "dynstr := \"hello\"") {
		t.Fatalf("expected generated source to contain native str_to_dyn emission\n%s", generated)
	}
	if !strings.Contains(generated, "dynint := 42") {
		t.Fatalf("expected generated source to contain native int_to_dyn emission\n%s", generated)
	}
	if !strings.Contains(generated, "dynfloat := 3.5") {
		t.Fatalf("expected generated source to contain native float_to_dyn emission\n%s", generated)
	}
	if !strings.Contains(generated, "dynbool := true") {
		t.Fatalf("expected generated source to contain native bool_to_dyn emission\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_ListMapReadOpsNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.AssignStmt{
							Target: "numbers",
							Value: &backendir.ListLiteralExpr{
								Type: &backendir.ListType{Elem: backendir.IntType},
								Elements: []backendir.Expr{
									&backendir.LiteralExpr{Kind: "int", Value: "1"},
									&backendir.LiteralExpr{Kind: "int", Value: "2"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "first",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "list_at"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "numbers"},
									&backendir.LiteralExpr{Kind: "int", Value: "0"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "count",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "list_size"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "numbers"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "setOk",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "list_set"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "numbers"},
									&backendir.LiteralExpr{Kind: "int", Value: "1"},
									&backendir.LiteralExpr{Kind: "int", Value: "9"},
								},
							},
						},
						&backendir.ExprStmt{
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "list_push"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "numbers"},
									&backendir.LiteralExpr{Kind: "int", Value: "3"},
								},
							},
						},
						&backendir.ExprStmt{
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "list_prepend"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "numbers"},
									&backendir.LiteralExpr{Kind: "int", Value: "0"},
								},
							},
						},
						&backendir.ExprStmt{
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "list_sort"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "numbers"},
									&backendir.IdentExpr{Name: "less"},
								},
							},
						},
						&backendir.ExprStmt{
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "list_swap"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "numbers"},
									&backendir.LiteralExpr{Kind: "int", Value: "0"},
									&backendir.LiteralExpr{Kind: "int", Value: "1"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "mapping",
							Value: &backendir.MapLiteralExpr{
								Type: &backendir.MapType{Key: backendir.StrType, Value: backendir.IntType},
								Entries: []backendir.MapEntry{
									{
										Key:   &backendir.LiteralExpr{Kind: "str", Value: "a"},
										Value: &backendir.LiteralExpr{Kind: "int", Value: "1"},
									},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "keys",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "map_keys"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "mapping"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "hasA",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "map_has"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "mapping"},
									&backendir.LiteralExpr{Kind: "str", Value: "a"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "mapCount",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "map_size"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "mapping"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "item",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "map_get"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "mapping"},
									&backendir.LiteralExpr{Kind: "str", Value: "a"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "mapSetOk",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "map_set"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "mapping"},
									&backendir.LiteralExpr{Kind: "str", Value: "b"},
									&backendir.LiteralExpr{Kind: "int", Value: "2"},
								},
							},
						},
						&backendir.ExprStmt{
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "map_drop"},
								Args: []backendir.Expr{
									&backendir.IdentExpr{Name: "mapping"},
									&backendir.LiteralExpr{Kind: "str", Value: "a"},
								},
							},
						},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "first"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "count"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "setOk"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "keys"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "hasA"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "mapCount"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "item"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "mapSetOk"}},
					},
				},
			},
		},
	}

	imports := map[string]string{
		helperImportPath: helperImportAlias,
		"sort":           "sort",
	}
	out, err := emitGoFileFromBackendIRWithImports(module, imports, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "numbers[0]") {
		t.Fatalf("expected generated source to contain native list_at emission\n%s", generated)
	}
	if !strings.Contains(generated, "len(numbers)") {
		t.Fatalf("expected generated source to contain native list_size emission\n%s", generated)
	}
	if !strings.Contains(generated, "ardgo.ListSet(numbers, 1, 9)") {
		t.Fatalf("expected generated source to contain native list_set emission\n%s", generated)
	}
	if !strings.Contains(generated, "numbers = append(numbers, 3)") {
		t.Fatalf("expected generated source to contain native list_push statement emission\n%s", generated)
	}
	if !strings.Contains(generated, "ardgo.ListPrepend(&numbers, 0)") {
		t.Fatalf("expected generated source to contain native list_prepend emission\n%s", generated)
	}
	if !strings.Contains(generated, "sort.SliceStable(numbers") {
		t.Fatalf("expected generated source to contain native list_sort emission\n%s", generated)
	}
	if !strings.Contains(generated, "ardgo.ListSwap(numbers, 0, 1)") {
		t.Fatalf("expected generated source to contain native list_swap emission\n%s", generated)
	}
	if !strings.Contains(generated, "ardgo.MapKeys(mapping)") {
		t.Fatalf("expected generated source to contain native map_keys emission\n%s", generated)
	}
	if !strings.Contains(generated, "_, ok := mapping[\"a\"]") {
		t.Fatalf("expected generated source to contain native map_has emission\n%s", generated)
	}
	if !strings.Contains(generated, "len(mapping)") {
		t.Fatalf("expected generated source to contain native map_size emission\n%s", generated)
	}
	if !strings.Contains(generated, "ardgo.MapGet(mapping, \"a\")") {
		t.Fatalf("expected generated source to contain native map_get emission\n%s", generated)
	}
	if !strings.Contains(generated, "ardgo.MapSet(mapping, \"b\", 2)") {
		t.Fatalf("expected generated source to contain native map_set emission\n%s", generated)
	}
	if !strings.Contains(generated, "ardgo.MapDrop(mapping, \"a\")") {
		t.Fatalf("expected generated source to contain native map_drop emission\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_ListCopyExprNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.AssignStmt{
							Target: "numbers",
							Value: &backendir.ListLiteralExpr{
								Type: &backendir.ListType{Elem: backendir.IntType},
								Elements: []backendir.Expr{
									&backendir.LiteralExpr{Kind: "int", Value: "1"},
									&backendir.LiteralExpr{Kind: "int", Value: "2"},
								},
							},
						},
						&backendir.AssignStmt{
							Target: "copied",
							Value: &backendir.CopyExpr{
								Value: &backendir.IdentExpr{Name: "numbers"},
								Type:  &backendir.ListType{Elem: backendir.IntType},
							},
						},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "copied"}},
					},
				},
			},
		},
	}

	imports := map[string]string{
		helperImportPath: helperImportAlias,
	}
	out, err := emitGoFileFromBackendIRWithImports(module, imports, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "append([]int(nil), numbers...)") {
		t.Fatalf("expected generated source to contain native list copy emission\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_FiberOpsNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name:   "work",
				Return: backendir.IntType,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.ReturnStmt{Value: &backendir.LiteralExpr{Kind: "int", Value: "1"}},
					},
				},
			},
			&backendir.FuncDecl{
				Name:   "main",
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.AssignStmt{
							Target: "started",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "fiber_start"},
								Args:   []backendir.Expr{&backendir.IdentExpr{Name: "work"}},
							},
						},
						&backendir.AssignStmt{
							Target: "evaluated",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "fiber_eval"},
								Args:   []backendir.Expr{&backendir.IdentExpr{Name: "work"}},
							},
						},
						&backendir.AssignStmt{
							Target: "executed",
							Value: &backendir.CallExpr{
								Callee: &backendir.IdentExpr{Name: "fiber_execution"},
								Args: []backendir.Expr{
									&backendir.LiteralExpr{Kind: "str", Value: "demo/worker"},
									&backendir.LiteralExpr{Kind: "str", Value: "run"},
								},
							},
						},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "started"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "evaluated"}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.IdentExpr{Name: "executed"}},
					},
				},
			},
		},
	}

	imports := map[string]string{
		helperImportPath:                        helperImportAlias,
		moduleImportPath("demo", "ard/async"):   packageNameForModulePath("ard/async"),
		moduleImportPath("demo", "demo/worker"): packageNameForModulePath("demo/worker"),
	}
	out, err := emitGoFileFromBackendIRWithImports(module, imports, true)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "async.Start(Work)") {
		t.Fatalf("expected generated source to contain native fiber_start emission\n%s", generated)
	}
	if !strings.Contains(generated, "async.Eval(Work)") {
		t.Fatalf("expected generated source to contain native fiber_eval emission\n%s", generated)
	}
	if !strings.Contains(generated, "async.Start(worker.Run)") {
		t.Fatalf("expected generated source to contain native fiber_execution emission\n%s", generated)
	}
}

func TestEmitGoFileFromBackendIR_FunctionLiteralAndCallableParamNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name:   "apply",
				Params: []backendir.Param{{Name: "of", Type: &backendir.FuncType{Params: []backendir.Type{backendir.IntType}, Return: backendir.IntType}}},
				Return: backendir.IntType,
				Body: &backendir.Block{Stmts: []backendir.Stmt{
					&backendir.ReturnStmt{Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "of"}, Args: []backendir.Expr{&backendir.LiteralExpr{Kind: "int", Value: "1"}}}},
				}},
			},
			&backendir.FuncDecl{
				Name:   "makeAdder",
				Return: &backendir.FuncType{Params: []backendir.Type{backendir.IntType}, Return: backendir.IntType},
				Body: &backendir.Block{Stmts: []backendir.Stmt{
					&backendir.ReturnStmt{Value: &backendir.FuncLiteralExpr{
						Params: []backendir.Param{{Name: "value", Type: backendir.IntType}},
						Return: backendir.IntType,
						Body: &backendir.Block{Stmts: []backendir.Stmt{
							&backendir.ReturnStmt{Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "int_add"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "value"}, &backendir.LiteralExpr{Kind: "int", Value: "1"}}}},
						}},
					}},
				}},
			},
		},
	}

	out, err := emitGoFileFromBackendIR(module, false)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	for _, want := range []string{"func Apply(of func(int) int) int", "return of(1)", "func(value int) int"} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected generated source to contain %q\n%s", want, generated)
		}
	}
}

func TestEmitGoFileFromBackendIR_ModuleStructLiteralNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Imports: map[string]string{
			moduleImportPath("demo", "ard/http"): packageNameForModulePath("ard/http"),
		},
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name:   "buildRequest",
				Return: &backendir.NamedType{Module: "ard/http", Name: "Request"},
				Body: &backendir.Block{Stmts: []backendir.Stmt{
					&backendir.ReturnStmt{Value: &backendir.StructLiteralExpr{
						Type: &backendir.NamedType{Module: "ard/http", Name: "Request"},
						Fields: []backendir.StructFieldValue{
							{Name: "method", Value: &backendir.EnumVariantExpr{Type: &backendir.NamedType{Module: "ard/http", Name: "Method"}, Discriminant: 1}},
							{Name: "url", Value: &backendir.LiteralExpr{Kind: "str", Value: "http://example.com"}},
						},
					}},
				}},
			},
		},
	}

	out, err := emitGoFileFromBackendIR(module, false)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	for _, want := range []string{"http.Request", "http.Method"} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected generated source to contain %q\n%s", want, generated)
		}
	}
}

func TestEmitGoFileFromBackendIR_MaybeResultConstructorsNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Imports:     map[string]string{helperImportPath: helperImportAlias},
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name:   "someInt",
				Return: &backendir.MaybeType{Of: backendir.IntType},
				Body: &backendir.Block{Stmts: []backendir.Stmt{
					&backendir.ReturnStmt{Value: &backendir.MaybeSomeExpr{
						Value: &backendir.LiteralExpr{Kind: "int", Value: "10"},
						Type:  &backendir.MaybeType{Of: backendir.IntType},
					}},
				}},
			},
			&backendir.FuncDecl{
				Name:   "noneInt",
				Return: &backendir.MaybeType{Of: backendir.IntType},
				Body: &backendir.Block{Stmts: []backendir.Stmt{
					&backendir.ReturnStmt{Value: &backendir.MaybeNoneExpr{Type: &backendir.MaybeType{Of: backendir.IntType}}},
				}},
			},
			&backendir.FuncDecl{
				Name:   "okInt",
				Return: &backendir.ResultType{Val: backendir.IntType, Err: backendir.StrType},
				Body: &backendir.Block{Stmts: []backendir.Stmt{
					&backendir.ReturnStmt{Value: &backendir.ResultOkExpr{
						Value: &backendir.LiteralExpr{Kind: "int", Value: "1"},
						Type:  &backendir.ResultType{Val: backendir.IntType, Err: backendir.StrType},
					}},
				}},
			},
			&backendir.FuncDecl{
				Name:   "errStr",
				Return: &backendir.ResultType{Val: backendir.IntType, Err: backendir.StrType},
				Body: &backendir.Block{Stmts: []backendir.Stmt{
					&backendir.ReturnStmt{Value: &backendir.ResultErrExpr{
						Value: &backendir.LiteralExpr{Kind: "str", Value: "bad"},
						Type:  &backendir.ResultType{Val: backendir.IntType, Err: backendir.StrType},
					}},
				}},
			},
		},
	}

	out, err := emitGoFileFromBackendIR(module, false)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	for _, want := range []string{
		"ardgo.Some[int](10)",
		"ardgo.None[int]()",
		"ardgo.Ok[int, string](1)",
		"ardgo.Err[int, string](\"bad\")",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected generated source to contain %q\n%s", want, generated)
		}
	}
}

func TestEmitGoFileFromBackendIR_MaybeResultMethodOpsNative(t *testing.T) {
	module := &backendir.Module{
		PackageName: "main",
		Decls: []backendir.Decl{
			&backendir.FuncDecl{
				Name: "useMaybeResultOps",
				Params: []backendir.Param{
					{Name: "maybeVal", Type: &backendir.MaybeType{Of: backendir.IntType}},
					{Name: "resultVal", Type: &backendir.ResultType{Val: backendir.IntType, Err: backendir.StrType}},
					{Name: "maybeMap", Type: &backendir.FuncType{Params: []backendir.Type{backendir.IntType}, Return: backendir.IntType}},
					{Name: "maybeBind", Type: &backendir.FuncType{Params: []backendir.Type{backendir.IntType}, Return: &backendir.MaybeType{Of: backendir.IntType}}},
					{Name: "resultMap", Type: &backendir.FuncType{Params: []backendir.Type{backendir.IntType}, Return: backendir.StrType}},
					{Name: "resultMapErr", Type: &backendir.FuncType{Params: []backendir.Type{backendir.StrType}, Return: backendir.IntType}},
					{Name: "resultBind", Type: &backendir.FuncType{Params: []backendir.Type{backendir.IntType}, Return: &backendir.ResultType{Val: backendir.StrType, Err: backendir.StrType}}},
				},
				Return: backendir.Void,
				Body: &backendir.Block{
					Stmts: []backendir.Stmt{
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "maybe_is_some"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "maybeVal"}}}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "maybe_is_none"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "maybeVal"}}}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "maybe_expect"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "maybeVal"}, &backendir.LiteralExpr{Kind: "str", Value: "missing"}}}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "maybe_or"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "maybeVal"}, &backendir.LiteralExpr{Kind: "int", Value: "7"}}}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "maybe_map"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "maybeVal"}, &backendir.IdentExpr{Name: "maybeMap"}}}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "maybe_and_then"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "maybeVal"}, &backendir.IdentExpr{Name: "maybeBind"}}}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "result_is_ok"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "resultVal"}}}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "result_is_err"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "resultVal"}}}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "result_expect"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "resultVal"}, &backendir.LiteralExpr{Kind: "str", Value: "bad"}}}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "result_or"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "resultVal"}, &backendir.LiteralExpr{Kind: "int", Value: "0"}}}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "result_map"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "resultVal"}, &backendir.IdentExpr{Name: "resultMap"}}}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "result_map_err"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "resultVal"}, &backendir.IdentExpr{Name: "resultMapErr"}}}},
						&backendir.AssignStmt{Target: "_", Value: &backendir.CallExpr{Callee: &backendir.IdentExpr{Name: "result_and_then"}, Args: []backendir.Expr{&backendir.IdentExpr{Name: "resultVal"}, &backendir.IdentExpr{Name: "resultBind"}}}},
					},
				},
			},
		},
	}

	imports := map[string]string{
		helperImportPath: helperImportAlias,
	}
	out, err := emitGoFileFromBackendIRWithImports(module, imports, false)
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "maybeval.IsSome()") {
		t.Fatalf("expected generated source to contain native maybe_is_some emission\n%s", generated)
	}
	if !strings.Contains(generated, "maybeval.Expect(\"missing\")") {
		t.Fatalf("expected generated source to contain native maybe_expect emission\n%s", generated)
	}
	if !strings.Contains(generated, "ardgo.MaybeMap(maybeval, maybemap)") {
		t.Fatalf("expected generated source to contain native maybe_map emission\n%s", generated)
	}
	if !strings.Contains(generated, "ardgo.MaybeAndThen(maybeval, maybebind)") {
		t.Fatalf("expected generated source to contain native maybe_and_then emission\n%s", generated)
	}
	if !strings.Contains(generated, "resultval.IsOk()") {
		t.Fatalf("expected generated source to contain native result_is_ok emission\n%s", generated)
	}
	if !strings.Contains(generated, "(&resultval).ExpectRef(\"bad\")") {
		t.Fatalf("expected generated source to contain native result_expect emission\n%s", generated)
	}
	if !strings.Contains(generated, "ardgo.ResultMap(resultval, resultmap)") {
		t.Fatalf("expected generated source to contain native result_map emission\n%s", generated)
	}
	if !strings.Contains(generated, "ardgo.ResultMapErr(resultval, resultmaperr)") {
		t.Fatalf("expected generated source to contain native result_map_err emission\n%s", generated)
	}
	if !strings.Contains(generated, "ardgo.ResultAndThen(resultval, resultbind)") {
		t.Fatalf("expected generated source to contain native result_and_then emission\n%s", generated)
	}
}

func TestCanEmitAssignTargetNatively(t *testing.T) {
	cases := []struct {
		target string
		want   bool
	}{
		{target: "_", want: true},
		{target: "value", want: true},
		{target: "self.value", want: true},
		{target: " self.value ", want: true},
		{target: "self.value.more", want: false},
		{target: "", want: false},
		{target: "<target:*checker.InstanceProperty>", want: false},
		{target: "1value", want: false},
	}
	for _, tc := range cases {
		if got := canEmitAssignTargetNatively(tc.target); got != tc.want {
			t.Fatalf("canEmitAssignTargetNatively(%q) = %v, want %v", tc.target, got, tc.want)
		}
	}
}

func TestCanEmitExprNatively_SelectorCalls(t *testing.T) {
	emitter := &backendIREmitter{}

	safe := &backendir.CallExpr{
		Callee: &backendir.SelectorExpr{
			Subject: &backendir.IdentExpr{Name: "self"},
			Name:    "get",
		},
		Args: []backendir.Expr{
			&backendir.LiteralExpr{Kind: "int", Value: "1"},
		},
	}
	if !emitter.canEmitExprNatively(safe) {
		t.Fatalf("expected selector call on local subject to be natively emittable")
	}

	moduleCall := &backendir.CallExpr{
		Callee: &backendir.SelectorExpr{
			Subject: &backendir.IdentExpr{Name: "ard/io"},
			Name:    "print",
		},
		Args: []backendir.Expr{
			&backendir.LiteralExpr{Kind: "str", Value: "ok"},
		},
	}
	if !emitter.canEmitExprNatively(moduleCall) {
		t.Fatalf("expected module selector call to be natively emittable")
	}
}

func TestCanEmitExprNatively_ListAndMapLiteralsWithDynamicTypesSupported(t *testing.T) {
	emitter := &backendIREmitter{}

	listDynamic := &backendir.ListLiteralExpr{
		Type: &backendir.ListType{Elem: backendir.Dynamic},
		Elements: []backendir.Expr{
			&backendir.LiteralExpr{Kind: "int", Value: "1"},
		},
	}
	if !emitter.canEmitExprNatively(listDynamic) {
		t.Fatalf("expected list literal with dynamic element type to be supported for native emission")
	}

	mapDynamic := &backendir.MapLiteralExpr{
		Type: &backendir.MapType{Key: backendir.StrType, Value: backendir.Dynamic},
		Entries: []backendir.MapEntry{
			{Key: &backendir.LiteralExpr{Kind: "str", Value: "a"}, Value: &backendir.LiteralExpr{Kind: "int", Value: "1"}},
		},
	}
	if !emitter.canEmitExprNatively(mapDynamic) {
		t.Fatalf("expected map literal with dynamic value type to be supported for native emission")
	}

	listTypeVar := &backendir.ListLiteralExpr{
		Type: &backendir.ListType{Elem: &backendir.TypeVarType{Name: "T"}},
		Elements: []backendir.Expr{
			&backendir.LiteralExpr{Kind: "int", Value: "1"},
		},
	}
	if !emitter.canEmitExprNatively(listTypeVar) {
		t.Fatalf("expected list literal with type-var element type to be supported for native emission")
	}

	ifExprNoElse := &backendir.IfExpr{
		Cond: &backendir.LiteralExpr{Kind: "bool", Value: "true"},
		Then: &backendir.Block{Stmts: []backendir.Stmt{
			&backendir.ReturnStmt{Value: &backendir.LiteralExpr{Kind: "int", Value: "1"}},
		}},
		Type: backendir.IntType,
	}
	if emitter.canEmitExprNatively(ifExprNoElse) {
		t.Fatalf("expected if expression without else for non-void type to remain unsupported for native emission")
	}
}

func assertParsesAsGo(t *testing.T, source []byte) {
	t.Helper()
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "generated.go", source, parser.AllErrors); err != nil {
		t.Fatalf("generated source is not valid Go: %v\n%s", err, string(source))
	}
}

// assertSyntheticMatchTempIsArdUnreachable verifies the synthetic match-subject
// temp prefix is built from a leading character that Ard's lexer cannot accept
// (anything outside ASCII `[A-Za-z_]`). This is the structural guarantee that
// hoist temporary names cannot collide with any legal user-defined Ard
// identifier — it is the contract proven by this regression suite.
func assertSyntheticMatchTempIsArdUnreachable(t *testing.T, name string) {
	t.Helper()
	if name == "" {
		t.Fatalf("synthetic match temp name is empty")
	}
	first := []rune(name)[0]
	if first < 0x80 {
		t.Fatalf("synthetic match temp %q must start with a non-ASCII rune so it cannot be lexed by Ard's identifier rules", name)
	}
	// Belt-and-suspenders: even if some non-ASCII letter were ever in Ard's
	// identifier alphabet, the leading rune must not be an ASCII letter or
	// underscore.
	if (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_' {
		t.Fatalf("synthetic match temp %q must not start with an Ard-legal identifier character", name)
	}
}

// occurrencesOfWholeWord counts occurrences of `word` in `source` only when the
// match is preceded and followed by a non-identifier rune (Go identifier
// characters: letters, digits, underscore). This avoids spurious overlaps
// where a user-named local like `ardmatchsubjectInt` would otherwise match
// inside a synthetic temp like `αardmatchsubjectInt`.
func occurrencesOfWholeWord(source, word string) int {
	if word == "" {
		return 0
	}
	count := 0
	idx := 0
	for {
		hit := strings.Index(source[idx:], word)
		if hit < 0 {
			return count
		}
		start := idx + hit
		end := start + len(word)
		var prev, next rune
		if start > 0 {
			prev, _ = utf8.DecodeLastRuneInString(source[:start])
		}
		if end < len(source) {
			next, _ = utf8.DecodeRuneInString(source[end:])
		}
		if !isGoIdentRune(prev) && !isGoIdentRune(next) {
			count++
		}
		idx = end
	}
}

func isGoIdentRune(r rune) bool {
	if r == 0 {
		return false
	}
	if r == '_' || (r >= '0' && r <= '9') {
		return true
	}
	return unicode.IsLetter(r)
}

// assertUserLocalNotMutatedByMatchTemp verifies that a user-defined local
// (matched by goLocal) is defined exactly once via `:=` and never reassigned
// by a stray `=`. The whole-word matcher excludes incidental substring hits
// inside the synthetic temp's name (which deliberately uses a leading non-ASCII
// rune that contains the user-name as a suffix when string-searched).
func assertUserLocalNotMutatedByMatchTemp(t *testing.T, generated, goLocal string) {
	t.Helper()
	defineLine := goLocal + " :="
	defines := occurrencesOfWholeWord(generated, defineLine)
	if defines != 1 {
		t.Fatalf("expected user local %q to be defined exactly once via `:=`, got %d whole-word occurrences\n%s", goLocal, defines, generated)
	}
	reassignLine := goLocal + " ="
	reassigns := occurrencesOfWholeWord(generated, reassignLine)
	if reassigns != 0 {
		t.Fatalf("expected user local %q never to be reassigned (no `=` mutation), got %d whole-word occurrences\n%s", goLocal, reassigns, generated)
	}
}

func TestEmitGoFileFromBackendIR_IntMatchUnsafeSubjectTempDoesNotCollideWithUserLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	// The user's local deliberately mirrors the prior synthetic match-subject
	// temp name (`__ardMatchSubject_int`) to prove that the hoist step cannot
	// reuse / shadow / mutate it.
	module := checkedModuleFromSource(t, dir, "main.ard", `
fn next() Int { 1 }

fn run() Int {
  let __ardMatchSubject_int = 99
  let outcome = match next() {
    1 => 100,
    _ => 200,
  }
  __ardMatchSubject_int + outcome
}

fn main() {
  let _ = run()
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)
	generated := string(out)

	syntheticTemp := matchSubjectTempName("int")
	assertSyntheticMatchTempIsArdUnreachable(t, syntheticTemp)

	userGoLocal := goName("__ardMatchSubject_int", false)
	if userGoLocal == "" {
		t.Fatalf("expected user local Go name to be non-empty")
	}
	if !strings.Contains(generated, userGoLocal+" := 99") {
		t.Fatalf("expected user local %q to be defined with literal value 99\n%s", userGoLocal, generated)
	}
	assertUserLocalNotMutatedByMatchTemp(t, generated, userGoLocal)

	syntheticGoTemp := goName(syntheticTemp, false)
	if syntheticGoTemp == userGoLocal {
		t.Fatalf("synthetic match temp Go name %q must not equal user local Go name %q", syntheticGoTemp, userGoLocal)
	}
	if !strings.Contains(generated, syntheticGoTemp) {
		t.Fatalf("expected generated source to reference synthetic match temp %q (Go-mapped from %q) — proves native single-eval emission\n%s", syntheticGoTemp, syntheticTemp, generated)
	}
}

// TestEmitGoFileFromBackendIR_IntMatchUnsafeSubjectSingleEvaluationNative
// verifies that an int match expression over a non-trivial subject (a
// function call with side effects) binds the subject to a synthetic temp
// and evaluates it exactly once. This is the structural guarantee that
// single-evaluation semantics are preserved in emitted Go.
func TestEmitGoFileFromBackendIR_IntMatchUnsafeSubjectSingleEvaluationNative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn next() Int { 1 }

fn run() Int {
  match next() {
    1 => 100,
    _ => 200,
  }
}

fn main() {
  let _ = run()
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)
	generated := string(out)

	syntheticTemp := matchSubjectTempName("int")
	assertSyntheticMatchTempIsArdUnreachable(t, syntheticTemp)
	syntheticGoTemp := goName(syntheticTemp, false)
	if !strings.Contains(generated, syntheticGoTemp) {
		t.Fatalf("expected generated source to reference synthetic match temp %q (Go-mapped from %q) — proves native single-eval emission\n%s", syntheticGoTemp, syntheticTemp, generated)
	}
	if !strings.Contains(generated, syntheticGoTemp) || !strings.Contains(generated, " := Next()") {
		t.Fatalf("expected synthetic match temp to be assigned exactly once from subject call Next()\n%s", generated)
	}
	// Subject call Next() should appear exactly twice in the source: once
	// in the `func Next() int` declaration and once at the hoist call site
	// `αardmatchsubjectInt := Next()`. Any extra occurrence indicates the
	// subject is being re-evaluated and single-evaluation is broken.
	if got := occurrencesOfWholeWord(generated, "Next()"); got != 2 {
		t.Fatalf("expected subject call Next() to be evaluated exactly once (1 decl + 1 hoist call), got %d total occurrences\n%s", got, generated)
	}

}

func TestEmitGoFileFromBackendIR_OptionMatchUnsafeSubjectTempDoesNotCollideWithUserLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/maybe

fn next() Int? { maybe::some(1) }

fn run() Int {
  let __ardMatchSubject_option = 99
  let outcome = match next() {
    n => n,
    _ => 200,
  }
  __ardMatchSubject_option + outcome
}

fn main() {
  let _ = run()
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)
	generated := string(out)

	syntheticTemp := matchSubjectTempName("option")
	assertSyntheticMatchTempIsArdUnreachable(t, syntheticTemp)

	userGoLocal := goName("__ardMatchSubject_option", false)
	if !strings.Contains(generated, userGoLocal+" := 99") {
		t.Fatalf("expected user local %q to be defined with literal value 99\n%s", userGoLocal, generated)
	}
	assertUserLocalNotMutatedByMatchTemp(t, generated, userGoLocal)

	syntheticGoTemp := goName(syntheticTemp, false)
	if syntheticGoTemp == userGoLocal {
		t.Fatalf("synthetic match temp Go name %q must not equal user local Go name %q", syntheticGoTemp, userGoLocal)
	}
	if !strings.Contains(generated, syntheticGoTemp) {
		t.Fatalf("expected generated source to reference synthetic match temp %q (Go-mapped from %q)\n%s", syntheticGoTemp, syntheticTemp, generated)
	}
}

func TestEmitGoFileFromBackendIR_ResultMatchUnsafeSubjectTempDoesNotCollideWithUserLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/result as Result

fn next() Int!Str { Result::ok(1) }

fn run() Int {
  let __ardMatchSubject_result = 99
  let outcome = match next() {
    ok(value) => value,
    err(_) => 200,
  }
  __ardMatchSubject_result + outcome
}

fn main() {
  let _ = run()
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)
	generated := string(out)

	syntheticTemp := matchSubjectTempName("result")
	assertSyntheticMatchTempIsArdUnreachable(t, syntheticTemp)

	userGoLocal := goName("__ardMatchSubject_result", false)
	if !strings.Contains(generated, userGoLocal+" := 99") {
		t.Fatalf("expected user local %q to be defined with literal value 99\n%s", userGoLocal, generated)
	}
	assertUserLocalNotMutatedByMatchTemp(t, generated, userGoLocal)

	syntheticGoTemp := goName(syntheticTemp, false)
	if syntheticGoTemp == userGoLocal {
		t.Fatalf("synthetic match temp Go name %q must not equal user local Go name %q", syntheticGoTemp, userGoLocal)
	}
	if !strings.Contains(generated, syntheticGoTemp) {
		t.Fatalf("expected generated source to reference synthetic match temp %q (Go-mapped from %q)\n%s", syntheticGoTemp, syntheticTemp, generated)
	}
}

func TestEmitGoFileFromBackendIR_EnumMatchUnsafeSubjectTempDoesNotCollideWithUserLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
enum Light { red, yellow, green }

fn next() Light { Light::green }

fn run() Int {
  let __ardMatchSubject_enum = 99
  let outcome = match next() {
    Light::red => 1,
    Light::yellow => 2,
    Light::green => 3,
  }
  __ardMatchSubject_enum + outcome
}

fn main() {
  let _ = run()
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)
	generated := string(out)

	syntheticTemp := matchSubjectTempName("enum")
	assertSyntheticMatchTempIsArdUnreachable(t, syntheticTemp)

	userGoLocal := goName("__ardMatchSubject_enum", false)
	if !strings.Contains(generated, userGoLocal+" := 99") {
		t.Fatalf("expected user local %q to be defined with literal value 99\n%s", userGoLocal, generated)
	}
	assertUserLocalNotMutatedByMatchTemp(t, generated, userGoLocal)

	syntheticGoTemp := goName(syntheticTemp, false)
	if syntheticGoTemp == userGoLocal {
		t.Fatalf("synthetic match temp Go name %q must not equal user local Go name %q", syntheticGoTemp, userGoLocal)
	}
	if !strings.Contains(generated, syntheticGoTemp) {
		t.Fatalf("expected generated source to reference synthetic match temp %q (Go-mapped from %q) — proves native single-eval emission\n%s", syntheticGoTemp, syntheticTemp, generated)
	}
}

// TestEmitGoFileFromBackendIR_EnumMatchUnsafeSubjectSingleEvaluationNative
// verifies the same single-evaluation emission contract for an enum
// match over a non-trivial subject. The synthetic match-subject temp
// must be present in the emitted Go and the unsafe subject call must be
// evaluated exactly once.
func TestEmitGoFileFromBackendIR_EnumMatchUnsafeSubjectSingleEvaluationNative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
enum Light { red, yellow, green }

fn next() Light { Light::green }

fn run() Int {
  match next() {
    Light::red => 1,
    Light::yellow => 2,
    Light::green => 3,
  }
}

fn main() {
  let _ = run()
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)
	generated := string(out)

	syntheticTemp := matchSubjectTempName("enum")
	assertSyntheticMatchTempIsArdUnreachable(t, syntheticTemp)
	syntheticGoTemp := goName(syntheticTemp, false)
	if !strings.Contains(generated, syntheticGoTemp) {
		t.Fatalf("expected generated source to reference synthetic match temp %q (Go-mapped from %q) — proves native single-eval emission\n%s", syntheticGoTemp, syntheticTemp, generated)
	}
	if !strings.Contains(generated, syntheticGoTemp) || !strings.Contains(generated, " := Next()") {
		t.Fatalf("expected synthetic match temp to be assigned exactly once from subject call Next()\n%s", generated)
	}
	// Subject call Next() should appear exactly twice in the source: once
	// in the `func Next() ... { ... }` declaration and once at the hoist
	// call site `αardmatchsubjectEnum := Next()`. Any extra occurrence
	// indicates the subject is being re-evaluated and single-evaluation
	// is broken.
	if got := occurrencesOfWholeWord(generated, "Next()"); got != 2 {
		t.Fatalf("expected subject call Next() to be evaluated exactly once (1 decl + 1 hoist call), got %d total occurrences\n%s", got, generated)
	}

}

// TestCompileModuleSourceViaBackendIR_MethodDeclarationTraitSignatureNative verifies
// that trait-typed method signatures remain concrete under native backend
// IR emission instead of erasing trait params to `any`.
func TestCompileModuleSourceViaBackendIR_MethodDeclarationTraitSignatureNative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/string as Str

struct Logger {
  prefix: Str,
}

impl Logger {
  fn show(item: Str::ToString) Str {
    self.prefix
  }
}

fn main() {
  let logger = Logger{prefix: "log:"}
  let _ = logger.show("hi")
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)

	generated := string(out)
	if !strings.Contains(generated, "func (self Logger) Show(item ardgo.ToString) string") {
		t.Fatalf("expected trait-typed method to keep ardgo.ToString in native emission\n%s", generated)
	}
	if strings.Contains(generated, "Show(item any)") {
		t.Fatalf("expected trait-typed method to NOT erase its trait param to `any`\n%s", generated)
	}
}

// TestEmitGoFileFromBackendIR_MethodDeclDeduplication verifies that the
// backend IR emitter deduplicates struct/enum method declarations using
// the per-emitter owner+method key, so methods are written exactly once
// even when the same owner declaration is encountered more than once
// during emission.
func TestEmitGoFileFromBackendIR_MethodsWithoutSourceModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Box {
  value: Int,
}

impl Box {
  fn get() Int {
    self.value
  }
}

enum Light { red, green }

impl Light {
  fn rank() Int {
    match self {
      Light::red => 0,
      Light::green => 1,
    }
  }
}

fn main() {
  let box = Box{value: 1}
  let _ = box.get()
  let light = Light::green
  let _ = light.rank()
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("backend IR lowering failed: %v", err)
	}

	fileIR, err := emitGoFileFromBackendIRWithImports(irModule, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("backend IR emitter failed without source module: %v", err)
	}
	rendered, err := renderGoFile(fileIR)
	if err != nil {
		t.Fatalf("renderGoFile failed: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	for _, want := range []string{
		"func (self Box) Get() int",
		"func (self Light) Rank() int",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected generated source to contain %q\n%s", want, generated)
		}
	}
}

func TestEmitGoFileFromBackendIR_MethodDeclDeduplication(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Box {
  value: Int,
}

impl Box {
  fn get() Int {
    self.value
  }
}

enum Light { red, green }

impl Light {
  fn rank() Int {
    match self {
      Light::red => 0,
      Light::green => 1,
    }
  }
}

fn main() {
  let box = Box{value: 1}
  let _ = box.get()
  let light = Light::green
  let _ = light.rank()
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("backend IR lowering failed: %v", err)
	}

	// Inject duplicate StructDecl/EnumDecl entries for Box/Light to
	// simulate scenarios where the same owner declaration could be
	// surfaced more than once during emission. The dedupe key in the
	// backend IR emitter must keep method emission to a single copy
	// per owner+method pair.
	var boxDecl, lightDecl backendir.Decl
	for _, decl := range irModule.Decls {
		switch typed := decl.(type) {
		case *backendir.StructDecl:
			if typed.Name == "Box" {
				boxDecl = decl
			}
		case *backendir.EnumDecl:
			if typed.Name == "Light" {
				lightDecl = decl
			}
		}
	}
	if boxDecl == nil {
		t.Fatalf("expected Box StructDecl in lowered IR module")
	}
	if lightDecl == nil {
		t.Fatalf("expected Light EnumDecl in lowered IR module")
	}
	irModule.Decls = append(irModule.Decls, boxDecl, lightDecl)

	fileIR, err := emitGoFileFromBackendIRWithImports(irModule, map[string]string{helperImportPath: helperImportAlias}, true)
	if err != nil {
		t.Fatalf("backend IR emitter failed: %v", err)
	}
	rendered, err := renderGoFile(fileIR)
	if err != nil {
		t.Fatalf("renderGoFile failed: %v", err)
	}
	generated := string(rendered)

	if got := strings.Count(generated, "func (self Box) Get()"); got != 1 {
		t.Fatalf("expected struct method Box.Get to be emitted exactly once, got %d\n%s", got, generated)
	}
	if got := strings.Count(generated, "func (self Light) Rank()"); got != 1 {
		t.Fatalf("expected enum method Light.Rank to be emitted exactly once, got %d\n%s", got, generated)
	}
}

// TestCompileModuleSourceViaBackendIR_UnionTypedFunctionSignatureNative
// verifies that native backend IR emission preserves concrete union
// interface names (e.g. `Shape`) in free-function signatures instead of
// erasing parameters or returns to `any`.
func TestCompileModuleSourceViaBackendIR_UnionTypedFunctionSignatureNative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Square { size: Int }
struct Circle { radius: Int }

type Shape = Square | Circle

fn pick(shape: Shape) Shape {
  shape
}

fn main() {
  let s = Square{size: 1}
  let _ = pick(s)
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)

	generated := string(out)
	if !strings.Contains(generated, "type Shape interface") {
		t.Fatalf("expected union interface declaration `type Shape interface` in generated Go\n%s", generated)
	}
	if !strings.Contains(generated, "func Pick(shape Shape) Shape") {
		t.Fatalf("expected native union-typed function signature `func Pick(shape Shape) Shape`\n%s", generated)
	}
	for _, unwanted := range []string{
		"Pick(shape any)",
		"shape any) any",
	} {
		if strings.Contains(generated, unwanted) {
			t.Fatalf("expected native union-typed function signature to avoid erased form %q\n%s", unwanted, generated)
		}
	}
}

// TestCompileModuleSourceViaBackendIR_UnionTypedMethodSignatureNative
// verifies that native backend IR method emission preserves concrete
// union interface names in both parameter and return positions.
func TestCompileModuleSourceViaBackendIR_UnionTypedMethodSignatureNative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Square { size: Int }
struct Circle { radius: Int }

type Shape = Square | Circle

struct Picker {}

impl Picker {
  fn choose(shape: Shape) Shape {
    shape
  }
}

fn main() {
  let p = Picker{}
  let s = Square{size: 1}
  let _ = p.choose(s)
}
`)

	out, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}
	assertParsesAsGo(t, out)

	generated := string(out)
	if !strings.Contains(generated, "type Shape interface") {
		t.Fatalf("expected union interface declaration `type Shape interface` in generated Go\n%s", generated)
	}
	if !strings.Contains(generated, "func (self Picker) Choose(shape Shape) Shape") {
		t.Fatalf("expected native union-typed method signature `func (self Picker) Choose(shape Shape) Shape`\n%s", generated)
	}
	for _, unwanted := range []string{
		"Choose(shape any)",
		"shape any) any",
	} {
		if strings.Contains(generated, unwanted) {
			t.Fatalf("expected native union-typed method signature to avoid erased form %q\n%s", unwanted, generated)
		}
	}
}
