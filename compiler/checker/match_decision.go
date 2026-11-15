package checker

import (
	"fmt"
	"strconv"
	"strings"
)

// Match compilation (ADR 0083).
//
// A match compiles to a decision tree, following Jules Jacobs' "How to compile
// pattern matching" as implemented by Gleam's compiler. Each arm becomes a row
// of pending checks of the form "match variable v is pattern p". The compiler
// takes a check from the first row, switches on its variable, and splits every
// row between the branches of that switch:
//
//   - A row that tests the variable with the branch's constructor continues in
//     that branch with checks on the constructor's payload in place of the
//     original check.
//   - A row that does not test the variable continues in every branch.
//   - A row whose test cannot hold in a branch is dropped from it.
//
// A row with no checks left matches, so the first such row ends its branch
// with a Run node for its arm. A branch with no rows left is a Fail node: no
// arm matches the values that reach it, so the match is not exhaustive and the
// path to the node describes a missing pattern. An arm that ends no branch can
// never run and is reported as unreachable.
//
// Bool, enum, Maybe, Result, and union variables have a fixed set of
// constructors, so a switch on them has one case per constructor that some row
// names, and a fallback for the rest. Int, Rune, Str, open-enum, and dynamic
// variables have unbounded values, so a switch on them is a binary test: the
// case holds when the value passes the first row's test, and the fallback
// covers every other value. Binary tests let integer ranges overlap literals
// and each other without special cases. Tuples always match, so a tuple check
// is replaced by checks on its elements before any switch.

type DecisionKind int

const (
	// DecisionFail is reached by values no arm matches.
	DecisionFail DecisionKind = iota
	// DecisionRun runs arm Arm.
	DecisionRun
	// DecisionSwitch tests variable Var against Cases in order.
	DecisionSwitch
)

// Decision is a node of a compiled match.
//
// For a switch, the first case whose test holds is taken. When no case holds,
// Fallback is taken; a nil Fallback means the cases cover every value, so the
// last case needs no runtime test. FallbackTests lists the constructors the
// fallback stands for when the variable has a fixed set of constructors.
type Decision struct {
	Kind          DecisionKind
	Arm           int
	Var           int
	Cases         []DecisionCase
	Fallback      *Decision
	FallbackTests []PatternTest
}

type DecisionCase struct {
	Test PatternTest
	Then *Decision
}

type PatternTestKind int

const (
	TestBool PatternTestKind = iota
	TestInt
	TestStr
	TestEnum
	TestSome
	TestNone
	TestOk
	TestErr
	TestUnionMember
	TestForeign
)

// PatternTest is a runtime check on one match variable.
type PatternTest struct {
	Kind PatternTestKind
	// TestBool
	Bool bool
	// TestInt: the value is in Start..End (inclusive). Rune marks a Rune
	// variable for display.
	Start int
	End   int
	Rune  bool
	// TestStr
	Str string
	// TestEnum: the value has Discriminant. Variant names it for display.
	Enum         *Enum
	Variant      int
	Discriminant int
	// TestUnionMember: the value is union member Index.
	Union *Union
	Index int
	// TestForeign: the dynamic type is Foreign.
	Foreign *ForeignType
}

// matchDecisionLimit bounds the size of a compiled match. Decision trees can
// grow exponentially with the number of tuple elements in pathological
// matches; no realistic match comes close.
const matchDecisionLimit = 100_000

type decisionCheck struct {
	v int
	p Pattern
}

type decisionRow struct {
	arm    int
	checks []decisionCheck
}

func (r decisionRow) checkOn(v int) (int, bool) {
	for i, check := range r.checks {
		if check.v == v {
			return i, true
		}
	}
	return 0, false
}

// replace returns the row with check i replaced by checks, which come first
// so a payload is tested before the row moves on to other variables.
func (r decisionRow) replace(i int, checks []decisionCheck) decisionRow {
	out := make([]decisionCheck, 0, len(r.checks)-1+len(checks))
	out = append(out, checks...)
	out = append(out, r.checks[:i]...)
	out = append(out, r.checks[i+1:]...)
	return decisionRow{arm: r.arm, checks: out}
}

type decisionCompiler struct {
	vars     *matchVarTable
	reached  []bool
	missing  bool
	nodes    int
	tooLarge bool
}

func newDecisionCompiler(vars *matchVarTable, arms int) *decisionCompiler {
	return &decisionCompiler{vars: vars, reached: make([]bool, arms)}
}

// compileMatch builds the decision tree for arm patterns that all match
// variable 0.
func (dc *decisionCompiler) compileMatch(patterns []Pattern) *Decision {
	rows := make([]decisionRow, 0, len(patterns))
	for arm, pattern := range patterns {
		if pattern == nil {
			continue
		}
		rows = append(rows, decisionRow{arm: arm, checks: []decisionCheck{{v: 0, p: pattern}}})
	}
	return dc.compile(rows)
}

