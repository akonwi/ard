package air

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/akonwi/ard/checker"
)

type LowerOptions struct {
	IncludeTests bool
}

func Lower(module checker.Module) (*Program, error) {
	return LowerWithOptions(module, LowerOptions{})
}

func LowerWithTests(module checker.Module) (*Program, error) {
	return LowerWithOptions(module, LowerOptions{IncludeTests: true})
}

func LowerWithOptions(module checker.Module, options LowerOptions) (*Program, error) {
	return LowerModulesWithOptions([]checker.Module{module}, options)
}

func LowerModulesWithTests(modules []checker.Module) (*Program, error) {
	return LowerModulesWithOptions(modules, LowerOptions{IncludeTests: true})
}

func LowerModulesWithOptions(modules []checker.Module, options LowerOptions) (*Program, error) {
	l := newLowerer(options)
	for _, module := range modules {
		if err := l.lowerModule(module); err != nil {
			return nil, err
		}
	}
	if err := l.lowerAllModuleGlobals(); err != nil {
		return nil, err
	}
	if err := Validate(&l.program); err != nil {
		return nil, err
	}
	return &l.program, nil
}

type lowerer struct {
	program Program

	moduleByPath map[string]ModuleID
	moduleByName map[string]checker.Module
	typeByKey    map[string]TypeID
	traits       map[string]TraitID
	impls        map[string]ImplID
	functions    map[string]FunctionID
	globals      map[string]GlobalID

	loweringModules          map[string]bool
	loweredModules           map[string]bool
	loweringFuncs            map[FunctionID]bool
	loweredFuncs             map[FunctionID]bool
	loweringGlobals          map[GlobalID]bool
	loweredGlobals           map[GlobalID]bool
	functionTypeVars         map[FunctionID]map[string]TypeID
	genericStructDefs        map[string]TypeID
	genericFunctionDefs      map[string]FunctionID
	genericFunctionOriginals map[string]*checker.FunctionDef
	genericMethodDefs        map[string]FunctionID
	defParams                map[string]int
	includeTests             bool
}

type functionLowerer struct {
	l             *lowerer
	locals        map[string]LocalID
	fn            *Function
	parent        *functionLowerer
	captureByName map[string]LocalID
	captureLocals []LocalID
	typeVars      map[string]TypeID
	// directLetValue is the initializer expression of the let statement being
	// lowered. Pointer-result foreign calls are only representable as direct
	// let bindings (they become pointer-backed locals); anywhere else they
	// have no value representation yet and must be rejected.
	directLetValue checker.Expression
}

func newLowerer(options LowerOptions) *lowerer {
	l := &lowerer{
		program: Program{
			Entry:  NoFunction,
			Script: NoFunction,
		},
		moduleByPath: map[string]ModuleID{},
		moduleByName: map[string]checker.Module{},
		typeByKey:    map[string]TypeID{},
		traits:       map[string]TraitID{},
		impls:        map[string]ImplID{},
		functions:    map[string]FunctionID{},
		globals:      map[string]GlobalID{},

		loweringModules:          map[string]bool{},
		loweredModules:           map[string]bool{},
		loweringFuncs:            map[FunctionID]bool{},
		loweredFuncs:             map[FunctionID]bool{},
		loweringGlobals:          map[GlobalID]bool{},
		loweredGlobals:           map[GlobalID]bool{},
		functionTypeVars:         map[FunctionID]map[string]TypeID{},
		genericStructDefs:        map[string]TypeID{},
		genericFunctionDefs:      map[string]FunctionID{},
		genericFunctionOriginals: map[string]*checker.FunctionDef{},
		genericMethodDefs:        map[string]FunctionID{},
		includeTests:             options.IncludeTests,
	}
	l.mustIntern(checker.Void)
	l.mustIntern(checker.Int)
	l.mustIntern(checker.Float64)
	l.mustIntern(checker.Bool)
	l.mustIntern(checker.Byte)
	l.mustIntern(checker.Rune)
	l.mustIntern(checker.Str)
	l.mustIntern(checker.Any)
	return l
}

func (l *lowerer) mustIntern(t checker.Type) TypeID {
	id, err := l.internType(t)
	if err != nil {
		panic(err)
	}
	return id
}

func (l *lowerer) structMethods(def *checker.StructDef) map[string]*checker.FunctionDef {
	if def == nil {
		return nil
	}
	return checker.StructMethodsInModules(l.moduleByName, checker.StructMethodOwner(def))
}

func (l *lowerer) findReachableModule(path string) checker.Module {
	if mod, ok := l.moduleByName[path]; ok {
		return mod
	}
	for _, mod := range l.moduleByName {
		if found := findReachableModuleSeen(mod, path, map[string]bool{}); found != nil {
			l.moduleByName[path] = found
			return found
		}
	}
	return nil
}

func findReachableModuleSeen(mod checker.Module, path string, seen map[string]bool) checker.Module {
	if mod == nil {
		return nil
	}
	modPath := mod.Path()
	if modPath == path {
		return mod
	}
	if seen[modPath] {
		return nil
	}
	seen[modPath] = true
	program := mod.Program()
	if program == nil {
		return nil
	}
	for _, imported := range program.Imports {
		if found := findReachableModuleSeen(imported, path, seen); found != nil {
			return found
		}
	}
	return nil
}

func (l *lowerer) hasStructMethod(def *checker.StructDef, name string) bool {
	methods := l.structMethods(def)
	return methods != nil && methods[name] != nil
}

func (l *lowerer) lowerModule(module checker.Module) error {
	if module == nil {
		return fmt.Errorf("cannot lower nil module")
	}
	path := module.Path()
	if l.loweredModules[path] {
		return nil
	}
	if l.loweringModules[path] {
		return nil
	}
	l.moduleByName[path] = module
	l.loweringModules[path] = true
	defer delete(l.loweringModules, path)

	modID := l.internModule(path)
	mod := &l.program.Modules[modID]
	prog := module.Program()
	if prog == nil {
		l.loweredModules[path] = true
		return nil
	}

	for _, imported := range prog.Imports {
		l.moduleByName[imported.Path()] = imported
		importID := l.internModule(imported.Path())
		mod.Imports = append(mod.Imports, importID)
	}

	for i := range prog.Statements {
		stmt := prog.Statements[i]
		switch node := stmt.Stmt.(type) {
		case *checker.StructDef:
			if len(node.GenericParams) > 0 {
				if _, err := l.internGenericStructDef(node); err != nil {
					return err
				}
				continue
			}
			if typeHasUnresolvedTypeVar(node) {
				continue
			}
			typeID, err := l.internType(node)
			if err != nil {
				return err
			}
			mod.Types = appendUniqueType(mod.Types, typeID)
		case *checker.Enum:
			typeID, err := l.internType(node)
			if err != nil {
				return err
			}
			mod.Types = appendUniqueType(mod.Types, typeID)
		case *checker.Union:
			if typeHasUnresolvedTypeVar(node) {
				continue
			}
			typeID, err := l.internType(node)
			if err != nil {
				return err
			}
			mod.Types = appendUniqueType(mod.Types, typeID)
		}

		switch node := stmt.Stmt.(type) {
		case *checker.VariableDef:
			if _, err := l.declareGlobal(modID, node); err != nil {
				return err
			}
		}

		switch expr := stmt.Expr.(type) {
		case *checker.FunctionDef:
			if functionHasUnresolvedTypeVar(expr) || (!l.includeTests && expr.IsTest) {
				// Register generic function definitions (with their $T parameters
				// intact) so call sites can recover the generic shape even for
				// private functions not exposed in the module's public symbols.
				if functionHasUnresolvedTypeVar(expr) {
					l.genericFunctionOriginals[functionKey(modID, expr.Name)] = expr
				}
				continue
			}
			if _, err := l.declareFunction(modID, expr); err != nil {
				return err
			}
		}
	}

	for i := range prog.Statements {
		stmt := prog.Statements[i]
		switch node := stmt.Stmt.(type) {
		case *checker.StructDef:
			if typeHasUnresolvedTypeVar(node) {
				continue
			}
			if err := l.declareTraitImplsForType(modID, node); err != nil {
				return err
			}
			if err := l.declareInherentImplMethodsForStruct(modID, node); err != nil {
				return err
			}
		case *checker.Enum:
			if err := l.declareTraitImplsForType(modID, node); err != nil {
				return err
			}
		}
	}

	for i := range prog.Statements {
		stmt := prog.Statements[i]
		if def, ok := stmt.Stmt.(*checker.VariableDef); ok {
			if err := l.lowerGlobal(modID, def); err != nil {
				return fmt.Errorf("lower global %s: %w", def.Name, err)
			}
		}
	}

	for i := range prog.Statements {
		stmt := prog.Statements[i]
		if def, ok := stmt.Expr.(*checker.FunctionDef); ok {
			if functionHasUnresolvedTypeVar(def) || (!l.includeTests && def.IsTest) {
				continue
			}
			if err := l.lowerFunction(modID, def); err != nil {
				return err
			}
		}
	}

	topLevel := topLevelExecutableStatements(prog.Statements)
	if len(topLevel) > 0 {
		scriptID, err := l.declareScriptFunction(modID)
		if err != nil {
			return err
		}
		fn := l.program.Functions[scriptID]
		fl := &functionLowerer{l: l, locals: map[string]LocalID{}, fn: &fn}
		body, err := fl.lowerBlock(topLevel)
		if err != nil {
			return err
		}
		fn.Body = body
		l.program.Functions[scriptID] = fn
		l.program.Script = scriptID
		mod.Functions = appendUniqueFunction(mod.Functions, scriptID)
	}

	l.loweredModules[path] = true
	return nil
}

func (l *lowerer) internModule(path string) ModuleID {
	if id, ok := l.moduleByPath[path]; ok {
		return id
	}
	id := ModuleID(len(l.program.Modules))
	l.moduleByPath[path] = id
	l.program.Modules = append(l.program.Modules, Module{
		ID:   id,
		Path: path,
	})
	return id
}

func (l *lowerer) declareGlobal(module ModuleID, def *checker.VariableDef) (GlobalID, error) {
	key := globalKey(module, def.Name)
	if id, ok := l.globals[key]; ok {
		return id, nil
	}
	typeID, err := l.internType(def.Type())
	if err != nil {
		return NoGlobal, err
	}
	id := GlobalID(len(l.program.Globals))
	l.globals[key] = id
	l.program.Globals = append(l.program.Globals, Global{
		ID:      id,
		Module:  module,
		Name:    def.Name,
		Type:    typeID,
		Mutable: def.Mutable,
		Private: def.Mutable,
	})
	l.program.Modules[module].Globals = appendUniqueGlobal(l.program.Modules[module].Globals, id)
	return id, nil
}

func (l *lowerer) lowerGlobal(module ModuleID, def *checker.VariableDef) error {
	id, ok := l.globals[globalKey(module, def.Name)]
	if !ok {
		return fmt.Errorf("global was not declared before lowering: %s", def.Name)
	}
	return l.lowerGlobalByID(id, def)
}

func (l *lowerer) lowerGlobalByID(id GlobalID, def *checker.VariableDef) error {
	if l.loweredGlobals[id] {
		return nil
	}
	if l.loweringGlobals[id] {
		return fmt.Errorf("cyclic global initializer %s", def.Name)
	}
	l.loweringGlobals[id] = true
	defer delete(l.loweringGlobals, id)

	global := l.program.Globals[id]
	fn := Function{Module: global.Module, Name: "<global>"}
	fl := l.newFunctionLowerer(&fn, nil, nil)
	contextType, err := fl.contextualType(global.Type)
	if err != nil {
		return err
	}
	value, actualType, err := fl.lowerContextualExpr(def.Value, contextType)
	if err != nil {
		return err
	}
	global.Type = actualType
	global.Value = *value
	l.program.Globals[id] = global
	l.loweredGlobals[id] = true
	return nil
}

