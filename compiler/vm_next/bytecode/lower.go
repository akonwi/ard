package bytecode

import (
	"fmt"

	"github.com/akonwi/ard/air"
)

func Lower(program *air.Program) (*Program, error) {
	if program == nil {
		return nil, fmt.Errorf("AIR program is nil")
	}
	out := &Program{
		Types:     append([]air.TypeInfo(nil), program.Types...),
		Traits:    append([]air.Trait(nil), program.Traits...),
		Impls:     append([]air.Impl(nil), program.Impls...),
		Externs:   append([]air.Extern(nil), program.Externs...),
		Functions: make([]Function, len(program.Functions)),
		Entry:     program.Entry,
		Script:    program.Script,
	}
	l := lowerer{program: program, out: out}
	for i := range program.Functions {
		fn, err := l.lowerFunction(&program.Functions[i])
		if err != nil {
			return nil, err
		}
		out.Functions[i] = fn
	}
	return out, nil
}

type lowerer struct {
	program *air.Program
	out     *Program
}

type functionLowerer struct {
	root *lowerer
	fn   *air.Function
	code []Instruction
}

func (l *lowerer) lowerFunction(fn *air.Function) (Function, error) {
	fl := &functionLowerer{root: l, fn: fn}
	if err := fl.lowerBlock(fn.Body, fn.Signature.Return); err != nil {
		return Function{}, fmt.Errorf("lower function %s: %w", fn.Name, err)
	}
	fl.emit(Instruction{Op: OpReturn})
	return Function{
		ID:       fn.ID,
		Module:   fn.Module,
		Name:     fn.Name,
		Arity:    len(fn.Signature.Params),
		Return:   fn.Signature.Return,
		Locals:   len(fn.Locals),
		Captures: append([]air.Capture(nil), fn.Captures...),
		Code:     fl.code,
	}, nil
}

func (fl *functionLowerer) lowerBlock(block air.Block, defaultType air.TypeID) error {
	for i := range block.Stmts {
		if err := fl.lowerStmt(&block.Stmts[i]); err != nil {
			return err
		}
	}
	if block.Result == nil {
		return fl.emitZero(defaultType)
	}
	return fl.lowerExpr(block.Result)
}

func (fl *functionLowerer) lowerStmt(stmt *air.Stmt) error {
	switch stmt.Kind {
	case air.StmtLet, air.StmtAssign:
		if err := fl.lowerExpr(stmt.Value); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpStoreLocal, A: int(stmt.Local)})
		return nil
	case air.StmtExpr:
		if err := fl.lowerExpr(stmt.Expr); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpPop})
		return nil
	default:
		return fmt.Errorf("unsupported statement kind %d", stmt.Kind)
	}
}

func (fl *functionLowerer) lowerExpr(expr *air.Expr) error {
	if expr == nil {
		return fl.emitZero(fl.mustTypeID(air.TypeVoid))
	}
	switch expr.Kind {
	case air.ExprConstVoid:
		fl.emit(Instruction{Op: OpConstVoid, A: int(expr.Type)})
	case air.ExprConstInt:
		fl.emit(Instruction{Op: OpConstInt, A: int(expr.Type), Imm: expr.Int})
	case air.ExprConstFloat:
		fl.emit(Instruction{Op: OpConstFloat, A: int(expr.Type), B: fl.addConst(Constant{Kind: ConstFloat, Float: expr.Float})})
	case air.ExprConstBool:
		imm := 0
		if expr.Bool {
			imm = 1
		}
		fl.emit(Instruction{Op: OpConstBool, A: int(expr.Type), Imm: imm})
	case air.ExprConstStr:
		fl.emit(Instruction{Op: OpConstStr, A: int(expr.Type), B: fl.addConst(Constant{Kind: ConstStr, Str: expr.Str})})
	case air.ExprLoadLocal:
		fl.emit(Instruction{Op: OpLoadLocal, A: int(expr.Local)})
	case air.ExprCall:
		for i := range expr.Args {
			if err := fl.lowerExpr(&expr.Args[i]); err != nil {
				return err
			}
		}
		fl.emit(Instruction{Op: OpCall, A: int(expr.Function), B: len(expr.Args)})
	case air.ExprBlock:
		fl.emit(Instruction{Op: OpBlock})
		return fl.lowerBlock(expr.Body, expr.Type)
	case air.ExprIf:
		if err := fl.lowerExpr(expr.Condition); err != nil {
			return err
		}
		jumpFalse := fl.emit(Instruction{Op: OpJumpIfFalse})
		if err := fl.lowerBlock(expr.Then, expr.Type); err != nil {
			return err
		}
		jumpEnd := fl.emit(Instruction{Op: OpJump})
		fl.patch(jumpFalse, len(fl.code))
		if err := fl.lowerBlock(expr.Else, expr.Type); err != nil {
			return err
		}
		fl.patch(jumpEnd, len(fl.code))
	case air.ExprIntAdd, air.ExprIntSub, air.ExprIntMul, air.ExprIntDiv, air.ExprIntMod,
		air.ExprFloatAdd, air.ExprFloatSub, air.ExprFloatMul, air.ExprFloatDiv,
		air.ExprStrConcat, air.ExprEq, air.ExprNotEq, air.ExprLt, air.ExprLte,
		air.ExprGt, air.ExprGte, air.ExprAnd, air.ExprOr:
		return fl.lowerBinary(expr)
	case air.ExprNot, air.ExprNeg, air.ExprToStr:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: unaryOpcode(expr.Kind), A: int(expr.Type)})
	case air.ExprMakeStruct:
		return fmt.Errorf("struct construction bytecode is not implemented yet")
	case air.ExprGetField:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpGetField, A: int(expr.Type), B: expr.Field})
	default:
		return fmt.Errorf("unsupported expression kind %d", expr.Kind)
	}
	return nil
}

