package vm_next

import (
	"fmt"
	"strconv"

	"github.com/akonwi/ard/air"
)

type VM struct {
	program *air.Program
	externs hostExternAdapters
}

type TestStatus string

const (
	TestPass  TestStatus = "pass"
	TestFail  TestStatus = "fail"
	TestPanic TestStatus = "panic"
)

type TestOutcome struct {
	Name    string
	Status  TestStatus
	Message string
}

func New(program *air.Program) (*VM, error) {
	return NewWithExterns(program, nil)
}

func (vm *VM) RunEntry() (Value, error) {
	if vm.program.Entry == air.NoFunction {
		return vm.zeroValue(vm.mustTypeID(air.TypeVoid)), nil
	}
	return vm.call(vm.program.Entry, nil)
}

func (vm *VM) RunScript() (Value, error) {
	if vm.program.Script == air.NoFunction {
		return vm.zeroValue(vm.mustTypeID(air.TypeVoid)), nil
	}
	return vm.call(vm.program.Script, nil)
}

func (vm *VM) Call(name string, args ...Value) (Value, error) {
	for _, fn := range vm.program.Functions {
		if fn.Name == name {
			return vm.call(fn.ID, args)
		}
	}
	return Value{}, fmt.Errorf("function not found: %s", name)
}

func (vm *VM) RunTests() []TestOutcome {
	outcomes := make([]TestOutcome, 0, len(vm.program.Tests))
	for _, test := range vm.program.Tests {
		outcomes = append(outcomes, vm.runTest(test))
	}
	return outcomes
}

func (vm *VM) runTest(test air.Test) TestOutcome {
	outcome := TestOutcome{Name: test.Name, Status: TestPanic}
	value, err := vm.call(test.Function, nil)
	if err != nil {
		outcome.Message = err.Error()
		return outcome
	}
	result, err := value.resultValue()
	if err != nil {
		outcome.Message = err.Error()
		return outcome
	}
	if result.Ok {
		outcome.Status = TestPass
		return outcome
	}
	outcome.Status = TestFail
	outcome.Message = result.Value.GoValueString()
	return outcome
}

func (vm *VM) call(id air.FunctionID, args []Value) (Value, error) {
	if id < 0 || int(id) >= len(vm.program.Functions) {
		return Value{}, fmt.Errorf("invalid function id %d", id)
	}
	fn := vm.program.Functions[id]
	if len(args) != len(fn.Signature.Params) {
		return Value{}, fmt.Errorf("%s expects %d args, got %d", fn.Name, len(fn.Signature.Params), len(args))
	}
	frame := &frame{
		vm:     vm,
		fn:     fn,
		locals: make([]Value, len(fn.Locals)),
	}
	for i, arg := range args {
		frame.locals[i] = arg
	}
	value, err := frame.evalBlock(fn.Body)
	if ret, ok := err.(earlyReturn); ok {
		return ret.value, nil
	}
	return value, err
}

func (vm *VM) callClosure(value Value, args []Value) (Value, error) {
	closure, err := value.closureValue()
	if err != nil {
		return Value{}, err
	}
	if closure.Function < 0 || int(closure.Function) >= len(vm.program.Functions) {
		return Value{}, fmt.Errorf("invalid closure function id %d", closure.Function)
	}
	fn := vm.program.Functions[closure.Function]
	if len(args) != len(fn.Signature.Params) {
		return Value{}, fmt.Errorf("%s expects %d args, got %d", fn.Name, len(fn.Signature.Params), len(args))
	}
	if len(closure.Captures) != len(fn.Captures) {
		return Value{}, fmt.Errorf("%s expects %d captures, got %d", fn.Name, len(fn.Captures), len(closure.Captures))
	}
	frame := &frame{
		vm:     vm,
		fn:     fn,
		locals: make([]Value, len(fn.Locals)),
	}
	for i, arg := range args {
		frame.locals[i] = arg
	}
	for i, capture := range fn.Captures {
		if int(capture.Local) < 0 || int(capture.Local) >= len(frame.locals) {
			return Value{}, fmt.Errorf("%s capture %s has invalid local %d", fn.Name, capture.Name, capture.Local)
		}
		frame.locals[capture.Local] = closure.Captures[i]
	}
	result, err := frame.evalBlock(fn.Body)
	if ret, ok := err.(earlyReturn); ok {
		return ret.value, nil
	}
	return result, err
}

type frame struct {
	vm     *VM
	fn     air.Function
	locals []Value
}

type earlyReturn struct {
	value Value
}

func (e earlyReturn) Error() string {
	return "early return"
}

type loopBreak struct{}

func (l loopBreak) Error() string {
	return "break"
}

func (f *frame) evalBlock(block air.Block) (Value, error) {
	return f.evalBlockWithDefault(block, f.fn.Signature.Return)
}

