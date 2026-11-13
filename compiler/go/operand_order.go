package gotarget

import (
	"go/ast"
	"go/token"

	"github.com/akonwi/ard/air"
)

// operand is one lowered operand of a composite expression: an element, a
// field value, a call argument, a receiver, or an assignment target.
//
// Operands are evaluated in source order (ADR 0078). Lowering an operand can
// emit setup statements; those run before the Go expression that combines the
// operands, so every operand to the left of one that emits statements must be
// evaluated before those statements. sequenceOperands does that.
type operand struct {
	stmts []ast.Stmt
	expr  ast.Expr
	// typ is the AIR type expr was lowered for. A materialized value is
	// declared with its Go type so an untyped constant expression keeps the
	// type its context gives it. air.NoType declares the temporary with `:=`;
	// use it only where the expression's own Go type is the one required.
	typ air.TypeID
	// place marks an operand that designates storage to assign or mutate
	// rather than a value. A place is materialized by address so later
	// writes still reach the original storage.
	place bool
	// source is the AIR expression the operand was lowered from, when expr is
	// that lowering without adaptation. For a place it is the base the place
	// lies in: the place is the base's own storage, a field of it, or, when
	// the base is a pointer, inside its pointee. nil when unknown; an operand
	// without a source is treated as a read a call could change.
	source *air.Expr
}

// operandBuffer holds the operands of a typical composite expression so that
// lowering it does not allocate an operand list.
type operandBuffer [8]operand

// operands returns n zeroed operands, backed by buf when they fit.
func (buf *operandBuffer) operands(n int) []operand {
	if n <= len(buf) {
		return buf[:n]
	}
	return make([]operand, n)
}

func valueOperand(lowered loweredExpr, typ air.TypeID) operand {
	return operand{stmts: lowered.stmts, expr: lowered.expr, typ: typ}
}

// sourcedOperand is valueOperand for an expression lowered from source.
func sourcedOperand(lowered loweredExpr, typ air.TypeID, source *air.Expr) operand {
	return operand{stmts: lowered.stmts, expr: lowered.expr, typ: typ, source: source}
}

// placeOperand is a place lying in base (see operand.source).
func placeOperand(lowered loweredExpr, base *air.Expr) operand {
	return operand{stmts: lowered.stmts, expr: lowered.expr, place: true, source: base}
}

// sequenceOperands returns the statements that evaluate ops in source order
// and leaves in each ops[i].expr an expression to use in its place.
//
// When an operand emits setup statements, each operand to its left is
// evaluated into a temporary right after its own statements, unless its
// expression cannot observe or cause side effects. Go orders only the calls
// in an expression, so each operand at or after the last one with statements
// whose value is a read that a later operand's call could change is also read
// in order (readOperandsInOrder). Otherwise every expression stays inline and
// the operands' own statements are returned unchanged.
func (l *lowerer) sequenceOperands(ops []operand) ([]ast.Stmt, error) {
	last := -1
	for i := range ops {
		if len(ops[i].stmts) > 0 {
			last = i
		}
	}
	placeBound := l.readOperandsInOrder(ops, last)
	if last <= 0 && placeBound < 0 {
		if last == 0 {
			return ops[0].stmts, nil
		}
		return nil, nil
	}
	size := 0
	for i := range ops {
		size += len(ops[i].stmts)
	}
	stmts := make([]ast.Stmt, 0, size+max(last, placeBound))
	for i := range ops {
		stmts = append(stmts, ops[i].stmts...)
		if i >= last && (i >= placeBound || !ops[i].place || !l.operandMayChange(&ops[i])) {
			continue
		}
		var err error
		stmts, err = l.materializeOperand(stmts, &ops[i])
		if err != nil {
			return nil, err
		}
	}
	return stmts, nil
}

// readOperandsInOrder finds the operands at or after last whose value a call
// in a later operand could change. It wraps each such value in readInOrder,
// which adds no statements, so contexts that cannot take statements, such as
// a while condition, still lower. A place cannot be wrapped and reused, so it
// is left to sequenceOperands to fix with a statement; every place belongs to
// an assignment that emits statements of its own. It returns the index of the
// last operand with a call when a place needs fixing, and -1 otherwise.
func (l *lowerer) readOperandsInOrder(ops []operand, last int) int {
	first := -1
	for i := max(last, 0); i < len(ops)-1; i++ {
		if l.operandMayChange(&ops[i]) {
			first = i
			break
		}
	}
	if first < 0 {
		return -1
	}
	bound := -1
	for j := len(ops) - 1; j > first; j-- {
		if l.exprContainsCall(ops[j].expr) {
			bound = j
			break
		}
	}
	placeBound := -1
	for i := first; i < bound; i++ {
		if i > first && !l.operandMayChange(&ops[i]) {
			continue
		}
		if ops[i].place {
			placeBound = bound
			continue
		}
		l.readInOrder(&ops[i])
	}
	return placeBound
}

