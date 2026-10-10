package checker

import (
	"strings"

	"github.com/akonwi/ard/parse"
)

// TextEdit is a machine-applicable source change attached to a diagnostic.
// Start is inclusive and End is exclusive; both use the parser's 1-based
// byte columns. An insertion has Start == End.
type TextEdit struct {
	FilePath string
	Start    parse.Point
	End      parse.Point
	NewText  string
	// TrimTrailingSpace extends the edit over spaces and tabs that directly
	// follow End, so removing a keyword does not leave a stray gap.
	TrimTrailingSpace bool
}

// Fixes returns the machine-applicable edits that resolve the diagnostic. Only
// edits that preserve the program's meaning are attached; a diagnostic without
// fixes needs a manual change.
func (d Diagnostic) Fixes() []TextEdit {
	return d.fixes
}

type legacyPointerSyntaxKind uint8

const (
	legacyPointerType legacyPointerSyntaxKind = iota
	legacyRedundantTypeMut
	legacyAddressOf
	legacyRedundantMut
	legacyTraitBorrow
	legacyDeref
	legacyTraitSnapshot
)

type legacyPointerSyntaxKey struct {
	location parse.Location
	kind     legacyPointerSyntaxKind
}

// legacyBorrowFunction identifies the body that owns a value parameter. It
// lets migration insert a writable shadow at the start of that body, rather
// than at an individual borrow site (which could be inside a loop).
type legacyBorrowFunction struct {
	declaration parse.Location
	firstBody   parse.Point
	function    *legacyBorrowFunction
}

type legacyBorrowParameter struct {
	name     string
	function *legacyBorrowFunction
}

func newLegacyBorrowFunction(declaration parse.Location, body []parse.Statement) *legacyBorrowFunction {
	function := &legacyBorrowFunction{declaration: declaration}
	if len(body) > 0 && body[0] != nil {
		function.firstBody = body[0].GetLocation().Start
	}
	return function
}

// checkLegacyBorrowBlock checks a body whose immutable bindings may be
// shadowed at its start by migration. Keeping the context active throughout
// the body prevents a nested closure from receiving a fix for a capture.
func (c *Checker) checkLegacyBorrowBlock(declaration parse.Location, body []parse.Statement, check func() *Block) *Block {
	previous := c.legacyBorrowFunction
	context := newLegacyBorrowFunction(declaration, body)
	context.function = c.legacyBorrowFunctionScope
	c.legacyBorrowFunction = context
	defer func() { c.legacyBorrowFunction = previous }()
	return check()
}

func (c *Checker) checkLoopBody(declaration parse.Location, body []parse.Statement, setup func()) *Block {
	return c.checkLegacyBorrowBlock(declaration, body, func() *Block {
		return c.checkBlock(body, c.markLoopScope(setup))
	})
}

func (c *Checker) legacyBorrowBinding(sym *Symbol) *Symbol {
	if c.legacyBorrowFunction != nil {
		sym.legacyBorrowParameter = &legacyBorrowParameter{name: sym.Name, function: c.legacyBorrowFunction}
	}
	return sym
}

// checkMatchArmBlockWithLegacyBinding handles multiline arm bodies only.
// Expression arms are left manual rather than changing their expression/value
// behavior by introducing a block.
func matchCaseAllowsLegacyBorrowShadow(matchCase parse.MatchCase) bool {
	// The parser deliberately flattens an arm body, so a one-statement body
	// does not reveal whether it was an expression arm or a braced block.
	// Leave that ambiguous case manual rather than inserting an invalid
	// declaration before an expression.
	return len(matchCase.Body) > 1
}

func (c *Checker) checkMatchArmBlockWithLegacyBinding(matchCase parse.MatchCase, setup func()) *Block {
	if !matchCaseAllowsLegacyBorrowShadow(matchCase) {
		return c.checkMatchArmBlock(matchCase.Body, setup)
	}
	return c.checkLegacyBorrowBlock(matchCase.GetLocation(), matchCase.Body, func() *Block {
		return c.checkMatchArmBlock(matchCase.Body, setup)
	})
}

// deprecatedPointerSyntaxDiagnostic reports a legacy ADR 0057 reference form
// that ADR 0073 replaces with explicit pointer syntax. The legacy forms keep
// working for one release; fixes rewrite them mechanically when the rewrite
// preserves meaning.
type deprecatedPointerSyntaxDiagnostic struct {
	Kind  legacyPointerSyntaxKind
	Span  SourceSpan
	Fixes []TextEdit
}

