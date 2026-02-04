package bytecode

import (
	"fmt"
	"slices"

	"github.com/akonwi/ard/checker"
)

type Emitter struct {
	program      Program
	typeIndex    map[string]TypeID
	funcIndex    map[string]int
	funcTypes    map[string]checker.Type
	anonCount    int
	modulePrefix string
	moduleFuncs  map[string]struct{}
	visitedMods  map[string]struct{}
	modules      map[string]checker.Module
}

type funcEmitter struct {
	emitter    *Emitter
	code       []Instruction
	locals     map[string]int
	localTop   int
	stack      int
	maxStack   int
	breakStack [][]int
	tempIndex  int
	parent     *funcEmitter
	captureIdx map[string]int
	captures   []string
	capLocals  []int
}

func NewEmitter() *Emitter {
	return &Emitter{
		program: Program{
			Constants: []Constant{},
			Types:     []TypeEntry{},
			Functions: []Function{},
		},
		typeIndex:   map[string]TypeID{},
		funcIndex:   map[string]int{},
		funcTypes:   map[string]checker.Type{},
		visitedMods: map[string]struct{}{},
	}
}

func (e *Emitter) EmitProgram(module checker.Module) (Program, error) {
	prog := module.Program()
	if prog == nil {
		return e.program, nil
	}
	e.modules = prog.Imports

	fn := &funcEmitter{
		emitter: e,
		code:    []Instruction{},
		locals:  map[string]int{},
	}

	for i := range prog.Statements {
		stmt := prog.Statements[i]
		if stmt.Expr != nil {
			if def, ok := stmt.Expr.(*checker.FunctionDef); ok {
				if err := e.emitFunction(def); err != nil {
					return Program{}, err
				}
			}
			if def, ok := stmt.Expr.(*checker.ExternalFunctionDef); ok {
				if _, err := e.emitExternWrapper(def.Name, def); err != nil {
					return Program{}, err
				}
			}
		}
		switch def := stmt.Stmt.(type) {
		case *checker.StructDef:
			if err := e.emitStructMethods(def); err != nil {
				return Program{}, err
			}
		case *checker.Enum:
			if err := e.emitEnumMethods(def); err != nil {
				return Program{}, err
			}
		default:
			_ = def
		}
	}
	if prog.Imports != nil {
		for _, mod := range prog.Imports {
			if err := e.emitModule(mod); err != nil {
				return Program{}, err
			}
		}
	}
	if err := fn.emitStatements(prog.Statements); err != nil {
		return Program{}, err
	}
	fn.ensureReturn()

	e.program.Functions = append(e.program.Functions, Function{
		Name:     "main",
		Arity:    0,
		Locals:   fn.localTop,
		MaxStack: fn.maxStack,
		Code:     fn.code,
	})

	return e.program, nil
}

func (e *Emitter) addType(t checker.Type) TypeID {
	if t == nil {
		return 0
	}
	name := t.String()
	if id, ok := e.typeIndex[name]; ok {
		return id
	}
	id := TypeID(len(e.program.Types) + 1)
	e.program.Types = append(e.program.Types, TypeEntry{ID: id, Name: name})
	e.typeIndex[name] = id
	return id
}

func (e *Emitter) addConst(c Constant) int {
	for i := range e.program.Constants {
		if e.program.Constants[i] == c {
			return i
		}
	}
	index := len(e.program.Constants)
	e.program.Constants = append(e.program.Constants, c)
	return index
}

func (e *Emitter) emitFunction(def *checker.FunctionDef) error {
	_, _, err := e.emitFunctionWithParent(def, nil)
	return err
}

func (e *Emitter) emitFunctionWithParent(def *checker.FunctionDef, parent *funcEmitter) (int, []string, error) {
	fnName := def.Name
	if fnName == "" {
		fnName = fmt.Sprintf("@anon%d", e.anonCount)
		e.anonCount++
	}
	if e.modulePrefix != "" {
		if _, ok := e.moduleFuncs[def.Name]; ok {
			fnName = fmt.Sprintf("%s::%s", e.modulePrefix, def.Name)
		}
	}
	index := -1
	if def.Name != "" {
		if idx, ok := e.funcIndex[fnName]; ok {
			return idx, nil, nil
		}
		index = len(e.program.Functions)
		e.funcIndex[fnName] = index
		e.funcTypes[fnName] = def
		e.program.Functions = append(e.program.Functions, Function{
			Name:  fnName,
			Arity: len(def.Parameters),
		})
	}

	fnEmitter := &funcEmitter{
		emitter:    e,
		code:       []Instruction{},
		locals:     map[string]int{},
		parent:     parent,
		captureIdx: map[string]int{},
	}
	for _, param := range def.Parameters {
		fnEmitter.defineLocal(param.Name)
	}
	if def.Body != nil {
		if err := fnEmitter.emitStatements(def.Body.Stmts); err != nil {
			return -1, nil, err
		}
	}
	fnEmitter.ensureReturn()

	if index == -1 {
		index = len(e.program.Functions)
	}
	fn := Function{
		Name:     fnName,
		Arity:    len(def.Parameters),
		Captures: append([]int{}, fnEmitter.capLocals...),
		Locals:   fnEmitter.localTop,
		MaxStack: fnEmitter.maxStack,
		Code:     fnEmitter.code,
	}
	if index < len(e.program.Functions) {
		e.program.Functions[index] = fn
	} else {
		e.program.Functions = append(e.program.Functions, fn)
	}
	return index, append([]string{}, fnEmitter.captures...), nil
}

func (e *Emitter) emitStructMethods(def *checker.StructDef) error {
	for name, method := range def.Methods {
		methodName := fmt.Sprintf("%s.%s", def.Name, name)
		if _, ok := e.funcIndex[methodName]; ok {
			continue
		}
		copy := *method
		methodDef := &copy
		methodDef.Name = methodName
		methodDef.Parameters = append([]checker.Parameter{
			{Name: "@", Type: def},
		}, method.Parameters...)
		if _, _, err := e.emitFunctionWithParent(methodDef, nil); err != nil {
			return err
		}
	}
	return nil
}

func (e *Emitter) emitEnumMethods(def *checker.Enum) error {
	for name, method := range def.Methods {
		methodName := fmt.Sprintf("%s.%s", def.Name, name)
		if _, ok := e.funcIndex[methodName]; ok {
			continue
		}
		copy := *method
		methodDef := &copy
		methodDef.Name = methodName
		methodDef.Parameters = append([]checker.Parameter{
			{Name: "@", Type: def},
		}, method.Parameters...)
		if _, _, err := e.emitFunctionWithParent(methodDef, nil); err != nil {
			return err
		}
	}
	return nil
}

func (e *Emitter) emitModule(mod checker.Module) error {
	if mod == nil {
		return nil
	}
	path := mod.Path()
	if path == "" {
		return nil
	}
	if _, ok := e.visitedMods[path]; ok {
		return nil
	}
	e.visitedMods[path] = struct{}{}
	prog := mod.Program()
	if prog == nil {
		return nil
	}
	if prog.Imports != nil {
		for _, dep := range prog.Imports {
			if err := e.emitModule(dep); err != nil {
				return err
			}
		}
	}
	moduleFuncs := e.collectModuleFuncs(prog)
	prevPrefix := e.modulePrefix
	prevFuncs := e.moduleFuncs
	e.modulePrefix = path
	e.moduleFuncs = moduleFuncs
	for i := range prog.Statements {
		stmt := prog.Statements[i]
		if stmt.Expr != nil {
			if def, ok := stmt.Expr.(*checker.FunctionDef); ok {
				if err := e.emitFunction(def); err != nil {
					e.modulePrefix = prevPrefix
					e.moduleFuncs = prevFuncs
					return err
				}
			}
			if def, ok := stmt.Expr.(*checker.ExternalFunctionDef); ok {
				name := e.qualifyName(def.Name)
				if _, err := e.emitExternWrapper(name, def); err != nil {
					e.modulePrefix = prevPrefix
					e.moduleFuncs = prevFuncs
					return err
				}
			}
		}
		switch def := stmt.Stmt.(type) {
		case *checker.StructDef:
			if err := e.emitStructMethods(def); err != nil {
				e.modulePrefix = prevPrefix
				e.moduleFuncs = prevFuncs
				return err
			}
		case *checker.Enum:
			if err := e.emitEnumMethods(def); err != nil {
				e.modulePrefix = prevPrefix
				e.moduleFuncs = prevFuncs
				return err
			}
		default:
			_ = def
		}
	}
	e.modulePrefix = prevPrefix
	e.moduleFuncs = prevFuncs
	return nil
}