// readInOrder wraps a value operand in a call to the runtime's identity
// function, Read. Go evaluates a call's arguments before the call, and the
// calls of an expression in source order, so the read happens before any call
// to its right.
func (l *lowerer) readInOrder(op *operand) {
	op.expr = &ast.CallExpr{Fun: l.runtimeQualified("Read"), Args: []ast.Expr{op.expr}}
}

func (l *lowerer) materializeOperand(stmts []ast.Stmt, op *operand) ([]ast.Stmt, error) {
	if op.place {
		return l.materializePlace(stmts, op), nil
	}
	if operandIsInert(op.expr) {
		return stmts, nil
	}
	temp := l.nextTemp()
	if op.typ == air.NoType || l.isVoidType(op.typ) {
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.DEFINE, Rhs: []ast.Expr{op.expr}})
		op.expr = l.ident(temp)
		return stmts, nil
	}
	typ, err := l.goType(op.typ)
	if err != nil {
		return nil, err
	}
	stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
		Names:  []*ast.Ident{l.ident(temp)},
		Type:   typ,
		Values: []ast.Expr{op.expr},
	}}}})
	op.expr = l.ident(temp)
	return stmts, nil
}

// materializePlace fixes the storage a place designates. A variable already
// names fixed storage; a dereference fixes its pointer; any other place is
// fixed by taking its address.
func (l *lowerer) materializePlace(stmts []ast.Stmt, op *operand) []ast.Stmt {
	switch place := op.expr.(type) {
	case *ast.Ident:
		return stmts
	case *ast.StarExpr:
		temp := l.nextTemp()
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.DEFINE, Rhs: []ast.Expr{place.X}})
		op.expr = &ast.StarExpr{X: l.ident(temp)}
		return stmts
	}
	temp := l.nextTemp()
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: op.expr}}})
	op.expr = &ast.StarExpr{X: l.ident(temp)}
	return stmts
}

// operandIsInert reports whether evaluating expr later gives the same value
// and has no effect: a constant, a function literal (it captures variables,
// not their values), or a Void value.
func operandIsInert(expr ast.Expr) bool {
	switch expr := expr.(type) {
	case *ast.BasicLit, *ast.FuncLit:
		return true
	case *ast.Ident:
		return expr.Name == "nil" || expr.Name == "true" || expr.Name == "false"
	case *ast.ParenExpr:
		return operandIsInert(expr.X)
	case *ast.UnaryExpr:
		return expr.Op != token.AND && expr.Op != token.ARROW && operandIsInert(expr.X)
	case *ast.CompositeLit:
		return isVoidExpr(expr)
	}
	return false
}

func operandExprs(ops []operand) []ast.Expr {
	exprs := make([]ast.Expr, len(ops))
	for i := range ops {
		exprs[i] = ops[i].expr
	}
	return exprs
}

// lowerValueOperand lowers expr as an operand in value position. With an
// expected type it is lowered to that type; a Void value is evaluated as a
// statement and replaced by the Void literal.
func (l *lowerer) lowerValueOperand(fn air.Function, expr *air.Expr, expected air.TypeID) (operand, error) {
	var lowered loweredExpr
	var err error
	typ := expected
	if expected != air.NoType {
		lowered, err = l.lowerExprWithExpectedType(fn, *expr, expected)
	} else {
		lowered, err = l.lowerExpr(fn, *expr)
		typ = expr.Type
	}
	if err != nil {
		return operand{}, err
	}
	if l.isVoidType(expected) || isVoidExpr(lowered.expr) {
		lowered = l.materializeVoidValue(lowered)
	}
	return sourcedOperand(lowered, typ, expr), nil
}

