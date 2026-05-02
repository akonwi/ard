package vm_next

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/akonwi/ard/air"
	stdlibffi "github.com/akonwi/ard/std_lib/ffi"
)

type HostFunctionRegistry map[string]any

type hostExternAdapters map[air.ExternID]hostExternAdapter

type hostExternAdapter struct {
	binding  string
	extern   air.Extern
	callable reflect.Value
	inputs   []reflect.Type
	buildErr error
}

var (
	errorInterface = reflect.TypeFor[error]()
	hostMaybeType  = reflect.TypeFor[stdlibffi.Maybe[any]]()
	hostResultType = reflect.TypeFor[stdlibffi.Result[any, any]]()
	anyInterface   = reflect.TypeFor[any]()
)

func NewWithExterns(program *air.Program, externs HostFunctionRegistry) (*VM, error) {
	if err := air.Validate(program); err != nil {
		return nil, err
	}
	registry := HostFunctionRegistry{}
	for name, fn := range stdlibffi.HostFunctions {
		registry[name] = fn
	}
	for name, fn := range externs {
		registry[name] = fn
	}
	vm := &VM{program: program, profile: newExecutionProfile()}
	vm.externs = vm.buildHostExternAdapters(registry)
	return vm, nil
}

func (vm *VM) callExtern(id air.ExternID, args []Value) (value Value, err error) {
	if id < 0 || int(id) >= len(vm.program.Externs) {
		return Value{}, fmt.Errorf("invalid extern id %d", id)
	}
	extern := vm.program.Externs[id]
	binding := goExternBinding(extern)
	adapter, ok := vm.externs[id]
	if !ok {
		return Value{}, fmt.Errorf("extern binding %q is not registered", binding)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprintf("panic in FFI function '%s': %v", binding, recovered)
			returnInfo, infoErr := vm.typeInfo(extern.Signature.Return)
			if infoErr == nil && returnInfo.Kind == air.TypeResult {
				result, resultErr := vm.resultErr(extern.Signature.Return, returnInfo.Error, errors.New(message))
				if resultErr == nil {
					value = result
					err = nil
					return
				}
				err = resultErr
				return
			}
			value = Value{}
			err = errors.New(message)
		}
	}()
	return adapter.call(vm, args)
}

func (vm *VM) buildHostExternAdapters(registry HostFunctionRegistry) hostExternAdapters {
	adapters := hostExternAdapters{}
	for _, extern := range vm.program.Externs {
		binding := goExternBinding(extern)
		fn, ok := registry[binding]
		if !ok {
			continue
		}
		adapter, err := vm.newHostExternAdapter(extern, binding, fn)
		if err != nil {
			adapter = hostExternAdapter{binding: binding, extern: extern, buildErr: err}
		}
		adapters[extern.ID] = adapter
	}
	return adapters
}

func (vm *VM) newHostExternAdapter(extern air.Extern, binding string, fn any) (hostExternAdapter, error) {
	callable := reflect.ValueOf(fn)
	if !callable.IsValid() || callable.Kind() != reflect.Func {
		return hostExternAdapter{}, fmt.Errorf("extern binding %q is %T, want func", binding, fn)
	}
	fnType := callable.Type()
	if fnType.NumIn() != len(extern.Signature.Params) {
		return hostExternAdapter{}, fmt.Errorf("extern %s expects %d params from AIR, host function accepts %d", binding, len(extern.Signature.Params), fnType.NumIn())
	}
	inputs := make([]reflect.Type, len(extern.Signature.Params))
	for i, param := range extern.Signature.Params {
		target := fnType.In(i)
		if err := vm.validateHostParamType(param.Type, target); err != nil {
			return hostExternAdapter{}, fmt.Errorf("arg %d %s: %w", i, param.Name, err)
		}
		inputs[i] = target
	}
	if err := vm.validateHostReturns(extern.Signature.Return, fnType); err != nil {
		return hostExternAdapter{}, err
	}
	return hostExternAdapter{
		binding:  binding,
		extern:   extern,
		callable: callable,
		inputs:   inputs,
	}, nil
}

func (adapter hostExternAdapter) call(vm *VM, args []Value) (Value, error) {
	if adapter.buildErr != nil {
		return Value{}, fmt.Errorf("extern %s adapter: %w", adapter.binding, adapter.buildErr)
	}
	if len(adapter.inputs) != len(args) {
		return Value{}, fmt.Errorf("extern %s expects %d args, got %d", adapter.binding, len(adapter.inputs), len(args))
	}
	if vm.profile == nil {
		inputs := make([]reflect.Value, len(args))
		for i, arg := range args {
			input, err := vm.valueToHost(arg, adapter.inputs[i])
			if err != nil {
				return Value{}, fmt.Errorf("extern %s arg %d: %w", adapter.binding, i, err)
			}
			inputs[i] = input
		}
		return vm.hostReturnsToValue(adapter.extern.Signature.Return, adapter.callable.Call(inputs))
	}

	convertInStart := time.Now()
	inputs := make([]reflect.Value, len(args))
	for i, arg := range args {
		input, err := vm.valueToHost(arg, adapter.inputs[i])
		if err != nil {
			return Value{}, fmt.Errorf("extern %s arg %d: %w", adapter.binding, i, err)
		}
		inputs[i] = input
	}
	convertIn := time.Since(convertInStart)
	hostStart := time.Now()
	hostReturns := adapter.callable.Call(inputs)
	hostDuration := time.Since(hostStart)
	convertOutStart := time.Now()
	value, err := vm.hostReturnsToValue(adapter.extern.Signature.Return, hostReturns)
	convertOut := time.Since(convertOutStart)
	vm.profile.RecordExternCall(adapter.binding, len(args), convertIn, hostDuration, convertOut)
	return value, err
}

