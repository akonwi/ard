package checker

import (
	"fmt"
	"strings"

	"github.com/akonwi/ard/parse"
)

type DiagnosticKind string

const (
	Error DiagnosticKind = "error"
	Warn  DiagnosticKind = "warn"
)

type DiagnosticCode string

const (
	DiagnosticCodeTypeMismatch                  DiagnosticCode = "type_mismatch"
	DiagnosticCodeDuplicateDeclaration          DiagnosticCode = "duplicate_declaration"
	DiagnosticCodeDuplicateFieldDeclaration     DiagnosticCode = "duplicate_field_declaration"
	DiagnosticCodeDuplicateImport               DiagnosticCode = "duplicate_import"
	DiagnosticCodeUndefinedMember               DiagnosticCode = "undefined_member"
	DiagnosticCodeUndefinedName                 DiagnosticCode = "undefined_name"
	DiagnosticCodeUndefinedType                 DiagnosticCode = "undefined_type"
	DiagnosticCodeUndefinedTrait                DiagnosticCode = "undefined_trait"
	DiagnosticCodeUndefinedModule               DiagnosticCode = "undefined_module"
	DiagnosticCodeUndefinedNamespace            DiagnosticCode = "undefined_namespace"
	DiagnosticCodeUnknownField                  DiagnosticCode = "unknown_field"
	DiagnosticCodeUndefinedQualifiedMember      DiagnosticCode = "undefined_qualified_member"
	DiagnosticCodeUndefinedGoFunction           DiagnosticCode = "undefined_go_function"
	DiagnosticCodeUndefinedEnumVariant          DiagnosticCode = "undefined_enum_variant"
	DiagnosticCodeInvalidStaticMember           DiagnosticCode = "invalid_static_member"
	DiagnosticCodeNotAStruct                    DiagnosticCode = "not_a_struct"
	DiagnosticCodeImmutableAssignment           DiagnosticCode = "immutable_assignment"
	DiagnosticCodeIncorrectArgumentType         DiagnosticCode = "incorrect_argument_type"
	DiagnosticCodeGoImportResolution            DiagnosticCode = "go_import_resolution"
	DiagnosticCodeImportResolution              DiagnosticCode = "import_resolution"
	DiagnosticCodeCircularImport                DiagnosticCode = "circular_import"
	DiagnosticCodeModuleLoadFailure             DiagnosticCode = "module_load_failure"
	DiagnosticCodeBuiltInTypeRedeclaration      DiagnosticCode = "built_in_type_redeclaration"
	DiagnosticCodeRecursiveTypeAlias            DiagnosticCode = "recursive_type_alias"
	DiagnosticCodeRecursiveStructLayout         DiagnosticCode = "recursive_struct_layout"
	DiagnosticCodeUnresolvedGeneric             DiagnosticCode = "unresolved_generic"
	DiagnosticCodeUnboundGenericTypeArg         DiagnosticCode = "unbound_generic_type_argument"
	DiagnosticCodeNonGenericSpecialization      DiagnosticCode = "non_generic_type_specialization"
	DiagnosticCodeIncorrectTypeArgCount         DiagnosticCode = "incorrect_type_argument_count"
	DiagnosticCodeMissingTypeArguments          DiagnosticCode = "missing_type_arguments"
	DiagnosticCodeRecursiveGenericReference     DiagnosticCode = "recursive_generic_self_reference"
	DiagnosticCodeMethodIntroducedGeneric       DiagnosticCode = "method_introduced_generic_parameter"
	DiagnosticCodeInvalidMapKeyType             DiagnosticCode = "invalid_map_key_type"
	DiagnosticCodeMalformedTypeNode             DiagnosticCode = "internal_malformed_type_node"
	DiagnosticCodeBranchTypeMismatch            DiagnosticCode = "branch_type_mismatch"
	DiagnosticCodeNonExhaustiveValueIf          DiagnosticCode = "non_exhaustive_value_if"
	DiagnosticCodeImmutableMutableReference     DiagnosticCode = "immutable_mutable_reference"
	DiagnosticCodeUnsupportedMutableReference   DiagnosticCode = "unsupported_mutable_reference"
	DiagnosticCodeInvalidForeignPointerBinding  DiagnosticCode = "invalid_foreign_pointer_binding"
	DiagnosticCodeUnreachableReferentAssignment DiagnosticCode = "unreachable_referent_assignment"
	DiagnosticCodeReferenceRebinding            DiagnosticCode = "reference_rebinding"
	DiagnosticCodeImmutablePropertyAssignment   DiagnosticCode = "immutable_property_assignment"
	DiagnosticCodeImmutableReceiver             DiagnosticCode = "immutable_receiver"
	DiagnosticCodeImmutablePointerReceiver      DiagnosticCode = "immutable_pointer_receiver"
	DiagnosticCodeGoConstantAssignment          DiagnosticCode = "go_constant_assignment"
	DiagnosticCodeNonAssignableStaticProperty   DiagnosticCode = "non_assignable_static_property"
	DiagnosticCodeNotCallable                   DiagnosticCode = "not_callable"
	DiagnosticCodeIncorrectArgumentCount        DiagnosticCode = "incorrect_argument_count"
	DiagnosticCodeMissingArgument               DiagnosticCode = "missing_argument"
	DiagnosticCodeUnknownNamedArgument          DiagnosticCode = "unknown_named_argument"
	DiagnosticCodeDuplicateArgument             DiagnosticCode = "duplicate_argument"
	DiagnosticCodeNamedArgumentsUnsupported     DiagnosticCode = "named_arguments_unsupported"
	DiagnosticCodeInvalidFunctionTypeArgs       DiagnosticCode = "invalid_function_type_arguments"
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

func mutationDiagnostic(code DiagnosticCode, legacyMessage string, title string, text string, primary DiagnosticLabel, declaration *SourceSpan, declarationMessage string) Diagnostic {
	secondary := []DiagnosticLabel{}
	if declaration != nil {
		secondary = append(secondary, DiagnosticLabel{Span: *declaration, Message: declarationMessage})
	}
	diagnostic := newLabeledDiagnostic(Error, legacyMessage, title, text, primary, secondary...)
	diagnostic.Code = code
	return diagnostic
}

type immutableMutableReferenceDiagnostic struct {
	Place           string
	Span            SourceSpan
	DeclarationSpan *SourceSpan
}

func (d immutableMutableReferenceDiagnostic) build() Diagnostic {
	return mutationDiagnostic(
		DiagnosticCodeImmutableMutableReference,
		fmt.Sprintf("Cannot take a mutable reference to immutable '%s'", d.Place),
		"Cannot take a mutable reference",
		"A mutable reference requires mutable storage.",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("`%s` is immutable", d.Place)},
		d.DeclarationSpan,
		"this binding is immutable",
	)
}

