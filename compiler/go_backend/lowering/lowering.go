package lowering

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/akonwi/ard/checker"
	backendir "github.com/akonwi/ard/go_backend/ir"
)

func LowerModuleToBackendIR(module checker.Module, packageName string, entrypoint bool, projectName string) (*backendir.Module, error) {
	return lowerModuleToBackendIR(module, packageName, entrypoint, projectName)
}

func LowerExpressionToBackendIR(expr checker.Expression) backendir.Expr {
	return lowerExpressionToBackendIR(expr)
}

func LowerCheckerTypeToBackendIR(t checker.Type) backendir.Type {
	return lowerCheckerTypeToBackendIR(t)
}

func LowerFunctionDeclToBackendIR(def *checker.FunctionDef) backendir.Decl {
	return lowerFunctionDeclToBackendIR(def)
}

func LowerUnionDeclToBackendIR(def *checker.Union) backendir.Decl {
	return lowerUnionDeclToBackendIR(def)
}

func LowerExternTypeDeclToBackendIR(def *checker.ExternType) backendir.Decl {
	return lowerExternTypeDeclToBackendIR(def)
}

func LowerNonProducingToBackendIR(node checker.NonProducing) []backendir.Stmt {
	return lowerNonProducingToBackendIR(node)
}

func IsVoidIRType(t backendir.Type) bool {
	return isVoidIRType(t)
}

func MatchSubjectTempName(kind string) string {
	return matchSubjectTempName(kind)
}

// topLevelExecutableStatements returns the subset of top-level checker
// statements that participate in the executable entrypoint stream. All
// declaration-only forms are excluded so the entrypoint executable block only
// contains genuinely executable semantics.
func topLevelExecutableStatements(stmts []checker.Statement) []checker.Statement {
	filtered := make([]checker.Statement, 0, len(stmts))
	for _, stmt := range stmts {
		switch stmt.Expr.(type) {
		case *checker.FunctionDef, *checker.ExternalFunctionDef:
			continue
		}
		switch stmt.Stmt.(type) {
		case *checker.StructDef, checker.StructDef,
			*checker.Enum, checker.Enum,
			*checker.Union, checker.Union,
			*checker.ExternType:
			continue
		}
		filtered = append(filtered, stmt)
	}
	return filtered
}

func sortedStringKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func effectiveFunctionReturnType(def *checker.FunctionDef) checker.Type {
	if def == nil {
		return checker.Void
	}
	if def.InferReturnTypeFromBody && def.Body != nil && def.Body.Type() != checker.Void {
		return def.Body.Type()
	}
	return def.ReturnType
}

func typeVarName(tv *checker.TypeVar) string {
	if tv == nil {
		return ""
	}
	return tv.Name()
}

func collectUnboundTypeParamNames(t checker.Type, out *[]string, seen map[string]struct{}) {
	if t == nil {
		return
	}
	switch typed := t.(type) {
	case *checker.TypeVar:
		if typed.Actual() != nil {
			return
		}
		name := typeVarName(typed)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		*out = append(*out, name)
	case *checker.List:
		collectUnboundTypeParamNames(typed.Of(), out, seen)
	case *checker.Map:
		collectUnboundTypeParamNames(typed.Key(), out, seen)
		collectUnboundTypeParamNames(typed.Value(), out, seen)
	case *checker.Maybe:
		collectUnboundTypeParamNames(typed.Of(), out, seen)
	case *checker.Result:
		collectUnboundTypeParamNames(typed.Val(), out, seen)
		collectUnboundTypeParamNames(typed.Err(), out, seen)
	case *checker.Union:
		for _, member := range typed.Types {
			collectUnboundTypeParamNames(member, out, seen)
		}
	case *checker.StructDef:
		for _, fieldName := range sortedStringKeys(typed.Fields) {
			collectUnboundTypeParamNames(typed.Fields[fieldName], out, seen)
		}
	case *checker.ExternType:
		for _, typeArg := range typed.TypeArgs {
			collectUnboundTypeParamNames(typeArg, out, seen)
		}
	case *checker.FunctionDef:
		for _, param := range typed.Parameters {
			collectUnboundTypeParamNames(param.Type, out, seen)
		}
		collectUnboundTypeParamNames(effectiveFunctionReturnType(typed), out, seen)
	}
}

func collectTypeParamNames(t checker.Type, out *[]string, seen map[string]struct{}) {
	if t == nil {
		return
	}
	switch typed := t.(type) {
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			collectTypeParamNames(actual, out, seen)
			return
		}
		name := typeVarName(typed)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		*out = append(*out, name)
	case *checker.List:
		collectTypeParamNames(typed.Of(), out, seen)
	case *checker.Map:
		collectTypeParamNames(typed.Key(), out, seen)
		collectTypeParamNames(typed.Value(), out, seen)
	case *checker.Maybe:
		collectTypeParamNames(typed.Of(), out, seen)
	case *checker.Result:
		collectTypeParamNames(typed.Val(), out, seen)
		collectTypeParamNames(typed.Err(), out, seen)
	case *checker.Union:
		for _, member := range typed.Types {
			collectTypeParamNames(member, out, seen)
		}
	case *checker.StructDef:
		for _, fieldName := range sortedStringKeys(typed.Fields) {
			collectTypeParamNames(typed.Fields[fieldName], out, seen)
		}
	case *checker.ExternType:
		for _, typeArg := range typed.TypeArgs {
			collectTypeParamNames(typeArg, out, seen)
		}
	case *checker.FunctionDef:
		for _, param := range typed.Parameters {
			collectTypeParamNames(param.Type, out, seen)
		}
		collectTypeParamNames(effectiveFunctionReturnType(typed), out, seen)
	}
}

func collectBoundTypeVarBindings(t checker.Type, bindings map[string]checker.Type) {
	switch typed := t.(type) {
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			bindings[typeVarName(typed)] = actual
			collectBoundTypeVarBindings(actual, bindings)
		}
	case *checker.List:
		collectBoundTypeVarBindings(typed.Of(), bindings)
	case *checker.Map:
		collectBoundTypeVarBindings(typed.Key(), bindings)
		collectBoundTypeVarBindings(typed.Value(), bindings)
	case *checker.Maybe:
		collectBoundTypeVarBindings(typed.Of(), bindings)
	case *checker.Result:
		collectBoundTypeVarBindings(typed.Val(), bindings)
		collectBoundTypeVarBindings(typed.Err(), bindings)
	case *checker.FunctionDef:
		for _, param := range typed.Parameters {
			collectBoundTypeVarBindings(param.Type, bindings)
		}
		collectBoundTypeVarBindings(typed.ReturnType, bindings)
	case *checker.Union:
		for _, member := range typed.Types {
			collectBoundTypeVarBindings(member, bindings)
		}
	}
}

func structTypeParamOrder(def *checker.StructDef) []string {
	if def == nil {
		return nil
	}
	if len(def.GenericParams) > 0 {
		return append([]string(nil), def.GenericParams...)
	}
	seen := make(map[string]struct{})
	order := make([]string, 0)
	for _, fieldName := range sortedStringKeys(def.Fields) {
		collectTypeParamNames(def.Fields[fieldName], &order, seen)
	}
	return order
}

func inferStructBoundTypeArgs(def *checker.StructDef, order []string, existing map[string]checker.Type) map[string]checker.Type {
	bindings := make(map[string]checker.Type, len(order))
	allowed := make(map[string]struct{}, len(order))
	for _, name := range order {
		allowed[name] = struct{}{}
	}
	for name, bound := range existing {
		if _, ok := allowed[name]; ok && bound != nil {
			bindings[name] = bound
		}
	}
	for _, fieldName := range sortedStringKeys(def.Fields) {
		collectBoundTypeVarBindings(def.Fields[fieldName], bindings)
	}
	for _, methodName := range sortedStringKeys(def.Methods) {
		method := def.Methods[methodName]
		for _, param := range method.Parameters {
			collectBoundTypeVarBindings(param.Type, bindings)
		}
		collectBoundTypeVarBindings(method.ReturnType, bindings)
	}
	for name := range bindings {
		if _, ok := allowed[name]; !ok {
			delete(bindings, name)
		}
	}
	return bindings
}

func withElseFallback(expr *checker.If, fallback *checker.Block) *checker.If {
	if expr == nil {
		return nil
	}
	copy := *expr
	if copy.ElseIf != nil {
		copy.ElseIf = withElseFallback(copy.ElseIf, fallback)
		return &copy
	}
	if copy.Else == nil {
		copy.Else = fallback
	}
	return &copy
}

func usesNameInStatements(stmts []checker.Statement, name string) bool {
	for _, stmt := range stmts {
		if variableDef, ok := stmt.Stmt.(*checker.VariableDef); ok && variableDef.Name == name {
			if usesNameInExpr(variableDef.Value, name) {
				return true
			}
			return false
		}
		if usesNameInStatement(stmt, name) {
			return true
		}
	}
	return false
}

func usesNameInStatement(stmt checker.Statement, name string) bool {
	if stmt.Expr != nil && usesNameInExpr(stmt.Expr, name) {
		return true
	}
	if stmt.Stmt != nil && usesNameInNonProducing(stmt.Stmt, name) {
		return true
	}
	return false
}

func usesNameInNonProducing(stmt checker.NonProducing, name string) bool {
	switch s := stmt.(type) {
	case *checker.VariableDef:
		return usesNameInExpr(s.Value, name)
	case *checker.Reassignment:
		return usesNameInExpr(s.Target, name) || usesNameInExpr(s.Value, name)
	case *checker.WhileLoop:
		return usesNameInExpr(s.Condition, name) || usesNameInStatements(s.Body.Stmts, name)
	case *checker.ForLoop:
		return usesNameInExpr(s.Init.Value, name) || usesNameInExpr(s.Condition, name) || usesNameInExpr(s.Update.Value, name) || usesNameInStatements(s.Body.Stmts, name)
	case *checker.ForIntRange:
		return usesNameInExpr(s.Start, name) || usesNameInExpr(s.End, name) || usesNameInStatements(s.Body.Stmts, name)
	case *checker.ForInStr:
		return usesNameInExpr(s.Value, name) || usesNameInStatements(s.Body.Stmts, name)
	case *checker.ForInList:
		return usesNameInExpr(s.List, name) || usesNameInStatements(s.Body.Stmts, name)
	case *checker.ForInMap:
		return usesNameInExpr(s.Map, name) || usesNameInStatements(s.Body.Stmts, name)
	default:
		return false
	}
}

func usesNameInExpr(expr checker.Expression, name string) bool {
	switch v := expr.(type) {
	case nil:
		return false
	case *checker.Identifier:
		return v.Name == name
	case checker.Variable:
		return v.Name() == name
	case *checker.Variable:
		return v.Name() == name
	case *checker.IntLiteral, *checker.FloatLiteral, *checker.StrLiteral, *checker.BoolLiteral, *checker.VoidLiteral:
		return false
	case *checker.IntAddition:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntSubtraction:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntMultiplication:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntDivision:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntModulo:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatAddition:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatSubtraction:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatMultiplication:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatDivision:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.StrAddition:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntGreater:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntGreaterEqual:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntLess:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntLessEqual:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatGreater:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatGreaterEqual:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatLess:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatLessEqual:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.Equality:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.And:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.Or:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.Negation:
		return usesNameInExpr(v.Value, name)
	case *checker.Not:
		return usesNameInExpr(v.Value, name)
	case *checker.FunctionCall:
		for _, arg := range v.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.CopyExpression:
		return usesNameInExpr(v.Expr, name)
	case *checker.ListLiteral:
		for _, element := range v.Elements {
			if usesNameInExpr(element, name) {
				return true
			}
		}
		return false
	case *checker.ListMethod:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, arg := range v.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.MapLiteral:
		for i := range v.Keys {
			if usesNameInExpr(v.Keys[i], name) || usesNameInExpr(v.Values[i], name) {
				return true
			}
		}
		return false
	case *checker.MapMethod:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, arg := range v.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.ResultMethod:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, arg := range v.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.TryOp:
		if usesNameInExpr(v.Expr(), name) {
			return true
		}
		if v.CatchBlock == nil {
			return false
		}
		if v.CatchVar != "" && v.CatchVar == name {
			return false
		}
		return usesNameInStatements(v.CatchBlock.Stmts, name)
	case *checker.MaybeMethod:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, arg := range v.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.TemplateStr:
		for _, chunk := range v.Chunks {
			if usesNameInExpr(chunk, name) {
				return true
			}
		}
		return false
	case *checker.StrMethod:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, arg := range v.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.IntMethod:
		return usesNameInExpr(v.Subject, name)
	case *checker.FloatMethod:
		return usesNameInExpr(v.Subject, name)
	case *checker.BoolMethod:
		return usesNameInExpr(v.Subject, name)
	case *checker.StructInstance:
		for _, fieldName := range sortedStringKeys(v.Fields) {
			if usesNameInExpr(v.Fields[fieldName], name) {
				return true
			}
		}
		return false
	case *checker.InstanceProperty:
		return usesNameInExpr(v.Subject, name)
	case *checker.InstanceMethod:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, arg := range v.Method.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.ModuleStructInstance:
		for _, fieldName := range sortedStringKeys(v.Property.Fields) {
			if usesNameInExpr(v.Property.Fields[fieldName], name) {
				return true
			}
		}
		return false
	case *checker.ModuleFunctionCall:
		for _, arg := range v.Call.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.If:
		if usesNameInExpr(v.Condition, name) || usesNameInStatements(v.Body.Stmts, name) {
			return true
		}
		if v.ElseIf != nil && usesNameInExpr(v.ElseIf, name) {
			return true
		}
		if v.Else != nil && usesNameInStatements(v.Else.Stmts, name) {
			return true
		}
		return false
	case *checker.BoolMatch:
		return usesNameInExpr(v.Subject, name) || usesNameInStatements(v.True.Stmts, name) || usesNameInStatements(v.False.Stmts, name)
	case *checker.IntMatch:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, block := range v.IntCases {
			if usesNameInStatements(block.Stmts, name) {
				return true
			}
		}
		for _, block := range v.RangeCases {
			if usesNameInStatements(block.Stmts, name) {
				return true
			}
		}
		if v.CatchAll != nil && usesNameInStatements(v.CatchAll.Stmts, name) {
			return true
		}
		return false
	case *checker.EnumMatch:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, block := range v.Cases {
			if block != nil && usesNameInStatements(block.Stmts, name) {
				return true
			}
		}
		if v.CatchAll != nil && usesNameInStatements(v.CatchAll.Stmts, name) {
			return true
		}
		return false
	case *checker.OptionMatch:
		return usesNameInExpr(v.Subject, name) || usesNameInStatements(v.Some.Body.Stmts, name) || usesNameInStatements(v.None.Stmts, name)
	case *checker.ResultMatch:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		okUses := false
		if v.Ok != nil {
			if v.Ok.Pattern != nil && v.Ok.Pattern.Name == name {
				okUses = false
			} else {
				okUses = usesNameInStatements(v.Ok.Body.Stmts, name)
			}
		}
		errUses := false
		if v.Err != nil {
			if v.Err.Pattern != nil && v.Err.Pattern.Name == name {
				errUses = false
			} else {
				errUses = usesNameInStatements(v.Err.Body.Stmts, name)
			}
		}
		return okUses || errUses
	case *checker.ConditionalMatch:
		for _, matchCase := range v.Cases {
			if usesNameInExpr(matchCase.Condition, name) || usesNameInStatements(matchCase.Body.Stmts, name) {
				return true
			}
		}
		if v.CatchAll != nil {
			return usesNameInStatements(v.CatchAll.Stmts, name)
		}
		return false
	case *checker.UnionMatch:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, matchCase := range v.TypeCases {
			if matchCase == nil {
				continue
			}
			if matchCase.Pattern != nil && matchCase.Pattern.Name == name {
				continue
			}
			if usesNameInStatements(matchCase.Body.Stmts, name) {
				return true
			}
		}
		if v.CatchAll != nil {
			return usesNameInStatements(v.CatchAll.Stmts, name)
		}
		return false
	case *checker.FunctionDef:
		for _, param := range v.Parameters {
			if param.Name == name {
				return false
			}
		}
		if v.Body != nil {
			return usesNameInStatements(v.Body.Stmts, name)
		}
		return false
	case *checker.FiberStart:
		if v.GetFn() == nil {
			return false
		}
		return usesNameInExpr(v.GetFn(), name)
	case *checker.FiberEval:
		if v.GetFn() == nil {
			return false
		}
		return usesNameInExpr(v.GetFn(), name)
	case *checker.FiberExecution:
		return false
	default:
		return false
	}
}

func lowerModuleToBackendIR(module checker.Module, packageName string, entrypoint bool, projectName string) (*backendir.Module, error) {
	if module == nil || module.Program() == nil {
		return nil, fmt.Errorf("module has no program")
	}

	out := &backendir.Module{
		Path:                module.Path(),
		PackageName:         packageName,
		Imports:             collectModuleImports(module, projectName),
		ImportedModulePaths: requiredImportedModulePaths(module, projectName),
		Decls:               make([]backendir.Decl, 0, len(module.Program().Statements)),
	}
	if entrypoint {
		out.Imports[helperImportPath] = helperImportAlias
	}
	seenDecls := make(map[string]struct{})

	for _, stmt := range module.Program().Statements {
		if stmt.Stmt != nil {
			switch def := stmt.Stmt.(type) {
			case *checker.StructDef:
				key := "struct:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				out.Decls = append(out.Decls, lowerStructDeclToBackendIR(def))
			case checker.StructDef:
				key := "struct:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				defCopy := def
				out.Decls = append(out.Decls, lowerStructDeclToBackendIR(&defCopy))
			case *checker.Enum:
				key := "enum:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				out.Decls = append(out.Decls, lowerEnumDeclToBackendIR(def))
			case checker.Enum:
				key := "enum:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				defCopy := def
				out.Decls = append(out.Decls, lowerEnumDeclToBackendIR(&defCopy))
			case *checker.Union:
				key := "union:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				out.Decls = append(out.Decls, lowerUnionDeclToBackendIR(def))
			case checker.Union:
				key := "union:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				defCopy := def
				out.Decls = append(out.Decls, lowerUnionDeclToBackendIR(&defCopy))
			case *checker.ExternType:
				key := "extern_type:" + def.Name_
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				out.Decls = append(out.Decls, lowerExternTypeDeclToBackendIR(def))
			case *checker.VariableDef:
				if entrypoint {
					break
				}
				key := "var:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				out.Decls = append(out.Decls, lowerVariableDeclToBackendIR(def))
			}
		}

		if stmt.Expr != nil {
			switch def := stmt.Expr.(type) {
			case *checker.FunctionDef:
				if def.Receiver != "" || def.IsTest {
					continue
				}
				out.Decls = append(out.Decls, lowerFunctionDeclToBackendIR(def))
			case *checker.ExternalFunctionDef:
				out.Decls = append(out.Decls, lowerExternFunctionDeclToBackendIR(def))
			}
		}
	}

	// The checker keeps certain type-declared identifiers in scope
	// without emitting a Statement entry for them. Two cases matter:
	//
	//   1. Enum declarations without an associated `impl` block.
	//   2. Union/type-alias declarations (`type X = A | B`), which the
	//      checker registers in scope but never returns as a Statement.
	//
	// To support native backend IR emission of signatures that reference
	// these types, we still need to surface them as IR declarations so
	// the Go emitter generates the corresponding type definitions
	// (`type X struct { Tag int }` for enums, `type X interface {}` for
	// unions). Walk type references in the program and synthesize the
	// missing declarations for any orphan enum or union we have not yet
	// seen.
	collectOrphanTypeDecls(module.Program(), out, seenDecls)

	out.Entrypoint = lowerEntrypointStatementsToBackendIRBlock(topLevelExecutableStatements(module.Program().Statements))
	rewriteImportedAnonFunctionCalls(out, module)
	qualifyImportedNamedTypes(out, module)
	return out, nil
}

func rewriteImportedAnonFunctionCalls(module *backendir.Module, source checker.Module) {
	if module == nil || source == nil || source.Program() == nil {
		return
	}
	bindings := importedAnonFunctionBindings(source)
	if len(bindings) == 0 {
		return
	}
	for _, decl := range module.Decls {
		rewriteImportedAnonFunctionCallsInDecl(decl, bindings)
	}
	rewriteImportedAnonFunctionCallsInBlock(module.Entrypoint, bindings)
}

