package transpile

import (
	"go/ast"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) lowerPackageVariableDeclNode(def *checker.VariableDef) (ast.Decl, bool, error) {
	value, ok, err := e.lowerExprAST(def.Value)
	if err != nil || !ok {
		return nil, ok, err
	}
	name := goName(def.Name, !def.Mutable)
	var typeExpr ast.Expr
	if typeNeedsExplicitVarAnnotation(def.Type()) {
		typeExpr, err = e.lowerTypeExpr(def.Type())
		if err != nil {
			return nil, false, err
		}
	}
	return astVarDecl(astValueSpec(name, typeExpr, value)), true, nil
}
