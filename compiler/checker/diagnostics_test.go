package checker_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestRemainingUnresolvedReferencesHaveStructuredDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		code    checker.DiagnosticCode
		message string
		span    func(*parse.Program) parse.Location
	}{
		{name: "type", source: "let value: Missing = 1", code: checker.DiagnosticCodeUndefinedType, message: "Unrecognized type: Missing"},
		{name: "struct field", source: "struct User { name: Str }\nUser{name: \"A\", missing: 1}", code: checker.DiagnosticCodeUnknownField, message: "Unknown field: missing"},
		{name: "assignment target", source: "missing = 1", code: checker.DiagnosticCodeUndefinedName, message: "Undefined: missing", span: func(program *parse.Program) parse.Location {
			return program.Statements[0].(*parse.VariableAssignment).Target.GetLocation()
		}},
		{name: "static root", source: "Missing::value", code: checker.DiagnosticCodeUndefinedName, message: "Undefined: Missing", span: func(program *parse.Program) parse.Location {
			return program.Statements[0].(*parse.StaticProperty).Target.GetLocation()
		}},
		{name: "enum variant", source: "enum Color { red }\nColor::missing", code: checker.DiagnosticCodeUndefinedEnumVariant, message: "Undefined: Color::missing", span: func(program *parse.Program) parse.Location {
			return program.Statements[1].(*parse.StaticProperty).Target.GetLocation()
		}},
		{name: "undefined struct", source: "Missing{}", code: checker.DiagnosticCodeUndefinedType, message: "Undefined: Missing", span: func(program *parse.Program) parse.Location {
			return program.Statements[0].(*parse.StructInstance).GetLocation()
		}},
		{name: "not a struct", source: "let value = 1\nvalue{}", code: checker.DiagnosticCodeNotAStruct, message: "Undefined: value", span: func(program *parse.Program) parse.Location {
			return program.Statements[1].(*parse.StructInstance).GetLocation()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			c := checker.New("main.ard", result.Program, nil)
			c.Check()
			for _, diagnostic := range c.Diagnostics() {
				if diagnostic.Message == tt.message {
					if diagnostic.Code != tt.code || diagnostic.Primary.Message == "" {
						t.Fatalf("diagnostic = %#v", diagnostic)
					}
					if tt.span != nil && diagnostic.Primary.Span.Location != tt.span(result.Program) {
						t.Fatalf("primary span = %v, want %v", diagnostic.Primary.Span.Location, tt.span(result.Program))
					}
					return
				}
			}
			t.Fatalf("diagnostics = %#v, missing %q", c.Diagnostics(), tt.message)
		})
	}
}

func TestUndefinedMembersInMaybeAccessorChainsHaveStructuredDiagnostics(t *testing.T) {
	prefix := "struct Profile { name: Str }\nfn test() {\n  let profile: Profile? = Maybe::new(Profile{name: \"A\"})\n  try "
	tests := []struct {
		name     string
		expr     string
		location func(parse.Statement) parse.Location
		title    string
		legacy   string
	}{
		{
			name: "field after maybe", expr: "profile.missing",
			location: func(stmt parse.Statement) parse.Location {
				return stmt.(*parse.InstanceProperty).Property.GetLocation()
			},
			title: "Undefined field", legacy: "Undefined: Profile.missing",
		},
		{
			name: "method after maybe", expr: "profile.missing()",
			location: func(stmt parse.Statement) parse.Location { return stmt.(*parse.InstanceMethod).Method.GetLocation() },
			title:    "Undefined method", legacy: "Undefined: Profile.missing",
		},
		{
			name: "later member in chain", expr: "profile.name.missing",
			location: func(stmt parse.Statement) parse.Location {
				return stmt.(*parse.InstanceProperty).Property.GetLocation()
			},
			title: "Undefined field", legacy: "Undefined: Str.missing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(prefix+tt.expr+" -> _ {\n  }\n}\n"), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			function := result.Program.Statements[len(result.Program.Statements)-1].(*parse.FunctionDeclaration)
			tryExpr := function.Body[len(function.Body)-1].(*parse.Try)
			location := tt.location(tryExpr.Expression)
			c := checker.New("main.ard", result.Program, nil)
			c.Check()
			if len(c.Diagnostics()) != 1 {
				t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
			}
			diagnostic := c.Diagnostics()[0]
			if diagnostic.Code != checker.DiagnosticCodeUndefinedMember || diagnostic.Title != tt.title || diagnostic.Message != tt.legacy {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if diagnostic.Primary.Span.Location != location {
				t.Fatalf("primary = %#v, want location %v", diagnostic.Primary, location)
			}
		})
	}
}

func TestUndefinedNamesHaveStructuredDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		title   string
		legacy  string
		message string
	}{
		{name: "variable", source: "missing", title: "Undefined variable", legacy: "Undefined variable: missing", message: "`missing` is not defined in this scope"},
		{name: "function", source: "missing()", title: "Undefined function", legacy: "Undefined function: missing", message: "`missing` is not defined in this scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			location := result.Program.Statements[0].GetLocation()
			c := checker.New("main.ard", result.Program, nil)
			c.Check()
			if len(c.Diagnostics()) != 1 {
				t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
			}
			diagnostic := c.Diagnostics()[0]
			if diagnostic.Code != checker.DiagnosticCodeUndefinedName || diagnostic.Title != tt.title || diagnostic.Message != tt.legacy {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if diagnostic.Primary.Span.Location != location || diagnostic.Primary.Message != tt.message {
				t.Fatalf("primary = %#v, want location %v", diagnostic.Primary, location)
			}
		})
	}
}

