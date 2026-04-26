package go_backend

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	legacy, err := compileModuleSource(module, "main", true, "")
	if err != nil {
		t.Fatalf("legacy compile failed: %v", err)
	}
	irOut, err := compileModuleSourceViaBackendIR(module, "main", true, "")
	if err != nil {
		t.Fatalf("backend IR compile failed: %v", err)
	}

	assertParsesAsGo(t, legacy)
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

func TestCompileModuleSourceViaBackendIR_ExternTraitParamFallsBackToLegacy(t *testing.T) {
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
		t.Fatalf("expected trait-typed extern signature to use legacy fallback lowering\n%s", generated)
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
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

func TestCompileModuleSourceViaBackendIR_FunctionCallBodyFallsBackToLegacy(t *testing.T) {
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
		t.Fatalf("expected legacy fallback trait wrapping for io::print call\n%s", generated)
	}
}

func TestCompileModuleSourceViaBackendIR_EntrypointFallsBackToLegacyForModuleCalls(t *testing.T) {
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
		t.Fatalf("expected entrypoint fallback trait wrapping for io::print call\n%s", generated)
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
	if !strings.Contains(generated, "self.Value = value") {
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
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
		t.Fatalf("expected generated source to contain native if-expression closure\n%s", generated)
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
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
		t.Fatalf("expected generated source to avoid panic_expr marker fallback\n%s", generated)
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
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
	if !strings.Contains(generated, "__ardTryValue.Expect(\"unreachable err in try success path\")") {
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
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
	if !strings.Contains(generated, "__ardTryValue.Expect(\"unreachable err in try success path\")") {
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
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

	out, err := emitGoFileFromBackendIR(module, nil, map[string]string{helperImportPath: helperImportAlias}, true, "")
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "out := func() int") {
		t.Fatalf("expected generated source to contain native union-match closure\n%s", generated)
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
	if !strings.Contains(generated, "return num") {
		t.Fatalf("expected case body to return bound pattern value\n%s", generated)
	}
	if strings.Contains(generated, "union_match(") {
		t.Fatalf("expected union-match emission to avoid union_match marker fallback\n%s", generated)
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
	out, err := emitGoFileFromBackendIR(module, nil, imports, true, "")
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
	out, err := emitGoFileFromBackendIR(module, nil, imports, true, "")
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
	if !strings.Contains(generated, "ardgo.ListPush(&numbers, 3)") {
		t.Fatalf("expected generated source to contain native list_push emission\n%s", generated)
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
	out, err := emitGoFileFromBackendIR(module, nil, imports, true, "")
	if err != nil {
		t.Fatalf("expected backend IR emitter to succeed, got error: %v", err)
	}
	rendered, err := renderGoFile(out)
	if err != nil {
		t.Fatalf("expected backend IR rendering to succeed, got error: %v", err)
	}
	assertParsesAsGo(t, rendered)

	generated := string(rendered)
	if !strings.Contains(generated, "append(numbers[0:0:0], numbers...)") {
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
	out, err := emitGoFileFromBackendIR(module, nil, imports, true, "demo")
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
	out, err := emitGoFileFromBackendIR(module, nil, imports, false, "")
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
	if !strings.Contains(generated, "resultval.Expect(\"bad\")") {
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
	if emitter.canEmitExprNatively(moduleCall) {
		t.Fatalf("expected module selector call to stay on fallback path")
	}
}

func TestCanEmitExprNatively_ListAndMapLiteralsWithDynamicTypesFallback(t *testing.T) {
	emitter := &backendIREmitter{}

	listDynamic := &backendir.ListLiteralExpr{
		Type: &backendir.ListType{Elem: backendir.Dynamic},
		Elements: []backendir.Expr{
			&backendir.LiteralExpr{Kind: "int", Value: "1"},
		},
	}
	if emitter.canEmitExprNatively(listDynamic) {
		t.Fatalf("expected list literal with dynamic element type to stay on fallback path")
	}

	mapDynamic := &backendir.MapLiteralExpr{
		Type: &backendir.MapType{Key: backendir.StrType, Value: backendir.Dynamic},
		Entries: []backendir.MapEntry{
			{Key: &backendir.LiteralExpr{Kind: "str", Value: "a"}, Value: &backendir.LiteralExpr{Kind: "int", Value: "1"}},
		},
	}
	if emitter.canEmitExprNatively(mapDynamic) {
		t.Fatalf("expected map literal with dynamic value type to stay on fallback path")
	}

	listTypeVar := &backendir.ListLiteralExpr{
		Type: &backendir.ListType{Elem: &backendir.TypeVarType{Name: "T"}},
		Elements: []backendir.Expr{
			&backendir.LiteralExpr{Kind: "int", Value: "1"},
		},
	}
	if emitter.canEmitExprNatively(listTypeVar) {
		t.Fatalf("expected list literal with type-var element type to stay on fallback path")
	}

	ifExprNoElse := &backendir.IfExpr{
		Cond: &backendir.LiteralExpr{Kind: "bool", Value: "true"},
		Then: &backendir.Block{Stmts: []backendir.Stmt{
			&backendir.ReturnStmt{Value: &backendir.LiteralExpr{Kind: "int", Value: "1"}},
		}},
		Type: backendir.IntType,
	}
	if emitter.canEmitExprNatively(ifExprNoElse) {
		t.Fatalf("expected if expression without else for non-void type to stay on fallback path")
	}
}

func assertParsesAsGo(t *testing.T, source []byte) {
	t.Helper()
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "generated.go", source, parser.AllErrors); err != nil {
		t.Fatalf("generated source is not valid Go: %v\n%s", err, string(source))
	}
}