func (f *frame) evalBlockWithDefault(block air.Block, defaultType air.TypeID) (Value, error) {
	for _, stmt := range block.Stmts {
		if _, err := f.evalStmt(stmt); err != nil {
			return Value{}, err
		}
	}
	if block.Result == nil {
		return f.vm.zeroValue(defaultType), nil
	}
	return f.evalExpr(*block.Result)
}

func (f *frame) evalStmt(stmt air.Stmt) (Value, error) {
	switch stmt.Kind {
	case air.StmtLet:
		value, err := f.evalExprPtr(stmt.Value)
		if err != nil {
			return Value{}, err
		}
		if int(stmt.Local) < 0 || int(stmt.Local) >= len(f.locals) {
			return Value{}, fmt.Errorf("invalid local id %d", stmt.Local)
		}
		f.locals[stmt.Local] = value
		return value, nil
	case air.StmtAssign:
		value, err := f.evalExprPtr(stmt.Value)
		if err != nil {
			return Value{}, err
		}
		if int(stmt.Local) < 0 || int(stmt.Local) >= len(f.locals) {
			return Value{}, fmt.Errorf("invalid local id %d", stmt.Local)
		}
		f.locals[stmt.Local] = value
		return value, nil
	case air.StmtExpr:
		return f.evalExprPtr(stmt.Expr)
	case air.StmtWhile:
		return f.evalWhile(stmt)
	case air.StmtBreak:
		return Value{}, loopBreak{}
	default:
		return Value{}, fmt.Errorf("unsupported stmt kind %d", stmt.Kind)
	}
}

func (f *frame) evalWhile(stmt air.Stmt) (Value, error) {
	for {
		condition, err := f.evalExprPtr(stmt.Condition)
		if err != nil {
			return Value{}, err
		}
		if condition.Kind != ValueBool {
			return Value{}, fmt.Errorf("while condition must be bool, got kind %d", condition.Kind)
		}
		if !condition.Bool {
			return f.vm.zeroValue(f.vm.mustTypeID(air.TypeVoid)), nil
		}
		if _, err := f.evalBlockWithDefault(stmt.Body, f.vm.mustTypeID(air.TypeVoid)); err != nil {
			if _, ok := err.(loopBreak); ok {
				return f.vm.zeroValue(f.vm.mustTypeID(air.TypeVoid)), nil
			}
			return Value{}, err
		}
	}
}

func (f *frame) evalExprPtr(expr *air.Expr) (Value, error) {
	if expr == nil {
		return f.vm.zeroValue(f.vm.mustTypeID(air.TypeVoid)), nil
	}
	return f.evalExpr(*expr)
}

