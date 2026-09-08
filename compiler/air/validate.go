package air

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

func validGoStructTagKey(key string) bool {
	prefixed := false
	for index, char := range key {
		letter := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char == '_'
		if index == 0 {
			if char == '$' {
				prefixed = true
				continue
			}
			if !letter {
				return false
			}
			continue
		}
		if prefixed && index == 1 && !letter {
			return false
		}
		if !letter && (char < '0' || char > '9') {
			return false
		}
	}
	return key != "" && key != "$"
}

func Validate(program *Program) error {
	if program == nil {
		return fmt.Errorf("AIR program is nil")
	}
	for i, typ := range program.Types {
		if typ.ID != TypeID(i+1) {
			return fmt.Errorf("type table entry %d has id %d", i, typ.ID)
		}
		if err := validateTypeInfo(program, typ); err != nil {
			return err
		}
	}
	if err := validateCanonicalNominalIdentities(program); err != nil {
		return err
	}
	for i, trait := range program.Traits {
		if trait.ID != TraitID(i) {
			return fmt.Errorf("trait table entry %d has id %d", i, trait.ID)
		}
		if err := validateTrait(program, trait); err != nil {
			return err
		}
	}
	for i, global := range program.Globals {
		if global.ID != GlobalID(i) {
			return fmt.Errorf("global table entry %d has id %d", i, global.ID)
		}
		if err := validateGlobal(program, global); err != nil {
			return err
		}
	}
	for i, fn := range program.Functions {
		if fn.ID != FunctionID(i) {
			return fmt.Errorf("function table entry %d has id %d", i, fn.ID)
		}
		if err := validateFunction(program, fn); err != nil {
			return err
		}
	}
	for i, impl := range program.Impls {
		if impl.ID != ImplID(i) {
			return fmt.Errorf("impl table entry %d has id %d", i, impl.ID)
		}
		if err := validateImpl(program, impl); err != nil {
			return err
		}
	}
	if program.Entry != NoFunction && !validFunctionID(program, program.Entry) {
		return fmt.Errorf("invalid entry function id %d", program.Entry)
	}
	if program.Script != NoFunction && !validFunctionID(program, program.Script) {
		return fmt.Errorf("invalid script function id %d", program.Script)
	}
	for _, test := range program.Tests {
		if !validFunctionID(program, test.Function) {
			return fmt.Errorf("test %s references invalid function %d", test.Name, test.Function)
		}
	}
	return nil
}

func validateCanonicalNominalIdentities(program *Program) error {
	seen := map[nominalTypeKey]TypeID{}
	params := map[typeParamKey]TypeID{}
	for _, typ := range program.Types {
		var key nominalTypeKey
		var nominal bool
		switch {
		case typ.Kind == TypeStruct && typ.Generic != NoType:
			key = applicationNominalKey(TypeStruct, typ.Generic, typ.GenericArgs)
			nominal = true
		case typ.Kind == TypeStruct:
			key = declarationNominalKey(TypeStruct, typ.ModulePath, typ.Name)
			nominal = true
		case typ.Kind == TypeEnum:
			key = declarationNominalKey(TypeEnum, typ.ModulePath, typ.Name)
			nominal = true
		case typ.Kind == TypeUnion:
			key = declarationNominalKey(TypeUnion, typ.ModulePath, typ.Name)
			nominal = true
		case typ.Kind == TypeForeignType:
			key = nominalTypeKey{Kind: TypeForeignType, Target: typ.ForeignTarget, Namespace: typ.ForeignNamespace, Symbol: typ.ForeignSymbol, Pointer: typ.ForeignPointer, Args: typeIDsKey(typ.GenericArgs)}
			nominal = true
		case typ.Kind == TypeParam:
			param := typeParamKey{owner: typ.ParamOwner, index: typ.ParamIndex}
			if previous, ok := params[param]; ok {
				return fmt.Errorf("type parameters %d and %d duplicate owner %q index %d", previous, typ.ID, typ.ParamOwner, typ.ParamIndex)
			}
			params[param] = typ.ID
		}
		if nominal {
			if previous, ok := seen[key]; ok {
				return fmt.Errorf("nominal types %d and %d have duplicate identity", previous, typ.ID)
			}
			seen[key] = typ.ID
		}
	}
	return nil
}

func validateTypeInfo(program *Program, typ TypeInfo) error {
	switch typ.Kind {
	case TypeList, TypeSlice, TypeMaybe, TypeChannel, TypeReceiver, TypeSender, TypeReference:
		if !validTypeID(program, typ.Elem) {
			return fmt.Errorf("type %s has invalid elem type %d", typ.Name, typ.Elem)
		}
	case TypeFixedArray:
		if typ.Length < 0 {
			return fmt.Errorf("type %s has invalid fixed array length %d", typ.Name, typ.Length)
		}
		if !validTypeID(program, typ.Elem) {
			return fmt.Errorf("type %s has invalid elem type %d", typ.Name, typ.Elem)
		}
	case TypeMap:
		if !validTypeID(program, typ.Key) {
			return fmt.Errorf("type %s has invalid key type %d", typ.Name, typ.Key)
		}
		if !validTypeID(program, typ.Value) {
			return fmt.Errorf("type %s has invalid value type %d", typ.Name, typ.Value)
		}
	case TypeResult:
		if !validTypeID(program, typ.Value) {
			return fmt.Errorf("type %s has invalid ok type %d", typ.Name, typ.Value)
		}
		if !validTypeID(program, typ.Error) {
			return fmt.Errorf("type %s has invalid err type %d", typ.Name, typ.Error)
		}
	case TypeStruct:
		jsonNames := make(map[string]bool, len(typ.Fields))
		jsonRepresentableFields := 0
		for i, field := range typ.Fields {
			if field.Index != i {
				return fmt.Errorf("type %s field %s has index %d, want %d", typ.Name, field.Name, field.Index, i)
			}
			if !validTypeID(program, field.Type) {
				return fmt.Errorf("type %s field %s has invalid type %d", typ.Name, field.Name, field.Type)
			}
			goTagKeys := make(map[string]bool, len(field.GoTags))
			for _, tag := range field.GoTags {
				if !validGoStructTagKey(tag.Key) {
					return fmt.Errorf("type %s field %s has invalid Go struct tag key %q", typ.Name, field.Name, tag.Key)
				}
				if tag.Key == "json" {
					return fmt.Errorf("type %s field %s has reserved Go struct tag key json", typ.Name, field.Name)
				}
				if goTagKeys[tag.Key] {
					return fmt.Errorf("type %s field %s has duplicate Go struct tag key %q", typ.Name, field.Name, tag.Key)
				}
				goTagKeys[tag.Key] = true
			}
			if field.JSON.OmitNone && program.Types[field.Type-1].Kind != TypeMaybe {
				return fmt.Errorf("type %s field %s omits none but is not Maybe", typ.Name, field.Name)
			}
			if field.JSON.Skip && (field.JSON.HasName || field.JSON.OmitNone) {
				return fmt.Errorf("type %s field %s has conflicting JSON metadata", typ.Name, field.Name)
			}
			if field.JSON.HasName && !checker.JSONFieldNameRepresentable(field.JSON.Name) {
				return fmt.Errorf("type %s field %s has unrepresentable JSON field name %q", typ.Name, field.Name, field.JSON.Name)
			}
			if !field.JSON.Skip {
				jsonRepresentableFields++
				jsonName := field.Name
				if field.JSON.HasName {
					jsonName = field.JSON.Name
				}
				if jsonNames[jsonName] {
					return fmt.Errorf("type %s has duplicate JSON field name %q", typ.Name, jsonName)
				}
				jsonNames[jsonName] = true
			}
		}
		if len(typ.Fields) > 0 && jsonRepresentableFields == 0 {
			return fmt.Errorf("type %s has no JSON-representable fields", typ.Name)
		}
	case TypeUnion:
		for _, member := range typ.Members {
			if !validTypeID(program, member.Type) {
				return fmt.Errorf("type %s union member %s has invalid type %d", typ.Name, member.Name, member.Type)
			}
		}
	case TypeFunction:
		if typ.Variadic && len(typ.Params) == 0 {
			return fmt.Errorf("type %s is variadic without a parameter", typ.Name)
		}
		for _, param := range typ.Params {
			if !validTypeID(program, param) {
				return fmt.Errorf("type %s has invalid function param type %d", typ.Name, param)
			}
		}
		if !validTypeID(program, typ.Return) {
			return fmt.Errorf("type %s has invalid function return type %d", typ.Name, typ.Return)
		}
	case TypeTraitObject:
		if !validTraitID(program, typ.Trait) {
			return fmt.Errorf("type %s has invalid trait id %d", typ.Name, typ.Trait)
		}
	}
	return nil
}

