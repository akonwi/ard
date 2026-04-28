package lowering

import (
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/akonwi/ard/checker"
)

const (
	helperImportPath  = "github.com/akonwi/ard/go"
	helperImportAlias = "ardgo"
	stringsImportPath = "strings"
	strconvImportPath = "strconv"
	sortImportPath    = "sort"
)

func collectModuleImports(module checker.Module, projectName string) map[string]string {
	imports := make(map[string]string)
	if module == nil || module.Program() == nil {
		return imports
	}
	for _, stmt := range module.Program().Statements {
		collectImportsFromStatement(stmt, imports, projectName)
	}
	return imports
}

func requiredImportedModulePaths(module checker.Module, projectName string) []string {
	if module == nil || module.Program() == nil {
		return nil
	}
	imports := collectModuleImports(module, projectName)
	paths := make([]string, 0, len(module.Program().Imports))
	for _, mod := range sortedModules(module.Program().Imports) {
		if _, ok := imports[moduleImportPath(projectName, mod.Path())]; ok {
			paths = append(paths, mod.Path())
		}
	}
	return paths
}

func collectImportsFromStatement(stmt checker.Statement, imports map[string]string, projectName string) {
	if fn, ok := stmt.Expr.(*checker.FunctionDef); ok && fn.IsTest {
		return
	}
	if extern, ok := stmt.Expr.(*checker.ExternalFunctionDef); ok {
		imports[helperImportPath] = helperImportAlias
		for _, param := range extern.Parameters {
			collectImportsFromType(param.Type, imports)
		}
		collectImportsFromType(extern.ReturnType, imports)
	}
	if stmt.Expr != nil {
		collectImportsFromExpr(stmt.Expr, imports, projectName)
		collectImportsFromExprTypes(stmt.Expr, imports)
	}
	if stmt.Stmt != nil {
		collectImportsFromNonProducing(stmt.Stmt, imports, projectName)
		collectImportsFromStmtTypes(stmt.Stmt, imports, projectName)
	}
}

func collectImportsFromExprTypes(expr checker.Expression, imports map[string]string) {
	if _, ok := expr.Type().(*checker.FunctionDef); !ok {
		collectImportsFromType(expr.Type(), imports)
	}
	if fn, ok := expr.(*checker.FunctionDef); ok {
		for _, param := range fn.Parameters {
			collectImportsFromType(param.Type, imports)
		}
		collectImportsFromType(fn.ReturnType, imports)
	}
}

