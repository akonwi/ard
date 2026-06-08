package runtime

import (
	"reflect"
	"strings"
)

func Equal(left, right any) bool {
	return equalValues(reflect.ValueOf(left), reflect.ValueOf(right))
}

func equalValues(left, right reflect.Value) bool {
	if !left.IsValid() || !right.IsValid() {
		return !left.IsValid() && !right.IsValid()
	}
	if left.Type() != right.Type() {
		return false
	}
	if isRuntimeGenericValue(left, "Maybe") {
		return maybeValuesEqual(left, right)
	}
	if isRuntimeGenericValue(left, "StructuralMap") {
		return structuralMapValuesEqual(left, right)
	}

	switch left.Kind() {
	case reflect.Bool:
		return left.Bool() == right.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return left.Int() == right.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return left.Uint() == right.Uint()
	case reflect.Float32, reflect.Float64:
		return left.Float() == right.Float()
	case reflect.Complex64, reflect.Complex128:
		return left.Complex() == right.Complex()
	case reflect.String:
		return left.String() == right.String()
	case reflect.Pointer:
		if left.IsNil() || right.IsNil() {
			return left.IsNil() && right.IsNil()
		}
		return equalValues(left.Elem(), right.Elem())
	case reflect.Interface:
		if left.IsNil() || right.IsNil() {
			return left.IsNil() && right.IsNil()
		}
		return equalValues(left.Elem(), right.Elem())
	case reflect.Struct:
		for i := 0; i < left.NumField(); i++ {
			if !equalValues(left.Field(i), right.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Array, reflect.Slice:
		if left.Kind() == reflect.Slice && (left.IsNil() || right.IsNil()) {
			return left.IsNil() && right.IsNil()
		}
		if left.Len() != right.Len() {
			return false
		}
		for i := 0; i < left.Len(); i++ {
			if !equalValues(left.Index(i), right.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		if left.IsNil() || right.IsNil() {
			return left.IsNil() && right.IsNil()
		}
		if left.Len() != right.Len() {
			return false
		}
		iter := left.MapRange()
		for iter.Next() {
			rightValue := right.MapIndex(iter.Key())
			if !rightValue.IsValid() || !equalValues(iter.Value(), rightValue) {
				return false
			}
		}
		return true
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		if left.CanInterface() && right.CanInterface() {
			return reflect.DeepEqual(left.Interface(), right.Interface())
		}
		return left.Pointer() == right.Pointer()
	default:
		if left.CanInterface() && right.CanInterface() {
			return reflect.DeepEqual(left.Interface(), right.Interface())
		}
		return false
	}
}

func maybeValuesEqual(left, right reflect.Value) bool {
	leftValue := left.FieldByName("value")
	rightValue := right.FieldByName("value")
	leftNone := leftValue.IsNil()
	rightNone := rightValue.IsNil()
	if leftNone || rightNone {
		return leftNone && rightNone
	}
	return equalValues(leftValue.Elem(), rightValue.Elem())
}

func structuralMapValuesEqual(left, right reflect.Value) bool {
	leftEntries := structuralMapReflectEntries(left)
	rightEntries := structuralMapReflectEntries(right)
	if len(leftEntries) != len(rightEntries) {
		return false
	}
	matched := make([]bool, len(rightEntries))
	for _, leftEntry := range leftEntries {
		found := false
		for i, rightEntry := range rightEntries {
			if matched[i] {
				continue
			}
			if equalValues(leftEntry.FieldByName("Key"), rightEntry.FieldByName("Key")) && equalValues(leftEntry.FieldByName("Value"), rightEntry.FieldByName("Value")) {
				matched[i] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func structuralMapReflectEntries(value reflect.Value) []reflect.Value {
	if value.IsNil() {
		return nil
	}
	entries := make([]reflect.Value, 0)
	iter := value.MapRange()
	for iter.Next() {
		bucket := iter.Value()
		for i := 0; i < bucket.Len(); i++ {
			entries = append(entries, bucket.Index(i))
		}
	}
	return entries
}

func isRuntimeGenericValue(value reflect.Value, name string) bool {
	typeInfo := value.Type()
	return typeInfo.PkgPath() == "github.com/akonwi/ard/runtime" && (typeInfo.Name() == name || strings.HasPrefix(typeInfo.Name(), name+"["))
}
