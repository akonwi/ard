package ardgo

import "testing"

type externDecodeError struct {
	Expected string
	Found    string
	Path     []string
}

type localDecodeError struct {
	Expected string
	Found    string
	Path     []string
}

func TestCoerceExternStruct(t *testing.T) {
	got := CoerceExtern[localDecodeError](externDecodeError{
		Expected: "Str",
		Found:    "Int",
		Path:     []string{"root"},
	})

	if got.Expected != "Str" || got.Found != "Int" {
		t.Fatalf("unexpected struct coercion result: %+v", got)
	}
	if len(got.Path) != 1 || got.Path[0] != "root" {
		t.Fatalf("unexpected path coercion result: %+v", got)
	}
}

func TestCoerceExternResultWithStructError(t *testing.T) {
	source := Err[string, externDecodeError](externDecodeError{
		Expected: "Str",
		Found:    "Bool",
		Path:     []string{"field"},
	})
	got := CoerceExtern[Result[string, localDecodeError]](source)

	if !got.IsErr() {
		t.Fatalf("expected coerced result to be err")
	}
	err := got.UnwrapErr()
	if err.Expected != "Str" || err.Found != "Bool" {
		t.Fatalf("unexpected coerced err: %+v", err)
	}
	if len(err.Path) != 1 || err.Path[0] != "field" {
		t.Fatalf("unexpected coerced path: %+v", err)
	}
}

func TestCoerceExternMapToDynamicMap(t *testing.T) {
	source := map[string]any{"count": 3, "ok": true}
	got := CoerceExtern[map[any]any](source)

	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got["count"] != 3 {
		t.Fatalf("expected count entry to equal 3, got %v", got["count"])
	}
	if got["ok"] != true {
		t.Fatalf("expected ok entry to equal true, got %v", got["ok"])
	}
}

func TestCoerceExternNilToDynamic(t *testing.T) {
	got := CoerceExtern[any](nil)
	if got != nil {
		t.Fatalf("expected nil dynamic coercion, got %v", got)
	}
}
