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
	for _, ext := range program.Externs {
		if err := validateSignature(program, ext.Signature); err != nil {
			return fmt.Errorf("extern %s: %w", ext.Name, err)
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
	case TypeList, TypeMaybe, TypeFiber:
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
		if method.Signature.Params[0].Type != impl.ForType {
			return fmt.Errorf("impl %d method %s receiver type %d does not match impl type %d", impl.ID, method.Name, method.Signature.Params[0].Type, impl.ForType)
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
	return nil
}

func validateSignature(program *Program, sig Signature) error {
	for _, param := range sig.Params {
		if !validTypeID(program, param.Type) {
			return fmt.Errorf("parameter %s has invalid type %d", param.Name, param.Type)
		}
	}
	if !validTypeID(program, sig.Return) {
		return fmt.Errorf("signature has invalid return type %d", sig.Return)
	}
	return nil
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
		if stmt.Condition != nil {
			if err := validateExpr(program, fn, *stmt.Condition); err != nil {
				return err
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
	if expr.Kind == ExprLoadLocal && (expr.Local < 0 || int(expr.Local) >= len(fn.Locals)) {
		return fmt.Errorf("expression loads invalid local %d", expr.Local)
	}
	if expr.Kind == ExprCall && !validFunctionID(program, expr.Function) {
		return fmt.Errorf("expression calls invalid function %d", expr.Function)
	}
	if expr.Kind == ExprSpawnFiber && expr.Target == nil && !validFunctionID(program, expr.Function) {
		return fmt.Errorf("expression spawns invalid fiber function %d", expr.Function)
	}
	if expr.Kind == ExprSpawnFiber && expr.Target != nil && expr.Target.Kind == ExprMakeClosure {
		for _, local := range expr.Target.CaptureLocals {
			if local < 0 || int(local) >= len(fn.Locals) {
				return fmt.Errorf("fiber spawn captures invalid local %d", local)
			}
			if fn.Locals[local].Mutable {
				return fmt.Errorf("fiber spawn closure cannot capture mutable local %s", fn.Locals[local].Name)
			}
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
	if expr.Kind == ExprCallExtern && (expr.Extern < 0 || int(expr.Extern) >= len(program.Externs)) {
		return fmt.Errorf("expression calls invalid extern %d", expr.Extern)
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
	if expr.Kind == ExprToDynamic && expr.Target == nil {
		return fmt.Errorf("to_dyn expression missing target")
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
		targetType, err := typeInfo(program, expr.Target.Type)
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
		if expr.SomeLocal < 0 || int(expr.SomeLocal) >= len(fn.Locals) {
			return fmt.Errorf("Maybe match binds invalid local %d", expr.SomeLocal)
		}
		if err := validateBlock(program, fn, expr.Some); err != nil {
			return err
		}
		if err := validateBlock(program, fn, expr.None); err != nil {
			return err
		}
	}
	if expr.Kind == ExprMatchResult {
		if expr.OkLocal < 0 || int(expr.OkLocal) >= len(fn.Locals) {
			return fmt.Errorf("Result match binds invalid ok local %d", expr.OkLocal)
		}
		if expr.ErrLocal < 0 || int(expr.ErrLocal) >= len(fn.Locals) {
			return fmt.Errorf("Result match binds invalid err local %d", expr.ErrLocal)
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

func validTraitID(program *Program, id TraitID) bool {
	return id >= 0 && int(id) < len(program.Traits)
}

func validImplID(program *Program, id ImplID) bool {
	return id >= 0 && int(id) < len(program.Impls)
}

func typeInfo(program *Program, id TypeID) (TypeInfo, error) {
	if !validTypeID(program, id) {
		return TypeInfo{}, fmt.Errorf("invalid type id %d", id)
	}
	return program.Types[id-1], nil
}

func unionMemberByTag(unionType TypeInfo, tag uint32) (UnionMember, bool) {
	for _, member := range unionType.Members {
		if member.Tag == tag {
			return member, true
		}
	}
	return UnionMember{}, false
}