func (f *frame) evalExpr(expr air.Expr) (Value, error) {
	switch expr.Kind {
	case air.ExprConstVoid:
		return Void(expr.Type), nil
	case air.ExprConstInt:
		return Int(expr.Type, expr.Int), nil
	case air.ExprConstFloat:
		return Float(expr.Type, expr.Float), nil
	case air.ExprConstBool:
		return Bool(expr.Type, expr.Bool), nil
	case air.ExprConstStr:
		return Str(expr.Type, expr.Str), nil
	case air.ExprEnumVariant:
		return Enum(expr.Type, expr.Discriminant), nil
	case air.ExprLoadLocal:
		if int(expr.Local) < 0 || int(expr.Local) >= len(f.locals) {
			return Value{}, fmt.Errorf("invalid local id %d", expr.Local)
		}
		return f.locals[expr.Local], nil
	case air.ExprCall:
		args, err := f.evalArgs(expr.Args)
		if err != nil {
			return Value{}, err
		}
		return f.vm.call(expr.Function, args)
	case air.ExprMakeClosure:
		return f.evalMakeClosure(expr)
	case air.ExprCallClosure:
		return f.evalCallClosure(expr)
	case air.ExprSpawnFiber:
		return f.evalSpawnFiber(expr)
	case air.ExprFiberGet:
		return f.evalFiberGet(expr)
	case air.ExprFiberJoin:
		return f.evalFiberJoin(expr)
	case air.ExprUnionWrap:
		return f.evalUnionWrap(expr)
	case air.ExprMatchUnion:
		return f.evalUnionMatch(expr)
	case air.ExprTraitUpcast:
		return f.evalTraitUpcast(expr)
	case air.ExprCallTrait:
		return f.evalTraitCall(expr)
	case air.ExprCallExtern:
		args, err := f.evalArgs(expr.Args)
		if err != nil {
			return Value{}, err
		}
		return f.vm.callExtern(expr.Extern, args)
	case air.ExprMakeList:
		return f.evalMakeList(expr)
	case air.ExprMakeMap:
		return f.evalMakeMap(expr)
	case air.ExprMakeStruct:
		return f.evalMakeStruct(expr)
	case air.ExprGetField:
		return f.evalGetField(expr)
	case air.ExprIntAdd, air.ExprIntSub, air.ExprIntMul, air.ExprIntDiv, air.ExprIntMod:
		return f.evalIntBinary(expr)
	case air.ExprFloatAdd, air.ExprFloatSub, air.ExprFloatMul, air.ExprFloatDiv:
		return f.evalFloatBinary(expr)
	case air.ExprStrConcat:
		left, right, err := f.evalBinaryOperands(expr)
		if err != nil {
			return Value{}, err
		}
		return Str(expr.Type, left.Str+right.Str), nil
	case air.ExprToStr:
		return f.evalToStr(expr)
	case air.ExprEq, air.ExprNotEq:
		return f.evalEquality(expr)
	case air.ExprLt, air.ExprLte, air.ExprGt, air.ExprGte:
		return f.evalComparison(expr)
	case air.ExprAnd, air.ExprOr:
		return f.evalBoolBinary(expr)
	case air.ExprNot:
		target, err := f.evalExprPtr(expr.Target)
		if err != nil {
			return Value{}, err
		}
		return Bool(expr.Type, !target.Bool), nil
	case air.ExprNeg:
		target, err := f.evalExprPtr(expr.Target)
		if err != nil {
			return Value{}, err
		}
		switch target.Kind {
		case ValueInt:
			return Int(expr.Type, -target.Int), nil
		case ValueFloat:
			return Float(expr.Type, -target.Float), nil
		default:
			return Value{}, fmt.Errorf("cannot negate value kind %d", target.Kind)
		}
	case air.ExprIf:
		condition, err := f.evalExprPtr(expr.Condition)
		if err != nil {
			return Value{}, err
		}
		if condition.Kind != ValueBool {
			return Value{}, fmt.Errorf("if condition must be bool, got kind %d", condition.Kind)
		}
		if condition.Bool {
			return f.evalBlockWithDefault(expr.Then, expr.Type)
		}
		return f.evalBlockWithDefault(expr.Else, expr.Type)
	case air.ExprMakeResultOk, air.ExprMakeResultErr:
		value, err := f.evalExprPtr(expr.Target)
		if err != nil {
			return Value{}, err
		}
		return Result(expr.Type, expr.Kind == air.ExprMakeResultOk, value), nil
	case air.ExprMatchEnum:
		return f.evalEnumMatch(expr)
	case air.ExprMakeMaybeSome:
		value, err := f.evalExprPtr(expr.Target)
		if err != nil {
			return Value{}, err
		}
		return Maybe(expr.Type, true, value), nil
	case air.ExprMakeMaybeNone:
		return Maybe(expr.Type, false, f.vm.zeroValue(f.mustMaybeElem(expr.Type))), nil
	case air.ExprMatchMaybe:
		return f.evalMaybeMatch(expr)
	case air.ExprMaybeExpect:
		return f.evalMaybeExpect(expr)
	case air.ExprMaybeIsNone, air.ExprMaybeIsSome:
		return f.evalMaybePredicate(expr)
	case air.ExprMaybeOr:
		return f.evalMaybeOr(expr)
	case air.ExprMaybeMap, air.ExprMaybeAndThen:
		return f.evalMaybeMap(expr)
	case air.ExprMatchResult:
		return f.evalResultMatch(expr)
	case air.ExprResultExpect:
		return f.evalResultExpect(expr)
	case air.ExprResultOr:
		return f.evalResultOr(expr)
	case air.ExprResultIsOk, air.ExprResultIsErr:
		return f.evalResultPredicate(expr)
	case air.ExprResultMap, air.ExprResultMapErr, air.ExprResultAndThen:
		return f.evalResultMap(expr)
	case air.ExprTryResult:
		return f.evalTryResult(expr)
	case air.ExprTryMaybe:
		return f.evalTryMaybe(expr)
	default:
		return Value{}, fmt.Errorf("unsupported expr kind %d", expr.Kind)
	}
}

func (f *frame) evalArgs(args []air.Expr) ([]Value, error) {
	out := make([]Value, len(args))
	for i, arg := range args {
		value, err := f.evalExpr(arg)
		if err != nil {
			return nil, err
		}
		out[i] = value
	}
	return out, nil
}

func (f *frame) evalMakeClosure(expr air.Expr) (Value, error) {
	captures := make([]Value, len(expr.CaptureLocals))
	for i, local := range expr.CaptureLocals {
		if int(local) < 0 || int(local) >= len(f.locals) {
			return Value{}, fmt.Errorf("invalid closure capture local id %d", local)
		}
		captures[i] = f.locals[local]
	}
	return Closure(expr.Type, expr.Function, captures), nil
}

func (f *frame) evalCallClosure(expr air.Expr) (Value, error) {
	target, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	args, err := f.evalArgs(expr.Args)
	if err != nil {
		return Value{}, err
	}
	return f.vm.callClosure(target, args)
}

