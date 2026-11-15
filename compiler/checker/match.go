package checker

import (
	"fmt"
	"strings"

	"github.com/akonwi/ard/parse"
)

// PatternMatch is a match over a subject value (ADR 0083). Arms are in source
// order; the first arm whose pattern matches runs. Tree is the compiled
// decision tree over Vars, which the backend uses to run the match.
type PatternMatch struct {
	Subject    Expression
	Arms       []PatternArm
	Vars       []MatchVar
	Tree       *Decision
	ResultType Type
}

// PatternArm is one arm of a PatternMatch. Bindings are the names its pattern
// binds, each to the match variable it reads.
type PatternArm struct {
	Pattern  Pattern
	Bindings []PatternBinding
	Body     *Block
}

type PatternBinding struct {
	Name string
	Var  int
	Type Type
}

func (m *PatternMatch) Type() Type {
	if m.ResultType != nil {
		return m.ResultType
	}
	for _, arm := range m.Arms {
		if arm.Body != nil {
			return arm.Body.Type()
		}
	}
	return Void
}

// missingPatternLimit bounds how many missing patterns a non-exhaustive match
// diagnostic lists.
const missingPatternLimit = 8

// checkPatternMatch checks a match with a subject: every arm pattern against
// the subject type, every arm body with the pattern's bindings in scope, then
// exhaustiveness and reachability over the compiled decision tree.
func (c *Checker) checkPatternMatch(s *parse.MatchExpression, subject Expression, allowMixedVoid bool) Expression {
	subjectType := deref(subject.Type())
	subjectKind := classifyMatchType(subjectType)
	if subjectKind == matchKindOpaque {
		legacy := fmt.Sprintf("Cannot match on %s", subject.Type())
		c.addDiagnostic(invalidMatchSubjectDiagnostic{Actual: subject.Type(), Span: c.sourceSpan(s.Subject.GetLocation()), LegacyMessage: legacy}.build())
		return nil
	}

	vars := newMatchVarTable(subjectType)
	legacyMaybe := false
	if subjectKind == matchKindMaybe {
		for _, matchCase := range s.Cases {
			if isLegacyMaybeArm(matchCase.Pattern) {
				legacyMaybe = true
				break
			}
		}
	}

	arms := make([]PatternArm, 0, len(s.Cases))
	patterns := make([]Pattern, 0, len(s.Cases))
	valid := true
	merged := true
	var resultType Type
	for _, matchCase := range s.Cases {
		ctx := &patternContext{vars: vars, legacyMaybe: legacyMaybe}
		pattern := c.checkPattern(matchCase.Pattern, 0, subjectType, true, ctx)
		if pattern == nil || ctx.failed {
			valid = false
			pattern = nil
		}
		bindings := ctx.bindings
		body := c.checkMatchArmBlock(matchCase.Body, func() {
			for _, binding := range bindings {
				// ADR 0073: only `mut` declarations create writable slots.
				sym := c.scope.add(binding.Name, binding.Type, false)
				if !binding.Implicit {
					c.recordBinding(binding.Location, sym)
				}
			}
		})
		arm := PatternArm{Pattern: pattern, Body: body}
		for _, binding := range bindings {
			arm.Bindings = append(arm.Bindings, PatternBinding{Name: binding.Name, Var: binding.Var, Type: binding.Type})
		}
		arms = append(arms, arm)
		patterns = append(patterns, pattern)
		if merged {
			resultType, merged = mergeMatchResultType(c, resultType, body.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
		}
	}
	if !valid {
		return nil
	}

	compiler := newDecisionCompiler(vars, len(arms))
	tree := compiler.compileMatch(patterns)
	if compiler.tooLarge {
		c.addNonExhaustiveMatch("Match is too complex to check", s.GetLocation(), "split this match into smaller matches")
		return nil
	}
	if compiler.missing {
		c.reportMissingPatterns(s, compiler.missingPatterns(tree, missingPatternLimit), legacyMaybe)
	}
	for i, arm := range arms {
		if compiler.reached[i] {
			continue
		}
		c.reportUnreachableArm(arm, arms[:i])
	}
	if compiler.missing || !merged {
		return nil
	}

	c.finalizeMatchSubject(subject)
	return &PatternMatch{
		Subject:    subject,
		Arms:       arms,
		Vars:       vars.vars,
		Tree:       tree,
		ResultType: resultType,
	}
}

// finalizeMatchSubject resolves type variables a constructor subject leaves
// unconstrained, such as the error type of `Result::ok(1)`, before the match
// reaches lowering.
func (c *Checker) finalizeMatchSubject(subject Expression) {
	if tuple, ok := subject.(*TupleLiteral); ok {
		for _, element := range tuple.Elements {
			c.finalizeMatchSubject(element)
		}
		return
	}
	switch typ := deref(subject.Type()).(type) {
	case *Maybe:
		bindDirectUnresolvedCallTypeVar(typ.Of(), Void)
	case *Result:
		finalizeErasedResultExpression(subject)
		if builtinResultConstructor(subject) == nil {
			bindDirectUnresolvedCallTypeVar(typ.Val(), Void)
			bindDirectUnresolvedCallTypeVar(typ.Err(), Void)
		}
	}
}

func (c *Checker) reportMissingPatterns(s *parse.MatchExpression, missing []string, legacyMaybe bool) {
	if legacyMaybe {
		// The legacy top-level Maybe form spells some(x) as a binding and
		// none as `_`.
		for _, pattern := range missing {
			if pattern == "none" {
				c.addNonExhaustiveMatch("Match on a Maybe type must include a wildcard (_) case", s.GetLocation(), "add a wildcard `_` case for the absent value")
			} else {
				c.addNonExhaustiveMatch("Match on a Maybe type must include a binding case", s.GetLocation(), "add a binding case for the present value")
			}
		}
		return
	}
	if len(missing) == 0 {
		missing = []string{"_"}
	}
	quoted := make([]string, len(missing))
	for i, pattern := range missing {
		quoted[i] = "`" + pattern + "`"
	}
	var legacy, label, text string
	if len(missing) == 1 {
		legacy = fmt.Sprintf("Incomplete match: missing case for '%s'", missing[0])
		if missing[0] == "_" {
			label = "add a catch-all `_` case"
		} else {
			label = fmt.Sprintf("add a case for %s", quoted[0])
		}
	} else {
		legacy = fmt.Sprintf("Incomplete match: missing cases for '%s'", strings.Join(missing, "', '"))
		label = fmt.Sprintf("add cases for %s", strings.Join(quoted, ", "))
		if len(missing) >= missingPatternLimit {
			legacy += ", and more"
			label += ", and more"
		}
		text = "The missing patterns are:\n\n    " + strings.Join(missing, "\n    ")
	}
	c.addDiagnostic(nonExhaustiveMatchDiagnostic{LegacyMessage: legacy, Span: c.sourceSpan(s.GetLocation()), Label: label, Text: text}.build())
}

// reportUnreachableArm warns about an arm no value can reach because earlier
// arms match everything it matches. When one earlier arm alone covers it, the
// diagnostic points there.
func (c *Checker) reportUnreachableArm(arm PatternArm, earlier []PatternArm) {
	if arm.Pattern == nil {
		return
	}
	var coveredBy *SourceSpan
	for _, previous := range earlier {
		if previous.Pattern != nil && patternSubsumes(previous.Pattern, arm.Pattern) {
			span := c.sourceSpan(previous.Pattern.PatternLocation())
			coveredBy = &span
			break
		}
	}
	c.addDiagnostic(unreachableMatchArmDiagnostic{
		Span:      c.sourceSpan(arm.Pattern.PatternLocation()),
		CoveredBy: coveredBy,
		Pattern:   patternString(arm.Pattern),
	}.build())
}