func TestUndefinedInstanceMembersHaveStructuredDiagnostics(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte("\"foo\".length\n\"foo\".save()\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	property := result.Program.Statements[0].(*parse.InstanceProperty)
	method := result.Program.Statements[1].(*parse.InstanceMethod)

	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 2 {
		t.Fatalf("diagnostics = %#v, want two", c.Diagnostics())
	}
	tests := []struct {
		diagnostic checker.Diagnostic
		location   parse.Location
		title      string
		legacy     string
	}{
		{diagnostic: c.Diagnostics()[0], location: property.Property.GetLocation(), title: "Undefined field", legacy: `Undefined: "foo".length`},
		{diagnostic: c.Diagnostics()[1], location: method.Method.GetLocation(), title: "Undefined method", legacy: `Undefined: "foo".save`},
	}
	for _, tt := range tests {
		if tt.diagnostic.Code != checker.DiagnosticCodeUndefinedMember || tt.diagnostic.Title != tt.title || tt.diagnostic.Message != tt.legacy {
			t.Fatalf("diagnostic = %#v", tt.diagnostic)
		}
		if tt.diagnostic.Primary.Span.Location != tt.location {
			t.Fatalf("primary location = %v, want %v", tt.diagnostic.Primary.Span.Location, tt.location)
		}
	}
}

func TestEnumDeclarationDiagnosticsAreStructured(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := parse.Parse([]byte("enum Empty {}\n"), "main.ard")
		enum := result.Program.Statements[0].(*parse.EnumDefinition)
		c := checker.New("main.ard", result.Program, nil)
		c.Check()
		diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeEmptyEnum)
		if diagnostic.Primary.Span.Location != enum.GetLocation() || diagnostic.Message != "Enums must have at least one variant" {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	})

	t.Run("duplicate variant", func(t *testing.T) {
		source := "enum Color {\n  Blue,\n  Green,\n  Blue\n}\n"
		result := parse.Parse([]byte(source), "main.ard")
		enum := result.Program.Statements[0].(*parse.EnumDefinition)
		c := checker.New("main.ard", result.Program, nil)
		c.Check()
		diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeDuplicateEnumVariant)
		if diagnostic.Primary.Span.Location != enum.GetLocation() || diagnostic.Message != "Duplicate variant: Blue" {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	})

	t.Run("invalid discriminant", func(t *testing.T) {
		result := parse.Parse([]byte("enum Status { Ready = \"ready\" }\n"), "main.ard")
		enum := result.Program.Statements[0].(*parse.EnumDefinition)
		c := checker.New("main.ard", result.Program, nil)
		c.Check()
		diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeInvalidEnumDiscriminant)
		if diagnostic.Primary.Span.Location != enum.Variants[0].Value.GetLocation() {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	})

	t.Run("duplicate explicit discriminant", func(t *testing.T) {
		source := "enum Status {\n  Pending = 1,\n  Active = 1\n}\n"
		result := parse.Parse([]byte(source), "main.ard")
		enum := result.Program.Statements[0].(*parse.EnumDefinition)
		c := checker.New("main.ard", result.Program, nil)
		c.Check()
		diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeDuplicateEnumDiscriminant)
		if diagnostic.Primary.Span.Location != enum.Variants[1].Value.GetLocation() || len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != enum.Variants[0].Value.GetLocation() {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	})

	t.Run("auto-assigned original omits secondary", func(t *testing.T) {
		source := "enum Status {\n  First = 1,\n  Second,\n  Third = 2\n}\n"
		result := parse.Parse([]byte(source), "main.ard")
		enum := result.Program.Statements[0].(*parse.EnumDefinition)
		c := checker.New("main.ard", result.Program, nil)
		c.Check()
		diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeDuplicateEnumDiscriminant)
		if diagnostic.Primary.Span.Location != enum.Variants[2].Value.GetLocation() || len(diagnostic.Secondary) != 0 {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	})
}

func TestImplementationDiagnosticsAreStructured(t *testing.T) {
	tests := []struct {
		name            string
		source          string
		code            checker.DiagnosticCode
		legacyMessage   string
		primaryMessage  string
		secondaryLabels int
	}{
		{
			name:          "invalid target",
			source:        "trait T {\n}\ntrait U {\n}\nimpl T for U {\n}\n",
			code:          checker.DiagnosticCodeInvalidImplementationTarget,
			legacyMessage: "U cannot implement a Trait",
		},
		{
			name:           "invalid contract",
			source:         "struct NotATrait {}\nstruct S {}\nimpl NotATrait for S {\n}\n",
			code:           checker.DiagnosticCodeInvalidImplementationTarget,
			primaryMessage: "`NotATrait` does not name a trait",
		},
		{
			name:          "unexpected method",
			source:        "trait T {\n}\nstruct S {}\nimpl T for S {\n  fn extra() {}\n}\n",
			code:          checker.DiagnosticCodeUnexpectedImplMethod,
			legacyMessage: "Method extra is not part of trait T",
		},
		{
			name:          "parameter count",
			source:        "trait T {\n  fn run(value: Int)\n}\nstruct S {}\nimpl T for S {\n  fn run() {}\n}\n",
			code:          checker.DiagnosticCodeImplParameterCount,
			legacyMessage: "Method run has wrong number of parameters",
		},
		{
			name:          "missing Go interface method",
			source:        "use go:io\nstruct Sink {}\nimpl io::Writer for Sink {\n}\n",
			code:          checker.DiagnosticCodeMissingImplMethod,
			legacyMessage: "Missing method 'write' in Go interface 'io::Writer'",
		},
		{
			name:            "parameter mutability",
			source:          "struct State {}\ntrait T {\n  fn update(value: mut State)\n}\nstruct S {}\nimpl T for S {\n  fn update(value: State) {}\n}\n",
			code:            checker.DiagnosticCodeImplParameterMutability,
			legacyMessage:   "Trait method 'update' parameter 'value' mutability mismatch",
			secondaryLabels: 1,
		},
		{
			name:          "return type",
			source:        "trait T {\n  fn id() Int\n}\nstruct S {}\nimpl T for S {\n  fn id() Str { \"s\" }\n}\n",
			code:          checker.DiagnosticCodeImplReturnType,
			legacyMessage: "Trait method 'id' has return type of Int",
		},
		{
			name:          "missing method",
			source:        "trait T {\n  fn run()\n}\nstruct S {}\nimpl T for S {\n}\n",
			code:          checker.DiagnosticCodeMissingImplMethod,
			legacyMessage: "Missing method 'run' in trait 'T'",
		},
		{
			name:          "mutating enum method",
			source:        "enum Status { Ready }\nimpl Status {\n  fn mut reset() {}\n}\n",
			code:          checker.DiagnosticCodeMutatingEnumMethod,
			legacyMessage: "Enum methods cannot be mutating",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			c := checker.New("main.ard", result.Program, nil)
			c.Check()
			diagnostic := requireDiagnosticCode(t, c.Diagnostics(), tt.code)
			if tt.legacyMessage != "" && diagnostic.Message != tt.legacyMessage {
				t.Fatalf("legacy message = %q, want %q", diagnostic.Message, tt.legacyMessage)
			}
			if diagnostic.Primary.Span.FilePath != "main.ard" || tt.primaryMessage != "" && diagnostic.Primary.Message != tt.primaryMessage {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if len(diagnostic.Secondary) != tt.secondaryLabels {
				t.Fatalf("secondary = %#v", diagnostic.Secondary)
			}
		})
	}
}