func (f *frame) evalSpawnFiber(expr air.Expr) (Value, error) {
	fiber := &FiberValue{
		Type: expr.Type,
		Done: make(chan struct{}),
	}
	if expr.Target != nil {
		target, err := f.evalExprPtr(expr.Target)
		if err != nil {
			return Value{}, err
		}
		go func() {
			defer close(fiber.Done)
			fiber.Result, fiber.Err = f.vm.callClosure(target, nil)
		}()
		return Fiber(expr.Type, fiber), nil
	}
	if expr.Function < 0 {
		return Value{}, fmt.Errorf("fiber spawn missing target")
	}
	go func() {
		defer close(fiber.Done)
		fiber.Result, fiber.Err = f.vm.call(expr.Function, nil)
	}()
	return Fiber(expr.Type, fiber), nil
}

func (f *frame) evalFiberGet(expr air.Expr) (Value, error) {
	target, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	fiber, err := target.fiberValue()
	if err != nil {
		return Value{}, err
	}
	<-fiber.Done
	if fiber.Err != nil {
		return Value{}, fiber.Err
	}
	return fiber.Result, nil
}

func (f *frame) evalFiberJoin(expr air.Expr) (Value, error) {
	target, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	fiber, err := target.fiberValue()
	if err != nil {
		return Value{}, err
	}
	<-fiber.Done
	if fiber.Err != nil {
		return Value{}, fiber.Err
	}
	return f.vm.zeroValue(expr.Type), nil
}

func (f *frame) evalMakeStruct(expr air.Expr) (Value, error) {
	typeInfo, err := f.vm.typeInfo(expr.Type)
	if err != nil {
		return Value{}, err
	}
	fields := make([]Value, len(typeInfo.Fields))
	for _, field := range typeInfo.Fields {
		if field.Index < 0 || field.Index >= len(fields) {
			return Value{}, fmt.Errorf("invalid field index %d on %s", field.Index, typeInfo.Name)
		}
		fields[field.Index] = f.vm.zeroValue(field.Type)
	}
	for _, field := range expr.Fields {
		if field.Index < 0 || field.Index >= len(fields) {
			return Value{}, fmt.Errorf("invalid struct field index %d", field.Index)
		}
		value, err := f.evalExpr(field.Value)
		if err != nil {
			return Value{}, err
		}
		fields[field.Index] = value
	}
	return Struct(expr.Type, fields), nil
}

func (f *frame) evalUnionWrap(expr air.Expr) (Value, error) {
	value, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	return Union(expr.Type, expr.Tag, value), nil
}

func (f *frame) evalTraitUpcast(expr air.Expr) (Value, error) {
	value, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	return TraitObject(expr.Type, expr.Trait, expr.Impl, value), nil
}

func (f *frame) evalToStr(expr air.Expr) (Value, error) {
	value, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	switch value.Kind {
	case ValueStr:
		return Str(expr.Type, value.Str), nil
	case ValueInt:
		return Str(expr.Type, strconv.Itoa(value.Int)), nil
	case ValueFloat:
		return Str(expr.Type, strconv.FormatFloat(value.Float, 'f', -1, 64)), nil
	case ValueBool:
		return Str(expr.Type, strconv.FormatBool(value.Bool)), nil
	default:
		return Value{}, fmt.Errorf("cannot convert value kind %d to Str", value.Kind)
	}
}

func (f *frame) evalMakeList(expr air.Expr) (Value, error) {
	items := make([]Value, len(expr.Args))
	for i, item := range expr.Args {
		value, err := f.evalExpr(item)
		if err != nil {
			return Value{}, err
		}
		items[i] = value
	}
	return List(expr.Type, items), nil
}

func (f *frame) evalMakeMap(expr air.Expr) (Value, error) {
	entries := make([]MapEntryValue, len(expr.Entries))
	for i, entry := range expr.Entries {
		key, err := f.evalExpr(entry.Key)
		if err != nil {
			return Value{}, err
		}
		value, err := f.evalExpr(entry.Value)
		if err != nil {
			return Value{}, err
		}
		entries[i] = MapEntryValue{Key: key, Value: value}
	}
	return Map(expr.Type, entries), nil
}

func (f *frame) evalGetField(expr air.Expr) (Value, error) {
	target, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	structValue, err := target.structValue()
	if err != nil {
		return Value{}, err
	}
	if expr.Field < 0 || expr.Field >= len(structValue.Fields) {
		return Value{}, fmt.Errorf("invalid field index %d", expr.Field)
	}
	return structValue.Fields[expr.Field], nil
}

