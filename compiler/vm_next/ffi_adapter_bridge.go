package vm_next

import (
	"fmt"
	"reflect"

	"github.com/akonwi/ard/air"
	stdlibffi "github.com/akonwi/ard/std_lib/ffi"
)

type generatedHostBridge struct {
	vm *VM
}

func (bridge generatedHostBridge) HostArg(args any, index int, target reflect.Type) (any, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return nil, err
	}
	return bridge.vm.generatedHostArg(value, target)
}

func (bridge generatedHostBridge) HostArgInt(args any, index int) (int, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return 0, err
	}
	if value.Kind != ValueInt && value.Kind != ValueEnum {
		return 0, fmt.Errorf("expected int-compatible value, got kind %d", value.Kind)
	}
	return value.Int, nil
}

func (bridge generatedHostBridge) HostArgFloat64(args any, index int) (float64, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return 0, err
	}
	if value.Kind != ValueFloat {
		return 0, fmt.Errorf("expected float value, got kind %d", value.Kind)
	}
	return value.Float, nil
}

func (bridge generatedHostBridge) HostArgBool(args any, index int) (bool, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return false, err
	}
	if value.Kind != ValueBool {
		return false, fmt.Errorf("expected bool value, got kind %d", value.Kind)
	}
	return value.Bool, nil
}

func (bridge generatedHostBridge) HostArgString(args any, index int) (string, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return "", err
	}
	if value.Kind != ValueStr {
		return "", fmt.Errorf("expected string value, got kind %d", value.Kind)
	}
	return value.Str, nil
}

func (bridge generatedHostBridge) HostArgMaybeBool(args any, index int) (any, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return nil, err
	}
	if value.Kind != ValueMaybe {
		return nil, fmt.Errorf("expected maybe value, got kind %d", value.Kind)
	}
	maybeValue, err := value.maybeValue()
	if err != nil {
		return nil, err
	}
	if !maybeValue.Some {
		return stdlibffi.None[bool](), nil
	}
	if maybeValue.Value.Kind != ValueBool {
		return nil, fmt.Errorf("expected bool maybe value, got kind %d", maybeValue.Value.Kind)
	}
	return stdlibffi.Some(maybeValue.Value.Bool), nil
}

func (bridge generatedHostBridge) HostArgMaybeInt(args any, index int) (any, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return nil, err
	}
	if value.Kind != ValueMaybe {
		return nil, fmt.Errorf("expected maybe value, got kind %d", value.Kind)
	}
	maybeValue, err := value.maybeValue()
	if err != nil {
		return nil, err
	}
	if !maybeValue.Some {
		return stdlibffi.None[int](), nil
	}
	if maybeValue.Value.Kind != ValueInt && maybeValue.Value.Kind != ValueEnum {
		return nil, fmt.Errorf("expected int maybe value, got kind %d", maybeValue.Value.Kind)
	}
	return stdlibffi.Some(maybeValue.Value.Int), nil
}

func (bridge generatedHostBridge) HostArgMaybeFloat64(args any, index int) (any, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return nil, err
	}
	if value.Kind != ValueMaybe {
		return nil, fmt.Errorf("expected maybe value, got kind %d", value.Kind)
	}
	maybeValue, err := value.maybeValue()
	if err != nil {
		return nil, err
	}
	if !maybeValue.Some {
		return stdlibffi.None[float64](), nil
	}
	if maybeValue.Value.Kind != ValueFloat {
		return nil, fmt.Errorf("expected float maybe value, got kind %d", maybeValue.Value.Kind)
	}
	return stdlibffi.Some(maybeValue.Value.Float), nil
}

func (bridge generatedHostBridge) HostArgMaybeString(args any, index int) (any, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return nil, err
	}
	if value.Kind != ValueMaybe {
		return nil, fmt.Errorf("expected maybe value, got kind %d", value.Kind)
	}
	maybeValue, err := value.maybeValue()
	if err != nil {
		return nil, err
	}
	if !maybeValue.Some {
		return stdlibffi.None[string](), nil
	}
	if maybeValue.Value.Kind != ValueStr {
		return nil, fmt.Errorf("expected string maybe value, got kind %d", maybeValue.Value.Kind)
	}
	return stdlibffi.Some(maybeValue.Value.Str), nil
}

