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

func (fl *functionLowerer) lowerWhile(stmt *air.Stmt) error {
	loopStart := len(fl.code)
	if err := fl.lowerExpr(stmt.Condition); err != nil {
		return err
	}
	jumpEnd := fl.emit(Instruction{Op: OpJumpIfFalse})
	fl.breakStack = append(fl.breakStack, nil)
	if err := fl.lowerBlock(stmt.Body, fl.mustTypeID(air.TypeVoid)); err != nil {
		return err
	}
	breaks := fl.breakStack[len(fl.breakStack)-1]
	fl.breakStack = fl.breakStack[:len(fl.breakStack)-1]
	fl.emit(Instruction{Op: OpJump, A: loopStart})
	loopEnd := len(fl.code)
	fl.patch(jumpEnd, loopEnd)
	for _, jump := range breaks {
		fl.patch(jump, loopEnd)
	}
	return nil
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
	case air.ExprMakeClosure:
		for _, local := range expr.CaptureLocals {
			fl.emit(Instruction{Op: OpLoadLocal, A: int(local)})
		}
		fl.emit(Instruction{Op: OpMakeClosure, A: int(expr.Type), B: len(expr.CaptureLocals), C: int(expr.Function)})
	case air.ExprCallClosure:
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
		fl.emit(Instruction{Op: OpLoadLocal, A: subjectLocal})
		fl.emit(Instruction{Op: OpUnionTag, A: int(fl.mustTypeID(air.TypeInt))})
		fl.emit(Instruction{Op: OpConstInt, A: int(fl.mustTypeID(air.TypeInt)), Imm: int(matchCase.Tag)})
		fl.emit(Instruction{Op: OpEq, A: int(fl.mustTypeID(air.TypeBool))})
		next := fl.emit(Instruction{Op: OpJumpIfFalse})
		fl.emit(Instruction{Op: OpLoadLocal, A: subjectLocal})
		fl.emit(Instruction{Op: OpUnionValue, A: int(expr.Type)})
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
	subjectLocal := fl.tempLocal()
	if err := fl.lowerExpr(expr.Target); err != nil {
		return err
	}
	fl.emit(Instruction{Op: OpStoreLocal, A: subjectLocal})
	fl.emit(Instruction{Op: OpLoadLocal, A: subjectLocal})
	fl.emit(Instruction{Op: OpResultIsOk, A: int(fl.mustTypeID(air.TypeBool))})
	jumpErr := fl.emit(Instruction{Op: OpJumpIfFalse})
	fl.emit(Instruction{Op: OpLoadLocal, A: subjectLocal})
	fl.emit(Instruction{Op: OpResultExpect, A: int(expr.Type)})
	fl.emit(Instruction{Op: OpStoreLocal, A: int(expr.OkLocal)})
	if err := fl.lowerBlock(expr.Ok, expr.Type); err != nil {
		return err
	}
	jumpEnd := fl.emit(Instruction{Op: OpJump})
	fl.patch(jumpErr, len(fl.code))
	fl.emit(Instruction{Op: OpLoadLocal, A: subjectLocal})
	fl.emit(Instruction{Op: OpResultErrValue, A: int(expr.Type)})
	fl.emit(Instruction{Op: OpStoreLocal, A: int(expr.ErrLocal)})
	if err := fl.lowerBlock(expr.Err, expr.Type); err != nil {
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
	if err := fl.lowerExpr(expr.Target); err != nil {
		return err
	}
	inst := Instruction{Op: op, A: int(fl.fn.Signature.Return), B: -1, C: int(expr.CatchLocal)}
	tryIndex := fl.emit(inst)
	if !expr.HasCatch {
		return nil
	}
	jumpNormal := fl.emit(Instruction{Op: OpJump})
	fl.patch(tryIndex, len(fl.code))
	if err := fl.lowerBlock(expr.Catch, fl.fn.Signature.Return); err != nil {
		return err
	}
	fl.emit(Instruction{Op: OpReturn})
	fl.patch(jumpNormal, len(fl.code))
	return nil
}

func (fl *functionLowerer) lowerListOp(expr *air.Expr) error {
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
	index := len(fl.code)
	fl.code = append(fl.code, inst)
	return index
}

func (fl *functionLowerer) patch(index int, target int) {
	fl.code[index].A = target
}
