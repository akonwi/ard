package checker

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/akonwi/ard/parse"
)

// GoImportScanEntry pairs a parsed program with its canonical module path
// for CollectGoImportPaths.
type GoImportScanEntry struct {
	Program    *parse.Program
	ModulePath string
}

// CollectGoImportPaths walks the Ard import graph reachable from the given
// entry programs and returns every `use go:` path, deduplicated and sorted.
// It parses reachable modules only for their import statements — no type
// checking — so a Go package resolver can be primed with the program's whole
// Go import set before any checking begins (ADR 0044).
//
// The scan is best-effort on invalid programs: imports that fail to resolve
// or parse are skipped here and reported properly by the checker at the
// importing `use` statement. For valid programs the scan is complete because
// `use go:` is the only mechanism that introduces a Go package path.
func CollectGoImportPaths(resolver *ModuleResolver, entries ...GoImportScanEntry) []string {
	goPaths := map[string]bool{}
	visited := map[string]bool{}
	queue := entries
	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]
		if entry.Program == nil {
			continue
		}
		for _, imp := range entry.Program.Imports {
			if imp.Kind == parse.ImportKindGo {
				goPaths[imp.Path] = true
				continue
			}
			if strings.HasPrefix(imp.Path, "ard/") || resolver == nil {
				continue
			}
			resolved, err := resolver.ResolveImport(entry.ModulePath, imp.Path)
			if err != nil {
				continue
			}
			filePath := filepath.Clean(resolved.FilePath)
			if visited[filePath] {
				continue
			}
			visited[filePath] = true
			program, err := resolver.LoadModuleFile(filePath)
			if err != nil {
				continue
			}
			queue = append(queue, GoImportScanEntry{Program: program, ModulePath: resolved.ModulePath})
		}
	}
	paths := make([]string, 0, len(goPaths))
	for path := range goPaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
