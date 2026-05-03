package vm_next

import (
	"fmt"
	"sort"

	"github.com/akonwi/ard/air"
	vmcode "github.com/akonwi/ard/vm_next/bytecode"
)

func (vm *VM) execBytecodeListOp(inst vmcode.Instruction, stack *[]Value) (Value, error) {
	args, target, targetIndex, err := methodArgsFromStack(stack, inst.B)
	if err != nil {
		return Value{}, err
	}
	vm.recordRefAccess(refAccessList)
	listValue, ok := listRef(target)
	if !ok {
		return Value{}, listValueError(target)
	}
	var out Value
	switch inst.Op {
	case vmcode.OpListAt:
		if len(args) != 1 || args[0].Kind != ValueInt {
			return Value{}, fmt.Errorf("list index must be Int")
		}
		index := args[0].Int
		if index < 0 || index >= len(listValue.Items) {
			return Value{}, fmt.Errorf("list index out of range")
		}
		out = listValue.Items[index]
	case vmcode.OpListPrepend:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("list prepend expects one arg")
		}
		listValue.Items = append([]Value{args[0]}, listValue.Items...)
		out = Int(air.TypeID(inst.A), len(listValue.Items))
	case vmcode.OpListPush:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("list push expects one arg")
		}
		listValue.Items = append(listValue.Items, args[0])
		out = Int(air.TypeID(inst.A), len(listValue.Items))
	case vmcode.OpListSet:
		if len(args) != 2 || args[0].Kind != ValueInt {
			return Value{}, fmt.Errorf("list set expects index and value")
		}
		index := args[0].Int
		if index < 0 || index >= len(listValue.Items) {
			out = Bool(air.TypeID(inst.A), false)
			break
		}
		listValue.Items[index] = args[1]
		out = Bool(air.TypeID(inst.A), true)
	case vmcode.OpListSize:
		out = Int(air.TypeID(inst.A), len(listValue.Items))
	case vmcode.OpListSwap:
		if len(args) != 2 || args[0].Kind != ValueInt || args[1].Kind != ValueInt {
			return Value{}, fmt.Errorf("list swap expects integer indexes")
		}
		left, right := args[0].Int, args[1].Int
		if left < 0 || left >= len(listValue.Items) || right < 0 || right >= len(listValue.Items) {
			return Value{}, fmt.Errorf("list index out of range")
		}
		listValue.Items[left], listValue.Items[right] = listValue.Items[right], listValue.Items[left]
		out = vm.zeroValue(air.TypeID(inst.A))
	case vmcode.OpListSort:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("list sort expects comparator")
		}
		comparator := args[0]
		var sortErr error
		sort.SliceStable(listValue.Items, func(i, j int) bool {
			if sortErr != nil {
				return false
			}
			if vm.profile != nil {
				vm.profile.RecordArgSliceAlloc(2)
			}
			value, err := vm.callClosure(comparator, []Value{listValue.Items[i], listValue.Items[j]})
			if err != nil {
				sortErr = err
				return false
			}
			if value.Kind != ValueBool {
				sortErr = fmt.Errorf("list sort comparator must return Bool, got kind %d", value.Kind)
				return false
			}
			return value.Bool
		})
		if sortErr != nil {
			return Value{}, sortErr
		}
		out = vm.zeroValue(air.TypeID(inst.A))
	default:
		return Value{}, fmt.Errorf("unsupported list opcode %s", inst.Op)
	}
	*stack = (*stack)[:targetIndex]
	return out, nil
}