func (e *Emitter) collectModuleFuncs(prog *checker.Program) map[string]struct{} {
	funcs := map[string]struct{}{}
	if prog == nil {
		return funcs
	}
	for i := range prog.Statements {
		stmt := prog.Statements[i]
		if stmt.Expr != nil {
			if def, ok := stmt.Expr.(*checker.FunctionDef); ok {
				if def.Name != "" {
					funcs[def.Name] = struct{}{}
				}
			}
			if def, ok := stmt.Expr.(*checker.ExternalFunctionDef); ok {
				if def.Name != "" {
					funcs[def.Name] = struct{}{}
				}
			}
		}
	}
	return funcs
}

func (e *Emitter) qualifyName(name string) string {
	if e.modulePrefix != "" {
		if _, ok := e.moduleFuncs[name]; ok {
			return fmt.Sprintf("%s::%s", e.modulePrefix, name)
		}
	}
	return name
}

func (e *Emitter) nextAnonName(prefix string) string {
	name := fmt.Sprintf("@%s%d", prefix, e.anonCount)
	e.anonCount++
	return name
}

func (e *Emitter) emitExternWrapper(name string, def *checker.ExternalFunctionDef) (int, error) {
	if idx, ok := e.funcIndex[name]; ok {
		return idx, nil
	}
	fnEmitter := &funcEmitter{
		emitter: e,
		code:    []Instruction{},
		locals:  map[string]int{},
	}
	for _, param := range def.Parameters {
		fnEmitter.defineLocal(param.Name)
	}
	for i := range def.Parameters {
		fnEmitter.emit(Instruction{Op: OpLoadLocal, A: i})
	}
	bindingIdx := e.addConst(Constant{Kind: ConstStr, Str: def.ExternalBinding})
	retID := e.addType(def.ReturnType)
	fnEmitter.emit(Instruction{Op: OpCallExtern, A: bindingIdx, Imm: len(def.Parameters), C: int(retID)})
	fnEmitter.adjustStack(len(def.Parameters), 1)
	fnEmitter.ensureReturn()
	index := len(e.program.Functions)
	e.funcIndex[name] = index
	e.funcTypes[name] = def
	e.program.Functions = append(e.program.Functions, Function{
		Name:     name,
		Arity:    len(def.Parameters),
		Locals:   fnEmitter.localTop,
		MaxStack: fnEmitter.maxStack,
		Code:     fnEmitter.code,
	})
	return index, nil
}

func (f *funcEmitter) emitStatements(stmts []checker.Statement) error {
	for i := range stmts {
		stmt := stmts[i]
		isLast := i == len(stmts)-1
		if stmt.Break {
			idx := f.emitJump(OpJump)
			if err := f.addBreakJump(idx); err != nil {
				return err
			}
			continue
		}
		if stmt.Stmt != nil {
			if err := f.emitStatement(stmt.Stmt); err != nil {
				return err
			}
			if isLast {
				f.emit(Instruction{Op: OpConstVoid})
			}
			continue
		}
		if stmt.Expr != nil {
			if err := f.emitExpr(stmt.Expr); err != nil {
				return err
			}
			if !isLast {
				f.emit(Instruction{Op: OpPop})
			}
			continue
		}
	}

	if len(stmts) == 0 {
		f.emit(Instruction{Op: OpConstVoid})
	}

	return nil
}

func (f *funcEmitter) emitStatementsDiscard(stmts []checker.Statement) error {
	for i := range stmts {
		stmt := stmts[i]
		if stmt.Break {
			idx := f.emitJump(OpJump)
			if err := f.addBreakJump(idx); err != nil {
				return err
			}
			continue
		}
		if stmt.Stmt != nil {
			if err := f.emitStatement(stmt.Stmt); err != nil {
				return err
			}
			continue
		}
		if stmt.Expr != nil {
			if err := f.emitExpr(stmt.Expr); err != nil {
				return err
			}
			f.emit(Instruction{Op: OpPop})
		}
		_ = i
	}
	return nil
}

func (f *funcEmitter) emitStatement(stmt checker.NonProducing) error {
	switch s := stmt.(type) {
	case *checker.VariableDef:
		if err := f.emitExpr(s.Value); err != nil {
			return err
		}
		index := f.localIndex(s.Name)
		f.emit(Instruction{Op: OpStoreLocal, A: index})
		return nil
	case *checker.Reassignment:
		switch target := s.Target.(type) {
		case *checker.InstanceProperty:
			if err := f.emitExpr(target.Subject); err != nil {
				return err
			}
			if err := f.emitExpr(s.Value); err != nil {
				return err
			}
			nameIdx := f.emitter.addConst(Constant{Kind: ConstStr, Str: target.Property})
			f.emit(Instruction{Op: OpSetField, A: nameIdx})
			f.adjustStack(2, 0)
		default:
			if err := f.emitExpr(s.Value); err != nil {
				return err
			}
			index, err := f.resolveTargetLocal(s.Target)
			if err != nil {
				return err
			}
			f.emit(Instruction{Op: OpStoreLocal, A: index})
		}
		return nil
	case *checker.WhileLoop:
		return f.emitWhileLoop(s)
	case *checker.ForIntRange:
		return f.emitForIntRange(s)
	case *checker.ForInList:
		return f.emitForInList(s)
	case *checker.ForInMap:
		return f.emitForInMap(s)
	case *checker.StructDef:
		return nil
	case *checker.Enum:
		return nil
	default:
		return fmt.Errorf("unsupported statement: %T", s)
	}
}