func goExternBinding(extern air.Extern) string {
	if binding := extern.Bindings["go"]; binding != "" {
		return binding
	}
	return extern.Name
}

func (vm *VM) validateHostParamType(typeID air.TypeID, target reflect.Type) error {
	return vm.validateHostType(typeID, target, true)
}

func (vm *VM) validateHostReturns(returnType air.TypeID, fnType reflect.Type) error {
	returnInfo, err := vm.typeInfo(returnType)
	if err != nil {
		return err
	}
	if returnInfo.Kind == air.TypeResult {
		return vm.validateHostResultReturns(returnInfo, fnType)
	}
	if returnInfo.Kind == air.TypeVoid && fnType.NumOut() == 0 {
		return nil
	}
	if fnType.NumOut() != 1 {
		return fmt.Errorf("extern must return exactly one value for %s, got %d", returnInfo.Name, fnType.NumOut())
	}
	return vm.validateHostType(returnType, fnType.Out(0), false)
}

func (vm *VM) validateHostResultReturns(returnInfo air.TypeInfo, fnType reflect.Type) error {
	switch fnType.NumOut() {
	case 1:
		if isHostResultType(fnType.Out(0)) {
			return vm.validateHostResultType(returnInfo, fnType.Out(0), false)
		}
		if !fnType.Out(0).Implements(errorInterface) {
			return fmt.Errorf("Result extern returning one value must return error, got %s", fnType.Out(0))
		}
		valueInfo, err := vm.typeInfo(returnInfo.Value)
		if err != nil {
			return err
		}
		if valueInfo.Kind != air.TypeVoid {
			return fmt.Errorf("Result extern returning only error requires Void ok type, got %s", valueInfo.Name)
		}
		return nil
	case 2:
		if !fnType.Out(1).Implements(errorInterface) {
			return fmt.Errorf("Result extern second return must be error, got %s", fnType.Out(1))
		}
		return vm.validateHostType(returnInfo.Value, fnType.Out(0), false)
	default:
		return fmt.Errorf("Result extern must return error or (value, error), got %d values", fnType.NumOut())
	}
}

func (vm *VM) validateHostResultType(typeInfo air.TypeInfo, target reflect.Type, param bool) error {
	valueField, ok := target.FieldByName("Value")
	if !ok {
		return fmt.Errorf("host Result type %s missing Value field", target)
	}
	errorField, ok := target.FieldByName("Error")
	if !ok {
		return fmt.Errorf("host Result type %s missing Error field", target)
	}
	okField, ok := target.FieldByName("Ok")
	if !ok {
		return fmt.Errorf("host Result type %s missing Ok field", target)
	}
	if okField.Type.Kind() != reflect.Bool {
		return fmt.Errorf("host Result type %s Ok field must be bool, got %s", target, okField.Type)
	}
	if err := vm.validateHostType(typeInfo.Value, valueField.Type, param); err != nil {
		return fmt.Errorf("Result value: %w", err)
	}
	if err := vm.validateHostType(typeInfo.Error, errorField.Type, param); err != nil {
		return fmt.Errorf("Result error: %w", err)
	}
	return nil
}