type unsupportedMutableReferenceDiagnostic struct {
	Type Type
	Span SourceSpan
}

func (d unsupportedMutableReferenceDiagnostic) build() Diagnostic {
	return mutationDiagnostic(
		DiagnosticCodeUnsupportedMutableReference,
		fmt.Sprintf("Cannot take a mutable reference to %s", d.Type),
		"Unsupported mutable reference",
		"This foreign value has no supported pointer form.",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("`%s` cannot be referenced mutably", d.Type)},
		nil,
		"",
	)
}

type invalidForeignPointerBindingDiagnostic struct{ Span SourceSpan }

func (d invalidForeignPointerBindingDiagnostic) build() Diagnostic {
	return mutationDiagnostic(
		DiagnosticCodeInvalidForeignPointerBinding,
		"A mut reference from a Go call must be bound with let; rebinding it is not supported",
		"Go pointer result must use let",
		"A Go pointer result already refers to mutable storage and cannot be rebound.",
		DiagnosticLabel{Span: d.Span, Message: "bind this pointer result with `let`"},
		nil,
		"",
	)
}

type unreachableReferentAssignmentDiagnostic struct {
	Name            string
	Span            SourceSpan
	DeclarationSpan *SourceSpan
}

func (d unreachableReferentAssignmentDiagnostic) build() Diagnostic {
	legacy := fmt.Sprintf("Cannot assign a new value through '%s': element writes share storage, but the referent binding is not reachable. Assign to the original binding instead", d.Name)
	return mutationDiagnostic(
		DiagnosticCodeUnreachableReferentAssignment,
		legacy,
		"Referent binding is not reachable",
		"Element writes share storage, but replacing the whole value requires assigning to the original binding.",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("cannot replace the whole value through `%s`", d.Name)},
		d.DeclarationSpan,
		"this reference was declared here",
	)
}

type referenceRebindingDiagnostic struct {
	Span            SourceSpan
	DeclarationSpan *SourceSpan
}

func (d referenceRebindingDiagnostic) build() Diagnostic {
	return mutationDiagnostic(
		DiagnosticCodeReferenceRebinding,
		"References cannot be rebound; assign the value directly",
		"References cannot be rebound",
		"Assignment through a reference updates its referent; it cannot change which value is referenced.",
		DiagnosticLabel{Span: d.Span, Message: "this value would rebind the reference"},
		d.DeclarationSpan,
		"this reference was declared here",
	)
}

