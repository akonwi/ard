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
	for i, fn := range program.Functions {
		if fn.ID != FunctionID(i) {
			return fmt.Errorf("function table entry %d has id %d", i, fn.ID)
		}
		if err := validateFunction(program, fn); err != nil {
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

func typeInfo(program *Program, id TypeID) (TypeInfo, error) {
	if !validTypeID(program, id) {
		return TypeInfo{}, fmt.Errorf("invalid type id %d", id)
	}
	return program.Types[id-1], nil
}
