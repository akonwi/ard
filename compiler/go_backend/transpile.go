package go_backend

import (
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/akonwi/ard/checker"
)

const (
	generatedGoVersion = "1.26.0"
	ardModulePath      = "github.com/akonwi/ard"
	helperImportPath   = ardModulePath + "/go"
	helperImportAlias  = "ardgo"
	stringsImportPath  = "strings"
	strconvImportPath  = "strconv"
	sortImportPath     = "sort"
	osArgsEnvVar       = "ARDGO_OS_ARGS_JSON"
)

type emitter struct {
	module          checker.Module
	packageName     string
	projectName     string
	entrypoint      bool
	imports         map[string]string
	functionNames   map[string]string
	emittedTypes    map[string]struct{}
	tempCounter     int
	fnReturnType    checker.Type
	localScopes     []map[string]string
	pointerScopes   []map[string]bool
	localNameCounts map[string]int
	typeParams      map[string]string
}

func (e *emitter) nextTemp(prefix string) string {
	name := fmt.Sprintf("__ard%s%d", prefix, e.tempCounter)
	e.tempCounter++
	return name
}

func (e *emitter) withFreshLocals(fn func() error) error {
	prevScopes := e.localScopes
	prevPointerScopes := e.pointerScopes
	prevCounts := e.localNameCounts
	e.localScopes = nil
	e.pointerScopes = nil
	e.localNameCounts = make(map[string]int)
	defer func() {
		e.localScopes = prevScopes
		e.pointerScopes = prevPointerScopes
		e.localNameCounts = prevCounts
	}()
	return fn()
}

func (e *emitter) pushScope() {
	e.localScopes = append(e.localScopes, make(map[string]string))
	e.pointerScopes = append(e.pointerScopes, make(map[string]bool))
}

func (e *emitter) popScope() {
	if len(e.localScopes) == 0 {
		return
	}
	e.localScopes = e.localScopes[:len(e.localScopes)-1]
	e.pointerScopes = e.pointerScopes[:len(e.pointerScopes)-1]
}

func (e *emitter) bindLocal(name string) string {
	return e.bindLocalWithPointer(name, false)
}

func (e *emitter) bindLocalWithPointer(name string, pointer bool) string {
	if len(e.localScopes) == 0 {
		e.pushScope()
	}
	base := goName(name, false)
	count := e.localNameCounts[base]
	resolved := base
	if count > 0 {
		resolved = fmt.Sprintf("%s%d", base, count+1)
	}
	e.localNameCounts[base] = count + 1
	e.localScopes[len(e.localScopes)-1][name] = resolved
	e.pointerScopes[len(e.pointerScopes)-1][name] = pointer
	return resolved
}

func (e *emitter) resolveLocal(name string) string {
	for i := len(e.localScopes) - 1; i >= 0; i-- {
		if resolved, ok := e.localScopes[i][name]; ok {
			return resolved
		}
	}
	return goName(name, false)
}

func (e *emitter) isPointerLocal(name string) bool {
	for i := len(e.pointerScopes) - 1; i >= 0; i-- {
		if pointer, ok := e.pointerScopes[i][name]; ok {
			return pointer
		}
	}
	return false
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

func cloneLocalScopes(scopes []map[string]string) []map[string]string {
	if len(scopes) == 0 {
		return nil
	}
	cloned := make([]map[string]string, len(scopes))
	for i := range scopes {
		cloned[i] = make(map[string]string, len(scopes[i]))
		for k, v := range scopes[i] {
			cloned[i][k] = v
		}
	}
	return cloned
}

func clonePointerScopes(scopes []map[string]bool) []map[string]bool {
	if len(scopes) == 0 {
		return nil
	}
	cloned := make([]map[string]bool, len(scopes))
	for i := range scopes {
		cloned[i] = make(map[string]bool, len(scopes[i]))
		for k, v := range scopes[i] {
			cloned[i][k] = v
		}
	}
	return cloned
}

func cloneLocalNameCounts(counts map[string]int) map[string]int {
	if len(counts) == 0 {
		return make(map[string]int)
	}
	cloned := make(map[string]int, len(counts))
	for k, v := range counts {
		cloned[k] = v
	}
	return cloned
}

func cloneTypeParams(params map[string]string) map[string]string {
	if len(params) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(params))
	for k, v := range params {
		cloned[k] = v
	}
	return cloned
}