type immutablePropertyAssignmentDiagnostic struct {
	Property        string
	Span            SourceSpan
	DeclarationSpan *SourceSpan
}

func (d immutablePropertyAssignmentDiagnostic) build() Diagnostic {
	return mutationDiagnostic(
		DiagnosticCodeImmutablePropertyAssignment,
		fmt.Sprintf("Immutable: %s", d.Property),
		"Cannot assign through an immutable value",
		"",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("`%s` is immutable", d.Property)},
		d.DeclarationSpan,
		"this binding is immutable",
	)
}

type immutableReceiverKind uint8

const (
	immutableArdReceiver immutableReceiverKind = iota
	immutableMaybeReceiver
	immutablePointerMethodAccess
	immutablePointerMethodCall
)

type immutableReceiverDiagnostic struct {
	Kind            immutableReceiverKind
	Receiver        string
	Method          string
	Span            SourceSpan
	DeclarationSpan *SourceSpan
}

func (d immutableReceiverDiagnostic) build() Diagnostic {
	code := DiagnosticCodeImmutableReceiver
	legacy := fmt.Sprintf("Cannot mutate immutable '%s' with '.%s()'", d.Receiver, d.Method)
	primary := fmt.Sprintf("`.%s()` requires a mutable receiver", d.Method)
	if d.Kind == immutableMaybeReceiver {
		legacy = fmt.Sprintf("Immutable: Maybe.%s receiver", d.Method)
		primary = fmt.Sprintf("`Maybe.%s` requires a mutable receiver", d.Method)
	} else if d.Kind == immutablePointerMethodAccess {
		code = DiagnosticCodeImmutablePointerReceiver
		legacy = fmt.Sprintf("Cannot access pointer receiver method %s.%s on immutable value", d.Receiver, d.Method)
		primary = "this method value requires a mutable receiver"
	} else if d.Kind == immutablePointerMethodCall {
		code = DiagnosticCodeImmutablePointerReceiver
		legacy = fmt.Sprintf("Cannot call pointer receiver method %s.%s on immutable value", d.Receiver, d.Method)
		primary = "this method call requires a mutable receiver"
	}
	return mutationDiagnostic(
		code,
		legacy,
		"Cannot use mutating method on immutable receiver",
		"",
		DiagnosticLabel{Span: d.Span, Message: primary},
		d.DeclarationSpan,
		"this receiver was bound immutably here",
	)
}

type nonAssignableStaticPropertyKind uint8

const (
	staticPropertyAssignment nonAssignableStaticPropertyKind = iota
	goConstantAssignment
)

type nonAssignableStaticPropertyDiagnostic struct {
	Kind   nonAssignableStaticPropertyKind
	Target string
	Span   SourceSpan
}

func (d nonAssignableStaticPropertyDiagnostic) build() Diagnostic {
	code := DiagnosticCodeNonAssignableStaticProperty
	legacy := fmt.Sprintf("Cannot assign to static property: %s", d.Target)
	title := "Cannot assign to static property"
	primary := "this static property is not assignable"
	if d.Kind == goConstantAssignment {
		code = DiagnosticCodeGoConstantAssignment
		legacy = fmt.Sprintf("Cannot assign to Go constant: %s", d.Target)
		title = "Cannot assign to Go constant"
		primary = "Go constants are not assignable"
	}
	return mutationDiagnostic(code, legacy, title, "", DiagnosticLabel{Span: d.Span, Message: primary}, nil, "")
}

type nonCallableLegacyStyle uint8

const (
	nonCallableSuffix nonCallableLegacyStyle = iota
	nonCallablePrefix
)

type nonCallableDiagnostic struct {
	Name            string
	Span            SourceSpan
	DeclarationSpan *SourceSpan
	LegacyStyle     nonCallableLegacyStyle
}

func (d nonCallableDiagnostic) build() Diagnostic {
	legacy := fmt.Sprintf("%s is not a function", d.Name)
	if d.LegacyStyle == nonCallablePrefix {
		legacy = fmt.Sprintf("Not a function: %s", d.Name)
	}
	secondary := []DiagnosticLabel{}
	if d.DeclarationSpan != nil {
		secondary = append(secondary, DiagnosticLabel{Span: *d.DeclarationSpan, Message: fmt.Sprintf("`%s` is declared here", d.Name)})
	}
	diagnostic := newLabeledDiagnostic(
		Error,
		legacy,
		"Value is not callable",
		"Only functions and function-typed values can be called.",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("`%s` is not a function", d.Name)},
		secondary...,
	)
	diagnostic.Code = DiagnosticCodeNotCallable
	return diagnostic
}