func (bridge generatedHostBridge) HostArgAny(args any, index int) (any, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return nil, err
	}
	if value.Kind == ValueDynamic {
		raw, err := value.dynamicRaw()
		if err != nil {
			return nil, err
		}
		return raw, nil
	}
	return bridge.vm.generatedHostArg(value, anyInterface)
}

func (bridge generatedHostBridge) HostArgAnySlice(args any, index int) ([]any, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return nil, err
	}
	if value.Kind != ValueList {
		return nil, fmt.Errorf("expected list value, got kind %d", value.Kind)
	}
	listValue, err := value.listValue()
	if err != nil {
		return nil, err
	}
	out := make([]any, len(listValue.Items))
	for i, item := range listValue.Items {
		hostItem, err := bridge.vm.generatedHostArg(item, anyInterface)
		if err != nil {
			return nil, fmt.Errorf("list item %d: %w", i, err)
		}
		out[i] = hostItem
	}
	return out, nil
}

func (bridge generatedHostBridge) HostArgStringSlice(args any, index int) ([]string, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return nil, err
	}
	if value.Kind != ValueList {
		return nil, fmt.Errorf("expected list value, got kind %d", value.Kind)
	}
	listValue, err := value.listValue()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(listValue.Items))
	for i, item := range listValue.Items {
		if item.Kind != ValueStr {
			return nil, fmt.Errorf("list item %d: expected string value, got kind %d", i, item.Kind)
		}
		out[i] = item.Str
	}
	return out, nil
}

func (bridge generatedHostBridge) HostArgStringMap(args any, index int) (map[string]string, error) {
	value, err := generatedHostArgValue(args, index)
	if err != nil {
		return nil, err
	}
	if value.Kind != ValueMap {
		return nil, fmt.Errorf("expected map value, got kind %d", value.Kind)
	}
	mapValue, err := value.mapValue()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(mapValue.Entries))
	for i, entry := range mapValue.Entries {
		if entry.Key.Kind != ValueStr {
			return nil, fmt.Errorf("map key %d: expected string key, got kind %d", i, entry.Key.Kind)
		}
		if entry.Value.Kind != ValueStr {
			return nil, fmt.Errorf("map value %q: expected string value, got kind %d", entry.Key.Str, entry.Value.Kind)
		}
		out[entry.Key.Str] = entry.Value.Str
	}
	return out, nil
}

func generatedHostArgValue(args any, index int) (Value, error) {
	values, ok := args.([]Value)
	if !ok {
		return Value{}, fmt.Errorf("generated host adapter args must be []vm_next.Value, got %T", args)
	}
	if index < 0 || index >= len(values) {
		return Value{}, fmt.Errorf("generated host adapter arg index %d out of range", index)
	}
	return values[index], nil
}

func (bridge generatedHostBridge) HostReturnVoid(returnType air.TypeID) (any, error) {
	return bridge.vm.zeroValue(returnType), nil
}

func (bridge generatedHostBridge) HostReturnValue(returnType air.TypeID, value any) (any, error) {
	return bridge.vm.generatedHostValueToValue(returnType, value)
}

func (bridge generatedHostBridge) HostReturnError(returnType air.TypeID, err error) (any, error) {
	returnInfo, infoErr := bridge.vm.typeInfo(returnType)
	if infoErr != nil {
		return Value{}, infoErr
	}
	if err != nil {
		return bridge.vm.resultErr(returnType, returnInfo.Error, err)
	}
	return Result(returnType, true, bridge.vm.zeroValue(returnInfo.Value)), nil
}

func (bridge generatedHostBridge) HostReturnValueError(returnType air.TypeID, value any, err error) (any, error) {
	returnInfo, infoErr := bridge.vm.typeInfo(returnType)
	if infoErr != nil {
		return Value{}, infoErr
	}
	if err != nil {
		return bridge.vm.resultErr(returnType, returnInfo.Error, err)
	}
	okValue, convertErr := bridge.vm.generatedHostValueToValue(returnInfo.Value, value)
	if convertErr != nil {
		return Value{}, convertErr
	}
	return Result(returnType, true, okValue), nil
}

func (bridge generatedHostBridge) HostReturnResult(returnType air.TypeID, value any, errValue any, ok bool) (any, error) {
	returnInfo, err := bridge.vm.typeInfo(returnType)
	if err != nil {
		return Value{}, err
	}
	if ok {
		okValue, err := bridge.vm.generatedHostValueToValue(returnInfo.Value, value)
		if err != nil {
			return Value{}, err
		}
		return Result(returnType, true, okValue), nil
	}
	convertedErr, err := bridge.vm.generatedHostValueToValue(returnInfo.Error, errValue)
	if err != nil {
		return Value{}, err
	}
	return Result(returnType, false, convertedErr), nil
}

