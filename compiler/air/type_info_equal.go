package air

// typeInfoEqual reports whether two TypeInfo values carry identical metadata.
// It is the hand-written equivalent of reflect.DeepEqual for TypeInfo,
// including DeepEqual's distinction between nil and empty slices, so the
// interner's conflict checks keep their exact semantics without boxing two
// large structs into interfaces and walking them reflectively on every
// repeated interning (which dominated allocation during lowering).
//
// TestTypeInfoEqualMatchesDeepEqual pins this function to the struct's field
// set; extend both when TypeInfo gains a field.
func typeInfoEqual(a, b TypeInfo) bool {
	return a.ID == b.ID &&
		a.Kind == b.Kind &&
		a.Name == b.Name &&
		a.ModulePath == b.ModulePath &&
		a.Private == b.Private &&
		a.Elem == b.Elem &&
		a.Length == b.Length &&
		a.Key == b.Key &&
		a.Value == b.Value &&
		a.Error == b.Error &&
		a.ForeignTarget == b.ForeignTarget &&
		a.ForeignNamespace == b.ForeignNamespace &&
		a.ForeignQualifier == b.ForeignQualifier &&
		a.ForeignSymbol == b.ForeignSymbol &&
		a.ForeignPointer == b.ForeignPointer &&
		a.ForeignInterface == b.ForeignInterface &&
		fieldInfosEqual(a.Fields, b.Fields) &&
		comparableSlicesEqual(a.Variants, b.Variants) &&
		a.EnumOpen == b.EnumOpen &&
		comparableSlicesEqual(a.Members, b.Members) &&
		comparableSlicesEqual(a.Params, b.Params) &&
		a.Return == b.Return &&
		a.Variadic == b.Variadic &&
		a.Trait == b.Trait &&
		comparableSlicesEqual(a.TypeParams, b.TypeParams) &&
		a.ParamOwner == b.ParamOwner &&
		a.ParamIndex == b.ParamIndex &&
		a.Generic == b.Generic &&
		comparableSlicesEqual(a.GenericArgs, b.GenericArgs) &&
		comparableSlicesEqual(a.GenericComparable, b.GenericComparable)
}

func fieldInfosEqual(a, b []FieldInfo) bool {
	if (a == nil) != (b == nil) || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name ||
			a[i].Type != b[i].Type ||
			a[i].Index != b[i].Index ||
			a[i].JSON != b[i].JSON ||
			!comparableSlicesEqual(a[i].GoTags, b[i].GoTags) {
			return false
		}
	}
	return true
}

// comparableSlicesEqual matches reflect.DeepEqual on slices of comparable
// elements: a nil slice and an empty non-nil slice are different.
func comparableSlicesEqual[T comparable](a, b []T) bool {
	if (a == nil) != (b == nil) || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
