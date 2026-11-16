package frontend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/platform"
)

// RepresentativePlatforms returns one platform for each distinct selection of
// `#platform` declarations in the program reachable from inputPath (ADR
// 0082). Checking each one checks every declaration the program can select.
//
// Modules are followed through every `use`, whatever its platform, and
// parsed only for their attributes. Within a group of platforms that select
// the same declarations, host is preferred, then linux/amd64, darwin/arm64,
// and windows/amd64, then the remaining platforms in list order. The result
// is in that order.
func RepresentativePlatforms(inputPath string, host platform.Platform) ([]platform.Platform, error) {
	analyses, err := reachablePlatformAnalyses(inputPath)
	if err != nil {
		return nil, err
	}
	if len(analyses) == 0 {
		return []platform.Platform{host}, nil
	}
	seen := map[string]bool{}
	var representatives []platform.Platform
	for _, candidate := range platform.Preferred(host) {
		var key strings.Builder
		for _, analysis := range analyses {
			key.WriteString(analysis.SelectionKey(candidate))
			key.WriteByte('|')
		}
		if seen[key.String()] {
			continue
		}
		seen[key.String()] = true
		representatives = append(representatives, candidate)
	}
	return representatives, nil
}

// reachablePlatformAnalyses parses every Ard module reachable from inputPath
// through any `use`, and returns the platform analyses of those with
// `#platform` attributes, in visit order.
func reachablePlatformAnalyses(inputPath string) ([]*checker.PlatformAnalysis, error) {
	source, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("error reading file %s - %v", inputPath, err)
	}
	resolver, err := checker.NewModuleResolver(filepath.Dir(inputPath))
	if err != nil {
		return nil, fmt.Errorf("error initializing module resolver: %w", err)
	}
	type entry struct {
		program    *parse.Program
		modulePath string
	}
	queue := []entry{{program: parse.Parse(source, inputPath).Program}}
	visited := map[string]bool{}
	if absolute, err := filepath.Abs(inputPath); err == nil {
		visited[filepath.Clean(absolute)] = true
	}
	var analyses []*checker.PlatformAnalysis
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.program == nil {
			continue
		}
		if analysis := checker.AnalyzePlatforms(current.program); analysis != nil {
			analyses = append(analyses, analysis)
		}
		for _, imp := range current.program.Imports {
			if imp.Kind == parse.ImportKindGo || strings.HasPrefix(imp.Path, "ard/") {
				continue
			}
			// Unresolvable imports are reported when each platform is checked.
			resolved, err := resolver.ResolveImport(current.modulePath, imp.Path)
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
			queue = append(queue, entry{program: program, modulePath: resolved.ModulePath})
		}
	}
	return analyses, nil
}
