package transpile

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

func renderGoFilePrelude(fileIR goFileIR) ([]byte, error) {
	file := &ast.File{
		Name: ast.NewIdent(fileIR.PackageName),
	}
	if len(fileIR.Imports) > 0 {
		specs := make([]ast.Spec, 0, len(fileIR.Imports))
		file.Imports = make([]*ast.ImportSpec, 0, len(fileIR.Imports))
		for _, imp := range fileIR.Imports {
			spec := &ast.ImportSpec{
				Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(imp.Path)},
			}
			if imp.Alias != "" {
				spec.Name = ast.NewIdent(imp.Alias)
			}
			file.Imports = append(file.Imports, spec)
			specs = append(specs, spec)
		}
		file.Decls = append(file.Decls, &ast.GenDecl{Tok: token.IMPORT, Specs: specs})
	}
	return formatGoFileAST(file)
}

func renderGoFile(fileIR goFileIR) ([]byte, error) {
	file := &ast.File{Name: ast.NewIdent(fileIR.PackageName)}
	if len(fileIR.Imports) > 0 {
		specs := make([]ast.Spec, 0, len(fileIR.Imports))
		file.Imports = make([]*ast.ImportSpec, 0, len(fileIR.Imports))
		for _, imp := range fileIR.Imports {
			spec := &ast.ImportSpec{
				Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(imp.Path)},
			}
			if imp.Alias != "" {
				spec.Name = ast.NewIdent(imp.Alias)
			}
			file.Imports = append(file.Imports, spec)
			specs = append(specs, spec)
		}
		file.Decls = append(file.Decls, &ast.GenDecl{Tok: token.IMPORT, Specs: specs})
	}
	for _, decl := range fileIR.Decls {
		file.Decls = append(file.Decls, decl.Decls...)
	}
	return formatGoFileAST(file)
}

func parseGoDecls(packageName string, source string) ([]ast.Decl, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "generated.go", "package "+packageName+"\n\n"+source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse generated go declaration: %w\n%s", err, source)
	}
	return file.Decls, nil
}
