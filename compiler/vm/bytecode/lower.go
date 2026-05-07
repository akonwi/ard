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
	root       *lowerer
	fn         *air.Function
	code       []Instruction
	breakStack [][]int
	localTop   int
}

func (l *lowerer) lowerFunction(fn *air.Function) (Function, error) {
	fl := &functionLowerer{root: l, fn: fn, localTop: len(fn.Locals)}
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
		Locals:   fl.localTop,
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
	case air.StmtSetField:
		if err := fl.lowerExpr(stmt.Target); err != nil {
			return err
		}
		if err := fl.lowerExpr(stmt.Value); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpSetField, A: int(stmt.Type), B: stmt.Field})
		return nil
	case air.StmtExpr:
		if ok, err := fl.lowerDiscardedExpr(stmt.Expr); ok || err != nil {
			return err
		}
		if err := fl.lowerExpr(stmt.Expr); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpPop})
		return nil
	case air.StmtWhile:
		return fl.lowerWhile(stmt)
	case air.StmtBreak:
		jump := fl.emit(Instruction{Op: OpJump})
		if len(fl.breakStack) == 0 {
			return fmt.Errorf("break outside loop")
		}
		fl.breakStack[len(fl.breakStack)-1] = append(fl.breakStack[len(fl.breakStack)-1], jump)
		return nil
	default:
		return fmt.Errorf("unsupported statement kind %d", stmt.Kind)
	}
}

func (fl *functionLowerer) lowerDiscardedExpr(expr *air.Expr) (bool, error) {
	if expr == nil {
		return false, nil
	}
	if expr.Kind == air.ExprBlock {
		return true, fl.lowerDiscardedBlock(expr.Body)
	}
	if expr.Kind == air.ExprIf {
		return true, fl.lowerDiscardedIf(expr)
	}
	if expr.Kind == air.ExprMatchResult {
		return true, fl.lowerResultMatchDiscarded(expr)
	}
	if expr.Kind == air.ExprListPush && expr.Target != nil && expr.Target.Kind == air.ExprLoadLocal && len(expr.Args) == 1 {
		if err := fl.lowerExpr(&expr.Args[0]); err != nil {
			return true, err
		}
		fl.emit(Instruction{Op: OpListPushLocalDrop, B: int(expr.Target.Local)})
		return true, nil
	}
	if expr.Kind == air.ExprMapSet && expr.Target != nil && expr.Target.Kind == air.ExprLoadLocal && len(expr.Args) == 2 {
		if expr.Args[0].Kind == air.ExprLoadLocal {
			if delta, ok := mapIncrementDelta(expr.Target.Local, expr.Args[0].Local, &expr.Args[1]); ok {
				fl.emit(Instruction{Op: OpMapIncrementIntLocalDrop, B: int(expr.Target.Local), C: int(expr.Args[0].Local), Imm: delta})
				return true, nil
			}
		}
		if deltaExpr, ok := mapIncrementValueDelta(expr.Target.Local, &expr.Args[0], &expr.Args[1]); ok {
			if err := fl.lowerExpr(&expr.Args[0]); err != nil {
				return true, err
			}
			fl.emit(Instruction{Op: OpMapGetOrConstIntLocalKey, A: int(expr.Args[1].Type), B: int(expr.Target.Local), Imm: 0})
			if err := fl.lowerExpr(deltaExpr); err != nil {
				return true, err
			}
			fl.emit(Instruction{Op: OpIntAdd, A: int(expr.Args[1].Type)})
			fl.emit(Instruction{Op: OpMapSetLocalStackKeyDrop, B: int(expr.Target.Local)})
			return true, nil
		}
		if expr.Args[0].Kind == air.ExprLoadLocal {
			if err := fl.lowerExpr(&expr.Args[1]); err != nil {
				return true, err
			}
			fl.emit(Instruction{Op: OpMapSetLocalDrop, B: int(expr.Target.Local), C: int(expr.Args[0].Local)})
			return true, nil
		}
	}
	return false, nil
}

func (fl *functionLowerer) lowerDiscardedBlock(block air.Block) error {
	for i := range block.Stmts {
		if err := fl.lowerStmt(&block.Stmts[i]); err != nil {
			return err
		}
	}
	if block.Result == nil {
		return nil
	}
	if ok, err := fl.lowerDiscardedExpr(block.Result); ok || err != nil {
		return err
	}
	if err := fl.lowerExpr(block.Result); err != nil {
		return err
	}
	fl.emit(Instruction{Op: OpPop})
	return nil
}

func (fl *functionLowerer) lowerDiscardedIf(expr *air.Expr) error {
	jumpFalse, ok, err := fl.lowerIfFalseJump(expr.Condition)
	if err != nil {
		return err
	}
	if !ok {
		if err := fl.lowerExpr(expr.Condition); err != nil {
			return err
		}
		jumpFalse = fl.emit(Instruction{Op: OpJumpIfFalse})
	}
	if err := fl.lowerDiscardedBlock(expr.Then); err != nil {
		return err
	}
	jumpEnd := fl.emit(Instruction{Op: OpJump})
	fl.patch(jumpFalse, len(fl.code))
	if err := fl.lowerDiscardedBlock(expr.Else); err != nil {
		return err
	}
	fl.patch(jumpEnd, len(fl.code))
	return nil
}

func (fl *functionLowerer) lowerWhile(stmt *air.Stmt) error {
	loopStart := len(fl.code)
	jumpEnd, ok := fl.lowerLoopConditionJump(stmt.Condition)
	if !ok {
		if err := fl.lowerExpr(stmt.Condition); err != nil {
			return err
		}
		jumpEnd = fl.emit(Instruction{Op: OpJumpIfFalse})
	}
	fl.breakStack = append(fl.breakStack, nil)
	if err := fl.lowerVoidBlock(stmt.Body); err != nil {
		return err
	}
	breaks := fl.breakStack[len(fl.breakStack)-1]
	fl.breakStack = fl.breakStack[:len(fl.breakStack)-1]
	if !fl.tryFuseLoopBackedgeIncrement(loopStart) {
		fl.emit(Instruction{Op: OpJump, A: loopStart})
	}
	loopEnd := len(fl.code)
	fl.patch(jumpEnd, loopEnd)
	for _, jump := range breaks {
		fl.patch(jump, loopEnd)
	}
	return nil
}