func importedAnonFunctionBindings(source checker.Module) map[string]map[string]string {
	bindings := make(map[string]map[string]string)
	if source == nil || source.Program() == nil {
		return bindings
	}
	for _, imported := range source.Program().Imports {
		if imported == nil || imported.Program() == nil {
			continue
		}
		path := strings.TrimSpace(imported.Path())
		if path == "" {
			continue
		}
		for _, stmt := range imported.Program().Statements {
			variableDef, ok := stmt.Stmt.(*checker.VariableDef)
			if !ok || variableDef == nil || variableDef.Value == nil {
				continue
			}
			functionDef, ok := variableDef.Value.(*checker.FunctionDef)
			if !ok || functionDef == nil || strings.TrimSpace(functionDef.Name) == "" {
				continue
			}
			if bindings[path] == nil {
				bindings[path] = make(map[string]string)
			}
			bindings[path][functionDef.Name] = variableDef.Name
		}
	}
	return bindings
}

func rewriteImportedAnonFunctionCallsInDecl(decl backendir.Decl, bindings map[string]map[string]string) {
	switch d := decl.(type) {
	case *backendir.FuncDecl:
		rewriteImportedAnonFunctionCallsInBlock(d.Body, bindings)
	case *backendir.VarDecl:
		rewriteImportedAnonFunctionCallsInExpr(d.Value, bindings)
	case *backendir.StructDecl:
		for _, method := range d.Methods {
			rewriteImportedAnonFunctionCallsInBlock(method.Body, bindings)
		}
	case *backendir.EnumDecl:
		for _, method := range d.Methods {
			rewriteImportedAnonFunctionCallsInBlock(method.Body, bindings)
		}
	}
}

func rewriteImportedAnonFunctionCallsInBlock(block *backendir.Block, bindings map[string]map[string]string) {
	if block == nil {
		return
	}
	for _, stmt := range block.Stmts {
		rewriteImportedAnonFunctionCallsInStmt(stmt, bindings)
	}
}

func rewriteImportedAnonFunctionCallsInStmt(stmt backendir.Stmt, bindings map[string]map[string]string) {
	switch s := stmt.(type) {
	case *backendir.ReturnStmt:
		rewriteImportedAnonFunctionCallsInExpr(s.Value, bindings)
	case *backendir.ExprStmt:
		rewriteImportedAnonFunctionCallsInExpr(s.Value, bindings)
	case *backendir.AssignStmt:
		rewriteImportedAnonFunctionCallsInExpr(s.Value, bindings)
	case *backendir.BindStmt:
		rewriteImportedAnonFunctionCallsInExpr(s.Value, bindings)
	case *backendir.MemberAssignStmt:
		rewriteImportedAnonFunctionCallsInExpr(s.Subject, bindings)
		rewriteImportedAnonFunctionCallsInExpr(s.Value, bindings)
	case *backendir.ForIntRangeStmt:
		rewriteImportedAnonFunctionCallsInExpr(s.Start, bindings)
		rewriteImportedAnonFunctionCallsInExpr(s.End, bindings)
		rewriteImportedAnonFunctionCallsInBlock(s.Body, bindings)
	case *backendir.ForLoopStmt:
		rewriteImportedAnonFunctionCallsInExpr(s.InitValue, bindings)
		rewriteImportedAnonFunctionCallsInExpr(s.Cond, bindings)
		rewriteImportedAnonFunctionCallsInStmt(s.Update, bindings)
		rewriteImportedAnonFunctionCallsInBlock(s.Body, bindings)
	case *backendir.ForInStrStmt:
		rewriteImportedAnonFunctionCallsInExpr(s.Value, bindings)
		rewriteImportedAnonFunctionCallsInBlock(s.Body, bindings)
	case *backendir.ForInListStmt:
		rewriteImportedAnonFunctionCallsInExpr(s.List, bindings)
		rewriteImportedAnonFunctionCallsInBlock(s.Body, bindings)
	case *backendir.ForInMapStmt:
		rewriteImportedAnonFunctionCallsInExpr(s.Map, bindings)
		rewriteImportedAnonFunctionCallsInBlock(s.Body, bindings)
	case *backendir.WhileStmt:
		rewriteImportedAnonFunctionCallsInExpr(s.Cond, bindings)
		rewriteImportedAnonFunctionCallsInBlock(s.Body, bindings)
	case *backendir.IfStmt:
		rewriteImportedAnonFunctionCallsInExpr(s.Cond, bindings)
		rewriteImportedAnonFunctionCallsInBlock(s.Then, bindings)
		rewriteImportedAnonFunctionCallsInBlock(s.Else, bindings)
	}
}

func rewriteImportedAnonFunctionCallsInExpr(expr backendir.Expr, bindings map[string]map[string]string) {
	switch e := expr.(type) {
	case *backendir.SelectorExpr:
		if subject, ok := e.Subject.(*backendir.IdentExpr); ok {
			if moduleBindings := bindings[strings.TrimSpace(subject.Name)]; len(moduleBindings) > 0 {
				if name := strings.TrimSpace(moduleBindings[strings.TrimSpace(e.Name)]); name != "" {
					e.Name = name
				}
			}
		}
		rewriteImportedAnonFunctionCallsInExpr(e.Subject, bindings)
	case *backendir.CallExpr:
		rewriteImportedAnonFunctionCallsInExpr(e.Callee, bindings)
		for _, arg := range e.Args {
			rewriteImportedAnonFunctionCallsInExpr(arg, bindings)
		}
	case *backendir.TraitCoerceExpr:
		rewriteImportedAnonFunctionCallsInExpr(e.Value, bindings)
	case *backendir.MaybeSomeExpr:
		rewriteImportedAnonFunctionCallsInExpr(e.Value, bindings)
	case *backendir.ResultOkExpr:
		rewriteImportedAnonFunctionCallsInExpr(e.Value, bindings)
	case *backendir.ResultErrExpr:
		rewriteImportedAnonFunctionCallsInExpr(e.Value, bindings)
	case *backendir.AddressOfExpr:
		rewriteImportedAnonFunctionCallsInExpr(e.Value, bindings)
	case *backendir.FuncLiteralExpr:
		rewriteImportedAnonFunctionCallsInBlock(e.Body, bindings)
	case *backendir.ListLiteralExpr:
		for _, element := range e.Elements {
			rewriteImportedAnonFunctionCallsInExpr(element, bindings)
		}
	case *backendir.MapLiteralExpr:
		for _, entry := range e.Entries {
			rewriteImportedAnonFunctionCallsInExpr(entry.Key, bindings)
			rewriteImportedAnonFunctionCallsInExpr(entry.Value, bindings)
		}
	case *backendir.StructLiteralExpr:
		for _, field := range e.Fields {
			rewriteImportedAnonFunctionCallsInExpr(field.Value, bindings)
		}
	case *backendir.IfExpr:
		rewriteImportedAnonFunctionCallsInExpr(e.Cond, bindings)
		rewriteImportedAnonFunctionCallsInBlock(e.Then, bindings)
		rewriteImportedAnonFunctionCallsInBlock(e.Else, bindings)
	case *backendir.UnionMatchExpr:
		rewriteImportedAnonFunctionCallsInExpr(e.Subject, bindings)
		for i := range e.Cases {
			rewriteImportedAnonFunctionCallsInBlock(e.Cases[i].Body, bindings)
		}
		rewriteImportedAnonFunctionCallsInBlock(e.CatchAll, bindings)
	case *backendir.TryExpr:
		rewriteImportedAnonFunctionCallsInExpr(e.Subject, bindings)
		rewriteImportedAnonFunctionCallsInBlock(e.Catch, bindings)
	case *backendir.PanicExpr:
		rewriteImportedAnonFunctionCallsInExpr(e.Message, bindings)
	case *backendir.CopyExpr:
		rewriteImportedAnonFunctionCallsInExpr(e.Value, bindings)
	case *backendir.BlockExpr:
		for _, setup := range e.Setup {
			rewriteImportedAnonFunctionCallsInStmt(setup, bindings)
		}
		rewriteImportedAnonFunctionCallsInExpr(e.Value, bindings)
	}
}

func qualifyImportedNamedTypes(module *backendir.Module, source checker.Module) {
	if module == nil || source == nil || source.Program() == nil {
		return
	}
	owners := importedTypeOwners(source)
	if len(owners) == 0 {
		return
	}
	local := make(map[string]struct{})
	for _, decl := range module.Decls {
		switch d := decl.(type) {
		case *backendir.StructDecl:
			local[d.Name] = struct{}{}
		case *backendir.EnumDecl:
			local[d.Name] = struct{}{}
		case *backendir.UnionDecl:
			local[d.Name] = struct{}{}
		case *backendir.ExternTypeDecl:
			local[d.Name] = struct{}{}
		}
	}
	for _, decl := range module.Decls {
		qualifyImportedNamedTypesInDecl(decl, owners, local)
	}
	qualifyImportedNamedTypesInBlock(module.Entrypoint, owners, local)
}

func importedTypeOwners(module checker.Module) map[string]string {
	owners := make(map[string]string)
	if module == nil || module.Program() == nil {
		return owners
	}
	for _, imported := range module.Program().Imports {
		if imported == nil || imported.Program() == nil {
			continue
		}
		path := strings.TrimSpace(imported.Path())
		for _, stmt := range imported.Program().Statements {
			if stmt.Stmt != nil {
				collectImportedTypeOwnersFromNonProducing(stmt.Stmt, path, owners)
			}
			if stmt.Expr != nil {
				collectImportedTypeOwnersFromExpr(stmt.Expr, path, owners)
			}
		}
	}
	return owners
}

func collectImportedTypeOwnersFromNonProducing(stmt checker.NonProducing, modulePath string, owners map[string]string) {
	switch def := stmt.(type) {
	case *checker.StructDef:
		owners[def.Name] = modulePath
		for _, typ := range def.Fields {
			collectImportedTypeOwnersFromType(typ, modulePath, owners)
		}
	case checker.StructDef:
		owners[def.Name] = modulePath
		for _, typ := range def.Fields {
			collectImportedTypeOwnersFromType(typ, modulePath, owners)
		}
	case *checker.Union:
		owners[def.Name] = modulePath
		for _, typ := range def.Types {
			collectImportedTypeOwnersFromType(typ, modulePath, owners)
		}
	case checker.Union:
		owners[def.Name] = modulePath
		for _, typ := range def.Types {
			collectImportedTypeOwnersFromType(typ, modulePath, owners)
		}
	case *checker.ExternType:
		owners[def.Name_] = modulePath
	case *checker.VariableDef:
		collectImportedTypeOwnersFromType(def.Type(), modulePath, owners)
	}
}

func collectImportedTypeOwnersFromExpr(expr checker.Expression, modulePath string, owners map[string]string) {
	switch def := expr.(type) {
	case *checker.FunctionDef:
		for _, param := range def.Parameters {
			collectImportedTypeOwnersFromType(param.Type, modulePath, owners)
		}
		collectImportedTypeOwnersFromType(effectiveFunctionReturnType(def), modulePath, owners)
	case *checker.ExternalFunctionDef:
		for _, param := range def.Parameters {
			collectImportedTypeOwnersFromType(param.Type, modulePath, owners)
		}
		collectImportedTypeOwnersFromType(def.ReturnType, modulePath, owners)
	default:
		collectImportedTypeOwnersFromType(expr.Type(), modulePath, owners)
	}
}

func collectImportedTypeOwnersFromType(t checker.Type, modulePath string, owners map[string]string) {
	switch typed := t.(type) {
	case *checker.StructDef:
		owners[typed.Name] = modulePath
		for _, fieldType := range typed.Fields {
			collectImportedTypeOwnersFromType(fieldType, modulePath, owners)
		}
	case *checker.Enum:
		owners[typed.Name] = modulePath
	case *checker.Union:
		owners[typed.Name] = modulePath
		for _, member := range typed.Types {
			collectImportedTypeOwnersFromType(member, modulePath, owners)
		}
	case checker.Union:
		owners[typed.Name] = modulePath
		for _, member := range typed.Types {
			collectImportedTypeOwnersFromType(member, modulePath, owners)
		}
	case *checker.ExternType:
		owners[typed.Name_] = modulePath
		for _, typeArg := range typed.TypeArgs {
			collectImportedTypeOwnersFromType(typeArg, modulePath, owners)
		}
	case *checker.List:
		collectImportedTypeOwnersFromType(typed.Of(), modulePath, owners)
	case *checker.Map:
		collectImportedTypeOwnersFromType(typed.Key(), modulePath, owners)
		collectImportedTypeOwnersFromType(typed.Value(), modulePath, owners)
	case *checker.Maybe:
		collectImportedTypeOwnersFromType(typed.Of(), modulePath, owners)
	case *checker.Result:
		collectImportedTypeOwnersFromType(typed.Val(), modulePath, owners)
		collectImportedTypeOwnersFromType(typed.Err(), modulePath, owners)
	case *checker.FunctionDef:
		for _, param := range typed.Parameters {
			collectImportedTypeOwnersFromType(param.Type, modulePath, owners)
		}
		collectImportedTypeOwnersFromType(effectiveFunctionReturnType(typed), modulePath, owners)
	}
}

func qualifyImportedNamedTypesInDecl(decl backendir.Decl, owners map[string]string, local map[string]struct{}) {
	switch d := decl.(type) {
	case *backendir.StructDecl:
		for i := range d.Fields {
			d.Fields[i].Type = qualifyImportedNamedType(d.Fields[i].Type, owners, local)
		}
		for _, method := range d.Methods {
			qualifyImportedNamedTypesInDecl(method, owners, local)
		}
	case *backendir.EnumDecl:
		for _, method := range d.Methods {
			qualifyImportedNamedTypesInDecl(method, owners, local)
		}
	case *backendir.UnionDecl:
		for i := range d.Types {
			d.Types[i] = qualifyImportedNamedType(d.Types[i], owners, local)
		}
	case *backendir.ExternTypeDecl:
		for i := range d.Args {
			d.Args[i] = qualifyImportedNamedType(d.Args[i], owners, local)
		}
	case *backendir.FuncDecl:
		for i := range d.Params {
			d.Params[i].Type = qualifyImportedNamedType(d.Params[i].Type, owners, local)
		}
		d.Return = qualifyImportedNamedType(d.Return, owners, local)
		qualifyImportedNamedTypesInBlock(d.Body, owners, local)
	case *backendir.VarDecl:
		d.Type = qualifyImportedNamedType(d.Type, owners, local)
		qualifyImportedNamedTypesInExpr(d.Value, owners, local)
	}
}

func qualifyImportedNamedTypesInBlock(block *backendir.Block, owners map[string]string, local map[string]struct{}) {
	if block == nil {
		return
	}
	for _, stmt := range block.Stmts {
		qualifyImportedNamedTypesInStmt(stmt, owners, local)
	}
}

func qualifyImportedNamedTypesInStmt(stmt backendir.Stmt, owners map[string]string, local map[string]struct{}) {
	switch s := stmt.(type) {
	case *backendir.ReturnStmt:
		qualifyImportedNamedTypesInExpr(s.Value, owners, local)
	case *backendir.ExprStmt:
		qualifyImportedNamedTypesInExpr(s.Value, owners, local)
	case *backendir.AssignStmt:
		qualifyImportedNamedTypesInExpr(s.Value, owners, local)
	case *backendir.BindStmt:
		qualifyImportedNamedTypesInExpr(s.Value, owners, local)
	case *backendir.MemberAssignStmt:
		qualifyImportedNamedTypesInExpr(s.Subject, owners, local)
		qualifyImportedNamedTypesInExpr(s.Value, owners, local)
	case *backendir.ForIntRangeStmt:
		qualifyImportedNamedTypesInExpr(s.Start, owners, local)
		qualifyImportedNamedTypesInExpr(s.End, owners, local)
		qualifyImportedNamedTypesInBlock(s.Body, owners, local)
	case *backendir.ForLoopStmt:
		qualifyImportedNamedTypesInExpr(s.InitValue, owners, local)
		qualifyImportedNamedTypesInExpr(s.Cond, owners, local)
		qualifyImportedNamedTypesInStmt(s.Update, owners, local)
		qualifyImportedNamedTypesInBlock(s.Body, owners, local)
	case *backendir.ForInStrStmt:
		qualifyImportedNamedTypesInExpr(s.Value, owners, local)
		qualifyImportedNamedTypesInBlock(s.Body, owners, local)
	case *backendir.ForInListStmt:
		s.CursorType = qualifyImportedNamedType(s.CursorType, owners, local)
		qualifyImportedNamedTypesInExpr(s.List, owners, local)
		qualifyImportedNamedTypesInBlock(s.Body, owners, local)
	case *backendir.ForInMapStmt:
		qualifyImportedNamedTypesInExpr(s.Map, owners, local)
		qualifyImportedNamedTypesInBlock(s.Body, owners, local)
	case *backendir.WhileStmt:
		qualifyImportedNamedTypesInExpr(s.Cond, owners, local)
		qualifyImportedNamedTypesInBlock(s.Body, owners, local)
	case *backendir.IfStmt:
		qualifyImportedNamedTypesInExpr(s.Cond, owners, local)
		qualifyImportedNamedTypesInBlock(s.Then, owners, local)
		qualifyImportedNamedTypesInBlock(s.Else, owners, local)
	}
}

func qualifyImportedNamedTypesInExpr(expr backendir.Expr, owners map[string]string, local map[string]struct{}) {
	switch e := expr.(type) {
	case *backendir.SelectorExpr:
		qualifyImportedNamedTypesInExpr(e.Subject, owners, local)
	case *backendir.CallExpr:
		qualifyImportedNamedTypesInExpr(e.Callee, owners, local)
		for i := range e.TypeArgs {
			e.TypeArgs[i] = qualifyImportedNamedType(e.TypeArgs[i], owners, local)
		}
		for _, arg := range e.Args {
			qualifyImportedNamedTypesInExpr(arg, owners, local)
		}
	case *backendir.TraitCoerceExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		qualifyImportedNamedTypesInExpr(e.Value, owners, local)
	case *backendir.MaybeSomeExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		qualifyImportedNamedTypesInExpr(e.Value, owners, local)
	case *backendir.MaybeNoneExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
	case *backendir.ResultOkExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		qualifyImportedNamedTypesInExpr(e.Value, owners, local)
	case *backendir.ResultErrExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		qualifyImportedNamedTypesInExpr(e.Value, owners, local)
	case *backendir.AddressOfExpr:
		qualifyImportedNamedTypesInExpr(e.Value, owners, local)
	case *backendir.FuncLiteralExpr:
		for i := range e.Params {
			e.Params[i].Type = qualifyImportedNamedType(e.Params[i].Type, owners, local)
		}
		e.Return = qualifyImportedNamedType(e.Return, owners, local)
		qualifyImportedNamedTypesInBlock(e.Body, owners, local)
	case *backendir.ListLiteralExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		for _, element := range e.Elements {
			qualifyImportedNamedTypesInExpr(element, owners, local)
		}
	case *backendir.MapLiteralExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		for _, entry := range e.Entries {
			qualifyImportedNamedTypesInExpr(entry.Key, owners, local)
			qualifyImportedNamedTypesInExpr(entry.Value, owners, local)
		}
	case *backendir.StructLiteralExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		for _, field := range e.Fields {
			qualifyImportedNamedTypesInExpr(field.Value, owners, local)
		}
	case *backendir.EnumVariantExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
	case *backendir.IfExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		qualifyImportedNamedTypesInExpr(e.Cond, owners, local)
		qualifyImportedNamedTypesInBlock(e.Then, owners, local)
		qualifyImportedNamedTypesInBlock(e.Else, owners, local)
	case *backendir.UnionMatchExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		qualifyImportedNamedTypesInExpr(e.Subject, owners, local)
		for i := range e.Cases {
			e.Cases[i].Type = qualifyImportedNamedType(e.Cases[i].Type, owners, local)
			qualifyImportedNamedTypesInBlock(e.Cases[i].Body, owners, local)
		}
		qualifyImportedNamedTypesInBlock(e.CatchAll, owners, local)
	case *backendir.TryExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		qualifyImportedNamedTypesInExpr(e.Subject, owners, local)
		qualifyImportedNamedTypesInBlock(e.Catch, owners, local)
	case *backendir.PanicExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		qualifyImportedNamedTypesInExpr(e.Message, owners, local)
	case *backendir.CopyExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		qualifyImportedNamedTypesInExpr(e.Value, owners, local)
	case *backendir.BlockExpr:
		e.Type = qualifyImportedNamedType(e.Type, owners, local)
		for _, setup := range e.Setup {
			qualifyImportedNamedTypesInStmt(setup, owners, local)
		}
		qualifyImportedNamedTypesInExpr(e.Value, owners, local)
	}
}