func validateTrait(program *Program, trait Trait) error {
	for _, method := range trait.Methods {
		if err := validateSignature(program, method.Signature); err != nil {
			return fmt.Errorf("trait %s method %s: %w", trait.Name, method.Name, err)
		}
	}
	return nil
}

func validateImpl(program *Program, impl Impl) error {
	if !validTraitID(program, impl.Trait) {
		return fmt.Errorf("impl %d has invalid trait id %d", impl.ID, impl.Trait)
	}
	if !validTypeID(program, impl.ForType) {
		return fmt.Errorf("impl %d has invalid type id %d", impl.ID, impl.ForType)
	}
	trait := program.Traits[impl.Trait]
	if len(impl.Methods) != len(trait.Methods) {
		return fmt.Errorf("impl %d has %d methods, trait %s requires %d", impl.ID, len(impl.Methods), trait.Name, len(trait.Methods))
	}
	for i, methodID := range impl.Methods {
		if !validFunctionID(program, methodID) {
			return fmt.Errorf("impl %d method %d has invalid function id %d", impl.ID, i, methodID)
		}
		method := program.Functions[methodID]
		traitMethod := trait.Methods[i]
		if len(method.Signature.Params) != len(traitMethod.Signature.Params)+1 {
			return fmt.Errorf("impl %d method %s has %d params, want receiver plus %d trait params", impl.ID, method.Name, len(method.Signature.Params), len(traitMethod.Signature.Params))
		}
		receiverType := method.Signature.Params[0].Type
		receiver, err := typeInfo(program, receiverType)
		if err != nil {
			return fmt.Errorf("impl %d method %s receiver: %w", impl.ID, method.Name, err)
		}
		receiverMutates := receiver.Kind == TypeReference
		receiverMatches := receiverType == impl.ForType || (receiverMutates && receiver.Elem == impl.ForType)
		if !receiverMatches {
			return fmt.Errorf("impl %d method %s receiver type %d does not match impl type %d", impl.ID, method.Name, receiverType, impl.ForType)
		}
		if receiverMutates && !traitMethod.Mutates {
			return fmt.Errorf("impl %d method %s mutates its receiver, but trait %s does not allow receiver mutation", impl.ID, method.Name, trait.Name)
		}
		for paramIndex, traitParam := range traitMethod.Signature.Params {
			methodParam := method.Signature.Params[paramIndex+1]
			if methodParam.Type != traitParam.Type {
				return fmt.Errorf("impl %d method %s param %d type %d does not match trait type %d", impl.ID, method.Name, paramIndex, methodParam.Type, traitParam.Type)
			}
		}
		if method.Signature.Return != traitMethod.Signature.Return {
			return fmt.Errorf("impl %d method %s return type %d does not match trait return type %d", impl.ID, method.Name, method.Signature.Return, traitMethod.Signature.Return)
		}
	}
	return nil
}

func validateGlobal(program *Program, global Global) error {
	if int(global.Module) < 0 || int(global.Module) >= len(program.Modules) {
		return fmt.Errorf("global %s has invalid module id %d", global.Name, global.Module)
	}
	if !validTypeID(program, global.Type) {
		return fmt.Errorf("global %s has invalid type %d", global.Name, global.Type)
	}
	initializer := global.Initializer
	if initializer.Value.Type == NoType {
		return fmt.Errorf("global %s has no initializer", global.Name)
	}
	if initializer.Value.Type != global.Type {
		return fmt.Errorf("global %s initializer type %d does not match global type %d", global.Name, initializer.Value.Type, global.Type)
	}
	for i, local := range initializer.Locals {
		if local.ID != LocalID(i) {
			return fmt.Errorf("global %s initializer local table entry %d has id %d", global.Name, i, local.ID)
		}
		if !validTypeID(program, local.Type) {
			return fmt.Errorf("global %s initializer local %s has invalid type %d", global.Name, local.Name, local.Type)
		}
	}
	context := Function{Module: global.Module, Name: "<global:" + global.Name + ">", Locals: initializer.Locals}
	if err := validateExpr(program, context, initializer.Value); err != nil {
		return fmt.Errorf("global %s: %w", global.Name, err)
	}
	return nil
}

func validateFunction(program *Program, fn Function) error {
	if fn.RequiredGoMethodName != "" && (fn.Receiver == NoType || fn.MethodName == "") {
		return fmt.Errorf("function %s requires Go method %s without method receiver metadata", fn.Name, fn.RequiredGoMethodName)
	}
	if int(fn.Module) < 0 || int(fn.Module) >= len(program.Modules) {
		return fmt.Errorf("function %s has invalid module id %d", fn.Name, fn.Module)
	}
	if err := validateSignature(program, fn.Signature); err != nil {
		return fmt.Errorf("function %s: %w", fn.Name, err)
	}
	for _, local := range fn.Locals {
		if !validTypeID(program, local.Type) {
			return fmt.Errorf("function %s local %s has invalid type %d", fn.Name, local.Name, local.Type)
		}
	}
	for _, capture := range fn.Captures {
		if capture.Mode > CaptureSlot {
			return fmt.Errorf("function %s capture %s has invalid mode %d", fn.Name, capture.Name, capture.Mode)
		}
		if !validTypeID(program, capture.Type) {
			return fmt.Errorf("function %s capture %s has invalid type %d", fn.Name, capture.Name, capture.Type)
		}
		if capture.Local < 0 || int(capture.Local) >= len(fn.Locals) {
			return fmt.Errorf("function %s capture %s has invalid local %d", fn.Name, capture.Name, capture.Local)
		}
		if fn.Locals[capture.Local].Type != capture.Type {
			return fmt.Errorf("function %s capture %s local type %d does not match capture type %d", fn.Name, capture.Name, fn.Locals[capture.Local].Type, capture.Type)
		}
	}
	if err := validateBlock(program, fn, fn.Body); err != nil {
		return fmt.Errorf("function %s: %w", fn.Name, err)
	}
	returnType := program.Types[fn.Signature.Return-1]
	if returnType.Kind != TypeVoid {
		if fn.Body.Result == nil {
			return fmt.Errorf("function %s has no result for return type %d", fn.Name, fn.Signature.Return)
		}
		if !typesAssignable(program, fn.Signature.Return, fn.Body.Result.Type) {
			return fmt.Errorf("function %s result type %d does not match return type %d", fn.Name, fn.Body.Result.Type, fn.Signature.Return)
		}
	}
	return nil
}

func validateSignature(program *Program, sig Signature) error {
	for _, param := range sig.Params {
		if !validTypeID(program, param.Type) {
			return fmt.Errorf("parameter %s has invalid type %d", param.Name, param.Type)
		}
		if err := validateABIParamMode(program, param.Type, param.ABI); err != nil {
			return fmt.Errorf("parameter %s: %w", param.Name, err)
		}
	}
	if !validTypeID(program, sig.Return) {
		return fmt.Errorf("signature has invalid return type %d", sig.Return)
	}
	return nil
}

func validateABIParamMode(program *Program, typeID TypeID, mode ABIParamMode) error {
	if mode > ABIParamDescriptorValue {
		return fmt.Errorf("invalid ABI parameter mode %d", mode)
	}
	if mode != ABIParamDescriptorValue {
		return nil
	}
	if !validTypeID(program, typeID) {
		return fmt.Errorf("descriptor-value ABI has invalid type %d", typeID)
	}
	reference := program.Types[typeID-1]
	if reference.Kind != TypeReference || !validTypeID(program, reference.Elem) {
		return fmt.Errorf("descriptor-value ABI requires a reference type, got %s", reference.Name)
	}
	referent := program.Types[reference.Elem-1]
	if referent.Kind == TypeList || referent.Kind == TypeSlice || referent.Kind == TypeMap {
		return nil
	}
	if referent.Kind == TypeForeignType && !referent.ForeignPointer &&
		(referent.Elem != NoType || referent.Key != NoType && referent.Value != NoType) {
		return nil
	}
	return fmt.Errorf("descriptor-value ABI requires a slice or map referent, got %s", referent.Name)
}