func (fl *functionLowerer) lowerVoidBlock(block air.Block) error {
	for i := range block.Stmts {
		if err := fl.lowerStmt(&block.Stmts[i]); err != nil {
			return err
		}
	}
	if block.Result != nil {
		if ok, err := fl.lowerDiscardedExpr(block.Result); ok || err != nil {
			return err
		}
		if err := fl.lowerExpr(block.Result); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpPop})
	}
	return nil
}

func (fl *functionLowerer) tryFuseLoopBackedgeIncrement(loopStart int) bool {
	if len(fl.code) < 2 {
		return false
	}
	if fl.hasPatchedJumpTo(len(fl.code)) {
		return false
	}
	store := fl.code[len(fl.code)-1]
	add := fl.code[len(fl.code)-2]
	if store.Op != OpStoreLocal || add.Op != OpIntAddConstLocal {
		return false
	}
	fl.code = fl.code[:len(fl.code)-2]
	fl.emit(Instruction{Op: OpStoreIntAddConstLocalJump, A: loopStart, B: store.A, C: add.B, Imm: add.Imm})
	return true
}

func (fl *functionLowerer) hasPatchedJumpTo(target int) bool {
	for _, inst := range fl.code {
		switch inst.Op {
		case OpJump, OpJumpIfFalse, OpStoreIntAddConstLocalJump, OpJumpIfListIndexGeLocal, OpJumpIfMapIndexGeLocal, OpJumpIfIntGtLocal, OpJumpIfIntModConstNotEqConstLocal:
			if inst.A == target {
				return true
			}
		case OpTryResult, OpTryMaybe:
			if inst.B == target {
				return true
			}
		case OpMapGetLocalTryMaybe:
			if inst.Imm == target {
				return true
			}
		}
	}
	return false
}

