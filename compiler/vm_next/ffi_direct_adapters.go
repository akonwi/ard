package vm_next

import (
	"fmt"

	"github.com/akonwi/ard/air"
	stdlibffi "github.com/akonwi/ard/std_lib/ffi"
)

// newDirectHostExternAdapter returns signature-driven direct adapters for common
// host FFI function shapes. These adapters avoid reflect.Call in steady-state
// execution, but they still call the registered host function and leave semantic
// behavior in the host stdlib implementation. Unsupported/custom signatures use
// the reflective adapter path in ffi.go.
func (vm *VM) newDirectHostExternAdapter(extern air.Extern, fn any) func(*VM, []Value) (Value, error) {
	returnInfo, returnInfoErr := vm.typeInfo(extern.Signature.Return)
	isResultReturn := returnInfoErr == nil && returnInfo.Kind == air.TypeResult
	var resultValueInfo air.TypeInfo
	if isResultReturn {
		resultValueInfo, _ = vm.typeInfo(returnInfo.Value)
	}
	switch typed := fn.(type) {
	case func(any) stdlibffi.Result[int, stdlibffi.Error]:
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects one arg", goExternBinding(extern))
			}
			result := typed(dynamicArg(args[0]))
			return vm.fastDecodeIntResultWithInfo(extern.Signature.Return, returnInfo, result)
		}
	case func(any) stdlibffi.Result[string, stdlibffi.Error]:
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects one arg", goExternBinding(extern))
			}
			result := typed(dynamicArg(args[0]))
			return vm.fastDecodeStringResultWithInfo(extern.Signature.Return, returnInfo, result)
		}
	case func(any) stdlibffi.Result[float64, stdlibffi.Error]:
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects one arg", goExternBinding(extern))
			}
			result := typed(dynamicArg(args[0]))
			return vm.fastDecodeFloatResultWithInfo(extern.Signature.Return, returnInfo, result)
		}
	case func(any) stdlibffi.Result[bool, stdlibffi.Error]:
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects one arg", goExternBinding(extern))
			}
			result := typed(dynamicArg(args[0]))
			return vm.fastDecodeBoolResultWithInfo(extern.Signature.Return, returnInfo, result)
		}
	case func(any) bool:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects one arg", goExternBinding(extern))
			}
			return Bool(extern.Signature.Return, typed(dynamicArg(args[0]))), nil
		}
	case func(string):
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			typed(args[0].Str)
			return Void(extern.Signature.Return), nil
		}
	case func(int):
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueInt {
				return Value{}, fmt.Errorf("extern %s expects Int", goExternBinding(extern))
			}
			typed(args[0].Int)
			return Void(extern.Signature.Return), nil
		}
	case func() string:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 0 {
				return Value{}, fmt.Errorf("extern %s expects no args", goExternBinding(extern))
			}
			return Str(extern.Signature.Return, typed()), nil
		}
	case func() any:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 0 {
				return Value{}, fmt.Errorf("extern %s expects no args", goExternBinding(extern))
			}
			return Dynamic(extern.Signature.Return, typed()), nil
		}
	case func(string) string:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			return Str(extern.Signature.Return, typed(args[0].Str)), nil
		}
	case func(string) any:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			return Dynamic(extern.Signature.Return, typed(args[0].Str)), nil
		}
	case func(int) float64:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueInt {
				return Value{}, fmt.Errorf("extern %s expects Int", goExternBinding(extern))
			}
			return Float(extern.Signature.Return, typed(args[0].Int)), nil
		}
	case func(int) any:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueInt {
				return Value{}, fmt.Errorf("extern %s expects Int", goExternBinding(extern))
			}
			return Dynamic(extern.Signature.Return, typed(args[0].Int)), nil
		}
	case func(float64) float64:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueFloat {
				return Value{}, fmt.Errorf("extern %s expects Float", goExternBinding(extern))
			}
			return Float(extern.Signature.Return, typed(args[0].Float)), nil
		}
	case func(float64) any:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueFloat {
				return Value{}, fmt.Errorf("extern %s expects Float", goExternBinding(extern))
			}
			return Dynamic(extern.Signature.Return, typed(args[0].Float)), nil
		}
	case func(bool) any:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueBool {
				return Value{}, fmt.Errorf("extern %s expects Bool", goExternBinding(extern))
			}
			return Dynamic(extern.Signature.Return, typed(args[0].Bool)), nil
		}
	case func(string) bool:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			return Bool(extern.Signature.Return, typed(args[0].Str)), nil
		}
	case func(string) (bool, error):
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			out, err := typed(args[0].Str)
			return vm.fastBoolStringErrorResultWithInfo(extern.Signature.Return, returnInfo, out, err)
		}
	case func(string) error:
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			return vm.fastVoidStringErrorResultWithInfo(extern.Signature.Return, returnInfo, typed(args[0].Str))
		}
	case func(string, string) error:
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 2 || args[0].Kind != ValueStr || args[1].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str, Str", goExternBinding(extern))
			}
			return vm.fastVoidStringErrorResultWithInfo(extern.Signature.Return, returnInfo, typed(args[0].Str, args[1].Str))
		}
	case func() (string, error):
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 0 {
				return Value{}, fmt.Errorf("extern %s expects no args", goExternBinding(extern))
			}
			out, err := typed()
			return vm.fastStringStringErrorResultWithInfo(extern.Signature.Return, returnInfo, out, err)
		}
	case func() []string:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 0 {
				return Value{}, fmt.Errorf("extern %s expects no args", goExternBinding(extern))
			}
			return stringListValue(vm, extern.Signature.Return, typed())
		}
	case func(string) (string, error):
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			out, err := typed(args[0].Str)
			return vm.fastStringStringErrorResultWithInfo(extern.Signature.Return, returnInfo, out, err)
		}
	case func(string) (map[string]bool, error):
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			out, err := typed(args[0].Str)
			return vm.fastStringBoolMapResultWithInfo(extern.Signature.Return, returnInfo, resultValueInfo, out, err)
		}
	case func(string) []string:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			return stringListValue(vm, extern.Signature.Return, typed(args[0].Str))
		}
	case func(string) (any, error):
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			out, err := typed(args[0].Str)
			return vm.fastDynamicResultWithInfo(extern.Signature.Return, returnInfo, out, err)
		}
	case func(any) ([]any, error):
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects one arg", goExternBinding(extern))
			}
			out, err := typed(dynamicArg(args[0]))
			return vm.fastDynamicListResultWithInfo(extern.Signature.Return, returnInfo, resultValueInfo, out, err)
		}
	case func(any) (map[any]any, error):
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects one arg", goExternBinding(extern))
			}
			out, err := typed(dynamicArg(args[0]))
			return vm.fastDynamicMapResultWithInfo(extern.Signature.Return, returnInfo, resultValueInfo, out, err)
		}
	case func(any) (map[string]any, error):
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects one arg", goExternBinding(extern))
			}
			out, err := typed(dynamicArg(args[0]))
			return vm.fastStringDynamicMapResultWithInfo(extern.Signature.Return, returnInfo, resultValueInfo, out, err)
		}
	case func(any, string) (any, error):
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 2 || args[1].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Dynamic and Str", goExternBinding(extern))
			}
			out, err := typed(dynamicArg(args[0]), args[1].Str)
			return vm.fastDynamicResultWithInfo(extern.Signature.Return, returnInfo, out, err)
		}
	case func(any, string, []any) error:
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 3 || args[1].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects connection, Str, list", goExternBinding(extern))
			}
			conn, err := hostAnyArg(args[0])
			if err != nil {
				return Value{}, err
			}
			values, err := hostAnyListArg(args[2])
			if err != nil {
				return Value{}, err
			}
			return vm.fastVoidStringErrorResultWithInfo(extern.Signature.Return, returnInfo, typed(conn, args[1].Str, values))
		}
	case func(any, string, []any) ([]any, error):
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 3 || args[1].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects connection, Str, list", goExternBinding(extern))
			}
			conn, err := hostAnyArg(args[0])
			if err != nil {
				return Value{}, err
			}
			values, err := hostAnyListArg(args[2])
			if err != nil {
				return Value{}, err
			}
			out, hostErr := typed(conn, args[1].Str, values)
			return vm.fastDynamicListResultWithInfo(extern.Signature.Return, returnInfo, resultValueInfo, out, hostErr)
		}
	case func(string) stdlibffi.Maybe[int]:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			return vm.fastMaybeIntValue(extern.Signature.Return, typed(args[0].Str))
		}
	case func(string) stdlibffi.Maybe[string]:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			return vm.fastMaybeStringValue(extern.Signature.Return, typed(args[0].Str))
		}
	case func(string) stdlibffi.Maybe[float64]:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 || args[0].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects Str", goExternBinding(extern))
			}
			return vm.fastMaybeFloatValue(extern.Signature.Return, typed(args[0].Str))
		}
	case func(stdlibffi.Db) error:
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects Db", goExternBinding(extern))
			}
			db, err := dbArg(args[0])
			if err != nil {
				return Value{}, err
			}
			return vm.fastVoidStringErrorResultWithInfo(extern.Signature.Return, returnInfo, typed(db))
		}
	case func(stdlibffi.Tx) error:
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects Tx", goExternBinding(extern))
			}
			tx, err := txArg(args[0])
			if err != nil {
				return Value{}, err
			}
			return vm.fastVoidStringErrorResultWithInfo(extern.Signature.Return, returnInfo, typed(tx))
		}
	case func(stdlibffi.Db) (stdlibffi.Tx, error):
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects Db", goExternBinding(extern))
			}
			db, err := dbArg(args[0])
			if err != nil {
				return Value{}, err
			}
			out, hostErr := typed(db)
			return vm.fastExternStringErrorResultWithInfo(extern.Signature.Return, returnInfo, out.Handle, hostErr)
		}
	case func(stdlibffi.RawResponse) int:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects RawResponse", goExternBinding(extern))
			}
			resp, err := rawResponseArg(args[0])
			if err != nil {
				return Value{}, err
			}
			return Int(extern.Signature.Return, typed(resp)), nil
		}
	case func(stdlibffi.RawResponse):
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects RawResponse", goExternBinding(extern))
			}
			resp, err := rawResponseArg(args[0])
			if err != nil {
				return Value{}, err
			}
			typed(resp)
			return Void(extern.Signature.Return), nil
		}
	case func(stdlibffi.RawResponse) (string, error):
		if !isResultReturn {
			return nil
		}
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects RawResponse", goExternBinding(extern))
			}
			resp, err := rawResponseArg(args[0])
			if err != nil {
				return Value{}, err
			}
			out, hostErr := typed(resp)
			return vm.fastStringStringErrorResultWithInfo(extern.Signature.Return, returnInfo, out, hostErr)
		}
	case func(stdlibffi.RawResponse) map[string]string:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects RawResponse", goExternBinding(extern))
			}
			resp, err := rawResponseArg(args[0])
			if err != nil {
				return Value{}, err
			}
			return stringStringMapValue(vm, extern.Signature.Return, typed(resp))
		}
	case func(stdlibffi.RawRequest) string:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, fmt.Errorf("extern %s expects RawRequest", goExternBinding(extern))
			}
			req, err := rawRequestArg(args[0])
			if err != nil {
				return Value{}, err
			}
			return Str(extern.Signature.Return, typed(req)), nil
		}
	case func(stdlibffi.RawRequest, string) string:
		return func(vm *VM, args []Value) (Value, error) {
			if len(args) != 2 || args[1].Kind != ValueStr {
				return Value{}, fmt.Errorf("extern %s expects RawRequest, Str", goExternBinding(extern))
			}
			req, err := rawRequestArg(args[0])
			if err != nil {
				return Value{}, err
			}
			return Str(extern.Signature.Return, typed(req, args[1].Str)), nil
		}
	}
	return nil
}

