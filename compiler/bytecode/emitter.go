package bytecode

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

type Emitter struct {
	program   Program
	typeIndex map[string]TypeID
}

type funcEmitter struct {
	emitter  *Emitter
	code     []Instruction
	locals   map[string]int
	localTop int
	stack    int
	maxStack int
}

func NewEmitter() *Emitter {
	return &Emitter{
		program: Program{
			Constants: []Constant{},
			Types:     []TypeEntry{},
			Functions: []Function{},
		},
		typeIndex: map[string]TypeID{},
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

func (f *funcEmitter) emitStatements(stmts []checker.Statement) error {
	for i := range stmts {
		stmt := stmts[i]
		isLast := i == len(stmts)-1
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
	default:
		return fmt.Errorf("unsupported binary expression: %T", expr)
	}

	if err := f.emitExpr(left); err != nil {
		return err
	}
	if err := f.emitExpr(right); err != nil {
		return err
	}
	if op == OpEq {
		f.emit(Instruction{Op: OpEq})
		return nil
	}
	f.emit(Instruction{Op: op})
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