func (l *lowerer) declareFunction(module ModuleID, def *checker.FunctionDef) (FunctionID, error) {
	if functionHasUnresolvedTypeVar(def) {
		return NoFunction, fmt.Errorf("cannot declare unspecialized generic function %s", def.Name)
	}
	key := functionKey(module, def.Name)
	if id, ok := l.functions[key]; ok {
		return id, nil
	}
	params := make([]Param, len(def.Parameters))
	for i, param := range def.Parameters {
		typeID, err := l.internType(param.Type)
		if err != nil {
			return NoFunction, err
		}
		params[i] = Param{Name: param.Name, Type: typeID, Mutable: param.Mutable}
	}
	returnType, err := l.internType(def.ReturnType)
	if err != nil {
		return NoFunction, err
	}
	id := FunctionID(len(l.program.Functions))
	l.functions[key] = id
	l.program.Functions = append(l.program.Functions, Function{
		ID:     id,
		Module: module,
		Name:   def.Name,
		Signature: Signature{
			Params: params,
			Return: returnType,
		},
		IsTest:  def.IsTest,
		Private: def.Private,
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	if def.Name == "main" {
		l.program.Entry = id
	}
	if l.includeTests && def.IsTest {
		l.program.Tests = append(l.program.Tests, Test{Name: def.Name, Function: id})
	}
	return id, nil
}

func (l *lowerer) declareFunctionSpecialization(module ModuleID, def *checker.FunctionDef) (FunctionID, error) {
	if functionHasUnresolvedTypeVar(def) {
		return NoFunction, fmt.Errorf("cannot declare unspecialized generic function %s", def.Name)
	}
	signature, err := l.signatureForFunction(def.Parameters, def.ReturnType)
	if err != nil {
		return NoFunction, err
	}
	return l.declareFunctionSpecializationWithSignature(module, def, signature)
}

func (l *lowerer) declareFunctionSpecializationWithSignature(module ModuleID, def *checker.FunctionDef, signature Signature) (FunctionID, error) {
	genericKey, err := l.genericBindingsKey(def)
	if err != nil {
		return NoFunction, err
	}
	return l.declareFunctionSpecializationWithSignatureAndGenericKey(module, def, signature, genericKey)
}

func (l *lowerer) declareFunctionSpecializationWithSignatureAndGenericKey(module ModuleID, def *checker.FunctionDef, signature Signature, genericKey string) (FunctionID, error) {
	if genericKey == "" {
		if id, ok := l.functions[functionKey(module, def.Name)]; ok {
			if signaturesEqual(l.program.Functions[id].Signature, signature) {
				return id, nil
			}
		}
	}
	key := concreteFunctionKey(module, def.Name, signature, genericKey)
	if id, ok := l.functions[key]; ok {
		return id, nil
	}
	id := FunctionID(len(l.program.Functions))
	l.functions[key] = id
	l.program.Functions = append(l.program.Functions, Function{
		ID:        id,
		Module:    module,
		Name:      def.Name,
		Signature: signature,
		IsTest:    def.IsTest,
		Private:   def.Private,
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	return id, nil
}

// declareGenericFunctionDef lowers a generic function exactly once as a Go
// generic definition (ADR 0031, Phase 2). Its signature and body reference
// TypeParam-kind types; call sites reference this single definition and supply
// concrete type arguments via Expr.TypeArgs.
// genericParamNames returns the ordered generic parameter names of a function
// definition. At call sites the checker leaves GenericParams empty but records
// the resolved type variables in GenericBindings, so fall back to the sorted
// binding keys to keep a stable ordering across call sites.
func genericParamNames(def *checker.FunctionDef) []string {
	if len(def.GenericParams) > 0 {
		return def.GenericParams
	}
	keys := make([]string, 0, len(def.GenericBindings))
	for k := range def.GenericBindings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// originalGenericFunctionDef returns the unsubstituted generic definition (with
// $T parameters) for a call-site definition. Call sites carry a derefed clone
// whose parameters are already concrete, so recover the original from the
// registry (covers private/same-module) or the module's public symbols.
func (l *lowerer) originalGenericFunctionDef(module ModuleID, callDef *checker.FunctionDef) *checker.FunctionDef {
	if orig, ok := l.genericFunctionOriginals[functionKey(module, callDef.Name)]; ok {
		return orig
	}
	if mod, ok := l.moduleByName[l.program.Modules[module].Path]; ok {
		if orig, ok := mod.Get(callDef.Name).Type.(*checker.FunctionDef); ok {
			return orig
		}
	}
	return callDef
}

func (l *lowerer) declareGenericFunctionDef(module ModuleID, callDef *checker.FunctionDef) (FunctionID, error) {
	key := functionKey(module, callDef.Name)
	if id, ok := l.genericFunctionDefs[key]; ok {
		return id, nil
	}
	def := l.originalGenericFunctionDef(module, callDef)
	paramNames := genericParamNames(callDef)
	params := map[string]int{}
	goParams := make([]string, len(paramNames))
	for i, p := range paramNames {
		params[p] = i
		goParams[i] = goifyTypeParamName(p)
	}
	prev := l.defParams
	l.defParams = params
	signature, err := l.signatureForFunction(def.Parameters, def.ReturnType)
	l.defParams = prev
	if err != nil {
		return NoFunction, err
	}
	id := FunctionID(len(l.program.Functions))
	l.functions[concreteFunctionKey(module, def.Name, signature, "genericdef")] = id
	l.genericFunctionDefs[key] = id
	l.program.Functions = append(l.program.Functions, Function{
		ID:         id,
		Module:     module,
		Name:       def.Name,
		Signature:  signature,
		TypeParams: goParams,
		IsTest:     def.IsTest,
		Private:    def.Private,
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	typeVars := make(map[string]TypeID, len(params))
	for p, idx := range params {
		tp, err := l.internTypeParam(p, idx)
		if err != nil {
			return NoFunction, err
		}
		typeVars[p] = tp
	}
	l.setFunctionTypeVars(id, typeVars)
	if err := l.lowerFunctionByID(id, def); err != nil {
		return NoFunction, err
	}
	return id, nil
}

func (l *lowerer) declareAndLowerFunction(module ModuleID, def *checker.FunctionDef) (FunctionID, error) {
	id, err := l.declareFunctionSpecialization(module, def)
	if err != nil {
		return NoFunction, err
	}
	if err := l.lowerFunctionByID(id, def); err != nil {
		return NoFunction, err
	}
	return id, nil
}

func (fl *functionLowerer) declareAndLowerFunctionCall(module ModuleID, def *checker.FunctionDef, call *checker.FunctionCall) (FunctionID, error) {
	// Generic functions are lowered once as a Go generic definition (ADR 0031,
	// Phase 2) rather than monomorphized per call.
	if len(def.GenericBindings) > 0 {
		return fl.l.declareGenericFunctionDef(module, def)
	}
	if functionHasUnresolvedTypeVar(def) && len(def.GenericBindings) == 0 {
		return NoFunction, fmt.Errorf("cannot declare unspecialized generic function %s", def.Name)
	}
	signature, err := fl.signatureForCall(call)
	if err != nil {
		return NoFunction, err
	}
	genericKey, typeVars, err := fl.genericBindingsKeyAndTypeVars(def)
	if err != nil {
		return NoFunction, err
	}
	id, err := fl.l.declareFunctionSpecializationWithSignatureAndGenericKey(module, def, signature, genericKey)
	if err != nil {
		return NoFunction, err
	}
	fl.l.setFunctionTypeVars(id, typeVars)
	if err := fl.l.lowerFunctionByID(id, def); err != nil {
		return NoFunction, err
	}
	return id, nil
}

func (l *lowerer) declareClosureFunction(module ModuleID, keyName string, def *checker.FunctionDef, typeID TypeID) (FunctionID, error) {
	typeInfo, ok := l.typeInfo(typeID)
	if !ok || typeInfo.Kind != TypeFunction {
		return NoFunction, fmt.Errorf("closure %s lowered with non-function AIR type %d", def.Name, typeID)
	}
	if len(def.Parameters) != len(typeInfo.Params) {
		return NoFunction, fmt.Errorf("closure %s expects %d params, got %d AIR params", def.Name, len(def.Parameters), len(typeInfo.Params))
	}
	params := make([]Param, len(def.Parameters))
	for i, param := range def.Parameters {
		// Match the expected function type's interned parameter shape exactly so
		// the closure's Go signature agrees with it. The expected type already
		// reconciled the two `mut T` representations via internFunctionParamType.
		mutable := param.Mutable
		if i < len(typeInfo.ParamMutable) {
			mutable = typeInfo.ParamMutable[i]
		}
		params[i] = Param{Name: param.Name, Type: typeInfo.Params[i], Mutable: mutable}
	}
	signature := Signature{Params: params, Return: typeInfo.Return}
	key := concreteFunctionKey(module, keyName, signature, "")
	if id, ok := l.functions[key]; ok {
		return id, nil
	}
	id := FunctionID(len(l.program.Functions))
	l.functions[key] = id
	l.program.Functions = append(l.program.Functions, Function{
		ID:        id,
		Module:    module,
		Name:      def.Name,
		Signature: signature,
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	return id, nil
}

func (l *lowerer) declareScriptFunction(module ModuleID) (FunctionID, error) {
	key := functionKey(module, "<script>")
	if id, ok := l.functions[key]; ok {
		return id, nil
	}
	returnType, err := l.internType(checker.Void)
	if err != nil {
		return NoFunction, err
	}
	id := FunctionID(len(l.program.Functions))
	l.functions[key] = id
	l.program.Functions = append(l.program.Functions, Function{
		ID:       id,
		Module:   module,
		Name:     "<script>",
		IsScript: true,
		Signature: Signature{
			Return: returnType,
		},
	})
	return id, nil
}

func (l *lowerer) lowerFunction(module ModuleID, def *checker.FunctionDef) error {
	id, ok := l.functions[functionKey(module, def.Name)]
	if !ok {
		return fmt.Errorf("function was not declared before lowering: %s", def.Name)
	}
	return l.lowerFunctionByID(id, def)
}

func (l *lowerer) lowerFunctionByID(id FunctionID, def *checker.FunctionDef) error {
	if l.loweredFuncs[id] {
		return nil
	}
	if l.loweringFuncs[id] {
		return nil
	}
	l.loweringFuncs[id] = true
	defer delete(l.loweringFuncs, id)
	fn := l.program.Functions[id]
	fl := l.newFunctionLowerer(&fn, def, nil)
	for _, param := range fn.Signature.Params {
		fl.defineLocal(param.Name, param.Type, param.Mutable)
	}
	if def.Body == nil {
		l.program.Functions[id] = fn
		l.loweredFuncs[id] = true
		return nil
	}
	body, err := fl.lowerBlock(def.Body.Stmts)
	if err != nil {
		return fmt.Errorf("lower function %s: %w", def.Name, err)
	}
	fn.Body = body
	l.program.Functions[id] = fn
	l.loweredFuncs[id] = true
	return nil
}

func (l *lowerer) newFunctionLowerer(fn *Function, def *checker.FunctionDef, parent *functionLowerer) *functionLowerer {
	fl := &functionLowerer{
		l:        l,
		locals:   map[string]LocalID{},
		fn:       fn,
		parent:   parent,
		typeVars: map[string]TypeID{},
	}
	if parent != nil {
		for name, typeID := range parent.typeVars {
			fl.typeVars[name] = typeID
		}
	}
	if def != nil {
		paramOffset := 0
		if len(fn.Signature.Params) == len(def.Parameters)+1 {
			receiver := def.Receiver
			if receiver == "" {
				receiver = "self"
			}
			if fn.Signature.Params[0].Name == receiver {
				paramOffset = 1
			}
		}
		for i, param := range def.Parameters {
			signatureIndex := i + paramOffset
			if signatureIndex < len(fn.Signature.Params) {
				fl.bindTypeVars(param.Type, fn.Signature.Params[signatureIndex].Type)
			}
		}
		fl.bindTypeVars(def.ReturnType, fn.Signature.Return)
		for name, typeID := range l.functionTypeVars[fn.ID] {
			fl.typeVars[name] = typeID
		}
		for name, typ := range def.GenericBindings {
			if _, ok := fl.typeVars[name]; ok {
				continue
			}
			typeID, err := fl.internResolvedType(typ)
			if err == nil {
				fl.typeVars[name] = typeID
			}
		}
	}
	return fl
}

func (fl *functionLowerer) bindTypeVars(pattern checker.Type, actual TypeID) {
	if pattern == nil || actual == NoType || !validTypeID(&fl.l.program, actual) {
		return
	}
	if tv, ok := pattern.(*checker.TypeVar); ok {
		if _, ok := fl.typeVars[tv.Name()]; !ok {
			fl.typeVars[tv.Name()] = actual
		}
		return
	}
	actualInfo, ok := fl.l.typeInfo(actual)
	if !ok {
		return
	}
	switch typ := pattern.(type) {
	case *checker.List:
		if actualInfo.Kind == TypeList {
			fl.bindTypeVars(typ.Of(), actualInfo.Elem)
		}
	case *checker.Chan:
		if actualInfo.Kind == TypeChannel {
			fl.bindTypeVars(typ.Of(), actualInfo.Elem)
		}
	case *checker.Receiver:
		if actualInfo.Kind == TypeReceiver {
			fl.bindTypeVars(typ.Of(), actualInfo.Elem)
		}
	case *checker.Sender:
		if actualInfo.Kind == TypeSender {
			fl.bindTypeVars(typ.Of(), actualInfo.Elem)
		}
	case *checker.Map:
		if actualInfo.Kind == TypeMap {
			fl.bindTypeVars(typ.Key(), actualInfo.Key)
			fl.bindTypeVars(typ.Value(), actualInfo.Value)
		}
	case *checker.Maybe:
		if actualInfo.Kind == TypeMaybe {
			fl.bindTypeVars(typ.Of(), actualInfo.Elem)
		}
	case *checker.MutableRef:
		fl.bindTypeVars(typ.Of(), actual)
	case *checker.Result:
		if actualInfo.Kind == TypeResult {
			fl.bindTypeVars(typ.Val(), actualInfo.Value)
			fl.bindTypeVars(typ.Err(), actualInfo.Error)
		}
	case *checker.FunctionDef:
		if actualInfo.Kind == TypeFunction {
			for i, param := range typ.Parameters {
				if i < len(actualInfo.Params) {
					fl.bindTypeVars(param.Type, actualInfo.Params[i])
				}
			}
			fl.bindTypeVars(typ.ReturnType, actualInfo.Return)
		}
	case *checker.StructDef:
		if actualInfo.Kind == TypeStruct {
			fieldsByName := map[string]FieldInfo{}
			for _, field := range actualInfo.Fields {
				fieldsByName[field.Name] = field
			}
			for name, fieldType := range typ.Fields {
				if field, ok := fieldsByName[name]; ok {
					fl.bindTypeVars(fieldType, field.Type)
				}
			}
		}
	}
}

func (fl *functionLowerer) internType(t checker.Type) (TypeID, error) {
	if tv, ok := t.(*checker.TypeVar); ok {
		if id, ok := fl.typeVars[tv.Name()]; ok {
			return id, nil
		}
		if tv.Actual() != nil {
			return fl.internType(tv.Actual())
		}
		return fl.l.internType(checker.Void)
	}
	if !typeContainsTypeVar(t) {
		return fl.l.internType(t)
	}
	return fl.internCompositeType(t)
}

func (fl *functionLowerer) internResolvedType(t checker.Type) (TypeID, error) {
	if t == nil {
		return NoType, fmt.Errorf("cannot intern nil type")
	}
	if tv, ok := t.(*checker.TypeVar); ok {
		if id, ok := fl.typeVars[tv.Name()]; ok {
			return id, nil
		}
		if tv.Actual() != nil {
			return fl.internResolvedType(tv.Actual())
		}
		return NoType, fmt.Errorf("unresolved generic type variable $%s", tv.Name())
	}
	if !typeContainsTypeVar(t) {
		return fl.l.internType(t)
	}
	return fl.internResolvedCompositeType(t)
}

func (fl *functionLowerer) internContextualCheckerType(t checker.Type) (TypeID, error) {
	typeID, err := fl.internType(t)
	if err == nil {
		return fl.contextualType(typeID)
	}
	if !typeHasUnresolvedTypeVar(t) {
		return NoType, err
	}
	return fl.internWeakContextType(t)
}

func (fl *functionLowerer) internWeakContextType(t checker.Type) (TypeID, error) {
	switch typ := t.(type) {
	case *checker.TypeVar:
		if id, ok := fl.typeVars[typ.Name()]; ok {
			return id, nil
		}
		if typ.Actual() != nil {
			return fl.internWeakContextType(typ.Actual())
		}
		return fl.l.internType(checker.Void)
	case *checker.Maybe:
		elem, err := fl.internWeakContextPart(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.internMaybeType(elem), nil
	case *checker.Result:
		value, err := fl.internWeakContextPart(typ.Val())
		if err != nil {
			return NoType, err
		}
		errType, err := fl.internWeakContextPart(typ.Err())
		if err != nil {
			return NoType, err
		}
		return fl.internResultType(value, errType), nil
	default:
		return NoType, fmt.Errorf("unresolved generic type variable in contextual type %s", t.String())
	}
}

func (fl *functionLowerer) internWeakContextPart(t checker.Type) (TypeID, error) {
	typeID, err := fl.internType(t)
	if err == nil {
		return typeID, nil
	}
	if !typeHasUnresolvedTypeVar(t) {
		return NoType, err
	}
	return fl.l.internType(checker.Void)
}

func (fl *functionLowerer) contextualType(typeID TypeID) (TypeID, error) {
	if !validTypeID(&fl.l.program, typeID) {
		return typeID, nil
	}
	info, _ := fl.l.typeInfo(typeID)
	switch info.Kind {
	case TypeMaybe:
		if validTypeID(&fl.l.program, info.Elem) {
			return typeID, nil
		}
		voidID, err := fl.l.internType(checker.Void)
		if err != nil {
			return NoType, err
		}
		return fl.internMaybeType(voidID), nil
	case TypeResult:
		if validTypeID(&fl.l.program, info.Value) && validTypeID(&fl.l.program, info.Error) {
			return typeID, nil
		}
		voidID, err := fl.l.internType(checker.Void)
		if err != nil {
			return NoType, err
		}
		value := info.Value
		if !validTypeID(&fl.l.program, value) {
			value = voidID
		}
		errType := info.Error
		if !validTypeID(&fl.l.program, errType) {
			errType = voidID
		}
		return fl.internResultType(value, errType), nil
	default:
		return typeID, nil
	}
}

func (fl *functionLowerer) internStructType(typ *checker.StructDef) (TypeID, error) {
	return fl.internStructTypeWithInterner(typ, fl.internType)
}

func (fl *functionLowerer) internResolvedStructType(typ *checker.StructDef) (TypeID, error) {
	return fl.internStructTypeWithInterner(typ, fl.internResolvedType)
}

// internFunctionParamType interns a function parameter's type and reports its
// effective AIR mutability, reconciling the two ways a `mut T` parameter is
// represented (a MutableRef baked into the type, or the Mutable flag with a
// plain type).
func (l *lowerer) internFunctionParamType(param checker.Parameter, intern func(checker.Type) (TypeID, error)) (TypeID, bool, error) {
	underlying := param.Type
	mutable := param.Mutable
	if ref, ok := param.Type.(*checker.MutableRef); ok {
		underlying = ref.Of()
		mutable = true
	}
	// A pointer-shaped foreign Go param is its own mutability marker: imported
	// Go signatures never set the Mutable flag while `mut pkg::T` spellings
	// do. Normalize to mutable so checker-equal function types intern to the
	// same AIR type (mirrors checker.normalizedParamMutability).
	if foreign, ok := underlying.(*checker.ForeignType); ok && foreign.Pointer {
		mutable = true
	}
	if mutable {
		id, err := intern(underlying)
		return id, true, err
	}
	id, err := intern(underlying)
	return id, false, err
}

func (l *lowerer) internStructFieldType(fieldTypeValue checker.Type, intern func(checker.Type) (TypeID, error)) (TypeID, bool, error) {
	if ref, ok := fieldTypeValue.(*checker.MutableRef); ok {
		id, err := intern(ref.Of())
		return id, true, err
	}
	id, err := intern(fieldTypeValue)
	return id, false, err
}

func (fl *functionLowerer) internStructTypeWithInterner(typ *checker.StructDef, intern func(checker.Type) (TypeID, error)) (TypeID, error) {
	fields := sortedFieldNames(typ.Fields)
	info := TypeInfo{Kind: TypeStruct, ModulePath: typ.ModulePath, Private: typ.Private}
	info.Fields = make([]FieldInfo, len(fields))
	for i, name := range fields {
		fieldType, fieldMutable, err := fl.l.internStructFieldType(typ.Fields[name], intern)
		if err != nil {
			return NoType, err
		}
		info.Fields[i] = FieldInfo{Name: name, Type: fieldType, Index: i, Mutable: fieldMutable}
	}
	name := typ.Name
	if len(typ.TypeArgs) > 0 {
		parts := make([]string, len(typ.TypeArgs))
		for i, typeArg := range typ.TypeArgs {
			typeID, err := intern(typeArg)
			if err != nil {
				return NoType, err
			}
			parts[i] = fl.l.typeName(typeID)
		}
		name += "<" + strings.Join(parts, ",") + ">"
	}
	key := "struct " + typ.ModulePath + "::" + name
	if id, ok := fl.l.typeByKey[key]; ok {
		return id, nil
	}
	// Tag instantiations of a generic struct so the backend emits `Def[args...]`
	// (ADR 0031). Interning the definition appends types, so resolve it before
	// reserving this instance's id.
	if len(typ.GenericParams) > 0 {
		defID, ok := fl.l.genericStructDefs[genericStructDefKey(typ.ModulePath, typ.Name)]
		if !ok {
			if genDef := fl.l.lookupGenericStructDef(typ.ModulePath, typ.Name); genDef != nil {
				if interned, err := fl.l.internGenericStructDef(genDef); err == nil {
					defID, ok = interned, true
				} else {
					return NoType, err
				}
			}
		}
		if ok {
			info.Generic = defID
			info.GenericArgs = make([]TypeID, 0, len(typ.TypeArgs))
			for _, typeArg := range typ.TypeArgs {
				argID, err := intern(typeArg)
				if err != nil {
					return NoType, err
				}
				info.GenericArgs = append(info.GenericArgs, argID)
			}
		}
	}
	id := TypeID(len(fl.l.program.Types) + 1)
	info.ID = id
	info.Name = name
	fl.l.typeByKey[key] = id
	fl.l.program.Types = append(fl.l.program.Types, info)
	return id, nil
}

func (fl *functionLowerer) internResolvedCompositeType(t checker.Type) (TypeID, error) {
	switch typ := t.(type) {
	case *checker.StructDef:
		return fl.internResolvedStructType(typ)
	case *checker.List:
		elem, err := fl.internResolvedType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("["+fl.l.typeName(elem)+"]", TypeInfo{Kind: TypeList, Elem: elem})
	case *checker.Chan:
		elem, err := fl.internResolvedType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("Chan<"+fl.l.typeName(elem)+">", TypeInfo{Kind: TypeChannel, Elem: elem})
	case *checker.Receiver:
		elem, err := fl.internResolvedType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("Receiver<"+fl.l.typeName(elem)+">", TypeInfo{Kind: TypeReceiver, Elem: elem})
	case *checker.Sender:
		elem, err := fl.internResolvedType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("Sender<"+fl.l.typeName(elem)+">", TypeInfo{Kind: TypeSender, Elem: elem})
	case *checker.Map:
		key, err := fl.internResolvedType(typ.Key())
		if err != nil {
			return NoType, err
		}
		value, err := fl.internResolvedType(typ.Value())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("["+fl.l.typeName(key)+":"+fl.l.typeName(value)+"]", TypeInfo{Kind: TypeMap, Key: key, Value: value})
	case *checker.Maybe:
		elemType := typ.Of()
		elemMutable := false
		if ref, ok := elemType.(*checker.MutableRef); ok {
			elemMutable = true
			elemType = ref.Of()
		}
		elem, err := fl.internResolvedType(elemType)
		if err != nil {
			return NoType, err
		}
		name := fl.l.typeName(elem) + "?"
		if elemMutable {
			name = "(mut " + fl.l.typeName(elem) + ")?"
		}
		return fl.l.internSyntheticType(name, TypeInfo{Kind: TypeMaybe, Elem: elem, ElemMutable: elemMutable})
	case *checker.Result:
		value, err := fl.internResolvedType(typ.Val())
		if err != nil {
			return NoType, err
		}
		errType, err := fl.internResolvedType(typ.Err())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType(fl.l.typeName(value)+"!"+fl.l.typeName(errType), TypeInfo{Kind: TypeResult, Value: value, Error: errType})
	case *checker.FunctionDef:
		params := make([]TypeID, len(typ.Parameters))
		mutable := make([]bool, len(typ.Parameters))
		for i, param := range typ.Parameters {
			paramType, paramMut, err := fl.l.internFunctionParamType(param, fl.internResolvedType)
			if err != nil {
				return NoType, err
			}
			params[i] = paramType
			mutable[i] = paramMut
		}
		returnType, err := fl.internResolvedType(typ.ReturnType)
		if err != nil {
			return NoType, err
		}
		name := "fn("
		for i, param := range params {
			if i > 0 {
				name += ","
			}
			name += fl.l.typeName(param)
		}
		name += ") " + fl.l.typeName(returnType)
		return fl.l.internSyntheticType(name, TypeInfo{Kind: TypeFunction, Params: params, ParamMutable: mutable, Return: returnType})
	}
	return NoType, fmt.Errorf("unresolved generic type variable in %s", t.String())
}

func (fl *functionLowerer) internCompositeType(t checker.Type) (TypeID, error) {
	switch typ := t.(type) {
	case *checker.StructDef:
		return fl.internStructType(typ)
	case *checker.List:
		elem, err := fl.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("["+fl.l.typeName(elem)+"]", TypeInfo{Kind: TypeList, Elem: elem})
	case *checker.Chan:
		elem, err := fl.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("Chan<"+fl.l.typeName(elem)+">", TypeInfo{Kind: TypeChannel, Elem: elem})
	case *checker.Receiver:
		elem, err := fl.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("Receiver<"+fl.l.typeName(elem)+">", TypeInfo{Kind: TypeReceiver, Elem: elem})
	case *checker.Sender:
		elem, err := fl.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("Sender<"+fl.l.typeName(elem)+">", TypeInfo{Kind: TypeSender, Elem: elem})
	case *checker.Map:
		key, err := fl.internType(typ.Key())
		if err != nil {
			return NoType, err
		}
		value, err := fl.internType(typ.Value())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("["+fl.l.typeName(key)+":"+fl.l.typeName(value)+"]", TypeInfo{Kind: TypeMap, Key: key, Value: value})
	case *checker.Maybe:
		elemType := typ.Of()
		elemMutable := false
		if ref, ok := elemType.(*checker.MutableRef); ok {
			elemMutable = true
			elemType = ref.Of()
		}
		elem, err := fl.internType(elemType)
		if err != nil {
			return NoType, err
		}
		name := fl.l.typeName(elem) + "?"
		if elemMutable {
			name = "(mut " + fl.l.typeName(elem) + ")?"
		}
		return fl.l.internSyntheticType(name, TypeInfo{Kind: TypeMaybe, Elem: elem, ElemMutable: elemMutable})
	case *checker.Result:
		value, err := fl.internType(typ.Val())
		if err != nil {
			return NoType, err
		}
		errType, err := fl.internType(typ.Err())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType(fl.l.typeName(value)+"!"+fl.l.typeName(errType), TypeInfo{Kind: TypeResult, Value: value, Error: errType})
	case *checker.FunctionDef:
		params := make([]TypeID, len(typ.Parameters))
		mutable := make([]bool, len(typ.Parameters))
		for i, param := range typ.Parameters {
			paramType, paramMut, err := fl.l.internFunctionParamType(param, fl.internType)
			if err != nil {
				return NoType, err
			}
			params[i] = paramType
			mutable[i] = paramMut
		}
		returnType, err := fl.internType(typ.ReturnType)
		if err != nil {
			return NoType, err
		}
		name := "fn("
		for i, param := range params {
			if i > 0 {
				name += ","
			}
			name += fl.l.typeName(param)
		}
		name += ") " + fl.l.typeName(returnType)
		return fl.l.internSyntheticType(name, TypeInfo{Kind: TypeFunction, Params: params, ParamMutable: mutable, Return: returnType})
	default:
		return fl.l.internType(t)
	}
}

func (l *lowerer) declareInherentImplMethodsForStruct(module ModuleID, def *checker.StructDef) error {
	if def == nil {
		return nil
	}
	ownerType, err := l.internType(def)
	if err != nil {
		return err
	}
	ownerInfo, ok := l.typeInfo(ownerType)
	if !ok {
		return fmt.Errorf("invalid struct method owner type %d", ownerType)
	}
	traitMethodNames := map[string]bool{}
	for _, trait := range def.Traits {
		if trait == nil {
			continue
		}
		for _, method := range trait.GetMethods() {
			traitMethodNames[method.Name] = true
		}
	}
	for name, method := range l.structMethods(def) {
		if traitMethodNames[name] || method == nil || functionHasUnresolvedTypeVar(method) {
			continue
		}
		id, err := l.declareInstanceMethodFunction(module, ownerInfo.Name, ownerType, method, nil, NoType)
		if err != nil {
			return err
		}
		if err := l.lowerInstanceMethodFunction(id, method); err != nil {
			return err
		}
	}
	return nil
}

func (l *lowerer) declareTraitImplsForType(module ModuleID, typ checker.Type) error {
	forType, err := l.internType(typ)
	if err != nil {
		return err
	}

	var traits []*checker.Trait
	var methods map[string]*checker.FunctionDef
	switch typed := typ.(type) {
	case *checker.StructDef:
		traits = typed.Traits
		methods = l.structMethods(typed)
	case *checker.Enum:
		traits = typed.Traits
		methods = typed.Methods
	default:
		return nil
	}

	for _, trait := range traits {
		if trait == nil {
			continue
		}
		if _, err := l.declareImpl(module, trait, typ, forType, methods); err != nil {
			return err
		}
	}
	return nil
}

func (l *lowerer) declareImpl(module ModuleID, trait *checker.Trait, owner checker.Type, ownerType TypeID, methods map[string]*checker.FunctionDef) (ImplID, error) {
	key := implKey(module, checkerTraitKey(trait), owner.String())
	if id, ok := l.impls[key]; ok {
		return id, nil
	}

	traitID, err := l.internTrait(trait)
	if err != nil {
		return 0, err
	}

	traitMethods := trait.GetMethods()
	methodIDs := make([]FunctionID, len(traitMethods))
	methodDefs := make([]*checker.FunctionDef, len(traitMethods))
	for i, traitMethod := range traitMethods {
		methodDef := methods[traitMethod.Name]
		if methodDef == nil {
			return 0, fmt.Errorf("missing method %s for impl %s on %s", traitMethod.Name, trait.Name, owner.String())
		}
		methodID, err := l.declareMethodFunction(module, owner, trait.Name, ownerType, methodDef)
		if err != nil {
			return 0, err
		}
		methodIDs[i] = methodID
		methodDefs[i] = methodDef
	}

	id := ImplID(len(l.program.Impls))
	l.impls[key] = id
	l.program.Impls = append(l.program.Impls, Impl{
		ID:      id,
		Trait:   traitID,
		ForType: ownerType,
		Methods: methodIDs,
	})

	for i, methodID := range methodIDs {
		if err := l.lowerMethodFunction(methodID, methodDefs[i]); err != nil {
			return 0, err
		}
	}

	return id, nil
}

func (l *lowerer) declareBuiltinTraitImpl(module ModuleID, traitID TraitID, ownerType TypeID) (ImplID, bool, error) {
	if !validTraitID(&l.program, traitID) {
		return 0, false, fmt.Errorf("invalid trait id %d", traitID)
	}
	trait := l.program.Traits[traitID]
	if len(trait.Methods) != 1 {
		return 0, false, nil
	}
	ownerInfo, ok := l.typeInfo(ownerType)
	if !ok {
		return 0, false, fmt.Errorf("invalid builtin trait owner type %d", ownerType)
	}
	switch ownerInfo.Kind {
	case TypeStr, TypeInt, TypeScalar, TypeFloat64, TypeBool, TypeByte, TypeRune:
	default:
		return 0, false, nil
	}
	if id, ok := l.lookupImpl(traitID, ownerType); ok {
		return id, true, nil
	}

	var methodID FunctionID
	var err error
	switch {
	case trait.Name == "ToString" && trait.Methods[0].Name == "to_str":
		methodID, err = l.declareBuiltinToStringMethod(module, ownerInfo)
	default:
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	id := ImplID(len(l.program.Impls))
	l.program.Impls = append(l.program.Impls, Impl{
		ID:      id,
		Trait:   traitID,
		ForType: ownerType,
		Methods: []FunctionID{methodID},
	})
	return id, true, nil
}

func (l *lowerer) declareBuiltinToStringMethod(module ModuleID, ownerInfo TypeInfo) (FunctionID, error) {
	key := methodFunctionKey(module, ownerInfo.Name, "ToString", "to_str")
	if id, ok := l.functions[key]; ok {
		return id, nil
	}
	strType, err := l.internType(checker.Str)
	if err != nil {
		return NoFunction, err
	}
	id := FunctionID(len(l.program.Functions))
	l.functions[key] = id
	receiver := Param{Name: "self", Type: ownerInfo.ID}
	l.program.Functions = append(l.program.Functions, Function{
		ID:         id,
		Module:     module,
		Name:       ownerInfo.Name + ".ToString.to_str",
		Receiver:   ownerInfo.ID,
		MethodName: "to_str",
		Signature: Signature{
			Params: []Param{receiver},
			Return: strType,
		},
		Locals: []Local{{ID: 0, Name: "self", Type: ownerInfo.ID}},
		Body: Block{Result: &Expr{
			Kind:   ExprToStr,
			Type:   strType,
			Target: &Expr{Kind: ExprLoadLocal, Type: ownerInfo.ID, Local: 0},
		}},
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	return id, nil
}

func (l *lowerer) declareMethodFunction(module ModuleID, owner checker.Type, traitName string, ownerType TypeID, def *checker.FunctionDef) (FunctionID, error) {
	key := methodFunctionKey(module, owner.String(), traitName, def.Name)
	if id, ok := l.functions[key]; ok {
		return id, nil
	}

	receiver := def.Receiver
	if receiver == "" {
		receiver = "self"
	}
	params := make([]Param, 0, len(def.Parameters)+1)
	params = append(params, Param{Name: receiver, Type: ownerType, Mutable: def.Mutates})
	for _, param := range def.Parameters {
		typeID, err := l.internType(param.Type)
		if err != nil {
			return NoFunction, err
		}
		params = append(params, Param{Name: param.Name, Type: typeID, Mutable: param.Mutable})
	}
	returnType, err := l.internType(def.ReturnType)
	if err != nil {
		return NoFunction, err
	}

	id := FunctionID(len(l.program.Functions))
	l.functions[key] = id
	l.program.Functions = append(l.program.Functions, Function{
		ID:         id,
		Module:     module,
		Name:       owner.String() + "." + traitName + "." + def.Name,
		Receiver:   ownerType,
		MethodName: def.Name,
		Signature: Signature{
			Params: params,
			Return: returnType,
		},
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	return id, nil
}

func (l *lowerer) lowerMethodFunction(id FunctionID, def *checker.FunctionDef) error {
	if !validFunctionID(&l.program, id) {
		return fmt.Errorf("method function has invalid id %d", id)
	}
	fn := l.program.Functions[id]
	fl := l.newFunctionLowerer(&fn, def, nil)
	for _, param := range fn.Signature.Params {
		fl.defineLocal(param.Name, param.Type, param.Mutable)
	}
	if def.Body != nil {
		body, err := fl.lowerBlock(def.Body.Stmts)
		if err != nil {
			return fmt.Errorf("lower method %s: %w", def.Name, err)
		}
		fn.Body = body
	}
	l.program.Functions[id] = fn
	return nil
}

func (l *lowerer) declareInstanceMethodFunction(module ModuleID, ownerName string, ownerType TypeID, def *checker.FunctionDef, args []checker.Expression, returnType TypeID) (FunctionID, error) {
	receiver := def.Receiver
	if receiver == "" {
		receiver = "self"
	}
	params := make([]Param, 0, len(def.Parameters)+1)
	params = append(params, Param{Name: receiver, Type: ownerType, Mutable: def.Mutates})
	for i, param := range def.Parameters {
		paramType := param.Type
		if typeHasUnresolvedTypeVar(paramType) && i < len(args) {
			paramType = args[i].Type()
		}
		typeID, err := l.internType(paramType)
		if err != nil {
			return NoFunction, err
		}
		params = append(params, Param{Name: param.Name, Type: typeID, Mutable: param.Mutable})
	}
	if !validTypeID(&l.program, returnType) {
		var err error
		returnType, err = l.internType(def.ReturnType)
		if err != nil {
			return NoFunction, err
		}
	}

	signature := Signature{Params: params, Return: returnType}
	genericKey, err := l.genericBindingsKey(def)
	if err != nil {
		return NoFunction, err
	}
	key := concreteFunctionKey(module, fmt.Sprintf("method/%s#%d/instance/%s", ownerName, ownerType, def.Name), signature, genericKey)
	if id, ok := l.functions[key]; ok {
		return id, nil
	}

	id := FunctionID(len(l.program.Functions))
	l.functions[key] = id
	l.program.Functions = append(l.program.Functions, Function{
		ID:         id,
		Module:     module,
		Name:       ownerName + "." + def.Name,
		Receiver:   ownerType,
		MethodName: def.Name,
		Signature: Signature{
			Params: params,
			Return: returnType,
		},
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	return id, nil
}

func (fl *functionLowerer) declareInstanceMethodFunction(module ModuleID, ownerName string, ownerType TypeID, def *checker.FunctionDef, args []checker.Expression, returnType TypeID) (FunctionID, error) {
	receiver := def.Receiver
	if receiver == "" {
		receiver = "self"
	}
	params := make([]Param, 0, len(def.Parameters)+1)
	params = append(params, Param{Name: receiver, Type: ownerType, Mutable: def.Mutates})
	for i, param := range def.Parameters {
		paramType := param.Type
		if typeHasUnresolvedTypeVar(paramType) && i < len(args) {
			paramType = args[i].Type()
		}
		typeID, err := fl.internType(paramType)
		if err != nil {
			return NoFunction, err
		}
		params = append(params, Param{Name: param.Name, Type: typeID, Mutable: param.Mutable})
	}
	if !validTypeID(&fl.l.program, returnType) {
		var err error
		returnType, err = fl.internType(def.ReturnType)
		if err != nil {
			return NoFunction, err
		}
	}

	signature := Signature{Params: params, Return: returnType}
	genericKey, typeVars, err := fl.genericBindingsKeyAndTypeVars(def)
	if err != nil {
		return NoFunction, err
	}
	key := concreteFunctionKey(module, fmt.Sprintf("method/%s#%d/instance/%s", ownerName, ownerType, def.Name), signature, genericKey)
	if id, ok := fl.l.functions[key]; ok {
		fl.l.setFunctionTypeVars(id, typeVars)
		return id, nil
	}

	id := FunctionID(len(fl.l.program.Functions))
	fl.l.functions[key] = id
	fl.l.setFunctionTypeVars(id, typeVars)
	fl.l.program.Functions = append(fl.l.program.Functions, Function{
		ID:         id,
		Module:     module,
		Name:       ownerName + "." + def.Name,
		Receiver:   ownerType,
		MethodName: def.Name,
		Signature: Signature{
			Params: params,
			Return: returnType,
		},
	})
	fl.l.program.Modules[module].Functions = appendUniqueFunction(fl.l.program.Modules[module].Functions, id)
	return id, nil
}

func (l *lowerer) lowerInstanceMethodFunction(id FunctionID, def *checker.FunctionDef) error {
	return l.lowerFunctionByID(id, def)
}

// genericSelfInstance returns the TypeID of a generic struct instantiated at its
// own type parameters (e.g. `Chan[T]`), used as the receiver type of a
// generic method definition (ADR 0031).
func (l *lowerer) genericSelfInstance(defID TypeID) (TypeID, error) {
	if !validTypeID(&l.program, defID) {
		return NoType, fmt.Errorf("invalid generic struct definition %d", defID)
	}
	def := l.program.Types[defID-1]
	args := make([]TypeID, len(def.TypeParams))
	for i, name := range def.TypeParams {
		tp, err := l.internTypeParam(name, i)
		if err != nil {
			return NoType, err
		}
		args[i] = tp
	}
	info := TypeInfo{
		Kind:        TypeStruct,
		ModulePath:  def.ModulePath,
		Private:     def.Private,
		Fields:      def.Fields,
		Generic:     defID,
		GenericArgs: args,
	}
	return l.internSyntheticType(def.Name+"<self>", info)
}

// declareGenericInstanceMethodFunction lowers a method on a generic struct once
// as a generic definition whose receiver is `Owner[T...]` and whose parameters,
// return type, and body reference the struct's type parameters (ADR 0031). It
// returns the function id and the concrete type arguments for the call site.
// methodUsesOnlyStructTypeParams reports whether a method's type variables are
// exactly the owning struct's type parameters (no method-introduced generics),
// which is the condition for lowering it as a Go generic-receiver method.
func methodUsesOnlyStructTypeParams(def *checker.FunctionDef, structParams []string) bool {
	if len(def.GenericBindings) == 0 {
		return true
	}
	allowed := make(map[string]bool, len(structParams))
	for _, p := range structParams {
		allowed[p] = true
	}
	for name := range def.GenericBindings {
		if !allowed[name] {
			return false
		}
	}
	return true
}

func (fl *functionLowerer) declareGenericInstanceMethodFunction(module ModuleID, instanceType TypeID, structType *checker.StructDef, callDef *checker.FunctionDef) (FunctionID, []TypeID, error) {
	info, ok := fl.l.typeInfo(instanceType)
	if !ok || info.Generic == NoType {
		return NoFunction, nil, fmt.Errorf("generic method receiver %d is not a generic instantiation", instanceType)
	}
	defID := info.Generic
	structDef := fl.l.program.Types[defID-1]
	paramNames := structDef.TypeParams
	typeArgs := info.GenericArgs

	key := "genericmethod:" + structDef.ModulePath + ":" + structDef.Name + ":" + callDef.Name
	if id, ok := fl.l.genericMethodDefs[key]; ok {
		return id, typeArgs, nil
	}

	orig := callDef
	if methods := fl.l.structMethods(structType); methods != nil {
		if m, ok := methods[callDef.Name]; ok && m != nil {
			orig = m
		}
	}

	recvType, err := fl.l.genericSelfInstance(defID)
	if err != nil {
		return NoFunction, nil, err
	}
	receiver := orig.Receiver
	if receiver == "" {
		receiver = "self"
	}

	params := map[string]int{}
	for i, p := range paramNames {
		params[p] = i
	}
	prev := fl.l.defParams
	fl.l.defParams = params
	methodParams := make([]Param, 0, len(orig.Parameters)+1)
	methodParams = append(methodParams, Param{Name: receiver, Type: recvType, Mutable: orig.Mutates})
	for _, p := range orig.Parameters {
		tid, err := fl.l.internType(p.Type)
		if err != nil {
			fl.l.defParams = prev
			return NoFunction, nil, err
		}
		methodParams = append(methodParams, Param{Name: p.Name, Type: tid, Mutable: p.Mutable})
	}
	returnType, err := fl.l.internType(orig.ReturnType)
	fl.l.defParams = prev
	if err != nil {
		return NoFunction, nil, err
	}

	signature := Signature{Params: methodParams, Return: returnType}
	id := FunctionID(len(fl.l.program.Functions))
	fl.l.genericMethodDefs[key] = id
	fl.l.functions[concreteFunctionKey(module, structDef.Name+"."+callDef.Name, signature, "genericmethod")] = id
	fl.l.program.Functions = append(fl.l.program.Functions, Function{
		ID:         id,
		Module:     module,
		Name:       structDef.Name + "." + callDef.Name,
		Receiver:   recvType,
		MethodName: callDef.Name,
		Signature:  signature,
		TypeParams: paramNames,
	})
	fl.l.program.Modules[module].Functions = appendUniqueFunction(fl.l.program.Modules[module].Functions, id)
	typeVars := make(map[string]TypeID, len(paramNames))
	for p, idx := range params {
		tp, err := fl.l.internTypeParam(p, idx)
		if err != nil {
			return NoFunction, nil, err
		}
		typeVars[p] = tp
	}
	fl.l.setFunctionTypeVars(id, typeVars)
	if err := fl.l.lowerFunctionByID(id, orig); err != nil {
		return NoFunction, nil, err
	}
	return id, typeArgs, nil
}

func (l *lowerer) signatureForCall(call *checker.FunctionCall) (Signature, error) {
	return signatureForCallWithInterner(call, l.internType)
}

func (fl *functionLowerer) signatureForCall(call *checker.FunctionCall) (Signature, error) {
	return signatureForCallWithInterner(call, fl.internType)
}

func (fl *functionLowerer) typeArgsForCall(call *checker.FunctionCall) ([]TypeID, error) {
	return typeArgsForCallWithInterner(call, fl.internType)
}

func typeArgsForCallWithInterner(call *checker.FunctionCall, intern func(checker.Type) (TypeID, error)) ([]TypeID, error) {
	if len(call.TypeArgs) == 0 {
		return nil, nil
	}
	typeArgs := make([]TypeID, len(call.TypeArgs))
	for i, typeArg := range call.TypeArgs {
		typeID, err := intern(typeArg)
		if err != nil {
			return nil, err
		}
		typeArgs[i] = typeID
	}
	return typeArgs, nil
}

func signatureForCallWithInterner(call *checker.FunctionCall, intern func(checker.Type) (TypeID, error)) (Signature, error) {
	if def := call.Definition(); def != nil {
		if !functionHasTypeVar(def) {
			return signatureForFunctionWithInterner(def.Parameters, def.ReturnType, intern)
		}
		params := make([]Param, len(def.Parameters))
		for i, param := range def.Parameters {
			paramType := param.Type
			if i < len(call.Args) {
				paramType = call.Args[i].Type()
			}
			typeID, err := intern(paramType)
			if err != nil {
				return Signature{}, err
			}
			params[i] = Param{Name: param.Name, Type: typeID, Mutable: param.Mutable}
		}
		returnType, err := intern(call.Type())
		if err != nil {
			return Signature{}, err
		}
		return Signature{Params: params, Return: returnType}, nil
	}
	params := make([]Param, len(call.Args))
	for i, arg := range call.Args {
		typeID, err := intern(arg.Type())
		if err != nil {
			return Signature{}, err
		}
		params[i] = Param{Name: fmt.Sprintf("arg%d", i), Type: typeID}
	}
	returnType, err := intern(call.Type())
	if err != nil {
		return Signature{}, err
	}
	return Signature{Params: params, Return: returnType}, nil
}

func genericStructDefKey(modulePath, name string) string {
	return "genericdef:" + modulePath + ":" + name
}

func goifyTypeParamName(name string) string {
	name = strings.TrimPrefix(name, "$")
	if name == "" {
		return "T"
	}
	return name
}

func (l *lowerer) internTypeParam(name string, idx int) (TypeID, error) {
	key := "typeparam:" + name
	if id, ok := l.typeByKey[key]; ok {
		return id, nil
	}
	id := TypeID(len(l.program.Types) + 1)
	l.typeByKey[key] = id
	l.program.Types = append(l.program.Types, TypeInfo{ID: id, Kind: TypeParam, Name: goifyTypeParamName(name), ParamIndex: idx})
	return id, nil
}

// internGenericStructDef interns the generic definition of a struct (fields
// reference TypeParam; TypeParams names the parameters) and registers it with
// its declaring module so the backend emits and qualifies it (ADR 0031).
func (l *lowerer) internGenericStructDef(typ *checker.StructDef) (TypeID, error) {
	key := genericStructDefKey(typ.ModulePath, typ.Name)
	if id, ok := l.genericStructDefs[key]; ok {
		return id, nil
	}
	id := TypeID(len(l.program.Types) + 1)
	idx := len(l.program.Types)
	l.program.Types = append(l.program.Types, TypeInfo{ID: id, Name: typ.Name})
	l.genericStructDefs[key] = id
	goParams := make([]string, len(typ.GenericParams))
	params := map[string]int{}
	for i, p := range typ.GenericParams {
		params[p] = i
		goParams[i] = goifyTypeParamName(p)
	}
	info := TypeInfo{ID: id, Kind: TypeStruct, Name: typ.Name, ModulePath: typ.ModulePath, Private: typ.Private, TypeParams: goParams}
	prev := l.defParams
	l.defParams = params
	fieldNames := sortedFieldNames(typ.Fields)
	info.Fields = make([]FieldInfo, len(fieldNames))
	for i, name := range fieldNames {
		ft := typ.Fields[name]
		mut := false
		if ref, ok := ft.(*checker.MutableRef); ok {
			ft = ref.Of()
			mut = true
		}
		ftid, err := l.internType(ft)
		if err != nil {
			l.defParams = prev
			return NoType, err
		}
		info.Fields[i] = FieldInfo{Name: name, Type: ftid, Index: i, Mutable: mut}
	}
	l.defParams = prev
	l.program.Types[idx] = info
	if modID, ok := l.moduleByPath[typ.ModulePath]; ok {
		l.program.Modules[modID].Types = appendUniqueType(l.program.Modules[modID].Types, id)
	}
	return id, nil
}

// lookupGenericStructDef finds the generic definition of a struct from the
// checker module scope (its fields still reference the type variables).
func (l *lowerer) lookupGenericStructDef(modulePath, name string) *checker.StructDef {
	mod, ok := l.moduleByName[modulePath]
	if !ok {
		return nil
	}
	if sd, ok := mod.Get(name).Type.(*checker.StructDef); ok && len(sd.GenericParams) > 0 {
		return sd
	}
	return nil
}

func (l *lowerer) internType(t checker.Type) (TypeID, error) {
	if t == nil {
		return NoType, fmt.Errorf("cannot intern nil type")
	}
	if tv, ok := t.(*checker.TypeVar); ok {
		// Inside a generic definition, a type variable naming one of the
		// definition's parameters lowers to a TypeParam reference (ADR 0031),
		// even when resolved to a concrete type by the triggering call site.
		if l.defParams != nil {
			if idx, ok := l.defParams[tv.Name()]; ok {
				return l.internTypeParam(tv.Name(), idx)
			}
		}
		if tv.Actual() != nil {
			return l.internType(tv.Actual())
		}
	}
	if ref, ok := t.(*checker.MutableRef); ok {
		return l.internType(ref.Of())
	}
	key := airTypeKey(t)
	// Within a generic definition, only types that actually reference a type
	// parameter need a distinct key; non-generic types (e.g. a plain struct used
	// in the body) must continue to share their concrete interned entry.
	if l.defParams != nil && typeContainsTypeVar(t) {
		key = "gdef:" + key
	}
	name := airTypeName(t)
	if id, ok := l.typeByKey[key]; ok {
		return id, nil
	}

	id := TypeID(len(l.program.Types) + 1)
	l.typeByKey[key] = id
	l.program.Types = append(l.program.Types, TypeInfo{ID: id, Name: name})
	idx := len(l.program.Types) - 1
	info := TypeInfo{ID: id, Name: name}

	switch typ := t.(type) {
	case *checker.List:
		elem, err := l.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		info.Kind = TypeList
		info.Elem = elem
	case *checker.Chan:
		elem, err := l.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		info.Kind = TypeChannel
		info.Elem = elem
	case *checker.Receiver:
		elem, err := l.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		info.Kind = TypeReceiver
		info.Elem = elem
	case *checker.Sender:
		elem, err := l.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		info.Kind = TypeSender
		info.Elem = elem
	case *checker.Map:
		key, err := l.internType(typ.Key())
		if err != nil {
			return NoType, err
		}
		value, err := l.internType(typ.Value())
		if err != nil {
			return NoType, err
		}
		info.Kind = TypeMap
		info.Key = key
		info.Value = value
	case *checker.Maybe:
		elemType := typ.Of()
		elemMutable := false
		if ref, ok := elemType.(*checker.MutableRef); ok {
			elemMutable = true
			elemType = ref.Of()
		}
		elem, err := l.internType(elemType)
		if err != nil {
			return NoType, err
		}
		info.Kind = TypeMaybe
		info.Elem = elem
		info.ElemMutable = elemMutable
	case *checker.MutableRef:
		return l.internType(typ.Of())
	case *checker.Result:
		value, err := l.internType(typ.Val())
		if err != nil {
			return NoType, err
		}
		errType, err := l.internType(typ.Err())
		if err != nil {
			return NoType, err
		}
		info.Kind = TypeResult
		info.Value = value
		info.Error = errType
	case *checker.StructDef:
		info.Private = typ.Private
		info.Kind = TypeStruct
		fields := sortedFieldNames(typ.Fields)
		info.Fields = make([]FieldInfo, len(fields))
		for i, name := range fields {
			fieldType, fieldMutable, err := l.internStructFieldType(typ.Fields[name], l.internType)
			if err != nil {
				return NoType, err
			}
			info.Fields[i] = FieldInfo{Name: name, Type: fieldType, Index: i, Mutable: fieldMutable}
		}
		// Tag concrete instantiations of a generic struct (ADR 0031). The
		// generic definition is interned lazily from the checker module scope,
		// since the declaring module's type loop may not have run.
		if len(typ.GenericParams) > 0 {
			defID, ok := l.genericStructDefs[genericStructDefKey(typ.ModulePath, typ.Name)]
			if !ok {
				if genDef := l.lookupGenericStructDef(typ.ModulePath, typ.Name); genDef != nil {
					interned, err := l.internGenericStructDef(genDef)
					if err != nil {
						return NoType, err
					}
					defID, ok = interned, true
				}
			}
			if ok {
				info.Generic = defID
				info.GenericArgs = make([]TypeID, 0, len(typ.TypeArgs))
				for _, arg := range typ.TypeArgs {
					argID, err := l.internType(arg)
					if err != nil {
						return NoType, err
					}
					info.GenericArgs = append(info.GenericArgs, argID)
				}
			}
		}
	case *checker.Enum:
		info.Kind = TypeEnum
		info.Private = typ.Private
		info.EnumOpen = typ.Open
		info.Variants = make([]VariantInfo, len(typ.Values))
		for i, variant := range typ.Values {
			info.Variants[i] = VariantInfo{Name: variant.Name, Discriminant: variant.Value}
		}
	case *checker.Union:
		info.Kind = TypeUnion
		info.Private = typ.Private
		info.Members = make([]UnionMember, len(typ.Types))
		for i, member := range typ.Types {
			memberID, err := l.internType(member)
			if err != nil {
				return NoType, err
			}
			info.Members[i] = UnionMember{Type: memberID, Tag: uint32(i), Name: member.String()}
		}
	case *checker.ForeignType:
		info.Kind = TypeForeignType
		info.Name = typ.String()
		info.ForeignTarget = typ.Target
		info.ForeignNamespace = typ.Namespace
		info.ForeignQualifier = typ.Qualifier
		info.ForeignSymbol = typ.Name
		info.ForeignPointer = typ.Pointer
		info.ForeignInterface = typ.Interface
		for _, arg := range typ.TypeArgs {
			argID, err := l.internType(arg)
			if err != nil {
				return NoType, err
			}
			info.GenericArgs = append(info.GenericArgs, argID)
		}
		if typ.Underlying != nil {
			underlying, err := l.internType(typ.Underlying)
			if err != nil {
				return NoType, err
			}
			info.Value = underlying
		}
		if typ.MapKey != nil {
			key, err := l.internType(typ.MapKey)
			if err != nil {
				return NoType, err
			}
			info.Key = key
		}
		if typ.Elem != nil {
			elem, err := l.internType(typ.Elem)
			if err != nil {
				return NoType, err
			}
			info.Elem = elem
		}
		if typ.MapValue != nil {
			value, err := l.internType(typ.MapValue)
			if err != nil {
				return NoType, err
			}
			info.Value = value
		}
	case *checker.FunctionDef:
		info.Kind = TypeFunction
		for _, param := range typ.Parameters {
			paramType, paramMut, err := l.internFunctionParamType(param, l.internType)
			if err != nil {
				return NoType, err
			}
			info.Params = append(info.Params, paramType)
			info.ParamMutable = append(info.ParamMutable, paramMut)
		}
		returnType, err := l.internType(typ.ReturnType)
		if err != nil {
			return NoType, err
		}
		info.Return = returnType
	case *checker.Trait:
		traitID, err := l.internTrait(typ)
		if err != nil {
			return NoType, err
		}
		info.Kind = TypeTraitObject
		info.Trait = traitID
	default:
		switch t {
		case checker.Void:
			info.Kind = TypeVoid
		case checker.Int:
			info.Kind = TypeInt
		case checker.Int8, checker.Int16, checker.Int32, checker.Int64, checker.Uint, checker.Uint8, checker.Uint16, checker.Uint32, checker.Uint64, checker.Uintptr, checker.Float32:
			info.Kind = TypeScalar
			info.Name = t.String()
		case checker.Float64:
			info.Kind = TypeFloat64
		case checker.Bool:
			info.Kind = TypeBool
		case checker.Byte:
			info.Kind = TypeByte
		case checker.Rune:
			info.Kind = TypeRune
		case checker.Str:
			info.Kind = TypeStr
		case checker.Any:
			info.Kind = TypeAny
		default:
			return NoType, fmt.Errorf("unsupported AIR type %T (%s)", t, t.String())
		}
	}

	info.ModulePath = l.typeOwnerPath(t)
	l.program.Types[idx] = info
	return id, nil
}

func (l *lowerer) internSyntheticType(name string, info TypeInfo) (TypeID, error) {
	key := syntheticTypeKey(name, info)
	if id, ok := l.typeByKey[key]; ok {
		return id, nil
	}
	id := TypeID(len(l.program.Types) + 1)
	info.ID = id
	info.Name = name
	l.typeByKey[key] = id
	l.program.Types = append(l.program.Types, info)
	return id, nil
}

func syntheticTypeKey(name string, info TypeInfo) string {
	switch info.Kind {
	case TypeList:
		return fmt.Sprintf("list:%d", info.Elem)
	case TypeMap:
		return fmt.Sprintf("map:%d:%d", info.Key, info.Value)
	case TypeMaybe:
		return fmt.Sprintf("maybe:%d:%t", info.Elem, info.ElemMutable)
	case TypeResult:
		return fmt.Sprintf("result:%d:%d", info.Value, info.Error)
	case TypeFunction:
		parts := make([]string, len(info.Params))
		for i, param := range info.Params {
			mut := ""
			if i < len(info.ParamMutable) && info.ParamMutable[i] {
				mut = "mut "
			}
			parts[i] = fmt.Sprintf("%s%d", mut, param)
		}
		return fmt.Sprintf("fn:(%s)->%d", strings.Join(parts, ","), info.Return)
	default:
		return "synthetic:" + name
	}
}

func (l *lowerer) typeOwnerPath(t checker.Type) string {
	if ref, ok := t.(*checker.MutableRef); ok {
		return l.typeOwnerPath(ref.Of())
	}
	var name string
	switch typ := t.(type) {
	case *checker.StructDef:
		if typ.ModulePath != "" {
			return typ.ModulePath
		}
		name = typ.Name
	case *checker.Enum:
		if typ.ModulePath != "" {
			return typ.ModulePath
		}
		name = typ.Name
	case *checker.Union:
		if typ.ModulePath != "" {
			return typ.ModulePath
		}
		name = typ.Name
	default:
		return ""
	}
	for path, module := range l.moduleByName {
		sym := module.Get(name)
		if sym.Type == t {
			return path
		}
	}
	return ""
}

func (l *lowerer) typeName(id TypeID) string {
	info, ok := l.typeInfo(id)
	if !ok {
		return fmt.Sprintf("<invalid:%d>", id)
	}
	return info.Name
}

func (l *lowerer) internTrait(trait *checker.Trait) (TraitID, error) {
	if trait == nil {
		return 0, fmt.Errorf("cannot intern nil trait")
	}
	key := checkerTraitKey(trait)
	if id, ok := l.traits[key]; ok {
		return id, nil
	}
	id := TraitID(len(l.program.Traits))
	l.traits[key] = id
	l.program.Traits = append(l.program.Traits, Trait{ID: id, Name: trait.Name, ModulePath: trait.ModulePath})

	methods := trait.GetMethods()
	loweredMethods := make([]TraitMethod, len(methods))
	for i, method := range methods {
		sig, err := l.signatureForFunction(method.Parameters, method.ReturnType)
		if err != nil {
			return 0, err
		}
		loweredMethods[i] = TraitMethod{Name: method.Name, Signature: sig}
	}
	l.program.Traits[id] = Trait{
		ID:         id,
		Name:       trait.Name,
		ModulePath: trait.ModulePath,
		Private:    trait.IsPrivate(),
		Methods:    loweredMethods,
	}
	return id, nil
}

func (l *lowerer) signatureForFunction(params []checker.Parameter, returnType checker.Type) (Signature, error) {
	return signatureForFunctionWithInterner(params, returnType, l.internType)
}

func signatureForFunctionWithInterner(params []checker.Parameter, returnType checker.Type, intern func(checker.Type) (TypeID, error)) (Signature, error) {
	loweredParams := make([]Param, len(params))
	for i, param := range params {
		typeID, err := intern(param.Type)
		if err != nil {
			return Signature{}, err
		}
		loweredParams[i] = Param{Name: param.Name, Type: typeID, Mutable: param.Mutable}
	}
	returnID, err := intern(returnType)
	if err != nil {
		return Signature{}, err
	}
	return Signature{Params: loweredParams, Return: returnID}, nil
}

func functionHasUnresolvedTypeVar(def *checker.FunctionDef) bool {
	if def == nil {
		return false
	}
	for _, param := range def.GenericParams {
		if _, ok := def.GenericBindings[param]; !ok {
			return true
		}
	}
	return typeHasUnresolvedTypeVarSeen(def, map[checker.Type]struct{}{})
}

func functionSignatureHasUnresolvedTypeVarSeen(def *checker.FunctionDef, seen map[checker.Type]struct{}) bool {
	if def == nil {
		return false
	}
	for _, param := range def.Parameters {
		if typeHasUnresolvedTypeVarSeen(param.Type, seen) {
			return true
		}
	}
	return typeHasUnresolvedTypeVarSeen(def.ReturnType, seen)
}

func functionHasTypeVar(def *checker.FunctionDef) bool {
	if def == nil {
		return false
	}
	for _, param := range def.Parameters {
		if typeContainsTypeVar(param.Type) {
			return true
		}
	}
	return typeContainsTypeVar(def.ReturnType)
}

func typeContainsTypeVar(t checker.Type) bool {
	return typeContainsTypeVarSeen(t, map[checker.Type]struct{}{})
}

func typeContainsTypeVarSeen(t checker.Type, seen map[checker.Type]struct{}) bool {
	if t == nil {
		return false
	}
	if _, ok := seen[t]; ok {
		return false
	}
	seen[t] = struct{}{}
	switch typ := t.(type) {
	case *checker.TypeVar:
		return true
	case *checker.List:
		return typeContainsTypeVarSeen(typ.Of(), seen)
	case *checker.Chan:
		return typeContainsTypeVarSeen(typ.Of(), seen)
	case *checker.Receiver:
		return typeContainsTypeVarSeen(typ.Of(), seen)
	case *checker.Sender:
		return typeContainsTypeVarSeen(typ.Of(), seen)
	case *checker.Map:
		return typeContainsTypeVarSeen(typ.Key(), seen) || typeContainsTypeVarSeen(typ.Value(), seen)
	case *checker.Maybe:
		return typeContainsTypeVarSeen(typ.Of(), seen)
	case *checker.Result:
		return typeContainsTypeVarSeen(typ.Val(), seen) || typeContainsTypeVarSeen(typ.Err(), seen)
	case *checker.MutableRef:
		return typeContainsTypeVarSeen(typ.Of(), seen)
	case *checker.Union:
		for _, member := range typ.Types {
			if typeContainsTypeVarSeen(member, seen) {
				return true
			}
		}
		return false
	case *checker.StructDef:
		for _, typeArg := range typ.TypeArgs {
			if typeContainsTypeVarSeen(typeArg, seen) {
				return true
			}
		}
		for _, fieldType := range typ.Fields {
			if typeContainsTypeVarSeen(fieldType, seen) {
				return true
			}
		}
		return false
	case *checker.FunctionDef:
		for _, param := range typ.Parameters {
			if typeContainsTypeVarSeen(param.Type, seen) {
				return true
			}
		}
		return typeContainsTypeVarSeen(typ.ReturnType, seen)
	default:
		return false
	}
}

func typeHasUnresolvedTypeVar(t checker.Type) bool {
	return typeHasUnresolvedTypeVarSeen(t, map[checker.Type]struct{}{})
}

func typeHasUnresolvedTypeVarSeen(t checker.Type, seen map[checker.Type]struct{}) bool {
	if t == nil {
		return false
	}
	if _, ok := seen[t]; ok {
		return false
	}
	seen[t] = struct{}{}
	switch typ := t.(type) {
	case *checker.TypeVar:
		if typ.Actual() == nil {
			return true
		}
		return typeHasUnresolvedTypeVarSeen(typ.Actual(), seen)
	case *checker.List:
		return typeHasUnresolvedTypeVarSeen(typ.Of(), seen)
	case *checker.Chan:
		return typeHasUnresolvedTypeVarSeen(typ.Of(), seen)
	case *checker.Receiver:
		return typeHasUnresolvedTypeVarSeen(typ.Of(), seen)
	case *checker.Sender:
		return typeHasUnresolvedTypeVarSeen(typ.Of(), seen)
	case *checker.Map:
		return typeHasUnresolvedTypeVarSeen(typ.Key(), seen) || typeHasUnresolvedTypeVarSeen(typ.Value(), seen)
	case *checker.Maybe:
		return typeHasUnresolvedTypeVarSeen(typ.Of(), seen)
	case *checker.Result:
		return typeHasUnresolvedTypeVarSeen(typ.Val(), seen) || typeHasUnresolvedTypeVarSeen(typ.Err(), seen)
	case *checker.MutableRef:
		return typeHasUnresolvedTypeVarSeen(typ.Of(), seen)
	case *checker.Union:
		for _, member := range typ.Types {
			if typeHasUnresolvedTypeVarSeen(member, seen) {
				return true
			}
		}
		return false
	case *checker.StructDef:
		for _, typeArg := range typ.TypeArgs {
			if typeHasUnresolvedTypeVarSeen(typeArg, seen) {
				return true
			}
		}
		for _, fieldType := range typ.Fields {
			if typeHasUnresolvedTypeVarSeen(fieldType, seen) {
				return true
			}
		}
		return false
	case *checker.FunctionDef:
		return functionSignatureHasUnresolvedTypeVarSeen(typ, seen)
	default:
		return false
	}
}

func canWrapAsAny(kind TypeKind) bool {
	switch kind {
	case TypeVoid, TypeInt, TypeScalar, TypeForeignType, TypeFloat64, TypeBool, TypeByte, TypeRune, TypeStr, TypeList, TypeMap, TypeStruct, TypeEnum, TypeMaybe, TypeResult, TypeUnion, TypeChannel, TypeReceiver, TypeSender, TypeAny:
		return true
	default:
		return false
	}
}

func signaturesEqual(left, right Signature) bool {
	if left.Return != right.Return || len(left.Params) != len(right.Params) {
		return false
	}
	for i := range left.Params {
		if left.Params[i].Type != right.Params[i].Type || left.Params[i].Mutable != right.Params[i].Mutable {
			return false
		}
	}
	return true
}

func airTypeName(t checker.Type) string {
	return t.String()
}

func airTypeKey(t checker.Type) string {
	return airTypeKeySeen(t, map[checker.Type]struct{}{})
}

func airTypeKeySeen(t checker.Type, seen map[checker.Type]struct{}) string {
	if t == nil {
		return "<nil>"
	}
	if tv, ok := t.(*checker.TypeVar); ok && tv.Actual() != nil {
		return airTypeKeySeen(tv.Actual(), seen)
	}
	if _, ok := seen[t]; ok {
		return t.String()
	}
	seen[t] = struct{}{}
	switch typ := t.(type) {
	case *checker.List:
		return "list<" + airTypeKeySeen(typ.Of(), seen) + ">"
	case *checker.Chan:
		return "channel<" + airTypeKeySeen(typ.Of(), seen) + ">"
	case *checker.Receiver:
		return "receiver<" + airTypeKeySeen(typ.Of(), seen) + ">"
	case *checker.Sender:
		return "sender<" + airTypeKeySeen(typ.Of(), seen) + ">"
	case *checker.Map:
		return "map<" + airTypeKeySeen(typ.Key(), seen) + "," + airTypeKeySeen(typ.Value(), seen) + ">"
	case *checker.Maybe:
		return "maybe<" + airTypeKeySeen(typ.Of(), seen) + ">"
	case *checker.Result:
		return "result<" + airTypeKeySeen(typ.Val(), seen) + "," + airTypeKeySeen(typ.Err(), seen) + ">"
	case *checker.MutableRef:
		return "mut<" + airTypeKeySeen(typ.Of(), seen) + ">"
	case *checker.StructDef:
		if hasSelfReference(typ) {
			return "recursive struct " + typ.ModulePath + "::" + typ.Name
		}
		return airStructKeySeen(typ, seen)
	case *checker.Enum:
		return airEnumKey(typ)
	case *checker.Union:
		return airUnionKeySeen(typ, seen)
	case *checker.FunctionDef:
		return airFunctionTypeKeySeen(typ.Parameters, typ.ReturnType, seen)
	case *checker.Trait:
		return "trait " + checkerTraitKey(typ)
	default:
		return t.String()
	}
}

func airStructKey(typ *checker.StructDef) string {
	return airStructKeySeen(typ, map[checker.Type]struct{}{})
}

func airStructKeySeen(typ *checker.StructDef, seen map[checker.Type]struct{}) string {
	fields := sortedFieldNames(typ.Fields)
	key := "struct " + typ.ModulePath + "::" + typ.Name
	if len(typ.TypeArgs) > 0 {
		key += "<"
		for i, typeArg := range typ.TypeArgs {
			if i > 0 {
				key += ","
			}
			key += airTypeKeySeen(typeArg, seen)
		}
		key += ">"
	}
	key += "{"
	for i, name := range fields {
		if i > 0 {
			key += ","
		}
		key += name + ":" + airTypeKeySeen(typ.Fields[name], seen)
	}
	key += "}"
	return key
}

func airEnumKey(typ *checker.Enum) string {
	key := "enum " + typ.ModulePath + "::" + typ.Name + "{"
	for i, variant := range typ.Values {
		if i > 0 {
			key += ","
		}
		key += variant.Name
	}
	key += "}"
	return key
}

func airUnionKey(typ *checker.Union) string {
	return airUnionKeySeen(typ, map[checker.Type]struct{}{})
}

func airUnionKeySeen(typ *checker.Union, seen map[checker.Type]struct{}) string {
	parts := make([]string, len(typ.Types))
	for i, member := range typ.Types {
		parts[i] = airTypeKeySeen(member, seen)
	}
	sort.Strings(parts)
	key := "union " + typ.ModulePath + "::" + typ.Name + "{"
	for i, part := range parts {
		if i > 0 {
			key += "|"
		}
		key += part
	}
	key += "}"
	return key
}

func airFunctionTypeKey(params []checker.Parameter, returnType checker.Type) string {
	return airFunctionTypeKeySeen(params, returnType, map[checker.Type]struct{}{})
}

func airFunctionTypeKeySeen(params []checker.Parameter, returnType checker.Type, seen map[checker.Type]struct{}) string {
	key := "fn("
	for i, param := range params {
		if i > 0 {
			key += ","
		}
		mutable := param.Mutable
		// Mirror internFunctionParamType: pointer-shaped foreign params are
		// canonically mutable so checker-equal function types share one key.
		if foreign, ok := param.Type.(*checker.ForeignType); ok && foreign.Pointer {
			mutable = true
		}
		if mutable {
			key += "mut "
		}
		key += airTypeKeySeen(param.Type, seen)
	}
	key += ")->" + airTypeKeySeen(returnType, seen)
	return key
}

func (fl *functionLowerer) lowerBlock(stmts []checker.Statement) (Block, error) {
	return fl.lowerBlockWithDefault(stmts, fl.fn.Signature.Return)
}

func (fl *functionLowerer) lowerBlockWithDefault(stmts []checker.Statement, defaultType TypeID) (Block, error) {
	// A block is a lexical scope: locals it declares must not leak into the
	// enclosing scope, so references after the block resolve to the right local.
	defer fl.scopeLocals()()
	var block Block
	last := len(stmts) - 1
	for last >= 0 && stmts[last].Expr == nil && stmts[last].Stmt == nil && !stmts[last].Break {
		last--
	}
	for i, stmt := range stmts {
		if stmt.Expr == nil && stmt.Stmt == nil && !stmt.Break {
			continue
		}
		if i == last && stmt.Expr != nil {
			expr, _, err := fl.lowerContextualExpr(stmt.Expr, defaultType)
			if err != nil {
				return block, err
			}
			block.Result = expr
			continue
		}
		lowered, err := fl.lowerStmts(stmt)
		if err != nil {
			return block, err
		}
		block.Stmts = append(block.Stmts, lowered...)
	}
	return block, nil
}

func (fl *functionLowerer) lowerContextualExpr(expr checker.Expression, expected TypeID) (*Expr, TypeID, error) {
	lowered, err := fl.lowerExprWithExpectedRaw(expr, expected)
	if err != nil {
		return nil, expected, err
	}
	if normalized := fl.normalizeContextualType(lowered, expected); normalized != NoType && normalized != expected {
		lowered, err = fl.lowerExprWithExpectedRaw(expr, normalized)
		if err != nil {
			return nil, normalized, err
		}
		return lowered, normalized, nil
	}
	return lowered, expected, nil
}

func (fl *functionLowerer) lowerExprWithExpected(expr checker.Expression, expected TypeID) (*Expr, error) {
	lowered, _, err := fl.lowerContextualExpr(expr, expected)
	return lowered, err
}

func (fl *functionLowerer) lowerExprWithExpectedRaw(expr checker.Expression, expected TypeID) (*Expr, error) {
	if wrapped, ok, err := fl.lowerUnionWrapIfNeeded(expr, expected); ok || err != nil {
		return wrapped, err
	}
	if wrapped, ok, err := fl.lowerTraitUpcastIfNeeded(expr, expected); ok || err != nil {
		return wrapped, err
	}
	if wrapped, ok, err := fl.lowerAnyWrapIfNeeded(expr, expected); ok || err != nil {
		return wrapped, err
	}
	if wrapped, ok, err := fl.lowerForeignScalarNarrowIfNeeded(expr, expected); ok || err != nil {
		return wrapped, err
	}
	if wrapped, ok, err := fl.lowerForeignScalarWidenIfNeeded(expr, expected); ok || err != nil {
		return wrapped, err
	}
	if list, ok := expr.(*checker.ListLiteral); ok {
		if expectedInfo, hasInfo := fl.l.typeInfo(expected); hasInfo {
			if expectedInfo.Kind == TypeList {
				return fl.lowerListLiteral(expected, list, expectedInfo.Elem)
			}
			// A list literal against a named Go slice type keeps the named
			// type so the composite literal is spelled with it.
			if expectedInfo.Kind == TypeForeignType && !expectedInfo.ForeignPointer && expectedInfo.Elem != NoType {
				return fl.lowerListLiteral(expected, list, expectedInfo.Elem)
			}
		}
	}
	if m, ok := expr.(*checker.MapLiteral); ok {
		if expectedInfo, hasInfo := fl.l.typeInfo(expected); hasInfo && expectedInfo.Kind == TypeForeignType && !expectedInfo.ForeignPointer && expectedInfo.Key != NoType && expectedInfo.Value != NoType {
			// A map literal against a named Go map type keeps the named type.
			return fl.lowerMapLiteral(expected, m, expectedInfo.Key, expectedInfo.Value)
		}
		if expectedInfo, hasInfo := fl.l.typeInfo(expected); hasInfo && expectedInfo.Kind == TypeMap {
			return fl.lowerMapLiteral(expected, m, expectedInfo.Key, expectedInfo.Value)
		}
	}
	if inst, ok := expr.(*checker.StructInstance); ok {
		if expectedInfo, hasInfo := fl.l.typeInfo(expected); hasInfo && expectedInfo.Kind == TypeStruct {
			return fl.lowerStructInstance(expected, inst)
		}
	}
	if closure, ok := expr.(*checker.FunctionDef); ok {
		if expectedInfo, hasInfo := fl.l.typeInfo(expected); hasInfo && expectedInfo.Kind == TypeFunction {
			return fl.lowerClosure(expected, closure)
		}
	}
	if symbol, ok := expr.(*checker.ModuleSymbol); ok {
		if expectedInfo, hasInfo := fl.l.typeInfo(expected); hasInfo && expectedInfo.Kind == TypeFunction {
			return fl.lowerModuleSymbol(expected, symbol)
		}
	}
	if call, ok := expr.(*checker.ModuleFunctionCall); ok {
		if kind, ok := resultConstructorKind(call); ok {
			return fl.lowerResultConstructor(kind, expected, call)
		}
		if kind, ok := maybeConstructorKind(call); ok {
			return fl.lowerMaybeConstructor(kind, expected, call)
		}
	}
	if match, ok := expr.(*checker.BoolMatch); ok {
		return fl.lowerBoolMatch(expected, match)
	}
	if match, ok := expr.(*checker.IntMatch); ok {
		return fl.lowerIntMatch(expected, match)
	}
	if match, ok := expr.(*checker.StrMatch); ok {
		return fl.lowerStrMatch(expected, match)
	}
	if ifExpr, ok := expr.(*checker.If); ok {
		return fl.lowerIf(expected, ifExpr)
	}
	if match, ok := expr.(*checker.ConditionalMatch); ok {
		return fl.lowerConditionalMatch(expected, match)
	}
	if op, ok := expr.(*checker.TryOp); ok && validTypeID(&fl.l.program, expected) {
		return fl.lowerTryOp(expected, op)
	}
	return fl.lowerExpr(expr)
}

func (fl *functionLowerer) normalizeContextualType(expr *Expr, expected TypeID) TypeID {
	if expr == nil || !validTypeID(&fl.l.program, expected) || !fl.isWeakContextType(expected) {
		return NoType
	}
	expectedInfo, _ := fl.l.typeInfo(expected)
	switch expectedInfo.Kind {
	case TypeMaybe:
		return fl.inferMaybeType(expr)
	case TypeResult:
		return fl.inferResultType(expr, expected)
	case TypeVoid:
		if inferred := fl.inferValueType(expr); inferred != NoType {
			return inferred
		}
		if inferred := fl.inferMaybeType(expr); inferred != NoType {
			return inferred
		}
		return fl.inferResultType(expr, expected)
	default:
		return NoType
	}
}

func (fl *functionLowerer) inferValueType(expr *Expr) TypeID {
	if expr == nil {
		return NoType
	}
	switch expr.Kind {
	case ExprTryMaybe:
		if expr.Target != nil {
			if targetInfo, ok := fl.l.typeInfo(expr.Target.Type); ok && targetInfo.Kind == TypeMaybe {
				return targetInfo.Elem
			}
		}
	case ExprTryResult:
		if expr.Target != nil {
			if targetInfo, ok := fl.l.typeInfo(expr.Target.Type); ok && targetInfo.Kind == TypeResult {
				return targetInfo.Value
			}
		}
	case ExprBlock:
		return fl.inferValueType(expr.Body.Result)
	case ExprIf:
		return fl.mergeValueTypes(fl.inferValueType(expr.Then.Result), fl.inferValueType(expr.Else.Result))
	case ExprMatchInt:
		return fl.inferValueTypeFromCases(expr.IntCases, expr.RangeCases, expr.CatchAll)
	case ExprMatchStr:
		return fl.inferValueTypeFromStrCases(expr.StrCases, expr.CatchAll)
	case ExprMatchMaybe:
		return fl.mergeValueTypes(fl.inferValueType(expr.Some.Result), fl.inferValueType(expr.None.Result))
	case ExprMatchResult:
		return fl.mergeValueTypes(fl.inferValueType(expr.Ok.Result), fl.inferValueType(expr.Err.Result))
	case ExprMatchEnum:
		value := NoType
		for _, c := range expr.EnumCases {
			value = fl.mergeValueTypes(value, fl.inferValueType(c.Body.Result))
		}
		return fl.mergeValueTypes(value, fl.inferValueType(expr.CatchAll.Result))
	case ExprMatchUnion:
		value := NoType
		for _, c := range expr.UnionCases {
			value = fl.mergeValueTypes(value, fl.inferValueType(c.Body.Result))
		}
		return fl.mergeValueTypes(value, fl.inferValueType(expr.CatchAll.Result))
	}
	return NoType
}

func (fl *functionLowerer) inferMaybeType(expr *Expr) TypeID {
	if expr == nil {
		return NoType
	}
	if info, ok := fl.l.typeInfo(expr.Type); ok && info.Kind == TypeMaybe && !fl.isWeakContextType(expr.Type) {
		return expr.Type
	}
	switch expr.Kind {
	case ExprMakeMaybeSome:
		if expr.Target != nil {
			return fl.internMaybeType(expr.Target.Type)
		}
	case ExprBlock:
		return fl.inferMaybeType(expr.Body.Result)
	case ExprIf:
		return fl.mergeValueTypes(fl.inferMaybeType(expr.Then.Result), fl.inferMaybeType(expr.Else.Result))
	case ExprMatchInt:
		return fl.inferMaybeTypeFromCases(expr.IntCases, expr.RangeCases, expr.CatchAll)
	case ExprMatchStr:
		return fl.inferMaybeTypeFromStrCases(expr.StrCases, expr.CatchAll)
	case ExprMatchMaybe:
		return fl.mergeValueTypes(fl.inferMaybeType(expr.Some.Result), fl.inferMaybeType(expr.None.Result))
	case ExprMatchResult:
		return fl.mergeValueTypes(fl.inferMaybeType(expr.Ok.Result), fl.inferMaybeType(expr.Err.Result))
	case ExprMatchEnum:
		value := NoType
		for _, c := range expr.EnumCases {
			value = fl.mergeValueTypes(value, fl.inferMaybeType(c.Body.Result))
		}
		return fl.mergeValueTypes(value, fl.inferMaybeType(expr.CatchAll.Result))
	case ExprMatchUnion:
		value := NoType
		for _, c := range expr.UnionCases {
			value = fl.mergeValueTypes(value, fl.inferMaybeType(c.Body.Result))
		}
		return fl.mergeValueTypes(value, fl.inferMaybeType(expr.CatchAll.Result))
	}
	return NoType
}

func (fl *functionLowerer) inferResultType(expr *Expr, fallback TypeID) TypeID {
	valueType, errType := fl.inferResultParts(expr)
	if info, ok := fl.l.typeInfo(fallback); ok && info.Kind == TypeResult {
		if valueType == NoType && validTypeID(&fl.l.program, info.Value) && !fl.isVoidType(info.Value) {
			valueType = info.Value
		}
		if errType == NoType && validTypeID(&fl.l.program, info.Error) && !fl.isVoidType(info.Error) {
			errType = info.Error
		}
	}
	if valueType == NoType || errType == NoType {
		return NoType
	}
	return fl.internResultType(valueType, errType)
}

func (fl *functionLowerer) inferResultParts(expr *Expr) (TypeID, TypeID) {
	if expr == nil {
		return NoType, NoType
	}
	if info, ok := fl.l.typeInfo(expr.Type); ok && info.Kind == TypeResult && !fl.isWeakContextType(expr.Type) {
		return info.Value, info.Error
	}
	switch expr.Kind {
	case ExprMakeResultOk:
		if expr.Target != nil {
			return expr.Target.Type, NoType
		}
	case ExprMakeResultErr:
		if expr.Target != nil {
			return NoType, expr.Target.Type
		}
	case ExprBlock:
		return fl.inferResultParts(expr.Body.Result)
	case ExprIf:
		leftValue, leftErr := fl.inferResultParts(expr.Then.Result)
		rightValue, rightErr := fl.inferResultParts(expr.Else.Result)
		return fl.mergeResultParts(leftValue, leftErr, rightValue, rightErr)
	case ExprMatchInt:
		return fl.inferResultPartsFromCases(expr.IntCases, expr.RangeCases, expr.CatchAll)
	case ExprMatchStr:
		return fl.inferResultPartsFromStrCases(expr.StrCases, expr.CatchAll)
	case ExprMatchMaybe:
		leftValue, leftErr := fl.inferResultParts(expr.Some.Result)
		rightValue, rightErr := fl.inferResultParts(expr.None.Result)
		return fl.mergeResultParts(leftValue, leftErr, rightValue, rightErr)
	case ExprMatchResult:
		leftValue, leftErr := fl.inferResultParts(expr.Ok.Result)
		rightValue, rightErr := fl.inferResultParts(expr.Err.Result)
		return fl.mergeResultParts(leftValue, leftErr, rightValue, rightErr)
	case ExprMatchEnum:
		valueType, errType := NoType, NoType
		for _, c := range expr.EnumCases {
			rightValue, rightErr := fl.inferResultParts(c.Body.Result)
			valueType, errType = fl.mergeResultParts(valueType, errType, rightValue, rightErr)
		}
		rightValue, rightErr := fl.inferResultParts(expr.CatchAll.Result)
		return fl.mergeResultParts(valueType, errType, rightValue, rightErr)
	case ExprMatchUnion:
		valueType, errType := NoType, NoType
		for _, c := range expr.UnionCases {
			rightValue, rightErr := fl.inferResultParts(c.Body.Result)
			valueType, errType = fl.mergeResultParts(valueType, errType, rightValue, rightErr)
		}
		rightValue, rightErr := fl.inferResultParts(expr.CatchAll.Result)
		return fl.mergeResultParts(valueType, errType, rightValue, rightErr)
	}
	return NoType, NoType
}

func (fl *functionLowerer) inferValueTypeFromCases(intCases []IntMatchCase, rangeCases []IntRangeMatchCase, catchAll Block) TypeID {
	value := NoType
	for _, c := range intCases {
		value = fl.mergeValueTypes(value, fl.inferValueType(c.Body.Result))
	}
	for _, c := range rangeCases {
		value = fl.mergeValueTypes(value, fl.inferValueType(c.Body.Result))
	}
	return fl.mergeValueTypes(value, fl.inferValueType(catchAll.Result))
}

func (fl *functionLowerer) inferValueTypeFromStrCases(strCases []StrMatchCase, catchAll Block) TypeID {
	value := NoType
	for _, c := range strCases {
		value = fl.mergeValueTypes(value, fl.inferValueType(c.Body.Result))
	}
	return fl.mergeValueTypes(value, fl.inferValueType(catchAll.Result))
}

func (fl *functionLowerer) inferMaybeTypeFromCases(intCases []IntMatchCase, rangeCases []IntRangeMatchCase, catchAll Block) TypeID {
	value := NoType
	for _, c := range intCases {
		value = fl.mergeValueTypes(value, fl.inferMaybeType(c.Body.Result))
	}
	for _, c := range rangeCases {
		value = fl.mergeValueTypes(value, fl.inferMaybeType(c.Body.Result))
	}
	return fl.mergeValueTypes(value, fl.inferMaybeType(catchAll.Result))
}

func (fl *functionLowerer) inferMaybeTypeFromStrCases(strCases []StrMatchCase, catchAll Block) TypeID {
	value := NoType
	for _, c := range strCases {
		value = fl.mergeValueTypes(value, fl.inferMaybeType(c.Body.Result))
	}
	return fl.mergeValueTypes(value, fl.inferMaybeType(catchAll.Result))
}

func (fl *functionLowerer) inferResultPartsFromStrCases(strCases []StrMatchCase, catchAll Block) (TypeID, TypeID) {
	valueType, errType := NoType, NoType
	for _, c := range strCases {
		rightValue, rightErr := fl.inferResultParts(c.Body.Result)
		valueType, errType = fl.mergeResultParts(valueType, errType, rightValue, rightErr)
	}
	rightValue, rightErr := fl.inferResultParts(catchAll.Result)
	return fl.mergeResultParts(valueType, errType, rightValue, rightErr)
}

func (fl *functionLowerer) inferResultPartsFromCases(intCases []IntMatchCase, rangeCases []IntRangeMatchCase, catchAll Block) (TypeID, TypeID) {
	valueType, errType := NoType, NoType
	for _, c := range intCases {
		rightValue, rightErr := fl.inferResultParts(c.Body.Result)
		valueType, errType = fl.mergeResultParts(valueType, errType, rightValue, rightErr)
	}
	for _, c := range rangeCases {
		rightValue, rightErr := fl.inferResultParts(c.Body.Result)
		valueType, errType = fl.mergeResultParts(valueType, errType, rightValue, rightErr)
	}
	rightValue, rightErr := fl.inferResultParts(catchAll.Result)
	return fl.mergeResultParts(valueType, errType, rightValue, rightErr)
}

func (fl *functionLowerer) mergeValueTypes(left TypeID, right TypeID) TypeID {
	if left == NoType {
		return right
	}
	if right == NoType || left == right {
		return left
	}
	return NoType
}

func (fl *functionLowerer) mergeResultParts(leftValue TypeID, leftErr TypeID, rightValue TypeID, rightErr TypeID) (TypeID, TypeID) {
	valueType := fl.mergeValueTypes(leftValue, rightValue)
	if valueType == NoType && leftValue != NoType && rightValue != NoType {
		return NoType, NoType
	}
	errType := fl.mergeValueTypes(leftErr, rightErr)
	if errType == NoType && leftErr != NoType && rightErr != NoType {
		return NoType, NoType
	}
	return valueType, errType
}

func (fl *functionLowerer) internMaybeType(elem TypeID) TypeID {
	if !validTypeID(&fl.l.program, elem) {
		return NoType
	}
	id, err := fl.l.internSyntheticType(fl.l.typeName(elem)+"?", TypeInfo{Kind: TypeMaybe, Elem: elem})
	if err != nil {
		return NoType
	}
	return id
}

func (fl *functionLowerer) internResultType(value TypeID, errType TypeID) TypeID {
	if !validTypeID(&fl.l.program, value) || !validTypeID(&fl.l.program, errType) {
		return NoType
	}
	id, err := fl.l.internSyntheticType(fl.l.typeName(value)+"!"+fl.l.typeName(errType), TypeInfo{Kind: TypeResult, Value: value, Error: errType})
	if err != nil {
		return NoType
	}
	return id
}

func (fl *functionLowerer) isVoidType(typeID TypeID) bool {
	if !validTypeID(&fl.l.program, typeID) {
		return false
	}
	info, _ := fl.l.typeInfo(typeID)
	return info.Kind == TypeVoid
}

func (fl *functionLowerer) isWeakContextType(typeID TypeID) bool {
	if !validTypeID(&fl.l.program, typeID) {
		return false
	}
	info, _ := fl.l.typeInfo(typeID)
	switch info.Kind {
	case TypeVoid:
		return true
	case TypeMaybe:
		return !validTypeID(&fl.l.program, info.Elem) || fl.isVoidType(info.Elem)
	case TypeResult:
		return !validTypeID(&fl.l.program, info.Value) || !validTypeID(&fl.l.program, info.Error) || fl.isVoidType(info.Value) || fl.isVoidType(info.Error)
	default:
		return false
	}
}

// lowerForeignScalarNarrowIfNeeded converts a foreign named scalar value to
// the expected primitive scalar (ADR 0028 boundary coercions), lowering to an
// explicit Go conversion such as string(v).
func (fl *functionLowerer) lowerForeignScalarNarrowIfNeeded(expr checker.Expression, expected TypeID) (*Expr, bool, error) {
	expectedInfo, ok := fl.l.typeInfo(expected)
	if !ok {
		return nil, false, nil
	}
	switch expectedInfo.Kind {
	case TypeInt, TypeScalar, TypeFloat64, TypeBool, TypeByte, TypeRune, TypeStr:
	default:
		return nil, false, nil
	}
	foreign, ok := expr.Type().(*checker.ForeignType)
	if !ok || foreign.Pointer || foreign.Underlying == nil {
		return nil, false, nil
	}
	underlyingID, err := fl.internType(foreign.Underlying)
	if err != nil || underlyingID != expected {
		return nil, false, nil
	}
	value, err := fl.lowerExpr(expr)
	if err != nil {
		return nil, true, err
	}
	return &Expr{Kind: ExprScalarConvert, Type: expected, Target: value}, true, nil
}

// lowerForeignScalarWidenIfNeeded converts a primitive Str or Bool value into
// an expected foreign named scalar type, lowering to an explicit Go conversion
// such as ui.IntentType(s).
func (fl *functionLowerer) lowerForeignScalarWidenIfNeeded(expr checker.Expression, expected TypeID) (*Expr, bool, error) {
	foreign, ok := expr.Type().(*checker.ForeignType)
	if ok && !foreign.Pointer {
		// Already the foreign type (or another foreign type); nothing to widen.
		return nil, false, nil
	}
	expectedForeign, ok := fl.l.typeInfo(expected)
	if !ok || expectedForeign.Kind != TypeForeignType || expectedForeign.ForeignPointer || expectedForeign.ForeignInterface {
		return nil, false, nil
	}
	if expectedForeign.Value == NoType || expectedForeign.Key != NoType {
		return nil, false, nil
	}
	underlyingInfo, ok := fl.l.typeInfo(expectedForeign.Value)
	if !ok || (underlyingInfo.Kind != TypeStr && underlyingInfo.Kind != TypeBool) {
		return nil, false, nil
	}
	actual, err := fl.internType(expr.Type())
	if err != nil || actual != expectedForeign.Value {
		return nil, false, nil
	}
	value, err := fl.lowerExpr(expr)
	if err != nil {
		return nil, true, err
	}
	return &Expr{Kind: ExprScalarConvert, Type: expected, Target: value}, true, nil
}

func (fl *functionLowerer) lowerAnyWrapIfNeeded(expr checker.Expression, expected TypeID) (*Expr, bool, error) {
	expectedInfo, ok := fl.l.typeInfo(expected)
	if !ok || expectedInfo.Kind != TypeAny {
		return nil, false, nil
	}
	actual, err := fl.internType(expr.Type())
	if err != nil {
		if typeHasUnresolvedTypeVar(expr.Type()) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if actual == expected {
		return nil, false, nil
	}
	actualInfo, ok := fl.l.typeInfo(actual)
	if !ok || !canWrapAsAny(actualInfo.Kind) {
		return nil, false, nil
	}
	value, err := fl.lowerExpr(expr)
	if err != nil {
		return nil, true, err
	}
	return &Expr{Kind: ExprToAny, Type: expected, Target: value}, true, nil
}

func (fl *functionLowerer) lowerUnionWrapIfNeeded(expr checker.Expression, expected TypeID) (*Expr, bool, error) {
	expectedInfo, ok := fl.l.typeInfo(expected)
	if !ok || expectedInfo.Kind != TypeUnion {
		return nil, false, nil
	}
	actual, err := fl.internType(expr.Type())
	if err != nil {
		return nil, false, err
	}
	if actual == expected {
		return nil, false, nil
	}
	for _, member := range expectedInfo.Members {
		if member.Type == actual {
			value, err := fl.lowerExpr(expr)
			if err != nil {
				return nil, true, err
			}
			return &Expr{Kind: ExprUnionWrap, Type: expected, Target: value, Tag: member.Tag}, true, nil
		}
	}
	return nil, false, nil
}

func (fl *functionLowerer) lowerTraitUpcastIfNeeded(expr checker.Expression, expected TypeID) (*Expr, bool, error) {
	expectedInfo, ok := fl.l.typeInfo(expected)
	if !ok || expectedInfo.Kind != TypeTraitObject {
		return nil, false, nil
	}
	actual, err := fl.internType(expr.Type())
	if err != nil {
		return nil, false, err
	}
	if actual == expected {
		return nil, false, nil
	}
	if err := fl.l.ensureModuleImportTraitImplsDeclared(fl.fn.Module); err != nil {
		return nil, false, err
	}
	if actualInfo, ok := fl.l.typeInfo(actual); ok && actualInfo.ModulePath != "" {
		if err := fl.l.ensureModuleTraitImplsDeclared(actualInfo.ModulePath); err != nil {
			return nil, false, err
		}
	}
	impl, ok := fl.l.lookupImpl(expectedInfo.Trait, actual)
	if !ok {
		var err error
		impl, ok, err = fl.l.declareBuiltinTraitImpl(fl.fn.Module, expectedInfo.Trait, actual)
		if err != nil {
			return nil, false, err
		}
	}
	if !ok {
		return nil, false, nil
	}
	value, err := fl.lowerExpr(expr)
	if err != nil {
		return nil, true, err
	}
	return &Expr{Kind: ExprTraitUpcast, Type: expected, Target: value, Impl: impl, Trait: expectedInfo.Trait}, true, nil
}

func (fl *functionLowerer) lowerStmt(stmt checker.Statement) (*Stmt, error) {
	if stmt.Break {
		return &Stmt{Kind: StmtBreak}, nil
	}
	if stmt.Expr != nil {
		expr, err := fl.lowerExpr(stmt.Expr)
		if err != nil {
			return nil, err
		}
		return &Stmt{Kind: StmtExpr, Expr: expr}, nil
	}
	switch s := stmt.Stmt.(type) {
	case *checker.VariableDef:
		typeID, err := fl.internContextualCheckerType(s.Type())
		if err != nil {
			return nil, err
		}
		previousLetValue := fl.directLetValue
		fl.directLetValue = s.Value
		value, actualType, err := fl.lowerContextualExpr(s.Value, typeID)
		fl.directLetValue = previousLetValue
		if err != nil {
			return nil, err
		}
		local := fl.defineLocal(s.Name, actualType, s.Mutable)
		if _, bindsRef := s.Type().(*checker.MutableRef); bindsRef && isMutableReferenceProducer(s.Value) {
			// The binding refers to live storage owned elsewhere (a Go pointer
			// produced by a foreign call); keep it pointer-backed in the backend.
			// A value-typed annotation instead coerces to a deref copy: the
			// backend snapshots through ForeignPointer on the call expr.
			fl.fn.Locals[local].Reference = true
		}
		return &Stmt{Kind: StmtLet, Local: local, Name: s.Name, Type: actualType, Mutable: s.Mutable, Value: value}, nil
	case *checker.Reassignment:
		switch target := s.Target.(type) {
		case *checker.Variable:
			local, ok, err := fl.resolveLocal(target.Name())
			if err != nil {
				return nil, err
			}
			if !ok {
				if global, found := fl.l.lookupGlobalInModule(fl.fn.Module, target.Name()); found {
					globalType := fl.l.program.Globals[global].Type
					value, err := fl.lowerExprWithExpected(s.Value, globalType)
					if err != nil {
						return nil, err
					}
					return &Stmt{Kind: StmtAssignGlobal, Global: global, Type: globalType, Value: value}, nil
				}
				return nil, fmt.Errorf("assignment to unknown local %s", target.Name())
			}
			value, err := fl.lowerExprWithExpected(s.Value, fl.fn.Locals[local].Type)
			if err != nil {
				return nil, err
			}
			return &Stmt{Kind: StmtAssign, Local: local, Value: value}, nil
		case *checker.InstanceProperty:
			return fl.lowerFieldAssignment(target, s.Value)
		case *checker.ForeignFieldAccess:
			return fl.lowerForeignFieldAssignment(target, s.Value)
		case *checker.ForeignValue:
			return fl.lowerForeignValueAssignment(target, s.Value)
		default:
			return nil, fmt.Errorf("unsupported AIR assignment target %T", s.Target)
		}
	case *checker.WhileLoop:
		return fl.lowerWhileLoop(s)
	default:
		return nil, fmt.Errorf("unsupported AIR statement %T", stmt.Stmt)
	}
}

func (fl *functionLowerer) lowerWhileLoop(loop *checker.WhileLoop) (*Stmt, error) {
	condition, err := fl.lowerExpr(loop.Condition)
	if err != nil {
		return nil, err
	}
	voidType, err := fl.l.internType(checker.Void)
	if err != nil {
		return nil, err
	}
	body, err := fl.lowerBlockWithDefault(loop.Body.Stmts, voidType)
	if err != nil {
		return nil, err
	}
	return &Stmt{Kind: StmtWhile, Condition: condition, Body: body}, nil
}

func (fl *functionLowerer) lowerStmts(stmt checker.Statement) ([]Stmt, error) {
	if stmt.Expr == nil && stmt.Stmt == nil && !stmt.Break {
		return nil, nil
	}
	// For loops introduce their iteration variables (and lowering machinery) as
	// locals; those are scoped to the loop, so restore on exit to avoid leaking
	// them (and shadowing the iteration name) into the enclosing scope.
	switch loop := stmt.Stmt.(type) {
	case *checker.ForIntRange:
		defer fl.scopeLocals()()
		return fl.lowerForIntRange(loop)
	case *checker.ForInStr:
		defer fl.scopeLocals()()
		return fl.lowerForInStr(loop)
	case *checker.ForInList:
		defer fl.scopeLocals()()
		return fl.lowerForInList(loop)
	case *checker.ForInMap:
		defer fl.scopeLocals()()
		return fl.lowerForInMap(loop)
	case *checker.ForLoop:
		defer fl.scopeLocals()()
		return fl.lowerForLoop(loop)
	}
	lowered, err := fl.lowerStmt(stmt)
	if err != nil || lowered == nil {
		return nil, err
	}
	return []Stmt{*lowered}, nil
}

func (fl *functionLowerer) lowerForIntRange(loop *checker.ForIntRange) ([]Stmt, error) {
	intType, err := fl.l.internType(checker.Int)
	if err != nil {
		return nil, err
	}
	boolType, err := fl.l.internType(checker.Bool)
	if err != nil {
		return nil, err
	}
	start, err := fl.lowerExprWithExpected(loop.Start, intType)
	if err != nil {
		return nil, err
	}
	end, err := fl.lowerExprWithExpected(loop.End, intType)
	if err != nil {
		return nil, err
	}

	counter := fl.defineLocal(loop.Cursor+"$range", intType, true)
	endLocal := fl.defineLocal(loop.Cursor+"$end", intType, false)
	stmts := []Stmt{
		{Kind: StmtLet, Local: counter, Name: loop.Cursor + "$range", Type: intType, Mutable: true, Value: start},
		{Kind: StmtLet, Local: endLocal, Name: loop.Cursor + "$end", Type: intType, Value: end},
	}

	cursor := fl.defineLocal(loop.Cursor, intType, false)
	var indexCounter LocalID
	var index LocalID
	if loop.Index != "" {
		indexCounter = fl.defineLocal(loop.Index+"$range", intType, true)
		index = fl.defineLocal(loop.Index, intType, false)
		stmts = append(stmts, Stmt{Kind: StmtLet, Local: indexCounter, Name: loop.Index + "$range", Type: intType, Mutable: true, Value: &Expr{Kind: ExprConstInt, Type: intType, Int: "0"}})
	}

	body, err := fl.lowerNonProducingBlock(loop.Body.Stmts)
	if err != nil {
		return nil, err
	}
	iterationLocals := []Stmt{{
		Kind:  StmtLet,
		Local: cursor,
		Name:  loop.Cursor,
		Type:  intType,
		Value: loadLocal(intType, counter),
	}}
	if loop.Index != "" {
		iterationLocals = append(iterationLocals, Stmt{
			Kind:  StmtLet,
			Local: index,
			Name:  loop.Index,
			Type:  intType,
			Value: loadLocal(intType, indexCounter),
		})
	}
	body.Stmts = append(iterationLocals, body.Stmts...)
	body.Stmts = append(body.Stmts, Stmt{
		Kind:  StmtAssign,
		Local: counter,
		Value: &Expr{Kind: ExprIntAdd, Type: intType, Left: loadLocal(intType, counter), Right: &Expr{Kind: ExprConstInt, Type: intType, Int: "1"}},
	})
	if loop.Index != "" {
		body.Stmts = append(body.Stmts, Stmt{
			Kind:  StmtAssign,
			Local: indexCounter,
			Value: &Expr{Kind: ExprIntAdd, Type: intType, Left: loadLocal(intType, indexCounter), Right: &Expr{Kind: ExprConstInt, Type: intType, Int: "1"}},
		})
	}

	stmts = append(stmts, Stmt{
		Kind:      StmtWhile,
		Condition: &Expr{Kind: ExprLte, Type: boolType, Left: loadLocal(intType, counter), Right: loadLocal(intType, endLocal)},
		Body:      body,
	})
	return stmts, nil
}

func (fl *functionLowerer) lowerForInStr(loop *checker.ForInStr) ([]Stmt, error) {
	strType, err := fl.l.internType(checker.Str)
	if err != nil {
		return nil, err
	}
	runeType, err := fl.l.internType(checker.Rune)
	if err != nil {
		return nil, err
	}
	runeListType, err := fl.l.internType(checker.MakeList(checker.Rune))
	if err != nil {
		return nil, err
	}
	intType, err := fl.l.internType(checker.Int)
	if err != nil {
		return nil, err
	}
	boolType, err := fl.l.internType(checker.Bool)
	if err != nil {
		return nil, err
	}
	str, err := fl.lowerExprWithExpected(loop.Value, strType)
	if err != nil {
		return nil, err
	}

	runesLocal := fl.defineLocal(loop.Cursor+"$runes", runeListType, false)
	indexName := loop.Cursor + "$index"
	if loop.Index != "" {
		indexName = loop.Index + "$index"
	}
	index := fl.defineLocal(indexName, intType, true)
	cursor := fl.defineLocal(loop.Cursor, runeType, false)
	var visibleIndex LocalID
	if loop.Index != "" {
		visibleIndex = fl.defineLocal(loop.Index, intType, false)
	}

	stmts := []Stmt{
		{Kind: StmtLet, Local: runesLocal, Name: loop.Cursor + "$runes", Type: runeListType, Value: &Expr{Kind: ExprStrRunes, Type: runeListType, Target: str}},
		{Kind: StmtLet, Local: index, Name: indexName, Type: intType, Mutable: true, Value: &Expr{Kind: ExprConstInt, Type: intType, Int: "0"}},
	}

	body, err := fl.lowerNonProducingBlock(loop.Body.Stmts)
	if err != nil {
		return nil, err
	}
	iterationLocals := []Stmt{{
		Kind:  StmtLet,
		Local: cursor,
		Name:  loop.Cursor,
		Type:  runeType,
		Value: &Expr{Kind: ExprListAt, Type: runeType, Target: loadLocal(runeListType, runesLocal), Args: []Expr{*loadLocal(intType, index)}},
	}}
	if loop.Index != "" {
		iterationLocals = append(iterationLocals, Stmt{
			Kind:  StmtLet,
			Local: visibleIndex,
			Name:  loop.Index,
			Type:  intType,
			Value: loadLocal(intType, index),
		})
	}
	body.Stmts = append(iterationLocals, body.Stmts...)
	body.Stmts = append(body.Stmts, Stmt{
		Kind:  StmtAssign,
		Local: index,
		Value: &Expr{Kind: ExprIntAdd, Type: intType, Left: loadLocal(intType, index), Right: &Expr{Kind: ExprConstInt, Type: intType, Int: "1"}},
	})

	stmts = append(stmts, Stmt{
		Kind:      StmtWhile,
		Condition: &Expr{Kind: ExprLt, Type: boolType, Left: loadLocal(intType, index), Right: &Expr{Kind: ExprListSize, Type: intType, Target: loadLocal(runeListType, runesLocal)}},
		Body:      body,
	})
	return stmts, nil
}

func (fl *functionLowerer) lowerForInList(loop *checker.ForInList) ([]Stmt, error) {
	intType, err := fl.l.internType(checker.Int)
	if err != nil {
		return nil, err
	}
	boolType, err := fl.l.internType(checker.Bool)
	if err != nil {
		return nil, err
	}
	list, err := fl.lowerExpr(loop.List)
	if err != nil {
		return nil, err
	}
	listType, ok := fl.l.typeInfo(list.Type)
	if !ok || listType.Kind != TypeList {
		return nil, fmt.Errorf("for-in list lowered with non-list subject %s", loop.List.Type().String())
	}

	listLocal := fl.defineLocal(loop.Cursor+"$list", list.Type, false)
	indexName := loop.Cursor + "$index"
	if loop.Index != "" {
		indexName = loop.Index + "$index"
	}
	index := fl.defineLocal(indexName, intType, true)
	cursor := fl.defineLocal(loop.Cursor, listType.Elem, false)
	var visibleIndex LocalID
	if loop.Index != "" {
		visibleIndex = fl.defineLocal(loop.Index, intType, false)
	}

	stmts := []Stmt{
		{Kind: StmtLet, Local: listLocal, Name: loop.Cursor + "$list", Type: list.Type, Value: list},
		{Kind: StmtLet, Local: index, Name: indexName, Type: intType, Mutable: true, Value: &Expr{Kind: ExprConstInt, Type: intType, Int: "0"}},
	}

	body, err := fl.lowerNonProducingBlock(loop.Body.Stmts)
	if err != nil {
		return nil, err
	}
	iterationLocals := []Stmt{{
		Kind:  StmtLet,
		Local: cursor,
		Name:  loop.Cursor,
		Type:  listType.Elem,
		Value: &Expr{Kind: ExprListAt, Type: listType.Elem, Target: loadLocal(list.Type, listLocal), Args: []Expr{*loadLocal(intType, index)}},
	}}
	if loop.Index != "" {
		iterationLocals = append(iterationLocals, Stmt{
			Kind:  StmtLet,
			Local: visibleIndex,
			Name:  loop.Index,
			Type:  intType,
			Value: loadLocal(intType, index),
		})
	}
	body.Stmts = append(iterationLocals, body.Stmts...)
	body.Stmts = append(body.Stmts, Stmt{
		Kind:  StmtAssign,
		Local: index,
		Value: &Expr{Kind: ExprIntAdd, Type: intType, Left: loadLocal(intType, index), Right: &Expr{Kind: ExprConstInt, Type: intType, Int: "1"}},
	})

	stmts = append(stmts, Stmt{
		Kind:      StmtWhile,
		Condition: &Expr{Kind: ExprLt, Type: boolType, Left: loadLocal(intType, index), Right: &Expr{Kind: ExprListSize, Type: intType, Target: loadLocal(list.Type, listLocal)}},
		Body:      body,
	})
	return stmts, nil
}

func (fl *functionLowerer) lowerForInMap(loop *checker.ForInMap) ([]Stmt, error) {
	m, err := fl.lowerExpr(loop.Map)
	if err != nil {
		return nil, err
	}
	mapType, ok := fl.l.typeInfo(m.Type)
	if !ok || mapType.Kind != TypeMap {
		return nil, fmt.Errorf("for-in map lowered with non-map subject %s", loop.Map.Type().String())
	}

	mapLocal := fl.defineLocal(loop.Key+"$map", m.Type, false)
	key := fl.defineLocal(loop.Key, mapType.Key, false)
	value := fl.defineLocal(loop.Val, mapType.Value, false)

	body, err := fl.lowerNonProducingBlock(loop.Body.Stmts)
	if err != nil {
		return nil, err
	}

	return []Stmt{
		{Kind: StmtLet, Local: mapLocal, Name: loop.Key + "$map", Type: m.Type, Value: m},
		{Kind: StmtForMap, Target: loadLocal(m.Type, mapLocal), Local: key, ValueLocal: value, Body: body},
	}, nil
}

func (fl *functionLowerer) lowerForLoop(loop *checker.ForLoop) ([]Stmt, error) {
	init, err := fl.lowerStmt(checker.Statement{Stmt: loop.Init})
	if err != nil {
		return nil, err
	}
	condition, err := fl.lowerExpr(loop.Condition)
	if err != nil {
		return nil, err
	}
	body, err := fl.lowerNonProducingBlock(loop.Body.Stmts)
	if err != nil {
		return nil, err
	}
	update, err := fl.lowerStmt(checker.Statement{Stmt: loop.Update})
	if err != nil {
		return nil, err
	}
	body.Stmts = append(body.Stmts, *update)
	return []Stmt{*init, Stmt{Kind: StmtWhile, Condition: condition, Body: body}}, nil
}

func (fl *functionLowerer) lowerFunctionTypeCall(name string, args []checker.Expression, target *Expr) (*Expr, error) {
	if target == nil {
		return nil, fmt.Errorf("function value call %s missing target", name)
	}
	functionTypeID, ok := fl.functionTypeIDForCallable(target.Type)
	if !ok {
		return nil, fmt.Errorf("%s is not a function", name)
	}
	loweredArgs, err := fl.lowerArgsForFunctionType(args, functionTypeID)
	if err != nil {
		return nil, err
	}
	typeInfo, _ := fl.l.typeInfo(functionTypeID)
	return &Expr{Kind: ExprCallClosure, Type: typeInfo.Return, Target: target, Args: loweredArgs}, nil
}

// functionTypeIDForCallable resolves a callee type to its function type: a
// function type directly, or a named Go func type through its underlying
// signature (foreign func values are called like ordinary Go func values).
func (fl *functionLowerer) functionTypeIDForCallable(typeID TypeID) (TypeID, bool) {
	typeInfo, ok := fl.l.typeInfo(typeID)
	if !ok {
		return NoType, false
	}
	if typeInfo.Kind == TypeFunction {
		return typeID, true
	}
	// TypeInfo.Value is overloaded for foreign types: it holds the underlying
	// type for named scalars/funcs, but the map value type for named Go maps
	// (which also set Key). Require Key to be unset so a named map whose value
	// type is a func is not misclassified as callable.
	if typeInfo.Kind == TypeForeignType && typeInfo.Value != NoType && typeInfo.Key == NoType {
		if underlying, ok := fl.l.typeInfo(typeInfo.Value); ok && underlying.Kind == TypeFunction {
			return typeInfo.Value, true
		}
	}
	return NoType, false
}

func (fl *functionLowerer) lowerNonProducingBlock(stmts []checker.Statement) (Block, error) {
	// Like lowerBlockWithDefault, this is a lexical scope; restore on exit so the
	// block's locals do not leak into the enclosing scope.
	defer fl.scopeLocals()()
	var block Block
	for _, stmt := range stmts {
		lowered, err := fl.lowerStmts(stmt)
		if err != nil {
			return block, err
		}
		block.Stmts = append(block.Stmts, lowered...)
	}
	return block, nil
}

// lowerChannelCall lowers the Chan static intrinsics to native channel AIR expressions.
func (fl *functionLowerer) lowerChannelCall(typeID TypeID, e *checker.ModuleFunctionCall) (*Expr, error) {
	switch e.Call.Name {
	case "new":
		if len(e.Call.Args) != 1 {
			return nil, fmt.Errorf("Chan::new expects one argument")
		}
		capacity, err := fl.lowerExpr(e.Call.Args[0])
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprMakeChannel, Type: typeID, Args: []Expr{*capacity}}, nil
	}
	return nil, fmt.Errorf("unknown Chan static function %s", e.Call.Name)
}

// lowerSelect lowers a checker Select into an ExprSelect with native channel
// arms (ADR 0032).
func (fl *functionLowerer) lowerSelect(typeID TypeID, sel *checker.Select) (*Expr, error) {
	result := &Expr{Kind: ExprSelect, Type: typeID}
	for _, arm := range sel.Arms {
		switch arm.Kind {
		case checker.SelectArmDefault:
			body, err := fl.lowerBlockWithDefault(arm.Body.Stmts, typeID)
			if err != nil {
				return nil, err
			}
			result.SelectCases = append(result.SelectCases, SelectMatchCase{Kind: SelectArmDefault, Body: body})

		case checker.SelectArmRecv:
			channel, err := fl.lowerExpr(arm.Channel)
			if err != nil {
				return nil, err
			}
			armCase := SelectMatchCase{Kind: SelectArmRecv, Channel: channel}
			if arm.Binding != nil {
				maybeTypeID, err := fl.internType(checker.MakeMaybe(arm.ElemType))
				if err != nil {
					return nil, err
				}
				name := arm.Binding.Name
				oldLocal, hadOld := fl.locals[name]
				bindLocal := fl.defineLocal(name, maybeTypeID, false)
				body, err := fl.lowerBlockWithDefault(arm.Body.Stmts, typeID)
				if hadOld {
					fl.locals[name] = oldLocal
				} else {
					delete(fl.locals, name)
				}
				if err != nil {
					return nil, err
				}
				armCase.HasBind = true
				armCase.BindLocal = bindLocal
				armCase.Body = body
			} else {
				body, err := fl.lowerBlockWithDefault(arm.Body.Stmts, typeID)
				if err != nil {
					return nil, err
				}
				armCase.Body = body
			}
			result.SelectCases = append(result.SelectCases, armCase)

		case checker.SelectArmSend:
			channel, err := fl.lowerExpr(arm.Channel)
			if err != nil {
				return nil, err
			}
			value, err := fl.lowerChannelValue(channel.Type, arm.Value)
			if err != nil {
				return nil, err
			}
			body, err := fl.lowerBlockWithDefault(arm.Body.Stmts, typeID)
			if err != nil {
				return nil, err
			}
			result.SelectCases = append(result.SelectCases, SelectMatchCase{Kind: SelectArmSend, Channel: channel, Value: value, Body: body})
		}
	}
	return result, nil
}

// lowerChannelValue lowers a value being sent on a channel, using the channel's
// element type as the expected type when it is known.
func (fl *functionLowerer) lowerChannelValue(chanTypeID TypeID, value checker.Expression) (*Expr, error) {
	if info, ok := fl.l.typeInfo(chanTypeID); ok && (info.Kind == TypeChannel || info.Kind == TypeReceiver || info.Kind == TypeSender) && info.Elem != NoType {
		return fl.lowerExprWithExpected(value, info.Elem)
	}
	return fl.lowerExpr(value)
}

func (fl *functionLowerer) lowerExpr(expr checker.Expression) (*Expr, error) {
	if e, ok := expr.(*checker.Identifier); ok {
		local, ok, err := fl.resolveLocal(e.Name)
		if err != nil {
			return nil, err
		}
		if !ok {
			if global, ok := fl.l.lookupGlobalInModule(fl.fn.Module, e.Name); ok {
				return &Expr{Kind: ExprLoadGlobal, Type: fl.l.program.Globals[global].Type, Global: global}, nil
			}
			return nil, fmt.Errorf("unknown local %s", e.Name)
		}
		return &Expr{Kind: ExprLoadLocal, Type: fl.fn.Locals[local].Type, Local: local}, nil
	}
	if e, ok := expr.(*checker.Variable); ok {
		local, ok, err := fl.resolveLocal(e.Name())
		if err != nil {
			return nil, err
		}
		if !ok {
			if global, ok := fl.l.lookupGlobalInModule(fl.fn.Module, e.Name()); ok {
				return &Expr{Kind: ExprLoadGlobal, Type: fl.l.program.Globals[global].Type, Global: global}, nil
			}
			if def, ok := e.Type().(*checker.FunctionDef); ok {
				functionType, err := fl.internType(e.Type())
				if err != nil {
					return nil, err
				}
				id, err := fl.l.declareAndLowerFunction(fl.fn.Module, def)
				if err != nil {
					return nil, err
				}
				return &Expr{Kind: ExprFunctionRef, Type: functionType, Function: id}, nil
			}
			return nil, fmt.Errorf("unknown local %s", e.Name())
		}
		return &Expr{Kind: ExprLoadLocal, Type: fl.fn.Locals[local].Type, Local: local}, nil
	}
	if e, ok := expr.(*checker.FunctionCall); ok {
		local, ok, err := fl.resolveLocal(e.Name)
		if err != nil {
			return nil, err
		}
		if ok && fl.localKind(local) == TypeFunction {
			target := &Expr{Kind: ExprLoadLocal, Type: fl.fn.Locals[local].Type, Local: local}
			return fl.lowerFunctionTypeCall(e.Name, e.Args, target)
		}
		if global, ok := fl.l.lookupGlobalInModule(fl.fn.Module, e.Name); ok {
			globalType := fl.l.program.Globals[global].Type
			if typeInfo, ok := fl.l.typeInfo(globalType); ok && typeInfo.Kind == TypeFunction {
				target := &Expr{Kind: ExprLoadGlobal, Type: globalType, Global: global}
				return fl.lowerFunctionTypeCall(e.Name, e.Args, target)
			}
		}
	}
	if prop, ok := expr.(*checker.InstanceProperty); ok {
		return fl.lowerInstanceProperty(NoType, prop)
	}
	typeID, err := fl.internType(expr.Type())
	if err != nil {
		return nil, err
	}
	switch e := expr.(type) {
	case *checker.VoidLiteral:
		return &Expr{Kind: ExprConstVoid, Type: typeID}, nil
	case *checker.IntLiteral:
		return &Expr{Kind: ExprConstInt, Type: typeID, Int: strconv.Itoa(e.Value)}, nil
	case *checker.TypedIntLiteral:
		return &Expr{Kind: ExprConstInt, Type: typeID, Int: e.String()}, nil
	case *checker.FloatLiteral:
		return &Expr{Kind: ExprConstFloat, Type: typeID, Float: e.String()}, nil
	case *checker.TypedFloatLiteral:
		return &Expr{Kind: ExprConstFloat, Type: typeID, Float: e.String()}, nil
	case *checker.BoolLiteral:
		return &Expr{Kind: ExprConstBool, Type: typeID, Bool: e.Value}, nil
	case *checker.StrLiteral:
		return &Expr{Kind: ExprConstStr, Type: typeID, Str: e.Value}, nil
	case *checker.RuneLiteral:
		return &Expr{Kind: ExprConstInt, Type: typeID, Int: strconv.Itoa(int(e.Value))}, nil
	case *checker.Panic:
		message, err := fl.lowerExprWithExpected(e.Message, fl.l.mustIntern(checker.Str))
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprPanic, Type: typeID, Target: message}, nil
	case *checker.TemplateStr:
		return fl.lowerTemplateStr(typeID, e)
	case *checker.FunctionDef:
		return fl.lowerClosure(typeID, e)
	case *checker.FunctionValueCall:
		target, err := fl.lowerExpr(e.Callee)
		if err != nil {
			return nil, err
		}
		return fl.lowerFunctionTypeCall("function value", e.Args, target)
	case *checker.FunctionCall:
		if local, ok := fl.locals[e.Name]; ok {
			if _, callable := fl.functionTypeIDForCallable(fl.fn.Locals[local].Type); callable {
				target := &Expr{Kind: ExprLoadLocal, Type: fl.fn.Locals[local].Type, Local: local}
				return fl.lowerFunctionTypeCall(e.Name, e.Args, target)
			}
		}
		if global, ok := fl.l.lookupGlobalInModule(fl.fn.Module, e.Name); ok {
			globalType := fl.l.program.Globals[global].Type
			if typeInfo, ok := fl.l.typeInfo(globalType); ok && typeInfo.Kind == TypeFunction {
				target := &Expr{Kind: ExprLoadGlobal, Type: globalType, Global: global}
				return fl.lowerFunctionTypeCall(e.Name, e.Args, target)
			}
		}
		if def := e.Definition(); def != nil {
			if def.Body != nil {
				id, err := fl.declareAndLowerFunctionCall(fl.fn.Module, def, e)
				if err != nil {
					return nil, err
				}
				return fl.buildResolvedCallExpr(id, def, e, e.Args)
			}
			id, ok := fl.l.lookupFunction(def.Name)
			if !ok {
				local, hasLocal, err := fl.resolveLocal(e.Name)
				if err != nil {
					return nil, err
				}
				if hasLocal && fl.localKind(local) == TypeFunction {
					target := &Expr{Kind: ExprLoadLocal, Type: fl.fn.Locals[local].Type, Local: local}
					args, err := fl.lowerArgsForFunctionType(e.Args, target.Type)
					if err != nil {
						return nil, err
					}
					returnType := typeID
					if typeInfo, ok := fl.l.typeInfo(target.Type); ok && typeInfo.Kind == TypeFunction {
						returnType = typeInfo.Return
					}
					return &Expr{Kind: ExprCallClosure, Type: returnType, Target: target, Args: args}, nil
				}
				return nil, fmt.Errorf("unknown function call target %s", def.Name)
			}
			args, err := fl.lowerArgsWithSignature(e.Args, fl.l.program.Functions[id].Signature)
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: ExprCall, Type: fl.l.program.Functions[id].Signature.Return, Function: id, Args: args}, nil
		}
		return nil, fmt.Errorf("unsupported unresolved function call %s", e.Name)
	case *checker.ForeignValue:
		return &Expr{Kind: ExprForeignValue, Type: typeID, ForeignTarget: e.Target, ForeignNamespace: e.Namespace, ForeignQualifier: e.Qualifier, ForeignSymbol: e.Symbol}, nil
	case *checker.ForeignInterfaceUpcast:
		value, err := fl.lowerExpr(e.Value)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprForeignInterfaceUpcast, Type: typeID, Target: value, ForeignInterfacePointer: e.Pointer}, nil
	case *checker.ForeignStructInstance:
		fields := make([]StructFieldValue, 0, len(e.Fields))
		for name, valueExpr := range e.Fields {
			value, err := fl.lowerExpr(valueExpr)
			if err != nil {
				return nil, err
			}
			fields = append(fields, StructFieldValue{Name: name, Value: *value})
		}
		return &Expr{Kind: ExprForeignStructInstance, Type: typeID, ForeignTarget: e.Target, ForeignNamespace: e.Namespace, ForeignQualifier: e.Qualifier, ForeignSymbol: e.Name, Fields: fields}, nil
	case *checker.ForeignScalarConvert:
		if !checker.ValidForeignScalarConversion(e.Value.Type(), e.Target) {
			return nil, fmt.Errorf("unsupported foreign scalar conversion: %s -> %s", e.Value.Type().String(), e.Target.String())
		}
		target, err := fl.lowerExpr(e.Value)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprScalarConvert, Type: typeID, Target: target}, nil
	case *checker.ForeignFieldAccess:
		target, err := fl.lowerExpr(e.Subject)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprForeignFieldAccess, Type: typeID, Target: target, ForeignTarget: e.Target, ForeignSymbol: e.Symbol}, nil
	case *checker.ForeignMethodValue:
		target, err := fl.lowerExpr(e.Subject)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprForeignMethodValue, Type: typeID, Target: target, ForeignTarget: e.Target, ForeignNamespace: e.Namespace, ForeignQualifier: e.Qualifier, ForeignReceiver: e.Receiver, ForeignPointer: e.Pointer, ForeignSymbol: e.Symbol}, nil
	case *checker.ForeignMethodCall:
		target, err := fl.lowerExpr(e.Subject)
		if err != nil {
			return nil, err
		}
		args := make([]Expr, len(e.Call.Args))
		methodDef := e.Call.Definition()
		for i, arg := range e.Call.Args {
			var lowered *Expr
			var err error
			if methodDef != nil && i < len(methodDef.Parameters) && methodDef.Parameters[i].Type != nil {
				var paramTypeID TypeID
				paramTypeID, err = fl.internType(methodDef.Parameters[i].Type)
				if err == nil {
					lowered, err = fl.lowerExprWithExpected(arg, paramTypeID)
				}
			} else {
				lowered, err = fl.lowerExpr(arg)
			}
			if err != nil {
				return nil, err
			}
			args[i] = *lowered
		}
		return &Expr{Kind: ExprForeignMethodCall, Type: typeID, Target: target, ForeignTarget: e.Target, ForeignNamespace: e.Namespace, ForeignQualifier: e.Qualifier, ForeignReceiver: e.Receiver, ForeignPointer: e.Pointer, ForeignSymbol: e.Symbol, Args: args}, nil
	case *checker.UnsafeCast:
		value, err := fl.lowerExprWithExpected(e.Value, fl.l.mustIntern(checker.Any))
		if err != nil {
			return nil, err
		}
		targetType := e.TargetType
		mutable := false
		if ref, ok := targetType.(*checker.MutableRef); ok {
			mutable = true
			targetType = ref.Of()
		}
		if foreign, ok := targetType.(*checker.ForeignType); ok && foreign.Pointer {
			// A `mut pkg::T` target resolves to the pointer-shaped foreign type;
			// the backend asserts against the value type and marks the pointer form.
			valueForm := foreign.ValueForm()
			if valueForm == nil {
				return nil, fmt.Errorf("unsafe::cast target %s has no value form", foreign)
			}
			mutable = true
			targetType = valueForm
		}
		targetTypeID, err := fl.l.internType(targetType)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprUnsafeCast, Type: typeID, Target: value, TypeArgs: []TypeID{targetTypeID}, ForeignPointer: mutable}, nil
	case *checker.UnsafeIsNil:
		value, err := fl.lowerExprWithExpected(e.Value, fl.l.mustIntern(checker.Any))
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprUnsafeIsNil, Type: typeID, Target: value}, nil
	case *checker.ForeignFunctionCall:
		if e.PointerResult && fl.directLetValue != checker.Expression(e) {
			return nil, fmt.Errorf("a Go call returning %s must be bound directly with let", e.Call.Type())
		}
		args := make([]Expr, len(e.Call.Args))
		fnDef := e.Call.Definition()
		for i, arg := range e.Call.Args {
			paramType, err := fl.internContextualCheckerType(fnDef.Parameters[i].Type)
			if err != nil {
				return nil, err
			}
			lowered, err := fl.lowerExprWithExpected(arg, paramType)
			if err != nil {
				return nil, err
			}
			args[i] = *lowered
		}
		var typeArgs []TypeID
		for _, typeArg := range e.TypeArgs {
			argID, err := fl.internContextualCheckerType(typeArg)
			if err != nil {
				return nil, err
			}
			typeArgs = append(typeArgs, argID)
		}
		return &Expr{Kind: ExprForeignCall, Type: typeID, ForeignTarget: e.Target, ForeignNamespace: e.Namespace, ForeignQualifier: e.Qualifier, ForeignSymbol: e.Symbol, TypeArgs: typeArgs, ForeignPointer: e.PointerResult, Args: args}, nil
	case *checker.ModuleFunctionCall:
		if kind, ok := resultConstructorKind(e); ok {
			return fl.lowerResultConstructor(kind, typeID, e)
		}
		if kind, ok := maybeConstructorKind(e); ok {
			return fl.lowerMaybeConstructor(kind, typeID, e)
		}
		if e.Module == "ard/list" && e.Call.Name == "new" {
			if len(e.Call.Args) != 0 {
				return nil, fmt.Errorf("ard/list::new expects no arguments")
			}
			return &Expr{Kind: ExprMakeList, Type: typeID}, nil
		}
		if e.Module == "ard/async" && e.Call.Name == "start" {
			if len(e.Call.Args) != 1 {
				return nil, fmt.Errorf("ard/async::start expects one argument")
			}
			task, err := fl.lowerExpr(e.Call.Args[0])
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: ExprAsyncStart, Type: typeID, Args: []Expr{*task}}, nil
		}
		if e.Module == "builtin/Chan" {
			return fl.lowerChannelCall(typeID, e)
		}
		if e.Module == "ard/json" && e.Call.Name == "parse" {
			if len(e.Call.Args) != 1 {
				return nil, fmt.Errorf("ard/json::parse expects one argument")
			}
			arg, err := fl.lowerExpr(e.Call.Args[0])
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: ExprJSONParse, Type: typeID, Target: arg}, nil
		}
		moduleID := fl.l.internModule(e.Module)
		if err := fl.l.ensureModuleGlobalsDeclared(e.Module); err != nil {
			return nil, err
		}
		if err := fl.l.ensureModuleTraitImplsDeclared(e.Module); err != nil {
			return nil, err
		}
		if global, ok := fl.l.lookupGlobalInModule(moduleID, e.Call.Name); ok {
			globalType := fl.l.program.Globals[global].Type
			if typeInfo, ok := fl.l.typeInfo(globalType); ok && typeInfo.Kind == TypeFunction {
				target := &Expr{Kind: ExprLoadGlobal, Type: globalType, Global: global}
				return fl.lowerFunctionTypeCall(e.Call.Name, e.Call.Args, target)
			}
		}
		if def := fl.l.moduleFunctionDefinitionForCall(e); def != nil && def.Body != nil {
			id, err := fl.declareAndLowerFunctionCall(moduleID, def, e.Call)
			if err != nil {
				return nil, err
			}
			return fl.buildResolvedCallExpr(id, def, e.Call, e.Call.Args)
		}
		if id, ok, err := fl.l.resolveModuleFunction(e.Module, e.Call.Name); err != nil {
			return nil, err
		} else if ok {
			args, err := fl.lowerArgsWithSignature(e.Call.Args, fl.l.program.Functions[id].Signature)
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: ExprCall, Type: fl.l.program.Functions[id].Signature.Return, Function: id, Args: args}, nil
		}
		return nil, fmt.Errorf("unsupported module function call %s::%s", e.Module, e.Call.Name)
	case *checker.ModuleSymbol:
		return fl.lowerModuleSymbol(typeID, e)
	case *checker.ListLiteral:
		return fl.lowerListLiteral(typeID, e, NoType)
	case *checker.MapLiteral:
		return fl.lowerMapLiteral(typeID, e, NoType, NoType)
	case *checker.StructInstance:
		return fl.lowerStructInstance(typeID, e)
	case *checker.ModuleStructInstance:
		if err := fl.l.ensureModuleTypesDeclared(e.Module); err != nil {
			return nil, err
		}
		return fl.lowerStructInstance(typeID, e.Property)
	case *checker.InstanceProperty:
		return fl.lowerInstanceProperty(typeID, e)
	case *checker.InstanceMethod:
		return fl.lowerInstanceMethod(typeID, e)
	case *checker.StrMethod:
		return fl.lowerStrMethod(typeID, e)
	case *checker.ByteMethod:
		if e.Kind == checker.ByteToInt {
			return fl.lowerUnary(ExprToInt, typeID, e.Subject)
		}
		if e.Kind == checker.ByteToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
		}
		return nil, fmt.Errorf("unsupported AIR Byte method %d", e.Kind)
	case *checker.RuneMethod:
		if e.Kind == checker.RuneToInt {
			return fl.lowerUnary(ExprToInt, typeID, e.Subject)
		}
		if e.Kind == checker.RuneToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
		}
		return nil, fmt.Errorf("unsupported AIR Rune method %d", e.Kind)
	case *checker.IntMethod:
		if e.Kind == checker.IntToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
		}
		if e.Kind == checker.IntToF64 {
			return fl.lowerUnary(ExprToF64, typeID, e.Subject)
		}
		return nil, fmt.Errorf("unsupported AIR Int method %d", e.Kind)
	case *checker.FloatMethod:
		if e.Kind == checker.FloatToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
		}
		if e.Kind == checker.FloatToInt {
			return fl.lowerUnary(ExprToInt, typeID, e.Subject)
		}
		return nil, fmt.Errorf("unsupported AIR Float method %d", e.Kind)
	case *checker.BoolMethod:
		if e.Kind == checker.BoolToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
		}
		return nil, fmt.Errorf("unsupported AIR Bool method %d", e.Kind)
	case *checker.ListMethod:
		return fl.lowerListMethod(typeID, e)
	case *checker.MapMethod:
		return fl.lowerMapMethod(typeID, e)
	case *checker.EnumVariant:
		return &Expr{Kind: ExprEnumVariant, Type: typeID, Variant: int(e.Variant), Discriminant: e.Discriminant}, nil
	case *checker.BoolMatch:
		return fl.lowerBoolMatch(typeID, e)
	case *checker.IntMatch:
		return fl.lowerIntMatch(typeID, e)
	case *checker.StrMatch:
		return fl.lowerStrMatch(typeID, e)
	case *checker.EnumMatch:
		return fl.lowerEnumMatch(typeID, e)
	case *checker.UnionMatch:
		return fl.lowerUnionMatch(typeID, e)
	case *checker.ForeignTypeMatch:
		return fl.lowerForeignTypeMatch(typeID, e)
	case *checker.MaybeMethod:
		return fl.lowerMaybeMethod(typeID, e)
	case *checker.OptionMatch:
		return fl.lowerOptionMatch(typeID, e)
	case *checker.Select:
		return fl.lowerSelect(typeID, e)
	case *checker.ResultMethod:
		return fl.lowerResultMethod(typeID, e)
	case *checker.ResultMatch:
		return fl.lowerResultMatch(typeID, e)
	case *checker.TryOp:
		return fl.lowerTryOp(typeID, e)
	case *checker.IntAddition:
		return fl.lowerBinary(ExprIntAdd, typeID, e.Left, e.Right)
	case *checker.IntSubtraction:
		return fl.lowerBinary(ExprIntSub, typeID, e.Left, e.Right)
	case *checker.IntMultiplication:
		return fl.lowerBinary(ExprIntMul, typeID, e.Left, e.Right)
	case *checker.IntDivision:
		return fl.lowerBinary(ExprIntDiv, typeID, e.Left, e.Right)
	case *checker.IntModulo:
		return fl.lowerBinary(ExprIntMod, typeID, e.Left, e.Right)
	case *checker.FloatAddition:
		return fl.lowerBinary(ExprFloatAdd, typeID, e.Left, e.Right)
	case *checker.FloatSubtraction:
		return fl.lowerBinary(ExprFloatSub, typeID, e.Left, e.Right)
	case *checker.FloatMultiplication:
		return fl.lowerBinary(ExprFloatMul, typeID, e.Left, e.Right)
	case *checker.FloatDivision:
		return fl.lowerBinary(ExprFloatDiv, typeID, e.Left, e.Right)
	case *checker.StrAddition:
		return fl.lowerBinary(ExprStrConcat, typeID, e.Left, e.Right)
	case *checker.Equality:
		return fl.lowerBinary(ExprEq, typeID, e.Left, e.Right)
	case *checker.Inequality:
		return fl.lowerBinary(ExprNotEq, typeID, e.Left, e.Right)
	case *checker.IntLess:
		return fl.lowerBinary(ExprLt, typeID, e.Left, e.Right)
	case *checker.IntLessEqual:
		return fl.lowerBinary(ExprLte, typeID, e.Left, e.Right)
	case *checker.IntGreater:
		return fl.lowerBinary(ExprGt, typeID, e.Left, e.Right)
	case *checker.IntGreaterEqual:
		return fl.lowerBinary(ExprGte, typeID, e.Left, e.Right)
	case *checker.FloatLess:
		return fl.lowerBinary(ExprLt, typeID, e.Left, e.Right)
	case *checker.FloatLessEqual:
		return fl.lowerBinary(ExprLte, typeID, e.Left, e.Right)
	case *checker.FloatGreater:
		return fl.lowerBinary(ExprGt, typeID, e.Left, e.Right)
	case *checker.FloatGreaterEqual:
		return fl.lowerBinary(ExprGte, typeID, e.Left, e.Right)
	case *checker.And:
		return fl.lowerBinary(ExprAnd, typeID, e.Left, e.Right)
	case *checker.Or:
		return fl.lowerBinary(ExprOr, typeID, e.Left, e.Right)
	case *checker.Not:
		value, err := fl.lowerExpr(e.Value)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprNot, Type: typeID, Target: value}, nil
	case *checker.Negation:
		value, err := fl.lowerExpr(e.Value)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprNeg, Type: typeID, Target: value}, nil
	case *checker.Block:
		body, err := fl.lowerBlockWithDefault(e.Stmts, typeID)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprBlock, Type: typeID, Body: body}, nil
	case *checker.UnsafeBlock:
		resultType := typeID
		resultInfo, ok := fl.l.typeInfo(resultType)
		if !ok || resultInfo.Kind != TypeResult {
			var err error
			resultType, err = fl.internType(e.Type())
			if err != nil {
				return nil, err
			}
			resultInfo, ok = fl.l.typeInfo(resultType)
			if !ok || resultInfo.Kind != TypeResult {
				return nil, fmt.Errorf("unsafe block lowered with non-Result type %s", e.Type().String())
			}
		}
		previousReturn := fl.fn.Signature.Return
		fl.fn.Signature.Return = resultType
		body, err := fl.lowerBlockWithDefault(e.Body.Stmts, resultInfo.Value)
		fl.fn.Signature.Return = previousReturn
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprUnsafeBlock, Type: resultType, Body: body}, nil
	case *checker.If:
		return fl.lowerIf(typeID, e)
	case *checker.ConditionalMatch:
		return fl.lowerConditionalMatch(typeID, e)
	default:
		return nil, fmt.Errorf("unsupported AIR expression %T", expr)
	}
}