type argumentCountDiagnostic struct {
	Expected      string
	Actual        int
	Span          SourceSpan
	LegacyMessage string
}

func (d argumentCountDiagnostic) build() Diagnostic {
	legacy := d.LegacyMessage
	if legacy == "" {
		legacy = fmt.Sprintf("Incorrect number of arguments: Expected %s, got %d", d.Expected, d.Actual)
	}
	diagnostic := newLabeledDiagnostic(
		Error,
		legacy,
		"Incorrect number of arguments",
		"",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("expected %s argument(s), but found %d", d.Expected, d.Actual)},
	)
	diagnostic.Code = DiagnosticCodeIncorrectArgumentCount
	return diagnostic
}

type missingArgumentDiagnostic struct {
	Parameter Parameter
	Span      SourceSpan
}

func (d missingArgumentDiagnostic) build() Diagnostic {
	secondary := []DiagnosticLabel{}
	if d.Parameter.declaredAt.FilePath != "" {
		secondary = append(secondary, DiagnosticLabel{
			Span:    d.Parameter.declaredAt,
			Message: fmt.Sprintf("parameter `%s` is required", d.Parameter.Name),
		})
	}
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("missing argument for parameter: %s", d.Parameter.Name),
		"Missing required argument",
		"",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("this call is missing `%s`", d.Parameter.Name)},
		secondary...,
	)
	diagnostic.Code = DiagnosticCodeMissingArgument
	return diagnostic
}

type argumentBindingDiagnosticKind uint8

const (
	tooManyPositionalArguments argumentBindingDiagnosticKind = iota
	unknownNamedArgument
	duplicateArgument
)

type argumentBindingDiagnostic struct {
	Kind          argumentBindingDiagnosticKind
	Name          string
	Span          SourceSpan
	PreviousSpan  *SourceSpan
	LegacyMessage string
}

func (d argumentBindingDiagnostic) build() Diagnostic {
	code := DiagnosticCodeIncorrectArgumentCount
	legacy := "too many positional arguments"
	title := "Too many positional arguments"
	primary := "this positional argument has no matching parameter"
	if d.Kind == unknownNamedArgument {
		code = DiagnosticCodeUnknownNamedArgument
		legacy = fmt.Sprintf("unknown parameter name: %s", d.Name)
		title = "Unknown named argument"
		primary = fmt.Sprintf("no parameter named `%s`", d.Name)
	} else if d.Kind == duplicateArgument {
		code = DiagnosticCodeDuplicateArgument
		legacy = fmt.Sprintf("parameter %s specified multiple times", d.Name)
		title = "Argument specified multiple times"
		primary = fmt.Sprintf("`%s` is supplied again here", d.Name)
	}
	if d.LegacyMessage != "" {
		legacy = d.LegacyMessage
	}
	secondary := []DiagnosticLabel{}
	if d.PreviousSpan != nil {
		secondary = append(secondary, DiagnosticLabel{Span: *d.PreviousSpan, Message: "first supplied here"})
	}
	diagnostic := newLabeledDiagnostic(Error, legacy, title, "", DiagnosticLabel{Span: d.Span, Message: primary}, secondary...)
	diagnostic.Code = code
	return diagnostic
}

type namedArgumentsUnsupportedDiagnostic struct {
	TargetKind string
	Span       SourceSpan
}

func (d namedArgumentsUnsupportedDiagnostic) build() Diagnostic {
	legacy := fmt.Sprintf("%s calls do not support named arguments", d.TargetKind)
	diagnostic := newLabeledDiagnostic(
		Error,
		legacy,
		"Named arguments are not supported",
		"",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("use a positional argument for this %s call", strings.ToLower(d.TargetKind))},
	)
	diagnostic.Code = DiagnosticCodeNamedArgumentsUnsupported
	return diagnostic
}

type invalidFunctionTypeArgumentsDiagnostic struct {
	Name          string
	Expected      int
	Actual        int
	TakesTypeArgs bool
	Span          SourceSpan
	LegacyMessage string
}

