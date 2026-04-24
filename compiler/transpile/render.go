package transpile

import (
	"go/ast"
	"go/token"
	"strconv"
)

func renderGoFilePrelude(fileIR goFileIR) ([]byte, error) {
	file := &ast.File{
		Name: ast.NewIdent(fileIR.PackageName),
	}
	if len(fileIR.Imports) > 0 {
		specs := make([]ast.Spec, 0, len(fileIR.Imports))
		for _, imp := range fileIR.Imports {
			spec := &ast.ImportSpec{
				Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(imp.Path)},
			}
			if imp.Alias != "" {
				spec.Name = ast.NewIdent(imp.Alias)
			}
			specs = append(specs, spec)
		}
		file.Decls = append(file.Decls, &ast.GenDecl{Tok: token.IMPORT, Specs: specs})
	}
	return formatGoFileAST(file)
}