func (e *emitter) withTypeParams(params map[string]string, fn func() error) error {
	prev := e.typeParams
	e.typeParams = cloneTypeParams(params)
	defer func() {
		e.typeParams = prev
	}()
	return fn()
}

func typeVarName(tv *checker.TypeVar) string {
	if tv == nil {
		return ""
	}
	return tv.Name()
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
	case *checker.FunctionDef:
		for _, param := range typed.Parameters {
			collectTypeParamNames(param.Type, out, seen)
		}
		collectTypeParamNames(effectiveFunctionReturnType(typed), out, seen)
	}
}

func collectTypeParamConstraints(t checker.Type, constraints map[string]string, mapKey bool) {
	if t == nil {
		return
	}
	switch typed := t.(type) {
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			collectTypeParamConstraints(actual, constraints, mapKey)
			return
		}
		name := typeVarName(typed)
		if name == "" {
			return
		}
		if mapKey {
			constraints[name] = "comparable"
			return
		}
		if constraints[name] == "" {
			constraints[name] = "any"
		}
	case *checker.List:
		collectTypeParamConstraints(typed.Of(), constraints, false)
	case *checker.Map:
		collectTypeParamConstraints(typed.Key(), constraints, true)
		collectTypeParamConstraints(typed.Value(), constraints, false)
	case *checker.Maybe:
		collectTypeParamConstraints(typed.Of(), constraints, false)
	case *checker.Result:
		collectTypeParamConstraints(typed.Val(), constraints, false)
		collectTypeParamConstraints(typed.Err(), constraints, false)
	case *checker.Union:
		for _, member := range typed.Types {
			collectTypeParamConstraints(member, constraints, false)
		}
	case *checker.FunctionDef:
		for _, param := range typed.Parameters {
			collectTypeParamConstraints(param.Type, constraints, false)
		}
		collectTypeParamConstraints(effectiveFunctionReturnType(typed), constraints, false)
	}
}

func signatureTypeParams(params []checker.Parameter, returnType checker.Type) ([]string, map[string]string, map[string]string) {
	seen := make(map[string]struct{})
	order := make([]string, 0)
	for _, param := range params {
		collectTypeParamNames(param.Type, &order, seen)
	}
	collectTypeParamNames(returnType, &order, seen)
	if len(order) == 0 {
		return nil, nil, nil
	}
	used := make(map[string]struct{})
	mapping := make(map[string]string, len(order))
	for _, name := range order {
		emitted := goName(name, true)
		if emitted == "Any" {
			emitted = "T"
		}
		if _, ok := used[emitted]; !ok {
			used[emitted] = struct{}{}
			mapping[name] = emitted
			continue
		}
		for i := 2; ; i++ {
			candidate := fmt.Sprintf("%s%d", emitted, i)
			if _, ok := used[candidate]; ok {
				continue
			}
			used[candidate] = struct{}{}
			mapping[name] = candidate
			break
		}
	}
	constraints := make(map[string]string, len(order))
	for _, param := range params {
		collectTypeParamConstraints(param.Type, constraints, false)
	}
	collectTypeParamConstraints(returnType, constraints, false)
	for _, name := range order {
		if constraints[name] == "" {
			constraints[name] = "any"
		}
	}
	return order, mapping, constraints
}

