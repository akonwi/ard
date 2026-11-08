package checker

import "github.com/akonwi/ard/parse"

// legacyPointerSyntaxKind names an ADR 0057 reference spelling that ADR 0073
// removed. The parser still accepts these spellings so the checker can
// reject each one with a diagnostic that names its replacement.
type legacyPointerSyntaxKind uint8

const (
	// legacyPointerType is `mut T` for a non-trait T.
	legacyPointerType legacyPointerSyntaxKind = iota
	// legacyRedundantTypeMut is `mut` on a type that is already a pointer.
	legacyRedundantTypeMut
	// legacyAddressOf is `mut expression`.
	legacyAddressOf
	// legacyRedundantMut is `mut` on an expression that is already a pointer.
	legacyRedundantMut
	// legacyTraitBorrow is `mut` on a trait-typed expression.
	legacyTraitBorrow
)

// removedPointerSyntaxDiagnostic rejects a removed ADR 0057 reference
// spelling and says what to write instead (ADR 0073).
type removedPointerSyntaxDiagnostic struct {
	Kind legacyPointerSyntaxKind
	Span SourceSpan
}

func (d removedPointerSyntaxDiagnostic) build() Diagnostic {
	var message, title, text, label string
	switch d.Kind {
	case legacyPointerType:
		message = "Removed pointer type syntax: write `&mut T` instead of `mut T`"
		title = "Removed `mut T` pointer type"
		text = "Writable pointer types are spelled `&mut T` (ADR 0073). In type position, `mut` is reserved for `mut Trait`."
		label = "write `&mut` here"
	case legacyRedundantTypeMut:
		message = "Removed pointer type syntax: `mut` on a pointer type"
		title = "`mut` on a pointer type"
		text = "This type is already a pointer (ADR 0073). Remove `mut`."
		label = "remove `mut`"
	case legacyAddressOf:
		message = "Removed pointer syntax: write `&mut expression` instead of `mut expression`"
		title = "Removed `mut` expression"
		text = "Pointers are created with `&mut` (ADR 0073). `&mut` requires a writable place, such as a `mut` binding or a field of one, or a fresh value."
		label = "write `&mut` here"
	case legacyRedundantMut:
		message = "Removed pointer syntax: `mut` on a pointer"
		title = "`mut` on a pointer"
		text = "This expression is already a pointer. Use it directly (ADR 0073)."
		label = "remove `mut`"
	case legacyTraitBorrow:
		message = "Removed pointer syntax: `mut` on a trait value"
		title = "`mut` on a trait value"
		text = "`mut Trait` values are created by widening a `&mut T` pointer (ADR 0073). Store a `mut Trait` value instead of borrowing a trait-typed place."
		label = "borrows a trait-typed place"
	}
	diagnostic := newLabeledDiagnostic(Error, message, title, text, DiagnosticLabel{Span: d.Span, Message: label})
	diagnostic.Code = DiagnosticCodeRemovedPointerSyntax
	return diagnostic
}

// removedPointerSyntaxKey identifies one removed-syntax report.
type removedPointerSyntaxKey struct {
	kind legacyPointerSyntaxKind
	span SourceSpan
}

// reportLegacyPointerSyntax records one diagnostic per source location.
// Types and expressions can be resolved more than once (signature pre-passes,
// contextual re-checks), so a diagnostic already present is not repeated.
// Speculative checks truncate or filter the diagnostics, so the index records
// where each report was stored and is trusted only while that slot still
// holds it; otherwise the current diagnostics are scanned.
func (c *Checker) reportLegacyPointerSyntax(kind legacyPointerSyntaxKind, location parse.Location) {
	diagnostic := removedPointerSyntaxDiagnostic{Kind: kind, Span: c.sourceSpan(location)}.build()
	key := removedPointerSyntaxKey{kind: kind, span: diagnostic.Primary.Span}
	if c.removedPointerSyntax == nil {
		c.removedPointerSyntax = map[removedPointerSyntaxKey]int{}
	}
	if index, ok := c.removedPointerSyntax[key]; ok {
		if index < len(c.diagnostics) && sameRemovedPointerSyntax(c.diagnostics[index], diagnostic) {
			return
		}
		for i, existing := range c.diagnostics {
			if sameRemovedPointerSyntax(existing, diagnostic) {
				c.removedPointerSyntax[key] = i
				return
			}
		}
	}
	c.removedPointerSyntax[key] = len(c.diagnostics)
	c.addDiagnostic(diagnostic)
}

func sameRemovedPointerSyntax(existing, diagnostic Diagnostic) bool {
	return existing.Code == diagnostic.Code && existing.Message == diagnostic.Message && existing.Primary.Span == diagnostic.Primary.Span
}

// reportLegacyPointerType rejects a `mut T` annotation. `mut Trait` is the one
// remaining type-level `mut` form and is accepted.
func (c *Checker) reportLegacyPointerType(location parse.Location, inner Type) {
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
		c.reportLegacyPointerSyntax(legacyRedundantTypeMut, location)
		return
	}
	c.reportLegacyPointerSyntax(legacyPointerType, location)
}

// reportLegacyAddressOf rejects a `mut <operand>` expression.
func (c *Checker) reportLegacyAddressOf(s *parse.MutRef, operand Expression) {
	location := s.GetLocation()
	if isReferenceValued(operand) {
		c.reportLegacyPointerSyntax(legacyRedundantMut, location)
		return
	}
	if _, isTrait := operand.Type().(*Trait); isTrait {
		c.reportLegacyPointerSyntax(legacyTraitBorrow, location)
		return
	}
	c.reportLegacyPointerSyntax(legacyAddressOf, location)
}
