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

func TestRenderUsesParserProducedStringSpans(t *testing.T) {
	tests := []struct {
		name, literal, carets string
	}{
		{name: "ordinary", literal: `"abc"`, carets: "^^^^^"},
		{name: "escaped", literal: `"a\n"`, carets: "^^^^^"},
		{name: "unicode", literal: `"é"`, carets: "^^^"},
		{name: "interpolated", literal: `"value = {1}"`, carets: "^^^^^^^^^^^^^"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := "let value: Int = " + tt.literal + " // sentinel\n"
			parsed := parse.Parse([]byte(source), "main.ard")
			if len(parsed.Errors) != 0 {
				t.Fatalf("parse errors: %#v", parsed.Errors)
			}
			c := checker.New("main.ard", parsed.Program, nil)
			c.Check()
			if len(c.Diagnostics()) != 1 {
				t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
			}

			provider := func(string) ([]byte, error) { return []byte(source), nil }
			var output bytes.Buffer
			if err := diagnostics.RenderDiagnosticWithOptions(&output, c.Diagnostics()[0], provider, diagnostics.RenderOptions{Color: diagnostics.ColorNever}); err != nil {
				t.Fatal(err)
			}
			want := "" +
				"error: Type mismatch\n" +
				" --> main.ard:1:18\n" +
				"  |\n" +
				"1 | " + strings.TrimSuffix(source, "\n") + "\n" +
				"  |                  " + tt.carets + " this expression has type `Str`\n" +
				" --> main.ard:1:12\n" +
				"  |\n" +
				"1 | " + strings.TrimSuffix(source, "\n") + "\n" +
				"  |            ^^^ this annotation requires `Int`\n"
			if output.String() != want {
				t.Fatalf("output:\n%s\nwant:\n%s", output.String(), want)
			}
		})
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

func TestRenderSummarizesDependencyWarnings(t *testing.T) {
	at := func(file string) checker.DiagnosticLabel {
		return checker.DiagnosticLabel{Span: checker.SourceSpan{FilePath: file, Location: parse.Location{Start: parse.Point{Row: 1, Col: 1}, End: parse.Point{Row: 1, Col: 1}}}}
	}
	diags := []checker.Diagnostic{
		{Kind: checker.Warn, Title: "root warning", Primary: at("main.ard")},
		{Kind: checker.Warn, Title: "dram warning one", Primary: at("dram.ard"), Dependency: "dram"},
		{Kind: checker.Warn, Title: "sql warning", Primary: at("sql.ard"), Dependency: "sql"},
		{Kind: checker.Error, Title: "sql error", Primary: at("sql.ard"), Dependency: "sql"},
		{Kind: checker.Warn, Title: "dram warning two", Primary: at("dram.ard"), Dependency: "dram"},
	}
	provider := func(string) ([]byte, error) { return []byte("x\n"), nil }

	var quiet bytes.Buffer
	if err := diagnostics.RenderWithOptions(&quiet, diags, provider, diagnostics.RenderOptions{Color: diagnostics.ColorNever}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"warning: root warning", "error: sql error", "warning: 3 warnings from dependencies not shown: dram (2), sql (1)\n"} {
		if !strings.Contains(quiet.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, quiet.String())
		}
	}
	for _, hidden := range []string{"dram warning", "sql warning"} {
		if strings.Contains(quiet.String(), hidden) {
			t.Fatalf("output should hide %q:\n%s", hidden, quiet.String())
		}
	}
	if !strings.HasSuffix(quiet.String(), "dram (2), sql (1)\n") {
		t.Fatalf("summary should be last:\n%s", quiet.String())
	}

	var single bytes.Buffer
	if err := diagnostics.RenderWithOptions(&single, diags[1:2], provider, diagnostics.RenderOptions{Color: diagnostics.ColorNever}); err != nil {
		t.Fatal(err)
	}
	if single.String() != "warning: 1 warning from dependencies not shown: dram (1)\n" {
		t.Fatalf("single summary = %q", single.String())
	}

	var verbose bytes.Buffer
	if err := diagnostics.RenderWithOptions(&verbose, diags, provider, diagnostics.RenderOptions{Color: diagnostics.ColorNever, ShowDependencyWarnings: true}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"dram warning one", "dram warning two", "sql warning"} {
		if !strings.Contains(verbose.String(), want) {
			t.Fatalf("verbose output missing %q:\n%s", want, verbose.String())
		}
	}
	if strings.Contains(verbose.String(), "not shown") {
		t.Fatalf("verbose output should not summarize:\n%s", verbose.String())
	}
}

