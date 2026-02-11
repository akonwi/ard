package ffi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestBodyMaybe(t *testing.T) {
	t.Run("non-empty body becomes some", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/auth/sign-up", strings.NewReader(`{"hello":"world"}`))
		body := requestBodyMaybe(req)
		if body.IsNone() {
			t.Fatal("expected body to be some, got none")
		}
		if got := body.AsString(); got != `{"hello":"world"}` {
			t.Fatalf("expected request body to be preserved, got %q", got)
		}
	})

	t.Run("empty body stays none", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/auth/sign-up", strings.NewReader(""))
		body := requestBodyMaybe(req)
		if !body.IsNone() {
			t.Fatalf("expected empty body to be none, got %q", body.AsString())
		}
	})
}
