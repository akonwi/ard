package diagnostics_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/diagnostics"
	"github.com/akonwi/ard/parse"
)

func TestRenderColorsDiagnosticLevels(t *testing.T) {
	tests := []struct {
		name   string
		kind   checker.DiagnosticKind
		header string
		label  string
	}{
		{"error", checker.Error, "\x1b[1;31merror: Problem\x1b[0m", "\x1b[31m^\x1b[0m \x1b[31mhere\x1b[0m"},
		{"warning", checker.Warn, "\x1b[1;33mwarning: Problem\x1b[0m", "\x1b[33m^\x1b[0m \x1b[33mhere\x1b[0m"},
		{"information", checker.DiagnosticKind("information"), "\x1b[1;36minformation: Problem\x1b[0m", "\x1b[36m^\x1b[0m \x1b[36mhere\x1b[0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostic := checker.Diagnostic{
				Kind:  tt.kind,
				Title: "Problem",
				Primary: checker.DiagnosticLabel{
					Span:    checker.SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 1}, End: parse.Point{Row: 1, Col: 1}}},
					Message: "here",
				},
			}
			provider := func(string) ([]byte, error) { return []byte("x\n"), nil }
			var output bytes.Buffer
			if err := diagnostics.RenderDiagnosticWithOptions(&output, diagnostic, provider, diagnostics.RenderOptions{Color: diagnostics.ColorAlways}); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{tt.header, "\x1b[36m --> main.ard:1:1\x1b[0m", "\x1b[2m  |\x1b[0m", tt.label} {
				if !strings.Contains(output.String(), want) {
					t.Fatalf("output missing %q:\n%q", want, output.String())
				}
			}
		})
	}
}

func TestRenderColorDetailsAndNeverMode(t *testing.T) {
	diagnostic := checker.Diagnostic{
		Kind:  checker.Error,
		Title: "Problem",
		Text:  "plain explanation",
		Primary: checker.DiagnosticLabel{
			Span:    checker.SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 1}, End: parse.Point{Row: 1, Col: 1}}},
			Message: "primary",
		},
		Secondary: []checker.DiagnosticLabel{{
			Span:    checker.SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 2}, End: parse.Point{Row: 1, Col: 2}}},
			Message: "related",
		}},
	}
	provider := func(string) ([]byte, error) { return []byte("xy\n"), nil }

	var colored bytes.Buffer
	if err := diagnostics.RenderDiagnosticWithOptions(&colored, diagnostic, provider, diagnostics.RenderOptions{Color: diagnostics.ColorAlways}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"\x1b[2m1 |\x1b[0m xy",
		"\x1b[36m^\x1b[0m \x1b[36mrelated\x1b[0m",
		"\x1b[2m  =\x1b[0m plain explanation\n",
	} {
		if !strings.Contains(colored.String(), want) {
			t.Fatalf("colored output missing %q:\n%q", want, colored.String())
		}
	}

	var plain bytes.Buffer
	if err := diagnostics.RenderDiagnosticWithOptions(&plain, diagnostic, provider, diagnostics.RenderOptions{Color: diagnostics.ColorNever}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatalf("ColorNever output contains ANSI escapes: %q", plain.String())
	}
}

func TestRenderLabeledDiagnostic(t *testing.T) {
	diagnostic := checker.Diagnostic{
		Kind:  checker.Error,
		Title: "Type mismatch",
		Primary: checker.DiagnosticLabel{
			Span: checker.SourceSpan{FilePath: "main.ard", Location: parse.Location{
				Start: parse.Point{Row: 1, Col: 17}, End: parse.Point{Row: 1, Col: 18},
			}},
			Message: "this expression has type `Int`",
		},
		Secondary: []checker.DiagnosticLabel{{
			Span: checker.SourceSpan{FilePath: "main.ard", Location: parse.Location{
				Start: parse.Point{Row: 1, Col: 11}, End: parse.Point{Row: 1, Col: 13},
			}},
			Message: "this annotation requires `Str`",
		}},
	}
	provider := func(string) ([]byte, error) { return []byte("let name: Str = 42\n"), nil }

	var output bytes.Buffer
	if err := diagnostics.RenderDiagnostic(&output, diagnostic, provider); err != nil {
		t.Fatal(err)
	}

	want := "" +
		"error: Type mismatch\n" +
		" --> main.ard:1:17\n" +
		"  |\n" +
		"1 | let name: Str = 42\n" +
		"  |                 ^^ this expression has type `Int`\n" +
		" --> main.ard:1:11\n" +
		"  |\n" +
		"1 | let name: Str = 42\n" +
		"  |           ^^^ this annotation requires `Str`\n"
	if output.String() != want {
		t.Fatalf("output:\n%s\nwant:\n%s", output.String(), want)
	}
}

