package checker

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestUnresolvedReferenceDiagnostic(t *testing.T) {
	span := SourceSpan{FilePath: "main.ard"}
	tests := []struct {
		kind    unresolvedReferenceKind
		name    string
		code    DiagnosticCode
		message string
		title   string
	}{
		{unrecognizedType, "Missing", DiagnosticCodeUndefinedType, "Unrecognized type: Missing", "Unrecognized type"},
		{unrecognizedReturnType, "Missing", DiagnosticCodeUndefinedType, "Unrecognized return type: Missing", "Unrecognized return type"},
		{undefinedType, "Missing", DiagnosticCodeUndefinedType, "Undefined type: Missing", "Undefined type"},
		{undefinedTrait, "Missing", DiagnosticCodeUndefinedTrait, "Undefined trait: Missing", "Undefined trait"},
		{unknownModule, "ard/missing", DiagnosticCodeUndefinedModule, "Unknown module: ard/missing", "Unknown module"},
		{undefinedModule, "missing", DiagnosticCodeUndefinedModule, "Undefined module: missing", "Undefined module"},
		{unknownGoNamespace, "missing", DiagnosticCodeUndefinedNamespace, "Unknown Go namespace: missing", "Unknown Go namespace"},
		{unknownStructField, "missing", DiagnosticCodeUnknownField, "Unknown field: missing", "Unknown field"},
		{undefinedAssignmentTarget, "missing", DiagnosticCodeUndefinedName, "Undefined: missing", "Undefined assignment target"},
		{undefinedQualifiedMember, "mod::missing", DiagnosticCodeUndefinedQualifiedMember, "Undefined: mod::missing", "Undefined qualified member"},
		{undefinedGoFunction, "fmt::Missing", DiagnosticCodeUndefinedGoFunction, "Undefined Go function: fmt::Missing", "Undefined Go function"},
		{undefinedGoType, "image::Missing", DiagnosticCodeUndefinedType, "Undefined Go type: image::Missing", "Undefined Go type"},
		{undefinedStaticRoot, "Missing", DiagnosticCodeUndefinedName, "Undefined: Missing", "Undefined name"},
		{undefinedEnumVariant, "Color::missing", DiagnosticCodeUndefinedEnumVariant, "Undefined: Color::missing", "Undefined enum variant"},
		{invalidStaticMember, "Str::missing", DiagnosticCodeInvalidStaticMember, "Undefined: Str::missing", "Invalid static member"},
		{undefinedStructType, "Missing", DiagnosticCodeUndefinedType, "Undefined: Missing", "Undefined struct type"},
		{notAStruct, "value", DiagnosticCodeNotAStruct, "Undefined: value", "Not a struct"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			diagnostic := (unresolvedReferenceDiagnostic{Kind: tt.kind, Name: tt.name, Span: span}).build()
			if diagnostic.Code != tt.code || diagnostic.Message != tt.message || diagnostic.Title != tt.title {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if diagnostic.Primary.Span != span || diagnostic.Primary.Message == "" {
				t.Fatalf("primary = %#v", diagnostic.Primary)
			}
		})
	}
}

func TestUndefinedNameDiagnostic(t *testing.T) {
	span := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 1}}}
	tests := []struct {
		name   string
		kind   undefinedNameKind
		title  string
		legacy string
	}{
		{name: "variable", kind: undefinedVariable, title: "Undefined variable", legacy: "Undefined variable: missing"},
		{name: "function", kind: undefinedFunction, title: "Undefined function", legacy: "Undefined function: missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostic := (undefinedNameDiagnostic{Kind: tt.kind, Name: "missing", Span: span}).build()
			if diagnostic.Code != DiagnosticCodeUndefinedName || diagnostic.Title != tt.title || diagnostic.Message != tt.legacy {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if diagnostic.Primary.Span != span || diagnostic.Primary.Message != "`missing` is not defined in this scope" {
				t.Fatalf("primary = %#v", diagnostic.Primary)
			}
		})
	}
}

func TestUndefinedMemberDiagnostic(t *testing.T) {
	span := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 2, Col: 8}}}
	tests := []struct {
		name        string
		kind        undefinedMemberKind
		title       string
		legacy      string
		primaryText string
	}{
		{name: "field", kind: undefinedField, title: "Undefined field", legacy: "Undefined: user.height", primaryText: "`height` is not defined for `user`"},
		{name: "method", kind: undefinedMethod, title: "Undefined method", legacy: "Undefined: user.save", primaryText: "`save` is not defined for `user`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			member := "height"
			if tt.kind == undefinedMethod {
				member = "save"
			}
			diagnostic := (undefinedMemberDiagnostic{Kind: tt.kind, Receiver: "user", Member: member, Span: span}).build()
			if diagnostic.Code != DiagnosticCodeUndefinedMember || diagnostic.Title != tt.title || diagnostic.Message != tt.legacy {
				t.Fatalf("code/title/message = %q/%q/%q", diagnostic.Code, diagnostic.Title, diagnostic.Message)
			}
			if diagnostic.Primary.Span != span || diagnostic.Primary.Message != tt.primaryText {
				t.Fatalf("primary = %#v", diagnostic.Primary)
			}
			if len(diagnostic.Secondary) != 0 {
				t.Fatalf("secondary = %#v, want none", diagnostic.Secondary)
			}
		})
	}
}

