package checker

import (
	"fmt"
	"strconv"

	"github.com/akonwi/ard/parse"
)

// Pattern is a checked match pattern (ADR 0083). Patterns nest: a tuple
// pattern holds one pattern per element, and constructor patterns such as
// `ok(p)` or `Str(p)` hold the pattern for their payload. Every pattern is
// checked against the type of the value it matches, which is identified by a
// match variable (see MatchVar).
type Pattern interface {
	PatternLocation() parse.Location
}

// WildcardPattern is `_`: it matches any value and binds nothing.
type WildcardPattern struct {
	Location parse.Location
}

// BindingPattern matches any value and binds it to Name. Implicit bindings
// come from the legacy top-level forms (`Str =>` binds `it`, `ok =>` binds
// `ok`) and have no source spelling of their own.
type BindingPattern struct {
	Location parse.Location
	Name     string
	Var      int
	Implicit bool
}

type BoolPattern struct {
	Location parse.Location
	Value    bool
}

// IntPattern matches an integer or rune in the inclusive range Start..End. A
// literal pattern has Start == End.
type IntPattern struct {
	Location parse.Location
	Start    int
	End      int
	Rune     bool
}

type StrPattern struct {
	Location parse.Location
	Value    string
}

// EnumPattern matches one enum variant. Variants that share a discriminant
// (imported Go enum-like aliases) match the same values.
type EnumPattern struct {
	Location     parse.Location
	Enum         *Enum
	Variant      int
	Discriminant int
}

type TuplePattern struct {
	Location parse.Location
	Elems    []Pattern
}

// ResultPattern is `ok(p)` or `err(p)`.
type ResultPattern struct {
	Location parse.Location
	Ok       bool
	Inner    Pattern
}

// MaybePattern is `some(p)` or `none`. Inner is nil for `none`.
type MaybePattern struct {
	Location parse.Location
	Some     bool
	Inner    Pattern
}

// UnionPattern matches one member of a union and its payload.
type UnionPattern struct {
	Location parse.Location
	Union    *Union
	Member   int
	Inner    Pattern
}

// ForeignTypePattern is a dynamic type test over an Any or foreign-interface
// value (ADR 0042): `pkg::Type(p)` matches when the dynamic type is Type and
// matches p against the narrowed value.
type ForeignTypePattern struct {
	Location parse.Location
	Type     *ForeignType
	Inner    Pattern
}

func (p *WildcardPattern) PatternLocation() parse.Location    { return p.Location }
func (p *BindingPattern) PatternLocation() parse.Location     { return p.Location }
func (p *BoolPattern) PatternLocation() parse.Location        { return p.Location }
func (p *IntPattern) PatternLocation() parse.Location         { return p.Location }
func (p *StrPattern) PatternLocation() parse.Location         { return p.Location }
func (p *EnumPattern) PatternLocation() parse.Location        { return p.Location }
func (p *TuplePattern) PatternLocation() parse.Location       { return p.Location }
func (p *ResultPattern) PatternLocation() parse.Location      { return p.Location }
func (p *MaybePattern) PatternLocation() parse.Location       { return p.Location }
func (p *UnionPattern) PatternLocation() parse.Location       { return p.Location }
func (p *ForeignTypePattern) PatternLocation() parse.Location { return p.Location }

// MatchVarAccess says how a match variable is read from its parent.
type MatchVarAccess int

const (
	// MatchAccessSubject is the match subject itself (variable 0).
	MatchAccessSubject MatchVarAccess = iota
	// MatchAccessTupleElem is element Index of a tuple.
	MatchAccessTupleElem
	// MatchAccessSome is the present value of a Maybe.
	MatchAccessSome
	// MatchAccessOk is the value of an ok Result.
	MatchAccessOk
	// MatchAccessErr is the error of an err Result.
	MatchAccessErr
	// MatchAccessUnionMember is the payload of union member Index.
	MatchAccessUnionMember
	// MatchAccessForeign is a dynamic value narrowed to Foreign.
	MatchAccessForeign
)

// MatchVar is a value a match tests or binds: the subject, or a part of
// another match variable. Variables are interned by access path, so the same
// part of the subject is always the same variable.
type MatchVar struct {
	Type    Type
	Parent  int
	Access  MatchVarAccess
	Index   int
	Foreign *ForeignType
}

type matchVarKey struct {
	parent  int
	access  MatchVarAccess
	index   int
	foreign string
}

