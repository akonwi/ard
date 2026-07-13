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
	DiagnosticCodeUndefinedType             DiagnosticCode = "undefined_type"
	DiagnosticCodeUndefinedTrait            DiagnosticCode = "undefined_trait"
	DiagnosticCodeUndefinedModule           DiagnosticCode = "undefined_module"
	DiagnosticCodeUndefinedNamespace        DiagnosticCode = "undefined_namespace"
	DiagnosticCodeUnknownField              DiagnosticCode = "unknown_field"
	DiagnosticCodeUndefinedQualifiedMember  DiagnosticCode = "undefined_qualified_member"
	DiagnosticCodeUndefinedGoFunction       DiagnosticCode = "undefined_go_function"
	DiagnosticCodeUndefinedEnumVariant      DiagnosticCode = "undefined_enum_variant"
	DiagnosticCodeInvalidStaticMember       DiagnosticCode = "invalid_static_member"
	DiagnosticCodeNotAStruct                DiagnosticCode = "not_a_struct"
	DiagnosticCodeImmutableAssignment       DiagnosticCode = "immutable_assignment"
	DiagnosticCodeIncorrectArgumentType     DiagnosticCode = "incorrect_argument_type"
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

type unresolvedReferenceKind uint8

const (
	unrecognizedType unresolvedReferenceKind = iota
	unrecognizedReturnType
	undefinedType
	undefinedTrait
	unknownModule
	undefinedModule
	unknownGoNamespace
	unknownStructField
	undefinedAssignmentTarget
	undefinedQualifiedMember
	undefinedGoFunction
	undefinedGoType
	undefinedStaticRoot
	undefinedEnumVariant
	invalidStaticMember
	undefinedStructType
	notAStruct
)

type unresolvedReferenceDiagnostic struct {
	Kind unresolvedReferenceKind
	Name string
	Span SourceSpan
}

func (d unresolvedReferenceDiagnostic) build() Diagnostic {
	var code DiagnosticCode
	var message, title, label string
	switch d.Kind {
	case unrecognizedType:
		code, message, title = DiagnosticCodeUndefinedType, "Unrecognized type: "+d.Name, "Unrecognized type"
		label = fmt.Sprintf("type `%s` could not be resolved", d.Name)
	case unrecognizedReturnType:
		code, message, title = DiagnosticCodeUndefinedType, "Unrecognized return type: "+d.Name, "Unrecognized return type"
		label = fmt.Sprintf("return type `%s` could not be resolved", d.Name)
	case undefinedType:
		code, message, title = DiagnosticCodeUndefinedType, "Undefined type: "+d.Name, "Undefined type"
		label = fmt.Sprintf("type `%s` is not defined", d.Name)
	case undefinedTrait:
		code, message, title = DiagnosticCodeUndefinedTrait, "Undefined trait: "+d.Name, "Undefined trait"
		label = fmt.Sprintf("trait `%s` is not defined", d.Name)
	case unknownModule:
		code, message, title = DiagnosticCodeUndefinedModule, "Unknown module: "+d.Name, "Unknown module"
		label = fmt.Sprintf("module `%s` could not be resolved", d.Name)
	case undefinedModule:
		code, message, title = DiagnosticCodeUndefinedModule, "Undefined module: "+d.Name, "Undefined module"
		label = fmt.Sprintf("module `%s` is not defined", d.Name)
	case unknownGoNamespace:
		code, message, title = DiagnosticCodeUndefinedNamespace, "Unknown Go namespace: "+d.Name, "Unknown Go namespace"
		label = fmt.Sprintf("Go namespace `%s` could not be resolved", d.Name)
	case unknownStructField:
		code, message, title = DiagnosticCodeUnknownField, "Unknown field: "+d.Name, "Unknown field"
		label = fmt.Sprintf("`%s` is not a field of this struct", d.Name)
	case undefinedAssignmentTarget:
		code, message, title = DiagnosticCodeUndefinedName, "Undefined: "+d.Name, "Undefined assignment target"
		label = fmt.Sprintf("`%s` is not defined in this scope", d.Name)
	case undefinedQualifiedMember:
		code, message, title = DiagnosticCodeUndefinedQualifiedMember, "Undefined: "+d.Name, "Undefined qualified member"
		label = fmt.Sprintf("`%s` could not be resolved", d.Name)
	case undefinedGoFunction:
		code, message, title = DiagnosticCodeUndefinedGoFunction, "Undefined Go function: "+d.Name, "Undefined Go function"
		label = fmt.Sprintf("Go function `%s` could not be resolved", d.Name)
	case undefinedGoType:
		code, message, title = DiagnosticCodeUndefinedType, "Undefined Go type: "+d.Name, "Undefined Go type"
		label = fmt.Sprintf("Go type `%s` could not be used here", d.Name)
	case undefinedStaticRoot:
		code, message, title = DiagnosticCodeUndefinedName, "Undefined: "+d.Name, "Undefined name"
		label = fmt.Sprintf("`%s` is not defined in this scope", d.Name)
	case undefinedEnumVariant:
		code, message, title = DiagnosticCodeUndefinedEnumVariant, "Undefined: "+d.Name, "Undefined enum variant"
		label = fmt.Sprintf("enum variant `%s` is not defined", d.Name)
	case invalidStaticMember:
		code, message, title = DiagnosticCodeInvalidStaticMember, "Undefined: "+d.Name, "Invalid static member"
		label = fmt.Sprintf("`%s` is not available as a static member", d.Name)
	case undefinedStructType:
		code, message, title = DiagnosticCodeUndefinedType, "Undefined: "+d.Name, "Undefined struct type"
		label = fmt.Sprintf("struct type `%s` is not defined", d.Name)
	case notAStruct:
		code, message, title = DiagnosticCodeNotAStruct, "Undefined: "+d.Name, "Not a struct"
		label = fmt.Sprintf("`%s` does not name a struct", d.Name)
	default:
		panic(fmt.Sprintf("unknown unresolved-reference kind: %d", d.Kind))
	}
	diagnostic := newLabeledDiagnostic(Error, message, title, "", DiagnosticLabel{Span: d.Span, Message: label})
	diagnostic.Code = code
	return diagnostic
}