// lazyOperandBody returns the body of the branch that evaluates an operand
// only when it is needed: the operand's setup statements followed by use, the
// statement that uses its expression. The right side of `and` and `or`, and
// the argument of `or` and `expect` on Maybe and Result (ADR 0079), are such
// operands. The caller evaluates every earlier operand before the branch, so
// they are not passed through sequenceOperands.
func lazyOperandBody(lazy loweredExpr, use ast.Stmt) []ast.Stmt {
	body := make([]ast.Stmt, 0, len(lazy.stmts)+1)
	body = append(body, lazy.stmts...)
	return append(body, use)
}

// evaluateBranchCallee evaluates callee, the lowered callback argument of a
// built-in method that calls it only inside a branch, into a temporary
// appended to stmts. The callback is an ordinary argument (ADR 0079): the
// expression that produces it is evaluated exactly once, whether or not the
// branch runs. The caller appends it after every earlier operand.
//
// A callback whose evaluation has no effect, such as a function literal or a
// named function, stays inline, which keeps its generated Go unchanged. A
// variable or field read can stay inline too, unlike a read to the left of a
// call (ADR 0078): it is read in the branch, after the target, and no Ard
// code runs in between, so it sees what source order gives it. The temporary
// takes the callee's own Go type because the branch calls it as it is.
func (l *lowerer) evaluateBranchCallee(stmts []ast.Stmt, callee *loweredExpr, arg air.Expr) []ast.Stmt {
	if evaluationHasNoEffect(arg) {
		return stmts
	}
	if _, ok := callee.expr.(*ast.Ident); ok || operandIsInert(callee.expr) {
		return stmts
	}
	temp := l.nextTemp()
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.DEFINE, Rhs: []ast.Expr{callee.expr}})
	callee.expr = l.ident(temp)
	return stmts
}

// evaluationHasNoEffect reports whether evaluating expr only reads a value or
// builds a function: a function literal, a reference to a function or a Go
// value, or a variable or field read.
func evaluationHasNoEffect(expr air.Expr) bool {
	switch expr.Kind {
	case air.ExprMakeClosure, air.ExprFunctionRef, air.ExprForeignValue, air.ExprLoadLocal, air.ExprLoadGlobal:
		return true
	case air.ExprGetField, air.ExprDiscardingFunctionCoercion:
		return expr.Target != nil && evaluationHasNoEffect(*expr.Target)
	}
	return false
}

// sequenceOperandPair is sequenceOperands for two value operands lowered
// separately, the first from firstSource. It updates first.expr and
// second.expr in place and returns the statements that evaluate both in order.
func (l *lowerer) sequenceOperandPair(first *loweredExpr, firstSource *air.Expr, second *loweredExpr, secondType air.TypeID) ([]ast.Stmt, error) {
	ops := [2]operand{sourcedOperand(*first, firstSource.Type, firstSource), valueOperand(*second, secondType)}
	if len(second.stmts) == 0 && !l.readBeforeCall(&ops[0], second.expr) {
		return first.stmts, nil
	}
	stmts, err := l.sequenceOperands(ops[:])
	if err != nil {
		return nil, err
	}
	first.expr, second.expr = ops[0].expr, ops[1].expr
	return stmts, nil
}

// sequenceOperandTriple is sequenceOperandPair for three value operands, the
// first two lowered from firstSource and secondSource.
func (l *lowerer) sequenceOperandTriple(first *loweredExpr, firstSource *air.Expr, second *loweredExpr, secondSource *air.Expr, third *loweredExpr, thirdType air.TypeID) ([]ast.Stmt, error) {
	ops := [3]operand{
		sourcedOperand(*first, firstSource.Type, firstSource),
		sourcedOperand(*second, secondSource.Type, secondSource),
		valueOperand(*third, thirdType),
	}
	if len(second.stmts) == 0 && len(third.stmts) == 0 &&
		!l.readBeforeCall(&ops[0], second.expr, third.expr) && !l.readBeforeCall(&ops[1], third.expr) {
		return first.stmts, nil
	}
	stmts, err := l.sequenceOperands(ops[:])
	if err != nil {
		return nil, err
	}
	first.expr, second.expr, third.expr = ops[0].expr, ops[1].expr, ops[2].expr
	return stmts, nil
}

// readBeforeCall reports whether op is a read that a call in one of later
// could change.
func (l *lowerer) readBeforeCall(op *operand, later ...ast.Expr) bool {
	if !l.operandMayChange(op) {
		return false
	}
	for _, expr := range later {
		if l.exprContainsCall(expr) {
			return true
		}
	}
	return false
}