type matchVarTable struct {
	vars  []MatchVar
	index map[matchVarKey]int
}

func newMatchVarTable(subject Type) *matchVarTable {
	return &matchVarTable{
		vars:  []MatchVar{{Type: subject, Parent: -1, Access: MatchAccessSubject}},
		index: map[matchVarKey]int{},
	}
}

func (t *matchVarTable) key(parent int, access MatchVarAccess, index int, foreign *ForeignType) matchVarKey {
	key := matchVarKey{parent: parent, access: access, index: index}
	if foreign != nil {
		key.foreign = foreign.String()
	}
	return key
}

func (t *matchVarTable) child(parent int, access MatchVarAccess, index int, foreign *ForeignType, typ Type) int {
	key := t.key(parent, access, index, foreign)
	if id, ok := t.index[key]; ok {
		return id
	}
	id := len(t.vars)
	t.vars = append(t.vars, MatchVar{Type: typ, Parent: parent, Access: access, Index: index, Foreign: foreign})
	t.index[key] = id
	return id
}

func (t *matchVarTable) lookup(parent int, access MatchVarAccess, index int, foreign *ForeignType) (int, bool) {
	id, ok := t.index[t.key(parent, access, index, foreign)]
	return id, ok
}

// matchTypeKind classifies a type by the patterns that can test it.
type matchTypeKind int

const (
	matchKindOpaque matchTypeKind = iota
	matchKindBool
	matchKindInt
	matchKindRune
	matchKindStr
	matchKindEnum
	matchKindMaybe
	matchKindResult
	matchKindUnion
	matchKindTuple
	matchKindDynamic
)

func classifyMatchType(t Type) matchTypeKind {
	t = deref(t)
	switch typed := t.(type) {
	case *Maybe:
		return matchKindMaybe
	case *Result:
		return matchKindResult
	case *Enum:
		return matchKindEnum
	case *Union:
		return matchKindUnion
	case *Tuple:
		return matchKindTuple
	case *anyType:
		return matchKindDynamic
	case *ForeignType:
		if typed.Interface {
			return matchKindDynamic
		}
	}
	switch t {
	case Bool:
		return matchKindBool
	case Int:
		return matchKindInt
	case Rune:
		return matchKindRune
	case Str:
		return matchKindStr
	}
	return matchKindOpaque
}

// patternBinding is a name bound by an arm's pattern.
type patternBinding struct {
	Name     string
	Var      int
	Type     Type
	Location parse.Location
	Implicit bool
}

type patternContext struct {
	vars        *matchVarTable
	legacyMaybe bool
	bindings    []patternBinding
	failed      bool
}

func (ctx *patternContext) bind(c *Checker, name string, varID int, typ Type, location parse.Location, implicit bool) Pattern {
	for _, existing := range ctx.bindings {
		if existing.Name == name {
			c.addInvalidMatchPattern(
				fmt.Sprintf("%s is bound more than once in this pattern", name),
				location,
				fmt.Sprintf("`%s` is already bound in this pattern", name),
			)
			ctx.failed = true
			return &WildcardPattern{Location: location}
		}
	}
	ctx.bindings = append(ctx.bindings, patternBinding{Name: name, Var: varID, Type: typ, Location: location, Implicit: implicit})
	return &BindingPattern{Location: location, Name: name, Var: varID, Implicit: implicit}
}

func (ctx *patternContext) invalid(c *Checker, location parse.Location, message string, label string) Pattern {
	c.addInvalidMatchPattern(message, location, label)
	ctx.failed = true
	return nil
}

// isLegacyMaybeArm reports whether a top-level arm uses the legacy Maybe
// binding form, `name => ...`, where the name binds the present value. Any
// name but `none` binds, including `some`, which existing code uses as a
// binding name.
func isLegacyMaybeArm(pattern parse.Expression) bool {
	id, ok := pattern.(*parse.Identifier)
	if !ok {
		return false
	}
	return id.Name != "_" && id.Name != "none"
}

func singlePatternArg(call *parse.FunctionCall) (parse.Expression, bool) {
	if len(call.TypeArgs) != 0 || len(call.Args) != 1 || call.Args[0].Name != "" || call.Args[0].Spread {
		return nil, false
	}
	return call.Args[0].Value, true
}