func (f *frame) evalEnumMatch(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	if subject.Kind != ValueEnum {
		return Value{}, fmt.Errorf("enum match subject must be enum, got kind %d", subject.Kind)
	}
	for _, matchCase := range expr.EnumCases {
		if subject.Int == matchCase.Discriminant {
			return f.evalBlockWithDefault(matchCase.Body, expr.Type)
		}
	}
	if expr.CatchAll.Result != nil || len(expr.CatchAll.Stmts) > 0 {
		return f.evalBlockWithDefault(expr.CatchAll, expr.Type)
	}
	return f.vm.zeroValue(expr.Type), nil
}

func (f *frame) evalUnionMatch(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	unionValue, err := subject.unionValue()
	if err != nil {
		return Value{}, err
	}
	for _, matchCase := range expr.UnionCases {
		if unionValue.Tag == matchCase.Tag {
			if int(matchCase.Local) < 0 || int(matchCase.Local) >= len(f.locals) {
				return Value{}, fmt.Errorf("invalid union match local id %d", matchCase.Local)
			}
			f.locals[matchCase.Local] = unionValue.Value
			return f.evalBlockWithDefault(matchCase.Body, expr.Type)
		}
	}
	if expr.CatchAll.Result != nil || len(expr.CatchAll.Stmts) > 0 {
		return f.evalBlockWithDefault(expr.CatchAll, expr.Type)
	}
	return f.vm.zeroValue(expr.Type), nil
}

func (f *frame) evalTraitCall(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	traitObject, err := subject.traitObjectValue()
	if err != nil {
		return Value{}, err
	}
	if traitObject.Trait != expr.Trait {
		return Value{}, fmt.Errorf("trait object has trait %d, call expects %d", traitObject.Trait, expr.Trait)
	}
	if traitObject.Impl < 0 || int(traitObject.Impl) >= len(f.vm.program.Impls) {
		return Value{}, fmt.Errorf("invalid trait object impl %d", traitObject.Impl)
	}
	impl := f.vm.program.Impls[traitObject.Impl]
	if impl.Trait != expr.Trait {
		return Value{}, fmt.Errorf("impl %d has trait %d, call expects %d", traitObject.Impl, impl.Trait, expr.Trait)
	}
	if expr.Method < 0 || expr.Method >= len(impl.Methods) {
		return Value{}, fmt.Errorf("invalid trait method index %d for impl %d", expr.Method, traitObject.Impl)
	}
	args, err := f.evalArgs(expr.Args)
	if err != nil {
		return Value{}, err
	}
	args = append([]Value{traitObject.Value}, args...)
	return f.vm.call(impl.Methods[expr.Method], args)
}

func (f *frame) evalMaybeMatch(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	maybeValue, err := subject.maybeValue()
	if err != nil {
		return Value{}, err
	}
	if maybeValue.Some {
		if int(expr.SomeLocal) < 0 || int(expr.SomeLocal) >= len(f.locals) {
			return Value{}, fmt.Errorf("invalid Maybe match local id %d", expr.SomeLocal)
		}
		f.locals[expr.SomeLocal] = maybeValue.Value
		return f.evalBlockWithDefault(expr.Some, expr.Type)
	}
	return f.evalBlockWithDefault(expr.None, expr.Type)
}

func (f *frame) evalMaybeExpect(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	maybeValue, err := subject.maybeValue()
	if err != nil {
		return Value{}, err
	}
	if maybeValue.Some {
		return maybeValue.Value, nil
	}
	message := "expected Maybe to contain a value"
	if len(expr.Args) > 0 {
		arg, err := f.evalExpr(expr.Args[0])
		if err != nil {
			return Value{}, err
		}
		message = arg.GoValueString()
	}
	return Value{}, fmt.Errorf("%s", message)
}

func (f *frame) evalMaybePredicate(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	maybeValue, err := subject.maybeValue()
	if err != nil {
		return Value{}, err
	}
	return Bool(expr.Type, expr.Kind == air.ExprMaybeIsSome && maybeValue.Some || expr.Kind == air.ExprMaybeIsNone && !maybeValue.Some), nil
}

func (f *frame) evalMaybeOr(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	maybeValue, err := subject.maybeValue()
	if err != nil {
		return Value{}, err
	}
	if maybeValue.Some {
		return maybeValue.Value, nil
	}
	if len(expr.Args) != 1 {
		return Value{}, fmt.Errorf("Maybe.or expects one fallback argument, got %d", len(expr.Args))
	}
	return f.evalExpr(expr.Args[0])
}

func (f *frame) evalMaybeMap(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	maybeValue, err := subject.maybeValue()
	if err != nil {
		return Value{}, err
	}
	if len(expr.Args) != 1 {
		return Value{}, fmt.Errorf("Maybe closure method expects one function argument, got %d", len(expr.Args))
	}
	if !maybeValue.Some {
		return Maybe(expr.Type, false, f.vm.zeroValue(f.mustMaybeElem(expr.Type))), nil
	}
	closure, err := f.evalExpr(expr.Args[0])
	if err != nil {
		return Value{}, err
	}
	mapped, err := f.vm.callClosure(closure, []Value{maybeValue.Value})
	if err != nil {
		return Value{}, err
	}
	if expr.Kind == air.ExprMaybeAndThen {
		return mapped, nil
	}
	return Maybe(expr.Type, true, mapped), nil
}

