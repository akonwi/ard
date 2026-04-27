package go_backend

import (
	"errors"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/akonwi/ard/checker"
)

var errStructuredLoweringUnsupported = errors.New("structured lowering unsupported")

func (e *emitter) lowerLocalValueExpr(name string) ast.Expr {
	resolved := e.resolveLocal(name)
	if e.isPointerLocal(name) {
		return &ast.StarExpr{X: ast.NewIdent(resolved)}
	}
	return ast.NewIdent(resolved)
}

func (e *emitter) lowerLocalTargetExpr(name string) ast.Expr {
	resolved := e.resolveLocal(name)
	if e.isPointerLocal(name) {
		return &ast.StarExpr{X: ast.NewIdent(resolved)}
	}
	return ast.NewIdent(resolved)
}

func (e *emitter) lowerFunctionCallTypeArgsAST(originalDef, specializedDef *checker.FunctionDef) ([]ast.Expr, error) {
	if originalDef == nil || specializedDef == nil {
		return nil, nil
	}
	order, _, _ := functionTypeParams(originalDef)
	if len(order) == 0 {
		return nil, nil
	}
	bindings := make(map[string]checker.Type, len(order))
	for i := 0; i < len(originalDef.Parameters) && i < len(specializedDef.Parameters); i++ {
		inferGenericTypeBindings(originalDef.Parameters[i].Type, specializedDef.Parameters[i].Type, bindings)
	}
	inferGenericTypeBindings(effectiveFunctionReturnType(originalDef), effectiveFunctionReturnType(specializedDef), bindings)
	parts := make([]ast.Expr, 0, len(order))
	for _, name := range order {
		bound := bindings[name]
		if bound == nil {
			return nil, nil
		}
		if tv, ok := bound.(*checker.TypeVar); ok {
			if actual := tv.Actual(); actual != nil {
				bound = actual
			} else {
				return nil, nil
			}
		}
		emitted, err := e.lowerTypeArgExprWithOptions(bound, e.typeParams, nil)
		if err != nil {
			return nil, err
		}
		parts = append(parts, emitted)
	}
	return parts, nil
}

func astCall(fun ast.Expr, typeArgs []ast.Expr, args []ast.Expr) ast.Expr {
	if len(typeArgs) > 0 {
		fun = indexExpr(fun, typeArgs)
	}
	return &ast.CallExpr{Fun: fun, Args: args}
}

func (e *emitter) lowerCallArgsWithParamsAST(call *checker.FunctionCall, params []checker.Parameter) ([]ast.Expr, bool, error) {
	args := make([]ast.Expr, 0, len(call.Args))
	for i, arg := range call.Args {
		hasParam := i < len(params)
		var (
			emitted ast.Expr
			ok      bool
			err     error
		)
		if hasParam && params[i].Mutable {
			emitted, ok, err = e.lowerMutableCallArgAST(arg, params[i])
		} else if hasParam {
			emitted, ok, err = e.lowerValueForTypeAST(arg, params[i].Type)
		} else {
			emitted, ok, err = e.lowerExprAST(arg)
		}
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, nil
		}
		args = append(args, emitted)
	}
	return args, true, nil
}

func (e *emitter) lowerCallArgsAST(call *checker.FunctionCall) ([]ast.Expr, bool, error) {
	var params []checker.Parameter
	if def := call.Definition(); def != nil {
		params = def.Parameters
	}
	return e.lowerCallArgsWithParamsAST(call, params)
}

func (e *emitter) lowerModuleCallArgsAST(modulePath string, call *checker.FunctionCall) ([]ast.Expr, bool, error) {
	var params []checker.Parameter
	if specialized := e.specializedModuleFunctionDef(modulePath, call); specialized != nil {
		params = specialized.Parameters
	}
	return e.lowerCallArgsWithParamsAST(call, params)
}