// checkPattern checks one pattern against the type of match variable varID.
// top is true only for the whole arm pattern, where Ard keeps its legacy
// forms: a bare Maybe binding, bare `ok`/`err`, and bare union member names
// that bind `it`. It returns nil when the pattern is invalid.
func (c *Checker) checkPattern(expr parse.Expression, varID int, typ Type, top bool, ctx *patternContext) Pattern {
	pattern := c.checkPatternShape(expr, varID, typ, top, ctx)
	if pattern == nil {
		ctx.recoverBindings(expr)
	}
	return pattern
}

// recoverBindings binds the names an invalid pattern appears to bind, with
// the error type, so the arm body does not report them as undefined
// (ADR 0074).
func (ctx *patternContext) recoverBindings(expr parse.Expression) {
	switch p := expr.(type) {
	case *parse.Identifier:
		if p.Name == "_" {
			return
		}
		for _, existing := range ctx.bindings {
			if existing.Name == p.Name {
				return
			}
		}
		ctx.bindings = append(ctx.bindings, patternBinding{Name: p.Name, Var: -1, Type: invalidType, Location: p.GetLocation()})
	case *parse.TupleLiteral:
		for _, item := range p.Items {
			ctx.recoverBindings(item)
		}
	case *parse.FunctionCall:
		for _, arg := range p.Args {
			ctx.recoverBindings(arg.Value)
		}
	case *parse.StaticFunction:
		for _, arg := range p.Function.Args {
			ctx.recoverBindings(arg.Value)
		}
	}
}

func (c *Checker) checkPatternShape(expr parse.Expression, varID int, typ Type, top bool, ctx *patternContext) Pattern {
	typ = deref(typ)
	location := expr.GetLocation()
	kind := classifyMatchType(typ)

	if id, ok := expr.(*parse.Identifier); ok {
		return c.checkIdentifierPattern(id, varID, typ, kind, top, ctx)
	}

	switch kind {
	case matchKindBool:
		if lit, ok := expr.(*parse.BoolLiteral); ok {
			return &BoolPattern{Location: location, Value: lit.Value}
		}
		return ctx.invalid(c, location, "Pattern in boolean match must be a boolean literal (true or false)", "expected `true`, `false`, `_`, or a name")
	case matchKindInt:
		return c.checkIntPattern(expr, ctx)
	case matchKindRune:
		if lit, ok := expr.(*parse.RuneLiteral); ok {
			value, valid := c.parseRuneLiteralValue(lit)
			if !valid {
				ctx.failed = true
				return nil
			}
			return &IntPattern{Location: location, Start: int(value), End: int(value), Rune: true}
		}
		return ctx.invalid(c, location, "Pattern in Rune match must be a rune literal or '_'", "expected a rune literal, `_`, or a name")
	case matchKindStr:
		if lit, ok := expr.(*parse.StrLiteral); ok {
			return &StrPattern{Location: location, Value: lit.Value}
		}
		return ctx.invalid(c, location, "Pattern in Str match must be a string literal or '_'", "expected a string literal, `_`, or a name")
	case matchKindEnum:
		return c.checkEnumPattern(expr, typ.(*Enum), ctx)
	case matchKindTuple:
		return c.checkTuplePattern(expr, varID, typ.(*Tuple), ctx)
	case matchKindResult:
		return c.checkResultPattern(expr, varID, typ.(*Result), ctx)
	case matchKindMaybe:
		return c.checkMaybePattern(expr, varID, typ.(*Maybe), ctx)
	case matchKindUnion:
		return c.checkUnionPattern(expr, varID, typ.(*Union), top, ctx)
	case matchKindDynamic:
		return c.checkForeignTypePattern(expr, varID, typ, ctx)
	}
	return ctx.invalid(c, location,
		fmt.Sprintf("Values of type %s can only be matched with '_' or a name", typ),
		fmt.Sprintf("`%s` values can only be matched with `_` or a name", typ),
	)
}

