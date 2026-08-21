package checker

import "fmt"

type typeEqualKey struct {
	left  string
	right string
}

type typeEqualContext struct {
	seen                   map[typeEqualKey]struct{}
	allowUnboundWildcard   bool
	allowInferenceWildcard bool
}

func equalTypes(left Type, right Type) bool {
	return equalTypesWithMode(left, right, true, true)
}

// validationEqualTypes compares representation identity during compatibility
// checking. Declaration generics are not wildcards here, while call-local and
// provisional variables remain matchable until inference binds or rejects them.
func validationEqualTypes(left Type, right Type) bool {
	return equalTypesWithMode(left, right, false, true)
}

// strictEqualTypes compares identity without inference wildcards. It is used
// in contexts such as equality where no later binding or conversion occurs.
func strictEqualTypes(left Type, right Type) bool {
	return equalTypesWithMode(left, right, false, false)
}

func equalTypesWithMode(left Type, right Type, allowUnboundWildcard, allowInferenceWildcard bool) bool {
	return equalTypesSeen(left, right, &typeEqualContext{
		seen:                   map[typeEqualKey]struct{}{},
		allowUnboundWildcard:   allowUnboundWildcard,
		allowInferenceWildcard: allowInferenceWildcard,
	})
}

func inferenceWildcardTypeVar(typeVar *TypeVar) bool {
	return typeVar != nil && (typeVar.owner != 0 || typeVar.provisional || typeVar.name == "Unreachable")
}

