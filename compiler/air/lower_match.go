package air

import (
	"fmt"

	"github.com/akonwi/ard/compiler/checker"
)

// lowerPatternMatch lowers a checked match (ADR 0083) to ExprMatch, carrying
// the checker's decision tree over AIR-typed match variables.
//
// A tuple literal subject such as `match (left, right)` is not built as a
// tuple: each element becomes its own subject, evaluated once in source
// order, unless an arm binds the whole tuple.
func (fl *functionLowerer) lowerPatternMatch(typeID TypeID, match *checker.PatternMatch) (*Expr, error) {
	if match.Tree == nil || len(match.Vars) == 0 {
		return nil, fmt.Errorf("match is missing its decision tree")
	}
	vars := make([]MatchVar, 0, len(match.Vars))
	varIDs := make([]MatchVarID, len(match.Vars))
	for i := range varIDs {
		varIDs[i] = -1
	}
	var subjects []Expr

	tuple, splitTuple := match.Subject.(*checker.TupleLiteral)
	if splitTuple {
		for _, arm := range match.Arms {
			for _, binding := range arm.Bindings {
				if binding.Var == 0 {
					splitTuple = false
				}
			}
		}
	}
	if splitTuple {
		tupleType, err := fl.internResolvedType(tuple.Type())
		if err != nil {
			return nil, err
		}
		info, ok := fl.l.typeInfo(tupleType)
		if !ok || info.Kind != TypeTuple || len(info.Elems) != len(tuple.Elements) {
			return nil, fmt.Errorf("match subject tuple lowered with non-matching AIR type %s", tuple.Type().String())
		}
		for i, element := range tuple.Elements {
			lowered, err := fl.lowerExprWithExpected(element, info.Elems[i])
			if err != nil {
				return nil, err
			}
			subjects = append(subjects, *lowered)
		}
	} else {
		subject, err := fl.lowerExpr(match.Subject)
		if err != nil {
			return nil, err
		}
		subjects = append(subjects, *subject)
		varIDs[0] = 0
		vars = append(vars, MatchVar{Type: subject.Type, Source: MatchVarSubject})
	}

	for i := 1; i < len(match.Vars); i++ {
		checked := match.Vars[i]
		if splitTuple && checked.Parent == 0 {
			if checked.Access != checker.MatchAccessTupleElem {
				return nil, fmt.Errorf("match tuple subject has a non-element variable")
			}
			varIDs[i] = MatchVarID(len(vars))
			vars = append(vars, MatchVar{Type: subjects[checked.Index].Type, Source: MatchVarSubject, Index: checked.Index})
			continue
		}
		parentID := varIDs[checked.Parent]
		if parentID < 0 {
			return nil, fmt.Errorf("match variable %d has no lowered parent", i)
		}
		parent, ok := fl.l.typeInfo(vars[parentID].Type)
		if !ok {
			return nil, fmt.Errorf("match variable %d has an invalid parent type", i)
		}
		lowered := MatchVar{Parent: parentID, Index: checked.Index}
		switch checked.Access {
		case checker.MatchAccessTupleElem:
			if parent.Kind != TypeTuple || checked.Index >= len(parent.Elems) {
				return nil, fmt.Errorf("match tuple element read from type kind %d", parent.Kind)
			}
			lowered.Source, lowered.Type = MatchVarTupleElem, parent.Elems[checked.Index]
		case checker.MatchAccessSome:
			if parent.Kind != TypeMaybe {
				return nil, fmt.Errorf("match Maybe value read from type kind %d", parent.Kind)
			}
			lowered.Source, lowered.Type = MatchVarMaybeSome, parent.Elem
		case checker.MatchAccessOk:
			if parent.Kind != TypeResult {
				return nil, fmt.Errorf("match Result value read from type kind %d", parent.Kind)
			}
			lowered.Source, lowered.Type = MatchVarResultOk, parent.Value
		case checker.MatchAccessErr:
			if parent.Kind != TypeResult {
				return nil, fmt.Errorf("match Result error read from type kind %d", parent.Kind)
			}
			lowered.Source, lowered.Type = MatchVarResultErr, parent.Error
		case checker.MatchAccessUnionMember:
			if parent.Kind != TypeUnion || checked.Index >= len(parent.Members) {
				return nil, fmt.Errorf("match union member read from type kind %d", parent.Kind)
			}
			member := parent.Members[checked.Index]
			lowered.Source, lowered.Type, lowered.Tag = MatchVarUnionMember, member.Type, member.Tag
		case checker.MatchAccessForeign:
			foreign, err := fl.l.internType(checked.Foreign)
			if err != nil {
				return nil, err
			}
			lowered.Source, lowered.Type = MatchVarForeignType, foreign
		default:
			return nil, fmt.Errorf("match variable %d has unknown access %d", i, checked.Access)
		}
		varIDs[i] = MatchVarID(len(vars))
		vars = append(vars, lowered)
	}

	tree, err := fl.lowerDecision(match.Tree, varIDs, vars)
	if err != nil {
		return nil, err
	}

	arms := make([]MatchArm, len(match.Arms))
	for i, arm := range match.Arms {
		restore := fl.scopeLocals()
		bindings := make([]MatchBinding, 0, len(arm.Bindings))
		for _, binding := range arm.Bindings {
			varID := varIDs[binding.Var]
			if varID < 0 {
				restore()
				return nil, fmt.Errorf("match binding %s reads an unlowered variable", binding.Name)
			}
			local := fl.defineLocal(binding.Name, vars[varID].Type, false)
			if vars[varID].Source == MatchVarMaybeSome {
				if elem, ok := fl.l.typeInfo(vars[varID].Type); ok {
					fl.fn.Locals[local].Reference = elem.Kind == TypeReference || (elem.Kind == TypeForeignType && elem.ForeignPointer)
				}
			}
			bindings = append(bindings, MatchBinding{Local: local, Var: varID})
		}
		body, err := fl.lowerBlockWithDefault(arm.Body.Stmts, typeID)
		restore()
		if err != nil {
			return nil, err
		}
		arms[i] = MatchArm{Bindings: bindings, Body: body}
	}

	return &Expr{Kind: ExprMatch, Type: typeID, Args: subjects, Payload: &MatchExprPayload{Vars: vars, Tree: tree, Arms: arms}}, nil
}