func (d deprecatedPointerSyntaxDiagnostic) build() Diagnostic {
	var message, title, text, label string
	switch d.Kind {
	case legacyPointerType:
		message = "Deprecated pointer type syntax: write `&mut T` instead of `mut T`"
		title = "Deprecated `mut T` pointer type"
		text = "Writable pointer types are spelled `&mut T` (ADR 0073). In type position, `mut` is reserved for `mut Trait`."
		label = "write `&mut` here"
	case legacyRedundantTypeMut:
		message = "Deprecated pointer type syntax: `mut` on a pointer type is redundant"
		title = "Redundant `mut` on a pointer type"
		text = "This type is already a pointer (ADR 0073)."
		label = "remove `mut`"
		if len(d.Fixes) == 0 {
			text += " Here `mut` adds a second pointer layer, which has no `&mut` spelling; change the type manually."
		}
	case legacyAddressOf:
		message = "Deprecated pointer syntax: write `&mut expression` instead of `mut expression`"
		title = "Deprecated `mut` expression"
		text = "Pointers are created with `&mut` (ADR 0073)."
		label = "write `&mut` here"
		if len(d.Fixes) == 0 {
			text += " `&mut` requires a writable place, so this expression needs a manual change: make the binding `mut`, or copy the value into a `mut` local first."
		}
	case legacyRedundantMut:
		message = "Deprecated pointer syntax: `mut` on a pointer is redundant"
		title = "Redundant `mut` on a pointer"
		text = "This expression is already a pointer. Use it directly (ADR 0073)."
		label = "remove `mut`"
	case legacyTraitBorrow:
		message = "Deprecated pointer syntax: `mut` on a trait value has no replacement"
		title = "Deprecated `mut` trait borrow"
		text = "`mut Trait` values are created by widening a `&mut T` pointer (ADR 0073). Store a `mut Trait` value instead of borrowing a trait-typed place."
		label = "borrows a trait-typed place"
	case legacyDeref:
		message = "Deprecated dereference syntax: write `.*` instead of `.@`"
		title = "Deprecated `.@` dereference"
		text = "Pointers are dereferenced with postfix `.*` (ADR 0073)."
		label = "write `.*` here"
		if len(d.Fixes) == 0 {
			text += " Here `.@` copies the pointee before it is borrowed, while `.*` names the pointee itself, so bind the copy to a local first."
		}
	case legacyTraitSnapshot:
		message = "Deprecated dereference syntax: `.@` on a `mut Trait` value will be removed"
		title = "Deprecated trait snapshot"
		text = "A `mut Trait` value is not a pointer, so it has no `.*` form (ADR 0073). Copy the concrete value with `pointer.*` before widening it to a trait."
		label = "snapshots a trait value"
	}
	diagnostic := newLabeledDiagnostic(Warn, message, title, text, DiagnosticLabel{Span: d.Span, Message: label})
	diagnostic.Code = DiagnosticCodeDeprecatedPointerSyntax
	diagnostic.fixes = d.Fixes
	return diagnostic
}

// reportLegacyPointerSyntax records one deprecation per source location. Types
// and expressions can be resolved more than once (signature pre-passes,
// contextual re-checks), so duplicates are dropped.
func (c *Checker) reportLegacyPointerSyntax(kind legacyPointerSyntaxKind, location parse.Location, fixes ...TextEdit) {
	key := legacyPointerSyntaxKey{location: location, kind: kind}
	if c.reportedLegacyPointerSyntax == nil {
		c.reportedLegacyPointerSyntax = map[legacyPointerSyntaxKey]bool{}
	}
	if c.reportedLegacyPointerSyntax[key] {
		return
	}
	c.reportedLegacyPointerSyntax[key] = true
	c.addDiagnostic(deprecatedPointerSyntaxDiagnostic{Kind: kind, Span: c.sourceSpan(location), Fixes: fixes}.build())
}

func (c *Checker) insertEdit(at parse.Point, text string) TextEdit {
	return TextEdit{FilePath: c.filePath, Start: at, End: at, NewText: text}
}

// keywordEdit replaces a keyword token that starts at start.
func (c *Checker) keywordEdit(start parse.Point, keyword string, text string) TextEdit {
	end := parse.Point{Row: start.Row, Col: start.Col + len(keyword)}
	return TextEdit{FilePath: c.filePath, Start: start, End: end, NewText: text}
}