func (f *funcEmitter) emitExpr(expr checker.Expression) error {
	switch e := expr.(type) {
	case *checker.IntLiteral:
		f.emit(Instruction{Op: OpConstInt, Imm: e.Value})
		return nil
	case *checker.FloatLiteral:
		idx := f.emitter.addConst(Constant{Kind: ConstFloat, Float: e.Value})
		f.emit(Instruction{Op: OpConst, A: idx})
		return nil
	case *checker.StrLiteral:
		idx := f.emitter.addConst(Constant{Kind: ConstStr, Str: e.Value})
		f.emit(Instruction{Op: OpConst, A: idx})
		return nil
	case *checker.TemplateStr:
		if len(e.Chunks) == 0 {
			idx := f.emitter.addConst(Constant{Kind: ConstStr, Str: ""})
			f.emit(Instruction{Op: OpConst, A: idx})
			return nil
		}
		if err := f.emitExpr(e.Chunks[0]); err != nil {
			return err
		}
		for i := 1; i < len(e.Chunks); i++ {
			if err := f.emitExpr(e.Chunks[i]); err != nil {
				return err
			}
			f.emit(Instruction{Op: OpAdd})
		}
		return nil
	case *checker.CopyExpression:
		if err := f.emitExpr(e.Expr); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpCopy})
		return nil
	case *checker.BoolLiteral:
		imm := 0
		if e.Value {
			imm = 1
		}
		f.emit(Instruction{Op: OpConstBool, Imm: imm})
		return nil
	case *checker.VoidLiteral:
		f.emit(Instruction{Op: OpConstVoid})
		return nil
	case *checker.Variable:
		name := e.Name()
		if idx, ok := f.localLookup(name); ok {
			f.emit(Instruction{Op: OpLoadLocal, A: idx})
			return nil
		}
		if f.parent != nil && f.parent.hasLocalInChain(name) {
			idx := f.captureLocal(name)
			f.emit(Instruction{Op: OpLoadLocal, A: idx})
			return nil
		}
		if fnIndex, ok := f.emitter.funcIndex[name]; ok {
			fnType, ok := f.emitter.funcTypes[name]
			if !ok {
				return fmt.Errorf("missing function type for %s", name)
			}
			typeID := f.emitter.addType(fnType)
			f.emit(Instruction{Op: OpMakeClosure, A: fnIndex, B: 0, C: int(typeID)})
			f.adjustStack(0, 1)
			return nil
		}
		return fmt.Errorf("unknown identifier: %s", name)
	case *checker.Identifier:
		name := e.Name
		if idx, ok := f.localLookup(name); ok {
			f.emit(Instruction{Op: OpLoadLocal, A: idx})
			return nil
		}
		if f.parent != nil && f.parent.hasLocalInChain(name) {
			idx := f.captureLocal(name)
			f.emit(Instruction{Op: OpLoadLocal, A: idx})
			return nil
		}
		if fnIndex, ok := f.emitter.funcIndex[name]; ok {
			fnType, ok := f.emitter.funcTypes[name]
			if !ok {
				return fmt.Errorf("missing function type for %s", name)
			}
			typeID := f.emitter.addType(fnType)
			f.emit(Instruction{Op: OpMakeClosure, A: fnIndex, B: 0, C: int(typeID)})
			f.adjustStack(0, 1)
			return nil
		}
		return fmt.Errorf("unknown identifier: %s", name)
	case *checker.Negation:
		if err := f.emitExpr(e.Value); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpNeg})
		return nil
	case *checker.Not:
		if err := f.emitExpr(e.Value); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpNot})
		return nil
	case *checker.IntAddition, *checker.FloatAddition, *checker.StrAddition:
		return f.emitBinary(expr, OpAdd)
	case *checker.IntSubtraction, *checker.FloatSubtraction:
		return f.emitBinary(expr, OpSub)
	case *checker.IntMultiplication, *checker.FloatMultiplication:
		return f.emitBinary(expr, OpMul)
	case *checker.IntDivision, *checker.FloatDivision:
		return f.emitBinary(expr, OpDiv)
	case *checker.IntModulo:
		return f.emitBinary(expr, OpMod)
	case *checker.IntGreater, *checker.FloatGreater:
		return f.emitBinary(expr, OpGt)
	case *checker.IntGreaterEqual, *checker.FloatGreaterEqual:
		return f.emitBinary(expr, OpGte)
	case *checker.IntLess, *checker.FloatLess:
		return f.emitBinary(expr, OpLt)
	case *checker.IntLessEqual, *checker.FloatLessEqual:
		return f.emitBinary(expr, OpLte)
	case *checker.Equality:
		return f.emitBinary(expr, OpEq)
	case *checker.And:
		return f.emitBinary(expr, OpAnd)
	case *checker.Or:
		return f.emitBinary(expr, OpOr)
	case *checker.Block:
		return f.emitBlockExpr(e)
	case *checker.If:
		return f.emitIfExpr(e)
	case *checker.FunctionDef:
		fnIndex, captures, err := f.emitter.emitFunctionWithParent(e, f)
		if err != nil {
			return err
		}
		for _, name := range captures {
			idx, ok := f.localLookup(name)
			if !ok {
				return fmt.Errorf("missing captured local: %s", name)
			}
			f.emit(Instruction{Op: OpLoadLocal, A: idx})
		}
		typeID := f.emitter.addType(e.Type())
		f.emit(Instruction{Op: OpMakeClosure, A: fnIndex, B: len(captures), C: int(typeID)})
		f.adjustStack(len(captures), 1)
		return nil
	case *checker.FunctionCall:
		return f.emitFunctionCall(e)
	case *checker.ListLiteral:
		return f.emitListLiteral(e)
	case *checker.MapLiteral:
		return f.emitMapLiteral(e)
	case *checker.ListMethod:
		return f.emitListMethod(e)
	case *checker.MapMethod:
		return f.emitMapMethod(e)
	case *checker.StrMethod:
		return f.emitStrMethod(e)
	case *checker.IntMethod:
		return f.emitIntMethod(e)
	case *checker.FloatMethod:
		return f.emitFloatMethod(e)
	case *checker.BoolMethod:
		return f.emitBoolMethod(e)
	case *checker.MaybeMethod:
		return f.emitMaybeMethod(e)
	case *checker.ResultMethod:
		return f.emitResultMethod(e)
	case *checker.ModuleFunctionCall:
		return f.emitModuleFunctionCall(e)
	case *checker.StructInstance:
		return f.emitStructInstance(e)
	case *checker.ModuleStructInstance:
		return f.emitModuleStructInstance(e)
	case *checker.EnumVariant:
		return f.emitEnumVariant(e)
	case *checker.InstanceProperty:
		return f.emitInstanceProperty(e)
	case *checker.InstanceMethod:
		return f.emitInstanceMethod(e)
	case *checker.BoolMatch:
		return f.emitBoolMatch(e)
	case *checker.IntMatch:
		return f.emitIntMatch(e)
	case *checker.OptionMatch:
		return f.emitOptionMatch(e)
	case *checker.ResultMatch:
		return f.emitResultMatch(e)
	case *checker.ConditionalMatch:
		return f.emitConditionalMatch(e)
	case *checker.EnumMatch:
		return f.emitEnumMatch(e)
	case *checker.UnionMatch:
		return f.emitUnionMatch(e)
	case *checker.TryOp:
		return f.emitTryOp(e)
	case *checker.Panic:
		if err := f.emitExpr(e.Message); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpPanic})
		return nil
	case *checker.ModuleSymbol:
		return f.emitModuleSymbol(e)
	case *checker.FiberExecution:
		return f.emitFiberExecution(e)
	case *checker.FiberEval:
		return f.emitFiberEval(e)
	default:
		return fmt.Errorf("unsupported expression: %T", e)
	}
}

func (f *funcEmitter) emitBinary(expr checker.Expression, op Opcode) error {
	var left checker.Expression
	var right checker.Expression
	switch e := expr.(type) {
	case *checker.IntAddition:
		left, right = e.Left, e.Right
	case *checker.IntSubtraction:
		left, right = e.Left, e.Right
	case *checker.IntMultiplication:
		left, right = e.Left, e.Right
	case *checker.IntDivision:
		left, right = e.Left, e.Right
	case *checker.IntModulo:
		left, right = e.Left, e.Right
	case *checker.IntGreater:
		left, right = e.Left, e.Right
	case *checker.IntGreaterEqual:
		left, right = e.Left, e.Right
	case *checker.IntLess:
		left, right = e.Left, e.Right
	case *checker.IntLessEqual:
		left, right = e.Left, e.Right
	case *checker.FloatAddition:
		left, right = e.Left, e.Right
	case *checker.FloatSubtraction:
		left, right = e.Left, e.Right
	case *checker.FloatMultiplication:
		left, right = e.Left, e.Right
	case *checker.FloatDivision:
		left, right = e.Left, e.Right
	case *checker.FloatGreater:
		left, right = e.Left, e.Right
	case *checker.FloatGreaterEqual:
		left, right = e.Left, e.Right
	case *checker.FloatLess:
		left, right = e.Left, e.Right
	case *checker.FloatLessEqual:
		left, right = e.Left, e.Right
	case *checker.StrAddition:
		left, right = e.Left, e.Right
	case *checker.Equality:
		left, right = e.Left, e.Right
	case *checker.And:
		left, right = e.Left, e.Right
	case *checker.Or:
		left, right = e.Left, e.Right
	default:
		return fmt.Errorf("unsupported binary expression: %T", expr)
	}

	if err := f.emitExpr(left); err != nil {
		return err
	}
	if err := f.emitExpr(right); err != nil {
		return err
	}
	f.emit(Instruction{Op: op})
	return nil
}

func (f *funcEmitter) emitBlockExpr(block *checker.Block) error {
	return f.emitStatements(block.Stmts)
}