func (fl *functionLowerer) lowerConditionalMatch(typeID TypeID, match *checker.ConditionalMatch) (*Expr, error) {
	return fl.lowerConditionalCases(typeID, match.Cases, match.CatchAll)
}

func (fl *functionLowerer) lowerConditionalCases(typeID TypeID, cases []checker.ConditionalCase, catchAll *checker.Block) (*Expr, error) {
	if len(cases) == 0 {
		body := Block{}
		if catchAll != nil {
			var err error
			body, err = fl.lowerBlockWithDefault(catchAll.Stmts, typeID)
			if err != nil {
				return nil, err
			}
		}
		return &Expr{Kind: ExprBlock, Type: typeID, Body: body}, nil
	}

	condition, err := fl.lowerExpr(cases[0].Condition)
	if err != nil {
		return nil, err
	}
	thenBlock, err := fl.lowerBlockWithDefault(cases[0].Body.Stmts, typeID)
	if err != nil {
		return nil, err
	}
	elseExpr, err := fl.lowerConditionalCases(typeID, cases[1:], catchAll)
	if err != nil {
		return nil, err
	}
	return &Expr{
		Kind:      ExprIf,
		Type:      typeID,
		Condition: condition,
		Then:      thenBlock,
		Else:      Block{Result: elseExpr},
	}, nil
}

