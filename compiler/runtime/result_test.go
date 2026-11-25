package runtime

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"strings"
	"testing"
)

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

func TestResultJSONEncodeHonorsCallerEncoderOptions(t *testing.T) {
	for _, result := range []Result[string, string]{Ok[string, string]("<ok>"), Err[string, string]("<err>")} {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(result); err != nil {
			t.Fatalf("Encode error = %v", err)
		}
		want := `"<ok>"`
		if !result.Ok {
			want = `"<err>"`
		}
		if got := strings.TrimSpace(buf.String()); got != want {
			t.Fatalf("Encode = %s, want %s (SetEscapeHTML(false) ignored)", got, want)
		}
	}
}

// TestResultJSONMethodSetsAgree keeps MarshalJSON and MarshalJSONTo
// equivalent under default options.
func TestResultJSONMethodSetsAgree(t *testing.T) {
	for _, result := range []Result[[]int, string]{Ok[[]int, string]([]int{1}), Ok[[]int, string](nil), Err[[]int, string]("bad")} {
		v1, err := result.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON error = %v", err)
		}
		v2, err := jsonv2.Marshal(result, json.DefaultOptionsV1())
		if err != nil {
			t.Fatalf("v2 Marshal error = %v", err)
		}
		if string(v1) != string(v2) {
			t.Fatalf("MarshalJSON = %s, MarshalJSONTo = %s", v1, v2)
		}
	}
}
