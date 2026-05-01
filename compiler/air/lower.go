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
	functions    map[string]FunctionID
	externs      map[string]ExternID

	loweringModules map[string]bool
	loweredModules  map[string]bool
}

type functionLowerer struct {
	l      *lowerer
	locals map[string]LocalID
	fn     *Function
}

func newLowerer() *lowerer {
	l := &lowerer{
		program: Program{
			Entry: NoFunction,
		},
		moduleByPath: map[string]ModuleID{},
		moduleByName: map[string]checker.Module{},
		typeByName:   map[string]TypeID{},
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
		if def, ok := stmt.Expr.(*checker.FunctionDef); ok {
			if err := l.lowerFunction(modID, def); err != nil {
				return err
			}
		}
	}

	topLevel := topLevelExecutableStatements(prog.Statements)
	if len(topLevel) > 0 {
		mainID, err := l.declareSyntheticMain(modID)
		if err != nil {
			return err
		}
		fn := l.program.Functions[mainID]
		fl := &functionLowerer{l: l, locals: map[string]LocalID{}, fn: &fn}
		body, err := fl.lowerBlock(topLevel)
		if err != nil {
			return err
		}
		fn.Body = body
		l.program.Functions[mainID] = fn
		l.program.Entry = mainID
		mod.Functions = appendUniqueFunction(mod.Functions, mainID)
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
	if includeTests && def.IsTest {
		l.program.Tests = append(l.program.Tests, Test{Name: def.Name, Function: id})
	}
	return id, nil
}

func (l *lowerer) declareSyntheticMain(module ModuleID) (FunctionID, error) {
	key := functionKey(module, "main")
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
		ID:     id,
		Module: module,
		Name:   "main",
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

func (l *lowerer) internType(t checker.Type) (TypeID, error) {
	if t == nil {
		return NoType, fmt.Errorf("cannot intern nil type")
	}
	if tv, ok := t.(*checker.TypeVar); ok && tv.Actual() != nil {
		return l.internType(tv.Actual())
	}
	name := t.String()
	if id, ok := l.typeByName[name]; ok {
		return id, nil
	}

	id := TypeID(len(l.program.Types) + 1)
	l.typeByName[name] = id
	l.program.Types = append(l.program.Types, TypeInfo{ID: id, Name: name})
	idx := len(l.program.Types) - 1
	info := &l.program.Types[idx]

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
		info.Kind = TypeTraitObject
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

	return id, nil
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
		lowered, err := fl.lowerStmt(stmt)
		if err != nil {
			return block, err
		}
		if lowered != nil {
			block.Stmts = append(block.Stmts, *lowered)
		}
	}
	return block, nil
}

func (fl *functionLowerer) lowerExprWithExpected(expr checker.Expression, expected TypeID) (*Expr, error) {
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
	case *checker.Variable:
		local, ok := fl.locals[e.Name()]
		if !ok {
			return nil, fmt.Errorf("unknown local %s", e.Name())
		}
		return &Expr{Kind: ExprLoadLocal, Type: typeID, Local: local}, nil
	case *checker.FunctionCall:
		args, err := fl.lowerArgs(e.Args)
		if err != nil {
			return nil, err
		}
		if def := e.Definition(); def != nil {
			id, ok := fl.l.lookupFunction(def.Name)
			if !ok {
				return nil, fmt.Errorf("unknown function call target %s", def.Name)
			}
			return &Expr{Kind: ExprCall, Type: typeID, Function: id, Args: args}, nil
		}
		if id, ok := fl.l.lookupExtern(e.Name); ok {
			return &Expr{Kind: ExprCallExtern, Type: typeID, Extern: id, Args: args}, nil
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
			args, err := fl.lowerArgs(e.Call.Args)
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: ExprCall, Type: typeID, Function: id, Args: args}, nil
		}
		args, err := fl.lowerArgs(e.Call.Args)
		if err != nil {
			return nil, err
		}
		extern, err := fl.l.declareModuleCallExtern(e)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: ExprCallExtern, Type: typeID, Extern: extern, Args: args}, nil
	case *checker.StructInstance:
		return fl.lowerStructInstance(typeID, e)
	case *checker.InstanceProperty:
		return fl.lowerInstanceProperty(typeID, e)
	case *checker.EnumVariant:
		return &Expr{Kind: ExprEnumVariant, Type: typeID, Variant: int(e.Variant), Discriminant: e.Discriminant}, nil
	case *checker.BoolMatch:
		return fl.lowerBoolMatch(typeID, e)
	case *checker.EnumMatch:
		return fl.lowerEnumMatch(typeID, e)
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
		nested, err := fl.lowerExprWithExpected(expr.ElseIf, defaultType)
		if err != nil {
			return Block{}, err
		}
		nested.Type = defaultType
		return Block{Result: nested}, nil
	}
	if expr.Else != nil {
		return fl.lowerBlockWithDefault(expr.Else.Stmts, defaultType)
	}
	return Block{}, nil
}

func (fl *functionLowerer) lowerResultConstructor(kind ExprKind, typeID TypeID, call *checker.ModuleFunctionCall) (*Expr, error) {
	args, err := fl.lowerArgs(call.Call.Args)
	if err != nil {
		return nil, err
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("%s::%s expects one argument", call.Module, call.Call.Name)
	}
	value := args[0]
	return &Expr{Kind: kind, Type: typeID, Target: &value}, nil
}

func (fl *functionLowerer) lowerMaybeConstructor(kind ExprKind, typeID TypeID, call *checker.ModuleFunctionCall) (*Expr, error) {
	switch kind {
	case ExprMakeMaybeSome:
		args, err := fl.lowerArgs(call.Call.Args)
		if err != nil {
			return nil, err
		}
		if len(args) != 1 {
			return nil, fmt.Errorf("%s::%s expects one argument", call.Module, call.Call.Name)
		}
		value := args[0]
		return &Expr{Kind: kind, Type: typeID, Target: &value}, nil
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
	case checker.MaybeMap, checker.MaybeAndThen:
		return nil, fmt.Errorf("unsupported AIR Maybe method requiring closures: %d", method.Kind)
	default:
		return nil, fmt.Errorf("unsupported AIR Maybe method %d", method.Kind)
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
	case checker.ResultMap, checker.ResultMapErr, checker.ResultAndThen:
		return nil, fmt.Errorf("unsupported AIR Result method requiring closures: %d", method.Kind)
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
	out := make([]Expr, len(args))
	for i, arg := range args {
		lowered, err := fl.lowerExpr(arg)
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

func (fl *functionLowerer) defineLocal(name string, typeID TypeID, mutable bool) LocalID {
	id := LocalID(len(fl.fn.Locals))
	fl.fn.Locals = append(fl.fn.Locals, Local{ID: id, Name: name, Type: typeID, Mutable: mutable})
	fl.locals[name] = id
	return id
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

func keyHasFunctionName(key, name string) bool {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == ':' {
			return key[i+1:] == name
		}
	}
	return key == name
}
