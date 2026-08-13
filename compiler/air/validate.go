package air

import "fmt"

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
		for i, field := range typ.Fields {
			if field.Index != i {
				return fmt.Errorf("type %s field %s has index %d, want %d", typ.Name, field.Name, field.Index, i)
			}
			if !validTypeID(program, field.Type) {
				return fmt.Errorf("type %s field %s has invalid type %d", typ.Name, field.Name, field.Type)
			}
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
		receiverMatches := receiverType == impl.ForType
		if !receiverMatches {
			if receiver, err := typeInfo(program, receiverType); err == nil {
				receiverMatches = receiver.Kind == TypeReference && receiver.Elem == impl.ForType
			}
		}
		if !receiverMatches {
			return fmt.Errorf("impl %d method %s receiver type %d does not match impl type %d", impl.ID, method.Name, receiverType, impl.ForType)
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
	if global.Value.Type == NoType {
		return fmt.Errorf("global %s has no initializer", global.Name)
	}
	if global.Value.Type != global.Type {
		return fmt.Errorf("global %s initializer type %d does not match global type %d", global.Name, global.Value.Type, global.Type)
	}
	if err := validateExpr(program, Function{Module: global.Module, Name: "<global>"}, global.Value); err != nil {
		return fmt.Errorf("global %s: %w", global.Name, err)
	}
	return nil
}

func validateFunction(program *Program, fn Function) error {
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

func validateExpr(program *Program, fn Function, expr Expr) error {
	if !validTypeID(program, expr.Type) {
		return fmt.Errorf("expression has invalid type %d", expr.Type)
	}
	if expr.Kind == ExprLoadLocal {
		if expr.Local < 0 || int(expr.Local) >= len(fn.Locals) {
			return fmt.Errorf("expression loads invalid local %d", expr.Local)
		}
		if expr.Type != fn.Locals[expr.Local].Type {
			return fmt.Errorf("local load type %d does not match local %s type %d", expr.Type, fn.Locals[expr.Local].Name, fn.Locals[expr.Local].Type)
		}
	}
	if expr.Kind == ExprMutRef {
		if expr.ReferenceMode < ExistingReference || expr.ReferenceMode > FreshValue {
			return fmt.Errorf("mutable reference expression has invalid mode %d", expr.ReferenceMode)
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
		if traitType.Kind != TypeTraitObject || traitType.Trait != expr.Trait {
			return fmt.Errorf("trait reference projection destination does not match trait %d", expr.Trait)
		}
		if !validImplID(program, expr.Impl) {
			return fmt.Errorf("trait reference projection has invalid impl id %d", expr.Impl)
		}
		impl := program.Impls[expr.Impl]
		if impl.Trait != expr.Trait || impl.ForType != source.Elem {
			return fmt.Errorf("trait reference projection impl %d does not match source referent", expr.Impl)
		}
	}
	isForeignCall := expr.Kind == ExprForeignCall || expr.Kind == ExprForeignMethodCall || expr.Kind == ExprForeignMethodValue || expr.Kind == ExprForeignValue
	if isForeignCall {
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
		if len(expr.ForeignArgABI) != argCount {
			return fmt.Errorf("foreign expression has %d ABI parameter modes for %d args", len(expr.ForeignArgABI), argCount)
		}
		for i, mode := range expr.ForeignArgABI {
			if mode > ABIParamDescriptorValue {
				return fmt.Errorf("foreign expression has invalid arg mode %d", mode)
			}
			if err := validateABIParamMode(program, argTypes[i], mode); err != nil {
				return fmt.Errorf("foreign expression arg %d: %w", i, err)
			}
		}
	} else if len(expr.ForeignArgABI) > 0 {
		return fmt.Errorf("non-foreign expression has ABI parameter modes")
	}
	if expr.Kind == ExprLoadGlobal {
		if !validGlobalID(program, expr.Global) {
			return fmt.Errorf("expression loads invalid global %d", expr.Global)
		}
		if expr.Type != program.Globals[expr.Global].Type {
			return fmt.Errorf("global load type %d does not match global %s type %d", expr.Type, program.Globals[expr.Global].Name, program.Globals[expr.Global].Type)
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
	if expr.Kind == ExprFunctionRef && !validFunctionID(program, expr.Function) {
		return fmt.Errorf("expression references invalid function %d", expr.Function)
	}
	if expr.Kind == ExprCall && !validFunctionID(program, expr.Function) {
		return fmt.Errorf("expression calls invalid function %d", expr.Function)
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
	if expr.Kind == ExprMakeClosure && !validFunctionID(program, expr.Function) {
		return fmt.Errorf("expression creates invalid closure function %d", expr.Function)
	}
	if expr.Kind == ExprMakeClosure && validFunctionID(program, expr.Function) {
		closureFn := program.Functions[expr.Function]
		if len(expr.CaptureLocals) != len(closureFn.Captures) {
			return fmt.Errorf("closure %s expects %d captures, got %d", closureFn.Name, len(closureFn.Captures), len(expr.CaptureLocals))
		}
		for i, local := range expr.CaptureLocals {
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
		member, ok := unionMemberByTag(unionType, expr.Tag)
		if !ok {
			return fmt.Errorf("union wrap has invalid tag %d for %s", expr.Tag, unionType.Name)
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
		if len(expr.TypeArgs) != 1 {
			return fmt.Errorf("unsafe::cast expression expects one target type, got %d", len(expr.TypeArgs))
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
		traitType, err := typeInfo(program, expr.Type)
		if err != nil {
			return err
		}
		if traitType.Kind != TypeTraitObject {
			return fmt.Errorf("trait upcast target type has kind %d", traitType.Kind)
		}
		if traitType.Trait != expr.Trait {
			return fmt.Errorf("trait upcast expression trait %d does not match type trait %d", expr.Trait, traitType.Trait)
		}
		if !validImplID(program, expr.Impl) {
			return fmt.Errorf("trait upcast has invalid impl id %d", expr.Impl)
		}
		impl := program.Impls[expr.Impl]
		if impl.Trait != expr.Trait {
			return fmt.Errorf("trait upcast impl %d has trait %d, want %d", expr.Impl, impl.Trait, expr.Trait)
		}
		if impl.ForType != expr.Target.Type {
			return fmt.Errorf("trait upcast impl %d is for type %d, got target type %d", expr.Impl, impl.ForType, expr.Target.Type)
		}
	}
	for _, local := range expr.CaptureLocals {
		if local < 0 || int(local) >= len(fn.Locals) {
			return fmt.Errorf("expression captures invalid local %d", local)
		}
	}
	if expr.Target != nil {
		if err := validateExpr(program, fn, *expr.Target); err != nil {
			return err
		}
	}
	if expr.Left != nil {
		if err := validateExpr(program, fn, *expr.Left); err != nil {
			return err
		}
	}
	if expr.Right != nil {
		if err := validateExpr(program, fn, *expr.Right); err != nil {
			return err
		}
	}
	if expr.Condition != nil {
		if err := validateExpr(program, fn, *expr.Condition); err != nil {
			return err
		}
	}
	if expr.Kind == ExprBlock {
		if err := validateBlock(program, fn, expr.Body); err != nil {
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
		helperFn := fn
		helperFn.Signature.Return = expr.Type
		if err := validateBlock(program, helperFn, expr.Body); err != nil {
			return err
		}
	}
	if expr.Kind == ExprIf {
		if err := validateBlock(program, fn, expr.Then); err != nil {
			return err
		}
		if err := validateBlock(program, fn, expr.Else); err != nil {
			return err
		}
	}
	if expr.Kind == ExprMatchEnum {
		for _, matchCase := range expr.EnumCases {
			if err := validateBlock(program, fn, matchCase.Body); err != nil {
				return err
			}
		}
		if err := validateBlock(program, fn, expr.CatchAll); err != nil {
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
		for _, matchCase := range expr.StrCases {
			if err := validateBlock(program, fn, matchCase.Body); err != nil {
				return err
			}
		}
		if err := validateBlock(program, fn, expr.CatchAll); err != nil {
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
		for _, matchCase := range expr.IntCases {
			if err := validateBlock(program, fn, matchCase.Body); err != nil {
				return err
			}
		}
		for _, matchCase := range expr.RangeCases {
			if matchCase.Start > matchCase.End {
				return fmt.Errorf("int match range start %d is greater than end %d", matchCase.Start, matchCase.End)
			}
			if err := validateBlock(program, fn, matchCase.Body); err != nil {
				return err
			}
		}
		if err := validateBlock(program, fn, expr.CatchAll); err != nil {
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
		for _, matchCase := range expr.UnionCases {
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
		if err := validateBlock(program, fn, expr.CatchAll); err != nil {
			return err
		}
	}
	if expr.Kind == ExprCallTrait {
		if expr.Target == nil {
			return fmt.Errorf("trait call missing target")
		}
		targetType, err := referentTypeInfo(program, expr.Target.Type)
		if err != nil {
			return err
		}
		if targetType.Kind != TypeTraitObject {
			return fmt.Errorf("trait call target has type kind %d", targetType.Kind)
		}
		if targetType.Trait != expr.Trait {
			return fmt.Errorf("trait call expression trait %d does not match target type trait %d", expr.Trait, targetType.Trait)
		}
		if !validTraitID(program, expr.Trait) {
			return fmt.Errorf("trait call has invalid trait id %d", expr.Trait)
		}
		trait := program.Traits[expr.Trait]
		if expr.Method < 0 || expr.Method >= len(trait.Methods) {
			return fmt.Errorf("trait call has invalid method index %d for trait %s", expr.Method, trait.Name)
		}
		method := trait.Methods[expr.Method]
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
		if expr.SomeLocal < 0 || int(expr.SomeLocal) >= len(fn.Locals) {
			return fmt.Errorf("Maybe match binds invalid local %d", expr.SomeLocal)
		}
		if fn.Locals[expr.SomeLocal].Type != maybeType.Elem {
			return fmt.Errorf("Maybe match local type %d does not match element type %d", fn.Locals[expr.SomeLocal].Type, maybeType.Elem)
		}
		if err := validateBlock(program, fn, expr.Some); err != nil {
			return err
		}
		if err := validateBlock(program, fn, expr.None); err != nil {
			return err
		}
	}
	if expr.Kind == ExprSelect {
		for _, arm := range expr.SelectCases {
			if arm.HasBind && (arm.BindLocal < 0 || int(arm.BindLocal) >= len(fn.Locals)) {
				return fmt.Errorf("select recv arm binds invalid local %d", arm.BindLocal)
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
		if expr.OkLocal < 0 || int(expr.OkLocal) >= len(fn.Locals) {
			return fmt.Errorf("Result match binds invalid ok local %d", expr.OkLocal)
		}
		if expr.ErrLocal < 0 || int(expr.ErrLocal) >= len(fn.Locals) {
			return fmt.Errorf("Result match binds invalid err local %d", expr.ErrLocal)
		}
		if fn.Locals[expr.OkLocal].Type != resultType.Value {
			return fmt.Errorf("Result ok match local type %d does not match value type %d", fn.Locals[expr.OkLocal].Type, resultType.Value)
		}
		if fn.Locals[expr.ErrLocal].Type != resultType.Error {
			return fmt.Errorf("Result err match local type %d does not match error type %d", fn.Locals[expr.ErrLocal].Type, resultType.Error)
		}
		if err := validateBlock(program, fn, expr.Ok); err != nil {
			return err
		}
		if err := validateBlock(program, fn, expr.Err); err != nil {
			return err
		}
	}
	if expr.Kind == ExprTryResult || expr.Kind == ExprTryMaybe {
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
		if !expr.HasCatch {
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
		if expr.HasCatch {
			if expr.Kind == ExprTryResult && (expr.CatchLocal < 0 || int(expr.CatchLocal) >= len(fn.Locals)) {
				return fmt.Errorf("Result try catch binds invalid local %d", expr.CatchLocal)
			}
			if expr.Kind == ExprTryResult && fn.Locals[expr.CatchLocal].Type != targetType.Error {
				return fmt.Errorf("Result try catch local type %d does not match error type %d", fn.Locals[expr.CatchLocal].Type, targetType.Error)
			}
			if err := validateBlock(program, fn, expr.Catch); err != nil {
				return err
			}
		}
	}
	for _, arg := range expr.Args {
		if err := validateExpr(program, fn, arg); err != nil {
			return err
		}
	}
	for _, entry := range expr.Entries {
		if err := validateExpr(program, fn, entry.Key); err != nil {
			return err
		}
		if err := validateExpr(program, fn, entry.Value); err != nil {
			return err
		}
	}
	for _, field := range expr.Fields {
		if err := validateExpr(program, fn, field.Value); err != nil {
			return err
		}
	}
	return nil
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
		return left.ParamIndex == right.ParamIndex && left.Name == right.Name
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