func validateBlock(program *Program, fn Function, block Block) error {
	for _, stmt := range block.Stmts {
		if stmt.Type != NoType && !validTypeID(program, stmt.Type) {
			return fmt.Errorf("statement has invalid type %d", stmt.Type)
		}
		if stmt.Value != nil {
			if err := validateExpr(program, fn, *stmt.Value); err != nil {
				return err
			}
		}
		if stmt.Expr != nil {
			if err := validateExpr(program, fn, *stmt.Expr); err != nil {
				return err
			}
		}
		if stmt.Target != nil {
			if err := validateExpr(program, fn, *stmt.Target); err != nil {
				return err
			}
		}
		if stmt.Condition != nil {
			if err := validateExpr(program, fn, *stmt.Condition); err != nil {
				return err
			}
		}
		if stmt.Predeclare && stmt.Kind != StmtLet {
			return fmt.Errorf("only let statements may predeclare a local")
		}
		if stmt.LocalFunction {
			if stmt.Kind != StmtLet || stmt.Value == nil || stmt.Value.Kind != ExprMakeClosure {
				return fmt.Errorf("local function binding must be a closure-valued let")
			}
		}
		if stmt.Kind == StmtLet || stmt.Kind == StmtAssign {
			if stmt.Local < 0 || int(stmt.Local) >= len(fn.Locals) {
				return fmt.Errorf("local statement references invalid local %d", stmt.Local)
			}
			if stmt.Value == nil {
				return fmt.Errorf("local statement for %s has no value", fn.Locals[stmt.Local].Name)
			}
			localType := fn.Locals[stmt.Local].Type
			if stmt.Kind == StmtLet && stmt.Type != localType {
				return fmt.Errorf("let statement type %d does not match local %s type %d", stmt.Type, fn.Locals[stmt.Local].Name, localType)
			}
			if !typesAssignable(program, localType, stmt.Value.Type) {
				return fmt.Errorf("local %s initializer type %d (%s) does not match local type %d (%s)", fn.Locals[stmt.Local].Name, stmt.Value.Type, program.Types[stmt.Value.Type-1].Name, localType, program.Types[localType-1].Name)
			}
		}
		if stmt.Kind == StmtAssignGlobal {
			if !validGlobalID(program, stmt.Global) {
				return fmt.Errorf("global assignment references invalid global %d", stmt.Global)
			}
			if stmt.Value == nil {
				return fmt.Errorf("global assignment missing value")
			}
			if !program.Globals[stmt.Global].Mutable {
				return fmt.Errorf("assignment to immutable global %s", program.Globals[stmt.Global].Name)
			}
			if stmt.Type != NoType && stmt.Type != program.Globals[stmt.Global].Type {
				return fmt.Errorf("global assignment type %d does not match global type %d", stmt.Type, program.Globals[stmt.Global].Type)
			}
			if !typesAssignable(program, program.Globals[stmt.Global].Type, stmt.Value.Type) {
				return fmt.Errorf("global assignment value type %d does not match global type %d", stmt.Value.Type, program.Globals[stmt.Global].Type)
			}
		}
		if stmt.Kind == StmtSetField {
			if stmt.Target == nil {
				return fmt.Errorf("field set statement missing target")
			}
			targetType, err := referentTypeInfo(program, stmt.Target.Type)
			if err != nil {
				return err
			}
			if targetType.Kind != TypeStruct {
				return fmt.Errorf("field set target has type kind %d", targetType.Kind)
			}
			if stmt.Field < 0 || stmt.Field >= len(targetType.Fields) {
				return fmt.Errorf("field set index %d out of range for %s", stmt.Field, targetType.Name)
			}
			if targetType.Fields[stmt.Field].Type != stmt.Type {
				return fmt.Errorf("field set type %d does not match field type %d", stmt.Type, targetType.Fields[stmt.Field].Type)
			}
		}
		if stmt.Kind == StmtWhile {
			if stmt.Condition == nil {
				return fmt.Errorf("while statement missing condition")
			}
			if err := validateBlock(program, fn, stmt.Body); err != nil {
				return err
			}
		}
		if stmt.Kind == StmtDefer {
			if stmt.Expr == nil && len(stmt.Body.Stmts) == 0 && stmt.Body.Result == nil {
				return fmt.Errorf("defer statement missing expression or body")
			}
			if err := validateBlock(program, fn, stmt.Body); err != nil {
				return err
			}
		}
	}
	if block.Result != nil {
		if err := validateExpr(program, fn, *block.Result); err != nil {
			return err
		}
	}
	return nil
}

func spreadTypeContainsParam(program *Program, typeID TypeID, seen map[TypeID]bool) bool {
	if !validTypeID(program, typeID) || seen[typeID] {
		return false
	}
	seen[typeID] = true
	typ := program.Types[typeID-1]
	if typ.Kind == TypeParam {
		return true
	}
	for _, nested := range []TypeID{typ.Elem, typ.Key, typ.Value, typ.Error, typ.Return} {
		if spreadTypeContainsParam(program, nested, seen) {
			return true
		}
	}
	for _, nested := range typ.Params {
		if spreadTypeContainsParam(program, nested, seen) {
			return true
		}
	}
	for _, nested := range typ.GenericArgs {
		if spreadTypeContainsParam(program, nested, seen) {
			return true
		}
	}
	for _, field := range typ.Fields {
		if spreadTypeContainsParam(program, field.Type, seen) {
			return true
		}
	}
	for _, member := range typ.Members {
		if spreadTypeContainsParam(program, member.Type, seen) {
			return true
		}
	}
	return false
}

func descriptorReferenceSpreadElement(program *Program, typeID TypeID) bool {
	if !validTypeID(program, typeID) {
		return false
	}
	typ := program.Types[typeID-1]
	var referent TypeInfo
	switch {
	case typ.Kind == TypeReference && validTypeID(program, typ.Elem):
		referent = program.Types[typ.Elem-1]
	case typ.Kind == TypeForeignType && typ.ForeignPointer:
		referent = typ
	default:
		return false
	}
	return referent.Kind == TypeList || referent.Kind == TypeSlice || referent.Kind == TypeMap ||
		(referent.Kind == TypeForeignType && (referent.Elem != NoType || referent.Key != NoType))
}

func spreadCallableType(program *Program, typeID TypeID) (TypeInfo, error) {
	if !validTypeID(program, typeID) {
		return TypeInfo{}, fmt.Errorf("variadic spread has invalid callable type %d", typeID)
	}
	callable := program.Types[typeID-1]
	if callable.Kind == TypeForeignType && validTypeID(program, callable.Value) && callable.Key == NoType {
		callable = program.Types[callable.Value-1]
	}
	if callable.Kind != TypeFunction {
		return TypeInfo{}, fmt.Errorf("variadic spread callable has non-function type %d", typeID)
	}
	return callable, nil
}

func validateTailSpread(program *Program, expr Expr) error {
	spread := spreadExprPayload(&expr)
	if spread == nil {
		return nil
	}
	switch expr.Kind {
	case ExprCallClosure, ExprForeignCall, ExprForeignMethodCall:
	default:
		return fmt.Errorf("expression kind %d cannot carry variadic spread", expr.Kind)
	}
	if len(expr.Args) == 0 {
		return fmt.Errorf("variadic spread call has no arguments")
	}
	if !validTypeID(program, spread.Element) {
		return fmt.Errorf("variadic spread has invalid element type %d", spread.Element)
	}
	if spreadTypeContainsParam(program, spread.Element, map[TypeID]bool{}) {
		return fmt.Errorf("variadic spread element type %d is not concrete", spread.Element)
	}
	if descriptorReferenceSpreadElement(program, spread.Element) {
		return fmt.Errorf("variadic spread element type %d has ambiguous descriptor ABI", spread.Element)
	}
	callable, err := spreadCallableType(program, spread.Callable)
	if err != nil {
		return err
	}
	if !callable.Variadic || len(callable.Params) == 0 || len(expr.Args) != len(callable.Params) || callable.Params[len(callable.Params)-1] != spread.Element {
		return fmt.Errorf("variadic spread call does not match its callable type")
	}
	if expr.Kind == ExprCallClosure {
		if expr.Target == nil {
			return fmt.Errorf("closure spread call is missing its target")
		}
		targetCallable, err := spreadCallableType(program, expr.Target.Type)
		if err != nil {
			return err
		}
		if targetCallable.ID != callable.ID {
			return fmt.Errorf("closure spread callable type %d does not match target type %d", callable.ID, targetCallable.ID)
		}
	}

	last := expr.Args[len(expr.Args)-1]
	if !validTypeID(program, last.Type) {
		return fmt.Errorf("variadic spread argument has invalid type %d", last.Type)
	}
	lastType := program.Types[last.Type-1]
	var container TypeInfo
	switch {
	case lastType.Kind == TypeReference && validTypeID(program, lastType.Elem):
		container = program.Types[lastType.Elem-1]
	case lastType.Kind == TypeList || lastType.Kind == TypeSlice:
		container = lastType
	case lastType.Kind == TypeForeignType && lastType.Elem != NoType:
		container = lastType
	default:
		return fmt.Errorf("variadic spread argument type %d is not slice-shaped (kind=%d pointer=%t elem=%d)", last.Type, lastType.Kind, lastType.ForeignPointer, lastType.Elem)
	}
	if container.Kind != TypeList && container.Kind != TypeSlice && !(container.Kind == TypeForeignType && container.Elem != NoType) {
		return fmt.Errorf("variadic spread referent has non-slice type kind %d", container.Kind)
	}
	if container.Elem != spread.Element {
		return fmt.Errorf("variadic spread container element type %d does not match %d", container.Elem, spread.Element)
	}
	if foreign := exprPayloadAs[*ForeignExprPayload](&expr); foreign != nil && len(foreign.ArgABI) > 0 && foreign.ArgABI[len(foreign.ArgABI)-1] != ABIParamExact {
		return fmt.Errorf("variadic spread requires exact foreign element ABI")
	}
	return nil
}