func (fl *functionLowerer) lowerIf(typeID TypeID, expr *checker.If) (*Expr, error) {
	if len(expr.Branches) == 0 {
		return nil, fmt.Errorf("if expression missing branches")
	}
	return fl.lowerIfBranches(typeID, expr.Branches, expr.Else)
}

func (fl *functionLowerer) lowerIfBranches(typeID TypeID, branches []checker.IfBranch, elseBlock *checker.Block) (*Expr, error) {
	if len(branches) == 0 {
		return nil, fmt.Errorf("if expression missing branches")
	}
	branch := branches[0]
	condition, err := fl.lowerExpr(branch.Condition)
	if err != nil {
		return nil, err
	}
	thenBlock, err := fl.lowerBlockWithDefault(branch.Body.Stmts, typeID)
	if err != nil {
		return nil, err
	}
	var airElse Block
	if len(branches) > 1 {
		nested, err := fl.lowerIfBranches(typeID, branches[1:], elseBlock)
		if err != nil {
			return nil, err
		}
		airElse = Block{Result: nested}
	} else if elseBlock != nil {
		airElse, err = fl.lowerBlockWithDefault(elseBlock.Stmts, typeID)
		if err != nil {
			return nil, err
		}
	}
	return &Expr{Kind: ExprIf, Type: typeID, Condition: condition, Then: thenBlock, Else: airElse}, nil
}

