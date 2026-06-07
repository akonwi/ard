package air

import "github.com/akonwi/ard/checker"

func isRecursiveNullableStructField(owner *checker.StructDef, field checker.Type) bool {
	maybe, ok := field.(*checker.Maybe)
	if !ok {
		return false
	}
	return nullablePayloadReferencesStruct(maybe.Of(), owner, map[checker.Type]struct{}{})
}

func nullablePayloadReferencesStruct(t checker.Type, owner *checker.StructDef, seen map[checker.Type]struct{}) bool {
	if t == nil {
		return false
	}
	if typeVar, ok := t.(*checker.TypeVar); ok && typeVar.Actual() != nil {
		t = typeVar.Actual()
	}
	if _, ok := seen[t]; ok {
		return false
	}
	seen[t] = struct{}{}
	switch typ := t.(type) {
	case *checker.StructDef:
		if sameStructIdentity(typ, owner) {
			return true
		}
		for _, field := range typ.Fields {
			if nullablePayloadReferencesStruct(field, owner, seen) {
				return true
			}
		}
		return false
	case *checker.Result:
		return nullablePayloadReferencesStruct(typ.Val(), owner, seen) || nullablePayloadReferencesStruct(typ.Err(), owner, seen)
	case *checker.Union:
		for _, member := range typ.Types {
			if nullablePayloadReferencesStruct(member, owner, seen) {
				return true
			}
		}
		return false
	case *checker.Maybe:
		return nullablePayloadReferencesStruct(typ.Of(), owner, seen)
	case *checker.MutableRef, *checker.List, *checker.Map, *checker.Trait, *checker.ExternType, *checker.FunctionDef, *checker.ExternalFunctionDef:
		return false
	default:
		return false
	}
}

func hasSelfReference(owner *checker.StructDef) bool {
	for _, field := range owner.Fields {
		if typeReferencesStruct(field, owner, map[checker.Type]struct{}{}) {
			return true
		}
	}
	return false
}

func typeReferencesStruct(t checker.Type, owner *checker.StructDef, seen map[checker.Type]struct{}) bool {
	if t == nil {
		return false
	}
	if typeVar, ok := t.(*checker.TypeVar); ok && typeVar.Actual() != nil {
		t = typeVar.Actual()
	}
	if _, ok := seen[t]; ok {
		return false
	}
	seen[t] = struct{}{}
	switch typ := t.(type) {
	case *checker.StructDef:
		if sameStructIdentity(typ, owner) {
			return true
		}
		for _, field := range typ.Fields {
			if typeReferencesStruct(field, owner, seen) {
				return true
			}
		}
		return false
	case *checker.List:
		return typeReferencesStruct(typ.Of(), owner, seen)
	case *checker.Map:
		return typeReferencesStruct(typ.Key(), owner, seen) || typeReferencesStruct(typ.Value(), owner, seen)
	case *checker.Maybe:
		return typeReferencesStruct(typ.Of(), owner, seen)
	case *checker.Result:
		return typeReferencesStruct(typ.Val(), owner, seen) || typeReferencesStruct(typ.Err(), owner, seen)
	case *checker.MutableRef:
		return typeReferencesStruct(typ.Of(), owner, seen)
	case *checker.Union:
		for _, member := range typ.Types {
			if typeReferencesStruct(member, owner, seen) {
				return true
			}
		}
	}
	return false
}

func sameStructIdentity(left *checker.StructDef, right *checker.StructDef) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left == right {
		return true
	}
	if left.ModulePath != "" && right.ModulePath != "" {
		return left.ModulePath == right.ModulePath && left.Name == right.Name
	}
	return left.Name == right.Name
}