func (fl *functionLowerer) lowerBinary(expr *air.Expr) error {
	if err := fl.lowerExpr(expr.Left); err != nil {
		return err
	}
	if err := fl.lowerExpr(expr.Right); err != nil {
		return err
	}
	fl.emit(Instruction{Op: binaryOpcode(expr.Kind), A: int(expr.Type)})
	return nil
}

func binaryOpcode(kind air.ExprKind) Opcode {
	switch kind {
	case air.ExprIntAdd:
		return OpIntAdd
	case air.ExprIntSub:
		return OpIntSub
	case air.ExprIntMul:
		return OpIntMul
	case air.ExprIntDiv:
		return OpIntDiv
	case air.ExprIntMod:
		return OpIntMod
	case air.ExprFloatAdd:
		return OpFloatAdd
	case air.ExprFloatSub:
		return OpFloatSub
	case air.ExprFloatMul:
		return OpFloatMul
	case air.ExprFloatDiv:
		return OpFloatDiv
	case air.ExprStrConcat:
		return OpStrConcat
	case air.ExprEq:
		return OpEq
	case air.ExprNotEq:
		return OpNotEq
	case air.ExprLt:
		return OpLt
	case air.ExprLte:
		return OpLte
	case air.ExprGt:
		return OpGt
	case air.ExprGte:
		return OpGte
	case air.ExprAnd:
		return OpAnd
	case air.ExprOr:
		return OpOr
	default:
		return OpNoop
	}
}

func unaryOpcode(kind air.ExprKind) Opcode {
	switch kind {
	case air.ExprNot:
		return OpNot
	case air.ExprNeg:
		return OpNeg
	case air.ExprToStr:
		return OpToStr
	default:
		return OpNoop
	}
}

func (fl *functionLowerer) emitZero(typeID air.TypeID) error {
	info, ok := fl.root.out.TypeInfo(typeID)
	if !ok {
		return fmt.Errorf("invalid zero type %d", typeID)
	}
	switch info.Kind {
	case air.TypeVoid:
		fl.emit(Instruction{Op: OpConstVoid, A: int(typeID)})
	case air.TypeInt:
		fl.emit(Instruction{Op: OpConstInt, A: int(typeID), Imm: 0})
	case air.TypeFloat:
		fl.emit(Instruction{Op: OpConstFloat, A: int(typeID), B: fl.addConst(Constant{Kind: ConstFloat, Float: 0})})
	case air.TypeBool:
		fl.emit(Instruction{Op: OpConstBool, A: int(typeID), Imm: 0})
	case air.TypeStr:
		fl.emit(Instruction{Op: OpConstStr, A: int(typeID), B: fl.addConst(Constant{Kind: ConstStr, Str: ""})})
	default:
		return fmt.Errorf("unsupported zero value for type %s", info.Name)
	}
	return nil
}

func (fl *functionLowerer) mustTypeID(kind air.TypeKind) air.TypeID {
	for _, typ := range fl.root.out.Types {
		if typ.Kind == kind {
			return typ.ID
		}
	}
	return air.NoType
}

func (fl *functionLowerer) addConst(c Constant) int {
	for i := range fl.root.out.Constants {
		if fl.root.out.Constants[i] == c {
			return i
		}
	}
	index := len(fl.root.out.Constants)
	fl.root.out.Constants = append(fl.root.out.Constants, c)
	return index
}

func (fl *functionLowerer) emit(inst Instruction) int {
	index := len(fl.code)
	fl.code = append(fl.code, inst)
	return index
}

func (fl *functionLowerer) patch(index int, target int) {
	fl.code[index].A = target
}