func validateExprPayload(expr Expr) error {
	if expr.Kind > ExprTryMaybe {
		return fmt.Errorf("invalid expression kind %d", expr.Kind)
	}
	if expr.Payload != nil && exprPayloadIsTypedNil(expr.Payload) {
		return fmt.Errorf("expression kind %d has a typed-nil payload %T", expr.Kind, expr.Payload)
	}
	compatible := false
	switch expr.Payload.(type) {
	case nil:
		compatible = true
	case *TextExprPayload:
		compatible = expr.Kind == ExprConstInt || expr.Kind == ExprConstFloat || expr.Kind == ExprConstStr
	case *BoolExprPayload:
		compatible = expr.Kind == ExprConstBool
	case *EnumExprPayload:
		compatible = expr.Kind == ExprEnumVariant
	case *LocalExprPayload:
		compatible = expr.Kind == ExprLoadLocal
	case *GlobalExprPayload:
		compatible = expr.Kind == ExprLoadGlobal
	case *CallExprPayload:
		compatible = callExprPayloadAllowed(expr.Kind)
	case *ForeignExprPayload:
		compatible = expr.Kind == ExprForeignCall || expr.Kind == ExprForeignMethodCall || expr.Kind == ExprForeignMethodValue || expr.Kind == ExprForeignFieldAccess || expr.Kind == ExprForeignStructInstance || expr.Kind == ExprForeignValue
	case *InterfaceExprPayload:
		compatible = expr.Kind == ExprInterfaceConversion
	case *ReferenceExprPayload:
		compatible = expr.Kind == ExprMutRef || expr.Kind == ExprDeref
	case *FieldExprPayload:
		compatible = expr.Kind == ExprGetField
	case *TagExprPayload:
		compatible = expr.Kind == ExprUnionWrap
	case *TraitExprPayload:
		compatible = expr.Kind == ExprTraitRefProject || expr.Kind == ExprTraitUpcast || expr.Kind == ExprCallTrait
	case *AggregateExprPayload:
		compatible = expr.Kind == ExprMakeMap || expr.Kind == ExprMakeStruct
	case *BinaryExprPayload:
		compatible = binaryExprPayloadAllowed(expr.Kind)
	case *BlockExprPayload:
		compatible = expr.Kind == ExprBlock || expr.Kind == ExprUnsafeBlock
	case *IfExprPayload:
		compatible = expr.Kind == ExprIf
	case *EnumMatchExprPayload:
		compatible = expr.Kind == ExprMatchEnum
	case *IntMatchExprPayload:
		compatible = expr.Kind == ExprMatchInt
	case *StrMatchExprPayload:
		compatible = expr.Kind == ExprMatchStr
	case *UnionMatchExprPayload:
		compatible = expr.Kind == ExprMatchUnion
	case *ForeignMatchExprPayload:
		compatible = expr.Kind == ExprMatchForeignType
	case *MaybeMatchExprPayload:
		compatible = expr.Kind == ExprMatchMaybe
	case *ResultMatchExprPayload:
		compatible = expr.Kind == ExprMatchResult
	case *TryExprPayload:
		compatible = expr.Kind == ExprTryResult || expr.Kind == ExprTryMaybe
	case *SelectExprPayload:
		compatible = expr.Kind == ExprSelect
	case *MaybeCallExprPayload:
		compatible = maybeCallExprPayloadAllowed(expr.Kind)
	case *UnsafeCastExprPayload:
		compatible = expr.Kind == ExprUnsafeCast
	}
	if !compatible {
		return fmt.Errorf("expression kind %d has incompatible payload %T", expr.Kind, expr.Payload)
	}
	if exprPayloadRequired(expr.Kind) && expr.Payload == nil {
		return fmt.Errorf("expression kind %d is missing its payload", expr.Kind)
	}
	return nil
}

func binaryExprPayloadAllowed(kind ExprKind) bool {
	switch kind {
	case ExprIntAdd, ExprIntSub, ExprIntMul, ExprIntDiv, ExprIntMod,
		ExprFloatAdd, ExprFloatSub, ExprFloatMul, ExprFloatDiv, ExprStrConcat,
		ExprEq, ExprNotEq, ExprLt, ExprLte, ExprGt, ExprGte, ExprAnd, ExprOr:
		return true
	default:
		return false
	}
}

func callExprPayloadAllowed(kind ExprKind) bool {
	switch kind {
	case ExprFunctionRef, ExprCall, ExprMakeClosure, ExprCallClosure,
		ExprListAtChecked, ExprListSlice, ExprListIsEmpty, ExprListToList,
		ExprListPrepend, ExprListPush, ExprListSet, ExprListSize, ExprListSort, ExprListSwap,
		ExprStrAt, ExprStrSlice, ExprStrBytes, ExprStrRunes, ExprStrSize, ExprStrIsEmpty,
		ExprStrContains, ExprStrReplace, ExprStrReplaceAll, ExprStrStartsWith, ExprStrEndsWith,
		ExprToStr, ExprStrTrim:
		return true
	default:
		return false
	}
}

func maybeCallExprPayloadAllowed(kind ExprKind) bool {
	switch kind {
	case ExprMaybeExpect, ExprMaybeIsNone, ExprMaybeIsSome, ExprMaybeOr,
		ExprMaybeMap, ExprMaybeAndThen, ExprMaybeSet, ExprMaybeClear:
		return true
	default:
		return false
	}
}

func exprPayloadRequired(kind ExprKind) bool {
	if binaryExprPayloadAllowed(kind) || maybeCallExprPayloadAllowed(kind) {
		return true
	}
	switch kind {
	case ExprConstInt, ExprConstFloat, ExprConstBool, ExprConstStr,
		ExprLoadLocal, ExprLoadGlobal, ExprFunctionRef, ExprCall,
		ExprForeignCall, ExprForeignMethodCall, ExprForeignMethodValue,
		ExprForeignFieldAccess, ExprForeignStructInstance, ExprForeignValue,
		ExprInterfaceConversion, ExprUnsafeCast, ExprMutRef, ExprDeref,
		ExprTraitRefProject, ExprMatchForeignType, ExprMakeClosure, ExprCallClosure,
		ExprUnionWrap, ExprMatchUnion, ExprTraitUpcast, ExprCallTrait,
		ExprMakeMap, ExprSelect, ExprMakeStruct, ExprGetField,
		ExprBlock, ExprUnsafeBlock, ExprIf, ExprEnumVariant,
		ExprMatchEnum, ExprMatchInt, ExprMatchStr, ExprMatchMaybe, ExprMatchResult:
		return true
	default:
		return false
	}
}

