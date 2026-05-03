package vm_next

import (
	"fmt"
	"sort"
	"sync"

	"github.com/akonwi/ard/air"
	vmcode "github.com/akonwi/ard/vm_next/bytecode"
)

type VM struct {
	program     *air.Program
	bytecode    *vmcode.Program
	externs     hostExternAdapters
	profile     *executionProfile
	valueSlices sync.Pool
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
	return vm.runBytecode(vm.program.Entry, nil)
}

func (vm *VM) RunScript() (Value, error) {
	if vm.program.Script == air.NoFunction {
		return vm.zeroValue(vm.mustTypeID(air.TypeVoid)), nil
	}
	return vm.runBytecode(vm.program.Script, nil)
}

func (vm *VM) Call(name string, args ...Value) (Value, error) {
	for _, fn := range vm.program.Functions {
		if fn.Name == name {
			return vm.runBytecode(fn.ID, args)
		}
	}
	return Value{}, fmt.Errorf("function not found: %s", name)
}

func (vm *VM) ProfileReport() string {
	if vm == nil || vm.profile == nil {
		return ""
	}
	return vm.profile.Report()
}

func (vm *VM) recordMaybeAlloc(some bool) {
	if vm == nil || vm.profile == nil {
		return
	}
	vm.profile.RecordMaybeAlloc(some)
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
	value, err := vm.runBytecode(test.Function, nil)
	if err != nil {
		outcome.Message = err.Error()
		return outcome
	}
	resultOK, resultValue, err := value.resultParts()
	if err != nil {
		outcome.Message = err.Error()
		return outcome
	}
	if resultOK {
		outcome.Status = TestPass
		return outcome
	}
	outcome.Status = TestFail
	outcome.Message = resultValue.GoValueString()
	return outcome
}

func (vm *VM) callClosure(value Value, args []Value) (Value, error) {
	vm.recordRefAccess(refAccessClosure)
	closure, err := value.closureValue()
	if err != nil {
		return Value{}, err
	}
	return vm.runBytecodeClosure(closure, args)
}

func (vm *VM) callClosure1(value Value, arg Value) (Value, error) {
	vm.recordRefAccess(refAccessClosure)
	closure, err := value.closureValue()
	if err != nil {
		return Value{}, err
	}
	return vm.runBytecodeClosure1(closure, arg)
}

func copyValue(value Value) Value {
	switch value.Kind {
	case ValueStruct:
		structValue, ok := value.Ref.(*StructValue)
		if !ok || structValue == nil {
			return value
		}
		fields := make([]Value, len(structValue.Fields))
		for i, field := range structValue.Fields {
			fields[i] = copyValue(field)
		}
		return Struct(value.Type, fields)
	case ValueList:
		listValue, ok := value.Ref.(*ListValue)
		if !ok || listValue == nil {
			return value
		}
		items := make([]Value, len(listValue.Items))
		for i, item := range listValue.Items {
			items[i] = copyValue(item)
		}
		return List(value.Type, items)
	case ValueMap:
		mapValue, ok := value.Ref.(*MapValue)
		if !ok || mapValue == nil {
			return value
		}
		entries := make([]MapEntryValue, len(mapValue.Entries))
		for i, entry := range mapValue.Entries {
			entries[i] = MapEntryValue{Key: copyValue(entry.Key), Value: copyValue(entry.Value)}
		}
		return Map(value.Type, entries)
	case ValueMaybe:
		maybeValue, ok := value.Ref.(*MaybeValue)
		if !ok || maybeValue == nil {
			return value
		}
		return Maybe(value.Type, maybeValue.Some, copyValue(maybeValue.Value))
	case ValueResult, ValueResultInt, ValueResultStr, ValueResultBool, ValueResultFloat:
		resultOK, resultValue, err := value.resultParts()
		if err != nil {
			return value
		}
		return Result(value.Type, resultOK, copyValue(resultValue))
	case ValueUnion:
		unionValue, ok := value.Ref.(*UnionValue)
		if !ok || unionValue == nil {
			return value
		}
		return Union(value.Type, unionValue.Tag, copyValue(unionValue.Value))
	default:
		return value
	}
}

func sortedMapEntries(mapValue *MapValue) []MapEntryValue {
	if mapValue == nil {
		return nil
	}
	if !mapValue.SortedDirty && len(mapValue.SortedEntries) == len(mapValue.Entries) {
		return mapValue.SortedEntries
	}
	entries := make([]MapEntryValue, len(mapValue.Entries))
	copy(entries, mapValue.Entries)
	sort.SliceStable(entries, func(i, j int) bool {
		return valuesLess(entries[i].Key, entries[j].Key)
	})
	mapValue.SortedEntries = entries
	mapValue.SortedDirty = false
	return entries
}

func mapEntryIndex(mapValue *MapValue, key Value) int {
	for i, entry := range mapValue.Entries {
		if valuesEqual(entry.Key, key) {
			return i
		}
	}
	return -1
}

func valuesEqual(left, right Value) bool {
	if (left.Kind == ValueResult || left.Kind == ValueResultInt || left.Kind == ValueResultStr || left.Kind == ValueResultBool || left.Kind == ValueResultFloat) && (right.Kind == ValueResult || right.Kind == ValueResultInt || right.Kind == ValueResultStr || right.Kind == ValueResultBool || right.Kind == ValueResultFloat) {
		leftOKTag, leftValue, leftErr := left.resultParts()
		rightOKTag, rightValue, rightErr := right.resultParts()
		if leftErr != nil || rightErr != nil || leftOKTag != rightOKTag {
			return false
		}
		return valuesEqual(leftValue, rightValue)
	}
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
	default:
		return left.GoValue() == right.GoValue()
	}
}

func valuesLess(left, right Value) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	switch left.Kind {
	case ValueInt, ValueEnum:
		return left.Int < right.Int
	case ValueFloat:
		return left.Float < right.Float
	case ValueBool:
		return !left.Bool && right.Bool
	case ValueStr:
		return left.Str < right.Str
	default:
		return fmt.Sprint(left.GoValue()) < fmt.Sprint(right.GoValue())
	}
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
	vm.recordZeroValue(zeroValueProfileKind(typeInfo.Kind))
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
		vm.recordMaybeDetailAlloc(false)
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

func zeroValueProfileKind(kind air.TypeKind) zeroValueKind {
	switch kind {
	case air.TypeVoid:
		return zeroValueVoid
	case air.TypeInt, air.TypeFloat, air.TypeBool, air.TypeStr:
		return zeroValueScalar
	case air.TypeList:
		return zeroValueList
	case air.TypeMap:
		return zeroValueMap
	case air.TypeDynamic:
		return zeroValueDynamic
	case air.TypeFiber:
		return zeroValueFiber
	case air.TypeEnum:
		return zeroValueEnum
	case air.TypeMaybe:
		return zeroValueMaybe
	case air.TypeStruct:
		return zeroValueStruct
	case air.TypeResult:
		return zeroValueResult
	case air.TypeUnion:
		return zeroValueUnion
	case air.TypeTraitObject:
		return zeroValueTraitObject
	case air.TypeExtern:
		return zeroValueExtern
	default:
		return zeroValueOther
	}
}

func closedFiberDone() chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