func collectImportsFromStmtTypes(stmt checker.NonProducing, imports map[string]string, projectName string) {
	switch s := stmt.(type) {
	case *checker.StructDef:
		for _, fieldName := range sortedStringKeys(s.Fields) {
			collectImportsFromType(s.Fields[fieldName], imports)
		}
		for _, methodName := range sortedStringKeys(s.Methods) {
			method := s.Methods[methodName]
			for _, param := range method.Parameters {
				collectImportsFromType(param.Type, imports)
			}
			collectImportsFromType(method.ReturnType, imports)
			for _, stmt := range method.Body.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
	case *checker.Enum:
		for _, methodName := range sortedStringKeys(s.Methods) {
			method := s.Methods[methodName]
			for _, param := range method.Parameters {
				collectImportsFromType(param.Type, imports)
			}
			collectImportsFromType(method.ReturnType, imports)
			for _, stmt := range method.Body.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
	case *checker.VariableDef:
		if !typeNeedsExplicitVarAnnotation(s.Type()) {
			return
		}
		collectImportsFromType(s.Type(), imports)
	}
}

func collectImportsFromType(t checker.Type, imports map[string]string) {
	switch typed := t.(type) {
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			collectImportsFromType(actual, imports)
		}
	case *checker.Result:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromType(typed.Val(), imports)
		collectImportsFromType(typed.Err(), imports)
	case *checker.Maybe:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromType(typed.Of(), imports)
	case *checker.List:
		collectImportsFromType(typed.Of(), imports)
	case *checker.Map:
		collectImportsFromType(typed.Key(), imports)
		collectImportsFromType(typed.Value(), imports)
	case *checker.StructDef:
		for _, fieldName := range sortedStringKeys(typed.Fields) {
			collectImportsFromType(typed.Fields[fieldName], imports)
		}
	case *checker.FunctionDef:
		for _, param := range typed.Parameters {
			collectImportsFromType(param.Type, imports)
		}
		collectImportsFromType(effectiveFunctionReturnType(typed), imports)
	case *checker.Trait:
		if typed.Name == "ToString" || typed.Name == "Encodable" {
			imports[helperImportPath] = helperImportAlias
		}
		for _, method := range typed.GetMethods() {
			for _, param := range method.Parameters {
				collectImportsFromType(param.Type, imports)
			}
			collectImportsFromType(method.ReturnType, imports)
		}
	}
}

func collectImportsFromNonProducing(stmt checker.NonProducing, imports map[string]string, projectName string) {
	switch s := stmt.(type) {
	case *checker.VariableDef:
		collectImportsFromExpr(s.Value, imports, projectName)
	case *checker.Reassignment:
		collectImportsFromExpr(s.Target, imports, projectName)
		collectImportsFromExpr(s.Value, imports, projectName)
	case *checker.WhileLoop:
		collectImportsFromExpr(s.Condition, imports, projectName)
		for _, stmt := range s.Body.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
	case *checker.ForLoop:
		collectImportsFromExpr(s.Init.Value, imports, projectName)
		collectImportsFromExpr(s.Condition, imports, projectName)
		collectImportsFromExpr(s.Update.Value, imports, projectName)
		for _, stmt := range s.Body.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
	case *checker.ForIntRange:
		collectImportsFromExpr(s.Start, imports, projectName)
		collectImportsFromExpr(s.End, imports, projectName)
		for _, stmt := range s.Body.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
	case *checker.ForInStr:
		collectImportsFromExpr(s.Value, imports, projectName)
		for _, stmt := range s.Body.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
	case *checker.ForInList:
		collectImportsFromExpr(s.List, imports, projectName)
		for _, stmt := range s.Body.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
	case *checker.ForInMap:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromExpr(s.Map, imports, projectName)
		for _, stmt := range s.Body.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
	}
}

func collectImportsFromExpr(expr checker.Expression, imports map[string]string, projectName string) {
	switch v := expr.(type) {
	case *checker.IntAddition:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.IntSubtraction:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.IntMultiplication:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.IntDivision:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.IntModulo:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.FloatAddition:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.FloatSubtraction:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.FloatMultiplication:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.FloatDivision:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.StrAddition:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.IntGreater:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.IntGreaterEqual:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.IntLess:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.IntLessEqual:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.FloatGreater:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.FloatGreaterEqual:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.FloatLess:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.FloatLessEqual:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.Equality:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.And:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.Or:
		collectImportsFromExpr(v.Left, imports, projectName)
		collectImportsFromExpr(v.Right, imports, projectName)
	case *checker.Negation:
		collectImportsFromExpr(v.Value, imports, projectName)
	case *checker.Not:
		collectImportsFromExpr(v.Value, imports, projectName)
	case *checker.FunctionCall:
		if def := v.Definition(); def != nil {
			for _, param := range def.Parameters {
				collectImportsFromType(param.Type, imports)
			}
		}
		for _, arg := range v.Args {
			collectImportsFromExpr(arg, imports, projectName)
		}
	case *checker.StructInstance:
		for _, fieldName := range sortedStringKeys(v.Fields) {
			collectImportsFromExpr(v.Fields[fieldName], imports, projectName)
		}
	case *checker.InstanceProperty:
		collectImportsFromExpr(v.Subject, imports, projectName)
	case *checker.InstanceMethod:
		collectImportsFromExpr(v.Subject, imports, projectName)
		if def := v.Method.Definition(); def != nil {
			for _, param := range def.Parameters {
				collectImportsFromType(param.Type, imports)
			}
		}
		for _, arg := range v.Method.Args {
			collectImportsFromExpr(arg, imports, projectName)
		}
	case *checker.CopyExpression:
		collectImportsFromExpr(v.Expr, imports, projectName)
	case *checker.ListLiteral:
		for _, element := range v.Elements {
			collectImportsFromExpr(element, imports, projectName)
		}
		collectImportsFromType(v.ListType, imports)
	case *checker.ListMethod:
		collectImportsFromExpr(v.Subject, imports, projectName)
		for _, arg := range v.Args {
			collectImportsFromExpr(arg, imports, projectName)
		}
		if v.Kind == checker.ListSort {
			imports[sortImportPath] = "sort"
		}
	case *checker.MapLiteral:
		for i := range v.Keys {
			collectImportsFromExpr(v.Keys[i], imports, projectName)
			collectImportsFromExpr(v.Values[i], imports, projectName)
		}
		collectImportsFromType(v.Type(), imports)
	case *checker.MapMethod:
		if v.Kind == checker.MapKeys {
			imports[helperImportPath] = helperImportAlias
		}
		collectImportsFromExpr(v.Subject, imports, projectName)
		for _, arg := range v.Args {
			collectImportsFromExpr(arg, imports, projectName)
		}
	case *checker.ResultMethod:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromExpr(v.Subject, imports, projectName)
		for _, arg := range v.Args {
			collectImportsFromExpr(arg, imports, projectName)
		}
	case *checker.TryOp:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromExpr(v.Expr(), imports, projectName)
		collectImportsFromType(v.OkType, imports)
		collectImportsFromType(v.ErrType, imports)
		if v.CatchBlock != nil {
			for _, stmt := range v.CatchBlock.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
	case *checker.MaybeMethod:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromExpr(v.Subject, imports, projectName)
		for _, arg := range v.Args {
			collectImportsFromExpr(arg, imports, projectName)
		}
	case *checker.TemplateStr:
		for _, chunk := range v.Chunks {
			collectImportsFromExpr(chunk, imports, projectName)
		}
	case *checker.StrMethod:
		collectImportsFromExpr(v.Subject, imports, projectName)
		for _, arg := range v.Args {
			collectImportsFromExpr(arg, imports, projectName)
		}
		switch v.Kind {
		case checker.StrContains, checker.StrReplace, checker.StrReplaceAll, checker.StrSplit, checker.StrStartsWith, checker.StrTrim:
			imports[stringsImportPath] = "strings"
		}
	case *checker.IntMethod:
		collectImportsFromExpr(v.Subject, imports, projectName)
		if v.Kind == checker.IntToStr {
			imports[strconvImportPath] = "strconv"
		}
	case *checker.FloatMethod:
		collectImportsFromExpr(v.Subject, imports, projectName)
		if v.Kind == checker.FloatToStr {
			imports[strconvImportPath] = "strconv"
		}
	case *checker.BoolMethod:
		collectImportsFromExpr(v.Subject, imports, projectName)
		if v.Kind == checker.BoolToStr {
			imports[strconvImportPath] = "strconv"
		}
	case *checker.ModuleStructInstance:
		imports[moduleImportPath(projectName, v.Module)] = packageNameForModulePath(v.Module)
		for _, fieldName := range sortedStringKeys(v.Property.Fields) {
			collectImportsFromExpr(v.Property.Fields[fieldName], imports, projectName)
		}
	case *checker.ModuleFunctionCall:
		if v.Module == "ard/maybe" || v.Module == "ard/result" {
			imports[helperImportPath] = helperImportAlias
		} else {
			imports[moduleImportPath(projectName, v.Module)] = packageNameForModulePath(v.Module)
		}
		if def := v.Call.Definition(); def != nil {
			for _, param := range def.Parameters {
				collectImportsFromType(param.Type, imports)
			}
		}
		for _, arg := range v.Call.Args {
			collectImportsFromExpr(arg, imports, projectName)
		}
	case *checker.ModuleSymbol:
		imports[moduleImportPath(projectName, v.Module)] = packageNameForModulePath(v.Module)
	case *checker.If:
		collectImportsFromExpr(v.Condition, imports, projectName)
		for _, stmt := range v.Body.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
		if v.ElseIf != nil {
			collectImportsFromExpr(v.ElseIf, imports, projectName)
		}
		if v.Else != nil {
			for _, stmt := range v.Else.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
	case *checker.UnionMatch:
		collectImportsFromExpr(v.Subject, imports, projectName)
		for _, matchCase := range v.TypeCases {
			if matchCase == nil {
				continue
			}
			for _, stmt := range matchCase.Body.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
		if v.CatchAll != nil {
			for _, stmt := range v.CatchAll.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
	case *checker.BoolMatch:
		collectImportsFromExpr(v.Subject, imports, projectName)
		for _, stmt := range v.True.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
		for _, stmt := range v.False.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
	case *checker.IntMatch:
		collectImportsFromExpr(v.Subject, imports, projectName)
		for _, block := range v.IntCases {
			for _, stmt := range block.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
		for _, block := range v.RangeCases {
			for _, stmt := range block.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
		if v.CatchAll != nil {
			for _, stmt := range v.CatchAll.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
	case *checker.EnumMatch:
		collectImportsFromExpr(v.Subject, imports, projectName)
		for _, block := range v.Cases {
			if block == nil {
				continue
			}
			for _, stmt := range block.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
		if v.CatchAll != nil {
			for _, stmt := range v.CatchAll.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
	case *checker.OptionMatch:
		collectImportsFromExpr(v.Subject, imports, projectName)
		for _, stmt := range v.Some.Body.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
		for _, stmt := range v.None.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
	case *checker.ResultMatch:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromExpr(v.Subject, imports, projectName)
		for _, stmt := range v.Ok.Body.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
		for _, stmt := range v.Err.Body.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
	case *checker.ConditionalMatch:
		for _, matchCase := range v.Cases {
			collectImportsFromExpr(matchCase.Condition, imports, projectName)
			for _, stmt := range matchCase.Body.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
		if v.CatchAll != nil {
			for _, stmt := range v.CatchAll.Stmts {
				collectImportsFromStatement(stmt, imports, projectName)
			}
		}
	case *checker.FunctionDef:
		for _, param := range v.Parameters {
			collectImportsFromType(param.Type, imports)
		}
		collectImportsFromType(effectiveFunctionReturnType(v), imports)
		for _, stmt := range v.Body.Stmts {
			collectImportsFromStatement(stmt, imports, projectName)
		}
	case *checker.FiberStart:
		imports[moduleImportPath(projectName, "ard/async")] = packageNameForModulePath("ard/async")
		if fn := v.GetFn(); fn != nil {
			collectImportsFromExpr(fn, imports, projectName)
		}
	case *checker.FiberEval:
		imports[moduleImportPath(projectName, "ard/async")] = packageNameForModulePath("ard/async")
		if fn := v.GetFn(); fn != nil {
			collectImportsFromExpr(fn, imports, projectName)
		}
	case *checker.FiberExecution:
		imports[moduleImportPath(projectName, "ard/async")] = packageNameForModulePath("ard/async")
		if mod := v.GetModule(); mod != nil {
			imports[moduleImportPath(projectName, mod.Path())] = packageNameForModulePath(mod.Path())
		}
	}
}

func sortedModules(imports map[string]checker.Module) []checker.Module {
	paths := make([]string, 0, len(imports))
	for path := range imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	modules := make([]checker.Module, 0, len(paths))
	for _, path := range paths {
		modules = append(modules, imports[path])
	}
	return modules
}

func moduleImportPath(projectName, modulePath string) string {
	if strings.HasPrefix(modulePath, "ard/") {
		relative := stdlibGeneratedRelativePath(modulePath)
		if projectName == "" {
			return relative
		}
		return path.Join(projectName, relative)
	}
	return modulePath
}

func stdlibGeneratedRelativePath(modulePath string) string {
	return path.Join("__ard_stdlib", strings.TrimPrefix(modulePath, "ard/"))
}

func packageNameForModulePath(modulePath string) string {
	base := filepath.Base(strings.TrimSuffix(modulePath, ".ard"))
	name := goName(base, false)
	if isGoPredeclaredIdentifier(name) {
		return name + "_"
	}
	return name
}

func typeNeedsExplicitVarAnnotation(t checker.Type) bool {
	switch t.(type) {
	case *checker.Maybe:
		return true
	default:
		return false
	}
}

func goName(name string, exported bool) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == ' ' || r == ':'
	})
	if len(parts) == 0 {
		return "value"
	}
	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
	}
	for i := range parts {
		if i == 0 && !exported {
			continue
		}
		parts[i] = upperFirst(parts[i])
	}
	result := strings.Join(parts, "")
	if !exported {
		result = lowerFirst(result)
	}
	if result == "" {
		result = "value"
	}
	if isGoKeyword(result) {
		return result + "_"
	}
	return result
}

func upperFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = []rune(strings.ToLower(string(runes[0])))[0]
	return string(runes)
}

func isGoKeyword(value string) bool {
	switch value {
	case "break", "default", "func", "interface", "select",
		"case", "defer", "go", "map", "struct",
		"chan", "else", "goto", "package", "switch",
		"const", "fallthrough", "if", "range", "type",
		"continue", "for", "import", "return", "var":
		return true
	default:
		return false
	}
}

func isGoPredeclaredIdentifier(value string) bool {
	switch value {
	case "any", "bool", "byte", "comparable", "complex64", "complex128",
		"error", "false", "float32", "float64", "iota", "int", "int8",
		"int16", "int32", "int64", "nil", "rune", "string", "true",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	default:
		return false
	}
}