func (fl *functionLowerer) lowerLoopConditionJump(expr *air.Expr) (int, bool) {
	if expr == nil || expr.Left == nil || expr.Left.Kind != air.ExprLoadLocal || expr.Right == nil {
		return 0, false
	}
	if expr.Kind == air.ExprLte && expr.Right.Kind == air.ExprLoadLocal {
		return fl.emit(Instruction{Op: OpJumpIfIntGtLocal, B: int(expr.Left.Local), C: int(expr.Right.Local)}), true
	}
	if expr.Kind != air.ExprLt {
		return 0, false
	}
	if expr.Right.Kind == air.ExprListSize && expr.Right.Target != nil && expr.Right.Target.Kind == air.ExprLoadLocal {
		return fl.emit(Instruction{Op: OpJumpIfListIndexGeLocal, B: int(expr.Left.Local), C: int(expr.Right.Target.Local)}), true
	}
	if expr.Right.Kind == air.ExprMapSize && expr.Right.Target != nil && expr.Right.Target.Kind == air.ExprLoadLocal {
		return fl.emit(Instruction{Op: OpJumpIfMapIndexGeLocal, B: int(expr.Left.Local), C: int(expr.Right.Target.Local)}), true
	}
	return 0, false
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
	case air.ExprPanic:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpPanic})
	case air.ExprLoadLocal:
		fl.emit(Instruction{Op: OpLoadLocal, A: int(expr.Local)})
	case air.ExprMakeClosure:
		for _, local := range expr.CaptureLocals {
			fl.emit(Instruction{Op: OpLoadLocal, A: int(local)})
		}
		fl.emit(Instruction{Op: OpMakeClosure, A: int(expr.Type), B: len(expr.CaptureLocals), C: int(expr.Function)})
	case air.ExprCallClosure:
		if expr.Target != nil && expr.Target.Kind == air.ExprLoadLocal {
			for i := range expr.Args {
				if err := fl.lowerExpr(&expr.Args[i]); err != nil {
					return err
				}
			}
			fl.emit(Instruction{Op: OpCallClosureLocal, A: int(expr.Type), B: int(expr.Target.Local), C: len(expr.Args)})
			break
		}
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		for i := range expr.Args {
			if err := fl.lowerExpr(&expr.Args[i]); err != nil {
				return err
			}
		}
		fl.emit(Instruction{Op: OpCallClosure, A: int(expr.Type), B: len(expr.Args)})
	case air.ExprSpawnFiber:
		if expr.Target != nil {
			if err := fl.lowerExpr(expr.Target); err != nil {
				return err
			}
			fl.emit(Instruction{Op: OpSpawnFiber, A: int(expr.Type), B: 1})
		} else {
			fl.emit(Instruction{Op: OpSpawnFiber, A: int(expr.Type), C: int(expr.Function)})
		}
	case air.ExprFiberGet:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpFiberGet, A: int(expr.Type)})
	case air.ExprFiberJoin:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpFiberJoin, A: int(expr.Type)})
	case air.ExprCall:
		for i := range expr.Args {
			if err := fl.lowerExpr(&expr.Args[i]); err != nil {
				return err
			}
		}
		fl.emit(Instruction{Op: OpCall, A: int(expr.Function), B: len(expr.Args)})
	case air.ExprCallExtern:
		for i := range expr.Args {
			if err := fl.lowerExpr(&expr.Args[i]); err != nil {
				return err
			}
		}
		fl.emit(Instruction{Op: OpCallExtern, A: int(expr.Extern), B: len(expr.Args)})
	case air.ExprCopy:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpCopy, A: int(expr.Type)})
	case air.ExprTraitUpcast:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpTraitUpcast, A: int(expr.Type), B: int(expr.Trait), C: int(expr.Impl)})
	case air.ExprCallTrait:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		for i := range expr.Args {
			if err := fl.lowerExpr(&expr.Args[i]); err != nil {
				return err
			}
		}
		fl.emit(Instruction{Op: OpCallTrait, A: int(expr.Type), B: int(expr.Trait), C: expr.Method, Imm: len(expr.Args)})
	case air.ExprUnionWrap:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpUnionWrap, A: int(expr.Type), Imm: int(expr.Tag)})
	case air.ExprBlock:
		fl.emit(Instruction{Op: OpBlock})
		return fl.lowerBlock(expr.Body, expr.Type)
	case air.ExprIf:
		jumpFalse, ok, err := fl.lowerIfFalseJump(expr.Condition)
		if err != nil {
			return err
		}
		if !ok {
			if err := fl.lowerExpr(expr.Condition); err != nil {
				return err
			}
			jumpFalse = fl.emit(Instruction{Op: OpJumpIfFalse})
		}
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
	case air.ExprNot, air.ExprNeg, air.ExprToStr, air.ExprToDynamic:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: unaryOpcode(expr.Kind), A: int(expr.Type)})
	case air.ExprEnumVariant:
		fl.emit(Instruction{Op: OpEnumVariant, A: int(expr.Type), Imm: expr.Discriminant})
	case air.ExprStrAt, air.ExprStrSize, air.ExprStrIsEmpty, air.ExprStrContains, air.ExprStrReplace, air.ExprStrReplaceAll, air.ExprStrSplit, air.ExprStrStartsWith, air.ExprStrTrim:
		return fl.lowerStrOp(expr)
	case air.ExprMakeList:
		for i := range expr.Args {
			if err := fl.lowerExpr(&expr.Args[i]); err != nil {
				return err
			}
		}
		fl.emit(Instruction{Op: OpMakeList, A: int(expr.Type), B: len(expr.Args)})
	case air.ExprListAt, air.ExprListPrepend, air.ExprListPush, air.ExprListSet, air.ExprListSize, air.ExprListSort, air.ExprListSwap:
		return fl.lowerListOp(expr)
	case air.ExprMakeMap:
		for i := range expr.Entries {
			if err := fl.lowerExpr(&expr.Entries[i].Key); err != nil {
				return err
			}
			if err := fl.lowerExpr(&expr.Entries[i].Value); err != nil {
				return err
			}
		}
		fl.emit(Instruction{Op: OpMakeMap, A: int(expr.Type), B: len(expr.Entries)})
	case air.ExprMapKeys, air.ExprMapSize, air.ExprMapGet, air.ExprMapSet, air.ExprMapDrop, air.ExprMapHas, air.ExprMapKeyAt, air.ExprMapValueAt:
		return fl.lowerMapOp(expr)
	case air.ExprMakeStruct:
		return fl.lowerMakeStruct(expr)
	case air.ExprGetField:
		if expr.Target != nil && expr.Target.Kind == air.ExprLoadLocal {
			fl.emit(Instruction{Op: OpGetFieldLocal, A: int(expr.Type), B: int(expr.Target.Local), C: expr.Field})
			return nil
		}
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpGetField, A: int(expr.Type), B: expr.Field})
	case air.ExprMakeMaybeSome:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpMakeMaybeSome, A: int(expr.Type)})
	case air.ExprMakeMaybeNone:
		fl.emit(Instruction{Op: OpMakeMaybeNone, A: int(expr.Type), B: int(fl.mustMaybeElem(expr.Type))})
	case air.ExprMatchEnum:
		return fl.lowerEnumMatch(expr)
	case air.ExprMatchInt:
		return fl.lowerIntMatch(expr)
	case air.ExprMatchMaybe:
		return fl.lowerMaybeMatch(expr)
	case air.ExprMatchUnion:
		return fl.lowerUnionMatch(expr)
	case air.ExprMatchResult:
		return fl.lowerResultMatch(expr)
	case air.ExprMaybeExpect, air.ExprMaybeIsNone, air.ExprMaybeIsSome, air.ExprMaybeOr, air.ExprMaybeMap, air.ExprMaybeAndThen:
		return fl.lowerMaybeOp(expr)
	case air.ExprMakeResultOk:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpMakeResultOk, A: int(expr.Type)})
	case air.ExprMakeResultErr:
		if err := fl.lowerExpr(expr.Target); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpMakeResultErr, A: int(expr.Type)})
	case air.ExprResultExpect, air.ExprResultOr, air.ExprResultIsOk, air.ExprResultIsErr, air.ExprResultMap, air.ExprResultMapErr, air.ExprResultAndThen:
		return fl.lowerResultOp(expr)
	case air.ExprTryResult:
		return fl.lowerTryOp(expr, OpTryResult)
	case air.ExprTryMaybe:
		return fl.lowerTryOp(expr, OpTryMaybe)
	default:
		return fmt.Errorf("unsupported expression kind %d", expr.Kind)
	}
	return nil
}

func (fl *functionLowerer) lowerIfFalseJump(expr *air.Expr) (int, bool, error) {
	if local, divisor, expected, ok := intModConstEqConstLocal(expr); ok {
		if divisor == 0 {
			return 0, false, fmt.Errorf("integer modulo by zero")
		}
		return fl.emit(Instruction{Op: OpJumpIfIntModConstNotEqConstLocal, B: int(local), C: divisor, Imm: expected}), true, nil
	}
	return 0, false, nil
}

func intModConstEqConstLocal(expr *air.Expr) (air.LocalID, int, int, bool) {
	if expr == nil || expr.Kind != air.ExprEq {
		return 0, 0, 0, false
	}
	if local, divisor, ok := intModConstLocal(expr.Left); ok && expr.Right != nil && expr.Right.Kind == air.ExprConstInt {
		return local, divisor, expr.Right.Int, true
	}
	if local, divisor, ok := intModConstLocal(expr.Right); ok && expr.Left != nil && expr.Left.Kind == air.ExprConstInt {
		return local, divisor, expr.Left.Int, true
	}
	return 0, 0, 0, false
}