func (vm *VM) validateHostType(typeID air.TypeID, target reflect.Type, param bool) error {
	typeInfo, err := vm.typeInfo(typeID)
	if err != nil {
		return err
	}
	if target == anyInterface && typeInfo.Kind != air.TypeDynamic && typeInfo.Kind != air.TypeExtern && typeInfo.Kind != air.TypeUnion && !vm.isEncodableTraitObject(typeInfo) {
		if param {
			return fmt.Errorf("empty interface parameters are only supported for Dynamic, extern, union, and Encodable trait extern values, got %s", typeInfo.Name)
		}
		return fmt.Errorf("empty interface returns are only supported for Dynamic, extern, union, and Encodable trait extern values, got %s", typeInfo.Name)
	}
	switch typeInfo.Kind {
	case air.TypeVoid:
		if target.Kind() == reflect.Struct && target.NumField() == 0 {
			return nil
		}
	case air.TypeInt, air.TypeEnum:
		if target.Kind() == reflect.Int {
			return nil
		}
	case air.TypeFloat:
		if target.Kind() == reflect.Float64 {
			return nil
		}
	case air.TypeBool:
		if target.Kind() == reflect.Bool {
			return nil
		}
	case air.TypeStr:
		if target.Kind() == reflect.String {
			return nil
		}
	case air.TypeDynamic:
		if target == anyInterface {
			return nil
		}
		return fmt.Errorf("Dynamic must use host any, got %s", target)
	case air.TypeUnion:
		if target == anyInterface {
			return nil
		}
		return fmt.Errorf("union %s must use host any until generated tagged union adapters exist, got %s", typeInfo.Name, target)
	case air.TypeTraitObject:
		if target == anyInterface && vm.isEncodableTraitObject(typeInfo) {
			return nil
		}
	case air.TypeList:
		if target.Kind() != reflect.Slice {
			return fmt.Errorf("list %s must use host slice, got %s", typeInfo.Name, target)
		}
		return vm.validateHostType(typeInfo.Elem, target.Elem(), param)
	case air.TypeMap:
		if target.Kind() != reflect.Map {
			return fmt.Errorf("map %s must use host map, got %s", typeInfo.Name, target)
		}
		if err := vm.validateHostType(typeInfo.Key, target.Key(), param); err != nil {
			return fmt.Errorf("map key: %w", err)
		}
		if err := vm.validateHostType(typeInfo.Value, target.Elem(), param); err != nil {
			return fmt.Errorf("map value: %w", err)
		}
		return nil
	case air.TypeMaybe:
		return vm.validateHostMaybeType(typeInfo, target, param)
	case air.TypeStruct:
		if param && target.Kind() == reflect.Pointer {
			return vm.validateHostStructType(typeInfo, target.Elem(), param)
		}
		return vm.validateHostStructType(typeInfo, target, param)
	case air.TypeExtern:
		return vm.validateHostExternType(typeInfo, target)
	case air.TypeFunction:
		return vm.validateHostFunctionType(typeInfo, target, param)
	}
	return fmt.Errorf("AIR type %s cannot be represented as host %s", typeInfo.Name, target)
}

func (vm *VM) validateHostMaybeType(typeInfo air.TypeInfo, target reflect.Type, param bool) error {
	if isHostMaybeType(target) {
		valueField, ok := target.FieldByName("Value")
		if !ok {
			return fmt.Errorf("host Maybe type %s missing Value field", target)
		}
		return vm.validateHostType(typeInfo.Elem, valueField.Type, param)
	}
	if target.Kind() == reflect.Pointer {
		return vm.validateHostType(typeInfo.Elem, target.Elem(), param)
	}
	return fmt.Errorf("Maybe %s must use host Maybe[T], got %s", typeInfo.Name, target)
}

func (vm *VM) validateHostStructType(typeInfo air.TypeInfo, target reflect.Type, param bool) error {
	if target.Kind() != reflect.Struct {
		return fmt.Errorf("struct %s must use host struct, got %s", typeInfo.Name, target)
	}
	for _, field := range typeInfo.Fields {
		hostField, ok := target.FieldByName(goExportedName(field.Name))
		if !ok {
			return fmt.Errorf("host struct %s missing field %s", target, goExportedName(field.Name))
		}
		if hostField.PkgPath != "" {
			return fmt.Errorf("host struct field %s is not exported", goExportedName(field.Name))
		}
		if err := vm.validateHostType(field.Type, hostField.Type, param); err != nil {
			return fmt.Errorf("field %s: %w", field.Name, err)
		}
	}
	return nil
}

func (vm *VM) validateHostExternType(typeInfo air.TypeInfo, target reflect.Type) error {
	if target == anyInterface {
		return nil
	}
	if target.Kind() == reflect.Struct {
		field, ok := target.FieldByName("Handle")
		if !ok {
			return fmt.Errorf("host extern struct %s missing Handle field", target)
		}
		if field.PkgPath != "" {
			return fmt.Errorf("host extern struct %s Handle field is not exported", target)
		}
		return nil
	}
	return fmt.Errorf("extern type %s must use host extern struct or any handle, got %s", typeInfo.Name, target)
}

func (vm *VM) validateHostFunctionType(typeInfo air.TypeInfo, target reflect.Type, param bool) error {
	if !isHostCallbackType(target) {
		return fmt.Errorf("function type %s must use host callback handle, got %s", typeInfo.Name, target)
	}
	callType, err := callbackCallType(target)
	if err != nil {
		return err
	}
	if callType.NumIn() != len(typeInfo.Params) {
		return fmt.Errorf("callback %s expects %d args, got %d", target, len(typeInfo.Params), callType.NumIn())
	}
	for i, paramType := range typeInfo.Params {
		if err := vm.validateHostType(paramType, callType.In(i), param); err != nil {
			return fmt.Errorf("callback arg %d: %w", i, err)
		}
	}
	if callType.NumOut() != 2 {
		return fmt.Errorf("callback %s Call must return (value, error), got %d returns", target, callType.NumOut())
	}
	if !callType.Out(1).Implements(errorInterface) {
		return fmt.Errorf("callback %s second return must be error, got %s", target, callType.Out(1))
	}
	if err := vm.validateHostType(typeInfo.Return, callType.Out(0), false); err != nil {
		return fmt.Errorf("callback return: %w", err)
	}
	return nil
}

