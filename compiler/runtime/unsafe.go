package runtime

import "reflect"

// IsNil reports whether value is nil, including typed nil values boxed in any.
func IsNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// TraitSnapshot removes one dynamic pointer layer from a trait value and
// returns an independent shallow copy. When only the pointer method set
// satisfies T, it returns a pointer to fresh copied storage instead.
func TraitSnapshot[T any](value T) T {
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Pointer {
		return value
	}
	if reflected.IsNil() {
		panic("cannot dereference a nil mutable trait")
	}

	elem := reflected.Elem()
	target := reflect.TypeOf((*T)(nil)).Elem()
	if elem.Type().AssignableTo(target) || (target.Kind() == reflect.Interface && elem.Type().Implements(target)) {
		return elem.Interface().(T)
	}
	copy := reflect.New(elem.Type())
	copy.Elem().Set(elem)
	if snapshot, ok := copy.Interface().(T); ok {
		return snapshot
	}
	panic("dereferenced mutable trait value no longer implements its trait")
}
