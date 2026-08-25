package checker_test

import (
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func checkAttributes(t *testing.T, source string) (*checker.Checker, *checker.StructDef) {
	t.Helper()
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	var def *checker.StructDef
	for _, statement := range c.Module().Program().Statements {
		if candidate, ok := statement.Stmt.(*checker.StructDef); ok {
			def = candidate
			break
		}
	}
	return c, def
}

func TestJSONFieldAttributesAreChecked(t *testing.T) {
	c, def := checkAttributes(t, `struct User {
  #json(name: "displayName", omit: none)
  display_name: Str?,
  #json(skip: true)
  password_hash: Str,
  #json(name: "snow☃")
  weather_code: Int,
}`)
	if diagnostics := c.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if def == nil {
		t.Fatal("missing checked struct")
	}
	display, ok := checker.StructFieldJSON(def, "display_name")
	if !ok || !display.HasName || display.Name != "displayName" || !display.OmitNone || display.Skip {
		t.Fatalf("display metadata = %#v, found=%v", display, ok)
	}
	password, ok := checker.StructFieldJSON(def, "password_hash")
	if !ok || !password.Skip || password.HasName || password.OmitNone {
		t.Fatalf("password metadata = %#v, found=%v", password, ok)
	}
	weather, ok := checker.StructFieldJSON(def, "weather_code")
	if !ok || !weather.HasName || weather.Name != "snow☃" {
		t.Fatalf("weather metadata = %#v, found=%v", weather, ok)
	}
}

func TestGoFieldTagAttributesAreChecked(t *testing.T) {
	c, def := checkAttributes(t, `struct Config<$T> {
  #json(name: "value")
  #go:yaml("global_context,omitempty")
  #go:validate("required")
  #go:type("kind")
  value: $T,
}`)
	if diagnostics := c.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if def == nil {
		t.Fatal("missing checked struct")
	}
	tags := checker.StructFieldGoTags(def, "value")
	if len(tags) != 3 || tags[0].Key != "yaml" || tags[0].Value != "global_context,omitempty" || tags[1].Key != "validate" || tags[1].Value != "required" || tags[2].Key != "type" || tags[2].Value != "kind" {
		t.Fatalf("Go field tags = %#v", tags)
	}
}

func TestInvalidGoFieldTagAttributesAreDiagnosed(t *testing.T) {
	tests := []struct {
		name    string
		attrs   string
		code    checker.DiagnosticCode
		message string
	}{
		{"unknown namespace", `#rust:serde("name")`, checker.DiagnosticCodeUnknownAttribute, "Unknown attribute: #rust:serde"},
		{"missing argument", `#go:yaml`, checker.DiagnosticCodeInvalidAttributeArgument, "#go:yaml requires exactly one positional string argument"},
		{"too many arguments", `#go:yaml("name", "other")`, checker.DiagnosticCodeInvalidAttributeArgument, "#go:yaml requires exactly one positional string argument"},
		{"named argument", `#go:yaml(name: "value")`, checker.DiagnosticCodeInvalidAttributeArgument, "#go:yaml requires exactly one positional string argument"},
		{"non-string argument", `#go:yaml(true)`, checker.DiagnosticCodeInvalidAttributeArgument, "#go:yaml requires exactly one positional string argument"},
		{"reserved JSON tag", `#go:json("name")`, checker.DiagnosticCodeInvalidAttributeArgument, "#go:json is reserved; use #json"},
		{"duplicate tag", "#go:yaml(\"one\")\n  #go:yaml(\"two\")", checker.DiagnosticCodeDuplicateAttribute, "Duplicate attribute: #go:yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := checkAttributes(t, "struct Example {\n  "+tt.attrs+"\n  value: Str,\n}\n")
			diagnostics := c.Diagnostics()
			if len(diagnostics) != 1 {
				t.Fatalf("diagnostics = %#v, want one", diagnostics)
			}
			diagnostic := requireDiagnosticCode(t, diagnostics, tt.code)
			if diagnostic.Message != tt.message {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if tt.name == "duplicate tag" && len(diagnostic.Secondary) != 1 {
				t.Fatalf("duplicate diagnostic = %#v", diagnostic)
			}
		})
	}
}

func TestInvalidGoFieldTagDoesNotSuppressValidJSONMetadata(t *testing.T) {
	for _, attributes := range []string{
		"#json(name: \"wire\")\n  #go:yaml(true)",
		"#go:yaml(true)\n  #json(name: \"wire\")",
	} {
		c, def := checkAttributes(t, "struct Example {\n  "+attributes+"\n  value: Str,\n}\n")
		if diagnostics := c.Diagnostics(); len(diagnostics) != 1 || diagnostics[0].Message != "#go:yaml requires exactly one positional string argument" {
			t.Fatalf("diagnostics = %#v", diagnostics)
		}
		json, ok := checker.StructFieldJSON(def, "value")
		if !ok || !json.HasName || json.Name != "wire" {
			t.Fatalf("JSON metadata = %#v, found=%v", json, ok)
		}
	}
}

func TestInvalidJSONMetadataDoesNotSuppressValidGoFieldTag(t *testing.T) {
	c, def := checkAttributes(t, `struct Example {
  #go:yaml("wire")
  #json(skip: false)
  value: Str,
}`)
	if diagnostics := c.Diagnostics(); len(diagnostics) != 1 || diagnostics[0].Message != "#json(skip: false) has no effect" {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	tags := checker.StructFieldGoTags(def, "value")
	if len(tags) != 1 || tags[0].Key != "yaml" || tags[0].Value != "wire" {
		t.Fatalf("Go field tags = %#v", tags)
	}
}

func TestInvalidGoFieldTagDoesNotSuppressAggregateJSONDiagnostics(t *testing.T) {
	c, _ := checkAttributes(t, `struct Secret {
  #json(skip: true)
  #go:yaml(true)
  value: Str,
}`)
	if diagnostics := c.Diagnostics(); len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want Go tag and all-skipped JSON errors", diagnostics)
	}
	requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeInvalidAttributeArgument)
	messages := []string{c.Diagnostics()[0].Message, c.Diagnostics()[1].Message}
	if !(strings.Contains(strings.Join(messages, "\n"), "#go:yaml requires") && strings.Contains(strings.Join(messages, "\n"), "#json cannot skip every field")) {
		t.Fatalf("diagnostics = %#v", c.Diagnostics())
	}
}