func qualifyImportedNamedType(t backendir.Type, owners map[string]string, local map[string]struct{}) backendir.Type {
	switch typed := t.(type) {
	case *backendir.NamedType:
		if strings.TrimSpace(typed.Module) == "" {
			name := strings.TrimSpace(typed.Name)
			if _, isLocal := local[name]; !isLocal {
				if owner := strings.TrimSpace(owners[name]); owner != "" {
					typed.Module = owner
				}
			}
		}
		for i := range typed.Args {
			typed.Args[i] = qualifyImportedNamedType(typed.Args[i], owners, local)
		}
	case *backendir.ListType:
		typed.Elem = qualifyImportedNamedType(typed.Elem, owners, local)
	case *backendir.MapType:
		typed.Key = qualifyImportedNamedType(typed.Key, owners, local)
		typed.Value = qualifyImportedNamedType(typed.Value, owners, local)
	case *backendir.MaybeType:
		typed.Of = qualifyImportedNamedType(typed.Of, owners, local)
	case *backendir.ResultType:
		typed.Val = qualifyImportedNamedType(typed.Val, owners, local)
		typed.Err = qualifyImportedNamedType(typed.Err, owners, local)
	case *backendir.FuncType:
		for i := range typed.Params {
			typed.Params[i] = qualifyImportedNamedType(typed.Params[i], owners, local)
		}
		typed.Return = qualifyImportedNamedType(typed.Return, owners, local)
	case *backendir.TraitType:
		for i := range typed.Methods {
			if methodType, ok := qualifyImportedNamedType(typed.Methods[i].Type, owners, local).(*backendir.FuncType); ok {
				typed.Methods[i].Type = methodType
			}
		}
	}
	return t
}

func collectOrphanTypeDecls(program *checker.Program, out *backendir.Module, seenDecls map[string]struct{}) {
	if program == nil {
		return
	}
	visited := make(map[string]struct{})
	collect := func(t checker.Type) {
		visitOrphanTypeDeclsInType(t, out, seenDecls, visited)
	}
	for _, stmt := range program.Statements {
		if stmt.Expr != nil {
			collect(stmt.Expr.Type())
			switch def := stmt.Expr.(type) {
			case *checker.FunctionDef:
				for _, param := range def.Parameters {
					collect(param.Type)
				}
				collect(effectiveFunctionReturnType(def))
			case *checker.ExternalFunctionDef:
				for _, param := range def.Parameters {
					collect(param.Type)
				}
				collect(def.ReturnType)
			}
		}
		if stmt.Stmt != nil {
			switch def := stmt.Stmt.(type) {
			case *checker.VariableDef:
				if def != nil {
					collect(def.Type())
					collectTypeDeclsInExpr(def.Value, collect)
				}
			case *checker.StructDef:
				if def != nil {
					for _, fieldType := range def.Fields {
						collect(fieldType)
					}
					for _, method := range def.Methods {
						for _, param := range method.Parameters {
							collect(param.Type)
						}
						collect(effectiveFunctionReturnType(method))
					}
				}
			case checker.StructDef:
				for _, fieldType := range def.Fields {
					collect(fieldType)
				}
				for _, method := range def.Methods {
					for _, param := range method.Parameters {
						collect(param.Type)
					}
					collect(effectiveFunctionReturnType(method))
				}
			}
		}
	}
}

func collectTypeDeclsInExpr(expr checker.Expression, collect func(checker.Type)) {
	if expr == nil {
		return
	}
	value := reflect.ValueOf(expr)
	if value.IsValid() && value.Kind() == reflect.Pointer && value.IsNil() {
		return
	}
	collect(expr.Type())
	collectTypeDeclsInValue(value, collect, make(map[uintptr]struct{}))
}

func collectTypeDeclsInBlock(block *checker.Block, collect func(checker.Type)) {
	if block == nil {
		return
	}
	for _, stmt := range block.Stmts {
		if stmt.Expr != nil {
			collectTypeDeclsInExpr(stmt.Expr, collect)
		}
		if variableDef, ok := stmt.Stmt.(*checker.VariableDef); ok && variableDef != nil {
			collect(variableDef.Type())
			collectTypeDeclsInExpr(variableDef.Value, collect)
		}
	}
}

func collectTypeDeclsInValue(value reflect.Value, collect func(checker.Type), seen map[uintptr]struct{}) {
	if !value.IsValid() {
		return
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return
		}
		value = value.Elem()
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		ptr := value.Pointer()
		if _, ok := seen[ptr]; ok {
			return
		}
		seen[ptr] = struct{}{}
		if expr, ok := value.Interface().(checker.Expression); ok && expr != nil {
			collect(expr.Type())
		}
		if block, ok := value.Interface().(*checker.Block); ok {
			collectTypeDeclsInBlock(block, collect)
			return
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		if value.Kind() == reflect.Slice {
			for i := 0; i < value.Len(); i++ {
				collectTypeDeclsInValue(value.Index(i), collect, seen)
			}
		}
		return
	}
	for i := 0; i < value.NumField(); i++ {
		fieldInfo := value.Type().Field(i)
		if fieldInfo.PkgPath != "" {
			continue
		}
		field := value.Field(i)
		if field.CanInterface() {
			if expr, ok := field.Interface().(checker.Expression); ok && expr != nil {
				collectTypeDeclsInExpr(expr, collect)
				continue
			}
			if block, ok := field.Interface().(*checker.Block); ok {
				collectTypeDeclsInBlock(block, collect)
				continue
			}
		}
		collectTypeDeclsInValue(field, collect, seen)
	}
}

func orphanTypeVisitKey(t checker.Type) string {
	if t == nil {
		return "<nil>"
	}
	value := reflect.ValueOf(t)
	if value.IsValid() && value.Kind() == reflect.Pointer && !value.IsNil() {
		return fmt.Sprintf("%T:%x", t, value.Pointer())
	}
	return fmt.Sprintf("%T:%s", t, t.String())
}

func visitOrphanTypeDeclsInType(t checker.Type, out *backendir.Module, seenDecls map[string]struct{}, visited map[string]struct{}) {
	if t == nil {
		return
	}
	visitKey := orphanTypeVisitKey(t)
	if _, seen := visited[visitKey]; seen {
		return
	}
	visited[visitKey] = struct{}{}
	switch typed := t.(type) {
	case checker.Enum:
		typedCopy := typed
		visitOrphanTypeDeclsInType(&typedCopy, out, seenDecls, visited)
	case *checker.Enum:
		if typed == nil {
			return
		}
		key := "enum:" + typed.Name
		if _, exists := seenDecls[key]; exists {
			return
		}
		seenDecls[key] = struct{}{}
		out.Decls = append(out.Decls, lowerEnumDeclToBackendIR(typed))
	case checker.Union:
		typedCopy := typed
		visitOrphanTypeDeclsInType(&typedCopy, out, seenDecls, visited)
	case *checker.Union:
		if typed == nil {
			return
		}
		name := strings.TrimSpace(typed.Name)
		if name != "" {
			key := "union:" + name
			if _, exists := seenDecls[key]; !exists {
				seenDecls[key] = struct{}{}
				out.Decls = append(out.Decls, lowerUnionDeclToBackendIR(typed))
			}
		}
		for _, memberType := range typed.Types {
			visitOrphanTypeDeclsInType(memberType, out, seenDecls, visited)
		}
	case *checker.TypeVar:
		if typed != nil {
			visitOrphanTypeDeclsInType(typed.Actual(), out, seenDecls, visited)
		}
	case *checker.List:
		if typed != nil {
			visitOrphanTypeDeclsInType(typed.Of(), out, seenDecls, visited)
		}
	case *checker.Map:
		if typed != nil {
			visitOrphanTypeDeclsInType(typed.Key(), out, seenDecls, visited)
			visitOrphanTypeDeclsInType(typed.Value(), out, seenDecls, visited)
		}
	case *checker.Maybe:
		if typed != nil {
			visitOrphanTypeDeclsInType(typed.Of(), out, seenDecls, visited)
		}
	case *checker.Result:
		if typed != nil {
			visitOrphanTypeDeclsInType(typed.Val(), out, seenDecls, visited)
			visitOrphanTypeDeclsInType(typed.Err(), out, seenDecls, visited)
		}
	case *checker.FunctionDef:
		if typed != nil {
			for _, param := range typed.Parameters {
				visitOrphanTypeDeclsInType(param.Type, out, seenDecls, visited)
			}
			visitOrphanTypeDeclsInType(effectiveFunctionReturnType(typed), out, seenDecls, visited)
		}
	case *checker.ExternalFunctionDef:
		if typed != nil {
			for _, param := range typed.Parameters {
				visitOrphanTypeDeclsInType(param.Type, out, seenDecls, visited)
			}
			visitOrphanTypeDeclsInType(typed.ReturnType, out, seenDecls, visited)
		}
	}
}

func lowerEntrypointStatementsToBackendIRBlock(stmts []checker.Statement) *backendir.Block {
	block := &backendir.Block{Stmts: []backendir.Stmt{}}
	for i, stmt := range stmts {
		block.Stmts = append(block.Stmts, lowerStatementToBackendIR(stmt)...)
		variableDef, ok := stmt.Stmt.(*checker.VariableDef)
		if !ok || variableDef == nil {
			continue
		}
		if variableDef.Name == "_" || strings.TrimSpace(variableDef.Name) == "" {
			continue
		}
		if usesNameInStatements(stmts[i+1:], variableDef.Name) {
			continue
		}
		block.Stmts = append(block.Stmts, &backendir.AssignStmt{
			Target: "_",
			Value:  &backendir.IdentExpr{Name: variableDef.Name},
		})
	}
	return block
}

func lowerStructDeclToBackendIR(def *checker.StructDef) backendir.Decl {
	fields := make([]backendir.Field, 0, len(def.Fields))
	for _, fieldName := range sortedStringKeys(def.Fields) {
		fields = append(fields, backendir.Field{
			Name: fieldName,
			Type: lowerCheckerTypeToBackendIR(def.Fields[fieldName]),
		})
	}
	methods := make([]*backendir.FuncDecl, 0, len(def.Methods))
	for _, methodName := range sortedStringKeys(def.Methods) {
		methodDecl, ok := lowerFunctionDeclToBackendIR(def.Methods[methodName]).(*backendir.FuncDecl)
		if !ok || methodDecl == nil {
			continue
		}
		methods = append(methods, methodDecl)
	}
	return &backendir.StructDecl{
		Name:       def.Name,
		TypeParams: structTypeParamOrder(def),
		Fields:     fields,
		Methods:    methods,
	}
}

func lowerEnumDeclToBackendIR(def *checker.Enum) backendir.Decl {
	values := make([]backendir.EnumValue, 0, len(def.Values))
	for _, value := range def.Values {
		values = append(values, backendir.EnumValue{
			Name:  value.Name,
			Value: value.Value,
		})
	}
	methods := make([]*backendir.FuncDecl, 0, len(def.Methods))
	for _, methodName := range sortedStringKeys(def.Methods) {
		methodDecl, ok := lowerFunctionDeclToBackendIR(def.Methods[methodName]).(*backendir.FuncDecl)
		if !ok || methodDecl == nil {
			continue
		}
		methods = append(methods, methodDecl)
	}
	return &backendir.EnumDecl{
		Name:    def.Name,
		Values:  values,
		Methods: methods,
	}
}

func lowerUnionDeclToBackendIR(def *checker.Union) backendir.Decl {
	types := make([]backendir.Type, 0, len(def.Types))
	for _, typ := range def.Types {
		types = append(types, lowerCheckerTypeToBackendIR(typ))
	}
	return &backendir.UnionDecl{
		Name:  def.Name,
		Types: types,
	}
}

func lowerExternTypeDeclToBackendIR(def *checker.ExternType) backendir.Decl {
	args := make([]backendir.Type, 0, len(def.TypeArgs))
	for _, arg := range def.TypeArgs {
		args = append(args, lowerCheckerTypeToBackendIR(arg))
	}
	return &backendir.ExternTypeDecl{
		Name: strings.TrimSpace(def.Name_),
		Args: args,
	}
}

func mutableParamNeedsPointer(t checker.Type) bool {
	switch typed := t.(type) {
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			return mutableParamNeedsPointer(actual)
		}
		return false
	case *checker.StructDef:
		return true
	case *checker.List:
		return true
	default:
		return false
	}
}

func lowerVariableDeclToBackendIR(def *checker.VariableDef) backendir.Decl {
	return &backendir.VarDecl{
		Name:    def.Name,
		Type:    lowerCheckerTypeToBackendIR(def.Type()),
		Value:   lowerExpressionOrOpaqueExpected(def.Value, def.Type()),
		Mutable: def.Mutable,
	}
}

func lowerFunctionDeclToBackendIR(def *checker.FunctionDef) backendir.Decl {
	params := make([]backendir.Param, 0, len(def.Parameters))
	for _, param := range def.Parameters {
		params = append(params, backendir.Param{
			Name:    param.Name,
			Type:    lowerCheckerTypeToBackendIR(param.Type),
			Mutable: param.Mutable,
			ByRef:   param.Mutable && mutableParamNeedsPointer(param.Type),
		})
	}

	returnType := lowerCheckerTypeToBackendIR(effectiveFunctionReturnType(def))
	body := lowerBlockToBackendIRWithReturnType(def.Body, effectiveFunctionReturnType(def))
	finalizeFunctionBodyForReturn(body, returnType)
	hoistNestedTryExprsInBlock(body)

	return &backendir.FuncDecl{
		Name:            def.Name,
		Params:          params,
		Return:          returnType,
		Body:            body,
		ReceiverName:    strings.TrimSpace(def.Receiver),
		ReceiverMutates: def.Mutates,
		IsExtern:        false,
		IsPrivate:       def.Private,
		IsTest:          def.IsTest,
	}
}

func finalizeFunctionBodyForReturn(body *backendir.Block, returnType backendir.Type) {
	if body == nil || isVoidIRType(returnType) || len(body.Stmts) == 0 {
		return
	}

	lastIndex := len(body.Stmts) - 1
	switch last := body.Stmts[lastIndex].(type) {
	case *backendir.ReturnStmt:
		return
	case *backendir.ExprStmt:
		body.Stmts[lastIndex] = &backendir.ReturnStmt{Value: last.Value}
	}
}

func finalizeCatchBlockForReturn(body *backendir.Block, returnType backendir.Type) {
	finalizeFunctionBodyForReturn(body, returnType)
	if body == nil || !isVoidIRType(returnType) || blockEndsInIRReturn(body.Stmts) {
		return
	}
	body.Stmts = append(body.Stmts, &backendir.ReturnStmt{})
}

func hoistNestedTryExprsInBlock(block *backendir.Block) {
	counter := 0
	hoistNestedTryExprsInBlockWithCounter(block, &counter)
}

func hoistNestedTryExprsInBlockWithCounter(block *backendir.Block, counter *int) {
	if block == nil {
		return
	}
	out := make([]backendir.Stmt, 0, len(block.Stmts))
	for _, stmt := range block.Stmts {
		switch s := stmt.(type) {
		case *backendir.AssignStmt:
			setup, value := hoistNestedTryExprsInExpr(s.Value, counter, true)
			out = append(out, setup...)
			s.Value = value
			out = append(out, s)
		case *backendir.ReturnStmt:
			setup, value := hoistNestedTryExprsInExpr(s.Value, counter, true)
			out = append(out, setup...)
			s.Value = value
			out = append(out, s)
		case *backendir.ExprStmt:
			setup, value := hoistNestedTryExprsInExpr(s.Value, counter, true)
			out = append(out, setup...)
			s.Value = value
			out = append(out, s)
		case *backendir.BindStmt:
			setup, value := hoistNestedTryExprsInExpr(s.Value, counter, true)
			out = append(out, setup...)
			s.Value = value
			out = append(out, s)
		case *backendir.MemberAssignStmt:
			setupSubject, subject := hoistNestedTryExprsInExpr(s.Subject, counter, true)
			setupValue, value := hoistNestedTryExprsInExpr(s.Value, counter, true)
			out = append(out, setupSubject...)
			out = append(out, setupValue...)
			s.Subject = subject
			s.Value = value
			out = append(out, s)
		case *backendir.IfStmt:
			setup, cond := hoistNestedTryExprsInExpr(s.Cond, counter, true)
			out = append(out, setup...)
			s.Cond = cond
			hoistNestedTryExprsInBlockWithCounter(s.Then, counter)
			hoistNestedTryExprsInBlockWithCounter(s.Else, counter)
			out = append(out, s)
		case *backendir.ForIntRangeStmt:
			setupStart, start := hoistNestedTryExprsInExpr(s.Start, counter, true)
			setupEnd, end := hoistNestedTryExprsInExpr(s.End, counter, true)
			out = append(out, setupStart...)
			out = append(out, setupEnd...)
			s.Start = start
			s.End = end
			hoistNestedTryExprsInBlockWithCounter(s.Body, counter)
			out = append(out, s)
		case *backendir.ForLoopStmt:
			setupInit, init := hoistNestedTryExprsInExpr(s.InitValue, counter, true)
			setupCond, cond := hoistNestedTryExprsInExpr(s.Cond, counter, true)
			out = append(out, setupInit...)
			out = append(out, setupCond...)
			s.InitValue = init
			s.Cond = cond
			hoistNestedTryExprsInBlockWithCounter(s.Body, counter)
			out = append(out, s)
		case *backendir.ForInStrStmt:
			setup, value := hoistNestedTryExprsInExpr(s.Value, counter, true)
			out = append(out, setup...)
			s.Value = value
			hoistNestedTryExprsInBlockWithCounter(s.Body, counter)
			out = append(out, s)
		case *backendir.ForInListStmt:
			setup, list := hoistNestedTryExprsInExpr(s.List, counter, true)
			out = append(out, setup...)
			s.List = list
			hoistNestedTryExprsInBlockWithCounter(s.Body, counter)
			out = append(out, s)
		case *backendir.ForInMapStmt:
			setup, value := hoistNestedTryExprsInExpr(s.Map, counter, true)
			out = append(out, setup...)
			s.Map = value
			hoistNestedTryExprsInBlockWithCounter(s.Body, counter)
			out = append(out, s)
		case *backendir.WhileStmt:
			setup, cond := hoistNestedTryExprsInExpr(s.Cond, counter, true)
			out = append(out, setup...)
			s.Cond = cond
			hoistNestedTryExprsInBlockWithCounter(s.Body, counter)
			out = append(out, s)
		default:
			out = append(out, stmt)
		}
	}
	block.Stmts = out
}

