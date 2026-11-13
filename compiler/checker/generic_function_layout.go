package checker

import (
	"fmt"

	"github.com/akonwi/ard/parse"
)

// The Go target lays out a function's results by its declared return type
// (ADRs 0038 and 0077): `fn() Int?` returns `(int, bool)`, while `fn() $T`
// returns one value. Go generics substitute a type parameter with the Go type
// of its argument, so with `$T` bound to `Int?` the two are different Go
// function types.
//
// The backend converts a single function value where it crosses between
// generic and concrete code, including one held in a Maybe, a Result, or a
// tuple. A list, map, fixed array, channel, pointer, or generic type argument
// holding such a function has no conversion, so one Ard type such as
// `[fn() Int?]` has two Go types, depending on whether the value was built in
// concrete code or in generic code as `[fn() $T]`.
//
// The checker follows where such a value was built and reports a use that
// needs the other Go type, instead of letting it fail in the Go compiler
// (#588). It reports only what it knows: a value whose origin it does not
// follow is let through.

// goReturnLayout is how the Go target lays out the results of a function
// declared to return a type. It mirrors returnShapeOf in compiler/go.
type goReturnLayout struct {
	kind goReturnLayoutKind
	// voidValue marks a Maybe or Result whose value is Void and so has no
	// value result.
	voidValue bool
	// spread marks a Result whose success value is a tuple that occupies one
	// result per element.
	spread bool
}

type goReturnLayoutKind uint8

const (
	goReturnNone goReturnLayoutKind = iota
	goReturnValue
	goReturnMaybe
	goReturnResult
	goReturnTuple
)

func goReturnLayoutOf(t Type) goReturnLayout {
	t = deref(t)
	if t == nil || t == Void {
		return goReturnLayout{kind: goReturnNone}
	}
	switch typ := t.(type) {
	case *Maybe:
		return goReturnLayout{kind: goReturnMaybe, voidValue: deref(typ.Of()) == Void}
	case *Result:
		// Only Str and Error errors use Go's error result (ADRs 0038, 0053).
		if errType := deref(typ.Err()); errType != Str && errType != BuiltinError {
			break
		}
		value := deref(typ.Val())
		_, spread := value.(*Tuple)
		return goReturnLayout{kind: goReturnResult, voidValue: value == Void, spread: spread}
	case *Tuple:
		return goReturnLayout{kind: goReturnTuple}
	}
	return goReturnLayout{kind: goReturnValue}
}

// functionLayoutCrossing describes a function type whose Go layout differs
// between two types of the same shape, in a position the backend cannot
// convert.
type functionLayoutCrossing struct {
	declared *FunctionDef
	concrete *FunctionDef
	// holder names what holds the function: "list", "map", and so on.
	holder string
}

// unconvertibleFunctionLayout compares the type a slot was declared with to
// the type a value was built with, and returns the first function type, held
// in something the backend cannot convert, whose Go layout differs between the
// two. It returns nil when every difference is convertible. A type parameter
// on either side matches anything: its Go type is whatever the value has.
func unconvertibleFunctionLayout(declared, concrete Type) *functionLayoutCrossing {
	return functionLayoutCrossingIn(declared, concrete, "")
}

