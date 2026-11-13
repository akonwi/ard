package gotarget

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/akonwi/ard/air"
)

// Go orders only the calls, method calls, and channel receives in an
// expression. Any other operand, such as a variable or field read, may be
// evaluated after a call that appears to its right, so a read can see a value
// the call wrote (#575). sequenceOperands reads such an operand in order, but
// only when a call could change what it reads (ADR 0078). The functions in
// this file decide that.

// operandReadScope holds the facts about the function being lowered that
// decide whether a call can change one of its locals.
type operandReadScope struct {
	fn air.Function
	// shared marks the locals a call can change: their address is taken, a
	// closure captures their binding slot, or an unsafe block, which lowers
	// to a Go function literal, writes them. Computed on first use.
	shared  []bool
	scanned bool
}

// enterOperandReadScope makes fn the function whose locals operand
// classification refers to, until the returned function restores the
// previous scope.
func (l *lowerer) enterOperandReadScope(fn air.Function) func() {
	previous := l.readScope
	l.readScope = operandReadScope{fn: fn}
	return func() { l.readScope = previous }
}

// operandMayChange reports whether op reads storage that a call evaluated
// after it could change.
func (l *lowerer) operandMayChange(op *operand) bool {
	if op.place {
		if _, fixed := op.expr.(*ast.Ident); fixed {
			return false
		}
		return op.source == nil || l.placeMayMove(op.source)
	}
	if l.isStableExpr(op.expr) {
		return false
	}
	if op.source == nil {
		// Without a source only the Go shape is known: a call is ordered
		// with the calls after it, and anything else may be a read.
		call, ok := op.expr.(*ast.CallExpr)
		return !ok || isConversionOrPureBuiltin(call)
	}
	return l.valueMayChange(op.source) || exposesDereference(op.expr)
}

// isStableExpr reports whether expr reads nothing a call could change: it is
// built only from constants, generated temporaries, function literals, reads
// already wrapped by readInOrder, and operators and predeclared conversions
// over those. Temporaries are assigned by setup statements that have already
// run; a temporary holding a pointer or map is never dereferenced here.
func (l *lowerer) isStableExpr(expr ast.Expr) bool {
	switch expr := expr.(type) {
	case *ast.BasicLit, *ast.FuncLit:
		return true
	case *ast.Ident:
		return strings.HasPrefix(expr.Name, "_") || expr.Name == "nil" || expr.Name == "true" || expr.Name == "false"
	case *ast.ParenExpr:
		return l.isStableExpr(expr.X)
	case *ast.UnaryExpr:
		return expr.Op != token.AND && expr.Op != token.ARROW && l.isStableExpr(expr.X)
	case *ast.BinaryExpr:
		return l.isStableExpr(expr.X) && l.isStableExpr(expr.Y)
	case *ast.KeyValueExpr:
		return l.isStableExpr(expr.Value)
	case *ast.CompositeLit:
		for _, elt := range expr.Elts {
			if !l.isStableExpr(elt) {
				return false
			}
		}
		return true
	case *ast.CallExpr:
		if fun, ok := l.runtimeFunction(expr); ok && fun == "Read" {
			// A read already wrapped by readInOrder.
			return true
		}
		fun, ok := expr.Fun.(*ast.Ident)
		if !ok || !goBuiltinTypes[fun.Name] || len(expr.Args) != 1 {
			return false
		}
		return l.isStableExpr(expr.Args[0])
	}
	return false
}

