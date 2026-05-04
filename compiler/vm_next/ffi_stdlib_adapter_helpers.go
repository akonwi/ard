package vm_next

import (
	"fmt"
	"reflect"

	"github.com/akonwi/ard/air"
	stdlibffi "github.com/akonwi/ard/std_lib/ffi"
)

func stdlibHostArg[T any](vm *VM, value Value) (T, error) {
	var zero T
	target := reflect.TypeFor[T]()
	switch target {
	case intType:
		if value.Kind != ValueInt && value.Kind != ValueEnum {
			return zero, fmt.Errorf("expected int-compatible value, got kind %d", value.Kind)
		}
		return any(value.Int).(T), nil
	case float64Type:
		if value.Kind != ValueFloat {
			return zero, fmt.Errorf("expected float value, got kind %d", value.Kind)
		}
		return any(value.Float).(T), nil
	case boolType:
		if value.Kind != ValueBool {
			return zero, fmt.Errorf("expected bool value, got kind %d", value.Kind)
		}
		return any(value.Bool).(T), nil
	case stringType:
		if value.Kind != ValueStr {
			return zero, fmt.Errorf("expected string value, got kind %d", value.Kind)
		}
		return any(value.Str).(T), nil
	case anyInterface:
		if value.Kind == ValueDynamic {
			raw, err := value.dynamicRaw()
			if err != nil {
				return zero, err
			}
			if raw == nil {
				return zero, nil
			}
			return any(raw).(T), nil
		}
	}

	hostValue, err := vm.valueToHost(value, target)
	if err != nil {
		return zero, err
	}
	if !hostValue.IsValid() {
		return zero, nil
	}
	if !hostValue.CanInterface() {
		return zero, fmt.Errorf("cannot pass host value %s", hostValue.Type())
	}
	out, ok := hostValue.Interface().(T)
	if !ok {
		return zero, fmt.Errorf("cannot pass host value %s as %s", hostValue.Type(), target)
	}
	return out, nil
}

func stdlibHostReturnVoid(vm *VM, returnType air.TypeID) (Value, error) {
	return vm.zeroValue(returnType), nil
}

func stdlibHostReturnValue[T any](vm *VM, returnType air.TypeID, value T) (Value, error) {
	return stdlibHostValueToValue(vm, returnType, value)
}

func stdlibHostReturnError(vm *VM, returnType air.TypeID, err error) (Value, error) {
	returnInfo, infoErr := vm.typeInfo(returnType)
	if infoErr != nil {
		return Value{}, infoErr
	}
	if err != nil {
		return vm.resultErr(returnType, returnInfo.Error, err)
	}
	return Result(returnType, true, vm.zeroValue(returnInfo.Value)), nil
}

func stdlibHostReturnValueError[T any](vm *VM, returnType air.TypeID, value T, err error) (Value, error) {
	returnInfo, infoErr := vm.typeInfo(returnType)
	if infoErr != nil {
		return Value{}, infoErr
	}
	if err != nil {
		return vm.resultErr(returnType, returnInfo.Error, err)
	}
	okValue, convertErr := stdlibHostValueToValue(vm, returnInfo.Value, value)
	if convertErr != nil {
		return Value{}, convertErr
	}
	return Result(returnType, true, okValue), nil
}

func stdlibHostReturnResult[T, E any](vm *VM, returnType air.TypeID, result stdlibffi.Result[T, E]) (Value, error) {
	returnInfo, err := vm.typeInfo(returnType)
	if err != nil {
		return Value{}, err
	}
	if result.Ok {
		okValue, err := stdlibHostValueToValue(vm, returnInfo.Value, result.Value)
		if err != nil {
			return Value{}, err
		}
		return Result(returnType, true, okValue), nil
	}
	errValue, err := stdlibHostValueToValue(vm, returnInfo.Error, result.Error)
	if err != nil {
		return Value{}, err
	}
	return Result(returnType, false, errValue), nil
}

func stdlibHostValueToValue[T any](vm *VM, typeID air.TypeID, value T) (Value, error) {
	typeInfo, err := vm.typeInfo(typeID)
	if err != nil {
		return Value{}, err
	}
	switch typeInfo.Kind {
	case air.TypeVoid:
		return Void(typeID), nil
	case air.TypeInt:
		if v, ok := any(value).(int); ok {
			return Int(typeID, v), nil
		}
	case air.TypeEnum:
		if v, ok := any(value).(int); ok {
			return Enum(typeID, v), nil
		}
	case air.TypeFloat:
		if v, ok := any(value).(float64); ok {
			return Float(typeID, v), nil
		}
	case air.TypeBool:
		if v, ok := any(value).(bool); ok {
			return Bool(typeID, v), nil
		}
	case air.TypeStr:
		if v, ok := any(value).(string); ok {
			return Str(typeID, v), nil
		}
	case air.TypeDynamic:
		return Dynamic(typeID, any(value)), nil
	}
	return vm.hostValueToValue(typeID, reflect.ValueOf(value))
}
