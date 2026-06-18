package checker

import "fmt"

type typeEqualKey struct {
	left  string
	right string
}

func equalTypes(left Type, right Type) bool {
	return equalTypesSeen(left, right, map[typeEqualKey]struct{}{})
}

func equalTypesSeen(left Type, right Type, seen map[typeEqualKey]struct{}) bool {
	if left == nil || right == nil {
		return left == right
	}
	key := typeEqualKey{left: typeEqualID(left), right: typeEqualID(right)}
	if _, ok := seen[key]; ok {
		return true
	}
	seen[key] = struct{}{}

	if r, ok := right.(*TypeVar); ok {
		if l, leftIsTypeVar := left.(*TypeVar); leftIsTypeVar && l == r {
			return true
		}
		return r.actual == nil || equalTypesSeen(left, r.actual, seen)
	}

	switch l := left.(type) {
	case *Trait:
		r, ok := right.(*Trait)
		if !ok || l.Name != r.Name || len(l.methods) != len(r.methods) {
			return false
		}
		for i := range l.methods {
			if !equalTypesSeen(&l.methods[i], &r.methods[i], seen) {
				return false
			}
		}
		return true
	case Trait:
		return equalTypesSeen(&l, right, seen)
	case *List:
		if r, ok := right.(*List); ok {
			return equalTypesSeen(l.of, r.of, seen)
		}
		if r, ok := right.(*TypeVar); ok {
			return r.actual == nil || equalTypesSeen(l, r.actual, seen)
		}
		if r, ok := right.(*Union); ok {
			return equalTypesSeen(r, l, seen)
		}
		return false
	case *Map:
		if r, ok := right.(*Map); ok {
			return equalTypesSeen(l.key, r.key, seen) && equalTypesSeen(l.value, r.value, seen)
		}
		if r, ok := right.(Map); ok {
			return equalTypesSeen(l.key, r.key, seen) && equalTypesSeen(l.value, r.value, seen)
		}
		if r, ok := right.(*TypeVar); ok {
			return r.actual == nil || equalTypesSeen(l, r.actual, seen)
		}
		if r, ok := right.(*Union); ok {
			return equalTypesSeen(r, l, seen)
		}
		return false
	case Map:
		if r, ok := right.(*Map); ok {
			return equalTypesSeen(l.key, r.key, seen) && equalTypesSeen(l.value, r.value, seen)
		}
		if r, ok := right.(Map); ok {
			return equalTypesSeen(l.key, r.key, seen) && equalTypesSeen(l.value, r.value, seen)
		}
		if r, ok := right.(*TypeVar); ok {
			return r.actual == nil || equalTypesSeen(l, r.actual, seen)
		}
		if r, ok := right.(*Union); ok {
			return equalTypesSeen(r, l, seen)
		}
		return false
	case *Maybe:
		r, ok := right.(*Maybe)
		return ok && equalTypesSeen(l.of, r.of, seen)
	case *TypeVar:
		if l == right {
			return true
		}
		return l.actual == nil || equalTypesSeen(l.actual, right, seen)
	case *Result:
		if r, ok := right.(*Result); ok {
			return equalTypesSeen(l.val, r.val, seen) && equalTypesSeen(l.err, r.err, seen)
		}
		if r, ok := right.(*TypeVar); ok {
			return r.actual == nil || equalTypesSeen(l, r.actual, seen)
		}
		return false
	case *MutableRef:
		if r, ok := right.(*MutableRef); ok {
			return equalTypesSeen(l.of, r.of, seen)
		}
		if r, ok := right.(*TypeVar); ok {
			return r.actual == nil || equalTypesSeen(l, r.actual, seen)
		}
		return false
	case *ExternType:
		if r, ok := right.(*TypeVar); ok && r.actual == nil {
			return true
		}
		r, ok := right.(*ExternType)
		if !ok || !externTypeNamesMatch(l, r) || len(l.TypeArgs) != len(r.TypeArgs) {
			return false
		}
		for i := range l.TypeArgs {
			if !equalTypesSeen(l.TypeArgs[i], r.TypeArgs[i], seen) {
				return false
			}
		}
		return true
	case *FunctionDef:
		return equalFunctionDefSeen(*l, right, seen)
	case FunctionDef:
		return equalFunctionDefSeen(l, right, seen)
	case *ExternalFunctionDef:
		return equalExternalFunctionDefSeen(*l, right, seen)
	case ExternalFunctionDef:
		return equalExternalFunctionDefSeen(l, right, seen)
	case *StructDef:
		return equalStructDefSeen(*l, right, seen)
	case StructDef:
		return equalStructDefSeen(l, right, seen)
	case *Union:
		return equalUnionSeen(*l, right, seen)
	case Union:
		return equalUnionSeen(l, right, seen)
	default:
		return left.equal(right)
	}
}

func equalFunctionDefSeen(left FunctionDef, right Type, seen map[typeEqualKey]struct{}) bool {
	if r, ok := right.(*ExternalFunctionDef); ok {
		if len(left.Parameters) != len(r.Parameters) {
			return false
		}
		for i := range left.Parameters {
			if !equalTypesSeen(left.Parameters[i].Type, r.Parameters[i].Type, seen) {
				return false
			}
		}
		return true
	}
	r, ok := right.(*FunctionDef)
	if !ok || len(left.Parameters) != len(r.Parameters) {
		return false
	}
	for i := range left.Parameters {
		if !equalTypesSeen(left.Parameters[i].Type, r.Parameters[i].Type, seen) {
			return false
		}
	}
	return left.Mutates == r.Mutates && equalTypesSeen(left.ReturnType, r.ReturnType, seen)
}

