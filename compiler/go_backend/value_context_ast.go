package go_backend

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
	if typeVar, ok := expectedType.(*checker.TypeVar); ok {
		if actual := typeVar.Actual(); actual != nil {
			expectedType = actual
		}
	}

	switch v := expr.(type) {
	case *checker.ListLiteral:
		if expectedList, ok := expectedType.(*checker.List); ok {
			bindings := make(map[string]checker.Type)
			for _, element := range v.Elements {
				inferBindingsFromArgExpr(expectedList.Of(), element, bindings)
			}
			elemType := applyTypeBindings(expectedList.Of(), bindings)
			if hasOpenTypeVars(elemType) && len(v.Elements) > 0 {
				inferredElem := v.Elements[0].Type()
				allSame := true
				for _, element := range v.Elements[1:] {
					if element.Type().String() != inferredElem.String() {
						allSame = false
						break
					}
				}
				if allSame {
					elemType = inferredElem
				}
			}
			listType := checker.MakeList(elemType)
			typeExpr, err := e.lowerTypeExpr(listType)
			if err != nil {
				return nil, false, err
			}
			elts := make([]ast.Expr, 0, len(v.Elements))
			for _, element := range v.Elements {
				item, ok, err := e.lowerValueForTypeAST(element, elemType)
				if err != nil || !ok {
					return nil, ok, err
				}
				elts = append(elts, item)
			}
			return &ast.CompositeLit{Type: typeExpr, Elts: elts}, true, nil
		}
	case *checker.MapLiteral:
		if expectedMap, ok := expectedType.(*checker.Map); ok {
			bindings := make(map[string]checker.Type)
			for _, key := range v.Keys {
				inferBindingsFromArgExpr(expectedMap.Key(), key, bindings)
			}
			for _, value := range v.Values {
				inferBindingsFromArgExpr(expectedMap.Value(), value, bindings)
			}
			keyType := applyTypeBindings(expectedMap.Key(), bindings)
			valueType := applyTypeBindings(expectedMap.Value(), bindings)
			mapType := checker.MakeMap(keyType, valueType)
			typeExpr, err := e.lowerTypeExpr(mapType)
			if err != nil {
				return nil, false, err
			}
			elts := make([]ast.Expr, 0, len(v.Keys))
			for i := range v.Keys {
				keyExpr, ok, err := e.lowerValueForTypeAST(v.Keys[i], keyType)
				if err != nil || !ok {
					return nil, ok, err
				}
				valueExpr, ok, err := e.lowerValueForTypeAST(v.Values[i], valueType)
				if err != nil || !ok {
					return nil, ok, err
				}
				elts = append(elts, &ast.KeyValueExpr{Key: keyExpr, Value: valueExpr})
			}
			return &ast.CompositeLit{Type: typeExpr, Elts: elts}, true, nil
		}
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
		if expectedType != nil {
			if value, ok, err := e.lowerModuleFunctionCallWithExpectedTypeAST(v, expectedType); ok || err != nil {
				if err != nil || !ok {
					return value, ok, err
				}
				value, err = e.wrapTraitValueAST(value, expectedType)
				if err != nil {
					return nil, false, err
				}
				return value, true, nil
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

func (e *emitter) lowerModuleFunctionCallWithExpectedTypeAST(call *checker.ModuleFunctionCall, expectedType checker.Type) (ast.Expr, bool, error) {
	if call == nil || call.Call == nil || expectedType == nil {
		return nil, false, nil
	}
	if expr, ok, err := e.lowerSpecialModuleCallAST(call); ok || err != nil {
		return expr, ok, err
	}
	original := e.originalModuleFunctionDef(call.Module, call.Call)
	if original == nil {
		return nil, false, nil
	}
	specialized := specializeFunctionDefForCall(original, call.Call.Args, expectedType)
	if specialized == nil {
		return nil, false, nil
	}
	args, ok, err := e.lowerCallArgsWithParamsAST(call.Call, specialized.Parameters)
	if err != nil || !ok {
		return nil, ok, err
	}
	typeArgs, err := e.lowerFunctionCallTypeArgsAST(original, specialized)
	if err != nil {
		return nil, false, err
	}
	alias := packageNameForModulePath(call.Module)
	name := goName(e.resolvedModuleFunctionName(call.Module, call.Call), true)
	return astCall(selectorExpr(ast.NewIdent(alias), name), typeArgs, args), true, nil
}

func hasOpenTypeVars(t checker.Type) bool {
	if t == nil {
		return false
	}
	switch typed := t.(type) {
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			return hasOpenTypeVars(actual)
		}
		return true
	case *checker.List:
		return hasOpenTypeVars(typed.Of())
	case *checker.Map:
		return hasOpenTypeVars(typed.Key()) || hasOpenTypeVars(typed.Value())
	case *checker.Maybe:
		return hasOpenTypeVars(typed.Of())
	case *checker.Result:
		return hasOpenTypeVars(typed.Val()) || hasOpenTypeVars(typed.Err())
	case *checker.FunctionDef:
		for _, param := range typed.Parameters {
			if hasOpenTypeVars(param.Type) {
				return true
			}
		}
		return hasOpenTypeVars(effectiveFunctionReturnType(typed))
	case *checker.StructDef:
		for _, fieldType := range typed.Fields {
			if hasOpenTypeVars(fieldType) {
				return true
			}
		}
		return false
	case *checker.ExternType:
		for _, arg := range typed.TypeArgs {
			if hasOpenTypeVars(arg) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