func hoistNestedTryExprsInExpr(expr backendir.Expr, counter *int, allowTopLevelTry bool) ([]backendir.Stmt, backendir.Expr) {
	if expr == nil {
		return nil, expr
	}
	if tryExpr, ok := expr.(*backendir.TryExpr); ok {
		hoistNestedTryExprsInBlockWithCounter(tryExpr.Catch, counter)
		if allowTopLevelTry {
			return nil, expr
		}
		name := fmt.Sprintf("__ardTryUnwrap%d", *counter)
		*counter++
		return []backendir.Stmt{&backendir.AssignStmt{Target: name, Value: tryExpr}}, &backendir.IdentExpr{Name: name}
	}
	switch v := expr.(type) {
	case *backendir.CallExpr:
		setup := make([]backendir.Stmt, 0)
		calleeSetup, callee := hoistNestedTryExprsInExpr(v.Callee, counter, false)
		setup = append(setup, calleeSetup...)
		v.Callee = callee
		for i, arg := range v.Args {
			argSetup, lowered := hoistNestedTryExprsInExpr(arg, counter, false)
			setup = append(setup, argSetup...)
			v.Args[i] = lowered
		}
		return setup, v
	case *backendir.SelectorExpr:
		setup, subject := hoistNestedTryExprsInExpr(v.Subject, counter, false)
		v.Subject = subject
		return setup, v
	case *backendir.ListLiteralExpr:
		setup := make([]backendir.Stmt, 0)
		for i, element := range v.Elements {
			elementSetup, lowered := hoistNestedTryExprsInExpr(element, counter, false)
			setup = append(setup, elementSetup...)
			v.Elements[i] = lowered
		}
		return setup, v
	case *backendir.MapLiteralExpr:
		setup := make([]backendir.Stmt, 0)
		for i := range v.Entries {
			keySetup, key := hoistNestedTryExprsInExpr(v.Entries[i].Key, counter, false)
			valueSetup, value := hoistNestedTryExprsInExpr(v.Entries[i].Value, counter, false)
			setup = append(setup, keySetup...)
			setup = append(setup, valueSetup...)
			v.Entries[i].Key = key
			v.Entries[i].Value = value
		}
		return setup, v
	case *backendir.StructLiteralExpr:
		setup := make([]backendir.Stmt, 0)
		for i := range v.Fields {
			fieldSetup, value := hoistNestedTryExprsInExpr(v.Fields[i].Value, counter, false)
			setup = append(setup, fieldSetup...)
			v.Fields[i].Value = value
		}
		return setup, v
	case *backendir.MaybeSomeExpr:
		setup, value := hoistNestedTryExprsInExpr(v.Value, counter, false)
		v.Value = value
		return setup, v
	case *backendir.ResultOkExpr:
		setup, value := hoistNestedTryExprsInExpr(v.Value, counter, false)
		v.Value = value
		return setup, v
	case *backendir.ResultErrExpr:
		setup, value := hoistNestedTryExprsInExpr(v.Value, counter, false)
		v.Value = value
		return setup, v
	case *backendir.AddressOfExpr:
		setup, value := hoistNestedTryExprsInExpr(v.Value, counter, false)
		v.Value = value
		return setup, v
	case *backendir.CopyExpr:
		setup, value := hoistNestedTryExprsInExpr(v.Value, counter, false)
		v.Value = value
		return setup, v
	case *backendir.TraitCoerceExpr:
		setup, value := hoistNestedTryExprsInExpr(v.Value, counter, false)
		v.Value = value
		return setup, v
	case *backendir.IfExpr:
		setup, cond := hoistNestedTryExprsInExpr(v.Cond, counter, false)
		v.Cond = cond
		hoistNestedTryExprsInBlockWithCounter(v.Then, counter)
		hoistNestedTryExprsInBlockWithCounter(v.Else, counter)
		return setup, v
	case *backendir.UnionMatchExpr:
		setup, subject := hoistNestedTryExprsInExpr(v.Subject, counter, false)
		v.Subject = subject
		for _, matchCase := range v.Cases {
			hoistNestedTryExprsInBlockWithCounter(matchCase.Body, counter)
		}
		hoistNestedTryExprsInBlockWithCounter(v.CatchAll, counter)
		return setup, v
	case *backendir.BlockExpr:
		hoistNestedTryExprsInBlockWithCounter(&backendir.Block{Stmts: v.Setup}, counter)
		setup, value := hoistNestedTryExprsInExpr(v.Value, counter, false)
		v.Setup = append(v.Setup, setup...)
		v.Value = value
		return nil, v
	case *backendir.FuncLiteralExpr:
		hoistNestedTryExprsInBlockWithCounter(v.Body, counter)
		return nil, v
	default:
		return nil, expr
	}
}

func blockEndsInIRReturn(stmts []backendir.Stmt) bool {
	if len(stmts) == 0 {
		return false
	}
	switch last := stmts[len(stmts)-1].(type) {
	case *backendir.ReturnStmt:
		return true
	case *backendir.IfStmt:
		return last.Then != nil && last.Else != nil && blockEndsInIRReturn(last.Then.Stmts) && blockEndsInIRReturn(last.Else.Stmts)
	default:
		return false
	}
}

func isVoidIRType(t backendir.Type) bool {
	_, ok := t.(*backendir.VoidType)
	return ok
}

func lowerExternFunctionDeclToBackendIR(def *checker.ExternalFunctionDef) backendir.Decl {
	params := make([]backendir.Param, 0, len(def.Parameters))
	for _, param := range def.Parameters {
		params = append(params, backendir.Param{
			Name:    param.Name,
			Type:    lowerCheckerTypeToBackendIR(param.Type),
			Mutable: param.Mutable,
			ByRef:   param.Mutable && mutableParamNeedsPointer(param.Type),
		})
	}

	binding := strings.TrimSpace(def.ExternalBinding)
	if binding == "" {
		binding = "<unresolved>"
	}

	return &backendir.FuncDecl{
		Name:          def.Name,
		Params:        params,
		Return:        lowerCheckerTypeToBackendIR(def.ReturnType),
		Body:          nil,
		ExternBinding: binding,
		IsExtern:      true,
		IsPrivate:     def.Private,
	}
}

func lowerBlockToBackendIR(block *checker.Block) *backendir.Block {
	return lowerBlockToBackendIRWithReturnType(block, nil)
}

func lowerBlockToBackendIRWithReturnType(block *checker.Block, returnType checker.Type) *backendir.Block {
	return lowerBlockToBackendIRWithContext(block, returnType, returnType)
}

func lowerBlockToBackendIRWithControlReturnType(block *checker.Block, returnType checker.Type) *backendir.Block {
	return lowerBlockToBackendIRWithContext(block, nil, returnType)
}

func lowerBlockToBackendIRWithContext(block *checker.Block, finalExpected checker.Type, returnType checker.Type) *backendir.Block {
	out := &backendir.Block{Stmts: []backendir.Stmt{}}
	if block == nil {
		return out
	}
	for i, stmt := range block.Stmts {
		var exprExpected checker.Type
		if i == len(block.Stmts)-1 {
			exprExpected = finalExpected
		}
		out.Stmts = append(out.Stmts, lowerStatementToBackendIRWithContext(stmt, exprExpected, returnType)...)
		variableDef, ok := stmt.Stmt.(*checker.VariableDef)
		if !ok || variableDef == nil {
			continue
		}
		if variableDef.Name == "_" || strings.TrimSpace(variableDef.Name) == "" {
			continue
		}
		if usesNameInStatements(block.Stmts[i+1:], variableDef.Name) {
			continue
		}
		out.Stmts = append(out.Stmts, &backendir.AssignStmt{
			Target: "_",
			Value:  &backendir.IdentExpr{Name: variableDef.Name},
		})
	}
	return out
}

func lowerStatementToBackendIR(stmt checker.Statement) []backendir.Stmt {
	return lowerStatementToBackendIRWithExpected(stmt, nil)
}

func lowerStatementToBackendIRWithExpected(stmt checker.Statement, expected checker.Type) []backendir.Stmt {
	return lowerStatementToBackendIRWithContext(stmt, expected, expected)
}

func lowerStatementToBackendIRWithContext(stmt checker.Statement, exprExpected checker.Type, returnType checker.Type) []backendir.Stmt {
	out := make([]backendir.Stmt, 0, 2)

	if stmt.Break {
		out = append(out, &backendir.BreakStmt{})
	}

	if stmt.Stmt != nil {
		out = append(out, lowerNonProducingToBackendIRWithReturnType(stmt.Stmt, returnType)...)
	}

	if stmt.Expr != nil {
		if ifExpr, ok := stmt.Expr.(*checker.If); ok {
			out = append(out, lowerIfChainToBackendIRWithReturnType(ifExpr, returnType))
		} else {
			out = append(out, &backendir.ExprStmt{
				Value: lowerExpressionToBackendIRWithContext(stmt.Expr, expectedForStatementExpr(stmt.Expr, exprExpected), returnType),
			})
		}
	}

	return out
}

func expectedForStatementExpr(expr checker.Expression, expected checker.Type) checker.Type {
	if expected != nil {
		return expected
	}
	switch expr.(type) {
	case *checker.If, *checker.BoolMatch, *checker.IntMatch, *checker.OptionMatch, *checker.ResultMatch, *checker.ConditionalMatch, *checker.EnumMatch, *checker.UnionMatch:
		return checker.Void
	default:
		return nil
	}
}

func lowerNonProducingToBackendIR(node checker.NonProducing) []backendir.Stmt {
	return lowerNonProducingToBackendIRWithReturnType(node, nil)
}

func lowerNonProducingToBackendIRWithReturnType(node checker.NonProducing, returnType checker.Type) []backendir.Stmt {
	switch n := node.(type) {
	case *checker.VariableDef:
		return []backendir.Stmt{
			&backendir.AssignStmt{
				Target: n.Name,
				Value:  lowerExpressionToBackendIRWithContext(n.Value, n.Type(), returnType),
			},
		}
	case *checker.Reassignment:
		return []backendir.Stmt{lowerReassignmentToBackendIRStmtWithReturnType(n, returnType)}
	case checker.ForIntRange:
		loop := n
		return lowerNonProducingToBackendIRWithReturnType(&loop, returnType)
	case *checker.ForIntRange:
		return []backendir.Stmt{
			&backendir.ForIntRangeStmt{
				Cursor: n.Cursor,
				Index:  n.Index,
				Start:  lowerExpressionOrOpaque(n.Start),
				End:    lowerExpressionOrOpaque(n.End),
				Body:   lowerBlockToBackendIRWithControlReturnType(n.Body, returnType),
			},
		}
	case checker.ForInStr:
		loop := n
		return lowerNonProducingToBackendIRWithReturnType(&loop, returnType)
	case *checker.ForInStr:
		return []backendir.Stmt{
			&backendir.ForInStrStmt{
				Cursor: n.Cursor,
				Index:  n.Index,
				Value:  lowerExpressionOrOpaque(n.Value),
				Body:   lowerBlockToBackendIRWithControlReturnType(n.Body, returnType),
			},
		}
	case checker.ForInList:
		loop := n
		return lowerNonProducingToBackendIRWithReturnType(&loop, returnType)
	case *checker.ForInList:
		var cursorType backendir.Type
		if listType, ok := n.List.Type().(*checker.List); ok && listType != nil {
			cursorType = lowerCheckerTypeToBackendIR(listType.Of())
		}
		return []backendir.Stmt{
			&backendir.ForInListStmt{
				Cursor:     n.Cursor,
				Index:      n.Index,
				CursorType: cursorType,
				List:       lowerExpressionOrOpaque(n.List),
				Body:       lowerBlockToBackendIRWithControlReturnType(n.Body, returnType),
			},
		}
	case checker.ForInMap:
		loop := n
		return lowerNonProducingToBackendIRWithReturnType(&loop, returnType)
	case *checker.ForInMap:
		return []backendir.Stmt{
			&backendir.ForInMapStmt{
				Key:   n.Key,
				Value: n.Val,
				Map:   lowerExpressionOrOpaque(n.Map),
				Body:  lowerBlockToBackendIRWithControlReturnType(n.Body, returnType),
			},
		}
	case checker.ForLoop:
		loop := n
		return lowerNonProducingToBackendIRWithReturnType(&loop, returnType)
	case *checker.ForLoop:
		update := lowerReassignmentToBackendIRStmt(n.Update)
		cond := lowerExpressionOrOpaque(n.Condition)
		if cond == nil {
			cond = literalExpr("bool", "true")
		}
		initName := "i"
		initValue := literalExpr("int", "0")
		if n.Init != nil {
			if strings.TrimSpace(n.Init.Name) != "" {
				initName = n.Init.Name
			}
			initValue = lowerExpressionOrOpaque(n.Init.Value)
		}
		return []backendir.Stmt{
			&backendir.ForLoopStmt{
				InitName:  initName,
				InitValue: initValue,
				Cond:      cond,
				Update:    update,
				Body:      lowerBlockToBackendIRWithControlReturnType(n.Body, returnType),
			},
		}
	case checker.WhileLoop:
		loop := n
		return lowerNonProducingToBackendIRWithReturnType(&loop, returnType)
	case *checker.WhileLoop:
		return []backendir.Stmt{
			&backendir.WhileStmt{
				Cond: lowerExpressionOrOpaque(n.Condition),
				Body: lowerBlockToBackendIRWithControlReturnType(n.Body, returnType),
			},
		}
	case checker.StructDef:
		def := n
		return lowerNonProducingToBackendIR(&def)
	case *checker.StructDef:
		return []backendir.Stmt{
			&backendir.ExprStmt{
				Value: callExpr("struct_decl_stmt", literalExpr("ident", n.Name)),
			},
		}
	case checker.Enum:
		def := n
		return lowerNonProducingToBackendIR(&def)
	case *checker.Enum:
		return []backendir.Stmt{
			&backendir.ExprStmt{
				Value: callExpr("enum_decl_stmt", literalExpr("ident", n.Name)),
			},
		}
	case checker.Union:
		def := n
		return lowerNonProducingToBackendIR(&def)
	case *checker.Union:
		args := make([]backendir.Expr, 0, len(n.Types)+1)
		args = append(args, literalExpr("ident", n.Name))
		for _, typ := range n.Types {
			args = append(args, typeExpr(lowerCheckerTypeToBackendIR(typ)))
		}
		return []backendir.Stmt{
			&backendir.ExprStmt{
				Value: callExpr("union_decl_stmt", args...),
			},
		}
	case *checker.ExternType:
		args := make([]backendir.Expr, 0, len(n.TypeArgs)+1)
		args = append(args, literalExpr("ident", n.Name_))
		for _, typeArg := range n.TypeArgs {
			args = append(args, typeExpr(lowerCheckerTypeToBackendIR(typeArg)))
		}
		return []backendir.Stmt{
			&backendir.ExprStmt{
				Value: callExpr("extern_type_decl_stmt", args...),
			},
		}
	default:
		return []backendir.Stmt{
			&backendir.ExprStmt{
				Value: callExpr(
					"nonproducing_stmt",
					literalExpr("type", fmt.Sprintf("%T", node)),
				),
			},
		}
	}
}

func lowerIfChainToBackendIR(node *checker.If) backendir.Stmt {
	return lowerIfChainToBackendIRWithReturnType(node, nil)
}

func lowerIfChainToBackendIRWithReturnType(node *checker.If, returnType checker.Type) backendir.Stmt {
	if node == nil {
		return &backendir.ExprStmt{Value: literalExpr("if_stmt", "nil")}
	}

	out := &backendir.IfStmt{
		Cond: lowerExpressionOrOpaque(node.Condition),
		Then: lowerBlockToBackendIRWithReturnType(node.Body, returnType),
	}

	if node.ElseIf != nil {
		out.Else = &backendir.Block{
			Stmts: []backendir.Stmt{
				lowerIfChainToBackendIRWithReturnType(withElseFallback(node.ElseIf, node.Else), returnType),
			},
		}
	} else if node.Else != nil {
		out.Else = lowerBlockToBackendIRWithReturnType(node.Else, returnType)
	}

	return out
}

func lowerBlockAsExpr(block *checker.Block) backendir.Expr {
	if block == nil {
		return literalExpr("block", "nil")
	}
	args := make([]backendir.Expr, 0, len(block.Stmts))
	for _, stmt := range block.Stmts {
		args = append(args, lowerStatementAsExpr(stmt))
	}
	return callExpr("block", args...)
}

func lowerStatementAsExpr(stmt checker.Statement) backendir.Expr {
	lowered := lowerStatementToBackendIR(stmt)
	if len(lowered) == 1 {
		return lowerIRStmtAsExpr(lowered[0])
	}
	args := make([]backendir.Expr, 0, len(lowered))
	for _, item := range lowered {
		args = append(args, lowerIRStmtAsExpr(item))
	}
	return callExpr("stmt_group", args...)
}

func lowerIRStmtAsExpr(stmt backendir.Stmt) backendir.Expr {
	switch s := stmt.(type) {
	case *backendir.ReturnStmt:
		if s.Value == nil {
			return callExpr("return_stmt")
		}
		return callExpr("return_stmt", s.Value)
	case *backendir.ExprStmt:
		return callExpr("expr_stmt", s.Value)
	case *backendir.BreakStmt:
		return callExpr("break_stmt")
	case *backendir.AssignStmt:
		return callExpr("assign_stmt", literalExpr("ident", s.Target), s.Value)
	case *backendir.MemberAssignStmt:
		return callExpr(
			"assign_member_stmt",
			s.Subject,
			literalExpr("ident", s.Field),
			s.Value,
		)
	case *backendir.ForIntRangeStmt:
		return callExpr(
			"for_int_range",
			literalExpr("ident", s.Cursor),
			literalExpr("ident", s.Index),
			s.Start,
			s.End,
			lowerIRBlockAsExpr(s.Body),
		)
	case *backendir.ForLoopStmt:
		return callExpr(
			"for_loop",
			literalExpr("ident", s.InitName),
			s.InitValue,
			s.Cond,
			lowerIRStmtAsExpr(s.Update),
			lowerIRBlockAsExpr(s.Body),
		)
	case *backendir.ForInStrStmt:
		return callExpr(
			"for_in_str",
			literalExpr("ident", s.Cursor),
			literalExpr("ident", s.Index),
			s.Value,
			lowerIRBlockAsExpr(s.Body),
		)
	case *backendir.ForInListStmt:
		return callExpr(
			"for_in_list",
			literalExpr("ident", s.Cursor),
			literalExpr("ident", s.Index),
			s.List,
			lowerIRBlockAsExpr(s.Body),
		)
	case *backendir.ForInMapStmt:
		return callExpr(
			"for_in_map",
			literalExpr("ident", s.Key),
			literalExpr("ident", s.Value),
			s.Map,
			lowerIRBlockAsExpr(s.Body),
		)
	case *backendir.WhileStmt:
		return callExpr(
			"while_loop",
			s.Cond,
			lowerIRBlockAsExpr(s.Body),
		)
	case *backendir.IfStmt:
		args := []backendir.Expr{
			s.Cond,
			lowerIRBlockAsExpr(s.Then),
		}
		if s.Else != nil {
			args = append(args, lowerIRBlockAsExpr(s.Else))
		}
		return callExpr("if_stmt", args...)
	default:
		return callExpr("stmt_unknown", literalExpr("type", fmt.Sprintf("%T", stmt)))
	}
}

func lowerIRBlockAsExpr(block *backendir.Block) backendir.Expr {
	if block == nil {
		return literalExpr("block", "nil")
	}
	args := make([]backendir.Expr, 0, len(block.Stmts))
	for _, stmt := range block.Stmts {
		args = append(args, lowerIRStmtAsExpr(stmt))
	}
	return callExpr("block_ir", args...)
}

func lowerReassignmentToBackendIRStmt(stmt *checker.Reassignment) backendir.Stmt {
	return lowerReassignmentToBackendIRStmtWithReturnType(stmt, nil)
}

func lowerReassignmentToBackendIRStmtWithReturnType(stmt *checker.Reassignment, returnType checker.Type) backendir.Stmt {
	if stmt == nil {
		return &backendir.AssignStmt{
			Target: "<target:nil>",
			Value:  literalExpr("reassign", "nil"),
		}
	}

	switch target := stmt.Target.(type) {
	case *checker.InstanceProperty:
		return &backendir.MemberAssignStmt{
			Subject: lowerExpressionOrOpaque(target.Subject),
			Field:   target.Property,
			Value:   lowerExpressionToBackendIRWithContext(stmt.Value, target.Type(), returnType),
		}
	default:
		return &backendir.AssignStmt{
			Target: lowerAssignmentTargetName(stmt.Target),
			Value:  lowerExpressionToBackendIRWithContext(stmt.Value, stmt.Target.Type(), returnType),
		}
	}
}

func lowerExpressionOrOpaque(expr checker.Expression) backendir.Expr {
	if expr == nil {
		return literalExpr("nil_expr", "")
	}
	return lowerExpressionToBackendIR(expr)
}

func lowerExpressionOrOpaqueExpected(expr checker.Expression, expected checker.Type) backendir.Expr {
	if expr == nil {
		return literalExpr("nil_expr", "")
	}
	return lowerExpressionToBackendIRWithExpected(expr, expected)
}

func lowerExpressionToBackendIRWithExpected(expr checker.Expression, expected checker.Type) backendir.Expr {
	return lowerExpressionToBackendIRWithContext(expr, expected, nil)
}