func intModConstLocal(expr *air.Expr) (air.LocalID, int, bool) {
	if expr == nil || expr.Kind != air.ExprIntMod || expr.Left == nil || expr.Left.Kind != air.ExprLoadLocal || expr.Right == nil || expr.Right.Kind != air.ExprConstInt {
		return 0, 0, false
	}
	return expr.Left.Local, expr.Right.Int, true
}

func (fl *functionLowerer) lowerEnumMatch(expr *air.Expr) error {
	return fl.lowerDiscriminantMatch(expr, true)
}

func (fl *functionLowerer) lowerIntMatch(expr *air.Expr) error {
	return fl.lowerDiscriminantMatch(expr, false)
}

func (fl *functionLowerer) lowerDiscriminantMatch(expr *air.Expr, enum bool) error {
	subjectLocal := fl.tempLocal()
	if err := fl.lowerExpr(expr.Target); err != nil {
		return err
	}
	fl.emit(Instruction{Op: OpStoreLocal, A: subjectLocal})
	endJumps := []int{}
	if enum {
		for _, matchCase := range expr.EnumCases {
			fl.emit(Instruction{Op: OpLoadLocal, A: subjectLocal})
			fl.emit(Instruction{Op: OpConstInt, A: int(fl.mustTypeID(air.TypeInt)), Imm: matchCase.Discriminant})
			fl.emit(Instruction{Op: OpEq, A: int(fl.mustTypeID(air.TypeBool))})
			next := fl.emit(Instruction{Op: OpJumpIfFalse})
			if err := fl.lowerBlock(matchCase.Body, expr.Type); err != nil {
				return err
			}
			endJumps = append(endJumps, fl.emit(Instruction{Op: OpJump}))
			fl.patch(next, len(fl.code))
		}
	} else {
		for _, matchCase := range expr.IntCases {
			fl.emit(Instruction{Op: OpLoadLocal, A: subjectLocal})
			fl.emit(Instruction{Op: OpConstInt, A: int(fl.mustTypeID(air.TypeInt)), Imm: matchCase.Value})
			fl.emit(Instruction{Op: OpEq, A: int(fl.mustTypeID(air.TypeBool))})
			next := fl.emit(Instruction{Op: OpJumpIfFalse})
			if err := fl.lowerBlock(matchCase.Body, expr.Type); err != nil {
				return err
			}
			endJumps = append(endJumps, fl.emit(Instruction{Op: OpJump}))
			fl.patch(next, len(fl.code))
		}
		for _, matchCase := range expr.RangeCases {
			fl.emit(Instruction{Op: OpLoadLocal, A: subjectLocal})
			fl.emit(Instruction{Op: OpConstInt, A: int(fl.mustTypeID(air.TypeInt)), Imm: matchCase.Start})
			fl.emit(Instruction{Op: OpGte, A: int(fl.mustTypeID(air.TypeBool))})
			fl.emit(Instruction{Op: OpLoadLocal, A: subjectLocal})
			fl.emit(Instruction{Op: OpConstInt, A: int(fl.mustTypeID(air.TypeInt)), Imm: matchCase.End})
			fl.emit(Instruction{Op: OpLte, A: int(fl.mustTypeID(air.TypeBool))})
			fl.emit(Instruction{Op: OpAnd, A: int(fl.mustTypeID(air.TypeBool))})
			next := fl.emit(Instruction{Op: OpJumpIfFalse})
			if err := fl.lowerBlock(matchCase.Body, expr.Type); err != nil {
				return err
			}
			endJumps = append(endJumps, fl.emit(Instruction{Op: OpJump}))
			fl.patch(next, len(fl.code))
		}
	}
	if expr.CatchAll.Result != nil || len(expr.CatchAll.Stmts) > 0 {
		if err := fl.lowerBlock(expr.CatchAll, expr.Type); err != nil {
			return err
		}
	} else if err := fl.emitZero(expr.Type); err != nil {
		return err
	}
	end := len(fl.code)
	for _, jump := range endJumps {
		fl.patch(jump, end)
	}
	return nil
}

func (fl *functionLowerer) lowerMaybeMatch(expr *air.Expr) error {
	subjectLocal := fl.tempLocal()
	if err := fl.lowerExpr(expr.Target); err != nil {
		return err
	}
	fl.emit(Instruction{Op: OpStoreLocal, A: subjectLocal})
	fl.emit(Instruction{Op: OpLoadLocal, A: subjectLocal})
	fl.emit(Instruction{Op: OpMaybeIsSome, A: int(fl.mustTypeID(air.TypeBool))})
	jumpNone := fl.emit(Instruction{Op: OpJumpIfFalse})
	fl.emit(Instruction{Op: OpLoadLocal, A: subjectLocal})
	fl.emit(Instruction{Op: OpMaybeExpect, A: int(expr.Type)})
	fl.emit(Instruction{Op: OpStoreLocal, A: int(expr.SomeLocal)})
	if err := fl.lowerBlock(expr.Some, expr.Type); err != nil {
		return err
	}
	jumpEnd := fl.emit(Instruction{Op: OpJump})
	fl.patch(jumpNone, len(fl.code))
	if err := fl.lowerBlock(expr.None, expr.Type); err != nil {
		return err
	}
	fl.patch(jumpEnd, len(fl.code))
	return nil
}