func (vm *VM) valueToHost(value Value, target reflect.Type) (reflect.Value, error) {
	if isHostMaybeType(target) {
		maybeValue, err := value.maybeValue()
		if err != nil {
			return reflect.Value{}, err
		}
		out := reflect.New(target).Elem()
		if !maybeValue.Some {
			return out, nil
		}
		valueField := out.FieldByName("Value")
		inner, err := vm.valueToHost(maybeValue.Value, valueField.Type())
		if err != nil {
			return reflect.Value{}, err
		}
		valueField.Set(inner)
		out.FieldByName("Some").SetBool(true)
		return out, nil
	}
	if value.Kind == ValueExtern {
		return vm.externToHost(value, target)
	}
	if value.Kind == ValueUnion && target.Kind() == reflect.Interface && target.NumMethod() == 0 {
		return vm.unionToHost(value, target)
	}
	if value.Kind == ValueTraitObject && target.Kind() == reflect.Interface && target.NumMethod() == 0 {
		return vm.traitObjectToHost(value, target)
	}
	if value.Kind == ValueClosure && isHostCallbackType(target) {
		return vm.closureToHostCallback(value, target)
	}
	if value.Kind == ValueList && target.Kind() == reflect.Slice {
		return vm.listToHost(value, target)
	}
	if value.Kind == ValueMap && target.Kind() == reflect.Map {
		return vm.mapToHost(value, target)
	}
	if target.Kind() == reflect.Interface && target.NumMethod() == 0 {
		if value.Kind == ValueDynamic {
			return vm.dynamicToHost(value, target)
		}
		typeInfo, err := vm.typeInfo(value.Type)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.Value{}, fmt.Errorf("empty interface parameters are only supported for Dynamic, extern, union, and Encodable trait extern values, got %s", typeInfo.Name)
	}
	if target.Kind() == reflect.Pointer {
		if value.Kind == ValueStruct {
			inner, err := vm.structToHost(value, target.Elem())
			if err != nil {
				return reflect.Value{}, err
			}
			out := reflect.New(target.Elem())
			out.Elem().Set(inner)
			return out, nil
		}
		if value.Kind == ValueMaybe {
			maybeValue, err := value.maybeValue()
			if err != nil {
				return reflect.Value{}, err
			}
			if !maybeValue.Some {
				return reflect.Zero(target), nil
			}
			inner, err := vm.valueToHost(maybeValue.Value, target.Elem())
			if err != nil {
				return reflect.Value{}, err
			}
			out := reflect.New(target.Elem())
			out.Elem().Set(inner)
			return out, nil
		}
		return reflect.Value{}, fmt.Errorf("cannot pass value kind %d as %s", value.Kind, target)
	}
	switch target.Kind() {
	case reflect.Int:
		if value.Kind != ValueInt && value.Kind != ValueEnum {
			return reflect.Value{}, fmt.Errorf("expected int-compatible value, got kind %d", value.Kind)
		}
		return reflect.ValueOf(value.Int).Convert(target), nil
	case reflect.Float64:
		if value.Kind != ValueFloat {
			return reflect.Value{}, fmt.Errorf("expected float value, got kind %d", value.Kind)
		}
		return reflect.ValueOf(value.Float).Convert(target), nil
	case reflect.Bool:
		if value.Kind != ValueBool {
			return reflect.Value{}, fmt.Errorf("expected bool value, got kind %d", value.Kind)
		}
		return reflect.ValueOf(value.Bool).Convert(target), nil
	case reflect.String:
		if value.Kind != ValueStr {
			return reflect.Value{}, fmt.Errorf("expected string value, got kind %d", value.Kind)
		}
		return reflect.ValueOf(value.Str).Convert(target), nil
	case reflect.Struct:
		if value.Kind == ValueVoid && target.NumField() == 0 {
			return reflect.Zero(target), nil
		}
		if value.Kind == ValueStruct {
			return vm.structToHost(value, target)
		}
	}
	return reflect.Value{}, fmt.Errorf("unsupported host parameter type %s", target)
}

func (vm *VM) hostReturnsToValue(returnType air.TypeID, returns []reflect.Value) (Value, error) {
	returnInfo, err := vm.typeInfo(returnType)
	if err != nil {
		return Value{}, err
	}
	if len(returns) == 0 {
		return vm.zeroValue(returnType), nil
	}
	if returnInfo.Kind == air.TypeResult {
		return vm.hostReturnsToResult(returnInfo, returnType, returns)
	}
	if len(returns) != 1 {
		return Value{}, fmt.Errorf("extern returned %d values for non-Result type", len(returns))
	}
	return vm.hostValueToValue(returnType, returns[0])
}

