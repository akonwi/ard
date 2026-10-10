package diagnostics

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/width"

	"github.com/akonwi/ard/checker"
)

type SourceProvider func(path string) ([]byte, error)

type ColorMode uint8

const (
	ColorAuto ColorMode = iota
	ColorNever
	ColorAlways
)

type RenderOptions struct {
	Color ColorMode
	// ShowDependencyWarnings renders warnings from dependency packages in
	// full. By default they are replaced by one summary line, since the user
	// cannot fix them until the dependency publishes a new version.
	ShowDependencyWarnings bool
}

const (
	ansiReset      = "\x1b[0m"
	ansiBoldRed    = "\x1b[1;31m"
	ansiBoldYellow = "\x1b[1;33m"
	ansiBoldCyan   = "\x1b[1;36m"
	ansiRed        = "\x1b[31m"
	ansiYellow     = "\x1b[33m"
	ansiCyan       = "\x1b[36m"
	ansiDim        = "\x1b[2m"
)

type renderStyle struct {
	enabled           bool
	header, primary   string
	secondary, gutter string
}

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
	return RenderWithOptions(w, diagnostics, source, RenderOptions{Color: ColorAuto})
}

func RenderWithOptions(w io.Writer, diagnostics []checker.Diagnostic, source SourceProvider, options RenderOptions) error {
	color := colorEnabled(w, options.Color)
	shown := diagnostics
	var hidden []checker.Diagnostic
	if !options.ShowDependencyWarnings {
		shown, hidden = splitDependencyWarnings(diagnostics)
	}
	for i, diagnostic := range shown {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if err := renderDiagnostic(w, diagnostic, source, diagnosticStyle(diagnostic.Kind, color)); err != nil {
			return err
		}
	}
	if len(hidden) == 0 {
		return nil
	}
	if len(shown) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	style := diagnosticStyle(checker.Warn, color)
	_, err := fmt.Fprintf(w, "%swarning: %s%s\n", style.header, dependencyWarningSummary(hidden), style.reset())
	return err
}

func splitDependencyWarnings(diagnostics []checker.Diagnostic) (shown, hidden []checker.Diagnostic) {
	for _, diagnostic := range diagnostics {
		if diagnostic.Dependency != "" && diagnostic.Kind == checker.Warn {
			hidden = append(hidden, diagnostic)
		} else {
			shown = append(shown, diagnostic)
		}
	}
	return shown, hidden
}

// dependencyWarningSummary describes hidden dependency warnings, listing
// packages from most to fewest warnings.
func dependencyWarningSummary(hidden []checker.Diagnostic) string {
	counts := map[string]int{}
	var names []string
	for _, diagnostic := range hidden {
		if counts[diagnostic.Dependency] == 0 {
			names = append(names, diagnostic.Dependency)
		}
		counts[diagnostic.Dependency]++
	}
	sort.SliceStable(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = fmt.Sprintf("%s (%d)", name, counts[name])
	}
	noun := "warnings"
	if len(hidden) == 1 {
		noun = "warning"
	}
	return fmt.Sprintf("%d %s from dependencies not shown: %s", len(hidden), noun, strings.Join(parts, ", "))
}

// RenderRelative renders diagnostics with source paths rebased from sourceRoot
// to displayRoot. This keeps canonical project-relative source identities out
// of presentation while producing terminal paths resolvable from the caller's
// working directory.
func RenderRelative(w io.Writer, diagnostics []checker.Diagnostic, sourceRoot, displayRoot string) error {
	return RenderRelativeWithOptions(w, diagnostics, sourceRoot, displayRoot, RenderOptions{Color: ColorAuto})
}

func RenderRelativeWithOptions(w io.Writer, diagnostics []checker.Diagnostic, sourceRoot, displayRoot string, options RenderOptions) error {
	rebased := make([]checker.Diagnostic, len(diagnostics))
	for i, diagnostic := range diagnostics {
		rebased[i] = diagnostic
		rebased[i].Primary = rebaseLabel(diagnostic.Primary, sourceRoot, displayRoot)
		rebased[i].Secondary = make([]checker.DiagnosticLabel, len(diagnostic.Secondary))
		for j, label := range diagnostic.Secondary {
			rebased[i].Secondary[j] = rebaseLabel(label, sourceRoot, displayRoot)
		}
	}
	return RenderWithOptions(w, rebased, FileSourceProvider(displayRoot), options)
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
	return RenderDiagnosticWithOptions(w, diagnostic, source, RenderOptions{Color: ColorAuto})
}

