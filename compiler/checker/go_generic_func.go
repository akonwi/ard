package checker

import (
	"fmt"
	"go/types"

	"github.com/akonwi/ard/parse"
)

// goTypeParamBindings maps a generic Go function's type parameters to the Ard
// types the call instantiates them with.
type goTypeParamBindings map[*types.TypeParam]Type

// instantiateGoFunctionCall resolves a call to a generic Go function. Type
// arguments come from the call site when written explicitly, or are inferred
// by unifying supplied arguments against the generic parameter types. The
// instantiated signature then maps through the ordinary boundary rules.
func (c *Checker) instantiateGoFunctionCall(modName, name string, goFn *types.Func, s *parse.StaticFunction) (*FunctionDef, []Type, bool) {
	sig := goFn.Type().(*types.Signature)
	tparams := sig.TypeParams()
	args := make([]Type, tparams.Len())

	var inferenceSpans []SourceSpan
	if len(s.Function.TypeArgs) > 0 {
		if len(s.Function.TypeArgs) != tparams.Len() {
			legacy := fmt.Sprintf("Go function %s::%s expects %d type argument(s), got %d", modName, name, tparams.Len(), len(s.Function.TypeArgs))
			span := c.sourceSpan(s.GetLocation())
			if len(s.Function.TypeArgs) > tparams.Len() {
				span = c.sourceSpan(declaredTypeLocation(s.Function.TypeArgs[tparams.Len()], s.GetLocation()))
			}
			c.addDiagnostic(invalidGoFunctionTypeArgumentsDiagnostic{
				Name: modName + "::" + name, Expected: tparams.Len(), Actual: len(s.Function.TypeArgs), Span: span, LegacyMessage: legacy,
			}.build())
			return nil, nil, false
		}
		for i, typeArg := range s.Function.TypeArgs {
			arg := c.resolveType(typeArg)
			if arg == nil {
				return nil, nil, false
			}
			args[i] = arg
		}
	} else {
		inferred := make([]Type, tparams.Len())
		inferenceSpans = make([]SourceSpan, tparams.Len())
		for i := 0; i < sig.Params().Len() && i < len(s.Function.Args); i++ {
			goParam := sig.Params().At(i).Type()
			if sig.Variadic() && i == sig.Params().Len()-1 {
				if slice, ok := goParam.(*types.Slice); ok {
					goParam = slice.Elem()
				}
			}
			if !goTypeMentionsTypeParam(goParam, tparams) {
				continue
			}
			value := c.checkExprForInference(s.Function.Args[i].Value)
			if value == nil {
				continue
			}
			currentSpan := c.sourceSpan(s.Function.Args[i].GetLocation())
			ok, conflictParam, previous, current, previousSpan := inferGoFuncTypeArgs(goParam, value.Type(), tparams, inferred, inferenceSpans, currentSpan)
			if !ok {
				legacy := fmt.Sprintf("Conflicting inferred type arguments for %s: %s and %s", conflictParam.Obj().Name(), previous, value.Type())
				c.addDiagnostic(conflictingGoTypeInferenceDiagnostic{
					Parameter: conflictParam.Obj().Name(), PreviousType: previous, CurrentType: current, CurrentSpan: currentSpan, PreviousSpan: sourceSpanIfPresent(previousSpan), LegacyMessage: legacy,
				}.build())
				return nil, nil, false
			}
		}
		for i := 0; i < tparams.Len(); i++ {
			if inferred[i] == nil {
				legacy := fmt.Sprintf("Could not infer type argument %s for Go function %s::%s", tparams.At(i).Obj().Name(), modName, name)
				c.addDiagnostic(goTypeInferenceFailureDiagnostic{
					Parameter: tparams.At(i).Obj().Name(), EntityKind: "function", EntityName: modName + "::" + name, Span: c.sourceSpan(s.GetLocation()), LegacyMessage: legacy,
				}.build())
				return nil, nil, false
			}
			args[i] = inferred[i]
		}
	}

	bindings := goTypeParamBindings{}
	for i := 0; i < tparams.Len(); i++ {
		tparam := tparams.At(i)
		constraint, ok := tparam.Constraint().Underlying().(*types.Interface)
		if ok && !constraint.Empty() {
			span := c.sourceSpan(s.GetLocation())
			if len(s.Function.TypeArgs) > i {
				span = c.sourceSpan(declaredTypeLocation(s.Function.TypeArgs[i], s.GetLocation()))
			} else if len(inferenceSpans) > i && inferenceSpans[i].FilePath != "" {
				span = inferenceSpans[i]
			}
			goArg, representable := checkerTypeToGoType(args[i])
			if !representable {
				legacy := fmt.Sprintf("Type argument %s cannot be validated against Go constraint %s", args[i], tparam.Constraint())
				c.addDiagnostic(goConstraintDiagnostic{Argument: args[i], Constraint: tparam.Constraint().String(), Span: span, LegacyMessage: legacy, Unvalidated: true}.build())
				return nil, nil, false
			}
			if !types.Satisfies(goArg, constraint) {
				legacy := fmt.Sprintf("Type argument %s does not satisfy Go constraint %s", args[i], tparam.Constraint())
				c.addDiagnostic(goConstraintDiagnostic{Argument: args[i], Constraint: tparam.Constraint().String(), Span: span, LegacyMessage: legacy}.build())
				return nil, nil, false
			}
		}
		bindings[tparam] = args[i]
	}

	fnDef, reason := functionDefFromGoSignatureBound(name, sig, bindings)
	if reason != "" {
		qualified := modName + "::" + name
		legacy := fmt.Sprintf("Unsupported Go function %s: %s", qualified, reason)
		c.addDiagnostic(unsupportedGoEntityDiagnostic{Kind: "function", Name: qualified, Reason: reason, Span: c.sourceSpan(s.GetLocation()), LegacyMessage: legacy}.build())
		return nil, nil, false
	}
	return fnDef, args, goSignatureHasPointerResult(sig, fnDef.ReturnType)
}

