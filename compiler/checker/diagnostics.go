package checker

import (
	"fmt"

	"github.com/akonwi/ard/parse"
)

type DiagnosticKind string

const (
	Error DiagnosticKind = "error"
	Warn  DiagnosticKind = "warn"
)

type DiagnosticCode string

const (
	DiagnosticCodeTypeMismatch              DiagnosticCode = "type_mismatch"
	DiagnosticCodeDuplicateDeclaration      DiagnosticCode = "duplicate_declaration"
	DiagnosticCodeDuplicateFieldDeclaration DiagnosticCode = "duplicate_field_declaration"
	DiagnosticCodeDuplicateImport           DiagnosticCode = "duplicate_import"
	DiagnosticCodeUndefinedMember           DiagnosticCode = "undefined_member"
	DiagnosticCodeUndefinedName             DiagnosticCode = "undefined_name"
)

type SourceSpan struct {
	FilePath string
	Location parse.Location
}

type DiagnosticLabel struct {
	Span    SourceSpan
	Message string
}

type Diagnostic struct {
	// Kind and Message remain the compatibility surface for diagnostic emitters
	// that have not migrated to structured diagnostics yet.
	Kind    DiagnosticKind
	Code    DiagnosticCode
	Message string

	Title     string
	Text      string
	Primary   DiagnosticLabel
	Secondary []DiagnosticLabel
}

func NewDiagnostic(kind DiagnosticKind, message string, filePath string, location parse.Location) Diagnostic {
	return Diagnostic{
		Kind:    kind,
		Message: message,
		Title:   message,
		Primary: DiagnosticLabel{Span: SourceSpan{FilePath: filePath, Location: location}},
	}
}

func newLabeledDiagnostic(kind DiagnosticKind, legacyMessage string, title string, text string, primary DiagnosticLabel, secondary ...DiagnosticLabel) Diagnostic {
	return Diagnostic{
		Kind:      kind,
		Message:   legacyMessage,
		Title:     title,
		Text:      text,
		Primary:   primary,
		Secondary: secondary,
	}
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s %s %s", d.Primary.Span.FilePath, d.Primary.Span.Location.Start, d.Message)
}

func (d Diagnostic) FilePath() string {
	return d.Primary.Span.FilePath
}

func (d Diagnostic) Location() parse.Location {
	return d.Primary.Span.Location
}

type undefinedNameKind uint8

const (
	undefinedVariable undefinedNameKind = iota
	undefinedFunction
)

type undefinedNameDiagnostic struct {
	Kind undefinedNameKind
	Name string
	Span SourceSpan
}

func (d undefinedNameDiagnostic) build() Diagnostic {
	var nameKind string
	switch d.Kind {
	case undefinedVariable:
		nameKind = "variable"
	case undefinedFunction:
		nameKind = "function"
	default:
		panic(fmt.Sprintf("unknown undefined-name kind: %d", d.Kind))
	}
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Undefined %s: %s", nameKind, d.Name),
		fmt.Sprintf("Undefined %s", nameKind),
		"",
		DiagnosticLabel{
			Span:    d.Span,
			Message: fmt.Sprintf("`%s` is not defined in this scope", d.Name),
		},
	)
	diagnostic.Code = DiagnosticCodeUndefinedName
	return diagnostic
}

type undefinedMemberKind uint8

const (
	undefinedField undefinedMemberKind = iota
	undefinedMethod
)

type undefinedMemberDiagnostic struct {
	Kind     undefinedMemberKind
	Receiver string
	Member   string
	Span     SourceSpan
}

func (d undefinedMemberDiagnostic) build() Diagnostic {
	var memberKind string
	switch d.Kind {
	case undefinedField:
		memberKind = "field"
	case undefinedMethod:
		memberKind = "method"
	default:
		panic(fmt.Sprintf("unknown undefined-member kind: %d", d.Kind))
	}
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Undefined: %s.%s", d.Receiver, d.Member),
		fmt.Sprintf("Undefined %s", memberKind),
		"",
		DiagnosticLabel{
			Span:    d.Span,
			Message: fmt.Sprintf("`%s` is not defined for `%s`", d.Member, d.Receiver),
		},
	)
	diagnostic.Code = DiagnosticCodeUndefinedMember
	return diagnostic
}

type duplicateImportDiagnostic struct {
	Name          string
	DuplicateSpan SourceSpan
	OriginalSpan  SourceSpan
}

func (d duplicateImportDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Warn,
		fmt.Sprintf("%s Duplicate import: %s", d.DuplicateSpan.Location.Start, d.Name),
		"Duplicate import",
		"",
		DiagnosticLabel{
			Span:    d.DuplicateSpan,
			Message: fmt.Sprintf("`%s` is imported again here", d.Name),
		},
		DiagnosticLabel{
			Span:    d.OriginalSpan,
			Message: "first imported here",
		},
	)
	diagnostic.Code = DiagnosticCodeDuplicateImport
	return diagnostic
}

type duplicateFieldDeclarationDiagnostic struct {
	Name          string
	DuplicateSpan SourceSpan
	OriginalSpan  SourceSpan
}

func (d duplicateFieldDeclarationDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Duplicate field: %s", d.Name),
		"Duplicate field declaration",
		"",
		DiagnosticLabel{
			Span:    d.DuplicateSpan,
			Message: fmt.Sprintf("field `%s` is declared again here", d.Name),
		},
		DiagnosticLabel{
			Span:    d.OriginalSpan,
			Message: "first declared here",
		},
	)
	diagnostic.Code = DiagnosticCodeDuplicateFieldDeclaration
	return diagnostic
}

type duplicateDeclarationDiagnostic struct {
	Name          string
	DuplicateSpan SourceSpan
	OriginalSpan  SourceSpan
}

func (d duplicateDeclarationDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Duplicate declaration: %s", d.Name),
		"Duplicate declaration",
		"",
		DiagnosticLabel{
			Span:    d.DuplicateSpan,
			Message: fmt.Sprintf("`%s` is declared again here", d.Name),
		},
		DiagnosticLabel{
			Span:    d.OriginalSpan,
			Message: "first declared here",
		},
	)
	diagnostic.Code = DiagnosticCodeDuplicateDeclaration
	return diagnostic
}

type typeMismatchDiagnostic struct {
	Expected    Type
	Actual      Type
	ActualSpan  SourceSpan
	Expectation *typeExpectation
}

type typeExpectation struct {
	Span SourceSpan
	Kind typeExpectationKind
}

type typeExpectationKind uint8

const (
	expectationUnknown typeExpectationKind = iota
	expectationAnnotation
)

func (d typeMismatchDiagnostic) build() Diagnostic {
	primaryMessage := fmt.Sprintf("this expression has type `%s`", d.Actual)
	if d.Expectation == nil {
		primaryMessage = fmt.Sprintf("expected `%s`, but this expression has type `%s`", d.Expected, d.Actual)
	}

	secondary := make([]DiagnosticLabel, 0, 1)
	if d.Expectation != nil {
		message := fmt.Sprintf("this requires `%s`", d.Expected)
		if d.Expectation.Kind == expectationAnnotation {
			message = fmt.Sprintf("this annotation requires `%s`", d.Expected)
		}
		secondary = append(secondary, DiagnosticLabel{Span: d.Expectation.Span, Message: message})
	}

	diagnostic := newLabeledDiagnostic(
		Error,
		typeMismatch(d.Expected, d.Actual),
		"Type mismatch",
		"",
		DiagnosticLabel{Span: d.ActualSpan, Message: primaryMessage},
		secondary...,
	)
	diagnostic.Code = DiagnosticCodeTypeMismatch
	return diagnostic
}
