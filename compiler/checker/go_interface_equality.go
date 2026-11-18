package checker

import (
	"fmt"
	"go/types"
)

// goInterfaceEqualityHint explains a rejected `==` or `!=` that involves a Go
// interface value and points at the match type pattern alternative
// (ADR 0085, ADR 0042). It returns "" when neither operand is a Go interface.
//
// When the other operand is a concrete Go value type that can be the dynamic
// type of the interface and that Ard can itself compare, the hint spells out
// the exact match. Otherwise it describes the pattern in general terms.
func goInterfaceEqualityHint(operator string, left, right Type) string {
	iface, other := goInterfaceOperand(deref(left)), deref(right)
	if iface == nil {
		iface, other = goInterfaceOperand(deref(right)), deref(left)
	}
	if iface == nil {
		return ""
	}
	if concrete, ok := other.(*ForeignType); ok && matchableComparableDynamicType(iface, concrete) {
		fallback := "false"
		if operator == "!=" {
			fallback = "true"
		}
		return fmt.Sprintf(
			"`%s` is a Go interface, which does not support `%s`. To test for a `%s`, match on the dynamic type: `match value { %s(v) => v %s other, _ => %s }`",
			iface, operator, concrete, concrete, operator, fallback,
		)
	}
	return fmt.Sprintf(
		"Go interface values do not support `%s`. To test the value inside, match on its dynamic type with a type pattern, like `match value { pkg::Type(v) => ..., _ => ... }`",
		operator,
	)
}

func goInterfaceOperand(t Type) *ForeignType {
	if foreign, ok := t.(*ForeignType); ok && foreign.Interface && !foreign.EmptyInterface() {
		return foreign
	}
	return nil
}

// matchableComparableDynamicType reports whether concrete can be written as a
// type pattern on a subject of type iface and its narrowed binding supports
// Ard `==`. Pointer patterns are not supported (ADR 0042), and generic
// instantiations are left to the general hint.
func matchableComparableDynamicType(iface, concrete *ForeignType) bool {
	if concrete.Interface || concrete.Pointer || concrete.GoType == nil || iface.GoType == nil {
		return false
	}
	if len(iface.TypeArgs) > 0 || len(concrete.TypeArgs) > 0 {
		return false
	}
	goInterface, ok := iface.GoType.Underlying().(*types.Interface)
	if !ok || !types.Implements(concrete.GoType, goInterface) {
		return false
	}
	return isComparableValueType(concrete)
}