func (vm *VM) hostReturnsToResult(returnInfo air.TypeInfo, returnType air.TypeID, returns []reflect.Value) (Value, error) {
	if len(returns) == 1 && isHostResultType(returns[0].Type()) {
		return vm.hostResultToValue(returnInfo, returnType, returns[0])
	}
	if len(returns) == 1 && isErrorValue(returns[0]) {
		if !returns[0].IsNil() {
			return vm.resultErr(returnType, returnInfo.Error, returns[0].Interface().(error))
		}
		return Result(returnType, true, vm.zeroValue(returnInfo.Value)), nil
	}
	if len(returns) == 2 && isErrorValue(returns[1]) {
		if !returns[1].IsNil() {
			return vm.resultErr(returnType, returnInfo.Error, returns[1].Interface().(error))
		}
		value, err := vm.hostValueToValue(returnInfo.Value, returns[0])
		if err != nil {
			return Value{}, err
		}
		return Result(returnType, true, value), nil
	}
	return Value{}, fmt.Errorf("Result extern must return error or (value, error), got %d values", len(returns))
}

func (vm *VM) hostResultToValue(returnInfo air.TypeInfo, returnType air.TypeID, value reflect.Value) (Value, error) {
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return Result(returnType, false, vm.zeroValue(returnInfo.Error)), nil
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return Value{}, fmt.Errorf("host Result value must be struct, got %s", value.Type())
	}
	if value.FieldByName("Ok").Bool() {
		okValue, err := vm.hostValueToValue(returnInfo.Value, value.FieldByName("Value"))
		if err != nil {
			return Value{}, err
		}
		return Result(returnType, true, okValue), nil
	}
	errValue, err := vm.hostValueToValue(returnInfo.Error, value.FieldByName("Error"))
	if err != nil {
		return Value{}, err
	}
	return Result(returnType, false, errValue), nil
}

func (vm *VM) resultErr(resultType, errType air.TypeID, err error) (Value, error) {
	errValue, convertErr := vm.hostValueToValue(errType, reflect.ValueOf(err.Error()))
	if convertErr != nil {
		return Value{}, convertErr
	}
	return Result(resultType, false, errValue), nil
}

func (vm *VM) hostValueToValue(typeID air.TypeID, value reflect.Value) (Value, error) {
	typeInfo, err := vm.typeInfo(typeID)
	if err != nil {
		return Value{}, err
	}
	if !value.IsValid() {
		if typeInfo.Kind == air.TypeMaybe {
			return Maybe(typeID, false, vm.zeroValue(typeInfo.Elem)), nil
		}
		return vm.zeroValue(typeID), nil
	}
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return vm.zeroValue(typeID), nil
		}
		value = value.Elem()
	}
	if typeInfo.Kind == air.TypeMaybe && isHostMaybeType(value.Type()) {
		some := value.FieldByName("Some").Bool()
		if !some {
			return Maybe(typeID, false, vm.zeroValue(typeInfo.Elem)), nil
		}
		inner, err := vm.hostValueToValue(typeInfo.Elem, value.FieldByName("Value"))
		if err != nil {
			return Value{}, err
		}
		return Maybe(typeID, true, inner), nil
	}
	switch typeInfo.Kind {
	case air.TypeVoid:
		return Void(typeID), nil
	case air.TypeInt:
		if value.Kind() < reflect.Int || value.Kind() > reflect.Int64 {
			return Value{}, fmt.Errorf("cannot convert %s to Int", value.Type())
		}
		return Int(typeID, int(value.Int())), nil
	case air.TypeEnum:
		if value.Kind() < reflect.Int || value.Kind() > reflect.Int64 {
			return Value{}, fmt.Errorf("cannot convert %s to enum %s", value.Type(), typeInfo.Name)
		}
		return Enum(typeID, int(value.Int())), nil
	case air.TypeFloat:
		if value.Kind() != reflect.Float64 {
			return Value{}, fmt.Errorf("cannot convert %s to Float", value.Type())
		}
		return Float(typeID, value.Float()), nil
	case air.TypeBool:
		if value.Kind() != reflect.Bool {
			return Value{}, fmt.Errorf("cannot convert %s to Bool", value.Type())
		}
		return Bool(typeID, value.Bool()), nil
	case air.TypeStr:
		if value.Kind() != reflect.String {
			return Value{}, fmt.Errorf("cannot convert %s to Str", value.Type())
		}
		return Str(typeID, value.String()), nil
	case air.TypeDynamic:
		if !value.CanInterface() {
			return Value{}, fmt.Errorf("cannot capture Dynamic value %s", value.Type())
		}
		return Dynamic(typeID, value.Interface()), nil
	case air.TypeList:
		return vm.hostListToValue(typeInfo, value)
	case air.TypeMap:
		return vm.hostMapToValue(typeInfo, value)
	case air.TypeMaybe:
		if value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return Maybe(typeID, false, vm.zeroValue(typeInfo.Elem)), nil
			}
			value = value.Elem()
		}
		inner, err := vm.hostValueToValue(typeInfo.Elem, value)
		if err != nil {
			return Value{}, err
		}
		return Maybe(typeID, true, inner), nil
	case air.TypeStruct:
		return vm.hostStructToValue(typeInfo, value)
	case air.TypeExtern:
		return vm.hostExternToValue(typeInfo.ID, value)
	default:
		return Value{}, fmt.Errorf("unsupported host return AIR type %s", typeInfo.Name)
	}
}