func (fl *functionLowerer) lowerResultConstructor(kind ExprKind, typeID TypeID, call *checker.ModuleFunctionCall) (*Expr, error) {
	if len(call.Call.Args) != 1 {
		return nil, fmt.Errorf("%s::%s expects one argument", call.Module, call.Call.Name)
	}
	valueType := NoType
	if resultInfo, ok := fl.l.typeInfo(typeID); ok && resultInfo.Kind == TypeResult {
		valueType = resultInfo.Value
		if kind == ExprMakeResultErr {
			valueType = resultInfo.Error
		}
	}
	var value *Expr
	var err error
	if valueType != NoType {
		value, err = fl.lowerExprWithExpected(call.Call.Args[0], valueType)
	} else {
		value, err = fl.lowerExpr(call.Call.Args[0])
	}
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: kind, Type: typeID, Target: value}, nil
}

func (fl *functionLowerer) lowerMaybeConstructor(kind ExprKind, typeID TypeID, call *checker.ModuleFunctionCall) (*Expr, error) {
	switch kind {
	case ExprMakeMaybeSome:
		if len(call.Call.Args) != 1 {
			return nil, fmt.Errorf("%s::%s expects one argument", call.Module, call.Call.Name)
		}
		valueType := NoType
		if maybeInfo, ok := fl.l.typeInfo(typeID); ok && maybeInfo.Kind == TypeMaybe {
			valueType = maybeInfo.Elem
		}
		var value *Expr
		var err error
		if valueType != NoType {
			value, err = fl.lowerExprWithExpected(call.Call.Args[0], valueType)
		} else {
			value, err = fl.lowerExpr(call.Call.Args[0])
		}
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: kind, Type: typeID, Target: value}, nil
	case ExprMakeMaybeNone:
		if len(call.Call.Args) != 0 {
			return nil, fmt.Errorf("%s::%s expects no arguments", call.Module, call.Call.Name)
		}
		return &Expr{Kind: kind, Type: typeID}, nil
	default:
		return nil, fmt.Errorf("invalid Maybe constructor kind %d", kind)
	}
}