func (d invalidFunctionTypeArgumentsDiagnostic) build() Diagnostic {
	legacy := d.LegacyMessage
	primary := "type arguments are not accepted here"
	if d.TakesTypeArgs {
		primary = fmt.Sprintf("expected %d type argument(s), but found %d", d.Expected, d.Actual)
		if legacy == "could not resolve type argument" {
			primary = "this type argument could not be resolved"
		}
	}
	if legacy == "" {
		if !d.TakesTypeArgs {
			legacy = fmt.Sprintf("function %s does not take type arguments", d.Name)
		} else {
			legacy = fmt.Sprintf("Expected %d type arguments, got %d", d.Expected, d.Actual)
		}
	}
	diagnostic := newLabeledDiagnostic(
		Error,
		legacy,
		"Invalid function type arguments",
		"",
		DiagnosticLabel{Span: d.Span, Message: primary},
	)
	diagnostic.Code = DiagnosticCodeInvalidFunctionTypeArgs
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

type branchTypeMismatchDiagnostic struct {
	Expected      Type
	Actual        Type
	ExpectedSpan  *SourceSpan
	ActualSpan    SourceSpan
	LegacyMessage string
	Title         string
}

func (d branchTypeMismatchDiagnostic) build() Diagnostic {
	secondary := []DiagnosticLabel{}
	if d.ExpectedSpan != nil {
		secondary = append(secondary, DiagnosticLabel{
			Span:    *d.ExpectedSpan,
			Message: fmt.Sprintf("an earlier branch produces `%s`", d.Expected),
		})
	}
	diagnostic := newLabeledDiagnostic(
		Error,
		d.LegacyMessage,
		d.Title,
		"",
		DiagnosticLabel{Span: d.ActualSpan, Message: fmt.Sprintf("this branch produces `%s`", d.Actual)},
		secondary...,
	)
	diagnostic.Code = DiagnosticCodeBranchTypeMismatch
	return diagnostic
}

type nonExhaustiveValueIfDiagnostic struct {
	IfSpan SourceSpan
}

func (d nonExhaustiveValueIfDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		"if used as a value must have an else branch",
		"Value-producing if requires an else branch",
		"Every control-flow path must produce a value.",
		DiagnosticLabel{Span: d.IfSpan, Message: "not every path produces a value"},
	)
	diagnostic.Code = DiagnosticCodeNonExhaustiveValueIf
	return diagnostic
}

type invalidMapKeyTypeDiagnostic struct {
	KeyType Type
	Span    SourceSpan
}

func (d invalidMapKeyTypeDiagnostic) build() Diagnostic {
	displayed := formatTypeForDisplay(d.KeyType)
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Invalid map key type %s: map keys must be comparable (primitives, enums, or structs)", displayed),
		"Invalid map key type",
		"Map keys must be comparable primitives, enums, or structs.",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("`%s` cannot be used as a map key", displayed)},
	)
	diagnostic.Code = DiagnosticCodeInvalidMapKeyType
	return diagnostic
}

type malformedTypeNodeDiagnostic struct {
	Span SourceSpan
}

func (d malformedTypeNodeDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		"internal error: malformed type node reached the checker (parser bug — please report)",
		"Internal compiler error",
		"A malformed type node reached the checker from a clean parse tree. This is a parser bug; please report it.",
		DiagnosticLabel{Span: d.Span},
	)
	diagnostic.Code = DiagnosticCodeMalformedTypeNode
	return diagnostic
}

type unresolvedGenericDiagnostic struct {
	Generic string
	Span    SourceSpan
}

func (d unresolvedGenericDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		"Unresolved generic: "+d.Generic,
		"Unresolved generic",
		"The generic type could not be inferred from this expression.",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("generic `%s` remains unresolved", d.Generic)},
	)
	diagnostic.Code = DiagnosticCodeUnresolvedGeneric
	return diagnostic
}

type unboundGenericTypeArgumentDiagnostic struct {
	Name string
	Span SourceSpan
}

func (d unboundGenericTypeArgumentDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("unbound generic type argument $%s", d.Name),
		"Unbound generic type argument",
		fmt.Sprintf("`$%s` is not bound in this function or closure context.", d.Name),
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("`$%s` cannot be used as a type argument here", d.Name)},
	)
	diagnostic.Code = DiagnosticCodeUnboundGenericTypeArg
	return diagnostic
}

type nonGenericTypeSpecializationDiagnostic struct {
	Span SourceSpan
}

func (d nonGenericTypeSpecializationDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		"Type is not generic and cannot be specialized.",
		"Type is not generic",
		"Only generic types can take explicit type arguments.",
		DiagnosticLabel{Span: d.Span, Message: "this type cannot be specialized"},
	)
	diagnostic.Code = DiagnosticCodeNonGenericSpecialization
	return diagnostic
}