func (c *Checker) checkIdentifierPattern(id *parse.Identifier, varID int, typ Type, kind matchTypeKind, top bool, ctx *patternContext) Pattern {
	location := id.GetLocation()
	switch kind {
	case matchKindMaybe:
		maybe := typ.(*Maybe)
		if top && ctx.legacyMaybe {
			if id.Name == "_" {
				return &MaybePattern{Location: location}
			}
			if id.Name != "none" {
				inner := ctx.bind(c, id.Name, ctx.vars.child(varID, MatchAccessSome, 0, nil, maybe.of), maybe.of, location, false)
				return &MaybePattern{Location: location, Some: true, Inner: inner}
			}
		}
		switch id.Name {
		case "none":
			return &MaybePattern{Location: location}
		case "some":
			return ctx.invalid(c, location, "A some pattern needs a pattern for the present value", "write `some(_)` or `some(name)`")
		}
	case matchKindResult:
		result := typ.(*Result)
		if id.Name == "ok" || id.Name == "err" {
			if !top {
				return ctx.invalid(c, location,
					fmt.Sprintf("A nested %s pattern needs a pattern for its value", id.Name),
					fmt.Sprintf("write `%s(_)` or `%s(name)`", id.Name, id.Name),
				)
			}
			isOk := id.Name == "ok"
			access, inner := MatchAccessErr, result.err
			if isOk {
				access, inner = MatchAccessOk, result.val
			}
			binding := ctx.bind(c, id.Name, ctx.vars.child(varID, access, 0, nil, inner), inner, location, true)
			return &ResultPattern{Location: location, Ok: isOk, Inner: binding}
		}
	case matchKindEnum:
		enum := typ.(*Enum)
		for i, value := range enum.Values {
			if value.Name == id.Name {
				return &EnumPattern{Location: location, Enum: enum, Variant: i, Discriminant: value.Value}
			}
		}
	case matchKindUnion:
		union := typ.(*Union)
		if id.Name != "_" {
			memberType, member, found, ambiguous := unqualifiedUnionMember(union, id.Name)
			if ambiguous {
				return ctx.invalid(c, location, fmt.Sprintf("Pattern %s is ambiguous in union %s", id.Name, union), "qualify the union member type")
			}
			if found {
				if !top {
					return ctx.invalid(c, location,
						fmt.Sprintf("A nested union member pattern needs a pattern for its value: %s(_)", id.Name),
						fmt.Sprintf("write `%s(_)` or `%s(name)`", id.Name, id.Name),
					)
				}
				inner := ctx.bind(c, "it", ctx.vars.child(varID, MatchAccessUnionMember, member, nil, memberType), memberType, location, true)
				return &UnionPattern{Location: location, Union: union, Member: member, Inner: inner}
			}
		}
	}
	if id.Name == "_" {
		return &WildcardPattern{Location: location}
	}
	return ctx.bind(c, id.Name, varID, typ, location, false)
}

func unqualifiedUnionMember(union *Union, name string) (Type, int, bool, bool) {
	found := -1
	for i, member := range union.Types {
		if member.String() != name {
			continue
		}
		if found >= 0 {
			return nil, 0, false, true
		}
		found = i
	}
	if found < 0 {
		return nil, 0, false, false
	}
	return union.Types[found], found, true, false
}

func (c *Checker) checkIntPattern(expr parse.Expression, ctx *patternContext) Pattern {
	location := expr.GetLocation()
	switch p := expr.(type) {
	case *parse.NumLiteral, *parse.UnaryExpression:
		value, err := c.extractIntFromPattern(expr)
		if err != nil {
			return ctx.invalid(c, location, fmt.Sprintf("Invalid integer literal: %s", err.Error()), "this is not a valid integer pattern")
		}
		return &IntPattern{Location: location, Start: value, End: value}
	case *parse.RangeExpression:
		start, startErr := c.extractIntFromPattern(p.Start)
		if startErr != nil {
			return ctx.invalid(c, p.Start.GetLocation(), fmt.Sprintf("Invalid start value in range: %s", startErr.Error()), "range start must be an integer pattern")
		}
		end, endErr := c.extractIntFromPattern(p.End)
		if endErr != nil {
			return ctx.invalid(c, p.End.GetLocation(), fmt.Sprintf("Invalid end value in range: %s", endErr.Error()), "range end must be an integer pattern")
		}
		if start > end {
			return ctx.invalid(c, location, "Range start must be less than or equal to end", "range start must not exceed its end")
		}
		return &IntPattern{Location: location, Start: start, End: end}
	case *parse.StaticProperty:
		// An enum variant names its integer discriminant.
		checked := c.checkExpr(p)
		if checked == nil {
			ctx.failed = true
			return nil
		}
		variant, ok := checked.(*EnumVariant)
		if !ok {
			return ctx.invalid(c, location, "Pattern in Int match must be an integer literal, range, or enum variant", "this does not resolve to an enum variant")
		}
		value := variant.enum.Values[variant.Variant].Value
		return &IntPattern{Location: location, Start: value, End: value}
	}
	return ctx.invalid(c, location, fmt.Sprintf("Invalid pattern for Int match: %T", expr), "expected an integer literal, range, enum variant, `_`, or a name")
}