func (fl *functionLowerer) lowerBoolMatch(typeID TypeID, match *checker.BoolMatch) (*Expr, error) {
	condition, err := fl.lowerExpr(match.Subject)
	if err != nil {
		return nil, err
	}
	if match.True == nil || match.False == nil {
		return nil, fmt.Errorf("Bool match missing branch")
	}
	trueBlock, err := fl.lowerBlockWithDefault(match.True.Stmts, typeID)
	if err != nil {
		return nil, err
	}
	falseBlock, err := fl.lowerBlockWithDefault(match.False.Stmts, typeID)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: ExprIf, Type: typeID, Condition: condition, Then: trueBlock, Else: falseBlock}, nil
}

func (fl *functionLowerer) lowerListLiteral(typeID TypeID, list *checker.ListLiteral, elem TypeID) (*Expr, error) {
	args := make([]Expr, len(list.Elements))
	for i, item := range list.Elements {
		var lowered *Expr
		var err error
		if elem != NoType {
			lowered, err = fl.lowerExprWithExpected(item, elem)
		} else {
			lowered, err = fl.lowerExpr(item)
		}
		if err != nil {
			return nil, err
		}
		args[i] = *lowered
	}
	return &Expr{Kind: ExprMakeList, Type: typeID, Args: args}, nil
}

func (fl *functionLowerer) lowerMapLiteral(typeID TypeID, m *checker.MapLiteral, keyType, valueType TypeID) (*Expr, error) {
	entries := make([]MapEntry, len(m.Keys))
	for i := range m.Keys {
		var key *Expr
		var value *Expr
		var err error
		if keyType != NoType {
			key, err = fl.lowerExprWithExpected(m.Keys[i], keyType)
		} else {
			key, err = fl.lowerExpr(m.Keys[i])
		}
		if err != nil {
			return nil, err
		}
		if valueType != NoType {
			value, err = fl.lowerExprWithExpected(m.Values[i], valueType)
		} else {
			value, err = fl.lowerExpr(m.Values[i])
		}
		if err != nil {
			return nil, err
		}
		entries[i] = MapEntry{Key: *key, Value: *value}
	}
	return &Expr{Kind: ExprMakeMap, Type: typeID, Entries: entries}, nil
}

func (fl *functionLowerer) lowerEnumMatch(typeID TypeID, match *checker.EnumMatch) (*Expr, error) {
	subject, err := fl.lowerExpr(match.Subject)
	if err != nil {
		return nil, err
	}
	enumType, ok := fl.l.typeInfo(subject.Type)
	if !ok || enumType.Kind != TypeEnum {
		return nil, fmt.Errorf("enum match lowered with non-enum subject %s", match.Subject.Type().String())
	}

	cases := make([]EnumMatchCase, 0, len(match.Cases))
	for variant, block := range match.Cases {
		if block == nil {
			continue
		}
		if variant < 0 || variant >= len(enumType.Variants) {
			return nil, fmt.Errorf("enum match case index %d out of range for %s", variant, enumType.Name)
		}
		lowered, err := fl.lowerBlockWithDefault(block.Stmts, typeID)
		if err != nil {
			return nil, err
		}
		cases = append(cases, EnumMatchCase{
			Variant:      variant,
			Discriminant: enumType.Variants[variant].Discriminant,
			Body:         lowered,
		})
	}

	var catchAll Block
	if match.CatchAll != nil {
		catchAll, err = fl.lowerBlockWithDefault(match.CatchAll.Stmts, typeID)
		if err != nil {
			return nil, err
		}
	}

	return &Expr{
		Kind:      ExprMatchEnum,
		Type:      typeID,
		Target:    subject,
		EnumCases: cases,
		CatchAll:  catchAll,
	}, nil
}

func (fl *functionLowerer) lowerIntMatch(typeID TypeID, match *checker.IntMatch) (*Expr, error) {
	subject, err := fl.lowerExpr(match.Subject)
	if err != nil {
		return nil, err
	}
	subjectType, ok := fl.l.typeInfo(subject.Type)
	if !ok || (subjectType.Kind != TypeInt && subjectType.Kind != TypeByte && subjectType.Kind != TypeRune) {
		return nil, fmt.Errorf("int match lowered with non-integer subject %s", match.Subject.Type().String())
	}

	intValues := make([]int, 0, len(match.IntCases))
	for value := range match.IntCases {
		intValues = append(intValues, value)
	}
	sort.Ints(intValues)
	intCases := make([]IntMatchCase, 0, len(intValues))
	for _, value := range intValues {
		block := match.IntCases[value]
		if block == nil {
			continue
		}
		lowered, err := fl.lowerBlockWithDefault(block.Stmts, typeID)
		if err != nil {
			return nil, err
		}
		intCases = append(intCases, IntMatchCase{Value: value, Body: lowered})
	}

	ranges := make([]checker.IntRange, 0, len(match.RangeCases))
	for intRange := range match.RangeCases {
		ranges = append(ranges, intRange)
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start == ranges[j].Start {
			return ranges[i].End < ranges[j].End
		}
		return ranges[i].Start < ranges[j].Start
	})
	rangeCases := make([]IntRangeMatchCase, 0, len(ranges))
	for _, intRange := range ranges {
		block := match.RangeCases[intRange]
		if block == nil {
			continue
		}
		lowered, err := fl.lowerBlockWithDefault(block.Stmts, typeID)
		if err != nil {
			return nil, err
		}
		rangeCases = append(rangeCases, IntRangeMatchCase{Start: intRange.Start, End: intRange.End, Body: lowered})
	}

	var catchAll Block
	if match.CatchAll != nil {
		catchAll, err = fl.lowerBlockWithDefault(match.CatchAll.Stmts, typeID)
		if err != nil {
			return nil, err
		}
	}

	return &Expr{
		Kind:       ExprMatchInt,
		Type:       typeID,
		Target:     subject,
		IntCases:   intCases,
		RangeCases: rangeCases,
		CatchAll:   catchAll,
	}, nil
}

func (fl *functionLowerer) lowerStrMatch(typeID TypeID, match *checker.StrMatch) (*Expr, error) {
	subject, err := fl.lowerExpr(match.Subject)
	if err != nil {
		return nil, err
	}
	subjectType, ok := fl.l.typeInfo(subject.Type)
	if !ok || subjectType.Kind != TypeStr {
		return nil, fmt.Errorf("str match lowered with non-str subject %s", match.Subject.Type().String())
	}

	values := make([]string, 0, len(match.Cases))
	for value := range match.Cases {
		values = append(values, value)
	}
	sort.Strings(values)
	strCases := make([]StrMatchCase, 0, len(values))
	for _, value := range values {
		block := match.Cases[value]
		if block == nil {
			continue
		}
		lowered, err := fl.lowerBlockWithDefault(block.Stmts, typeID)
		if err != nil {
			return nil, err
		}
		strCases = append(strCases, StrMatchCase{Value: value, Body: lowered})
	}

	var catchAll Block
	if match.CatchAll != nil {
		catchAll, err = fl.lowerBlockWithDefault(match.CatchAll.Stmts, typeID)
		if err != nil {
			return nil, err
		}
	}

	return &Expr{Kind: ExprMatchStr, Type: typeID, Target: subject, StrCases: strCases, CatchAll: catchAll}, nil
}

func (fl *functionLowerer) lowerUnionMatch(typeID TypeID, match *checker.UnionMatch) (*Expr, error) {
	subject, err := fl.lowerExpr(match.Subject)
	if err != nil {
		return nil, err
	}
	unionType, ok := fl.l.typeInfo(subject.Type)
	if !ok || unionType.Kind != TypeUnion {
		return nil, fmt.Errorf("union match lowered with non-union subject %s", match.Subject.Type().String())
	}

	cases := make([]UnionMatchCase, 0, len(match.TypeCases))
	for _, member := range unionType.Members {
		matchCase := match.TypeCases[member.Name]
		if matchCase == nil {
			continue
		}
		local, body, err := fl.lowerBoundBlockWithDefault(matchCase.Pattern.Name, member.Type, matchCase.Body.Stmts, typeID)
		if err != nil {
			return nil, err
		}
		cases = append(cases, UnionMatchCase{Tag: member.Tag, Local: local, Body: body})
	}

	var catchAll Block
	if match.CatchAll != nil {
		catchAll, err = fl.lowerBlockWithDefault(match.CatchAll.Stmts, typeID)
		if err != nil {
			return nil, err
		}
	}

	return &Expr{
		Kind:       ExprMatchUnion,
		Type:       typeID,
		Target:     subject,
		UnionCases: cases,
		CatchAll:   catchAll,
	}, nil
}

