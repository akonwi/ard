package gotarget

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/akonwi/ard/air"
)

// Generic declarations are emitted as Go generics, and Go substitutes a type
// parameter with the Go type of its argument. A function's Go results follow
// its declared return type (ADRs 0038 and 0077), so the same Ard function type
// can have two Go layouts: a slot declared `fn() $T` holds a
// `func() runtime.Maybe[int]` when T is Int?, while the concrete type
// `fn() Int?` is `func() (int, bool)`.
//
// The layout of a value is described by its concrete type c together with a
// view: the type it was declared with. The view is c itself for ordinary
// values, and a type mentioning type parameters for a value that lives in a
// generic slot (a field of a generic struct, a parameter or the return of a
// generic function). The layout differs from c's own only where the view has
// a function type whose results are laid out differently, possibly nested in
// another function type, a Maybe, a Result, or a tuple.
//
// adaptLayout converts a value between its concrete layout and a declared
// view where it enters or leaves a generic slot. Lists, maps, and generic type
// arguments holding such functions are not converted.

// returnShape is how a function declared to return a type lays out its Go
// results: no result, one value, or the ABI results of a Maybe, a Result, or a
// tuple.
type returnShape struct {
	kind returnShapeKind
	// voidValue marks a Maybe or Result whose value is Void and so has no
	// value result.
	voidValue bool
	// spread marks a Result whose success value is a tuple that occupies one
	// result per element.
	spread bool
}

type returnShapeKind uint8

const (
	returnNone returnShapeKind = iota
	returnValue
	returnMaybe
	returnResult
	returnTuple
)

func (l *lowerer) returnShapeOf(typeID air.TypeID) returnShape {
	if typeID == air.NoType || l.isVoidType(typeID) {
		return returnShape{kind: returnNone}
	}
	if !l.abiReturnShapeAvailable(typeID) {
		return returnShape{kind: returnValue}
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeMaybe:
		return returnShape{kind: returnMaybe, voidValue: l.isVoidType(info.Elem)}
	case air.TypeResult:
		_, spread := l.abiSpreadTuple(info.Value)
		return returnShape{kind: returnResult, voidValue: l.isVoidType(info.Value), spread: spread}
	default:
		return returnShape{kind: returnTuple}
	}
}

// layoutDiffers reports whether a value of concrete type c has a different Go
// type when it lives in a slot declared with type view.
func (l *lowerer) layoutDiffers(c, view air.TypeID) bool {
	if c == view || !validTypeID(l.program, c) || !validTypeID(l.program, view) {
		return false
	}
	ci := &l.program.Types[c-1]
	vi := &l.program.Types[view-1]
	// A type parameter is instantiated with c's own Go type.
	if ci.Kind != vi.Kind {
		return false
	}
	switch vi.Kind {
	case air.TypeFunction:
		// Only foreign functions are variadic, and those are never declared
		// with type parameters.
		if len(ci.Params) != len(vi.Params) || ci.Variadic || vi.Variadic {
			return false
		}
		if l.returnShapeOf(ci.Return) != l.returnShapeOf(vi.Return) || l.layoutDiffers(ci.Return, vi.Return) {
			return true
		}
		for i := range vi.Params {
			if l.layoutDiffers(ci.Params[i], vi.Params[i]) {
				return true
			}
		}
	case air.TypeMaybe:
		return l.layoutDiffers(ci.Elem, vi.Elem)
	case air.TypeResult:
		return l.layoutDiffers(ci.Value, vi.Value) || l.layoutDiffers(ci.Error, vi.Error)
	case air.TypeTuple:
		if len(ci.Elems) != len(vi.Elems) {
			return false
		}
		for i := range vi.Elems {
			if l.layoutDiffers(ci.Elems[i], vi.Elems[i]) {
				return true
			}
		}
	}
	return false
}