func TestDuplicateDeclarationDiagnosticBuildsBothLabels(t *testing.T) {
	original := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 8}}}
	duplicate := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 2, Col: 6}}}
	diagnostic := (duplicateDeclarationDiagnostic{
		Name:          "User",
		DuplicateSpan: duplicate,
		OriginalSpan:  original,
	}).build()

	if diagnostic.Kind != Error || diagnostic.Code != DiagnosticCodeDuplicateDeclaration {
		t.Fatalf("kind/code = %q/%q", diagnostic.Kind, diagnostic.Code)
	}
	if diagnostic.Message != "Duplicate declaration: User" || diagnostic.Title != "Duplicate declaration" {
		t.Fatalf("message/title = %q/%q", diagnostic.Message, diagnostic.Title)
	}
	if diagnostic.Primary.Span != duplicate || diagnostic.Primary.Message != "`User` is declared again here" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span != original || diagnostic.Secondary[0].Message != "first declared here" {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestInvalidForClauseDiagnosticCodes(t *testing.T) {
	span := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 1}, End: parse.Point{Row: 1, Col: 4}}}
	tests := []struct {
		clause string
		code   DiagnosticCode
	}{
		{"initializer", DiagnosticCodeInvalidForInitializer},
		{"update", DiagnosticCodeInvalidForUpdate},
	}
	for _, tt := range tests {
		diagnostic := (invalidForClauseDiagnostic{Clause: tt.clause, Span: span, LegacyMessage: "legacy", Label: "invalid"}).build()
		if diagnostic.Code != tt.code || diagnostic.Primary.Span != span {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	}
}

func TestDeclaredTypeLocationFallsBackForNil(t *testing.T) {
	fallback := parse.Location{Start: parse.Point{Row: 3, Col: 4}, End: parse.Point{Row: 3, Col: 8}}
	if got := declaredTypeLocation(nil, fallback); got != fallback {
		t.Fatalf("location = %#v, want fallback %#v", got, fallback)
	}
}

func TestDuplicateMethodDiagnosticUsesOriginalDeclaration(t *testing.T) {
	original := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 1}}}
	duplicate := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 2, Col: 1}}}
	diagnostic := duplicateMethodDiagnostic{Method: "save", Span: duplicate, OriginalSpan: &original}.build()
	if diagnostic.Code != DiagnosticCodeDuplicateMethod || diagnostic.Primary.Span != duplicate {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span != original {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestGenericTestDiagnostic(t *testing.T) {
	span := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 1}, End: parse.Point{Row: 1, Col: 20}}}
	diagnostic := invalidTestFunctionDiagnostic{Kind: genericTestNotAllowed, Span: span}.build()
	if diagnostic.Code != DiagnosticCodeGenericTestNotAllowed || diagnostic.Message != "test functions must not be generic" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if diagnostic.Primary.Span != span || diagnostic.Primary.Message != "remove generic type parameters" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}

	nonTopLevel := invalidTestFunctionDiagnostic{Kind: testNotTopLevel, Span: span}.build()
	if nonTopLevel.Code != DiagnosticCodeTestNotTopLevel || nonTopLevel.Primary.Message != "move this test to the module level" {
		t.Fatalf("non-top-level diagnostic = %#v", nonTopLevel)
	}
}

func TestTypeMismatchDiagnosticWithoutExpectationLabelsPrimary(t *testing.T) {
	span := SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 1}}}
	diagnostic := (typeMismatchDiagnostic{
		Expected:   Str,
		Actual:     Int,
		ActualSpan: span,
	}).build()

	if diagnostic.Kind != Error {
		t.Fatalf("kind = %q, want error", diagnostic.Kind)
	}
	if diagnostic.Code != DiagnosticCodeTypeMismatch {
		t.Fatalf("code = %q, want type mismatch", diagnostic.Code)
	}
	if diagnostic.Title != "Type mismatch" || diagnostic.Text != "" {
		t.Fatalf("title/text = %q/%q", diagnostic.Title, diagnostic.Text)
	}
	if diagnostic.Primary.Message != "expected `Str`, but this expression has type `Int`" {
		t.Fatalf("primary label = %q", diagnostic.Primary.Message)
	}
	if len(diagnostic.Secondary) != 0 {
		t.Fatalf("secondary labels = %#v, want none", diagnostic.Secondary)
	}
	if diagnostic.Message != "Type mismatch: Expected Str, got Int" {
		t.Fatalf("legacy message = %q", diagnostic.Message)
	}
	if diagnostic.FilePath() != span.FilePath || diagnostic.Location() != span.Location {
		t.Fatalf("compatibility span = %q %v, want %#v", diagnostic.FilePath(), diagnostic.Location(), span)
	}
}

