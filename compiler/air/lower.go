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
	typeByName   map[string]TypeID
	traits       map[string]TraitID
	impls        map[string]ImplID
	functions    map[string]FunctionID
	externs      map[string]ExternID

	loweringModules map[string]bool
	loweredModules  map[string]bool
}

type functionLowerer struct {
	l             *lowerer
	locals        map[string]LocalID
	fn            *Function
	parent        *functionLowerer
	captureByName map[string]LocalID
	captureLocals []LocalID
}

func newLowerer() *lowerer {
	l := &lowerer{
		program: Program{
			Entry:  NoFunction,
			Script: NoFunction,
		},
		moduleByPath: map[string]ModuleID{},
		moduleByName: map[string]checker.Module{},
		typeByName:   map[string]TypeID{},
		traits:       map[string]TraitID{},
		impls:        map[string]ImplID{},
		functions:    map[string]FunctionID{},
		externs:      map[string]ExternID{},

		loweringModules: map[string]bool{},
		loweredModules:  map[string]bool{},
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
			typeID, err := l.internType(node)
			if err != nil {
				return err
			}
			mod.Types = appendUniqueType(mod.Types, typeID)
		}

		switch expr := stmt.Expr.(type) {
		case *checker.FunctionDef:
			if _, err := l.declareFunction(modID, expr, includeTests); err != nil {
				return err
			}
		case *checker.ExternalFunctionDef:
			if _, err := l.declareExtern(modID, expr); err != nil {
				return err
			}
		}
	}

	for i := range prog.Statements {
		stmt := prog.Statements[i]
		switch node := stmt.Stmt.(type) {
		case *checker.StructDef:
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

	fn := l.program.Functions[id]
	fl := &functionLowerer{l: l, locals: map[string]LocalID{}, fn: &fn}
	for _, param := range fn.Signature.Params {
		fl.defineLocal(param.Name, param.Type, param.Mutable)
	}
	if def.Body == nil {
		return nil
	}
	body, err := fl.lowerBlock(def.Body.Stmts)
	if err != nil {
		return fmt.Errorf("lower function %s: %w", def.Name, err)
	}
	fn.Body = body
	l.program.Functions[id] = fn
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
	if trait.Name != "ToString" || len(trait.Methods) != 1 || trait.Methods[0].Name != "to_str" {
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

	methodID, err := l.declareBuiltinToStringMethod(module, ownerInfo)
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
	fl := &functionLowerer{l: l, locals: map[string]LocalID{}, fn: &fn}
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

func (l *lowerer) declareExtern(module ModuleID, def *checker.ExternalFunctionDef) (ExternID, error) {
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
	key := functionKey(module, call.Name)
	if id, ok := l.externs[key]; ok {
		return id, nil
	}
	params := make([]Param, len(call.Args))
	if def := call.Definition(); def != nil {
		params = make([]Param, len(def.Parameters))
		for i, param := range def.Parameters {
			typeID, err := l.internType(param.Type)
			if err != nil {
				return 0, err
			}
			params[i] = Param{Name: param.Name, Type: typeID, Mutable: param.Mutable}
		}
	} else {
		for i, arg := range call.Args {
			typeID, err := l.internType(arg.Type())
			if err != nil {
				return 0, err
			}
			params[i] = Param{Name: fmt.Sprintf("arg%d", i), Type: typeID}
		}
	}
	returnType, err := l.internType(call.Type())
	if err != nil {
		return 0, err
	}
	id := ExternID(len(l.program.Externs))
	l.externs[key] = id
	l.program.Externs = append(l.program.Externs, Extern{
		ID:     id,
		Module: module,
		Name:   call.Name,
		Signature: Signature{
			Params: params,
			Return: returnType,
		},
		Bindings: map[string]string{"go": call.ExternalBinding},
	})
	return id, nil
}

func (l *lowerer) internType(t checker.Type) (TypeID, error) {
	if t == nil {
		return NoType, fmt.Errorf("cannot intern nil type")
	}
	if tv, ok := t.(*checker.TypeVar); ok && tv.Actual() != nil {
		return l.internType(tv.Actual())
	}
	name := airTypeName(t)
	if id, ok := l.typeByName[name]; ok {
		return id, nil
	}

	id := TypeID(len(l.program.Types) + 1)
	l.typeByName[name] = id
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

func airTypeName(t checker.Type) string {
	if typ, ok := t.(*checker.StructDef); ok && typ.Name == "Fiber" {
		if elem, ok := typ.Fields["result"]; ok {
			return "Fiber<" + elem.String() + ">"
		}
	}
	return t.String()
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
			expr, err := fl.lowerExprWithExpected(stmt.Expr, defaultType)
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

func (fl *functionLowerer) lowerExprWithExpected(expr checker.Expression, expected TypeID) (*Expr, error) {
	if wrapped, ok, err := fl.lowerUnionWrapIfNeeded(expr, expected); ok || err != nil {
		return wrapped, err
	}
	if wrapped, ok, err := fl.lowerTraitUpcastIfNeeded(expr, expected); ok || err != nil {
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
	if ifExpr, ok := expr.(*checker.If); ok {
		return fl.lowerIf(expected, ifExpr)
	}
	return fl.lowerExpr(expr)
}

func (fl *functionLowerer) lowerUnionWrapIfNeeded(expr checker.Expression, expected TypeID) (*Expr, bool, error) {
	expectedInfo, ok := fl.l.typeInfo(expected)
	if !ok || expectedInfo.Kind != TypeUnion {
		return nil, false, nil
	}
	actual, err := fl.l.internType(expr.Type())
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
	actual, err := fl.l.internType(expr.Type())
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
		typeID, err := fl.l.internType(s.Type())
		if err != nil {
			return nil, err
		}
		local := fl.defineLocal(s.Name, typeID, s.Mutable)
		value, err := fl.lowerExprWithExpected(s.Value, typeID)
		if err != nil {
			return nil, err
		}
		return &Stmt{Kind: StmtLet, Local: local, Name: s.Name, Type: typeID, Mutable: s.Mutable, Value: value}, nil
	case *checker.Reassignment:
		target, ok := s.Target.(*checker.Variable)
		if !ok {
			return nil, fmt.Errorf("unsupported AIR assignment target %T", s.Target)
		}
		local, ok := fl.locals[target.Name()]
		if !ok {
			return nil, fmt.Errorf("assignment to unknown local %s", target.Name())
		}
		value, err := fl.lowerExprWithExpected(s.Value, fl.fn.Locals[local].Type)
		if err != nil {
			return nil, err
		}
		return &Stmt{Kind: StmtAssign, Local: local, Value: value}, nil
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
	if loop, ok := stmt.Stmt.(*checker.ForInList); ok {
		return fl.lowerForInList(loop)
	}
	if loop, ok := stmt.Stmt.(*checker.ForInMap); ok {
		return fl.lowerForInMap(loop)
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

	cursor := fl.defineLocal(loop.Cursor, intType, true)
	endLocal := fl.defineLocal(loop.Cursor+"$end", intType, false)
	stmts := []Stmt{
		{Kind: StmtLet, Local: cursor, Name: loop.Cursor, Type: intType, Mutable: true, Value: start},
		{Kind: StmtLet, Local: endLocal, Name: loop.Cursor + "$end", Type: intType, Value: end},
	}

	var index LocalID
	if loop.Index != "" {
		index = fl.defineLocal(loop.Index, intType, true)
		stmts = append(stmts, Stmt{Kind: StmtLet, Local: index, Name: loop.Index, Type: intType, Mutable: true, Value: &Expr{Kind: ExprConstInt, Type: intType, Int: 0}})
	}

	body, err := fl.lowerNonProducingBlock(loop.Body.Stmts)
	if err != nil {
		return nil, err
	}
	body.Stmts = append(body.Stmts, Stmt{
		Kind:  StmtAssign,
		Local: cursor,
		Value: &Expr{Kind: ExprIntAdd, Type: intType, Left: loadLocal(intType, cursor), Right: &Expr{Kind: ExprConstInt, Type: intType, Int: 1}},
	})
	if loop.Index != "" {
		body.Stmts = append(body.Stmts, Stmt{
			Kind:  StmtAssign,
			Local: index,
			Value: &Expr{Kind: ExprIntAdd, Type: intType, Left: loadLocal(intType, index), Right: &Expr{Kind: ExprConstInt, Type: intType, Int: 1}},
		})
	}

	stmts = append(stmts, Stmt{
		Kind:      StmtWhile,
		Condition: &Expr{Kind: ExprLte, Type: boolType, Left: loadLocal(intType, cursor), Right: loadLocal(intType, endLocal)},
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
	indexName := loop.Index
	if indexName == "" {
		indexName = loop.Cursor + "$index"
	}
	index := fl.defineLocal(indexName, intType, true)
	cursor := fl.defineLocal(loop.Cursor, listType.Elem, false)

	stmts := []Stmt{
		{Kind: StmtLet, Local: listLocal, Name: loop.Cursor + "$list", Type: list.Type, Value: list},
		{Kind: StmtLet, Local: index, Name: indexName, Type: intType, Mutable: true, Value: &Expr{Kind: ExprConstInt, Type: intType, Int: 0}},
	}

	body, err := fl.lowerNonProducingBlock(loop.Body.Stmts)
	if err != nil {
		return nil, err
	}
	body.Stmts = append([]Stmt{{
		Kind:  StmtLet,
		Local: cursor,
		Name:  loop.Cursor,
		Type:  listType.Elem,
		Value: &Expr{Kind: ExprListAt, Type: listType.Elem, Target: loadLocal(list.Type, listLocal), Args: []Expr{*loadLocal(intType, index)}},
	}}, body.Stmts...)
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
	typeID, err := fl.l.internType(expr.Type())
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
	case *checker.TemplateStr:
		return fl.lowerTemplateStr(typeID, e)
	case *checker.CopyExpression:
		value, err := fl.lowerExprWithExpected(e.Expr, typeID)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprCopy, Type: typeID, Target: value}, nil
	case *checker.Variable:
		local, ok, err := fl.resolveLocal(e.Name())
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("unknown local %s", e.Name())
		}
		return &Expr{Kind: ExprLoadLocal, Type: typeID, Local: local}, nil
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
	case *checker.ListLiteral:
		return fl.lowerListLiteral(typeID, e, NoType)
	case *checker.MapLiteral:
		return fl.lowerMapLiteral(typeID, e, NoType, NoType)
	case *checker.StructInstance:
		return fl.lowerStructInstance(typeID, e)
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
	case *checker.If:
		return fl.lowerIf(typeID, e)
	default:
		return nil, fmt.Errorf("unsupported AIR expression %T", expr)
	}
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
		errType, err := fl.l.internType(op.ErrType)
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
	oldLocal, hadOldLocal := fl.locals[name]
	local := fl.defineLocal(name, typeID, false)
	block, err := fl.lowerBlockWithDefault(stmts, defaultType)
	if hadOldLocal {
		fl.locals[name] = oldLocal
	} else {
		delete(fl.locals, name)
	}
	return local, block, err
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

func (fl *functionLowerer) lowerClosure(typeID TypeID, def *checker.FunctionDef) (*Expr, error) {
	id, err := fl.l.declareFunction(fl.fn.Module, def, false)
	if err != nil {
		return nil, err
	}
	fn := fl.l.program.Functions[id]
	child := &functionLowerer{
		l:             fl.l,
		locals:        map[string]LocalID{},
		fn:            &fn,
		parent:        fl,
		captureByName: map[string]LocalID{},
	}
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