func equalTypesSeen(left Type, right Type, context *typeEqualContext) bool {
	if left == nil || right == nil {
		return left == right
	}
	key := typeEqualKey{left: typeEqualID(left), right: typeEqualID(right)}
	if _, ok := context.seen[key]; ok {
		return true
	}
	context.seen[key] = struct{}{}

	if r, ok := right.(*TypeVar); ok {
		if l, leftIsTypeVar := left.(*TypeVar); leftIsTypeVar {
			if l == r {
				return true
			}
			if l.actual != nil {
				return equalTypesSeen(l.actual, right, context)
			}
			if r.actual != nil {
				return equalTypesSeen(left, r.actual, context)
			}
			if context.allowUnboundWildcard || (context.allowInferenceWildcard && (inferenceWildcardTypeVar(l) || inferenceWildcardTypeVar(r))) {
				return true
			}
			// Declaration generics are independently resolved nodes today, so
			// strict identity uses their scoped name. Call-local inference
			// variables also carry an owner to prevent cross-call matches.
			return l.name == r.name && l.owner == r.owner
		}
		if r.actual == nil {
			return context.allowUnboundWildcard || (context.allowInferenceWildcard && inferenceWildcardTypeVar(r))
		}
		return equalTypesSeen(left, r.actual, context)
	}

	switch l := left.(type) {
	case *Trait:
		r, ok := right.(*Trait)
		if !ok || l.Name != r.Name || l.ModulePath != r.ModulePath || len(l.methods) != len(r.methods) {
			return false
		}
		for i := range l.methods {
			if !equalTypesSeen(&l.methods[i], &r.methods[i], context) {
				return false
			}
		}
		return true
	case Trait:
		return equalTypesSeen(&l, right, context)
	case *List:
		if r, ok := right.(*List); ok {
			return equalTypesSeen(l.of, r.of, context)
		}
		if r, ok := right.(*TypeVar); ok {
			return (r.actual == nil && context.allowUnboundWildcard) || equalTypesSeen(l, r.actual, context)
		}
		if r, ok := right.(*Union); ok {
			return equalTypesSeen(r, l, context)
		}
		return false
	case *Slice:
		if r, ok := right.(*Slice); ok {
			return equalTypesSeen(l.of, r.of, context)
		}
		if r, ok := right.(*TypeVar); ok {
			return (r.actual == nil && context.allowUnboundWildcard) || equalTypesSeen(l, r.actual, context)
		}
		if r, ok := right.(*Union); ok {
			return equalTypesSeen(r, l, context)
		}
		return false
	case *FixedArray:
		if r, ok := right.(*FixedArray); ok {
			return l.length == r.length && equalTypesSeen(l.of, r.of, context)
		}
		if r, ok := right.(*TypeVar); ok {
			return (r.actual == nil && context.allowUnboundWildcard) || equalTypesSeen(l, r.actual, context)
		}
		if r, ok := right.(*Union); ok {
			return equalTypesSeen(r, l, context)
		}
		return false
	case *Chan:
		if r, ok := right.(*Chan); ok {
			return equalTypesSeen(l.of, r.of, context)
		}
		if r, ok := right.(*TypeVar); ok {
			return (r.actual == nil && context.allowUnboundWildcard) || equalTypesSeen(l, r.actual, context)
		}
		return false
	case *Receiver:
		if r, ok := right.(*Receiver); ok {
			return equalTypesSeen(l.of, r.of, context)
		}
		if r, ok := right.(*TypeVar); ok {
			return (r.actual == nil && context.allowUnboundWildcard) || equalTypesSeen(l, r.actual, context)
		}
		return false
	case *Sender:
		if r, ok := right.(*Sender); ok {
			return equalTypesSeen(l.of, r.of, context)
		}
		if r, ok := right.(*TypeVar); ok {
			return (r.actual == nil && context.allowUnboundWildcard) || equalTypesSeen(l, r.actual, context)
		}
		return false
	case *Map:
		if r, ok := right.(*Map); ok {
			return equalTypesSeen(l.key, r.key, context) && equalTypesSeen(l.value, r.value, context)
		}
		if r, ok := right.(Map); ok {
			return equalTypesSeen(l.key, r.key, context) && equalTypesSeen(l.value, r.value, context)
		}
		if r, ok := right.(*TypeVar); ok {
			return (r.actual == nil && context.allowUnboundWildcard) || equalTypesSeen(l, r.actual, context)
		}
		if r, ok := right.(*Union); ok {
			return equalTypesSeen(r, l, context)
		}
		return false
	case Map:
		if r, ok := right.(*Map); ok {
			return equalTypesSeen(l.key, r.key, context) && equalTypesSeen(l.value, r.value, context)
		}
		if r, ok := right.(Map); ok {
			return equalTypesSeen(l.key, r.key, context) && equalTypesSeen(l.value, r.value, context)
		}
		if r, ok := right.(*TypeVar); ok {
			return (r.actual == nil && context.allowUnboundWildcard) || equalTypesSeen(l, r.actual, context)
		}
		if r, ok := right.(*Union); ok {
			return equalTypesSeen(r, l, context)
		}
		return false
	case *Maybe:
		r, ok := right.(*Maybe)
		return ok && equalTypesSeen(l.of, r.of, context)
	case *TypeVar:
		if l == right {
			return true
		}
		if l.actual == nil {
			return context.allowUnboundWildcard || (context.allowInferenceWildcard && inferenceWildcardTypeVar(l))
		}
		return equalTypesSeen(l.actual, right, context)
	case *Result:
		if r, ok := right.(*Result); ok {
			return equalTypesSeen(l.val, r.val, context) && equalTypesSeen(l.err, r.err, context)
		}
		if r, ok := right.(*TypeVar); ok {
			return (r.actual == nil && context.allowUnboundWildcard) || equalTypesSeen(l, r.actual, context)
		}
		return false
	case *MutableRef:
		if r, ok := right.(*MutableRef); ok {
			return equalTypesSeen(l.of, r.of, context)
		}
		if r, ok := right.(*TypeVar); ok {
			return (r.actual == nil && context.allowUnboundWildcard) || equalTypesSeen(l, r.actual, context)
		}
		return false
	case *ForeignType:
		r, ok := right.(*ForeignType)
		if !ok || l.Target != r.Target || l.Namespace != r.Namespace || l.Name != r.Name || l.Pointer != r.Pointer || len(l.TypeArgs) != len(r.TypeArgs) {
			return false
		}
		for i := range l.TypeArgs {
			if !equalTypesSeen(l.TypeArgs[i], r.TypeArgs[i], context) {
				return false
			}
		}
		return true
	case *FunctionDef:
		return equalFunctionDefSeen(*l, right, context)
	case FunctionDef:
		return equalFunctionDefSeen(l, right, context)
	case *StructDef:
		return equalStructDefSeen(*l, right, context)
	case StructDef:
		return equalStructDefSeen(l, right, context)
	case *Union:
		return equalUnionSeen(*l, right, context)
	case Union:
		return equalUnionSeen(l, right, context)
	default:
		return left.equal(right)
	}
}

