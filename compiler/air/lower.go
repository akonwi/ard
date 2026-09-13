package air

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
	// A single root has one complete, immutable import graph, so owner lookups
	// remain valid as its transitive modules are registered in moduleByName.
	// Multiple roots may contain independent graphs with competing method tables;
	// preserve their existing uncached lookup behavior rather than sharing entries.
	l := newLowerer(options, len(modules))
	for _, module := range modules {
		if err := l.lowerModule(module); err != nil {
			return nil, err
		}
	}
	if err := l.lowerAllModuleGlobals(); err != nil {
		return nil, err
	}
	if err := l.typeInterner.validateComplete(); err != nil {
		return nil, err
	}
	if err := Validate(&l.program); err != nil {
		return nil, err
	}
	// Detach the result from the lowerer allocation so lowering-only caches and
	// checker graph references can be collected while the backend uses the AIR.
	program := l.program
	return &program, nil
}

type lowerer struct {
	program Program

	moduleByPath  map[string]ModuleID
	moduleByName  map[string]checker.Module
	typeInterner  *typeInterner
	traits        map[string]TraitID
	impls         map[string]ImplID
	functions     map[string]FunctionID
	globals       map[string]GlobalID
	embeddedBlobs map[string]EmbeddedBlobID
	embeddedSets  map[string]EmbeddedSetID

	cacheMethodLookups       bool
	structMethodsByOwner     map[checker.MethodOwner]map[string]*checker.FunctionDef
	traitMethodsByOwner      map[checker.TraitMethodOwner]map[string]*checker.FunctionDef
	requiredGoMethodsByOwner map[checker.MethodOwner]map[string]*checker.FunctionDef
	inherentMethodsByOwner   map[checker.MethodOwner]map[string]*checker.FunctionDef
	unresolvedTypeVarByType  map[checker.Type]bool

	loweringModules     map[string]bool
	loweredModules      map[string]bool
	loweringFuncs       map[FunctionID]bool
	loweredFuncs        map[FunctionID]bool
	loweringGlobals     map[GlobalID]bool
	loweredGlobals      map[GlobalID]bool
	functionTypeVars    map[FunctionID]map[string]TypeID
	genericStructDefs   map[string]TypeID
	genericFunctionDefs map[string]FunctionID
	genericMethodDefs   map[string]FunctionID
	localNamedFunctions map[*checker.FunctionDef]FunctionID
	defParams           map[string]int
	defParamOwner       string
	includeTests        bool
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

func newLowerer(options LowerOptions, rootCount int) *lowerer {
	l := &lowerer{
		program: Program{
			Entry:  NoFunction,
			Script: NoFunction,
		},
		moduleByPath:  map[string]ModuleID{},
		moduleByName:  map[string]checker.Module{},
		traits:        map[string]TraitID{},
		impls:         map[string]ImplID{},
		functions:     map[string]FunctionID{},
		globals:       map[string]GlobalID{},
		embeddedBlobs: map[string]EmbeddedBlobID{},
		embeddedSets:  map[string]EmbeddedSetID{},

		cacheMethodLookups:      rootCount == 1,
		unresolvedTypeVarByType: map[checker.Type]bool{},

		loweringModules:     map[string]bool{},
		loweredModules:      map[string]bool{},
		loweringFuncs:       map[FunctionID]bool{},
		loweredFuncs:        map[FunctionID]bool{},
		loweringGlobals:     map[GlobalID]bool{},
		loweredGlobals:      map[GlobalID]bool{},
		functionTypeVars:    map[FunctionID]map[string]TypeID{},
		genericStructDefs:   map[string]TypeID{},
		genericFunctionDefs: map[string]FunctionID{},
		genericMethodDefs:   map[string]FunctionID{},
		localNamedFunctions: map[*checker.FunctionDef]FunctionID{},
		includeTests:        options.IncludeTests,
	}
	l.typeInterner = newTypeInterner(&l.program)
	if l.cacheMethodLookups {
		l.structMethodsByOwner = map[checker.MethodOwner]map[string]*checker.FunctionDef{}
		l.traitMethodsByOwner = map[checker.TraitMethodOwner]map[string]*checker.FunctionDef{}
		l.requiredGoMethodsByOwner = map[checker.MethodOwner]map[string]*checker.FunctionDef{}
		l.inherentMethodsByOwner = map[checker.MethodOwner]map[string]*checker.FunctionDef{}
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

func (l *lowerer) typeHasUnresolvedTypeVar(t checker.Type) bool {
	if unresolved, ok := l.unresolvedTypeVarByType[t]; ok {
		return unresolved
	}
	unresolved := typeHasUnresolvedTypeVar(t)
	l.unresolvedTypeVarByType[t] = unresolved
	return unresolved
}

func (l *lowerer) functionHasUnresolvedTypeVar(def *checker.FunctionDef) bool {
	if def == nil {
		return false
	}
	for _, param := range def.GenericParams {
		if _, ok := def.GenericBindings[param]; !ok {
			return true
		}
	}
	return l.typeHasUnresolvedTypeVar(def)
}

func (l *lowerer) internEmbeddedBlob(data []byte, direct bool) (EmbeddedBlobID, error) {
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if id, ok := l.embeddedBlobs[digest]; ok {
		if !bytes.Equal(l.program.EmbeddedBlobs[id].Data, data) {
			return 0, fmt.Errorf("embedded resource digest collision for %s", digest)
		}
		if direct {
			l.program.EmbeddedBlobs[id].Direct = true
		}
		return id, nil
	}
	id := EmbeddedBlobID(len(l.program.EmbeddedBlobs))
	l.program.EmbeddedBlobs = append(l.program.EmbeddedBlobs, EmbeddedBlob{
		ID:     id,
		Data:   append([]byte(nil), data...),
		Digest: digest,
		Direct: direct,
	})
	l.embeddedBlobs[digest] = id
	return id, nil
}

func (l *lowerer) internEmbeddedSet(set checker.EmbeddedFileSet) (EmbeddedSetID, error) {
	entries := make([]EmbeddedEntry, len(set.Entries))
	hash := sha256.New()
	_, _ = hash.Write([]byte(set.OwnerPackageIdentity))
	_, _ = hash.Write([]byte{0})
	for index, entry := range set.Entries {
		blob, err := l.internEmbeddedBlob(entry.Data, false)
		if err != nil {
			return 0, err
		}
		entries[index] = EmbeddedEntry{Path: entry.LogicalPath, Blob: blob}
		_, _ = hash.Write([]byte(entry.LogicalPath))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(l.program.EmbeddedBlobs[blob].Digest))
		_, _ = hash.Write([]byte{0})
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if id, ok := l.embeddedSets[digest]; ok {
		return id, nil
	}
	id := EmbeddedSetID(len(l.program.EmbeddedSets))
	l.program.EmbeddedSets = append(l.program.EmbeddedSets, EmbeddedSet{
		ID:                   id,
		OwnerPackageIdentity: set.OwnerPackageIdentity,
		Entries:              entries,
		Digest:               digest,
	})
	l.embeddedSets[digest] = id
	return id, nil
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
	owner := checker.StructMethodOwner(def)
	if !l.cacheMethodLookups {
		return checker.StructMethodsInModules(l.moduleByName, owner)
	}
	if methods, ok := l.structMethodsByOwner[owner]; ok {
		return methods
	}
	methods := checker.StructMethodsInModules(l.moduleByName, owner)
	l.structMethodsByOwner[owner] = methods
	return methods
}

func (l *lowerer) traitMethods(typ checker.Type, trait *checker.Trait) map[string]*checker.FunctionDef {
	owner, ok := checker.MethodOwnerForType(typ)
	if !ok || trait == nil {
		return nil
	}
	if !l.cacheMethodLookups {
		return checker.TraitMethodsInModules(l.moduleByName, owner, trait)
	}
	key := checker.TraitMethodOwner{
		MethodOwner:     owner,
		TraitModulePath: trait.ModulePath,
		TraitName:       trait.Name,
	}
	if methods, ok := l.traitMethodsByOwner[key]; ok {
		return methods
	}
	methods := checker.TraitMethodsInModules(l.moduleByName, owner, trait)
	l.traitMethodsByOwner[key] = methods
	return methods
}

func (l *lowerer) requiredGoMethods(def *checker.StructDef) map[string]*checker.FunctionDef {
	if def == nil {
		return nil
	}
	owner := checker.StructMethodOwner(def)
	if !l.cacheMethodLookups {
		return checker.RequiredGoMethodsInModules(l.moduleByName, owner)
	}
	if methods, ok := l.requiredGoMethodsByOwner[owner]; ok {
		return methods
	}
	methods := checker.RequiredGoMethodsInModules(l.moduleByName, owner)
	l.requiredGoMethodsByOwner[owner] = methods
	return methods
}

func (l *lowerer) inherentMethods(def *checker.StructDef) map[string]*checker.FunctionDef {
	if def == nil {
		return nil
	}
	owner := checker.StructMethodOwner(def)
	if !l.cacheMethodLookups {
		return checker.InherentMethodsInModules(l.moduleByName, owner)
	}
	if methods, ok := l.inherentMethodsByOwner[owner]; ok {
		return methods
	}
	methods := checker.InherentMethodsInModules(l.moduleByName, owner)
	l.inherentMethodsByOwner[owner] = methods
	return methods
}

func (l *lowerer) findReachableModule(path string) checker.Module {
	if mod, ok := l.moduleByName[path]; ok {
		return mod
	}
	for _, mod := range sortedModules(l.moduleByName) {
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
	for _, imported := range sortedModules(program.Imports) {
		if found := findReachableModuleSeen(imported, path, seen); found != nil {
			return found
		}
	}
	return nil
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

	for _, imported := range sortedModules(prog.Imports) {
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
			if l.typeHasUnresolvedTypeVar(node) {
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
			if l.typeHasUnresolvedTypeVar(node) {
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
			if l.functionHasUnresolvedTypeVar(expr) || (!l.includeTests && expr.IsTest) {
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
			if len(node.GenericParams) > 0 {
				if err := l.declareGenericStructMethodsAndTraitImpls(modID, node); err != nil {
					return err
				}
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
			if l.functionHasUnresolvedTypeVar(def) || (!l.includeTests && def.IsTest) {
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
	value, actualType, err := fl.lowerContextualExpr(def.Value, global.Type)
	if err != nil {
		return err
	}
	global.Type = actualType
	global.Initializer = GlobalInitializer{
		Locals: append([]Local(nil), fn.Locals...),
		Value:  *value,
	}
	l.program.Globals[id] = global
	l.loweredGlobals[id] = true
	return nil
}

func (l *lowerer) declareFunction(module ModuleID, def *checker.FunctionDef) (FunctionID, error) {
	if l.functionHasUnresolvedTypeVar(def) {
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
		params[i] = Param{Name: param.Name, Type: typeID, ABI: lowerABIParamMode(param)}
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
	if l.functionHasUnresolvedTypeVar(def) {
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
// declaration or specialized call signature. Source declarations record
// call-owned inference parameters in CallGenericParams; older/synthetic call
// signatures can fall back to GenericParams or their sorted binding keys.
func genericParamNames(def *checker.FunctionDef) []string {
	if len(def.CallGenericParams) > 0 {
		return def.CallGenericParams
	}
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

func (l *lowerer) declareGenericFunctionDef(module ModuleID, declaration *checker.FunctionDef) (FunctionID, error) {
	key := functionKey(module, declaration.Name)
	if id, ok := l.genericFunctionDefs[key]; ok {
		return id, nil
	}
	paramNames := genericParamNames(declaration)
	params := map[string]int{}
	goParams := make([]string, len(paramNames))
	for i, p := range paramNames {
		params[p] = i
		goParams[i] = goifyTypeParamName(p)
	}
	paramOwner := "genericfunction:" + key
	prev := l.defParams
	prevOwner := l.defParamOwner
	l.defParams = params
	l.defParamOwner = paramOwner
	signature, err := l.signatureForFunction(declaration.Parameters, declaration.ReturnType)
	l.defParams = prev
	l.defParamOwner = prevOwner
	if err != nil {
		return NoFunction, err
	}
	id := FunctionID(len(l.program.Functions))
	l.functions[concreteFunctionKey(module, declaration.Name, signature, "genericdef")] = id
	l.genericFunctionDefs[key] = id
	l.program.Functions = append(l.program.Functions, Function{
		ID:             id,
		Module:         module,
		Name:           declaration.Name,
		Signature:      signature,
		TypeParams:     goParams,
		TypeParamOwner: paramOwner,
		IsTest:         declaration.IsTest,
		Private:        declaration.Private,
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	typeVars := make(map[string]TypeID, len(params))
	for _, p := range paramNames {
		idx := params[p]
		tp, err := l.internTypeParam(paramOwner, p, idx)
		if err != nil {
			return NoFunction, err
		}
		typeVars[p] = tp
	}
	l.setFunctionTypeVars(id, typeVars)
	if err := l.lowerFunctionByID(id, declaration); err != nil {
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

func (fl *functionLowerer) declareAndLowerFunctionCall(module ModuleID, declaration *checker.FunctionDef, call *checker.FunctionCall) (FunctionID, error) {
	specialized := call.Signature()
	if specialized == nil {
		return NoFunction, fmt.Errorf("call to %s has no specialized signature", call.Name)
	}
	// Generic functions are lowered once as a Go generic definition (ADR 0031,
	// Phase 2) rather than monomorphized per call.
	if len(specialized.GenericBindings) > 0 {
		return fl.l.declareGenericFunctionDef(module, declaration)
	}
	if fl.l.functionHasUnresolvedTypeVar(specialized) {
		return NoFunction, fmt.Errorf("cannot declare unspecialized generic function %s", declaration.Name)
	}
	signature, err := fl.signatureForCall(call)
	if err != nil {
		return NoFunction, err
	}
	genericKey, typeVars, err := fl.genericBindingsKeyAndTypeVars(specialized)
	if err != nil {
		return NoFunction, err
	}
	id, err := fl.l.declareFunctionSpecializationWithSignatureAndGenericKey(module, declaration, signature, genericKey)
	if err != nil {
		return NoFunction, err
	}
	fl.l.setFunctionTypeVars(id, typeVars)
	if err := fl.l.lowerFunctionByID(id, declaration); err != nil {
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
		// Match the expected function type's canonical parameter shape exactly so
		// the closure's Go signature agrees with it.
		params[i] = Param{Name: param.Name, Type: typeInfo.Params[i]}
	}
	signature := Signature{Params: params, Return: typeInfo.Return}
	if def.LocalNamed {
		if id, ok := l.localNamedFunctions[def]; ok {
			return id, nil
		}
		// FunctionID assignment follows deterministic source traversal. Keep the
		// canonical checker declaration as the cache identity so same-named local
		// declarations can never share a lifted helper.
		keyName = fmt.Sprintf("local-closure/%d", len(l.localNamedFunctions))
	}
	key := concreteFunctionKey(module, keyName, signature, "")
	if id, ok := l.functions[key]; ok {
		return id, nil
	}
	id := FunctionID(len(l.program.Functions))
	l.functions[key] = id
	if def.LocalNamed {
		l.localNamedFunctions[def] = id
	}
	l.program.Functions = append(l.program.Functions, Function{
		ID:        id,
		Module:    module,
		Name:      def.Name,
		Signature: signature,
		Private:   def.LocalNamed || def.Private,
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	return id, nil
}

func lowerABIParamMode(param checker.Parameter) ABIParamMode {
	if param.ForeignABI == checker.ForeignParameterDescriptorValue {
		return ABIParamDescriptorValue
	}
	return ABIParamExact
}

func lowerABIParamModes(params []checker.Parameter, count int) []ABIParamMode {
	modes := make([]ABIParamMode, count)
	for index := 0; index < count && len(params) > 0; index++ {
		paramIndex := index
		if paramIndex >= len(params) {
			paramIndex = len(params) - 1
		}
		modes[index] = lowerABIParamMode(params[paramIndex])
	}
	return modes
}

func lowerForeignResultShape(shape checker.ForeignResultShape) ForeignResultShape {
	switch shape {
	case checker.ForeignResultDirect:
		return ForeignResultDirect
	case checker.ForeignResultValueError:
		return ForeignResultValueError
	case checker.ForeignResultErrorOnly:
		return ForeignResultErrorOnly
	case checker.ForeignResultValueBool:
		return ForeignResultValueBool
	default:
		return ForeignResultUnknown
	}
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
		fl.defineLocal(param.Name, param.Type, false)
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
		bindingNames := make([]string, 0, len(def.GenericBindings))
		for name := range def.GenericBindings {
			bindingNames = append(bindingNames, name)
		}
		sort.Strings(bindingNames)
		for _, name := range bindingNames {
			if _, ok := fl.typeVars[name]; ok {
				continue
			}
			typeID, err := fl.internResolvedType(def.GenericBindings[name])
			if err == nil {
				fl.typeVars[name] = typeID
			}
		}
	}
	return fl
}

type typeVarBindingVisit struct {
	pattern *checker.StructDef
	actual  TypeID
}

func (fl *functionLowerer) bindTypeVars(pattern checker.Type, actual TypeID) {
	fl.bindTypeVarsSeen(pattern, actual, map[typeVarBindingVisit]struct{}{})
}

func (fl *functionLowerer) bindTypeVarsSeen(pattern checker.Type, actual TypeID, seen map[typeVarBindingVisit]struct{}) {
	if pattern == nil || actual == NoType || !validTypeID(&fl.l.program, actual) {
		return
	}
	if structPattern, ok := pattern.(*checker.StructDef); ok {
		visit := typeVarBindingVisit{pattern: structPattern, actual: actual}
		if _, ok := seen[visit]; ok {
			return
		}
		seen[visit] = struct{}{}
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
			fl.bindTypeVarsSeen(typ.Of(), actualInfo.Elem, seen)
		}
	case *checker.Slice:
		if actualInfo.Kind == TypeSlice {
			fl.bindTypeVarsSeen(typ.Of(), actualInfo.Elem, seen)
		}
	case *checker.Chan:
		if actualInfo.Kind == TypeChannel {
			fl.bindTypeVarsSeen(typ.Of(), actualInfo.Elem, seen)
		}
	case *checker.Receiver:
		if actualInfo.Kind == TypeReceiver {
			fl.bindTypeVarsSeen(typ.Of(), actualInfo.Elem, seen)
		}
	case *checker.Sender:
		if actualInfo.Kind == TypeSender {
			fl.bindTypeVarsSeen(typ.Of(), actualInfo.Elem, seen)
		}
	case *checker.Map:
		if actualInfo.Kind == TypeMap {
			fl.bindTypeVarsSeen(typ.Key(), actualInfo.Key, seen)
			fl.bindTypeVarsSeen(typ.Value(), actualInfo.Value, seen)
		}
	case *checker.Maybe:
		if actualInfo.Kind == TypeMaybe {
			fl.bindTypeVarsSeen(typ.Of(), actualInfo.Elem, seen)
		}
	case *checker.MutableRef:
		if actualInfo.Kind == TypeReference {
			fl.bindTypeVarsSeen(typ.Of(), actualInfo.Elem, seen)
		}
	case *checker.Result:
		if actualInfo.Kind == TypeResult {
			fl.bindTypeVarsSeen(typ.Val(), actualInfo.Value, seen)
			fl.bindTypeVarsSeen(typ.Err(), actualInfo.Error, seen)
		}
	case *checker.FunctionDef:
		if actualInfo.Kind == TypeFunction {
			for i, param := range typ.Parameters {
				if i < len(actualInfo.Params) {
					fl.bindTypeVarsSeen(param.Type, actualInfo.Params[i], seen)
				}
			}
			fl.bindTypeVarsSeen(typ.ReturnType, actualInfo.Return, seen)
		}
	case *checker.StructDef:
		if actualInfo.Kind == TypeStruct {
			fieldsByName := map[string]FieldInfo{}
			for _, field := range actualInfo.Fields {
				fieldsByName[field.Name] = field
			}
			for _, name := range sortedFieldNames(typ.Fields) {
				if field, ok := fieldsByName[name]; ok {
					fl.bindTypeVarsSeen(typ.Fields[name], field.Type, seen)
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
		return NoType, fmt.Errorf("unresolved generic type variable $%s", tv.Name())
	}
	if typ, ok := t.(*checker.StructDef); ok && len(typ.GenericParams) > 0 {
		return fl.internStructType(typ)
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
	if typ, ok := t.(*checker.StructDef); ok && len(typ.GenericParams) > 0 {
		return fl.internResolvedStructType(typ)
	}
	if !typeContainsTypeVar(t) {
		return fl.l.internType(t)
	}
	return fl.internResolvedCompositeType(t)
}

func (fl *functionLowerer) internStructType(typ *checker.StructDef) (TypeID, error) {
	return fl.internStructTypeWithInterner(typ, fl.internType)
}

func (fl *functionLowerer) internResolvedStructType(typ *checker.StructDef) (TypeID, error) {
	return fl.internStructTypeWithInterner(typ, fl.internResolvedType)
}

// internFunctionParamType preserves first-class reference identity in the
// parameter TypeID.
func (l *lowerer) internFunctionParamType(param checker.Parameter, intern func(checker.Type) (TypeID, error)) (TypeID, error) {
	return intern(param.Type)
}

func (l *lowerer) internStructFieldType(fieldTypeValue checker.Type, intern func(checker.Type) (TypeID, error)) (TypeID, error) {
	return intern(fieldTypeValue)
}

func (l *lowerer) internGenericArgument(t checker.Type, intern func(checker.Type) (TypeID, error)) (TypeID, error) {
	switch typ := t.(type) {
	case *checker.MutableRef:
		return intern(typ)
	case *checker.List:
		elem, err := l.internGenericArgument(typ.Of(), intern)
		if err != nil {
			return NoType, err
		}
		return l.internSyntheticType("["+l.typeName(elem)+"]", TypeInfo{Kind: TypeList, Elem: elem})
	case *checker.Slice:
		elem, err := l.internGenericArgument(typ.Of(), intern)
		if err != nil {
			return NoType, err
		}
		return l.internSyntheticType("Slice<"+l.typeName(elem)+">", TypeInfo{Kind: TypeSlice, Elem: elem})
	case *checker.FixedArray:
		elem, err := l.internGenericArgument(typ.Of(), intern)
		if err != nil {
			return NoType, err
		}
		name := fmt.Sprintf("[%s; %d]", l.typeName(elem), typ.Len())
		return l.internSyntheticType(name, TypeInfo{Kind: TypeFixedArray, Elem: elem, Length: typ.Len()})
	case *checker.Map:
		key, err := l.internGenericArgument(typ.Key(), intern)
		if err != nil {
			return NoType, err
		}
		value, err := l.internGenericArgument(typ.Value(), intern)
		if err != nil {
			return NoType, err
		}
		name := "[" + l.typeName(key) + ":" + l.typeName(value) + "]"
		return l.internSyntheticType(name, TypeInfo{Kind: TypeMap, Key: key, Value: value})
	case *checker.Chan:
		elem, err := l.internGenericArgument(typ.Of(), intern)
		if err != nil {
			return NoType, err
		}
		return l.internSyntheticType("Chan<"+l.typeName(elem)+">", TypeInfo{Kind: TypeChannel, Elem: elem})
	case *checker.Receiver:
		elem, err := l.internGenericArgument(typ.Of(), intern)
		if err != nil {
			return NoType, err
		}
		return l.internSyntheticType("Receiver<"+l.typeName(elem)+">", TypeInfo{Kind: TypeReceiver, Elem: elem})
	case *checker.Sender:
		elem, err := l.internGenericArgument(typ.Of(), intern)
		if err != nil {
			return NoType, err
		}
		return l.internSyntheticType("Sender<"+l.typeName(elem)+">", TypeInfo{Kind: TypeSender, Elem: elem})
	case *checker.Result:
		value, err := l.internGenericArgument(typ.Val(), intern)
		if err != nil {
			return NoType, err
		}
		errType, err := l.internGenericArgument(typ.Err(), intern)
		if err != nil {
			return NoType, err
		}
		name := l.typeName(value) + "!" + l.typeName(errType)
		return l.internSyntheticType(name, TypeInfo{Kind: TypeResult, Value: value, Error: errType})
	default:
		return intern(t)
	}
}

func (l *lowerer) internForeignApplicationWithInterner(typ *checker.ForeignType, intern func(checker.Type) (TypeID, error)) (TypeID, error) {
	genericArgs := make([]TypeID, len(typ.TypeArgs))
	for index, typeArg := range typ.TypeArgs {
		typeID, err := l.internGenericArgument(typeArg, intern)
		if err != nil {
			return NoType, err
		}
		genericArgs[index] = typeID
	}
	key := foreignNominalKey(typ, genericArgs)
	seed := TypeInfo{
		Kind:              TypeForeignType,
		Name:              typ.String(),
		ForeignTarget:     typ.Target,
		ForeignNamespace:  typ.Namespace,
		ForeignQualifier:  typ.Qualifier,
		ForeignSymbol:     typ.Name,
		ForeignPointer:    typ.Pointer,
		ForeignInterface:  typ.Interface,
		GenericArgs:       genericArgs,
		GenericComparable: typ.ComparableTypeArgs(),
	}
	id, build, err := l.typeInterner.reserveNominal(key, seed)
	if err != nil || !build {
		return id, err
	}
	info := seed
	shape := typ
	if typ.Pointer {
		if value := typ.ValueForm(); value != nil {
			shape = value
		}
	}
	if shape.Underlying != nil {
		underlying, shapeErr := l.internGenericArgument(shape.Underlying, intern)
		if shapeErr != nil {
			return NoType, l.typeInterner.failNominal(key, shapeErr)
		}
		info.Value = underlying
	}
	if shape.MapKey != nil {
		mapKey, shapeErr := l.internGenericArgument(shape.MapKey, intern)
		if shapeErr != nil {
			return NoType, l.typeInterner.failNominal(key, shapeErr)
		}
		info.Key = mapKey
	}
	if shape.MapValue != nil {
		mapValue, shapeErr := l.internGenericArgument(shape.MapValue, intern)
		if shapeErr != nil {
			return NoType, l.typeInterner.failNominal(key, shapeErr)
		}
		info.Value = mapValue
	}
	if shape.Elem != nil {
		elem, shapeErr := l.internGenericArgument(shape.Elem, intern)
		if shapeErr != nil {
			return NoType, l.typeInterner.failNominal(key, shapeErr)
		}
		info.Elem = elem
	}
	return l.typeInterner.completeNominal(key, info)
}

func (l *lowerer) internStructApplicationWithInterner(typ *checker.StructDef, intern func(checker.Type) (TypeID, error)) (TypeID, error) {
	genericArgs := make([]TypeID, len(typ.TypeArgs))
	for index, typeArg := range typ.TypeArgs {
		typeID, err := l.internGenericArgument(typeArg, intern)
		if err != nil {
			return NoType, err
		}
		genericArgs[index] = typeID
	}

	definition := l.lookupGenericStructDef(typ.ModulePath, typ.Name)
	if definition == nil {
		return NoType, fmt.Errorf("generic struct definition not found for %s", typ.Name)
	}
	defID, ok := l.genericStructDefs[genericStructDefKey(definition.ModulePath, definition.Name)]
	if !ok {
		interned, err := l.internGenericStructDef(definition)
		if err != nil {
			return NoType, err
		}
		defID = interned
	}

	key := applicationNominalKey(TypeStruct, defID, genericArgs)
	parts := make([]string, len(genericArgs))
	for index, typeID := range genericArgs {
		parts[index] = l.typeName(typeID)
	}
	seed := TypeInfo{
		Kind:        TypeStruct,
		Name:        definition.Name + "<" + strings.Join(parts, ",") + ">",
		ModulePath:  definition.ModulePath,
		Private:     definition.Private,
		Generic:     defID,
		GenericArgs: append([]TypeID(nil), genericArgs...),
	}
	id, build, err := l.typeInterner.reserveNominal(key, seed)
	if err != nil {
		return NoType, err
	}
	if !build {
		return id, nil
	}

	structFields := checker.StructFields(typ)
	fields := sortedFieldNames(structFields)
	info := seed
	info.Fields = make([]FieldInfo, len(fields))
	for index, fieldName := range fields {
		fieldType, fieldErr := l.internStructFieldType(structFields[fieldName], intern)
		if fieldErr != nil {
			return NoType, l.typeInterner.failNominal(key, fieldErr)
		}
		info.Fields[index] = lowerStructFieldInfo(typ, fieldName, fieldType, index)
	}
	return l.typeInterner.completeNominal(key, info)
}

func (fl *functionLowerer) internStructTypeWithInterner(typ *checker.StructDef, intern func(checker.Type) (TypeID, error)) (TypeID, error) {
	if len(typ.TypeArgs) > 0 {
		return fl.l.internStructApplicationWithInterner(typ, intern)
	}
	if len(typ.GenericParams) == 0 {
		return fl.l.internType(typ)
	}

	genericArgs := make([]TypeID, 0, len(typ.GenericParams))
	for _, param := range typ.GenericParams {
		typeID, ok := fl.typeVars[param]
		if !ok {
			return NoType, fmt.Errorf("cannot resolve generic argument %s for %s", param, typ.Name)
		}
		genericArgs = append(genericArgs, typeID)
	}
	definition := fl.l.lookupGenericStructDef(typ.ModulePath, typ.Name)
	if definition == nil {
		return NoType, fmt.Errorf("generic struct definition not found for %s", typ.Name)
	}
	defID, ok := fl.l.genericStructDefs[genericStructDefKey(definition.ModulePath, definition.Name)]
	if !ok {
		interned, err := fl.l.internGenericStructDef(definition)
		if err != nil {
			return NoType, err
		}
		defID = interned
	}

	parts := make([]string, len(genericArgs))
	for index, typeID := range genericArgs {
		parts[index] = fl.l.typeName(typeID)
	}
	key := applicationNominalKey(TypeStruct, defID, genericArgs)
	seed := TypeInfo{
		Kind:        TypeStruct,
		Name:        definition.Name + "<" + strings.Join(parts, ",") + ">",
		ModulePath:  definition.ModulePath,
		Private:     definition.Private,
		Generic:     defID,
		GenericArgs: append([]TypeID(nil), genericArgs...),
	}
	id, build, err := fl.l.typeInterner.reserveNominal(key, seed)
	if err != nil {
		return NoType, err
	}
	if !build {
		return id, nil
	}

	structFields := checker.StructFields(typ)
	fields := sortedFieldNames(structFields)
	info := seed
	info.Fields = make([]FieldInfo, len(fields))
	for index, fieldName := range fields {
		fieldType, fieldErr := fl.l.internStructFieldType(structFields[fieldName], intern)
		if fieldErr != nil {
			return NoType, fl.l.typeInterner.failNominal(key, fieldErr)
		}
		info.Fields[index] = lowerStructFieldInfo(typ, fieldName, fieldType, index)
	}
	return fl.l.typeInterner.completeNominal(key, info)
}

func (fl *functionLowerer) internResolvedCompositeType(t checker.Type) (TypeID, error) {
	switch typ := t.(type) {
	case *checker.StructDef:
		return fl.internResolvedStructType(typ)
	case *checker.ForeignType:
		return fl.l.internForeignApplicationWithInterner(typ, fl.internResolvedType)
	case *checker.List:
		elem, err := fl.internResolvedType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("["+fl.l.typeName(elem)+"]", TypeInfo{Kind: TypeList, Elem: elem})
	case *checker.Slice:
		elem, err := fl.internResolvedType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("Slice<"+fl.l.typeName(elem)+">", TypeInfo{Kind: TypeSlice, Elem: elem})
	case *checker.FixedArray:
		elem, err := fl.internResolvedType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType(fmt.Sprintf("[%s; %d]", fl.l.typeName(elem), typ.Len()), TypeInfo{Kind: TypeFixedArray, Elem: elem, Length: typ.Len()})
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
		elem, err := fl.internResolvedType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType(fl.l.typeName(elem)+"?", TypeInfo{Kind: TypeMaybe, Elem: elem})
	case *checker.MutableRef:
		elem, err := fl.internResolvedType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("mut "+fl.l.typeName(elem), TypeInfo{Kind: TypeReference, Elem: elem})
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
		for i, param := range typ.Parameters {
			paramType, err := fl.l.internFunctionParamType(param, fl.internResolvedType)
			if err != nil {
				return NoType, err
			}
			params[i] = paramType
		}
		returnType, err := fl.internResolvedType(typ.ReturnType)
		if err != nil {
			return NoType, err
		}
		variadic := len(typ.Parameters) > 0 && typ.Parameters[len(typ.Parameters)-1].Variadic
		name := "fn("
		for i, param := range params {
			if i > 0 {
				name += ","
			}
			if variadic && i == len(params)-1 {
				name += "..."
			}
			name += fl.l.typeName(param)
		}
		name += ") " + fl.l.typeName(returnType)
		return fl.l.internSyntheticType(name, TypeInfo{Kind: TypeFunction, Params: params, Return: returnType, Variadic: variadic})
	}
	return NoType, fmt.Errorf("unresolved generic type variable in %s", t.String())
}

func (fl *functionLowerer) internCompositeType(t checker.Type) (TypeID, error) {
	switch typ := t.(type) {
	case *checker.StructDef:
		return fl.internStructType(typ)
	case *checker.ForeignType:
		return fl.l.internForeignApplicationWithInterner(typ, fl.internType)
	case *checker.List:
		elem, err := fl.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("["+fl.l.typeName(elem)+"]", TypeInfo{Kind: TypeList, Elem: elem})
	case *checker.Slice:
		elem, err := fl.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("Slice<"+fl.l.typeName(elem)+">", TypeInfo{Kind: TypeSlice, Elem: elem})
	case *checker.FixedArray:
		elem, err := fl.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType(fmt.Sprintf("[%s; %d]", fl.l.typeName(elem), typ.Len()), TypeInfo{Kind: TypeFixedArray, Elem: elem, Length: typ.Len()})
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
		elem, err := fl.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType(fl.l.typeName(elem)+"?", TypeInfo{Kind: TypeMaybe, Elem: elem})
	case *checker.MutableRef:
		elem, err := fl.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("mut "+fl.l.typeName(elem), TypeInfo{Kind: TypeReference, Elem: elem})
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
		for i, param := range typ.Parameters {
			paramType, err := fl.l.internFunctionParamType(param, fl.internType)
			if err != nil {
				return NoType, err
			}
			params[i] = paramType
		}
		returnType, err := fl.internType(typ.ReturnType)
		if err != nil {
			return NoType, err
		}
		variadic := len(typ.Parameters) > 0 && typ.Parameters[len(typ.Parameters)-1].Variadic
		name := "fn("
		for i, param := range params {
			if i > 0 {
				name += ","
			}
			if variadic && i == len(params)-1 {
				name += "..."
			}
			name += fl.l.typeName(param)
		}
		name += ") " + fl.l.typeName(returnType)
		return fl.l.internSyntheticType(name, TypeInfo{Kind: TypeFunction, Params: params, Return: returnType, Variadic: variadic})
	default:
		return fl.l.internType(t)
	}
}

func (l *lowerer) declareGenericStructMethodsAndTraitImpls(module ModuleID, def *checker.StructDef) error {
	if def == nil || len(def.GenericParams) == 0 {
		return nil
	}
	defType, err := l.internGenericStructDef(def)
	if err != nil {
		return err
	}
	selfType, err := l.genericSelfInstance(defType)
	if err != nil {
		return err
	}
	fl := &functionLowerer{l: l}
	pendingMethods := map[FunctionID]*checker.FunctionDef{}

	// Required Go methods use their exact native name. Builtin Error shares
	// that method function with its Ard trait implementation.
	requiredMethods := map[string]*checker.FunctionDef{}
	for _, method := range sortedMethodDefinitions(l.requiredGoMethods(def)) {
		if method != nil {
			requiredMethods[method.Name] = method
		}
	}
	for _, trait := range def.Traits {
		if !checker.IsBuiltinError(trait) {
			continue
		}
		methods := l.traitMethods(def, trait)
		methodNames := make([]string, 0, len(methods))
		for name := range methods {
			methodNames = append(methodNames, name)
		}
		sort.Strings(methodNames)
		for _, name := range methodNames {
			if method := methods[name]; method != nil {
				requiredMethods[name] = method
			}
		}
	}
	names := make([]string, 0, len(requiredMethods))
	for name := range requiredMethods {
		names = append(names, name)
	}
	sort.Strings(names)
	declaredRequired := map[string]FunctionID{}
	for _, name := range names {
		method := requiredMethods[name]
		key := "genericmethod:" + def.ModulePath + ":" + def.Name + ":" + method.Name
		id, _, err := fl.declareGenericMethodFunction(module, selfType, method, key, def.Name+"."+method.Name, "genericmethod")
		if err != nil {
			return fmt.Errorf("declare required Go method %s.%s: %w", def.Name, method.RequiredGoMethodName, err)
		}
		declaredRequired[name] = id
		pendingMethods[id] = method
	}

	type implPlan struct {
		key     string
		trait   TraitID
		methods []FunctionID
	}
	plans := []implPlan{}
	plannedKeys := map[string]bool{}
	for _, trait := range def.Traits {
		if trait == nil {
			continue
		}
		key := implKey(module, checkerTraitKey(trait), def.String())
		if _, exists := l.impls[key]; exists || plannedKeys[key] {
			continue
		}
		plannedKeys[key] = true
		traitID, err := l.internTrait(trait)
		if err != nil {
			return err
		}
		traitMethods := trait.GetMethods()
		methodIDs := make([]FunctionID, len(traitMethods))
		if checker.IsBuiltinError(trait) {
			for i, traitMethod := range traitMethods {
				id, ok := declaredRequired[traitMethod.Name]
				if !ok {
					return fmt.Errorf("generic Error implementation %s is missing method %s", def.Name, traitMethod.Name)
				}
				methodIDs[i] = id
			}
		} else {
			methods := l.traitMethods(def, trait)
			for i, traitMethod := range traitMethods {
				method := methods[traitMethod.Name]
				if method == nil {
					return fmt.Errorf("generic trait implementation %s for %s is missing method %s", trait.Name, def.Name, traitMethod.Name)
				}
				id, err := fl.declareGenericTraitMethodFunction(module, selfType, def, trait, method)
				if err != nil {
					return fmt.Errorf("declare generic trait method %s.%s: %w", def.Name, method.Name, err)
				}
				methodIDs[i] = id
				pendingMethods[id] = method
			}
		}
		plans = append(plans, implPlan{key: key, trait: traitID, methods: methodIDs})
	}

	// Register every implementation before lowering any method body. Required,
	// Error, and ordinary trait methods may project self across these contracts.
	for _, plan := range plans {
		id := ImplID(len(l.program.Impls))
		l.impls[plan.key] = id
		l.program.Impls = append(l.program.Impls, Impl{ID: id, Trait: plan.trait, ForType: selfType, Methods: plan.methods})
	}
	methodIDs := make([]int, 0, len(pendingMethods))
	for id := range pendingMethods {
		methodIDs = append(methodIDs, int(id))
	}
	sort.Ints(methodIDs)
	for _, rawID := range methodIDs {
		id := FunctionID(rawID)
		if err := l.lowerFunctionByID(id, pendingMethods[id]); err != nil {
			return err
		}
	}
	return nil
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
	traitMethodDefs := map[*checker.FunctionDef]bool{}
	for _, trait := range def.Traits {
		if trait == nil {
			continue
		}
		for _, method := range sortedMethodDefinitions(l.traitMethods(def, trait)) {
			traitMethodDefs[method] = true
		}
	}
	for _, method := range sortedMethodDefinitions(l.requiredGoMethods(def)) {
		if method == nil || traitMethodDefs[method] || l.functionHasUnresolvedTypeVar(method) {
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
	for _, method := range sortedMethodDefinitions(l.inherentMethods(def)) {
		if method == nil || l.functionHasUnresolvedTypeVar(method) {
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
	switch typed := typ.(type) {
	case *checker.StructDef:
		traits = typed.Traits
	case *checker.Enum:
		traits = typed.Traits
	default:
		return nil
	}

	for _, trait := range traits {
		if trait == nil {
			continue
		}
		methods := l.traitMethods(typ, trait)
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
		methodID, err := l.declareMethodFunction(module, owner, checkerTraitKey(trait), trait.Name, ownerType, methodDef)
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
			Target: &Expr{Kind: ExprLoadLocal, Type: ownerInfo.ID, Payload: &LocalExprPayload{Local: 0}},
		}},
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	return id, nil
}

func (l *lowerer) declareMethodFunction(module ModuleID, owner checker.Type, traitKey, traitName string, ownerType TypeID, def *checker.FunctionDef) (FunctionID, error) {
	key := methodFunctionKey(module, owner.String(), traitKey, def.Name)
	if id, ok := l.functions[key]; ok {
		return id, nil
	}

	receiver := def.Receiver
	if receiver == "" {
		receiver = "self"
	}
	receiverType, err := l.methodReceiverType(ownerType, def.Mutates)
	if err != nil {
		return NoFunction, err
	}
	params := make([]Param, 0, len(def.Parameters)+1)
	params = append(params, Param{Name: receiver, Type: receiverType})
	for _, param := range def.Parameters {
		typeID, err := l.internType(param.Type)
		if err != nil {
			return NoFunction, err
		}
		params = append(params, Param{Name: param.Name, Type: typeID, ABI: lowerABIParamMode(param)})
	}
	returnType, err := l.internType(def.ReturnType)
	if err != nil {
		return NoFunction, err
	}
	id := FunctionID(len(l.program.Functions))
	l.functions[key] = id
	l.program.Functions = append(l.program.Functions, Function{
		ID:                   id,
		Module:               module,
		Name:                 owner.String() + "." + traitName + "." + def.Name,
		Receiver:             ownerType,
		MethodName:           def.Name,
		RequiredGoMethodName: def.RequiredGoMethodName,
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
		fl.defineLocal(param.Name, param.Type, false)
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
	receiverType, err := l.methodReceiverType(ownerType, def.Mutates)
	if err != nil {
		return NoFunction, err
	}
	params := make([]Param, 0, len(def.Parameters)+1)
	params = append(params, Param{Name: receiver, Type: receiverType})
	for i, param := range def.Parameters {
		paramType := param.Type
		if l.typeHasUnresolvedTypeVar(paramType) && i < len(args) {
			paramType = args[i].Type()
		}
		typeID, err := l.internType(paramType)
		if err != nil {
			return NoFunction, err
		}
		params = append(params, Param{Name: param.Name, Type: typeID, ABI: lowerABIParamMode(param)})
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
		ID:                   id,
		Module:               module,
		Name:                 ownerName + "." + def.Name,
		Receiver:             ownerType,
		MethodName:           def.Name,
		RequiredGoMethodName: def.RequiredGoMethodName,
		Signature:            signature,
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	return id, nil
}

func (fl *functionLowerer) declareInstanceMethodFunction(module ModuleID, ownerName string, ownerType TypeID, def *checker.FunctionDef, args []checker.Expression, returnType TypeID) (FunctionID, error) {
	receiver := def.Receiver
	if receiver == "" {
		receiver = "self"
	}
	receiverType, err := fl.l.methodReceiverType(ownerType, def.Mutates)
	if err != nil {
		return NoFunction, err
	}
	params := make([]Param, 0, len(def.Parameters)+1)
	params = append(params, Param{Name: receiver, Type: receiverType})
	for i, param := range def.Parameters {
		paramType := param.Type
		if fl.l.typeHasUnresolvedTypeVar(paramType) && i < len(args) {
			paramType = args[i].Type()
		}
		typeID, err := fl.internType(paramType)
		if err != nil {
			return NoFunction, err
		}
		params = append(params, Param{Name: param.Name, Type: typeID, ABI: lowerABIParamMode(param)})
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
		ID:                   id,
		Module:               module,
		Name:                 ownerName + "." + def.Name,
		Receiver:             ownerType,
		MethodName:           def.Name,
		RequiredGoMethodName: def.RequiredGoMethodName,
		Signature:            signature,
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
	def, ok := l.typeInfo(defID)
	if !ok {
		return NoType, fmt.Errorf("generic struct definition %d is invalid or incomplete", defID)
	}
	paramOwner := genericStructDefKey(def.ModulePath, def.Name)
	args := make([]TypeID, len(def.TypeParams))
	for i, name := range def.TypeParams {
		tp, err := l.internTypeParam(paramOwner, name, i)
		if err != nil {
			return NoType, err
		}
		args[i] = tp
	}
	key := applicationNominalKey(TypeStruct, defID, args)
	info := TypeInfo{
		Kind:        TypeStruct,
		Name:        def.Name + "<self>",
		ModulePath:  def.ModulePath,
		Private:     def.Private,
		Fields:      def.Fields,
		Generic:     defID,
		GenericArgs: args,
	}
	id, build, err := l.typeInterner.reserveNominal(key, info)
	if err != nil || !build {
		return id, err
	}
	return l.typeInterner.completeNominal(key, info)
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

func (fl *functionLowerer) declareGenericInstanceMethodFunction(module ModuleID, instanceType TypeID, structType *checker.StructDef, declaration *checker.FunctionDef) (FunctionID, []TypeID, error) {
	definition := checker.StructDefinition(structType)
	if definition == nil {
		return NoFunction, nil, fmt.Errorf("generic method %s has no struct definition", declaration.Name)
	}
	key := "genericmethod:" + definition.ModulePath + ":" + definition.Name + ":" + declaration.Name
	id, typeArgs, err := fl.declareGenericMethodFunction(module, instanceType, declaration, key, definition.Name+"."+declaration.Name, "genericmethod")
	if err != nil {
		return NoFunction, nil, err
	}
	if err := fl.l.lowerFunctionByID(id, declaration); err != nil {
		return NoFunction, nil, err
	}
	return id, typeArgs, nil
}

func (fl *functionLowerer) declareGenericTraitMethodFunction(module ModuleID, instanceType TypeID, structType *checker.StructDef, trait *checker.Trait, method *checker.FunctionDef) (FunctionID, error) {
	definition := checker.StructDefinition(structType)
	if definition == nil {
		return NoFunction, fmt.Errorf("generic trait method %s has no struct definition", method.Name)
	}
	traitKey := checkerTraitKey(trait)
	key := "generictraitmethod:" + definition.ModulePath + ":" + definition.Name + ":" + traitKey + ":" + method.Name
	id, _, err := fl.declareGenericMethodFunction(module, instanceType, method, key, definition.Name+"."+trait.Name+"."+method.Name, "generictraitmethod:"+traitKey)
	return id, err
}

// declareGenericMethodFunction declares a generic receiver method without
// lowering its body. Trait implementations use this split phase so their AIR
// Impl can be registered before a body projects self to its current trait.
func (fl *functionLowerer) declareGenericMethodFunction(module ModuleID, instanceType TypeID, method *checker.FunctionDef, key, functionName, discriminator string) (FunctionID, []TypeID, error) {
	info, ok := fl.l.typeInfo(instanceType)
	if !ok || info.Generic == NoType {
		return NoFunction, nil, fmt.Errorf("generic method receiver %d is not a generic instantiation", instanceType)
	}
	defID := info.Generic
	structDef, ok := fl.l.typeInfo(defID)
	if !ok {
		return NoFunction, nil, fmt.Errorf("generic method definition %d is invalid or incomplete", defID)
	}
	paramNames := structDef.TypeParams
	typeArgs := info.GenericArgs
	if id, ok := fl.l.genericMethodDefs[key]; ok {
		return id, typeArgs, nil
	}

	recvType, err := fl.l.genericSelfInstance(defID)
	if err != nil {
		return NoFunction, nil, err
	}
	receiver := method.Receiver
	if receiver == "" {
		receiver = "self"
	}

	params := map[string]int{}
	for i, p := range paramNames {
		params[p] = i
	}
	paramOwner := genericStructDefKey(structDef.ModulePath, structDef.Name)
	prev := fl.l.defParams
	prevOwner := fl.l.defParamOwner
	fl.l.defParams = params
	fl.l.defParamOwner = paramOwner
	receiverType, err := fl.l.methodReceiverType(recvType, method.Mutates)
	if err != nil {
		fl.l.defParams = prev
		fl.l.defParamOwner = prevOwner
		return NoFunction, nil, err
	}
	methodParams := make([]Param, 0, len(method.Parameters)+1)
	methodParams = append(methodParams, Param{Name: receiver, Type: receiverType})
	for _, p := range method.Parameters {
		tid, err := fl.l.internType(p.Type)
		if err != nil {
			fl.l.defParams = prev
			fl.l.defParamOwner = prevOwner
			return NoFunction, nil, err
		}
		methodParams = append(methodParams, Param{Name: p.Name, Type: tid, ABI: lowerABIParamMode(p)})
	}
	returnType, err := fl.l.internType(method.ReturnType)
	fl.l.defParams = prev
	fl.l.defParamOwner = prevOwner
	if err != nil {
		return NoFunction, nil, err
	}

	signature := Signature{Params: methodParams, Return: returnType}
	id := FunctionID(len(fl.l.program.Functions))
	fl.l.genericMethodDefs[key] = id
	fl.l.functions[concreteFunctionKey(module, functionName, signature, discriminator)] = id
	fl.l.program.Functions = append(fl.l.program.Functions, Function{
		ID:                   id,
		Module:               module,
		Name:                 functionName,
		Receiver:             recvType,
		MethodName:           method.Name,
		RequiredGoMethodName: method.RequiredGoMethodName,
		Signature:            signature,
		TypeParams:           paramNames,
		TypeParamOwner:       paramOwner,
	})
	fl.l.program.Modules[module].Functions = appendUniqueFunction(fl.l.program.Modules[module].Functions, id)
	typeVars := make(map[string]TypeID, len(paramNames))
	for _, p := range paramNames {
		idx := params[p]
		tp, err := fl.l.internTypeParam(paramOwner, p, idx)
		if err != nil {
			return NoFunction, nil, err
		}
		typeVars[p] = tp
	}
	fl.l.setFunctionTypeVars(id, typeVars)
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

func (fl *functionLowerer) spreadElementTypeForCall(call *checker.FunctionCall) (TypeID, error) {
	if call == nil || !call.TailSpread {
		return NoType, nil
	}
	definition := call.Signature()
	if definition == nil || len(definition.Parameters) == 0 || !definition.Parameters[len(definition.Parameters)-1].Variadic {
		return NoType, fmt.Errorf("spread call is missing a variadic parameter")
	}
	return fl.internResolvedType(definition.Parameters[len(definition.Parameters)-1].Type)
}

func (fl *functionLowerer) spreadCallableTypeForCall(call *checker.FunctionCall) (TypeID, error) {
	if call == nil || !call.TailSpread {
		return NoType, nil
	}
	definition := call.Signature()
	if definition == nil {
		return NoType, fmt.Errorf("spread call is missing its callable definition")
	}
	return fl.internResolvedType(definition)
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
	if def := call.Signature(); def != nil {
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
			params[i] = Param{Name: param.Name, Type: typeID, ABI: lowerABIParamMode(param)}
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

func (l *lowerer) internTypeParam(owner, name string, idx int) (TypeID, error) {
	key := typeParamKey{owner: owner, index: idx}
	info := TypeInfo{Kind: TypeParam, Name: goifyTypeParamName(name), ParamOwner: owner, ParamIndex: idx}
	return l.typeInterner.internTypeParam(key, info)
}

// internGenericStructDef interns the generic definition of a struct (fields
// reference TypeParam; TypeParams names the parameters) and registers it with
// its declaring module so the backend emits and qualifies it (ADR 0031).
func (l *lowerer) internGenericStructDef(typ *checker.StructDef) (TypeID, error) {
	key := genericStructDefKey(typ.ModulePath, typ.Name)
	nominalKey := declarationNominalKey(TypeStruct, typ.ModulePath, typ.Name)
	seed := TypeInfo{Kind: TypeStruct, Name: typ.Name, ModulePath: typ.ModulePath, Private: typ.Private}
	id, build, err := l.typeInterner.reserveNominal(nominalKey, seed)
	if err != nil || !build {
		return id, err
	}
	l.genericStructDefs[key] = id
	goParams := make([]string, len(typ.GenericParams))
	params := map[string]int{}
	for i, p := range typ.GenericParams {
		params[p] = i
		goParams[i] = goifyTypeParamName(p)
	}
	info := TypeInfo{ID: id, Kind: TypeStruct, Name: typ.Name, ModulePath: typ.ModulePath, Private: typ.Private, TypeParams: goParams}
	prev := l.defParams
	prevOwner := l.defParamOwner
	l.defParams = params
	l.defParamOwner = key
	fieldNames := sortedFieldNames(typ.Fields)
	info.Fields = make([]FieldInfo, len(fieldNames))
	for i, name := range fieldNames {
		ft := typ.Fields[name]
		ftid, err := l.internStructFieldType(ft, l.internType)
		if err != nil {
			l.defParams = prev
			l.defParamOwner = prevOwner
			return NoType, l.typeInterner.failNominal(nominalKey, err)
		}
		info.Fields[i] = lowerStructFieldInfo(typ, name, ftid, i)
	}
	l.defParams = prev
	l.defParamOwner = prevOwner
	if _, err := l.typeInterner.completeNominal(nominalKey, info); err != nil {
		return NoType, err
	}
	if modID, ok := l.moduleByPath[typ.ModulePath]; ok {
		l.program.Modules[modID].Types = appendUniqueType(l.program.Modules[modID].Types, id)
	}
	return id, nil
}

func structDefInModule(mod checker.Module, name string, generic bool) *checker.StructDef {
	if mod == nil || mod.Program() == nil {
		return nil
	}
	for _, stmt := range mod.Program().Statements {
		sd, ok := stmt.Stmt.(*checker.StructDef)
		if ok && sd.Name == name && (len(sd.GenericParams) > 0) == generic {
			return sd
		}
	}
	return nil
}

func collectReachableModules(mod checker.Module, seen map[string]checker.Module) {
	if mod == nil || seen[mod.Path()] != nil {
		return
	}
	seen[mod.Path()] = mod
	if mod.Program() == nil {
		return
	}
	for _, imported := range sortedModules(mod.Program().Imports) {
		collectReachableModules(imported, seen)
	}
}

func (l *lowerer) lookupStructDef(modulePath, name string, generic bool) *checker.StructDef {
	if name == "" {
		return nil
	}
	if modulePath != "" {
		mod := l.findReachableModule(modulePath)
		return structDefInModule(mod, name, generic)
	}

	// Some checker-created closure signatures lose the owner path while copying
	// an otherwise nominal type. Search the complete reachable import graph and
	// recover only a unique declaration of the same generic shape. Ambiguity must
	// not merge unrelated nominal types.
	modules := map[string]checker.Module{}
	for _, mod := range sortedModules(l.moduleByName) {
		collectReachableModules(mod, modules)
	}
	var found *checker.StructDef
	for _, mod := range sortedModules(modules) {
		sd := structDefInModule(mod, name, generic)
		if sd == nil {
			continue
		}
		if found != nil && found != sd {
			return nil
		}
		found = sd
	}
	return found
}

// lookupGenericStructDef finds the generic definition of a struct from the
// checker module scope (its fields still reference the type variables).
func (l *lowerer) lookupGenericStructDef(modulePath, name string) *checker.StructDef {
	return l.lookupStructDef(modulePath, name, true)
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
				return l.internTypeParam(l.defParamOwner, tv.Name(), idx)
			}
		}
		if tv.Actual() != nil {
			return l.internType(tv.Actual())
		}
	}
	if ref, ok := t.(*checker.MutableRef); ok {
		elem, err := l.internType(ref.Of())
		if err != nil {
			return NoType, err
		}
		return l.internSyntheticType("mut "+l.typeName(elem), TypeInfo{Kind: TypeReference, Elem: elem})
	}
	if typ, ok := t.(*checker.StructDef); ok {
		if len(typ.TypeArgs) > 0 {
			return l.internStructApplicationWithInterner(typ, l.internType)
		}
		if len(typ.GenericParams) == 0 {
			if canonical := l.lookupStructDef(typ.ModulePath, typ.Name, false); canonical != nil {
				typ = canonical
			}
			return l.internNominalStruct(typ, l.internType)
		}
		return l.internGenericStructDef(typ)
	}
	if typ, ok := t.(*checker.Enum); ok {
		return l.internNominalEnum(typ)
	}
	if typ, ok := t.(*checker.Union); ok {
		return l.internNominalUnion(typ, l.internType)
	}
	if typ, ok := t.(*checker.ForeignType); ok {
		return l.internForeignApplicationWithInterner(typ, l.internType)
	}
	if id, structural, err := l.internCheckerStructuralType(t); structural {
		return id, err
	}
	return l.internAtomicOrTraitType(t)
}

func (l *lowerer) internAtomicOrTraitType(t checker.Type) (TypeID, error) {
	if trait, ok := t.(*checker.Trait); ok {
		traitID, err := l.internTrait(trait)
		if err != nil {
			return NoType, err
		}
		info := TypeInfo{Kind: TypeTraitObject, Name: trait.String(), Trait: traitID}
		return l.typeInterner.internTraitObject(traitID, info)
	}
	info := TypeInfo{Name: airTypeName(t)}
	switch t {
	case checker.Void:
		info.Kind = TypeVoid
	case checker.Int:
		info.Kind = TypeInt
	case checker.Int8, checker.Int16, checker.Int32, checker.Int64, checker.Uint, checker.Uint8, checker.Uint16, checker.Uint32, checker.Uint64, checker.Uintptr, checker.Float32:
		info.Kind = TypeScalar
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
	case checker.EmbeddedFS:
		info.Kind = TypeEmbeddedFS
	case checker.Any:
		info.Kind = TypeAny
	default:
		return NoType, fmt.Errorf("unsupported AIR type %T (%s)", t, t.String())
	}
	return l.typeInterner.internAtomic(t, info)
}

func (l *lowerer) methodReceiverType(owner TypeID, mutates bool) (TypeID, error) {
	if !mutates {
		return owner, nil
	}
	if info, ok := l.typeInfo(owner); ok && info.Kind == TypeReference {
		return owner, nil
	}
	return l.internSyntheticType("mut "+l.typeName(owner), TypeInfo{Kind: TypeReference, Elem: owner})
}

func (l *lowerer) internSyntheticType(_ string, info TypeInfo) (TypeID, error) {
	return l.typeInterner.internStructural(info)
}

func (l *lowerer) typeName(id TypeID) string {
	if l.typeInterner == nil {
		// Some isolated package tests construct finalized lowerer fixtures without
		// lowering lifecycle state. Such fixtures cannot contain reservations.
		info, ok := l.typeInfo(id)
		if !ok {
			return fmt.Sprintf("<invalid:%d>", id)
		}
		return info.Name
	}
	name, err := l.typeInterner.displayName(id)
	if err != nil {
		return fmt.Sprintf("<invalid:%d>", id)
	}
	return name
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
		loweredMethods[i] = TraitMethod{Name: method.Name, Mutates: method.Mutates, Signature: sig}
	}
	l.program.Traits[id] = Trait{
		ID:                  id,
		Name:                trait.Name,
		ModulePath:          trait.ModulePath,
		Private:             trait.IsPrivate(),
		BuiltinError:        checker.IsBuiltinError(trait),
		GoInterfaceFallback: !trait.UsesNaturalGoInterface(),
		Methods:             loweredMethods,
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
		loweredParams[i] = Param{Name: param.Name, Type: typeID, ABI: lowerABIParamMode(param)}
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
	case *checker.Slice:
		return typeContainsTypeVarSeen(typ.Of(), seen)
	case *checker.FixedArray:
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
	case *checker.ForeignType:
		for _, typeArg := range typ.TypeArgs {
			if typeContainsTypeVarSeen(typeArg, seen) {
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
	case *checker.Slice:
		return typeHasUnresolvedTypeVarSeen(typ.Of(), seen)
	case *checker.FixedArray:
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
	case *checker.ForeignType:
		for _, typeArg := range typ.TypeArgs {
			if typeHasUnresolvedTypeVarSeen(typeArg, seen) {
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
	case TypeVoid, TypeInt, TypeScalar, TypeForeignType, TypeFloat64, TypeBool, TypeByte, TypeRune, TypeStr, TypeList, TypeSlice, TypeFixedArray, TypeMap, TypeStruct, TypeEnum, TypeMaybe, TypeResult, TypeUnion, TypeChannel, TypeReceiver, TypeSender, TypeAny, TypeReference:
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
		if left.Params[i].Type != right.Params[i].Type || left.Params[i].ABI != right.Params[i].ABI {
			return false
		}
	}
	return true
}

func airTypeName(t checker.Type) string {
	return t.String()
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
			expected := defaultType
			if fl.isVoidType(defaultType) && !checker.IsNever(stmt.Expr.Type()) {
				checkedType, err := fl.internResolvedType(stmt.Expr.Type())
				if err != nil {
					return block, err
				}
				if !fl.isVoidType(checkedType) {
					expected = checkedType
				}
			}
			if def := localNamedFunctionDeclaration(stmt.Expr); def != nil {
				binding, _, err := fl.lowerLocalNamedFunction(def)
				if err != nil {
					return block, err
				}
				block.Stmts = append(block.Stmts, *binding)
				expr, _, err := fl.lowerContextualExpr(stmt.Expr, expected)
				if err != nil {
					return block, err
				}
				block.Result = expr
				continue
			}
			expr, _, err := fl.lowerContextualExpr(stmt.Expr, expected)
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
	return lowered, expected, nil
}

func (fl *functionLowerer) lowerExprWithExpected(expr checker.Expression, expected TypeID) (*Expr, error) {
	lowered, _, err := fl.lowerContextualExpr(expr, expected)
	return lowered, err
}

func (fl *functionLowerer) lowerExprWithExpectedRaw(expr checker.Expression, expected TypeID) (*Expr, error) {
	if coercion, ok := expr.(*checker.NeverCoercion); ok && validTypeID(&fl.l.program, expected) {
		return fl.lowerExprWithExpected(coercion.Value, expected)
	}
	if method, ok := expr.(*checker.ResultMethod); ok && checker.IsNever(expr.Type()) && validTypeID(&fl.l.program, expected) {
		return fl.lowerResultMethod(expected, method)
	}
	if blockExpr, ok := expr.(*checker.Block); ok && validTypeID(&fl.l.program, expected) {
		body, err := fl.lowerBlockWithDefault(blockExpr.Stmts, expected)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprBlock, Type: expected, Payload: &BlockExprPayload{Body: body}}, nil
	}
	if panicExpr, ok := expr.(*checker.Panic); ok && validTypeID(&fl.l.program, expected) {
		message, err := fl.lowerExprWithExpected(panicExpr.Message, fl.l.mustIntern(checker.Str))
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprPanic, Type: expected, Target: message}, nil
	}
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
			if expectedInfo.Kind == TypeList || expectedInfo.Kind == TypeFixedArray {
				return fl.lowerListLiteral(expected, list, expectedInfo.Elem)
			}
			// A list literal against a named Go slice or array type keeps the
			// named type so the composite literal is spelled with it.
			if expectedInfo.Kind == TypeForeignType && !expectedInfo.ForeignPointer && expectedInfo.Elem != NoType {
				return fl.lowerListLiteral(expected, list, expectedInfo.Elem)
			}
			if expectedInfo.Kind == TypeForeignType && !expectedInfo.ForeignPointer && expectedInfo.Value != NoType {
				if underlyingInfo, ok := fl.l.typeInfo(expectedInfo.Value); ok && underlyingInfo.Kind == TypeFixedArray {
					return fl.lowerListLiteral(expected, list, underlyingInfo.Elem)
				}
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
		if closure.LocalNamed {
			return fl.lowerLocalNamedFunctionReference(closure)
		}
		if expectedInfo, hasInfo := fl.l.typeInfo(expected); hasInfo {
			if expectedInfo.Kind == TypeFunction {
				return fl.lowerClosure(expected, closure)
			}
			// A named Go func type carries its signature as the foreign
			// type's underlying function (interned into Value with no Key or
			// Elem, distinguishing it from named Go map and slice types).
			// Lower the closure against that signature; Go's
			// unnamed-to-named assignability accepts the literal at the
			// boundary.
			if expectedInfo.Kind == TypeForeignType && !expectedInfo.ForeignPointer && expectedInfo.Value != NoType && expectedInfo.Key == NoType && expectedInfo.Elem == NoType {
				if underlyingInfo, ok := fl.l.typeInfo(expectedInfo.Value); ok && underlyingInfo.Kind == TypeFunction {
					return fl.lowerClosure(expectedInfo.Value, closure)
				}
			}
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
		if errorConstructor(call) {
			return fl.lowerErrorConstructor(expected, call)
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

func (fl *functionLowerer) isVoidType(typeID TypeID) bool {
	if !validTypeID(&fl.l.program, typeID) {
		return false
	}
	info, _ := fl.l.typeInfo(typeID)
	return info.Kind == TypeVoid
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
	actual, err := fl.internResolvedType(expr.Type())
	if err != nil {
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
			return &Expr{Kind: ExprUnionWrap, Type: expected, Target: value, Payload: &TagExprPayload{Tag: member.Tag}}, true, nil
		}
	}
	return nil, false, nil
}

func referencePlaceRootLocal(expr *Expr) (LocalID, bool) {
	if expr == nil {
		return 0, false
	}
	switch expr.Kind {
	case ExprLoadLocal:
		payload := exprPayloadAs[*LocalExprPayload](expr)
		if payload == nil {
			return 0, false
		}
		return payload.Local, true
	case ExprGetField, ExprForeignFieldAccess:
		return referencePlaceRootLocal(expr.Target)
	default:
		return 0, false
	}
}

func lowerReferenceMode(mode checker.ReferenceMode) (ReferenceMode, error) {
	switch mode {
	case checker.ExistingReference:
		return ExistingReference, nil
	case checker.AddressablePlace:
		return AddressablePlace, nil
	case checker.FreshValue:
		return FreshValue, nil
	default:
		return InvalidReferenceMode, fmt.Errorf("unsupported checker reference mode %d", mode)
	}
}

func (fl *functionLowerer) lowerReferenceTraitProjection(typeID TypeID, projection *checker.ReferenceTraitProjection) (*Expr, error) {
	expectedRef, ok := fl.l.typeInfo(typeID)
	if !ok || expectedRef.Kind != TypeReference {
		return nil, fmt.Errorf("trait reference projection has non-reference destination %d", typeID)
	}
	traitInfo, ok := fl.l.typeInfo(expectedRef.Elem)
	if !ok || traitInfo.Kind != TypeTraitObject {
		return nil, fmt.Errorf("trait reference projection has non-trait referent %d", expectedRef.Elem)
	}
	actualRefID, err := fl.internType(projection.Value.Type())
	if err != nil {
		return nil, err
	}
	actualRef, ok := fl.l.typeInfo(actualRefID)
	if !ok || actualRef.Kind != TypeReference {
		return nil, fmt.Errorf("trait reference projection source has non-reference type %d", actualRefID)
	}
	if err := fl.l.ensureModuleImportTraitImplsDeclared(fl.fn.Module); err != nil {
		return nil, err
	}
	if actualInfo, ok := fl.l.typeInfo(actualRef.Elem); ok && actualInfo.ModulePath != "" {
		if err := fl.l.ensureModuleTraitImplsDeclared(actualInfo.ModulePath); err != nil {
			return nil, err
		}
	}
	impl, ok := fl.l.lookupImpl(traitInfo.Trait, actualRef.Elem)
	if !ok {
		var declareErr error
		impl, ok, declareErr = fl.l.declareBuiltinTraitImpl(fl.fn.Module, traitInfo.Trait, actualRef.Elem)
		if declareErr != nil {
			return nil, declareErr
		}
	}
	if !ok {
		return nil, fmt.Errorf("missing trait impl %d for reference referent %d", traitInfo.Trait, actualRef.Elem)
	}
	value, err := fl.lowerExpr(projection.Value)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: ExprTraitRefProject, Type: typeID, Target: value, Payload: &TraitExprPayload{Impl: impl, Trait: traitInfo.Trait}}, nil
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
	actualImplType := actual
	actualInfo, _ := fl.l.typeInfo(actual)
	if actualInfo.Kind == TypeReference {
		actualImplType = actualInfo.Elem
		if actualImplType == expected {
			value, err := fl.lowerExpr(expr)
			if err != nil {
				return nil, true, err
			}
			return &Expr{Kind: ExprTraitUpcast, Type: expected, Target: value, Payload: &TraitExprPayload{Impl: -1, Trait: expectedInfo.Trait}}, true, nil
		}
	}
	if err := fl.l.ensureModuleImportTraitImplsDeclared(fl.fn.Module); err != nil {
		return nil, false, err
	}
	if actualInfo, ok := fl.l.typeInfo(actualImplType); ok && actualInfo.ModulePath != "" {
		if err := fl.l.ensureModuleTraitImplsDeclared(actualInfo.ModulePath); err != nil {
			return nil, false, err
		}
	}
	impl, ok := fl.l.lookupImpl(expectedInfo.Trait, actualImplType)
	if !ok {
		var err error
		impl, ok, err = fl.l.declareBuiltinTraitImpl(fl.fn.Module, expectedInfo.Trait, actualImplType)
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
	return &Expr{Kind: ExprTraitUpcast, Type: expected, Target: value, Payload: &TraitExprPayload{Impl: impl, Trait: expectedInfo.Trait}}, true, nil
}

func localNamedFunctionDeclaration(expr checker.Expression) *checker.FunctionDef {
	switch expr := expr.(type) {
	case *checker.FunctionDef:
		if expr.LocalNamed {
			return expr
		}
	case *checker.InterfaceConversion:
		return localNamedFunctionDeclaration(expr.Value)
	case *checker.ReferenceTraitProjection:
		return localNamedFunctionDeclaration(expr.Value)
	case *checker.DiscardingFunctionCoercion:
		return localNamedFunctionDeclaration(expr.Value)
	}
	return nil
}

func (fl *functionLowerer) lowerLocalNamedFunctionReference(def *checker.FunctionDef) (*Expr, error) {
	if def == nil || !def.LocalNamed {
		return nil, fmt.Errorf("expected local named function declaration")
	}
	local, ok, err := fl.resolveLocal(def.Name)
	if err != nil {
		return nil, err
	}
	if !ok || fl.localKind(local) != TypeFunction {
		return nil, fmt.Errorf("local function declaration %s reached expression lowering without a binding", def.Name)
	}
	return loadLocal(fl.fn.Locals[local].Type, local), nil
}

func (fl *functionLowerer) lowerLocalNamedFunction(def *checker.FunctionDef) (*Stmt, *Expr, error) {
	if def == nil || !def.LocalNamed {
		return nil, nil, fmt.Errorf("expected local named function declaration")
	}
	if len(def.CallGenericParams) > 0 || fl.l.functionHasUnresolvedTypeVar(def) {
		return nil, nil, fmt.Errorf("generic local function %s reached AIR", def.Name)
	}
	typeID, err := fl.internType(def.Type())
	if err != nil {
		return nil, nil, err
	}
	local := fl.defineLocal(def.Name, typeID, false)
	value, recursive, err := fl.lowerClosureWithSelf(typeID, def, local)
	if err != nil {
		return nil, nil, err
	}
	binding := &Stmt{
		Kind:          StmtLet,
		Local:         local,
		Name:          def.Name,
		Type:          typeID,
		Predeclare:    recursive,
		LocalFunction: true,
		Value:         value,
	}
	return binding, loadLocal(typeID, local), nil
}

func (fl *functionLowerer) lowerStmt(stmt checker.Statement) (*Stmt, error) {
	if stmt.Break {
		return &Stmt{Kind: StmtBreak}, nil
	}
	if stmt.Expr != nil {
		if def, ok := stmt.Expr.(*checker.FunctionDef); ok && def.LocalNamed {
			binding, _, err := fl.lowerLocalNamedFunction(def)
			return binding, err
		}
		expr, err := fl.lowerExpr(stmt.Expr)
		if err != nil {
			return nil, err
		}
		return &Stmt{Kind: StmtExpr, Expr: expr}, nil
	}
	switch s := stmt.Stmt.(type) {
	case *checker.VariableDef:
		typeID, err := fl.internResolvedType(s.Type())
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
	case *checker.Defer:
		if s.Expr != nil {
			expr, err := fl.lowerExpr(s.Expr)
			if err != nil {
				return nil, err
			}
			return &Stmt{Kind: StmtDefer, Expr: expr}, nil
		}
		if s.Body == nil {
			return nil, fmt.Errorf("defer missing expression or body")
		}
		voidType, err := fl.l.internType(checker.Void)
		if err != nil {
			return nil, err
		}
		body, err := fl.lowerBlockWithDefault(s.Body.Stmts, voidType)
		if err != nil {
			return nil, err
		}
		return &Stmt{Kind: StmtDefer, Body: body}, nil
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
			fl.markCaptureSlot(local)
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
		stmts = append(stmts, Stmt{Kind: StmtLet, Local: indexCounter, Name: loop.Index + "$range", Type: intType, Mutable: true, Value: &Expr{Kind: ExprConstInt, Type: intType, Payload: &TextExprPayload{Value: "0"}}})
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
		Value: &Expr{Kind: ExprIntAdd, Type: intType, Payload: &BinaryExprPayload{Left: loadLocal(intType, counter), Right: &Expr{Kind: ExprConstInt, Type: intType, Payload: &TextExprPayload{Value: "1"}}}},
	})
	if loop.Index != "" {
		body.Stmts = append(body.Stmts, Stmt{
			Kind:  StmtAssign,
			Local: indexCounter,
			Value: &Expr{Kind: ExprIntAdd, Type: intType, Payload: &BinaryExprPayload{Left: loadLocal(intType, indexCounter), Right: &Expr{Kind: ExprConstInt, Type: intType, Payload: &TextExprPayload{Value: "1"}}}},
		})
	}

	stmts = append(stmts, Stmt{
		Kind:      StmtWhile,
		Condition: &Expr{Kind: ExprLte, Type: boolType, Payload: &BinaryExprPayload{Left: loadLocal(intType, counter), Right: loadLocal(intType, endLocal)}},
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
		{Kind: StmtLet, Local: index, Name: indexName, Type: intType, Mutable: true, Value: &Expr{Kind: ExprConstInt, Type: intType, Payload: &TextExprPayload{Value: "0"}}},
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
		Value: &Expr{Kind: ExprIntAdd, Type: intType, Payload: &BinaryExprPayload{Left: loadLocal(intType, index), Right: &Expr{Kind: ExprConstInt, Type: intType, Payload: &TextExprPayload{Value: "1"}}}},
	})

	stmts = append(stmts, Stmt{
		Kind:      StmtWhile,
		Condition: &Expr{Kind: ExprLt, Type: boolType, Payload: &BinaryExprPayload{Left: loadLocal(intType, index), Right: &Expr{Kind: ExprListSize, Type: intType, Target: loadLocal(runeListType, runesLocal)}}},
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
	if !ok || (listType.Kind != TypeList && listType.Kind != TypeSlice && listType.Kind != TypeFixedArray) {
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
		{Kind: StmtLet, Local: index, Name: indexName, Type: intType, Mutable: true, Value: &Expr{Kind: ExprConstInt, Type: intType, Payload: &TextExprPayload{Value: "0"}}},
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
		Value: &Expr{Kind: ExprIntAdd, Type: intType, Payload: &BinaryExprPayload{Left: loadLocal(intType, index), Right: &Expr{Kind: ExprConstInt, Type: intType, Payload: &TextExprPayload{Value: "1"}}}},
	})

	stmts = append(stmts, Stmt{
		Kind:      StmtWhile,
		Condition: &Expr{Kind: ExprLt, Type: boolType, Payload: &BinaryExprPayload{Left: loadLocal(intType, index), Right: &Expr{Kind: ExprListSize, Type: intType, Target: loadLocal(list.Type, listLocal)}}},
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

func (fl *functionLowerer) lowerFunctionTypeCall(name string, args []checker.Expression, target *Expr, tailSpread bool) (*Expr, error) {
	if target == nil {
		return nil, fmt.Errorf("function value call %s missing target", name)
	}
	functionTypeID, ok := fl.functionTypeIDForCallable(target.Type)
	if !ok {
		return nil, fmt.Errorf("%s is not a function", name)
	}
	loweredArgs, err := fl.lowerArgsForFunctionType(args, functionTypeID, tailSpread)
	if err != nil {
		return nil, err
	}
	typeInfo, _ := fl.l.typeInfo(functionTypeID)
	spreadElement := NoType
	spreadCallable := NoType
	if tailSpread {
		if !typeInfo.Variadic || len(typeInfo.Params) == 0 {
			return nil, fmt.Errorf("function value spread call %s is not variadic", name)
		}
		spreadElement = typeInfo.Params[len(typeInfo.Params)-1]
		spreadCallable = functionTypeID
	}
	return &Expr{Kind: ExprCallClosure, Type: typeInfo.Return, Target: target, Args: loweredArgs, Payload: &CallExprPayload{Spread: newSpreadExprPayload(tailSpread, spreadElement, spreadCallable)}}, nil
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
	selectPayload := &SelectExprPayload{}
	result := &Expr{Kind: ExprSelect, Type: typeID, Payload: selectPayload}
	for _, arm := range sel.Arms {
		switch arm.Kind {
		case checker.SelectArmDefault:
			body, err := fl.lowerBlockWithDefault(arm.Body.Stmts, typeID)
			if err != nil {
				return nil, err
			}
			selectPayload.Cases = append(selectPayload.Cases, SelectMatchCase{Kind: SelectArmDefault, Body: body})

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
			selectPayload.Cases = append(selectPayload.Cases, armCase)

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
			selectPayload.Cases = append(selectPayload.Cases, SelectMatchCase{Kind: SelectArmSend, Channel: channel, Value: value, Body: body})
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
				return &Expr{Kind: ExprLoadGlobal, Type: fl.l.program.Globals[global].Type, Payload: &GlobalExprPayload{Global: global}}, nil
			}
			return nil, fmt.Errorf("unknown local %s", e.Name)
		}
		return &Expr{Kind: ExprLoadLocal, Type: fl.fn.Locals[local].Type, Payload: &LocalExprPayload{Local: local}}, nil
	}
	if e, ok := expr.(*checker.Variable); ok {
		local, ok, err := fl.resolveLocal(e.Name())
		if err != nil {
			return nil, err
		}
		if !ok {
			if global, ok := fl.l.lookupGlobalInModule(fl.fn.Module, e.Name()); ok {
				return &Expr{Kind: ExprLoadGlobal, Type: fl.l.program.Globals[global].Type, Payload: &GlobalExprPayload{Global: global}}, nil
			}
			if declaration := e.Declaration(); declaration != nil {
				if declaration.LocalNamed {
					return nil, fmt.Errorf("local function reference %s has no lexical binding", declaration.Name)
				}
				functionType, err := fl.internType(e.Type())
				if err != nil {
					return nil, err
				}
				id, err := fl.l.declareAndLowerFunction(fl.fn.Module, declaration)
				if err != nil {
					return nil, err
				}
				return &Expr{Kind: ExprFunctionRef, Type: functionType, Payload: &CallExprPayload{Function: id}}, nil
			}
			return nil, fmt.Errorf("unknown local %s", e.Name())
		}
		return &Expr{Kind: ExprLoadLocal, Type: fl.fn.Locals[local].Type, Payload: &LocalExprPayload{Local: local}}, nil
	}
	if e, ok := expr.(*checker.FunctionCall); ok {
		local, ok, err := fl.resolveLocal(e.Name)
		if err != nil {
			return nil, err
		}
		if ok && fl.localKind(local) == TypeFunction {
			target := &Expr{Kind: ExprLoadLocal, Type: fl.fn.Locals[local].Type, Payload: &LocalExprPayload{Local: local}}
			return fl.lowerFunctionTypeCall(e.Name, e.Args, target, e.TailSpread)
		}
		if global, ok := fl.l.lookupGlobalInModule(fl.fn.Module, e.Name); ok {
			globalType := fl.l.program.Globals[global].Type
			if typeInfo, ok := fl.l.typeInfo(globalType); ok && typeInfo.Kind == TypeFunction {
				target := &Expr{Kind: ExprLoadGlobal, Type: globalType, Payload: &GlobalExprPayload{Global: global}}
				return fl.lowerFunctionTypeCall(e.Name, e.Args, target, e.TailSpread)
			}
		}
	}
	if prop, ok := expr.(*checker.InstanceProperty); ok {
		return fl.lowerInstanceProperty(NoType, prop)
	}
	var typeID TypeID
	var err error
	if checker.IsNever(expr.Type()) {
		typeID, err = fl.l.internType(checker.Void)
	} else {
		typeID, err = fl.internResolvedType(expr.Type())
	}
	if err != nil {
		return nil, err
	}
	switch e := expr.(type) {
	case *checker.VoidLiteral:
		return &Expr{Kind: ExprConstVoid, Type: typeID}, nil
	case *checker.IntLiteral:
		return &Expr{Kind: ExprConstInt, Type: typeID, Payload: &TextExprPayload{Value: strconv.Itoa(e.Value)}}, nil
	case *checker.TypedIntLiteral:
		return &Expr{Kind: ExprConstInt, Type: typeID, Payload: &TextExprPayload{Value: e.String()}}, nil
	case *checker.FloatLiteral:
		return &Expr{Kind: ExprConstFloat, Type: typeID, Payload: &TextExprPayload{Value: e.String()}}, nil
	case *checker.TypedFloatLiteral:
		return &Expr{Kind: ExprConstFloat, Type: typeID, Payload: &TextExprPayload{Value: e.String()}}, nil
	case *checker.BoolLiteral:
		return &Expr{Kind: ExprConstBool, Type: typeID, Payload: &BoolExprPayload{Value: e.Value}}, nil
	case *checker.StrLiteral:
		return &Expr{Kind: ExprConstStr, Type: typeID, Payload: &TextExprPayload{Value: e.Value}}, nil
	case *checker.EmbeddedText:
		blob, err := fl.l.internEmbeddedBlob(e.Resource.Data, true)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprEmbeddedText, Type: typeID, Payload: &EmbeddedBlobExprPayload{Blob: blob}}, nil
	case *checker.EmbeddedBytes:
		blob, err := fl.l.internEmbeddedBlob(e.Resource.Data, true)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprEmbeddedBytes, Type: typeID, Payload: &EmbeddedBlobExprPayload{Blob: blob}}, nil
	case *checker.EmbeddedFSValue:
		set, err := fl.l.internEmbeddedSet(e.Set)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprMakeEmbeddedFS, Type: typeID, Payload: &EmbeddedSetExprPayload{Set: set}}, nil
	case *checker.RuneLiteral:
		return &Expr{Kind: ExprConstInt, Type: typeID, Payload: &TextExprPayload{Value: strconv.Itoa(int(e.Value))}}, nil
	case *checker.NeverCoercion:
		return fl.lowerExprWithExpected(e.Value, typeID)
	case *checker.Panic:
		message, err := fl.lowerExprWithExpected(e.Message, fl.l.mustIntern(checker.Str))
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprPanic, Type: typeID, Target: message}, nil
	case *checker.TemplateStr:
		return fl.lowerTemplateStr(typeID, e)
	case *checker.FunctionDef:
		if e.LocalNamed {
			return fl.lowerLocalNamedFunctionReference(e)
		}
		return fl.lowerClosure(typeID, e)
	case *checker.FunctionValueCall:
		target, err := fl.lowerExpr(e.Callee)
		if err != nil {
			return nil, err
		}
		return fl.lowerFunctionTypeCall("function value", e.Args, target, e.TailSpread)
	case *checker.FunctionCall:
		if local, ok := fl.locals[e.Name]; ok {
			if _, callable := fl.functionTypeIDForCallable(fl.fn.Locals[local].Type); callable {
				target := &Expr{Kind: ExprLoadLocal, Type: fl.fn.Locals[local].Type, Payload: &LocalExprPayload{Local: local}}
				return fl.lowerFunctionTypeCall(e.Name, e.Args, target, e.TailSpread)
			}
		}
		if global, ok := fl.l.lookupGlobalInModule(fl.fn.Module, e.Name); ok {
			globalType := fl.l.program.Globals[global].Type
			if typeInfo, ok := fl.l.typeInfo(globalType); ok && typeInfo.Kind == TypeFunction {
				target := &Expr{Kind: ExprLoadGlobal, Type: globalType, Payload: &GlobalExprPayload{Global: global}}
				return fl.lowerFunctionTypeCall(e.Name, e.Args, target, e.TailSpread)
			}
		}
		if local, hasLocal, err := fl.resolveLocal(e.Name); err != nil {
			return nil, err
		} else if hasLocal && fl.localKind(local) == TypeFunction {
			target := &Expr{Kind: ExprLoadLocal, Type: fl.fn.Locals[local].Type, Payload: &LocalExprPayload{Local: local}}
			return fl.lowerFunctionTypeCall(e.Name, e.Args, target, e.TailSpread)
		}
		if declaration := e.Declaration(); declaration != nil {
			if declaration.LocalNamed {
				return nil, fmt.Errorf("local function call %s has no lexical binding", declaration.Name)
			}
			if declaration.Body == nil {
				return nil, fmt.Errorf("function call target %s has no checked body", declaration.Name)
			}
			id, err := fl.declareAndLowerFunctionCall(fl.fn.Module, declaration, e)
			if err != nil {
				return nil, err
			}
			return fl.buildResolvedCallExpr(id, e, e.Args)
		}
		return nil, fmt.Errorf("unresolved function call %s has no declaration", e.Name)
	case *checker.ForeignValue:
		var argABI []ABIParamMode
		if functionDef, ok := e.Type().(*checker.FunctionDef); ok {
			argABI = lowerABIParamModes(functionDef.Parameters, len(functionDef.Parameters))
		}
		return &Expr{Kind: ExprForeignValue, Type: typeID, Payload: &ForeignExprPayload{Target: e.Target, Namespace: e.Namespace, Qualifier: e.Qualifier, Symbol: e.Symbol, ResultShape: lowerForeignResultShape(e.ForeignResultShape), ArgABI: argABI}}, nil
	case *checker.InterfaceConversion:
		value, err := fl.lowerExpr(e.Value)
		if err != nil {
			return nil, err
		}
		mode := InterfaceValue
		switch e.Mode {
		case checker.InterfaceValue:
		case checker.InterfaceOwnedPointer:
			mode = InterfaceOwnedPointer
		case checker.InterfaceReference:
			mode = InterfaceReference
		default:
			return nil, fmt.Errorf("unsupported checker interface conversion mode %d", e.Mode)
		}
		return &Expr{Kind: ExprInterfaceConversion, Type: typeID, Target: value, Payload: &InterfaceExprPayload{Mode: mode}}, nil
	case *checker.DiscardingFunctionCoercion:
		value, err := fl.lowerExpr(e.Value)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprDiscardingFunctionCoercion, Type: typeID, Target: value}, nil
	case *checker.ForeignStructInstance:
		fields := make([]StructFieldValue, 0, len(e.Fields))
		fieldNames := make([]string, 0, len(e.Fields))
		for name := range e.Fields {
			fieldNames = append(fieldNames, name)
		}
		sort.Strings(fieldNames)
		for _, name := range fieldNames {
			value, err := fl.lowerExpr(e.Fields[name])
			if err != nil {
				return nil, err
			}
			fields = append(fields, StructFieldValue{Name: name, Value: *value})
		}
		return &Expr{Kind: ExprForeignStructInstance, Type: typeID, Payload: &ForeignExprPayload{Target: e.Target, Namespace: e.Namespace, Qualifier: e.Qualifier, Symbol: e.Name, Fields: fields}}, nil
	case *checker.ForeignScalarConvert:
		if !checker.ValidForeignScalarConversion(e.Value.Type(), e.Target) {
			return nil, fmt.Errorf("unsupported foreign scalar conversion: %s -> %s", e.Value.Type().String(), e.Target.String())
		}
		target, err := fl.lowerExpr(e.Value)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprScalarConvert, Type: typeID, Target: target}, nil
	case *checker.ScalarFrom:
		// Tiered numeric conversion (#284, #500, ADR 0072). `from` is lossless
		// and lowers to Go's T(x); `try` and `fit` carry their own kinds so the
		// backend can emit the checked and saturating forms. For `try`, typeID
		// is already the Maybe wrapping the target.
		value, err := fl.lowerExpr(e.Value)
		if err != nil {
			return nil, err
		}
		kind := ExprScalarConvert
		switch e.Tier {
		case checker.ConversionTry:
			kind = ExprScalarTryConvert
		case checker.ConversionFit:
			kind = ExprScalarFitConvert
		}
		return &Expr{Kind: kind, Type: typeID, Target: value}, nil
	case *checker.ForeignFieldAccess:
		target, err := fl.lowerExpr(e.Subject)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprForeignFieldAccess, Type: typeID, Target: target, Payload: &ForeignExprPayload{Target: e.Target, Symbol: e.Symbol}}, nil
	case *checker.ForeignMethodValue:
		target, err := fl.lowerExpr(e.Subject)
		if err != nil {
			return nil, err
		}
		var argABI []ABIParamMode
		if methodDef, ok := e.Type().(*checker.FunctionDef); ok {
			argABI = lowerABIParamModes(methodDef.Parameters, len(methodDef.Parameters))
		}
		return &Expr{Kind: ExprForeignMethodValue, Type: typeID, Target: target, Payload: &ForeignExprPayload{Target: e.Target, Namespace: e.Namespace, Qualifier: e.Qualifier, Receiver: e.Receiver, Pointer: e.Pointer, Symbol: e.Symbol, ResultShape: lowerForeignResultShape(e.ForeignResultShape), ArgABI: argABI}}, nil
	case *checker.ForeignMethodCall:
		target, err := fl.lowerExpr(e.Subject)
		if err != nil {
			return nil, err
		}
		args := make([]Expr, len(e.Call.Args))
		methodDef := e.Call.Signature()
		for i, arg := range e.Call.Args {
			var lowered *Expr
			var err error
			if e.Call.TailSpread && i == len(e.Call.Args)-1 {
				lowered, err = fl.lowerExpr(arg)
			} else if methodDef != nil && i < len(methodDef.Parameters) && methodDef.Parameters[i].Type != nil {
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
		var argABI []ABIParamMode
		if methodDef != nil {
			argABI = lowerABIParamModes(methodDef.Parameters, len(args))
		}
		spreadElement, err := fl.spreadElementTypeForCall(e.Call)
		if err != nil {
			return nil, err
		}
		spreadCallable, err := fl.spreadCallableTypeForCall(e.Call)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprForeignMethodCall, Type: typeID, Target: target, Args: args, Payload: &ForeignExprPayload{Target: e.Target, Namespace: e.Namespace, Qualifier: e.Qualifier, Receiver: e.Receiver, Pointer: e.Pointer, Symbol: e.Symbol, ResultShape: lowerForeignResultShape(e.ForeignResultShape), ArgABI: argABI, Spread: newSpreadExprPayload(e.Call.TailSpread, spreadElement, spreadCallable)}}, nil
	case *checker.UnsafeCast:
		value, err := fl.lowerExprWithExpected(e.Value, fl.l.mustIntern(checker.Any))
		if err != nil {
			return nil, err
		}
		targetType := e.TargetType
		mutable := false
		if ref, ok := targetType.(*checker.MutableRef); ok {
			targetType = ref.Of()
			_, trait := targetType.(*checker.Trait)
			mutable = !trait
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
		targetTypeID, err := fl.internType(targetType)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprUnsafeCast, Type: typeID, Target: value, Payload: &UnsafeCastExprPayload{TargetType: targetTypeID, Pointer: mutable}}, nil
	case *checker.UnsafeIsNil:
		value, err := fl.lowerExprWithExpected(e.Value, fl.l.mustIntern(checker.Any))
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprUnsafeIsNil, Type: typeID, Target: value}, nil
	case *checker.MutableRefExpr:
		operand, err := fl.lowerExpr(e.Operand)
		if err != nil {
			return nil, err
		}
		mode, err := lowerReferenceMode(e.Mode)
		if err != nil {
			return nil, err
		}
		if mode == AddressablePlace {
			if root, ok := referencePlaceRootLocal(operand); ok {
				fl.markCaptureSlot(root)
			}
		}
		return &Expr{Kind: ExprMutRef, Type: typeID, Target: operand, Payload: &ReferenceExprPayload{Mode: mode}}, nil
	case *checker.DerefExpr:
		operand, err := fl.lowerExpr(e.Operand)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprDeref, Type: typeID, Target: operand, Payload: &ReferenceExprPayload{Observational: e.Observational}}, nil
	case *checker.ReferenceTraitProjection:
		return fl.lowerReferenceTraitProjection(typeID, e)
	case *checker.ForeignFunctionCall:
		if e.PointerResult && fl.directLetValue != checker.Expression(e) {
			return nil, fmt.Errorf("a Go call returning %s must be bound directly with let", e.Call.Type())
		}
		args := make([]Expr, len(e.Call.Args))
		fnDef := e.Call.Signature()
		for i, arg := range e.Call.Args {
			var lowered *Expr
			if e.Call.TailSpread && i == len(e.Call.Args)-1 {
				lowered, err = fl.lowerExpr(arg)
			} else {
				var paramType TypeID
				paramType, err = fl.internResolvedType(fnDef.Parameters[i].Type)
				if err == nil {
					lowered, err = fl.lowerExprWithExpected(arg, paramType)
				}
			}
			if err != nil {
				return nil, err
			}
			args[i] = *lowered
		}
		var typeArgs []TypeID
		for _, typeArg := range e.TypeArgs {
			argID, err := fl.internResolvedType(typeArg)
			if err != nil {
				return nil, err
			}
			typeArgs = append(typeArgs, argID)
		}
		argABI := lowerABIParamModes(fnDef.Parameters, len(args))
		spreadElement, err := fl.spreadElementTypeForCall(e.Call)
		if err != nil {
			return nil, err
		}
		spreadCallable, err := fl.spreadCallableTypeForCall(e.Call)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprForeignCall, Type: typeID, Args: args, Payload: &ForeignExprPayload{Target: e.Target, Namespace: e.Namespace, Qualifier: e.Qualifier, Symbol: e.Symbol, TypeArgs: typeArgs, Pointer: e.PointerResult, ResultShape: lowerForeignResultShape(e.ForeignResultShape), ArgABI: argABI, Spread: newSpreadExprPayload(e.Call.TailSpread, spreadElement, spreadCallable)}}, nil
	case *checker.ModuleFunctionCall:
		if kind, ok := resultConstructorKind(e); ok {
			return fl.lowerResultConstructor(kind, typeID, e)
		}
		if kind, ok := maybeConstructorKind(e); ok {
			return fl.lowerMaybeConstructor(kind, typeID, e)
		}
		if errorConstructor(e) {
			return fl.lowerErrorConstructor(typeID, e)
		}
		if e.Module == "ard/list" && e.Call.Name == "new" {
			if len(e.Call.Args) != 1 {
				return nil, fmt.Errorf("ard/list::new expects one checked argument")
			}
			size, err := fl.lowerExpr(e.Call.Args[0])
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: ExprMakeListSized, Type: typeID, Args: []Expr{*size}}, nil
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
		moduleID := fl.l.internModule(e.Module)
		if err := fl.l.ensureModuleGlobalsDeclared(e.Module); err != nil {
			return nil, err
		}
		if err := fl.l.ensureModuleTraitImplsDeclared(e.Module); err != nil {
			return nil, err
		}
		if global, ok := fl.l.lookupGlobalInModule(moduleID, e.Call.Name); ok {
			globalType := fl.l.program.Globals[global].Type
			if _, callable := fl.functionTypeIDForCallable(globalType); callable {
				target := &Expr{Kind: ExprLoadGlobal, Type: globalType, Payload: &GlobalExprPayload{Global: global}}
				return fl.lowerFunctionTypeCall(e.Call.Name, e.Call.Args, target, e.Call.TailSpread)
			}
		}
		declaration := e.Call.Declaration()
		if declaration == nil {
			return nil, fmt.Errorf("module function call %s::%s has no declaration", e.Module, e.Call.Name)
		}
		if declaration.Body == nil {
			return nil, fmt.Errorf("module function call target %s::%s has no checked body", e.Module, declaration.Name)
		}
		id, err := fl.declareAndLowerFunctionCall(moduleID, declaration, e.Call)
		if err != nil {
			return nil, err
		}
		return fl.buildResolvedCallExpr(id, e.Call, e.Call.Args)
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
	case *checker.EmbeddedFSMethod:
		target, err := fl.lowerExpr(e.Subject)
		if err != nil {
			return nil, err
		}
		args, err := fl.lowerArgs(e.Args)
		if err != nil {
			return nil, err
		}
		kind := ExprEmbeddedFSReadFile
		switch e.Kind {
		case checker.EmbeddedFSReadText:
			kind = ExprEmbeddedFSReadText
		case checker.EmbeddedFSReadDir:
			kind = ExprEmbeddedFSReadDir
		case checker.EmbeddedFSStat:
			kind = ExprEmbeddedFSStat
		case checker.EmbeddedFSSub:
			kind = ExprEmbeddedFSSub
		}
		return &Expr{Kind: kind, Type: typeID, Target: target, Args: args}, nil
	case *checker.ByteMethod:
		if e.Kind == checker.ByteToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
		}
		return nil, fmt.Errorf("unsupported AIR Byte method %d", e.Kind)
	case *checker.RuneMethod:
		if e.Kind == checker.RuneToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
		}
		return nil, fmt.Errorf("unsupported AIR Rune method %d", e.Kind)
	case *checker.IntMethod:
		if e.Kind == checker.IntToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
		}
		return nil, fmt.Errorf("unsupported AIR Int method %d", e.Kind)
	case *checker.ScalarMethod:
		if e.Kind == checker.ScalarToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
		}
		return nil, fmt.Errorf("unsupported AIR scalar method %d", e.Kind)
	case *checker.FloatMethod:
		if e.Kind == checker.FloatToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
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
		return &Expr{Kind: ExprEnumVariant, Type: typeID, Payload: &EnumExprPayload{Variant: int(e.Variant), Discriminant: e.Discriminant}}, nil
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
		return &Expr{Kind: ExprBlock, Type: typeID, Payload: &BlockExprPayload{Body: body}}, nil
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
		return &Expr{Kind: ExprUnsafeBlock, Type: resultType, Payload: &BlockExprPayload{Body: body}}, nil
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
		return &Expr{Kind: ExprBlock, Type: typeID, Payload: &BlockExprPayload{Body: body}}, nil
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
		Kind: ExprIf,
		Type: typeID,
		Payload: &IfExprPayload{
			Condition: condition,
			Then:      thenBlock,
			Else:      Block{Result: elseExpr},
		},
	}, nil
}

func (fl *functionLowerer) lowerIf(typeID TypeID, expr *checker.If) (*Expr, error) {
	if len(expr.Branches) == 0 {
		return nil, fmt.Errorf("if expression missing branches")
	}
	// Defense in depth for issue #267: the checker types an else-less if
	// chain as Void, so one can never reach lowering with a value type. The
	// Go backend would materialize the missing path as a zero value, so fail
	// loudly instead of generating it.
	if expr.Else == nil && typeID != NoType && !fl.isVoidType(typeID) {
		return nil, fmt.Errorf("internal: value-position if without else reached lowering")
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
	return &Expr{Kind: ExprIf, Type: typeID, Payload: &IfExprPayload{Condition: condition, Then: thenBlock, Else: airElse}}, nil
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

func (fl *functionLowerer) lowerErrorConstructor(typeID TypeID, call *checker.ModuleFunctionCall) (*Expr, error) {
	if len(call.Call.Args) != 1 {
		return nil, fmt.Errorf("Error::new expects one argument")
	}
	message, err := fl.lowerExpr(call.Call.Args[0])
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: ExprMakeError, Type: typeID, Target: message}, nil
}

func (fl *functionLowerer) lowerMaybeConstructor(kind ExprKind, typeID TypeID, call *checker.ModuleFunctionCall) (*Expr, error) {
	switch kind {
	case ExprMakeMaybeNew:
		if len(call.Call.Args) != 1 {
			return nil, fmt.Errorf("%s::%s expects one checked argument", call.Module, call.Call.Name)
		}
		return fl.lowerExprWithExpected(call.Call.Args[0], typeID)
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
	return &Expr{Kind: ExprIf, Type: typeID, Payload: &IfExprPayload{Condition: condition, Then: trueBlock, Else: falseBlock}}, nil
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
	kind := ExprMakeList
	if info, ok := fl.l.typeInfo(typeID); ok && info.Kind == TypeFixedArray {
		kind = ExprMakeFixedArray
	}
	return &Expr{Kind: kind, Type: typeID, Args: args}, nil
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
	return &Expr{Kind: ExprMakeMap, Type: typeID, Payload: &AggregateExprPayload{Entries: entries}}, nil
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

	return &Expr{Kind: ExprMatchEnum, Type: typeID, Target: subject, Payload: &EnumMatchExprPayload{Cases: cases, CatchAll: catchAll}}, nil
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

	return &Expr{Kind: ExprMatchInt, Type: typeID, Target: subject, Payload: &IntMatchExprPayload{Cases: intCases, RangeCases: rangeCases, CatchAll: catchAll}}, nil
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

	return &Expr{Kind: ExprMatchStr, Type: typeID, Target: subject, Payload: &StrMatchExprPayload{Cases: strCases, CatchAll: catchAll}}, nil
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

	cases := make([]UnionMatchCase, 0, len(match.TypeCasesByIndex))
	for memberIndex, member := range unionType.Members {
		matchCase := match.TypeCasesByIndex[memberIndex]
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

	return &Expr{Kind: ExprMatchUnion, Type: typeID, Target: subject, Payload: &UnionMatchExprPayload{Cases: cases, CatchAll: catchAll}}, nil
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
	return &Expr{Kind: ExprMatchForeignType, Type: typeID, Target: subject, Payload: &ForeignMatchExprPayload{Cases: cases, CatchAll: catchAll}}, nil
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
	if elem, ok := fl.l.typeInfo(maybeType.Elem); ok {
		fl.fn.Locals[someLocal].Reference = elem.Kind == TypeReference || (elem.Kind == TypeForeignType && elem.ForeignPointer)
	}
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

	return &Expr{Kind: ExprMatchMaybe, Type: typeID, Target: subject, Payload: &MaybeMatchExprPayload{SomeLocal: someLocal, Some: someBlock, None: noneBlock}}, nil
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

	returnsReference := false
	if maybeType, ok := fl.l.referentTypeInfo(target.Type); ok && maybeType.Kind == TypeMaybe {
		if elem, ok := fl.l.typeInfo(maybeType.Elem); ok {
			returnsReference = elem.Kind == TypeReference
		}
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
	case checker.MaybeSet:
		kind = ExprMaybeSet
	case checker.MaybeClear:
		kind = ExprMaybeClear
	default:
		return nil, fmt.Errorf("unsupported AIR Maybe method %d", method.Kind)
	}
	return &Expr{Kind: kind, Type: typeID, Target: target, Args: args, Payload: &MaybeCallExprPayload{ReturnsReference: returnsReference && (method.Kind == checker.MaybeExpect || method.Kind == checker.MaybeOr)}}, nil
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
	maybeIntType, err := fl.l.internType(checker.MakeMaybe(checker.Int))
	if err != nil {
		return nil, err
	}

	var kind ExprKind
	var expected []TypeID
	switch method.Kind {
	case checker.StrAt:
		kind = ExprStrAt
		expected = []TypeID{intType}
	case checker.StrSlice:
		kind = ExprStrSlice
		expected = []TypeID{maybeIntType, maybeIntType}
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
	return &Expr{Kind: kind, Type: typeID, Target: target, Args: args, Payload: &CallExprPayload{ArgOrder: append([]int(nil), method.ArgOrder...)}}, nil
}

func (fl *functionLowerer) lowerListMethod(typeID TypeID, method *checker.ListMethod) (*Expr, error) {
	target, err := fl.lowerExpr(method.Subject)
	if err != nil {
		return nil, err
	}
	listType, ok := fl.l.referentTypeInfo(target.Type)
	if !ok {
		return nil, fmt.Errorf("List method lowered with non-list subject %s", method.Subject.Type().String())
	}
	isNamedFixedArray := false
	if listType.Kind == TypeForeignType && listType.Value != NoType {
		if underlying, ok := fl.l.typeInfo(listType.Value); ok && underlying.Kind == TypeFixedArray {
			isNamedFixedArray = true
			listType.Elem = underlying.Elem
			listType.Length = underlying.Length
		}
	}
	if listType.Kind != TypeList && listType.Kind != TypeSlice && listType.Kind != TypeFixedArray && !(listType.Kind == TypeForeignType && (listType.Elem != NoType || isNamedFixedArray)) {
		return nil, fmt.Errorf("List method lowered with non-list subject %s", method.Subject.Type().String())
	}

	intType, err := fl.l.internType(checker.Int)
	if err != nil {
		return nil, err
	}
	maybeIntType, err := fl.l.internType(checker.MakeMaybe(checker.Int))
	if err != nil {
		return nil, err
	}

	var kind ExprKind
	var expected []TypeID
	if (listType.Kind == TypeFixedArray || isNamedFixedArray) && method.Kind != checker.ListAt && method.Kind != checker.ListSize {
		return nil, fmt.Errorf("unsupported fixed-array method %d", method.Kind)
	}
	switch method.Kind {
	case checker.ListAt:
		kind = ExprListAtChecked
		expected = []TypeID{intType}
	case checker.ListSlice:
		kind = ExprListSlice
		expected = []TypeID{maybeIntType, maybeIntType}
	case checker.ListIsEmpty:
		kind = ExprListIsEmpty
	case checker.ListToList:
		kind = ExprListToList
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
	return &Expr{Kind: kind, Type: typeID, Target: target, Args: args, Payload: &CallExprPayload{ArgOrder: append([]int(nil), method.ArgOrder...)}}, nil
}

func (fl *functionLowerer) lowerMapMethod(typeID TypeID, method *checker.MapMethod) (*Expr, error) {
	target, err := fl.lowerExpr(method.Subject)
	if err != nil {
		return nil, err
	}
	mapType, ok := fl.l.referentTypeInfo(target.Type)
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
	case checker.MapDelete:
		kind = ExprMapDelete
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

	return &Expr{Kind: ExprMatchResult, Type: typeID, Target: subject, Payload: &ResultMatchExprPayload{OkLocal: okLocal, ErrLocal: errLocal, Ok: okBlock, Err: errBlock}}, nil
}

func (fl *functionLowerer) lowerResultMethod(typeID TypeID, method *checker.ResultMethod) (*Expr, error) {
	subjectType, err := fl.resultMethodSubjectType(method)
	if err != nil {
		return nil, err
	}
	target, err := fl.lowerExprWithExpected(method.Subject, subjectType)
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

func (fl *functionLowerer) resultMethodSubjectType(method *checker.ResultMethod) (TypeID, error) {
	subjectType, ok := method.Subject.Type().(*checker.Result)
	if !ok {
		return NoType, fmt.Errorf("Result method subject has type %T", method.Subject.Type())
	}
	return fl.internResolvedType(subjectType)
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
	expr := &Expr{Kind: kind, Type: typeID, Target: target}
	if op.CatchBlock == nil {
		return expr, nil
	}

	catchPayload := &TryExprPayload{CatchLocal: -1}
	expr.Payload = catchPayload
	if op.Kind == checker.TryResult {
		errType, err := fl.internType(op.ErrType)
		if err != nil {
			return nil, err
		}
		catchLocal, catchBlock, err := fl.lowerBoundBlock(op.CatchVar, errType, op.CatchBlock.Stmts)
		if err != nil {
			return nil, err
		}
		catchPayload.CatchLocal = catchLocal
		catchPayload.Catch = catchBlock
		return expr, nil
	}

	catchBlock, err := fl.lowerBlock(op.CatchBlock.Stmts)
	if err != nil {
		return nil, err
	}
	catchPayload.Catch = catchBlock
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

func errorConstructor(call *checker.ModuleFunctionCall) bool {
	return call.Module == "builtin/Error" && call.Call.Name == "new"
}

func maybeConstructorKind(call *checker.ModuleFunctionCall) (ExprKind, bool) {
	if call.Module != "builtin/Maybe" {
		return 0, false
	}
	switch call.Call.Name {
	case "new":
		return ExprMakeMaybeNew, true
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

func (fl *functionLowerer) lowerArgsForFunctionType(args []checker.Expression, typeID TypeID, tailSpread bool) ([]Expr, error) {
	typeInfo, ok := fl.l.typeInfo(typeID)
	if !ok || typeInfo.Kind != TypeFunction {
		return fl.lowerArgs(args)
	}
	expected := typeInfo.Params
	if typeInfo.Variadic && len(typeInfo.Params) > 0 && len(args) > len(typeInfo.Params) {
		expected = make([]TypeID, len(args))
		copy(expected, typeInfo.Params)
		for i := len(typeInfo.Params); i < len(expected); i++ {
			expected[i] = typeInfo.Params[len(typeInfo.Params)-1]
		}
	}
	if tailSpread && len(args) > 0 {
		expected = append([]TypeID(nil), expected...)
		expected[len(args)-1] = NoType
	}
	return fl.lowerArgsWithTypeIDs(args, expected)
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
	leftExpected, rightExpected := NoType, NoType
	if checker.IsNever(leftExpr.Type()) {
		var expected checker.Type = checker.Void
		if !checker.IsNever(rightExpr.Type()) {
			expected = rightExpr.Type()
		}
		var err error
		leftExpected, err = fl.internResolvedType(expected)
		if err != nil {
			return nil, err
		}
	}
	if checker.IsNever(rightExpr.Type()) {
		var expected checker.Type = checker.Void
		if !checker.IsNever(leftExpr.Type()) {
			expected = leftExpr.Type()
		}
		var err error
		rightExpected, err = fl.internResolvedType(expected)
		if err != nil {
			return nil, err
		}
	}
	var left, right *Expr
	var err error
	if leftExpected != NoType {
		left, err = fl.lowerExprWithExpected(leftExpr, leftExpected)
	} else {
		left, err = fl.lowerExpr(leftExpr)
	}
	if err != nil {
		return nil, err
	}
	if rightExpected != NoType {
		right, err = fl.lowerExprWithExpected(rightExpr, rightExpected)
	} else {
		right, err = fl.lowerExpr(rightExpr)
	}
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: kind, Type: typeID, Payload: &BinaryExprPayload{Left: left, Right: right}}, nil
}

func (fl *functionLowerer) lowerUnary(kind ExprKind, typeID TypeID, valueExpr checker.Expression) (*Expr, error) {
	value, err := fl.lowerExprWithExpected(valueExpr, typeID)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: kind, Type: typeID, Target: value}, nil
}

func (fl *functionLowerer) lowerTemplateStr(typeID TypeID, template *checker.TemplateStr) (*Expr, error) {
	if len(template.Chunks) == 0 {
		return &Expr{Kind: ExprConstStr, Type: typeID, Payload: &TextExprPayload{Value: ""}}, nil
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
		current = &Expr{Kind: ExprStrConcat, Type: typeID, Payload: &BinaryExprPayload{Left: current, Right: next}}
	}
	return current, nil
}

func loadLocal(typeID TypeID, local LocalID) *Expr {
	return &Expr{Kind: ExprLoadLocal, Type: typeID, Payload: &LocalExprPayload{Local: local}}
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
	return &Expr{Kind: ExprMakeStruct, Type: typeID, Payload: &AggregateExprPayload{Fields: fields}}, nil
}

func (fl *functionLowerer) lowerInstanceProperty(typeID TypeID, prop *checker.InstanceProperty) (*Expr, error) {
	target, err := fl.lowerExpr(prop.Subject)
	if err != nil {
		return nil, err
	}
	targetInfo, ok := fl.l.referentTypeInfo(target.Type)
	if !ok || targetInfo.Kind != TypeStruct {
		return nil, fmt.Errorf("property access on non-struct AIR type %s", prop.Subject.Type().String())
	}
	for _, field := range targetInfo.Fields {
		if field.Name == prop.Property {
			if !validTypeID(&fl.l.program, typeID) {
				typeID = field.Type
			}
			return &Expr{Kind: ExprGetField, Type: typeID, Target: target, Payload: &FieldExprPayload{Field: field.Index}}, nil
		}
	}
	return nil, fmt.Errorf("field %s not found on %s", prop.Property, targetInfo.Name)
}

func (fl *functionLowerer) lowerForeignFieldAssignment(prop *checker.ForeignFieldAccess, valueExpr checker.Expression) (*Stmt, error) {
	target, err := fl.lowerExpr(prop.Subject)
	if err != nil {
		return nil, err
	}
	fieldType, err := fl.internResolvedType(prop.Type())
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
	valueType, err := fl.internResolvedType(prop.Type())
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
	targetInfo, ok := fl.l.referentTypeInfo(target.Type)
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
	expr, _, err := fl.lowerClosureWithSelf(typeID, def, -1)
	return expr, err
}

func (fl *functionLowerer) lowerClosureWithSelf(typeID TypeID, def *checker.FunctionDef, selfLocal LocalID) (*Expr, bool, error) {
	keyName := fmt.Sprintf("closure/%d/%s", fl.fn.ID, def.Name)
	id, err := fl.l.declareClosureFunction(fl.fn.Module, keyName, def, typeID)
	if err != nil {
		return nil, false, err
	}
	// A closure created inside a generic definition is lifted to a top-level Go
	// function whose body references the enclosing type parameters, so it must
	// inherit them and be instantiated with them at its creation site (ADR 0031).
	if len(fl.fn.TypeParams) > 0 {
		fl.l.program.Functions[id].TypeParams = fl.fn.TypeParams
		fl.l.program.Functions[id].TypeParamOwner = fl.fn.TypeParamOwner
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
		child.defineLocal(param.Name, param.Type, false)
	}
	if def.Body != nil {
		body, err := child.lowerBlock(def.Body.Stmts)
		if err != nil {
			return nil, false, fmt.Errorf("lower closure %s: %w", def.Name, err)
		}
		fn.Body = body
	}
	fn.Captures = child.fn.Captures
	fn.Locals = child.fn.Locals
	recursive := false
	if selfLocal >= 0 {
		for index, capturedLocal := range child.captureLocals {
			if capturedLocal == selfLocal && index < len(fn.Captures) {
				fn.Captures[index].Mode = CaptureSlot
				recursive = true
			}
		}
	}
	fl.l.program.Functions[id] = fn
	var typeArgs []TypeID
	if len(fn.TypeParams) > 0 {
		typeArgs = make([]TypeID, len(fn.TypeParams))
	}
	for index, name := range fn.TypeParams {
		typeArg, ok := fl.typeVars[name]
		if !ok {
			return nil, false, fmt.Errorf("closure %s cannot resolve inherited type parameter %s", def.Name, name)
		}
		typeArgs[index] = typeArg
	}
	for index, capture := range fn.Captures {
		if capture.Mode == CaptureSlot && index < len(child.captureLocals) {
			// A nested slot capture must propagate through every enclosing
			// closure rather than degrading into a value snapshot.
			fl.markCaptureSlot(child.captureLocals[index])
		}
	}

	return &Expr{Kind: ExprMakeClosure, Type: typeID, Payload: &CallExprPayload{Function: id, TypeArgs: typeArgs, CaptureLocals: child.captureLocals}}, recursive, nil
}

func (fl *functionLowerer) lowerModuleSymbol(typeID TypeID, symbol *checker.ModuleSymbol) (*Expr, error) {
	if global, ok, err := fl.l.resolveModuleGlobal(symbol.Module, symbol.Symbol.Name); err != nil {
		return nil, err
	} else if ok {
		return &Expr{Kind: ExprLoadGlobal, Type: fl.l.program.Globals[global].Type, Payload: &GlobalExprPayload{Global: global}}, nil
	}

	if err := fl.l.ensureModuleGlobalsDeclared(symbol.Module); err != nil {
		return nil, err
	}
	declaration := symbol.Declaration()
	if declaration == nil || declaration.Body == nil {
		return nil, fmt.Errorf("unsupported AIR module symbol %s::%s of type %s", symbol.Module, symbol.Symbol.Name, symbol.Type().String())
	}
	module := fl.l.internModule(symbol.Module)
	if err := fl.l.ensureModuleTraitImplsDeclared(symbol.Module); err != nil {
		return nil, err
	}
	id, err := fl.l.declareAndLowerFunction(module, declaration)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: ExprFunctionRef, Type: typeID, Payload: &CallExprPayload{Function: id}}, nil
}

func (fl *functionLowerer) lowerInstanceMethod(typeID TypeID, method *checker.InstanceMethod) (*Expr, error) {
	target, err := fl.lowerExpr(method.Subject)
	if err != nil {
		return nil, err
	}
	if method.ReceiverMode != nil {
		if method.ReceiverType == nil {
			return nil, fmt.Errorf("mutating interior receiver has no reference type")
		}
		referenceType, err := fl.internType(method.ReceiverType)
		if err != nil {
			return nil, err
		}
		mode, err := lowerReferenceMode(*method.ReceiverMode)
		if err != nil {
			return nil, err
		}
		target = &Expr{Kind: ExprMutRef, Type: referenceType, Target: target, Payload: &ReferenceExprPayload{Mode: mode}}
	}
	typeInfo, ok := fl.l.referentTypeInfo(target.Type)
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
		if !method.HasTraitMethodSlot {
			return nil, fmt.Errorf("trait call %s.%s has no resolved method slot", trait.Name, method.Method.Name)
		}
		slot := method.TraitMethodSlot
		if slot < 0 || slot >= len(trait.Methods) {
			return nil, fmt.Errorf("trait call %s.%s has invalid method slot %d", trait.Name, method.Method.Name, slot)
		}
		traitMethod := trait.Methods[slot]
		args, err := fl.lowerArgsWithSignature(method.Method.Args, traitMethod.Signature)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprCallTrait, Type: typeID, Target: target, Args: args, Payload: &TraitExprPayload{Trait: typeInfo.Trait, Method: slot}}, nil
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
	declaration := method.Method.Declaration()
	if declaration != nil && !declaration.Mutates {
		if reference, ok := fl.l.typeInfo(target.Type); ok && reference.Kind == TypeReference {
			// A value-receiver method observes through a reference rather than
			// passing the handle to a value-shaped receiver parameter. Preserve
			// that implicit read as an explicit AIR dereference (ADR 0057).
			target = &Expr{Kind: ExprDeref, Type: reference.Elem, Target: target, Payload: &ReferenceExprPayload{Observational: true}}
		}
	}
	if method.DispatchTrait != nil {
		expr, ok, err := fl.lowerStaticTraitMethod(typeID, target, method)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("AIR trait implementation %s has no resolved method %s", method.DispatchTrait.Name, method.Method.Name)
		}
		return expr, nil
	}
	if declaration == nil || declaration.Body == nil {
		return nil, fmt.Errorf("unsupported AIR instance method %s on %s", method.Method.Name, method.Subject.Type().String())
	}
	return fl.lowerUserDefinedInstanceMethod(typeID, target, typeInfo, method, declaration)
}

func (fl *functionLowerer) lowerStaticTraitMethod(typeID TypeID, target *Expr, method *checker.InstanceMethod) (*Expr, bool, error) {
	if method.DispatchTrait == nil || !method.HasTraitMethodSlot {
		return nil, false, nil
	}
	module := fl.l.moduleForInstanceMethod(method, fl.fn.Module)
	if module >= 0 && int(module) < len(fl.l.program.Modules) {
		path := fl.l.program.Modules[module].Path
		if mod, ok := fl.l.moduleByName[path]; ok && !fl.l.loweredModules[path] {
			if err := fl.l.lowerModule(mod); err != nil {
				return nil, false, err
			}
		}
	}

	implTargetType := target.Type
	if reference, ok := fl.l.typeInfo(target.Type); ok && reference.Kind == TypeReference {
		implTargetType = reference.Elem
	}
	implTargetInfo, _ := fl.l.typeInfo(implTargetType)
	for _, impl := range fl.l.program.Impls {
		implInfo, _ := fl.l.typeInfo(impl.ForType)
		genericMatch := implTargetInfo.Generic != NoType && (impl.ForType == implTargetInfo.Generic || implInfo.Generic == implTargetInfo.Generic)
		if impl.ForType != implTargetType && !genericMatch {
			continue
		}
		if !validTraitID(&fl.l.program, impl.Trait) {
			return nil, false, fmt.Errorf("impl %d references invalid trait %d", impl.ID, impl.Trait)
		}
		trait := fl.l.program.Traits[impl.Trait]
		if trait.ModulePath != method.DispatchTrait.ModulePath || trait.Name != method.DispatchTrait.Name {
			continue
		}
		slot := method.TraitMethodSlot
		if slot < 0 || slot >= len(trait.Methods) || slot >= len(impl.Methods) {
			return nil, false, fmt.Errorf("impl %d has invalid method slot %d for trait %s", impl.ID, slot, trait.Name)
		}
		id := impl.Methods[slot]
		if !validFunctionID(&fl.l.program, id) {
			return nil, false, fmt.Errorf("impl %d method %d has invalid function id %d", impl.ID, slot, id)
		}
		fn := fl.l.program.Functions[id]
		args := make([]Expr, 0, len(method.Method.Args)+1)
		args = append(args, *target)
		loweredArgs, err := fl.lowerArgsWithSignature(method.Method.Args, Signature{Params: fn.Signature.Params[1:], Return: fn.Signature.Return})
		if err != nil {
			return nil, false, err
		}
		args = append(args, loweredArgs...)
		var typeArgs []TypeID
		if genericMatch {
			typeArgs = append([]TypeID(nil), implTargetInfo.GenericArgs...)
		}
		return &Expr{Kind: ExprCall, Type: typeID, Args: args, Payload: &CallExprPayload{Function: id, TypeArgs: typeArgs}}, true, nil
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
	usesOnlyStructTypeParams := false
	if typeInfo.Generic != NoType {
		definition, ok := fl.l.typeInfo(typeInfo.Generic)
		if !ok {
			return nil, fmt.Errorf("generic method definition %d is invalid or incomplete", typeInfo.Generic)
		}
		usesOnlyStructTypeParams = methodUsesOnlyStructTypeParams(def, definition.TypeParams)
	}
	if typeInfo.Generic != NoType && usesOnlyStructTypeParams {
		id, typeArgs, err := fl.declareGenericInstanceMethodFunction(module, typeInfo.ID, method.StructType, def)
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
		return &Expr{Kind: ExprCall, Type: typeID, Args: args, Payload: &CallExprPayload{Function: id, TypeArgs: typeArgs}}, nil
	}
	id, err := fl.declareInstanceMethodFunction(module, typeInfo.Name, typeInfo.ID, def, method.Method.Args, typeID)
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
	return &Expr{Kind: ExprCall, Type: typeID, Args: args, Payload: &CallExprPayload{Function: id}}, nil
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
	sourceReference := int(sourceLocal) >= 0 && int(sourceLocal) < len(fl.parent.fn.Locals) && fl.parent.fn.Locals[sourceLocal].Reference
	local := fl.defineLocal(name, typeID, mutable)
	fl.fn.Locals[local].Reference = sourceReference
	fl.captureByName[name] = local
	fl.captureLocals = append(fl.captureLocals, sourceLocal)
	mode := CaptureValue
	if info, ok := fl.l.typeInfo(typeID); ok && (info.Kind == TypeReference || info.Kind == TypeForeignType && info.ForeignPointer) {
		mode = CaptureReference
	}
	fl.fn.Captures = append(fl.fn.Captures, Capture{Name: name, Type: typeID, Local: local, Mode: mode})
	return local, typeID, mutable, true, nil
}

func (fl *functionLowerer) markCaptureSlot(local LocalID) {
	for index := range fl.fn.Captures {
		if fl.fn.Captures[index].Local == local {
			fl.fn.Captures[index].Mode = CaptureSlot
			return
		}
	}
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

func (l *lowerer) lookupGlobalInModule(module ModuleID, name string) (GlobalID, bool) {
	id, ok := l.globals[globalKey(module, name)]
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
	for _, imported := range sortedModules(mod.Program().Imports) {
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
	for _, imported := range sortedModules(mod.Program().Imports) {
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
			if len(node.GenericParams) > 0 {
				if err := l.declareGenericStructMethodsAndTraitImpls(modID, node); err != nil {
					return err
				}
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
	for _, imported := range sortedModules(prog.Imports) {
		l.moduleByName[imported.Path()] = imported
		importedID := l.internModule(imported.Path())
		l.program.Modules[modID].Imports = appendUniqueModule(l.program.Modules[modID].Imports, importedID)
	}
	for _, stmt := range prog.Statements {
		switch node := stmt.Stmt.(type) {
		case *checker.StructDef:
			if len(node.GenericParams) > 0 {
				if _, err := l.internGenericStructDef(node); err != nil {
					return err
				}
				continue
			}
			if l.typeHasUnresolvedTypeVar(node) {
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
			if l.typeHasUnresolvedTypeVar(node) {
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
	for _, imported := range sortedModules(prog.Imports) {
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

func (l *lowerer) moduleForInstanceMethod(method *checker.InstanceMethod, fallback ModuleID) ModuleID {
	if method == nil || method.Method == nil {
		return fallback
	}
	ownerModulePath := ""
	switch {
	case method.StructType != nil:
		ownerModulePath = method.StructType.ModulePath
	case method.EnumType != nil:
		ownerModulePath = method.EnumType.ModulePath
	}
	if ownerModulePath == "" {
		return fallback
	}
	l.findReachableModule(ownerModulePath)
	return l.internModule(ownerModulePath)
}

func (l *lowerer) typeInfo(id TypeID) (TypeInfo, bool) {
	if id <= 0 || int(id) > len(l.program.Types) || (l.typeInterner != nil && !l.typeInterner.nominalAvailable(id)) {
		return TypeInfo{}, false
	}
	return l.program.Types[id-1], true
}

// referentTypeInfo resolves the one reference layer that observational member
// and builtin operations act through while preserving the operand's own AIR
// type and handle identity (ADR 0057).
func (l *lowerer) referentTypeInfo(id TypeID) (TypeInfo, bool) {
	info, ok := l.typeInfo(id)
	if !ok {
		return TypeInfo{}, false
	}
	if info.Kind == TypeReference {
		return l.typeInfo(info.Elem)
	}
	return info, true
}

func (l *lowerer) lookupImpl(trait TraitID, forType TypeID) (ImplID, bool) {
	forTypeInfo, _ := l.typeInfo(forType)
	for _, impl := range l.program.Impls {
		if impl.Trait != trait {
			continue
		}
		if impl.ForType == forType {
			return impl.ID, true
		}
		implInfo, _ := l.typeInfo(impl.ForType)
		if forTypeInfo.Generic != NoType && (impl.ForType == forTypeInfo.Generic || implInfo.Generic == forTypeInfo.Generic) {
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

func lowerStructFieldInfo(def *checker.StructDef, name string, typeID TypeID, index int) FieldInfo {
	field := FieldInfo{Name: name, Type: typeID, Index: index}
	if options, ok := checker.StructFieldJSON(def, name); ok {
		field.JSON = JSONFieldInfo{
			Name:     options.Name,
			HasName:  options.HasName,
			OmitNone: options.OmitNone,
			Skip:     options.Skip,
		}
	}
	for _, tag := range checker.StructFieldGoTags(def, name) {
		field.GoTags = append(field.GoTags, GoFieldTag{Key: tag.Key, Value: tag.Value})
	}
	return field
}

func sortedFieldNames(fields map[string]checker.Type) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// sortedModules makes checker map traversal safe for AIR ID allocation. Module,
// type, and function IDs become part of generated names, so discovery order must
// not depend on Go's randomized map iteration.
func sortedModules(modules map[string]checker.Module) []checker.Module {
	type moduleEntry struct {
		key    string
		path   string
		module checker.Module
	}
	entries := make([]moduleEntry, 0, len(modules))
	for key, module := range modules {
		path := ""
		if module != nil {
			path = module.Path()
		}
		entries = append(entries, moduleEntry{key: key, path: path, module: module})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].path == entries[j].path {
			return entries[i].key < entries[j].key
		}
		return entries[i].path < entries[j].path
	})
	ordered := make([]checker.Module, len(entries))
	for i, entry := range entries {
		ordered[i] = entry.module
	}
	return ordered
}

// sortedMethodDefinitions preserves the method table's canonical key order
// before declarations allocate AIR function IDs.
func sortedMethodDefinitions(methods map[string]*checker.FunctionDef) []*checker.FunctionDef {
	names := make([]string, 0, len(methods))
	for name := range methods {
		names = append(names, name)
	}
	sort.Strings(names)
	ordered := make([]*checker.FunctionDef, len(names))
	for i, name := range names {
		ordered[i] = methods[name]
	}
	return ordered
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
	key, _, err := l.genericBindingsKeyWithInterner(def, func(typ checker.Type) (TypeID, error) {
		return l.internGenericArgument(typ, l.internType)
	})
	return key, err
}

func (fl *functionLowerer) genericBindingsKey(def *checker.FunctionDef) (string, error) {
	key, _, err := fl.genericBindingsKeyAndTypeVars(def)
	return key, err
}

func (fl *functionLowerer) genericBindingsKeyAndTypeVars(def *checker.FunctionDef) (string, map[string]TypeID, error) {
	return fl.l.genericBindingsKeyWithInterner(def, func(typ checker.Type) (TypeID, error) {
		return fl.l.internGenericArgument(typ, fl.internResolvedType)
	})
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
		id, err := fl.l.internGenericArgument(binding, fl.internResolvedType)
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
func (fl *functionLowerer) buildResolvedCallExpr(id FunctionID, call *checker.FunctionCall, callArgs []checker.Expression) (*Expr, error) {
	signature := fl.l.program.Functions[id].Signature
	var typeArgs []TypeID
	if len(fl.l.program.Functions[id].TypeParams) > 0 {
		concrete, err := fl.signatureForCall(call)
		if err != nil {
			return nil, err
		}
		signature = concrete
		typeArgs, err = fl.genericCallTypeArgs(call.Signature())
		if err != nil {
			return nil, err
		}
	}
	args, err := fl.lowerArgsWithSignature(callArgs, signature)
	if err != nil {
		return nil, err
	}
	spreadElement, err := fl.spreadElementTypeForCall(call)
	if err != nil {
		return nil, err
	}
	spreadCallable, err := fl.spreadCallableTypeForCall(call)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: ExprCall, Type: signature.Return, Args: args, Payload: &CallExprPayload{Function: id, TypeArgs: typeArgs, Spread: newSpreadExprPayload(call.TailSpread, spreadElement, spreadCallable)}}, nil
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
		key += fmt.Sprintf("%d:%d", param.Type, param.ABI)
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

func isMutableReferenceProducer(expr checker.Expression) bool {
	if expr == nil {
		return false
	}
	if _, ok := expr.Type().(*checker.MutableRef); ok {
		return true
	}
	call, ok := expr.(*checker.ForeignFunctionCall)
	return ok && call.PointerResult
}
