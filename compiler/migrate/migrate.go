// Package migrate applies the machine-applicable fixes the checker attaches
// to deprecation diagnostics. It drives `ard migrate`, which rewrites the
// legacy ADR 0057 reference syntax to the ADR 0073 pointer syntax.
package migrate

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// PointerSyntaxFixes returns the fixes attached to deprecated pointer syntax
// diagnostics, grouped by file path.
func PointerSyntaxFixes(diagnostics []checker.Diagnostic) map[string][]checker.TextEdit {
	fixes := map[string][]checker.TextEdit{}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code != checker.DiagnosticCodeDeprecatedPointerSyntax {
			continue
		}
		for _, fix := range diagnostic.Fixes() {
			fixes[fix.FilePath] = append(fixes[fix.FilePath], fix)
		}
	}
	return fixes
}

// ManualPointerSyntax returns the deprecated pointer syntax diagnostics that
// carry no fix and need a manual change.
func ManualPointerSyntax(diagnostics []checker.Diagnostic) []checker.Diagnostic {
	var manual []checker.Diagnostic
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == checker.DiagnosticCodeDeprecatedPointerSyntax && len(diagnostic.Fixes()) == 0 {
			manual = append(manual, diagnostic)
		}
	}
	return manual
}

type byteEdit struct {
	start, end int
	text       string
	order      int
}

// Apply applies edits to source. Identical edits apply once; overlapping
// edits are an error. Insertions at the same offset keep their given order.
func Apply(source []byte, edits []checker.TextEdit) ([]byte, error) {
	lineStarts := lineStartOffsets(source)
	resolved := make([]byteEdit, 0, len(edits))
	seen := map[byteEdit]bool{}
	for i, edit := range edits {
		start, err := offset(source, lineStarts, edit.Start)
		if err != nil {
			return nil, err
		}
		end, err := offset(source, lineStarts, edit.End)
		if err != nil {
			return nil, err
		}
		if end < start {
			return nil, fmt.Errorf("edit at %s ends before it starts", edit.Start)
		}
		if edit.TrimTrailingSpace {
			for end < len(source) && (source[end] == ' ' || source[end] == '\t') {
				end++
			}
		}
		key := byteEdit{start: start, end: end, text: edit.NewText}
		if seen[key] {
			continue
		}
		seen[key] = true
		key.order = i
		resolved = append(resolved, key)
	}
	sort.SliceStable(resolved, func(i, j int) bool {
		if resolved[i].start != resolved[j].start {
			return resolved[i].start < resolved[j].start
		}
		return resolved[i].end < resolved[j].end
	})
	for i := 1; i < len(resolved); i++ {
		if resolved[i].start < resolved[i-1].end {
			return nil, fmt.Errorf("overlapping edits at byte offset %d", resolved[i].start)
		}
	}
	var out bytes.Buffer
	cursor := 0
	for _, edit := range resolved {
		out.Write(source[cursor:edit.start])
		out.WriteString(edit.text)
		cursor = edit.end
	}
	out.Write(source[cursor:])
	return out.Bytes(), nil
}

func lineStartOffsets(source []byte) []int {
	starts := []int{0}
	for i, b := range source {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// offset converts a 1-based row and byte column to a byte offset. The column
// may address the line's content or the position just after it, but never
// spill into the next line.
func offset(source []byte, lineStarts []int, point parse.Point) (int, error) {
	if point.Row < 1 || point.Row > len(lineStarts) || point.Col < 1 {
		return 0, fmt.Errorf("edit position %s is outside the source", point)
	}
	lineStart := lineStarts[point.Row-1]
	lineEnd := len(source)
	if point.Row < len(lineStarts) {
		lineEnd = lineStarts[point.Row] - 1 // the '\n'
	}
	if lineEnd > lineStart && source[lineEnd-1] == '\r' {
		lineEnd--
	}
	at := lineStart + point.Col - 1
	if at > lineEnd {
		return 0, fmt.Errorf("edit position %s is outside its line", point)
	}
	return at, nil
}

// RewriteSource checks a standalone source file and applies its pointer
// syntax fixes. It returns the rewritten source and the deprecations that
// still need a manual change. Programs with parse errors are returned as-is.
func RewriteSource(filePath string, source []byte, options ...checker.CheckOptions) ([]byte, []checker.Diagnostic, error) {
	result := parse.Parse(source, filePath)
	if len(result.Errors) > 0 {
		return source, nil, nil
	}
	c := checker.New(filePath, result.Program, nil, options...)
	c.Check()
	rewritten, err := Apply(source, PointerSyntaxFixes(c.Diagnostics())[filePath])
	if err != nil {
		return nil, nil, err
	}
	return rewritten, ManualPointerSyntax(c.Diagnostics()), nil
}

// FileResult is the outcome of migrating one project file.
type FileResult struct {
	Path        string
	Original    []byte
	Rewritten   []byte
	Manual      []checker.Diagnostic
	HasErrors   bool
	ProjectRoot string
}

// Changed reports whether migration rewrote the file.
func (r FileResult) Changed() bool {
	return !bytes.Equal(r.Original, r.Rewritten)
}

// RewriteFile checks a file in its project context, so imports resolve, and
// applies its pointer syntax fixes. It does not write the result.
func RewriteFile(path string) (FileResult, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return FileResult{}, fmt.Errorf("error reading file %s - %w", path, err)
	}
	result := FileResult{Path: path, Original: source, Rewritten: source}
	parsed := parse.Parse(source, path)
	if len(parsed.Errors) > 0 {
		result.HasErrors = true
		return result, nil
	}
	resolver, err := checker.NewModuleResolverWithOptions(filepath.Dir(path), checker.BuildOptions{})
	if err != nil {
		return FileResult{}, fmt.Errorf("error initializing module resolver: %w", err)
	}
	project := resolver.GetProjectInfo()
	result.ProjectRoot = project.RootPath
	relPath := path
	if absPath, absErr := filepath.Abs(path); absErr == nil {
		if projectRelative, relErr := filepath.Rel(project.RootPath, absPath); relErr == nil {
			relPath = projectRelative
		}
	}
	goResolver := checker.NewGoPackagesResolver(project.RootPath, project.Go.BuildTags)
	goResolver.DependencyModuleRoots = checker.DependencyGoModuleRoots(project)
	c := checker.New(relPath, parsed.Program, resolver, checker.CheckOptions{GoResolver: goResolver})
	c.Check()
	result.HasErrors = c.HasErrors()
	rewritten, err := Apply(source, PointerSyntaxFixes(c.Diagnostics())[relPath])
	if err != nil {
		return FileResult{}, fmt.Errorf("error migrating %s - %w", path, err)
	}
	result.Rewritten = rewritten
	for _, diagnostic := range ManualPointerSyntax(c.Diagnostics()) {
		if diagnostic.FilePath() == relPath {
			result.Manual = append(result.Manual, diagnostic)
		}
	}
	return result, nil
}
