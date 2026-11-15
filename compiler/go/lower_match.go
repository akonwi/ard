package gotarget

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/akonwi/ard/air"
)

// matchLowering lowers one ExprMatch (ADR 0083). Each subject is evaluated
// once into a temporary, then the decision tree becomes nested if
// statements. Match variables are pure reads of the subjects (field reads,
// Maybe values, type assertions after a passing test), so they are rebuilt
// wherever they are needed instead of being stored.
//
// An arm that ends exactly one branch of the tree is emitted inline at that
// branch. An arm that ends several branches (typically a trailing catch-all)
// is emitted once after the tree: each of its branches records the arm in a
// selector, and the selector picks the arm's body.
type matchLowering struct {
	l            *lowerer
	fn           air.Function
	payload      *air.MatchExprPayload
	subjects     []string
	resultType   air.TypeID
	assignTarget ast.Expr
	runs         []int
	selector     string
}

func (l *lowerer) lowerMatch(fn air.Function, expr air.Expr) (loweredExpr, error) {
	payload := expr.MatchPayload()
	if payload == nil {
		return loweredExpr{}, fmt.Errorf("match is missing its payload")
	}
	m := &matchLowering{l: l, fn: fn, payload: payload, resultType: expr.Type, runs: make([]int, len(payload.Arms))}
	air.WalkDecision(&payload.Tree, func(d *air.Decision) {
		if d.Kind == air.DecisionRun {
			m.runs[d.Arm]++
		}
	})

	used := m.usedSubjects()
	stmts := []ast.Stmt{}
	m.subjects = make([]string, len(expr.Args))
	for i, arg := range expr.Args {
		subject, err := l.lowerExpr(fn, arg)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, subject.stmts...)
		if !used[i] {
			if !isPureIdent(subject.expr) {
				stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{subject.expr}})
			}
			continue
		}
		if ident, ok := subject.expr.(*ast.Ident); ok && arg.Kind == air.ExprLoadLocal && len(expr.Args) == 1 {
			// A lone local subject needs no copy: nothing runs between the
			// tests and the bindings that could change it. With several
			// subjects, a later one may assign the local, so each is copied
			// when it is evaluated.
			m.subjects[i] = ident.Name
			continue
		}
		temp := l.nextTemp()
		decls, err := l.declareTemp(arg.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{subject.expr}})
		m.subjects[i] = temp
	}

	resultExpr := l.ident("nil")
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		m.assignTarget = l.ident(temp)
		resultExpr = l.ident(temp)
	}

	joined := false
	for _, count := range m.runs {
		if count > 1 {
			joined = true
		}
	}
	if joined {
		m.selector = l.nextTemp()
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(m.selector)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "-1"}}})
	}

	tree, err := m.decision(&payload.Tree)
	if err != nil {
		return loweredExpr{}, err
	}
	// Arm bindings are declared where the tree runs the arm. A tree that is
	// not a single if statement runs an arm directly, so it gets its own block
	// to keep those declarations out of the enclosing scope.
	if len(tree) == 1 {
		if _, ok := tree[0].(*ast.IfStmt); ok {
			stmts = append(stmts, tree...)
		} else {
			stmts = append(stmts, &ast.BlockStmt{List: tree})
		}
	} else if len(tree) > 0 {
		stmts = append(stmts, &ast.BlockStmt{List: tree})
	}

	if joined {
		var chain []ast.Stmt
		for arm := len(payload.Arms) - 1; arm >= 0; arm-- {
			if m.runs[arm] <= 1 {
				continue
			}
			body, err := m.arm(arm)
			if err != nil {
				return loweredExpr{}, err
			}
			chain = []ast.Stmt{&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: l.ident(m.selector), Op: token.EQL, Y: &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(arm)}},
				Body: &ast.BlockStmt{List: body},
				Else: elseOf(chain),
			}}
		}
		stmts = append(stmts, chain...)
	}
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func isPureIdent(expr ast.Expr) bool {
	_, ok := expr.(*ast.Ident)
	return ok
}

// usedSubjects reports which subjects the tree tests or an arm binds.
func (m *matchLowering) usedSubjects() []bool {
	usedVars := make([]bool, len(m.payload.Vars))
	air.WalkDecision(&m.payload.Tree, func(d *air.Decision) {
		if d.Kind == air.DecisionSwitch {
			usedVars[d.Var] = true
		}
	})
	for arm, body := range m.payload.Arms {
		if m.runs[arm] == 0 {
			continue
		}
		for _, binding := range body.Bindings {
			usedVars[binding.Var] = true
		}
	}
	used := make([]bool, len(m.payload.Vars))
	for i := len(m.payload.Vars) - 1; i >= 0; i-- {
		if !usedVars[i] {
			continue
		}
		v := m.payload.Vars[i]
		if v.Source == air.MatchVarSubject {
			if v.Index < len(used) {
				used[v.Index] = true
			}
			continue
		}
		usedVars[v.Parent] = true
	}
	return used
}