func (vm *VM) externToHost(value Value, target reflect.Type) (reflect.Value, error) {
	externValue, err := value.externValue()
	if err != nil {
		return reflect.Value{}, err
	}
	if externValue.Handle == nil {
		return reflect.Zero(target), nil
	}
	handle := reflect.ValueOf(externValue.Handle)
	if handle.Type().AssignableTo(target) {
		return handle, nil
	}
	if handle.Type().ConvertibleTo(target) {
		return handle.Convert(target), nil
	}
	if target.Kind() == reflect.Interface && target.NumMethod() == 0 {
		return handle, nil
	}
	if target.Kind() == reflect.Struct {
		out := reflect.New(target).Elem()
		handleField := out.FieldByName("Handle")
		if !handleField.IsValid() {
			return reflect.Value{}, fmt.Errorf("host extern struct %s missing Handle field", target)
		}
		if !handleField.CanSet() {
			return reflect.Value{}, fmt.Errorf("host extern struct %s Handle field cannot be set", target)
		}
		if handle.Type().AssignableTo(handleField.Type()) {
			handleField.Set(handle)
			return out, nil
		}
		if handle.Type().ConvertibleTo(handleField.Type()) {
			handleField.Set(handle.Convert(handleField.Type()))
			return out, nil
		}
		if handleField.Type().Kind() == reflect.Interface && handleField.Type().NumMethod() == 0 {
			handleField.Set(handle)
			return out, nil
		}
		return reflect.Value{}, fmt.Errorf("cannot assign extern handle %s to %s.Handle", handle.Type(), target)
	}
	return reflect.Value{}, fmt.Errorf("cannot pass extern handle as %s", target)
}

func (vm *VM) hostExternToValue(typeID air.TypeID, value reflect.Value) (Value, error) {
	if !value.IsValid() {
		return Extern(typeID, nil), nil
	}
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return Extern(typeID, nil), nil
		}
		value = value.Elem()
	}
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return Extern(typeID, nil), nil
	}
	if value.Kind() == reflect.Struct {
		handle := value.FieldByName("Handle")
		if handle.IsValid() && handle.CanInterface() {
			return Extern(typeID, handle.Interface()), nil
		}
	}
	if value.CanInterface() {
		return Extern(typeID, value.Interface()), nil
	}
	return Value{}, fmt.Errorf("cannot capture host extern value %s", value.Type())
}

func (vm *VM) dynamicToHost(value Value, target reflect.Type) (reflect.Value, error) {
	dynamicValue, err := value.dynamicValue()
	if err != nil {
		return reflect.Value{}, err
	}
	if dynamicValue.Raw == nil {
		return reflect.Zero(target), nil
	}
	raw := reflect.ValueOf(dynamicValue.Raw)
	if raw.Type().AssignableTo(target) {
		return raw, nil
	}
	return reflect.Value{}, fmt.Errorf("cannot pass Dynamic payload %s as %s", raw.Type(), target)
}

func (vm *VM) unionToHost(value Value, target reflect.Type) (reflect.Value, error) {
	unionValue, err := value.unionValue()
	if err != nil {
		return reflect.Value{}, err
	}
	member := unionValue.Value
	switch member.Kind {
	case ValueVoid:
		return reflect.Zero(target), nil
	case ValueInt:
		return reflect.ValueOf(member.Int), nil
	case ValueFloat:
		return reflect.ValueOf(member.Float), nil
	case ValueBool:
		return reflect.ValueOf(member.Bool), nil
	case ValueStr:
		return reflect.ValueOf(member.Str), nil
	case ValueDynamic:
		return vm.dynamicToHost(member, target)
	case ValueExtern:
		return vm.externToHost(member, target)
	default:
		typeInfo, err := vm.typeInfo(member.Type)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.Value{}, fmt.Errorf("cannot pass union member %s as host any", typeInfo.Name)
	}
}

func (vm *VM) traitObjectToHost(value Value, target reflect.Type) (reflect.Value, error) {
	typeInfo, err := vm.typeInfo(value.Type)
	if err != nil {
		return reflect.Value{}, err
	}
	if !vm.isEncodableTraitObject(typeInfo) {
		return reflect.Value{}, fmt.Errorf("trait object %s cannot be passed as host any", typeInfo.Name)
	}
	traitObject, err := value.traitObjectValue()
	if err != nil {
		return reflect.Value{}, err
	}
	switch traitObject.Value.Kind {
	case ValueInt:
		return reflect.ValueOf(traitObject.Value.Int), nil
	case ValueFloat:
		return reflect.ValueOf(traitObject.Value.Float), nil
	case ValueBool:
		return reflect.ValueOf(traitObject.Value.Bool), nil
	case ValueStr:
		return reflect.ValueOf(traitObject.Value.Str), nil
	default:
		memberInfo, err := vm.typeInfo(traitObject.Value.Type)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.Value{}, fmt.Errorf("Encodable member %s cannot be passed as host any", memberInfo.Name)
	}
}

