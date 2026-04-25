package go_backend

import (
	"go/ast"
	"go/token"

	"github.com/akonwi/ard/checker"
)

func appendASTDecl(fileIR *goFileIR, decl ast.Decl) {
	if decl == nil {
		return
	}
	fileIR.Decls = append(fileIR.Decls, goDeclIR{Decls: []ast.Decl{decl}})
}

func (e *emitter) lowerStructTypeDeclNode(def *checker.StructDef) (ast.Decl, error) {
	if _, ok := e.emittedTypes["struct:"+def.Name]; ok {
		return nil, nil
	}
	e.emittedTypes["struct:"+def.Name] = struct{}{}
	order, mapping, constraints := structTypeParams(def)
	var decl ast.Decl
	err := e.withTypeParams(mapping, func() error {
		fields := make([]*ast.Field, 0, len(def.Fields))
		for _, fieldName := range sortedStringKeys(def.Fields) {
			typeExpr, err := e.lowerTypeExpr(def.Fields[fieldName])
			if err != nil {
				return err
			}
			fields = append(fields, &ast.Field{
				Names: []*ast.Ident{ast.NewIdent(goName(fieldName, true))},
				Type:  typeExpr,
			})
		}
		decl = &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
			Name:       ast.NewIdent(goName(def.Name, true)),
			TypeParams: typeParamFieldList(order, mapping, constraints),
			Type:       &ast.StructType{Fields: &ast.FieldList{List: fields}},
		}}}
		return nil
	})
	return decl, err
}

func (e *emitter) lowerEnumTypeDeclNode(def *checker.Enum) ast.Decl {
	if _, ok := e.emittedTypes["enum:"+def.Name]; ok {
		return nil
	}
	e.emittedTypes["enum:"+def.Name] = struct{}{}
	return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name: ast.NewIdent(goName(def.Name, true)),
		Type: &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ast.NewIdent("Tag")},
			Type:  ast.NewIdent("int"),
		}}}},
	}}}
}