// goSignatureHasPointerResult reports whether the instantiated Go call
// actually returns a raw Go pointer that Ard represents as `mut T`. It is
// derived from the Go signature, not the Ard result type, so value-returning
// instantiations (e.g. Identity[T] echoing a dereferenced mut argument) are
// not misclassified as references.
func goSignatureHasPointerResult(sig *types.Signature, ret Type) bool {
	if _, ok := ret.(*MutableRef); !ok {
		return false
	}
	if sig.Results().Len() != 1 {
		return false
	}
	_, isPointer := sig.Results().At(0).Type().(*types.Pointer)
	return isPointer
}

// functionDefFromGoSignatureBound maps a generic Go signature to an Ard
// FunctionDef with every type parameter replaced by its bound Ard type. It
// mirrors functionDefFromGoSignature, including the variadic-as-one-argument
// rule and the (T, error) / (T, bool) result adaptations.
func functionDefFromGoSignatureBound(name string, sig *types.Signature, bindings goTypeParamBindings) (*FunctionDef, string) {
	params := make([]Parameter, 0, sig.Params().Len())
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		goType := param.Type()
		mutable := false
		variadic := false
		if sig.Variadic() && i == sig.Params().Len()-1 {
			slice, ok := goType.(*types.Slice)
			if !ok {
				return nil, fmt.Sprintf("variadic parameter %d is not a slice", i+1)
			}
			goType = slice.Elem()
			variadic = true
		} else if _, ok := goType.Underlying().(*types.Slice); ok {
			mutable = true
		} else if _, ok := goType.Underlying().(*types.Map); ok {
			mutable = true
		}
		ardType, reason := boundTypeFromGo(goType, sig.TypeParams(), bindings)
		if reason != "" {
			return nil, fmt.Sprintf("parameter %d has unsupported type %s: %s", i+1, goType.String(), reason)
		}
		paramName := param.Name()
		if paramName == "" {
			paramName = fmt.Sprintf("arg%d", i+1)
		}
		params = append(params, Parameter{Name: paramName, Type: ardType, Mutable: mutable, Variadic: variadic})
	}

	ret, reason := boundReturnTypeFromGo(sig.Results(), sig.TypeParams(), bindings)
	if reason != "" {
		return nil, reason
	}
	return &FunctionDef{Name: name, Parameters: params, ReturnType: ret}, ""
}