func (fl *functionLowerer) lowerUnionMatch(expr *air.Expr) error {
	subjectLocal := fl.tempLocal()
	if err := fl.lowerExpr(expr.Target); err != nil {
		return err
	}
	fl.emit(Instruction{Op: OpStoreLocal, A: subjectLocal})
	endJumps := []int{}
	for _, matchCase := range expr.UnionCases {
		fl.emit(Instruction{Op: OpUnionTagLocal, A: int(fl.mustTypeID(air.TypeInt)), B: subjectLocal})
		fl.emit(Instruction{Op: OpConstInt, A: int(fl.mustTypeID(air.TypeInt)), Imm: int(matchCase.Tag)})
		fl.emit(Instruction{Op: OpEq, A: int(fl.mustTypeID(air.TypeBool))})
		next := fl.emit(Instruction{Op: OpJumpIfFalse})
		fl.emit(Instruction{Op: OpUnionValueLocal, A: int(expr.Type), B: subjectLocal})
		fl.emit(Instruction{Op: OpStoreLocal, A: int(matchCase.Local)})
		if err := fl.lowerBlock(matchCase.Body, expr.Type); err != nil {
			return err
		}
		endJumps = append(endJumps, fl.emit(Instruction{Op: OpJump}))
		fl.patch(next, len(fl.code))
	}
	if expr.CatchAll.Result != nil || len(expr.CatchAll.Stmts) > 0 {
		if err := fl.lowerBlock(expr.CatchAll, expr.Type); err != nil {
			return err
		}
	} else if err := fl.emitZero(expr.Type); err != nil {
		return err
	}
	end := len(fl.code)
	for _, jump := range endJumps {
		fl.patch(jump, end)
	}
	return nil
}

func (fl *functionLowerer) lowerResultMatch(expr *air.Expr) error {
	return fl.lowerResultMatchWithBlocks(expr, fl.lowerBlock)
}

func (fl *functionLowerer) lowerResultMatchDiscarded(expr *air.Expr) error {
	return fl.lowerResultMatchWithBlocks(expr, func(block air.Block, _ air.TypeID) error {
		return fl.lowerVoidBlock(block)
	})
}

func (fl *functionLowerer) lowerResultMatchWithBlocks(expr *air.Expr, lowerBlock func(air.Block, air.TypeID) error) error {
	subjectLocal := fl.tempLocal()
	if err := fl.lowerExpr(expr.Target); err != nil {
		return err
	}
	fl.emit(Instruction{Op: OpStoreLocal, A: subjectLocal})
	fl.emit(Instruction{Op: OpResultIsOkLocal, A: int(fl.mustTypeID(air.TypeBool)), B: subjectLocal})
	jumpErr := fl.emit(Instruction{Op: OpJumpIfFalse})
	fl.emit(Instruction{Op: OpResultExpectLocal, A: int(expr.Type), B: subjectLocal})
	fl.emit(Instruction{Op: OpStoreLocal, A: int(expr.OkLocal)})
	if err := lowerBlock(expr.Ok, expr.Type); err != nil {
		return err
	}
	jumpEnd := fl.emit(Instruction{Op: OpJump})
	fl.patch(jumpErr, len(fl.code))
	fl.emit(Instruction{Op: OpResultErrValueLocal, A: int(expr.Type), B: subjectLocal})
	fl.emit(Instruction{Op: OpStoreLocal, A: int(expr.ErrLocal)})
	if err := lowerBlock(expr.Err, expr.Type); err != nil {
		return err
	}
	fl.patch(jumpEnd, len(fl.code))
	return nil
}

func (fl *functionLowerer) lowerStrOp(expr *air.Expr) error {
	if err := fl.lowerExpr(expr.Target); err != nil {
		return err
	}
	for i := range expr.Args {
		if err := fl.lowerExpr(&expr.Args[i]); err != nil {
			return err
		}
	}
	fl.emit(Instruction{Op: strOpcode(expr.Kind), A: int(expr.Type), B: len(expr.Args)})
	return nil
}

func (fl *functionLowerer) lowerMaybeOp(expr *air.Expr) error {
	if expr.Kind == air.ExprMaybeOr && len(expr.Args) == 1 && expr.Args[0].Kind == air.ExprConstInt && expr.Target != nil && expr.Target.Kind == air.ExprMapGet && expr.Target.Target != nil && expr.Target.Target.Kind == air.ExprLoadLocal && len(expr.Target.Args) == 1 && expr.Target.Args[0].Kind == air.ExprLoadLocal {
		fl.emit(Instruction{Op: OpMapGetOrConstIntLocal, A: int(expr.Type), B: int(expr.Target.Target.Local), C: int(expr.Target.Args[0].Local), Imm: expr.Args[0].Int})
		return nil
	}
	if err := fl.lowerExpr(expr.Target); err != nil {
		return err
	}
	for i := range expr.Args {
		if err := fl.lowerExpr(&expr.Args[i]); err != nil {
			return err
		}
	}
	fl.emit(Instruction{Op: maybeOpcode(expr.Kind), A: int(expr.Type), B: len(expr.Args)})
	return nil
}

func (fl *functionLowerer) lowerResultOp(expr *air.Expr) error {
	if err := fl.lowerExpr(expr.Target); err != nil {
		return err
	}
	for i := range expr.Args {
		if err := fl.lowerExpr(&expr.Args[i]); err != nil {
			return err
		}
	}
	fl.emit(Instruction{Op: resultOpcode(expr.Kind), A: int(expr.Type), B: len(expr.Args)})
	return nil
}

func (fl *functionLowerer) lowerTryOp(expr *air.Expr, op Opcode) error {
	if op == OpTryMaybe && expr.Target != nil && expr.Target.Kind == air.ExprMapGet && expr.Target.Target != nil && expr.Target.Target.Kind == air.ExprLoadLocal && len(expr.Target.Args) == 1 && expr.Target.Args[0].Kind == air.ExprLoadLocal {
		inst := Instruction{Op: OpMapGetLocalTryMaybe, A: int(fl.fn.Signature.Return), B: int(expr.Target.Target.Local), C: int(expr.Target.Args[0].Local), Imm: -1}
		tryIndex := fl.emit(inst)
		if !expr.HasCatch {
			return nil
		}
		jumpNormal := fl.emit(Instruction{Op: OpJump})
		fl.code[tryIndex].Imm = len(fl.code)
		if err := fl.lowerBlock(expr.Catch, fl.fn.Signature.Return); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpReturn})
		fl.patch(jumpNormal, len(fl.code))
		return nil
	}
	if err := fl.lowerExpr(expr.Target); err != nil {
		return err
	}
	inst := Instruction{Op: op, A: int(fl.fn.Signature.Return), B: -1, C: int(expr.CatchLocal)}
	tryIndex := fl.emit(inst)
	if !expr.HasCatch {
		return nil
	}
	jumpNormal := fl.emit(Instruction{Op: OpJump})
	fl.code[tryIndex].B = len(fl.code)
	if err := fl.lowerBlock(expr.Catch, fl.fn.Signature.Return); err != nil {
		return err
	}
	fl.emit(Instruction{Op: OpReturn})
	fl.patch(jumpNormal, len(fl.code))
	return nil
}

