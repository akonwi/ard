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

func placeOperand(lowered loweredExpr) operand {
	return operand{stmts: lowered.stmts, expr: lowered.expr, place: true}
}

// sequenceOperands returns the statements that evaluate ops in source order
// and leaves in each ops[i].expr an expression to use in its place.
//
// When no operand after the first emits statements, the operands' own
// statements are returned unchanged and every expression stays inline: Go
// evaluates the calls in a composite expression left to right, so nothing is
// added. Otherwise each operand to the left of the last one that emits
// statements is evaluated into a temporary right after its own statements,
// unless its expression cannot observe or cause side effects.
func (l *lowerer) sequenceOperands(ops []operand) ([]ast.Stmt, error) {
	last := -1
	for i := range ops {
		if len(ops[i].stmts) > 0 {
			last = i
		}
	}
	if last <= 0 {
		if last == 0 {
			return ops[0].stmts, nil
		}
		return nil, nil
	}
	size := 0
	for i := range ops {
		size += len(ops[i].stmts)
	}
	stmts := make([]ast.Stmt, 0, size+last)
	for i := range ops {
		stmts = append(stmts, ops[i].stmts...)
		if i >= last {
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
func (l *lowerer) lowerValueOperand(fn air.Function, expr air.Expr, expected air.TypeID) (operand, error) {
	var lowered loweredExpr
	var err error
	typ := expected
	if expected != air.NoType {
		lowered, err = l.lowerExprWithExpectedType(fn, expr, expected)
	} else {
		lowered, err = l.lowerExpr(fn, expr)
		typ = expr.Type
	}
	if err != nil {
		return operand{}, err
	}
	if l.isVoidType(expected) || isVoidExpr(lowered.expr) {
		lowered = l.materializeVoidValue(lowered)
	}
	return valueOperand(lowered, typ), nil
}

// sequenceOperandPair is sequenceOperands for two value operands lowered
// separately. It updates first.expr and second.expr in place and returns the
// statements that evaluate both in order.
func (l *lowerer) sequenceOperandPair(first *loweredExpr, firstType air.TypeID, second *loweredExpr, secondType air.TypeID) ([]ast.Stmt, error) {
	if len(second.stmts) == 0 {
		return first.stmts, nil
	}
	ops := [2]operand{valueOperand(*first, firstType), valueOperand(*second, secondType)}
	stmts, err := l.sequenceOperands(ops[:])
	if err != nil {
		return nil, err
	}
	first.expr, second.expr = ops[0].expr, ops[1].expr
	return stmts, nil
}

// sequenceOperandTriple is sequenceOperandPair for three value operands.
func (l *lowerer) sequenceOperandTriple(first *loweredExpr, firstType air.TypeID, second *loweredExpr, secondType air.TypeID, third *loweredExpr, thirdType air.TypeID) ([]ast.Stmt, error) {
	if len(second.stmts) == 0 && len(third.stmts) == 0 {
		return first.stmts, nil
	}
	ops := [3]operand{valueOperand(*first, firstType), valueOperand(*second, secondType), valueOperand(*third, thirdType)}
	stmts, err := l.sequenceOperands(ops[:])
	if err != nil {
		return nil, err
	}
	first.expr, second.expr, third.expr = ops[0].expr, ops[1].expr, ops[2].expr
	return stmts, nil
}
