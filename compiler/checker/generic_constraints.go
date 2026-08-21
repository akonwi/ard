package checker

import (
	"fmt"
	gotypes "go/types"

	"github.com/akonwi/ard/parse"
)

type comparableConstraintCall struct {
	caller   *FunctionDef
	callee   *FunctionDef
	bindings map[string]Type
	span     SourceSpan
}

type genericTraitConstraintCheck struct {
	target       *StructDef
	contract     string
	contractKind string
	method       *FunctionDef
	span         SourceSpan
}

type comparableTypeVisit struct {
	id                string
	requireComparable bool
}

type comparableConstraintFailure struct {
	actual Type
	span   SourceSpan
}

type comparableBindingRequirement struct {
	generics   map[string]bool
	impossible bool
}

func comparableGenericNames(t Type, requireComparable bool) map[string]bool {
	result := map[string]bool{}
	if t == nil || !hasGenericsInType(t) {
		return result
	}
	collectComparableGenericNames(t, requireComparable, map[comparableTypeVisit]bool{}, result, true)
	return result
}

func collectComparableGenericNames(t Type, requireComparable bool, seen map[comparableTypeVisit]bool, result map[string]bool, analyzeForeignStructure bool) {
	t = deref(t)
	if t == nil {
		return
	}
	visit := comparableTypeVisit{id: comparableConstraintTypeID(t), requireComparable: requireComparable}
	if seen[visit] {
		return
	}
	seen[visit] = true

	switch typ := t.(type) {
	case *TypeVar:
		if typ.actual != nil {
			collectComparableGenericNames(typ.actual, requireComparable, seen, result, analyzeForeignStructure)
		} else if requireComparable && typ.name != "" && typ.name != "unknown" && typ.name != "Unreachable" {
			result[typ.name] = true
		}
	case *Map:
		collectComparableGenericNames(typ.Key(), true, seen, result, analyzeForeignStructure)
		collectComparableGenericNames(typ.Value(), false, seen, result, analyzeForeignStructure)
	case Map:
		collectComparableGenericNames(typ.Key(), true, seen, result, analyzeForeignStructure)
		collectComparableGenericNames(typ.Value(), false, seen, result, analyzeForeignStructure)
	case *StructDef:
		for _, fieldType := range structFields(typ) {
			collectComparableGenericNames(fieldType, requireComparable, seen, result, analyzeForeignStructure)
		}
	case StructDef:
		collectComparableGenericNames(&typ, requireComparable, seen, result, analyzeForeignStructure)
	case *FixedArray:
		collectComparableGenericNames(typ.Of(), requireComparable, seen, result, analyzeForeignStructure)
	case *List:
		collectComparableGenericNames(typ.Of(), false, seen, result, analyzeForeignStructure)
	case *Slice:
		collectComparableGenericNames(typ.Of(), false, seen, result, analyzeForeignStructure)
	case *Chan:
		collectComparableGenericNames(typ.Of(), false, seen, result, analyzeForeignStructure)
	case *Receiver:
		collectComparableGenericNames(typ.Of(), false, seen, result, analyzeForeignStructure)
	case *Sender:
		collectComparableGenericNames(typ.Of(), false, seen, result, analyzeForeignStructure)
	case *Maybe:
		collectComparableGenericNames(typ.Of(), false, seen, result, analyzeForeignStructure)
	case *Result:
		collectComparableGenericNames(typ.Val(), false, seen, result, analyzeForeignStructure)
		collectComparableGenericNames(typ.Err(), false, seen, result, analyzeForeignStructure)
	case *MutableRef:
		collectComparableGenericNames(typ.Of(), false, seen, result, analyzeForeignStructure)
	case *FunctionDef:
		for _, parameter := range typ.Parameters {
			collectComparableGenericNames(parameter.Type, false, seen, result, analyzeForeignStructure)
		}
		collectComparableGenericNames(typ.ReturnType, false, seen, result, analyzeForeignStructure)
	case FunctionDef:
		collectComparableGenericNames(&typ, requireComparable, seen, result, analyzeForeignStructure)
	case *ForeignType:
		if requireComparable && analyzeForeignStructure {
			requirement := comparableBindingRequirements(typ)
			if !requirement.impossible {
				for name := range requirement.generics {
					result[name] = true
				}
			}
			return
		}
		comparableArgs := typ.ComparableTypeArgs()
		for i, argument := range typ.TypeArgs {
			required := i < len(comparableArgs) && comparableArgs[i]
			collectComparableGenericNames(argument, required, seen, result, analyzeForeignStructure)
		}
	case *Union:
		for _, member := range typ.Types {
			collectComparableGenericNames(member, requireComparable, seen, result, analyzeForeignStructure)
		}
	case Union:
		collectComparableGenericNames(&typ, requireComparable, seen, result, analyzeForeignStructure)
	}
}

func receiverProvidedComparableGenericNames(t Type) map[string]bool {
	result := map[string]bool{}
	if t == nil || !hasGenericsInType(t) {
		return result
	}
	collectComparableGenericNames(t, false, map[comparableTypeVisit]bool{}, result, false)
	return result
}