func (vm *VM) isEncodableTraitObject(typeInfo air.TypeInfo) bool {
	if typeInfo.Kind != air.TypeTraitObject {
		return false
	}
	if typeInfo.Trait < 0 || int(typeInfo.Trait) >= len(vm.program.Traits) {
		return false
	}
	return vm.program.Traits[typeInfo.Trait].Name == "Encodable"
}

func (vm *VM) closureToHostCallback(value Value, target reflect.Type) (reflect.Value, error) {
	closure, err := value.closureValue()
	if err != nil {
		return reflect.Value{}, err
	}
	if closure.Function < 0 || int(closure.Function) >= len(vm.program.Functions) {
		return reflect.Value{}, fmt.Errorf("invalid closure function id %d", closure.Function)
	}
	fn := vm.program.Functions[closure.Function]
	callType, err := callbackCallType(target)
	if err != nil {
		return reflect.Value{}, err
	}
	if callType.NumIn() != len(fn.Signature.Params) {
		return reflect.Value{}, fmt.Errorf("callback %s expects %d args, closure accepts %d", target, callType.NumIn(), len(fn.Signature.Params))
	}
	callback := reflect.MakeFunc(callType, func(inputs []reflect.Value) []reflect.Value {
		zero := reflect.Zero(callType.Out(0))
		errorValue := reflect.Zero(errorInterface)
		args := make([]Value, len(inputs))
		for i, input := range inputs {
			arg, err := vm.hostValueToValue(fn.Signature.Params[i].Type, input)
			if err != nil {
				return []reflect.Value{zero, reflect.ValueOf(err)}
			}
			args[i] = arg
		}
		result, err := vm.callClosure(value, args)
		if err != nil {
			return []reflect.Value{zero, reflect.ValueOf(err)}
		}
		for i, input := range inputs {
			if i >= len(fn.Signature.Params) || !fn.Signature.Params[i].Mutable {
				continue
			}
			if input.Kind() != reflect.Pointer || input.IsNil() {
				continue
			}
			hostArg, err := vm.valueToHost(args[i], input.Type().Elem())
			if err != nil {
				return []reflect.Value{zero, reflect.ValueOf(err)}
			}
			if !input.Elem().CanSet() {
				return []reflect.Value{zero, reflect.ValueOf(fmt.Errorf("callback arg %d is not settable", i))}
			}
			input.Elem().Set(hostArg)
		}
		hostResult, err := vm.valueToHost(result, callType.Out(0))
		if err != nil {
			return []reflect.Value{zero, reflect.ValueOf(err)}
		}
		return []reflect.Value{hostResult, errorValue}
	})
	out := reflect.New(target).Elem()
	out.FieldByName("Call").Set(callback)
	return out, nil
}

func (vm *VM) listToHost(value Value, target reflect.Type) (reflect.Value, error) {
	listValue, err := value.listValue()
	if err != nil {
		return reflect.Value{}, err
	}
	out := reflect.MakeSlice(target, len(listValue.Items), len(listValue.Items))
	for i, item := range listValue.Items {
		hostItem, err := vm.valueToHost(item, target.Elem())
		if err != nil {
			return reflect.Value{}, fmt.Errorf("list item %d: %w", i, err)
		}
		out.Index(i).Set(hostItem)
	}
	return out, nil
}

func (vm *VM) hostListToValue(typeInfo air.TypeInfo, value reflect.Value) (Value, error) {
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return List(typeInfo.ID, nil), nil
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
		return Value{}, fmt.Errorf("cannot convert %s to list %s", value.Type(), typeInfo.Name)
	}
	items := make([]Value, value.Len())
	for i := 0; i < value.Len(); i++ {
		item, err := vm.hostValueToValue(typeInfo.Elem, value.Index(i))
		if err != nil {
			return Value{}, fmt.Errorf("list item %d: %w", i, err)
		}
		items[i] = item
	}
	return List(typeInfo.ID, items), nil
}

func (vm *VM) mapToHost(value Value, target reflect.Type) (reflect.Value, error) {
	mapValue, err := value.mapValue()
	if err != nil {
		return reflect.Value{}, err
	}
	out := reflect.MakeMapWithSize(target, len(mapValue.Entries))
	for i, entry := range mapValue.Entries {
		hostKey, err := vm.valueToHost(entry.Key, target.Key())
		if err != nil {
			return reflect.Value{}, fmt.Errorf("map key %d: %w", i, err)
		}
		hostValue, err := vm.valueToHost(entry.Value, target.Elem())
		if err != nil {
			return reflect.Value{}, fmt.Errorf("map value %d: %w", i, err)
		}
		out.SetMapIndex(hostKey, hostValue)
	}
	return out, nil
}