// normalize drops checks that always hold and replaces tuple checks with
// checks on the elements.
func (dc *decisionCompiler) normalize(row decisionRow) decisionRow {
	out := make([]decisionCheck, 0, len(row.checks))
	pending := append([]decisionCheck(nil), row.checks...)
	for len(pending) > 0 {
		check := pending[0]
		pending = pending[1:]
		switch p := check.p.(type) {
		case *WildcardPattern, *BindingPattern:
			continue
		case *TuplePattern:
			tuple := deref(dc.vars.vars[check.v].Type).(*Tuple)
			elems := make([]decisionCheck, 0, len(p.Elems))
			for i, elem := range p.Elems {
				child := dc.vars.child(check.v, MatchAccessTupleElem, i, nil, tuple.elements[i])
				elems = append(elems, decisionCheck{v: child, p: elem})
			}
			pending = append(elems, pending...)
		default:
			out = append(out, check)
		}
	}
	return decisionRow{arm: row.arm, checks: out}
}

func (dc *decisionCompiler) compile(rows []decisionRow) *Decision {
	dc.nodes++
	if dc.nodes > matchDecisionLimit {
		dc.tooLarge = true
		return &Decision{Kind: DecisionFail}
	}
	normalized := make([]decisionRow, len(rows))
	for i, row := range rows {
		normalized[i] = dc.normalize(row)
	}
	rows = normalized
	if len(rows) == 0 {
		dc.missing = true
		return &Decision{Kind: DecisionFail}
	}
	first := rows[0]
	if len(first.checks) == 0 {
		dc.reached[first.arm] = true
		return &Decision{Kind: DecisionRun, Arm: first.arm}
	}
	pivot := first.checks[0]
	if ctors := dc.constructors(pivot.v); ctors != nil {
		return dc.splitConstructors(pivot.v, ctors, rows)
	}
	return dc.splitBinary(pivot.v, binaryTest(pivot.p), rows)
}

// constructors lists the constructors of a variable with a fixed set of them,
// or nil when its values are unbounded.
func (dc *decisionCompiler) constructors(v int) []PatternTest {
	switch typ := deref(dc.vars.vars[v].Type).(type) {
	case *Maybe:
		return []PatternTest{{Kind: TestSome}, {Kind: TestNone}}
	case *Result:
		return []PatternTest{{Kind: TestOk}, {Kind: TestErr}}
	case *Union:
		ctors := make([]PatternTest, len(typ.Types))
		for i := range typ.Types {
			ctors[i] = PatternTest{Kind: TestUnionMember, Union: typ, Index: i}
		}
		return ctors
	case *Enum:
		if typ.Open {
			return nil
		}
		seen := map[int]bool{}
		var ctors []PatternTest
		for i, value := range typ.Values {
			if seen[value.Value] {
				continue
			}
			seen[value.Value] = true
			ctors = append(ctors, PatternTest{Kind: TestEnum, Enum: typ, Variant: i, Discriminant: value.Value})
		}
		return ctors
	}
	if deref(dc.vars.vars[v].Type) == Bool {
		return []PatternTest{{Kind: TestBool, Bool: true}, {Kind: TestBool, Bool: false}}
	}
	return nil
}

func constructorIndex(ctors []PatternTest, p Pattern) int {
	for i, ctor := range ctors {
		switch p := p.(type) {
		case *BoolPattern:
			if ctor.Bool == p.Value {
				return i
			}
		case *EnumPattern:
			if ctor.Discriminant == p.Discriminant {
				return i
			}
		case *MaybePattern:
			if (ctor.Kind == TestSome) == p.Some {
				return i
			}
		case *ResultPattern:
			if (ctor.Kind == TestOk) == p.Ok {
				return i
			}
		case *UnionPattern:
			if ctor.Index == p.Member {
				return i
			}
		}
	}
	panic(fmt.Sprintf("pattern %T does not name a constructor", p))
}

// payloadChecks returns the checks a constructor pattern makes on its payload.
func (dc *decisionCompiler) payloadChecks(v int, p Pattern) []decisionCheck {
	typ := deref(dc.vars.vars[v].Type)
	switch p := p.(type) {
	case *MaybePattern:
		if !p.Some {
			return nil
		}
		child := dc.vars.child(v, MatchAccessSome, 0, nil, typ.(*Maybe).of)
		return []decisionCheck{{v: child, p: p.Inner}}
	case *ResultPattern:
		result := typ.(*Result)
		access, inner := MatchAccessErr, result.err
		if p.Ok {
			access, inner = MatchAccessOk, result.val
		}
		return []decisionCheck{{v: dc.vars.child(v, access, 0, nil, inner), p: p.Inner}}
	case *UnionPattern:
		member := typ.(*Union).Types[p.Member]
		return []decisionCheck{{v: dc.vars.child(v, MatchAccessUnionMember, p.Member, nil, member), p: p.Inner}}
	case *ForeignTypePattern:
		return []decisionCheck{{v: dc.vars.child(v, MatchAccessForeign, 0, p.Type, p.Type), p: p.Inner}}
	}
	return nil
}