func (e *emitter) lowerExprAST(expr checker.Expression) (ast.Expr, bool, error) {
	switch v := expr.(type) {
	case *checker.IntLiteral:
		return &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(v.Value)}, true, nil
	case *checker.FloatLiteral:
		return &ast.BasicLit{Kind: token.FLOAT, Value: strconv.FormatFloat(v.Value, 'g', -1, 64)}, true, nil
	case *checker.StrLiteral:
		return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(v.Value)}, true, nil
	case *checker.BoolLiteral:
		if v.Value {
			return ast.NewIdent("true"), true, nil
		}
		return ast.NewIdent("false"), true, nil
	case *checker.VoidLiteral:
		return &ast.CompositeLit{Type: &ast.StructType{Fields: &ast.FieldList{}}}, true, nil
	case *checker.TemplateStr:
		if len(v.Chunks) == 0 {
			return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("")}, true, nil
		}
		var out ast.Expr
		for i, chunk := range v.Chunks {
			item, ok, err := e.lowerExprAST(chunk)
			if err != nil || !ok {
				return nil, ok, err
			}
			if i == 0 {
				out = item
			} else {
				out = &ast.BinaryExpr{X: out, Op: token.ADD, Y: item}
			}
		}
		return out, true, nil
	case *checker.Identifier:
		return e.lowerLocalValueExpr(v.Name), true, nil
	case checker.Variable:
		return e.lowerLocalValueExpr(v.Name()), true, nil
	case *checker.Variable:
		return e.lowerLocalValueExpr(v.Name()), true, nil
	case *checker.InstanceProperty:
		subject, ok, err := e.lowerExprAST(v.Subject)
		if err != nil || !ok {
			return nil, ok, err
		}
		return selectorExpr(subject, goName(v.Property, true)), true, nil
	case *checker.Negation:
		inner, ok, err := e.lowerExprAST(v.Value)
		if err != nil || !ok {
			return nil, ok, err
		}
		return &ast.UnaryExpr{Op: token.SUB, X: inner}, true, nil
	case *checker.Not:
		inner, ok, err := e.lowerExprAST(v.Value)
		if err != nil || !ok {
			return nil, ok, err
		}
		return &ast.UnaryExpr{Op: token.NOT, X: inner}, true, nil
	case *checker.IntAddition:
		return e.lowerBinaryExprAST(v.Left, token.ADD, v.Right)
	case *checker.IntSubtraction:
		return e.lowerBinaryExprAST(v.Left, token.SUB, v.Right)
	case *checker.IntMultiplication:
		return e.lowerBinaryExprAST(v.Left, token.MUL, v.Right)
	case *checker.IntDivision:
		return e.lowerBinaryExprAST(v.Left, token.QUO, v.Right)
	case *checker.IntModulo:
		return e.lowerBinaryExprAST(v.Left, token.REM, v.Right)
	case *checker.FloatAddition:
		return e.lowerBinaryExprAST(v.Left, token.ADD, v.Right)
	case *checker.FloatSubtraction:
		return e.lowerBinaryExprAST(v.Left, token.SUB, v.Right)
	case *checker.FloatMultiplication:
		return e.lowerBinaryExprAST(v.Left, token.MUL, v.Right)
	case *checker.FloatDivision:
		return e.lowerBinaryExprAST(v.Left, token.QUO, v.Right)
	case *checker.StrAddition:
		return e.lowerBinaryExprAST(v.Left, token.ADD, v.Right)
	case *checker.IntGreater:
		return e.lowerBinaryExprAST(v.Left, token.GTR, v.Right)
	case *checker.IntGreaterEqual:
		return e.lowerBinaryExprAST(v.Left, token.GEQ, v.Right)
	case *checker.IntLess:
		return e.lowerBinaryExprAST(v.Left, token.LSS, v.Right)
	case *checker.IntLessEqual:
		return e.lowerBinaryExprAST(v.Left, token.LEQ, v.Right)
	case *checker.FloatGreater:
		return e.lowerBinaryExprAST(v.Left, token.GTR, v.Right)
	case *checker.FloatGreaterEqual:
		return e.lowerBinaryExprAST(v.Left, token.GEQ, v.Right)
	case *checker.FloatLess:
		return e.lowerBinaryExprAST(v.Left, token.LSS, v.Right)
	case *checker.FloatLessEqual:
		return e.lowerBinaryExprAST(v.Left, token.LEQ, v.Right)
	case *checker.Equality:
		return e.lowerBinaryExprAST(v.Left, token.EQL, v.Right)
	case *checker.And:
		return e.lowerBinaryExprAST(v.Left, token.LAND, v.Right)
	case *checker.Or:
		return e.lowerBinaryExprAST(v.Left, token.LOR, v.Right)
	case *checker.ListLiteral:
		typeExpr, err := e.lowerTypeExpr(v.ListType)
		if err != nil {
			return nil, false, err
		}
		elts := make([]ast.Expr, 0, len(v.Elements))
		for _, element := range v.Elements {
			item, ok, err := e.lowerExprAST(element)
			if err != nil || !ok {
				return nil, ok, err
			}
			elts = append(elts, item)
		}
		return &ast.CompositeLit{Type: typeExpr, Elts: elts}, true, nil
	case *checker.MapLiteral:
		typeExpr, err := e.lowerTypeExpr(v.Type())
		if err != nil {
			return nil, false, err
		}
		elts := make([]ast.Expr, 0, len(v.Keys))
		for i := range v.Keys {
			key, ok, err := e.lowerExprAST(v.Keys[i])
			if err != nil || !ok {
				return nil, ok, err
			}
			value, ok, err := e.lowerExprAST(v.Values[i])
			if err != nil || !ok {
				return nil, ok, err
			}
			elts = append(elts, &ast.KeyValueExpr{Key: key, Value: value})
		}
		return &ast.CompositeLit{Type: typeExpr, Elts: elts}, true, nil
	case *checker.StructInstance:
		typeExpr, err := e.lowerTypeExpr(v.StructType)
		if err != nil {
			return nil, false, err
		}
		elts := make([]ast.Expr, 0, len(v.Fields))
		for _, fieldName := range sortedStringKeys(v.Fields) {
			value, ok, err := e.lowerValueForTypeAST(v.Fields[fieldName], v.FieldTypes[fieldName])
			if err != nil || !ok {
				return nil, ok, err
			}
			elts = append(elts, &ast.KeyValueExpr{Key: ast.NewIdent(goName(fieldName, true)), Value: value})
		}
		return &ast.CompositeLit{Type: typeExpr, Elts: elts}, true, nil
	case *checker.ModuleStructInstance:
		typeExpr, err := e.lowerTypeExpr(v.StructType)
		if err != nil {
			return nil, false, err
		}
		elts := make([]ast.Expr, 0, len(v.Property.Fields))
		for _, fieldName := range sortedStringKeys(v.Property.Fields) {
			value, ok, err := e.lowerValueForTypeAST(v.Property.Fields[fieldName], v.FieldTypes[fieldName])
			if err != nil || !ok {
				return nil, ok, err
			}
			elts = append(elts, &ast.KeyValueExpr{Key: ast.NewIdent(goName(fieldName, true)), Value: value})
		}
		return &ast.CompositeLit{Type: typeExpr, Elts: elts}, true, nil
	case *checker.EnumVariant:
		if v != nil && v.EnumType != nil {
			if enumType, ok := v.EnumType.(*checker.Enum); ok && len(enumType.Methods) > 0 {
				typeExpr, err := e.lowerTypeExpr(enumType)
				if err != nil {
					return nil, false, err
				}
				return &ast.CompositeLit{Type: typeExpr, Elts: []ast.Expr{&ast.KeyValueExpr{Key: ast.NewIdent("Tag"), Value: &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(v.Discriminant)}}}}, true, nil
			}
		}
		return &ast.CompositeLit{Type: &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("Tag")}, Type: ast.NewIdent("int")}}}}, Elts: []ast.Expr{&ast.KeyValueExpr{Key: ast.NewIdent("Tag"), Value: &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(v.Discriminant)}}}}, true, nil
	case *checker.CopyExpression:
		inner, ok, err := e.lowerExprAST(v.Expr)
		if err != nil || !ok {
			return nil, ok, err
		}
		switch typed := v.Type_.(type) {
		case *checker.List:
			typeExpr, err := e.lowerTypeExpr(typed)
			if err != nil {
				return nil, false, err
			}
			return &ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{&ast.CallExpr{Fun: typeExpr, Args: []ast.Expr{ast.NewIdent("nil")}}, inner}, Ellipsis: token.Pos(1)}, true, nil
		default:
			return inner, true, nil
		}
	case *checker.FunctionCall:
		args, ok, err := e.lowerCallArgsAST(v)
		if err != nil || !ok {
			return nil, ok, err
		}
		typeArgs, err := e.lowerFunctionCallTypeArgsAST(e.originalFunctionDef(v), v.Definition())
		if err != nil {
			return nil, false, err
		}
		name := e.functionNames[v.Name]
		if name == "" {
			name = goName(v.Name, false)
		}
		return astCall(ast.NewIdent(name), typeArgs, args), true, nil
	case *checker.ModuleFunctionCall:
		if expr, ok, err := e.lowerSpecialModuleCallAST(v); ok || err != nil {
			return expr, ok, err
		}
		args, ok, err := e.lowerModuleCallArgsAST(v.Module, v.Call)
		if err != nil || !ok {
			return nil, ok, err
		}
		original := e.originalModuleFunctionDef(v.Module, v.Call)
		specialized := e.specializedModuleFunctionDef(v.Module, v.Call)
		typeArgs, err := e.lowerFunctionCallTypeArgsAST(original, specialized)
		if err != nil {
			return nil, false, err
		}
		alias := packageNameForModulePath(v.Module)
		name := goName(e.resolvedModuleFunctionName(v.Module, v.Call), true)
		return astCall(selectorExpr(ast.NewIdent(alias), name), typeArgs, args), true, nil
	case *checker.ModuleSymbol:
		return selectorExpr(ast.NewIdent(packageNameForModulePath(v.Module)), goName(v.Symbol.Name, true)), true, nil
	case *checker.InstanceMethod:
		subject, ok, err := e.lowerExprAST(v.Subject)
		if err != nil || !ok {
			return nil, ok, err
		}
		args, ok, err := e.lowerCallArgsAST(v.Method)
		if err != nil || !ok {
			return nil, ok, err
		}
		methodName := goName(v.Method.Name, false)
		if v.StructType != nil {
			if method, ok := v.StructType.Methods[v.Method.Name]; ok {
				methodName = goName(method.Name, !method.Private)
			}
		}
		if v.EnumType != nil {
			if method, ok := v.EnumType.Methods[v.Method.Name]; ok {
				methodName = goName(method.Name, !method.Private)
			}
		}
		if v.TraitType != nil {
			methodName = goName(v.Method.Name, true)
		}
		return &ast.CallExpr{Fun: selectorExpr(subject, methodName), Args: args}, true, nil
	case *checker.StrMethod:
		return e.lowerStrMethodAST(v)
	case *checker.ListMethod:
		return e.lowerListMethodAST(v)
	case *checker.MapMethod:
		return e.lowerMapMethodAST(v)
	case *checker.MaybeMethod:
		return e.lowerMaybeMethodAST(v)
	case *checker.ResultMethod:
		return e.lowerResultMethodAST(v)
	case *checker.IntMethod:
		subject, ok, err := e.lowerExprAST(v.Subject)
		if err != nil || !ok {
			return nil, ok, err
		}
		switch v.Kind {
		case checker.IntToStr:
			return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent("strconv"), "Itoa"), Args: []ast.Expr{subject}}, true, nil
		default:
			return nil, false, nil
		}
	case *checker.FloatMethod:
		subject, ok, err := e.lowerExprAST(v.Subject)
		if err != nil || !ok {
			return nil, ok, err
		}
		switch v.Kind {
		case checker.FloatToStr:
			return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent("strconv"), "FormatFloat"), Args: []ast.Expr{subject, &ast.BasicLit{Kind: token.CHAR, Value: "'f'"}, &ast.BasicLit{Kind: token.INT, Value: "2"}, &ast.BasicLit{Kind: token.INT, Value: "64"}}}, true, nil
		case checker.FloatToInt:
			out, err := e.inlineFuncCallAST(v.Type(), []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("value")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("float64"), Args: []ast.Expr{subject}}}},
				&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int"), Args: []ast.Expr{ast.NewIdent("value")}}}},
			})
			return out, err == nil, err
		default:
			return nil, false, nil
		}
	case *checker.BoolMethod:
		subject, ok, err := e.lowerExprAST(v.Subject)
		if err != nil || !ok {
			return nil, ok, err
		}
		switch v.Kind {
		case checker.BoolToStr:
			return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent("strconv"), "FormatBool"), Args: []ast.Expr{subject}}, true, nil
		default:
			return nil, false, nil
		}
	case *checker.FiberStart:
		if v == nil || v.GetFn() == nil {
			return nil, false, nil
		}
		fn, ok, err := e.lowerExprAST(v.GetFn())
		if err != nil || !ok {
			return nil, ok, err
		}
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(packageNameForModulePath("ard/async")), goName("start", true)), Args: []ast.Expr{fn}}, true, nil
	case *checker.FiberEval:
		if v == nil || v.GetFn() == nil {
			return nil, false, nil
		}
		fn, ok, err := e.lowerExprAST(v.GetFn())
		if err != nil || !ok {
			return nil, ok, err
		}
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(packageNameForModulePath("ard/async")), goName("eval", true)), Args: []ast.Expr{fn}}, true, nil
	case *checker.FiberExecution:
		if v == nil || v.GetModule() == nil {
			return nil, false, nil
		}
		moduleAlias := packageNameForModulePath(v.GetModule().Path())
		asyncAlias := packageNameForModulePath("ard/async")
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(asyncAlias), goName("start", true)), Args: []ast.Expr{selectorExpr(ast.NewIdent(moduleAlias), goName(v.GetMainName(), true))}}, true, nil
	case *checker.FunctionDef:
		return e.lowerFunctionLiteralAST(v)
	case *checker.If:
		return e.lowerIfExprAST(v)
	case *checker.BoolMatch:
		return e.lowerBoolMatchAST(v)
	case *checker.IntMatch:
		return e.lowerIntMatchAST(v)
	case *checker.ConditionalMatch:
		return e.lowerConditionalMatchAST(v)
	case *checker.OptionMatch:
		return e.lowerOptionMatchAST(v)
	case *checker.ResultMatch:
		return e.lowerResultMatchAST(v)
	case *checker.EnumMatch:
		return e.lowerEnumMatchAST(v)
	case *checker.UnionMatch:
		return e.lowerUnionMatchAST(v)
	case *checker.Panic:
		return e.lowerPanicExprAST(v.Message, v.Type())
	case checker.Panic:
		return e.lowerPanicExprAST(v.Message, v.Type())
	default:
		return nil, false, nil
	}
}