func lowerExpressionToBackendIRWithContext(expr checker.Expression, expected checker.Type, returnType checker.Type) backendir.Expr {
	if moduleCall, ok := expr.(*checker.ModuleFunctionCall); ok {
		if special := lowerSpecialModuleConstructorToBackendIR(moduleCall, expected); special != nil {
			return special
		}
	}
	if boolMatch, ok := expr.(*checker.BoolMatch); ok && expected != nil {
		return lowerBoolMatchExprToBackendIRWithContext(boolMatch, expected, returnType)
	}
	if intMatch, ok := expr.(*checker.IntMatch); ok && expected != nil {
		return lowerIntMatchExprToBackendIRWithContext(intMatch, expected, returnType)
	}
	if conditionalMatch, ok := expr.(*checker.ConditionalMatch); ok && expected != nil {
		return lowerConditionalMatchExprToBackendIRWithContext(conditionalMatch, expected, returnType)
	}
	if optionMatch, ok := expr.(*checker.OptionMatch); ok && expected != nil {
		return lowerOptionMatchExprToBackendIRWithContext(optionMatch, expected, returnType)
	}
	if resultMatch, ok := expr.(*checker.ResultMatch); ok && expected != nil {
		return lowerResultMatchExprToBackendIRWithContext(resultMatch, expected, returnType)
	}
	if unionMatch, ok := expr.(*checker.UnionMatch); ok && expected != nil {
		return lowerUnionMatchExprToBackendIRWithContext(unionMatch, expected, returnType)
	}
	if enumMatch, ok := expr.(*checker.EnumMatch); ok && expected != nil {
		return lowerEnumMatchExprToBackendIRWithContext(enumMatch, expected, returnType)
	}
	if tryOp, ok := expr.(*checker.TryOp); ok && (expected != nil || returnType != nil) {
		return lowerTryOpExprToBackendIRWithContext(tryOp, expected, returnType)
	}
	if functionCall, ok := expr.(*checker.FunctionCall); ok && expected != nil {
		return lowerFunctionCallToBackendIRWithExpected(functionCall, expected)
	}
	if moduleFunctionCall, ok := expr.(*checker.ModuleFunctionCall); ok && expected != nil {
		return lowerModuleFunctionCallToBackendIRWithExpected(moduleFunctionCall, expected)
	}
	return lowerExpressionToBackendIR(expr)
}

func lowerExpressionToBackendIR(expr checker.Expression) backendir.Expr {
	switch v := expr.(type) {
	case *checker.Identifier:
		return &backendir.IdentExpr{Name: v.Name}
	case checker.Variable:
		return &backendir.IdentExpr{Name: v.Name()}
	case *checker.Variable:
		return &backendir.IdentExpr{Name: v.Name()}
	case *checker.StrLiteral:
		return &backendir.LiteralExpr{Kind: "str", Value: v.Value}
	case *checker.TemplateStr:
		args := make([]backendir.Expr, 0, len(v.Chunks))
		for _, chunk := range v.Chunks {
			args = append(args, lowerExpressionOrOpaque(chunk))
		}
		return &backendir.CallExpr{
			Callee: &backendir.IdentExpr{Name: "template"},
			Args:   args,
		}
	case *checker.BoolLiteral:
		return &backendir.LiteralExpr{Kind: "bool", Value: strconv.FormatBool(v.Value)}
	case *checker.VoidLiteral:
		return &backendir.LiteralExpr{Kind: "void", Value: "()"}
	case *checker.IntLiteral:
		return &backendir.LiteralExpr{Kind: "int", Value: strconv.Itoa(v.Value)}
	case *checker.FloatLiteral:
		return &backendir.LiteralExpr{Kind: "float", Value: strconv.FormatFloat(v.Value, 'g', 10, 64)}
	case *checker.FunctionCall:
		return lowerFunctionCallToBackendIR(v)
	case *checker.ModuleFunctionCall:
		return lowerModuleFunctionCallToBackendIR(v)
	case *checker.InstanceProperty:
		return &backendir.SelectorExpr{
			Subject: lowerExpressionOrOpaque(v.Subject),
			Name:    v.Property,
		}
	case *checker.InstanceMethod:
		if v.Method == nil {
			return callExpr("instance_method", literalExpr("nil", "method"))
		}
		args := make([]backendir.Expr, 0, len(v.Method.Args))
		for _, arg := range v.Method.Args {
			args = append(args, lowerExpressionOrOpaque(arg))
		}
		return &backendir.CallExpr{
			Callee: &backendir.SelectorExpr{
				Subject: lowerExpressionOrOpaque(v.Subject),
				Name:    v.Method.Name,
			},
			Args: args,
		}
	case *checker.ModuleSymbol:
		return &backendir.SelectorExpr{
			Subject: &backendir.IdentExpr{Name: v.Module},
			Name:    v.Symbol.Name,
		}
	case *checker.Block:
		return callExpr("block_expr", lowerBlockAsExpr(v))
	case checker.Enum:
		enum := v
		return lowerExpressionToBackendIR(&enum)
	case *checker.Enum:
		return callExpr("enum_type", literalExpr("ident", v.Name))
	case checker.Union:
		union := v
		return lowerExpressionToBackendIR(&union)
	case *checker.Union:
		args := make([]backendir.Expr, 0, len(v.Types)+1)
		args = append(args, literalExpr("ident", v.Name))
		for _, typ := range v.Types {
			args = append(args, typeExpr(lowerCheckerTypeToBackendIR(typ)))
		}
		return callExpr("union_type", args...)
	case *checker.StrMethod:
		args := make([]backendir.Expr, 0, len(v.Args)+1)
		args = append(args, lowerExpressionOrOpaque(v.Subject))
		for _, arg := range v.Args {
			args = append(args, lowerExpressionOrOpaque(arg))
		}
		switch v.Kind {
		case checker.StrSize:
			return callExpr("str_size", args...)
		case checker.StrIsEmpty:
			return callExpr("str_is_empty", args...)
		case checker.StrContains:
			return callExpr("str_contains", args...)
		case checker.StrReplace:
			return callExpr("str_replace", args...)
		case checker.StrReplaceAll:
			return callExpr("str_replace_all", args...)
		case checker.StrSplit:
			return callExpr("str_split", args...)
		case checker.StrStartsWith:
			return callExpr("str_starts_with", args...)
		case checker.StrToStr:
			return callExpr("str_to_str", args...)
		case checker.StrToDyn:
			return callExpr("str_to_dyn", args...)
		case checker.StrTrim:
			return callExpr("str_trim", args...)
		default:
			return callExpr("str_method:"+strMethodKindName(v.Kind), args...)
		}
	case *checker.IntMethod:
		switch v.Kind {
		case checker.IntToStr:
			return callExpr("int_to_str", lowerExpressionOrOpaque(v.Subject))
		case checker.IntToDyn:
			return callExpr("int_to_dyn", lowerExpressionOrOpaque(v.Subject))
		default:
			return callExpr("int_method:"+intMethodKindName(v.Kind), lowerExpressionOrOpaque(v.Subject))
		}
	case *checker.FloatMethod:
		switch v.Kind {
		case checker.FloatToStr:
			return callExpr("float_to_str", lowerExpressionOrOpaque(v.Subject))
		case checker.FloatToInt:
			return callExpr("float_to_int", lowerExpressionOrOpaque(v.Subject))
		case checker.FloatToDyn:
			return callExpr("float_to_dyn", lowerExpressionOrOpaque(v.Subject))
		default:
			return callExpr("float_method:"+floatMethodKindName(v.Kind), lowerExpressionOrOpaque(v.Subject))
		}
	case *checker.BoolMethod:
		switch v.Kind {
		case checker.BoolToStr:
			return callExpr("bool_to_str", lowerExpressionOrOpaque(v.Subject))
		case checker.BoolToDyn:
			return callExpr("bool_to_dyn", lowerExpressionOrOpaque(v.Subject))
		default:
			return callExpr("bool_method:"+boolMethodKindName(v.Kind), lowerExpressionOrOpaque(v.Subject))
		}
	case *checker.ListMethod:
		args := make([]backendir.Expr, 0, len(v.Args)+1)
		args = append(args, lowerExpressionOrOpaque(v.Subject))
		for _, arg := range v.Args {
			args = append(args, lowerExpressionOrOpaque(arg))
		}
		listValueTypeArgs := unionListMethodTypeArgs(v.ElementType)
		switch v.Kind {
		case checker.ListSize:
			return callExpr("list_size", args...)
		case checker.ListAt:
			return callExpr("list_at", args...)
		case checker.ListPush:
			return callExprWithTypeArgs("list_push", listValueTypeArgs, args...)
		case checker.ListPrepend:
			return callExprWithTypeArgs("list_prepend", listValueTypeArgs, args...)
		case checker.ListSet:
			return callExprWithTypeArgs("list_set", listValueTypeArgs, args...)
		case checker.ListSort:
			return callExpr("list_sort", args...)
		case checker.ListSwap:
			return callExpr("list_swap", args...)
		default:
			return callExpr("list_method:"+listMethodKindName(v.Kind), args...)
		}
	case *checker.MapMethod:
		args := make([]backendir.Expr, 0, len(v.Args)+1)
		args = append(args, lowerExpressionOrOpaque(v.Subject))
		for _, arg := range v.Args {
			args = append(args, lowerExpressionOrOpaque(arg))
		}
		switch v.Kind {
		case checker.MapSize:
			return callExpr("map_size", args...)
		case checker.MapKeys:
			return callExpr("map_keys", args...)
		case checker.MapHas:
			return callExpr("map_has", args...)
		case checker.MapGet:
			return callExpr("map_get", args...)
		case checker.MapSet:
			return callExpr("map_set", args...)
		case checker.MapDrop:
			return callExpr("map_drop", args...)
		default:
			return callExpr("map_method:"+mapMethodKindName(v.Kind), args...)
		}
	case *checker.MaybeMethod:
		args := make([]backendir.Expr, 0, len(v.Args)+1)
		args = append(args, lowerExpressionOrOpaque(v.Subject))
		for _, arg := range v.Args {
			args = append(args, lowerExpressionOrOpaque(arg))
		}
		switch v.Kind {
		case checker.MaybeExpect:
			return callExpr("maybe_expect", args...)
		case checker.MaybeIsNone:
			return callExpr("maybe_is_none", args...)
		case checker.MaybeIsSome:
			return callExpr("maybe_is_some", args...)
		case checker.MaybeOr:
			return callExpr("maybe_or", args...)
		case checker.MaybeMap:
			return callExpr("maybe_map", args...)
		case checker.MaybeAndThen:
			return callExpr("maybe_and_then", args...)
		default:
			return callExpr("maybe_method:"+maybeMethodKindName(v.Kind), args...)
		}
	case *checker.ResultMethod:
		args := make([]backendir.Expr, 0, len(v.Args)+1)
		args = append(args, lowerExpressionOrOpaque(v.Subject))
		for _, arg := range v.Args {
			args = append(args, lowerExpressionOrOpaque(arg))
		}
		switch v.Kind {
		case checker.ResultExpect:
			return callExpr("result_expect", args...)
		case checker.ResultOr:
			return callExpr("result_or", args...)
		case checker.ResultIsOk:
			return callExpr("result_is_ok", args...)
		case checker.ResultIsErr:
			return callExpr("result_is_err", args...)
		case checker.ResultMap:
			return callExpr("result_map", args...)
		case checker.ResultMapErr:
			return callExpr("result_map_err", args...)
		case checker.ResultAndThen:
			return callExpr("result_and_then", args...)
		default:
			return callExpr("result_method:"+resultMethodKindName(v.Kind), args...)
		}
	case *checker.ListLiteral:
		elements := make([]backendir.Expr, 0, len(v.Elements))
		for _, element := range v.Elements {
			elements = append(elements, lowerExpressionOrOpaque(element))
		}
		listType := lowerCheckerTypeToBackendIR(v.ListType)
		if _, ok := listType.(*backendir.ListType); !ok {
			listType = &backendir.ListType{Elem: backendir.Dynamic}
		}
		return &backendir.ListLiteralExpr{
			Type:     listType,
			Elements: elements,
		}
	case *checker.MapLiteral:
		entries := make([]backendir.MapEntry, 0, len(v.Keys))
		for i := range v.Keys {
			if i >= len(v.Values) {
				continue
			}
			entries = append(entries, backendir.MapEntry{
				Key:   lowerExpressionOrOpaque(v.Keys[i]),
				Value: lowerExpressionOrOpaque(v.Values[i]),
			})
		}
		mapType := lowerCheckerTypeToBackendIR(v.Type())
		if _, ok := mapType.(*backendir.MapType); !ok {
			mapType = &backendir.MapType{Key: backendir.Dynamic, Value: backendir.Dynamic}
		}
		return &backendir.MapLiteralExpr{
			Type:    mapType,
			Entries: entries,
		}
	case *checker.StructInstance:
		structType := lowerCheckerTypeToBackendIR(v.StructType)
		if _, ok := structType.(*backendir.NamedType); !ok {
			structType = &backendir.NamedType{Name: v.Name}
		}
		fields := make([]backendir.StructFieldValue, 0, len(v.Fields))
		for _, field := range sortedStringKeys(v.Fields) {
			fields = append(fields, backendir.StructFieldValue{
				Name:  field,
				Value: lowerExpressionOrOpaqueExpected(v.Fields[field], v.FieldTypes[field]),
			})
		}
		return &backendir.StructLiteralExpr{
			Type:   structType,
			Fields: fields,
		}
	case *checker.ModuleStructInstance:
		if v.Property == nil {
			return callExpr("module_struct_literal", literalExpr("nil", "property"))
		}
		structType := lowerCheckerTypeToBackendIR(v.StructType)
		if named, ok := structType.(*backendir.NamedType); ok {
			named.Module = strings.TrimSpace(v.Module)
		} else {
			structType = &backendir.NamedType{Module: strings.TrimSpace(v.Module), Name: v.Property.Name}
		}
		fields := make([]backendir.StructFieldValue, 0, len(v.Property.Fields))
		for _, field := range sortedStringKeys(v.Property.Fields) {
			value := lowerExpressionOrOpaqueExpected(v.Property.Fields[field], v.FieldTypes[field])
			if enumVariant, ok := value.(*backendir.EnumVariantExpr); ok {
				if named, ok := enumVariant.Type.(*backendir.NamedType); ok && strings.TrimSpace(named.Module) == "" {
					named.Module = strings.TrimSpace(v.Module)
				}
			}
			fields = append(fields, backendir.StructFieldValue{
				Name:  field,
				Value: value,
			})
		}
		return &backendir.StructLiteralExpr{
			Type:   structType,
			Fields: fields,
		}
	case *checker.EnumVariant:
		enumType := lowerCheckerTypeToBackendIR(v.EnumType)
		if named, ok := enumType.(*backendir.NamedType); !ok || strings.TrimSpace(named.Name) == "" {
			enumName := ""
			if sourceEnum, ok := v.EnumType.(*checker.Enum); ok {
				enumName = sourceEnum.Name
			}
			enumType = &backendir.NamedType{Name: enumName}
		}
		return &backendir.EnumVariantExpr{
			Type:         enumType,
			Discriminant: v.Discriminant,
		}
	case checker.EnumVariant:
		variant := v
		return lowerExpressionToBackendIR(&variant)
	case *checker.Not:
		return &backendir.CallExpr{
			Callee: &backendir.IdentExpr{Name: "not"},
			Args:   []backendir.Expr{lowerExpressionOrOpaque(v.Value)},
		}
	case *checker.Negation:
		return &backendir.CallExpr{
			Callee: &backendir.IdentExpr{Name: "neg"},
			Args:   []backendir.Expr{lowerExpressionOrOpaque(v.Value)},
		}
	case *checker.IntAddition:
		return lowerBinaryExprToBackendIR("int_add", v.Left, v.Right)
	case *checker.IntSubtraction:
		return lowerBinaryExprToBackendIR("int_sub", v.Left, v.Right)
	case *checker.IntMultiplication:
		return lowerBinaryExprToBackendIR("int_mul", v.Left, v.Right)
	case *checker.IntDivision:
		return lowerBinaryExprToBackendIR("int_div", v.Left, v.Right)
	case *checker.IntModulo:
		return lowerBinaryExprToBackendIR("int_mod", v.Left, v.Right)
	case *checker.IntGreater:
		return lowerBinaryExprToBackendIR("int_gt", v.Left, v.Right)
	case *checker.IntGreaterEqual:
		return lowerBinaryExprToBackendIR("int_gte", v.Left, v.Right)
	case *checker.IntLess:
		return lowerBinaryExprToBackendIR("int_lt", v.Left, v.Right)
	case *checker.IntLessEqual:
		return lowerBinaryExprToBackendIR("int_lte", v.Left, v.Right)
	case *checker.FloatAddition:
		return lowerBinaryExprToBackendIR("float_add", v.Left, v.Right)
	case *checker.FloatSubtraction:
		return lowerBinaryExprToBackendIR("float_sub", v.Left, v.Right)
	case *checker.FloatMultiplication:
		return lowerBinaryExprToBackendIR("float_mul", v.Left, v.Right)
	case *checker.FloatDivision:
		return lowerBinaryExprToBackendIR("float_div", v.Left, v.Right)
	case *checker.FloatGreater:
		return lowerBinaryExprToBackendIR("float_gt", v.Left, v.Right)
	case *checker.FloatGreaterEqual:
		return lowerBinaryExprToBackendIR("float_gte", v.Left, v.Right)
	case *checker.FloatLess:
		return lowerBinaryExprToBackendIR("float_lt", v.Left, v.Right)
	case *checker.FloatLessEqual:
		return lowerBinaryExprToBackendIR("float_lte", v.Left, v.Right)
	case *checker.StrAddition:
		return lowerBinaryExprToBackendIR("str_add", v.Left, v.Right)
	case *checker.Equality:
		return lowerBinaryExprToBackendIR("eq", v.Left, v.Right)
	case *checker.And:
		return lowerBinaryExprToBackendIR("and", v.Left, v.Right)
	case *checker.Or:
		return lowerBinaryExprToBackendIR("or", v.Left, v.Right)
	case *checker.If:
		return lowerIfExprToBackendIR(v)
	case *checker.BoolMatch:
		return lowerBoolMatchExprToBackendIR(v)
	case *checker.IntMatch:
		return lowerIntMatchExprToBackendIR(v)
	case *checker.ConditionalMatch:
		return lowerConditionalMatchExprToBackendIR(v)
	case *checker.OptionMatch:
		return lowerOptionMatchExprToBackendIR(v)
	case *checker.ResultMatch:
		return lowerResultMatchExprToBackendIR(v)
	case checker.ResultMatch:
		match := v
		return lowerExpressionToBackendIR(&match)
	case *checker.EnumMatch:
		return lowerEnumMatchExprToBackendIR(v)
	case *checker.UnionMatch:
		return lowerUnionMatchExprToBackendIR(v)
	case checker.TryOp:
		tryOp := v
		return lowerExpressionToBackendIR(&tryOp)
	case *checker.TryOp:
		return lowerTryOpExprToBackendIR(v)
	case *checker.CopyExpression:
		if _, ok := v.Type_.(*checker.List); ok {
			return &backendir.CopyExpr{
				Value: lowerExpressionOrOpaque(v.Expr),
				Type:  lowerCheckerTypeToBackendIR(v.Type_),
			}
		}
		return lowerExpressionOrOpaque(v.Expr)
	case checker.FiberStart:
		start := v
		return lowerExpressionToBackendIR(&start)
	case *checker.FiberStart:
		return callExpr("fiber_start", lowerExpressionOrOpaque(v.GetFn()))
	case checker.FiberEval:
		eval := v
		return lowerExpressionToBackendIR(&eval)
	case *checker.FiberEval:
		return callExpr("fiber_eval", lowerExpressionOrOpaque(v.GetFn()))
	case checker.FiberExecution:
		execution := v
		return lowerExpressionToBackendIR(&execution)
	case *checker.FiberExecution:
		modulePath := ""
		mainName := ""
		if v.GetModule() != nil {
			modulePath = v.GetModule().Path()
		}
		mainName = v.GetMainName()
		return callExpr(
			"fiber_execution",
			literalExpr("str", modulePath),
			literalExpr("str", mainName),
		)
	case *checker.FunctionDef:
		params := make([]backendir.Param, 0, len(v.Parameters))
		for _, param := range v.Parameters {
			params = append(params, backendir.Param{
				Name:    param.Name,
				Type:    lowerCheckerTypeToBackendIR(param.Type),
				Mutable: param.Mutable,
				ByRef:   param.Mutable && mutableParamNeedsPointer(param.Type),
			})
		}
		returnType := effectiveFunctionReturnType(v)
		body := lowerBlockToBackendIRWithReturnType(v.Body, returnType)
		irReturnType := lowerCheckerTypeToBackendIR(returnType)
		finalizeFunctionBodyForReturn(body, irReturnType)
		hoistNestedTryExprsInBlock(body)
		return &backendir.FuncLiteralExpr{
			Params: params,
			Return: irReturnType,
			Body:   body,
		}
	case *checker.ExternalFunctionDef:
		params := make([]backendir.Expr, 0, len(v.Parameters))
		for _, param := range v.Parameters {
			params = append(params, callExpr(
				"param",
				literalExpr("ident", param.Name),
				typeExpr(lowerCheckerTypeToBackendIR(param.Type)),
				literalExpr("bool", strconv.FormatBool(param.Mutable)),
			))
		}
		return callExpr(
			"extern_fn_literal",
			literalExpr("ident", v.Name),
			literalExpr("binding", strings.TrimSpace(v.ExternalBinding)),
			typeExpr(lowerCheckerTypeToBackendIR(v.ReturnType)),
			callExpr("params", params...),
		)
	case *checker.Panic:
		return &backendir.PanicExpr{
			Message: lowerExpressionOrOpaque(v.Message),
			Type:    lowerCheckerTypeToBackendIR(v.Type()),
		}
	case checker.Panic:
		return &backendir.PanicExpr{
			Message: lowerExpressionOrOpaque(v.Message),
			Type:    lowerCheckerTypeToBackendIR(v.Type()),
		}
	default:
		return callExpr(
			"unknown_expr",
			literalExpr("type", fmt.Sprintf("%T", expr)),
		)
	}
}

