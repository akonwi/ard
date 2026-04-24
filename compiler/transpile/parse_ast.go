package transpile

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

func parseGoBlockStatements(source string) ([]ast.Stmt, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "generated.go", "package generated\nfunc __ard_wrapper() {\n"+source+"\n}\n", parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse generated go block: %w\n%s", err, source)
	}
	if len(file.Decls) == 0 {
		return nil, nil
	}
	fn, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok || fn.Body == nil {
		return nil, fmt.Errorf("expected wrapper function body while parsing generated block")
	}
	return fn.Body.List, nil
}
