package transpile

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
)

func formatGoFileAST(file *ast.File) ([]byte, error) {
	fset := token.NewFileSet()
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, fmt.Errorf("format generated go AST: %w", err)
	}
	return buf.Bytes(), nil
}

func formatGeneratedGoSource(source string) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "generated.go", source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse generated go: %w\n%s", err, source)
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, fmt.Errorf("format generated go: %w\n%s", err, source)
	}
	return buf.Bytes(), nil
}