func lowerCallArgToBackendIR(arg checker.Expression, param checker.Parameter) backendir.Expr {
	return lowerCallArgToBackendIRWithBindings(arg, param, nil)
}

func lowerCallArgToBackendIRWithBindings(arg checker.Expression, param checker.Parameter, bindings map[string]checker.Type) backendir.Expr {
	value := lowerExpressionOrOpaqueExpected(arg, param.Type)
	if _, isListLiteral := arg.(*checker.ListLiteral); isListLiteral && len(bindings) > 0 {
		if expected := substituteBackendIRTypeVars(lowerCheckerTypeToBackendIR(param.Type), backendIRTypeBindings(bindings)); expected != nil {
			value = lowerExpressionOrOpaqueExpectedBackendIR(arg, expected)
		}
	}
	if param.Mutable && mutableParamNeedsPointer(param.Type) {
		value = &backendir.AddressOfExpr{Value: value}
	}
	if trait, ok := param.Type.(*checker.Trait); ok {
		return &backendir.TraitCoerceExpr{Value: value, Type: lowerTraitTypeToBackendIR(trait)}
	}
	if trait, ok := param.Type.(checker.Trait); ok {
		traitCopy := trait
		return &backendir.TraitCoerceExpr{Value: value, Type: lowerTraitTypeToBackendIR(&traitCopy)}
	}
	return value
}

func lowerExpressionOrOpaqueExpectedBackendIR(expr checker.Expression, expected backendir.Type) backendir.Expr {
	if expr == nil {
		return literalExpr("nil_expr", "")
	}
	if listLiteral, ok := expr.(*checker.ListLiteral); ok {
		elements := make([]backendir.Expr, 0, len(listLiteral.Elements))
		for _, element := range listLiteral.Elements {
			elements = append(elements, lowerExpressionOrOpaque(element))
		}
		if _, ok := expected.(*backendir.ListType); ok {
			return &backendir.ListLiteralExpr{Type: expected, Elements: elements}
		}
	}
	return lowerExpressionOrOpaqueExpected(expr, nil)
}

func lowerCallArgsToBackendIR(args []checker.Expression, def *checker.FunctionDef) []backendir.Expr {
	return lowerCallArgsToBackendIRWithBindings(args, def, nil)
}

func lowerCallArgsToBackendIRWithBindings(args []checker.Expression, def *checker.FunctionDef, bindings map[string]checker.Type) []backendir.Expr {
	out := make([]backendir.Expr, 0, len(args))
	for i, arg := range args {
		if def != nil && i < len(def.Parameters) {
			out = append(out, lowerCallArgToBackendIRWithBindings(arg, def.Parameters[i], bindings))
			continue
		}
		out = append(out, lowerExpressionOrOpaque(arg))
	}
	return out
}

func inferGenericCallTypeArgs(def *checker.FunctionDef, actualReturn checker.Type) []backendir.Type {
	_, args := inferGenericCallTypeArgBindings(def, nil, actualReturn)
	return args
}

func inferGenericCallTypeArgBindings(def *checker.FunctionDef, callArgs []checker.Expression, actualReturn checker.Type) (map[string]checker.Type, []backendir.Type) {
	if def == nil || actualReturn == nil {
		return nil, nil
	}
	order := make([]string, 0)
	seen := make(map[string]struct{})
	for _, param := range def.Parameters {
		collectUnboundTypeParamNames(param.Type, &order, seen)
	}
	collectUnboundTypeParamNames(effectiveFunctionReturnType(def), &order, seen)
	if len(order) == 0 {
		return nil, nil
	}
	bindings := make(map[string]checker.Type)
	bindGenericTypeArgs(effectiveFunctionReturnType(def), actualReturn, bindings)
	for i, arg := range callArgs {
		if i >= len(def.Parameters) {
			break
		}
		bindGenericTypeArgsFromExpr(def.Parameters[i].Type, arg, bindings)
	}
	if len(order) == 1 && bindings[order[0]] == nil {
		bindings[order[0]] = singleGenericCallTypeArg(actualReturn)
	}
	args := backendIRTypeArgsFromBindings(order, bindings)
	if len(args) == 0 {
		return bindings, nil
	}
	return bindings, args
}

func backendIRTypeArgsFromBindings(order []string, bindings map[string]checker.Type) []backendir.Type {
	args := make([]backendir.Type, 0, len(order))
	for _, name := range order {
		bound := bindings[name]
		if tv, ok := bound.(*checker.TypeVar); ok && tv.Actual() != nil {
			bound = tv.Actual()
		}
		if bound == nil {
			return nil
		}
		args = append(args, lowerCheckerTypeToBackendIR(bound))
	}
	return args
}

func collectTypeParamNamesInBlock(block *checker.Block, out *[]string, seen map[string]struct{}) {
	if block == nil {
		return
	}
	for _, stmt := range block.Stmts {
		if stmt.Expr != nil {
			collectTypeParamNames(stmt.Expr.Type(), out, seen)
		}
		if variableDef, ok := stmt.Stmt.(*checker.VariableDef); ok && variableDef != nil {
			collectTypeParamNames(variableDef.Type(), out, seen)
			if variableDef.Value != nil {
				collectTypeParamNames(variableDef.Value.Type(), out, seen)
			}
		}
	}
}

func shouldInferZeroArgGenericCallTypeArgs(def *checker.FunctionDef, moduleName string, funcName string) bool {
	if functionDefHasDeclaredTypeParams(def) {
		return true
	}
	switch strings.TrimSpace(moduleName) + "::" + strings.TrimSpace(funcName) {
	case "ard/list::new", "ard/map::new":
		return true
	default:
		return false
	}
}

func functionDefHasDeclaredTypeParams(def *checker.FunctionDef) bool {
	if def == nil {
		return false
	}
	order := make([]string, 0)
	seen := make(map[string]struct{})
	for _, param := range def.Parameters {
		collectDeclaredTypeParamNames(param.Type, &order, seen)
	}
	collectDeclaredTypeParamNames(effectiveFunctionReturnType(def), &order, seen)
	collectDeclaredTypeParamNamesInBlock(def.Body, &order, seen)
	return len(order) > 0
}

func collectDeclaredTypeParamNamesInBlock(block *checker.Block, out *[]string, seen map[string]struct{}) {
	if block == nil {
		return
	}
	for _, stmt := range block.Stmts {
		if stmt.Expr != nil {
			collectDeclaredTypeParamNames(stmt.Expr.Type(), out, seen)
		}
		if variableDef, ok := stmt.Stmt.(*checker.VariableDef); ok && variableDef != nil {
			collectDeclaredTypeParamNames(variableDef.Type(), out, seen)
			if variableDef.Value != nil {
				collectDeclaredTypeParamNames(variableDef.Value.Type(), out, seen)
			}
		}
	}
}

func collectDeclaredTypeParamNames(t checker.Type, out *[]string, seen map[string]struct{}) {
	if t == nil {
		return
	}
	switch typed := t.(type) {
	case *checker.TypeVar:
		name := typeVarName(typed)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		*out = append(*out, name)
	case *checker.List:
		collectDeclaredTypeParamNames(typed.Of(), out, seen)
	case *checker.Map:
		collectDeclaredTypeParamNames(typed.Key(), out, seen)
		collectDeclaredTypeParamNames(typed.Value(), out, seen)
	case *checker.Maybe:
		collectDeclaredTypeParamNames(typed.Of(), out, seen)
	case *checker.Result:
		collectDeclaredTypeParamNames(typed.Val(), out, seen)
		collectDeclaredTypeParamNames(typed.Err(), out, seen)
	case *checker.Union:
		for _, member := range typed.Types {
			collectDeclaredTypeParamNames(member, out, seen)
		}
	case *checker.StructDef:
		for _, fieldName := range sortedStringKeys(typed.Fields) {
			collectDeclaredTypeParamNames(typed.Fields[fieldName], out, seen)
		}
	case *checker.ExternType:
		for _, typeArg := range typed.TypeArgs {
			collectDeclaredTypeParamNames(typeArg, out, seen)
		}
	case *checker.FunctionDef:
		for _, param := range typed.Parameters {
			collectDeclaredTypeParamNames(param.Type, out, seen)
		}
		collectDeclaredTypeParamNames(effectiveFunctionReturnType(typed), out, seen)
	}
}

func inferZeroArgGenericCallTypeArgs(actualReturn checker.Type) []backendir.Type {
	arg := singleGenericCallTypeArg(actualReturn)
	if arg == nil {
		return nil
	}
	return []backendir.Type{lowerCheckerTypeToBackendIR(arg)}
}

func singleGenericCallTypeArg(t checker.Type) checker.Type {
	if t == nil {
		return nil
	}
	if list, ok := t.(*checker.List); ok {
		return list.Of()
	}
	if maybe, ok := t.(*checker.Maybe); ok {
		return maybe.Of()
	}
	if result, ok := t.(*checker.Result); ok {
		return result.Val()
	}
	if mapType, ok := t.(*checker.Map); ok {
		return mapType.Value()
	}
	return nil
}

func bindGenericTypeArgs(pattern checker.Type, actual checker.Type, bindings map[string]checker.Type) {
	if pattern == nil || actual == nil {
		return
	}
	if actualTypeVar, ok := actual.(*checker.TypeVar); ok && actualTypeVar.Actual() != nil {
		actual = actualTypeVar.Actual()
	}
	switch p := pattern.(type) {
	case *checker.TypeVar:
		bindings[typeVarName(p)] = actual
	case *checker.List:
		if a, ok := actual.(*checker.List); ok {
			bindGenericTypeArgs(p.Of(), a.Of(), bindings)
		}
	case *checker.Map:
		if a, ok := actual.(*checker.Map); ok {
			bindGenericTypeArgs(p.Key(), a.Key(), bindings)
			bindGenericTypeArgs(p.Value(), a.Value(), bindings)
		}
	case *checker.Maybe:
		if a, ok := actual.(*checker.Maybe); ok {
			bindGenericTypeArgs(p.Of(), a.Of(), bindings)
		}
	case *checker.Result:
		if a, ok := actual.(*checker.Result); ok {
			bindGenericTypeArgs(p.Val(), a.Val(), bindings)
			bindGenericTypeArgs(p.Err(), a.Err(), bindings)
		}
	case *checker.StructDef:
		if a, ok := actual.(*checker.StructDef); ok && p.Name == a.Name {
			for _, fieldName := range sortedStringKeys(p.Fields) {
				bindGenericTypeArgs(p.Fields[fieldName], a.Fields[fieldName], bindings)
			}
		}
	case *checker.ExternType:
		if a, ok := actual.(*checker.ExternType); ok && p.Name_ == a.Name_ {
			for i, patternArg := range p.TypeArgs {
				if i < len(a.TypeArgs) {
					bindGenericTypeArgs(patternArg, a.TypeArgs[i], bindings)
				}
			}
		}
	case *checker.FunctionDef:
		if a, ok := actual.(*checker.FunctionDef); ok {
			for i := range p.Parameters {
				if i < len(a.Parameters) {
					bindGenericTypeArgs(p.Parameters[i].Type, a.Parameters[i].Type, bindings)
				}
			}
			bindGenericTypeArgs(p.ReturnType, a.ReturnType, bindings)
		}
	}
}

func bindGenericTypeArgsFromExpr(pattern checker.Type, expr checker.Expression, bindings map[string]checker.Type) {
	if pattern == nil || expr == nil {
		return
	}
	if patternList, ok := pattern.(*checker.List); ok {
		if listLiteral, ok := expr.(*checker.ListLiteral); ok {
			for _, element := range listLiteral.Elements {
				bindGenericTypeArgs(patternList.Of(), element.Type(), bindings)
			}
			return
		}
	}
	bindGenericTypeArgs(pattern, expr.Type(), bindings)
}

func backendIRTypeBindings(bindings map[string]checker.Type) map[string]backendir.Type {
	if len(bindings) == 0 {
		return nil
	}
	out := make(map[string]backendir.Type, len(bindings))
	for name, typ := range bindings {
		if strings.TrimSpace(name) == "" || typ == nil {
			continue
		}
		out[name] = lowerCheckerTypeToBackendIR(typ)
	}
	return out
}

func substituteBackendIRTypeVars(t backendir.Type, bindings map[string]backendir.Type) backendir.Type {
	if t == nil || len(bindings) == 0 {
		return t
	}
	switch typed := t.(type) {
	case *backendir.TypeVarType:
		if bound := bindings[typed.Name]; bound != nil {
			return bound
		}
		return typed
	case *backendir.NamedType:
		copy := *typed
		copy.Args = make([]backendir.Type, 0, len(typed.Args))
		for _, arg := range typed.Args {
			copy.Args = append(copy.Args, substituteBackendIRTypeVars(arg, bindings))
		}
		return &copy
	case *backendir.ListType:
		return &backendir.ListType{Elem: substituteBackendIRTypeVars(typed.Elem, bindings)}
	case *backendir.MapType:
		return &backendir.MapType{Key: substituteBackendIRTypeVars(typed.Key, bindings), Value: substituteBackendIRTypeVars(typed.Value, bindings)}
	case *backendir.MaybeType:
		return &backendir.MaybeType{Of: substituteBackendIRTypeVars(typed.Of, bindings)}
	case *backendir.ResultType:
		return &backendir.ResultType{Val: substituteBackendIRTypeVars(typed.Val, bindings), Err: substituteBackendIRTypeVars(typed.Err, bindings)}
	case *backendir.FuncType:
		copy := *typed
		copy.Params = make([]backendir.Type, 0, len(typed.Params))
		for _, param := range typed.Params {
			copy.Params = append(copy.Params, substituteBackendIRTypeVars(param, bindings))
		}
		copy.ParamByRef = append([]bool(nil), typed.ParamByRef...)
		copy.Return = substituteBackendIRTypeVars(typed.Return, bindings)
		return &copy
	default:
		return typed
	}
}

func lowerFunctionCallToBackendIR(call *checker.FunctionCall) backendir.Expr {
	return lowerFunctionCallToBackendIRWithExpected(call, nil)
}

func lowerFunctionCallToBackendIRWithExpected(call *checker.FunctionCall, expected checker.Type) backendir.Expr {
	if call == nil {
		return callExpr("call", literalExpr("nil", "call"))
	}
	name := strings.TrimSpace(call.Name)
	if name == "" {
		name = "anonymous_fn"
	}
	actualReturn := call.ReturnType
	if expected != nil {
		actualReturn = expected
	}
	bindings, typeArgs := inferGenericCallTypeArgBindings(call.Definition(), call.Args, actualReturn)
	args := lowerCallArgsToBackendIRWithBindings(call.Args, call.Definition(), bindings)
	if len(typeArgs) == 0 && len(args) == 0 && shouldInferZeroArgGenericCallTypeArgs(call.Definition(), "", name) {
		typeArgs = inferZeroArgGenericCallTypeArgs(actualReturn)
	}
	return &backendir.CallExpr{
		Callee:   &backendir.IdentExpr{Name: name},
		Args:     args,
		TypeArgs: typeArgs,
	}
}

func lowerModuleFunctionCallToBackendIR(call *checker.ModuleFunctionCall) backendir.Expr {
	return lowerModuleFunctionCallToBackendIRWithExpected(call, nil)
}

func lowerModuleFunctionCallToBackendIRWithExpected(call *checker.ModuleFunctionCall, expected checker.Type) backendir.Expr {
	if call == nil || call.Call == nil {
		return callExpr("module_call", literalExpr("nil", "call"))
	}
	if special := lowerSpecialModuleConstructorToBackendIR(call, nil); special != nil {
		return special
	}
	moduleName := strings.TrimSpace(call.Module)
	if moduleName == "" {
		moduleName = "module"
	}
	funcName := strings.TrimSpace(call.Call.Name)
	if funcName == "" {
		funcName = "fn"
	}
	actualReturn := call.Call.ReturnType
	if expected != nil {
		actualReturn = expected
	}
	bindings, typeArgs := inferGenericCallTypeArgBindings(call.Call.Definition(), call.Call.Args, actualReturn)
	args := lowerCallArgsToBackendIRWithBindings(call.Call.Args, call.Call.Definition(), bindings)
	if len(typeArgs) == 0 && len(args) == 0 && shouldInferZeroArgGenericCallTypeArgs(call.Call.Definition(), moduleName, funcName) {
		typeArgs = inferZeroArgGenericCallTypeArgs(actualReturn)
	}
	return &backendir.CallExpr{
		Callee: &backendir.SelectorExpr{
			Subject: &backendir.IdentExpr{Name: moduleName},
			Name:    funcName,
		},
		Args:     args,
		TypeArgs: typeArgs,
	}
}

func lowerSpecialModuleConstructorToBackendIR(call *checker.ModuleFunctionCall, expected checker.Type) backendir.Expr {
	moduleName := strings.TrimSpace(call.Module)
	funcName := strings.TrimSpace(call.Call.Name)
	switch moduleName {
	case "ard/maybe":
		maybeType, ok := lowerMaybeConstructorType(call.Call.ReturnType, expected)
		if !ok {
			return nil
		}
		switch funcName {
		case "some":
			if len(call.Call.Args) != 1 {
				return nil
			}
			return &backendir.MaybeSomeExpr{Value: lowerExpressionOrOpaque(call.Call.Args[0]), Type: maybeType}
		case "none":
			return &backendir.MaybeNoneExpr{Type: maybeType}
		}
	case "ard/result":
		resultType, ok := lowerResultConstructorType(call.Call.ReturnType, expected)
		if !ok {
			return nil
		}
		switch funcName {
		case "ok":
			if len(call.Call.Args) != 1 {
				return nil
			}
			return &backendir.ResultOkExpr{Value: lowerExpressionOrOpaque(call.Call.Args[0]), Type: resultType}
		case "err":
			if len(call.Call.Args) != 1 {
				return nil
			}
			return &backendir.ResultErrExpr{Value: lowerExpressionOrOpaque(call.Call.Args[0]), Type: resultType}
		}
	}
	return nil
}

func lowerMaybeConstructorType(t checker.Type, expected checker.Type) (backendir.Type, bool) {
	if expectedMaybe, ok := derefMaybeType(expected); ok {
		return &backendir.MaybeType{Of: lowerNestedCheckerTypeToBackendIR(expectedMaybe.Of())}, true
	}
	return lowerMaybeCallReturnType(t)
}

func lowerMaybeCallReturnType(t checker.Type) (backendir.Type, bool) {
	maybe, ok := derefMaybeType(t)
	if !ok || maybe == nil {
		return nil, false
	}
	return &backendir.MaybeType{Of: lowerNestedCheckerTypeToBackendIR(maybe.Of())}, true
}