func (vm *VM) generatedHostArg(value Value, target reflect.Type) (any, error) {
	switch target {
	case intType:
		if value.Kind != ValueInt && value.Kind != ValueEnum {
			return nil, fmt.Errorf("expected int-compatible value, got kind %d", value.Kind)
		}
		return value.Int, nil
	case float64Type:
		if value.Kind != ValueFloat {
			return nil, fmt.Errorf("expected float value, got kind %d", value.Kind)
		}
		return value.Float, nil
	case boolType:
		if value.Kind != ValueBool {
			return nil, fmt.Errorf("expected bool value, got kind %d", value.Kind)
		}
		return value.Bool, nil
	case stringType:
		if value.Kind != ValueStr {
			return nil, fmt.Errorf("expected string value, got kind %d", value.Kind)
		}
		return value.Str, nil
	case anyInterface:
		if value.Kind == ValueDynamic {
			raw, err := value.dynamicRaw()
			if err != nil {
				return nil, err
			}
			return raw, nil
		}
	}

	hostValue, err := vm.valueToHost(value, target)
	if err != nil {
		return nil, err
	}
	if !hostValue.IsValid() {
		return nil, nil
	}
	if !hostValue.CanInterface() {
		return nil, fmt.Errorf("cannot pass host value %s", hostValue.Type())
	}
	out := hostValue.Interface()
	if out == nil {
		return nil, nil
	}
	outValue := reflect.ValueOf(out)
	if !outValue.Type().AssignableTo(target) {
		return nil, fmt.Errorf("cannot pass host value %s as %s", outValue.Type(), target)
	}
	return out, nil
}