func dynamicArg(value Value) any {
	if value.Kind == ValueDynamic {
		return value.Ref
	}
	return value.GoValue()
}

func externHandleArg(value Value) (any, error) {
	externValue, err := value.externValue()
	if err != nil {
		return nil, err
	}
	return externValue.Handle, nil
}

func dbArg(value Value) (stdlibffi.Db, error) {
	handle, err := externHandleArg(value)
	return stdlibffi.Db{Handle: handle}, err
}

func txArg(value Value) (stdlibffi.Tx, error) {
	handle, err := externHandleArg(value)
	return stdlibffi.Tx{Handle: handle}, err
}

func rawRequestArg(value Value) (stdlibffi.RawRequest, error) {
	handle, err := externHandleArg(value)
	return stdlibffi.RawRequest{Handle: handle}, err
}

func rawResponseArg(value Value) (stdlibffi.RawResponse, error) {
	handle, err := externHandleArg(value)
	return stdlibffi.RawResponse{Handle: handle}, err
}

func hostAnyArg(value Value) (any, error) {
	switch value.Kind {
	case ValueExtern:
		return externHandleArg(value)
	case ValueUnion:
		unionValue, err := value.unionValue()
		if err != nil {
			return nil, err
		}
		return hostAnyArg(unionValue.Value)
	case ValueDynamic:
		return value.Ref, nil
	case ValueVoid:
		return nil, nil
	case ValueInt:
		return value.Int, nil
	case ValueFloat:
		return value.Float, nil
	case ValueBool:
		return value.Bool, nil
	case ValueStr:
		return value.Str, nil
	default:
		return value.GoValue(), nil
	}
}

