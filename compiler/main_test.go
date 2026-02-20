package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFormatArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		path       string
		checkOnly  bool
		expectErr  bool
		errMessage string
	}{
		{
			name:      "single file path",
			args:      []string{"samples/hello.ard"},
			path:      "samples/hello.ard",
			checkOnly: false,
		},
		{
			name:      "check mode",
			args:      []string{"--check", "samples/hello.ard"},
			path:      "samples/hello.ard",
			checkOnly: true,
		},
		{
			name:       "unknown flag",
			args:       []string{"--watch", "samples/hello.ard"},
			expectErr:  true,
			errMessage: "unknown flag: --watch",
		},
		{
			name:       "missing filepath",
			args:       []string{"--check"},
			expectErr:  true,
			errMessage: "expected filepath argument",
		},
		{
			name:       "unexpected extra argument",
			args:       []string{"a.ard", "b.ard"},
			expectErr:  true,
			errMessage: "unexpected argument: b.ard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, checkOnly, err := parseFormatArgs(tt.args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.errMessage)
				}
				if err.Error() != tt.errMessage {
					t.Fatalf("expected error %q, got %q", tt.errMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if path != tt.path {
				t.Fatalf("expected path %q, got %q", tt.path, path)
			}
			if checkOnly != tt.checkOnly {
				t.Fatalf("expected checkOnly %t, got %t", tt.checkOnly, checkOnly)
			}
		})
	}
}

func TestFormatFile(t *testing.T) {
	t.Run("writes formatted source", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "example.ard")
		if err := os.WriteFile(path, []byte("let x = 1  \n"), 0o644); err != nil {
			t.Fatalf("failed to seed test file: %v", err)
		}

		changed, err := formatFile(path, false)
		if err != nil {
			t.Fatalf("did not expect error: %v", err)
		}
		if !changed {
			t.Fatalf("expected file to change")
		}

		out, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read formatted file: %v", err)
		}
		if string(out) != "let x = 1\n" {
			t.Fatalf("expected formatted content, got %q", string(out))
		}
	})

	t.Run("check mode does not write file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "example.ard")
		original := "let x = 1  \n"
		if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
			t.Fatalf("failed to seed test file: %v", err)
		}

		changed, err := formatFile(path, true)
		if err != nil {
			t.Fatalf("did not expect error: %v", err)
		}
		if !changed {
			t.Fatalf("expected check mode to report changes")
		}

		out, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read source file: %v", err)
		}
		if string(out) != original {
			t.Fatalf("expected file to stay unchanged, got %q", string(out))
		}
	})
}

func TestFormatPath(t *testing.T) {
	t.Run("formats directories recursively", func(t *testing.T) {
		dir := t.TempDir()
		nestedDir := filepath.Join(dir, "nested")
		if err := os.MkdirAll(nestedDir, 0o755); err != nil {
			t.Fatalf("failed to create nested dir: %v", err)
		}

		first := filepath.Join(dir, "first.ard")
		second := filepath.Join(nestedDir, "second.ard")
		if err := os.WriteFile(first, []byte("let x = 1  \n"), 0o644); err != nil {
			t.Fatalf("failed to seed first file: %v", err)
		}
		if err := os.WriteFile(second, []byte("let y = 2\n"), 0o644); err != nil {
			t.Fatalf("failed to seed second file: %v", err)
		}

		changedPaths, err := formatPath(dir, false)
		if err != nil {
			t.Fatalf("did not expect error: %v", err)
		}
		if len(changedPaths) != 1 {
			t.Fatalf("expected one changed path, got %d", len(changedPaths))
		}
		if changedPaths[0] != first && changedPaths[0] != second {
			t.Fatalf("unexpected changed path %q", changedPaths[0])
		}
	})
}