func (c *Checker) checkEnumPattern(expr parse.Expression, enum *Enum, ctx *patternContext) Pattern {
	location := expr.GetLocation()
	prop, ok := expr.(*parse.StaticProperty)
	if !ok {
		return ctx.invalid(c, location, "Pattern in enum match must be an enum variant or wildcard", "expected an enum variant, `_`, or a name")
	}
	checked := c.checkExpr(prop)
	if checked == nil {
		ctx.failed = true
		return nil
	}
	variant, ok := checked.(*EnumVariant)
	if !ok {
		return ctx.invalid(c, location, "Pattern in enum match must be an enum variant", "this does not resolve to an enum variant")
	}
	if !variant.enum.equal(enum) {
		return ctx.invalid(c, location,
			fmt.Sprintf("Cannot match %s variant against %s enum", variant.enum.Name, enum.Name),
			fmt.Sprintf("this variant belongs to `%s`, not `%s`", variant.enum.Name, enum.Name),
		)
	}
	index := int(variant.Variant)
	return &EnumPattern{Location: location, Enum: enum, Variant: index, Discriminant: enum.Values[index].Value}
}

func (c *Checker) checkTuplePattern(expr parse.Expression, varID int, tuple *Tuple, ctx *patternContext) Pattern {
	location := expr.GetLocation()
	literal, ok := expr.(*parse.TupleLiteral)
	if !ok {
		return ctx.invalid(c, location,
			fmt.Sprintf("Pattern for a tuple of type %s must be a tuple pattern", tuple),
			fmt.Sprintf("expected a tuple pattern like `(%s)`, `_`, or a name", tupleWildcards(len(tuple.elements))),
		)
	}
	if len(literal.Items) != len(tuple.elements) {
		return ctx.invalid(c, location,
			fmt.Sprintf("Tuple pattern has %d elements, but %s has %d", len(literal.Items), tuple, len(tuple.elements)),
			fmt.Sprintf("expected %d elements to match `%s`", len(tuple.elements), tuple),
		)
	}
	elems := make([]Pattern, len(literal.Items))
	valid := true
	for i, item := range literal.Items {
		elemType := tuple.elements[i]
		child := ctx.vars.child(varID, MatchAccessTupleElem, i, nil, elemType)
		elems[i] = c.checkPattern(item, child, elemType, false, ctx)
		if elems[i] == nil {
			valid = false
		}
	}
	if !valid {
		return nil
	}
	return &TuplePattern{Location: location, Elems: elems}
}

func tupleWildcards(count int) string {
	out := ""
	for i := 0; i < count; i++ {
		if i > 0 {
			out += ", "
		}
		out += "_"
	}
	return out
}

func (c *Checker) checkResultPattern(expr parse.Expression, varID int, result *Result, ctx *patternContext) Pattern {
	location := expr.GetLocation()
	var call *parse.FunctionCall
	switch p := expr.(type) {
	case *parse.FunctionCall:
		call = p
	case *parse.StaticFunction:
		if target, ok := p.Target.(*parse.Identifier); ok && target.Name == "Result" {
			call = &p.Function
		}
	}
	if call == nil || (call.Name != "ok" && call.Name != "err") {
		return ctx.invalid(c, location, "Pattern in Result match must be ok(...) or err(...)", "expected `ok(...)`, `err(...)`, `_`, or a name")
	}
	arg, ok := singlePatternArg(call)
	if !ok {
		return ctx.invalid(c, location,
			fmt.Sprintf("A %s pattern takes exactly one pattern for its value", call.Name),
			fmt.Sprintf("write `%s(_)` or `%s(name)`", call.Name, call.Name),
		)
	}
	isOk := call.Name == "ok"
	access, innerType := MatchAccessErr, result.err
	if isOk {
		access, innerType = MatchAccessOk, result.val
	}
	child := ctx.vars.child(varID, access, 0, nil, innerType)
	inner := c.checkPattern(arg, child, innerType, false, ctx)
	if inner == nil {
		return nil
	}
	return &ResultPattern{Location: location, Ok: isOk, Inner: inner}
}