func TestRenderSummarizesDeprecatedPointerSyntax(t *testing.T) {
	at := func(file string) checker.DiagnosticLabel {
		return checker.DiagnosticLabel{Span: checker.SourceSpan{FilePath: file, Location: parse.Location{Start: parse.Point{Row: 1, Col: 1}, End: parse.Point{Row: 1, Col: 1}}}}
	}
	deprecated := func(file string) checker.Diagnostic {
		return checker.Diagnostic{Kind: checker.Warn, Code: checker.DiagnosticCodeDeprecatedPointerSyntax, Title: "deprecated in " + file, Primary: at(file)}
	}
	diags := []checker.Diagnostic{
		deprecated("a.ard"),
		{Kind: checker.Error, Title: "real error", Primary: at("a.ard")},
		deprecated("a.ard"),
		deprecated("b.ard"),
		{Kind: checker.Warn, Code: checker.DiagnosticCodeDeprecatedPointerSyntax, Title: "dependency deprecated", Primary: at("dep.ard"), Dependency: "dram"},
	}
	provider := func(string) ([]byte, error) { return []byte("x\n"), nil }

	var quiet bytes.Buffer
	if err := diagnostics.RenderWithOptions(&quiet, diags, provider, diagnostics.RenderOptions{Color: diagnostics.ColorNever}); err != nil {
		t.Fatal(err)
	}
	want := "warning: 3 deprecated pointer uses in 2 files not shown; run `ard migrate .` to update them\n\n" +
		"warning: 1 warning from dependencies not shown: dram (1)\n"
	if !strings.Contains(quiet.String(), "error: real error") || !strings.HasSuffix(quiet.String(), want) {
		t.Fatalf("output should keep errors and end with summaries %q:\n%s", want, quiet.String())
	}
	if strings.Contains(quiet.String(), "deprecated in") {
		t.Fatalf("deprecated uses should be collapsed:\n%s", quiet.String())
	}

	var single bytes.Buffer
	if err := diagnostics.RenderWithOptions(&single, diags[3:4], provider, diagnostics.RenderOptions{Color: diagnostics.ColorNever, MigratePath: "app"}); err != nil {
		t.Fatal(err)
	}
	if single.String() != "warning: 1 deprecated pointer use in 1 file not shown; run `ard migrate app` to update it\n" {
		t.Fatalf("single summary = %q", single.String())
	}

	var verbose bytes.Buffer
	if err := diagnostics.RenderWithOptions(&verbose, diags, provider, diagnostics.RenderOptions{Color: diagnostics.ColorNever, ShowDeprecationWarnings: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(verbose.String(), "warning: deprecated in") != 3 || strings.Contains(verbose.String(), "deprecated pointer use") {
		t.Fatalf("verbose output should show each deprecated use:\n%s", verbose.String())
	}
}

func TestRenderRelativeMigrateHintPointsAtProjectRoot(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Dir(root)
	diag := checker.Diagnostic{Kind: checker.Warn, Code: checker.DiagnosticCodeDeprecatedPointerSyntax, Title: "deprecated", Primary: checker.DiagnosticLabel{Span: checker.SourceSpan{FilePath: "main.ard"}}}
	var output bytes.Buffer
	if err := diagnostics.RenderRelativeWithOptions(&output, []checker.Diagnostic{diag}, root, cwd, diagnostics.RenderOptions{Color: diagnostics.ColorNever}); err != nil {
		t.Fatal(err)
	}
	if want := "run `ard migrate " + filepath.Base(root) + "`"; !strings.Contains(output.String(), want) {
		t.Fatalf("output missing %q:\n%s", want, output.String())
	}
}

func TestRenderRelativeMigrateHintPrefersShorterPath(t *testing.T) {
	root := t.TempDir()
	diag := checker.Diagnostic{Kind: checker.Warn, Code: checker.DiagnosticCodeDeprecatedPointerSyntax, Title: "deprecated", Primary: checker.DiagnosticLabel{Span: checker.SourceSpan{FilePath: "main.ard"}}}
	for _, tt := range []struct{ name, cwd, want string }{
		{"inside project", root, "run `ard migrate .`"},
		{"nearby directory", filepath.Join(root, "src", "lib"), "run `ard migrate ../..`"},
		{"distant directory", filepath.Join(append([]string{root}, strings.Split(strings.Repeat("deep/", 60), "/")...)...), "run `ard migrate " + root + "`"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := diagnostics.RenderRelativeWithOptions(&output, []checker.Diagnostic{diag}, root, tt.cwd, diagnostics.RenderOptions{Color: diagnostics.ColorNever}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), tt.want) {
				t.Fatalf("output missing %q:\n%s", tt.want, output.String())
			}
		})
	}
}