func equalFunctionDefSeen(left FunctionDef, right Type, context *typeEqualContext) bool {
	r, ok := right.(*FunctionDef)
	if !ok || len(left.Parameters) != len(r.Parameters) {
		return false
	}
	for i := range left.Parameters {
		lMut, lType := normalizedParamMutability(left.Parameters[i])
		rMut, rType := normalizedParamMutability(r.Parameters[i])
		if lMut != rMut || left.Parameters[i].Variadic != r.Parameters[i].Variadic || !equalTypesSeen(lType, rType, context) {
			return false
		}
	}
	return left.Mutates == r.Mutates && equalTypesSeen(left.ReturnType, r.ReturnType, context)
}

// normalizedParamMutability reconciles the two ways a `mut T` parameter can be
// represented: as a `MutableRef` baked into the parameter type (the `name: mut T`
// and closure form) or as the `Mutable` flag with a plain type (the `fn(mut T)`
// function-type form). It returns a canonical (isMutable, underlyingType) pair.
func normalizedParamMutability(p Parameter) (bool, Type) {
	if mr, ok := p.Type.(*MutableRef); ok {
		return true, mr.Of()
	}
	// A pointer-shaped foreign Go type is its own mutability marker (ADR
	// 0040): `mut` adds no extra indirection, and imported Go signatures
	// carry no Mutable flag. Canonicalize so `mut pkg::T` annotations and
	// imported `*pkg.T` parameters compare equal.
	if foreign, ok := p.Type.(*ForeignType); ok && foreign.Pointer {
		return true, foreign
	}
	return p.Mutable, p.Type
}

func equalStructDefSeen(left StructDef, right Type, context *typeEqualContext) bool {
	r, ok := right.(*StructDef)
	if !ok {
		return false
	}
	leftDef := canonicalStructDefinition(&left)
	rightDef := canonicalStructDefinition(r)
	if leftDef.Name != rightDef.Name || leftDef.ModulePath != rightDef.ModulePath || len(left.TypeArgs) != len(r.TypeArgs) {
		return false
	}
	for i := range left.TypeArgs {
		if !equalTypesSeen(left.TypeArgs[i], r.TypeArgs[i], context) {
			return false
		}
	}
	return true
}

func namedTypeOwnersDiffer(left string, right string) bool {
	return left != "" && right != "" && left != right
}

func equalUnionSeen(left Union, right Type, context *typeEqualContext) bool {
	if r, ok := right.(*Union); ok {
		if namedTypeOwnersDiffer(left.ModulePath, r.ModulePath) || len(left.Types) != len(r.Types) {
			return false
		}
		for _, leftType := range left.Types {
			found := false
			for _, rightType := range r.Types {
				if equalTypesSeen(leftType, rightType, context) {
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
		if equalTypesSeen(t, right, context) {
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
		return fmt.Sprintf("Trait:%s:%s", v.ModulePath, v.Name)
	case *List:
		return fmt.Sprintf("List:%p", v)
	case *Slice:
		return fmt.Sprintf("Slice:%p", v)
	case *FixedArray:
		return fmt.Sprintf("FixedArray:%p", v)
	case *Chan:
		return fmt.Sprintf("Chan:%p", v)
	case *Receiver:
		return fmt.Sprintf("Receiver:%p", v)
	case *Sender:
		return fmt.Sprintf("Sender:%p", v)
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
	case *ForeignType:
		return fmt.Sprintf("Foreign:%p", v)
	case *FunctionDef:
		return fmt.Sprintf("Function:%p", v)
	case FunctionDef:
		return fmt.Sprintf("Function:%s", v.Name)
	case *Enum:
		return fmt.Sprintf("Enum:%s:%s", v.ModulePath, v.Name)
	case Enum:
		return fmt.Sprintf("Enum:%s:%s", v.ModulePath, v.Name)
	case *StructDef:
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
