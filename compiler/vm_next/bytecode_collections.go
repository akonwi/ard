package vm_next

import (
	"fmt"
	"sort"

	"github.com/akonwi/ard/air"
	vmcode "github.com/akonwi/ard/vm_next/bytecode"
)

func (vm *VM) execBytecodeListOp(inst vmcode.Instruction, pop func() (Value, error)) (Value, error) {
	args, target, err := popMethodArgs(pop, inst.B)
	if err != nil {
		return Value{}, err
	}
	listValue, err := target.listValue()
	if err != nil {
		return Value{}, err
	}
	switch inst.Op {
	case vmcode.OpListAt:
		if len(args) != 1 || args[0].Kind != ValueInt {
			return Value{}, fmt.Errorf("list index must be Int")
		}
		index := args[0].Int
		if index < 0 || index >= len(listValue.Items) {
			return Value{}, fmt.Errorf("list index out of range")
		}
		return listValue.Items[index], nil
	case vmcode.OpListPrepend:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("list prepend expects one arg")
		}
		listValue.Items = append([]Value{args[0]}, listValue.Items...)
		return Int(air.TypeID(inst.A), len(listValue.Items)), nil
	case vmcode.OpListPush:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("list push expects one arg")
		}
		listValue.Items = append(listValue.Items, args[0])
		return Int(air.TypeID(inst.A), len(listValue.Items)), nil
	case vmcode.OpListSet:
		if len(args) != 2 || args[0].Kind != ValueInt {
			return Value{}, fmt.Errorf("list set expects index and value")
		}
		index := args[0].Int
		if index < 0 || index >= len(listValue.Items) {
			return Bool(air.TypeID(inst.A), false), nil
		}
		listValue.Items[index] = args[1]
		return Bool(air.TypeID(inst.A), true), nil
	case vmcode.OpListSize:
		return Int(air.TypeID(inst.A), len(listValue.Items)), nil
	case vmcode.OpListSwap:
		if len(args) != 2 || args[0].Kind != ValueInt || args[1].Kind != ValueInt {
			return Value{}, fmt.Errorf("list swap expects integer indexes")
		}
		left, right := args[0].Int, args[1].Int
		if left < 0 || left >= len(listValue.Items) || right < 0 || right >= len(listValue.Items) {
			return Value{}, fmt.Errorf("list index out of range")
		}
		listValue.Items[left], listValue.Items[right] = listValue.Items[right], listValue.Items[left]
		return vm.zeroValue(air.TypeID(inst.A)), nil
	case vmcode.OpListSort:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("list sort expects comparator")
		}
		var sortErr error
		sort.SliceStable(listValue.Items, func(i, j int) bool {
			if sortErr != nil {
				return false
			}
			value, err := vm.callClosure(args[0], []Value{listValue.Items[i], listValue.Items[j]})
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
		return vm.zeroValue(air.TypeID(inst.A)), nil
	default:
		return Value{}, fmt.Errorf("unsupported list opcode %s", inst.Op)
	}
}

func (vm *VM) execBytecodeMapOp(inst vmcode.Instruction, pop func() (Value, error)) (Value, error) {
	args, target, err := popMethodArgs(pop, inst.B)
	if err != nil {
		return Value{}, err
	}
	mapValue, err := target.mapValue()
	if err != nil {
		return Value{}, err
	}
	switch inst.Op {
	case vmcode.OpMapKeys:
		keys := make([]Value, len(mapValue.Entries))
		for i, entry := range mapValue.Entries {
			keys[i] = entry.Key
		}
		sort.SliceStable(keys, func(i, j int) bool { return valuesLess(keys[i], keys[j]) })
		return List(air.TypeID(inst.A), keys), nil
	case vmcode.OpMapSize:
		return Int(air.TypeID(inst.A), len(mapValue.Entries)), nil
	case vmcode.OpMapGet:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("map get expects key")
		}
		if index := mapEntryIndex(mapValue, args[0]); index >= 0 {
			return Maybe(air.TypeID(inst.A), true, mapValue.Entries[index].Value), nil
		}
		return Maybe(air.TypeID(inst.A), false, vm.zeroValue(vm.bytecodeMaybeElem(air.TypeID(inst.A)))), nil
	case vmcode.OpMapSet:
		if len(args) != 2 {
			return Value{}, fmt.Errorf("map set expects key and value")
		}
		if index := mapEntryIndex(mapValue, args[0]); index >= 0 {
			mapValue.Entries[index].Value = args[1]
		} else {
			mapValue.Entries = append(mapValue.Entries, MapEntryValue{Key: args[0], Value: args[1]})
		}
		return Bool(air.TypeID(inst.A), true), nil
	case vmcode.OpMapDrop:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("map drop expects key")
		}
		if index := mapEntryIndex(mapValue, args[0]); index >= 0 {
			mapValue.Entries = append(mapValue.Entries[:index], mapValue.Entries[index+1:]...)
		}
		return vm.zeroValue(air.TypeID(inst.A)), nil
	case vmcode.OpMapHas:
		if len(args) != 1 {
			return Value{}, fmt.Errorf("map has expects key")
		}
		return Bool(air.TypeID(inst.A), mapEntryIndex(mapValue, args[0]) >= 0), nil
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
			return entries[index].Key, nil
		}
		return entries[index].Value, nil
	default:
		return Value{}, fmt.Errorf("unsupported map opcode %s", inst.Op)
	}
}

func popMethodArgs(pop func() (Value, error), argCount int) ([]Value, Value, error) {
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
