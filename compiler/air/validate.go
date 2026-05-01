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
	if expr.Kind == ExprCallExtern && (expr.Extern < 0 || int(expr.Extern) >= len(program.Externs)) {
		return fmt.Errorf("expression calls invalid extern %d", expr.Extern)
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
	for _, arg := range expr.Args {
		if err := validateExpr(program, fn, arg); err != nil {
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
