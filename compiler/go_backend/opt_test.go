package go_backend

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestOptimizeGoFileIR(t *testing.T) {
	fileIR := goFileIR{
		PackageName: "main",
		Imports: []goImportIR{
			{Alias: helperImportAlias, Path: helperImportPath},
			{Alias: helperImportAlias, Path: helperImportPath},
		},
		Decls: []goDeclIR{{}},
	}
	appendASTDecl(&fileIR, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent("Person"), Type: &ast.StructType{Fields: &ast.FieldList{}}}}})
	optimized := optimizeGoFileIR(fileIR)

	if len(optimized.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(optimized.Imports))
	}
	if len(optimized.Decls) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(optimized.Decls))
	}
	if len(optimized.Decls[0].Decls) != 1 {
		t.Fatalf("expected 1 parsed declaration, got %d", len(optimized.Decls[0].Decls))
	}
}