func functionTypeParams(def *checker.FunctionDef) ([]string, map[string]string, map[string]string) {
	if def == nil {
		return nil, nil, nil
	}
	return signatureTypeParams(def.Parameters, effectiveFunctionReturnType(def))
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

func structTypeParams(def *checker.StructDef) ([]string, map[string]string, map[string]string) {
	order := structTypeParamOrder(def)
	if len(order) == 0 {
		return nil, nil, nil
	}
	used := make(map[string]struct{}, len(order))
	mapping := make(map[string]string, len(order))
	for _, name := range order {
		emitted := goName(name, true)
		if emitted == "Any" {
			emitted = "T"
		}
		if _, ok := used[emitted]; !ok {
			used[emitted] = struct{}{}
			mapping[name] = emitted
			continue
		}
		for i := 2; ; i++ {
			candidate := fmt.Sprintf("%s%d", emitted, i)
			if _, ok := used[candidate]; ok {
				continue
			}
			used[candidate] = struct{}{}
			mapping[name] = candidate
			break
		}
	}
	constraints := make(map[string]string, len(order))
	for _, fieldName := range sortedStringKeys(def.Fields) {
		collectTypeParamConstraints(def.Fields[fieldName], constraints, false)
	}
	for _, name := range order {
		if constraints[name] == "" {
			constraints[name] = "any"
		}
	}
	return order, mapping, constraints
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

func formatTypeParamDecls(order []string, mapping map[string]string, constraints map[string]string) string {
	if len(order) == 0 || len(mapping) == 0 {
		return ""
	}
	parts := make([]string, 0, len(order))
	for _, name := range order {
		emitted := mapping[name]
		if emitted == "" {
			continue
		}
		constraint := constraints[name]
		if constraint == "" {
			constraint = "any"
		}
		parts = append(parts, emitted+" "+constraint)
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatTypeParamUses(order []string, mapping map[string]string) string {
	if len(order) == 0 || len(mapping) == 0 {
		return ""
	}
	parts := make([]string, 0, len(order))
	for _, name := range order {
		emitted := mapping[name]
		if emitted == "" {
			continue
		}
		parts = append(parts, emitted)
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func sameImportedType(target checker.Type, sym checker.Type) bool {
	if target == nil || sym == nil {
		return false
	}
	vt := reflect.ValueOf(target)
	vs := reflect.ValueOf(sym)
	if vt.IsValid() && vs.IsValid() && vt.Kind() == reflect.Pointer && vs.Kind() == reflect.Pointer && !vt.IsNil() && !vs.IsNil() {
		if vt.Pointer() == vs.Pointer() {
			return true
		}
	}
	if reflect.TypeOf(target) != reflect.TypeOf(sym) {
		return false
	}
	return target.String() == sym.String()
}

func (e *emitter) importedTypeAlias(name string, t checker.Type) string {
	if e == nil || e.module == nil || e.module.Program() == nil {
		return ""
	}
	for _, mod := range sortedModules(e.module.Program().Imports) {
		sym := mod.Get(name)
		if sym.IsZero() || !sameImportedType(t, sym.Type) {
			continue
		}
		return packageNameForModulePath(mod.Path())
	}
	return ""
}

func (e *emitter) structTypeTemplate(def *checker.StructDef) *checker.StructDef {
	if e == nil || def == nil || e.module == nil || e.module.Program() == nil {
		return def
	}
	for _, stmt := range e.module.Program().Statements {
		switch candidate := stmt.Stmt.(type) {
		case *checker.StructDef:
			if candidate.Name == def.Name {
				return candidate
			}
		case checker.StructDef:
			if candidate.Name == def.Name {
				candidateCopy := candidate
				return &candidateCopy
			}
		}
	}
	for _, mod := range sortedModules(e.module.Program().Imports) {
		sym := mod.Get(def.Name)
		if sym.IsZero() {
			continue
		}
		if candidate, ok := sym.Type.(*checker.StructDef); ok {
			return candidate
		}
	}
	return def
}

func CompileEntrypoint(module checker.Module) ([]byte, error) {
	return compileModuleSource(module, "main", true, "")
}

func compilePackageSource(module checker.Module, projectName string) ([]byte, error) {
	return compileModuleSource(module, packageNameForModulePath(module.Path()), false, projectName)
}

func compileModuleSource(module checker.Module, packageName string, entrypoint bool, projectName string) ([]byte, error) {
	if module == nil || module.Program() == nil {
		return nil, fmt.Errorf("module has no program")
	}
	if !entrypoint && module.Path() == "ard/async" {
		fileIR, err := lowerAsyncModuleFileIR(packageName)
		if err != nil {
			return nil, err
		}
		return renderGoFile(optimizeGoFileIR(fileIR))
	}
	fileIR, err := lowerModuleFileIR(module, packageName, entrypoint, projectName)
	if err != nil {
		return nil, err
	}
	return renderGoFile(optimizeGoFileIR(fileIR))
}

func topLevelExecutableStatements(stmts []checker.Statement) []checker.Statement {
	filtered := make([]checker.Statement, 0, len(stmts))
	for _, stmt := range stmts {
		switch stmt.Expr.(type) {
		case *checker.FunctionDef, *checker.ExternalFunctionDef:
			continue
		}
		switch stmt.Stmt.(type) {
		case *checker.StructDef, checker.StructDef, *checker.Enum, checker.Enum:
			continue
		}
		filtered = append(filtered, stmt)
	}
	return filtered
}

func entrypointMainExpr(stmts []checker.Statement) checker.Expression {
	for _, stmt := range stmts {
		switch def := stmt.Expr.(type) {
		case *checker.FunctionDef:
			if def.IsTest || def.Name != "main" {
				continue
			}
			return def
		case *checker.ExternalFunctionDef:
			if def.Name != "main" {
				continue
			}
			return def
		}
	}
	return nil
}

func collectModuleImports(stmts []checker.Statement, projectName string) map[string]string {
	imports := make(map[string]string)
	for _, stmt := range stmts {
		collectImportsFromStatement(stmt, imports, projectName)
	}
	return imports
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
		} else if !(v.Module == "ard/list" && v.Call != nil && v.Call.Name == "concat") {
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

func sortedImportPaths(imports map[string]string) []string {
	paths := make([]string, 0, len(imports))
	for path := range imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
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

func sortedIntKeys[T any](values map[int]T) []int {
	keys := make([]int, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

func sortedIntRanges(values map[checker.IntRange]*checker.Block) []checker.IntRange {
	ranges := make([]checker.IntRange, 0, len(values))
	for key := range values {
		ranges = append(ranges, key)
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start != ranges[j].Start {
			return ranges[i].Start < ranges[j].Start
		}
		return ranges[i].End < ranges[j].End
	})
	return ranges
}

func sortedStringKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func packageNameForModulePath(modulePath string) string {
	base := filepath.Base(strings.TrimSuffix(modulePath, ".ard"))
	name := goName(base, false)
	if isGoPredeclaredIdentifier(name) {
		return name + "_"
	}
	return name
}

func stdlibGeneratedRelativePath(modulePath string) string {
	return path.Join("__ard_stdlib", strings.TrimPrefix(modulePath, "ard/"))
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

func generatedPathForModule(generatedDir, projectName, modulePath string) (string, error) {
	var relative string
	if strings.HasPrefix(modulePath, "ard/") {
		relative = stdlibGeneratedRelativePath(modulePath)
	} else {
		prefix := projectName + "/"
		if !strings.HasPrefix(modulePath, prefix) {
			return "", fmt.Errorf("module path %q does not match project %q", modulePath, projectName)
		}
		relative = strings.TrimPrefix(modulePath, prefix)
	}
	dir := filepath.Join(generatedDir, filepath.FromSlash(relative))
	base := filepath.Base(relative)
	return filepath.Join(dir, base+".go"), nil
}

func (e *emitter) indexFunctions() {
	usedNames := make(map[string]struct{})
	for _, stmt := range e.module.Program().Statements {
		switch def := stmt.Stmt.(type) {
		case *checker.StructDef:
			usedNames[goName(def.Name, true)] = struct{}{}
		case checker.StructDef:
			usedNames[goName(def.Name, true)] = struct{}{}
		case *checker.Enum:
			usedNames[goName(def.Name, true)] = struct{}{}
		case checker.Enum:
			usedNames[goName(def.Name, true)] = struct{}{}
		}
	}
	for _, stmt := range e.module.Program().Statements {
		switch def := stmt.Expr.(type) {
		case *checker.FunctionDef:
			if def.IsTest {
				continue
			}
			name := goName(def.Name, !def.Private)
			if e.packageName == "main" && name == "main" {
				name = "ardMain"
			}
			resolved := uniquePackageName(name, usedNames)
			e.functionNames[def.Name] = resolved
		case *checker.ExternalFunctionDef:
			name := goName(def.Name, !def.Private)
			if e.packageName == "main" && name == "main" {
				name = "ardMain"
			}
			resolved := uniquePackageName(name, usedNames)
			e.functionNames[def.Name] = resolved
		}
	}
}

func uniquePackageName(base string, used map[string]struct{}) string {
	name := base
	if _, ok := used[name]; !ok {
		used[name] = struct{}{}
		return name
	}
	name = base + "Fn"
	if _, ok := used[name]; !ok {
		used[name] = struct{}{}
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%sFn%d", base, i)
		if _, ok := used[candidate]; !ok {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

func typeNeedsExplicitVarAnnotation(t checker.Type) bool {
	switch t.(type) {
	case *checker.Maybe:
		return true
	default:
		return false
	}
}

func isFunctionLiteralDef(def *checker.FunctionDef) bool {
	if def == nil {
		return false
	}
	return strings.HasPrefix(def.Name, "anon_func_") || strings.HasPrefix(def.Name, "start_func_") || strings.HasPrefix(def.Name, "eval_func_")
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

func lastMeaningfulStatementIndex(stmts []checker.Statement) int {
	for i := len(stmts) - 1; i >= 0; i-- {
		if stmts[i].Break || stmts[i].Expr != nil || stmts[i].Stmt != nil {
			return i
		}
	}
	return -1
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

func resultHasUnresolvedTypeVar(resultType *checker.Result) bool {
	if resultType == nil {
		return true
	}
	if tv, ok := resultType.Val().(*checker.TypeVar); ok && tv.Actual() == nil {
		return true
	}
	if tv, ok := resultType.Err().(*checker.TypeVar); ok && tv.Actual() == nil {
		return true
	}
	return false
}

func maybeHasUnresolvedTypeVar(maybeType *checker.Maybe) bool {
	if maybeType == nil {
		return true
	}
	if tv, ok := maybeType.Of().(*checker.TypeVar); ok && tv.Actual() == nil {
		return true
	}
	return false
}

func inferGenericTypeBindings(original, specialized checker.Type, bindings map[string]checker.Type) {
	if original == nil || specialized == nil {
		return
	}
	if tv, ok := specialized.(*checker.TypeVar); ok {
		if actual := tv.Actual(); actual != nil {
			specialized = actual
		}
	}
	switch originalTyped := original.(type) {
	case *checker.TypeVar:
		if actual := originalTyped.Actual(); actual != nil {
			inferGenericTypeBindings(actual, specialized, bindings)
			return
		}
		name := typeVarName(originalTyped)
		if name == "" {
			return
		}
		if _, ok := bindings[name]; !ok {
			bindings[name] = specialized
		}
	case *checker.List:
		specializedTyped, ok := specialized.(*checker.List)
		if !ok {
			return
		}
		inferGenericTypeBindings(originalTyped.Of(), specializedTyped.Of(), bindings)
	case *checker.Map:
		specializedTyped, ok := specialized.(*checker.Map)
		if !ok {
			return
		}
		inferGenericTypeBindings(originalTyped.Key(), specializedTyped.Key(), bindings)
		inferGenericTypeBindings(originalTyped.Value(), specializedTyped.Value(), bindings)
	case *checker.Maybe:
		specializedTyped, ok := specialized.(*checker.Maybe)
		if !ok {
			return
		}
		inferGenericTypeBindings(originalTyped.Of(), specializedTyped.Of(), bindings)
	case *checker.Result:
		specializedTyped, ok := specialized.(*checker.Result)
		if !ok {
			return
		}
		inferGenericTypeBindings(originalTyped.Val(), specializedTyped.Val(), bindings)
		inferGenericTypeBindings(originalTyped.Err(), specializedTyped.Err(), bindings)
	case *checker.FunctionDef:
		specializedTyped, ok := specialized.(*checker.FunctionDef)
		if !ok {
			return
		}
		for i := 0; i < len(originalTyped.Parameters) && i < len(specializedTyped.Parameters); i++ {
			inferGenericTypeBindings(originalTyped.Parameters[i].Type, specializedTyped.Parameters[i].Type, bindings)
		}
		inferGenericTypeBindings(effectiveFunctionReturnType(originalTyped), effectiveFunctionReturnType(specializedTyped), bindings)
	case *checker.StructDef:
		specializedTyped, ok := specialized.(*checker.StructDef)
		if !ok {
			return
		}
		for _, fieldName := range sortedStringKeys(originalTyped.Fields) {
			specializedField, ok := specializedTyped.Fields[fieldName]
			if !ok {
				continue
			}
			inferGenericTypeBindings(originalTyped.Fields[fieldName], specializedField, bindings)
		}
	case *checker.Union:
		specializedTyped, ok := specialized.(*checker.Union)
		if !ok {
			return
		}
		for i := 0; i < len(originalTyped.Types) && i < len(specializedTyped.Types); i++ {
			inferGenericTypeBindings(originalTyped.Types[i], specializedTyped.Types[i], bindings)
		}
	}
}

func (e *emitter) originalFunctionDef(call *checker.FunctionCall) *checker.FunctionDef {
	if e == nil || e.module == nil || e.module.Program() == nil || call == nil {
		return nil
	}
	for _, stmt := range e.module.Program().Statements {
		switch def := stmt.Expr.(type) {
		case *checker.FunctionDef:
			if def.Name == call.Name {
				return def
			}
		case *checker.ExternalFunctionDef:
			if def.Name == call.Name {
				return functionDefFromType(def)
			}
		}
	}
	return nil
}

func (e *emitter) originalModuleFunctionDef(modulePath string, call *checker.FunctionCall) *checker.FunctionDef {
	if e == nil || e.module == nil || e.module.Program() == nil || call == nil {
		return nil
	}
	mod := e.module.Program().Imports[modulePath]
	if mod == nil {
		return nil
	}
	sym := mod.Get(call.Name)
	if sym.IsZero() {
		return nil
	}
	return functionDefFromType(sym.Type)
}

func functionDefFromType(t checker.Type) *checker.FunctionDef {
	switch fn := t.(type) {
	case *checker.FunctionDef:
		return fn
	case *checker.ExternalFunctionDef:
		return &checker.FunctionDef{
			Name:       fn.Name,
			Parameters: fn.Parameters,
			ReturnType: fn.ReturnType,
			Private:    fn.Private,
		}
	default:
		return nil
	}
}

func moduleFunctionValueName(module checker.Module, def *checker.FunctionDef) string {
	if module == nil || module.Program() == nil || def == nil {
		return ""
	}
	for _, stmt := range module.Program().Statements {
		variableDef, ok := stmt.Stmt.(*checker.VariableDef)
		if !ok {
			continue
		}
		if valueFn, ok := variableDef.Value.(*checker.FunctionDef); ok && (valueFn == def || valueFn.Name == def.Name) {
			return variableDef.Name
		}
		if typeFn, ok := variableDef.Type().(*checker.FunctionDef); ok && (typeFn == def || typeFn.Name == def.Name) {
			return variableDef.Name
		}
	}
	return ""
}

func (e *emitter) resolvedModuleFunctionName(modulePath string, call *checker.FunctionCall) string {
	if call == nil {
		return ""
	}
	name := call.Name
	if def := call.Definition(); def != nil && isFunctionLiteralDef(def) {
		if resolved := moduleFunctionValueName(e.module.Program().Imports[modulePath], def); resolved != "" {
			return resolved
		}
	}
	return name
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
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
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