func functionLayoutCrossingIn(declared, concrete Type, holder string) *functionLayoutCrossing {
	declared = deref(declared)
	concrete = deref(concrete)
	switch decl := declared.(type) {
	case *FunctionDef:
		conc, ok := concrete.(*FunctionDef)
		if !ok || len(decl.Parameters) != len(conc.Parameters) {
			return nil
		}
		if holder != "" && goReturnLayoutOf(decl.ReturnType) != goReturnLayoutOf(conc.ReturnType) {
			return &functionLayoutCrossing{declared: decl, concrete: conc, holder: holder}
		}
		// A function nested in another function type is converted with it.
		for i := range decl.Parameters {
			if crossing := functionLayoutCrossingIn(decl.Parameters[i].Type, conc.Parameters[i].Type, holder); crossing != nil {
				return crossing
			}
		}
		return functionLayoutCrossingIn(decl.ReturnType, conc.ReturnType, holder)
	case *Maybe:
		if conc, ok := concrete.(*Maybe); ok {
			return functionLayoutCrossingIn(decl.Of(), conc.Of(), holder)
		}
	case *Result:
		if conc, ok := concrete.(*Result); ok {
			if crossing := functionLayoutCrossingIn(decl.Val(), conc.Val(), holder); crossing != nil {
				return crossing
			}
			return functionLayoutCrossingIn(decl.Err(), conc.Err(), holder)
		}
	case *Tuple:
		if conc, ok := concrete.(*Tuple); ok && len(decl.elements) == len(conc.elements) {
			for i := range decl.elements {
				if crossing := functionLayoutCrossingIn(decl.elements[i], conc.elements[i], holder); crossing != nil {
					return crossing
				}
			}
		}
	case *List:
		if conc, ok := concrete.(*List); ok {
			return functionLayoutCrossingIn(decl.Of(), conc.Of(), "list")
		}
	case *Slice:
		if conc, ok := concrete.(*Slice); ok {
			return functionLayoutCrossingIn(decl.of, conc.of, "slice")
		}
	case *FixedArray:
		if conc, ok := concrete.(*FixedArray); ok {
			return functionLayoutCrossingIn(decl.of, conc.of, "fixed array")
		}
	case *Map:
		if conc, ok := concrete.(*Map); ok {
			if crossing := functionLayoutCrossingIn(decl.Key(), conc.Key(), "map"); crossing != nil {
				return crossing
			}
			return functionLayoutCrossingIn(decl.Value(), conc.Value(), "map")
		}
	case *Chan:
		if conc, ok := concrete.(*Chan); ok {
			return functionLayoutCrossingIn(decl.Of(), conc.Of(), "channel")
		}
	case *Receiver:
		if conc, ok := concrete.(*Receiver); ok {
			return functionLayoutCrossingIn(decl.Of(), conc.Of(), "channel")
		}
	case *Sender:
		if conc, ok := concrete.(*Sender); ok {
			return functionLayoutCrossingIn(decl.Of(), conc.Of(), "channel")
		}
	case *MutableRef:
		if conc, ok := concrete.(*MutableRef); ok {
			return functionLayoutCrossingIn(decl.Of(), conc.Of(), "pointer")
		}
	case *StructDef:
		if conc, ok := concrete.(*StructDef); ok {
			return functionLayoutCrossingInTypeArgs(decl.TypeArgs, conc.TypeArgs)
		}
	case *ForeignType:
		if conc, ok := concrete.(*ForeignType); ok {
			return functionLayoutCrossingInTypeArgs(decl.TypeArgs, conc.TypeArgs)
		}
	}
	return nil
}

func functionLayoutCrossingInTypeArgs(declared, concrete []Type) *functionLayoutCrossing {
	if len(declared) != len(concrete) {
		return nil
	}
	for i := range declared {
		if crossing := functionLayoutCrossingIn(declared[i], concrete[i], "type argument"); crossing != nil {
			return crossing
		}
	}
	return nil
}