func (f *funcEmitter) emitIfExpr(expr *checker.If) error {
	if err := f.emitExpr(expr.Condition); err != nil {
		return err
	}
	elseJump := f.emitJump(OpJumpIfFalse)
	if err := f.emitBlockExpr(expr.Body); err != nil {
		return err
	}
	endJump := f.emitJump(OpJump)
	elseLabel := len(f.code)
	f.patchJump(elseJump, elseLabel)
	if expr.ElseIf != nil {
		if err := f.emitIfExpr(expr.ElseIf); err != nil {
			return err
		}
	} else if expr.Else != nil {
		if err := f.emitBlockExpr(expr.Else); err != nil {
			return err
		}
	} else {
		f.emit(Instruction{Op: OpConstVoid})
	}
	endLabel := len(f.code)
	f.patchJump(endJump, endLabel)
	return nil
}

func (f *funcEmitter) emitWhileLoop(loop *checker.WhileLoop) error {
	start := len(f.code)
	if err := f.emitExpr(loop.Condition); err != nil {
		return err
	}
	endJump := f.emitJump(OpJumpIfFalse)
	f.pushBreakList()
	if err := f.emitStatementsDiscard(loop.Body.Stmts); err != nil {
		return err
	}
	breaks := f.popBreakList()
	f.emit(Instruction{Op: OpJump, A: start})
	end := len(f.code)
	f.patchJump(endJump, end)
	f.patchBreaks(breaks, end)
	return nil
}

func (f *funcEmitter) emitForIntRange(loop *checker.ForIntRange) error {
	if err := f.emitExpr(loop.Start); err != nil {
		return err
	}
	cursorIndex := f.localIndex(loop.Cursor)
	f.emit(Instruction{Op: OpStoreLocal, A: cursorIndex})

	endTemp := f.tempLocal()
	if err := f.emitExpr(loop.End); err != nil {
		return err
	}
	endIndex := f.localIndex(endTemp)
	f.emit(Instruction{Op: OpStoreLocal, A: endIndex})

	indexIndex := -1
	if loop.Index != "" {
		f.emit(Instruction{Op: OpConstInt, Imm: 0})
		indexIndex = f.localIndex(loop.Index)
		f.emit(Instruction{Op: OpStoreLocal, A: indexIndex})
	}

	start := len(f.code)
	f.emit(Instruction{Op: OpLoadLocal, A: cursorIndex})
	f.emit(Instruction{Op: OpLoadLocal, A: endIndex})
	f.emit(Instruction{Op: OpLte})
	endJump := f.emitJump(OpJumpIfFalse)

	f.pushBreakList()
	if err := f.emitStatementsDiscard(loop.Body.Stmts); err != nil {
		return err
	}
	breaks := f.popBreakList()

	f.emit(Instruction{Op: OpLoadLocal, A: cursorIndex})
	f.emit(Instruction{Op: OpConstInt, Imm: 1})
	f.emit(Instruction{Op: OpAdd})
	f.emit(Instruction{Op: OpStoreLocal, A: cursorIndex})
	if indexIndex != -1 {
		f.emit(Instruction{Op: OpLoadLocal, A: indexIndex})
		f.emit(Instruction{Op: OpConstInt, Imm: 1})
		f.emit(Instruction{Op: OpAdd})
		f.emit(Instruction{Op: OpStoreLocal, A: indexIndex})
	}

	f.emit(Instruction{Op: OpJump, A: start})
	end := len(f.code)
	f.patchJump(endJump, end)
	f.patchBreaks(breaks, end)
	return nil
}

func (f *funcEmitter) emitForInList(loop *checker.ForInList) error {
	listTemp := f.tempLocal()
	if err := f.emitExpr(loop.List); err != nil {
		return err
	}
	listIndex := f.localIndex(listTemp)
	f.emit(Instruction{Op: OpStoreLocal, A: listIndex})

	indexName := loop.Index
	if indexName == "" {
		indexName = f.tempLocal()
	}
	f.emit(Instruction{Op: OpConstInt, Imm: 0})
	indexIndex := f.localIndex(indexName)
	f.emit(Instruction{Op: OpStoreLocal, A: indexIndex})

	start := len(f.code)
	f.emit(Instruction{Op: OpLoadLocal, A: indexIndex})
	f.emit(Instruction{Op: OpLoadLocal, A: listIndex})
	f.emit(Instruction{Op: OpListLen})
	f.emit(Instruction{Op: OpLt})
	endJump := f.emitJump(OpJumpIfFalse)

	// cursor = list[index]
	f.emit(Instruction{Op: OpLoadLocal, A: listIndex})
	f.emit(Instruction{Op: OpLoadLocal, A: indexIndex})
	f.emit(Instruction{Op: OpListGet})
	cursorIndex := f.localIndex(loop.Cursor)
	f.emit(Instruction{Op: OpStoreLocal, A: cursorIndex})
	if loop.Index != "" {
		f.emit(Instruction{Op: OpLoadLocal, A: indexIndex})
		f.emit(Instruction{Op: OpStoreLocal, A: f.localIndex(loop.Index)})
	}

	f.pushBreakList()
	if err := f.emitStatementsDiscard(loop.Body.Stmts); err != nil {
		return err
	}
	breaks := f.popBreakList()

	f.emit(Instruction{Op: OpLoadLocal, A: indexIndex})
	f.emit(Instruction{Op: OpConstInt, Imm: 1})
	f.emit(Instruction{Op: OpAdd})
	f.emit(Instruction{Op: OpStoreLocal, A: indexIndex})

	f.emit(Instruction{Op: OpJump, A: start})
	end := len(f.code)
	f.patchJump(endJump, end)
	f.patchBreaks(breaks, end)
	return nil
}

func (f *funcEmitter) emitForInMap(loop *checker.ForInMap) error {
	mapTemp := f.tempLocal()
	if err := f.emitExpr(loop.Map); err != nil {
		return err
	}
	mapIndex := f.localIndex(mapTemp)
	f.emit(Instruction{Op: OpStoreLocal, A: mapIndex})

	keysTemp := f.tempLocal()
	f.emit(Instruction{Op: OpLoadLocal, A: mapIndex})
	f.emit(Instruction{Op: OpMapKeys})
	keysIndex := f.localIndex(keysTemp)
	f.emit(Instruction{Op: OpStoreLocal, A: keysIndex})

	indexTemp := f.tempLocal()
	f.emit(Instruction{Op: OpConstInt, Imm: 0})
	indexIndex := f.localIndex(indexTemp)
	f.emit(Instruction{Op: OpStoreLocal, A: indexIndex})

	start := len(f.code)
	f.emit(Instruction{Op: OpLoadLocal, A: indexIndex})
	f.emit(Instruction{Op: OpLoadLocal, A: keysIndex})
	f.emit(Instruction{Op: OpListLen})
	f.emit(Instruction{Op: OpLt})
	endJump := f.emitJump(OpJumpIfFalse)

	// key = keys[index]
	f.emit(Instruction{Op: OpLoadLocal, A: keysIndex})
	f.emit(Instruction{Op: OpLoadLocal, A: indexIndex})
	f.emit(Instruction{Op: OpListGet})
	keyIndex := f.localIndex(loop.Key)
	f.emit(Instruction{Op: OpStoreLocal, A: keyIndex})

	// val = map[key]
	f.emit(Instruction{Op: OpLoadLocal, A: mapIndex})
	f.emit(Instruction{Op: OpLoadLocal, A: keyIndex})
	f.emit(Instruction{Op: OpMapGetValue})
	valIndex := f.localIndex(loop.Val)
	f.emit(Instruction{Op: OpStoreLocal, A: valIndex})

	f.pushBreakList()
	if err := f.emitStatementsDiscard(loop.Body.Stmts); err != nil {
		return err
	}
	breaks := f.popBreakList()

	f.emit(Instruction{Op: OpLoadLocal, A: indexIndex})
	f.emit(Instruction{Op: OpConstInt, Imm: 1})
	f.emit(Instruction{Op: OpAdd})
	f.emit(Instruction{Op: OpStoreLocal, A: indexIndex})

	f.emit(Instruction{Op: OpJump, A: start})
	end := len(f.code)
	f.patchJump(endJump, end)
	f.patchBreaks(breaks, end)
	return nil
}

