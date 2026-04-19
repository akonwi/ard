package checker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestStdlibImportTargetValidation(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		source      string
		wantErrPart string
	}{
		{
			name:   "env allowed on js-server",
			target: backend.TargetJSServer,
			source: "use ard/env\nfn main() Int { 1 }",
		},
		{
			name:   "argv allowed on js-server",
			target: backend.TargetJSServer,
			source: "use ard/argv\nfn main() Int { 1 }",
		},
		{
			name:        "env blocked on js-browser",
			target:      backend.TargetJSBrowser,
			source:      "use ard/env\nfn main() Int { 1 }",
			wantErrPart: "Cannot import ard/env when targeting js-browser; allowed targets: bytecode, go, js-server",
		},
		{
			name:        "fs blocked on js-server",
			target:      backend.TargetJSServer,
			source:      "use ard/fs\nfn main() Int { 1 }",
			wantErrPart: "Cannot import ard/fs when targeting js-server; allowed targets: bytecode, go",
		},
		{
			name:        "sql blocked on js-browser",
			target:      backend.TargetJSBrowser,
			source:      "use ard/sql\nfn main() Int { 1 }",
			wantErrPart: "Cannot import ard/sql when targeting js-browser; allowed targets: bytecode, go",
		},
		{
			name:   "unrestricted module still allowed",
			target: backend.TargetJSBrowser,
			source: "use ard/io\nfn main() Int { 1 }",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("unexpected parse error: %s", result.Errors[0].Message)
			}

			c := checker.New("main.ard", result.Program, nil, checker.CheckOptions{Target: tt.target})
			c.Check()

			if tt.wantErrPart == "" {
				if c.HasErrors() {
					t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
				}
				return
			}

			if !c.HasErrors() {
				t.Fatalf("expected diagnostics containing %q", tt.wantErrPart)
			}

			messages := make([]string, 0, len(c.Diagnostics()))
			for _, diagnostic := range c.Diagnostics() {
				messages = append(messages, diagnostic.String())
			}
			joined := strings.Join(messages, "\n")
			if !strings.Contains(joined, tt.wantErrPart) {
				t.Fatalf("expected diagnostics to contain %q, got:\n%s", tt.wantErrPart, joined)
			}
		})
	}
}

func TestStdlibImportValidationUsesProjectTargetByDefault(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\ntarget = \"js-browser\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	result := parse.Parse([]byte("use ard/env\nfn main() Int { 1 }"), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected parse error: %s", result.Errors[0].Message)
	}

	c := checker.New("main.ard", result.Program, resolver)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected diagnostics for js-browser project target")
	}

	joined := ""
	for _, diagnostic := range c.Diagnostics() {
		joined += diagnostic.String() + "\n"
	}
	if !strings.Contains(joined, "Cannot import ard/env when targeting js-browser") {
		t.Fatalf("unexpected diagnostics:\n%s", joined)
	}
}

func TestStdlibImportValidationIsTransitiveForUserModules(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "db.ard"), []byte("use ard/sql\nfn load() Int { 1 }"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	source := "use demo/db\nfn main() Int { db::load() }"
	result := parse.Parse([]byte(source), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected parse error: %s", result.Errors[0].Message)
	}

	c := checker.New("main.ard", result.Program, resolver, checker.CheckOptions{Target: backend.TargetJSBrowser})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected transitive target validation error")
	}

	joined := ""
	for _, diagnostic := range c.Diagnostics() {
		joined += diagnostic.String() + "\n"
	}
	if !strings.Contains(joined, "Cannot import ard/sql when targeting js-browser") {
		t.Fatalf("unexpected diagnostics:\n%s", joined)
	}
}