// goLayoutType is the Go type of a value of concrete type c that lives in a
// slot declared with type view.
func (l *lowerer) goLayoutType(c, view air.TypeID) (ast.Expr, error) {
	if !l.layoutDiffers(c, view) {
		return l.goType(c)
	}
	ci := l.program.Types[c-1]
	vi := l.program.Types[view-1]
	switch vi.Kind {
	case air.TypeFunction:
		params := make([]*ast.Field, len(ci.Params))
		for i := range ci.Params {
			typ, err := l.goLayoutType(ci.Params[i], vi.Params[i])
			if err != nil {
				return nil, err
			}
			params[i] = &ast.Field{Type: typ}
		}
		fnType := &ast.FuncType{Params: &ast.FieldList{List: params}}
		results, err := l.goLayoutReturnFields(ci.Return, vi.Return)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			fnType.Results = &ast.FieldList{List: results}
		}
		return fnType, nil
	case air.TypeMaybe:
		elem, err := l.goLayoutType(ci.Elem, vi.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.IndexExpr{X: l.runtimeQualified("Maybe"), Index: elem}, nil
	case air.TypeResult:
		l.markRuntimeHelper("result")
		value, err := l.goLayoutType(ci.Value, vi.Value)
		if err != nil {
			return nil, err
		}
		errType, err := l.goLayoutType(ci.Error, vi.Error)
		if err != nil {
			return nil, err
		}
		return &ast.IndexListExpr{X: l.runtimeQualified("Result"), Indices: []ast.Expr{value, errType}}, nil
	default: // air.TypeTuple
		fields := make([]*ast.Field, len(ci.Elems))
		for i := range ci.Elems {
			elem, err := l.goLayoutType(ci.Elems[i], vi.Elems[i])
			if err != nil {
				return nil, err
			}
			fields[i] = &ast.Field{Names: []*ast.Ident{l.ident(air.TupleGoFieldName(i))}, Type: elem}
		}
		return &ast.StructType{Fields: &ast.FieldList{List: fields}}, nil
	}
}

// goLayoutReturnFields is the Go result list of a function returning concrete
// type c whose return is declared as view.
func (l *lowerer) goLayoutReturnFields(c, view air.TypeID) ([]*ast.Field, error) {
	var types []ast.Expr
	appendType := func(c, view air.TypeID) error {
		typ, err := l.goLayoutType(c, view)
		types = append(types, typ)
		return err
	}
	var err error
	switch shape := l.returnShapeOf(view); shape.kind {
	case returnNone:
	case returnValue:
		err = appendType(c, view)
	case returnMaybe:
		if !shape.voidValue {
			err = appendType(l.program.Types[c-1].Elem, l.program.Types[view-1].Elem)
		}
		types = append(types, l.ident("bool"))
	case returnTuple:
		ci, vi := l.program.Types[c-1], l.program.Types[view-1]
		for i := range vi.Elems {
			if err = appendType(ci.Elems[i], vi.Elems[i]); err != nil {
				break
			}
		}
	case returnResult:
		cv, vv := l.program.Types[c-1].Value, l.program.Types[view-1].Value
		if shape.spread {
			ci, vi := l.program.Types[cv-1], l.program.Types[vv-1]
			for i := range vi.Elems {
				if err = appendType(ci.Elems[i], vi.Elems[i]); err != nil {
					break
				}
			}
		} else if !shape.voidValue {
			err = appendType(cv, vv)
		}
		types = append(types, l.ident("error"))
	}
	if err != nil {
		return nil, err
	}
	fields := make([]*ast.Field, len(types))
	for i, typ := range types {
		fields[i] = &ast.Field{Type: typ}
	}
	return fields, nil
}