func (f *funcEmitter) emit(inst Instruction) {
	f.code = append(f.code, inst)
	if effect := inst.Op.StackEffect(); effect.Pop > 0 || effect.Push > 0 {
		f.stack = f.stack - effect.Pop + effect.Push
		if f.stack > f.maxStack {
			f.maxStack = f.stack
		}
	}
}

func (f *funcEmitter) adjustStack(pop, push int) {
	f.stack = f.stack - pop + push
	if f.stack > f.maxStack {
		f.maxStack = f.stack
	}
}

func (f *funcEmitter) emitJump(op Opcode) int {
	idx := len(f.code)
	f.emit(Instruction{Op: op, A: -1})
	return idx
}

func (f *funcEmitter) patchJump(index int, target int) {
	if index < 0 || index >= len(f.code) {
		return
	}
	f.code[index].A = target
}

func (f *funcEmitter) patchBreaks(breaks []int, target int) {
	for _, idx := range breaks {
		f.patchJump(idx, target)
	}
}

func (f *funcEmitter) pushBreakList() {
	f.breakStack = append(f.breakStack, []int{})
}

func (f *funcEmitter) popBreakList() []int {
	if len(f.breakStack) == 0 {
		return nil
	}
	idx := len(f.breakStack) - 1
	list := f.breakStack[idx]
	f.breakStack = f.breakStack[:idx]
	return list
}

func (f *funcEmitter) addBreakJump(index int) error {
	if len(f.breakStack) == 0 {
		return fmt.Errorf("break used outside of loop")
	}
	idx := len(f.breakStack) - 1
	f.breakStack[idx] = append(f.breakStack[idx], index)
	return nil
}

func (f *funcEmitter) tempLocal() string {
	name := fmt.Sprintf("@tmp%d", f.tempIndex)
	f.tempIndex++
	return name
}

func (f *funcEmitter) localIndex(name string) int {
	if idx, ok := f.locals[name]; ok {
		return idx
	}
	idx := f.localTop
	f.locals[name] = idx
	f.localTop++
	return idx
}

func (f *funcEmitter) defineLocal(name string) int {
	return f.localIndex(name)
}

func (f *funcEmitter) localLookup(name string) (int, bool) {
	idx, ok := f.locals[name]
	return idx, ok
}

func (f *funcEmitter) hasLocalInChain(name string) bool {
	if _, ok := f.locals[name]; ok {
		return true
	}
	if f.parent != nil {
		return f.parent.hasLocalInChain(name)
	}
	return false
}

func (f *funcEmitter) captureLocal(name string) int {
	if idx, ok := f.captureIdx[name]; ok {
		return idx
	}
	idx := f.localIndex(name)
	f.captureIdx[name] = idx
	f.captures = append(f.captures, name)
	f.capLocals = append(f.capLocals, idx)
	return idx
}

func (f *funcEmitter) resolveLocal(name string) (int, error) {
	if idx, ok := f.locals[name]; ok {
		return idx, nil
	}
	if f.parent != nil && f.parent.hasLocalInChain(name) {
		return f.captureLocal(name), nil
	}
	return 0, fmt.Errorf("unknown local: %s", name)
}

func (f *funcEmitter) resolveTargetLocal(expr checker.Expression) (int, error) {
	switch e := expr.(type) {
	case *checker.Variable:
		return f.resolveLocal(e.Name())
	case *checker.Identifier:
		return f.resolveLocal(e.Name)
	default:
		return 0, fmt.Errorf("unsupported reassignment target: %T", expr)
	}
}

func (f *funcEmitter) ensureReturn() {
	if len(f.code) == 0 {
		f.emit(Instruction{Op: OpConstVoid})
		f.emit(Instruction{Op: OpReturn})
		return
	}
	last := f.code[len(f.code)-1]
	if last.Op == OpReturn {
		return
	}
	f.emit(Instruction{Op: OpReturn})
}

func (f *funcEmitter) emitFunctionCall(call *checker.FunctionCall) error {
	argc := len(call.Args)
	if call.ExternalBinding != "" {
		for i := range call.Args {
			if err := f.emitExpr(call.Args[i]); err != nil {
				return err
			}
		}
		bindingIdx := f.emitter.addConst(Constant{Kind: ConstStr, Str: call.ExternalBinding})
		retID := f.emitter.addType(call.ReturnType)
		f.emit(Instruction{Op: OpCallExtern, A: bindingIdx, Imm: argc, C: int(retID)})
		f.adjustStack(argc, 1)
		return nil
	}
	if _, ok := f.locals[call.Name]; ok || (f.parent != nil && f.parent.hasLocalInChain(call.Name)) {
		idx, err := f.resolveLocal(call.Name)
		if err != nil {
			return err
		}
		f.emit(Instruction{Op: OpLoadLocal, A: idx})
		for i := range call.Args {
			if err := f.emitExpr(call.Args[i]); err != nil {
				return err
			}
		}
		f.emit(Instruction{Op: OpCallClosure, B: argc})
		f.adjustStack(argc+1, 1)
		return nil
	}
	qualified := f.emitter.qualifyName(call.Name)
	idx, ok := f.emitter.funcIndex[qualified]
	if !ok {
		return fmt.Errorf("unknown function: %s", call.Name)
	}
	for i := range call.Args {
		if err := f.emitExpr(call.Args[i]); err != nil {
			return err
		}
	}
	f.emit(Instruction{Op: OpCall, A: idx, B: argc})
	return nil
}

func (f *funcEmitter) emitModuleFunctionCall(call *checker.ModuleFunctionCall) error {
	argc := len(call.Call.Args)
	if call.Call.ExternalBinding != "" {
		for i := range call.Call.Args {
			if err := f.emitExpr(call.Call.Args[i]); err != nil {
				return err
			}
		}
		bindingIdx := f.emitter.addConst(Constant{Kind: ConstStr, Str: call.Call.ExternalBinding})
		retID := f.emitter.addType(call.Call.ReturnType)
		f.emit(Instruction{Op: OpCallExtern, A: bindingIdx, Imm: argc, C: int(retID)})
		f.adjustStack(argc, 1)
		return nil
	}
	qualified := fmt.Sprintf("%s::%s", call.Module, call.Call.Name)
	if idx, ok := f.emitter.funcIndex[qualified]; ok {
		for i := range call.Call.Args {
			if err := f.emitExpr(call.Call.Args[i]); err != nil {
				return err
			}
		}
		f.emit(Instruction{Op: OpCall, A: idx, B: argc})
		return nil
	}
	if f.emitter.modules != nil {
		if mod, ok := f.emitter.modules[call.Module]; ok {
			if err := f.emitter.emitModule(mod); err != nil {
				return err
			}
			if idx, ok := f.emitter.funcIndex[qualified]; ok {
				for i := range call.Call.Args {
					if err := f.emitExpr(call.Call.Args[i]); err != nil {
						return err
					}
				}
				f.emit(Instruction{Op: OpCall, A: idx, B: argc})
				return nil
			}
		}
	}
	for i := range call.Call.Args {
		if err := f.emitExpr(call.Call.Args[i]); err != nil {
			return err
		}
	}
	modIdx := f.emitter.addConst(Constant{Kind: ConstStr, Str: call.Module})
	fnIdx := f.emitter.addConst(Constant{Kind: ConstStr, Str: call.Call.Name})
	retID := f.emitter.addType(call.Call.ReturnType)
	f.emit(Instruction{Op: OpCallModule, A: modIdx, B: fnIdx, Imm: argc, C: int(retID)})
	f.adjustStack(argc, 1)
	return nil
}

func (f *funcEmitter) emitListLiteral(lit *checker.ListLiteral) error {
	for i := range lit.Elements {
		if err := f.emitExpr(lit.Elements[i]); err != nil {
			return err
		}
	}
	typeID := f.emitter.addType(lit.ListType)
	count := len(lit.Elements)
	f.emit(Instruction{Op: OpMakeList, A: int(typeID), B: count})
	f.stack = f.stack - count + 1
	if f.stack > f.maxStack {
		f.maxStack = f.stack
	}
	return nil
}

