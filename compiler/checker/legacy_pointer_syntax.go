package checker

import "github.com/akonwi/ard/parse"

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
		message = "Deprecated pointer type syntax: write `*mut T` instead of `mut T`"
		title = "Deprecated `mut T` pointer type"
		text = "Writable pointer types are spelled `*mut T` (ADR 0073). In type position, `mut` is reserved for `mut Trait`."
		label = "write `*mut` here"
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
		text = "`mut Trait` values are created by widening a `*mut T` pointer (ADR 0073). Store a `mut Trait` value instead of borrowing a trait-typed place."
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
// `mut` form and is not deprecated.
func (c *Checker) reportLegacyPointerType(location parse.Location, mutStart parse.Point, inner Type) {
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
		return
	}
	c.reportLegacyPointerSyntax(legacyPointerType, location, c.insertEdit(mutStart, "*"))
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
		root := c.legacyBorrowRootLet(operand)
		if root == nil {
			return nil, false
		}
		fixes = append(fixes, c.keywordEdit(*root, "let", "mut"))
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
	for {
		switch e := expr.(type) {
		case *Variable:
			return e.sym.letKeyword
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