// valueMayChange reports whether the Go expression lowered from expr reads,
// outside any call it makes, storage that a call could write. A call
// evaluates its own arguments before it runs, so reads inside it are ordered.
// Unlisted kinds are assumed to read changeable storage.
func (l *lowerer) valueMayChange(expr *air.Expr) bool {
	switch expr.Kind {
	case air.ExprConstVoid, air.ExprConstInt, air.ExprConstFloat, air.ExprConstBool, air.ExprConstStr,
		air.ExprFunctionRef, air.ExprEnumVariant, air.ExprMakeMaybeNone, air.ExprMakeClosure:
		return false
	case air.ExprCall, air.ExprForeignCall, air.ExprForeignMethodCall, air.ExprCallClosure, air.ExprCallTrait,
		air.ExprMakeMaybeSome, air.ExprMakeError,
		air.ExprStrContains, air.ExprStrReplace, air.ExprStrReplaceAll, air.ExprStrStartsWith, air.ExprStrEndsWith, air.ExprStrTrim:
		// These lower to a function call, which evaluates the reads in its
		// arguments before it runs (TestReadOrderCallKindsLowerToCalls).
		return false
	case air.ExprToStr:
		// A rune converts with string(r); other values format with a call.
		return expr.Target == nil || l.isRuneType(expr.Target.Type) && l.valueMayChange(expr.Target)
	case air.ExprLoadLocal:
		payload := expr.LocalPayload()
		return payload == nil || l.localMayChange(payload.Local)
	case air.ExprLoadGlobal:
		payload := expr.GlobalPayload()
		return payload == nil || payload.Global < 0 || int(payload.Global) >= len(l.program.Globals) || l.program.Globals[payload.Global].Mutable
	case air.ExprForeignValue:
		payload := expr.ForeignPayload()
		return payload == nil || payload.Variable
	case air.ExprGetField, air.ExprTupleElem, air.ExprForeignFieldAccess:
		// A field of a value is part of that value; a field reached through a
		// pointer is not.
		return expr.Target == nil || !l.holdsValueInline(expr.Target.Type) || l.valueMayChange(expr.Target)
	case air.ExprStrSize, air.ExprStrIsEmpty, air.ExprNot, air.ExprNeg,
		air.ExprToInt, air.ExprToF64, air.ExprToAny,
		air.ExprScalarConvert, air.ExprInterfaceConversion, air.ExprTraitUpcast, air.ExprTraitRefProject,
		air.ExprChannelNarrow, air.ExprUnionWrap, air.ExprMakeResultOk, air.ExprMakeResultErr:
		return expr.Target == nil || l.valueMayChange(expr.Target)
	case air.ExprMutRef:
		payload := expr.ReferencePayload()
		if expr.Target == nil || payload == nil {
			return true
		}
		if payload.Mode == air.AddressablePlace {
			// The address of a place changes only if the place can move.
			return l.placeMayMove(expr.Target)
		}
		return l.valueMayChange(expr.Target)
	case air.ExprListSize, air.ExprListIsEmpty:
		// The length of a list value is part of the value; a list reached
		// through a pointer can grow.
		return expr.Target == nil || l.isReferenceType(expr.Target.Type) || l.valueMayChange(expr.Target)
	case air.ExprIntAdd, air.ExprIntSub, air.ExprIntMul, air.ExprIntDiv, air.ExprIntMod,
		air.ExprFloatAdd, air.ExprFloatSub, air.ExprFloatMul, air.ExprFloatDiv,
		air.ExprStrConcat, air.ExprEq, air.ExprNotEq, air.ExprLt, air.ExprLte, air.ExprGt, air.ExprGte,
		air.ExprAnd, air.ExprOr:
		payload := expr.BinaryPayload()
		return payload == nil || payload.Left == nil || payload.Right == nil ||
			l.valueMayChange(payload.Left) || l.valueMayChange(payload.Right)
	case air.ExprMakeList, air.ExprMakeFixedArray, air.ExprMakeTuple, air.ExprMakeChannel:
		for i := range expr.Args {
			if l.valueMayChange(&expr.Args[i]) {
				return true
			}
		}
		return false
	case air.ExprMakeStruct:
		payload := expr.AggregatePayload()
		if payload == nil {
			return true
		}
		for i := range payload.Fields {
			if l.valueMayChange(&payload.Fields[i].Value) {
				return true
			}
		}
		return false
	case air.ExprForeignStructInstance:
		payload := expr.ForeignPayload()
		if payload == nil {
			return true
		}
		for i := range payload.Fields {
			if l.valueMayChange(&payload.Fields[i].Value) {
				return true
			}
		}
		return false
	}
	return true
}

// holdsValueInline reports whether a value of typeID holds its fields in
// itself rather than behind a pointer.
func (l *lowerer) holdsValueInline(typeID air.TypeID) bool {
	info, ok := l.typeInfo(typeID)
	if !ok {
		return false
	}
	switch info.Kind {
	case air.TypeStruct, air.TypeTuple:
		return true
	case air.TypeForeignType:
		return !info.ForeignPointer && !info.ForeignInterface
	}
	return false
}

// placeMayMove reports whether the storage a place lying in base designates
// could be moved by a call: the place is fixed by base's own storage, or by
// the pointer base holds.
func (l *lowerer) placeMayMove(base *air.Expr) bool {
	if !l.holdsValueInline(base.Type) && l.isPointerShaped(base.Type) {
		return l.valueMayChange(base)
	}
	switch base.Kind {
	case air.ExprLoadLocal, air.ExprLoadGlobal:
		return false
	case air.ExprGetField, air.ExprTupleElem, air.ExprForeignFieldAccess:
		return base.Target == nil || l.placeMayMove(base.Target)
	case air.ExprDeref:
		return base.Target == nil || l.valueMayChange(base.Target)
	}
	return true
}

func (l *lowerer) isRuneType(typeID air.TypeID) bool {
	info, ok := l.typeInfo(typeID)
	return !ok || info.Kind == air.TypeRune
}

