package vm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBytecodeHttpMethod(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "Method implements ToString",
			input: `
				use ard/http
				let method = http::Method::Post
				"{method}"
			`,
			want: "POST",
		},
	})
}

func TestBytecodeHttpSendUsesRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	input := fmt.Sprintf(`
		use ard/http
		use ard/duration
		use ard/maybe

		http::send(http::Request{
			method: http::Method::Get,
			url: %q,
			headers: [:],
			timeout: maybe::some(duration::from_millis(10)),
		}).or(http::Response::new(-1, "")).status
	`, server.URL)

	if got := runBytecode(t, input); got != -1 {
		t.Fatalf("Expected request timeout fallback status -1, got %v", got)
	}
}

func TestBytecodeHttpSendCallSiteTimeoutOverridesRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer server.Close()

	input := fmt.Sprintf(`
		use ard/http
		use ard/duration
		use ard/maybe

		let req = http::Request{
			method: http::Method::Get,
			url: %q,
			headers: [:],
			timeout: maybe::some(duration::from_millis(10)),
		}

		http::send(req, maybe::some(duration::from_millis(100))).or(http::Response::new(-1, "")).status
	`, server.URL)

	if got := runBytecode(t, input); got != http.StatusCreated {
		t.Fatalf("Expected override timeout to succeed with %d, got %v", http.StatusCreated, got)
	}
}