func (f *frame) evalResultMatch(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	resultValue, err := subject.resultValue()
	if err != nil {
		return Value{}, err
	}
	if resultValue.Ok {
		if int(expr.OkLocal) < 0 || int(expr.OkLocal) >= len(f.locals) {
			return Value{}, fmt.Errorf("invalid Result match ok local id %d", expr.OkLocal)
		}
		f.locals[expr.OkLocal] = resultValue.Value
		return f.evalBlockWithDefault(expr.Ok, expr.Type)
	}
	if int(expr.ErrLocal) < 0 || int(expr.ErrLocal) >= len(f.locals) {
		return Value{}, fmt.Errorf("invalid Result match err local id %d", expr.ErrLocal)
	}
	f.locals[expr.ErrLocal] = resultValue.Value
	return f.evalBlockWithDefault(expr.Err, expr.Type)
}

func (f *frame) evalResultExpect(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	resultValue, err := subject.resultValue()
	if err != nil {
		return Value{}, err
	}
	if resultValue.Ok {
		return resultValue.Value, nil
	}
	message := "expected Result to be ok"
	if len(expr.Args) > 0 {
		arg, err := f.evalExpr(expr.Args[0])
		if err != nil {
			return Value{}, err
		}
		message = arg.GoValueString()
	}
	return Value{}, fmt.Errorf("%s", message)
}

func (f *frame) evalResultOr(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	resultValue, err := subject.resultValue()
	if err != nil {
		return Value{}, err
	}
	if resultValue.Ok {
		return resultValue.Value, nil
	}
	if len(expr.Args) != 1 {
		return Value{}, fmt.Errorf("Result.or expects one fallback argument, got %d", len(expr.Args))
	}
	return f.evalExpr(expr.Args[0])
}

func (f *frame) evalResultPredicate(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	resultValue, err := subject.resultValue()
	if err != nil {
		return Value{}, err
	}
	return Bool(expr.Type, expr.Kind == air.ExprResultIsOk && resultValue.Ok || expr.Kind == air.ExprResultIsErr && !resultValue.Ok), nil
}

func (f *frame) evalResultMap(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	resultValue, err := subject.resultValue()
	if err != nil {
		return Value{}, err
	}
	if len(expr.Args) != 1 {
		return Value{}, fmt.Errorf("Result closure method expects one function argument, got %d", len(expr.Args))
	}
	switch expr.Kind {
	case air.ExprResultMap:
		if !resultValue.Ok {
			return Result(expr.Type, false, resultValue.Value), nil
		}
		closure, err := f.evalExpr(expr.Args[0])
		if err != nil {
			return Value{}, err
		}
		mapped, err := f.vm.callClosure(closure, []Value{resultValue.Value})
		if err != nil {
			return Value{}, err
		}
		return Result(expr.Type, true, mapped), nil
	case air.ExprResultMapErr:
		if resultValue.Ok {
			return Result(expr.Type, true, resultValue.Value), nil
		}
		closure, err := f.evalExpr(expr.Args[0])
		if err != nil {
			return Value{}, err
		}
		mapped, err := f.vm.callClosure(closure, []Value{resultValue.Value})
		if err != nil {
			return Value{}, err
		}
		return Result(expr.Type, false, mapped), nil
	case air.ExprResultAndThen:
		if !resultValue.Ok {
			return Result(expr.Type, false, resultValue.Value), nil
		}
		closure, err := f.evalExpr(expr.Args[0])
		if err != nil {
			return Value{}, err
		}
		return f.vm.callClosure(closure, []Value{resultValue.Value})
	default:
		return Value{}, fmt.Errorf("unsupported Result closure method %d", expr.Kind)
	}
}

func (f *frame) evalTryResult(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	resultValue, err := subject.resultValue()
	if err != nil {
		return Value{}, err
	}
	if resultValue.Ok {
		return resultValue.Value, nil
	}
	if expr.HasCatch {
		if int(expr.CatchLocal) < 0 || int(expr.CatchLocal) >= len(f.locals) {
			return Value{}, fmt.Errorf("invalid Result try catch local id %d", expr.CatchLocal)
		}
		f.locals[expr.CatchLocal] = resultValue.Value
		value, err := f.evalBlockWithDefault(expr.Catch, f.fn.Signature.Return)
		if err != nil {
			return Value{}, err
		}
		return Value{}, earlyReturn{value: value}
	}
	return Value{}, earlyReturn{value: Result(f.fn.Signature.Return, false, resultValue.Value)}
}