// reportLegacyPointerType reports a legacy `mut T` annotation whose `mut`
// keyword starts at mutStart. `mut Trait` is the one remaining type-level
// `mut` form and is not deprecated. In parameter position `mut` on a type
// that is already a pointer is idempotent, so it can simply be removed.
func (c *Checker) reportLegacyPointerType(location parse.Location, mutStart parse.Point, inner Type, parameter bool) {
	if inner == nil {
		return
	}
	if typeVar, ok := inner.(*TypeVar); ok && typeVar.name == "unknown" {
		// An unresolved referent might be a trait, which keeps `mut`.
		return
	}
	if _, isTrait := inner.(*Trait); isTrait {
		return
	}
	if isReferenceType(inner) {
		if !parameter {
			c.reportLegacyPointerSyntax(legacyRedundantTypeMut, location)
			return
		}
		removal := c.keywordEdit(mutStart, "mut", "")
		removal.TrimTrailingSpace = true
		c.reportLegacyPointerSyntax(legacyRedundantTypeMut, location, removal)
		return
	}
	c.reportLegacyPointerSyntax(legacyPointerType, location, c.insertEdit(mutStart, "&"))
}

// reportLegacyAddressOf reports a legacy `mut <operand>` expression and, when
// the rewrite keeps its meaning, attaches the `&mut` fix.
func (c *Checker) reportLegacyAddressOf(s *parse.MutRef, operand Expression) {
	location := s.GetLocation()
	if isReferenceValued(operand) {
		c.reportLegacyPointerSyntax(legacyRedundantMut, location, TextEdit{
			FilePath:          c.filePath,
			Start:             location.Start,
			End:               parse.Point{Row: location.Start.Row, Col: location.Start.Col + len("mut")},
			TrimTrailingSpace: true,
		})
		return
	}
	if _, isTrait := operand.Type().(*Trait); isTrait {
		c.reportLegacyPointerSyntax(legacyTraitBorrow, location)
		return
	}
	if c.legacyDescriptorArgument == s && c.isAddressablePlace(operand) && !c.isWritablePlace(operand) {
		// Go []T and map[K]V parameters take a descriptor value. For an
		// immutable operand, preserving the legacy borrow would require an
		// unnecessary writable copy; pass the value directly instead.
		removal := c.keywordEdit(location.Start, "mut", "")
		removal.TrimTrailingSpace = true
		c.reportLegacyPointerSyntax(legacyAddressOf, location, removal)
		return
	}
	if legacyDeref, ok := s.Operand.(*parse.Deref); ok && !legacyDeref.Star {
		// `mut pointer.@` borrows a fresh copy of the pointee, but
		// `&mut pointer.*` would alias it. Neither half rewrites
		// mechanically.
		c.reportLegacyPointerSyntax(legacyAddressOf, location)
		return
	}
	fixes, ok := c.legacyAddressOfFixes(s, operand)
	if !ok {
		c.reportLegacyPointerSyntax(legacyAddressOf, location)
		return
	}
	c.reportLegacyPointerSyntax(legacyAddressOf, location, fixes...)
}

func (c *Checker) legacyAddressOfFixes(s *parse.MutRef, operand Expression) ([]TextEdit, bool) {
	var fixes []TextEdit
	if c.isAddressablePlace(operand) && !c.isWritablePlace(operand) {
		// ADR 0057 borrowed `let` bindings; `&mut` requires a writable
		// place. Making the root binding `mut` keeps the program valid.
		if root := c.legacyBorrowRootLet(operand); root != nil {
			fixes = append(fixes, c.keywordEdit(*root, "let", "mut"))
		} else if parameter := c.legacyBorrowRootParameter(operand); parameter != nil {
			fixes = append(fixes, c.legacyBorrowParameterShadow(*parameter))
		} else {
			return nil, false
		}
	} else if !c.isAddressablePlace(operand) && isPlaceExpression(operand) {
		return nil, false
	}
	start := s.GetLocation().Start
	if addressOfOperandNeedsParens(s.Operand) {
		end := s.Operand.GetLocation().End
		fixes = append(fixes,
			TextEdit{FilePath: c.filePath, Start: start, End: parse.Point{Row: start.Row, Col: start.Col + len("mut")}, NewText: "&mut (", TrimTrailingSpace: true},
			c.insertEdit(parse.Point{Row: end.Row, Col: end.Col + 1}, ")"),
		)
		return fixes, true
	}
	return append(fixes, c.insertEdit(start, "&")), true
}

