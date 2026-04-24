package transpile

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
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