func TestTypeMismatchDiagnosticUsesNeutralExpectationLabel(t *testing.T) {
	expectedSpan := SourceSpan{FilePath: "types.ard", Location: parse.Location{Start: parse.Point{Row: 2, Col: 3}}}
	diagnostic := (typeMismatchDiagnostic{
		Expected:   Str,
		Actual:     Int,
		ActualSpan: SourceSpan{FilePath: "main.ard"},
		Expectation: &typeExpectation{
			Span: expectedSpan,
			Kind: expectationUnknown,
		},
	}).build()

	if len(diagnostic.Secondary) != 1 {
		t.Fatalf("secondary labels = %#v, want one", diagnostic.Secondary)
	}
	if diagnostic.Secondary[0].Span != expectedSpan || diagnostic.Secondary[0].Message != "this requires `Str`" {
		t.Fatalf("secondary label = %#v", diagnostic.Secondary[0])
	}
}

func TestMethodIntroducedGenericDiagnosticReasons(t *testing.T) {
	span := SourceSpan{FilePath: "main.ard"}
	tests := []struct {
		name   string
		reason methodIntroducedGenericReason
		label  string
	}{
		{"explicit declaration", methodGenericExplicitDeclaration, "methods cannot declare their own generic parameters"},
		{"semantic leak", methodGenericSemanticLeak, "this method contains a generic not provided by its receiver type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostic := (methodIntroducedGenericDiagnostic{Reason: tt.reason, Span: span}).build()
			if diagnostic.Code != DiagnosticCodeMethodIntroducedGeneric || diagnostic.Primary.Message != tt.label {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
		})
	}
}

func TestImportDiagnosticBuilders(t *testing.T) {
	span := SourceSpan{FilePath: "main.ard", Location: parse.Location{
		Start: parse.Point{Row: 1, Col: 5},
		End:   parse.Point{Row: 1, Col: 15},
	}}
	tests := []struct {
		name    string
		got     Diagnostic
		code    DiagnosticCode
		message string
		title   string
		label   string
		text    string
	}{
		{
			name: "Go resolution",
			got: (goImportResolutionDiagnostic{
				Path: "example.com/pkg", Cause: "package unavailable", Span: span,
			}).build(),
			code:    DiagnosticCodeGoImportResolution,
			message: "Failed to resolve Go import 'example.com/pkg': package unavailable",
			title:   "Failed to resolve Go import",
			label:   "could not resolve Go package `example.com/pkg`",
			text:    "package unavailable",
		},
		{
			name: "Ard resolution",
			got: (ardImportResolutionDiagnostic{
				Path: "app/missing", Cause: "module not found", Span: span,
			}).build(),
			code:    DiagnosticCodeImportResolution,
			message: "Failed to resolve import 'app/missing': module not found",
			title:   "Failed to resolve import",
			label:   "could not resolve module `app/missing`",
			text:    "module not found",
		},
		{
			name: "module load",
			got: (moduleLoadDiagnostic{
				ImportPath: "app/broken", TargetFile: "/app/broken.ard",
				Cause: "parse failed", ImportSpan: span,
			}).build(),
			code:    DiagnosticCodeModuleLoadFailure,
			message: "Failed to load module /app/broken.ard: parse failed",
			title:   "Failed to load module",
			label:   "module `app/broken` could not be loaded",
			text:    "/app/broken.ard: parse failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.Kind != Error || tt.got.Code != tt.code {
				t.Fatalf("kind/code = %q/%q", tt.got.Kind, tt.got.Code)
			}
			if tt.got.Message != tt.message || tt.got.Title != tt.title || tt.got.Text != tt.text {
				t.Fatalf("message/title/text = %q/%q/%q", tt.got.Message, tt.got.Title, tt.got.Text)
			}
			if tt.got.Primary.Span != span || tt.got.Primary.Message != tt.label {
				t.Fatalf("primary = %#v", tt.got.Primary)
			}
			if len(tt.got.Secondary) != 0 {
				t.Fatalf("secondary = %#v", tt.got.Secondary)
			}
		})
	}
}

func TestCircularImportDiagnosticCopiesChainIntoOutput(t *testing.T) {
	span := SourceSpan{FilePath: "b.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 5}}}
	chain := []string{"app/a", "app/b", "app/a"}
	diagnostic := (circularImportDiagnostic{Chain: append([]string(nil), chain...), ClosingSpan: span}).build()
	chain[0] = "changed"

	if diagnostic.Code != DiagnosticCodeCircularImport || diagnostic.Title != "Circular dependency" {
		t.Fatalf("code/title = %q/%q", diagnostic.Code, diagnostic.Title)
	}
	if diagnostic.Message != "circular dependency detected: app/a -> app/b -> app/a" || diagnostic.Text != "app/a -> app/b -> app/a" {
		t.Fatalf("message/text = %q/%q", diagnostic.Message, diagnostic.Text)
	}
	if diagnostic.Primary.Span != span || diagnostic.Primary.Message != "this import closes the dependency cycle" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
}