func boundReturnTypeFromGo(results *types.Tuple, tparams *types.TypeParamList, bindings goTypeParamBindings) (Type, string) {
	switch results.Len() {
	case 0:
		return Void, ""
	case 1:
		if isGoError(results.At(0).Type()) {
			return MakeResult(Void, Str), ""
		}
		return boundTypeFromGo(results.At(0).Type(), tparams, bindings)
	case 2:
		if isGoError(results.At(1).Type()) {
			val, reason := boundTypeFromGo(results.At(0).Type(), tparams, bindings)
			if reason != "" {
				return nil, fmt.Sprintf("result 1 has unsupported type %s: %s", results.At(0).Type().String(), reason)
			}
			if _, ok := val.(*MutableRef); ok {
				return nil, "mut results inside Result are not supported yet"
			}
			return MakeResult(val, Str), ""
		}
		if isGoBool(results.At(1).Type()) {
			val, reason := boundTypeFromGo(results.At(0).Type(), tparams, bindings)
			if reason != "" {
				return nil, fmt.Sprintf("result 1 has unsupported type %s: %s", results.At(0).Type().String(), reason)
			}
			if _, ok := val.(*MutableRef); ok {
				return nil, "mut results inside Maybe are not supported yet"
			}
			return MakeMaybe(val), ""
		}
	}
	return nil, fmt.Sprintf("unsupported result shape %s", results.String())
}

// boundTypeFromGo maps a Go type that may mention type parameters to an Ard
// type, substituting bound Ard types for the parameters. Types that do not
// mention any type parameter map through the ordinary rules.
func boundTypeFromGo(t types.Type, tparams *types.TypeParamList, bindings goTypeParamBindings) (Type, string) {
	if !goTypeMentionsTypeParam(t, tparams) {
		if _, ok := t.(*types.TypeParam); !ok {
			return typeFromGo(t)
		}
	}
	switch typ := t.(type) {
	case *types.TypeParam:
		if bound, ok := bindings[typ]; ok {
			return bound, ""
		}
		return nil, fmt.Sprintf("type parameter %s requires instantiation", typ.Obj().Name())
	case *types.Pointer:
		if tp, ok := typ.Elem().(*types.TypeParam); ok {
			bound, ok := bindings[tp]
			if !ok {
				return nil, fmt.Sprintf("type parameter %s requires instantiation", tp.Obj().Name())
			}
			mutable, reason := mutableBoundType(bound)
			if reason != "" {
				return nil, reason
			}
			return mutable, ""
		}
		return nil, fmt.Sprintf("generic pointer type %s is not supported yet", typ.String())
	case *types.Slice:
		elem, reason := boundTypeFromGo(typ.Elem(), tparams, bindings)
		if reason != "" {
			return nil, "slice element " + reason
		}
		return MakeList(elem), ""
	case *types.Map:
		key, reason := boundTypeFromGo(typ.Key(), tparams, bindings)
		if reason != "" {
			return nil, "map key " + reason
		}
		value, reason := boundTypeFromGo(typ.Elem(), tparams, bindings)
		if reason != "" {
			return nil, "map value " + reason
		}
		return MakeMap(key, value), ""
	case *types.Chan:
		elem, reason := boundTypeFromGo(typ.Elem(), tparams, bindings)
		if reason != "" {
			return nil, "channel element " + reason
		}
		switch typ.Dir() {
		case types.SendRecv:
			return MakeChan(elem), ""
		case types.RecvOnly:
			return MakeReceiver(elem), ""
		case types.SendOnly:
			return MakeSender(elem), ""
		default:
			return nil, "unsupported channel direction"
		}
	case *types.Signature:
		params := make([]Parameter, 0, typ.Params().Len())
		for i := 0; i < typ.Params().Len(); i++ {
			param, reason := boundTypeFromGo(typ.Params().At(i).Type(), tparams, bindings)
			if reason != "" {
				return nil, fmt.Sprintf("callback parameter %d %s", i+1, reason)
			}
			name := typ.Params().At(i).Name()
			if name == "" {
				name = fmt.Sprintf("arg%d", i+1)
			}
			params = append(params, Parameter{Name: name, Type: param})
		}
		var ret Type = Void
		if typ.Results().Len() == 1 {
			result, reason := boundTypeFromGo(typ.Results().At(0).Type(), tparams, bindings)
			if reason != "" {
				return nil, "callback result " + reason
			}
			ret = result
		} else if typ.Results().Len() > 1 {
			return nil, fmt.Sprintf("callback multi-result shape %s is not supported yet", typ.Results().String())
		}
		return &FunctionDef{Name: "<function>", Parameters: params, ReturnType: ret}, ""
	}
	return nil, fmt.Sprintf("generic type %s is not supported yet", t.String())
}

