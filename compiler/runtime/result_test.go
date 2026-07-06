package runtime

import "testing"

func TestResultMarshalJSONUnwrapsValue(t *testing.T) {
	ok := Ok[int, string](200)
	if got, _ := ok.MarshalJSON(); string(got) != "200" {
		t.Fatalf("ok marshal = %q, want 200", got)
	}
	err := Err[int, bool](false)
	if got, _ := err.MarshalJSON(); string(got) != "false" {
		t.Fatalf("err marshal = %q, want false", got)
	}
}