func comparableBindingRequirements(t Type) comparableBindingRequirement {
	result := comparableBindingRequirement{generics: map[string]bool{}}
	collectComparableBindingRequirements(deref(t), map[string]bool{}, &result)
	return result
}

func collectComparableBindingRequirements(t Type, seen map[string]bool, result *comparableBindingRequirement) {
	t = deref(t)
	if t == nil || result.impossible {
		return
	}
	id := comparableConstraintTypeID(t)
	if seen[id] {
		return
	}
	seen[id] = true

	switch typ := t.(type) {
	case *TypeVar:
		if typ.actual != nil {
			collectComparableBindingRequirements(typ.actual, seen, result)
		} else if typ.name != "" && typ.name != "unknown" && typ.name != "Unreachable" {
			result.generics[typ.name] = true
		}
	case *StructDef:
		for _, fieldType := range structFields(typ) {
			collectComparableBindingRequirements(fieldType, seen, result)
		}
	case StructDef:
		collectComparableBindingRequirements(&typ, seen, result)
	case *FixedArray:
		collectComparableBindingRequirements(typ.Of(), seen, result)
	case *Result:
		collectComparableBindingRequirements(typ.Val(), seen, result)
		collectComparableBindingRequirements(typ.Err(), seen, result)
	case *MutableRef, *Maybe, *Chan, *Receiver, *Sender, *Trait, Trait, *anyType:
		// These lower to pointer, channel, or interface-backed values whose
		// comparability does not depend on an enclosed type parameter.
	case *ForeignType:
		collectForeignComparableBindingRequirements(typ, seen, result)
	case *Union:
		for _, member := range typ.Types {
			collectComparableBindingRequirements(member, seen, result)
		}
	case Union:
		for _, member := range typ.Types {
			collectComparableBindingRequirements(member, seen, result)
		}
	case *List, *Slice, *Map, Map, *FunctionDef, FunctionDef:
		result.impossible = true
	default:
		if !isValidMapKeyType(typ) {
			result.impossible = true
		}
	}
}

func collectForeignComparableBindingRequirements(foreign *ForeignType, seen map[string]bool, result *comparableBindingRequirement) {
	if foreign == nil || foreign.GoType == nil || result.impossible {
		return
	}
	goType := foreign.GoType
	if _, pointer := goType.(*gotypes.Pointer); pointer {
		return
	}
	named, ok := goType.(*gotypes.Named)
	if !ok || len(foreign.TypeArgs) == 0 {
		if !gotypes.Comparable(goType) {
			result.impossible = true
		}
		return
	}
	origin := named.Origin()
	params := origin.TypeParams()
	if params == nil || params.Len() != len(foreign.TypeArgs) {
		if !gotypes.Comparable(goType) {
			result.impossible = true
		}
		return
	}
	bindings := make(map[*gotypes.TypeParam]Type, params.Len())
	for i, argument := range foreign.TypeArgs {
		bindings[params.At(i)] = argument
	}
	collectGoComparableBindingRequirements(origin.Underlying(), bindings, map[gotypes.Type]bool{}, seen, result)
}

func collectGoComparableBindingRequirements(goType gotypes.Type, bindings map[*gotypes.TypeParam]Type, goSeen map[gotypes.Type]bool, seen map[string]bool, result *comparableBindingRequirement) {
	if goType == nil || result.impossible || goSeen[goType] {
		return
	}
	goSeen[goType] = true
	switch typ := goType.(type) {
	case *gotypes.TypeParam:
		if binding := bindings[typ]; binding != nil {
			collectComparableBindingRequirements(binding, seen, result)
			return
		}
		if constraint, ok := typ.Constraint().Underlying().(*gotypes.Interface); !ok || !constraint.Complete().IsComparable() {
			result.impossible = true
		}
	case *gotypes.Named:
		collectGoComparableBindingRequirements(typ.Underlying(), bindings, goSeen, seen, result)
	case *gotypes.Alias:
		collectGoComparableBindingRequirements(gotypes.Unalias(typ), bindings, goSeen, seen, result)
	case *gotypes.Array:
		collectGoComparableBindingRequirements(typ.Elem(), bindings, goSeen, seen, result)
	case *gotypes.Struct:
		for i := 0; i < typ.NumFields(); i++ {
			collectGoComparableBindingRequirements(typ.Field(i).Type(), bindings, goSeen, seen, result)
		}
	case *gotypes.Slice, *gotypes.Map, *gotypes.Signature:
		result.impossible = true
	default:
		if !gotypes.Comparable(goType) {
			result.impossible = true
		}
	}
}

func comparableConstraintTypeID(t Type) string {
	if structType, ok := t.(*StructDef); ok {
		definition := canonicalStructDefinition(structType)
		return fmt.Sprintf("Struct:%s:%s:%s", definition.ModulePath, definition.Name, structType.String())
	}
	return typeEqualID(t)
}

