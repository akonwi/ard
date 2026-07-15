package diagnostics

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/width"

	"github.com/akonwi/ard/checker"
)

type SourceProvider func(path string) ([]byte, error)

func FileSourceProvider(roots ...string) SourceProvider {
	return func(path string) ([]byte, error) {
		if filepath.IsAbs(path) {
			return os.ReadFile(path)
		}
		var lastErr error
		for _, root := range roots {
			source, err := os.ReadFile(filepath.Join(root, path))
			if err == nil {
				return source, nil
			}
			lastErr = err
		}
		source, err := os.ReadFile(path)
		if err == nil {
			return source, nil
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, err
	}
}

func Render(w io.Writer, diagnostics []checker.Diagnostic, source SourceProvider) error {
	for i, diagnostic := range diagnostics {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if err := RenderDiagnostic(w, diagnostic, source); err != nil {
			return err
		}
	}
	return nil
}

// RenderRelative renders diagnostics with source paths rebased from sourceRoot
// to displayRoot. This keeps canonical project-relative source identities out
// of presentation while producing terminal paths resolvable from the caller's
// working directory.
func RenderRelative(w io.Writer, diagnostics []checker.Diagnostic, sourceRoot, displayRoot string) error {
	rebased := make([]checker.Diagnostic, len(diagnostics))
	for i, diagnostic := range diagnostics {
		rebased[i] = diagnostic
		rebased[i].Primary = rebaseLabel(diagnostic.Primary, sourceRoot, displayRoot)
		rebased[i].Secondary = make([]checker.DiagnosticLabel, len(diagnostic.Secondary))
		for j, label := range diagnostic.Secondary {
			rebased[i].Secondary[j] = rebaseLabel(label, sourceRoot, displayRoot)
		}
	}
	return Render(w, rebased, FileSourceProvider(displayRoot))
}

func rebaseLabel(label checker.DiagnosticLabel, sourceRoot, displayRoot string) checker.DiagnosticLabel {
	path := label.Span.FilePath
	if path == "" {
		return label
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(sourceRoot, path)
	}
	if relative, err := filepath.Rel(displayRoot, path); err == nil {
		label.Span.FilePath = relative
	} else {
		label.Span.FilePath = path
	}
	return label
}

func RenderDiagnostic(w io.Writer, diagnostic checker.Diagnostic, source SourceProvider) error {
	title := diagnostic.Title
	if title == "" {
		title = diagnostic.Message
	}
	if _, err := fmt.Fprintf(w, "%s: %s\n", diagnostic.Kind, title); err != nil {
		return err
	}

	span := diagnostic.Primary.Span
	if source == nil || span.FilePath == "" {
		_, err := fmt.Fprintf(w, " --> %s:%d:%d\n", span.FilePath, span.Location.Start.Row, span.Location.Start.Col)
		return err
	}
	primary := diagnostic.Primary
	if primary.Message == "" {
		primary.Message = title
	}
	labels := append([]checker.DiagnosticLabel{primary}, diagnostic.Secondary...)
	gutterWidth := 1
	for _, label := range labels {
		if width := len(strconv.Itoa(label.Span.Location.Start.Row)); width > gutterWidth {
			gutterWidth = width
		}
		if err := renderLabel(w, label, source); err != nil {
			return err
		}
	}

	if diagnostic.Text != "" {
		if _, err := fmt.Fprintf(w, "%*s |\n%*s = %s\n", gutterWidth, "", gutterWidth, "", diagnostic.Text); err != nil {
			return err
		}
	}
	return nil
}

func renderLabel(w io.Writer, label checker.DiagnosticLabel, source SourceProvider) error {
	span := label.Span
	contents, err := source(span.FilePath)
	if err != nil {
		_, writeErr := fmt.Fprintf(w, " --> %s:%d:%d %s\n", span.FilePath, span.Location.Start.Row, span.Location.Start.Col, label.Message)
		return writeErr
	}
	if _, err := fmt.Fprintf(w, " --> %s:%d:%d\n", span.FilePath, span.Location.Start.Row, span.Location.Start.Col); err != nil {
		return err
	}
	row := span.Location.Start.Row
	gutterWidth := len(strconv.Itoa(row))
	if _, err := fmt.Fprintf(w, "%*s |\n", gutterWidth, ""); err != nil {
		return err
	}
	lines := strings.Split(string(contents), "\n")
	if row < 1 || row > len(lines) {
		return nil
	}
	line := lines[row-1]
	if _, err := fmt.Fprintf(w, "%*d | %s\n", gutterWidth, row, line); err != nil {
		return err
	}
	start, underlineWidth := underline(label, line)
	_, err = fmt.Fprintf(w, "%*s | %s%s %s\n", gutterWidth, "", strings.Repeat(" ", start), strings.Repeat("^", underlineWidth), label.Message)
	return err
}

func underline(label checker.DiagnosticLabel, line string) (int, int) {
	startByte := label.Span.Location.Start.Col - 1
	if startByte < 0 {
		startByte = 0
	}
	if startByte > len(line) {
		startByte = len(line)
	}
	endByte := label.Span.Location.End.Col
	if label.Span.Location.End.Row != label.Span.Location.Start.Row {
		endByte = len(line)
	}
	if endByte < startByte {
		endByte = startByte
	}
	if endByte > len(line) {
		endByte = len(line)
	}

	start := displayWidth(line[:startByte], 0)
	underlineWidth := displayWidth(line[startByte:endByte], start)
	if underlineWidth < 1 {
		underlineWidth = 1
	}
	return start, underlineWidth
}

func displayWidth(s string, column int) int {
	start := column
	for _, r := range s {
		switch {
		case r == '\t':
			column += 8 - column%8
		case unicode.Is(unicode.Mn, r), unicode.Is(unicode.Me, r), unicode.Is(unicode.Cf, r):
			// Combining and formatting runes occupy no terminal cell.
		case isWideRune(r):
			column += 2
		default:
			column++
		}
	}
	return column - start
}

func isWideRune(r rune) bool {
	kind := width.LookupRune(r).Kind()
	if kind == width.EastAsianWide || kind == width.EastAsianFullwidth {
		return true
	}
	// Emoji are generally classified as neutral by Unicode's East Asian width
	// data but rendered as two cells by modern terminals.
	return r >= 0x1F000 && r <= 0x1FAFF
}