// elseOf turns lowered statements into an if statement's else branch,
// flattening a lone if into `else if`.
func elseOf(stmts []ast.Stmt) ast.Stmt {
	if len(stmts) == 0 {
		return nil
	}
	if len(stmts) == 1 {
		if ifStmt, ok := stmts[0].(*ast.IfStmt); ok {
			return ifStmt
		}
	}
	return &ast.BlockStmt{List: stmts}
}

func (m *matchLowering) decision(d *air.Decision) ([]ast.Stmt, error) {
	switch d.Kind {
	case air.DecisionFail:
		return []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{
			Fun:  m.l.ident("panic"),
			Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("unreachable: no match arm applies")}},
		}}}, nil
	case air.DecisionRun:
		if m.runs[d.Arm] > 1 {
			return []ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{m.l.ident(m.selector)},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(d.Arm)}},
			}}, nil
		}
		return m.arm(d.Arm)
	case air.DecisionSwitch:
	default:
		return nil, fmt.Errorf("unknown match decision kind %d", d.Kind)
	}

	cases := d.Cases
	var otherwise []ast.Stmt
	if d.Fallback != nil {
		fallback, err := m.decision(d.Fallback)
		if err != nil {
			return nil, err
		}
		otherwise = fallback
	} else {
		// The cases cover every value, so the last one needs no test.
		last, err := m.decision(&cases[len(cases)-1].Then)
		if err != nil {
			return nil, err
		}
		otherwise = last
		cases = cases[:len(cases)-1]
	}
	for i := len(cases) - 1; i >= 0; i-- {
		cond, init, err := m.test(d.Var, cases[i].Test)
		if err != nil {
			return nil, err
		}
		body, err := m.decision(&cases[i].Then)
		if err != nil {
			return nil, err
		}
		otherwise = []ast.Stmt{&ast.IfStmt{Init: init, Cond: cond, Body: &ast.BlockStmt{List: body}, Else: elseOf(otherwise)}}
	}
	return otherwise, nil
}

// arm binds an arm's pattern names and runs its body. All names bind in one
// short declaration so every right-hand side reads the subjects before any
// new name can shadow them.
func (m *matchLowering) arm(index int) ([]ast.Stmt, error) {
	arm := m.payload.Arms[index]
	stmts := []ast.Stmt{}
	if len(arm.Bindings) > 0 {
		names := make([]ast.Expr, len(arm.Bindings))
		values := make([]ast.Expr, len(arm.Bindings))
		discards := make([]ast.Expr, len(arm.Bindings))
		for i, binding := range arm.Bindings {
			value, err := m.varExpr(binding.Var)
			if err != nil {
				return nil, err
			}
			name := m.l.localName(m.fn, binding.Local)
			m.l.declaredLocals[binding.Local] = true
			names[i] = m.l.ident(name)
			values[i] = value
			discards[i] = m.l.ident("_")
		}
		stmts = append(stmts, &ast.AssignStmt{Lhs: names, Tok: token.DEFINE, Rhs: values})
		uses := make([]ast.Expr, len(arm.Bindings))
		for i, binding := range arm.Bindings {
			uses[i] = m.l.ident(m.l.localName(m.fn, binding.Local))
		}
		stmts = append(stmts, &ast.AssignStmt{Lhs: discards, Tok: token.ASSIGN, Rhs: uses})
	}
	body, err := m.l.lowerValueBlock(m.fn, arm.Body, m.resultType, m.assignTarget)
	if err != nil {
		return nil, err
	}
	return append(stmts, body...), nil
}