func hostAnyListArg(value Value) ([]any, error) {
	listValue, err := value.listValue()
	if err != nil {
		return nil, err
	}
	out := make([]any, len(listValue.Items))
	for i, item := range listValue.Items {
		hostItem, err := hostAnyArg(item)
		if err != nil {
			return nil, fmt.Errorf("list item %d: %w", i, err)
		}
		out[i] = hostItem
	}
	return out, nil
}

func stringListValue(vm *VM, typeID air.TypeID, raw []string) (Value, error) {
	listInfo, err := vm.typeInfo(typeID)
	if err != nil {
		return Value{}, err
	}
	items := make([]Value, len(raw))
	for i, item := range raw {
		items[i] = Str(listInfo.Elem, item)
	}
	return List(typeID, items), nil
}

func stringStringMapValue(vm *VM, typeID air.TypeID, raw map[string]string) (Value, error) {
	mapInfo, err := vm.typeInfo(typeID)
	if err != nil {
		return Value{}, err
	}
	entries := make([]MapEntryValue, 0, len(raw))
	for key, value := range raw {
		entries = append(entries, MapEntryValue{Key: Str(mapInfo.Key, key), Value: Str(mapInfo.Value, value)})
	}
	return Map(typeID, entries), nil
}

func (vm *VM) fastExternStringErrorResultWithInfo(returnType air.TypeID, info air.TypeInfo, handle any, hostErr error) (Value, error) {
	if vm.profile != nil {
		vm.profile.RecordValueAlloc(valueAllocResult)
	}
	if hostErr != nil {
		return Result(returnType, false, vm.fastStringErrorValue(info.Error, hostErr)), nil
	}
	return Result(returnType, true, Extern(info.Value, handle)), nil
}