type incorrectTypeArgumentCountDiagnostic struct {
	Expected      int
	Actual        int
	LegacyMessage string
	Span          SourceSpan
}

func (d incorrectTypeArgumentCountDiagnostic) build() Diagnostic {
	legacyMessage := d.LegacyMessage
	if legacyMessage == "" {
		legacyMessage = fmt.Sprintf("Incorrect number of type arguments: expected %d, got %d", d.Expected, d.Actual)
	}
	diagnostic := newLabeledDiagnostic(
		Error,
		legacyMessage,
		"Incorrect number of type arguments",
		"",
		DiagnosticLabel{
			Span:    d.Span,
			Message: fmt.Sprintf("expected %d type arguments, but found %d", d.Expected, d.Actual),
		},
	)
	diagnostic.Code = DiagnosticCodeIncorrectTypeArgCount
	return diagnostic
}

type missingTypeArgumentsDiagnostic struct {
	TypeName string
	Span     SourceSpan
}

func (d missingTypeArgumentsDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Generic type %s requires type arguments", d.TypeName),
		"Missing type arguments",
		"",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("generic type `%s` requires type arguments", d.TypeName)},
	)
	diagnostic.Code = DiagnosticCodeMissingTypeArguments
	return diagnostic
}

type recursiveGenericSelfReferenceDiagnostic struct {
	TypeName string
	Span     SourceSpan
}

func (d recursiveGenericSelfReferenceDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Recursive generic self-reference %s is not supported yet", d.TypeName),
		"Recursive generic self-reference",
		"Recursive generic specialization is not supported while the type is being defined.",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("`%s` is specialized while its definition is still being resolved", d.TypeName)},
	)
	diagnostic.Code = DiagnosticCodeRecursiveGenericReference
	return diagnostic
}

type methodIntroducedGenericReason uint8

const (
	methodGenericExplicitDeclaration methodIntroducedGenericReason = iota
	methodGenericInvalidOccurrence
	methodGenericSemanticLeak
)

type methodIntroducedGenericDiagnostic struct {
	Name   string
	Reason methodIntroducedGenericReason
	Span   SourceSpan
}

func (d methodIntroducedGenericDiagnostic) build() Diagnostic {
	label := "methods cannot declare their own generic parameters"
	switch d.Reason {
	case methodGenericExplicitDeclaration:
	case methodGenericInvalidOccurrence:
		label = fmt.Sprintf("`$%s` is not a generic parameter of the receiver type", d.Name)
	case methodGenericSemanticLeak:
		label = "this method contains a generic not provided by its receiver type"
	default:
		panic(fmt.Sprintf("unknown method-introduced generic reason: %d", d.Reason))
	}
	diagnostic := newLabeledDiagnostic(
		Error,
		"methods cannot introduce generic type parameters; use the receiver type's generics",
		"Method cannot introduce generic parameters",
		"Methods may only use generic parameters declared by their receiver type.",
		DiagnosticLabel{Span: d.Span, Message: label},
	)
	diagnostic.Code = DiagnosticCodeMethodIntroducedGeneric
	return diagnostic
}

type builtInTypeRedeclarationDiagnostic struct {
	Name string
	Span SourceSpan
}

func (d builtInTypeRedeclarationDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("%s is a built-in type and cannot be redeclared", d.Name),
		"Built-in type cannot be redeclared",
		fmt.Sprintf("`%s` is reserved as a built-in type.", d.Name),
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("`%s` cannot be declared here", d.Name)},
	)
	diagnostic.Code = DiagnosticCodeBuiltInTypeRedeclaration
	return diagnostic
}

type recursiveTypeAliasReference struct {
	From string
	To   string
	Span SourceSpan
}

type recursiveTypeAliasDiagnostic struct {
	Name         string
	FallbackSpan SourceSpan
	References   []recursiveTypeAliasReference
}

func (d recursiveTypeAliasDiagnostic) build() Diagnostic {
	closing := recursiveTypeAliasReference{From: d.Name, To: d.Name, Span: d.FallbackSpan}
	priorReferences := d.References
	if len(d.References) > 0 {
		closing = d.References[len(d.References)-1]
		priorReferences = d.References[:len(d.References)-1]
	}
	secondary := make([]DiagnosticLabel, 0, len(priorReferences))
	for _, reference := range priorReferences {
		secondary = append(secondary, DiagnosticLabel{
			Span:    reference.Span,
			Message: fmt.Sprintf("`%s` refers to `%s` here", reference.From, reference.To),
		})
	}
	diagnostic := newLabeledDiagnostic(
		Error,
		"Recursive type alias: "+d.Name,
		"Recursive type alias",
		fmt.Sprintf("Type alias `%s` eventually refers to itself.", d.Name),
		DiagnosticLabel{
			Span:    closing.Span,
			Message: fmt.Sprintf("`%s` refers back to `%s` here", closing.From, closing.To),
		},
		secondary...,
	)
	diagnostic.Code = DiagnosticCodeRecursiveTypeAlias
	return diagnostic
}