// layoutOrigin returns the type that gives expr's value its Go layout, or nil
// when the checker does not follow it. The origin is the declared type of the
// slot or expression that produced the value: a field's declared type, a
// function's declared return type, or a literal's own type.
func layoutOrigin(expr Expression) Type {
	switch e := expr.(type) {
	case *ListLiteral:
		return e.Type()
	case *MapLiteral:
		return e.Type()
	case *StructInstance:
		return e.Type()
	case *ModuleStructInstance:
		return e.Type()
	case Variable:
		return e.sym.layoutOrigin
	case *Variable:
		return e.sym.layoutOrigin
	case *Identifier:
		return e.sym.layoutOrigin
	case *InstanceProperty:
		return declaredFieldLayout(e.Subject.Type(), e.Property)
	case *FunctionCall:
		if e.declaration != nil {
			return e.declaration.ReturnType
		}
		// A call through a binding that holds a function value.
		if e.binding != nil {
			if callee, ok := e.binding.layoutOrigin.(*FunctionDef); ok {
				return callee.ReturnType
			}
		}
	case *ModuleFunctionCall:
		return layoutOrigin(e.Call)
	case *InstanceMethod:
		return layoutOrigin(e.Method)
	case *FunctionValueCall:
		if callee, ok := layoutOrigin(e.Callee).(*FunctionDef); ok {
			return callee.ReturnType
		}
	case *DerefExpr:
		if ref, ok := layoutOrigin(e.Operand).(*MutableRef); ok {
			return ref.Of()
		}
	case *MutableRefExpr:
		if operand := layoutOrigin(e.Operand); operand != nil {
			return &MutableRef{of: operand, readOnly: e.ReadOnly}
		}
	case *FunctionDef:
		// A function literal is built with its own type.
		return e
	}
	return nil
}

// declaredFieldLayout is the type a struct field was declared with, which a
// generic struct's Go field keeps for every instantiation.
func declaredFieldLayout(subject Type, field string) Type {
	instance, ok := derefMutableRef(deref(subject)).(*StructDef)
	if !ok {
		return nil
	}
	definition := canonicalStructDefinition(instance)
	if definition == nil {
		return nil
	}
	declared, ok := definition.Fields[field]
	if !ok {
		return nil
	}
	return declared
}

// rejectFunctionLayout reports value used where the Go layout of slot is
// needed, when value is known to have been built with a different layout that
// the backend cannot convert. It returns true when it reported a diagnostic.
func (c *Checker) rejectFunctionLayout(slot Type, value Expression, location parse.Location) bool {
	if value == nil {
		return false
	}
	return c.rejectLayoutOrigin(slot, layoutOrigin(value), location)
}

// rejectLayoutOrigin is rejectFunctionLayout for a value whose origin the
// caller already knows. A nil slot or origin is unknown and reports nothing.
func (c *Checker) rejectLayoutOrigin(slot, origin Type, location parse.Location) bool {
	diagnostic := c.layoutDiagnostic(slot, origin, location)
	if diagnostic == nil {
		return false
	}
	c.addDiagnostic(diagnostic.build())
	return true
}

func (c *Checker) layoutDiagnostic(slot, origin Type, location parse.Location) *unconvertibleFunctionLayoutDiagnostic {
	if slot == nil || origin == nil || slot == origin {
		return nil
	}
	crossing := unconvertibleFunctionLayout(slot, origin)
	if crossing == nil {
		return nil
	}
	return &unconvertibleFunctionLayoutDiagnostic{
		Slot:           slot,
		Origin:         origin,
		SlotFunction:   crossing.declared,
		OriginFunction: crossing.concrete,
		Holder:         crossing.holder,
		Span:           c.sourceSpan(location),
	}
}

// rejectReturnLayout checks the value a function body produces against the
// function's return type, which gives the Go function its result types.
func (c *Checker) rejectReturnLayout(returnType Type, body *Block, location parse.Location) {
	if body == nil {
		return
	}
	c.rejectFunctionLayout(returnType, body.finalExpression(), location)
}

// rejectArgumentLayouts checks call arguments against the parameters they are
// passed to. params are the callee's declared parameters, which give the Go
// function its parameter types.
func (c *Checker) rejectArgumentLayouts(params []Parameter, args []Expression, sources []parse.Expression, callLocation parse.Location) {
	for i, arg := range args {
		if i >= len(params) {
			return
		}
		location := callLocation
		if i < len(sources) && sources[i] != nil {
			location = sources[i].GetLocation()
		}
		if c.rejectFunctionLayout(params[i].Type, arg, location) {
			return
		}
	}
}