func (fl *functionLowerer) lowerListOp(expr *air.Expr) error {
	if expr.Kind == air.ExprListSize && expr.Target != nil && expr.Target.Kind == air.ExprLoadLocal && len(expr.Args) == 0 {
		fl.emit(Instruction{Op: OpListSizeLocal, A: int(expr.Type), B: int(expr.Target.Local)})
		return nil
	}
	if expr.Kind == air.ExprListAt && expr.Target != nil && expr.Target.Kind == air.ExprLoadLocal && len(expr.Args) == 1 {
		if expr.Args[0].Kind == air.ExprLoadLocal {
			fl.emit(Instruction{Op: OpListAtLocal, A: int(expr.Type), B: int(expr.Target.Local), C: int(expr.Args[0].Local)})
			return nil
		}
		if indexLocal, offset, ok := listAtModuloIndex(expr.Target.Local, &expr.Args[0]); ok {
			fl.emit(Instruction{Op: OpListAtModLocal, A: int(expr.Type), B: int(expr.Target.Local), C: int(indexLocal), Imm: offset})
			return nil
		}
	}
	if expr.Kind == air.ExprListPush && expr.Target != nil && expr.Target.Kind == air.ExprLoadLocal && len(expr.Args) == 1 {
		if err := fl.lowerExpr(&expr.Args[0]); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpListPushLocal, A: int(expr.Type), B: int(expr.Target.Local)})
		return nil
	}
	if err := fl.lowerExpr(expr.Target); err != nil {
		return err
	}
	for i := range expr.Args {
		if err := fl.lowerExpr(&expr.Args[i]); err != nil {
			return err
		}
	}
	fl.emit(Instruction{Op: listOpcode(expr.Kind), A: int(expr.Type), B: len(expr.Args)})
	return nil
}

func (fl *functionLowerer) lowerMapOp(expr *air.Expr) error {
	if expr.Kind == air.ExprMapSize && expr.Target != nil && expr.Target.Kind == air.ExprLoadLocal && len(expr.Args) == 0 {
		fl.emit(Instruction{Op: OpMapSizeLocal, A: int(expr.Type), B: int(expr.Target.Local)})
		return nil
	}
	if expr.Target != nil && expr.Target.Kind == air.ExprLoadLocal && len(expr.Args) == 1 && expr.Args[0].Kind == air.ExprLoadLocal {
		if expr.Kind == air.ExprMapGet {
			fl.emit(Instruction{Op: OpMapGetLocal, A: int(expr.Type), B: int(expr.Target.Local), C: int(expr.Args[0].Local)})
			return nil
		}
		if expr.Kind == air.ExprMapKeyAt || expr.Kind == air.ExprMapValueAt {
			op := OpMapKeyAtLocal
			if expr.Kind == air.ExprMapValueAt {
				op = OpMapValueAtLocal
			}
			fl.emit(Instruction{Op: op, A: int(expr.Type), B: int(expr.Target.Local), C: int(expr.Args[0].Local)})
			return nil
		}
	}
	if expr.Kind == air.ExprMapSet && expr.Target != nil && expr.Target.Kind == air.ExprLoadLocal && len(expr.Args) == 2 && expr.Args[0].Kind == air.ExprLoadLocal {
		if delta, ok := mapIncrementDelta(expr.Target.Local, expr.Args[0].Local, &expr.Args[1]); ok {
			fl.emit(Instruction{Op: OpMapIncrementIntLocal, A: int(expr.Type), B: int(expr.Target.Local), C: int(expr.Args[0].Local), Imm: delta})
			return nil
		}
		if err := fl.lowerExpr(&expr.Args[1]); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpMapSetLocal, A: int(expr.Type), B: int(expr.Target.Local), C: int(expr.Args[0].Local)})
		return nil
	}
	if err := fl.lowerExpr(expr.Target); err != nil {
		return err
	}
	for i := range expr.Args {
		if err := fl.lowerExpr(&expr.Args[i]); err != nil {
			return err
		}
	}
	fl.emit(Instruction{Op: mapOpcode(expr.Kind), A: int(expr.Type), B: len(expr.Args)})
	return nil
}

func (fl *functionLowerer) lowerMakeStruct(expr *air.Expr) error {
	typeInfo, ok := fl.root.out.TypeInfo(expr.Type)
	if !ok {
		return fmt.Errorf("invalid struct type %d", expr.Type)
	}
	for _, field := range typeInfo.Fields {
		found := false
		for i := range expr.Fields {
			if expr.Fields[i].Index == field.Index {
				if err := fl.lowerExpr(&expr.Fields[i].Value); err != nil {
					return err
				}
				found = true
				break
			}
		}
		if !found {
			if err := fl.emitZero(field.Type); err != nil {
				return err
			}
		}
	}
	fl.emit(Instruction{Op: OpMakeStruct, A: int(expr.Type), B: len(typeInfo.Fields)})
	return nil
}

