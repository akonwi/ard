package gotarget

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/token"
)

func renderFile(file *ast.File) ([]byte, error) {
	var out bytes.Buffer
	if err := format.Node(&out, token.NewFileSet(), file); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
