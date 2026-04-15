package ardgo

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

var (
	resultType = reflect.TypeFor[Result[any, any]]()
	maybeType  = reflect.TypeFor[Maybe[any]]()
)

func CoerceExtern[T any](value any) T {
	targetType := reflect.TypeFor[T]()
	coerced, err := coerceExternValue(reflect.ValueOf(value), targetType)
	if err != nil {
		panic(err)
	}
	var zero T
	if !coerced.IsValid() {
		return zero
	}
	if targetType != nil && targetType.Kind() == reflect.Interface && valueCanBeNil(coerced.Kind()) && coerced.IsNil() {
		return zero
	}
	return coerced.Interface().(T)
}

func coerceExternValue(source reflect.Value, targetType reflect.Type) (reflect.Value, error) {
	if !source.IsValid() {
		return zeroValue(targetType)
	}

	for source.Kind() == reflect.Interface {
		if source.IsNil() {
			return zeroValue(targetType)
		}
		source = source.Elem()
	}

	if source.Type().AssignableTo(targetType) {
		return source, nil
	}
	if source.Type().ConvertibleTo(targetType) {
		return source.Convert(targetType), nil
	}

	if isResultType(targetType) {
		return coerceResultValue(source, targetType)
	}
	if isMaybeType(targetType) {
		return coerceMaybeValue(source, targetType)
	}

	switch targetType.Kind() {
	case reflect.Interface:
		if source.Type().AssignableTo(targetType) {
			return source, nil
		}
		if source.Type().ConvertibleTo(targetType) {
			return source.Convert(targetType), nil
		}
		if targetType.NumMethod() == 0 {
			wrapped := reflect.New(targetType).Elem()
			wrapped.Set(source)
			return wrapped, nil
		}
		return reflect.Value{}, fmt.Errorf("cannot coerce %s to interface %s", source.Type(), targetType)
	case reflect.Pointer:
		if source.Kind() == reflect.Pointer {
			if source.IsNil() {
				return reflect.Zero(targetType), nil
			}
			return coercePointerValue(source, targetType)
		}
		coerced, err := coerceExternValue(source, targetType.Elem())
		if err != nil {
			return reflect.Value{}, err
		}
		out := reflect.New(targetType.Elem())
		out.Elem().Set(coerced)
		return out, nil
	case reflect.Struct:
		return coerceStructValue(source, targetType)
	case reflect.Slice:
		return coerceSliceValue(source, targetType)
	case reflect.Map:
		return coerceMapValue(source, targetType)
	default:
		return reflect.Value{}, fmt.Errorf("cannot coerce %s to %s", source.Type(), targetType)
	}
}

func zeroValue(targetType reflect.Type) (reflect.Value, error) {
	if targetType == nil {
		return reflect.Value{}, fmt.Errorf("nil target type")
	}
	return reflect.Zero(targetType), nil
}

func valueCanBeNil(kind reflect.Kind) bool {
	switch kind {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return true
	default:
		return false
	}
}

func coercePointerValue(source reflect.Value, targetType reflect.Type) (reflect.Value, error) {
	if source.IsNil() {
		return reflect.Zero(targetType), nil
	}
	coerced, err := coerceExternValue(source.Elem(), targetType.Elem())
	if err != nil {
		return reflect.Value{}, err
	}
	out := reflect.New(targetType.Elem())
	out.Elem().Set(coerced)
	return out, nil
}

func coerceStructValue(source reflect.Value, targetType reflect.Type) (reflect.Value, error) {
	if source.Kind() == reflect.Pointer {
		if source.IsNil() {
			return reflect.Zero(targetType), nil
		}
		source = source.Elem()
	}
	if source.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("cannot coerce %s to struct %s", source.Type(), targetType)
	}
	out := reflect.New(targetType).Elem()
	for i := 0; i < targetType.NumField(); i++ {
		targetField := targetType.Field(i)
		if !targetField.IsExported() {
			continue
		}
		sourceField := source.FieldByName(targetField.Name)
		if !sourceField.IsValid() {
			continue
		}
		coerced, err := coerceExternValue(sourceField, targetField.Type)
		if err != nil {
			return reflect.Value{}, err
		}
		out.Field(i).Set(coerced)
	}
	return out, nil
}