func (f *funcEmitter) emitMapLiteral(lit *checker.MapLiteral) error {
	if len(lit.Keys) != len(lit.Values) {
		return fmt.Errorf("map literal keys/values mismatch")
	}
	for i := range lit.Keys {
		if err := f.emitExpr(lit.Keys[i]); err != nil {
			return err
		}
		if err := f.emitExpr(lit.Values[i]); err != nil {
			return err
		}
	}
	typeID := f.emitter.addType(lit.Type())
	count := len(lit.Keys)
	f.emit(Instruction{Op: OpMakeMap, A: int(typeID), B: count})
	f.stack = f.stack - (count * 2) + 1
	if f.stack > f.maxStack {
		f.maxStack = f.stack
	}
	return nil
}

func (f *funcEmitter) emitListMethod(method *checker.ListMethod) error {
	if err := f.emitExpr(method.Subject); err != nil {
		return err
	}
	switch method.Kind {
	case checker.ListSize:
		f.emit(Instruction{Op: OpListLen})
		return nil
	case checker.ListAt:
		if err := f.emitExpr(method.Args[0]); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpListGet})
		return nil
	case checker.ListPush:
		if err := f.emitExpr(method.Args[0]); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpListPush})
		return nil
	case checker.ListPrepend:
		if err := f.emitExpr(method.Args[0]); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpListPrepend})
		return nil
	case checker.ListSet:
		if err := f.emitExpr(method.Args[0]); err != nil {
			return err
		}
		if err := f.emitExpr(method.Args[1]); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpListSet})
		return nil
	default:
		return fmt.Errorf("unsupported list method: %v", method.Kind)
	}
}

func (f *funcEmitter) emitMapMethod(method *checker.MapMethod) error {
	if err := f.emitExpr(method.Subject); err != nil {
		return err
	}
	switch method.Kind {
	case checker.MapKeys:
		f.emit(Instruction{Op: OpMapKeys})
		return nil
	case checker.MapSize:
		f.emit(Instruction{Op: OpMapSize})
		return nil
	case checker.MapGet:
		if err := f.emitExpr(method.Args[0]); err != nil {
			return err
		}
		mapType := checker.MakeMap(method.KeyType, method.ValueType)
		f.emit(Instruction{Op: OpMapGet, A: int(f.emitter.addType(mapType))})
		return nil
	case checker.MapSet:
		if err := f.emitExpr(method.Args[0]); err != nil {
			return err
		}
		if err := f.emitExpr(method.Args[1]); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpMapSet})
		return nil
	case checker.MapDrop:
		if err := f.emitExpr(method.Args[0]); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpMapDrop})
		return nil
	case checker.MapHas:
		if err := f.emitExpr(method.Args[0]); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpMapHas})
		return nil
	default:
		return fmt.Errorf("unsupported map method: %v", method.Kind)
	}
}

func (f *funcEmitter) emitStrMethod(method *checker.StrMethod) error {
	if err := f.emitExpr(method.Subject); err != nil {
		return err
	}
	for i := range method.Args {
		if err := f.emitExpr(method.Args[i]); err != nil {
			return err
		}
	}
	f.emit(Instruction{Op: OpStrMethod, A: int(method.Kind), B: len(method.Args)})
	f.adjustStack(len(method.Args)+1, 1)
	return nil
}

func (f *funcEmitter) emitIntMethod(method *checker.IntMethod) error {
	if err := f.emitExpr(method.Subject); err != nil {
		return err
	}
	f.emit(Instruction{Op: OpIntMethod, A: int(method.Kind)})
	f.adjustStack(1, 1)
	return nil
}

func (f *funcEmitter) emitFloatMethod(method *checker.FloatMethod) error {
	if err := f.emitExpr(method.Subject); err != nil {
		return err
	}
	f.emit(Instruction{Op: OpFloatMethod, A: int(method.Kind)})
	f.adjustStack(1, 1)
	return nil
}

func (f *funcEmitter) emitBoolMethod(method *checker.BoolMethod) error {
	if err := f.emitExpr(method.Subject); err != nil {
		return err
	}
	f.emit(Instruction{Op: OpBoolMethod, A: int(method.Kind)})
	f.adjustStack(1, 1)
	return nil
}

func (f *funcEmitter) emitMaybeMethod(method *checker.MaybeMethod) error {
	if err := f.emitExpr(method.Subject); err != nil {
		return err
	}
	for i := range method.Args {
		if err := f.emitExpr(method.Args[i]); err != nil {
			return err
		}
	}
	retID := f.emitter.addType(method.ReturnType)
	f.emit(Instruction{Op: OpMaybeMethod, A: int(method.Kind), B: len(method.Args), Imm: int(retID)})
	f.adjustStack(len(method.Args)+1, 1)
	return nil
}

func (f *funcEmitter) emitResultMethod(method *checker.ResultMethod) error {
	if err := f.emitExpr(method.Subject); err != nil {
		return err
	}
	for i := range method.Args {
		if err := f.emitExpr(method.Args[i]); err != nil {
			return err
		}
	}
	retID := f.emitter.addType(method.ReturnType)
	f.emit(Instruction{Op: OpResultMethod, A: int(method.Kind), B: len(method.Args), Imm: int(retID)})
	f.adjustStack(len(method.Args)+1, 1)
	return nil
}

func (f *funcEmitter) emitBoolMatch(match *checker.BoolMatch) error {
	if err := f.emitExpr(match.Subject); err != nil {
		return err
	}
	condTemp := f.tempLocal()
	condIndex := f.localIndex(condTemp)
	f.emit(Instruction{Op: OpStoreLocal, A: condIndex})
	f.emit(Instruction{Op: OpLoadLocal, A: condIndex})
	falseJump := f.emitJump(OpJumpIfFalse)
	if err := f.emitBlockExpr(match.True); err != nil {
		return err
	}
	endJump := f.emitJump(OpJump)
	falseLabel := len(f.code)
	f.patchJump(falseJump, falseLabel)
	if err := f.emitBlockExpr(match.False); err != nil {
		return err
	}
	endLabel := len(f.code)
	f.patchJump(endJump, endLabel)
	return nil
}

func (f *funcEmitter) emitIntMatch(match *checker.IntMatch) error {
	if err := f.emitExpr(match.Subject); err != nil {
		return err
	}
	subjectTemp := f.tempLocal()
	subjectIndex := f.localIndex(subjectTemp)
	f.emit(Instruction{Op: OpStoreLocal, A: subjectIndex})

	caseJumps := []int{}

	intKeys := make([]int, 0, len(match.IntCases))
	for key := range match.IntCases {
		intKeys = append(intKeys, key)
	}
	if len(intKeys) > 1 {
		slices.Sort(intKeys)
	}
	for _, key := range intKeys {
		caseBlock := match.IntCases[key]
		if caseBlock == nil {
			continue
		}
		f.emit(Instruction{Op: OpLoadLocal, A: subjectIndex})
		f.emit(Instruction{Op: OpConstInt, Imm: key})
		f.emit(Instruction{Op: OpEq})
		caseJump := f.emitJump(OpJumpIfFalse)
		if err := f.emitBlockExpr(caseBlock); err != nil {
			return err
		}
		caseJumps = append(caseJumps, f.emitJump(OpJump))
		f.patchJump(caseJump, len(f.code))
	}

	ranges := make([]checker.IntRange, 0, len(match.RangeCases))
	for r := range match.RangeCases {
		ranges = append(ranges, r)
	}
	if len(ranges) > 1 {
		slices.SortFunc(ranges, func(a, b checker.IntRange) int {
			if a.Start != b.Start {
				return a.Start - b.Start
			}
			return a.End - b.End
		})
	}
	for _, r := range ranges {
		caseBlock := match.RangeCases[r]
		if caseBlock == nil {
			continue
		}
		f.emit(Instruction{Op: OpLoadLocal, A: subjectIndex})
		f.emit(Instruction{Op: OpConstInt, Imm: r.Start})
		f.emit(Instruction{Op: OpGte})
		f.emit(Instruction{Op: OpLoadLocal, A: subjectIndex})
		f.emit(Instruction{Op: OpConstInt, Imm: r.End})
		f.emit(Instruction{Op: OpLte})
		f.emit(Instruction{Op: OpAnd})
		caseJump := f.emitJump(OpJumpIfFalse)
		if err := f.emitBlockExpr(caseBlock); err != nil {
			return err
		}
		caseJumps = append(caseJumps, f.emitJump(OpJump))
		f.patchJump(caseJump, len(f.code))
	}

	if match.CatchAll != nil {
		if err := f.emitBlockExpr(match.CatchAll); err != nil {
			return err
		}
	} else {
		f.emit(Instruction{Op: OpConstVoid})
	}

	endLabel := len(f.code)
	for _, j := range caseJumps {
		f.patchJump(j, endLabel)
	}
	return nil
}

