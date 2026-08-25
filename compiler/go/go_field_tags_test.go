package gotarget

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/akonwi/ard/air"
)

func TestGoStructFieldTagComposesAndQuotesOpaqueValues(t *testing.T) {
	field := air.FieldInfo{
		Name: "global_context",
		JSON: air.JSONFieldInfo{Name: "globalContext", HasName: true, OmitNone: true},
		GoTags: []air.GoFieldTag{
			{Key: "yaml", Value: "global_context,omitempty"},
			{Key: "validate", Value: "required"},
			{Key: "opaque", Value: "quote\" slash\\ tick` line\n雪"},
		},
	}
	literal := goStructFieldTag(field)
	tag, err := strconv.Unquote(literal.Value)
	if err != nil {
		t.Fatalf("invalid generated Go tag literal %q: %v", literal.Value, err)
	}
	structTag := reflect.StructTag(tag)
	for key, want := range map[string]string{
		"json":     "globalContext,omitzero",
		"yaml":     "global_context,omitempty",
		"validate": "required",
		"opaque":   "quote\" slash\\ tick` line\n雪",
	} {
		got, ok := structTag.Lookup(key)
		if !ok || got != want {
			t.Fatalf("tag %q = %q, found=%v; complete tag %q", key, got, ok, tag)
		}
	}
}

func TestGoFieldTagAttributesLowerToCanonicalGoTags(t *testing.T) {
	program := lowerSource(t, `
		struct Config<$T> {
			#go:yaml("global_context,omitempty")
			#json(name: "globalContext")
			#go:validate("required")
			global_context: $T,

			#json(skip: true)
			#go:yaml("-")
			internal: Str,
		}

		let config = Config<Str>{global_context: "value", internal: "secret"}
	`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveStructFieldTag(files, "Config", "GlobalContext", "`json:\"globalContext\" validate:\"required\" yaml:\"global_context,omitempty\"`") {
		t.Fatal("generated Config.GlobalContext missing canonical composed tags")
	}
	if !astFilesHaveStructFieldTag(files, "Config", "Internal", "`json:\"-\" yaml:\"-\"`") {
		t.Fatal("generated Config.Internal missing composed skip tags")
	}
}
