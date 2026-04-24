package transpile

import (
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
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
	builder         strings.Builder
	indent          int
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

func (e *emitter) emitLocalValue(name string) string {
	resolved := e.resolveLocal(name)
	if e.isPointerLocal(name) {
		return "(*" + resolved + ")"
	}
	return resolved
}

func (e *emitter) emitLocalTarget(name string) string {
	resolved := e.resolveLocal(name)
	if e.isPointerLocal(name) {
		return "(*" + resolved + ")"
	}
	return resolved
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

func (e *emitter) emitStructType(def *checker.StructDef) (string, error) {
	baseName := goName(def.Name, true)
	alias := e.importedTypeAlias(def.Name, def)
	if alias != "" {
		baseName = alias + "." + goName(def.Name, true)
	}
	template := e.structTypeTemplate(def)
	order := structTypeParamOrder(template)
	if len(order) == 0 {
		return baseName, nil
	}
	bindings := make(map[string]checker.Type, len(order))
	if template != nil {
		for _, fieldName := range sortedStringKeys(template.Fields) {
			specializedField, ok := def.Fields[fieldName]
			if !ok {
				continue
			}
			inferGenericTypeBindings(template.Fields[fieldName], specializedField, bindings)
		}
	}
	bindings = inferStructBoundTypeArgs(def, order, bindings)
	args := make([]string, 0, len(order))
	for _, name := range order {
		if resolved := e.typeParams[name]; resolved != "" {
			args = append(args, resolved)
			continue
		}
		bound := bindings[name]
		if tv, ok := bound.(*checker.TypeVar); ok {
			if actual := tv.Actual(); actual != nil {
				bound = actual
			} else {
				bound = nil
			}
		}
		if bound != nil {
			emitted, err := e.emitTypeArg(bound)
			if err != nil {
				return "", err
			}
			args = append(args, emitted)
			continue
		}
		args = append(args, "any")
	}
	return fmt.Sprintf("%s[%s]", baseName, strings.Join(args, ", ")), nil
}

func (e *emitter) emitType(t checker.Type) (string, error) {
	if structDef, ok := t.(*checker.StructDef); ok {
		return e.emitStructType(structDef)
	}
	return emitTypeWithOptions(t, e.typeParams, func(name string, typ checker.Type) string {
		alias := e.importedTypeAlias(name, typ)
		if alias == "" {
			return goName(name, true)
		}
		return alias + "." + goName(name, true)
	})
}

func (e *emitter) emitTypeArg(t checker.Type) (string, error) {
	typeName, err := e.emitType(t)
	if err != nil {
		return "", err
	}
	if typeName == "" {
		return "struct{}", nil
	}
	return typeName, nil
}

func EmitEntrypoint(module checker.Module) ([]byte, error) {
	return emitModuleSource(module, "main", true, "")
}

func emitPackageSource(module checker.Module, projectName string) ([]byte, error) {
	return emitModuleSource(module, packageNameForModulePath(module.Path()), false, projectName)
}

func emitModuleSource(module checker.Module, packageName string, entrypoint bool, projectName string) ([]byte, error) {
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

func (e *emitter) emitStructDef(def *checker.StructDef) error {
	if _, ok := e.emittedTypes["struct:"+def.Name]; ok {
		return nil
	}
	e.emittedTypes["struct:"+def.Name] = struct{}{}
	order, mapping, constraints := structTypeParams(def)
	decls := formatTypeParamDecls(order, mapping, constraints)
	return e.withTypeParams(mapping, func() error {
		e.line("type " + goName(def.Name, true) + decls + " struct {")
		e.indent++
		fieldNames := sortedStringKeys(def.Fields)
		for _, fieldName := range fieldNames {
			typeName, err := e.emitType(def.Fields[fieldName])
			if err != nil {
				return fmt.Errorf("struct %s: %w", def.Name, err)
			}
			e.line(fmt.Sprintf("%s %s", goName(fieldName, true), typeName))
		}
		e.indent--
		e.line("}")
		methodNames := sortedStringKeys(def.Methods)
		for _, methodName := range methodNames {
			e.line("")
			if err := e.emitStructMethod(def, def.Methods[methodName]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (e *emitter) emitStructMethod(def *checker.StructDef, method *checker.FunctionDef) error {
	order, mapping, _ := structTypeParams(def)
	receiverType := goName(def.Name, true) + formatTypeParamUses(order, mapping)
	return e.emitReceiverMethod(def.Name, receiverType, mapping, method)
}

func (e *emitter) emitEnumMethod(def *checker.Enum, method *checker.FunctionDef) error {
	return e.emitReceiverMethod(def.Name, goName(def.Name, true), nil, method)
}

func (e *emitter) emitReceiverMethod(typeName, receiverType string, typeParams map[string]string, method *checker.FunctionDef) error {
	return e.withFreshLocals(func() error {
		return e.withTypeParams(typeParams, func() error {
			e.pushScope()
			if method.Mutates {
				receiverType = "*" + receiverType
			}
			receiverName := e.bindLocal(method.Receiver)
			params, err := e.emitBoundFunctionParams(method.Parameters)
			if err != nil {
				return fmt.Errorf("method %s.%s: %w", typeName, method.Name, err)
			}
			name := goName(method.Name, !method.Private)
			signature := fmt.Sprintf("func (%s %s) %s(%s)", receiverName, receiverType, name, strings.Join(params, ", "))
			if method.ReturnType != checker.Void {
				returnType, err := e.emitType(method.ReturnType)
				if err != nil {
					return fmt.Errorf("method %s.%s: %w", typeName, method.Name, err)
				}
				signature += " " + returnType
			}
			e.line(signature + " {")
			e.indent++
			prevReturnType := e.fnReturnType
			e.fnReturnType = method.ReturnType
			defer func() {
				e.fnReturnType = prevReturnType
			}()
			if err := e.emitStatements(method.Body.Stmts, method.ReturnType); err != nil {
				return err
			}
			e.indent--
			e.line("}")
			return nil
		})
	})
}

func (e *emitter) emitValueForType(expr checker.Expression, expectedType checker.Type) (string, error) {
	if call, ok := expr.(*checker.ModuleFunctionCall); ok {
		switch call.Module {
		case "ard/maybe":
			return e.emitMaybeModuleCall(call, expectedType)
		case "ard/result":
			return e.emitResultModuleCall(call, expectedType)
		}
	}
	value, err := e.emitExpr(expr)
	if err != nil {
		return "", err
	}
	trait, ok := expectedType.(*checker.Trait)
	if !ok {
		return value, nil
	}
	switch trait.Name {
	case "ToString":
		return fmt.Sprintf("%s.AsToString(%s)", helperImportAlias, value), nil
	case "Encodable":
		return fmt.Sprintf("%s.AsEncodable(%s)", helperImportAlias, value), nil
	default:
		return value, nil
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

func emitCopiedValue(value string, t checker.Type) (string, error) {
	switch typed := t.(type) {
	case *checker.List:
		typeName, err := emitType(typed)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("append(%s(nil), %s...)", typeName, value), nil
	default:
		return value, nil
	}
}

func (e *emitter) emitEnumDef(def *checker.Enum) error {
	if _, ok := e.emittedTypes["enum:"+def.Name]; ok {
		return nil
	}
	e.emittedTypes["enum:"+def.Name] = struct{}{}
	e.line("type " + goName(def.Name, true) + " struct {")
	e.indent++
	e.line("Tag int")
	e.indent--
	e.line("}")
	methodNames := sortedStringKeys(def.Methods)
	for _, methodName := range methodNames {
		e.line("")
		if err := e.emitEnumMethod(def, def.Methods[methodName]); err != nil {
			return err
		}
	}
	return nil
}

func (e *emitter) emitPackageVariable(def *checker.VariableDef) error {
	value, err := e.emitValueForType(def.Value, def.Type())
	if err != nil {
		return err
	}
	name := goName(def.Name, !def.Mutable)
	if !typeNeedsExplicitVarAnnotation(def.Type()) {
		e.line(fmt.Sprintf("var %s = %s", name, value))
		return nil
	}
	typeName, err := e.emitType(def.Type())
	if err != nil || typeName == "" {
		e.line(fmt.Sprintf("var %s = %s", name, value))
		return nil
	}
	e.line(fmt.Sprintf("var %s %s = %s", name, typeName, value))
	return nil
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

func emitFunctionParamsWithOptions(params []checker.Parameter, includeNames bool, typeParams map[string]string, namedTypeRef func(string, checker.Type) string) ([]string, error) {
	parts := make([]string, 0, len(params))
	for _, param := range params {
		typeName, err := emitTypeWithOptions(param.Type, typeParams, namedTypeRef)
		if err != nil {
			return nil, err
		}
		if param.Mutable && mutableParamNeedsPointer(param.Type) {
			typeName = "*" + typeName
		}
		if includeNames {
			parts = append(parts, fmt.Sprintf("%s %s", goName(param.Name, false), typeName))
		} else {
			parts = append(parts, typeName)
		}
	}
	return parts, nil
}

func emitFunctionParams(params []checker.Parameter, includeNames bool) ([]string, error) {
	return emitFunctionParamsWithOptions(params, includeNames, nil, nil)
}

func (e *emitter) emitBoundFunctionParams(params []checker.Parameter) ([]string, error) {
	parts := make([]string, 0, len(params))
	for _, param := range params {
		typeName, err := e.emitType(param.Type)
		if err != nil {
			return nil, err
		}
		usePointer := param.Mutable && mutableParamNeedsPointer(param.Type)
		name := e.bindLocalWithPointer(param.Name, usePointer)
		if usePointer {
			typeName = "*" + typeName
		}
		parts = append(parts, fmt.Sprintf("%s %s", name, typeName))
	}
	return parts, nil
}

func emitFunctionTypeWithOptions(def *checker.FunctionDef, typeParams map[string]string, namedTypeRef func(string, checker.Type) string) (string, error) {
	params, err := emitFunctionParamsWithOptions(def.Parameters, false, typeParams, namedTypeRef)
	if err != nil {
		return "", err
	}
	typeName := fmt.Sprintf("func(%s)", strings.Join(params, ", "))
	returnType := effectiveFunctionReturnType(def)
	if returnType != checker.Void {
		emittedReturnType, err := emitTypeWithOptions(returnType, typeParams, namedTypeRef)
		if err != nil {
			return "", err
		}
		typeName += " " + emittedReturnType
	}
	return typeName, nil
}

func emitFunctionType(def *checker.FunctionDef) (string, error) {
	return emitFunctionTypeWithOptions(def, nil, nil)
}

func (e *emitter) emitFunctionLiteral(def *checker.FunctionDef) (string, error) {
	returnType := effectiveFunctionReturnType(def)
	inner := &emitter{
		module:          e.module,
		packageName:     e.packageName,
		projectName:     e.projectName,
		entrypoint:      e.entrypoint,
		imports:         e.imports,
		functionNames:   e.functionNames,
		emittedTypes:    e.emittedTypes,
		indent:          1,
		tempCounter:     e.tempCounter,
		fnReturnType:    returnType,
		localScopes:     cloneLocalScopes(e.localScopes),
		pointerScopes:   clonePointerScopes(e.pointerScopes),
		localNameCounts: cloneLocalNameCounts(e.localNameCounts),
		typeParams:      cloneTypeParams(e.typeParams),
	}
	inner.pushScope()
	params, err := inner.emitBoundFunctionParams(def.Parameters)
	if err != nil {
		return "", err
	}
	if err := inner.emitStatements(def.Body.Stmts, returnType); err != nil {
		return "", err
	}
	e.tempCounter = inner.tempCounter
	var builder strings.Builder
	builder.WriteString("func(")
	builder.WriteString(strings.Join(params, ", "))
	builder.WriteString(")")
	if returnType != checker.Void {
		emittedReturnType, err := inner.emitType(returnType)
		if err != nil {
			return "", err
		}
		builder.WriteString(" ")
		builder.WriteString(emittedReturnType)
	}
	builder.WriteString(" {\n")
	builder.WriteString(inner.builder.String())
	builder.WriteString("}")
	return builder.String(), nil
}

func (e *emitter) emitExternFunction(def *checker.ExternalFunctionDef) error {
	order, paramsMap, constraints := signatureTypeParams(def.Parameters, def.ReturnType)
	return e.withFreshLocals(func() error {
		return e.withTypeParams(paramsMap, func() error {
			e.pushScope()
			params := make([]string, 0, len(def.Parameters))
			args := make([]string, 0, len(def.Parameters))
			for _, param := range def.Parameters {
				typeName, err := e.emitType(param.Type)
				if err != nil {
					return fmt.Errorf("extern function %s: %w", def.Name, err)
				}
				paramName := e.bindLocal(param.Name)
				params = append(params, fmt.Sprintf("%s %s", paramName, typeName))
				args = append(args, paramName)
			}

			name := e.functionNames[def.Name]
			signature := fmt.Sprintf("func %s%s(%s)", name, formatTypeParamDecls(order, paramsMap, constraints), strings.Join(params, ", "))
			returnType := ""
			if def.ReturnType != checker.Void {
				var err error
				returnType, err = e.emitType(def.ReturnType)
				if err != nil {
					return fmt.Errorf("extern function %s: %w", def.Name, err)
				}
				signature += " " + returnType
			}

			e.line(signature + " {")
			e.indent++
			call := fmt.Sprintf("%s.CallExtern(%q", helperImportAlias, def.ExternalBinding)
			if len(args) > 0 {
				call += ", " + strings.Join(args, ", ")
			}
			call += ")"
			resultBinding := "result"
			if def.ReturnType == checker.Void {
				resultBinding = "_"
			}
			e.line(resultBinding + ", err := " + call)
			e.line("if err != nil {")
			e.indent++
			e.line("panic(err)")
			e.indent--
			e.line("}")
			if def.ReturnType != checker.Void {
				e.line(fmt.Sprintf("return %s.CoerceExtern[%s](result)", helperImportAlias, returnType))
			}
			e.indent--
			e.line("}")
			return nil
		})
	})
}

func (e *emitter) emitFunction(def *checker.FunctionDef) error {
	order, paramsMap, constraints := functionTypeParams(def)
	return e.withFreshLocals(func() error {
		return e.withTypeParams(paramsMap, func() error {
			e.pushScope()
			params, err := e.emitBoundFunctionParams(def.Parameters)
			if err != nil {
				return fmt.Errorf("function %s: %w", def.Name, err)
			}

			returnType := effectiveFunctionReturnType(def)
			signature := fmt.Sprintf("func %s%s(%s)", e.functionNames[def.Name], formatTypeParamDecls(order, paramsMap, constraints), strings.Join(params, ", "))
			if returnType != checker.Void {
				emittedReturnType, err := e.emitType(returnType)
				if err != nil {
					return fmt.Errorf("function %s: %w", def.Name, err)
				}
				signature += " " + emittedReturnType
			}
			e.line(signature + " {")
			e.indent++
			prevReturnType := e.fnReturnType
			e.fnReturnType = returnType
			defer func() {
				e.fnReturnType = prevReturnType
			}()
			if err := e.emitStatements(def.Body.Stmts, returnType); err != nil {
				return err
			}
			e.indent--
			e.line("}")
			return nil
		})
	})
}

func (e *emitter) emitStatements(stmts []checker.Statement, returnType checker.Type) error {
	e.pushScope()
	defer e.popScope()
	lastMeaningful := lastMeaningfulStatementIndex(stmts)
	for i, stmt := range stmts {
		if stmt.Break {
			e.line("break")
			continue
		}
		isLastExpr := i == lastMeaningful && stmt.Expr != nil
		remaining := stmts[i+1:]
		if stmt.Stmt != nil {
			if err := e.emitNonProducing(stmt.Stmt, remaining, returnType); err != nil {
				return err
			}
			continue
		}
		if stmt.Expr == nil {
			continue
		}
		if err := e.emitExpressionStatement(stmt.Expr, returnType, isLastExpr); err != nil {
			return err
		}
	}
	return nil
}

func lastMeaningfulStatementIndex(stmts []checker.Statement) int {
	for i := len(stmts) - 1; i >= 0; i-- {
		if stmts[i].Break || stmts[i].Expr != nil || stmts[i].Stmt != nil {
			return i
		}
	}
	return -1
}

func (e *emitter) emitNonProducing(stmt checker.NonProducing, remaining []checker.Statement, returnType checker.Type) error {
	switch s := stmt.(type) {
	case *checker.VariableDef:
		name := e.bindLocal(s.Name)
		if tryOp, ok := s.Value.(*checker.TryOp); ok {
			if err := e.emitTryOp(tryOp, returnType, func(successValue string) error {
				if typeNeedsExplicitVarAnnotation(s.Type()) {
					typeName, err := e.emitType(s.Type())
					if err != nil || typeName == "" {
						e.line(fmt.Sprintf("%s := %s", name, successValue))
					} else {
						e.line(fmt.Sprintf("var %s %s = %s", name, typeName, successValue))
					}
				} else {
					e.line(fmt.Sprintf("%s := %s", name, successValue))
				}
				return nil
			}, nil); err != nil {
				return err
			}
			if !usesNameInStatements(remaining, s.Name) {
				e.line(fmt.Sprintf("_ = %s", name))
			}
			return nil
		}
		value, err := e.emitValueForType(s.Value, s.Type())
		if err != nil {
			return err
		}
		if typeNeedsExplicitVarAnnotation(s.Type()) {
			typeName, err := e.emitType(s.Type())
			if err != nil || typeName == "" {
				e.line(fmt.Sprintf("%s := %s", name, value))
			} else {
				e.line(fmt.Sprintf("var %s %s = %s", name, typeName, value))
			}
		} else {
			e.line(fmt.Sprintf("%s := %s", name, value))
		}
		if !usesNameInStatements(remaining, s.Name) {
			e.line(fmt.Sprintf("_ = %s", name))
		}
		return nil
	case *checker.Reassignment:
		targetName, err := e.emitAssignmentTarget(s.Target)
		if err != nil {
			return err
		}
		if tryOp, ok := s.Value.(*checker.TryOp); ok {
			return e.emitTryOp(tryOp, returnType, func(successValue string) error {
				e.line(fmt.Sprintf("%s = %s", targetName, successValue))
				return nil
			}, nil)
		}
		value, err := e.emitValueForType(s.Value, s.Target.Type())
		if err != nil {
			return err
		}
		e.line(fmt.Sprintf("%s = %s", targetName, value))
		return nil
	case *checker.WhileLoop:
		return e.emitWhileLoop(s, returnType)
	case *checker.ForLoop:
		return e.emitForLoop(s, returnType)
	case *checker.ForIntRange:
		return e.emitForIntRange(s, returnType)
	case *checker.ForInStr:
		return e.emitForInStr(s, returnType)
	case *checker.ForInList:
		return e.emitForInList(s, returnType)
	case *checker.ForInMap:
		return e.emitForInMap(s, returnType)
	default:
		return fmt.Errorf("unsupported statement: %T", stmt)
	}
}

func (e *emitter) emitForIntRange(loop *checker.ForIntRange, returnType checker.Type) error {
	start, err := e.emitExpr(loop.Start)
	if err != nil {
		return err
	}
	end, err := e.emitExpr(loop.End)
	if err != nil {
		return err
	}
	e.pushScope()
	defer e.popScope()
	cursor := e.bindLocal(loop.Cursor)
	if loop.Index == "" {
		e.line(fmt.Sprintf("for %s := %s; %s <= %s; %s++ {", cursor, start, cursor, end, cursor))
	} else {
		index := e.bindLocal(loop.Index)
		e.line(fmt.Sprintf("for %s, %s := %s, 0; %s <= %s; %s, %s = %s+1, %s+1 {", cursor, index, start, cursor, end, cursor, index, cursor, index))
	}
	e.indent++
	if err := e.emitStatements(loop.Body.Stmts, checker.Void); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitForInStr(loop *checker.ForInStr, returnType checker.Type) error {
	value, err := e.emitExpr(loop.Value)
	if err != nil {
		return err
	}
	e.pushScope()
	defer e.popScope()
	cursor := e.bindLocal(loop.Cursor)
	indexName := "_"
	if loop.Index != "" {
		indexName = e.bindLocal(loop.Index)
	}
	e.line(fmt.Sprintf("for %s, __ardRune := range []rune(%s) {", indexName, value))
	e.indent++
	e.line(fmt.Sprintf("%s := string(__ardRune)", cursor))
	if err := e.emitStatements(loop.Body.Stmts, checker.Void); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitForInList(loop *checker.ForInList, returnType checker.Type) error {
	list, err := e.emitExpr(loop.List)
	if err != nil {
		return err
	}
	e.pushScope()
	defer e.popScope()
	cursor := e.bindLocal(loop.Cursor)
	indexName := "_"
	if loop.Index != "" {
		indexName = e.bindLocal(loop.Index)
	}
	e.line(fmt.Sprintf("for %s, %s := range %s {", indexName, cursor, list))
	e.indent++
	if err := e.emitStatements(loop.Body.Stmts, checker.Void); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitForInMap(loop *checker.ForInMap, returnType checker.Type) error {
	mapExpr, err := e.emitExpr(loop.Map)
	if err != nil {
		return err
	}
	e.pushScope()
	defer e.popScope()
	mapName := e.nextTemp("Map")
	e.line(fmt.Sprintf("%s := %s", mapName, mapExpr))
	key := e.bindLocal(loop.Key)
	val := e.bindLocal(loop.Val)
	e.line(fmt.Sprintf("for _, %s := range %s.MapKeys(%s) {", key, helperImportAlias, mapName))
	e.indent++
	e.line(fmt.Sprintf("%s := %s[%s]", val, mapName, key))
	if err := e.emitStatements(loop.Body.Stmts, checker.Void); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitWhileLoop(loop *checker.WhileLoop, returnType checker.Type) error {
	condition, err := e.emitExpr(loop.Condition)
	if err != nil {
		return err
	}
	e.line("for " + condition + " {")
	e.indent++
	if err := e.emitStatements(loop.Body.Stmts, checker.Void); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitForLoop(loop *checker.ForLoop, returnType checker.Type) error {
	if loop.Init == nil || loop.Update == nil {
		return fmt.Errorf("unsupported for loop: missing init or update")
	}
	e.pushScope()
	defer e.popScope()
	initName := e.bindLocal(loop.Init.Name)
	initValue, err := e.emitExpr(loop.Init.Value)
	if err != nil {
		return err
	}
	condition, err := e.emitExpr(loop.Condition)
	if err != nil {
		return err
	}
	updateTarget, err := e.emitAssignmentTarget(loop.Update.Target)
	if err != nil {
		return err
	}
	updateValue, err := e.emitExpr(loop.Update.Value)
	if err != nil {
		return err
	}
	e.line(fmt.Sprintf("for %s := %s; %s; %s = %s {", initName, initValue, condition, updateTarget, updateValue))
	e.indent++
	if err := e.emitStatements(loop.Body.Stmts, checker.Void); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
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

func (e *emitter) emitExpressionIntoValue(expr checker.Expression, expectedType checker.Type, onValue func(string) error) error {
	if ifExpr, ok := expr.(*checker.If); ok {
		return e.emitIfIntoValue(ifExpr, expectedType, onValue)
	}
	if tryOp, ok := expr.(*checker.TryOp); ok {
		return e.emitTryOp(tryOp, e.fnReturnType, onValue, nil)
	}
	value, err := e.emitValueForType(expr, expectedType)
	if err != nil {
		return err
	}
	copied, err := emitCopiedValue(value, expr.Type())
	if err != nil {
		return err
	}
	return onValue(copied)
}

func (e *emitter) emitBlockValue(block *checker.Block, expectedType checker.Type, onValue func(string) error) error {
	if block == nil {
		return fmt.Errorf("expected value-producing block, got nil")
	}
	if block.Type() == checker.Void {
		return fmt.Errorf("expected value-producing block, got Void")
	}
	e.pushScope()
	defer e.popScope()
	lastMeaningful := lastMeaningfulStatementIndex(block.Stmts)
	for i, stmt := range block.Stmts {
		if stmt.Break {
			e.line("break")
			continue
		}
		remaining := block.Stmts[i+1:]
		if stmt.Stmt != nil {
			if err := e.emitNonProducing(stmt.Stmt, remaining, e.fnReturnType); err != nil {
				return err
			}
			continue
		}
		if stmt.Expr == nil {
			continue
		}
		if i == lastMeaningful {
			return e.emitExpressionIntoValue(stmt.Expr, expectedType, onValue)
		}
		if err := e.emitExpressionStatement(stmt.Expr, e.fnReturnType, false); err != nil {
			return err
		}
	}
	return fmt.Errorf("expected value-producing block")
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

func (e *emitter) emitIfIntoValue(expr *checker.If, expectedType checker.Type, onValue func(string) error) error {
	condition, err := e.emitExpr(expr.Condition)
	if err != nil {
		return err
	}
	e.line("if " + condition + " {")
	e.indent++
	if err := e.emitBlockValue(expr.Body, expectedType, onValue); err != nil {
		return err
	}
	e.indent--
	if expr.ElseIf != nil {
		e.line("} else {")
		e.indent++
		if err := e.emitIfIntoValue(withElseFallback(expr.ElseIf, expr.Else), expectedType, onValue); err != nil {
			return err
		}
		e.indent--
		e.line("}")
		return nil
	}
	if expr.Else != nil {
		e.line("} else {")
		e.indent++
		if err := e.emitBlockValue(expr.Else, expectedType, onValue); err != nil {
			return err
		}
		e.indent--
		e.line("}")
		return nil
	}
	return fmt.Errorf("if expression without else is not supported in value position")
}

func (e *emitter) emitTryExpr(op *checker.TryOp) (string, error) {
	if e.fnReturnType == nil {
		return "", fmt.Errorf("try expressions are only supported in function bodies")
	}
	tempName := e.nextTemp("TryValue")
	typeName, err := e.emitType(op.Type())
	if err != nil {
		return "", err
	}
	e.line(fmt.Sprintf("var %s %s", tempName, typeName))
	assignValue := func(value string) error {
		copied, err := emitCopiedValue(value, op.Type())
		if err != nil {
			return err
		}
		e.line(fmt.Sprintf("%s = %s", tempName, copied))
		return nil
	}
	if err := e.emitTryOp(op, e.fnReturnType, assignValue, nil); err != nil {
		return "", err
	}
	return tempName, nil
}

func (e *emitter) emitTryOp(op *checker.TryOp, returnType checker.Type, onSuccess func(successValue string) error, onCatchValue func(catchValue string) error) error {
	subject, err := e.emitExpr(op.Expr())
	if err != nil {
		return err
	}
	tempName := e.nextTemp("Try")
	e.line(tempName + " := " + subject)

	switch op.Kind {
	case checker.TryResult:
		e.line("if " + tempName + ".IsErr() {")
		e.indent++
		if op.CatchBlock != nil {
			e.pushScope()
			if op.CatchVar != "" && op.CatchVar != "_" {
				catchName := e.bindLocal(op.CatchVar)
				e.line(fmt.Sprintf("%s := %s.UnwrapErr()", catchName, tempName))
				if !usesNameInStatements(op.CatchBlock.Stmts, op.CatchVar) {
					e.line("_ = " + catchName)
				}
			}
			if onCatchValue != nil {
				if op.CatchBlock.Type() == checker.Void {
					e.popScope()
					return fmt.Errorf("void try catch block is not supported in value position")
				}
				if err := e.emitBlockValue(op.CatchBlock, op.CatchBlock.Type(), onCatchValue); err != nil {
					e.popScope()
					return err
				}
			} else {
				if err := e.emitStatements(op.CatchBlock.Stmts, returnType); err != nil {
					e.popScope()
					return err
				}
				if op.CatchBlock.Type() == checker.Void {
					if returnType == nil || returnType == checker.Void {
						e.line("return")
					} else {
						e.popScope()
						return fmt.Errorf("void try catch block is not supported for return type %s", returnType)
					}
				}
			}
			e.popScope()
		} else {
			resultType, ok := e.fnReturnType.(*checker.Result)
			if !ok {
				return fmt.Errorf("try without catch on Result requires function to return a Result type, got %v", e.fnReturnType)
			}
			valueType, err := e.emitTypeArg(resultType.Val())
			if err != nil {
				return err
			}
			errType, err := e.emitTypeArg(resultType.Err())
			if err != nil {
				return err
			}
			e.line(fmt.Sprintf("return %s.Err[%s, %s](%s.UnwrapErr())", helperImportAlias, valueType, errType, tempName))
		}
		e.indent--
		e.line("}")

		if onSuccess != nil {
			successValue, err := emitCopiedValue(tempName+".UnwrapOk()", op.OkType)
			if err != nil {
				return err
			}
			return onSuccess(successValue)
		}
		return nil
	case checker.TryMaybe:
		e.line("if " + tempName + ".IsNone() {")
		e.indent++
		if op.CatchBlock != nil {
			e.pushScope()
			if onCatchValue != nil {
				if op.CatchBlock.Type() == checker.Void {
					e.popScope()
					return fmt.Errorf("void try catch block is not supported in value position")
				}
				if err := e.emitBlockValue(op.CatchBlock, op.CatchBlock.Type(), onCatchValue); err != nil {
					e.popScope()
					return err
				}
			} else {
				if err := e.emitStatements(op.CatchBlock.Stmts, returnType); err != nil {
					e.popScope()
					return err
				}
				if op.CatchBlock.Type() == checker.Void {
					if returnType == nil || returnType == checker.Void {
						e.line("return")
					} else {
						e.popScope()
						return fmt.Errorf("void try catch block is not supported for return type %s", returnType)
					}
				}
			}
			e.popScope()
		} else {
			maybeType, ok := e.fnReturnType.(*checker.Maybe)
			if !ok {
				return fmt.Errorf("try without catch on Maybe requires function to return a Maybe type, got %v", e.fnReturnType)
			}
			innerType, err := e.emitTypeArg(maybeType.Of())
			if err != nil {
				return err
			}
			e.line(fmt.Sprintf("return %s.None[%s]()", helperImportAlias, innerType))
		}
		e.indent--
		e.line("}")

		if onSuccess != nil {
			successValue, err := emitCopiedValue(tempName+".Expect("+strconv.Quote("unreachable none in try success path")+")", op.OkType)
			if err != nil {
				return err
			}
			return onSuccess(successValue)
		}
		return nil
	default:
		return fmt.Errorf("unsupported try kind: %v", op.Kind)
	}
}

func (e *emitter) emitExpressionStatement(expr checker.Expression, returnType checker.Type, isLast bool) error {
	if panicExpr, ok := expr.(*checker.Panic); ok {
		message, err := e.emitExpr(panicExpr.Message)
		if err != nil {
			return err
		}
		e.line("panic(" + message + ")")
		return nil
	}
	if panicExpr, ok := expr.(checker.Panic); ok {
		message, err := e.emitExpr(panicExpr.Message)
		if err != nil {
			return err
		}
		e.line("panic(" + message + ")")
		return nil
	}
	if ifExpr, ok := expr.(*checker.If); ok {
		return e.emitIfStatement(ifExpr, returnType, isLast)
	}
	if tryOp, ok := expr.(*checker.TryOp); ok {
		var onSuccess func(string) error
		if isLast && returnType != nil && returnType != checker.Void {
			onSuccess = func(successValue string) error {
				e.line("return " + successValue)
				return nil
			}
		}
		return e.emitTryOp(tryOp, returnType, onSuccess, nil)
	}
	if !(isLast && returnType != nil && returnType != checker.Void) {
		switch typed := expr.(type) {
		case *checker.BoolMatch:
			return e.emitBoolMatchStatement(typed)
		case *checker.IntMatch:
			return e.emitIntMatchStatement(typed)
		case *checker.EnumMatch:
			return e.emitEnumMatchStatement(typed)
		case *checker.OptionMatch:
			return e.emitOptionMatchStatement(typed)
		case *checker.ResultMatch:
			return e.emitResultMatchStatement(typed)
		case *checker.ConditionalMatch:
			return e.emitConditionalMatchStatement(typed)
		case *checker.UnionMatch:
			return e.emitUnionMatchStatement(typed)
		}
	}
	if isLast && returnType != nil && returnType != checker.Void {
		switch typed := expr.(type) {
		case *checker.BoolMatch:
			value, err := e.emitBoolMatch(typed, returnType)
			if err != nil {
				return err
			}
			e.line("return " + value)
			return nil
		case *checker.IntMatch:
			value, err := e.emitIntMatch(typed, returnType)
			if err != nil {
				return err
			}
			e.line("return " + value)
			return nil
		case *checker.EnumMatch:
			value, err := e.emitEnumMatch(typed, returnType)
			if err != nil {
				return err
			}
			e.line("return " + value)
			return nil
		case *checker.OptionMatch:
			value, err := e.emitOptionMatch(typed, returnType)
			if err != nil {
				return err
			}
			e.line("return " + value)
			return nil
		case *checker.ResultMatch:
			value, err := e.emitResultMatch(typed, returnType)
			if err != nil {
				return err
			}
			e.line("return " + value)
			return nil
		case *checker.ConditionalMatch:
			value, err := e.emitConditionalMatch(typed, returnType)
			if err != nil {
				return err
			}
			e.line("return " + value)
			return nil
		case *checker.UnionMatch:
			value, err := e.emitUnionMatch(typed, returnType)
			if err != nil {
				return err
			}
			e.line("return " + value)
			return nil
		}
		value, err := e.emitValueForType(expr, returnType)
		if err != nil {
			return err
		}
		e.line("return " + value)
		return nil
	}

	value, err := e.emitExpr(expr)
	if err != nil {
		return err
	}
	if isCallExpression(expr) {
		e.line(value)
		return nil
	}
	if returnType == checker.Void && isLast {
		e.line("_ = " + value)
		return nil
	}
	e.line("_ = " + value)
	return nil
}

func (e *emitter) emitIfStatement(expr *checker.If, returnType checker.Type, isLast bool) error {
	condition, err := e.emitExpr(expr.Condition)
	if err != nil {
		return err
	}
	e.line("if " + condition + " {")
	e.indent++
	var branchReturnType checker.Type = checker.Void
	if isLast && returnType != nil && returnType != checker.Void {
		branchReturnType = returnType
	}
	if err := e.emitStatements(expr.Body.Stmts, branchReturnType); err != nil {
		return err
	}
	e.indent--
	if expr.ElseIf != nil {
		e.line("} else {")
		e.indent++
		if err := e.emitIfStatement(withElseFallback(expr.ElseIf, expr.Else), returnType, isLast); err != nil {
			return err
		}
		e.indent--
		e.line("}")
		return nil
	}
	if expr.Else != nil {
		e.line("} else {")
		e.indent++
		if err := e.emitStatements(expr.Else.Stmts, branchReturnType); err != nil {
			return err
		}
		e.indent--
		e.line("}")
		return nil
	}
	if isLast && returnType != nil && returnType != checker.Void {
		return fmt.Errorf("if expression without else is not supported in return position")
	}
	e.line("}")
	return nil
}

func isCallExpression(expr checker.Expression) bool {
	switch expr.(type) {
	case *checker.FunctionCall, *checker.ModuleFunctionCall, *checker.InstanceMethod:
		return true
	default:
		return false
	}
}

func (e *emitter) emitExpr(expr checker.Expression) (string, error) {
	switch v := expr.(type) {
	case *checker.IntLiteral:
		return strconv.Itoa(v.Value), nil
	case *checker.FloatLiteral:
		return strconv.FormatFloat(v.Value, 'g', -1, 64), nil
	case *checker.StrLiteral:
		return strconv.Quote(v.Value), nil
	case *checker.BoolLiteral:
		if v.Value {
			return "true", nil
		}
		return "false", nil
	case *checker.TemplateStr:
		if len(v.Chunks) == 0 {
			return strconv.Quote(""), nil
		}
		chunks := make([]string, 0, len(v.Chunks))
		for _, chunk := range v.Chunks {
			emitted, err := e.emitExpr(chunk)
			if err != nil {
				return "", err
			}
			chunks = append(chunks, emitted)
		}
		return "(" + strings.Join(chunks, " + ") + ")", nil
	case *checker.VoidLiteral:
		return "struct{}{}", nil
	case *checker.EnumVariant:
		return e.emitEnumVariant(v)
	case checker.Panic:
		return e.emitPanicExpr(v.Message, v.Type())
	case *checker.Panic:
		return e.emitPanicExpr(v.Message, v.Type())
	case *checker.CopyExpression:
		return e.emitCopyExpr(v)
	case *checker.ListLiteral:
		elements := make([]string, 0, len(v.Elements))
		for _, element := range v.Elements {
			emitted, err := e.emitExpr(element)
			if err != nil {
				return "", err
			}
			elements = append(elements, emitted)
		}
		typeName, err := e.emitType(v.ListType)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s{%s}", typeName, strings.Join(elements, ", ")), nil
	case *checker.MapLiteral:
		entries := make([]string, 0, len(v.Keys))
		for i := range v.Keys {
			key, err := e.emitExpr(v.Keys[i])
			if err != nil {
				return "", err
			}
			value, err := e.emitExpr(v.Values[i])
			if err != nil {
				return "", err
			}
			entries = append(entries, fmt.Sprintf("%s: %s", key, value))
		}
		typeName, err := e.emitType(v.Type())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s{%s}", typeName, strings.Join(entries, ", ")), nil
	case *checker.BoolMatch:
		return e.emitBoolMatch(v, nil)
	case *checker.IntMatch:
		return e.emitIntMatch(v, nil)
	case *checker.EnumMatch:
		return e.emitEnumMatch(v, nil)
	case *checker.OptionMatch:
		return e.emitOptionMatch(v, nil)
	case *checker.ResultMatch:
		return e.emitResultMatch(v, nil)
	case *checker.ConditionalMatch:
		return e.emitConditionalMatch(v, nil)
	case *checker.UnionMatch:
		return e.emitUnionMatch(v, nil)
	case *checker.TryOp:
		return e.emitTryExpr(v)
	case *checker.ResultMethod:
		return e.emitResultMethod(v)
	case *checker.MaybeMethod:
		return e.emitMaybeMethod(v)
	case *checker.StrMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		switch v.Kind {
		case checker.StrSize:
			return fmt.Sprintf("len(%s)", subject), nil
		case checker.StrIsEmpty:
			return fmt.Sprintf("len(%s) == 0", subject), nil
		case checker.StrContains:
			arg, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("strings.Contains(%s, %s)", subject, arg), nil
		case checker.StrReplace:
			oldValue, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			newValue, err := e.emitExpr(v.Args[1])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("strings.Replace(%s, %s, %s, 1)", subject, oldValue, newValue), nil
		case checker.StrReplaceAll:
			oldValue, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			newValue, err := e.emitExpr(v.Args[1])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("strings.ReplaceAll(%s, %s, %s)", subject, oldValue, newValue), nil
		case checker.StrSplit:
			arg, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("strings.Split(%s, %s)", subject, arg), nil
		case checker.StrStartsWith:
			arg, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("strings.HasPrefix(%s, %s)", subject, arg), nil
		case checker.StrToStr:
			return subject, nil
		case checker.StrTrim:
			return fmt.Sprintf("strings.TrimSpace(%s)", subject), nil
		default:
			return "", fmt.Errorf("unsupported string method: %v", v.Kind)
		}
	case *checker.IntMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		switch v.Kind {
		case checker.IntToStr:
			return fmt.Sprintf("strconv.Itoa(%s)", subject), nil
		default:
			return "", fmt.Errorf("unsupported int method: %v", v.Kind)
		}
	case *checker.FloatMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		switch v.Kind {
		case checker.FloatToStr:
			return fmt.Sprintf("strconv.FormatFloat(%s, 'f', 2, 64)", subject), nil
		case checker.FloatToInt:
			return fmt.Sprintf("func() int { value := float64(%s); return int(value) }()", subject), nil
		default:
			return "", fmt.Errorf("unsupported float method: %v", v.Kind)
		}
	case *checker.BoolMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		switch v.Kind {
		case checker.BoolToStr:
			return fmt.Sprintf("strconv.FormatBool(%s)", subject), nil
		default:
			return "", fmt.Errorf("unsupported bool method: %v", v.Kind)
		}
	case *checker.StructInstance:
		fields := make([]string, 0, len(v.Fields))
		for _, fieldName := range sortedStringKeys(v.Fields) {
			expectedType := v.FieldTypes[fieldName]
			value, err := e.emitValueForType(v.Fields[fieldName], expectedType)
			if err != nil {
				return "", err
			}
			fields = append(fields, fmt.Sprintf("%s: %s", goName(fieldName, true), value))
		}
		typeName, err := e.emitType(v.StructType)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s{%s}", typeName, strings.Join(fields, ", ")), nil
	case *checker.Identifier:
		return e.emitLocalValue(v.Name), nil
	case checker.Variable:
		return e.emitLocalValue(v.Name()), nil
	case *checker.Variable:
		return e.emitLocalValue(v.Name()), nil
	case *checker.IntAddition:
		return e.emitBinary(v.Left, "+", v.Right)
	case *checker.IntSubtraction:
		return e.emitBinary(v.Left, "-", v.Right)
	case *checker.IntMultiplication:
		return e.emitBinary(v.Left, "*", v.Right)
	case *checker.IntDivision:
		return e.emitBinary(v.Left, "/", v.Right)
	case *checker.IntModulo:
		return e.emitBinary(v.Left, "%", v.Right)
	case *checker.FloatAddition:
		return e.emitBinary(v.Left, "+", v.Right)
	case *checker.FloatSubtraction:
		return e.emitBinary(v.Left, "-", v.Right)
	case *checker.FloatMultiplication:
		return e.emitBinary(v.Left, "*", v.Right)
	case *checker.FloatDivision:
		return e.emitBinary(v.Left, "/", v.Right)
	case *checker.StrAddition:
		return e.emitBinary(v.Left, "+", v.Right)
	case *checker.IntGreater:
		return e.emitBinary(v.Left, ">", v.Right)
	case *checker.IntGreaterEqual:
		return e.emitBinary(v.Left, ">=", v.Right)
	case *checker.IntLess:
		return e.emitBinary(v.Left, "<", v.Right)
	case *checker.IntLessEqual:
		return e.emitBinary(v.Left, "<=", v.Right)
	case *checker.FloatGreater:
		return e.emitBinary(v.Left, ">", v.Right)
	case *checker.FloatGreaterEqual:
		return e.emitBinary(v.Left, ">=", v.Right)
	case *checker.FloatLess:
		return e.emitBinary(v.Left, "<", v.Right)
	case *checker.FloatLessEqual:
		return e.emitBinary(v.Left, "<=", v.Right)
	case *checker.Equality:
		return e.emitBinary(v.Left, "==", v.Right)
	case *checker.And:
		return e.emitBinary(v.Left, "&&", v.Right)
	case *checker.Or:
		return e.emitBinary(v.Left, "||", v.Right)
	case *checker.Negation:
		inner, err := e.emitExpr(v.Value)
		if err != nil {
			return "", err
		}
		return "(-" + inner + ")", nil
	case *checker.Not:
		inner, err := e.emitExpr(v.Value)
		if err != nil {
			return "", err
		}
		return "(!(" + inner + "))", nil
	case *checker.FunctionDef:
		return e.emitFunctionLiteral(v)
	case *checker.FunctionCall:
		args, err := e.emitCallArgs(v)
		if err != nil {
			return "", err
		}
		typeArgs, err := e.emitFunctionCallTypeArgsFromDefs(e.originalFunctionDef(v), v.Definition())
		if err != nil {
			return "", err
		}
		name := e.functionNames[v.Name]
		if name == "" {
			name = goName(v.Name, false)
		}
		return fmt.Sprintf("%s%s(%s)", name, typeArgs, strings.Join(args, ", ")), nil
	case *checker.InstanceProperty:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.%s", subject, goName(v.Property, true)), nil
	case *checker.InstanceMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		args, err := e.emitCallArgs(v.Method)
		if err != nil {
			return "", err
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
		return fmt.Sprintf("%s.%s(%s)", subject, methodName, strings.Join(args, ", ")), nil
	case *checker.ListMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		switch v.Kind {
		case checker.ListSize:
			return fmt.Sprintf("len(%s)", subject), nil
		case checker.ListAt:
			if len(v.Args) != 1 {
				return "", fmt.Errorf("list.at expects one arg")
			}
			index, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s[%s]", subject, index), nil
		case checker.ListPush, checker.ListPrepend, checker.ListSet, checker.ListSort, checker.ListSwap:
			return e.emitListMutationExpr(v)
		default:
			return "", fmt.Errorf("unsupported list method: %v", v.Kind)
		}
	case *checker.MapMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		switch v.Kind {
		case checker.MapKeys:
			mapType, ok := v.Subject.Type().(*checker.Map)
			if !ok {
				return "", fmt.Errorf("expected map subject, got %s", v.Subject.Type())
			}
			keysType, err := e.emitType(checker.MakeList(mapType.Key()))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("func() %s { return %s.MapKeys(%s) }()", keysType, helperImportAlias, subject), nil
		case checker.MapSize:
			return fmt.Sprintf("len(%s)", subject), nil
		case checker.MapGet:
			if len(v.Args) != 1 {
				return "", fmt.Errorf("map.get expects one arg")
			}
			key, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			resultType, err := e.emitType(v.Type())
			if err != nil {
				return "", err
			}
			maybeType, ok := v.Type().(*checker.Maybe)
			if !ok {
				return "", fmt.Errorf("expected maybe return type for map.get, got %s", v.Type())
			}
			innerType, err := e.emitTypeArg(maybeType.Of())
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("func() %s { if value, ok := %s[%s]; ok { return %s.Some(value) }; return %s.None[%s]() }()", resultType, subject, key, helperImportAlias, helperImportAlias, innerType), nil
		case checker.MapHas:
			if len(v.Args) != 1 {
				return "", fmt.Errorf("map.has expects one arg")
			}
			key, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("func() bool { _, ok := %s[%s]; return ok }()", subject, key), nil
		case checker.MapSet, checker.MapDrop:
			return e.emitMapMutationExpr(v)
		default:
			return "", fmt.Errorf("unsupported map method: %v", v.Kind)
		}
	case *checker.ModuleStructInstance:
		fields := make([]string, 0, len(v.Property.Fields))
		for _, fieldName := range sortedStringKeys(v.Property.Fields) {
			expectedType := v.FieldTypes[fieldName]
			value, err := e.emitValueForType(v.Property.Fields[fieldName], expectedType)
			if err != nil {
				return "", err
			}
			fields = append(fields, fmt.Sprintf("%s: %s", goName(fieldName, true), value))
		}
		typeName, err := e.emitType(v.StructType)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s{%s}", typeName, strings.Join(fields, ", ")), nil
	case *checker.ModuleFunctionCall:
		if v.Module == "ard/maybe" {
			return e.emitMaybeModuleCall(v, nil)
		}
		if v.Module == "ard/result" {
			return e.emitResultModuleCall(v, nil)
		}
		if v.Module == "ard/list" {
			if emitted, ok, err := e.emitListModuleCall(v); ok || err != nil {
				return emitted, err
			}
		}
		if v.Module == "ard/async" {
			if emitted, ok, err := e.emitAsyncModuleCall(v); ok || err != nil {
				return emitted, err
			}
		}
		args, err := e.emitCallArgs(v.Call)
		if err != nil {
			return "", err
		}
		typeArgs, err := e.emitFunctionCallTypeArgsFromDefs(e.originalModuleFunctionDef(v.Module, v.Call), v.Call.Definition())
		if err != nil {
			return "", err
		}
		alias := packageNameForModulePath(v.Module)
		name := goName(e.resolvedModuleFunctionName(v.Module, v.Call), true)
		return fmt.Sprintf("%s.%s%s(%s)", alias, name, typeArgs, strings.Join(args, ", ")), nil
	case *checker.ModuleSymbol:
		alias := packageNameForModulePath(v.Module)
		name := goName(v.Symbol.Name, true)
		return fmt.Sprintf("%s.%s", alias, name), nil
	case *checker.FiberStart:
		return e.emitFiberStart(v)
	case *checker.FiberEval:
		return e.emitFiberEval(v)
	case *checker.FiberExecution:
		return e.emitFiberExecution(v)
	default:
		return "", fmt.Errorf("unsupported expression: %T", expr)
	}
}

func (e *emitter) emitAssignmentTarget(expr checker.Expression) (string, error) {
	switch target := expr.(type) {
	case *checker.Identifier:
		return e.emitLocalTarget(target.Name), nil
	case checker.Variable:
		return e.emitLocalTarget(target.Name()), nil
	case *checker.Variable:
		return e.emitLocalTarget(target.Name()), nil
	case *checker.InstanceProperty:
		subject, err := e.emitAssignmentTarget(target.Subject)
		if err != nil {
			subjectExpr, exprErr := e.emitBareExpr(target.Subject)
			if exprErr != nil {
				return "", exprErr
			}
			subject = subjectExpr
		}
		return fmt.Sprintf("%s.%s", subject, goName(target.Property, true)), nil
	default:
		return "", fmt.Errorf("unsupported reassignment target: %T", expr)
	}
}

func (e *emitter) emitBareExpr(expr checker.Expression) (string, error) {
	switch v := expr.(type) {
	case *checker.Identifier:
		return e.emitLocalValue(v.Name), nil
	case checker.Variable:
		return e.emitLocalValue(v.Name()), nil
	case *checker.Variable:
		return e.emitLocalValue(v.Name()), nil
	case *checker.InstanceProperty:
		subject, err := e.emitBareExpr(v.Subject)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.%s", subject, goName(v.Property, true)), nil
	default:
		return "", fmt.Errorf("unsupported bare expression: %T", expr)
	}
}

func (e *emitter) emitCopyExpr(copy *checker.CopyExpression) (string, error) {
	inner, err := e.emitExpr(copy.Expr)
	if err != nil {
		return "", err
	}
	switch typed := copy.Type_.(type) {
	case *checker.List:
		typeName, err := e.emitType(typed)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("append(%s(nil), %s...)", typeName, inner), nil
	default:
		return inner, nil
	}
}

func (e *emitter) emitListMutationExpr(method *checker.ListMethod) (string, error) {
	target, err := e.emitAssignmentTarget(method.Subject)
	if err != nil {
		return "", err
	}
	subjectType, ok := method.Subject.Type().(*checker.List)
	if !ok {
		return "", fmt.Errorf("expected list subject, got %s", method.Subject.Type())
	}
	typeName, err := e.emitType(subjectType)
	if err != nil {
		return "", err
	}

	switch method.Kind {
	case checker.ListPush:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("list.push expects one arg")
		}
		value, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() %s { %s = append(%s, %s); return %s }()", typeName, target, target, value, target), nil
	case checker.ListPrepend:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("list.prepend expects one arg")
		}
		value, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() %s { %s = append(%s{%s}, %s...); return %s }()", typeName, target, typeName, value, target, target), nil
	case checker.ListSet:
		if len(method.Args) != 2 {
			return "", fmt.Errorf("list.set expects two args")
		}
		index, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		value, err := e.emitExpr(method.Args[1])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() bool { if %s >= 0 && %s < len(%s) { %s[%s] = %s; return true }; return false }()", index, index, target, target, index, value), nil
	case checker.ListSort:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("list.sort expects one arg")
		}
		cmp, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() struct{} { sort.SliceStable(%s, func(i, j int) bool { return %s(%s[i], %s[j]) }); return struct{}{} }()", target, cmp, target, target), nil
	case checker.ListSwap:
		if len(method.Args) != 2 {
			return "", fmt.Errorf("list.swap expects two args")
		}
		left, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		right, err := e.emitExpr(method.Args[1])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() struct{} { %s[%s], %s[%s] = %s[%s], %s[%s]; return struct{}{} }()", target, left, target, right, target, right, target, left), nil
	default:
		return "", fmt.Errorf("unsupported mutable list method: %v", method.Kind)
	}
}

func (e *emitter) emitEnumVariant(variant *checker.EnumVariant) (string, error) {
	if variant != nil && variant.EnumType != nil {
		if enumType, ok := variant.EnumType.(*checker.Enum); ok && len(enumType.Methods) > 0 {
			typeName, err := e.emitTypeArg(enumType)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s{Tag: %d}", typeName, variant.Discriminant), nil
		}
	}
	return fmt.Sprintf("struct{ Tag int }{Tag: %d}", variant.Discriminant), nil
}

func matchReturnType(expected, fallback checker.Type) checker.Type {
	if expected != nil {
		return expected
	}
	return fallback
}

func (e *emitter) emitBoolMatchStatement(match *checker.BoolMatch) error {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return err
	}
	e.line("if " + subject + " {")
	e.indent++
	if err := e.emitStatements(match.True.Stmts, checker.Void); err != nil {
		return err
	}
	e.indent--
	e.line("} else {")
	e.indent++
	if err := e.emitStatements(match.False.Stmts, checker.Void); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitIntMatchStatement(match *checker.IntMatch) error {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return err
	}
	e.line("switch {")
	e.indent++
	for _, value := range sortedIntKeys(match.IntCases) {
		block := match.IntCases[value]
		e.line(fmt.Sprintf("case %s == %d:", subject, value))
		e.indent++
		if err := e.emitStatements(block.Stmts, checker.Void); err != nil {
			return err
		}
		e.indent--
	}
	for _, intRange := range sortedIntRanges(match.RangeCases) {
		block := match.RangeCases[intRange]
		e.line(fmt.Sprintf("case %s >= %d && %s <= %d:", subject, intRange.Start, subject, intRange.End))
		e.indent++
		if err := e.emitStatements(block.Stmts, checker.Void); err != nil {
			return err
		}
		e.indent--
	}
	if match.CatchAll != nil {
		e.line("default:")
		e.indent++
		if err := e.emitStatements(match.CatchAll.Stmts, checker.Void); err != nil {
			return err
		}
		e.indent--
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitConditionalMatchStatement(match *checker.ConditionalMatch) error {
	for i, matchCase := range match.Cases {
		condition, err := e.emitExpr(matchCase.Condition)
		if err != nil {
			return err
		}
		prefix := "if"
		if i > 0 {
			prefix = "} else if"
		}
		e.line(prefix + " " + condition + " {")
		e.indent++
		if err := e.emitStatements(matchCase.Body.Stmts, checker.Void); err != nil {
			return err
		}
		e.indent--
	}
	if match.CatchAll != nil {
		if len(match.Cases) == 0 {
			e.line("{")
		} else {
			e.line("} else {")
		}
		e.indent++
		if err := e.emitStatements(match.CatchAll.Stmts, checker.Void); err != nil {
			return err
		}
		e.indent--
	}
	if len(match.Cases) > 0 || match.CatchAll != nil {
		e.line("}")
	}
	return nil
}

func (e *emitter) emitOptionMatchStatement(match *checker.OptionMatch) error {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return err
	}
	maybeName := e.nextTemp("Maybe")
	e.line(maybeName + " := " + subject)
	e.line("if " + maybeName + ".IsSome() {")
	e.indent++
	e.pushScope()
	if match.Some != nil && match.Some.Pattern != nil {
		patternName := e.bindLocal(match.Some.Pattern.Name)
		e.line(fmt.Sprintf("%s := %s.Expect(%q)", patternName, maybeName, "unreachable none in maybe match"))
		if !usesNameInStatements(match.Some.Body.Stmts, match.Some.Pattern.Name) {
			e.line("_ = " + patternName)
		}
	}
	if err := e.emitStatements(match.Some.Body.Stmts, checker.Void); err != nil {
		e.popScope()
		return err
	}
	e.popScope()
	e.indent--
	e.line("} else {")
	e.indent++
	if err := e.emitStatements(match.None.Stmts, checker.Void); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitResultMatchStatement(match *checker.ResultMatch) error {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return err
	}
	resultName := e.nextTemp("Result")
	e.line(resultName + " := " + subject)
	e.line("if " + resultName + ".IsOk() {")
	e.indent++
	e.pushScope()
	if match.Ok != nil && match.Ok.Pattern != nil {
		boundValue, err := emitCopiedValue(resultName+".UnwrapOk()", match.OkType)
		if err != nil {
			e.popScope()
			return err
		}
		okName := e.bindLocal(match.Ok.Pattern.Name)
		e.line(fmt.Sprintf("%s := %s", okName, boundValue))
		if !usesNameInStatements(match.Ok.Body.Stmts, match.Ok.Pattern.Name) {
			e.line("_ = " + okName)
		}
	}
	if err := e.emitStatements(match.Ok.Body.Stmts, checker.Void); err != nil {
		e.popScope()
		return err
	}
	e.popScope()
	e.indent--
	e.line("} else {")
	e.indent++
	e.pushScope()
	if match.Err != nil && match.Err.Pattern != nil {
		errName := e.bindLocal(match.Err.Pattern.Name)
		e.line(fmt.Sprintf("%s := %s.UnwrapErr()", errName, resultName))
		if !usesNameInStatements(match.Err.Body.Stmts, match.Err.Pattern.Name) {
			e.line("_ = " + errName)
		}
	}
	if err := e.emitStatements(match.Err.Body.Stmts, checker.Void); err != nil {
		e.popScope()
		return err
	}
	e.popScope()
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitEnumMatchStatement(match *checker.EnumMatch) error {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return err
	}
	e.line("switch " + subject + ".Tag {")
	e.indent++
	discriminants := make([]int, 0, len(match.DiscriminantToIndex))
	for discriminant := range match.DiscriminantToIndex {
		discriminants = append(discriminants, discriminant)
	}
	sort.Ints(discriminants)
	for _, discriminant := range discriminants {
		idx := match.DiscriminantToIndex[discriminant]
		if idx < 0 || int(idx) >= len(match.Cases) || match.Cases[idx] == nil {
			continue
		}
		e.line(fmt.Sprintf("case %d:", discriminant))
		e.indent++
		if err := e.emitStatements(match.Cases[idx].Stmts, checker.Void); err != nil {
			return err
		}
		e.indent--
	}
	if match.CatchAll != nil {
		e.line("default:")
		e.indent++
		if err := e.emitStatements(match.CatchAll.Stmts, checker.Void); err != nil {
			return err
		}
		e.indent--
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitUnionMatchStatement(match *checker.UnionMatch) error {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return err
	}
	subjectName := e.nextTemp("Union")
	e.line("switch " + subjectName + " := any(" + subject + ").(type) {")
	e.indent++
	caseNames := sortedStringKeys(match.TypeCases)
	for _, caseName := range caseNames {
		matchCase := match.TypeCases[caseName]
		if matchCase == nil {
			continue
		}
		caseType := checker.Type(nil)
		for t := range match.TypeCasesByType {
			if t.String() == caseName {
				caseType = t
				break
			}
		}
		if caseType == nil {
			return fmt.Errorf("missing union case type for %s", caseName)
		}
		typeName, err := e.emitTypeArg(caseType)
		if err != nil {
			return err
		}
		e.line("case " + typeName + ":")
		e.indent++
		e.pushScope()
		if matchCase.Pattern != nil {
			boundName := e.bindLocal(matchCase.Pattern.Name)
			e.line(fmt.Sprintf("%s := %s", boundName, subjectName))
			if !usesNameInStatements(matchCase.Body.Stmts, matchCase.Pattern.Name) {
				e.line("_ = " + boundName)
			}
		}
		if err := e.emitStatements(matchCase.Body.Stmts, checker.Void); err != nil {
			e.popScope()
			return err
		}
		e.popScope()
		e.indent--
	}
	if match.CatchAll != nil {
		e.line("default:")
		e.indent++
		if err := e.emitStatements(match.CatchAll.Stmts, checker.Void); err != nil {
			return err
		}
		e.indent--
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitValueTemp(prefix string, valueType checker.Type) (string, func(string) error, error) {
	if valueType == nil || valueType == checker.Void {
		return "", nil, fmt.Errorf("expected non-void value type for %s", prefix)
	}
	tempName := e.nextTemp(prefix)
	typeName, err := e.emitType(valueType)
	if err != nil {
		return "", nil, err
	}
	e.line(fmt.Sprintf("var %s %s", tempName, typeName))
	assign := func(value string) error {
		e.line(fmt.Sprintf("%s = %s", tempName, value))
		return nil
	}
	return tempName, assign, nil
}

func (e *emitter) emitBoolMatch(match *checker.BoolMatch, expectedType checker.Type) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	returnType := matchReturnType(expectedType, match.Type())
	tempName, assign, err := e.emitValueTemp("BoolMatch", returnType)
	if err != nil {
		return "", err
	}
	e.line("if " + subject + " {")
	e.indent++
	if err := e.emitBlockValue(match.True, returnType, assign); err != nil {
		return "", err
	}
	e.indent--
	e.line("} else {")
	e.indent++
	if err := e.emitBlockValue(match.False, returnType, assign); err != nil {
		return "", err
	}
	e.indent--
	e.line("}")
	return tempName, nil
}

func (e *emitter) emitIntMatch(match *checker.IntMatch, expectedType checker.Type) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	returnType := matchReturnType(expectedType, match.Type())
	tempName, assign, err := e.emitValueTemp("IntMatch", returnType)
	if err != nil {
		return "", err
	}
	e.line("switch {")
	e.indent++
	for _, value := range sortedIntKeys(match.IntCases) {
		block := match.IntCases[value]
		e.line(fmt.Sprintf("case %s == %d:", subject, value))
		e.indent++
		if err := e.emitBlockValue(block, returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	}
	for _, intRange := range sortedIntRanges(match.RangeCases) {
		block := match.RangeCases[intRange]
		e.line(fmt.Sprintf("case %s >= %d && %s <= %d:", subject, intRange.Start, subject, intRange.End))
		e.indent++
		if err := e.emitBlockValue(block, returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	}
	if match.CatchAll != nil {
		e.line("default:")
		e.indent++
		if err := e.emitBlockValue(match.CatchAll, returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	} else if returnType != checker.Void {
		e.line("default:")
		e.indent++
		e.line(`panic("non-exhaustive int match")`)
		e.indent--
	}
	e.indent--
	e.line("}")
	return tempName, nil
}

func (e *emitter) emitConditionalMatch(match *checker.ConditionalMatch, expectedType checker.Type) (string, error) {
	returnType := matchReturnType(expectedType, match.Type())
	tempName, assign, err := e.emitValueTemp("ConditionalMatch", returnType)
	if err != nil {
		return "", err
	}
	for i, matchCase := range match.Cases {
		condition, err := e.emitExpr(matchCase.Condition)
		if err != nil {
			return "", err
		}
		prefix := "if"
		if i > 0 {
			prefix = "} else if"
		}
		e.line(prefix + " " + condition + " {")
		e.indent++
		if err := e.emitBlockValue(matchCase.Body, returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	}
	if match.CatchAll != nil {
		if len(match.Cases) == 0 {
			e.line("{")
		} else {
			e.line("} else {")
		}
		e.indent++
		if err := e.emitBlockValue(match.CatchAll, returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	} else if returnType != checker.Void {
		if len(match.Cases) == 0 {
			e.line("{")
		} else {
			e.line("} else {")
		}
		e.indent++
		e.line(`panic("non-exhaustive conditional match")`)
		e.indent--
	}
	if len(match.Cases) > 0 || match.CatchAll != nil || returnType != checker.Void {
		e.line("}")
	}
	return tempName, nil
}

func (e *emitter) emitOptionMatch(match *checker.OptionMatch, expectedType checker.Type) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	returnType := matchReturnType(expectedType, match.Type())
	tempName, assign, err := e.emitValueTemp("MaybeMatch", returnType)
	if err != nil {
		return "", err
	}
	maybeName := e.nextTemp("Maybe")
	e.line(maybeName + " := " + subject)
	e.line("if " + maybeName + ".IsSome() {")
	e.indent++
	e.pushScope()
	if match.Some != nil && match.Some.Pattern != nil {
		patternName := e.bindLocal(match.Some.Pattern.Name)
		e.line(fmt.Sprintf("%s := %s.Expect(%q)", patternName, maybeName, "unreachable none in maybe match"))
		if !usesNameInStatements(match.Some.Body.Stmts, match.Some.Pattern.Name) {
			e.line("_ = " + patternName)
		}
	}
	if err := e.emitBlockValue(match.Some.Body, returnType, assign); err != nil {
		e.popScope()
		return "", err
	}
	e.popScope()
	e.indent--
	e.line("} else {")
	e.indent++
	if err := e.emitBlockValue(match.None, returnType, assign); err != nil {
		return "", err
	}
	e.indent--
	e.line("}")
	return tempName, nil
}

func (e *emitter) emitResultMatch(match *checker.ResultMatch, expectedType checker.Type) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	returnType := matchReturnType(expectedType, match.Type())
	tempName, assign, err := e.emitValueTemp("ResultMatch", returnType)
	if err != nil {
		return "", err
	}
	resultName := e.nextTemp("Result")
	e.line(resultName + " := " + subject)
	e.line("if " + resultName + ".IsOk() {")
	e.indent++
	e.pushScope()
	if match.Ok != nil && match.Ok.Pattern != nil {
		boundValue, err := emitCopiedValue(resultName+".UnwrapOk()", match.OkType)
		if err != nil {
			e.popScope()
			return "", err
		}
		okName := e.bindLocal(match.Ok.Pattern.Name)
		e.line(fmt.Sprintf("%s := %s", okName, boundValue))
		if !usesNameInStatements(match.Ok.Body.Stmts, match.Ok.Pattern.Name) {
			e.line("_ = " + okName)
		}
	}
	if err := e.emitBlockValue(match.Ok.Body, returnType, assign); err != nil {
		e.popScope()
		return "", err
	}
	e.popScope()
	e.indent--
	e.line("} else {")
	e.indent++
	e.pushScope()
	if match.Err != nil && match.Err.Pattern != nil {
		errName := e.bindLocal(match.Err.Pattern.Name)
		e.line(fmt.Sprintf("%s := %s.UnwrapErr()", errName, resultName))
		if !usesNameInStatements(match.Err.Body.Stmts, match.Err.Pattern.Name) {
			e.line("_ = " + errName)
		}
	}
	if err := e.emitBlockValue(match.Err.Body, returnType, assign); err != nil {
		e.popScope()
		return "", err
	}
	e.popScope()
	e.indent--
	e.line("}")
	return tempName, nil
}

func (e *emitter) emitEnumMatch(match *checker.EnumMatch, expectedType checker.Type) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	returnType := matchReturnType(expectedType, match.Type())
	tempName, assign, err := e.emitValueTemp("EnumMatch", returnType)
	if err != nil {
		return "", err
	}
	e.line("switch " + subject + ".Tag {")
	e.indent++
	discriminants := make([]int, 0, len(match.DiscriminantToIndex))
	for discriminant := range match.DiscriminantToIndex {
		discriminants = append(discriminants, discriminant)
	}
	sort.Ints(discriminants)
	for _, discriminant := range discriminants {
		idx := match.DiscriminantToIndex[discriminant]
		if idx < 0 || int(idx) >= len(match.Cases) || match.Cases[idx] == nil {
			continue
		}
		e.line(fmt.Sprintf("case %d:", discriminant))
		e.indent++
		if err := e.emitBlockValue(match.Cases[idx], returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	}
	if match.CatchAll != nil {
		e.line("default:")
		e.indent++
		if err := e.emitBlockValue(match.CatchAll, returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	} else if returnType != checker.Void {
		e.line("default:")
		e.indent++
		e.line(`panic("non-exhaustive enum match")`)
		e.indent--
	}
	e.indent--
	e.line("}")
	return tempName, nil
}

func (e *emitter) emitUnionMatch(match *checker.UnionMatch, expectedType checker.Type) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	returnType := matchReturnType(expectedType, match.Type())
	tempName, assign, err := e.emitValueTemp("UnionMatch", returnType)
	if err != nil {
		return "", err
	}
	subjectName := e.nextTemp("Union")
	e.line("switch " + subjectName + " := any(" + subject + ").(type) {")
	e.indent++
	caseNames := sortedStringKeys(match.TypeCases)
	for _, caseName := range caseNames {
		matchCase := match.TypeCases[caseName]
		if matchCase == nil {
			continue
		}
		caseType := checker.Type(nil)
		for t := range match.TypeCasesByType {
			if t.String() == caseName {
				caseType = t
				break
			}
		}
		if caseType == nil {
			return "", fmt.Errorf("missing union case type for %s", caseName)
		}
		typeName, err := e.emitTypeArg(caseType)
		if err != nil {
			return "", err
		}
		e.line("case " + typeName + ":")
		e.indent++
		e.pushScope()
		if matchCase.Pattern != nil {
			boundName := e.bindLocal(matchCase.Pattern.Name)
			e.line(fmt.Sprintf("%s := %s", boundName, subjectName))
			if !usesNameInStatements(matchCase.Body.Stmts, matchCase.Pattern.Name) {
				e.line("_ = " + boundName)
			}
		}
		if err := e.emitBlockValue(matchCase.Body, returnType, assign); err != nil {
			e.popScope()
			return "", err
		}
		e.popScope()
		e.indent--
	}
	if match.CatchAll != nil {
		e.line("default:")
		e.indent++
		if err := e.emitBlockValue(match.CatchAll, returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	} else if returnType != checker.Void {
		e.line("default:")
		e.indent++
		e.line(`panic("non-exhaustive union match")`)
		e.indent--
	}
	e.indent--
	e.line("}")
	return tempName, nil
}

func (e *emitter) emitPanicExpr(message checker.Expression, resultType checker.Type) (string, error) {
	messageExpr, err := e.emitExpr(message)
	if err != nil {
		return "", err
	}
	return e.emitInlineFunc(resultType, func(inner *emitter) error {
		inner.line("panic(" + messageExpr + ")")
		return nil
	})
}

func (e *emitter) emitInlineFunc(returnType checker.Type, body func(inner *emitter) error) (string, error) {
	inner := &emitter{
		module:          e.module,
		packageName:     e.packageName,
		projectName:     e.projectName,
		entrypoint:      e.entrypoint,
		imports:         e.imports,
		functionNames:   e.functionNames,
		emittedTypes:    e.emittedTypes,
		indent:          1,
		fnReturnType:    e.fnReturnType,
		localScopes:     cloneLocalScopes(e.localScopes),
		pointerScopes:   clonePointerScopes(e.pointerScopes),
		localNameCounts: cloneLocalNameCounts(e.localNameCounts),
		typeParams:      cloneTypeParams(e.typeParams),
	}
	if err := body(inner); err != nil {
		return "", err
	}
	var builder strings.Builder
	builder.WriteString("func()")
	if returnType != nil && returnType != checker.Void {
		typeName, err := inner.emitType(returnType)
		if err != nil {
			return "", err
		}
		builder.WriteString(" ")
		builder.WriteString(typeName)
	}
	builder.WriteString(" {\n")
	builder.WriteString(inner.builder.String())
	builder.WriteString("}()")
	return builder.String(), nil
}

func (e *emitter) emitListModuleCall(call *checker.ModuleFunctionCall) (string, bool, error) {
	if call == nil || call.Call == nil {
		return "", false, nil
	}
	switch call.Call.Name {
	case "concat":
		if len(call.Call.Args) != 2 {
			return "", true, fmt.Errorf("List::concat expects two args")
		}
		left, err := e.emitExpr(call.Call.Args[0])
		if err != nil {
			return "", true, err
		}
		right, err := e.emitExpr(call.Call.Args[1])
		if err != nil {
			return "", true, err
		}
		typeName, err := e.emitType(call.Call.ReturnType)
		if err != nil {
			return "", true, err
		}
		return fmt.Sprintf("func() %s { out := append(%s(nil), %s...); return append(out, %s...) }()", typeName, typeName, left, right), true, nil
	default:
		return "", false, nil
	}
}

func (e *emitter) emitAsyncModuleCall(call *checker.ModuleFunctionCall) (string, bool, error) {
	if call == nil || call.Call == nil {
		return "", false, nil
	}
	switch call.Call.Name {
	case "join":
		if len(call.Call.Args) != 1 {
			return "", true, fmt.Errorf("async::join expects one arg")
		}
		list, ok := call.Call.Args[0].(*checker.ListLiteral)
		if !ok {
			return "", false, nil
		}
		elements := make([]string, 0, len(list.Elements))
		for _, element := range list.Elements {
			emitted, err := e.emitExpr(element)
			if err != nil {
				return "", true, err
			}
			elements = append(elements, emitted)
		}
		alias := packageNameForModulePath("ard/async")
		return fmt.Sprintf("%s.JoinAny([]any{%s})", alias, strings.Join(elements, ", ")), true, nil
	default:
		return "", false, nil
	}
}

func (e *emitter) emitFiberStart(start *checker.FiberStart) (string, error) {
	if start == nil || start.GetFn() == nil {
		return "", fmt.Errorf("missing fiber start function")
	}
	fn, err := e.emitExpr(start.GetFn())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%s(%s)", packageNameForModulePath("ard/async"), goName("start", true), fn), nil
}

func (e *emitter) emitFiberEval(eval *checker.FiberEval) (string, error) {
	if eval == nil || eval.GetFn() == nil {
		return "", fmt.Errorf("missing fiber eval function")
	}
	fn, err := e.emitExpr(eval.GetFn())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%s(%s)", packageNameForModulePath("ard/async"), goName("eval", true), fn), nil
}

func (e *emitter) emitFiberExecution(exec *checker.FiberExecution) (string, error) {
	if exec == nil || exec.GetModule() == nil {
		return "", fmt.Errorf("missing fiber execution module")
	}
	moduleAlias := packageNameForModulePath(exec.GetModule().Path())
	asyncAlias := packageNameForModulePath("ard/async")
	return fmt.Sprintf("%s.%s(%s.%s)", asyncAlias, goName("start", true), moduleAlias, goName(exec.GetMainName(), true)), nil
}

func (e *emitter) emitResultModuleCall(call *checker.ModuleFunctionCall, expectedType checker.Type) (string, error) {
	switch call.Call.Name {
	case "ok":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("Result::ok expects one arg")
		}
		resultType, ok := call.Call.ReturnType.(*checker.Result)
		if (!ok || resultHasUnresolvedTypeVar(resultType)) && expectedType != nil {
			if expectedResult, ok := expectedType.(*checker.Result); ok {
				resultType = expectedResult
				ok = true
			}
		}
		if !ok {
			return "", fmt.Errorf("Result::ok expected Result return type, got %s", call.Call.ReturnType)
		}
		arg, err := e.emitValueForType(call.Call.Args[0], resultType.Val())
		if err != nil {
			return "", err
		}
		valueType, err := e.emitTypeArg(resultType.Val())
		if err != nil {
			return "", err
		}
		errType, err := e.emitTypeArg(resultType.Err())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Ok[%s, %s](%s)", helperImportAlias, valueType, errType, arg), nil
	case "err":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("Result::err expects one arg")
		}
		resultType, ok := call.Call.ReturnType.(*checker.Result)
		if (!ok || resultHasUnresolvedTypeVar(resultType)) && expectedType != nil {
			if expectedResult, ok := expectedType.(*checker.Result); ok {
				resultType = expectedResult
				ok = true
			}
		}
		if !ok {
			return "", fmt.Errorf("Result::err expected Result return type, got %s", call.Call.ReturnType)
		}
		arg, err := e.emitValueForType(call.Call.Args[0], resultType.Err())
		if err != nil {
			return "", err
		}
		valueType, err := e.emitTypeArg(resultType.Val())
		if err != nil {
			return "", err
		}
		errType, err := e.emitTypeArg(resultType.Err())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Err[%s, %s](%s)", helperImportAlias, valueType, errType, arg), nil
	default:
		return "", fmt.Errorf("unsupported result module call: %s", call.Call.Name)
	}
}

func (e *emitter) emitMaybeMethod(method *checker.MaybeMethod) (string, error) {
	subject, err := e.emitExpr(method.Subject)
	if err != nil {
		return "", err
	}
	emitMaybeArg := func(index int) (string, error) {
		if index >= len(method.Args) {
			return "", fmt.Errorf("maybe method missing arg %d", index)
		}
		return e.emitExpr(method.Args[index])
	}
	switch method.Kind {
	case checker.MaybeExpect:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("maybe.expect expects one arg")
		}
		message, err := emitMaybeArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Expect(%s)", subject, message), nil
	case checker.MaybeIsNone:
		return fmt.Sprintf("%s.IsNone()", subject), nil
	case checker.MaybeIsSome:
		return fmt.Sprintf("%s.IsSome()", subject), nil
	case checker.MaybeOr:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("maybe.or expects one arg")
		}
		fallback, err := emitMaybeArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Or(%s)", subject, fallback), nil
	case checker.MaybeMap:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("maybe.map expects one arg")
		}
		mapper, err := emitMaybeArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.MaybeMap(%s, %s)", helperImportAlias, subject, mapper), nil
	case checker.MaybeAndThen:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("maybe.and_then expects one arg")
		}
		mapper, err := emitMaybeArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.MaybeAndThen(%s, %s)", helperImportAlias, subject, mapper), nil
	default:
		return "", fmt.Errorf("unsupported maybe method: %v", method.Kind)
	}
}

func (e *emitter) emitResultMethod(method *checker.ResultMethod) (string, error) {
	subject, err := e.emitExpr(method.Subject)
	if err != nil {
		return "", err
	}
	emitResultArg := func(index int) (string, error) {
		if index >= len(method.Args) {
			return "", fmt.Errorf("result method missing arg %d", index)
		}
		return e.emitExpr(method.Args[index])
	}
	switch method.Kind {
	case checker.ResultExpect:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("result.expect expects one arg")
		}
		message, err := emitResultArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Expect(%s)", subject, message), nil
	case checker.ResultOr:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("result.or expects one arg")
		}
		fallback, err := emitResultArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Or(%s)", subject, fallback), nil
	case checker.ResultIsOk:
		return fmt.Sprintf("%s.IsOk()", subject), nil
	case checker.ResultIsErr:
		return fmt.Sprintf("%s.IsErr()", subject), nil
	case checker.ResultMap:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("result.map expects one arg")
		}
		mapper, err := emitResultArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.ResultMap(%s, %s)", helperImportAlias, subject, mapper), nil
	case checker.ResultMapErr:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("result.map_err expects one arg")
		}
		mapper, err := emitResultArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.ResultMapErr(%s, %s)", helperImportAlias, subject, mapper), nil
	case checker.ResultAndThen:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("result.and_then expects one arg")
		}
		mapper, err := emitResultArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.ResultAndThen(%s, %s)", helperImportAlias, subject, mapper), nil
	default:
		return "", fmt.Errorf("unsupported result method: %v", method.Kind)
	}
}

func (e *emitter) emitMaybeModuleCall(call *checker.ModuleFunctionCall, expectedType checker.Type) (string, error) {
	switch call.Call.Name {
	case "some":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("maybe::some expects one arg")
		}
		maybeType, ok := call.Call.ReturnType.(*checker.Maybe)
		if expectedType != nil {
			if expectedMaybe, expectedOk := expectedType.(*checker.Maybe); expectedOk {
				maybeType = expectedMaybe
				ok = true
			}
		} else if !ok || maybeHasUnresolvedTypeVar(maybeType) {
			ok = false
		}
		if !ok {
			return "", fmt.Errorf("maybe::some expected Maybe return type, got %s", call.Call.ReturnType)
		}
		arg, err := e.emitValueForType(call.Call.Args[0], maybeType.Of())
		if err != nil {
			return "", err
		}
		innerType, err := e.emitTypeArg(maybeType.Of())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Some[%s](%s)", helperImportAlias, innerType, arg), nil
	case "none":
		maybeType, ok := call.Call.ReturnType.(*checker.Maybe)
		if expectedType != nil {
			if expectedMaybe, expectedOk := expectedType.(*checker.Maybe); expectedOk {
				maybeType = expectedMaybe
				ok = true
			}
		} else if !ok || maybeHasUnresolvedTypeVar(maybeType) {
			ok = false
		}
		if !ok {
			return "", fmt.Errorf("maybe::none expected Maybe return type, got %s", call.Call.ReturnType)
		}
		innerType, err := e.emitTypeArg(maybeType.Of())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.None[%s]()", helperImportAlias, innerType), nil
	default:
		return "", fmt.Errorf("unsupported maybe module call: %s", call.Call.Name)
	}
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

func (e *emitter) emitMutableCallArg(arg checker.Expression, param checker.Parameter) (string, error) {
	if !mutableParamNeedsPointer(param.Type) {
		return e.emitValueForType(arg, param.Type)
	}
	typeName, err := e.emitType(param.Type)
	if err != nil {
		return "", err
	}
	switch v := arg.(type) {
	case *checker.CopyExpression:
		value, err := e.emitExpr(v)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() *%s { value := %s; return &value }()", typeName, value), nil
	case *checker.Identifier:
		resolved := e.resolveLocal(v.Name)
		if e.isPointerLocal(v.Name) {
			return resolved, nil
		}
		return "&" + resolved, nil
	case checker.Variable:
		resolved := e.resolveLocal(v.Name())
		if e.isPointerLocal(v.Name()) {
			return resolved, nil
		}
		return "&" + resolved, nil
	case *checker.Variable:
		resolved := e.resolveLocal(v.Name())
		if e.isPointerLocal(v.Name()) {
			return resolved, nil
		}
		return "&" + resolved, nil
	default:
		target, err := e.emitAssignmentTarget(arg)
		if err == nil {
			return "&" + target, nil
		}
		value, err := e.emitExpr(arg)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() *%s { value := %s; return &value }()", typeName, value), nil
	}
}

func (e *emitter) emitCallArgs(call *checker.FunctionCall) ([]string, error) {
	args := make([]string, 0, len(call.Args))
	var params []checker.Parameter
	if def := call.Definition(); def != nil {
		params = def.Parameters
	}
	for i, arg := range call.Args {
		expectedType := checker.Type(nil)
		var param checker.Parameter
		hasParam := i < len(params)
		if hasParam {
			param = params[i]
			expectedType = param.Type
		}
		var emitted string
		var err error
		if hasParam && param.Mutable {
			emitted, err = e.emitMutableCallArg(arg, param)
		} else {
			emitted, err = e.emitValueForType(arg, expectedType)
		}
		if err != nil {
			return nil, err
		}
		args = append(args, emitted)
	}
	return args, nil
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

func (e *emitter) emitFunctionCallTypeArgsFromDefs(originalDef, specializedDef *checker.FunctionDef) (string, error) {
	if originalDef == nil || specializedDef == nil {
		return "", nil
	}
	order, _, _ := functionTypeParams(originalDef)
	if len(order) == 0 {
		return "", nil
	}
	bindings := make(map[string]checker.Type, len(order))
	for i := 0; i < len(originalDef.Parameters) && i < len(specializedDef.Parameters); i++ {
		inferGenericTypeBindings(originalDef.Parameters[i].Type, specializedDef.Parameters[i].Type, bindings)
	}
	inferGenericTypeBindings(effectiveFunctionReturnType(originalDef), effectiveFunctionReturnType(specializedDef), bindings)
	parts := make([]string, 0, len(order))
	for _, name := range order {
		bound := bindings[name]
		if bound == nil {
			return "", nil
		}
		if tv, ok := bound.(*checker.TypeVar); ok {
			if actual := tv.Actual(); actual != nil {
				bound = actual
			} else {
				return "", nil
			}
		}
		emitted, err := e.emitTypeArg(bound)
		if err != nil {
			return "", err
		}
		parts = append(parts, emitted)
	}
	if len(parts) == 0 {
		return "", nil
	}
	return "[" + strings.Join(parts, ", ") + "]", nil
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

func (e *emitter) emitMapMutationExpr(method *checker.MapMethod) (string, error) {
	target, err := e.emitAssignmentTarget(method.Subject)
	if err != nil {
		return "", err
	}
	switch method.Kind {
	case checker.MapSet:
		if len(method.Args) != 2 {
			return "", fmt.Errorf("map.set expects two args")
		}
		key, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		value, err := e.emitExpr(method.Args[1])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() bool { %s[%s] = %s; return true }()", target, key, value), nil
	case checker.MapDrop:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("map.drop expects one arg")
		}
		key, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() struct{} { delete(%s, %s); return struct{}{} }()", target, key), nil
	default:
		return "", fmt.Errorf("unsupported mutable map method: %v", method.Kind)
	}
}

func (e *emitter) emitBinary(left checker.Expression, op string, right checker.Expression) (string, error) {
	leftExpr, err := e.emitExpr(left)
	if err != nil {
		return "", err
	}
	rightExpr, err := e.emitExpr(right)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("(%s %s %s)", leftExpr, op, rightExpr), nil
}

func emitTypeArgWithOptions(t checker.Type, typeParams map[string]string, namedTypeRef func(string, checker.Type) string) (string, error) {
	typeName, err := emitTypeWithOptions(t, typeParams, namedTypeRef)
	if err != nil {
		return "", err
	}
	if typeName == "" {
		return "struct{}", nil
	}
	return typeName, nil
}

func emitTypeArg(t checker.Type) (string, error) {
	return emitTypeArgWithOptions(t, nil, nil)
}

func emitTraitTypeWithOptions(trait *checker.Trait, typeParams map[string]string, namedTypeRef func(string, checker.Type) string) (string, error) {
	if trait == nil {
		return "", fmt.Errorf("nil trait")
	}
	switch trait.Name {
	case "ToString":
		return fmt.Sprintf("%s.ToString", helperImportAlias), nil
	case "Encodable":
		return fmt.Sprintf("%s.Encodable", helperImportAlias), nil
	}
	methods := trait.GetMethods()
	parts := make([]string, 0, len(methods))
	for _, method := range methods {
		params, err := emitFunctionParamsWithOptions(method.Parameters, false, typeParams, namedTypeRef)
		if err != nil {
			return "", err
		}
		signature := fmt.Sprintf("%s(%s)", goName(method.Name, true), strings.Join(params, ", "))
		if method.ReturnType != checker.Void {
			returnType, err := emitTypeWithOptions(method.ReturnType, typeParams, namedTypeRef)
			if err != nil {
				return "", err
			}
			signature += " " + returnType
		}
		parts = append(parts, signature)
	}
	return fmt.Sprintf("interface{ %s }", strings.Join(parts, "; ")), nil
}

func emitTraitType(trait *checker.Trait) (string, error) {
	return emitTraitTypeWithOptions(trait, nil, nil)
}

func emitTypeWithOptions(t checker.Type, typeParams map[string]string, namedTypeRef func(string, checker.Type) string) (string, error) {
	switch t {
	case checker.Int:
		return "int", nil
	case checker.Float:
		return "float64", nil
	case checker.Str:
		return "string", nil
	case checker.Bool:
		return "bool", nil
	case checker.Void:
		return "", nil
	case checker.Dynamic:
		return "any", nil
	}

	switch typed := t.(type) {
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			return emitTypeWithOptions(actual, typeParams, namedTypeRef)
		}
		if typeParams != nil {
			if resolved := typeParams[typeVarName(typed)]; resolved != "" {
				return resolved, nil
			}
		}
		return "any", nil
	case *checker.Result:
		valueType, err := emitTypeArgWithOptions(typed.Val(), typeParams, namedTypeRef)
		if err != nil {
			return "", err
		}
		errType, err := emitTypeArgWithOptions(typed.Err(), typeParams, namedTypeRef)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Result[%s, %s]", helperImportAlias, valueType, errType), nil
	case *checker.Enum:
		if len(typed.Methods) > 0 {
			if namedTypeRef != nil {
				return namedTypeRef(typed.Name, typed), nil
			}
			return goName(typed.Name, true), nil
		}
		return "struct{ Tag int }", nil
	case *checker.Maybe:
		innerType, err := emitTypeArgWithOptions(typed.Of(), typeParams, namedTypeRef)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Maybe[%s]", helperImportAlias, innerType), nil
	case *checker.List:
		elementType, err := emitTypeWithOptions(typed.Of(), typeParams, namedTypeRef)
		if err != nil {
			return "", err
		}
		return "[]" + elementType, nil
	case *checker.Map:
		keyType, err := emitTypeWithOptions(typed.Key(), typeParams, namedTypeRef)
		if err != nil {
			return "", err
		}
		valueType, err := emitTypeWithOptions(typed.Value(), typeParams, namedTypeRef)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("map[%s]%s", keyType, valueType), nil
	case *checker.StructDef:
		baseName := goName(typed.Name, true)
		if namedTypeRef != nil {
			baseName = namedTypeRef(typed.Name, typed)
		}
		order := structTypeParamOrder(typed)
		if len(order) == 0 {
			return baseName, nil
		}
		bindings := inferStructBoundTypeArgs(typed, order, nil)
		args := make([]string, 0, len(order))
		for _, name := range order {
			if typeParams != nil {
				if resolved := typeParams[name]; resolved != "" {
					args = append(args, resolved)
					continue
				}
			}
			bound := bindings[name]
			if tv, ok := bound.(*checker.TypeVar); ok {
				if actual := tv.Actual(); actual != nil {
					bound = actual
				} else {
					bound = nil
				}
			}
			if bound != nil {
				emitted, err := emitTypeArgWithOptions(bound, typeParams, namedTypeRef)
				if err != nil {
					return "", err
				}
				args = append(args, emitted)
				continue
			}
			args = append(args, "any")
		}
		return fmt.Sprintf("%s[%s]", baseName, strings.Join(args, ", ")), nil
	case *checker.FunctionDef:
		return emitFunctionTypeWithOptions(typed, typeParams, namedTypeRef)
	case *checker.Trait:
		return emitTraitTypeWithOptions(typed, typeParams, namedTypeRef)
	case *checker.ExternType:
		return "any", nil
	case *checker.Union:
		return "any", nil
	default:
		return "", fmt.Errorf("unsupported type: %s", t.String())
	}
}

func emitType(t checker.Type) (string, error) {
	return emitTypeWithOptions(t, nil, nil)
}

func (e *emitter) line(text string) {
	if text == "" {
		e.builder.WriteString("\n")
		return
	}
	e.builder.WriteString(strings.Repeat("\t", e.indent))
	e.builder.WriteString(text)
	e.builder.WriteString("\n")
}

func (e *emitter) captureOutput(fn func() error) (string, error) {
	prevBuilder := e.builder
	prevIndent := e.indent
	e.builder = strings.Builder{}
	e.indent = 0
	defer func() {
		e.builder = prevBuilder
		e.indent = prevIndent
	}()
	if err := fn(); err != nil {
		return "", err
	}
	return e.builder.String(), nil
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