func (f *funcEmitter) emitOptionMatch(match *checker.OptionMatch) error {
	if err := f.emitExpr(match.Subject); err != nil {
		return err
	}
	subjectTemp := f.tempLocal()
	subjectIndex := f.localIndex(subjectTemp)
	f.emit(Instruction{Op: OpStoreLocal, A: subjectIndex})

	f.emit(Instruction{Op: OpLoadLocal, A: subjectIndex})
	retID := f.emitter.addType(checker.Bool)
	f.emit(Instruction{Op: OpMaybeMethod, A: int(checker.MaybeIsNone), B: 0, Imm: int(retID)})
	falseJump := f.emitJump(OpJumpIfFalse)
	if err := f.emitBlockExpr(match.None); err != nil {
		return err
	}
	endJump := f.emitJump(OpJump)

	falseLabel := len(f.code)
	f.patchJump(falseJump, falseLabel)
	if match.Some != nil {
		f.emit(Instruction{Op: OpLoadLocal, A: subjectIndex})
		innerID := f.emitter.addType(match.InnerType)
		f.emit(Instruction{Op: OpMaybeUnwrap, A: int(innerID)})
		f.emit(Instruction{Op: OpStoreLocal, A: f.localIndex(match.Some.Pattern.Name)})
		if err := f.emitBlockExpr(match.Some.Body); err != nil {
			return err
		}
	} else {
		f.emit(Instruction{Op: OpConstVoid})
	}
	endLabel := len(f.code)
	f.patchJump(endJump, endLabel)
	return nil
}

func (f *funcEmitter) emitResultMatch(match *checker.ResultMatch) error {
	if err := f.emitExpr(match.Subject); err != nil {
		return err
	}
	subjectTemp := f.tempLocal()
	subjectIndex := f.localIndex(subjectTemp)
	f.emit(Instruction{Op: OpStoreLocal, A: subjectIndex})

	f.emit(Instruction{Op: OpLoadLocal, A: subjectIndex})
	retID := f.emitter.addType(checker.Bool)
	f.emit(Instruction{Op: OpResultMethod, A: int(checker.ResultIsErr), B: 0, Imm: int(retID)})
	errJump := f.emitJump(OpJumpIfFalse)

	if match.Err != nil {
		f.emit(Instruction{Op: OpLoadLocal, A: subjectIndex})
		errTypeID := f.emitter.addType(match.ErrType)
		f.emit(Instruction{Op: OpResultUnwrap, A: int(errTypeID)})
		f.emit(Instruction{Op: OpStoreLocal, A: f.localIndex(match.Err.Pattern.Name)})
		if err := f.emitBlockExpr(match.Err.Body); err != nil {
			return err
		}
	} else {
		f.emit(Instruction{Op: OpConstVoid})
	}
	endJump := f.emitJump(OpJump)

	falseLabel := len(f.code)
	f.patchJump(errJump, falseLabel)
	if match.Ok != nil {
		f.emit(Instruction{Op: OpLoadLocal, A: subjectIndex})
		okTypeID := f.emitter.addType(match.OkType)
		f.emit(Instruction{Op: OpResultUnwrap, A: int(okTypeID)})
		f.emit(Instruction{Op: OpStoreLocal, A: f.localIndex(match.Ok.Pattern.Name)})
		if err := f.emitBlockExpr(match.Ok.Body); err != nil {
			return err
		}
	} else {
		f.emit(Instruction{Op: OpConstVoid})
	}
	endLabel := len(f.code)
	f.patchJump(endJump, endLabel)
	return nil
}

func (f *funcEmitter) emitConditionalMatch(match *checker.ConditionalMatch) error {
	caseJumps := []int{}
	for _, c := range match.Cases {
		if err := f.emitExpr(c.Condition); err != nil {
			return err
		}
		caseJump := f.emitJump(OpJumpIfFalse)
		if err := f.emitBlockExpr(c.Body); err != nil {
			return err
		}
		caseJumps = append(caseJumps, f.emitJump(OpJump))
		f.patchJump(caseJump, len(f.code))
	}
	if match.CatchAll != nil {
		if err := f.emitBlockExpr(match.CatchAll); err != nil {
			return err
		}
	} else {
		f.emit(Instruction{Op: OpConstVoid})
	}
	endLabel := len(f.code)
	for _, j := range caseJumps {
		f.patchJump(j, endLabel)
	}
	return nil
}

func (f *funcEmitter) emitEnumMatch(match *checker.EnumMatch) error {
	if err := f.emitExpr(match.Subject); err != nil {
		return err
	}
	subjectTemp := f.tempLocal()
	subjectIndex := f.localIndex(subjectTemp)
	f.emit(Instruction{Op: OpStoreLocal, A: subjectIndex})

	caseJumps := []int{}
	discriminants := make([]int, 0, len(match.DiscriminantToIndex))
	for disc := range match.DiscriminantToIndex {
		discriminants = append(discriminants, disc)
	}
	if len(discriminants) > 1 {
		slices.Sort(discriminants)
	}
	for _, disc := range discriminants {
		idx := match.DiscriminantToIndex[disc]
		if int(idx) < 0 || int(idx) >= len(match.Cases) {
			continue
		}
		caseBlock := match.Cases[idx]
		if caseBlock == nil {
			continue
		}
		f.emit(Instruction{Op: OpLoadLocal, A: subjectIndex})
		f.emit(Instruction{Op: OpConstInt, Imm: disc})
		f.emit(Instruction{Op: OpEq})
		caseJump := f.emitJump(OpJumpIfFalse)
		if err := f.emitBlockExpr(caseBlock); err != nil {
			return err
		}
		caseJumps = append(caseJumps, f.emitJump(OpJump))
		f.patchJump(caseJump, len(f.code))
	}
	if match.CatchAll != nil {
		if err := f.emitBlockExpr(match.CatchAll); err != nil {
			return err
		}
	} else {
		f.emit(Instruction{Op: OpConstVoid})
	}
	endLabel := len(f.code)
	for _, j := range caseJumps {
		f.patchJump(j, endLabel)
	}
	return nil
}

