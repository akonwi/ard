package air

import "fmt"

// Pattern matching (ADR 0083). The checker compiles every match with a
// subject into a decision tree. AIR carries that tree: ExprMatch evaluates its
// subjects once, runs the tree's tests on match variables (the subjects and
// parts read from them), and runs one arm.

// MatchVarID indexes MatchExprPayload.Vars.
type MatchVarID int

// MatchVarSource says how a match variable is read.
type MatchVarSource uint8

const (
	// MatchVarSubject is the ExprMatch argument at Index, evaluated once.
	MatchVarSubject MatchVarSource = iota
	// MatchVarTupleElem is element Index of the tuple Parent.
	MatchVarTupleElem
	// MatchVarMaybeSome is the present value of the Maybe Parent. It is read
	// only after a MatchTestMaybeSome test on Parent holds.
	MatchVarMaybeSome
	// MatchVarResultOk is the value of the Result Parent after a
	// MatchTestResultOk test holds.
	MatchVarResultOk
	// MatchVarResultErr is the error of the Result Parent after a
	// MatchTestResultErr test holds.
	MatchVarResultErr
	// MatchVarUnionMember is the payload of the union Parent's member with
	// Tag after a MatchTestUnionTag test holds.
	MatchVarUnionMember
	// MatchVarForeignType is the dynamic value Parent narrowed to the
	// variable's Type after a MatchTestForeignType test holds.
	MatchVarForeignType
)

// MatchVar is a value an ExprMatch tests or binds.
type MatchVar struct {
	Type   TypeID
	Source MatchVarSource
	Parent MatchVarID
	Index  int
	Tag    uint32
}

// MatchTestKind is the kind of runtime check a decision switch performs.
type MatchTestKind uint8

const (
	MatchTestBool MatchTestKind = iota
	// MatchTestIntRange holds when Start <= value <= End. Int, Rune, and Byte
	// variables use it; a literal has Start == End.
	MatchTestIntRange
	MatchTestStr
	// MatchTestEnum holds when the enum value has Discriminant.
	MatchTestEnum
	MatchTestMaybeSome
	MatchTestMaybeNone
	MatchTestResultOk
	MatchTestResultErr
	// MatchTestUnionTag holds when the union value is the member with Tag.
	MatchTestUnionTag
	// MatchTestForeignType holds when the dynamic value's type is Type.
	MatchTestForeignType
)

type MatchTest struct {
	Kind         MatchTestKind
	Bool         bool
	Start        int
	End          int
	Str          string
	Discriminant int
	Tag          uint32
	Type         TypeID
}

type DecisionKind uint8

const (
	// DecisionFail is reached by no value. The checker rejects matches whose
	// trees can reach it, so backends may treat it as unreachable.
	DecisionFail DecisionKind = iota
	// DecisionRun runs arm Arm.
	DecisionRun
	// DecisionSwitch tests Var against Cases in order and takes the first
	// case whose test holds, or Fallback when none does. A nil Fallback means
	// the cases cover every value, so the last case needs no runtime test.
	DecisionSwitch
)

type Decision struct {
	Kind     DecisionKind
	Arm      int
	Var      MatchVarID
	Cases    []DecisionCase
	Fallback *Decision
}

type DecisionCase struct {
	Test MatchTest
	Then Decision
}

// MatchArm is one arm in source order. Bindings bind the arm's pattern names
// to match variables before Body runs. An arm that no Run node names is
// unreachable and never runs.
type MatchArm struct {
	Bindings []MatchBinding
	Body     Block
}

type MatchBinding struct {
	Local LocalID
	Var   MatchVarID
}

// MatchExprPayload is the payload of ExprMatch. The expression's Args are the
// subjects; Vars[i] for MatchVarSubject reads Args[Index].
type MatchExprPayload struct {
	Vars []MatchVar
	Tree Decision
	Arms []MatchArm
}

func (*MatchExprPayload) exprPayload() {}

// WalkDecision calls visit for every node of a decision tree.
func WalkDecision(d *Decision, visit func(*Decision)) {
	if d == nil {
		return
	}
	visit(d)
	for i := range d.Cases {
		WalkDecision(&d.Cases[i].Then, visit)
	}
	WalkDecision(d.Fallback, visit)
}

