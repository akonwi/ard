package checker

import (
	"maps"
	"slices"

	"github.com/akonwi/ard/compiler/parse"
)

// Generic named functions as values (issue #593).
//
// A reference to a generic named function, such as `same` for
// `fn same(x: $T) $T`, has no single type on its own. Where the expected type
// is a function type, the reference is instantiated from it the way a call is
// instantiated from its arguments: the reference owns fresh call-local
// variables (ADR 0055), the expected parameter and return types constrain
// them, and the result is one specialized signature carrying its bindings.
// `same` where `fn(Int) Int` is expected is `same` with `$T = Int`. Where the
// expected type leaves a variable open, or there is no expected function type
// at all, the reference is rejected as an unresolved generic.

// genericFunctionValueExpectation carries the expected function type to the
// one reference node being checked against it. It is keyed by node so the
// expectation never reaches a nested expression.
type genericFunctionValueExpectation struct {
	expr     parse.Expression
	expected Type
	target   *FunctionDef
	context  *typeExpectation
	argument *Parameter
}

// checkGenericFunctionValueAs checks a reference to a generic named function
// against an expected function type. It reports false when expr is not such a
// reference, leaving it to the ordinary checkExprAs path.
func (c *Checker) checkGenericFunctionValueAs(expr parse.Expression, expectedType Type, expectation *typeExpectation, argumentParameter *Parameter) (Expression, bool) {
	target, ok := functionDefForCallableType(expectedType)
	if !ok || !c.isGenericFunctionReference(expr) {
		return nil, false
	}
	previous := c.functionValueExpectation
	c.functionValueExpectation = &genericFunctionValueExpectation{
		expr:     expr,
		expected: expectedType,
		target:   target,
		context:  expectation,
		argument: argumentParameter,
	}
	defer func() { c.functionValueExpectation = previous }()
	checked := c.checkExpr(expr)
	return c.finishCheckExprAs(expr, expectedType, expectation, argumentParameter, checked), true
}

// isGenericFunctionReference reports whether expr names a generic Ard
// function: a local or top-level function, an imported module function, or
// a static function. It only looks names up; checking the reference reports
// any errors.
func (c *Checker) isGenericFunctionReference(expr parse.Expression) bool {
	switch s := expr.(type) {
	case *parse.Identifier:
		if sym, ok := c.scope.get(s.Name); ok {
			return isUnspecializedGenericFunction(sym.Type)
		}
	case *parse.StaticProperty:
		prop, ok := s.Property.(*parse.Identifier)
		if !ok {
			return false
		}
		if id, ok := s.Target.(*parse.Identifier); ok {
			if ref, ok := c.localStaticFunction(id.Name, prop.Name); ok {
				return isUnspecializedGenericFunction(ref.symbol.Type)
			}
			if c.program.GoImports[id.Name] != nil {
				return false
			}
			if mod := c.resolveModule(id.Name); mod != nil {
				return isUnspecializedGenericFunction(mod.Get(prop.Name).Type)
			}
		}
		if ref, ok := c.lookupStaticFunction(s.Target, prop.Name); ok {
			return isUnspecializedGenericFunction(ref.symbol.Type)
		}
	}
	return false
}

func isUnspecializedGenericFunction(typ Type) bool {
	fn, ok := typ.(*FunctionDef)
	return ok && len(fn.CallGenericParams) > 0
}

// functionValueType returns the type a function reference node has as a
// value. A non-generic function is its declared type. A generic function is
// specialized from ownerBindings, the type arguments an alias owner applies to
// a static function's generics (issue #607), and from the expected function
// type supplied for this node. With neither, or when they leave a variable
// open, the reference is rejected and ok is false.
func (c *Checker) functionValueType(expr parse.Expression, typ Type, ownerBindings map[string]Type, location parse.Location) (Type, bool) {
	fn, isFn := typ.(*FunctionDef)
	if !isFn || len(fn.CallGenericParams) == 0 {
		return typ, true
	}
	expectation := c.functionValueExpectation
	if expectation != nil && expectation.expr != expr {
		expectation = nil
	}
	if expectation == nil && len(ownerBindings) == 0 {
		c.rejectUnspecializedGenericFunctionValue(fn, location)
		return nil, false
	}
	specialized := c.instantiateGenericFunctionValue(fn, expectation, ownerBindings, location)
	if specialized == nil {
		return nil, false
	}
	return specialized, true
}