func strOpcode(kind air.ExprKind) Opcode {
	switch kind {
	case air.ExprStrAt:
		return OpStrAt
	case air.ExprStrSize:
		return OpStrSize
	case air.ExprStrIsEmpty:
		return OpStrIsEmpty
	case air.ExprStrContains:
		return OpStrContains
	case air.ExprStrReplace:
		return OpStrReplace
	case air.ExprStrReplaceAll:
		return OpStrReplaceAll
	case air.ExprStrSplit:
		return OpStrSplit
	case air.ExprStrStartsWith:
		return OpStrStartsWith
	case air.ExprStrTrim:
		return OpStrTrim
	default:
		return OpNoop
	}
}

func maybeOpcode(kind air.ExprKind) Opcode {
	switch kind {
	case air.ExprMaybeExpect:
		return OpMaybeExpect
	case air.ExprMaybeIsNone:
		return OpMaybeIsNone
	case air.ExprMaybeIsSome:
		return OpMaybeIsSome
	case air.ExprMaybeOr:
		return OpMaybeOr
	case air.ExprMaybeMap:
		return OpMaybeMap
	case air.ExprMaybeAndThen:
		return OpMaybeAndThen
	default:
		return OpNoop
	}
}

func resultOpcode(kind air.ExprKind) Opcode {
	switch kind {
	case air.ExprResultExpect:
		return OpResultExpect
	case air.ExprResultOr:
		return OpResultOr
	case air.ExprResultIsOk:
		return OpResultIsOk
	case air.ExprResultIsErr:
		return OpResultIsErr
	case air.ExprResultMap:
		return OpResultMap
	case air.ExprResultMapErr:
		return OpResultMapErr
	case air.ExprResultAndThen:
		return OpResultAndThen
	default:
		return OpNoop
	}
}

func listOpcode(kind air.ExprKind) Opcode {
	switch kind {
	case air.ExprListAt:
		return OpListAt
	case air.ExprListPrepend:
		return OpListPrepend
	case air.ExprListPush:
		return OpListPush
	case air.ExprListSet:
		return OpListSet
	case air.ExprListSize:
		return OpListSize
	case air.ExprListSort:
		return OpListSort
	case air.ExprListSwap:
		return OpListSwap
	default:
		return OpNoop
	}
}

func listAtModuloIndex(listLocal air.LocalID, expr *air.Expr) (air.LocalID, int, bool) {
	if expr == nil || expr.Kind != air.ExprIntMod || expr.Right == nil || expr.Right.Kind != air.ExprListSize || expr.Right.Target == nil || expr.Right.Target.Kind != air.ExprLoadLocal || expr.Right.Target.Local != listLocal {
		return 0, 0, false
	}
	if expr.Left != nil && expr.Left.Kind == air.ExprLoadLocal {
		return expr.Left.Local, 0, true
	}
	if expr.Left != nil && expr.Left.Kind == air.ExprIntAdd {
		if expr.Left.Left != nil && expr.Left.Left.Kind == air.ExprLoadLocal && expr.Left.Right != nil && expr.Left.Right.Kind == air.ExprConstInt {
			return expr.Left.Left.Local, expr.Left.Right.Int, true
		}
		if expr.Left.Right != nil && expr.Left.Right.Kind == air.ExprLoadLocal && expr.Left.Left != nil && expr.Left.Left.Kind == air.ExprConstInt {
			return expr.Left.Right.Local, expr.Left.Left.Int, true
		}
	}
	return 0, 0, false
}

func mapIncrementDelta(mapLocal air.LocalID, keyLocal air.LocalID, value *air.Expr) (int, bool) {
	if value == nil || value.Kind != air.ExprIntAdd {
		return 0, false
	}
	if delta, ok := mapIncrementSide(mapLocal, keyLocal, value.Left, value.Right); ok {
		return delta, true
	}
	return mapIncrementSide(mapLocal, keyLocal, value.Right, value.Left)
}

func mapIncrementSide(mapLocal air.LocalID, keyLocal air.LocalID, maybeExpr *air.Expr, deltaExpr *air.Expr) (int, bool) {
	if deltaExpr == nil || deltaExpr.Kind != air.ExprConstInt {
		return 0, false
	}
	if maybeExpr == nil || maybeExpr.Kind != air.ExprMaybeOr || len(maybeExpr.Args) != 1 || maybeExpr.Args[0].Kind != air.ExprConstInt || maybeExpr.Args[0].Int != 0 {
		return 0, false
	}
	getExpr := maybeExpr.Target
	if getExpr == nil || getExpr.Kind != air.ExprMapGet || getExpr.Target == nil || getExpr.Target.Kind != air.ExprLoadLocal || getExpr.Target.Local != mapLocal || len(getExpr.Args) != 1 || getExpr.Args[0].Kind != air.ExprLoadLocal || getExpr.Args[0].Local != keyLocal {
		return 0, false
	}
	return deltaExpr.Int, true
}

func mapIncrementValueDelta(mapLocal air.LocalID, keyExpr *air.Expr, value *air.Expr) (*air.Expr, bool) {
	if keyExpr == nil || !safeSingleEvalMapKeyExpr(keyExpr) || value == nil || value.Kind != air.ExprIntAdd {
		return nil, false
	}
	if delta, ok := mapIncrementValueSide(mapLocal, keyExpr, value.Left, value.Right); ok {
		return delta, true
	}
	return mapIncrementValueSide(mapLocal, keyExpr, value.Right, value.Left)
}

func mapIncrementValueSide(mapLocal air.LocalID, keyExpr *air.Expr, maybeExpr *air.Expr, deltaExpr *air.Expr) (*air.Expr, bool) {
	if deltaExpr == nil || maybeExpr == nil || maybeExpr.Kind != air.ExprMaybeOr || len(maybeExpr.Args) != 1 || maybeExpr.Args[0].Kind != air.ExprConstInt || maybeExpr.Args[0].Int != 0 {
		return nil, false
	}
	getExpr := maybeExpr.Target
	if getExpr == nil || getExpr.Kind != air.ExprMapGet || getExpr.Target == nil || getExpr.Target.Kind != air.ExprLoadLocal || getExpr.Target.Local != mapLocal || len(getExpr.Args) != 1 || !sameMapKeyExpr(keyExpr, &getExpr.Args[0]) {
		return nil, false
	}
	return deltaExpr, true
}