func (c *Checker) checkMaybePattern(expr parse.Expression, varID int, maybe *Maybe, ctx *patternContext) Pattern {
	location := expr.GetLocation()
	call, ok := expr.(*parse.FunctionCall)
	if !ok || call.Name != "some" {
		return ctx.invalid(c, location, "Pattern in Maybe match must be some(...) or none", "expected `some(...)`, `none`, `_`, or a name")
	}
	arg, ok := singlePatternArg(call)
	if !ok {
		return ctx.invalid(c, location, "A some pattern takes exactly one pattern for the present value", "write `some(_)` or `some(name)`")
	}
	child := ctx.vars.child(varID, MatchAccessSome, 0, nil, maybe.of)
	inner := c.checkPattern(arg, child, maybe.of, false, ctx)
	if inner == nil {
		return nil
	}
	return &MaybePattern{Location: location, Some: true, Inner: inner}
}

func (c *Checker) checkUnionPattern(expr parse.Expression, varID int, union *Union, top bool, ctx *patternContext) Pattern {
	location := expr.GetLocation()
	var (
		typeName  string
		member    int
		memberTyp Type
		found     bool
		ambiguous bool
		arg       parse.Expression
		hasArg    bool
		argValid  = true
	)
	switch p := expr.(type) {
	case *parse.FunctionCall:
		typeName = p.Name
		memberTyp, member, found, ambiguous = unqualifiedUnionMember(union, p.Name)
		arg, argValid = singlePatternArg(p)
		hasArg = true
	case *parse.StaticProperty:
		typeName = p.String()
		memberTyp, member, found, ambiguous = c.qualifiedUnionPatternMember(union, p.Target, p.Property)
	case *parse.StaticFunction:
		typeName = fmt.Sprintf("%s::%s", p.Target, p.Function.Name)
		memberTyp, member, found, ambiguous = c.qualifiedUnionPatternMember(union, p.Target, &parse.Identifier{Name: p.Function.Name})
		arg, argValid = singlePatternArg(&p.Function)
		hasArg = true
	default:
		return ctx.invalid(c, location, "Invalid union match pattern", "use a union member type with an optional binding")
	}
	if ambiguous {
		return ctx.invalid(c, location, fmt.Sprintf("Pattern %s is ambiguous in union %s", typeName, union), "qualify the union member type")
	}
	if !found {
		return ctx.invalid(c, location,
			fmt.Sprintf("Type %s is not part of union %s", typeName, union),
			fmt.Sprintf("`%s` is not a member of `%s`", typeName, union),
		)
	}
	if hasArg && !argValid {
		return ctx.invalid(c, location, "Invalid union match pattern", fmt.Sprintf("use `%s(binding)` with exactly one pattern", typeName))
	}
	child := ctx.vars.child(varID, MatchAccessUnionMember, member, nil, memberTyp)
	if !hasArg {
		if !top {
			return ctx.invalid(c, location,
				fmt.Sprintf("A nested union member pattern needs a pattern for its value: %s(_)", typeName),
				fmt.Sprintf("write `%s(_)` or `%s(name)`", typeName, typeName),
			)
		}
		inner := ctx.bind(c, "it", child, memberTyp, location, true)
		return &UnionPattern{Location: location, Union: union, Member: member, Inner: inner}
	}
	inner := c.checkPattern(arg, child, memberTyp, false, ctx)
	if inner == nil {
		return nil
	}
	return &UnionPattern{Location: location, Union: union, Member: member, Inner: inner}
}