func TestInvalidJSONFieldAttributesAreDiagnosed(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		code    checker.DiagnosticCode
		message string
	}{
		{"unknown attribute", `#metadata(name: "value")\n  value: Str`, checker.DiagnosticCodeUnknownAttribute, "Unknown attribute: metadata"},
		{"unknown argument", `#json(rename: "value")\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "Unknown #json argument: rename"},
		{"duplicate attribute", `#json(name: "one")\n  #json(name: "two")\n  value: Str`, checker.DiagnosticCodeDuplicateAttribute, "Duplicate attribute: #json"},
		{"duplicate argument", `#json(name: "one", name: "two")\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "Duplicate #json argument: name"},
		{"name type", `#json(name: true)\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json argument `name` must be a string"},
		{"empty name", `#json(name: "")\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json name cannot be represented by Go 1.27 JSON struct tags"},
		{"dash name", `#json(name: "-")\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json name cannot be represented by Go 1.27 JSON struct tags"},
		{"comma name", `#json(name: "a,b")\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json name cannot be represented by Go 1.27 JSON struct tags"},
		{"backslash name", `#json(name: "a\\b")\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json name cannot be represented by Go 1.27 JSON struct tags"},
		{"apostrophe name", `#json(name: "a'b")\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json name cannot be represented by Go 1.27 JSON struct tags"},
		{"quote name", `#json(name: "a\"b")\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json name cannot be represented by Go 1.27 JSON struct tags"},
		{"backtick name", "#json(name: \"a`b\")\\n  value: Str", checker.DiagnosticCodeInvalidAttributeArgument, "#json name cannot be represented by Go 1.27 JSON struct tags"},
		{"omit value", `#json(omit: empty)\n  value: Str?`, checker.DiagnosticCodeInvalidAttributeArgument, "#json argument `omit` only supports `none`"},
		{"omit type", `#json(omit: none)\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json(omit: none) requires a nullable field"},
		{"skip type", `#json(skip: "yes")\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json argument `skip` must be `true`"},
		{"skip false", `#json(skip: false)\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json(skip: false) has no effect"},
		{"skip conflict", `#json(name: "value", skip: true)\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json argument `skip` cannot be combined with `name` or `omit`"},
		{"no arguments", `#json\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json requires at least one argument"},
		{"positional argument", `#json("value")\n  value: Str`, checker.DiagnosticCodeInvalidAttributeArgument, "#json only accepts named arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := strings.ReplaceAll(tt.field, `\n`, "\n")
			c, _ := checkAttributes(t, "struct Example {\n  "+field+",\n}\n")
			diagnostics := c.Diagnostics()
			if len(diagnostics) != 1 {
				t.Fatalf("diagnostics = %#v, want one", diagnostics)
			}
			diagnostic := requireDiagnosticCode(t, diagnostics, tt.code)
			if diagnostic.Message != tt.message || diagnostic.Primary.Message == "" {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
		})
	}
}

func TestInvalidJSONMetadataDoesNotCascadeIntoAggregateDiagnostics(t *testing.T) {
	c, _ := checkAttributes(t, `struct Example {
  #json(name: "wire", skip: true)
  a: Str,
  #json(name: "a")
  b: Str,
}`)
	diagnostics := c.Diagnostics()
	if len(diagnostics) != 1 || diagnostics[0].Message != "#json argument `skip` cannot be combined with `name` or `omit`" {
		t.Fatalf("diagnostics = %#v, want only the argument conflict", diagnostics)
	}
}

func TestJSONCannotSkipEveryField(t *testing.T) {
	c, _ := checkAttributes(t, `struct Secret {
  #json(skip: true)
  #go:yaml("value")
  value: Str,
}`)
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeInvalidAttributeArgument)
	if diagnostic.Message != "#json cannot skip every field in a non-empty struct" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestDuplicateJSONFieldNamesAreDiagnosed(t *testing.T) {
	c, _ := checkAttributes(t, `struct User {
  #json(name: "id")
  user_id: Int,
  id: Str,
}`)
	diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeDuplicateJSONFieldName)
	if diagnostic.Message != "Duplicate JSON field name: id" || len(diagnostic.Secondary) != 1 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}