func (fl *functionLowerer) lowerDecision(decision *checker.Decision, varIDs []MatchVarID, vars []MatchVar) (Decision, error) {
	switch decision.Kind {
	case checker.DecisionFail:
		return Decision{Kind: DecisionFail}, nil
	case checker.DecisionRun:
		return Decision{Kind: DecisionRun, Arm: decision.Arm}, nil
	case checker.DecisionSwitch:
	default:
		return Decision{}, fmt.Errorf("unknown decision kind %d", decision.Kind)
	}
	varID := varIDs[decision.Var]
	if varID < 0 {
		return Decision{}, fmt.Errorf("match switch on an unlowered variable")
	}
	lowered := Decision{Kind: DecisionSwitch, Var: varID}
	for _, matchCase := range decision.Cases {
		test, err := fl.lowerMatchTest(matchCase.Test, vars[varID].Type)
		if err != nil {
			return Decision{}, err
		}
		then, err := fl.lowerDecision(matchCase.Then, varIDs, vars)
		if err != nil {
			return Decision{}, err
		}
		lowered.Cases = append(lowered.Cases, DecisionCase{Test: test, Then: then})
	}
	if decision.Fallback != nil {
		fallback, err := fl.lowerDecision(decision.Fallback, varIDs, vars)
		if err != nil {
			return Decision{}, err
		}
		lowered.Fallback = &fallback
	}
	return lowered, nil
}

func (fl *functionLowerer) lowerMatchTest(test checker.PatternTest, varType TypeID) (MatchTest, error) {
	switch test.Kind {
	case checker.TestBool:
		return MatchTest{Kind: MatchTestBool, Bool: test.Bool}, nil
	case checker.TestInt:
		return MatchTest{Kind: MatchTestIntRange, Start: test.Start, End: test.End}, nil
	case checker.TestStr:
		return MatchTest{Kind: MatchTestStr, Str: test.Str}, nil
	case checker.TestEnum:
		return MatchTest{Kind: MatchTestEnum, Discriminant: test.Discriminant}, nil
	case checker.TestSome:
		return MatchTest{Kind: MatchTestMaybeSome}, nil
	case checker.TestNone:
		return MatchTest{Kind: MatchTestMaybeNone}, nil
	case checker.TestOk:
		return MatchTest{Kind: MatchTestResultOk}, nil
	case checker.TestErr:
		return MatchTest{Kind: MatchTestResultErr}, nil
	case checker.TestUnionMember:
		info, ok := fl.l.typeInfo(varType)
		if !ok || info.Kind != TypeUnion || test.Index >= len(info.Members) {
			return MatchTest{}, fmt.Errorf("union match test on a non-union variable")
		}
		return MatchTest{Kind: MatchTestUnionTag, Tag: info.Members[test.Index].Tag}, nil
	case checker.TestForeign:
		typeID, err := fl.l.internType(test.Foreign)
		if err != nil {
			return MatchTest{}, err
		}
		return MatchTest{Kind: MatchTestForeignType, Type: typeID}, nil
	}
	return MatchTest{}, fmt.Errorf("unknown match test kind %d", test.Kind)
}