// adaptLayout converts value, of concrete type c, between c's own layout and
// the layout of a slot declared with type view: into the slot when toView is
// set, out of it otherwise. A function is wrapped in a function literal with
// the target layout; a Maybe, Result, or tuple is rebuilt around its adapted
// contents. Statements the conversion needs are appended to stmts.
func (l *lowerer) adaptLayout(stmts []ast.Stmt, value ast.Expr, c, view air.TypeID, toView bool) ([]ast.Stmt, ast.Expr, error) {
	if !l.layoutDiffers(c, view) {
		return stmts, value, nil
	}
	ci := l.program.Types[c-1]
	vi := l.program.Types[view-1]
	if vi.Kind == air.TypeFunction {
		return l.adaptFunctionLayout(stmts, value, ci, vi, toView)
	}
	targetType, err := l.layoutTypeFor(c, view, toView)
	if err != nil {
		return nil, nil, err
	}
	switch vi.Kind {
	case air.TypeMaybe:
		stmts, value = l.bindTemp(stmts, value)
		elemType, err := l.layoutTypeFor(ci.Elem, vi.Elem, toView)
		if err != nil {
			return nil, nil, err
		}
		body, elem, err := l.adaptLayout(nil, l.maybeValueExpr(value), ci.Elem, vi.Elem, toView)
		if err != nil {
			return nil, nil, err
		}
		temp := l.nextTemp()
		some := &ast.CallExpr{Fun: &ast.IndexExpr{X: l.runtimeQualified("Some"), Index: elemType}, Args: []ast.Expr{elem}}
		body = append(body, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{some}})
		stmts = append(stmts,
			&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(temp)}, Type: targetType}}}},
			&ast.IfStmt{Cond: l.maybeIsSomeExpr(value), Body: &ast.BlockStmt{List: body}},
		)
		return stmts, l.ident(temp), nil
	case air.TypeResult:
		stmts, value = l.bindTemp(stmts, value)
		stmts, okValue, err := l.adaptLayout(stmts, selectorExpr(value, "Value"), ci.Value, vi.Value, toView)
		if err != nil {
			return nil, nil, err
		}
		stmts, errValue, err := l.adaptLayout(stmts, selectorExpr(value, "Err"), ci.Error, vi.Error, toView)
		if err != nil {
			return nil, nil, err
		}
		return stmts, &ast.CompositeLit{Type: targetType, Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: l.ident("Value"), Value: okValue},
			&ast.KeyValueExpr{Key: l.ident("Err"), Value: errValue},
			&ast.KeyValueExpr{Key: l.ident("Ok"), Value: selectorExpr(value, "Ok")},
		}}, nil
	default: // air.TypeTuple
		stmts, value = l.bindTemp(stmts, value)
		elts := make([]ast.Expr, len(ci.Elems))
		for i := range ci.Elems {
			var elem ast.Expr
			stmts, elem, err = l.adaptLayout(stmts, selectorExpr(value, air.TupleGoFieldName(i)), ci.Elems[i], vi.Elems[i], toView)
			if err != nil {
				return nil, nil, err
			}
			elts[i] = &ast.KeyValueExpr{Key: l.ident(air.TupleGoFieldName(i)), Value: elem}
		}
		return stmts, &ast.CompositeLit{Type: targetType, Elts: elts}, nil
	}
}

// layoutTypeFor is the Go type adaptLayout produces: the view's layout when
// converting into the slot, c's own otherwise.
func (l *lowerer) layoutTypeFor(c, view air.TypeID, toView bool) (ast.Expr, error) {
	if toView {
		return l.goLayoutType(c, view)
	}
	return l.goType(c)
}

// adaptFunctionLayout wraps the function value fn in a function literal with
// the target layout. The literal adapts each argument the other way, calls fn,
// and converts its results.
func (l *lowerer) adaptFunctionLayout(stmts []ast.Stmt, fn ast.Expr, c, view air.TypeInfo, toView bool) ([]ast.Stmt, ast.Expr, error) {
	stmts, callee := l.bindTemp(stmts, fn)
	var body []ast.Stmt
	params := make([]*ast.Field, len(c.Params))
	args := make([]ast.Expr, len(c.Params))
	for i := range c.Params {
		typ, err := l.layoutTypeFor(c.Params[i], view.Params[i], toView)
		if err != nil {
			return nil, nil, err
		}
		// Parameters get fresh names: a nested wrapper's callee may be an
		// enclosing wrapper's parameter.
		name := l.nextTemp()
		params[i] = &ast.Field{Names: []*ast.Ident{l.ident(name)}, Type: typ}
		body, args[i], err = l.adaptLayout(body, l.ident(name), c.Params[i], view.Params[i], !toView)
		if err != nil {
			return nil, nil, err
		}
	}
	call := &ast.CallExpr{Fun: callee, Args: args}

	// from is the view the callee returns by, to the one the literal returns by.
	from, to := view.Return, c.Return
	if toView {
		from, to = c.Return, view.Return
	}
	results, err := l.goLayoutReturnFields(c.Return, to)
	if err != nil {
		return nil, nil, err
	}
	if l.returnShapeOf(c.Return) == l.returnShapeOf(view.Return) && !l.layoutDiffers(c.Return, view.Return) {
		if len(results) == 0 {
			body = append(body, &ast.ExprStmt{X: call})
		} else {
			body = append(body, &ast.ReturnStmt{Results: []ast.Expr{call}})
		}
	} else {
		var value ast.Expr
		body, value, err = l.packLayoutResults(body, call, c.Return, from)
		if err != nil {
			return nil, nil, err
		}
		body, value, err = l.adaptLayout(body, value, c.Return, view.Return, toView)
		if err != nil {
			return nil, nil, err
		}
		unpacked, err := l.unpackLayoutResults(body, value, c.Return, to)
		if err != nil {
			return nil, nil, err
		}
		body = unpacked
	}
	fnType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if len(results) > 0 {
		fnType.Results = &ast.FieldList{List: results}
	}
	return stmts, &ast.FuncLit{Type: fnType, Body: &ast.BlockStmt{List: body}}, nil
}