func TestInvalidTestFunctionDiagnosticsUsePreciseSpans(t *testing.T) {
	t.Run("parameters", func(t *testing.T) {
		result := parse.Parse([]byte("test fn invalid(name: Str) Void!Str { Result::ok(()) }\n"), "main.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("parse errors: %v", result.Errors)
		}
		function := result.Program.Statements[0].(*parse.FunctionDeclaration)
		c := checker.New("main.ard", result.Program, nil)
		c.Check()
		diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeTestParametersNotAllowed)
		if diagnostic.Primary.Span.Location != function.Parameters[0].GetLocation() || diagnostic.Primary.Message != "remove test function parameters" {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	})

	t.Run("return annotation", func(t *testing.T) {
		result := parse.Parse([]byte("test fn invalid() Int { 42 }\n"), "main.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("parse errors: %v", result.Errors)
		}
		function := result.Program.Statements[0].(*parse.FunctionDeclaration)
		c := checker.New("main.ard", result.Program, nil)
		c.Check()
		diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeInvalidTestReturnType)
		if diagnostic.Primary.Span.Location != function.ReturnType.GetLocation() || diagnostic.Primary.Message != "test functions must return `Void!Str`" {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	})

	t.Run("missing return annotation", func(t *testing.T) {
		result := parse.Parse([]byte("test fn invalid() {}\n"), "main.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("parse errors: %v", result.Errors)
		}
		function := result.Program.Statements[0].(*parse.FunctionDeclaration)
		c := checker.New("main.ard", result.Program, nil)
		c.Check()
		diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeInvalidTestReturnType)
		if diagnostic.Primary.Span.Location != function.GetLocation() {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	})
}