func (e *emitter) specializedModuleFunctionDef(modulePath string, call *checker.FunctionCall) *checker.FunctionDef {
	if call == nil {
		return nil
	}
	original := e.originalModuleFunctionDef(modulePath, call)
	if specialized := specializeFunctionDefForCall(original, call.Args, nil); specialized != nil {
		return specialized
	}
	if def := call.Definition(); def != nil {
		return def
	}
	return original
}

func specializeFunctionDefFromArgs(original *checker.FunctionDef, args []checker.Expression) *checker.FunctionDef {
	return specializeFunctionDefForCall(original, args, nil)
}

func specializeFunctionDefForCall(original *checker.FunctionDef, args []checker.Expression, expectedReturn checker.Type) *checker.FunctionDef {
	if original == nil {
		return nil
	}
	bindings := make(map[string]checker.Type)
	for i := 0; i < len(original.Parameters) && i < len(args); i++ {
		inferBindingsFromArgExpr(original.Parameters[i].Type, args[i], bindings)
	}
	if expectedReturn != nil {
		inferGenericTypeBindings(effectiveFunctionReturnType(original), expectedReturn, bindings)
	}
	if len(bindings) == 0 {
		return original
	}
	params := make([]checker.Parameter, len(original.Parameters))
	for i := range original.Parameters {
		params[i] = checker.Parameter{
			Name:    original.Parameters[i].Name,
			Type:    applyTypeBindings(original.Parameters[i].Type, bindings),
			Mutable: original.Parameters[i].Mutable,
		}
	}
	return &checker.FunctionDef{
		Name:       original.Name,
		Parameters: params,
		ReturnType: applyTypeBindings(effectiveFunctionReturnType(original), bindings),
		Mutates:    original.Mutates,
		Private:    original.Private,
	}
}