// packLayoutResults evaluates call, a call to a function returning concrete
// type c whose return is declared as view, and returns one Go value holding
// its results, with the view's layout.
func (l *lowerer) packLayoutResults(stmts []ast.Stmt, call ast.Expr, c, view air.TypeID) ([]ast.Stmt, ast.Expr, error) {
	shape := l.returnShapeOf(view)
	switch shape.kind {
	case returnNone:
		return append(stmts, &ast.ExprStmt{X: call}), l.voidValueExpr(), nil
	case returnValue:
		return stmts, call, nil
	}
	valueType, err := l.goLayoutType(c, view)
	if err != nil {
		return nil, nil, err
	}
	ci := l.program.Types[c-1]
	vi := l.program.Types[view-1]
	temps := func(n int) []ast.Expr {
		names := make([]ast.Expr, n)
		for i := range names {
			names[i] = l.ident(l.nextTemp())
		}
		return names
	}
	define := func(lhs []ast.Expr) ast.Stmt {
		return &ast.AssignStmt{Lhs: lhs, Tok: token.DEFINE, Rhs: []ast.Expr{call}}
	}
	tupleLit := func(typ ast.Expr, elems []ast.Expr) ast.Expr {
		elts := make([]ast.Expr, len(elems))
		for i, elem := range elems {
			elts[i] = &ast.KeyValueExpr{Key: l.ident(air.TupleGoFieldName(i)), Value: elem}
		}
		return &ast.CompositeLit{Type: typ, Elts: elts}
	}
	switch shape.kind {
	case returnTuple:
		elems := temps(len(vi.Elems))
		stmts = append(stmts, define(elems))
		return stmts, tupleLit(valueType, elems), nil
	case returnMaybe:
		var elem ast.Expr = l.voidValueExpr()
		elemType := l.voidTypeExpr()
		lhs := temps(1)
		if !shape.voidValue {
			elem = lhs[0]
			if elemType, err = l.goLayoutType(ci.Elem, vi.Elem); err != nil {
				return nil, nil, err
			}
			lhs = append(lhs, l.ident(l.nextTemp()))
		}
		ok := lhs[len(lhs)-1]
		result := l.nextTemp()
		some := &ast.CallExpr{Fun: &ast.IndexExpr{X: l.runtimeQualified("Some"), Index: elemType}, Args: []ast.Expr{elem}}
		stmts = append(stmts,
			define(lhs),
			&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(result)}, Type: valueType}}}},
			&ast.IfStmt{Cond: ok, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(result)}, Tok: token.ASSIGN, Rhs: []ast.Expr{some}}}}},
		)
		return stmts, l.ident(result), nil
	default: // returnResult
		var value ast.Expr
		var lhs []ast.Expr
		switch {
		case shape.spread:
			valueInfo := l.program.Types[vi.Value-1]
			lhs = temps(len(valueInfo.Elems))
			tupleType, err := l.goLayoutType(ci.Value, vi.Value)
			if err != nil {
				return nil, nil, err
			}
			value = tupleLit(tupleType, lhs)
		case !shape.voidValue:
			lhs = temps(1)
			value = lhs[0]
		}
		errTemp := l.ident(l.nextTemp())
		lhs = append(lhs, errTemp)
		var errValue ast.Expr = errTemp
		if !l.isBuiltinErrorType(ci.Error) {
			errValue = &ast.CallExpr{Fun: &ast.SelectorExpr{X: errTemp, Sel: l.ident("Error")}}
		}
		okElts := []ast.Expr{&ast.KeyValueExpr{Key: l.ident("Ok"), Value: l.ident("true")}}
		if value != nil {
			okElts = append([]ast.Expr{&ast.KeyValueExpr{Key: l.ident("Value"), Value: value}}, okElts...)
		}
		result := l.nextTemp()
		assign := func(elts []ast.Expr) *ast.BlockStmt {
			return &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(result)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CompositeLit{Type: valueType, Elts: elts}}}}}
		}
		stmts = append(stmts,
			define(lhs),
			&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(result)}, Type: valueType}}}},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: errTemp, Op: token.NEQ, Y: l.ident("nil")},
				Body: assign([]ast.Expr{&ast.KeyValueExpr{Key: l.ident("Err"), Value: errValue}}),
				Else: assign(okElts),
			},
		)
		return stmts, l.ident(result), nil
	}
}