// validateMatch checks that an ExprMatch's variables read parts their parents
// have, that its tree tests variables of matching kinds and names valid arms,
// and that each binding's local has its variable's type.
func validateMatch(program *Program, fn Function, expr Expr) error {
	payload := exprPayloadAs[*MatchExprPayload](&expr)
	if payload == nil {
		return fmt.Errorf("match is missing its payload")
	}
	if len(expr.Args) == 0 {
		return fmt.Errorf("match has no subject")
	}
	for i, v := range payload.Vars {
		if !validTypeID(program, v.Type) {
			return fmt.Errorf("match variable %d has invalid type %d", i, v.Type)
		}
		if v.Source == MatchVarSubject {
			if v.Index < 0 || v.Index >= len(expr.Args) {
				return fmt.Errorf("match variable %d reads missing subject %d", i, v.Index)
			}
			if expr.Args[v.Index].Type != v.Type {
				return fmt.Errorf("match variable %d type %d does not match subject type %d", i, v.Type, expr.Args[v.Index].Type)
			}
			continue
		}
		if v.Parent < 0 || int(v.Parent) >= i {
			return fmt.Errorf("match variable %d has invalid parent %d", i, v.Parent)
		}
		parent, err := typeInfo(program, payload.Vars[v.Parent].Type)
		if err != nil {
			return err
		}
		var want TypeID
		switch v.Source {
		case MatchVarTupleElem:
			if parent.Kind != TypeTuple || v.Index < 0 || v.Index >= len(parent.Elems) {
				return fmt.Errorf("match variable %d reads a tuple element of type kind %d", i, parent.Kind)
			}
			want = parent.Elems[v.Index]
		case MatchVarMaybeSome:
			if parent.Kind != TypeMaybe {
				return fmt.Errorf("match variable %d reads a Maybe value of type kind %d", i, parent.Kind)
			}
			want = parent.Elem
		case MatchVarResultOk, MatchVarResultErr:
			if parent.Kind != TypeResult {
				return fmt.Errorf("match variable %d reads a Result part of type kind %d", i, parent.Kind)
			}
			want = parent.Value
			if v.Source == MatchVarResultErr {
				want = parent.Error
			}
		case MatchVarUnionMember:
			if parent.Kind != TypeUnion {
				return fmt.Errorf("match variable %d reads a union member of type kind %d", i, parent.Kind)
			}
			member, ok := unionMemberByTag(parent, v.Tag)
			if !ok {
				return fmt.Errorf("match variable %d reads invalid union tag %d", i, v.Tag)
			}
			want = member.Type
		case MatchVarForeignType:
			if !isDynamicMatchType(parent) {
				return fmt.Errorf("match variable %d narrows a value of type kind %d", i, parent.Kind)
			}
			if err := validateConcreteForeignType(program, v.Type); err != nil {
				return fmt.Errorf("match variable %d: %w", i, err)
			}
			want = v.Type
		default:
			return fmt.Errorf("match variable %d has invalid source %d", i, v.Source)
		}
		if want != v.Type {
			return fmt.Errorf("match variable %d type %d does not match its source type %d", i, v.Type, want)
		}
	}
	var validateDecision func(d *Decision) error
	validateDecision = func(d *Decision) error {
		switch d.Kind {
		case DecisionFail:
			return nil
		case DecisionRun:
			if d.Arm < 0 || d.Arm >= len(payload.Arms) {
				return fmt.Errorf("match runs invalid arm %d", d.Arm)
			}
			return nil
		case DecisionSwitch:
		default:
			return fmt.Errorf("match has invalid decision kind %d", d.Kind)
		}
		if d.Var < 0 || int(d.Var) >= len(payload.Vars) {
			return fmt.Errorf("match switches on invalid variable %d", d.Var)
		}
		if len(d.Cases) == 0 {
			return fmt.Errorf("match switch has no cases")
		}
		info, err := typeInfo(program, payload.Vars[d.Var].Type)
		if err != nil {
			return err
		}
		for _, c := range d.Cases {
			if err := validateMatchTest(program, info, c.Test); err != nil {
				return err
			}
			if err := validateDecision(&c.Then); err != nil {
				return err
			}
		}
		if d.Fallback != nil {
			return validateDecision(d.Fallback)
		}
		return nil
	}
	if err := validateDecision(&payload.Tree); err != nil {
		return err
	}
	for i, arm := range payload.Arms {
		for _, binding := range arm.Bindings {
			if binding.Local < 0 || int(binding.Local) >= len(fn.Locals) {
				return fmt.Errorf("match arm %d binds invalid local %d", i, binding.Local)
			}
			if binding.Var < 0 || int(binding.Var) >= len(payload.Vars) {
				return fmt.Errorf("match arm %d binds invalid variable %d", i, binding.Var)
			}
			if fn.Locals[binding.Local].Type != payload.Vars[binding.Var].Type {
				return fmt.Errorf("match arm %d binding local type %d does not match variable type %d", i, fn.Locals[binding.Local].Type, payload.Vars[binding.Var].Type)
			}
		}
		if err := validateBlock(program, fn, arm.Body); err != nil {
			return err
		}
	}
	return nil
}

func validateMatchTest(program *Program, info TypeInfo, test MatchTest) error {
	kindOK := false
	switch test.Kind {
	case MatchTestBool:
		kindOK = info.Kind == TypeBool
	case MatchTestIntRange:
		kindOK = (info.Kind == TypeInt || info.Kind == TypeRune || info.Kind == TypeByte) && test.Start <= test.End
	case MatchTestStr:
		kindOK = info.Kind == TypeStr
	case MatchTestEnum:
		kindOK = info.Kind == TypeEnum
	case MatchTestMaybeSome, MatchTestMaybeNone:
		kindOK = info.Kind == TypeMaybe
	case MatchTestResultOk, MatchTestResultErr:
		kindOK = info.Kind == TypeResult
	case MatchTestUnionTag:
		_, found := unionMemberByTag(info, test.Tag)
		kindOK = info.Kind == TypeUnion && found
	case MatchTestForeignType:
		kindOK = isDynamicMatchType(info) && validateConcreteForeignType(program, test.Type) == nil
	}
	if !kindOK {
		return fmt.Errorf("match test kind %d does not apply to type kind %d", test.Kind, info.Kind)
	}
	return nil
}

// isDynamicMatchType reports whether a value's dynamic type can be tested:
// Any, or a foreign Go interface (ADR 0042).
func isDynamicMatchType(info TypeInfo) bool {
	return info.Kind == TypeAny || (info.Kind == TypeForeignType && info.ForeignInterface)
}

func validateConcreteForeignType(program *Program, id TypeID) error {
	info, err := typeInfo(program, id)
	if err != nil {
		return err
	}
	if info.Kind != TypeForeignType || info.ForeignInterface {
		return fmt.Errorf("dynamic type test names type kind %d, not a concrete foreign type", info.Kind)
	}
	return nil
}
