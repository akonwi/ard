package go_backend

import (
	"go/ast"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) lowerPackageVariableValueAST(def *checker.VariableDef) (ast.Expr, bool, error) {
	if def == nil {
		return nil, false, nil
	}
	value, ok, err := e.lowerValueForTypeAST(def.Value, def.Type())
	if err == nil && ok {
		return value, true, nil
	}
	prelude, lowered, preludeOK, preludeErr := e.lowerExprWithPreludeAST(def.Value, nil)
	if preludeErr != nil || !preludeOK {
		return nil, ok, err
	}
	lowered, preludeErr = e.wrapTraitValueAST(lowered, def.Type())
	if preludeErr != nil {
		return nil, false, preludeErr
	}
	if len(prelude) == 0 {
		return lowered, true, nil
	}
	body := append(prelude, &ast.ReturnStmt{Results: []ast.Expr{lowered}})
	wrapped, wrapErr := e.inlineFuncCallAST(def.Type(), body)
	if wrapErr != nil {
		return nil, false, wrapErr
	}
	return wrapped, true, nil
}

func (e *emitter) lowerPackageVariableDeclNode(def *checker.VariableDef) (ast.Decl, bool, error) {
	var (
		value ast.Expr
		ok    bool
		err   error
	)
	if freshErr := e.withFreshLocals(func() error {
		value, ok, err = e.lowerPackageVariableValueAST(def)
		if err != nil {
			return err
		}
		return nil
	}); freshErr != nil {
		return nil, false, freshErr
	}
	if !ok {
		return nil, false, nil
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