func lowerResultConstructorType(t checker.Type, expected checker.Type) (backendir.Type, bool) {
	if expectedResult, ok := derefResultType(expected); ok {
		return &backendir.ResultType{
			Val: lowerNestedCheckerTypeToBackendIR(expectedResult.Val()),
			Err: lowerNestedCheckerTypeToBackendIR(expectedResult.Err()),
		}, true
	}
	return lowerResultCallReturnType(t)
}

func lowerResultCallReturnType(t checker.Type) (backendir.Type, bool) {
	result, ok := derefResultType(t)
	if !ok || result == nil {
		return nil, false
	}
	return &backendir.ResultType{
		Val: lowerNestedCheckerTypeToBackendIR(result.Val()),
		Err: lowerNestedCheckerTypeToBackendIR(result.Err()),
	}, true
}

func derefMaybeType(t checker.Type) (*checker.Maybe, bool) {
	if tv, ok := t.(*checker.TypeVar); ok {
		if actual := tv.Actual(); actual != nil {
			return derefMaybeType(actual)
		}
	}
	maybe, ok := t.(*checker.Maybe)
	return maybe, ok && maybe != nil
}

func derefResultType(t checker.Type) (*checker.Result, bool) {
	if tv, ok := t.(*checker.TypeVar); ok {
		if actual := tv.Actual(); actual != nil {
			return derefResultType(actual)
		}
	}
	result, ok := t.(*checker.Result)
	return result, ok && result != nil
}

func lowerBinaryExprToBackendIR(name string, left checker.Expression, right checker.Expression) backendir.Expr {
	return &backendir.CallExpr{
		Callee: &backendir.IdentExpr{Name: name},
		Args: []backendir.Expr{
			lowerExpressionOrOpaque(left),
			lowerExpressionOrOpaque(right),
		},
	}
}

func lowerIfExprToBackendIR(expr *checker.If) backendir.Expr {
	if expr == nil {
		return callExpr("if_expr", literalExpr("nil", "if"))
	}
	resultType := lowerCheckerTypeToBackendIR(expr.Type())
	thenBlock := lowerBlockToBackendIR(expr.Body)
	finalizeFunctionBodyForReturn(thenBlock, resultType)

	var elseBlock *backendir.Block
	if expr.ElseIf != nil {
		nested := lowerIfExprToBackendIR(withElseFallback(expr.ElseIf, expr.Else))
		if isVoidIRType(resultType) {
			elseBlock = &backendir.Block{
				Stmts: []backendir.Stmt{
					&backendir.ExprStmt{Value: nested},
				},
			}
		} else {
			elseBlock = &backendir.Block{
				Stmts: []backendir.Stmt{
					&backendir.ReturnStmt{Value: nested},
				},
			}
		}
	} else if expr.Else != nil {
		elseBlock = lowerBlockToBackendIR(expr.Else)
		finalizeFunctionBodyForReturn(elseBlock, resultType)
	}

	return &backendir.IfExpr{
		Cond: lowerExpressionOrOpaque(expr.Condition),
		Then: thenBlock,
		Else: elseBlock,
		Type: resultType,
	}
}

func lowerBoolMatchExprToBackendIR(match *checker.BoolMatch) backendir.Expr {
	return lowerBoolMatchExprToBackendIRWithExpected(match, nil)
}

func lowerBoolMatchExprToBackendIRWithExpected(match *checker.BoolMatch, expected checker.Type) backendir.Expr {
	return lowerBoolMatchExprToBackendIRWithContext(match, expected, expected)
}

func lowerBoolMatchExprToBackendIRWithContext(match *checker.BoolMatch, expected checker.Type, returnType checker.Type) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "bool")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())
	if expected != nil {
		resultType = lowerCheckerTypeToBackendIR(expected)
	}
	thenBlock := lowerBlockToBackendIRWithContext(match.True, expected, returnType)
	finalizeFunctionBodyForReturn(thenBlock, resultType)
	elseBlock := lowerBlockToBackendIRWithContext(match.False, expected, returnType)
	finalizeFunctionBodyForReturn(elseBlock, resultType)
	return &backendir.IfExpr{
		Cond: lowerExpressionOrOpaque(match.Subject),
		Then: thenBlock,
		Else: elseBlock,
		Type: resultType,
	}
}

func lowerIntMatchExprToBackendIR(match *checker.IntMatch) backendir.Expr {
	return lowerIntMatchExprToBackendIRWithContext(match, nil, nil)
}

func lowerIntMatchExprToBackendIRWithContext(match *checker.IntMatch, expected checker.Type, returnType checker.Type) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "int")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())
	if expected != nil {
		resultType = lowerCheckerTypeToBackendIR(expected)
	}

	subject, setup := matchSubjectExpr(match.Subject, "int")
	body, ok := buildIntMatchIfChain(match, subject, resultType, expected, returnType)
	if !ok {
		// The checker should always produce an int match with at least
		// one branch or a non-void result that gets a synthetic panic
		// catch-all. This branch only fires for malformed checker output,
		// so emit an explicit invariant-failure PanicExpr.
		return invariantMatchFailureExpr(resultType, "int")
	}
	return wrapWithMatchSubjectSetup(body, setup, resultType)
}

func buildIntMatchIfChain(match *checker.IntMatch, subject backendir.Expr, resultType backendir.Type, expected checker.Type, returnType checker.Type) (backendir.Expr, bool) {
	branches := make([]intMatchBranch, 0, len(match.IntCases)+len(match.RangeCases))
	for _, key := range sortedIntCaseKeys(match.IntCases) {
		block := match.IntCases[key]
		if block == nil {
			continue
		}
		branches = append(branches, intMatchBranch{
			Cond: callExpr("eq", subject, literalExpr("int", strconv.Itoa(key))),
			Body: block,
		})
	}
	for _, key := range sortedIntRangeCaseKeys(match.RangeCases) {
		block := match.RangeCases[key]
		if block == nil {
			continue
		}
		branches = append(branches, intMatchBranch{
			Cond: callExpr(
				"and",
				callExpr("int_gte", subject, literalExpr("int", strconv.Itoa(key.Start))),
				callExpr("int_lte", subject, literalExpr("int", strconv.Itoa(key.End))),
			),
			Body: block,
		})
	}
	// Lower the deepest else as a semantic backend IR Block (for catch-all)
	// or as a panic-bearing expression (for non-exhaustive non-void matches)
	// while preserving single-evaluation semantics for unsafe-subject matches.
	var deepestElseBlock *backendir.Block
	var deepestElseExpr backendir.Expr
	if match.CatchAll != nil {
		deepestElseBlock = lowerBlockToBackendIRWithContext(match.CatchAll, expected, returnType)
		finalizeFunctionBodyForReturn(deepestElseBlock, resultType)
	} else if !isVoidIRType(resultType) {
		deepestElseExpr = nonExhaustiveMatchExpr(resultType, "non-exhaustive int match")
	}
	if len(branches) == 0 {
		if deepestElseBlock != nil {
			return lowerBlockAsExpr(match.CatchAll), true
		}
		if deepestElseExpr != nil {
			return deepestElseExpr, true
		}
		return nil, false
	}

	var nested *backendir.IfExpr
	for i := len(branches) - 1; i >= 0; i-- {
		thenBlock := lowerBlockToBackendIRWithContext(branches[i].Body, expected, returnType)
		finalizeFunctionBodyForReturn(thenBlock, resultType)
		var elseBlock *backendir.Block
		switch {
		case nested != nil:
			elseBlock = wrapExprAsIfElseBlock(nested, resultType)
		case deepestElseBlock != nil:
			elseBlock = deepestElseBlock
		case deepestElseExpr != nil:
			elseBlock = wrapExprAsIfElseBlock(deepestElseExpr, resultType)
		}
		nested = &backendir.IfExpr{
			Cond: branches[i].Cond,
			Then: thenBlock,
			Else: elseBlock,
			Type: resultType,
		}
	}
	if nested == nil {
		return nil, false
	}
	return nested, true
}

type intMatchBranch struct {
	Cond backendir.Expr
	Body *checker.Block
}

func canSafelyDuplicateMatchSubject(subject checker.Expression) bool {
	switch subject.(type) {
	case *checker.Identifier, checker.Variable, *checker.Variable, *checker.IntLiteral, *checker.BoolLiteral, *checker.StrLiteral, *checker.FloatLiteral, *checker.ModuleSymbol:
		return true
	default:
		return false
	}
}

// matchSubjectTempPrefix is the prefix for synthetic match-subject
// hoist temporaries. Its first character is a non-ASCII Unicode letter (Greek
// lowercase alpha, U+03B1) which Go accepts in identifiers but Ard's lexer
// cannot produce — Ard restricts identifier starts to ASCII `[A-Za-z_]` (see
// parse/lexer.go isAlpha). This guarantees that synthetic match temps cannot
// collide with any legal user-defined Ard identifier reaching the Go backend,
// so user locals are never silently shadowed or mutated by hoisting.
const matchSubjectTempPrefix = "\u03b1ardMatchSubject_"

// matchSubjectTempName returns a synthetic identifier used to bind a
// non-trivial match subject so it is evaluated only once before branch
// dispatch. The name is namespaced per match shape (int/option/result/enum)
// for readability of generated Go code, and is guaranteed to be unreachable
// from user-written Ard source (see matchSubjectTempPrefix).
func matchSubjectTempName(kind string) string {
	return matchSubjectTempPrefix + strings.TrimSpace(kind)
}

// matchSubjectExpr returns an expression that should be used inside the
// match's lowered branches to refer to its subject, along with the optional
// setup statement that hoists the subject's evaluation.
//
// For trivially duplicable subjects (identifiers/literals), the subject is
// lowered directly with no setup. For non-trivial subjects (calls, complex
// expressions), the subject is bound once to a synthetic temporary so that
// reuse across multiple branch conditions does not duplicate side effects.
func matchSubjectExpr(subject checker.Expression, kind string) (backendir.Expr, backendir.Stmt) {
	if subject == nil {
		return literalExpr("nil", "subject"), nil
	}
	if canSafelyDuplicateMatchSubject(subject) {
		return lowerExpressionOrOpaque(subject), nil
	}
	temp := matchSubjectTempName(kind) + "_" + strings.TrimPrefix(fmt.Sprintf("%p", subject), "0x")
	return &backendir.IdentExpr{Name: temp}, &backendir.AssignStmt{
		Target: temp,
		Value:  lowerExpressionOrOpaque(subject),
	}
}

// wrapWithMatchSubjectSetup wraps a lowered match body in a BlockExpr when a
// subject hoisting setup is required, preserving single-evaluation semantics
// for non-trivial match subjects.
func wrapWithMatchSubjectSetup(body backendir.Expr, setup backendir.Stmt, resultType backendir.Type) backendir.Expr {
	if body == nil {
		return body
	}
	if setup == nil {
		return body
	}
	return &backendir.BlockExpr{
		Setup: []backendir.Stmt{setup},
		Value: body,
		Type:  resultType,
	}
}

func wrapExprAsIfElseBlock(expr backendir.Expr, resultType backendir.Type) *backendir.Block {
	if expr == nil {
		return nil
	}
	if isVoidIRType(resultType) {
		return &backendir.Block{
			Stmts: []backendir.Stmt{
				&backendir.ExprStmt{Value: expr},
			},
		}
	}
	return &backendir.Block{
		Stmts: []backendir.Stmt{
			&backendir.ReturnStmt{Value: expr},
		},
	}
}

func lowerBlockToExpr(block *checker.Block, resultType backendir.Type) backendir.Expr {
	return lowerBlockToExprWithContext(block, resultType, nil, nil)
}

func lowerBlockToExprWithContext(block *checker.Block, resultType backendir.Type, expected checker.Type, returnType checker.Type) backendir.Expr {
	lowered := lowerBlockToBackendIRWithContext(block, expected, returnType)
	finalizeFunctionBodyForReturn(lowered, resultType)
	if lowered == nil || len(lowered.Stmts) == 0 {
		if isVoidIRType(resultType) {
			return literalExpr("void", "()")
		}
		return zeroValueExprForIRType(resultType)
	}
	lastIndex := len(lowered.Stmts) - 1
	if ret, ok := lowered.Stmts[lastIndex].(*backendir.ReturnStmt); ok {
		if len(lowered.Stmts) == 1 {
			return ret.Value
		}
		return &backendir.BlockExpr{Setup: lowered.Stmts[:lastIndex], Value: ret.Value, Type: resultType}
	}
	return &backendir.BlockExpr{Setup: lowered.Stmts, Value: literalExpr("void", "()"), Type: backendir.Void}
}

func zeroValueExprForIRType(t backendir.Type) backendir.Expr {
	switch t.(type) {
	case *backendir.PrimitiveType:
		if t == backendir.BoolType {
			return literalExpr("bool", "false")
		}
		if t == backendir.StrType {
			return literalExpr("str", "")
		}
		return literalExpr("int", "0")
	case *backendir.VoidType:
		return literalExpr("void", "()")
	default:
		return &backendir.PanicExpr{Message: literalExpr("str", "missing conditional match value"), Type: t}
	}
}

func lowerConditionalMatchExprToBackendIR(match *checker.ConditionalMatch) backendir.Expr {
	return lowerConditionalMatchExprToBackendIRWithContext(match, nil, nil)
}

func lowerConditionalMatchExprToBackendIRWithContext(match *checker.ConditionalMatch, expected checker.Type, returnType checker.Type) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "conditional")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())
	if expected != nil {
		resultType = lowerCheckerTypeToBackendIR(expected)
	}

	branches := make([]conditionalMatchBranch, 0, len(match.Cases))
	for _, matchCase := range match.Cases {
		if matchCase.Condition == nil || matchCase.Body == nil {
			continue
		}
		branches = append(branches, conditionalMatchBranch{
			Cond: lowerExpressionOrOpaque(matchCase.Condition),
			Body: matchCase.Body,
		})
	}
	var nested backendir.Expr
	if match.CatchAll != nil {
		nested = lowerBlockToExprWithContext(match.CatchAll, resultType, expected, returnType)
	} else if !isVoidIRType(resultType) {
		nested = nonExhaustiveMatchExpr(resultType, "non-exhaustive conditional match")
	}
	if len(branches) == 0 {
		if nested != nil {
			return nested
		}
		// Branches and catch-all both empty: emit an explicit
		// invariant-failure PanicExpr so misuse is surfaced as a clear
		// runtime panic.
		return invariantMatchFailureExpr(resultType, "conditional")
	}

	for i := len(branches) - 1; i >= 0; i-- {
		thenBlock := lowerBlockToBackendIRWithContext(branches[i].Body, expected, returnType)
		finalizeFunctionBodyForReturn(thenBlock, resultType)
		nested = &backendir.IfExpr{
			Cond: branches[i].Cond,
			Then: thenBlock,
			Else: wrapExprAsIfElseBlock(nested, resultType),
			Type: resultType,
		}
	}
	if nested == nil {
		return invariantMatchFailureExpr(resultType, "conditional")
	}
	return nested
}

type conditionalMatchBranch struct {
	Cond backendir.Expr
	Body *checker.Block
}

func lowerOptionMatchExprToBackendIR(match *checker.OptionMatch) backendir.Expr {
	return lowerOptionMatchExprToBackendIRWithContext(match, nil, nil)
}

func lowerOptionMatchExprToBackendIRWithContext(match *checker.OptionMatch, expected checker.Type, returnType checker.Type) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "option")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())
	if expected != nil {
		resultType = lowerCheckerTypeToBackendIR(expected)
	}
	if match.Some == nil || match.None == nil {
		// Structurally invalid checker output (an option match must always
		// produce both Some and None branches). Emit an explicit
		// invariant-failure PanicExpr so misuse is observable at runtime.
		return invariantMatchFailureExpr(resultType, "option")
	}
	subject, setup := matchSubjectExpr(match.Subject, "option")
	thenBlock := lowerBlockToBackendIRWithContext(match.Some.Body, expected, returnType)
	prependBindingAssign(
		thenBlock,
		match.Some.Body,
		matchPatternName(match.Some.Pattern),
		callExpr(
			"maybe_expect",
			subject,
			literalExpr("str", "expected some"),
		),
	)
	finalizeFunctionBodyForReturn(thenBlock, resultType)
	elseBlock := lowerBlockToBackendIRWithContext(match.None, expected, returnType)
	finalizeFunctionBodyForReturn(elseBlock, resultType)
	body := &backendir.IfExpr{
		Cond: callExpr("maybe_is_some", subject),
		Then: thenBlock,
		Else: elseBlock,
		Type: resultType,
	}
	return wrapWithMatchSubjectSetup(body, setup, resultType)
}

func lowerResultMatchExprToBackendIR(match *checker.ResultMatch) backendir.Expr {
	return lowerResultMatchExprToBackendIRWithContext(match, nil, nil)
}

func lowerResultMatchExprToBackendIRWithContext(match *checker.ResultMatch, expected checker.Type, returnType checker.Type) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "result")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())
	if expected != nil {
		resultType = lowerCheckerTypeToBackendIR(expected)
	}
	if match.Ok == nil || match.Err == nil {
		// Structurally invalid checker output (a result match must always
		// produce both Ok and Err branches). Emit an explicit
		// invariant-failure PanicExpr instead.
		return invariantMatchFailureExpr(resultType, "result")
	}
	subject, setup := matchSubjectExpr(match.Subject, "result")
	thenBlock := lowerBlockToBackendIRWithContext(match.Ok.Body, expected, returnType)
	prependBindingAssign(
		thenBlock,
		match.Ok.Body,
		matchPatternName(match.Ok.Pattern),
		callExpr(
			"result_expect",
			subject,
			literalExpr("str", "expected ok"),
		),
	)
	finalizeFunctionBodyForReturn(thenBlock, resultType)
	elseBlock := lowerBlockToBackendIRWithContext(match.Err.Body, expected, returnType)
	prependBindingAssign(
		elseBlock,
		match.Err.Body,
		matchPatternName(match.Err.Pattern),
		&backendir.CallExpr{
			Callee: &backendir.SelectorExpr{
				Subject: subject,
				Name:    "unwrap_err",
			},
			Args: nil,
		},
	)
	finalizeFunctionBodyForReturn(elseBlock, resultType)
	body := &backendir.IfExpr{
		Cond: callExpr("result_is_ok", subject),
		Then: thenBlock,
		Else: elseBlock,
		Type: resultType,
	}
	return wrapWithMatchSubjectSetup(body, setup, resultType)
}

func lowerEnumMatchExprToBackendIR(match *checker.EnumMatch) backendir.Expr {
	return lowerEnumMatchExprToBackendIRWithContext(match, nil, nil)
}

func lowerEnumMatchExprToBackendIRWithContext(match *checker.EnumMatch, expected checker.Type, returnType checker.Type) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "enum")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())
	if expected != nil {
		resultType = lowerCheckerTypeToBackendIR(expected)
	}

	subject, setup := matchSubjectExpr(match.Subject, "enum")
	subjectTag := &backendir.SelectorExpr{
		Subject: subject,
		Name:    "tag",
	}
	// Lower the deepest else as a semantic backend IR Block (for catch-all)
	// or as a panic-bearing expression (for non-exhaustive non-void matches)
	// while preserving single-evaluation semantics for unsafe-subject matches.
	var deepestElseBlock *backendir.Block
	var deepestElseExpr backendir.Expr
	if match.CatchAll != nil {
		deepestElseBlock = lowerBlockToBackendIRWithContext(match.CatchAll, expected, returnType)
		finalizeFunctionBodyForReturn(deepestElseBlock, resultType)
	} else if !isVoidIRType(resultType) {
		deepestElseExpr = nonExhaustiveMatchExpr(resultType, "non-exhaustive enum match")
	}

	var nested *backendir.IfExpr
	for index := len(match.Cases) - 1; index >= 0; index-- {
		block := match.Cases[index]
		if block == nil {
			continue
		}
		thenBlock := lowerBlockToBackendIRWithContext(block, expected, returnType)
		finalizeFunctionBodyForReturn(thenBlock, resultType)
		var elseBlock *backendir.Block
		switch {
		case nested != nil:
			elseBlock = wrapExprAsIfElseBlock(nested, resultType)
		case deepestElseBlock != nil:
			elseBlock = deepestElseBlock
		case deepestElseExpr != nil:
			elseBlock = wrapExprAsIfElseBlock(deepestElseExpr, resultType)
		}
		nested = &backendir.IfExpr{
			Cond: callExpr("eq", subjectTag, literalExpr("int", strconv.Itoa(index))),
			Then: thenBlock,
			Else: elseBlock,
			Type: resultType,
		}
	}
	if nested == nil {
		// All cases were nil-bodied or empty, leaving no usable branches.
		// Emit an explicit invariant-failure PanicExpr.
		return invariantMatchFailureExpr(resultType, "enum")
	}
	return wrapWithMatchSubjectSetup(nested, setup, resultType)
}

