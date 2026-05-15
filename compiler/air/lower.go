package air

import (
	"fmt"
	"sort"

	"github.com/akonwi/ard/checker"
)

func Lower(module checker.Module) (*Program, error) {
	l := newLowerer()
	if err := l.lowerModule(module, true); err != nil {
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
	externs      map[string]ExternID

	loweringModules map[string]bool
	loweredModules  map[string]bool
	loweringFuncs   map[FunctionID]bool
	loweredFuncs    map[FunctionID]bool
}

type functionLowerer struct {
	l             *lowerer
	locals        map[string]LocalID
	fn            *Function
	parent        *functionLowerer
	captureByName map[string]LocalID
	captureLocals []LocalID
	typeVars      map[string]TypeID
}

func newLowerer() *lowerer {
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
		externs:      map[string]ExternID{},

		loweringModules: map[string]bool{},
		loweredModules:  map[string]bool{},
		loweringFuncs:   map[FunctionID]bool{},
		loweredFuncs:    map[FunctionID]bool{},
	}
	l.mustIntern(checker.Void)
	l.mustIntern(checker.Int)
	l.mustIntern(checker.Float)
	l.mustIntern(checker.Bool)
	l.mustIntern(checker.Str)
	l.mustIntern(checker.Dynamic)
	return l
}

func (l *lowerer) mustIntern(t checker.Type) TypeID {
	id, err := l.internType(t)
	if err != nil {
		panic(err)
	}
	return id
}