func (f *funcEmitter) emitUnionMatch(match *checker.UnionMatch) error {
	if err := f.emitExpr(match.Subject); err != nil {
		return err
	}
	subjectTemp := f.tempLocal()
	subjectIndex := f.localIndex(subjectTemp)
	f.emit(Instruction{Op: OpStoreLocal, A: subjectIndex})

	typeTemp := f.tempLocal()
	f.emit(Instruction{Op: OpLoadLocal, A: subjectIndex})
	f.emit(Instruction{Op: OpTypeName})
	typeIndex := f.localIndex(typeTemp)
	f.emit(Instruction{Op: OpStoreLocal, A: typeIndex})

	caseJumps := []int{}

	keys := make([]string, 0, len(match.TypeCases))
	for key := range match.TypeCases {
		keys = append(keys, key)
	}
	if len(keys) > 1 {
		slices.Sort(keys)
	}
	for _, key := range keys {
		caseMatch := match.TypeCases[key]
		if caseMatch == nil {
			continue
		}
		idx := f.emitter.addConst(Constant{Kind: ConstStr, Str: key})
		f.emit(Instruction{Op: OpLoadLocal, A: typeIndex})
		f.emit(Instruction{Op: OpConst, A: idx})
		f.emit(Instruction{Op: OpEq})
		caseJump := f.emitJump(OpJumpIfFalse)
		f.emit(Instruction{Op: OpLoadLocal, A: subjectIndex})
		f.emit(Instruction{Op: OpStoreLocal, A: f.localIndex(caseMatch.Pattern.Name)})
		if err := f.emitBlockExpr(caseMatch.Body); err != nil {
			return err
		}
		caseJumps = append(caseJumps, f.emitJump(OpJump))
		f.patchJump(caseJump, len(f.code))
	}

	if match.CatchAll != nil {
		if err := f.emitBlockExpr(match.CatchAll); err != nil {
			return err
		}
	} else {
		f.emit(Instruction{Op: OpConstVoid})
	}
	endLabel := len(f.code)
	for _, j := range caseJumps {
		f.patchJump(j, endLabel)
	}
	return nil
}

func (f *funcEmitter) emitTryOp(op *checker.TryOp) error {
	if err := f.emitExpr(op.Expr()); err != nil {
		return err
	}

	okTypeID := f.emitter.addType(op.OkType)
	errTypeID := f.emitter.addType(op.ErrType)
	catchIndex := -1
	if op.CatchVar != "" {
		catchIndex = f.localIndex(op.CatchVar)
	}
	catchJump := f.emitJump(OpJump)
	tryOp := Instruction{Op: OpTryResult, A: -1, B: catchIndex, Imm: int(okTypeID), C: int(errTypeID)}
	if op.Kind == checker.TryMaybe {
		tryOp.Op = OpTryMaybe
		tryOp.C = 0
	}
	// replace jump with try opcode
	f.code[catchJump] = tryOp

	if op.CatchBlock != nil {
		endJump := f.emitJump(OpJump)
		catchLabel := len(f.code)
		// patch try opcode with catch label
		f.code[catchJump].A = catchLabel
		if err := f.emitBlockExpr(op.CatchBlock); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpReturn})
		endLabel := len(f.code)
		f.patchJump(endJump, endLabel)
	} else {
		f.code[catchJump].A = -1
	}
	return nil
}

func (f *funcEmitter) emitModuleSymbol(sym *checker.ModuleSymbol) error {
	name := fmt.Sprintf("%s::%s", sym.Module, sym.Symbol.Name)
	switch def := sym.Symbol.Type.(type) {
	case *checker.FunctionDef:
		idx, ok := f.emitter.funcIndex[name]
		if !ok {
			return fmt.Errorf("unknown module function: %s", name)
		}
		typeID := f.emitter.addType(def)
		f.emit(Instruction{Op: OpMakeClosure, A: idx, B: 0, C: int(typeID)})
		f.adjustStack(0, 1)
		return nil
	case *checker.ExternalFunctionDef:
		idx, err := f.emitter.emitExternWrapper(name, def)
		if err != nil {
			return err
		}
		typeID := f.emitter.addType(def)
		f.emit(Instruction{Op: OpMakeClosure, A: idx, B: 0, C: int(typeID)})
		f.adjustStack(0, 1)
		return nil
	default:
		return fmt.Errorf("unsupported module symbol type: %T", sym.Symbol.Type)
	}
}

func (f *funcEmitter) emitFiberExecution(exec *checker.FiberExecution) error {
	mod := exec.GetModule()
	if mod == nil {
		return fmt.Errorf("missing fiber module")
	}
	prog := mod.Program()
	if prog == nil {
		return fmt.Errorf("missing fiber program")
	}
	var target *checker.FunctionDef
	for i := range prog.Statements {
		stmt := prog.Statements[i]
		if def, ok := stmt.Expr.(*checker.FunctionDef); ok {
			if def.Name == exec.GetMainName() {
				target = def
				break
			}
		}
	}
	if target == nil {
		return fmt.Errorf("fiber function not found: %s", exec.GetMainName())
	}
	copy := *target
	copy.Name = f.emitter.nextAnonName("fiber")
	fnIndex, _, err := f.emitter.emitFunctionWithParent(&copy, nil)
	if err != nil {
		return err
	}
	fnTypeID := f.emitter.addType(copy.Type())
	f.emit(Instruction{Op: OpMakeClosure, A: fnIndex, B: 0, C: int(fnTypeID)})
	fiberTypeID := f.emitter.addType(exec.FiberType)
	f.emit(Instruction{Op: OpAsyncStart, C: int(fiberTypeID)})
	return nil
}

func (f *funcEmitter) emitFiberEval(eval *checker.FiberEval) error {
	if err := f.emitExpr(eval.GetFn()); err != nil {
		return err
	}
	fiberTypeID := f.emitter.addType(eval.FiberType)
	f.emit(Instruction{Op: OpAsyncEval, C: int(fiberTypeID)})
	return nil
}

func (f *funcEmitter) emitStructInstance(inst *checker.StructInstance) error {
	fieldNames := make([]string, 0, len(inst.FieldTypes))
	for name := range inst.FieldTypes {
		fieldNames = append(fieldNames, name)
	}
	if len(fieldNames) > 1 {
		slices.Sort(fieldNames)
	}
	for _, name := range fieldNames {
		nameIdx := f.emitter.addConst(Constant{Kind: ConstStr, Str: name})
		f.emit(Instruction{Op: OpConst, A: nameIdx})
		if expr, ok := inst.Fields[name]; ok {
			if err := f.emitExpr(expr); err != nil {
				return err
			}
		} else {
			fieldType := inst.FieldTypes[name]
			typeID := f.emitter.addType(fieldType)
			f.emit(Instruction{Op: OpMakeNone, A: int(typeID)})
		}
	}
	structType := inst.StructType
	if structType == nil {
		structType = inst.Type()
	}
	structTypeID := f.emitter.addType(structType)
	f.emit(Instruction{Op: OpMakeStruct, A: int(structTypeID), B: len(fieldNames)})
	f.adjustStack(len(fieldNames)*2, 1)
	return nil
}

func (f *funcEmitter) emitModuleStructInstance(inst *checker.ModuleStructInstance) error {
	inner := inst.Property
	if inner == nil {
		return fmt.Errorf("missing struct instance for module struct")
	}
	return f.emitStructInstance(inner)
}

func (f *funcEmitter) emitEnumVariant(variant *checker.EnumVariant) error {
	if variant.EnumType == nil {
		return fmt.Errorf("missing enum type")
	}
	f.emit(Instruction{Op: OpConstInt, Imm: variant.Discriminant})
	var enumType checker.Type = variant.EnumType
	structTypeID := f.emitter.addType(enumType)
	f.emit(Instruction{Op: OpMakeEnum, A: int(structTypeID)})
	f.adjustStack(1, 1)
	return nil
}

func (f *funcEmitter) emitInstanceProperty(prop *checker.InstanceProperty) error {
	if err := f.emitExpr(prop.Subject); err != nil {
		return err
	}
	nameIdx := f.emitter.addConst(Constant{Kind: ConstStr, Str: prop.Property})
	f.emit(Instruction{Op: OpGetField, A: nameIdx})
	f.adjustStack(1, 1)
	return nil
}

func (f *funcEmitter) emitInstanceMethod(method *checker.InstanceMethod) error {
	if err := f.emitExpr(method.Subject); err != nil {
		return err
	}
	for i := range method.Method.Args {
		if err := f.emitExpr(method.Method.Args[i]); err != nil {
			return err
		}
	}
	nameIdx := f.emitter.addConst(Constant{Kind: ConstStr, Str: method.Method.Name})
	f.emit(Instruction{Op: OpCallMethod, A: nameIdx, B: len(method.Method.Args)})
	f.adjustStack(len(method.Method.Args)+1, 1)
	return nil
}
