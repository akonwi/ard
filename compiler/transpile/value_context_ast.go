package transpile

import (
	"go/ast"

	"github.com/akonwi/ard/checker"
)

func (e *emitter) wrapTraitValueAST(value ast.Expr, expectedType checker.Type) (ast.Expr, error) {
	trait, ok := expectedType.(*checker.Trait)
	if !ok {
		return value, nil
	}
	switch trait.Name {
	case "ToString":
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "AsToString"), Args: []ast.Expr{value}}, nil
	case "Encodable":
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "AsEncodable"), Args: []ast.Expr{value}}, nil
	default:
		return value, nil
	}
}

func (e *emitter) lowerValueForTypeAST(expr checker.Expression, expectedType checker.Type) (ast.Expr, bool, error) {
	switch v := expr.(type) {
	case *checker.ModuleFunctionCall:
		switch v.Module {
		case "ard/maybe":
			if expectedType != nil {
				if _, ok := expectedType.(*checker.Maybe); !ok {
					return nil, false, errStructuredLoweringUnsupported
				}
			}
			if value, ok, err := e.lowerMaybeModuleCallWithExpectedAST(v, expectedType); ok || err != nil {
				return value, ok, err
			}
		case "ard/result":
			if expectedType != nil {
				if _, ok := expectedType.(*checker.Result); !ok {
					return nil, false, errStructuredLoweringUnsupported
				}
			}
			if value, ok, err := e.lowerResultModuleCallWithExpectedAST(v, expectedType); ok || err != nil {
				return value, ok, err
			}
		}
	case *checker.BoolMatch:
		if value, ok, err := e.lowerBoolMatchWithExpectedAST(v, expectedType); ok || err != nil {
			return value, ok, err
		}
	case *checker.IntMatch:
		if value, ok, err := e.lowerIntMatchWithExpectedAST(v, expectedType); ok || err != nil {
			return value, ok, err
		}
	case *checker.ConditionalMatch:
		if value, ok, err := e.lowerConditionalMatchWithExpectedAST(v, expectedType); ok || err != nil {
			return value, ok, err
		}
	case *checker.OptionMatch:
		if value, ok, err := e.lowerOptionMatchWithExpectedAST(v, expectedType); ok || err != nil {
			return value, ok, err
		}
	case *checker.ResultMatch:
		if value, ok, err := e.lowerResultMatchWithExpectedAST(v, expectedType); ok || err != nil {
			return value, ok, err
		}
	case *checker.EnumMatch:
		if value, ok, err := e.lowerEnumMatchWithExpectedAST(v, expectedType); ok || err != nil {
			return value, ok, err
		}
	case *checker.UnionMatch:
		if value, ok, err := e.lowerUnionMatchWithExpectedAST(v, expectedType); ok || err != nil {
			return value, ok, err
		}
	}
	value, ok, err := e.lowerExprAST(expr)
	if err != nil || !ok {
		return nil, ok, err
	}
	value, err = e.wrapTraitValueAST(value, expectedType)
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}