type recursiveStructLayoutReference struct {
	StructName string
	FieldName  string
	Span       SourceSpan
}

type recursiveStructLayoutDiagnostic struct {
	Cycle []recursiveStructLayoutReference
}

func (d recursiveStructLayoutDiagnostic) build() Diagnostic {
	primary := d.Cycle[0]
	secondary := make([]DiagnosticLabel, 0, len(d.Cycle)-1)
	for _, reference := range d.Cycle[1:] {
		secondary = append(secondary, DiagnosticLabel{
			Span:    reference.Span,
			Message: fmt.Sprintf("`%s.%s` continues the inline cycle", reference.StructName, reference.FieldName),
		})
	}
	legacyMessage := fmt.Sprintf(
		"Recursive field %s.%s has infinite size. %s",
		primary.StructName,
		primary.FieldName,
		recursiveLayoutDiagnostic,
	)
	diagnostic := newLabeledDiagnostic(
		Error,
		legacyMessage,
		"Recursive field has infinite size",
		recursiveLayoutDiagnostic,
		DiagnosticLabel{
			Span:    primary.Span,
			Message: fmt.Sprintf("`%s.%s` creates an infinite-size recursive layout", primary.StructName, primary.FieldName),
		},
		secondary...,
	)
	diagnostic.Code = DiagnosticCodeRecursiveStructLayout
	return diagnostic
}

type goImportResolutionDiagnostic struct {
	Path  string
	Cause string
	Span  SourceSpan
}

func (d goImportResolutionDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Failed to resolve Go import '%s': %s", d.Path, d.Cause),
		"Failed to resolve Go import",
		d.Cause,
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("could not resolve Go package `%s`", d.Path)},
	)
	diagnostic.Code = DiagnosticCodeGoImportResolution
	return diagnostic
}

type ardImportResolutionDiagnostic struct {
	Path  string
	Cause string
	Span  SourceSpan
}

func (d ardImportResolutionDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Failed to resolve import '%s': %s", d.Path, d.Cause),
		"Failed to resolve import",
		d.Cause,
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("could not resolve module `%s`", d.Path)},
	)
	diagnostic.Code = DiagnosticCodeImportResolution
	return diagnostic
}

type circularImportDiagnostic struct {
	Chain       []string
	ClosingSpan SourceSpan
}

func (d circularImportDiagnostic) build() Diagnostic {
	chain := strings.Join(d.Chain, " -> ")
	diagnostic := newLabeledDiagnostic(
		Error,
		"circular dependency detected: "+chain,
		"Circular dependency",
		chain,
		DiagnosticLabel{Span: d.ClosingSpan, Message: "this import closes the dependency cycle"},
	)
	diagnostic.Code = DiagnosticCodeCircularImport
	return diagnostic
}

func reanchorCircularImportDiagnostic(diagnostic Diagnostic, importerSpan SourceSpan) Diagnostic {
	if diagnostic.Code != DiagnosticCodeCircularImport {
		return diagnostic
	}
	secondary := make([]DiagnosticLabel, 0, len(diagnostic.Secondary)+1)
	secondary = append(secondary, diagnostic.Primary)
	secondary = append(secondary, diagnostic.Secondary...)
	diagnostic.Primary = DiagnosticLabel{
		Span:    importerSpan,
		Message: "this import leads to a dependency cycle",
	}
	diagnostic.Secondary = secondary
	return diagnostic
}

type moduleLoadDiagnostic struct {
	ImportPath string
	TargetFile string
	Cause      string
	ImportSpan SourceSpan
}

func (d moduleLoadDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Failed to load module %s: %s", d.TargetFile, d.Cause),
		"Failed to load module",
		fmt.Sprintf("%s: %s", d.TargetFile, d.Cause),
		DiagnosticLabel{Span: d.ImportSpan, Message: fmt.Sprintf("module `%s` could not be loaded", d.ImportPath)},
	)
	diagnostic.Code = DiagnosticCodeModuleLoadFailure
	return diagnostic
}

type duplicateImportDiagnostic struct {
	Name           string
	StatementStart parse.Point
	DuplicateSpan  SourceSpan
	OriginalSpan   SourceSpan
}

