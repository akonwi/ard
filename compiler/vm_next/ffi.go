package vm_next

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/akonwi/ard/air"
	stdlibffi "github.com/akonwi/ard/std_lib/ffi"
)

type HostFunctionRegistry map[string]any

var (
	errorInterface = reflect.TypeFor[error]()
	hostMaybeType  = reflect.TypeFor[stdlibffi.Maybe[any]]()
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
	return &VM{program: program, externs: registry}, nil
}

func (vm *VM) callExtern(id air.ExternID, args []Value) (value Value, err error) {
	if id < 0 || int(id) >= len(vm.program.Externs) {
		return Value{}, fmt.Errorf("invalid extern id %d", id)
	}
	extern := vm.program.Externs[id]
	binding := extern.Bindings["go"]
	if binding == "" {
		binding = extern.Name
	}
	fn, ok := vm.externs[binding]
	if !ok {
		return Value{}, fmt.Errorf("extern binding %q is not registered", binding)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			value = Value{}
			err = fmt.Errorf("extern %s panicked: %v", binding, recovered)
		}
	}()
	return vm.invokeHostFunction(extern, binding, fn, args)
}

func (vm *VM) invokeHostFunction(extern air.Extern, binding string, fn any, args []Value) (Value, error) {
	callable := reflect.ValueOf(fn)
	if !callable.IsValid() || callable.Kind() != reflect.Func {
		return Value{}, fmt.Errorf("extern binding %q is %T, want func", binding, fn)
	}
	fnType := callable.Type()
	if fnType.NumIn() != len(args) {
		return Value{}, fmt.Errorf("extern %s expects %d args, got %d", binding, fnType.NumIn(), len(args))
	}
	inputs := make([]reflect.Value, len(args))
	for i, arg := range args {
		input, err := vm.valueToHost(arg, fnType.In(i))
		if err != nil {
			return Value{}, fmt.Errorf("extern %s arg %d: %w", binding, i, err)
		}
		inputs[i] = input
	}
	return vm.hostReturnsToValue(extern.Signature.Return, callable.Call(inputs))
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
	if target.Kind() == reflect.Pointer {
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
	if target.Kind() == reflect.Interface && target.NumMethod() == 0 {
		goValue := value.GoValue()
		if goValue == nil {
			return reflect.Zero(target), nil
		}
		return reflect.ValueOf(goValue), nil
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