func (fl *functionLowerer) lowerForeignTypeMatch(typeID TypeID, match *checker.ForeignTypeMatch) (*Expr, error) {
	subject, err := fl.lowerExpr(match.Subject)
	if err != nil {
		return nil, err
	}
	cases := make([]ForeignTypeMatchCase, 0, len(match.Cases))
	for _, matchCase := range match.Cases {
		if matchCase.Body == nil {
			continue
		}
		caseTypeID, err := fl.l.internType(matchCase.Type)
		if err != nil {
			return nil, err
		}
		bound := matchCase.Binding != "" && matchCase.Binding != "_"
		var local LocalID
		var body Block
		if bound {
			local, body, err = fl.lowerBoundBlockWithDefault(matchCase.Binding, caseTypeID, matchCase.Body.Stmts, typeID)
		} else {
			body, err = fl.lowerBlockWithDefault(matchCase.Body.Stmts, typeID)
		}
		if err != nil {
			return nil, err
		}
		cases = append(cases, ForeignTypeMatchCase{Type: caseTypeID, Local: local, Bound: bound, Body: body})
	}
	if match.CatchAll == nil {
		return nil, fmt.Errorf("foreign type match missing catch-all")
	}
	catchAll, err := fl.lowerBlockWithDefault(match.CatchAll.Stmts, typeID)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: ExprMatchForeignType, Type: typeID, Target: subject, ForeignCases: cases, CatchAll: catchAll}, nil
}

func (fl *functionLowerer) lowerOptionMatch(typeID TypeID, match *checker.OptionMatch) (*Expr, error) {
	subject, err := fl.lowerExpr(match.Subject)
	if err != nil {
		return nil, err
	}
	maybeType, ok := fl.l.typeInfo(subject.Type)
	if !ok || maybeType.Kind != TypeMaybe {
		return nil, fmt.Errorf("Maybe match lowered with non-Maybe subject %s", match.Subject.Type().String())
	}
	if match.Some == nil || match.Some.Pattern == nil || match.Some.Body == nil {
		return nil, fmt.Errorf("Maybe match missing binding case")
	}
	if match.None == nil {
		return nil, fmt.Errorf("Maybe match missing none case")
	}

	pattern := match.Some.Pattern.Name
	oldLocal, hadOldLocal := fl.locals[pattern]
	someLocal := fl.defineLocal(pattern, maybeType.Elem, false)
	someBlock, err := fl.lowerBlockWithDefault(match.Some.Body.Stmts, typeID)
	if hadOldLocal {
		fl.locals[pattern] = oldLocal
	} else {
		delete(fl.locals, pattern)
	}
	if err != nil {
		return nil, err
	}
	noneBlock, err := fl.lowerBlockWithDefault(match.None.Stmts, typeID)
	if err != nil {
		return nil, err
	}

	return &Expr{
		Kind:      ExprMatchMaybe,
		Type:      typeID,
		Target:    subject,
		SomeLocal: someLocal,
		Some:      someBlock,
		None:      noneBlock,
	}, nil
}

func (fl *functionLowerer) lowerMaybeMethod(typeID TypeID, method *checker.MaybeMethod) (*Expr, error) {
	target, err := fl.lowerExpr(method.Subject)
	if err != nil {
		return nil, err
	}
	args, err := fl.lowerArgs(method.Args)
	if err != nil {
		return nil, err
	}

	var kind ExprKind
	switch method.Kind {
	case checker.MaybeExpect:
		kind = ExprMaybeExpect
	case checker.MaybeIsNone:
		kind = ExprMaybeIsNone
	case checker.MaybeIsSome:
		kind = ExprMaybeIsSome
	case checker.MaybeOr:
		kind = ExprMaybeOr
	case checker.MaybeMap:
		kind = ExprMaybeMap
	case checker.MaybeAndThen:
		kind = ExprMaybeAndThen
	default:
		return nil, fmt.Errorf("unsupported AIR Maybe method %d", method.Kind)
	}
	return &Expr{Kind: kind, Type: typeID, Target: target, Args: args}, nil
}

func (fl *functionLowerer) lowerStrMethod(typeID TypeID, method *checker.StrMethod) (*Expr, error) {
	strType, err := fl.l.internType(checker.Str)
	if err != nil {
		return nil, err
	}
	intType, err := fl.l.internType(checker.Int)
	if err != nil {
		return nil, err
	}

	var kind ExprKind
	var expected []TypeID
	switch method.Kind {
	case checker.StrAt:
		kind = ExprStrAt
		expected = []TypeID{intType}
	case checker.StrBytes:
		kind = ExprStrBytes
	case checker.StrRunes:
		kind = ExprStrRunes
	case checker.StrSize:
		kind = ExprStrSize
	case checker.StrIsEmpty:
		kind = ExprStrIsEmpty
	case checker.StrContains:
		kind = ExprStrContains
		expected = []TypeID{strType}
	case checker.StrReplace:
		kind = ExprStrReplace
		expected = []TypeID{strType, strType}
	case checker.StrReplaceAll:
		kind = ExprStrReplaceAll
		expected = []TypeID{strType, strType}
	case checker.StrSplit:
		kind = ExprStrSplit
		expected = []TypeID{strType}
	case checker.StrStartsWith:
		kind = ExprStrStartsWith
		expected = []TypeID{strType}
	case checker.StrEndsWith:
		kind = ExprStrEndsWith
		expected = []TypeID{strType}
	case checker.StrToStr:
		kind = ExprToStr
	case checker.StrTrim:
		kind = ExprStrTrim
	default:
		return nil, fmt.Errorf("unsupported AIR Str method %d", method.Kind)
	}

	target, err := fl.lowerExpr(method.Subject)
	if err != nil {
		return nil, err
	}
	args, err := fl.lowerArgsWithTypeIDs(method.Args, expected)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: kind, Type: typeID, Target: target, Args: args}, nil
}

func (fl *functionLowerer) lowerListMethod(typeID TypeID, method *checker.ListMethod) (*Expr, error) {
	target, err := fl.lowerExpr(method.Subject)
	if err != nil {
		return nil, err
	}
	listType, ok := fl.l.typeInfo(target.Type)
	if !ok || (listType.Kind != TypeList && !(listType.Kind == TypeForeignType && listType.Elem != NoType)) {
		return nil, fmt.Errorf("List method lowered with non-list subject %s", method.Subject.Type().String())
	}

	intType, err := fl.l.internType(checker.Int)
	if err != nil {
		return nil, err
	}

	var kind ExprKind
	var expected []TypeID
	switch method.Kind {
	case checker.ListAt:
		kind = ExprListAt
		expected = []TypeID{intType}
	case checker.ListPrepend:
		kind = ExprListPrepend
		expected = []TypeID{listType.Elem}
	case checker.ListPush:
		kind = ExprListPush
		expected = []TypeID{listType.Elem}
	case checker.ListSet:
		kind = ExprListSet
		expected = []TypeID{intType, listType.Elem}
	case checker.ListSize:
		kind = ExprListSize
	case checker.ListSort:
		kind = ExprListSort
	case checker.ListSwap:
		kind = ExprListSwap
		expected = []TypeID{intType, intType}
	default:
		return nil, fmt.Errorf("unsupported AIR List method %d", method.Kind)
	}

	args, err := fl.lowerArgsWithTypeIDs(method.Args, expected)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: kind, Type: typeID, Target: target, Args: args}, nil
}

func (fl *functionLowerer) lowerMapMethod(typeID TypeID, method *checker.MapMethod) (*Expr, error) {
	target, err := fl.lowerExpr(method.Subject)
	if err != nil {
		return nil, err
	}
	mapType, ok := fl.l.typeInfo(target.Type)
	if !ok || (mapType.Kind != TypeMap && !(mapType.Kind == TypeForeignType && mapType.Key != NoType && mapType.Value != NoType)) {
		return nil, fmt.Errorf("Map method lowered with non-map subject %s", method.Subject.Type().String())
	}

	var kind ExprKind
	var expected []TypeID
	switch method.Kind {
	case checker.MapKeys:
		kind = ExprMapKeys
	case checker.MapSize:
		kind = ExprMapSize
	case checker.MapGet:
		kind = ExprMapGet
		expected = []TypeID{mapType.Key}
	case checker.MapSet:
		kind = ExprMapSet
		expected = []TypeID{mapType.Key, mapType.Value}
	case checker.MapDrop:
		kind = ExprMapDrop
		expected = []TypeID{mapType.Key}
	case checker.MapHas:
		kind = ExprMapHas
		expected = []TypeID{mapType.Key}
	default:
		return nil, fmt.Errorf("unsupported AIR Map method %d", method.Kind)
	}

	args, err := fl.lowerArgsWithTypeIDs(method.Args, expected)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: kind, Type: typeID, Target: target, Args: args}, nil
}

func (fl *functionLowerer) lowerResultMatch(typeID TypeID, match *checker.ResultMatch) (*Expr, error) {
	subject, err := fl.lowerExpr(match.Subject)
	if err != nil {
		return nil, err
	}
	resultType, ok := fl.l.typeInfo(subject.Type)
	if !ok || resultType.Kind != TypeResult {
		return nil, fmt.Errorf("Result match lowered with non-Result subject %s", match.Subject.Type().String())
	}
	if match.Ok == nil || match.Ok.Pattern == nil || match.Ok.Body == nil {
		return nil, fmt.Errorf("Result match missing ok case")
	}
	if match.Err == nil || match.Err.Pattern == nil || match.Err.Body == nil {
		return nil, fmt.Errorf("Result match missing err case")
	}

	okLocal, okBlock, err := fl.lowerBoundBlockWithDefault(match.Ok.Pattern.Name, resultType.Value, match.Ok.Body.Stmts, typeID)
	if err != nil {
		return nil, err
	}
	errLocal, errBlock, err := fl.lowerBoundBlockWithDefault(match.Err.Pattern.Name, resultType.Error, match.Err.Body.Stmts, typeID)
	if err != nil {
		return nil, err
	}

	return &Expr{
		Kind:     ExprMatchResult,
		Type:     typeID,
		Target:   subject,
		OkLocal:  okLocal,
		ErrLocal: errLocal,
		Ok:       okBlock,
		Err:      errBlock,
	}, nil
}

func (fl *functionLowerer) lowerResultMethod(typeID TypeID, method *checker.ResultMethod) (*Expr, error) {
	var target *Expr
	var err error
	if subjectType, ok := fl.resultMethodSubjectType(method); ok {
		target, err = fl.lowerExprWithExpected(method.Subject, subjectType)
	} else {
		target, err = fl.lowerExpr(method.Subject)
	}
	if err != nil {
		return nil, err
	}
	args, err := fl.lowerArgs(method.Args)
	if err != nil {
		return nil, err
	}

	var kind ExprKind
	switch method.Kind {
	case checker.ResultExpect:
		kind = ExprResultExpect
	case checker.ResultOr:
		kind = ExprResultOr
	case checker.ResultIsOk:
		kind = ExprResultIsOk
	case checker.ResultIsErr:
		kind = ExprResultIsErr
	case checker.ResultMap:
		kind = ExprResultMap
	case checker.ResultMapErr:
		kind = ExprResultMapErr
	case checker.ResultAndThen:
		kind = ExprResultAndThen
	default:
		return nil, fmt.Errorf("unsupported AIR Result method %d", method.Kind)
	}
	return &Expr{Kind: kind, Type: typeID, Target: target, Args: args}, nil
}

func (fl *functionLowerer) resultMethodSubjectType(method *checker.ResultMethod) (TypeID, bool) {
	subjectType, ok := method.Subject.Type().(*checker.Result)
	if !ok {
		return NoType, false
	}
	valueType := subjectType.Val()
	errType := subjectType.Err()
	if returnType, ok := method.ReturnType.(*checker.Result); ok {
		if typeHasUnresolvedTypeVar(valueType) {
			valueType = returnType.Val()
		}
		if typeHasUnresolvedTypeVar(errType) {
			errType = returnType.Err()
		}
	}
	typeID, err := fl.internContextualCheckerType(checker.MakeResult(valueType, errType))
	if err != nil {
		return NoType, false
	}
	return typeID, true
}

func (fl *functionLowerer) lowerTryOp(typeID TypeID, op *checker.TryOp) (*Expr, error) {
	target, err := fl.lowerExpr(op.Expr())
	if err != nil {
		return nil, err
	}

	kind := ExprTryResult
	if op.Kind == checker.TryMaybe {
		kind = ExprTryMaybe
	}
	expr := &Expr{
		Kind:       kind,
		Type:       typeID,
		Target:     target,
		CatchLocal: -1,
	}
	if op.CatchBlock == nil {
		return expr, nil
	}

	expr.HasCatch = true
	if op.Kind == checker.TryResult {
		errType, err := fl.internType(op.ErrType)
		if err != nil {
			return nil, err
		}
		catchLocal, catchBlock, err := fl.lowerBoundBlock(op.CatchVar, errType, op.CatchBlock.Stmts)
		if err != nil {
			return nil, err
		}
		expr.CatchLocal = catchLocal
		expr.Catch = catchBlock
		return expr, nil
	}

	catchBlock, err := fl.lowerBlock(op.CatchBlock.Stmts)
	if err != nil {
		return nil, err
	}
	expr.Catch = catchBlock
	return expr, nil
}

func (fl *functionLowerer) lowerBoundBlock(name string, typeID TypeID, stmts []checker.Statement) (LocalID, Block, error) {
	return fl.lowerBoundBlockWithDefault(name, typeID, stmts, fl.fn.Signature.Return)
}

func (fl *functionLowerer) lowerBoundBlockWithDefault(name string, typeID TypeID, stmts []checker.Statement, defaultType TypeID) (LocalID, Block, error) {
	oldLocals := fl.cloneLocals()
	local := fl.defineLocal(name, typeID, false)
	block, err := fl.lowerBlockWithDefault(stmts, defaultType)
	fl.locals = oldLocals
	return local, block, err
}

// scopeLocals snapshots the currently visible locals and returns a restore
// function intended for `defer fl.scopeLocals()()`. It makes the caller a
// lexical scope: locals defined after the snapshot are dropped on restore, so
// block-local bindings never leak into the enclosing scope. Restoring only
// affects name visibility; the locals remain in fl.fn.Locals with stable ids.
func (fl *functionLowerer) scopeLocals() func() {
	saved := fl.cloneLocals()
	return func() { fl.locals = saved }
}

func (fl *functionLowerer) cloneLocals() map[string]LocalID {
	locals := make(map[string]LocalID, len(fl.locals))
	for name, local := range fl.locals {
		locals[name] = local
	}
	return locals
}

func resultConstructorKind(call *checker.ModuleFunctionCall) (ExprKind, bool) {
	if call.Module != "ard/result" {
		return 0, false
	}
	switch call.Call.Name {
	case "ok":
		return ExprMakeResultOk, true
	case "err":
		return ExprMakeResultErr, true
	default:
		return 0, false
	}
}

func maybeConstructorKind(call *checker.ModuleFunctionCall) (ExprKind, bool) {
	if call.Module != "ard/maybe" {
		return 0, false
	}
	switch call.Call.Name {
	case "some":
		return ExprMakeMaybeSome, true
	case "none":
		return ExprMakeMaybeNone, true
	default:
		return 0, false
	}
}

func (fl *functionLowerer) lowerArgs(args []checker.Expression) ([]Expr, error) {
	return fl.lowerArgsWithTypeIDs(args, nil)
}

func (fl *functionLowerer) lowerArgsWithSignature(args []checker.Expression, sig Signature) ([]Expr, error) {
	expected := make([]TypeID, len(sig.Params))
	for i, param := range sig.Params {
		expected[i] = param.Type
	}
	return fl.lowerArgsWithTypeIDs(args, expected)
}

func (fl *functionLowerer) lowerArgsForFunctionType(args []checker.Expression, typeID TypeID) ([]Expr, error) {
	typeInfo, ok := fl.l.typeInfo(typeID)
	if !ok || typeInfo.Kind != TypeFunction {
		return fl.lowerArgs(args)
	}
	return fl.lowerArgsWithTypeIDs(args, typeInfo.Params)
}

func (fl *functionLowerer) lowerArgsWithTypeIDs(args []checker.Expression, expected []TypeID) ([]Expr, error) {
	out := make([]Expr, len(args))
	for i, arg := range args {
		var lowered *Expr
		var err error
		if i < len(expected) && expected[i] != NoType {
			lowered, err = fl.lowerExprWithExpected(arg, expected[i])
		} else {
			lowered, err = fl.lowerExpr(arg)
		}
		if err != nil {
			return nil, err
		}
		out[i] = *lowered
	}
	return out, nil
}

func (fl *functionLowerer) lowerBinary(kind ExprKind, typeID TypeID, leftExpr, rightExpr checker.Expression) (*Expr, error) {
	left, err := fl.lowerExpr(leftExpr)
	if err != nil {
		return nil, err
	}
	right, err := fl.lowerExpr(rightExpr)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: kind, Type: typeID, Left: left, Right: right}, nil
}

func (fl *functionLowerer) lowerUnary(kind ExprKind, typeID TypeID, valueExpr checker.Expression) (*Expr, error) {
	value, err := fl.lowerExpr(valueExpr)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: kind, Type: typeID, Target: value}, nil
}

func (fl *functionLowerer) lowerTemplateStr(typeID TypeID, template *checker.TemplateStr) (*Expr, error) {
	if len(template.Chunks) == 0 {
		return &Expr{Kind: ExprConstStr, Type: typeID}, nil
	}

	current, err := fl.lowerExprWithExpected(template.Chunks[0], typeID)
	if err != nil {
		return nil, err
	}
	for i := 1; i < len(template.Chunks); i++ {
		next, err := fl.lowerExprWithExpected(template.Chunks[i], typeID)
		if err != nil {
			return nil, err
		}
		current = &Expr{Kind: ExprStrConcat, Type: typeID, Left: current, Right: next}
	}
	return current, nil
}

func loadLocal(typeID TypeID, local LocalID) *Expr {
	return &Expr{Kind: ExprLoadLocal, Type: typeID, Local: local}
}

func (fl *functionLowerer) lowerStructInstance(typeID TypeID, inst *checker.StructInstance) (*Expr, error) {
	typeInfo, ok := fl.l.typeInfo(typeID)
	if !ok || typeInfo.Kind != TypeStruct {
		return nil, fmt.Errorf("struct instance lowered with non-struct type %s", inst.Type().String())
	}
	fields := make([]StructFieldValue, 0, len(typeInfo.Fields))
	for _, field := range typeInfo.Fields {
		fieldExpr, ok := inst.Fields[field.Name]
		if !ok {
			continue
		}
		value, err := fl.lowerExprWithExpected(fieldExpr, field.Type)
		if err != nil {
			return nil, err
		}
		fields = append(fields, StructFieldValue{Index: field.Index, Name: field.Name, Value: *value})
	}
	return &Expr{Kind: ExprMakeStruct, Type: typeID, Fields: fields}, nil
}

func (fl *functionLowerer) lowerInstanceProperty(typeID TypeID, prop *checker.InstanceProperty) (*Expr, error) {
	target, err := fl.lowerExpr(prop.Subject)
	if err != nil {
		return nil, err
	}
	targetInfo, ok := fl.l.typeInfo(target.Type)
	if !ok || targetInfo.Kind != TypeStruct {
		return nil, fmt.Errorf("property access on non-struct AIR type %s", prop.Subject.Type().String())
	}
	for _, field := range targetInfo.Fields {
		if field.Name == prop.Property {
			if !validTypeID(&fl.l.program, typeID) {
				typeID = field.Type
			}
			return &Expr{Kind: ExprGetField, Type: typeID, Target: target, Field: field.Index}, nil
		}
	}
	return nil, fmt.Errorf("field %s not found on %s", prop.Property, targetInfo.Name)
}

func (fl *functionLowerer) lowerForeignFieldAssignment(prop *checker.ForeignFieldAccess, valueExpr checker.Expression) (*Stmt, error) {
	target, err := fl.lowerExpr(prop.Subject)
	if err != nil {
		return nil, err
	}
	fieldType, err := fl.internContextualCheckerType(prop.Type())
	if err != nil {
		return nil, err
	}
	value, err := fl.lowerExprWithExpected(valueExpr, fieldType)
	if err != nil {
		return nil, err
	}
	return &Stmt{Kind: StmtSetForeignField, Target: target, ForeignTarget: prop.Target, ForeignSymbol: prop.Symbol, Type: fieldType, Value: value}, nil
}

func (fl *functionLowerer) lowerForeignValueAssignment(prop *checker.ForeignValue, valueExpr checker.Expression) (*Stmt, error) {
	if !prop.Assignable {
		return nil, fmt.Errorf("assignment to non-assignable foreign value %s::%s", prop.Namespace, prop.Symbol)
	}
	valueType, err := fl.internContextualCheckerType(prop.Type())
	if err != nil {
		return nil, err
	}
	value, err := fl.lowerExprWithExpected(valueExpr, valueType)
	if err != nil {
		return nil, err
	}
	return &Stmt{Kind: StmtSetForeignValue, ForeignTarget: prop.Target, ForeignNamespace: prop.Namespace, ForeignQualifier: prop.Qualifier, ForeignSymbol: prop.Symbol, Type: valueType, Value: value}, nil
}

func (fl *functionLowerer) lowerFieldAssignment(prop *checker.InstanceProperty, valueExpr checker.Expression) (*Stmt, error) {
	target, err := fl.lowerExpr(prop.Subject)
	if err != nil {
		return nil, err
	}
	targetInfo, ok := fl.l.typeInfo(target.Type)
	if !ok || targetInfo.Kind != TypeStruct {
		return nil, fmt.Errorf("field assignment on non-struct AIR type %s", prop.Subject.Type().String())
	}
	for _, field := range targetInfo.Fields {
		if field.Name != prop.Property {
			continue
		}
		value, err := fl.lowerExprWithExpected(valueExpr, field.Type)
		if err != nil {
			return nil, err
		}
		return &Stmt{Kind: StmtSetField, Target: target, Field: field.Index, Type: field.Type, Value: value}, nil
	}
	return nil, fmt.Errorf("field %s not found on %s", prop.Property, targetInfo.Name)
}

func (fl *functionLowerer) lowerClosure(typeID TypeID, def *checker.FunctionDef) (*Expr, error) {
	keyName := fmt.Sprintf("closure/%d/%s", fl.fn.ID, def.Name)
	id, err := fl.l.declareClosureFunction(fl.fn.Module, keyName, def, typeID)
	if err != nil {
		return nil, err
	}
	// A closure created inside a generic definition is lifted to a top-level Go
	// function whose body references the enclosing type parameters, so it must
	// inherit them and be instantiated with them at its creation site (ADR 0031).
	if len(fl.fn.TypeParams) > 0 {
		fl.l.program.Functions[id].TypeParams = fl.fn.TypeParams
	}
	fn := fl.l.program.Functions[id]
	// Closure function declarations are keyed by their concrete signature and can be
	// encountered more than once while lowering methods/trait impls. Re-lowering
	// must start from declaration-only state; otherwise captures discovered during
	// the previous pass accumulate on the function while the new closure value only
	// carries captures from the current pass.
	fn.Captures = nil
	fn.Locals = nil
	fn.Body = Block{}
	child := fl.l.newFunctionLowerer(&fn, def, fl)
	child.captureByName = map[string]LocalID{}
	for _, param := range fn.Signature.Params {
		child.defineLocal(param.Name, param.Type, param.Mutable)
	}
	if def.Body != nil {
		body, err := child.lowerBlock(def.Body.Stmts)
		if err != nil {
			return nil, fmt.Errorf("lower closure %s: %w", def.Name, err)
		}
		fn.Body = body
	}
	fn.Captures = child.fn.Captures
	fn.Locals = child.fn.Locals
	fl.l.program.Functions[id] = fn

	return &Expr{Kind: ExprMakeClosure, Type: typeID, Function: id, CaptureLocals: child.captureLocals}, nil
}

func (fl *functionLowerer) lowerModuleSymbol(typeID TypeID, symbol *checker.ModuleSymbol) (*Expr, error) {
	if global, ok, err := fl.l.resolveModuleGlobal(symbol.Module, symbol.Symbol.Name); err != nil {
		return nil, err
	} else if ok {
		return &Expr{Kind: ExprLoadGlobal, Type: fl.l.program.Globals[global].Type, Global: global}, nil
	}

	if err := fl.l.ensureModuleGlobalsDeclared(symbol.Module); err != nil {
		return nil, err
	}
	def, ok := fl.l.moduleFunctionDefinitionForSymbol(symbol)
	if ok {
		module := fl.l.internModule(symbol.Module)
		if err := fl.l.ensureModuleTraitImplsDeclared(symbol.Module); err != nil {
			return nil, err
		}
		id, err := fl.l.declareAndLowerFunction(module, def)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprFunctionRef, Type: typeID, Function: id}, nil
	}

	return nil, fmt.Errorf("unsupported AIR module symbol %s::%s of type %s", symbol.Module, symbol.Symbol.Name, symbol.Type().String())
}

func (fl *functionLowerer) lowerInstanceMethod(typeID TypeID, method *checker.InstanceMethod) (*Expr, error) {
	target, err := fl.lowerExpr(method.Subject)
	if err != nil {
		return nil, err
	}
	typeInfo, ok := fl.l.typeInfo(target.Type)
	if !ok {
		return nil, fmt.Errorf("unsupported AIR instance method %s on %s", method.Method.Name, method.Subject.Type().String())
	}
	if typeInfo.Kind == TypeTraitObject {
		if err := fl.l.ensureModuleImportTraitImplsDeclared(fl.fn.Module); err != nil {
			return nil, err
		}
		if !validTraitID(&fl.l.program, typeInfo.Trait) {
			return nil, fmt.Errorf("trait object %s references invalid trait %d", typeInfo.Name, typeInfo.Trait)
		}
		trait := fl.l.program.Traits[typeInfo.Trait]
		for i, traitMethod := range trait.Methods {
			if traitMethod.Name != method.Method.Name {
				continue
			}
			args, err := fl.lowerArgsWithSignature(method.Method.Args, traitMethod.Signature)
			if err != nil {
				return nil, err
			}
			return &Expr{
				Kind:   ExprCallTrait,
				Type:   typeID,
				Target: target,
				Trait:  typeInfo.Trait,
				Method: i,
				Args:   args,
			}, nil
		}
		return nil, fmt.Errorf("trait %s has no method %s", trait.Name, method.Method.Name)
	}
	if typeInfo.Kind == TypeChannel || typeInfo.Kind == TypeReceiver || typeInfo.Kind == TypeSender {
		return fl.lowerChanMethod(typeID, target, method)
	}
	if typeInfo.Kind == TypeStruct || typeInfo.Kind == TypeEnum {
		return fl.lowerUserInstanceMethod(typeID, target, typeInfo, method)
	}
	return nil, fmt.Errorf("unsupported AIR instance method %s on %s", method.Method.Name, method.Subject.Type().String())
}

