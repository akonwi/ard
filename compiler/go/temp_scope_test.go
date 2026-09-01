package gotarget

import (
	"go/ast"
	"testing"
)

func TestGeneratedTemporaryNamesRestartForEachFunction(t *testing.T) {
	program := lowerSource(t, `
		fn first(value: Int?) Int {
			let actual = try value -> _ { 0 }
			actual
		}

		fn second(value: Int?) Int {
			let actual = try value -> _ { 0 }
			actual
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	for _, functionName := range []string{"First", "Second"} {
		function := findGeneratedFunction(t, files, functionName)
		if !astNodeContainsIdent(function, "_tmp_0") {
			t.Fatalf("generated function %s does not restart temporary names at _tmp_0", functionName)
		}
	}
}

func findGeneratedFunction(t *testing.T, files map[string]*ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Name != nil && function.Name.Name == name {
				return function
			}
		}
	}
	t.Fatalf("generated function %s not found", name)
	return nil
}

func astNodeContainsIdent(node ast.Node, name string) bool {
	found := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		identifier, ok := candidate.(*ast.Ident)
		if ok && identifier.Name == name {
			found = true
			return false
		}
		return !found
	})
	return found
}