func (f *frame) evalTryMaybe(expr air.Expr) (Value, error) {
	subject, err := f.evalExprPtr(expr.Target)
	if err != nil {
		return Value{}, err
	}
	maybeValue, err := subject.maybeValue()
	if err != nil {
		return Value{}, err
	}
	if maybeValue.Some {
		return maybeValue.Value, nil
	}
	if expr.HasCatch {
		value, err := f.evalBlockWithDefault(expr.Catch, f.fn.Signature.Return)
		if err != nil {
			return Value{}, err
		}
		return Value{}, earlyReturn{value: value}
	}
	return Value{}, earlyReturn{value: Maybe(f.fn.Signature.Return, false, f.zeroMaybeForFunctionReturn())}
}

func (f *frame) evalIntBinary(expr air.Expr) (Value, error) {
	left, right, err := f.evalBinaryOperands(expr)
	if err != nil {
		return Value{}, err
	}
	switch expr.Kind {
	case air.ExprIntAdd:
		return Int(expr.Type, left.Int+right.Int), nil
	case air.ExprIntSub:
		return Int(expr.Type, left.Int-right.Int), nil
	case air.ExprIntMul:
		return Int(expr.Type, left.Int*right.Int), nil
	case air.ExprIntDiv:
		return Int(expr.Type, left.Int/right.Int), nil
	case air.ExprIntMod:
		return Int(expr.Type, left.Int%right.Int), nil
	default:
		return Value{}, fmt.Errorf("unsupported int op %d", expr.Kind)
	}
}

func (f *frame) evalFloatBinary(expr air.Expr) (Value, error) {
	left, right, err := f.evalBinaryOperands(expr)
	if err != nil {
		return Value{}, err
	}
	switch expr.Kind {
	case air.ExprFloatAdd:
		return Float(expr.Type, left.Float+right.Float), nil
	case air.ExprFloatSub:
		return Float(expr.Type, left.Float-right.Float), nil
	case air.ExprFloatMul:
		return Float(expr.Type, left.Float*right.Float), nil
	case air.ExprFloatDiv:
		return Float(expr.Type, left.Float/right.Float), nil
	default:
		return Value{}, fmt.Errorf("unsupported float op %d", expr.Kind)
	}
}

func (f *frame) evalEquality(expr air.Expr) (Value, error) {
	left, right, err := f.evalBinaryOperands(expr)
	if err != nil {
		return Value{}, err
	}
	equal := valuesEqual(left, right)
	if expr.Kind == air.ExprNotEq {
		equal = !equal
	}
	return Bool(expr.Type, equal), nil
}

func valuesEqual(left, right Value) bool {
	if left.Kind == ValueMaybe && right.Kind == ValueMaybe {
		leftMaybe, leftOK := left.Ref.(*MaybeValue)
		rightMaybe, rightOK := right.Ref.(*MaybeValue)
		if !leftOK || !rightOK {
			return false
		}
		if leftMaybe.Some != rightMaybe.Some {
			return false
		}
		if !leftMaybe.Some {
			return true
		}
		return valuesEqual(leftMaybe.Value, rightMaybe.Value)
	}
	if left.Kind != right.Kind {
		if (left.Kind == ValueInt || left.Kind == ValueEnum) && (right.Kind == ValueInt || right.Kind == ValueEnum) {
			return left.Int == right.Int
		}
		return false
	}
	switch left.Kind {
	case ValueVoid:
		return true
	case ValueInt, ValueEnum:
		return left.Int == right.Int
	case ValueFloat:
		return left.Float == right.Float
	case ValueBool:
		return left.Bool == right.Bool
	case ValueStr:
		return left.Str == right.Str
	case ValueStruct:
		leftStruct, leftOK := left.Ref.(*StructValue)
		rightStruct, rightOK := right.Ref.(*StructValue)
		if !leftOK || !rightOK || len(leftStruct.Fields) != len(rightStruct.Fields) {
			return false
		}
		for i := range leftStruct.Fields {
			if !valuesEqual(leftStruct.Fields[i], rightStruct.Fields[i]) {
				return false
			}
		}
		return true
	case ValueResult:
		leftResult, leftOK := left.Ref.(*ResultValue)
		rightResult, rightOK := right.Ref.(*ResultValue)
		if !leftOK || !rightOK || leftResult.Ok != rightResult.Ok {
			return false
		}
		return valuesEqual(leftResult.Value, rightResult.Value)
	default:
		return left.GoValue() == right.GoValue()
	}
}