func (vm *VM) hostMapToValue(typeInfo air.TypeInfo, value reflect.Value) (Value, error) {
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return Map(typeInfo.ID, nil), nil
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Map {
		return Value{}, fmt.Errorf("cannot convert %s to map %s", value.Type(), typeInfo.Name)
	}
	keys := value.MapKeys()
	entries := make([]MapEntryValue, 0, len(keys))
	for i, key := range keys {
		mapKey, err := vm.hostValueToValue(typeInfo.Key, key)
		if err != nil {
			return Value{}, fmt.Errorf("map key %d: %w", i, err)
		}
		mapValue, err := vm.hostValueToValue(typeInfo.Value, value.MapIndex(key))
		if err != nil {
			return Value{}, fmt.Errorf("map value %d: %w", i, err)
		}
		entries = append(entries, MapEntryValue{Key: mapKey, Value: mapValue})
	}
	return Map(typeInfo.ID, entries), nil
}

func (vm *VM) structToHost(value Value, target reflect.Type) (reflect.Value, error) {
	typeInfo, err := vm.typeInfo(value.Type)
	if err != nil {
		return reflect.Value{}, err
	}
	if typeInfo.Kind != air.TypeStruct {
		return reflect.Value{}, fmt.Errorf("cannot pass AIR type %s as Go struct", typeInfo.Name)
	}
	structValue, err := value.structValue()
	if err != nil {
		return reflect.Value{}, err
	}
	out := reflect.New(target).Elem()
	for _, field := range typeInfo.Fields {
		if field.Index < 0 || field.Index >= len(structValue.Fields) {
			return reflect.Value{}, fmt.Errorf("invalid field index %d on %s", field.Index, typeInfo.Name)
		}
		hostField := out.FieldByName(goExportedName(field.Name))
		if !hostField.IsValid() {
			return reflect.Value{}, fmt.Errorf("host struct %s missing field %s", target, goExportedName(field.Name))
		}
		if !hostField.CanSet() {
			return reflect.Value{}, fmt.Errorf("host struct field %s cannot be set", goExportedName(field.Name))
		}
		hostValue, err := vm.valueToHost(structValue.Fields[field.Index], hostField.Type())
		if err != nil {
			return reflect.Value{}, fmt.Errorf("field %s: %w", field.Name, err)
		}
		hostField.Set(hostValue)
	}
	return out, nil
}

func (vm *VM) hostStructToValue(typeInfo air.TypeInfo, value reflect.Value) (Value, error) {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return vm.zeroValue(typeInfo.ID), nil
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return Value{}, fmt.Errorf("cannot convert %s to struct %s", value.Type(), typeInfo.Name)
	}
	fields := make([]Value, len(typeInfo.Fields))
	for _, field := range typeInfo.Fields {
		hostField := value.FieldByName(goExportedName(field.Name))
		if !hostField.IsValid() {
			return Value{}, fmt.Errorf("host struct %s missing field %s", value.Type(), goExportedName(field.Name))
		}
		fieldValue, err := vm.hostValueToValue(field.Type, hostField)
		if err != nil {
			return Value{}, fmt.Errorf("field %s: %w", field.Name, err)
		}
		fields[field.Index] = fieldValue
	}
	return Struct(typeInfo.ID, fields), nil
}

func isErrorValue(value reflect.Value) bool {
	return value.IsValid() && value.Type().Implements(errorInterface)
}

func isHostMaybeType(typ reflect.Type) bool {
	return typ.Kind() == reflect.Struct &&
		typ.PkgPath() == hostMaybeType.PkgPath() &&
		strings.HasPrefix(typ.Name(), "Maybe[")
}

func isHostResultType(typ reflect.Type) bool {
	return typ.Kind() == reflect.Struct &&
		typ.PkgPath() == hostResultType.PkgPath() &&
		strings.HasPrefix(typ.Name(), "Result[")
}

func isHostCallbackType(typ reflect.Type) bool {
	return typ.Kind() == reflect.Struct &&
		typ.PkgPath() == hostMaybeType.PkgPath() &&
		strings.HasPrefix(typ.Name(), "Callback")
}

func callbackCallType(typ reflect.Type) (reflect.Type, error) {
	field, ok := typ.FieldByName("Call")
	if !ok {
		return nil, fmt.Errorf("host callback %s missing Call field", typ)
	}
	if field.PkgPath != "" {
		return nil, fmt.Errorf("host callback %s Call field is not exported", typ)
	}
	if field.Type.Kind() != reflect.Func {
		return nil, fmt.Errorf("host callback %s Call field must be func, got %s", typ, field.Type)
	}
	return field.Type, nil
}

func goExportedName(name string) string {
	if !strings.ContainsAny(name, "_- :") {
		return goUpperFirst(name)
	}
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == ' ' || r == ':'
	})
	if len(parts) == 0 {
		return "Value"
	}
	for i := range parts {
		if containsUpper(parts[i]) {
			parts[i] = goUpperFirst(parts[i])
		} else {
			parts[i] = goUpperFirst(strings.ToLower(parts[i]))
		}
	}
	result := strings.Join(parts, "")
	if result == "" {
		return "Value"
	}
	return result
}

func goUpperFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func containsUpper(value string) bool {
	for _, r := range value {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}