// varExpr builds the Go expression that reads a match variable.
func (m *matchLowering) varExpr(id air.MatchVarID) (ast.Expr, error) {
	if id < 0 || int(id) >= len(m.payload.Vars) {
		return nil, fmt.Errorf("invalid match variable %d", id)
	}
	v := m.payload.Vars[id]
	if v.Source == air.MatchVarSubject {
		if v.Index < 0 || v.Index >= len(m.subjects) || m.subjects[v.Index] == "" {
			return nil, fmt.Errorf("match variable %d reads an unevaluated subject", id)
		}
		return m.l.ident(m.subjects[v.Index]), nil
	}
	parent, err := m.varExpr(v.Parent)
	if err != nil {
		return nil, err
	}
	switch v.Source {
	case air.MatchVarTupleElem:
		return selectorExpr(parent, air.TupleGoFieldName(v.Index)), nil
	case air.MatchVarMaybeSome:
		return m.l.maybeValueExpr(parent), nil
	case air.MatchVarResultOk:
		return selectorExpr(parent, "Value"), nil
	case air.MatchVarResultErr:
		return selectorExpr(parent, "Err"), nil
	case air.MatchVarUnionMember:
		union, err := m.unionInfo(v.Parent)
		if err != nil {
			return nil, err
		}
		for _, member := range union.Members {
			if member.Tag == v.Tag {
				return selectorExpr(parent, unionMemberFieldName(union, member)), nil
			}
		}
		return nil, fmt.Errorf("invalid union member tag %d", v.Tag)
	case air.MatchVarForeignType:
		typ, err := m.l.goType(v.Type)
		if err != nil {
			return nil, err
		}
		return &ast.TypeAssertExpr{X: parent, Type: typ}, nil
	}
	return nil, fmt.Errorf("match variable %d has unknown source %d", id, v.Source)
}

func (m *matchLowering) unionInfo(id air.MatchVarID) (air.TypeInfo, error) {
	typeID := m.payload.Vars[id].Type
	if !validTypeID(m.l.program, typeID) {
		return air.TypeInfo{}, fmt.Errorf("invalid union type %d", typeID)
	}
	return m.l.program.Types[typeID-1], nil
}

// test builds the condition for one decision case. A dynamic type test needs
// an init statement: `if _, ok := v.(T); ok`.
func (m *matchLowering) test(id air.MatchVarID, test air.MatchTest) (ast.Expr, ast.Stmt, error) {
	value, err := m.varExpr(id)
	if err != nil {
		return nil, nil, err
	}
	intLit := func(n int) ast.Expr {
		return &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(n)}
	}
	switch test.Kind {
	case air.MatchTestBool:
		if test.Bool {
			return value, nil, nil
		}
		return &ast.UnaryExpr{Op: token.NOT, X: value}, nil, nil
	case air.MatchTestIntRange:
		if test.Start == test.End {
			return &ast.BinaryExpr{X: value, Op: token.EQL, Y: intLit(test.Start)}, nil, nil
		}
		upper, err := m.varExpr(id)
		if err != nil {
			return nil, nil, err
		}
		return &ast.BinaryExpr{
			X:  &ast.BinaryExpr{X: value, Op: token.GEQ, Y: intLit(test.Start)},
			Op: token.LAND,
			Y:  &ast.BinaryExpr{X: upper, Op: token.LEQ, Y: intLit(test.End)},
		}, nil, nil
	case air.MatchTestStr:
		return &ast.BinaryExpr{X: value, Op: token.EQL, Y: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(test.Str)}}, nil, nil
	case air.MatchTestEnum:
		return &ast.BinaryExpr{X: value, Op: token.EQL, Y: intLit(test.Discriminant)}, nil, nil
	case air.MatchTestMaybeSome:
		return m.l.maybeIsSomeExpr(value), nil, nil
	case air.MatchTestMaybeNone:
		return m.l.maybeIsNoneExpr(value), nil, nil
	case air.MatchTestResultOk:
		return selectorExpr(value, "Ok"), nil, nil
	case air.MatchTestResultErr:
		return &ast.UnaryExpr{Op: token.NOT, X: selectorExpr(value, "Ok")}, nil, nil
	case air.MatchTestUnionTag:
		union, err := m.unionInfo(id)
		if err != nil {
			return nil, nil, err
		}
		return &ast.BinaryExpr{X: selectorExpr(value, unionTagFieldName(union)), Op: token.EQL, Y: &ast.BasicLit{Kind: token.INT, Value: strconv.FormatUint(uint64(test.Tag), 10)}}, nil, nil
	case air.MatchTestForeignType:
		typ, err := m.l.goType(test.Type)
		if err != nil {
			return nil, nil, err
		}
		ok := m.l.nextTemp()
		init := &ast.AssignStmt{
			Lhs: []ast.Expr{m.l.ident("_"), m.l.ident(ok)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.TypeAssertExpr{X: value, Type: typ}},
		}
		return m.l.ident(ok), init, nil
	}
	return nil, nil, fmt.Errorf("unknown match test kind %d", test.Kind)
}