func (dc *decisionCompiler) splitConstructors(v int, ctors []PatternTest, rows []decisionRow) *Decision {
	named := make([]bool, len(ctors))
	for _, row := range rows {
		if i, ok := row.checkOn(v); ok {
			named[constructorIndex(ctors, row.checks[i].p)] = true
		}
	}
	caseRows := make([][]decisionRow, len(ctors))
	var fallbackRows []decisionRow
	for _, row := range rows {
		i, ok := row.checkOn(v)
		if !ok {
			for c := range ctors {
				if named[c] {
					caseRows[c] = append(caseRows[c], row)
				}
			}
			fallbackRows = append(fallbackRows, row)
			continue
		}
		p := row.checks[i].p
		c := constructorIndex(ctors, p)
		caseRows[c] = append(caseRows[c], row.replace(i, dc.payloadChecks(v, p)))
	}
	decision := &Decision{Kind: DecisionSwitch, Var: v}
	var unnamed []PatternTest
	for c, ctor := range ctors {
		if !named[c] {
			unnamed = append(unnamed, ctor)
			continue
		}
		decision.Cases = append(decision.Cases, DecisionCase{Test: ctor, Then: dc.compile(caseRows[c])})
	}
	if len(unnamed) > 0 {
		decision.Fallback = dc.compile(fallbackRows)
		decision.FallbackTests = unnamed
	}
	return decision
}

// binaryTest is the runtime test a pattern on an unbounded variable makes.
func binaryTest(p Pattern) PatternTest {
	switch p := p.(type) {
	case *IntPattern:
		return PatternTest{Kind: TestInt, Start: p.Start, End: p.End, Rune: p.Rune}
	case *StrPattern:
		return PatternTest{Kind: TestStr, Str: p.Value}
	case *EnumPattern:
		return PatternTest{Kind: TestEnum, Enum: p.Enum, Variant: p.Variant, Discriminant: p.Discriminant}
	case *ForeignTypePattern:
		return PatternTest{Kind: TestForeign, Foreign: p.Type}
	}
	panic(fmt.Sprintf("pattern %T has no binary test", p))
}

// relateTest compares a binary test with a pattern on the same variable.
// implied: every value passing the test matches the pattern's own test.
// disjoint: no value passing the test matches the pattern's test.
// covered: every value matching the pattern's test passes the test.
func relateTest(test PatternTest, p Pattern) (implied, disjoint, covered bool) {
	switch p := p.(type) {
	case *IntPattern:
		implied = p.Start <= test.Start && test.End <= p.End
		disjoint = test.End < p.Start || p.End < test.Start
		covered = test.Start <= p.Start && p.End <= test.End
		return
	case *StrPattern:
		same := p.Value == test.Str
		return same, !same, same
	case *EnumPattern:
		same := p.Discriminant == test.Discriminant
		return same, !same, same
	case *ForeignTypePattern:
		same := test.Foreign != nil && p.Type.String() == test.Foreign.String()
		return same, !same, same
	}
	panic(fmt.Sprintf("pattern %T has no binary test", p))
}

func (dc *decisionCompiler) splitBinary(v int, test PatternTest, rows []decisionRow) *Decision {
	var passRows, failRows []decisionRow
	for _, row := range rows {
		i, ok := row.checkOn(v)
		if !ok {
			passRows = append(passRows, row)
			failRows = append(failRows, row)
			continue
		}
		p := row.checks[i].p
		implied, disjoint, covered := relateTest(test, p)
		switch {
		case implied:
			passRows = append(passRows, row.replace(i, dc.payloadChecks(v, p)))
		case !disjoint:
			passRows = append(passRows, row)
		}
		if !covered {
			failRows = append(failRows, row)
		}
	}
	return &Decision{
		Kind:     DecisionSwitch,
		Var:      v,
		Cases:    []DecisionCase{{Test: test, Then: dc.compile(passRows)}},
		Fallback: dc.compile(failRows),
	}
}