func (l *lowerer) lowerModule(module checker.Module, includeTests bool) error {
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

		switch expr := stmt.Expr.(type) {
		case *checker.FunctionDef:
			if functionHasUnresolvedTypeVar(expr) {
				continue
			}
			if _, err := l.declareFunction(modID, expr, includeTests); err != nil {
				return err
			}
		case *checker.ExternalFunctionDef:
			if externalFunctionHasUnresolvedTypeVar(expr) {
				continue
			}
			if _, err := l.declareExtern(modID, expr); err != nil {
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
		case *checker.Enum:
			if err := l.declareTraitImplsForType(modID, node); err != nil {
				return err
			}
		}
	}

	for i := range prog.Statements {
		stmt := prog.Statements[i]
		if def, ok := stmt.Expr.(*checker.FunctionDef); ok {
			if functionHasUnresolvedTypeVar(def) {
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

func (l *lowerer) declareFunction(module ModuleID, def *checker.FunctionDef, includeTests bool) (FunctionID, error) {
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
		IsTest: def.IsTest,
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	if includeTests && def.Name == "main" {
		l.program.Entry = id
	}
	if includeTests && def.IsTest {
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
	if id, ok := l.functions[functionKey(module, def.Name)]; ok {
		if signaturesEqual(l.program.Functions[id].Signature, signature) {
			return id, nil
		}
	}
	key := concreteFunctionKey(module, def.Name, signature)
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
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
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

func (l *lowerer) declareAndLowerFunctionCall(module ModuleID, def *checker.FunctionDef, call *checker.FunctionCall) (FunctionID, error) {
	signature, err := l.signatureForCall(call)
	if err != nil {
		return NoFunction, err
	}
	id, err := l.declareFunctionSpecializationWithSignature(module, def, signature)
	if err != nil {
		return NoFunction, err
	}
	if err := l.lowerFunctionByID(id, def); err != nil {
		return NoFunction, err
	}
	return id, nil
}

func (l *lowerer) declareClosureFunction(module ModuleID, def *checker.FunctionDef, typeID TypeID) (FunctionID, error) {
	typeInfo, ok := l.typeInfo(typeID)
	if !ok || typeInfo.Kind != TypeFunction {
		return NoFunction, fmt.Errorf("closure %s lowered with non-function AIR type %d", def.Name, typeID)
	}
	if len(def.Parameters) != len(typeInfo.Params) {
		return NoFunction, fmt.Errorf("closure %s expects %d params, got %d AIR params", def.Name, len(def.Parameters), len(typeInfo.Params))
	}
	params := make([]Param, len(def.Parameters))
	for i, param := range def.Parameters {
		params[i] = Param{Name: param.Name, Type: typeInfo.Params[i], Mutable: param.Mutable}
	}
	signature := Signature{Params: params, Return: typeInfo.Return}
	key := concreteFunctionKey(module, def.Name, signature)
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
	if def != nil {
		for i, param := range def.Parameters {
			if i < len(fn.Signature.Params) {
				fl.bindTypeVars(param.Type, fn.Signature.Params[i].Type)
			}
		}
		fl.bindTypeVars(def.ReturnType, fn.Signature.Return)
	}
	return fl
}

func (fl *functionLowerer) bindTypeVars(pattern checker.Type, actual TypeID) {
	if pattern == nil || actual == NoType || !validTypeID(&fl.l.program, actual) {
		return
	}
	if tv, ok := pattern.(*checker.TypeVar); ok {
		if tv.Actual() == nil {
			fl.typeVars[tv.Name()] = actual
			return
		}
		fl.bindTypeVars(tv.Actual(), actual)
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
	case *checker.Map:
		if actualInfo.Kind == TypeMap {
			fl.bindTypeVars(typ.Key(), actualInfo.Key)
			fl.bindTypeVars(typ.Value(), actualInfo.Value)
		}
	case *checker.Maybe:
		if actualInfo.Kind == TypeMaybe {
			fl.bindTypeVars(typ.Of(), actualInfo.Elem)
		}
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
		if actualInfo.Kind == TypeStruct || actualInfo.Kind == TypeFiber {
			fieldsByName := map[string]FieldInfo{}
			for _, field := range actualInfo.Fields {
				fieldsByName[field.Name] = field
			}
			for name, fieldType := range typ.Fields {
				if field, ok := fieldsByName[name]; ok {
					fl.bindTypeVars(fieldType, field.Type)
				}
			}
			if actualInfo.Kind == TypeFiber {
				if result, ok := typ.Fields["result"]; ok {
					fl.bindTypeVars(result, actualInfo.Elem)
				}
			}
		}
	}
}

func (fl *functionLowerer) internType(t checker.Type) (TypeID, error) {
	if tv, ok := t.(*checker.TypeVar); ok {
		if tv.Actual() != nil {
			return fl.internType(tv.Actual())
		}
		if id, ok := fl.typeVars[tv.Name()]; ok {
			return id, nil
		}
		return fl.l.internType(checker.Void)
	}
	if !typeHasUnresolvedTypeVar(t) {
		return fl.l.internType(t)
	}
	return fl.internCompositeType(t)
}

func (fl *functionLowerer) internCompositeType(t checker.Type) (TypeID, error) {
	switch typ := t.(type) {
	case *checker.List:
		elem, err := fl.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		return fl.l.internSyntheticType("["+fl.l.typeName(elem)+"]", TypeInfo{Kind: TypeList, Elem: elem})
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
			paramType, err := fl.internType(param.Type)
			if err != nil {
				return NoType, err
			}
			params[i] = paramType
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
		return fl.l.internSyntheticType(name, TypeInfo{Kind: TypeFunction, Params: params, Return: returnType})
	default:
		return fl.l.internType(t)
	}
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
		methods = typed.Methods
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
	key := implKey(module, trait.Name, owner.String())
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
	case TypeStr, TypeInt, TypeFloat, TypeBool:
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
	case trait.Name == "Encodable" && trait.Methods[0].Name == "to_dyn":
		methodID, err = l.declareBuiltinToDynamicMethod(module, ownerInfo)
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
		ID:     id,
		Module: module,
		Name:   ownerInfo.Name + ".ToString.to_str",
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

func (l *lowerer) declareBuiltinToDynamicMethod(module ModuleID, ownerInfo TypeInfo) (FunctionID, error) {
	key := methodFunctionKey(module, ownerInfo.Name, "Encodable", "to_dyn")
	if id, ok := l.functions[key]; ok {
		return id, nil
	}
	dynamicType, err := l.internType(checker.Dynamic)
	if err != nil {
		return NoFunction, err
	}
	id := FunctionID(len(l.program.Functions))
	l.functions[key] = id
	receiver := Param{Name: "self", Type: ownerInfo.ID}
	l.program.Functions = append(l.program.Functions, Function{
		ID:     id,
		Module: module,
		Name:   ownerInfo.Name + ".Encodable.to_dyn",
		Signature: Signature{
			Params: []Param{receiver},
			Return: dynamicType,
		},
		Locals: []Local{{ID: 0, Name: "self", Type: ownerInfo.ID}},
		Body: Block{Result: &Expr{
			Kind:   ExprToDynamic,
			Type:   dynamicType,
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
		ID:     id,
		Module: module,
		Name:   owner.String() + "." + traitName + "." + def.Name,
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

func (l *lowerer) declareInstanceMethodFunction(module ModuleID, ownerName string, ownerType TypeID, def *checker.FunctionDef) (FunctionID, error) {
	key := methodFunctionKey(module, ownerName, "instance", def.Name)
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
		ID:     id,
		Module: module,
		Name:   ownerName + "." + def.Name,
		Signature: Signature{
			Params: params,
			Return: returnType,
		},
	})
	l.program.Modules[module].Functions = appendUniqueFunction(l.program.Modules[module].Functions, id)
	return id, nil
}

func (l *lowerer) lowerInstanceMethodFunction(id FunctionID, def *checker.FunctionDef) error {
	return l.lowerFunctionByID(id, def)
}

func (l *lowerer) declareExtern(module ModuleID, def *checker.ExternalFunctionDef) (ExternID, error) {
	if externalFunctionHasUnresolvedTypeVar(def) {
		return 0, fmt.Errorf("cannot declare unspecialized generic extern %s", def.Name)
	}
	key := functionKey(module, def.Name)
	if id, ok := l.externs[key]; ok {
		return id, nil
	}
	params := make([]Param, len(def.Parameters))
	for i, param := range def.Parameters {
		typeID, err := l.internType(param.Type)
		if err != nil {
			return 0, err
		}
		params[i] = Param{Name: param.Name, Type: typeID, Mutable: param.Mutable}
	}
	returnType, err := l.internType(def.ReturnType)
	if err != nil {
		return 0, err
	}
	id := ExternID(len(l.program.Externs))
	l.externs[key] = id
	bindings := map[string]string{}
	for target, binding := range def.ExternalBindings {
		bindings[target] = binding
	}
	if def.ExternalBindingTarget != "" && def.ExternalBinding != "" {
		if _, ok := bindings[def.ExternalBindingTarget]; !ok {
			bindings[def.ExternalBindingTarget] = def.ExternalBinding
		}
	}
	if len(bindings) == 0 && def.ExternalBinding != "" {
		bindings["go"] = def.ExternalBinding
	}
	l.program.Externs = append(l.program.Externs, Extern{
		ID:     id,
		Module: module,
		Name:   def.Name,
		Signature: Signature{
			Params: params,
			Return: returnType,
		},
		Bindings: bindings,
	})
	return id, nil
}

func (l *lowerer) declareModuleCallExtern(call *checker.ModuleFunctionCall) (ExternID, error) {
	name := call.Module + "::" + call.Call.Name
	key := "module:" + name
	if id, ok := l.externs[key]; ok {
		return id, nil
	}
	params := make([]Param, len(call.Call.Args))
	for i, arg := range call.Call.Args {
		typeID, err := l.internType(arg.Type())
		if err != nil {
			return 0, err
		}
		params[i] = Param{Name: fmt.Sprintf("arg%d", i), Type: typeID}
	}
	returnType, err := l.internType(call.Type())
	if err != nil {
		return 0, err
	}
	id := ExternID(len(l.program.Externs))
	l.externs[key] = id
	l.program.Externs = append(l.program.Externs, Extern{
		ID:   id,
		Name: name,
		Signature: Signature{
			Params: params,
			Return: returnType,
		},
		Bindings: map[string]string{},
	})
	return id, nil
}

func (l *lowerer) declareFunctionCallExtern(module ModuleID, call *checker.FunctionCall) (ExternID, error) {
	return l.declareConcreteExternCall(module, call.Name, call)
}

func (l *lowerer) declareConcreteExternCall(module ModuleID, name string, call *checker.FunctionCall) (ExternID, error) {
	signature, err := l.signatureForCall(call)
	if err != nil {
		return 0, err
	}
	key := functionKey(module, name)
	if id, ok := l.externs[key]; ok {
		if signaturesEqual(l.program.Externs[id].Signature, signature) {
			return id, nil
		}
	}
	key = concreteExternKey(module, name, signature)
	if id, ok := l.externs[key]; ok {
		return id, nil
	}
	id := ExternID(len(l.program.Externs))
	l.externs[key] = id
	bindings := map[string]string{}
	if call.ExternalBinding != "" {
		bindings["go"] = call.ExternalBinding
	}
	l.program.Externs = append(l.program.Externs, Extern{
		ID:        id,
		Module:    module,
		Name:      name,
		Signature: signature,
		Bindings:  bindings,
	})
	return id, nil
}

func (l *lowerer) signatureForCall(call *checker.FunctionCall) (Signature, error) {
	if def := call.Definition(); def != nil {
		if !functionHasTypeVar(def) {
			return l.signatureForFunction(def.Parameters, def.ReturnType)
		}
		params := make([]Param, len(def.Parameters))
		for i, param := range def.Parameters {
			paramType := param.Type
			if i < len(call.Args) {
				paramType = call.Args[i].Type()
			}
			typeID, err := l.internType(paramType)
			if err != nil {
				return Signature{}, err
			}
			params[i] = Param{Name: param.Name, Type: typeID, Mutable: param.Mutable}
		}
		returnType, err := l.internType(call.Type())
		if err != nil {
			return Signature{}, err
		}
		return Signature{Params: params, Return: returnType}, nil
	}
	params := make([]Param, len(call.Args))
	for i, arg := range call.Args {
		typeID, err := l.internType(arg.Type())
		if err != nil {
			return Signature{}, err
		}
		params[i] = Param{Name: fmt.Sprintf("arg%d", i), Type: typeID}
	}
	returnType, err := l.internType(call.Type())
	if err != nil {
		return Signature{}, err
	}
	return Signature{Params: params, Return: returnType}, nil
}

func (l *lowerer) internType(t checker.Type) (TypeID, error) {
	if t == nil {
		return NoType, fmt.Errorf("cannot intern nil type")
	}
	if tv, ok := t.(*checker.TypeVar); ok && tv.Actual() != nil {
		return l.internType(tv.Actual())
	}
	key := airTypeKey(t)
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
		elem, err := l.internType(typ.Of())
		if err != nil {
			return NoType, err
		}
		info.Kind = TypeMaybe
		info.Elem = elem
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
		if typ.Name == "Fiber" {
			elemType, ok := typ.Fields["result"]
			if !ok {
				return NoType, fmt.Errorf("Fiber type missing result field")
			}
			elem, err := l.internType(elemType)
			if err != nil {
				return NoType, err
			}
			info.Kind = TypeFiber
			info.Elem = elem
			break
		}
		info.Kind = TypeStruct
		fields := sortedFieldNames(typ.Fields)
		info.Fields = make([]FieldInfo, len(fields))
		for i, name := range fields {
			fieldType, err := l.internType(typ.Fields[name])
			if err != nil {
				return NoType, err
			}
			info.Fields[i] = FieldInfo{Name: name, Type: fieldType, Index: i}
		}
	case *checker.Enum:
		info.Kind = TypeEnum
		info.Variants = make([]VariantInfo, len(typ.Values))
		for i, variant := range typ.Values {
			info.Variants[i] = VariantInfo{Name: variant.Name, Discriminant: variant.Value}
		}
	case *checker.Union:
		info.Kind = TypeUnion
		info.Members = make([]UnionMember, len(typ.Types))
		for i, member := range typ.Types {
			memberID, err := l.internType(member)
			if err != nil {
				return NoType, err
			}
			info.Members[i] = UnionMember{Type: memberID, Tag: uint32(i), Name: member.String()}
		}
	case *checker.ExternType:
		info.Kind = TypeExtern
		info.ExternBinding = typ.ExternalBinding
	case *checker.FunctionDef:
		info.Kind = TypeFunction
		for _, param := range typ.Parameters {
			paramType, err := l.internType(param.Type)
			if err != nil {
				return NoType, err
			}
			info.Params = append(info.Params, paramType)
		}
		returnType, err := l.internType(typ.ReturnType)
		if err != nil {
			return NoType, err
		}
		info.Return = returnType
	case *checker.ExternalFunctionDef:
		info.Kind = TypeFunction
		for _, param := range typ.Parameters {
			paramType, err := l.internType(param.Type)
			if err != nil {
				return NoType, err
			}
			info.Params = append(info.Params, paramType)
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
		case checker.Float:
			info.Kind = TypeFloat
		case checker.Bool:
			info.Kind = TypeBool
		case checker.Str:
			info.Kind = TypeStr
		case checker.Dynamic:
			info.Kind = TypeDynamic
		default:
			return NoType, fmt.Errorf("unsupported AIR type %T (%s)", t, t.String())
		}
	}

	l.program.Types[idx] = info
	return id, nil
}

func (l *lowerer) internSyntheticType(name string, info TypeInfo) (TypeID, error) {
	if id, ok := l.typeByKey[name]; ok {
		return id, nil
	}
	id := TypeID(len(l.program.Types) + 1)
	info.ID = id
	info.Name = name
	l.typeByKey[name] = id
	l.program.Types = append(l.program.Types, info)
	return id, nil
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
	if id, ok := l.traits[trait.Name]; ok {
		return id, nil
	}
	id := TraitID(len(l.program.Traits))
	l.traits[trait.Name] = id

	methods := trait.GetMethods()
	loweredMethods := make([]TraitMethod, len(methods))
	for i, method := range methods {
		sig, err := l.signatureForFunction(method.Parameters, method.ReturnType)
		if err != nil {
			return 0, err
		}
		loweredMethods[i] = TraitMethod{Name: method.Name, Signature: sig}
	}
	l.program.Traits = append(l.program.Traits, Trait{
		ID:      id,
		Name:    trait.Name,
		Methods: loweredMethods,
	})
	return id, nil
}

func (l *lowerer) signatureForFunction(params []checker.Parameter, returnType checker.Type) (Signature, error) {
	loweredParams := make([]Param, len(params))
	for i, param := range params {
		typeID, err := l.internType(param.Type)
		if err != nil {
			return Signature{}, err
		}
		loweredParams[i] = Param{Name: param.Name, Type: typeID, Mutable: param.Mutable}
	}
	returnID, err := l.internType(returnType)
	if err != nil {
		return Signature{}, err
	}
	return Signature{Params: loweredParams, Return: returnID}, nil
}

func functionHasUnresolvedTypeVar(def *checker.FunctionDef) bool {
	if def == nil {
		return false
	}
	for _, param := range def.Parameters {
		if typeHasUnresolvedTypeVar(param.Type) {
			return true
		}
	}
	return typeHasUnresolvedTypeVar(def.ReturnType)
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

func externalFunctionHasUnresolvedTypeVar(def *checker.ExternalFunctionDef) bool {
	if def == nil {
		return false
	}
	for _, param := range def.Parameters {
		if typeHasUnresolvedTypeVar(param.Type) {
			return true
		}
	}
	return typeHasUnresolvedTypeVar(def.ReturnType)
}

func typeContainsTypeVar(t checker.Type) bool {
	switch typ := t.(type) {
	case nil:
		return false
	case *checker.TypeVar:
		return true
	case *checker.List:
		return typeContainsTypeVar(typ.Of())
	case *checker.Map:
		return typeContainsTypeVar(typ.Key()) || typeContainsTypeVar(typ.Value())
	case *checker.Maybe:
		return typeContainsTypeVar(typ.Of())
	case *checker.Result:
		return typeContainsTypeVar(typ.Val()) || typeContainsTypeVar(typ.Err())
	case *checker.Union:
		for _, member := range typ.Types {
			if typeContainsTypeVar(member) {
				return true
			}
		}
		return false
	case *checker.StructDef:
		for _, fieldType := range typ.Fields {
			if typeContainsTypeVar(fieldType) {
				return true
			}
		}
		return false
	case *checker.FunctionDef:
		return functionHasTypeVar(typ)
	case *checker.ExternalFunctionDef:
		for _, param := range typ.Parameters {
			if typeContainsTypeVar(param.Type) {
				return true
			}
		}
		return typeContainsTypeVar(typ.ReturnType)
	default:
		return false
	}
}

func typeHasUnresolvedTypeVar(t checker.Type) bool {
	switch typ := t.(type) {
	case nil:
		return false
	case *checker.TypeVar:
		if typ.Actual() == nil {
			return true
		}
		return typeHasUnresolvedTypeVar(typ.Actual())
	case *checker.List:
		return typeHasUnresolvedTypeVar(typ.Of())
	case *checker.Map:
		return typeHasUnresolvedTypeVar(typ.Key()) || typeHasUnresolvedTypeVar(typ.Value())
	case *checker.Maybe:
		return typeHasUnresolvedTypeVar(typ.Of())
	case *checker.Result:
		return typeHasUnresolvedTypeVar(typ.Val()) || typeHasUnresolvedTypeVar(typ.Err())
	case *checker.Union:
		for _, member := range typ.Types {
			if typeHasUnresolvedTypeVar(member) {
				return true
			}
		}
		return false
	case *checker.StructDef:
		for _, fieldType := range typ.Fields {
			if typeHasUnresolvedTypeVar(fieldType) {
				return true
			}
		}
		return false
	case *checker.FunctionDef:
		return functionHasUnresolvedTypeVar(typ)
	case *checker.ExternalFunctionDef:
		return externalFunctionHasUnresolvedTypeVar(typ)
	case *checker.ExternType:
		for _, typeArg := range typ.TypeArgs {
			if typeHasUnresolvedTypeVar(typeArg) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func canWrapAsDynamic(kind TypeKind) bool {
	switch kind {
	case TypeVoid, TypeInt, TypeFloat, TypeBool, TypeStr, TypeList, TypeMap, TypeStruct, TypeEnum, TypeMaybe, TypeResult, TypeUnion, TypeDynamic:
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
	if typ, ok := t.(*checker.StructDef); ok && typ.Name == "Fiber" {
		if elem, ok := typ.Fields["result"]; ok {
			return "Fiber<" + elem.String() + ">"
		}
	}
	return t.String()
}

func airTypeKey(t checker.Type) string {
	if t == nil {
		return "<nil>"
	}
	if tv, ok := t.(*checker.TypeVar); ok && tv.Actual() != nil {
		return airTypeKey(tv.Actual())
	}
	switch typ := t.(type) {
	case *checker.List:
		return "list<" + airTypeKey(typ.Of()) + ">"
	case *checker.Map:
		return "map<" + airTypeKey(typ.Key()) + "," + airTypeKey(typ.Value()) + ">"
	case *checker.Maybe:
		return "maybe<" + airTypeKey(typ.Of()) + ">"
	case *checker.Result:
		return "result<" + airTypeKey(typ.Val()) + "," + airTypeKey(typ.Err()) + ">"
	case *checker.StructDef:
		if typ.Name == "Fiber" {
			if elem, ok := typ.Fields["result"]; ok {
				return "fiber<" + airTypeKey(elem) + ">"
			}
		}
		return airStructKey(typ)
	case *checker.Enum:
		return airEnumKey(typ)
	case *checker.Union:
		return airUnionKey(typ)
	case *checker.FunctionDef:
		return airFunctionTypeKey(typ.Parameters, typ.ReturnType)
	case *checker.ExternalFunctionDef:
		return airFunctionTypeKey(typ.Parameters, typ.ReturnType)
	default:
		return t.String()
	}
}

func airStructKey(typ *checker.StructDef) string {
	fields := sortedFieldNames(typ.Fields)
	key := "struct " + typ.Name + "{"
	for i, name := range fields {
		if i > 0 {
			key += ","
		}
		key += name + ":" + airTypeKey(typ.Fields[name])
	}
	key += "}"
	if len(typ.Methods) == 0 {
		return key
	}
	methodNames := make([]string, 0, len(typ.Methods))
	for name := range typ.Methods {
		methodNames = append(methodNames, name)
	}
	sort.Strings(methodNames)
	key += " methods{"
	for i, name := range methodNames {
		if i > 0 {
			key += ","
		}
		method := typ.Methods[name]
		key += name + ":" + airFunctionTypeKey(method.Parameters, method.ReturnType)
	}
	key += "}"
	return key
}

func airEnumKey(typ *checker.Enum) string {
	key := "enum " + typ.Name + "{"
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
	parts := make([]string, len(typ.Types))
	for i, member := range typ.Types {
		parts[i] = airTypeKey(member)
	}
	sort.Strings(parts)
	key := "union " + typ.Name + "{"
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
	key := "fn("
	for i, param := range params {
		if i > 0 {
			key += ","
		}
		key += airTypeKey(param.Type)
	}
	key += ")->" + airTypeKey(returnType)
	return key
}

func (fl *functionLowerer) lowerBlock(stmts []checker.Statement) (Block, error) {
	return fl.lowerBlockWithDefault(stmts, fl.fn.Signature.Return)
}

func (fl *functionLowerer) lowerBlockWithDefault(stmts []checker.Statement, defaultType TypeID) (Block, error) {
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
	if wrapped, ok, err := fl.lowerDynamicWrapIfNeeded(expr, expected); ok || err != nil {
		return wrapped, err
	}
	if list, ok := expr.(*checker.ListLiteral); ok {
		if expectedInfo, hasInfo := fl.l.typeInfo(expected); hasInfo && expectedInfo.Kind == TypeList {
			return fl.lowerListLiteral(expected, list, expectedInfo.Elem)
		}
	}
	if m, ok := expr.(*checker.MapLiteral); ok {
		if expectedInfo, hasInfo := fl.l.typeInfo(expected); hasInfo && expectedInfo.Kind == TypeMap {
			return fl.lowerMapLiteral(expected, m, expectedInfo.Key, expectedInfo.Value)
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
	if ifExpr, ok := expr.(*checker.If); ok {
		return fl.lowerIf(expected, ifExpr)
	}
	if match, ok := expr.(*checker.ConditionalMatch); ok {
		return fl.lowerConditionalMatch(expected, match)
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

func (fl *functionLowerer) lowerDynamicWrapIfNeeded(expr checker.Expression, expected TypeID) (*Expr, bool, error) {
	expectedInfo, ok := fl.l.typeInfo(expected)
	if !ok || expectedInfo.Kind != TypeDynamic {
		return nil, false, nil
	}
	actual, err := fl.internType(expr.Type())
	if err != nil {
		return nil, false, err
	}
	if actual == expected {
		return nil, false, nil
	}
	actualInfo, ok := fl.l.typeInfo(actual)
	if !ok || !canWrapAsDynamic(actualInfo.Kind) {
		return nil, false, nil
	}
	value, err := fl.lowerExpr(expr)
	if err != nil {
		return nil, true, err
	}
	return &Expr{Kind: ExprToDynamic, Type: expected, Target: value}, true, nil
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
		typeID, err := fl.internType(s.Type())
		if err != nil {
			return nil, err
		}
		value, actualType, err := fl.lowerContextualExpr(s.Value, typeID)
		if err != nil {
			return nil, err
		}
		local := fl.defineLocal(s.Name, actualType, s.Mutable)
		return &Stmt{Kind: StmtLet, Local: local, Name: s.Name, Type: actualType, Mutable: s.Mutable, Value: value}, nil
	case *checker.Reassignment:
		switch target := s.Target.(type) {
		case *checker.Variable:
			local, ok := fl.locals[target.Name()]
			if !ok {
				return nil, fmt.Errorf("assignment to unknown local %s", target.Name())
			}
			value, err := fl.lowerExprWithExpected(s.Value, fl.fn.Locals[local].Type)
			if err != nil {
				return nil, err
			}
			return &Stmt{Kind: StmtAssign, Local: local, Value: value}, nil
		case *checker.InstanceProperty:
			return fl.lowerFieldAssignment(target, s.Value)
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
	if loop, ok := stmt.Stmt.(*checker.ForIntRange); ok {
		return fl.lowerForIntRange(loop)
	}
	if loop, ok := stmt.Stmt.(*checker.ForInStr); ok {
		return fl.lowerForInStr(loop)
	}
	if loop, ok := stmt.Stmt.(*checker.ForInList); ok {
		return fl.lowerForInList(loop)
	}
	if loop, ok := stmt.Stmt.(*checker.ForInMap); ok {
		return fl.lowerForInMap(loop)
	}
	if loop, ok := stmt.Stmt.(*checker.ForLoop); ok {
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
		stmts = append(stmts, Stmt{Kind: StmtLet, Local: indexCounter, Name: loop.Index + "$range", Type: intType, Mutable: true, Value: &Expr{Kind: ExprConstInt, Type: intType, Int: 0}})
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
		Value: &Expr{Kind: ExprIntAdd, Type: intType, Left: loadLocal(intType, counter), Right: &Expr{Kind: ExprConstInt, Type: intType, Int: 1}},
	})
	if loop.Index != "" {
		body.Stmts = append(body.Stmts, Stmt{
			Kind:  StmtAssign,
			Local: indexCounter,
			Value: &Expr{Kind: ExprIntAdd, Type: intType, Left: loadLocal(intType, indexCounter), Right: &Expr{Kind: ExprConstInt, Type: intType, Int: 1}},
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

	strLocal := fl.defineLocal(loop.Cursor+"$str", strType, false)
	indexName := loop.Cursor + "$index"
	if loop.Index != "" {
		indexName = loop.Index + "$index"
	}
	index := fl.defineLocal(indexName, intType, true)
	cursor := fl.defineLocal(loop.Cursor, strType, false)
	var visibleIndex LocalID
	if loop.Index != "" {
		visibleIndex = fl.defineLocal(loop.Index, intType, false)
	}

	stmts := []Stmt{
		{Kind: StmtLet, Local: strLocal, Name: loop.Cursor + "$str", Type: strType, Value: str},
		{Kind: StmtLet, Local: index, Name: indexName, Type: intType, Mutable: true, Value: &Expr{Kind: ExprConstInt, Type: intType, Int: 0}},
	}

	body, err := fl.lowerNonProducingBlock(loop.Body.Stmts)
	if err != nil {
		return nil, err
	}
	iterationLocals := []Stmt{{
		Kind:  StmtLet,
		Local: cursor,
		Name:  loop.Cursor,
		Type:  strType,
		Value: &Expr{Kind: ExprStrAt, Type: strType, Target: loadLocal(strType, strLocal), Args: []Expr{*loadLocal(intType, index)}},
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
		Value: &Expr{Kind: ExprIntAdd, Type: intType, Left: loadLocal(intType, index), Right: &Expr{Kind: ExprConstInt, Type: intType, Int: 1}},
	})

	stmts = append(stmts, Stmt{
		Kind:      StmtWhile,
		Condition: &Expr{Kind: ExprLt, Type: boolType, Left: loadLocal(intType, index), Right: &Expr{Kind: ExprStrSize, Type: intType, Target: loadLocal(strType, strLocal)}},
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
		{Kind: StmtLet, Local: index, Name: indexName, Type: intType, Mutable: true, Value: &Expr{Kind: ExprConstInt, Type: intType, Int: 0}},
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
		Value: &Expr{Kind: ExprIntAdd, Type: intType, Left: loadLocal(intType, index), Right: &Expr{Kind: ExprConstInt, Type: intType, Int: 1}},
	})

	stmts = append(stmts, Stmt{
		Kind:      StmtWhile,
		Condition: &Expr{Kind: ExprLt, Type: boolType, Left: loadLocal(intType, index), Right: &Expr{Kind: ExprListSize, Type: intType, Target: loadLocal(list.Type, listLocal)}},
		Body:      body,
	})
	return stmts, nil
}

func (fl *functionLowerer) lowerForInMap(loop *checker.ForInMap) ([]Stmt, error) {
	intType, err := fl.l.internType(checker.Int)
	if err != nil {
		return nil, err
	}
	boolType, err := fl.l.internType(checker.Bool)
	if err != nil {
		return nil, err
	}
	m, err := fl.lowerExpr(loop.Map)
	if err != nil {
		return nil, err
	}
	mapType, ok := fl.l.typeInfo(m.Type)
	if !ok || mapType.Kind != TypeMap {
		return nil, fmt.Errorf("for-in map lowered with non-map subject %s", loop.Map.Type().String())
	}

	mapLocal := fl.defineLocal(loop.Key+"$map", m.Type, false)
	index := fl.defineLocal(loop.Key+"$index", intType, true)
	key := fl.defineLocal(loop.Key, mapType.Key, false)
	value := fl.defineLocal(loop.Val, mapType.Value, false)

	stmts := []Stmt{
		{Kind: StmtLet, Local: mapLocal, Name: loop.Key + "$map", Type: m.Type, Value: m},
		{Kind: StmtLet, Local: index, Name: loop.Key + "$index", Type: intType, Mutable: true, Value: &Expr{Kind: ExprConstInt, Type: intType, Int: 0}},
	}

	body, err := fl.lowerNonProducingBlock(loop.Body.Stmts)
	if err != nil {
		return nil, err
	}
	body.Stmts = append([]Stmt{
		{
			Kind:  StmtLet,
			Local: key,
			Name:  loop.Key,
			Type:  mapType.Key,
			Value: &Expr{Kind: ExprMapKeyAt, Type: mapType.Key, Target: loadLocal(m.Type, mapLocal), Args: []Expr{*loadLocal(intType, index)}},
		},
		{
			Kind:  StmtLet,
			Local: value,
			Name:  loop.Val,
			Type:  mapType.Value,
			Value: &Expr{Kind: ExprMapValueAt, Type: mapType.Value, Target: loadLocal(m.Type, mapLocal), Args: []Expr{*loadLocal(intType, index)}},
		},
	}, body.Stmts...)
	body.Stmts = append(body.Stmts, Stmt{
		Kind:  StmtAssign,
		Local: index,
		Value: &Expr{Kind: ExprIntAdd, Type: intType, Left: loadLocal(intType, index), Right: &Expr{Kind: ExprConstInt, Type: intType, Int: 1}},
	})

	stmts = append(stmts, Stmt{
		Kind:      StmtWhile,
		Condition: &Expr{Kind: ExprLt, Type: boolType, Left: loadLocal(intType, index), Right: &Expr{Kind: ExprMapSize, Type: intType, Target: loadLocal(m.Type, mapLocal)}},
		Body:      body,
	})
	return stmts, nil
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

func (fl *functionLowerer) lowerNonProducingBlock(stmts []checker.Statement) (Block, error) {
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

func (fl *functionLowerer) lowerExpr(expr checker.Expression) (*Expr, error) {
	if e, ok := expr.(*checker.Identifier); ok {
		local, ok, err := fl.resolveLocal(e.Name)
		if err != nil {
			return nil, err
		}
		if !ok {
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
			if def, ok := e.Type().(*checker.FunctionDef); ok {
				functionType, err := fl.internType(e.Type())
				if err != nil {
					return nil, err
				}
				id, err := fl.l.declareAndLowerFunction(fl.fn.Module, def)
				if err != nil {
					return nil, err
				}
				return &Expr{Kind: ExprMakeClosure, Type: functionType, Function: id}, nil
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
			args, err := fl.lowerArgsForFunctionType(e.Args, target.Type)
			if err != nil {
				return nil, err
			}
			typeInfo, ok := fl.l.typeInfo(target.Type)
			if !ok || typeInfo.Kind != TypeFunction {
				return nil, fmt.Errorf("local %s is not a function", e.Name)
			}
			return &Expr{Kind: ExprCallClosure, Type: typeInfo.Return, Target: target, Args: args}, nil
		}
	}
	typeID, err := fl.internType(expr.Type())
	if err != nil {
		return nil, err
	}
	switch e := expr.(type) {
	case *checker.VoidLiteral:
		return &Expr{Kind: ExprConstVoid, Type: typeID}, nil
	case *checker.IntLiteral:
		return &Expr{Kind: ExprConstInt, Type: typeID, Int: e.Value}, nil
	case *checker.FloatLiteral:
		return &Expr{Kind: ExprConstFloat, Type: typeID, Float: e.Value}, nil
	case *checker.BoolLiteral:
		return &Expr{Kind: ExprConstBool, Type: typeID, Bool: e.Value}, nil
	case *checker.StrLiteral:
		return &Expr{Kind: ExprConstStr, Type: typeID, Str: e.Value}, nil
	case *checker.Panic:
		message, err := fl.lowerExprWithExpected(e.Message, fl.l.mustIntern(checker.Str))
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprPanic, Type: typeID, Target: message}, nil
	case *checker.TemplateStr:
		return fl.lowerTemplateStr(typeID, e)
	case *checker.CopyExpression:
		value, err := fl.lowerExprWithExpected(e.Expr, typeID)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprCopy, Type: typeID, Target: value}, nil
	case *checker.FunctionDef:
		return fl.lowerClosure(typeID, e)
	case *checker.FiberStart:
		return fl.lowerFiberSpawn(typeID, e.GetFn())
	case *checker.FiberEval:
		return fl.lowerFiberSpawn(typeID, e.GetFn())
	case *checker.FiberExecution:
		return fl.lowerFiberExecution(typeID, e)
	case *checker.FunctionCall:
		if local, ok := fl.locals[e.Name]; ok && fl.localKind(local) == TypeFunction {
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
		if e.ExternalBinding != "" {
			id, err := fl.l.declareFunctionCallExtern(fl.fn.Module, e)
			if err != nil {
				return nil, err
			}
			args, err := fl.lowerArgsWithSignature(e.Args, fl.l.program.Externs[id].Signature)
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: ExprCallExtern, Type: fl.l.program.Externs[id].Signature.Return, Extern: id, Args: args}, nil
		}
		if def := e.Definition(); def != nil {
			if def.Body != nil {
				id, err := fl.l.declareAndLowerFunctionCall(fl.fn.Module, def, e)
				if err != nil {
					return nil, err
				}
				args, err := fl.lowerArgsWithSignature(e.Args, fl.l.program.Functions[id].Signature)
				if err != nil {
					return nil, err
				}
				return &Expr{Kind: ExprCall, Type: fl.l.program.Functions[id].Signature.Return, Function: id, Args: args}, nil
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
		if id, ok := fl.l.lookupExtern(e.Name); ok {
			args, err := fl.lowerArgsWithSignature(e.Args, fl.l.program.Externs[id].Signature)
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: ExprCallExtern, Type: fl.l.program.Externs[id].Signature.Return, Extern: id, Args: args}, nil
		}
		return nil, fmt.Errorf("unsupported unresolved function call %s", e.Name)
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
		moduleID := fl.l.internModule(e.Module)
		if e.Call.ExternalBinding != "" {
			id, err := fl.l.declareConcreteExternCall(moduleID, e.Call.Name, e.Call)
			if err != nil {
				return nil, err
			}
			args, err := fl.lowerArgsWithSignature(e.Call.Args, fl.l.program.Externs[id].Signature)
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: ExprCallExtern, Type: fl.l.program.Externs[id].Signature.Return, Extern: id, Args: args}, nil
		}
		if def := fl.l.moduleFunctionDefinitionForCall(e); def != nil && def.Body != nil {
			id, err := fl.l.declareAndLowerFunctionCall(moduleID, def, e.Call)
			if err != nil {
				return nil, err
			}
			args, err := fl.lowerArgsWithSignature(e.Call.Args, fl.l.program.Functions[id].Signature)
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: ExprCall, Type: fl.l.program.Functions[id].Signature.Return, Function: id, Args: args}, nil
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
		if id, ok, err := fl.l.resolveModuleExtern(e.Module, e.Call.Name); err != nil {
			return nil, err
		} else if ok {
			args, err := fl.lowerArgsWithSignature(e.Call.Args, fl.l.program.Externs[id].Signature)
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: ExprCallExtern, Type: fl.l.program.Externs[id].Signature.Return, Extern: id, Args: args}, nil
		}
		extern, err := fl.l.declareModuleCallExtern(e)
		if err != nil {
			return nil, err
		}
		args, err := fl.lowerArgsWithSignature(e.Call.Args, fl.l.program.Externs[extern].Signature)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprCallExtern, Type: fl.l.program.Externs[extern].Signature.Return, Extern: extern, Args: args}, nil
	case *checker.ModuleSymbol:
		return fl.lowerModuleSymbol(typeID, e)
	case *checker.ListLiteral:
		return fl.lowerListLiteral(typeID, e, NoType)
	case *checker.MapLiteral:
		return fl.lowerMapLiteral(typeID, e, NoType, NoType)
	case *checker.StructInstance:
		return fl.lowerStructInstance(typeID, e)
	case *checker.ModuleStructInstance:
		return fl.lowerStructInstance(typeID, e.Property)
	case *checker.InstanceProperty:
		return fl.lowerInstanceProperty(typeID, e)
	case *checker.InstanceMethod:
		return fl.lowerInstanceMethod(typeID, e)
	case *checker.StrMethod:
		return fl.lowerStrMethod(typeID, e)
	case *checker.IntMethod:
		if e.Kind == checker.IntToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
		}
		if e.Kind == checker.IntToDyn {
			return fl.lowerUnary(ExprToDynamic, typeID, e.Subject)
		}
		return nil, fmt.Errorf("unsupported AIR Int method %d", e.Kind)
	case *checker.FloatMethod:
		if e.Kind == checker.FloatToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
		}
		if e.Kind == checker.FloatToDyn {
			return fl.lowerUnary(ExprToDynamic, typeID, e.Subject)
		}
		return nil, fmt.Errorf("unsupported AIR Float method %d", e.Kind)
	case *checker.BoolMethod:
		if e.Kind == checker.BoolToStr {
			return fl.lowerUnary(ExprToStr, typeID, e.Subject)
		}
		if e.Kind == checker.BoolToDyn {
			return fl.lowerUnary(ExprToDynamic, typeID, e.Subject)
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
	case *checker.EnumMatch:
		return fl.lowerEnumMatch(typeID, e)
	case *checker.UnionMatch:
		return fl.lowerUnionMatch(typeID, e)
	case *checker.MaybeMethod:
		return fl.lowerMaybeMethod(typeID, e)
	case *checker.OptionMatch:
		return fl.lowerOptionMatch(typeID, e)
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
	condition, err := fl.lowerExpr(expr.Condition)
	if err != nil {
		return nil, err
	}
	thenBlock, err := fl.lowerBlockWithDefault(expr.Body.Stmts, typeID)
	if err != nil {
		return nil, err
	}
	elseBlock, err := fl.lowerElse(expr, typeID)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: ExprIf, Type: typeID, Condition: condition, Then: thenBlock, Else: elseBlock}, nil
}

func (fl *functionLowerer) lowerElse(expr *checker.If, defaultType TypeID) (Block, error) {
	if expr.ElseIf != nil {
		condition, err := fl.lowerExpr(expr.ElseIf.Condition)
		if err != nil {
			return Block{}, err
		}
		thenBlock, err := fl.lowerBlockWithDefault(expr.ElseIf.Body.Stmts, defaultType)
		if err != nil {
			return Block{}, err
		}
		elseBlock, err := fl.lowerElse(expr.ElseIf, defaultType)
		if err != nil {
			return Block{}, err
		}
		if expr.Else != nil {
			elseBlock, err = fl.lowerBlockWithDefault(expr.Else.Stmts, defaultType)
			if err != nil {
				return Block{}, err
			}
		}
		return Block{Result: &Expr{
			Kind:      ExprIf,
			Type:      defaultType,
			Condition: condition,
			Then:      thenBlock,
			Else:      elseBlock,
		}}, nil
	}
	if expr.Else != nil {
		return fl.lowerBlockWithDefault(expr.Else.Stmts, defaultType)
	}
	return Block{}, nil
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
	if !ok || subjectType.Kind != TypeInt {
		return nil, fmt.Errorf("int match lowered with non-int subject %s", match.Subject.Type().String())
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

	var kind ExprKind
	var expected []TypeID
	switch method.Kind {
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
	case checker.StrToStr:
		kind = ExprToStr
	case checker.StrToDyn:
		kind = ExprToDynamic
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
	if !ok || listType.Kind != TypeList {
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
	if !ok || mapType.Kind != TypeMap {
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
			return &Expr{Kind: ExprGetField, Type: typeID, Target: target, Field: field.Index}, nil
		}
	}
	return nil, fmt.Errorf("field %s not found on %s", prop.Property, targetInfo.Name)
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
	id, err := fl.l.declareClosureFunction(fl.fn.Module, def, typeID)
	if err != nil {
		return nil, err
	}
	fn := fl.l.program.Functions[id]
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
	def, ok := fl.l.moduleFunctionDefinitionForSymbol(symbol)
	if ok {
		module := fl.l.internModule(symbol.Module)
		id, err := fl.l.declareAndLowerFunction(module, def)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprMakeClosure, Type: typeID, Function: id}, nil
	}

	if def, ok := symbol.Symbol.Type.(*checker.ExternalFunctionDef); ok {
		id, err := fl.lowerExternModuleSymbol(typeID, symbol.Module, symbol.Symbol.Name, def)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprMakeClosure, Type: typeID, Function: id}, nil
	}

	return nil, fmt.Errorf("unsupported AIR module symbol %s::%s of type %s", symbol.Module, symbol.Symbol.Name, symbol.Type().String())
}

func (fl *functionLowerer) lowerExternModuleSymbol(typeID TypeID, modulePath string, name string, def *checker.ExternalFunctionDef) (FunctionID, error) {
	typeInfo, ok := fl.l.typeInfo(typeID)
	if !ok || typeInfo.Kind != TypeFunction {
		return NoFunction, fmt.Errorf("extern module symbol %s::%s lowered with non-function AIR type %d", modulePath, name, typeID)
	}
	module := fl.l.internModule(modulePath)
	extern, err := fl.l.declareExtern(module, def)
	if err != nil {
		return NoFunction, err
	}

	signature := fl.l.program.Externs[extern].Signature
	if len(signature.Params) != len(typeInfo.Params) {
		return NoFunction, fmt.Errorf("extern module symbol %s::%s expects %d params, got %d", modulePath, name, len(signature.Params), len(typeInfo.Params))
	}

	key := fmt.Sprintf("extern-symbol:%d:%s:%d", module, name, typeID)
	if id, ok := fl.l.functions[key]; ok {
		return id, nil
	}

	params := make([]Param, len(signature.Params))
	locals := make([]Local, len(signature.Params))
	args := make([]Expr, len(signature.Params))
	for i, param := range signature.Params {
		params[i] = param
		locals[i] = Local{ID: LocalID(i), Name: param.Name, Type: param.Type, Mutable: param.Mutable}
		args[i] = *loadLocal(param.Type, LocalID(i))
	}

	id := FunctionID(len(fl.l.program.Functions))
	fl.l.functions[key] = id
	fl.l.program.Functions = append(fl.l.program.Functions, Function{
		ID:     id,
		Module: module,
		Name:   modulePath + "::" + name,
		Signature: Signature{
			Params: params,
			Return: signature.Return,
		},
		Locals: locals,
		Body: Block{Result: &Expr{
			Kind:   ExprCallExtern,
			Type:   signature.Return,
			Extern: extern,
			Args:   args,
		}},
	})
	fl.l.program.Modules[module].Functions = appendUniqueFunction(fl.l.program.Modules[module].Functions, id)
	return id, nil
}

func (fl *functionLowerer) lowerFiberSpawn(typeID TypeID, fn checker.Expression) (*Expr, error) {
	target, err := fl.lowerExpr(fn)
	if err != nil {
		return nil, err
	}
	return &Expr{Kind: ExprSpawnFiber, Type: typeID, Target: target}, nil
}

func (fl *functionLowerer) lowerFiberExecution(typeID TypeID, exec *checker.FiberExecution) (*Expr, error) {
	id, ok, err := fl.l.resolveModuleFunction(exec.GetModule().Path(), exec.GetMainName())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("unknown fiber execution target %s::%s", exec.GetModule().Path(), exec.GetMainName())
	}
	return &Expr{Kind: ExprSpawnFiber, Type: typeID, Function: id}, nil
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
	if typeInfo.Kind == TypeStruct || typeInfo.Kind == TypeEnum {
		return fl.lowerUserInstanceMethod(typeID, target, typeInfo, method)
	}
	if typeInfo.Kind != TypeFiber {
		return nil, fmt.Errorf("unsupported AIR instance method %s on %s", method.Method.Name, method.Subject.Type().String())
	}
	switch method.Method.Name {
	case "get":
		return &Expr{Kind: ExprFiberGet, Type: typeID, Target: target}, nil
	case "join":
		return &Expr{Kind: ExprFiberJoin, Type: typeID, Target: target}, nil
	default:
		return nil, fmt.Errorf("unsupported AIR Fiber method %s", method.Method.Name)
	}
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
			if err := fl.l.lowerModule(mod, false); err != nil {
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
	id, err := fl.l.declareInstanceMethodFunction(module, typeInfo.Name, target.Type, def)
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
	local, _, ok, err := fl.captureLocal(name)
	return local, ok, err
}

func (fl *functionLowerer) ensureLocalForNestedCapture(name string) (LocalID, TypeID, bool, error) {
	if local, ok := fl.locals[name]; ok {
		return local, fl.fn.Locals[local].Type, true, nil
	}
	if fl.parent == nil {
		return 0, NoType, false, nil
	}
	return fl.captureLocal(name)
}

func (fl *functionLowerer) captureLocal(name string) (LocalID, TypeID, bool, error) {
	if fl.captureByName == nil {
		fl.captureByName = map[string]LocalID{}
	}
	if local, ok := fl.captureByName[name]; ok {
		return local, fl.fn.Locals[local].Type, true, nil
	}
	sourceLocal, typeID, ok, err := fl.parent.ensureLocalForNestedCapture(name)
	if err != nil || !ok {
		return 0, NoType, ok, err
	}
	local := fl.defineLocal(name, typeID, false)
	fl.captureByName[name] = local
	fl.captureLocals = append(fl.captureLocals, sourceLocal)
	fl.fn.Captures = append(fl.fn.Captures, Capture{Name: name, Type: typeID, Local: local})
	return local, typeID, true, nil
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

func (l *lowerer) lookupFunctionInModule(modulePath, name string) (FunctionID, bool) {
	moduleID, ok := l.moduleByPath[modulePath]
	if !ok {
		return NoFunction, false
	}
	id, ok := l.functions[functionKey(moduleID, name)]
	return id, ok
}

func (l *lowerer) lookupExternInModule(modulePath, name string) (ExternID, bool) {
	moduleID, ok := l.moduleByPath[modulePath]
	if !ok {
		return 0, false
	}
	id, ok := l.externs[functionKey(moduleID, name)]
	return id, ok
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
	if err := l.lowerModule(mod, false); err != nil {
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
	switch {
	case method.StructType != nil:
		ownerName = method.StructType.Name
	case method.EnumType != nil:
		ownerName = method.EnumType.Name
	default:
		return fallback
	}
	for modulePath, mod := range l.moduleByName {
		if mod.Program() == nil {
			continue
		}
		for _, stmt := range mod.Program().Statements {
			switch def := stmt.Stmt.(type) {
			case *checker.StructDef:
				if def.Name == ownerName && def.Methods[method.Method.Name] != nil {
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

func (l *lowerer) resolveModuleExtern(modulePath, name string) (ExternID, bool, error) {
	if id, ok := l.lookupExternInModule(modulePath, name); ok {
		return id, true, nil
	}

	mod, ok := l.moduleByName[modulePath]
	if !ok {
		return 0, false, nil
	}
	if mod.Program() == nil {
		return 0, false, nil
	}
	if err := l.lowerModule(mod, false); err != nil {
		return 0, false, err
	}
	id, ok := l.lookupExternInModule(modulePath, name)
	return id, ok, nil
}

func (l *lowerer) lookupExtern(name string) (ExternID, bool) {
	for key, id := range l.externs {
		if keyHasFunctionName(key, name) {
			return id, true
		}
	}
	return 0, false
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
		case *checker.FunctionDef, *checker.ExternalFunctionDef:
			continue
		}
		if stmt.Stmt != nil {
			switch stmt.Stmt.(type) {
			case *checker.StructDef, *checker.Enum, *checker.Union, *checker.ExternType:
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

func appendUniqueFunction(items []FunctionID, id FunctionID) []FunctionID {
	for _, item := range items {
		if item == id {
			return items
		}
	}
	return append(items, id)
}

func functionKey(module ModuleID, name string) string {
	return fmt.Sprintf("%d:%s", module, name)
}

func concreteFunctionKey(module ModuleID, name string, signature Signature) string {
	return fmt.Sprintf("%d:%s:%s", module, name, signatureKey(signature))
}

func concreteExternKey(module ModuleID, name string, signature Signature) string {
	return fmt.Sprintf("%d:extern:%s:%s", module, name, signatureKey(signature))
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