// mutableBoundType maps a Go `*T` result/parameter whose type parameter is
// bound to an Ard type. Foreign named types use their first-class pointer
// form. An Ard struct becomes a mutable reference: the Go pointer is live
// shared storage, so the binding lowers to a pointer-backed local.
func mutableBoundType(bound Type) (Type, string) {
	if foreign, ok := bound.(*ForeignType); ok {
		if foreign.Pointer {
			return foreign, ""
		}
		if pointer := foreign.PointerForm(); pointer != nil {
			return pointer, ""
		}
		return nil, fmt.Sprintf("Go type %s has no pointer form", foreign)
	}
	if strct, ok := bound.(*StructDef); ok {
		return MakeMutableRef(strct), ""
	}
	return nil, fmt.Sprintf("pointer to %s is not supported yet", bound)
}

// inferGoFuncTypeArgs unifies a generic Go parameter type against the Ard type
// of a supplied argument, recording inferred bindings. It returns false when a
// conflict is found (the caller reports the diagnostic).
func inferGoFuncTypeArgs(pattern types.Type, actual Type, tparams *types.TypeParamList, inferred []Type, inferredSpans []SourceSpan, currentSpan SourceSpan) (bool, *types.TypeParam, Type, Type, SourceSpan) {
	// A `mut T` argument infers the referenced value type. Reference-ness is
	// not part of the binding: a Go `T` parameter position receives a copy
	// (ordinary pass-by-value), and only `*T` positions preserve the pointer.
	if ref, ok := actual.(*MutableRef); ok {
		actual = ref.Of()
	}
	switch pattern := pattern.(type) {
	case *types.TypeParam:
		for i := 0; i < tparams.Len(); i++ {
			if tparams.At(i) != pattern {
				continue
			}
			if inferred[i] != nil && !inferred[i].equal(actual) {
				return false, pattern, inferred[i], actual, inferredSpans[i]
			}
			if inferred[i] == nil {
				inferredSpans[i] = currentSpan
			}
			inferred[i] = actual
			return true, nil, nil, nil, SourceSpan{}
		}
	case *types.Slice:
		if list, ok := actual.(*List); ok {
			return inferGoFuncTypeArgs(pattern.Elem(), list.Of(), tparams, inferred, inferredSpans, currentSpan)
		}
	case *types.Map:
		if m, ok := actual.(*Map); ok {
			if ok, tp, prev, current, span := inferGoFuncTypeArgs(pattern.Key(), m.Key(), tparams, inferred, inferredSpans, currentSpan); !ok {
				return false, tp, prev, current, span
			}
			return inferGoFuncTypeArgs(pattern.Elem(), m.Value(), tparams, inferred, inferredSpans, currentSpan)
		}
	case *types.Pointer:
		if foreign, ok := actual.(*ForeignType); ok && foreign.Pointer {
			if value := foreign.ValueForm(); value != nil {
				return inferGoFuncTypeArgs(pattern.Elem(), value, tparams, inferred, inferredSpans, currentSpan)
			}
		}
	}
	return true, nil, nil, nil, SourceSpan{}
}