// unpackLayoutResults appends the statements that return value, of concrete
// type c, as the results of a function whose return is declared as view.
func (l *lowerer) unpackLayoutResults(stmts []ast.Stmt, value ast.Expr, c, view air.TypeID) ([]ast.Stmt, error) {
	shape := l.returnShapeOf(view)
	switch shape.kind {
	case returnNone:
		return l.appendVoidValueEval(stmts, value), nil
	case returnValue:
		return append(stmts, &ast.ReturnStmt{Results: []ast.Expr{value}}), nil
	}
	stmts, value = l.bindTemp(stmts, value)
	vi := l.program.Types[view-1]
	spreadTuple := func(tuple ast.Expr, n int) []ast.Expr {
		elems := make([]ast.Expr, n)
		for i := range elems {
			elems[i] = selectorExpr(tuple, air.TupleGoFieldName(i))
		}
		return elems
	}
	switch shape.kind {
	case returnTuple:
		return append(stmts, &ast.ReturnStmt{Results: spreadTuple(value, len(vi.Elems))}), nil
	case returnMaybe:
		results := []ast.Expr{l.maybeIsSomeExpr(value)}
		if !shape.voidValue {
			results = append([]ast.Expr{l.maybeValueExpr(value)}, results...)
		}
		return append(stmts, &ast.ReturnStmt{Results: results}), nil
	default: // returnResult
		var success []ast.Expr
		switch {
		case shape.spread:
			success = spreadTuple(selectorExpr(value, "Value"), len(l.program.Types[vi.Value-1].Elems))
		case !shape.voidValue:
			success = []ast.Expr{selectorExpr(value, "Value")}
		}
		errValue := l.goErrorValueExpr(l.program.Types[c-1].Error, selectorExpr(value, "Err"))
		failure := append(append([]ast.Expr{}, success...), errValue)
		return append(stmts,
			&ast.IfStmt{Cond: &ast.UnaryExpr{Op: token.NOT, X: selectorExpr(value, "Ok")}, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: failure}}}},
			&ast.ReturnStmt{Results: append(success, l.ident("nil"))},
		), nil
	}
}

// lowerSlotRead lowers expr, adapting a value read out of a generic slot to
// its concrete Go layout.
func (l *lowerer) lowerSlotRead(fn air.Function, expr air.Expr) (loweredExpr, error) {
	lowered, declared, err := l.lowerInDeclaredLayout(fn, expr)
	if err != nil || declared == air.NoType {
		return lowered, err
	}
	stmts, value, err := l.adaptLayout(lowered.stmts, lowered.expr, expr.Type, declared, false)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: stmts, expr: value}, nil
}

// lowerInDeclaredLayout lowers expr without adapting a value read out of a
// generic slot: a field of a generic struct or the return of a generic
// function. When the value's Go layout is that of the slot's declared type
// rather than expr.Type's, it returns that declared type, and air.NoType
// otherwise.
func (l *lowerer) lowerInDeclaredLayout(fn air.Function, expr air.Expr) (loweredExpr, air.TypeID, error) {
	switch expr.Kind {
	case air.ExprGetField:
		return l.lowerGetField(fn, expr)
	case air.ExprCall:
		return l.lowerCall(fn, expr)
	default:
		lowered, err := l.lowerExpr(fn, expr)
		return lowered, air.NoType, err
	}
}

// declaredFieldLayout returns the declared type of field when typ is an
// instance of a generic struct and the field's Go layout follows that
// declaration rather than the field's concrete type, and air.NoType otherwise.
func (l *lowerer) declaredFieldLayout(typ air.TypeInfo, field air.FieldInfo) air.TypeID {
	if typ.Generic == air.NoType || !validTypeID(l.program, typ.Generic) {
		return air.NoType
	}
	declared, ok := structFieldByName(l.program.Types[typ.Generic-1], field.Name)
	if !ok || !l.layoutDiffers(field.Type, declared.Type) {
		return air.NoType
	}
	return declared.Type
}

// packLayoutCall packs the results of call, which returns concrete type c in
// the layout of its declared return type view, into one value with that
// layout.
func (l *lowerer) packLayoutCall(stmts []ast.Stmt, call ast.Expr, writeback []ast.Stmt, c, view air.TypeID) (loweredExpr, error) {
	if len(writeback) > 0 {
		return loweredExpr{}, fmt.Errorf("call through a generic function layout with writeback args is not supported")
	}
	stmts, value, err := l.packLayoutResults(stmts, call, c, view)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: stmts, expr: value}, nil
}
