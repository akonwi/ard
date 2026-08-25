package air

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestJSONFieldMetadataSurvivesAIRLowering(t *testing.T) {
	program := lowerSource(t, `
		struct User {
			#json(name: "displayName", omit: none)
			display_name: Str?,
			#json(skip: true)
			password_hash: Str,
		}
	`)
	var user TypeInfo
	for _, typ := range program.Types {
		if typ.Kind == TypeStruct && typ.Name == "User" {
			user = typ
			break
		}
	}
	if user.ID == NoType {
		t.Fatal("User type missing from AIR")
	}
	fields := map[string]FieldInfo{}
	for _, field := range user.Fields {
		fields[field.Name] = field
	}
	display := fields["display_name"].JSON
	if !display.HasName || display.Name != "displayName" || !display.OmitNone || display.Skip {
		t.Fatalf("display metadata = %#v", display)
	}
	password := fields["password_hash"].JSON
	if !password.Skip || password.HasName || password.OmitNone {
		t.Fatalf("password metadata = %#v", password)
	}
}

func TestGoFieldTagsSurviveAIRLowering(t *testing.T) {
	program := lowerSource(t, `
		struct Config {
			#go:yaml("global_context,omitempty")
			#go:validate("required")
			global_context: Str,
		}
	`)
	var field FieldInfo
	for _, typ := range program.Types {
		if typ.Kind == TypeStruct && typ.Name == "Config" {
			field = typ.Fields[0]
			break
		}
	}
	if len(field.GoTags) != 2 || field.GoTags[0].Key != "yaml" || field.GoTags[0].Value != "global_context,omitempty" || field.GoTags[1].Key != "validate" || field.GoTags[1].Value != "required" {
		t.Fatalf("Go field tags = %#v", field.GoTags)
	}
}

func TestImportedGenericStructGoFieldTagsComeFromCanonicalDefinition(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "box.ard"), []byte(`
		struct Box<$T> {
			#go:yaml("item")
			value: $T,
		}
	`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	result := parse.Parse([]byte(`
		use app/box
		let value = box::Box<Str>{value: "item"}
	`), mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(mainPath, result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := Lower(c.Module())
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, typ := range program.Types {
		if typ.Kind != TypeStruct || (typ.Name != "Box" && typ.Generic == NoType) {
			continue
		}
		seen++
		tags := typ.Fields[0].GoTags
		if len(tags) != 1 || tags[0].Key != "yaml" || tags[0].Value != "item" {
			t.Fatalf("%s Go field tags = %#v", typ.Name, tags)
		}
	}
	if seen == 0 {
		t.Fatal("imported generic Box metadata was not lowered")
	}
}

func TestGenericStructJSONFieldMetadataComesFromCanonicalDefinition(t *testing.T) {
	program := lowerSource(t, `
		struct Box<$T> {
			#json(name: "item", omit: none)
			#go:yaml("item,omitempty")
			value: $T?,
		}

		let box = Box<Str>{}
	`)
	seen := 0
	for _, typ := range program.Types {
		if typ.Kind != TypeStruct || (typ.Name != "Box" && typ.Generic == NoType) {
			continue
		}
		seen++
		if len(typ.Fields) != 1 {
			t.Fatalf("Box fields = %#v", typ.Fields)
		}
		json := typ.Fields[0].JSON
		if !json.HasName || json.Name != "item" || !json.OmitNone {
			t.Fatalf("%s metadata = %#v", typ.Name, json)
		}
		goTags := typ.Fields[0].GoTags
		if len(goTags) != 1 || goTags[0].Key != "yaml" || goTags[0].Value != "item,omitempty" {
			t.Fatalf("%s Go field tags = %#v", typ.Name, goTags)
		}
	}
	if seen == 0 {
		t.Fatal("generic Box metadata was not checked")
	}
}
