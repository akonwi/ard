package go_backend

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestRenderGoFilePrelude(t *testing.T) {
	got, err := renderGoFilePrelude(lowerGoFileIR("main", map[string]string{
		helperImportPath: helperImportAlias,
		"sync":           "sync",
	}))
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	want := "package main\n\nimport (\n\tardgo \"github.com/akonwi/ard/go\"\n\tsync \"sync\"\n)\n"
	if string(got) != want {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", want, string(got))
	}
}

func TestRenderGoFile(t *testing.T) {
	fileIR := lowerGoFileIR("main", map[string]string{helperImportPath: helperImportAlias})
	appendASTDecl(&fileIR, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent("Person"), Type: &ast.StructType{Fields: &ast.FieldList{}}}}})
	appendASTDecl(&fileIR, &ast.FuncDecl{Name: ast.NewIdent("greet"), Type: &ast.FuncType{Params: &ast.FieldList{}, Results: funcResults(ast.NewIdent("string"))}, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"hi"`}}}}}})

	got, err := renderGoFile(fileIR)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	want := "package main\n\nimport ardgo \"github.com/akonwi/ard/go\"\n\ntype Person struct {\n}\n\nfunc greet() string {\n\treturn \"hi\"\n}\n"
	if string(got) != want {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", want, string(got))
	}
}