func (vm *VM) generatedHostValueToValue(typeID air.TypeID, value any) (Value, error) {
	typeInfo, err := vm.typeInfo(typeID)
	if err != nil {
		return Value{}, err
	}
	switch typeInfo.Kind {
	case air.TypeVoid:
		return Void(typeID), nil
	case air.TypeInt:
		if v, ok := value.(int); ok {
			return Int(typeID, v), nil
		}
	case air.TypeEnum:
		if v, ok := value.(int); ok {
			return Enum(typeID, v), nil
		}
	case air.TypeFloat:
		if v, ok := value.(float64); ok {
			return Float(typeID, v), nil
		}
	case air.TypeBool:
		if v, ok := value.(bool); ok {
			return Bool(typeID, v), nil
		}
	case air.TypeStr:
		if v, ok := value.(string); ok {
			return Str(typeID, v), nil
		}
	case air.TypeDynamic:
		return Dynamic(typeID, value), nil
	case air.TypeMaybe:
		switch v := value.(type) {
		case stdlibffi.Maybe[bool]:
			if !v.Some {
				return Maybe(typeID, false, vm.zeroValue(typeInfo.Elem)), nil
			}
			inner, err := vm.generatedHostValueToValue(typeInfo.Elem, v.Value)
			if err != nil {
				return Value{}, err
			}
			return Maybe(typeID, true, inner), nil
		case stdlibffi.Maybe[int]:
			if !v.Some {
				return Maybe(typeID, false, vm.zeroValue(typeInfo.Elem)), nil
			}
			inner, err := vm.generatedHostValueToValue(typeInfo.Elem, v.Value)
			if err != nil {
				return Value{}, err
			}
			return Maybe(typeID, true, inner), nil
		case stdlibffi.Maybe[float64]:
			if !v.Some {
				return Maybe(typeID, false, vm.zeroValue(typeInfo.Elem)), nil
			}
			inner, err := vm.generatedHostValueToValue(typeInfo.Elem, v.Value)
			if err != nil {
				return Value{}, err
			}
			return Maybe(typeID, true, inner), nil
		case stdlibffi.Maybe[string]:
			if !v.Some {
				return Maybe(typeID, false, vm.zeroValue(typeInfo.Elem)), nil
			}
			inner, err := vm.generatedHostValueToValue(typeInfo.Elem, v.Value)
			if err != nil {
				return Value{}, err
			}
			return Maybe(typeID, true, inner), nil
		case stdlibffi.Maybe[any]:
			if !v.Some {
				return Maybe(typeID, false, vm.zeroValue(typeInfo.Elem)), nil
			}
			inner, err := vm.generatedHostValueToValue(typeInfo.Elem, v.Value)
			if err != nil {
				return Value{}, err
			}
			return Maybe(typeID, true, inner), nil
		}
	case air.TypeList:
		if values, ok := value.([]any); ok {
			items := make([]Value, len(values))
			for i, item := range values {
				converted, err := vm.generatedHostValueToValue(typeInfo.Elem, item)
				if err != nil {
					return Value{}, fmt.Errorf("list item %d: %w", i, err)
				}
				items[i] = converted
			}
			return List(typeID, items), nil
		}
		if values, ok := value.([]string); ok {
			items := make([]Value, len(values))
			for i, item := range values {
				converted, err := vm.generatedHostValueToValue(typeInfo.Elem, item)
				if err != nil {
					return Value{}, fmt.Errorf("list item %d: %w", i, err)
				}
				items[i] = converted
			}
			return List(typeID, items), nil
		}
		if values, ok := value.([]int); ok {
			items := make([]Value, len(values))
			for i, item := range values {
				converted, err := vm.generatedHostValueToValue(typeInfo.Elem, item)
				if err != nil {
					return Value{}, fmt.Errorf("list item %d: %w", i, err)
				}
				items[i] = converted
			}
			return List(typeID, items), nil
		}
		if values, ok := value.([]float64); ok {
			items := make([]Value, len(values))
			for i, item := range values {
				converted, err := vm.generatedHostValueToValue(typeInfo.Elem, item)
				if err != nil {
					return Value{}, fmt.Errorf("list item %d: %w", i, err)
				}
				items[i] = converted
			}
			return List(typeID, items), nil
		}
		if values, ok := value.([]bool); ok {
			items := make([]Value, len(values))
			for i, item := range values {
				converted, err := vm.generatedHostValueToValue(typeInfo.Elem, item)
				if err != nil {
					return Value{}, fmt.Errorf("list item %d: %w", i, err)
				}
				items[i] = converted
			}
			return List(typeID, items), nil
		}
	case air.TypeMap:
		if values, ok := value.(map[string]any); ok {
			entries := make([]MapEntryValue, 0, len(values))
			for key, item := range values {
				convertedKey, err := vm.generatedHostValueToValue(typeInfo.Key, key)
				if err != nil {
					return Value{}, fmt.Errorf("map key %q: %w", key, err)
				}
				convertedValue, err := vm.generatedHostValueToValue(typeInfo.Value, item)
				if err != nil {
					return Value{}, fmt.Errorf("map value %q: %w", key, err)
				}
				entries = append(entries, MapEntryValue{Key: convertedKey, Value: convertedValue})
			}
			return Map(typeID, entries), nil
		}
		if values, ok := value.(map[string]string); ok {
			entries := make([]MapEntryValue, 0, len(values))
			for key, item := range values {
				convertedKey, err := vm.generatedHostValueToValue(typeInfo.Key, key)
				if err != nil {
					return Value{}, fmt.Errorf("map key %q: %w", key, err)
				}
				convertedValue, err := vm.generatedHostValueToValue(typeInfo.Value, item)
				if err != nil {
					return Value{}, fmt.Errorf("map value %q: %w", key, err)
				}
				entries = append(entries, MapEntryValue{Key: convertedKey, Value: convertedValue})
			}
			return Map(typeID, entries), nil
		}
		if values, ok := value.(map[string]bool); ok {
			entries := make([]MapEntryValue, 0, len(values))
			for key, item := range values {
				convertedKey, err := vm.generatedHostValueToValue(typeInfo.Key, key)
				if err != nil {
					return Value{}, fmt.Errorf("map key %q: %w", key, err)
				}
				convertedValue, err := vm.generatedHostValueToValue(typeInfo.Value, item)
				if err != nil {
					return Value{}, fmt.Errorf("map value %q: %w", key, err)
				}
				entries = append(entries, MapEntryValue{Key: convertedKey, Value: convertedValue})
			}
			return Map(typeID, entries), nil
		}
	}
	if value == nil {
		return vm.hostValueToValue(typeID, reflect.Value{})
	}
	return vm.hostValueToValue(typeID, reflect.ValueOf(value))
}