func (vm *VM) fastMaybeIntValue(typeID air.TypeID, raw stdlibffi.Maybe[int]) (Value, error) {
	info, err := vm.typeInfo(typeID)
	if err != nil {
		return Value{}, err
	}
	if !raw.Some {
		vm.recordMaybeAlloc(false)
		return Maybe(typeID, false, vm.zeroValue(info.Elem)), nil
	}
	vm.recordMaybeAlloc(true)
	return Maybe(typeID, true, Int(info.Elem, raw.Value)), nil
}

func (vm *VM) fastMaybeStringValue(typeID air.TypeID, raw stdlibffi.Maybe[string]) (Value, error) {
	info, err := vm.typeInfo(typeID)
	if err != nil {
		return Value{}, err
	}
	if !raw.Some {
		vm.recordMaybeAlloc(false)
		return Maybe(typeID, false, vm.zeroValue(info.Elem)), nil
	}
	vm.recordMaybeAlloc(true)
	return Maybe(typeID, true, Str(info.Elem, raw.Value)), nil
}

func (vm *VM) fastMaybeFloatValue(typeID air.TypeID, raw stdlibffi.Maybe[float64]) (Value, error) {
	info, err := vm.typeInfo(typeID)
	if err != nil {
		return Value{}, err
	}
	if !raw.Some {
		vm.recordMaybeAlloc(false)
		return Maybe(typeID, false, vm.zeroValue(info.Elem)), nil
	}
	vm.recordMaybeAlloc(true)
	return Maybe(typeID, true, Float(info.Elem, raw.Value)), nil
}