func inferBindingsFromArgExpr(expected checker.Type, arg checker.Expression, bindings map[string]checker.Type) {
	if expected == nil || arg == nil {
		return
	}
	inferGenericTypeBindings(expected, arg.Type(), bindings)

	switch expectedTyped := expected.(type) {
	case *checker.List:
		if listLiteral, ok := arg.(*checker.ListLiteral); ok {
			for _, element := range listLiteral.Elements {
				inferBindingsFromArgExpr(expectedTyped.Of(), element, bindings)
			}
		}
	case *checker.Map:
		if mapLiteral, ok := arg.(*checker.MapLiteral); ok {
			for _, key := range mapLiteral.Keys {
				inferBindingsFromArgExpr(expectedTyped.Key(), key, bindings)
			}
			for _, value := range mapLiteral.Values {
				inferBindingsFromArgExpr(expectedTyped.Value(), value, bindings)
			}
		}
	}
}

func applyTypeBindings(t checker.Type, bindings map[string]checker.Type) checker.Type {
	if t == nil {
		return nil
	}
	switch typed := t.(type) {
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			return applyTypeBindings(actual, bindings)
		}
		if bound, ok := bindings[typed.Name()]; ok {
			return bound
		}
		return typed
	case *checker.List:
		return checker.MakeList(applyTypeBindings(typed.Of(), bindings))
	case *checker.Map:
		return checker.MakeMap(
			applyTypeBindings(typed.Key(), bindings),
			applyTypeBindings(typed.Value(), bindings),
		)
	case *checker.Maybe:
		return checker.MakeMaybe(applyTypeBindings(typed.Of(), bindings))
	case *checker.Result:
		return checker.MakeResult(
			applyTypeBindings(typed.Val(), bindings),
			applyTypeBindings(typed.Err(), bindings),
		)
	case *checker.FunctionDef:
		params := make([]checker.Parameter, len(typed.Parameters))
		for i := range typed.Parameters {
			params[i] = checker.Parameter{
				Name:    typed.Parameters[i].Name,
				Type:    applyTypeBindings(typed.Parameters[i].Type, bindings),
				Mutable: typed.Parameters[i].Mutable,
			}
		}
		return &checker.FunctionDef{
			Name:                    typed.Name,
			Receiver:                typed.Receiver,
			Parameters:              params,
			ReturnType:              applyTypeBindings(effectiveFunctionReturnType(typed), bindings),
			InferReturnTypeFromBody: typed.InferReturnTypeFromBody,
			Mutates:                 typed.Mutates,
			IsTest:                  typed.IsTest,
			Body:                    typed.Body,
			Private:                 typed.Private,
		}
	case *checker.ExternType:
		args := make([]checker.Type, 0, len(typed.TypeArgs))
		for _, typeArg := range typed.TypeArgs {
			args = append(args, applyTypeBindings(typeArg, bindings))
		}
		return &checker.ExternType{
			Name_:         typed.Name_,
			GenericParams: append([]string(nil), typed.GenericParams...),
			TypeArgs:      args,
		}
	default:
		return t
	}
}