func (f *frame) evalComparison(expr air.Expr) (Value, error) {
	left, right, err := f.evalBinaryOperands(expr)
	if err != nil {
		return Value{}, err
	}
	var result bool
	switch left.Kind {
	case ValueInt, ValueEnum:
		switch expr.Kind {
		case air.ExprLt:
			result = left.Int < right.Int
		case air.ExprLte:
			result = left.Int <= right.Int
		case air.ExprGt:
			result = left.Int > right.Int
		case air.ExprGte:
			result = left.Int >= right.Int
		}
	case ValueFloat:
		switch expr.Kind {
		case air.ExprLt:
			result = left.Float < right.Float
		case air.ExprLte:
			result = left.Float <= right.Float
		case air.ExprGt:
			result = left.Float > right.Float
		case air.ExprGte:
			result = left.Float >= right.Float
		}
	default:
		return Value{}, fmt.Errorf("cannot compare value kind %d", left.Kind)
	}
	return Bool(expr.Type, result), nil
}

func (f *frame) evalBoolBinary(expr air.Expr) (Value, error) {
	left, right, err := f.evalBinaryOperands(expr)
	if err != nil {
		return Value{}, err
	}
	switch expr.Kind {
	case air.ExprAnd:
		return Bool(expr.Type, left.Bool && right.Bool), nil
	case air.ExprOr:
		return Bool(expr.Type, left.Bool || right.Bool), nil
	default:
		return Value{}, fmt.Errorf("unsupported bool op %d", expr.Kind)
	}
}

func (f *frame) evalBinaryOperands(expr air.Expr) (Value, Value, error) {
	if expr.Left == nil || expr.Right == nil {
		return Value{}, Value{}, fmt.Errorf("binary expression %d missing operand", expr.Kind)
	}
	left, err := f.evalExprPtr(expr.Left)
	if err != nil {
		return Value{}, Value{}, err
	}
	right, err := f.evalExprPtr(expr.Right)
	if err != nil {
		return Value{}, Value{}, err
	}
	return left, right, nil
}

func (vm *VM) typeInfo(id air.TypeID) (air.TypeInfo, error) {
	if id <= 0 || int(id) > len(vm.program.Types) {
		return air.TypeInfo{}, fmt.Errorf("invalid type id %d", id)
	}
	return vm.program.Types[id-1], nil
}

func (vm *VM) mustTypeID(kind air.TypeKind) air.TypeID {
	for _, typ := range vm.program.Types {
		if typ.Kind == kind {
			return typ.ID
		}
	}
	return air.NoType
}

func (vm *VM) zeroValue(typeID air.TypeID) Value {
	typeInfo, err := vm.typeInfo(typeID)
	if err != nil {
		return Value{}
	}
	switch typeInfo.Kind {
	case air.TypeVoid:
		return Void(typeID)
	case air.TypeInt:
		return Int(typeID, 0)
	case air.TypeFloat:
		return Float(typeID, 0)
	case air.TypeBool:
		return Bool(typeID, false)
	case air.TypeStr:
		return Str(typeID, "")
	case air.TypeList:
		return List(typeID, nil)
	case air.TypeMap:
		return Map(typeID, nil)
	case air.TypeDynamic:
		return Dynamic(typeID, nil)
	case air.TypeFiber:
		return Fiber(typeID, &FiberValue{Type: typeID, Done: closedFiberDone()})
	case air.TypeEnum:
		if len(typeInfo.Variants) == 0 {
			return Enum(typeID, 0)
		}
		return Enum(typeID, typeInfo.Variants[0].Discriminant)
	case air.TypeMaybe:
		return Maybe(typeID, false, vm.zeroValue(typeInfo.Elem))
	case air.TypeStruct:
		fields := make([]Value, len(typeInfo.Fields))
		for _, field := range typeInfo.Fields {
			fields[field.Index] = vm.zeroValue(field.Type)
		}
		return Struct(typeID, fields)
	case air.TypeResult:
		return Result(typeID, true, vm.zeroValue(typeInfo.Value))
	case air.TypeUnion:
		if len(typeInfo.Members) == 0 {
			return Value{Type: typeID}
		}
		member := typeInfo.Members[0]
		return Union(typeID, member.Tag, vm.zeroValue(member.Type))
	case air.TypeTraitObject:
		return Value{Type: typeID, Kind: ValueTraitObject}
	case air.TypeExtern:
		return Extern(typeID, nil)
	default:
		return Value{Type: typeID}
	}
}

func closedFiberDone() chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

func (f *frame) mustMaybeElem(typeID air.TypeID) air.TypeID {
	typeInfo, err := f.vm.typeInfo(typeID)
	if err != nil || typeInfo.Kind != air.TypeMaybe {
		return f.vm.mustTypeID(air.TypeVoid)
	}
	return typeInfo.Elem
}

func (f *frame) zeroMaybeForFunctionReturn() Value {
	typeInfo, err := f.vm.typeInfo(f.fn.Signature.Return)
	if err != nil || typeInfo.Kind != air.TypeMaybe {
		return f.vm.zeroValue(f.vm.mustTypeID(air.TypeVoid))
	}
	return f.vm.zeroValue(typeInfo.Elem)
}