func (d duplicateImportDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Warn,
		fmt.Sprintf("%s Duplicate import: %s", d.StatementStart, d.Name),
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

type fixedArrayLengthMismatchDiagnostic struct {
	Expected int
	Actual   int
	Span     SourceSpan
}

func (d fixedArrayLengthMismatchDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Type mismatch: Expected %d elements, got %d", d.Expected, d.Actual),
		"Fixed array length mismatch",
		"",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("expected %d elements, but found %d", d.Expected, d.Actual)},
	)
	diagnostic.Code = DiagnosticCodeTypeMismatch
	return diagnostic
}

type homogeneousListMismatchDiagnostic struct {
	Expected     Type
	Actual       Type
	ExpectedSpan SourceSpan
	ActualSpan   SourceSpan
}

func (d homogeneousListMismatchDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		"Type mismatch: A list can only contain values of single type",
		"List element type mismatch",
		"All values in a list must have the same type.",
		DiagnosticLabel{Span: d.ActualSpan, Message: fmt.Sprintf("this element has type `%s`", d.Actual)},
		DiagnosticLabel{Span: d.ExpectedSpan, Message: fmt.Sprintf("the first element established type `%s`", d.Expected)},
	)
	diagnostic.Code = DiagnosticCodeTypeMismatch
	return diagnostic
}

type unexpectedListDiagnostic struct {
	Expected Type
	Span     SourceSpan
}

func (d unexpectedListDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Expected %s but got a list", formatTypeForDisplay(d.Expected)),
		"Unexpected list",
		"",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("expected `%s`, but found a list", d.Expected)},
	)
	diagnostic.Code = DiagnosticCodeTypeMismatch
	return diagnostic
}

type expectedMapTypeDiagnostic struct {
	Actual Type
	Span   SourceSpan
}

func (d expectedMapTypeDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Expected map type but got %s", d.Actual),
		"Expected a map type",
		"",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("`%s` is not a map type", d.Actual)},
	)
	diagnostic.Code = DiagnosticCodeTypeMismatch
	return diagnostic
}

type stringInterpolationMismatchDiagnostic struct {
	Actual Type
	Span   SourceSpan
}

func (d stringInterpolationMismatchDiagnostic) build() Diagnostic {
	diagnostic := newLabeledDiagnostic(
		Error,
		fmt.Sprintf("Type mismatch: Expected stringable value, got %s", d.Actual),
		"Value cannot be interpolated",
		"Interpolated values must support string conversion.",
		DiagnosticLabel{Span: d.Span, Message: fmt.Sprintf("`%s` cannot be converted to a string", d.Actual)},
	)
	diagnostic.Code = DiagnosticCodeTypeMismatch
	return diagnostic
}

type typeMismatchDiagnostic struct {
	Expected      Type
	Actual        Type
	ActualSpan    SourceSpan
	Expectation   *typeExpectation
	LegacyMessage string
}

type typeExpectation struct {
	Span SourceSpan
	Kind typeExpectationKind
}

type typeExpectationKind uint8

const (
	expectationUnknown typeExpectationKind = iota
	expectationAnnotation
	expectationReturnAnnotation
)

func (d typeMismatchDiagnostic) build() Diagnostic {
	primaryMessage := fmt.Sprintf("this expression has type `%s`", d.Actual)
	if d.Expectation == nil {
		primaryMessage = fmt.Sprintf("expected `%s`, but this expression has type `%s`", d.Expected, d.Actual)
	}

	secondary := make([]DiagnosticLabel, 0, 1)
	if d.Expectation != nil {
		message := fmt.Sprintf("this requires `%s`", d.Expected)
		switch d.Expectation.Kind {
		case expectationAnnotation:
			message = fmt.Sprintf("this annotation requires `%s`", d.Expected)
		case expectationReturnAnnotation:
			message = fmt.Sprintf("this return annotation requires `%s`", d.Expected)
		}
		secondary = append(secondary, DiagnosticLabel{Span: d.Expectation.Span, Message: message})
	}

	legacyMessage := d.LegacyMessage
	if legacyMessage == "" {
		legacyMessage = typeMismatch(d.Expected, d.Actual)
	}
	diagnostic := newLabeledDiagnostic(
		Error,
		legacyMessage,
		"Type mismatch",
		"",
		DiagnosticLabel{Span: d.ActualSpan, Message: primaryMessage},
		secondary...,
	)
	diagnostic.Code = DiagnosticCodeTypeMismatch
	return diagnostic
}