func lowerUnionMatchExprToBackendIR(match *checker.UnionMatch) backendir.Expr {
	return lowerUnionMatchExprToBackendIRWithContext(match, nil, nil)
}

func lowerUnionMatchExprToBackendIRWithContext(match *checker.UnionMatch, expected checker.Type, returnType checker.Type) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "union")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())
	if expected != nil {
		resultType = lowerCheckerTypeToBackendIR(expected)
	}

	cases := make([]backendir.UnionMatchCase, 0, len(match.TypeCases))
	for _, caseName := range sortedStringKeys(match.TypeCases) {
		matchCase := match.TypeCases[caseName]
		if matchCase == nil || matchCase.Body == nil {
			continue
		}
		caseType := unionMatchCaseTypeByName(match, caseName)
		if caseType == nil {
			// Missing case type for a named union case is a structural
			// invariant violation. Emit an explicit invariant-failure
			// PanicExpr instead.
			return invariantMatchFailureExpr(resultType, "union")
		}
		body := lowerBlockToBackendIRWithContext(matchCase.Body, expected, returnType)
		finalizeFunctionBodyForReturn(body, resultType)
		cases = append(cases, backendir.UnionMatchCase{
			Type:    lowerCheckerTypeToBackendIR(caseType),
			Pattern: matchPatternName(matchCase.Pattern),
			Body:    body,
		})
	}
	if len(cases) == 0 {
		// No usable cases at all — surface the invariant violation directly.
		return invariantMatchFailureExpr(resultType, "union")
	}

	var catchAll *backendir.Block
	if match.CatchAll != nil {
		catchAll = lowerBlockToBackendIRWithContext(match.CatchAll, expected, returnType)
		finalizeFunctionBodyForReturn(catchAll, resultType)
	} else if !isVoidIRType(resultType) {
		catchAll = nonExhaustiveMatchBlock("non-exhaustive union match")
	}
	return &backendir.UnionMatchExpr{
		Subject:  lowerExpressionOrOpaque(match.Subject),
		Cases:    cases,
		CatchAll: catchAll,
		Type:     resultType,
	}
}

func unionMatchCaseTypeByName(match *checker.UnionMatch, caseName string) checker.Type {
	if match == nil {
		return nil
	}
	for caseType := range match.TypeCasesByType {
		if caseType != nil && caseType.String() == caseName {
			return caseType
		}
	}
	return nil
}

// invariantMatchFailureExpr produces a typed PanicExpr that fails loudly if
// reached. These paths are only entered for structurally invalid checker
// output (for example, an option/result match missing one of its branches,
// or a union match with no usable cases). Surfacing the violation as an
// explicit PanicExpr keeps the lowering output structurally valid while still
// terminating execution if the unreachable path is somehow reached.
func invariantMatchFailureExpr(resultType backendir.Type, kind string) backendir.Expr {
	if resultType == nil {
		resultType = backendir.Void
	}
	message := fmt.Sprintf("invariant: %s match lowering reached unreachable fallback", strings.TrimSpace(kind))
	return &backendir.PanicExpr{
		Message: literalExpr("str", message),
		Type:    resultType,
	}
}

func nonExhaustiveMatchExpr(resultType backendir.Type, message string) backendir.Expr {
	// Emit the non-exhaustive panic directly as a typed PanicExpr so the
	// surrounding else branch of the lowered match IfExpr-chain can be
	// emitted natively (PanicExpr is natively emittable, whereas an
	// IfExpr-with-no-else of non-void type cannot be). The PanicExpr is
	// expression-positioned and never falls through, so its return type
	// is satisfied by the panic itself.
	return &backendir.PanicExpr{
		Message: literalExpr("str", strings.TrimSpace(message)),
		Type:    resultType,
	}
}

func nonExhaustiveMatchBlock(message string) *backendir.Block {
	return &backendir.Block{
		Stmts: []backendir.Stmt{
			&backendir.ExprStmt{
				Value: &backendir.PanicExpr{
					Message: literalExpr("str", strings.TrimSpace(message)),
					Type:    backendir.Void,
				},
			},
		},
	}
}

func lowerTryOpExprToBackendIR(op *checker.TryOp) backendir.Expr {
	return lowerTryOpExprToBackendIRWithContext(op, nil, nil)
}

func lowerTryOpExprToBackendIRWithExpected(op *checker.TryOp, expected checker.Type) backendir.Expr {
	return lowerTryOpExprToBackendIRWithContext(op, expected, nil)
}

func lowerTryOpExprToBackendIRWithContext(op *checker.TryOp, expected checker.Type, returnType checker.Type) backendir.Expr {
	if op == nil {
		return &backendir.PanicExpr{
			Message: literalExpr("str", "invalid try expression"),
			Type:    backendir.Void,
		}
	}
	kind := ""
	switch op.Kind {
	case checker.TryMaybe:
		kind = "maybe"
	default:
		kind = "result"
	}
	resultType := lowerCheckerTypeToBackendIR(op.Type())
	if op.CatchBlock == nil {
		return &backendir.TryExpr{
			Kind:    kind,
			Subject: lowerExpressionOrOpaque(op.Expr()),
			Catch:   nil,
			Type:    resultType,
		}
	}
	catchExpected := op.CatchBlock.Type()
	if returnType != nil {
		catchExpected = returnType
	} else if expected != nil {
		catchExpected = expected
	}
	catchBlock := lowerBlockToBackendIRWithReturnType(op.CatchBlock, catchExpected)
	// The catch block always early-returns from the enclosing function. Per the
	// checker, the catch block's value type matches the function's return type
	// (whether or not it equals the unwrapped success type). Finalize the catch
	// block so its trailing expression becomes a return statement, mirroring the
	// VM's `OpReturn` after the catch body. Void catch blocks still need an
	// explicit return so execution does not continue into the success unwrap.
	finalizeCatchBlockForReturn(catchBlock, lowerCheckerTypeToBackendIR(catchExpected))
	return &backendir.TryExpr{
		Kind:     kind,
		Subject:  lowerExpressionOrOpaque(op.Expr()),
		CatchVar: strings.TrimSpace(op.CatchVar),
		Catch:    catchBlock,
		Type:     resultType,
	}
}

func prependBindingAssign(block *backendir.Block, source *checker.Block, name string, value backendir.Expr) {
	name = strings.TrimSpace(name)
	if block == nil || value == nil || name == "" || name == "_" {
		return
	}
	prefix := []backendir.Stmt{
		&backendir.BindStmt{
			Name:  name,
			Value: value,
		},
	}
	if source == nil || !usesNameInStatements(source.Stmts, name) {
		prefix = append(prefix, &backendir.AssignStmt{
			Target: "_",
			Value:  &backendir.IdentExpr{Name: name},
		})
	}
	block.Stmts = append(prefix, block.Stmts...)
}

func matchPatternName(pattern *checker.Identifier) string {
	if pattern == nil {
		return ""
	}
	return strings.TrimSpace(pattern.Name)
}

func lowerAssignmentTargetName(expr checker.Expression) string {
	switch target := expr.(type) {
	case *checker.Identifier:
		return target.Name
	case checker.Variable:
		return target.Name()
	case *checker.Variable:
		return target.Name()
	case *checker.InstanceProperty:
		return fmt.Sprintf("%s.%s", expressionDebugName(target.Subject), target.Property)
	default:
		return fmt.Sprintf("<target:%T>", expr)
	}
}

func expressionDebugName(expr checker.Expression) string {
	switch value := expr.(type) {
	case *checker.Identifier:
		return value.Name
	case checker.Variable:
		return value.Name()
	case *checker.Variable:
		return value.Name()
	default:
		return fmt.Sprintf("%T", expr)
	}
}

func callExpr(name string, args ...backendir.Expr) backendir.Expr {
	return callExprWithTypeArgs(name, nil, args...)
}

func callExprWithTypeArgs(name string, typeArgs []backendir.Type, args ...backendir.Expr) backendir.Expr {
	return &backendir.CallExpr{
		Callee:   &backendir.IdentExpr{Name: name},
		Args:     args,
		TypeArgs: typeArgs,
	}
}

func unionListMethodTypeArgs(elementType checker.Type) []backendir.Type {
	if !isNamedUnionType(elementType) {
		return nil
	}
	return []backendir.Type{lowerCheckerTypeToBackendIR(elementType)}
}

func isNamedUnionType(t checker.Type) bool {
	switch typed := t.(type) {
	case *checker.Union:
		return typed != nil && strings.TrimSpace(typed.Name) != ""
	case checker.Union:
		return strings.TrimSpace(typed.Name) != ""
	default:
		return false
	}
}

func literalExpr(kind, value string) backendir.Expr {
	return &backendir.LiteralExpr{
		Kind:  kind,
		Value: value,
	}
}

func typeExpr(t backendir.Type) backendir.Expr {
	return literalExpr("type", backendIRTypeName(t))
}

func sortedIntCaseKeys(values map[int]*checker.Block) []int {
	if len(values) == 0 {
		return nil
	}
	keys := make([]int, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sortedIntRangeCaseKeys(values map[checker.IntRange]*checker.Block) []checker.IntRange {
	if len(values) == 0 {
		return nil
	}
	keys := make([]checker.IntRange, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(left checker.IntRange, right checker.IntRange) int {
		if left.Start != right.Start {
			return left.Start - right.Start
		}
		return left.End - right.End
	})
	return keys
}

func strMethodKindName(kind checker.StrMethodKind) string {
	switch kind {
	case checker.StrSize:
		return "size"
	case checker.StrIsEmpty:
		return "is_empty"
	case checker.StrContains:
		return "contains"
	case checker.StrReplace:
		return "replace"
	case checker.StrReplaceAll:
		return "replace_all"
	case checker.StrSplit:
		return "split"
	case checker.StrStartsWith:
		return "starts_with"
	case checker.StrToStr:
		return "to_str"
	case checker.StrToDyn:
		return "to_dyn"
	case checker.StrTrim:
		return "trim"
	default:
		return "unknown"
	}
}

func intMethodKindName(kind checker.IntMethodKind) string {
	switch kind {
	case checker.IntToStr:
		return "to_str"
	case checker.IntToDyn:
		return "to_dyn"
	default:
		return "unknown"
	}
}

func floatMethodKindName(kind checker.FloatMethodKind) string {
	switch kind {
	case checker.FloatToStr:
		return "to_str"
	case checker.FloatToInt:
		return "to_int"
	case checker.FloatToDyn:
		return "to_dyn"
	default:
		return "unknown"
	}
}

func boolMethodKindName(kind checker.BoolMethodKind) string {
	switch kind {
	case checker.BoolToStr:
		return "to_str"
	case checker.BoolToDyn:
		return "to_dyn"
	default:
		return "unknown"
	}
}

func listMethodKindName(kind checker.ListMethodKind) string {
	switch kind {
	case checker.ListAt:
		return "at"
	case checker.ListPrepend:
		return "prepend"
	case checker.ListPush:
		return "push"
	case checker.ListSet:
		return "set"
	case checker.ListSize:
		return "size"
	case checker.ListSort:
		return "sort"
	case checker.ListSwap:
		return "swap"
	default:
		return "unknown"
	}
}

func mapMethodKindName(kind checker.MapMethodKind) string {
	switch kind {
	case checker.MapKeys:
		return "keys"
	case checker.MapSize:
		return "size"
	case checker.MapGet:
		return "get"
	case checker.MapSet:
		return "set"
	case checker.MapDrop:
		return "drop"
	case checker.MapHas:
		return "has"
	default:
		return "unknown"
	}
}

func maybeMethodKindName(kind checker.MaybeMethodKind) string {
	switch kind {
	case checker.MaybeExpect:
		return "expect"
	case checker.MaybeIsNone:
		return "is_none"
	case checker.MaybeIsSome:
		return "is_some"
	case checker.MaybeOr:
		return "or"
	case checker.MaybeMap:
		return "map"
	case checker.MaybeAndThen:
		return "and_then"
	default:
		return "unknown"
	}
}

func resultMethodKindName(kind checker.ResultMethodKind) string {
	switch kind {
	case checker.ResultExpect:
		return "expect"
	case checker.ResultOr:
		return "or"
	case checker.ResultIsOk:
		return "is_ok"
	case checker.ResultIsErr:
		return "is_err"
	case checker.ResultMap:
		return "map"
	case checker.ResultMapErr:
		return "map_err"
	case checker.ResultAndThen:
		return "and_then"
	default:
		return "unknown"
	}
}

func backendIRTypeName(t backendir.Type) string {
	switch typed := t.(type) {
	case nil:
		return "Unknown"
	case *backendir.PrimitiveType:
		return typed.Name
	case *backendir.DynamicType:
		return "Dynamic"
	case *backendir.VoidType:
		return "Void"
	case *backendir.TypeVarType:
		return "$" + typed.Name
	case *backendir.NamedType:
		if len(typed.Args) == 0 {
			return typed.Name
		}
		parts := make([]string, 0, len(typed.Args))
		for _, arg := range typed.Args {
			parts = append(parts, backendIRTypeName(arg))
		}
		return typed.Name + "<" + strings.Join(parts, ", ") + ">"
	case *backendir.ListType:
		return "[" + backendIRTypeName(typed.Elem) + "]"
	case *backendir.MapType:
		return "[" + backendIRTypeName(typed.Key) + ":" + backendIRTypeName(typed.Value) + "]"
	case *backendir.MaybeType:
		return backendIRTypeName(typed.Of) + "?"
	case *backendir.ResultType:
		return backendIRTypeName(typed.Val) + "!" + backendIRTypeName(typed.Err)
	case *backendir.FuncType:
		params := make([]string, 0, len(typed.Params))
		for _, param := range typed.Params {
			params = append(params, backendIRTypeName(param))
		}
		return "fn(" + strings.Join(params, ",") + ") " + backendIRTypeName(typed.Return)
	default:
		return fmt.Sprintf("%T", t)
	}
}

// lowerNestedCheckerTypeToBackendIR lowers checker types when they appear
// nested inside other type constructors (list element, map key/value,
// maybe inner, result val/err). Union types in such nested positions are
// kept as Dynamic to preserve runtime FFI compatibility — the existing
// extern bridge expects `[]any`/`map[string]any` payloads for
// dynamically-typed extern arguments. Direct (non-nested) union types,
// such as a function parameter or return type whose immediate type IS a
// union, still lower to backend IR NamedType so signature emission can
// reference the concrete union interface.
func lowerNestedCheckerTypeToBackendIR(t checker.Type) backendir.Type {
	switch typed := t.(type) {
	case *checker.Union:
		if typed == nil || strings.TrimSpace(typed.Name) == "" {
			return backendir.Dynamic
		}
	case checker.Union:
		if strings.TrimSpace(typed.Name) == "" {
			return backendir.Dynamic
		}
	}
	return lowerCheckerTypeToBackendIR(t)
}

func lowerTraitTypeToBackendIR(trait *checker.Trait) backendir.Type {
	if trait == nil {
		return backendir.Dynamic
	}
	methods := trait.GetMethods()
	irMethods := make([]backendir.TraitMethod, 0, len(methods))
	for _, method := range methods {
		params, paramByRef := lowerFunctionTypeParamsToBackendIR(method.Parameters)
		irMethods = append(irMethods, backendir.TraitMethod{
			Name: method.Name,
			Type: &backendir.FuncType{
				Params:     params,
				ParamByRef: paramByRef,
				Return:     lowerCheckerTypeToBackendIR(effectiveFunctionReturnType(&method)),
			},
		})
	}
	return &backendir.TraitType{Name: strings.TrimSpace(trait.Name), Methods: irMethods}
}

func lowerFunctionTypeParamsToBackendIR(params []checker.Parameter) ([]backendir.Type, []bool) {
	out := make([]backendir.Type, 0, len(params))
	byRef := make([]bool, 0, len(params))
	for _, param := range params {
		out = append(out, lowerCheckerTypeToBackendIR(param.Type))
		byRef = append(byRef, param.Mutable && mutableParamNeedsPointer(param.Type))
	}
	return out, byRef
}

func lowerCheckerTypeToBackendIR(t checker.Type) backendir.Type {
	if t == nil {
		return backendir.UnknownType
	}

	switch t {
	case checker.Int:
		return backendir.IntType
	case checker.Float:
		return backendir.FloatType
	case checker.Str:
		return backendir.StrType
	case checker.Bool:
		return backendir.BoolType
	case checker.Dynamic:
		return backendir.Dynamic
	case checker.Void:
		return backendir.Void
	}

	switch typed := t.(type) {
	case checker.Trait:
		trait := typed
		return lowerCheckerTypeToBackendIR(&trait)
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			return lowerCheckerTypeToBackendIR(actual)
		}
		name := strings.TrimSpace(typed.Name())
		if name == "" {
			name = "T"
		}
		return &backendir.TypeVarType{Name: name}
	case *checker.List:
		return &backendir.ListType{Elem: lowerNestedCheckerTypeToBackendIR(typed.Of())}
	case *checker.Map:
		return &backendir.MapType{
			Key:   lowerNestedCheckerTypeToBackendIR(typed.Key()),
			Value: lowerNestedCheckerTypeToBackendIR(typed.Value()),
		}
	case *checker.Maybe:
		return &backendir.MaybeType{Of: lowerNestedCheckerTypeToBackendIR(typed.Of())}
	case *checker.Result:
		return &backendir.ResultType{
			Val: lowerNestedCheckerTypeToBackendIR(typed.Val()),
			Err: lowerNestedCheckerTypeToBackendIR(typed.Err()),
		}
	case *checker.FunctionDef:
		params, paramByRef := lowerFunctionTypeParamsToBackendIR(typed.Parameters)
		return &backendir.FuncType{
			Params:     params,
			ParamByRef: paramByRef,
			Return:     lowerCheckerTypeToBackendIR(effectiveFunctionReturnType(typed)),
		}
	case *checker.ExternalFunctionDef:
		params, paramByRef := lowerFunctionTypeParamsToBackendIR(typed.Parameters)
		return &backendir.FuncType{
			Params:     params,
			ParamByRef: paramByRef,
			Return:     lowerCheckerTypeToBackendIR(typed.ReturnType),
		}
	case *checker.Trait:
		return lowerTraitTypeToBackendIR(typed)
	case *checker.StructDef:
		order := structTypeParamOrder(typed)
		if len(order) == 0 {
			return &backendir.NamedType{Name: typed.Name}
		}
		bindings := inferStructBoundTypeArgs(typed, order, nil)
		args := make([]backendir.Type, 0, len(order))
		for _, name := range order {
			bound := bindings[name]
			if tv, ok := bound.(*checker.TypeVar); ok {
				if actual := tv.Actual(); actual != nil {
					bound = actual
				} else {
					bound = nil
				}
			}
			if bound == nil {
				args = append(args, &backendir.TypeVarType{Name: name})
				continue
			}
			args = append(args, lowerCheckerTypeToBackendIR(bound))
		}
		return &backendir.NamedType{Name: typed.Name, Args: args}
	case *checker.Enum:
		return &backendir.NamedType{Name: typed.Name}
	case checker.Union:
		name := strings.TrimSpace(typed.Name)
		if name == "" {
			return backendir.Dynamic
		}
		return &backendir.NamedType{Name: name}
	case *checker.Union:
		if typed == nil {
			return backendir.Dynamic
		}
		name := strings.TrimSpace(typed.Name)
		if name == "" {
			return backendir.Dynamic
		}
		return &backendir.NamedType{Name: name}
	case *checker.ExternType:
		args := make([]backendir.Type, 0, len(typed.TypeArgs))
		for _, typeArg := range typed.TypeArgs {
			args = append(args, lowerCheckerTypeToBackendIR(typeArg))
		}
		name := strings.TrimSpace(typed.Name_)
		if name == "" {
			name = "Extern"
		}
		return &backendir.NamedType{Name: name, Args: args}
	default:
		return &backendir.NamedType{Name: t.String()}
	}
}