func TestRenderAlignsMultiDigitLineGutter(t *testing.T) {
	source := "\n\n\n\n\n\n\n\n\n\n  fmt::Println(\"age = {age}\")\n"
	diagnostic := checker.Diagnostic{
		Kind:  checker.Error,
		Title: "Undefined variable",
		Text:  "declare the variable before using it",
		Primary: checker.DiagnosticLabel{
			Span: checker.SourceSpan{FilePath: "variables.ard", Location: parse.Location{
				Start: parse.Point{Row: 11, Col: 24}, End: parse.Point{Row: 11, Col: 26},
			}},
			Message: "`age` is not defined in this scope",
		},
	}
	provider := func(string) ([]byte, error) { return []byte(source), nil }

	var output bytes.Buffer
	if err := diagnostics.RenderDiagnostic(&output, diagnostic, provider); err != nil {
		t.Fatal(err)
	}
	want := "   |\n11 |   fmt::Println(\"age = {age}\")\n   |                        ^^^ `age` is not defined in this scope\n   |\n   = declare the variable before using it\n"
	if !bytes.Contains(output.Bytes(), []byte(want)) {
		t.Fatalf("output missing aligned gutter:\n%s", output.String())
	}
}

func TestRenderRelativeRebasesProjectPathsToWorkingDirectory(t *testing.T) {
	workingDir := t.TempDir()
	projectRoot := filepath.Join(workingDir, "samples")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "variables.ard"), []byte("missing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diagnostic := checker.NewDiagnostic(checker.Error, "Undefined variable: missing", "variables.ard", parse.Location{Start: parse.Point{Row: 1, Col: 1}, End: parse.Point{Row: 1, Col: 7}})

	var output bytes.Buffer
	if err := diagnostics.RenderRelative(&output, []checker.Diagnostic{diagnostic}, projectRoot, workingDir); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), " --> samples/variables.ard:1:1") {
		t.Fatalf("output has wrong display path:\n%s", output.String())
	}
}

func TestRenderUsesTerminalDisplayColumns(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		startByte  int
		wantSpaces int
	}{
		{name: "BMP rune", line: "é x", startByte: len("é "), wantSpaces: 2},
		{name: "astral wide rune", line: "😀 x", startByte: len("😀 "), wantSpaces: 3},
		{name: "tab", line: "\tx", startByte: 1, wantSpaces: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostic := checker.Diagnostic{
				Kind: checker.Error,
				Primary: checker.DiagnosticLabel{
					Span: checker.SourceSpan{FilePath: "main.ard", Location: parse.Location{
						Start: parse.Point{Row: 1, Col: tt.startByte + 1},
						End:   parse.Point{Row: 1, Col: tt.startByte + 1},
					}},
					Message: "here",
				},
			}
			provider := func(string) ([]byte, error) { return []byte(tt.line + "\n"), nil }

			var output bytes.Buffer
			if err := diagnostics.RenderDiagnostic(&output, diagnostic, provider); err != nil {
				t.Fatal(err)
			}
			want := "  | " + string(bytes.Repeat([]byte(" "), tt.wantSpaces)) + "^ here\n"
			if !bytes.Contains(output.Bytes(), []byte(want)) {
				t.Fatalf("output missing %q:\n%s", want, output.String())
			}
		})
	}
}

func TestRenderLoadsCrossFileSecondarySource(t *testing.T) {
	diagnostic := checker.Diagnostic{
		Kind:  checker.Error,
		Title: "Incorrect argument type",
		Primary: checker.DiagnosticLabel{
			Span:    checker.SourceSpan{FilePath: "main.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 7}, End: parse.Point{Row: 1, Col: 8}}},
			Message: "this argument has type `Int`",
		},
		Secondary: []checker.DiagnosticLabel{{
			Span:    checker.SourceSpan{FilePath: "api.ard", Location: parse.Location{Start: parse.Point{Row: 1, Col: 10}, End: parse.Point{Row: 1, Col: 12}}},
			Message: "this parameter requires `Str`",
		}},
	}
	provider := func(path string) ([]byte, error) {
		sources := map[string]string{"main.ard": "greet(42)\n", "api.ard": "fn greet(name: Str) {}\n"}
		return []byte(sources[path]), nil
	}

	var output bytes.Buffer
	if err := diagnostics.RenderDiagnostic(&output, diagnostic, provider); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"main.ard:1:7", "greet(42)", "api.ard:1:10", "fn greet(name: Str)"} {
		if !bytes.Contains(output.Bytes(), []byte(want)) {
			t.Fatalf("output missing %q:\n%s", want, output.String())
		}
	}
}

func TestRenderFallsBackWhenSourceIsUnavailable(t *testing.T) {
	diagnostic := checker.NewDiagnostic(checker.Error, "Undefined variable: name", "main.ard", parse.Location{Start: parse.Point{Row: 3, Col: 5}})
	provider := func(string) ([]byte, error) { return nil, errors.New("missing") }

	var output bytes.Buffer
	if err := diagnostics.RenderDiagnostic(&output, diagnostic, provider); err != nil {
		t.Fatal(err)
	}
	if want := "error: Undefined variable: name\n --> main.ard:3:5 Undefined variable: name\n"; output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}
