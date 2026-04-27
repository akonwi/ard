package go_backend

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"strconv"
)

type goFileIR struct {
	PackageName string
	Imports     []goImportIR
	Decls       []goDeclIR
}

type goDeclIR struct {
	Decls []ast.Decl
}

type goImportIR struct {
	Alias string
	Path  string
}

func lowerGoFileIR(packageName string, imports map[string]string) goFileIR {
	file := goFileIR{PackageName: packageName}
	paths := sortedImportPaths(imports)
	file.Imports = make([]goImportIR, 0, len(paths))
	for _, importPath := range paths {
		file.Imports = append(file.Imports, goImportIR{
			Alias: imports[importPath],
			Path:  importPath,
		})
	}
	return file
}

func formatGoFileAST(file *ast.File) ([]byte, error) {
	fset := token.NewFileSet()
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, fmt.Errorf("format generated go AST: %w", err)
	}
	return buf.Bytes(), nil
}

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