func validateExpr(program *Program, fn Function, expr Expr) error {
	if err := validateExprPayload(expr); err != nil {
		return err
	}
	if !validTypeID(program, expr.Type) {
		return fmt.Errorf("expression has invalid type %d", expr.Type)
	}
	if err := validateTailSpread(program, expr); err != nil {
		return err
	}
	if expr.Kind == ExprLoadLocal {
		payload := exprPayloadAs[*LocalExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("local load is missing its payload")
		}
		if payload.Local < 0 || int(payload.Local) >= len(fn.Locals) {
			return fmt.Errorf("expression loads invalid local %d", payload.Local)
		}
		if expr.Type != fn.Locals[payload.Local].Type {
			return fmt.Errorf("local load type %d does not match local %s type %d", expr.Type, fn.Locals[payload.Local].Name, fn.Locals[payload.Local].Type)
		}
	}
	if expr.Kind == ExprMutRef {
		payload := exprPayloadAs[*ReferenceExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("mutable reference expression is missing its payload")
		}
		if payload.Mode < ExistingReference || payload.Mode > FreshValue {
			return fmt.Errorf("mutable reference expression has invalid mode %d", payload.Mode)
		}
		if expr.Target == nil {
			return fmt.Errorf("mutable reference expression has no target")
		}
	}
	if (expr.Kind == ExprDeref || expr.Kind == ExprTraitRefProject) && expr.Target == nil {
		return fmt.Errorf("reference expression kind %d has no target", expr.Kind)
	}
	if expr.Kind == ExprDeref && expr.Target != nil {
		targetType, err := typeInfo(program, expr.Target.Type)
		if err != nil {
			return err
		}
		if targetType.Kind == TypeReference {
			if expr.Type != targetType.Elem {
				return fmt.Errorf("dereference type %d does not match referent type %d", expr.Type, targetType.Elem)
			}
		} else if targetType.Kind != TypeForeignType || !targetType.ForeignPointer {
			return fmt.Errorf("dereference target has non-reference type kind %d", targetType.Kind)
		}
	}
	if expr.Kind == ExprTraitRefProject && expr.Target != nil {
		payload := exprPayloadAs[*TraitExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("trait reference projection is missing its payload")
		}
		destination, err := typeInfo(program, expr.Type)
		if err != nil {
			return err
		}
		source, err := typeInfo(program, expr.Target.Type)
		if err != nil {
			return err
		}
		if destination.Kind != TypeReference || source.Kind != TypeReference {
			return fmt.Errorf("trait reference projection requires reference source and destination")
		}
		traitType, err := typeInfo(program, destination.Elem)
		if err != nil {
			return err
		}
		if traitType.Kind != TypeTraitObject || traitType.Trait != payload.Trait {
			return fmt.Errorf("trait reference projection destination does not match trait %d", payload.Trait)
		}
		if !validImplID(program, payload.Impl) {
			return fmt.Errorf("trait reference projection has invalid impl id %d", payload.Impl)
		}
		impl := program.Impls[payload.Impl]
		if impl.Trait != payload.Trait || !implMatchesType(program, impl.ForType, source.Elem) {
			return fmt.Errorf("trait reference projection impl %d does not match source referent", payload.Impl)
		}
	}
	isForeignCall := expr.Kind == ExprForeignCall || expr.Kind == ExprForeignMethodCall || expr.Kind == ExprForeignMethodValue || expr.Kind == ExprForeignValue
	if isForeignCall {
		payload := exprPayloadAs[*ForeignExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("foreign expression is missing its payload")
		}
		argCount := len(expr.Args)
		argTypes := make([]TypeID, argCount)
		for i := range expr.Args {
			argTypes[i] = expr.Args[i].Type
		}
		if (expr.Kind == ExprForeignMethodValue || expr.Kind == ExprForeignValue) && validTypeID(program, expr.Type) {
			if functionType := program.Types[expr.Type-1]; functionType.Kind == TypeFunction {
				argCount = len(functionType.Params)
				argTypes = functionType.Params
			}
		}
		if len(payload.ArgABI) != argCount {
			return fmt.Errorf("foreign expression has %d ABI parameter modes for %d args", len(payload.ArgABI), argCount)
		}
		for i, mode := range payload.ArgABI {
			if mode > ABIParamDescriptorValue {
				return fmt.Errorf("foreign expression has invalid arg mode %d", mode)
			}
			if err := validateABIParamMode(program, argTypes[i], mode); err != nil {
				return fmt.Errorf("foreign expression arg %d: %w", i, err)
			}
		}
	}
	if expr.Kind == ExprLoadGlobal {
		payload := exprPayloadAs[*GlobalExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("global load is missing its payload")
		}
		if !validGlobalID(program, payload.Global) {
			return fmt.Errorf("expression loads invalid global %d", payload.Global)
		}
		if expr.Type != program.Globals[payload.Global].Type {
			return fmt.Errorf("global load type %d does not match global %s type %d", expr.Type, program.Globals[payload.Global].Name, program.Globals[payload.Global].Type)
		}
	}
	if expr.Kind == ExprMakeMaybeSome || expr.Kind == ExprMakeMaybeNone {
		maybeType, err := typeInfo(program, expr.Type)
		if err != nil {
			return err
		}
		if maybeType.Kind != TypeMaybe {
			return fmt.Errorf("Maybe constructor has type kind %d", maybeType.Kind)
		}
		if expr.Kind == ExprMakeMaybeSome {
			if expr.Target == nil {
				return fmt.Errorf("Maybe constructor missing value")
			}
			if expr.Target.Type != maybeType.Elem {
				return fmt.Errorf("Maybe constructor value type %d does not match element type %d", expr.Target.Type, maybeType.Elem)
			}
		}
	}
	if expr.Kind == ExprMakeResultOk || expr.Kind == ExprMakeResultErr {
		resultType, err := typeInfo(program, expr.Type)
		if err != nil {
			return err
		}
		if resultType.Kind != TypeResult {
			return fmt.Errorf("Result constructor has type kind %d", resultType.Kind)
		}
		if expr.Target == nil {
			return fmt.Errorf("Result constructor missing value")
		}
		expected := resultType.Value
		if expr.Kind == ExprMakeResultErr {
			expected = resultType.Error
		}
		if expr.Target.Type != expected {
			return fmt.Errorf("Result constructor value type %d does not match variant type %d", expr.Target.Type, expected)
		}
	}
	if expr.Kind == ExprFunctionRef || expr.Kind == ExprCall || expr.Kind == ExprMakeClosure {
		payload := exprPayloadAs[*CallExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("function expression kind %d is missing its payload", expr.Kind)
		}
		if !validFunctionID(program, payload.Function) {
			return fmt.Errorf("function expression kind %d has invalid function %d", expr.Kind, payload.Function)
		}
	}
	if expr.Kind == ExprCallClosure {
		if expr.Target == nil {
			return fmt.Errorf("closure call missing target")
		}
		callable, err := typeInfo(program, expr.Target.Type)
		if err != nil {
			return err
		}
		if callable.Kind == TypeForeignType && validTypeID(program, callable.Value) && callable.Key == NoType {
			callable = program.Types[callable.Value-1]
		}
		if callable.Kind != TypeFunction {
			return fmt.Errorf("closure call target has non-function type %d", expr.Target.Type)
		}
		minimum := len(callable.Params)
		if callable.Variadic {
			minimum--
		}
		if len(expr.Args) < minimum || (!callable.Variadic && len(expr.Args) != len(callable.Params)) {
			return fmt.Errorf("closure call has %d arguments for function with %d parameters (variadic=%t)", len(expr.Args), len(callable.Params), callable.Variadic)
		}
	}
	if expr.Kind == ExprMakeClosure {
		payload := exprPayloadAs[*CallExprPayload](&expr)
		closureFn := program.Functions[payload.Function]
		if len(payload.CaptureLocals) != len(closureFn.Captures) {
			return fmt.Errorf("closure %s expects %d captures, got %d", closureFn.Name, len(closureFn.Captures), len(payload.CaptureLocals))
		}
		for i, local := range payload.CaptureLocals {
			if local < 0 || int(local) >= len(fn.Locals) {
				return fmt.Errorf("expression captures invalid local %d", local)
			}
			if fn.Locals[local].Type != closureFn.Captures[i].Type {
				return fmt.Errorf("closure %s capture %s type %d does not match source local type %d", closureFn.Name, closureFn.Captures[i].Name, closureFn.Captures[i].Type, fn.Locals[local].Type)
			}
		}
	}
	if expr.Kind == ExprUnionWrap {
		if expr.Target == nil {
			return fmt.Errorf("union wrap missing target")
		}
		unionType, err := typeInfo(program, expr.Type)
		if err != nil {
			return err
		}
		if unionType.Kind != TypeUnion {
			return fmt.Errorf("union wrap target type has kind %d", unionType.Kind)
		}
		payload := exprPayloadAs[*TagExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("union wrap is missing its payload")
		}
		member, ok := unionMemberByTag(unionType, payload.Tag)
		if !ok {
			return fmt.Errorf("union wrap has invalid tag %d for %s", payload.Tag, unionType.Name)
		}
		if member.Type != expr.Target.Type {
			return fmt.Errorf("union wrap member %s expects type %d, got %d", member.Name, member.Type, expr.Target.Type)
		}
	}
	if expr.Kind == ExprToStr && expr.Target == nil {
		return fmt.Errorf("to_str expression missing target")
	}
	if expr.Kind == ExprToAny && expr.Target == nil {
		return fmt.Errorf("to_dyn expression missing target")
	}
	if expr.Kind == ExprUnsafeCast {
		if expr.Target == nil {
			return fmt.Errorf("unsafe::cast expression missing target")
		}
		payload := exprPayloadAs[*UnsafeCastExprPayload](&expr)
		if payload == nil || !validTypeID(program, payload.TargetType) {
			return fmt.Errorf("unsafe::cast expression has invalid target payload")
		}
	}
	if expr.Kind == ExprUnsafeIsNil && expr.Target == nil {
		return fmt.Errorf("unsafe::is_nil expression missing target")
	}
	if expr.Kind == ExprPanic && expr.Target == nil {
		return fmt.Errorf("panic expression missing target")
	}
	if expr.Kind == ExprTraitUpcast {
		if expr.Target == nil {
			return fmt.Errorf("trait upcast missing target")
		}
		payload := exprPayloadAs[*TraitExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("trait upcast is missing its payload")
		}
		traitType, err := typeInfo(program, expr.Type)
		if err != nil {
			return err
		}
		if traitType.Kind != TypeTraitObject {
			return fmt.Errorf("trait upcast target type has kind %d", traitType.Kind)
		}
		if traitType.Trait != payload.Trait {
			return fmt.Errorf("trait upcast expression trait %d does not match type trait %d", payload.Trait, traitType.Trait)
		}
		targetImplType := expr.Target.Type
		erasedMutableTrait := false
		if targetType, err := typeInfo(program, expr.Target.Type); err == nil && targetType.Kind == TypeReference {
			targetImplType = targetType.Elem
			erasedMutableTrait = targetImplType == expr.Type && payload.Impl < 0
		}
		if !erasedMutableTrait {
			if !validImplID(program, payload.Impl) {
				return fmt.Errorf("trait upcast has invalid impl id %d", payload.Impl)
			}
			impl := program.Impls[payload.Impl]
			if impl.Trait != payload.Trait {
				return fmt.Errorf("trait upcast impl %d has trait %d, want %d", payload.Impl, impl.Trait, payload.Trait)
			}
			if !implMatchesType(program, impl.ForType, targetImplType) {
				return fmt.Errorf("trait upcast impl %d is for type %d, got target type %d", payload.Impl, impl.ForType, targetImplType)
			}
		}
	}
	if payload := exprPayloadAs[*CallExprPayload](&expr); payload != nil {
		for _, local := range payload.CaptureLocals {
			if local < 0 || int(local) >= len(fn.Locals) {
				return fmt.Errorf("expression captures invalid local %d", local)
			}
		}
	}
	if expr.Target != nil {
		if err := validateExpr(program, fn, *expr.Target); err != nil {
			return err
		}
	}
	if payload := exprPayloadAs[*BinaryExprPayload](&expr); payload != nil {
		if payload.Left == nil || payload.Right == nil {
			return fmt.Errorf("binary expression kind %d is missing an operand", expr.Kind)
		}
		if err := validateExpr(program, fn, *payload.Left); err != nil {
			return err
		}
		if err := validateExpr(program, fn, *payload.Right); err != nil {
			return err
		}
	}
	if expr.Kind == ExprBlock {
		payload := exprPayloadAs[*BlockExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("block expression is missing its payload")
		}
		if err := validateBlock(program, fn, payload.Body); err != nil {
			return err
		}
	}
	if expr.Kind == ExprUnsafeBlock {
		typeInfo, err := typeInfo(program, expr.Type)
		if err != nil {
			return err
		}
		if typeInfo.Kind != TypeResult {
			return fmt.Errorf("unsafe block has type kind %d", typeInfo.Kind)
		}
		payload := exprPayloadAs[*BlockExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("unsafe block expression is missing its payload")
		}
		helperFn := fn
		helperFn.Signature.Return = expr.Type
		if err := validateBlock(program, helperFn, payload.Body); err != nil {
			return err
		}
	}
	if expr.Kind == ExprIf {
		payload := exprPayloadAs[*IfExprPayload](&expr)
		if payload == nil || payload.Condition == nil {
			return fmt.Errorf("if expression is missing its payload")
		}
		if err := validateExpr(program, fn, *payload.Condition); err != nil {
			return err
		}
		if err := validateBlock(program, fn, payload.Then); err != nil {
			return err
		}
		if err := validateBlock(program, fn, payload.Else); err != nil {
			return err
		}
	}
	if expr.Kind == ExprMatchEnum {
		payload := exprPayloadAs[*EnumMatchExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("enum match is missing its payload")
		}
		for _, matchCase := range payload.Cases {
			if err := validateBlock(program, fn, matchCase.Body); err != nil {
				return err
			}
		}
		if err := validateBlock(program, fn, payload.CatchAll); err != nil {
			return err
		}
	}
	if expr.Kind == ExprMatchStr {
		if expr.Target == nil {
			return fmt.Errorf("str match missing target")
		}
		targetType, err := typeInfo(program, expr.Target.Type)
		if err != nil {
			return err
		}
		if targetType.Kind != TypeStr {
			return fmt.Errorf("str match target has type kind %d", targetType.Kind)
		}
		payload := exprPayloadAs[*StrMatchExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("str match is missing its payload")
		}
		for _, matchCase := range payload.Cases {
			if err := validateBlock(program, fn, matchCase.Body); err != nil {
				return err
			}
		}
		if err := validateBlock(program, fn, payload.CatchAll); err != nil {
			return err
		}
	}
	if expr.Kind == ExprMatchInt {
		if expr.Target == nil {
			return fmt.Errorf("int match missing target")
		}
		targetType, err := typeInfo(program, expr.Target.Type)
		if err != nil {
			return err
		}
		if targetType.Kind != TypeInt && targetType.Kind != TypeByte && targetType.Kind != TypeRune {
			return fmt.Errorf("int match target has type kind %d", targetType.Kind)
		}
		payload := exprPayloadAs[*IntMatchExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("int match is missing its payload")
		}
		for _, matchCase := range payload.Cases {
			if err := validateBlock(program, fn, matchCase.Body); err != nil {
				return err
			}
		}
		for _, matchCase := range payload.RangeCases {
			if matchCase.Start > matchCase.End {
				return fmt.Errorf("int match range start %d is greater than end %d", matchCase.Start, matchCase.End)
			}
			if err := validateBlock(program, fn, matchCase.Body); err != nil {
				return err
			}
		}
		if err := validateBlock(program, fn, payload.CatchAll); err != nil {
			return err
		}
	}
	if expr.Kind == ExprMatchUnion {
		if expr.Target == nil {
			return fmt.Errorf("union match missing target")
		}
		unionType, err := typeInfo(program, expr.Target.Type)
		if err != nil {
			return err
		}
		if unionType.Kind != TypeUnion {
			return fmt.Errorf("union match target has type kind %d", unionType.Kind)
		}
		payload := exprPayloadAs[*UnionMatchExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("union match is missing its payload")
		}
		for _, matchCase := range payload.Cases {
			member, ok := unionMemberByTag(unionType, matchCase.Tag)
			if !ok {
				return fmt.Errorf("union match has invalid tag %d for %s", matchCase.Tag, unionType.Name)
			}
			if matchCase.Local < 0 || int(matchCase.Local) >= len(fn.Locals) {
				return fmt.Errorf("union match binds invalid local %d", matchCase.Local)
			}
			if fn.Locals[matchCase.Local].Type != member.Type {
				return fmt.Errorf("union match member %s local type %d does not match member type %d", member.Name, fn.Locals[matchCase.Local].Type, member.Type)
			}
			if err := validateBlock(program, fn, matchCase.Body); err != nil {
				return err
			}
		}
		if err := validateBlock(program, fn, payload.CatchAll); err != nil {
			return err
		}
	}
	if expr.Kind == ExprMatchForeignType {
		if expr.Target == nil {
			return fmt.Errorf("foreign type match missing target")
		}
		payload := exprPayloadAs[*ForeignMatchExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("foreign type match is missing its payload")
		}
		for _, matchCase := range payload.Cases {
			if !validTypeID(program, matchCase.Type) {
				return fmt.Errorf("foreign type match has invalid case type %d", matchCase.Type)
			}
			if matchCase.Bound {
				if matchCase.Local < 0 || int(matchCase.Local) >= len(fn.Locals) {
					return fmt.Errorf("foreign type match binds invalid local %d", matchCase.Local)
				}
				if fn.Locals[matchCase.Local].Type != matchCase.Type {
					return fmt.Errorf("foreign type match local type %d does not match case type %d", fn.Locals[matchCase.Local].Type, matchCase.Type)
				}
			}
			if err := validateBlock(program, fn, matchCase.Body); err != nil {
				return err
			}
		}
		if err := validateBlock(program, fn, payload.CatchAll); err != nil {
			return err
		}
	}
	if expr.Kind == ExprCallTrait {
		if expr.Target == nil {
			return fmt.Errorf("trait call missing target")
		}
		payload := exprPayloadAs[*TraitExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("trait call is missing its payload")
		}
		targetType, err := referentTypeInfo(program, expr.Target.Type)
		if err != nil {
			return err
		}
		if targetType.Kind != TypeTraitObject {
			return fmt.Errorf("trait call target has type kind %d", targetType.Kind)
		}
		if targetType.Trait != payload.Trait {
			return fmt.Errorf("trait call expression trait %d does not match target type trait %d", payload.Trait, targetType.Trait)
		}
		if !validTraitID(program, payload.Trait) {
			return fmt.Errorf("trait call has invalid trait id %d", payload.Trait)
		}
		trait := program.Traits[payload.Trait]
		if payload.Method < 0 || payload.Method >= len(trait.Methods) {
			return fmt.Errorf("trait call has invalid method index %d for trait %s", payload.Method, trait.Name)
		}
		method := trait.Methods[payload.Method]
		if method.Mutates {
			target, err := typeInfo(program, expr.Target.Type)
			if err != nil {
				return err
			}
			if target.Kind != TypeReference {
				return fmt.Errorf("mutating trait method %s requires a reference target", method.Name)
			}
		}
		if len(expr.Args) != len(method.Signature.Params) {
			return fmt.Errorf("trait call method %s expects %d args, got %d", method.Name, len(method.Signature.Params), len(expr.Args))
		}
	}
	if expr.Kind == ExprMatchMaybe {
		if expr.Target == nil {
			return fmt.Errorf("Maybe match missing target")
		}
		maybeType, err := typeInfo(program, expr.Target.Type)
		if err != nil {
			return err
		}
		if maybeType.Kind != TypeMaybe {
			return fmt.Errorf("Maybe match target has type kind %d", maybeType.Kind)
		}
		payload := exprPayloadAs[*MaybeMatchExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("Maybe match is missing its payload")
		}
		if payload.SomeLocal < 0 || int(payload.SomeLocal) >= len(fn.Locals) {
			return fmt.Errorf("Maybe match binds invalid local %d", payload.SomeLocal)
		}
		if fn.Locals[payload.SomeLocal].Type != maybeType.Elem {
			return fmt.Errorf("Maybe match local type %d does not match element type %d", fn.Locals[payload.SomeLocal].Type, maybeType.Elem)
		}
		if err := validateBlock(program, fn, payload.Some); err != nil {
			return err
		}
		if err := validateBlock(program, fn, payload.None); err != nil {
			return err
		}
	}
	if expr.Kind == ExprSelect {
		payload := exprPayloadAs[*SelectExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("select expression is missing its payload")
		}
		hasDefault := false
		for _, arm := range payload.Cases {
			switch arm.Kind {
			case SelectArmDefault:
				if hasDefault {
					return fmt.Errorf("select expression has multiple default arms")
				}
				hasDefault = true
				if arm.Channel != nil || arm.Value != nil || arm.HasBind {
					return fmt.Errorf("select default arm carries channel, value, or binding metadata")
				}
			case SelectArmRecv:
				if arm.Channel == nil || arm.Value != nil {
					return fmt.Errorf("select receive arm must carry only a channel operand")
				}
				if err := validateExpr(program, fn, *arm.Channel); err != nil {
					return err
				}
				channelType, err := selectChannelType(program, arm.Channel.Type)
				if err != nil {
					return err
				}
				if channelType.Kind != TypeChannel && channelType.Kind != TypeReceiver {
					return fmt.Errorf("select receive arm has non-receivable channel kind %d", channelType.Kind)
				}
				if arm.HasBind {
					if arm.BindLocal < 0 || int(arm.BindLocal) >= len(fn.Locals) {
						return fmt.Errorf("select receive arm binds invalid local %d", arm.BindLocal)
					}
					bindingType, err := typeInfo(program, fn.Locals[arm.BindLocal].Type)
					if err != nil {
						return err
					}
					if bindingType.Kind != TypeMaybe || bindingType.Elem != channelType.Elem {
						return fmt.Errorf("select receive binding type %d does not match channel element type %d", fn.Locals[arm.BindLocal].Type, channelType.Elem)
					}
				}
			case SelectArmSend:
				if arm.Channel == nil || arm.Value == nil || arm.HasBind {
					return fmt.Errorf("select send arm requires channel and value operands without a binding")
				}
				if err := validateExpr(program, fn, *arm.Channel); err != nil {
					return err
				}
				if err := validateExpr(program, fn, *arm.Value); err != nil {
					return err
				}
				channelType, err := selectChannelType(program, arm.Channel.Type)
				if err != nil {
					return err
				}
				if channelType.Kind != TypeChannel && channelType.Kind != TypeSender {
					return fmt.Errorf("select send arm has non-sendable channel kind %d", channelType.Kind)
				}
				if !typesAssignable(program, channelType.Elem, arm.Value.Type) {
					return fmt.Errorf("select send value type %d does not match channel element type %d", arm.Value.Type, channelType.Elem)
				}
			default:
				return fmt.Errorf("select expression has invalid arm kind %d", arm.Kind)
			}
			if err := validateBlock(program, fn, arm.Body); err != nil {
				return err
			}
		}
	}
	if expr.Kind == ExprMatchResult {
		if expr.Target == nil {
			return fmt.Errorf("Result match missing target")
		}
		resultType, err := typeInfo(program, expr.Target.Type)
		if err != nil {
			return err
		}
		if resultType.Kind != TypeResult {
			return fmt.Errorf("Result match target has type kind %d", resultType.Kind)
		}
		payload := exprPayloadAs[*ResultMatchExprPayload](&expr)
		if payload == nil {
			return fmt.Errorf("Result match is missing its payload")
		}
		if payload.OkLocal < 0 || int(payload.OkLocal) >= len(fn.Locals) {
			return fmt.Errorf("Result match binds invalid ok local %d", payload.OkLocal)
		}
		if payload.ErrLocal < 0 || int(payload.ErrLocal) >= len(fn.Locals) {
			return fmt.Errorf("Result match binds invalid err local %d", payload.ErrLocal)
		}
		if fn.Locals[payload.OkLocal].Type != resultType.Value {
			return fmt.Errorf("Result ok match local type %d does not match value type %d", fn.Locals[payload.OkLocal].Type, resultType.Value)
		}
		if fn.Locals[payload.ErrLocal].Type != resultType.Error {
			return fmt.Errorf("Result err match local type %d does not match error type %d", fn.Locals[payload.ErrLocal].Type, resultType.Error)
		}
		if err := validateBlock(program, fn, payload.Ok); err != nil {
			return err
		}
		if err := validateBlock(program, fn, payload.Err); err != nil {
			return err
		}
	}
	if expr.Kind == ExprTryResult || expr.Kind == ExprTryMaybe {
		catchPayload := exprPayloadAs[*TryExprPayload](&expr)
		if expr.Target == nil {
			return fmt.Errorf("try expression missing target")
		}
		targetType, err := typeInfo(program, expr.Target.Type)
		if err != nil {
			return err
		}
		if expr.Kind == ExprTryResult && targetType.Kind != TypeResult {
			return fmt.Errorf("Result try target has type kind %d", targetType.Kind)
		}
		if expr.Kind == ExprTryMaybe && targetType.Kind != TypeMaybe {
			return fmt.Errorf("Maybe try target has type kind %d", targetType.Kind)
		}
		resultType := targetType.Value
		if expr.Kind == ExprTryMaybe {
			resultType = targetType.Elem
		}
		if !typesAssignable(program, resultType, expr.Type) {
			return fmt.Errorf("try result type %d does not match target value type %d", expr.Type, resultType)
		}
		if catchPayload == nil {
			returnType, err := typeInfo(program, fn.Signature.Return)
			if err != nil {
				return err
			}
			if expr.Kind == ExprTryResult && returnType.Kind != TypeResult {
				return fmt.Errorf("Result try without catch in non-Result function %s", fn.Name)
			}
			if expr.Kind == ExprTryMaybe && returnType.Kind != TypeMaybe {
				return fmt.Errorf("Maybe try without catch in non-Maybe function %s", fn.Name)
			}
		}
		if catchPayload != nil {
			if expr.Kind == ExprTryResult && (catchPayload.CatchLocal < 0 || int(catchPayload.CatchLocal) >= len(fn.Locals)) {
				return fmt.Errorf("Result try catch binds invalid local %d", catchPayload.CatchLocal)
			}
			if expr.Kind == ExprTryResult && fn.Locals[catchPayload.CatchLocal].Type != targetType.Error {
				return fmt.Errorf("Result try catch local type %d does not match error type %d", fn.Locals[catchPayload.CatchLocal].Type, targetType.Error)
			}
			if err := validateBlock(program, fn, catchPayload.Catch); err != nil {
				return err
			}
		}
	}
	for _, arg := range expr.Args {
		if err := validateExpr(program, fn, arg); err != nil {
			return err
		}
	}
	if payload := exprPayloadAs[*AggregateExprPayload](&expr); payload != nil {
		for _, entry := range payload.Entries {
			if err := validateExpr(program, fn, entry.Key); err != nil {
				return err
			}
			if err := validateExpr(program, fn, entry.Value); err != nil {
				return err
			}
		}
		for _, field := range payload.Fields {
			if err := validateExpr(program, fn, field.Value); err != nil {
				return err
			}
		}
	}
	if payload := exprPayloadAs[*ForeignExprPayload](&expr); payload != nil {
		for _, field := range payload.Fields {
			if err := validateExpr(program, fn, field.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

func selectChannelType(program *Program, typeID TypeID) (TypeInfo, error) {
	channelType, err := typeInfo(program, typeID)
	if err != nil {
		return TypeInfo{}, err
	}
	if channelType.Kind == TypeReference {
		return typeInfo(program, channelType.Elem)
	}
	return channelType, nil
}

func validTypeID(program *Program, id TypeID) bool {
	return id > 0 && int(id) <= len(program.Types)
}

func validFunctionID(program *Program, id FunctionID) bool {
	return id >= 0 && int(id) < len(program.Functions)
}

func validGlobalID(program *Program, id GlobalID) bool {
	return id >= 0 && int(id) < len(program.Globals)
}

func validTraitID(program *Program, id TraitID) bool {
	return id >= 0 && int(id) < len(program.Traits)
}

func validImplID(program *Program, id ImplID) bool {
	return id >= 0 && int(id) < len(program.Impls)
}

func typesAssignable(program *Program, destination TypeID, source TypeID) bool {
	if destination == source {
		return true
	}
	if !validTypeID(program, destination) || !validTypeID(program, source) {
		return false
	}
	destinationInfo := program.Types[destination-1]
	sourceInfo := program.Types[source-1]
	if destinationInfo.Kind == TypeTraitObject && sourceInfo.Kind == TypeReference && sourceInfo.Elem == destination {
		return true
	}
	return foreignTypeAssignableTo(program, destinationInfo, source, sourceInfo) ||
		foreignTypeAssignableTo(program, sourceInfo, destination, destinationInfo) ||
		typesStructurallyEquivalent(program, destination, source, map[[2]TypeID]bool{})
}

func foreignTypeAssignableTo(program *Program, foreign TypeInfo, otherID TypeID, other TypeInfo) bool {
	if foreign.Kind != TypeForeignType || foreign.ForeignPointer {
		return false
	}
	if foreign.Key == NoType && validTypeID(program, foreign.Value) && foreign.Value == otherID {
		underlying := program.Types[foreign.Value-1]
		switch underlying.Kind {
		case TypeVoid, TypeInt, TypeScalar, TypeFloat64, TypeBool, TypeByte, TypeRune, TypeStr:
			// Named Go scalars require an explicit AIR conversion. Treating their
			// underlying primitive as directly assignable would let malformed AIR
			// pass validation and produce invalid Go assignments.
			return false
		default:
			return true
		}
	}
	if validTypeID(program, foreign.Key) && validTypeID(program, foreign.Value) && other.Kind == TypeMap {
		return foreign.Key == other.Key && foreign.Value == other.Value
	}
	if validTypeID(program, foreign.Elem) && (other.Kind == TypeList || other.Kind == TypeSlice) {
		return foreign.Elem == other.Elem
	}
	return false
}

func typesStructurallyEquivalent(program *Program, leftID TypeID, rightID TypeID, seen map[[2]TypeID]bool) bool {
	if leftID == rightID {
		return true
	}
	if !validTypeID(program, leftID) || !validTypeID(program, rightID) {
		return false
	}
	pair := [2]TypeID{leftID, rightID}
	if seen[pair] {
		return true
	}
	seen[pair] = true
	left := program.Types[leftID-1]
	right := program.Types[rightID-1]
	if left.Kind != right.Kind {
		return false
	}
	equivalent := func(a, b TypeID) bool {
		return typesStructurallyEquivalent(program, a, b, seen)
	}
	switch left.Kind {
	case TypeVoid, TypeInt, TypeFloat64, TypeBool, TypeByte, TypeRune, TypeStr, TypeAny:
		return true
	case TypeScalar:
		return left.Name == right.Name
	case TypeParam:
		return left.ParamOwner == right.ParamOwner && left.ParamIndex == right.ParamIndex
	case TypeList, TypeSlice, TypeMaybe, TypeChannel, TypeReceiver, TypeSender, TypeReference:
		return equivalent(left.Elem, right.Elem)
	case TypeFixedArray:
		return left.Length == right.Length && equivalent(left.Elem, right.Elem)
	case TypeMap:
		return equivalent(left.Key, right.Key) && equivalent(left.Value, right.Value)
	case TypeResult:
		return equivalent(left.Value, right.Value) && equivalent(left.Error, right.Error)
	case TypeFunction:
		if left.Variadic != right.Variadic || len(left.Params) != len(right.Params) {
			return false
		}
		for i := range left.Params {
			if !equivalent(left.Params[i], right.Params[i]) {
				return false
			}
		}
		return equivalent(left.Return, right.Return)
	case TypeStruct:
		if left.Generic == NoType || left.Generic != right.Generic || len(left.GenericArgs) != len(right.GenericArgs) {
			return false
		}
		for i := range left.GenericArgs {
			if !equivalent(left.GenericArgs[i], right.GenericArgs[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func implMatchesType(program *Program, implType, actualType TypeID) bool {
	if implType == actualType {
		return true
	}
	actual, err := typeInfo(program, actualType)
	if err != nil || actual.Generic == NoType {
		return false
	}
	if implType == actual.Generic {
		return true
	}
	impl, err := typeInfo(program, implType)
	return err == nil && impl.Generic == actual.Generic
}

func typeInfo(program *Program, id TypeID) (TypeInfo, error) {
	if !validTypeID(program, id) {
		return TypeInfo{}, fmt.Errorf("invalid type id %d", id)
	}
	return program.Types[id-1], nil
}

func referentTypeInfo(program *Program, id TypeID) (TypeInfo, error) {
	info, err := typeInfo(program, id)
	if err != nil {
		return TypeInfo{}, err
	}
	if info.Kind == TypeReference {
		return typeInfo(program, info.Elem)
	}
	return info, nil
}

func unionMemberByTag(unionType TypeInfo, tag uint32) (UnionMember, bool) {
	for _, member := range unionType.Members {
		if member.Tag == tag {
			return member, true
		}
	}
	return UnionMember{}, false
}