// missingPatterns describes the values that reach Fail nodes, as Ard
// patterns, up to limit of them.
func (dc *decisionCompiler) missingPatterns(tree *Decision, limit int) []string {
	constraints := map[int]PatternTest{}
	seen := map[string]bool{}
	var out []string
	var walk func(*Decision)
	constrain := func(v int, test PatternTest, next *Decision) {
		previous, had := constraints[v]
		constraints[v] = test
		walk(next)
		if had {
			constraints[v] = previous
		} else {
			delete(constraints, v)
		}
	}
	walk = func(d *Decision) {
		if d == nil || len(out) >= limit {
			return
		}
		switch d.Kind {
		case DecisionFail:
			missing := dc.printVar(0, constraints)
			if !seen[missing] {
				seen[missing] = true
				out = append(out, missing)
			}
		case DecisionSwitch:
			for _, c := range d.Cases {
				constrain(d.Var, c.Test, c.Then)
			}
			if d.Fallback == nil {
				return
			}
			if len(d.FallbackTests) > 0 {
				for _, test := range d.FallbackTests {
					constrain(d.Var, test, d.Fallback)
				}
				return
			}
			// The fallback of a binary test has no single describing
			// pattern; the variable prints as `_`.
			previous, had := constraints[d.Var]
			delete(constraints, d.Var)
			walk(d.Fallback)
			if had {
				constraints[d.Var] = previous
			}
		}
	}
	walk(tree)
	return out
}

func (dc *decisionCompiler) printVar(v int, constraints map[int]PatternTest) string {
	child := func(access MatchVarAccess, index int, foreign *ForeignType) string {
		if id, ok := dc.vars.lookup(v, access, index, foreign); ok {
			return dc.printVar(id, constraints)
		}
		return "_"
	}
	if tuple, ok := deref(dc.vars.vars[v].Type).(*Tuple); ok {
		parts := make([]string, len(tuple.elements))
		for i := range tuple.elements {
			parts[i] = child(MatchAccessTupleElem, i, nil)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	}
	test, ok := constraints[v]
	if !ok {
		return "_"
	}
	switch test.Kind {
	case TestBool:
		return strconv.FormatBool(test.Bool)
	case TestInt:
		return patternString(&IntPattern{Start: test.Start, End: test.End, Rune: test.Rune})
	case TestStr:
		return strconv.Quote(test.Str)
	case TestEnum:
		return fmt.Sprintf("%s::%s", test.Enum.Name, test.Enum.Values[test.Variant].Name)
	case TestSome:
		return "some(" + child(MatchAccessSome, 0, nil) + ")"
	case TestNone:
		return "none"
	case TestOk:
		return "ok(" + child(MatchAccessOk, 0, nil) + ")"
	case TestErr:
		return "err(" + child(MatchAccessErr, 0, nil) + ")"
	case TestUnionMember:
		name := unionMemberDisplayName(test.Union, test.Index)
		payload := child(MatchAccessUnionMember, test.Index, nil)
		if v == 0 && payload == "_" {
			// A whole-subject union member is spelled `Type =>`.
			return name
		}
		return name + "(" + payload + ")"
	case TestForeign:
		return test.Foreign.String() + "(" + child(MatchAccessForeign, 0, test.Foreign) + ")"
	}
	return "_"
}

// patternSubsumes reports whether every value matched by q is matched by p.
func patternSubsumes(p, q Pattern) bool {
	switch p := p.(type) {
	case *WildcardPattern, *BindingPattern:
		return true
	case *BoolPattern:
		other, ok := q.(*BoolPattern)
		return ok && other.Value == p.Value
	case *IntPattern:
		other, ok := q.(*IntPattern)
		return ok && p.Start <= other.Start && other.End <= p.End
	case *StrPattern:
		other, ok := q.(*StrPattern)
		return ok && other.Value == p.Value
	case *EnumPattern:
		other, ok := q.(*EnumPattern)
		return ok && other.Discriminant == p.Discriminant
	case *TuplePattern:
		other, ok := q.(*TuplePattern)
		if !ok || len(other.Elems) != len(p.Elems) {
			return false
		}
		for i := range p.Elems {
			if !patternSubsumes(p.Elems[i], other.Elems[i]) {
				return false
			}
		}
		return true
	case *ResultPattern:
		other, ok := q.(*ResultPattern)
		return ok && other.Ok == p.Ok && patternSubsumes(p.Inner, other.Inner)
	case *MaybePattern:
		other, ok := q.(*MaybePattern)
		if !ok || other.Some != p.Some {
			return false
		}
		return !p.Some || patternSubsumes(p.Inner, other.Inner)
	case *UnionPattern:
		other, ok := q.(*UnionPattern)
		return ok && other.Member == p.Member && patternSubsumes(p.Inner, other.Inner)
	case *ForeignTypePattern:
		other, ok := q.(*ForeignTypePattern)
		return ok && other.Type.String() == p.Type.String() && patternSubsumes(p.Inner, other.Inner)
	}
	return false
}
