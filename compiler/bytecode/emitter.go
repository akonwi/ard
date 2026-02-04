package bytecode

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

type Emitter struct {
	program   Program
	typeIndex map[string]TypeID
	funcIndex map[string]int
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
}

func NewEmitter() *Emitter {
	return &Emitter{
		program: Program{
			Constants: []Constant{},
			Types:     []TypeEntry{},
			Functions: []Function{},
		},
		typeIndex: map[string]TypeID{},
		funcIndex: map[string]int{},
	}
}

func (e *Emitter) EmitProgram(module checker.Module) (Program, error) {
	prog := module.Program()
	if prog == nil {
		return e.program, nil
	}

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
		}
		_ = stmt.Stmt
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
	if def.Name == "" {
		return fmt.Errorf("anonymous functions not yet supported")
	}
	if _, ok := e.funcIndex[def.Name]; ok {
		return nil
	}

	fnEmitter := &funcEmitter{
		emitter: e,
		code:    []Instruction{},
		locals:  map[string]int{},
	}
	for _, param := range def.Parameters {
		fnEmitter.localIndex(param.Name)
	}
	if def.Body != nil {
		if err := fnEmitter.emitStatements(def.Body.Stmts); err != nil {
			return err
		}
	}
	fnEmitter.ensureReturn()

	index := len(e.program.Functions)
	e.funcIndex[def.Name] = index
	e.program.Functions = append(e.program.Functions, Function{
		Name:     def.Name,
		Arity:    len(def.Parameters),
		Locals:   fnEmitter.localTop,
		MaxStack: fnEmitter.maxStack,
		Code:     fnEmitter.code,
	})
	return nil
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
		if err := f.emitExpr(s.Value); err != nil {
			return err
		}
		index, err := f.resolveTargetLocal(s.Target)
		if err != nil {
			return err
		}
		f.emit(Instruction{Op: OpStoreLocal, A: index})
		return nil
	case *checker.WhileLoop:
		return f.emitWhileLoop(s)
	case *checker.ForIntRange:
		return f.emitForIntRange(s)
	case *checker.ForInList:
		return f.emitForInList(s)
	case *checker.ForInMap:
		return f.emitForInMap(s)
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
		index := f.localIndex(e.Name())
		f.emit(Instruction{Op: OpLoadLocal, A: index})
		return nil
	case *checker.Identifier:
		index := f.localIndex(e.Name)
		f.emit(Instruction{Op: OpLoadLocal, A: index})
		return nil
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
		if err := f.emitter.emitFunction(e); err != nil {
			return err
		}
		f.emit(Instruction{Op: OpConstVoid})
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

func (f *funcEmitter) resolveTargetLocal(expr checker.Expression) (int, error) {
	switch e := expr.(type) {
	case *checker.Variable:
		if idx, ok := f.locals[e.Name()]; ok {
			return idx, nil
		}
		return f.localIndex(e.Name()), nil
	case *checker.Identifier:
		if idx, ok := f.locals[e.Name]; ok {
			return idx, nil
		}
		return f.localIndex(e.Name), nil
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
	for i := range call.Args {
		if err := f.emitExpr(call.Args[i]); err != nil {
			return err
		}
	}
	idx, ok := f.emitter.funcIndex[call.Name]
	if !ok {
		return fmt.Errorf("unknown function: %s", call.Name)
	}
	f.emit(Instruction{Op: OpCall, A: idx, B: len(call.Args)})
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