// lowerChanMethod lowers the Chan<T> methods (send/recv/close) to the native
// channel AIR expressions, with the channel receiver as Args[0].
func (fl *functionLowerer) lowerChanMethod(typeID TypeID, target *Expr, method *checker.InstanceMethod) (*Expr, error) {
	switch method.Method.Name {
	case "send":
		if len(method.Method.Args) != 1 {
			return nil, fmt.Errorf("Chan.send expects one argument")
		}
		// Lower the sent value against the channel's element type so contextual
		// expressions (empty literals, union wrapping, maybe/none, any) lower
		// correctly.
		value, err := fl.lowerChannelValue(target.Type, method.Method.Args[0])
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprChannelSend, Type: typeID, Args: []Expr{*target, *value}}, nil
	case "recv":
		return &Expr{Kind: ExprChannelRecv, Type: typeID, Args: []Expr{*target}}, nil
	case "close":
		return &Expr{Kind: ExprChannelClose, Type: typeID, Args: []Expr{*target}}, nil
	case "receiver", "sender":
		return &Expr{Kind: ExprChannelNarrow, Type: typeID, Args: []Expr{*target}}, nil
	}
	return nil, fmt.Errorf("unknown Chan method %s", method.Method.Name)
}

func (fl *functionLowerer) lowerUserInstanceMethod(typeID TypeID, target *Expr, typeInfo TypeInfo, method *checker.InstanceMethod) (*Expr, error) {
	def := method.Method.Definition()
	if def == nil || def.Body == nil {
		if expr, ok, err := fl.lowerStaticTraitMethod(typeID, target, method); ok || err != nil {
			return expr, err
		}
		return nil, fmt.Errorf("unsupported AIR instance method %s on %s", method.Method.Name, method.Subject.Type().String())
	}
	return fl.lowerUserDefinedInstanceMethod(typeID, target, typeInfo, method, def)
}

func (fl *functionLowerer) lowerStaticTraitMethod(typeID TypeID, target *Expr, method *checker.InstanceMethod) (*Expr, bool, error) {
	module := fl.l.moduleForInstanceMethod(method, fl.fn.Module)
	if module >= 0 && int(module) < len(fl.l.program.Modules) {
		path := fl.l.program.Modules[module].Path
		if mod, ok := fl.l.moduleByName[path]; ok && !fl.l.loweredModules[path] {
			if err := fl.l.lowerModule(mod); err != nil {
				return nil, false, err
			}
		}
	}

	for _, impl := range fl.l.program.Impls {
		if impl.ForType != target.Type {
			continue
		}
		if !validTraitID(&fl.l.program, impl.Trait) {
			return nil, false, fmt.Errorf("impl %d references invalid trait %d", impl.ID, impl.Trait)
		}
		trait := fl.l.program.Traits[impl.Trait]
		for i, traitMethod := range trait.Methods {
			if traitMethod.Name != method.Method.Name {
				continue
			}
			if i >= len(impl.Methods) {
				return nil, false, fmt.Errorf("impl %d missing method %d for trait %s", impl.ID, i, trait.Name)
			}
			id := impl.Methods[i]
			if !validFunctionID(&fl.l.program, id) {
				return nil, false, fmt.Errorf("impl %d method %d has invalid function id %d", impl.ID, i, id)
			}
			fn := fl.l.program.Functions[id]
			args := make([]Expr, 0, len(method.Method.Args)+1)
			args = append(args, *target)
			loweredArgs, err := fl.lowerArgsWithSignature(method.Method.Args, Signature{Params: fn.Signature.Params[1:], Return: fn.Signature.Return})
			if err != nil {
				return nil, false, err
			}
			args = append(args, loweredArgs...)
			return &Expr{Kind: ExprCall, Type: typeID, Function: id, Args: args}, true, nil
		}
	}
	return nil, false, nil
}

func (fl *functionLowerer) lowerUserDefinedInstanceMethod(typeID TypeID, target *Expr, typeInfo TypeInfo, method *checker.InstanceMethod, def *checker.FunctionDef) (*Expr, error) {
	if def == nil || def.Body == nil {
		return nil, fmt.Errorf("unsupported AIR instance method %s on %s", method.Method.Name, method.Subject.Type().String())
	}
	module := fl.l.moduleForInstanceMethod(method, fl.fn.Module)
	if module >= 0 && int(module) < len(fl.l.program.Modules) {
		fl.l.program.Modules[module].Types = appendUniqueType(fl.l.program.Modules[module].Types, target.Type)
		if err := fl.l.ensureModuleGlobalsDeclared(fl.l.program.Modules[module].Path); err != nil {
			return nil, err
		}
	}
	// A method on a generic struct lowers once as a generic method definition
	// (ADR 0031) when it uses the struct's type parameters. Methods cannot
	// introduce their own generic parameters, because Go method receivers cannot
	// express them. The call references the definition with the receiver's
	// concrete type arguments and lowers its arguments against their concrete
	// types.
	if typeInfo.Generic != NoType && methodUsesOnlyStructTypeParams(def, fl.l.program.Types[typeInfo.Generic-1].TypeParams) {
		id, typeArgs, err := fl.declareGenericInstanceMethodFunction(module, target.Type, method.StructType, def)
		if err != nil {
			return nil, err
		}
		args := make([]Expr, 0, len(method.Method.Args)+1)
		args = append(args, *target)
		concreteParams := make([]Param, len(method.Method.Args))
		for i, arg := range method.Method.Args {
			tid, err := fl.internType(arg.Type())
			if err != nil {
				return nil, err
			}
			concreteParams[i] = Param{Type: tid}
		}
		loweredArgs, err := fl.lowerArgsWithSignature(method.Method.Args, Signature{Params: concreteParams, Return: typeID})
		if err != nil {
			return nil, err
		}
		args = append(args, loweredArgs...)
		return &Expr{Kind: ExprCall, Type: typeID, Function: id, Args: args, TypeArgs: typeArgs}, nil
	}
	id, err := fl.declareInstanceMethodFunction(module, typeInfo.Name, target.Type, def, method.Method.Args, typeID)
	if err != nil {
		return nil, err
	}
	if err := fl.l.lowerInstanceMethodFunction(id, def); err != nil {
		return nil, err
	}
	fn := fl.l.program.Functions[id]
	args := make([]Expr, 0, len(method.Method.Args)+1)
	args = append(args, *target)
	loweredArgs, err := fl.lowerArgsWithSignature(method.Method.Args, Signature{Params: fn.Signature.Params[1:], Return: fn.Signature.Return})
	if err != nil {
		return nil, err
	}
	args = append(args, loweredArgs...)
	return &Expr{Kind: ExprCall, Type: typeID, Function: id, Args: args}, nil
}

func (fl *functionLowerer) defineLocal(name string, typeID TypeID, mutable bool) LocalID {
	id := LocalID(len(fl.fn.Locals))
	fl.fn.Locals = append(fl.fn.Locals, Local{ID: id, Name: name, Type: typeID, Mutable: mutable})
	fl.locals[name] = id
	return id
}

func (fl *functionLowerer) resolveLocal(name string) (LocalID, bool, error) {
	if local, ok := fl.locals[name]; ok {
		return local, true, nil
	}
	if fl.parent == nil {
		return 0, false, nil
	}
	local, _, _, ok, err := fl.captureLocal(name)
	return local, ok, err
}

func (fl *functionLowerer) ensureLocalForNestedCapture(name string) (LocalID, TypeID, bool, error) {
	local, typeID, _, ok, err := fl.ensureLocalForNestedCaptureWithMutability(name)
	return local, typeID, ok, err
}

func (fl *functionLowerer) ensureLocalForNestedCaptureWithMutability(name string) (LocalID, TypeID, bool, bool, error) {
	if local, ok := fl.locals[name]; ok {
		return local, fl.fn.Locals[local].Type, fl.fn.Locals[local].Mutable, true, nil
	}
	if fl.parent == nil {
		return 0, NoType, false, false, nil
	}
	return fl.captureLocal(name)
}

func (fl *functionLowerer) captureLocal(name string) (LocalID, TypeID, bool, bool, error) {
	if fl.captureByName == nil {
		fl.captureByName = map[string]LocalID{}
	}
	if local, ok := fl.captureByName[name]; ok {
		return local, fl.fn.Locals[local].Type, fl.fn.Locals[local].Mutable, true, nil
	}
	sourceLocal, typeID, mutable, ok, err := fl.parent.ensureLocalForNestedCaptureWithMutability(name)
	if err != nil || !ok {
		return 0, NoType, false, ok, err
	}
	if int(sourceLocal) >= 0 && int(sourceLocal) < len(fl.parent.fn.Locals) && fl.parent.fn.Locals[sourceLocal].Reference {
		return 0, NoType, false, false, fmt.Errorf("closures cannot capture %q: capturing a mut reference from a Go call is not supported yet", name)
	}
	local := fl.defineLocal(name, typeID, mutable)
	fl.captureByName[name] = local
	fl.captureLocals = append(fl.captureLocals, sourceLocal)
	fl.fn.Captures = append(fl.fn.Captures, Capture{Name: name, Type: typeID, Local: local})
	return local, typeID, mutable, true, nil
}

func (fl *functionLowerer) localKind(local LocalID) TypeKind {
	if int(local) < 0 || int(local) >= len(fl.fn.Locals) {
		return TypeVoid
	}
	info, ok := fl.l.typeInfo(fl.fn.Locals[local].Type)
	if !ok {
		return TypeVoid
	}
	return info.Kind
}

func (l *lowerer) lookupFunction(name string) (FunctionID, bool) {
	for key, id := range l.functions {
		if keyHasFunctionName(key, name) {
			return id, true
		}
	}
	return NoFunction, false
}

func (l *lowerer) lookupGlobalInModule(module ModuleID, name string) (GlobalID, bool) {
	id, ok := l.globals[globalKey(module, name)]
	return id, ok
}

func (l *lowerer) lookupFunctionInModule(modulePath, name string) (FunctionID, bool) {
	moduleID, ok := l.moduleByPath[modulePath]
	if !ok {
		return NoFunction, false
	}
	id, ok := l.functions[functionKey(moduleID, name)]
	return id, ok
}

func (l *lowerer) ensureModuleImportTraitImplsDeclared(moduleID ModuleID) error {
	if moduleID < 0 || int(moduleID) >= len(l.program.Modules) {
		return nil
	}
	mod, ok := l.moduleByName[l.program.Modules[moduleID].Path]
	if !ok || mod.Program() == nil {
		return nil
	}
	for _, imported := range mod.Program().Imports {
		importedID := l.internModule(imported.Path())
		l.program.Modules[moduleID].Imports = appendUniqueModule(l.program.Modules[moduleID].Imports, importedID)
		if err := l.ensureModuleTraitImplsDeclaredRecursive(imported.Path(), map[string]bool{}); err != nil {
			return err
		}
	}
	return nil
}

func (l *lowerer) ensureModuleTraitImplsDeclaredRecursive(modulePath string, seen map[string]bool) error {
	if seen[modulePath] {
		return nil
	}
	seen[modulePath] = true
	if err := l.ensureModuleTraitImplsDeclared(modulePath); err != nil {
		return err
	}
	mod, ok := l.moduleByName[modulePath]
	if !ok || mod.Program() == nil {
		return nil
	}
	for _, imported := range mod.Program().Imports {
		if err := l.ensureModuleTraitImplsDeclaredRecursive(imported.Path(), seen); err != nil {
			return err
		}
	}
	return nil
}

func (l *lowerer) ensureModuleTraitImplsDeclared(modulePath string) error {
	if err := l.ensureModuleGlobalsDeclared(modulePath); err != nil {
		return err
	}
	if err := l.ensureModuleTypesDeclared(modulePath); err != nil {
		return err
	}
	mod, ok := l.moduleByName[modulePath]
	if !ok || mod.Program() == nil {
		return nil
	}
	modID := l.internModule(modulePath)
	prog := mod.Program()
	for _, stmt := range prog.Statements {
		switch node := stmt.Stmt.(type) {
		case *checker.StructDef:
			if typeHasUnresolvedTypeVar(node) {
				continue
			}
			if err := l.declareTraitImplsForType(modID, node); err != nil {
				return err
			}
			if err := l.declareInherentImplMethodsForStruct(modID, node); err != nil {
				return err
			}
		case *checker.Enum:
			if err := l.declareTraitImplsForType(modID, node); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *lowerer) ensureModuleTypesDeclared(modulePath string) error {
	mod, ok := l.moduleByName[modulePath]
	if !ok || mod.Program() == nil {
		return nil
	}
	modID := l.internModule(modulePath)
	prog := mod.Program()
	for _, imported := range prog.Imports {
		l.moduleByName[imported.Path()] = imported
		importedID := l.internModule(imported.Path())
		l.program.Modules[modID].Imports = appendUniqueModule(l.program.Modules[modID].Imports, importedID)
	}
	for _, stmt := range prog.Statements {
		switch node := stmt.Stmt.(type) {
		case *checker.StructDef:
			if typeHasUnresolvedTypeVar(node) {
				continue
			}
			typeID, err := l.internType(node)
			if err != nil {
				return err
			}
			l.program.Modules[modID].Types = appendUniqueType(l.program.Modules[modID].Types, typeID)
		case *checker.Enum:
			typeID, err := l.internType(node)
			if err != nil {
				return err
			}
			l.program.Modules[modID].Types = appendUniqueType(l.program.Modules[modID].Types, typeID)
		case *checker.Union:
			if typeHasUnresolvedTypeVar(node) {
				continue
			}
			typeID, err := l.internType(node)
			if err != nil {
				return err
			}
			l.program.Modules[modID].Types = appendUniqueType(l.program.Modules[modID].Types, typeID)
		}
	}
	return nil
}

func (l *lowerer) ensureModuleGlobalsDeclared(modulePath string) error {
	mod, ok := l.moduleByName[modulePath]
	if !ok || mod.Program() == nil {
		return nil
	}
	modID := l.internModule(modulePath)
	prog := mod.Program()
	for _, imported := range prog.Imports {
		l.moduleByName[imported.Path()] = imported
		importedID := l.internModule(imported.Path())
		l.program.Modules[modID].Imports = appendUniqueModule(l.program.Modules[modID].Imports, importedID)
	}
	for _, stmt := range prog.Statements {
		def, ok := stmt.Stmt.(*checker.VariableDef)
		if !ok || def.Mutable {
			continue
		}
		if _, err := l.declareGlobal(modID, def); err != nil {
			return err
		}
	}
	return nil
}

func (l *lowerer) lowerAllModuleGlobals() error {
	for i := 0; i < len(l.program.Modules); i++ {
		module := l.program.Modules[i]
		if err := l.lowerModuleGlobals(module.Path); err != nil {
			return err
		}
	}
	return nil
}

func (l *lowerer) lowerModuleGlobals(modulePath string) error {
	if err := l.ensureModuleGlobalsDeclared(modulePath); err != nil {
		return err
	}
	mod, ok := l.moduleByName[modulePath]
	if !ok || mod.Program() == nil {
		return nil
	}
	modID := l.internModule(modulePath)
	for _, stmt := range mod.Program().Statements {
		def, ok := stmt.Stmt.(*checker.VariableDef)
		if !ok || def.Mutable {
			continue
		}
		if err := l.lowerGlobal(modID, def); err != nil {
			return fmt.Errorf("lower global %s: %w", def.Name, err)
		}
	}
	return nil
}

func (l *lowerer) resolveModuleGlobal(modulePath, name string) (GlobalID, bool, error) {
	moduleID, ok := l.moduleByPath[modulePath]
	if ok {
		if id, ok := l.lookupGlobalInModule(moduleID, name); ok {
			return id, true, nil
		}
	}

	if err := l.ensureModuleGlobalsDeclared(modulePath); err != nil {
		return NoGlobal, false, err
	}
	moduleID, ok = l.moduleByPath[modulePath]
	if !ok {
		return NoGlobal, false, nil
	}
	id, ok := l.lookupGlobalInModule(moduleID, name)
	return id, ok, nil
}

func (l *lowerer) resolveModuleFunction(modulePath, name string) (FunctionID, bool, error) {
	if id, ok := l.lookupFunctionInModule(modulePath, name); ok {
		return id, true, nil
	}

	mod, ok := l.moduleByName[modulePath]
	if !ok {
		return NoFunction, false, nil
	}
	if mod.Program() == nil {
		return NoFunction, false, nil
	}
	if err := l.lowerModule(mod); err != nil {
		return NoFunction, false, err
	}
	id, ok := l.lookupFunctionInModule(modulePath, name)
	return id, ok, nil
}

func (l *lowerer) moduleFunctionDefinitionForCall(call *checker.ModuleFunctionCall) *checker.FunctionDef {
	if call == nil || call.Call == nil {
		return nil
	}
	def := call.Call.Definition()
	if def == nil {
		return nil
	}
	if def.Body != nil {
		return def
	}
	bodyDef := l.lookupModuleFunctionDefinition(call.Module, call.Call.Name)
	if bodyDef == nil || bodyDef.Body == nil {
		return def
	}
	return &checker.FunctionDef{
		Name:                    def.Name,
		Receiver:                bodyDef.Receiver,
		Parameters:              def.Parameters,
		ReturnType:              def.ReturnType,
		InferReturnTypeFromBody: bodyDef.InferReturnTypeFromBody,
		Mutates:                 bodyDef.Mutates,
		IsTest:                  bodyDef.IsTest,
		Body:                    bodyDef.Body,
		Private:                 bodyDef.Private,
	}
}

func (l *lowerer) moduleFunctionDefinitionForSymbol(symbol *checker.ModuleSymbol) (*checker.FunctionDef, bool) {
	if symbol == nil {
		return nil, false
	}
	def, ok := symbol.Symbol.Type.(*checker.FunctionDef)
	if !ok {
		return nil, false
	}
	bodyDef := l.lookupModuleFunctionDefinition(symbol.Module, symbol.Symbol.Name)
	if bodyDef == nil || bodyDef.Body == nil || def.Body != nil {
		return def, true
	}
	return &checker.FunctionDef{
		Name:                    def.Name,
		Receiver:                bodyDef.Receiver,
		Parameters:              def.Parameters,
		ReturnType:              def.ReturnType,
		InferReturnTypeFromBody: bodyDef.InferReturnTypeFromBody,
		Mutates:                 bodyDef.Mutates,
		IsTest:                  bodyDef.IsTest,
		Body:                    bodyDef.Body,
		Private:                 bodyDef.Private,
	}, true
}

func (l *lowerer) lookupModuleFunctionDefinition(modulePath, name string) *checker.FunctionDef {
	mod, ok := l.moduleByName[modulePath]
	if !ok || mod.Program() == nil {
		return nil
	}
	for _, stmt := range mod.Program().Statements {
		if def, ok := stmt.Expr.(*checker.FunctionDef); ok && def.Name == name {
			return def
		}
	}
	return nil
}

func (l *lowerer) moduleForInstanceMethod(method *checker.InstanceMethod, fallback ModuleID) ModuleID {
	if method == nil || method.Method == nil {
		return fallback
	}
	ownerName := ""
	ownerModulePath := ""
	switch {
	case method.StructType != nil:
		ownerName = method.StructType.Name
		ownerModulePath = method.StructType.ModulePath
	case method.EnumType != nil:
		ownerName = method.EnumType.Name
		ownerModulePath = method.EnumType.ModulePath
	default:
		return fallback
	}
	if ownerModulePath != "" {
		l.findReachableModule(ownerModulePath)
		return l.internModule(ownerModulePath)
	}
	for modulePath, mod := range l.moduleByName {
		if mod.Program() == nil {
			continue
		}
		for _, stmt := range mod.Program().Statements {
			switch def := stmt.Stmt.(type) {
			case *checker.StructDef:
				if def.Name == ownerName && l.hasStructMethod(def, method.Method.Name) {
					return l.internModule(modulePath)
				}
			case *checker.Enum:
				if def.Name == ownerName && def.Methods[method.Method.Name] != nil {
					return l.internModule(modulePath)
				}
			}
		}
	}
	return fallback
}

func (l *lowerer) typeInfo(id TypeID) (TypeInfo, bool) {
	if id <= 0 || int(id) > len(l.program.Types) {
		return TypeInfo{}, false
	}
	return l.program.Types[id-1], true
}

func (l *lowerer) lookupImpl(trait TraitID, forType TypeID) (ImplID, bool) {
	for _, impl := range l.program.Impls {
		if impl.Trait == trait && impl.ForType == forType {
			return impl.ID, true
		}
	}
	return 0, false
}

func topLevelExecutableStatements(stmts []checker.Statement) []checker.Statement {
	filtered := make([]checker.Statement, 0, len(stmts))
	for _, stmt := range stmts {
		switch stmt.Expr.(type) {
		case *checker.FunctionDef:
			continue
		}
		if stmt.Stmt != nil {
			switch stmt.Stmt.(type) {
			case *checker.StructDef, *checker.Enum, *checker.Union:
				continue
			case *checker.VariableDef:
				// Module-level variables (both let and mut) are AIR globals.
				continue
			}
		}
		filtered = append(filtered, stmt)
	}
	return filtered
}

func sortedFieldNames(fields map[string]checker.Type) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func appendUniqueType(items []TypeID, id TypeID) []TypeID {
	for _, item := range items {
		if item == id {
			return items
		}
	}
	return append(items, id)
}

func appendUniqueGlobal(items []GlobalID, id GlobalID) []GlobalID {
	for _, item := range items {
		if item == id {
			return items
		}
	}
	return append(items, id)
}

func appendUniqueFunction(items []FunctionID, id FunctionID) []FunctionID {
	for _, item := range items {
		if item == id {
			return items
		}
	}
	return append(items, id)
}

func appendUniqueModule(items []ModuleID, id ModuleID) []ModuleID {
	for _, item := range items {
		if item == id {
			return items
		}
	}
	return append(items, id)
}

func (l *lowerer) setFunctionTypeVars(id FunctionID, typeVars map[string]TypeID) {
	if len(typeVars) == 0 {
		return
	}
	if l.functionTypeVars[id] == nil {
		l.functionTypeVars[id] = make(map[string]TypeID, len(typeVars))
	}
	for name, typeID := range typeVars {
		l.functionTypeVars[id][name] = typeID
	}
}

func functionKey(module ModuleID, name string) string {
	return fmt.Sprintf("%d:%s", module, name)
}

func globalKey(module ModuleID, name string) string {
	return fmt.Sprintf("%d:global:%s", module, name)
}

func concreteFunctionKey(module ModuleID, name string, signature Signature, genericKey string) string {
	if genericKey == "" {
		return fmt.Sprintf("%d:%s:%s", module, name, signatureKey(signature))
	}
	return fmt.Sprintf("%d:%s:%s:%s", module, name, signatureKey(signature), genericKey)
}

func (l *lowerer) genericBindingsKey(def *checker.FunctionDef) (string, error) {
	key, _, err := l.genericBindingsKeyWithInterner(def, l.internType)
	return key, err
}

func (fl *functionLowerer) genericBindingsKey(def *checker.FunctionDef) (string, error) {
	key, _, err := fl.genericBindingsKeyAndTypeVars(def)
	return key, err
}

func (fl *functionLowerer) genericBindingsKeyAndTypeVars(def *checker.FunctionDef) (string, map[string]TypeID, error) {
	return fl.l.genericBindingsKeyWithInterner(def, fl.internResolvedType)
}

// genericCallTypeArgs returns the concrete type arguments for a call to a
// generic function, ordered by the definition's generic parameters.
func (fl *functionLowerer) genericCallTypeArgs(def *checker.FunctionDef) ([]TypeID, error) {
	paramNames := genericParamNames(def)
	if len(paramNames) == 0 {
		return nil, nil
	}
	args := make([]TypeID, len(paramNames))
	for i, p := range paramNames {
		binding, ok := def.GenericBindings[p]
		if !ok {
			return nil, fmt.Errorf("missing generic binding for %s in call to %s", p, def.Name)
		}
		id, err := fl.internResolvedType(binding)
		if err != nil {
			return nil, err
		}
		args[i] = id
	}
	return args, nil
}

// buildResolvedCallExpr builds an ExprCall for a resolved function definition.
// For a generic definition the call carries concrete type arguments and is
// typed/argument-lowered against the call's concrete signature, while the
// referenced function remains the single generic definition.
func (fl *functionLowerer) buildResolvedCallExpr(id FunctionID, def *checker.FunctionDef, call *checker.FunctionCall, callArgs []checker.Expression) (*Expr, error) {
	signature := fl.l.program.Functions[id].Signature
	var typeArgs []TypeID
	if len(fl.l.program.Functions[id].TypeParams) > 0 {
		concrete, err := fl.signatureForCall(call)
		if err != nil {
			return nil, err
		}
		signature = concrete
		typeArgs, err = fl.genericCallTypeArgs(def)
		if err != nil {
			return nil, err
		}
	}
	args, err := fl.lowerArgsWithSignature(callArgs, signature)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: ExprCall, Type: signature.Return, Function: id, Args: args, TypeArgs: typeArgs}, nil
}

func (l *lowerer) genericBindingsKeyWithInterner(def *checker.FunctionDef, intern func(checker.Type) (TypeID, error)) (string, map[string]TypeID, error) {
	if def == nil {
		return "", nil, nil
	}
	for _, param := range def.GenericParams {
		if _, ok := def.GenericBindings[param]; !ok {
			return "", nil, fmt.Errorf("cannot declare unspecialized generic function %s", def.Name)
		}
	}
	if len(def.GenericBindings) == 0 {
		return "", nil, nil
	}
	keys := make([]string, 0, len(def.GenericBindings))
	for key := range def.GenericBindings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	bindingIDs := make([]TypeID, 0, len(keys))
	typeVars := make(map[string]TypeID, len(keys))
	for _, key := range keys {
		typeID, err := intern(def.GenericBindings[key])
		if err != nil {
			return "", nil, err
		}
		bindingIDs = append(bindingIDs, typeID)
		typeVars[key] = typeID
	}
	return typeIDsKey(bindingIDs), typeVars, nil
}

func typeIDsKey(typeIDs []TypeID) string {
	if len(typeIDs) == 0 {
		return "<>"
	}
	key := "<"
	for i, typeID := range typeIDs {
		if i > 0 {
			key += ","
		}
		key += fmt.Sprintf("%d", typeID)
	}
	key += ">"
	return key
}

func typeIDsEqual(left, right []TypeID) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func signatureKey(signature Signature) string {
	key := "("
	for i, param := range signature.Params {
		if i > 0 {
			key += ","
		}
		mut := ""
		if param.Mutable {
			mut = "mut "
		}
		key += fmt.Sprintf("%s%d", mut, param.Type)
	}
	key += fmt.Sprintf(")->%d", signature.Return)
	return key
}

func checkerTraitKey(trait *checker.Trait) string {
	if trait == nil {
		return "<nil>"
	}
	return trait.ModulePath + "::" + trait.Name
}

func implKey(module ModuleID, traitName, typeName string) string {
	return fmt.Sprintf("%d:%s:%s", module, traitName, typeName)
}

func methodFunctionKey(module ModuleID, typeName, traitName, methodName string) string {
	return functionKey(module, fmt.Sprintf("method/%s/%s/%s", typeName, traitName, methodName))
}

func keyHasFunctionName(key, name string) bool {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == ':' {
			return key[i+1:] == name
		}
	}
	return key == name
}

// isMutableReferenceProducer reports whether an expression yields live mutable
// storage represented as a Go pointer: a generic Go function call whose
// instantiated result is `mut T` for an Ard-owned type. Foreign named types
// carry pointer-ness in the type itself and do not need the local flag.
func isMutableReferenceProducer(expr checker.Expression) bool {
	call, ok := expr.(*checker.ForeignFunctionCall)
	return ok && call.PointerResult
}