// isPlaceExpression reports whether expr names storage rather than computing
// a fresh value.
func isPlaceExpression(expr Expression) bool {
	switch expr.(type) {
	case *Variable, *InstanceProperty, *ForeignFieldAccess, *ForeignValue, *ModuleSymbol:
		return true
	}
	return false
}

// legacyBorrowRootLet returns the `let` keyword of the local binding that a
// legacy borrow reaches through inline fields only, or nil when the borrow
// cannot be made writable by changing that binding.
func (c *Checker) legacyBorrowRootLet(expr Expression) *parse.Point {
	root := legacyBorrowRootVariable(expr)
	if root == nil {
		return nil
	}
	return root.sym.letKeyword
}

// legacyBorrowRootParameter returns a value parameter reached through inline
// fields. Captures are intentionally left manual: the shadow would be outside
// the closure that reports this diagnostic, and changing capture storage is
// not a mechanical rewrite.
func (c *Checker) legacyBorrowRootParameter(expr Expression) *legacyBorrowParameter {
	root := legacyBorrowRootVariable(expr)
	if root == nil || root.sym.legacyBorrowParameter == nil || c.legacyBorrowFunctionScope != root.sym.legacyBorrowParameter.function.function {
		return nil
	}
	return root.sym.legacyBorrowParameter
}

func legacyBorrowRootVariable(expr Expression) *Variable {
	for {
		switch e := expr.(type) {
		case *Variable:
			return e
		case *InstanceProperty:
			if isReferenceValued(e.Subject) {
				return nil
			}
			expr = e.Subject
		case *ForeignFieldAccess:
			if isReferenceValued(e.Subject) {
				return nil
			}
			expr = e.Subject
		default:
			return nil
		}
	}
}

func (c *Checker) legacyBorrowParameterShadow(parameter legacyBorrowParameter) TextEdit {
	at := parameter.function.firstBody
	indent := ""
	if at.Row != parameter.function.declaration.Start.Row {
		indent = strings.Repeat(" ", at.Col-1)
	}
	return c.insertEdit(at, "mut "+parameter.name+" = "+parameter.name+"\n"+indent)
}

// addressOfOperandNeedsParens reports whether operand would no longer be the
// whole operand once legacy `mut` (loose precedence) becomes `&mut` (unary
// precedence).
func addressOfOperandNeedsParens(operand parse.Expression) bool {
	switch operand.(type) {
	case *parse.Identifier, *parse.InstanceProperty, *parse.InstanceMethod, *parse.FunctionCall,
		*parse.StaticFunction, *parse.StaticProperty, *parse.StructInstance, *parse.ListLiteral,
		*parse.MapLiteral, *parse.Deref, *parse.MutRef, *parse.StrLiteral, *parse.NumLiteral,
		*parse.BoolLiteral, *parse.RuneLiteral, *parse.VoidLiteral, *parse.UnaryExpression,
		*parse.InterpolatedStr, *parse.FunctionValueCall:
		return false
	}
	return true
}

// reportLegacyDeref reports a legacy `.@` dereference.
func (c *Checker) reportLegacyDeref(s *parse.Deref, operandType Type) {
	if ref, ok := operandType.(*MutableRef); ok {
		if _, isTrait := ref.Of().(*Trait); isTrait {
			c.reportLegacyPointerSyntax(legacyTraitSnapshot, s.OperatorLocation)
			return
		}
	}
	if c.legacyDerefBorrowed[s] {
		c.reportLegacyPointerSyntax(legacyDeref, s.OperatorLocation)
		return
	}
	at := s.OperatorLocation.End
	c.reportLegacyPointerSyntax(legacyDeref, s.OperatorLocation, TextEdit{
		FilePath: c.filePath,
		Start:    at,
		End:      parse.Point{Row: at.Row, Col: at.Col + 1},
		NewText:  "*",
	})
}

// markLegacyDerefBorrow records that a legacy `.@` is the direct operand of a
// borrow, where `.@` and `.*` differ: the former borrows a copy.
func (c *Checker) markLegacyDerefBorrow(s *parse.MutRef) {
	deref, ok := s.Operand.(*parse.Deref)
	if !ok || deref.Star {
		return
	}
	if c.legacyDerefBorrowed == nil {
		c.legacyDerefBorrowed = map[*parse.Deref]bool{}
	}
	c.legacyDerefBorrowed[deref] = true
}