func (vm *VM) execBytecodeMapOp(inst vmcode.Instruction, stack *[]Value) (Value, error) {
	args, target, targetIndex, err := methodArgsFromStack(stack, inst.B)
	if err != nil {
		return Value{}, err
	}
	vm.recordRefAccess(refAccessMap)
	mapValue, ok := mapRef(target)
	if !ok {
		return Value{}, mapValueError(target)
	}
	var out Value
	switch inst.Op {
	case vmcode.OpMapKeys:
		keys := make([]Value, len(mapValue.Entries))
		for i, entry := range mapValue.Entries {
			keys[i] = entry.Key
		}
		sort.SliceStable(keys, func(i, j int) bool { return valuesLess(keys[i], keys[j]) })
		out = List(air.TypeID(inst.A), keys)
	case vmcode.OpMapSize:
		out = Int(air.TypeID(inst.A), len(mapValue.Entries))
	case vmcode.OpMapGet:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("map get expects key")
		}
		if index := mapEntryIndex(mapValue, args[0]); index >= 0 {
			vm.recordMaybeDetailAlloc(true)
			out = Maybe(air.TypeID(inst.A), true, mapValue.Entries[index].Value)
			break
		}
		vm.recordMaybeDetailAlloc(false)
		out = Maybe(air.TypeID(inst.A), false, vm.zeroValue(vm.bytecodeMaybeElem(air.TypeID(inst.A))))
	case vmcode.OpMapSet:
		if len(args) != 2 {
			return Value{}, fmt.Errorf("map set expects key and value")
		}
		if index := mapEntryIndex(mapValue, args[0]); index >= 0 {
			mapValue.Entries[index].Value = args[1]
		} else {
			mapValue.Entries = append(mapValue.Entries, MapEntryValue{Key: args[0], Value: args[1]})
		}
		mapValue.SortedDirty = true
		out = Bool(air.TypeID(inst.A), true)
	case vmcode.OpMapDrop:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("map drop expects key")
		}
		if index := mapEntryIndex(mapValue, args[0]); index >= 0 {
			mapValue.Entries = append(mapValue.Entries[:index], mapValue.Entries[index+1:]...)
			mapValue.SortedDirty = true
		}
		out = vm.zeroValue(air.TypeID(inst.A))
	case vmcode.OpMapHas:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("map has expects key")
		}
		out = Bool(air.TypeID(inst.A), mapEntryIndex(mapValue, args[0]) >= 0)
	case vmcode.OpMapKeyAt, vmcode.OpMapValueAt:
		if len(args) != 1 || args[0].Kind != ValueInt {
			return Value{}, fmt.Errorf("map entry index must be Int")
		}
		index := args[0].Int
		if index < 0 || index >= len(mapValue.Entries) {
			return Value{}, fmt.Errorf("map entry index out of range")
		}
		entries := sortedMapEntries(mapValue)
		if inst.Op == vmcode.OpMapKeyAt {
			out = entries[index].Key
		} else {
			out = entries[index].Value
		}
	default:
		return Value{}, fmt.Errorf("unsupported map opcode %s", inst.Op)
	}
	*stack = (*stack)[:targetIndex]
	return out, nil
}

func methodArgsFromStack(stack *[]Value, argCount int) ([]Value, Value, int, error) {
	if argCount < 0 || argCount+1 > len(*stack) {
		return nil, Value{}, 0, fmt.Errorf("method call: stack underflow")
	}
	targetIndex := len(*stack) - argCount - 1
	return (*stack)[targetIndex+1:], (*stack)[targetIndex], targetIndex, nil
}

func popMethodArgs(profile *executionProfile, pop func() (Value, error), argCount int) ([]Value, Value, error) {
	if profile != nil {
		profile.RecordArgSliceAlloc(argCount)
	}
	args := make([]Value, argCount)
	for i := argCount - 1; i >= 0; i-- {
		value, err := pop()
		if err != nil {
			return nil, Value{}, err
		}
		args[i] = value
	}
	target, err := pop()
	if err != nil {
		return nil, Value{}, err
	}
	return args, target, nil
}

func (vm *VM) bytecodeMaybeElem(typeID air.TypeID) air.TypeID {
	typeInfo, err := vm.typeInfo(typeID)
	if err != nil || typeInfo.Kind != air.TypeMaybe {
		return vm.mustTypeID(air.TypeVoid)
	}
	return typeInfo.Elem
}