func (l *lowerer) isPointerShaped(typeID air.TypeID) bool {
	info, ok := l.typeInfo(typeID)
	return ok && (info.Kind == air.TypeReference || info.Kind == air.TypeForeignType && info.ForeignPointer)
}

// localMayChange reports whether a call could change local in the current
// function. A local whose storage is not shared cannot be written by a call:
// a closure snapshots it, and a callee reaches it only through its address.
func (l *lowerer) localMayChange(local air.LocalID) bool {
	scope := &l.readScope
	index := int(local)
	if index < 0 || index >= len(scope.fn.Locals) {
		return true
	}
	// Mirror localValueExpr: a captured slot and a pointer parameter are
	// read through a pointer; a reference-typed local holds its handle.
	if l.captureMode(scope.fn, local) == air.CaptureSlot {
		return true
	}
	if !l.foreignABIValueReferenceParam(scope.fn, local) && !l.isReferenceType(scope.fn.Locals[index].Type) && l.localIsPointerParam(scope.fn, local) {
		return true
	}
	if !scope.scanned {
		scope.shared = l.sharedLocals(scope.fn)
		scope.scanned = true
	}
	return scope.shared != nil && scope.shared[index]
}

// sharedLocals marks the locals of fn whose storage something other than fn's
// own statements can write. It returns nil when there are none.
func (l *lowerer) sharedLocals(fn air.Function) []bool {
	var shared []bool
	mark := func(local air.LocalID) {
		if int(local) < 0 || int(local) >= len(fn.Locals) {
			return
		}
		if shared == nil {
			shared = make([]bool, len(fn.Locals))
		}
		shared[local] = true
	}
	var visit func(air.Expr)
	visit = func(expr air.Expr) {
		switch expr.Kind {
		case air.ExprMutRef:
			if payload := expr.ReferencePayload(); payload != nil && payload.Mode == air.AddressablePlace {
				if local, ok := l.placeRootLocal(expr.Target); ok {
					mark(local)
				}
			}
		case air.ExprMakeClosure:
			payload := expr.CallPayload()
			if payload == nil || !validFunctionID(l.program, payload.Function) {
				return
			}
			captures := l.program.Functions[payload.Function].Captures
			for i, local := range payload.CaptureLocals {
				if i < len(captures) && captures[i].Mode == air.CaptureSlot {
					mark(local)
				}
			}
		case air.ExprUnsafeBlock:
			// The body runs inside a Go function literal called in place, so
			// its writes to fn's locals can land after a read to its left.
			// Unsafe blocks are rare; treat every local as shared.
			for i := range fn.Locals {
				mark(air.LocalID(i))
			}
		}
	}
	walkBlockExprs(fn.Body, visit)
	return shared
}

// placeRootLocal returns the local whose own storage a place expression lies
// in. A place reached through a pointer lies in the pointee instead.
func (l *lowerer) placeRootLocal(expr *air.Expr) (air.LocalID, bool) {
	for expr != nil {
		switch expr.Kind {
		case air.ExprLoadLocal:
			payload := expr.LocalPayload()
			if payload == nil {
				return 0, false
			}
			return payload.Local, true
		case air.ExprGetField, air.ExprTupleElem, air.ExprForeignFieldAccess:
			if expr.Target == nil || l.isPointerShaped(expr.Target.Type) {
				return 0, false
			}
			expr = expr.Target
		default:
			return 0, false
		}
	}
	return 0, false
}

// exposesDereference reports whether expr dereferences a pointer or receives
// from a channel outside any call. It guards against an operand whose Go
// expression was adapted after lowering in a way its source does not show.
func exposesDereference(expr ast.Expr) bool {
	switch expr := expr.(type) {
	case *ast.StarExpr:
		return true
	case *ast.UnaryExpr:
		return expr.Op == token.ARROW || exposesDereference(expr.X)
	case *ast.ParenExpr:
		return exposesDereference(expr.X)
	case *ast.SelectorExpr:
		return exposesDereference(expr.X)
	case *ast.BinaryExpr:
		return exposesDereference(expr.X) || exposesDereference(expr.Y)
	case *ast.IndexExpr:
		return exposesDereference(expr.X) || exposesDereference(expr.Index)
	case *ast.CallExpr:
		if !isConversionOrPureBuiltin(expr) {
			return false
		}
		for _, arg := range expr.Args {
			if exposesDereference(arg) {
				return true
			}
		}
	case *ast.CompositeLit:
		for _, elt := range expr.Elts {
			if exposesDereference(elt) {
				return true
			}
		}
	case *ast.KeyValueExpr:
		return exposesDereference(expr.Value)
	}
	return false
}

