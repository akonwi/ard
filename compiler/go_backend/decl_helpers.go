package go_backend

import "go/ast"

func appendASTDecl(fileIR *goFileIR, decl ast.Decl) {
	if decl == nil {
		return
	}
	fileIR.Decls = append(fileIR.Decls, goDeclIR{Decls: []ast.Decl{decl}})
}

func typeParamFieldList(order []string, mapping map[string]string, constraints map[string]string) *ast.FieldList {
	if len(order) == 0 || len(mapping) == 0 {
		return nil
	}
	fields := make([]*ast.Field, 0, len(order))
	for _, name := range order {
		emitted := mapping[name]
		if emitted == "" {
			continue
		}
		constraint := constraints[name]
		if constraint == "" {
			constraint = "any"
		}
		fields = append(fields, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(emitted)},
			Type:  ast.NewIdent(constraint),
		})
	}
	if len(fields) == 0 {
		return nil
	}
	return &ast.FieldList{List: fields}
}

func funcResults(returnType ast.Expr) *ast.FieldList {
	if returnType == nil {
		return nil
	}
	return &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
}