// rejectValueArgumentLayouts checks the arguments of a call through a
// function value. calleeOrigin is the type the function value was built with,
// which gives the Go function its parameter types; a nil origin is unknown.
func (c *Checker) rejectValueArgumentLayouts(calleeOrigin Type, args []Expression, sources []parse.Expression, callLocation parse.Location) {
	if built, ok := calleeOrigin.(*FunctionDef); ok && args != nil {
		c.rejectArgumentLayouts(built.Parameters, args, sources, callLocation)
	}
}

// rejectElementLayout checks a use of a collection that reads or writes its
// elements, such as `.at()`. Concrete code handles the elements with the
// collection's own type, so the collection must have been built with it.
func (c *Checker) rejectElementLayout(collection Expression, location parse.Location) bool {
	diagnostic := c.elementLayoutDiagnostic(collection, location)
	if diagnostic == nil {
		return false
	}
	c.addDiagnostic(diagnostic.build())
	return true
}

// elementLayoutDiagnostic is the diagnostic rejectElementLayout would report,
// or nil. A `for` loop holds it until the loop uses an element.
func (c *Checker) elementLayoutDiagnostic(collection Expression, location parse.Location) *unconvertibleFunctionLayoutDiagnostic {
	if collection == nil {
		return nil
	}
	return c.layoutDiagnostic(derefMutableRef(deref(collection.Type())), layoutOrigin(collection), location)
}

// reportDeferredLayout reports, once, the layout diagnostic a loop left on
// its cursor.
func (c *Checker) reportDeferredLayout(sym *Symbol) {
	if sym.deferredLayout == nil {
		return
	}
	c.addDiagnostic(sym.deferredLayout.build())
	sym.deferredLayout = nil
}

type unconvertibleFunctionLayoutDiagnostic struct {
	// Slot is the type whose layout the use needs; Origin is the type the
	// value was built with.
	Slot           Type
	Origin         Type
	SlotFunction   *FunctionDef
	OriginFunction *FunctionDef
	Holder         string
	Span           SourceSpan
}

func (d unconvertibleFunctionLayoutDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Cannot use %s as %s: Ard does not convert a function held in a %s", d.Origin, d.Slot, d.Holder),
		"Generic function type cannot be converted",
		fmt.Sprintf("`%s` and `%s` return their results differently in Go, and Ard converts a single function between them but not one held in a %s. %s", d.OriginFunction, d.SlotFunction, d.Holder, d.fixes()),
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("built as `%s`, used as `%s`", d.Origin, d.Slot)},
	)
	diagnostic.Code = DiagnosticCodeUnconvertibleFunctionLayout
	return diagnostic
}

// fixes describes the two ways to write the program so both sides agree:
// spell the return shape in the generic declaration, or hold a struct that
// has the function as a field, which the backend converts.
func (d unconvertibleFunctionLayoutDiagnostic) fixes() string {
	generic, specific := d.OriginFunction, d.SlotFunction
	param, spelling := specificReturnSpelling(generic, specific)
	if spelling == "" {
		generic, specific = specific, generic
		param, spelling = specificReturnSpelling(generic, specific)
	}
	if spelling == "" {
		return "Declare both with the same return type, or wrap the function in a struct and hold that struct instead."
	}
	return fmt.Sprintf("In the generic declaration, write the return type as `%s` instead of `%s`. To keep `%s` open to any type, wrap the function in a struct, such as `struct Handler { run: %s }`, and hold that struct instead.", spelling, param, param, generic)
}

// specificReturnSpelling returns the type parameter that generic returns and
// how to write that return type so it has the Go layout of specific's. It
// returns empty strings when generic does not return a bare type parameter,
// or when specific's return shape has no spelling with one.
func specificReturnSpelling(generic, specific *FunctionDef) (param, spelling string) {
	typeVar, ok := deref(generic.ReturnType).(*TypeVar)
	if !ok {
		return "", ""
	}
	param = typeVar.String()
	switch returned := deref(specific.ReturnType).(type) {
	case *Maybe:
		return param, param + "?"
	case *Result:
		return param, fmt.Sprintf("%s!%s", param, deref(returned.Err()))
	}
	if deref(specific.ReturnType) == Void {
		return param, "Void"
	}
	return "", ""
}