func (c *Checker) addUnresolvedReference(kind unresolvedReferenceKind, name string, location parse.Location) {
	c.addDiagnostic(unresolvedReferenceDiagnostic{
		Kind: kind,
		Name: name,
		Span: c.sourceSpan(location),
	}.build())
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

type immutableAssignmentDiagnostic struct {
	Name            string
	AssignmentSpan  SourceSpan
	DeclarationSpan SourceSpan
}

func (d immutableAssignmentDiagnostic) build() Diagnostic {
	secondary := []DiagnosticLabel{}
	if d.DeclarationSpan.FilePath != "" {
		secondary = append(secondary, DiagnosticLabel{
			Span:    d.DeclarationSpan,
			Message: fmt.Sprintf("`%s` was declared immutable here", d.Name),
		})
	}
	diagnostic := newLabeledDiagnostic(
		Error,
		"Immutable variable: "+d.Name,
		"Cannot assign to immutable variable",
		"",
		DiagnosticLabel{
			Span:    d.AssignmentSpan,
			Message: fmt.Sprintf("cannot assign to `%s`", d.Name),
		},
		secondary...,
	)
	diagnostic.Code = DiagnosticCodeImmutableAssignment
	return diagnostic
}

type incorrectArgumentTypeDiagnostic struct {
	LegacyMessage   string
	Expected        Type
	Actual          Type
	ArgumentSpan    SourceSpan
	ParameterName   string
	ParameterSpan   *SourceSpan
	RequiresMutable bool
}

func (d incorrectArgumentTypeDiagnostic) build() Diagnostic {
	primaryMessage := fmt.Sprintf("this argument has type `%s`", d.Actual)
	if d.RequiresMutable {
		primaryMessage = "this argument is not mutable"
	}

	secondary := make([]DiagnosticLabel, 0, 1)
	if d.ParameterSpan != nil {
		message := fmt.Sprintf("parameter `%s` requires `%s`", d.ParameterName, d.Expected)
		if d.RequiresMutable {
			message = fmt.Sprintf("parameter `%s` requires a mutable `%s`", d.ParameterName, d.Expected)
		}
		secondary = append(secondary, DiagnosticLabel{Span: *d.ParameterSpan, Message: message})
	} else if !d.RequiresMutable {
		primaryMessage = fmt.Sprintf("expected `%s`, but this argument has type `%s`", d.Expected, d.Actual)
	}

	diagnostic := newLabeledDiagnostic(
		Error,
		d.LegacyMessage,
		"Incorrect argument type",
		"",
		DiagnosticLabel{Span: d.ArgumentSpan, Message: primaryMessage},
		secondary...,
	)
	diagnostic.Code = DiagnosticCodeIncorrectArgumentType
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