// instantiateGenericFunctionValue specializes a generic function from the
// owner bindings and the expected function type, either of which may be
// absent. The expected parameter and return types are constraint sources;
// unresolved variables owned by an enclosing call provide no evidence (ADR
// 0055's left-to-right rule: an earlier argument or other context must fix
// them first). Conflicting evidence is reported as a type mismatch between
// the expected type and the specialization.
func (c *Checker) instantiateGenericFunctionValue(fn *FunctionDef, expectation *genericFunctionValueExpectation, ownerBindings map[string]Type, location parse.Location) *FunctionDef {
	if expectation != nil && !sameFunctionShape(fn, expectation.target) {
		c.reportFunctionValueMismatch(expectation, fn, location)
		return nil
	}

	genericParams := append([]string(nil), callGenericParamsForFunction(fn)...)
	fnCopy, genericScope, err := c.setupFunctionCallWithBindings(fn, genericParams, ownerBindings, genericParams, nil, nil)
	if err != nil {
		c.addGenericFunctionResolutionError(err, location)
		return nil
	}
	conflict := false
	if expectation != nil {
		target := expectation.target
		for i := range fnCopy.Parameters {
			if c.constrainFunctionValueType(fnCopy.Parameters[i].Type, target.Parameters[i].Type, genericScope) != nil {
				conflict = true
			}
		}
		// A Void expectation discards the return value
		// (coerceDiscardingFunction), so it is no evidence for the function's
		// own return type.
		if target.ReturnType != Void {
			if c.constrainFunctionValueType(fnCopy.ReturnType, target.ReturnType, genericScope) != nil {
				conflict = true
			}
		}
	}

	for _, name := range slices.Sorted(maps.Keys(*genericScope.genericContext)) {
		if unresolved := firstUnresolvedCallTypeVar((*genericScope.genericContext)[name]); unresolved != nil {
			if conflict {
				c.reportFunctionValueMismatch(expectation, fn, location)
			} else {
				c.rejectUnspecializedGenericFunctionValue(fn, location)
			}
			return nil
		}
	}

	bindings := genericScope.getGenericBindings()
	genericArgs := make([]Type, len(genericParams))
	for i, param := range genericParams {
		genericArgs[i] = bindings[param]
	}
	if param, actual, invalid := mutableGenericTraitArgument([]Type{fn}, genericParams, genericArgs); invalid {
		legacy := "Type argument " + actual.String() + " cannot be used for $" + param + " in " + fn.Name + ": direct mutable generic parameters cannot specialize to an Ard trait"
		c.addInvalidFunctionTypeArguments(fn.Name, len(genericParams), len(genericArgs), true, location, legacy)
		return nil
	}
	c.recordComparableConstraintCall(fn, bindings, location)

	// Like a call, the specialization carries its bindings; a conflict that
	// still resolved every variable surfaces as a mismatch against the
	// expected type when the caller checks compatibility. Unlike a call's
	// signature, the specialization is the value's type: a concrete function
	// type that a binding such as `let wrap = IntCell::wrap` keeps, so it
	// declares no generics of its own. AIR takes the parameter order from the
	// declaration.
	specialized := specializeFunctionSignature(fnCopy, bindings)
	specialized.GenericParams = nil
	specialized.CallGenericParams = nil
	return specialized
}

// constrainFunctionValueType binds the reference's call-local variables in
// pattern from one part of the expected function type. Enclosing declaration
// generics are rigid evidence; variables still open in an enclosing call are
// skipped rather than linked, so they never become this reference's binding.
func (c *Checker) constrainFunctionValueType(pattern, expected Type, genericScope *SymbolTable) error {
	if firstUnresolvedCallTypeVar(expected) == nil {
		return c.unifyTypes(pattern, expected, genericScope)
	}
	return c.inferBindingsFromExpectedReturn(pattern, expected, genericScope)
}

// sameFunctionShape reports whether two function types agree on arity and
// parameter metadata, which inference cannot change.
func sameFunctionShape(fn, target *FunctionDef) bool {
	if len(fn.Parameters) != len(target.Parameters) {
		return false
	}
	for i := range fn.Parameters {
		if fn.Parameters[i].Mutable != target.Parameters[i].Mutable || fn.Parameters[i].Variadic != target.Parameters[i].Variadic {
			return false
		}
	}
	return true
}

func (c *Checker) reportFunctionValueMismatch(expectation *genericFunctionValueExpectation, actual Type, location parse.Location) {
	switch {
	case expectation.argument != nil:
		c.addIncorrectArgumentType(typeMismatch(expectation.expected, actual), expectation.expected, actual, location, *expectation.argument, false)
	case expectation.context != nil:
		c.addDiagnostic(c.buildTypeMismatch(typeMismatchDiagnostic{
			Expected:    expectation.expected,
			Actual:      actual,
			ActualSpan:  c.sourceSpan(location),
			Expectation: expectation.context,
		}))
	default:
		c.addTypeMismatch(expectation.expected, actual, location)
	}
}

// specializeFunctionSignature substitutes completed bindings into a call-local
// function copy, producing the specialized signature that AIR instantiates.
func specializeFunctionSignature(fnCopy *FunctionDef, bindings map[string]Type) *FunctionDef {
	specialized := &FunctionDef{
		Name:                    fnCopy.Name,
		GenericParams:           append([]string(nil), fnCopy.GenericParams...),
		CallGenericParams:       append([]string(nil), fnCopy.CallGenericParams...),
		LocalNamed:              fnCopy.LocalNamed,
		DefaultVoidGeneric:      fnCopy.DefaultVoidGeneric,
		DeferCallCompleteness:   fnCopy.DeferCallCompleteness,
		Parameters:              make([]Parameter, len(fnCopy.Parameters)),
		ReturnType:              substituteType(fnCopy.ReturnType, bindings),
		ForeignResultShape:      fnCopy.ForeignResultShape,
		InferReturnTypeFromBody: fnCopy.InferReturnTypeFromBody,
		Mutates:                 fnCopy.Mutates,
		Private:                 fnCopy.Private,
		GenericBindings:         cloneTypeMap(bindings),
	}
	for i, param := range fnCopy.Parameters {
		specialized.Parameters[i] = Parameter{
			Name:       param.Name,
			Type:       substituteType(param.Type, bindings),
			Mutable:    param.Mutable,
			Loc:        param.Loc,
			declaredAt: param.declaredAt,
			Variadic:   param.Variadic,
		}
	}
	return specialized
}
