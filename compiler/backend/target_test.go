package backend_test

import (
	"testing"

	"github.com/akonwi/ard/backend"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "default empty", raw: "", want: backend.TargetBytecode},
		{name: "bytecode", raw: backend.TargetBytecode, want: backend.TargetBytecode},
		{name: "vm_next", raw: backend.TargetVMNext, want: backend.TargetVMNext},
		{name: "go", raw: backend.TargetGo, want: backend.TargetGo},
		{name: "js-browser", raw: backend.TargetJSBrowser, want: backend.TargetJSBrowser},
		{name: "js-server", raw: backend.TargetJSServer, want: backend.TargetJSServer},
		{name: "unknown", raw: "wasm", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := backend.ParseTarget(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