func (c *Checker) checkForeignTypePattern(expr parse.Expression, varID int, subjectType Type, ctx *patternContext) Pattern {
	location := expr.GetLocation()
	invalid := func(loc parse.Location, message, label string) Pattern {
		c.addInvalidForeignTypePattern(message, loc, label)
		ctx.failed = true
		return nil
	}
	p, ok := expr.(*parse.StaticFunction)
	if !ok {
		return invalid(location, "Match on a dynamic value requires foreign type patterns like pkg::Type(binding) or a catch-all '_'", "expected `pkg::Type(binding)` or `_`")
	}
	nsIdent, ok := p.Target.(*parse.Identifier)
	if !ok {
		return invalid(location, "Foreign type pattern must be qualified as pkg::Type(binding)", "qualify this pattern as `pkg::Type(binding)`")
	}
	goPkg := c.program.GoImports[nsIdent.Name]
	if goPkg == nil {
		c.addUnresolvedReference(unknownGoNamespace, nsIdent.Name, nsIdent.GetLocation())
		ctx.failed = true
		return nil
	}
	typ := goPkg.Types[p.Function.Name]
	if typ == nil {
		if reason := goPkg.UnsupportedTypes[p.Function.Name]; reason != "" {
			qualified := nsIdent.Name + "::" + p.Function.Name
			legacy := fmt.Sprintf("Unsupported Go type %s: %s", qualified, reason)
			c.addDiagnostic(unsupportedGoEntityDiagnostic{Kind: "type", Name: qualified, Reason: reason, Span: c.sourceSpan(location), LegacyMessage: legacy}.build())
		} else {
			c.addUnresolvedReference(unrecognizedType, fmt.Sprintf("%s::%s", nsIdent.Name, p.Function.Name), location)
		}
		ctx.failed = true
		return nil
	}
	foreign, ok := typ.(*ForeignType)
	if !ok || foreign.Interface {
		return invalid(location,
			fmt.Sprintf("Foreign type pattern must name a concrete foreign type, got %s::%s", nsIdent.Name, p.Function.Name),
			"this pattern does not name a concrete foreign type",
		)
	}
	if subjectForeign, ok := subjectType.(*ForeignType); ok {
		if reason := impossibleDynamicType(subjectForeign, foreign); reason != "" {
			return invalid(location,
				fmt.Sprintf("Foreign type pattern %s cannot be the dynamic type of %s (%s)", foreign, subjectForeign, reason),
				fmt.Sprintf("`%s` can never hold a `%s` (%s)", subjectForeign, foreign, reason),
			)
		}
	}
	if len(p.Function.Args) != 1 {
		return invalid(location, "Foreign type pattern requires exactly one binding, like pkg::Type(binding)", "provide exactly one binding")
	}
	arg, ok := singlePatternArg(&p.Function)
	if !ok {
		return invalid(p.Function.Args[0].GetLocation(), "Foreign type pattern binding must be an identifier", "use an identifier binding here")
	}
	if _, isIdent := arg.(*parse.Identifier); !isIdent {
		return invalid(arg.GetLocation(), "Foreign type pattern binding must be an identifier", "use an identifier binding here")
	}
	child := ctx.vars.child(varID, MatchAccessForeign, 0, foreign, foreign)
	inner := c.checkPattern(arg, child, foreign, false, ctx)
	if inner == nil {
		return nil
	}
	return &ForeignTypePattern{Location: location, Type: foreign, Inner: inner}
}

// patternString renders a checked pattern in Ard syntax for diagnostics.
func patternString(p Pattern) string {
	switch p := p.(type) {
	case *WildcardPattern:
		return "_"
	case *BindingPattern:
		if p.Implicit {
			return "_"
		}
		return p.Name
	case *BoolPattern:
		return strconv.FormatBool(p.Value)
	case *IntPattern:
		if p.Rune {
			if p.Start == p.End {
				return strconv.QuoteRune(rune(p.Start))
			}
			return strconv.QuoteRune(rune(p.Start)) + ".." + strconv.QuoteRune(rune(p.End))
		}
		if p.Start == p.End {
			return strconv.Itoa(p.Start)
		}
		return fmt.Sprintf("%d..%d", p.Start, p.End)
	case *StrPattern:
		return strconv.Quote(p.Value)
	case *EnumPattern:
		return fmt.Sprintf("%s::%s", p.Enum.Name, p.Enum.Values[p.Variant].Name)
	case *TuplePattern:
		out := "("
		for i, elem := range p.Elems {
			if i > 0 {
				out += ", "
			}
			out += patternString(elem)
		}
		return out + ")"
	case *ResultPattern:
		name := "err"
		if p.Ok {
			name = "ok"
		}
		return fmt.Sprintf("%s(%s)", name, patternString(p.Inner))
	case *MaybePattern:
		if !p.Some {
			return "none"
		}
		return fmt.Sprintf("some(%s)", patternString(p.Inner))
	case *UnionPattern:
		return fmt.Sprintf("%s(%s)", unionMemberDisplayName(p.Union, p.Member), patternString(p.Inner))
	case *ForeignTypePattern:
		return fmt.Sprintf("%s(%s)", p.Type, patternString(p.Inner))
	}
	return "_"
}

func unionMemberDisplayName(union *Union, member int) string {
	if member < len(union.MemberNames) && union.MemberNames[member] != "" {
		return union.MemberNames[member]
	}
	return union.Types[member].String()
}