func RenderDiagnosticWithOptions(w io.Writer, diagnostic checker.Diagnostic, source SourceProvider, options RenderOptions) error {
	return renderDiagnostic(w, diagnostic, source, diagnosticStyle(diagnostic.Kind, colorEnabled(w, options.Color)))
}

func renderDiagnostic(w io.Writer, diagnostic checker.Diagnostic, source SourceProvider, style renderStyle) error {
	title := diagnostic.Title
	if title == "" {
		title = diagnostic.Message
	}
	if _, err := fmt.Fprintf(w, "%s%s: %s%s\n", style.header, diagnosticLevelLabel(diagnostic.Kind), title, style.reset()); err != nil {
		return err
	}

	span := diagnostic.Primary.Span
	if source == nil || span.FilePath == "" {
		_, err := fmt.Fprintf(w, "%s --> %s:%d:%d%s\n", style.secondary, span.FilePath, span.Location.Start.Row, span.Location.Start.Col, style.reset())
		return err
	}
	primary := diagnostic.Primary
	if primary.Message == "" {
		primary.Message = title
	}
	labels := append([]checker.DiagnosticLabel{primary}, diagnostic.Secondary...)
	gutterWidth := 1
	for i, label := range labels {
		if width := len(strconv.Itoa(label.Span.Location.Start.Row)); width > gutterWidth {
			gutterWidth = width
		}
		labelColor := style.secondary
		if i == 0 {
			labelColor = style.primary
		}
		if err := renderLabel(w, label, source, style, labelColor); err != nil {
			return err
		}
	}

	if diagnostic.Text != "" {
		if _, err := fmt.Fprintf(w, "%s%*s |%s\n%s%*s =%s %s\n", style.gutter, gutterWidth, "", style.reset(), style.gutter, gutterWidth, "", style.reset(), diagnostic.Text); err != nil {
			return err
		}
	}
	return nil
}

func renderLabel(w io.Writer, label checker.DiagnosticLabel, source SourceProvider, style renderStyle, labelColor string) error {
	span := label.Span
	contents, err := source(span.FilePath)
	if err != nil {
		_, writeErr := fmt.Fprintf(w, "%s --> %s:%d:%d%s %s%s%s\n", style.secondary, span.FilePath, span.Location.Start.Row, span.Location.Start.Col, style.reset(), labelColor, label.Message, style.reset())
		return writeErr
	}
	if _, err := fmt.Fprintf(w, "%s --> %s:%d:%d%s\n", style.secondary, span.FilePath, span.Location.Start.Row, span.Location.Start.Col, style.reset()); err != nil {
		return err
	}
	row := span.Location.Start.Row
	gutterWidth := len(strconv.Itoa(row))
	if _, err := fmt.Fprintf(w, "%s%*s |%s\n", style.gutter, gutterWidth, "", style.reset()); err != nil {
		return err
	}
	lines := strings.Split(string(contents), "\n")
	if row < 1 || row > len(lines) {
		return nil
	}
	line := lines[row-1]
	if _, err := fmt.Fprintf(w, "%s%*d |%s %s\n", style.gutter, gutterWidth, row, style.reset(), line); err != nil {
		return err
	}
	start, underlineWidth := underline(label, line)
	_, err = fmt.Fprintf(w, "%s%*s |%s %s%s%s%s %s%s%s\n", style.gutter, gutterWidth, "", style.reset(), strings.Repeat(" ", start), labelColor, strings.Repeat("^", underlineWidth), style.reset(), labelColor, label.Message, style.reset())
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

func colorEnabled(w io.Writer, mode ColorMode) bool {
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	}
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func diagnosticLevelLabel(kind checker.DiagnosticKind) string {
	if kind == checker.Warn {
		return "warning"
	}
	return string(kind)
}

func diagnosticStyle(kind checker.DiagnosticKind, enabled bool) renderStyle {
	if !enabled {
		return renderStyle{}
	}
	style := renderStyle{enabled: true, secondary: ansiCyan, gutter: ansiDim}
	switch kind {
	case checker.Error:
		style.header, style.primary = ansiBoldRed, ansiRed
	case checker.Warn:
		style.header, style.primary = ansiBoldYellow, ansiYellow
	default:
		style.header, style.primary = ansiBoldCyan, ansiCyan
	}
	return style
}

func (s renderStyle) reset() string {
	if s.enabled {
		return ansiReset
	}
	return ""
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
