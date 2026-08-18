package air

import "testing"

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

func TestGenericStructJSONFieldMetadataComesFromCanonicalDefinition(t *testing.T) {
	program := lowerSource(t, `
		struct Box<$T> {
			#json(name: "item", omit: none)
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
	}
	if seen == 0 {
		t.Fatal("generic Box metadata was not checked")
	}
}