func safeSingleEvalMapKeyExpr(expr *air.Expr) bool {
	if expr == nil {
		return false
	}
	switch expr.Kind {
	case air.ExprLoadLocal, air.ExprConstInt, air.ExprConstBool, air.ExprConstStr, air.ExprEnumVariant:
		return true
	case air.ExprGetField:
		return expr.Target != nil && expr.Target.Kind == air.ExprLoadLocal
	default:
		return false
	}
}

func sameMapKeyExpr(left *air.Expr, right *air.Expr) bool {
	if left == nil || right == nil || left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case air.ExprLoadLocal:
		return left.Local == right.Local
	case air.ExprConstInt:
		return left.Int == right.Int
	case air.ExprConstBool:
		return left.Bool == right.Bool
	case air.ExprConstStr:
		return left.Str == right.Str
	case air.ExprEnumVariant:
		return left.Type == right.Type && left.Variant == right.Variant && left.Discriminant == right.Discriminant
	case air.ExprGetField:
		return left.Field == right.Field && left.Target != nil && right.Target != nil && left.Target.Kind == air.ExprLoadLocal && right.Target.Kind == air.ExprLoadLocal && left.Target.Local == right.Target.Local
	default:
		return false
	}
}

func mapOpcode(kind air.ExprKind) Opcode {
	switch kind {
	case air.ExprMapKeys:
		return OpMapKeys
	case air.ExprMapSize:
		return OpMapSize
	case air.ExprMapGet:
		return OpMapGet
	case air.ExprMapSet:
		return OpMapSet
	case air.ExprMapDrop:
		return OpMapDrop
	case air.ExprMapHas:
		return OpMapHas
	case air.ExprMapKeyAt:
		return OpMapKeyAt
	case air.ExprMapValueAt:
		return OpMapValueAt
	default:
		return OpNoop
	}
}

func (fl *functionLowerer) lowerBinary(expr *air.Expr) error {
	if expr.Kind == air.ExprIntAdd {
		if expr.Left != nil && expr.Left.Kind == air.ExprLoadLocal && expr.Right != nil && expr.Right.Kind == air.ExprConstInt {
			fl.emit(Instruction{Op: OpIntAddConstLocal, A: int(expr.Type), B: int(expr.Left.Local), Imm: expr.Right.Int})
			return nil
		}
		if expr.Right != nil && expr.Right.Kind == air.ExprLoadLocal && expr.Left != nil && expr.Left.Kind == air.ExprConstInt {
			fl.emit(Instruction{Op: OpIntAddConstLocal, A: int(expr.Type), B: int(expr.Right.Local), Imm: expr.Left.Int})
			return nil
		}
	}
	if expr.Kind == air.ExprLt && expr.Left != nil && expr.Left.Kind == air.ExprLoadLocal && expr.Right != nil {
		if expr.Right.Kind == air.ExprListSize && expr.Right.Target != nil && expr.Right.Target.Kind == air.ExprLoadLocal {
			fl.emit(Instruction{Op: OpListIndexLtLocal, A: int(expr.Type), B: int(expr.Left.Local), C: int(expr.Right.Target.Local)})
			return nil
		}
		if expr.Right.Kind == air.ExprMapSize && expr.Right.Target != nil && expr.Right.Target.Kind == air.ExprLoadLocal {
			fl.emit(Instruction{Op: OpMapIndexLtLocal, A: int(expr.Type), B: int(expr.Left.Local), C: int(expr.Right.Target.Local)})
			return nil
		}
	}
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
	case air.ExprToDynamic:
		return OpToDynamic
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
	case air.TypeList:
		fl.emit(Instruction{Op: OpMakeList, A: int(typeID), B: 0})
	case air.TypeMap:
		fl.emit(Instruction{Op: OpMakeMap, A: int(typeID), B: 0})
	case air.TypeMaybe:
		fl.emit(Instruction{Op: OpMakeMaybeNone, A: int(typeID), B: int(info.Elem)})
	case air.TypeResult:
		if err := fl.emitZero(info.Value); err != nil {
			return err
		}
		fl.emit(Instruction{Op: OpMakeResultOk, A: int(typeID)})
	case air.TypeDynamic:
		fl.emit(Instruction{Op: OpConstVoid, A: int(fl.mustTypeID(air.TypeVoid))})
		fl.emit(Instruction{Op: OpToDynamic, A: int(typeID)})
	case air.TypeStruct:
		for _, field := range info.Fields {
			if err := fl.emitZero(field.Type); err != nil {
				return err
			}
		}
		fl.emit(Instruction{Op: OpMakeStruct, A: int(typeID), B: len(info.Fields)})
	default:
		return fmt.Errorf("unsupported zero value for type %s", info.Name)
	}
	return nil
}

func (fl *functionLowerer) tempLocal() int {
	local := fl.localTop
	fl.localTop++
	return local
}

func (fl *functionLowerer) mustMaybeElem(typeID air.TypeID) air.TypeID {
	typeInfo, ok := fl.root.out.TypeInfo(typeID)
	if !ok || typeInfo.Kind != air.TypeMaybe {
		return fl.mustTypeID(air.TypeVoid)
	}
	return typeInfo.Elem
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
	if inst.Op == OpPop && len(fl.code) > 0 {
		last := &fl.code[len(fl.code)-1]
		switch last.Op {
		case OpListPushLocal:
			last.Op = OpListPushLocalDrop
			return len(fl.code) - 1
		case OpMapSetLocal:
			last.Op = OpMapSetLocalDrop
			return len(fl.code) - 1
		case OpMapIncrementIntLocal:
			last.Op = OpMapIncrementIntLocalDrop
			return len(fl.code) - 1
		}
	}
	index := len(fl.code)
	fl.code = append(fl.code, inst)
	return index
}

func (fl *functionLowerer) patch(index int, target int) {
	fl.code[index].A = target
}
