package parse

import (
	"testing"
	"time"
)

func TestStructFieldAttributes(t *testing.T) {
	result := Parse([]byte(`struct User {
  #json(name: "displayName", omit: none)
  display_name: Str?,
  #json(skip: true)
  password_hash: Str,
}`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	def := result.Program.Statements[0].(*StructDefinition)
	if got := len(def.Fields); got != 2 {
		t.Fatalf("field count = %d, want 2", got)
	}
	first := def.Fields[0]
	if got := len(first.Attributes); got != 1 {
		t.Fatalf("attribute count = %d, want 1", got)
	}
	attribute := first.Attributes[0]
	if attribute.Name.Name != "json" {
		t.Fatalf("attribute name = %q, want json", attribute.Name.Name)
	}
	if got := len(attribute.Arguments); got != 2 {
		t.Fatalf("argument count = %d, want 2", got)
	}
	if argument := attribute.Arguments[0]; argument.Name != "name" || argument.Value.Kind != AttributeString || argument.Value.Text != "displayName" {
		t.Fatalf("name argument = %#v", argument)
	}
	if argument := attribute.Arguments[1]; argument.Name != "omit" || argument.Value.Kind != AttributeSymbol || argument.Value.Text != "none" {
		t.Fatalf("omit argument = %#v", argument)
	}
	if argument := def.Fields[1].Attributes[0].Arguments[0]; argument.Name != "skip" || argument.Value.Kind != AttributeBool || !argument.Value.Bool {
		t.Fatalf("skip argument = %#v", argument)
	}
}

func TestAttributeStaticValues(t *testing.T) {
	result := Parse([]byte(`struct Value {
  #metadata("text", -2, true, symbol, [1, false])
  field: Str,
}`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	values := result.Program.Statements[0].(*StructDefinition).Fields[0].Attributes[0].Arguments
	wantKinds := []AttributeValueKind{AttributeString, AttributeInteger, AttributeBool, AttributeSymbol, AttributeList}
	if len(values) != len(wantKinds) {
		t.Fatalf("argument count = %d, want %d", len(values), len(wantKinds))
	}
	for i, want := range wantKinds {
		if values[i].Value.Kind != want {
			t.Fatalf("argument %d kind = %q, want %q", i, values[i].Value.Kind, want)
		}
	}
	if values[1].Value.Text != "-2" {
		t.Fatalf("integer = %q, want -2", values[1].Value.Text)
	}
	if items := values[4].Value.Items; len(items) != 2 || items[0].Text != "1" || items[1].Kind != AttributeBool || items[1].Bool {
		t.Fatalf("list value = %#v", items)
	}
}

func TestCommentsBetweenAttributesAndFieldsPreserveFollowingFields(t *testing.T) {
	result := Parse([]byte(`struct Value {
  #json(name: "external")
  // Wire name differs from the Ard name.
  value: Str,
  next: Int,
}`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	def := result.Program.Statements[0].(*StructDefinition)
	if len(def.Fields) != 2 || def.Fields[0].Name.Name != "value" || def.Fields[1].Name.Name != "next" {
		t.Fatalf("fields = %#v", def.Fields)
	}
	if len(def.Comments) != 1 || def.Comments[0].Value != "Wire name differs from the Ard name." {
		t.Fatalf("comments = %#v", def.Comments)
	}
}

func TestDanglingStructFieldAttributesAreRejected(t *testing.T) {
	for _, source := range []string{
		"struct Value {\n  #json(skip: true)\n}\n",
		"struct Value {\n  #json(skip: true)\n  // missing field\n}\n",
	} {
		result := Parse([]byte(source), "test.ard")
		if len(result.Errors) != 1 || result.Errors[0].Message != "Expected field after attribute" {
			t.Fatalf("errors for %q = %#v", source, result.Errors)
		}
	}
}

func TestMalformedAttributesTerminate(t *testing.T) {
	for _, source := range []string{
		"struct Value {\n  #json(name: \"x\"\n}\n",
		"struct Value {\n  #metadata([1, 2)\n  field: Str,\n}\n",
	} {
		done := make(chan ParseResult, 1)
		go func() { done <- Parse([]byte(source), "test.ard") }()
		select {
		case result := <-done:
			if len(result.Errors) == 0 {
				t.Fatalf("expected parse errors for %q", source)
			}
		case <-time.After(time.Second):
			t.Fatalf("parser did not terminate for %q", source)
		}
	}
}

func TestAttributesAreRejectedOutsideStructFields(t *testing.T) {
	result := Parse([]byte("#json(name: \"run\")\nfn run() {}\n"), "test.ard")
	if len(result.Errors) == 0 || result.Errors[0].Message != "Attributes are only supported on struct fields" {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestAttributeArgumentsCannotMixForms(t *testing.T) {
	result := Parse([]byte("struct Value {\n  #metadata(name: \"value\", symbol)\n  field: Str,\n}\n"), "test.ard")
	if len(result.Errors) == 0 || result.Errors[0].Message != "Attribute arguments cannot mix named and positional forms" {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestAttributeValuesRejectFloats(t *testing.T) {
	result := Parse([]byte("struct Value {\n  #metadata(1.5)\n  field: Str,\n}\n"), "test.ard")
	if len(result.Errors) == 0 || result.Errors[0].Message != "Attribute numbers must be integers" {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestAttributeStringInterpolationIsRejected(t *testing.T) {
	result := Parse([]byte("struct Value {\n  #json(name: \"{field}\")\n  field: Str,\n}\n"), "test.ard")
	if len(result.Errors) == 0 || result.Errors[0].Message != "Attribute strings cannot contain interpolation" {
		t.Fatalf("errors = %#v", result.Errors)
	}
}