func (e *emitter) lowerBinaryExprAST(left checker.Expression, op token.Token, right checker.Expression) (ast.Expr, bool, error) {
	l, ok, err := e.lowerExprAST(left)
	if err != nil || !ok {
		return nil, ok, err
	}
	r, ok, err := e.lowerExprAST(right)
	if err != nil || !ok {
		return nil, ok, err
	}
	return &ast.BinaryExpr{X: l, Op: op, Y: r}, true, nil
}

func (e *emitter) lowerStrMethodAST(v *checker.StrMethod) (ast.Expr, bool, error) {
	subject, ok, err := e.lowerExprAST(v.Subject)
	if err != nil || !ok {
		return nil, ok, err
	}
	arg := func(i int) (ast.Expr, bool, error) {
		if i >= len(v.Args) {
			return nil, false, errStructuredLoweringUnsupported
		}
		return e.lowerExprAST(v.Args[i])
	}
	switch v.Kind {
	case checker.StrSize:
		return &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{subject}}, true, nil
	case checker.StrIsEmpty:
		return &ast.BinaryExpr{X: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{subject}}, Op: token.EQL, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}}, true, nil
	case checker.StrContains:
		a, ok, err := arg(0)
		if err != nil || !ok {
			return nil, ok, err
		}
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent("strings"), "Contains"), Args: []ast.Expr{subject, a}}, true, nil
	case checker.StrReplace:
		oldV, ok, err := arg(0)
		if err != nil || !ok {
			return nil, ok, err
		}
		newV, ok, err := arg(1)
		if err != nil || !ok {
			return nil, ok, err
		}
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent("strings"), "Replace"), Args: []ast.Expr{subject, oldV, newV, &ast.BasicLit{Kind: token.INT, Value: "1"}}}, true, nil
	case checker.StrReplaceAll:
		oldV, ok, err := arg(0)
		if err != nil || !ok {
			return nil, ok, err
		}
		newV, ok, err := arg(1)
		if err != nil || !ok {
			return nil, ok, err
		}
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent("strings"), "ReplaceAll"), Args: []ast.Expr{subject, oldV, newV}}, true, nil
	case checker.StrSplit:
		a, ok, err := arg(0)
		if err != nil || !ok {
			return nil, ok, err
		}
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent("strings"), "Split"), Args: []ast.Expr{subject, a}}, true, nil
	case checker.StrStartsWith:
		a, ok, err := arg(0)
		if err != nil || !ok {
			return nil, ok, err
		}
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent("strings"), "HasPrefix"), Args: []ast.Expr{subject, a}}, true, nil
	case checker.StrToStr:
		return subject, true, nil
	case checker.StrTrim:
		return &ast.CallExpr{Fun: selectorExpr(ast.NewIdent("strings"), "TrimSpace"), Args: []ast.Expr{subject}}, true, nil
	default:
		return nil, false, nil
	}
}

func (e *emitter) lowerAssignmentTargetAST(expr checker.Expression) (ast.Expr, bool, error) {
	switch target := expr.(type) {
	case *checker.Identifier:
		return e.lowerLocalTargetExpr(target.Name), true, nil
	case checker.Variable:
		return e.lowerLocalTargetExpr(target.Name()), true, nil
	case *checker.Variable:
		return e.lowerLocalTargetExpr(target.Name()), true, nil
	case *checker.InstanceProperty:
		subject, ok, err := e.lowerAssignmentTargetAST(target.Subject)
		if err != nil || !ok {
			subject, ok, err = e.lowerExprAST(target.Subject)
			if err != nil || !ok {
				return nil, ok, err
			}
		}
		return selectorExpr(subject, goName(target.Property, true)), true, nil
	default:
		return nil, false, nil
	}
}