func externTypeNamesMatch(left *ExternType, right *ExternType) bool {
	leftBinding, leftOK := parseCanonicalDirectGoBinding(left.ExternalBinding)
	rightBinding, rightOK := parseCanonicalDirectGoBinding(right.ExternalBinding)
	if leftOK || rightOK {
		return leftOK && rightOK && len(leftBinding.Symbols) == 1 && len(rightBinding.Symbols) == 1 && leftBinding.ImportPath == rightBinding.ImportPath && leftBinding.Symbols[0] == rightBinding.Symbols[0]
	}
	return left.Name_ == right.Name_
}

func equalExternalFunctionDefSeen(left ExternalFunctionDef, right Type, seen map[typeEqualKey]struct{}) bool {
	if r, ok := right.(*ExternalFunctionDef); ok {
		if len(left.Parameters) != len(r.Parameters) || len(left.ExternalBindings) != len(r.ExternalBindings) {
			return false
		}
		for i := range left.Parameters {
			if !equalTypesSeen(left.Parameters[i].Type, r.Parameters[i].Type, seen) {
				return false
			}
		}
		if !equalTypesSeen(left.ReturnType, r.ReturnType, seen) || left.ExternalBinding != r.ExternalBinding {
			return false
		}
		for key, value := range left.ExternalBindings {
			if r.ExternalBindings[key] != value {
				return false
			}
		}
		return true
	}
	if r, ok := right.(*FunctionDef); ok {
		if len(left.Parameters) != len(r.Parameters) {
			return false
		}
		for i := range left.Parameters {
			if !equalTypesSeen(left.Parameters[i].Type, r.Parameters[i].Type, seen) {
				return false
			}
		}
		return equalTypesSeen(left.ReturnType, r.ReturnType, seen)
	}
	return false
}

func equalStructDefSeen(left StructDef, right Type, seen map[typeEqualKey]struct{}) bool {
	r, ok := right.(*StructDef)
	if !ok || left.Name != r.Name || namedTypeOwnersDiffer(left.ModulePath, r.ModulePath) || len(left.Fields) != len(r.Fields) || len(left.TypeArgs) != len(r.TypeArgs) {
		return false
	}
	for i := range left.TypeArgs {
		if !equalTypesSeen(left.TypeArgs[i], r.TypeArgs[i], seen) {
			return false
		}
	}
	for name, fieldType := range left.Fields {
		otherFieldType, ok := r.Fields[name]
		if !ok || !equalTypesSeen(fieldType, otherFieldType, seen) {
			return false
		}
	}
	return true
}

func namedTypeOwnersDiffer(left string, right string) bool {
	return left != "" && right != "" && left != right
}

func equalUnionSeen(left Union, right Type, seen map[typeEqualKey]struct{}) bool {
	if r, ok := right.(*Union); ok {
		if namedTypeOwnersDiffer(left.ModulePath, r.ModulePath) || len(left.Types) != len(r.Types) {
			return false
		}
		for _, leftType := range left.Types {
			found := false
			for _, rightType := range r.Types {
				if equalTypesSeen(leftType, rightType, seen) {
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
	for _, t := range left.Types {
		if equalTypesSeen(t, right, seen) {
			return true
		}
	}
	return false
}

func typeEqualID(t Type) string {
	switch v := t.(type) {
	case *Trait:
		return fmt.Sprintf("Trait:%p", v)
	case Trait:
		return fmt.Sprintf("Trait:%s", v.Name)
	case *List:
		return fmt.Sprintf("List:%p", v)
	case *Map:
		return fmt.Sprintf("Map:%p", v)
	case Map:
		return fmt.Sprintf("Map:%p", &v)
	case *Maybe:
		return fmt.Sprintf("Maybe:%p", v)
	case *TypeVar:
		return fmt.Sprintf("TypeVar:%p", v)
	case *Result:
		return fmt.Sprintf("Result:%p", v)
	case *MutableRef:
		return fmt.Sprintf("MutableRef:%p", v)
	case *ExternType:
		return fmt.Sprintf("Extern:%p", v)
	case *FunctionDef:
		return fmt.Sprintf("Function:%p", v)
	case FunctionDef:
		return fmt.Sprintf("Function:%s", v.Name)
	case *ExternalFunctionDef:
		return fmt.Sprintf("ExternalFunction:%p", v)
	case *Enum:
		return fmt.Sprintf("Enum:%s:%s", v.ModulePath, v.Name)
	case Enum:
		return fmt.Sprintf("Enum:%s:%s", v.ModulePath, v.Name)
	case *StructDef:
		if v.Name != "" {
			return fmt.Sprintf("Struct:%s:%s", v.ModulePath, v.Name)
		}
		return fmt.Sprintf("Struct:%p", v)
	case StructDef:
		return fmt.Sprintf("Struct:%s:%s", v.ModulePath, v.Name)
	case *Union:
		if v.Name != "" {
			return fmt.Sprintf("Union:%s:%s", v.ModulePath, v.Name)
		}
		return fmt.Sprintf("Union:%p", v)
	case Union:
		return fmt.Sprintf("Union:%s:%s", v.ModulePath, v.Name)
	default:
		return fmt.Sprintf("%T:%s", t, t.String())
	}
}