// exprContainsCall reports whether evaluating expr calls a function that
// could change what a read sees, or receives from a channel. Conversions, the
// len and cap built-ins, the runtime package's functions, and the factory
// that snapshots a closure's captures are not such calls; a function
// literal's body is not evaluated.
func (l *lowerer) exprContainsCall(expr ast.Expr) bool {
	switch expr := expr.(type) {
	case nil, *ast.Ident, *ast.BasicLit, *ast.FuncLit:
		return false
	case *ast.CallExpr:
		if isClosureFactory(expr) {
			return false
		}
		if _, ok := l.runtimeFunction(expr); ok {
			for _, arg := range expr.Args {
				if l.exprContainsCall(arg) {
					return true
				}
			}
			return false
		}
		if !isConversionOrPureBuiltin(expr) {
			return true
		}
		for _, arg := range expr.Args {
			if l.exprContainsCall(arg) {
				return true
			}
		}
		return false
	case *ast.UnaryExpr:
		return expr.Op == token.ARROW || l.exprContainsCall(expr.X)
	case *ast.ParenExpr:
		return l.exprContainsCall(expr.X)
	case *ast.StarExpr:
		return l.exprContainsCall(expr.X)
	case *ast.SelectorExpr:
		return l.exprContainsCall(expr.X)
	case *ast.BinaryExpr:
		return l.exprContainsCall(expr.X) || l.exprContainsCall(expr.Y)
	case *ast.IndexExpr:
		return l.exprContainsCall(expr.X) || l.exprContainsCall(expr.Index)
	case *ast.IndexListExpr:
		return l.exprContainsCall(expr.X)
	case *ast.SliceExpr:
		return l.exprContainsCall(expr.X) || l.exprContainsCall(expr.Low) || l.exprContainsCall(expr.High) || l.exprContainsCall(expr.Max)
	case *ast.TypeAssertExpr:
		return l.exprContainsCall(expr.X)
	case *ast.KeyValueExpr:
		return l.exprContainsCall(expr.Key) || l.exprContainsCall(expr.Value)
	case *ast.CompositeLit:
		for _, elt := range expr.Elts {
			if l.exprContainsCall(elt) {
				return true
			}
		}
		return false
	}
	return true
}

// runtimeFunction returns the name of the generated runtime package's
// function that call calls, if it calls one. None of them writes program
// state.
func (l *lowerer) runtimeFunction(call *ast.CallExpr) (string, bool) {
	fun := call.Fun
	switch indexed := fun.(type) {
	case *ast.IndexExpr:
		fun = indexed.X
	case *ast.IndexListExpr:
		fun = indexed.X
	}
	selector, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	alias, ok := l.resolvedImportAliases[importAliasKey{alias: "ard", path: l.runtimeImportPath()}]
	if !ok || pkg.Name != alias {
		return "", false
	}
	return selector.Sel.Name, true
}

// isClosureFactory reports whether call is the function literal that
// lowerInlineClosure calls in place to snapshot a closure's captures: it
// copies locals or their addresses and returns the closure, so it changes
// nothing a read could see.
func isClosureFactory(call *ast.CallExpr) bool {
	factory, ok := call.Fun.(*ast.FuncLit)
	if !ok || len(call.Args) != 0 || factory.Body == nil || len(factory.Body.List) == 0 {
		return false
	}
	stmts := factory.Body.List
	for _, stmt := range stmts[:len(stmts)-1] {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Rhs) != 1 || !isLocalSnapshot(assign.Rhs[0]) {
			return false
		}
	}
	ret, ok := stmts[len(stmts)-1].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	_, ok = ret.Results[0].(*ast.FuncLit)
	return ok
}

func isLocalSnapshot(expr ast.Expr) bool {
	switch expr := expr.(type) {
	case *ast.Ident:
		return true
	case *ast.StarExpr:
		_, ok := expr.X.(*ast.Ident)
		return ok
	case *ast.UnaryExpr:
		_, ok := expr.X.(*ast.Ident)
		return expr.Op == token.AND && ok
	}
	return false
}

// goBuiltinTypes are the predeclared Go type names the backend converts to.
var goBuiltinTypes = map[string]bool{
	"any": true, "bool": true, "string": true, "error": true, "rune": true, "byte": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"float32": true, "float64": true,
}

// isConversionOrPureBuiltin reports whether call is a conversion to a
// predeclared or literal type, or len or cap. Other conversions, to named
// types, look like calls and are treated as calls.
func isConversionOrPureBuiltin(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return goBuiltinTypes[fun.Name] || fun.Name == "len" || fun.Name == "cap"
	case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType, *ast.InterfaceType, *ast.StructType:
		return true
	case *ast.ParenExpr:
		switch fun.X.(type) {
		case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType, *ast.InterfaceType, *ast.StarExpr:
			return true
		}
	}
	return false
}
