package vm_next

import (
	"fmt"

	"github.com/akonwi/ard/air"
)

type VM struct {
	program *air.Program
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
	if err := air.Validate(program); err != nil {
		return nil, err
	}
	return &VM{program: program}, nil
}

func (vm *VM) RunEntry() (Value, error) {
	if vm.program.Entry == air.NoFunction {
		return vm.zeroValue(vm.mustTypeID(air.TypeVoid)), nil
	}
	return vm.call(vm.program.Entry, nil)
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
	return frame.evalBlock(fn.Body)
}

type frame struct {
	vm     *VM
	fn     air.Function
	locals []Value
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
	default:
		return Value{}, fmt.Errorf("unsupported stmt kind %d", stmt.Kind)
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
	equal := left.GoValue() == right.GoValue()
	if expr.Kind == air.ExprNotEq {
		equal = !equal
	}
	return Bool(expr.Type, equal), nil
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
	case air.TypeEnum:
		if len(typeInfo.Variants) == 0 {
			return Enum(typeID, 0)
		}
		return Enum(typeID, typeInfo.Variants[0].Discriminant)
	case air.TypeStruct:
		fields := make([]Value, len(typeInfo.Fields))
		for _, field := range typeInfo.Fields {
			fields[field.Index] = vm.zeroValue(field.Type)
		}
		return Struct(typeID, fields)
	case air.TypeResult:
		return Result(typeID, true, vm.zeroValue(typeInfo.Value))
	default:
		return Value{Type: typeID}
	}
}