func (c *Checker) currentConstraintFunction() *FunctionDef {
	if len(c.constraintFunctionStack) == 0 {
		return nil
	}
	return c.constraintFunctionStack[len(c.constraintFunctionStack)-1]
}

func (c *Checker) pushConstraintFunction(fn *FunctionDef, location parse.Location) {
	c.constraintFunctionStack = append(c.constraintFunctionStack, fn)
	if fn == nil {
		return
	}
	span := c.sourceSpan(location)
	for _, parameter := range fn.Parameters {
		c.markFunctionComparableRequirements(fn, parameter.Type, false, span)
	}
	c.markFunctionComparableRequirements(fn, fn.ReturnType, false, span)
}

func (c *Checker) popConstraintFunction() {
	if len(c.constraintFunctionStack) > 0 {
		c.constraintFunctionStack = c.constraintFunctionStack[:len(c.constraintFunctionStack)-1]
	}
}

func (c *Checker) markCurrentComparableRequirement(t Type, location parse.Location) {
	c.markFunctionComparableRequirements(c.currentConstraintFunction(), t, true, c.sourceSpan(location))
}

func (c *Checker) markFunctionComparableRequirements(fn *FunctionDef, t Type, requireComparable bool, span SourceSpan) {
	if fn == nil {
		return
	}
	if fn.requiredComparable == nil {
		fn.requiredComparable = map[string]SourceSpan{}
	}
	for name := range comparableGenericNames(t, requireComparable) {
		if _, exists := fn.requiredComparable[name]; !exists {
			fn.requiredComparable[name] = span
		}
	}
}

func (c *Checker) recordComparableConstraintCall(callee *FunctionDef, bindings map[string]Type, location parse.Location) {
	caller := c.currentConstraintFunction()
	if caller == nil || callee == nil {
		return
	}
	c.comparableConstraintCalls = append(c.comparableConstraintCalls, comparableConstraintCall{
		caller: caller, callee: callee, bindings: cloneTypeMap(bindings), span: c.sourceSpan(location),
	})
}

func (c *Checker) recordComparableConstraintClosure(parent, closure *FunctionDef, location parse.Location) {
	if parent == nil || closure == nil {
		return
	}
	c.comparableConstraintCalls = append(c.comparableConstraintCalls, comparableConstraintCall{
		caller: parent, callee: closure, span: c.sourceSpan(location),
	})
}

func (c *Checker) finalizeComparableConstraints() {
	for changed := true; changed; {
		changed = false
		for _, edge := range c.comparableConstraintCalls {
			if edge.caller == nil || edge.callee == nil {
				continue
			}
			if edge.callee.invalidComparable != nil && edge.caller.invalidComparable == nil {
				edge.caller.invalidComparable = &comparableConstraintFailure{actual: edge.callee.invalidComparable.actual, span: edge.span}
				changed = true
			}
			for calleeParam := range edge.callee.requiredComparable {
				binding := edge.bindings[calleeParam]
				if binding == nil {
					if edge.caller.requiredComparable == nil {
						edge.caller.requiredComparable = map[string]SourceSpan{}
					}
					if _, exists := edge.caller.requiredComparable[calleeParam]; !exists {
						edge.caller.requiredComparable[calleeParam] = edge.span
						changed = true
					}
					continue
				}
				requirement := comparableBindingRequirements(binding)
				if requirement.impossible {
					if edge.caller.invalidComparable == nil {
						edge.caller.invalidComparable = &comparableConstraintFailure{actual: binding, span: edge.span}
						changed = true
					}
					continue
				}
				if edge.caller.requiredComparable == nil {
					edge.caller.requiredComparable = map[string]SourceSpan{}
				}
				for name := range requirement.generics {
					if _, exists := edge.caller.requiredComparable[name]; !exists {
						edge.caller.requiredComparable[name] = edge.span
						changed = true
					}
				}
			}
		}
	}

	for _, check := range c.genericTraitConstraintChecks {
		if check.target == nil || check.contract == "" || check.method == nil {
			continue
		}
		if failure := check.method.invalidComparable; failure != nil {
			c.addDiagnostic(genericTraitReceiverConstraintDiagnostic{
				Contract: check.contract, ContractKind: check.contractKind, Target: check.target.Name, Actual: failure.actual, Span: failure.span,
			}.build())
			continue
		}
		available := map[string]bool{}
		for _, fieldType := range structFields(check.target) {
			for name := range receiverProvidedComparableGenericNames(fieldType) {
				available[name] = true
			}
		}
		for _, generic := range genericParamsForType(check.target) {
			if !check.method.requiresComparable(generic) || available[generic] {
				continue
			}
			requirementSpan := check.method.requiredComparable[generic]
			if requirementSpan.FilePath == "" {
				requirementSpan = check.span
			}
			c.addDiagnostic(genericTraitReceiverConstraintDiagnostic{
				Contract: check.contract, ContractKind: check.contractKind, Target: check.target.Name, Generic: generic, Span: requirementSpan,
			}.build())
		}
	}
}
