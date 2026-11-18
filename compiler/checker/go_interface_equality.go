package checker

import (
	"fmt"
	"go/types"
	"maps"
	"slices"
)

// goInterfaceEqualityHint explains a rejected `==` or `!=` that involves a Go
// interface value and points at the match type pattern alternative
// (ADR 0085, ADR 0042). It returns "" when neither operand is a Go interface.
//
// When the other operand is a concrete Go value type that can be the dynamic
// type of the interface, that Ard can itself compare, and that this module can
// name in a type pattern, the hint spells out the exact match. Otherwise it
// describes the pattern in general terms.
func (c *Checker) goInterfaceEqualityHint(operator string, left, right Type) string {
	iface, other := goInterfaceOperand(deref(left)), deref(right)
	if iface == nil {
		iface, other = goInterfaceOperand(deref(right)), deref(left)
	}
	if iface == nil {
		return ""
	}
	if concrete, ok := other.(*ForeignType); ok && matchableComparableDynamicType(iface, concrete) {
		if pattern, ok := c.foreignTypePatternName(concrete); ok {
			return exactGoInterfaceEqualityHint(operator, iface, pattern)
		}
	}
	return fmt.Sprintf(
		"Go interface values do not support `%s`. To test the value inside, match on its dynamic type with a type pattern, like `match value { pkg::Type(v) => ..., _ => ... }`",
		operator,
	)
}

func exactGoInterfaceEqualityHint(operator string, iface *ForeignType, pattern string) string {
	fallback := "false"
	if operator == "!=" {
		fallback = "true"
	}
	return fmt.Sprintf(
		"`%s` is a Go interface, which does not support `%s`. To test for a `%s`, match on the dynamic type: `match value { %s(v) => v %s other, _ => %s }`",
		iface, operator, pattern, pattern, operator, fallback,
	)
}

// foreignTypePatternName returns how this module spells concrete in a type
// pattern: `alias::Key`, where alias is one of the module's Go imports and Key
// is an entry of that package's Types that denotes the same Go type. This is
// the lookup checkForeignTypePattern performs, so a spelling is only offered
// when the pattern would resolve. Key is usually the type's own name, but it
// may be an exported Go alias, including an alias of an unexported type or a
// re-export from another package. Types with no such entry (an unexported type
// without an alias, or a type from a package the module does not import) have
// no spelling.
func (c *Checker) foreignTypePatternName(concrete *ForeignType) (string, bool) {
	if c.program == nil || concrete.GoType == nil {
		return "", false
	}
	aliases := slices.Sorted(maps.Keys(c.program.GoImports))
	denotes := func(pkg *GoPackage, key string) bool {
		candidate, ok := pkg.Types[key].(*ForeignType)
		return ok && !candidate.Interface && !candidate.Pointer && candidate.GoType != nil && types.Identical(candidate.GoType, concrete.GoType)
	}
	// Prefer the type's own name, then fall back to any alias of it.
	if named, ok := types.Unalias(concrete.GoType).(*types.Named); ok {
		name := named.Obj().Name()
		for _, alias := range aliases {
			if pkg := c.program.GoImports[alias]; pkg != nil && denotes(pkg, name) {
				return alias + "::" + name, true
			}
		}
	}
	for _, alias := range aliases {
		pkg := c.program.GoImports[alias]
		if pkg == nil {
			continue
		}
		for _, key := range slices.Sorted(maps.Keys(pkg.Types)) {
			if denotes(pkg, key) {
				return alias + "::" + key, true
			}
		}
	}
	return "", false
}

func goInterfaceOperand(t Type) *ForeignType {
	if foreign, ok := t.(*ForeignType); ok && foreign.Interface && !foreign.EmptyInterface() {
		return foreign
	}
	return nil
}

// matchableComparableDynamicType reports whether concrete can be the dynamic
// type of iface in a type pattern and its narrowed binding supports Ard `==`.
// Pointer patterns are not supported (ADR 0042), and generic instantiations
// are left to the general hint. Whether the module can spell the pattern is
// checked separately by foreignTypePatternName.
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