func coerceSliceValue(source reflect.Value, targetType reflect.Type) (reflect.Value, error) {
	if source.Kind() == reflect.Pointer {
		if source.IsNil() {
			return reflect.Zero(targetType), nil
		}
		source = source.Elem()
	}
	if source.Kind() != reflect.Slice && source.Kind() != reflect.Array {
		return reflect.Value{}, fmt.Errorf("cannot coerce %s to slice %s", source.Type(), targetType)
	}
	out := reflect.MakeSlice(targetType, source.Len(), source.Len())
	for i := 0; i < source.Len(); i++ {
		coerced, err := coerceExternValue(source.Index(i), targetType.Elem())
		if err != nil {
			return reflect.Value{}, err
		}
		out.Index(i).Set(coerced)
	}
	return out, nil
}

func coerceMapValue(source reflect.Value, targetType reflect.Type) (reflect.Value, error) {
	if source.Kind() == reflect.Pointer {
		if source.IsNil() {
			return reflect.Zero(targetType), nil
		}
		source = source.Elem()
	}
	if source.Kind() != reflect.Map {
		return reflect.Value{}, fmt.Errorf("cannot coerce %s to map %s", source.Type(), targetType)
	}
	out := reflect.MakeMapWithSize(targetType, source.Len())
	iter := source.MapRange()
	for iter.Next() {
		key, err := coerceExternValue(iter.Key(), targetType.Key())
		if err != nil {
			return reflect.Value{}, err
		}
		value, err := coerceExternValue(iter.Value(), targetType.Elem())
		if err != nil {
			return reflect.Value{}, err
		}
		out.SetMapIndex(key, value)
	}
	return out, nil
}

func coerceResultValue(source reflect.Value, targetType reflect.Type) (reflect.Value, error) {
	isOk, err := callBoolMethod(source, "IsOk")
	if err != nil {
		return reflect.Value{}, err
	}
	out := reflect.New(targetType).Elem()
	setUnexportedField(out.FieldByName("ok"), reflect.ValueOf(isOk))
	if isOk {
		okValue, err := callValueMethod(source, "UnwrapOk")
		if err != nil {
			return reflect.Value{}, err
		}
		coerced, err := coerceExternValue(okValue, targetType.Field(0).Type)
		if err != nil {
			return reflect.Value{}, err
		}
		setUnexportedField(out.FieldByName("value"), coerced)
		return out, nil
	}
	errValue, err := callValueMethod(source, "UnwrapErr")
	if err != nil {
		return reflect.Value{}, err
	}
	coerced, err := coerceExternValue(errValue, targetType.Field(1).Type)
	if err != nil {
		return reflect.Value{}, err
	}
	setUnexportedField(out.FieldByName("err"), coerced)
	return out, nil
}

func coerceMaybeValue(source reflect.Value, targetType reflect.Type) (reflect.Value, error) {
	isNone, err := callBoolMethod(source, "IsNone")
	if err != nil {
		return reflect.Value{}, err
	}
	out := reflect.New(targetType).Elem()
	setUnexportedField(out.FieldByName("none"), reflect.ValueOf(isNone))
	if isNone {
		return out, nil
	}
	value, err := callValueMethod(source, "Or", reflect.Zero(source.Type().Field(0).Type))
	if err != nil {
		return reflect.Value{}, err
	}
	coerced, err := coerceExternValue(value, targetType.Field(0).Type)
	if err != nil {
		return reflect.Value{}, err
	}
	setUnexportedField(out.FieldByName("value"), coerced)
	return out, nil
}

func callBoolMethod(source reflect.Value, name string) (bool, error) {
	result, err := callValueMethod(source, name)
	if err != nil {
		return false, err
	}
	return result.Bool(), nil
}

func callValueMethod(source reflect.Value, name string, args ...reflect.Value) (reflect.Value, error) {
	method := source.MethodByName(name)
	if !method.IsValid() && source.CanAddr() {
		method = source.Addr().MethodByName(name)
	}
	if !method.IsValid() {
		return reflect.Value{}, fmt.Errorf("source type %s does not implement %s", source.Type(), name)
	}
	results := method.Call(args)
	if len(results) != 1 {
		return reflect.Value{}, fmt.Errorf("method %s on %s returned %d values", name, source.Type(), len(results))
	}
	return results[0], nil
}

func setUnexportedField(field reflect.Value, value reflect.Value) {
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(value)
}

func isResultType(t reflect.Type) bool {
	return t.PkgPath() == resultType.PkgPath() && strings.HasPrefix(t.Name(), "Result[")
}

func isMaybeType(t reflect.Type) bool {
	return t.PkgPath() == maybeType.PkgPath() && strings.HasPrefix(t.Name(), "Maybe[")
}