func TestNonCallableHasStructuredLabels(t *testing.T) {
	result := parse.Parse([]byte("let value = 1\nvalue()\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[0].(*parse.VariableDeclaration)
	call := result.Program.Statements[1]

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeNotCallable)
	if diagnostic.Primary.Span.Location != call.GetLocation() || diagnostic.Message != "Not a function: value" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != declaration.NameLocation {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestMissingArgumentHasParameterProvenance(t *testing.T) {
	result := parse.Parse([]byte("fn add(a: Int, b: Int) {}\nadd(1)\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	function := result.Program.Statements[0].(*parse.FunctionDeclaration)
	call := result.Program.Statements[1]

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeMissingArgument)
	if diagnostic.Primary.Span.Location != call.GetLocation() || diagnostic.Primary.Message != "this call is missing `b`" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != function.Parameters[1].GetLocation() {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestNamedArgumentBindingDiagnosticsUseArgumentSpans(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		result := parse.Parse([]byte("fn greet(name: Str) {}\ngreet(who: \"A\")\n"), "main.ard")
		call := result.Program.Statements[1].(*parse.FunctionCall)
		c := checker.New("main.ard", result.Program, nil)
		c.Check()
		diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeUnknownNamedArgument)
		if diagnostic.Primary.Span.Location != call.Args[0].GetLocation() || diagnostic.Message != "unknown parameter name: who" {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		result := parse.Parse([]byte("fn greet(name: Str) {}\ngreet(name: \"A\", name: \"B\")\n"), "main.ard")
		call := result.Program.Statements[1].(*parse.FunctionCall)
		c := checker.New("main.ard", result.Program, nil)
		c.Check()
		diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeDuplicateArgument)
		if diagnostic.Primary.Span.Location != call.Args[1].GetLocation() || len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != call.Args[0].GetLocation() {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	})
}

func TestIncorrectArgumentCountHasStructuredDiagnostic(t *testing.T) {
	result := parse.Parse([]byte("fn ping() {}\nping(1)\n"), "main.ard")
	call := result.Program.Statements[1]
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeIncorrectArgumentCount)
	if diagnostic.Primary.Span.Location != call.GetLocation() || diagnostic.Message != "Incorrect number of arguments: Expected 0, got 1" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestInvalidFunctionTypeArgumentsHasStructuredDiagnostic(t *testing.T) {
	result := parse.Parse([]byte("fn ping() {}\nping<Int>()\n"), "main.ard")
	call := result.Program.Statements[1]
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeInvalidFunctionTypeArgs)
	if diagnostic.Primary.Span.Location != call.GetLocation() || diagnostic.Message != "function ping does not take type arguments" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestWrongFunctionTypeArgumentCountHasTruthfulLabel(t *testing.T) {
	source := "fn choose(a: $A, b: $B) {}\nchoose<Int>(1, 2)\n"
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeInvalidFunctionTypeArgs)
	if diagnostic.Message != "Expected 2 type arguments, got 1" || diagnostic.Primary.Message != "expected 2 type argument(s), but found 1" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestGoNamedArgumentHasStructuredDiagnostic(t *testing.T) {
	result := parse.Parse([]byte("use go:fmt\nfmt::Println(value: \"hello\")\n"), "main.ard")
	call := result.Program.Statements[0].(*parse.StaticFunction)
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeNamedArgumentsUnsupported)
	if diagnostic.Primary.Span.Location != call.Function.Args[0].GetLocation() || diagnostic.Message != "Go function calls do not support named arguments" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestImmutableMutableReferenceHasStructuredLabels(t *testing.T) {
	result := parse.Parse([]byte("let counter = 0\nlet r = mut counter\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[0].(*parse.VariableDeclaration)
	borrow := result.Program.Statements[1].(*parse.VariableDeclaration)
	operand := borrow.Value.(*parse.MutRef).Operand

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeImmutableMutableReference)
	if diagnostic.Primary.Span.Location != operand.GetLocation() || diagnostic.Primary.Message != "`counter` is immutable" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != declaration.NameLocation || diagnostic.Secondary[0].Message != "this binding is immutable" {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestReferenceRebindingHasStructuredLabels(t *testing.T) {
	source := "mut first = 1\nmut second = 2\nlet ref = mut first\nref = mut second\n"
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[2].(*parse.VariableDeclaration)
	assignment := result.Program.Statements[3].(*parse.VariableAssignment)

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeReferenceRebinding)
	if diagnostic.Primary.Span.Location != assignment.Value.GetLocation() || diagnostic.Primary.Message != "this value would rebind the reference" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != declaration.NameLocation {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestUnreachableReferentAssignmentHasStructuredLabels(t *testing.T) {
	source := "mut items = [1, 2]\nlet ref = mut items\nref = [9, 9]\n"
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[1].(*parse.VariableDeclaration)
	assignment := result.Program.Statements[2].(*parse.VariableAssignment)

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeUnreachableReferentAssignment)
	if diagnostic.Primary.Span.Location != assignment.Target.GetLocation() || len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != declaration.NameLocation {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestImmutablePropertyAssignmentHasStructuredLabels(t *testing.T) {
	source := "struct Box { value: Int }\nlet box = Box{value: 1}\nbox.value = 2\n"
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[1].(*parse.VariableDeclaration)
	assignment := result.Program.Statements[2].(*parse.VariableAssignment)

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeImmutablePropertyAssignment)
	if diagnostic.Primary.Span.Location != assignment.Target.GetLocation() || diagnostic.Primary.Message != "`box.value` is immutable" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != declaration.NameLocation {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestImmutableMutatingReceiverHasStructuredLabels(t *testing.T) {
	result := parse.Parse([]byte("let values = [1]\nvalues.push(2)\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[0].(*parse.VariableDeclaration)

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeImmutableReceiver)
	if diagnostic.Primary.Message != "`.push()` requires a mutable receiver" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != declaration.NameLocation {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestGoConstantAssignmentHasStructuredDiagnostic(t *testing.T) {
	source := "use go:time\nfn main() {\n  time::Nanosecond = 1\n}\n"
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeGoConstantAssignment)
	if diagnostic.Message != "Cannot assign to Go constant: time::Nanosecond" || diagnostic.Primary.Message != "Go constants are not assignable" || len(diagnostic.Secondary) != 0 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestImmutableVariableAssignmentHasStructuredLabels(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte("let count = 0\ncount = 1\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	declaration := result.Program.Statements[0].(*parse.VariableDeclaration).NameLocation
	assignment := result.Program.Statements[1].(*parse.VariableAssignment).Target.GetLocation()
	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	diagnostic := c.Diagnostics()[0]
	if diagnostic.Kind != checker.Error || diagnostic.Code != checker.DiagnosticCodeImmutableAssignment {
		t.Fatalf("kind/code = %q/%q", diagnostic.Kind, diagnostic.Code)
	}
	if diagnostic.Message != "Immutable variable: count" || diagnostic.Title != "Cannot assign to immutable variable" {
		t.Fatalf("message/title = %q/%q", diagnostic.Message, diagnostic.Title)
	}
	if diagnostic.Primary.Span != (checker.SourceSpan{FilePath: filePath, Location: assignment}) || diagnostic.Primary.Message != "cannot assign to `count`" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span != (checker.SourceSpan{FilePath: filePath, Location: declaration}) || diagnostic.Secondary[0].Message != "`count` was declared immutable here" {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestImmutableAssignmentUsesInnermostBindingProvenance(t *testing.T) {
	result := parse.Parse([]byte("let value = 1\nfn main() {\n  let value = 2\n  value = 3\n}\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}
	secondary := c.Diagnostics()[0].Secondary
	if len(secondary) != 1 || secondary[0].Span.Location.Start.Row != 3 {
		t.Fatalf("secondary = %#v, want inner declaration", secondary)
	}
}

func TestMutableArgumentMismatchPointsToParameter(t *testing.T) {
	result := parse.Parse([]byte("fn bump(value: mut Int) {}\nlet value = 1\nbump(value)\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[0].(*parse.FunctionDeclaration)

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeIncorrectArgumentType || diagnostic.Primary.Message != "this argument is not mutable" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != declaration.Parameters[0].GetLocation() {
		t.Fatalf("secondary = %#v, want mutable parameter", diagnostic.Secondary)
	}
	if diagnostic.Secondary[0].Message != "parameter `value` requires a mutable `Int`" {
		t.Fatalf("secondary label = %q", diagnostic.Secondary[0].Message)
	}
}

func TestGenericArgumentMismatchRetainsParameterProvenance(t *testing.T) {
	result := parse.Parse([]byte("fn same(first: $T, second: $T) {}\nsame(1, \"x\")\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[0].(*parse.FunctionDeclaration)

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeIncorrectArgumentType || len(diagnostic.Secondary) != 1 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if diagnostic.Secondary[0].Span.Location != declaration.Parameters[1].GetLocation() {
		t.Fatalf("secondary = %#v, want second parameter", diagnostic.Secondary[0])
	}
}

func TestGoFunctionArgumentOmitsSyntheticParameterLabel(t *testing.T) {
	result := parse.Parse([]byte("use go:fmt\nfmt::Fprint(\"hello\")\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeIncorrectArgumentType {
		t.Fatalf("code = %q, want incorrect argument type", diagnostic.Code)
	}
	if len(diagnostic.Secondary) != 0 {
		t.Fatalf("secondary = %#v, want no source label for Go parameter", diagnostic.Secondary)
	}
	if diagnostic.Primary.Message != "expected `io::Writer`, but this argument has type `Str`" {
		t.Fatalf("primary label = %q", diagnostic.Primary.Message)
	}
}

func TestImportedFunctionArgumentPointsToParameterModule(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.27.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	apiPath := filepath.Join(root, "api.ard")
	apiSource := []byte("fn greet(name: Str) {}\n")
	if err := os.WriteFile(apiPath, apiSource, 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	mainResult := parse.Parse([]byte("use app/api\napi::greet(42)\n"), mainPath)
	if len(mainResult.Errors) > 0 {
		t.Fatalf("main parse errors: %v", mainResult.Errors)
	}
	apiResult := parse.Parse(apiSource, apiPath)
	if len(apiResult.Errors) > 0 {
		t.Fatalf("api parse errors: %v", apiResult.Errors)
	}
	parameterLocation := apiResult.Program.Statements[0].(*parse.FunctionDeclaration).Parameters[0].GetLocation()

	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(mainPath, mainResult.Program, resolver)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeIncorrectArgumentType || len(diagnostic.Secondary) != 1 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	secondary := diagnostic.Secondary[0].Span
	if filepath.Base(secondary.FilePath) != "api.ard" || secondary.Location != parameterLocation {
		t.Fatalf("parameter span = %#v, want api.ard at %v", secondary, parameterLocation)
	}
}

func TestIncorrectFunctionArgumentHasStructuredLabels(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte("fn greet(name: Str) {}\ngreet(42)\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[0].(*parse.FunctionDeclaration)
	call := result.Program.Statements[1].(*parse.FunctionCall)

	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeIncorrectArgumentType {
		t.Fatalf("code = %q, want incorrect argument type", diagnostic.Code)
	}
	if diagnostic.Message != "Type mismatch: Expected Str, got Int" || diagnostic.Title != "Incorrect argument type" {
		t.Fatalf("message/title = %q/%q", diagnostic.Message, diagnostic.Title)
	}
	if diagnostic.Primary.Span.Location != call.Args[0].Value.GetLocation() || diagnostic.Primary.Message != "this argument has type `Int`" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != declaration.Parameters[0].GetLocation() {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
	if diagnostic.Secondary[0].Message != "parameter `name` requires `Str`" {
		t.Fatalf("secondary label = %q", diagnostic.Secondary[0].Message)
	}
}

func requireDiagnosticCode(t *testing.T, diagnostics []checker.Diagnostic, code checker.DiagnosticCode) checker.Diagnostic {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return diagnostic
		}
	}
	t.Fatalf("diagnostics = %#v, want code %q", diagnostics, code)
	return checker.Diagnostic{}
}

func TestFunctionReturnMismatchHasStructuredLabels(t *testing.T) {
	result := parse.Parse([]byte("fn answer() Int {\n  false\n}\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	function := result.Program.Statements[0].(*parse.FunctionDeclaration)

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeTypeMismatch)
	if diagnostic.Primary.Span.Location != function.Body[0].GetLocation() || diagnostic.Primary.Message != "this expression has type `Bool`" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != function.ReturnType.GetLocation() || diagnostic.Secondary[0].Message != "this return annotation requires `Int`" {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestIfBranchMismatchHasStructuredLabels(t *testing.T) {
	result := parse.Parse([]byte("if true {\n  1\n} else {\n  \"no\"\n}\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	chain := result.Program.Statements[0].(*parse.IfStatement)
	elseBranch := chain.Else.(*parse.IfStatement)

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeBranchTypeMismatch)
	if diagnostic.Message != "All branches must have the same result type" || diagnostic.Primary.Span.Location != elseBranch.Body[0].GetLocation() {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != chain.Body[0].GetLocation() {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestMatchBranchMismatchHasStructuredDiagnostic(t *testing.T) {
	source := `
fn side_effect() {}
fn get() Str!Str { Result::ok("ok") }
fn main() {
  let category = match get() {
    ok(value) => value,
    err(_) => side_effect(),
  }
}
`
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeBranchTypeMismatch)
	if diagnostic.Message != "Type mismatch in match branches: expected Str, got Void" || diagnostic.Title != "Incompatible match branch types" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if diagnostic.Primary.Message != "this branch produces `Void`" || len(diagnostic.Secondary) != 0 {
		t.Fatalf("labels = %#v / %#v", diagnostic.Primary, diagnostic.Secondary)
	}
}

func TestValueIfWithoutElseHasStructuredDiagnostic(t *testing.T) {
	result := parse.Parse([]byte("fn answer() Int {\n  if true {\n    1\n  }\n}\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	function := result.Program.Statements[0].(*parse.FunctionDeclaration)
	chain := function.Body[0].(*parse.IfStatement)

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeNonExhaustiveValueIf)
	if diagnostic.Message != "if used as a value must have an else branch" || diagnostic.Primary.Span.Location != chain.GetLocation() {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestInvalidMapKeyTypeHasStructuredDiagnostic(t *testing.T) {
	tests := []struct {
		name   string
		source string
		span   func(*parse.Program) parse.Location
	}{
		{
			name:   "declared map key",
			source: "let values: [[Int]: Str] = [:]\n",
			span: func(program *parse.Program) parse.Location {
				declaration := program.Statements[0].(*parse.VariableDeclaration)
				return declaration.Type.(*parse.Map).Key.GetLocation()
			},
		},
		{
			name:   "inferred map key",
			source: "let values = [[1]: \"one\"]\n",
			span: func(program *parse.Program) parse.Location {
				declaration := program.Statements[0].(*parse.VariableDeclaration)
				return declaration.Value.(*parse.MapLiteral).Entries[0].Key.GetLocation()
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			c := checker.New("main.ard", result.Program, nil)
			c.Check()
			diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeInvalidMapKeyType)
			if diagnostic.Primary.Span.Location != tt.span(result.Program) || diagnostic.Primary.Message != "`[Int]` cannot be used as a map key" {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if len(diagnostic.Secondary) != 0 {
				t.Fatalf("secondary = %#v", diagnostic.Secondary)
			}
		})
	}
}

func TestGenericTypeUsageHasStructuredDiagnostics(t *testing.T) {
	tests := []struct {
		name          string
		source        string
		code          checker.DiagnosticCode
		legacyMessage string
	}{
		{
			name:          "non-generic specialization",
			source:        "struct Plain {\n  value: Int,\n}\nfn consume(value: Plain<Int>) {}\n",
			code:          checker.DiagnosticCodeNonGenericSpecialization,
			legacyMessage: "Type is not generic and cannot be specialized.",
		},
		{
			name:          "incorrect type argument count",
			source:        "struct Pair {\n  first: $A,\n  second: $B,\n}\nfn consume(value: Pair<Int>) {}\n",
			code:          checker.DiagnosticCodeIncorrectTypeArgCount,
			legacyMessage: "Incorrect number of type arguments: expected 2, got 1",
		},
		{
			name:          "missing named type arguments",
			source:        "struct Box {\n  value: $T,\n}\nfn consume(value: Box) {}\n",
			code:          checker.DiagnosticCodeMissingTypeArguments,
			legacyMessage: "Generic type Box requires type arguments",
		},
		{
			name:          "missing builtin type arguments",
			source:        "fn consume(value: Maybe) {}\n",
			code:          checker.DiagnosticCodeIncorrectTypeArgCount,
			legacyMessage: "Generic type Maybe requires type arguments",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			c := checker.New("main.ard", result.Program, nil)
			c.Check()
			diagnostic := requireDiagnosticCode(t, c.Diagnostics(), tt.code)
			if diagnostic.Message != tt.legacyMessage || diagnostic.Primary.Span.FilePath != "main.ard" || diagnostic.Primary.Message == "" {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
		})
	}
}

func TestGenericDeclarationRulesHaveStructuredDiagnostics(t *testing.T) {
	t.Run("recursive generic self-reference", func(t *testing.T) {
		result := parse.Parse([]byte("struct Node {\n  next: Node<$T>,\n}\n"), "main.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("parse errors: %v", result.Errors)
		}
		c := checker.New("main.ard", result.Program, nil)
		c.Check()
		diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeRecursiveGenericReference)
		if diagnostic.Message != "Recursive generic self-reference Node is not supported yet" {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	})

	t.Run("method introduced generic", func(t *testing.T) {
		source := "struct Box {\n  item: Int,\n}\nimpl Box {\n  fn get(value: $U) Int { self.item }\n}\n"
		result := parse.Parse([]byte(source), "main.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("parse errors: %v", result.Errors)
		}
		c := checker.New("main.ard", result.Program, nil)
		c.Check()
		diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeMethodIntroducedGeneric)
		if diagnostic.Primary.Message != "`$U` is not a generic parameter of the receiver type" {
			t.Fatalf("diagnostic = %#v", diagnostic)
		}
	})
}

func TestUnboundGenericTypeArgumentHasStructuredDiagnostic(t *testing.T) {
	result := parse.Parse([]byte("fn raw(value: $T) $T { value }\nraw<$U>(1)\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeUnboundGenericTypeArg)
	if diagnostic.Message != "unbound generic type argument $U" || diagnostic.Primary.Message != "`$U` cannot be used as a type argument here" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestBuiltInTypeRedeclarationHasStructuredDiagnostic(t *testing.T) {
	result := parse.Parse([]byte("struct Sender {}\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[0].(*parse.StructDefinition)

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeBuiltInTypeRedeclaration || diagnostic.Message != "Sender is a built-in type and cannot be redeclared" {
		t.Fatalf("code/message = %q/%q", diagnostic.Code, diagnostic.Message)
	}
	if diagnostic.Primary.Span.Location != declaration.Name.GetLocation() || len(diagnostic.Secondary) != 0 {
		t.Fatalf("labels = %#v / %#v", diagnostic.Primary, diagnostic.Secondary)
	}
}

func TestRecursiveTypeAliasHasStructuredCycleLabels(t *testing.T) {
	result := parse.Parse([]byte("type A = B\ntype B = A\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	first := result.Program.Statements[0].(*parse.TypeDeclaration)
	second := result.Program.Statements[1].(*parse.TypeDeclaration)

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeRecursiveTypeAlias || diagnostic.Message != "Recursive type alias: A" {
		t.Fatalf("code/message = %q/%q", diagnostic.Code, diagnostic.Message)
	}
	if diagnostic.Primary.Span.Location != second.Type[0].GetLocation() {
		t.Fatalf("primary = %#v, want closing B -> A reference", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != first.Type[0].GetLocation() {
		t.Fatalf("secondary = %#v, want opening A -> B reference", diagnostic.Secondary)
	}
}

func TestRecursiveTypeAliasDirectCycleHasNoSecondaryLabel(t *testing.T) {
	result := parse.Parse([]byte("type Node = Node\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[0].(*parse.TypeDeclaration)
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 || c.Diagnostics()[0].Code != checker.DiagnosticCodeRecursiveTypeAlias {
		t.Fatalf("diagnostics = %#v", c.Diagnostics())
	}
	if c.Diagnostics()[0].Primary.Span.Location != declaration.Type[0].GetLocation() || len(c.Diagnostics()[0].Secondary) != 0 {
		t.Fatalf("labels = %#v / %#v", c.Diagnostics()[0].Primary, c.Diagnostics()[0].Secondary)
	}
}

func TestNestedRecursiveTypeAliasesHaveCompleteCycleLabels(t *testing.T) {
	result := parse.Parse([]byte("type A = [B]\ntype B = A\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	first := result.Program.Statements[0].(*parse.TypeDeclaration)
	second := result.Program.Statements[1].(*parse.TypeDeclaration)
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 || c.Diagnostics()[0].Code != checker.DiagnosticCodeRecursiveTypeAlias {
		t.Fatalf("diagnostics = %#v", c.Diagnostics())
	}
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Primary.Span.Location != second.Type[0].GetLocation() {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != first.Type[0].(*parse.List).Element.GetLocation() {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestNestedDirectRecursiveTypeAliasDoesNotPanic(t *testing.T) {
	result := parse.Parse([]byte("type Node = [Node]\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 || c.Diagnostics()[0].Code != checker.DiagnosticCodeRecursiveTypeAlias {
		t.Fatalf("diagnostics = %#v", c.Diagnostics())
	}
}

func TestNonRecursiveTypeAliasChainIsAllowed(t *testing.T) {
	result := parse.Parse([]byte("type A = [Int]\ntype B = A\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 0 {
		t.Fatalf("diagnostics = %#v, want none", c.Diagnostics())
	}
}

func TestRecursiveStructLayoutHasStructuredCycleLabels(t *testing.T) {
	result := parse.Parse([]byte("struct A {\n  b: B,\n}\nstruct B {\n  a: A,\n}\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	first := result.Program.Statements[0].(*parse.StructDefinition)
	second := result.Program.Statements[1].(*parse.StructDefinition)

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeRecursiveStructLayout || diagnostic.Message != "Recursive field A.b has infinite size. "+"Put the recursive reference behind mut, list, map, nullable, trait, or function indirection." {
		t.Fatalf("code/message = %q/%q", diagnostic.Code, diagnostic.Message)
	}
	if diagnostic.Primary.Span.Location != first.Fields[0].Name.GetLocation() {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != second.Fields[0].Name.GetLocation() {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestRecursiveStructLayoutAllowsIndirectRecursion(t *testing.T) {
	result := parse.Parse([]byte("struct Node {\n  children: [Node],\n}\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 0 {
		t.Fatalf("diagnostics = %#v, want none", c.Diagnostics())
	}
}

type failingGoResolver struct{}

func (failingGoResolver) ResolveGoPackage(string) (*checker.GoPackage, error) {
	return nil, errors.New("package unavailable")
}

func TestFailedGoImportHasStructuredDiagnostic(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte("use go:example.invalid/pkg\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	c := checker.New(filePath, result.Program, nil, checker.CheckOptions{GoResolver: failingGoResolver{}})
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeGoImportResolution || diagnostic.Text != "package unavailable" {
		t.Fatalf("code/text = %q/%q", diagnostic.Code, diagnostic.Text)
	}
	if diagnostic.Primary.Span.Location != result.Program.Imports[0].PathLocation {
		t.Fatalf("primary span = %v, want path span %v", diagnostic.Primary.Span.Location, result.Program.Imports[0].PathLocation)
	}
	if diagnostic.Primary.Message != "could not resolve Go package `example.invalid/pkg`" {
		t.Fatalf("primary message = %q", diagnostic.Primary.Message)
	}
}

func TestFailedArdImportHasStructuredDiagnostic(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, "main.ard")
	result := parse.Parse([]byte("use app/missing\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	c := checker.New(filePath, result.Program, resolver, checker.CheckOptions{ModulePath: "app/main"})
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeImportResolution {
		t.Fatalf("code = %q", diagnostic.Code)
	}
	if diagnostic.Primary.Span.Location != result.Program.Imports[0].PathLocation || diagnostic.Primary.Message != "could not resolve module `app/missing`" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if diagnostic.Text == "" {
		t.Fatal("resolution cause was omitted")
	}
}

func TestCircularImportHasStructuredDiagnostic(t *testing.T) {
	root := t.TempDir()
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ard.toml", "name = \"app\"\nard = \">= 0.1.0\"\n")
	write("a.ard", "use app/b\n")
	write("b.ard", "use app/a\n")

	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, "main.ard")
	result := parse.Parse([]byte("use app/a\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	aResult := parse.Parse([]byte("use app/b\n"), filepath.Join(root, "a.ard"))
	bResult := parse.Parse([]byte("use app/a\n"), filepath.Join(root, "b.ard"))

	c := checker.New(filePath, result.Program, resolver, checker.CheckOptions{ModulePath: "app/main"})
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeCircularImport || diagnostic.Text != "app/a -> app/b -> app/a" {
		t.Fatalf("code/text = %q/%q", diagnostic.Code, diagnostic.Text)
	}
	if diagnostic.Primary.Span.FilePath != filePath || diagnostic.Primary.Span.Location != result.Program.Imports[0].PathLocation {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 2 {
		t.Fatalf("secondary = %#v, want both cycle edges", diagnostic.Secondary)
	}
	if diagnostic.Secondary[0].Span.FilePath != filepath.Join(root, "a.ard") || diagnostic.Secondary[0].Span.Location != aResult.Program.Imports[0].PathLocation {
		t.Fatalf("first secondary = %#v", diagnostic.Secondary[0])
	}
	if diagnostic.Secondary[1].Span.FilePath != filepath.Join(root, "b.ard") || diagnostic.Secondary[1].Span.Location != bResult.Program.Imports[0].PathLocation {
		t.Fatalf("second secondary = %#v", diagnostic.Secondary[1])
	}
}

func TestModuleLoadFailureHasStructuredDiagnostic(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "broken.ard"), []byte("fn broken("), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, "main.ard")
	result := parse.Parse([]byte("use app/broken\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	c := checker.New(filePath, result.Program, resolver, checker.CheckOptions{ModulePath: "app/main"})
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeModuleLoadFailure {
		t.Fatalf("code = %q", diagnostic.Code)
	}
	if diagnostic.Primary.Span.Location != result.Program.Imports[0].PathLocation || diagnostic.Primary.Message != "module `app/broken` could not be loaded" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if diagnostic.Text == "" {
		t.Fatal("load cause was omitted")
	}
}

func TestImportFailuresLeaveResolverReadyForLaterChecks(t *testing.T) {
	root := t.TempDir()
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ard.toml", "name = \"app\"\nard = \">= 0.1.0\"\n")
	write("broken.ard", "fn broken(")
	write("a.ard", "use app/b\n")
	write("b.ard", "use app/a\n")
	write("valid.ard", "fn value() Int { 1 }\n")

	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	checkImport := func(fileName, modulePath, source string) []checker.Diagnostic {
		t.Helper()
		filePath := filepath.Join(root, fileName)
		result := parse.Parse([]byte(source), filePath)
		if len(result.Errors) > 0 {
			t.Fatalf("parse errors: %v", result.Errors)
		}
		c := checker.New(filePath, result.Program, resolver, checker.CheckOptions{ModulePath: modulePath})
		c.Check()
		return c.Diagnostics()
	}

	if got := checkImport("first.ard", "app/first", "use app/broken\n"); len(got) != 1 || got[0].Code != checker.DiagnosticCodeModuleLoadFailure {
		t.Fatalf("load diagnostics = %#v", got)
	}
	if got := checkImport("second.ard", "app/second", "use app/a\n"); len(got) != 1 || got[0].Code != checker.DiagnosticCodeCircularImport {
		t.Fatalf("cycle diagnostics = %#v", got)
	}
	if got := checkImport("third.ard", "app/third", "use app/valid\n"); len(got) != 0 {
		t.Fatalf("valid import after failures produced diagnostics: %#v", got)
	}
}

func TestDuplicateImportHasStructuredLabels(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte("use ard/list as shared\nuse ard/map as shared\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	original := result.Program.Imports[0].PathLocation
	duplicate := result.Program.Imports[1].PathLocation
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Kind != checker.Warn || diagnostic.Code != checker.DiagnosticCodeDuplicateImport {
		t.Fatalf("kind/code = %q/%q", diagnostic.Kind, diagnostic.Code)
	}
	if diagnostic.Message != "[2:1] Duplicate import: shared" {
		t.Fatalf("legacy message = %q", diagnostic.Message)
	}
	if diagnostic.Primary.Span.Location != duplicate || diagnostic.Primary.Message != "`shared` is imported again here" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != original || diagnostic.Secondary[0].Message != "first imported here" {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestDuplicateStructFieldHasStructuredLabels(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte("struct User {\n  name: Str,\n  name: Int,\n}\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	definition := result.Program.Statements[0].(*parse.StructDefinition)
	original := definition.Fields[0].Name.GetLocation()
	duplicate := definition.Fields[1].Name.GetLocation()
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeDuplicateFieldDeclaration {
		t.Fatalf("code = %q, want duplicate field declaration", diagnostic.Code)
	}
	if diagnostic.Message != "Duplicate field: name" || diagnostic.Title != "Duplicate field declaration" {
		t.Fatalf("message/title = %q/%q", diagnostic.Message, diagnostic.Title)
	}
	if diagnostic.Primary.Span.Location != duplicate || diagnostic.Primary.Message != "field `name` is declared again here" {
		t.Fatalf("primary = %#v", diagnostic.Primary)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != original || diagnostic.Secondary[0].Message != "first declared here" {
		t.Fatalf("secondary = %#v", diagnostic.Secondary)
	}
}

func TestDuplicateTopLevelTypeDeclarationHasStructuredLabels(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte("struct User {}\nenum User { guest }\n"), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	original := result.Program.Statements[0].(*parse.StructDefinition).Name.GetLocation()
	duplicate := result.Program.Statements[1].(*parse.EnumDefinition).NameLocation
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeDuplicateDeclaration {
		t.Fatalf("code = %q, want duplicate declaration", diagnostic.Code)
	}
	if diagnostic.Message != "Duplicate declaration: User" {
		t.Fatalf("legacy message = %q", diagnostic.Message)
	}
	if diagnostic.Primary.Span.FilePath != filePath || diagnostic.Primary.Span.Location != duplicate {
		t.Fatalf("primary span = %#v, want second declaration at %v", diagnostic.Primary.Span, duplicate)
	}
	if diagnostic.Primary.Message != "`User` is declared again here" {
		t.Fatalf("primary label = %q", diagnostic.Primary.Message)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != original {
		t.Fatalf("secondary labels = %#v, want first declaration at %v", diagnostic.Secondary, original)
	}
	if diagnostic.Secondary[0].Message != "first declared here" {
		t.Fatalf("secondary label = %q", diagnostic.Secondary[0].Message)
	}
}

func TestDuplicateTopLevelTypesPointBackToFirstDeclaration(t *testing.T) {
	result := parse.Parse([]byte("struct User {}\nenum User { guest }\ntrait User {\n}\n"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	first := result.Program.Statements[0].(*parse.StructDefinition).Name.GetLocation()

	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 2 {
		t.Fatalf("diagnostics = %#v, want two", c.Diagnostics())
	}
	for i, diagnostic := range c.Diagnostics() {
		if diagnostic.Primary.Span.Location.Start.Row != i+2 {
			t.Fatalf("diagnostic %d primary = %#v", i, diagnostic.Primary.Span)
		}
		if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != first {
			t.Fatalf("diagnostic %d secondary = %#v, want first declaration", i, diagnostic.Secondary)
		}
	}
}

func TestAnnotatedBindingTypeMismatchHasStructuredLabels(t *testing.T) {
	const filePath = "main.ard"
	result := parse.Parse([]byte(`let name: Str = 42`), filePath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	binding, ok := result.Program.Statements[0].(*parse.VariableDeclaration)
	if !ok {
		t.Fatalf("statement = %T, want *parse.VariableDeclaration", result.Program.Statements[0])
	}

	c := checker.New(filePath, result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}

	diagnostic := c.Diagnostics()[0]
	if diagnostic.Kind != checker.Error {
		t.Fatalf("kind = %q, want error", diagnostic.Kind)
	}
	if diagnostic.Code != checker.DiagnosticCodeTypeMismatch {
		t.Fatalf("code = %q, want %q", diagnostic.Code, checker.DiagnosticCodeTypeMismatch)
	}
	if diagnostic.Title != "Type mismatch" {
		t.Fatalf("title = %q, want Type mismatch", diagnostic.Title)
	}
	if diagnostic.Primary.Span.FilePath != filePath {
		t.Fatalf("primary file = %q, want %q", diagnostic.Primary.Span.FilePath, filePath)
	}
	if diagnostic.Primary.Span.Location != binding.Value.GetLocation() {
		t.Fatalf("primary location = %v, want initializer %v", diagnostic.Primary.Span.Location, binding.Value.GetLocation())
	}
	if diagnostic.Primary.Message != "this expression has type `Int`" {
		t.Fatalf("primary label = %q", diagnostic.Primary.Message)
	}
	if len(diagnostic.Secondary) != 1 {
		t.Fatalf("secondary labels = %#v, want one", diagnostic.Secondary)
	}
	related := diagnostic.Secondary[0]
	if related.Span.FilePath != filePath || related.Span.Location != binding.Type.GetLocation() {
		t.Fatalf("related span = %#v, want annotation in %s at %v", related.Span, filePath, binding.Type.GetLocation())
	}
	if related.Message != "this annotation requires `Str`" {
		t.Fatalf("related label = %q", related.Message)
	}
}
