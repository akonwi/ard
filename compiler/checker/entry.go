package checker

import (
	"path/filepath"

	"github.com/akonwi/ard/parse"
)

// EntryResult is the outcome of CheckEntry.
type EntryResult struct {
	Module      Module
	Diagnostics []Diagnostic
	// Reused reports that an earlier import already checked this file. Its
	// diagnostics were reported by that importer, so Diagnostics is empty.
	Reused bool
}

func (r EntryResult) HasErrors() bool {
	for _, diagnostic := range r.Diagnostics {
		if diagnostic.Kind == Error {
			return true
		}
	}
	return false
}

// CheckEntry checks input as an entry module while sharing the resolver's
// import cache, so a project file that is both an entry (such as an `ard
// test` file) and another entry's import is checked once per resolver.
//
// sourcePath locates the file on disk; filePath is the path recorded in
// diagnostics (see New). The cache is shared only when importing
// options.ModulePath resolves to the same file. A shared entry adopts the
// canonical module path that import resolves to (a package's root module
// `app/app` is imported as `app`), so the entry and its imports are one
// module. Otherwise the entry is checked as with New. An entry with errors is
// not cached, matching how imports with errors are handled.
func CheckEntry(sourcePath, filePath string, input *parse.Program, resolver *ModuleResolver, options CheckOptions) EntryResult {
	cacheKey, canonicalPath, shared := entryCacheKey(sourcePath, resolver, options.ModulePath)
	if shared {
		if cached, ok := resolver.moduleCache[cacheKey]; ok {
			return EntryResult{Module: cached, Reused: true}
		}
		options.ModulePath = canonicalPath
	}
	c := New(filePath, input, resolver, options)
	c.Check()
	result := EntryResult{Module: c.Module(), Diagnostics: c.Diagnostics()}
	if shared && !result.HasErrors() {
		resolver.moduleCache[cacheKey] = result.Module
	}
	return result
}

// entryCacheKey returns the import cache key and canonical module path for an
// entry module, and whether importing modulePath resolves to the entry's file.
func entryCacheKey(sourcePath string, resolver *ModuleResolver, modulePath string) (string, string, bool) {
	if resolver == nil || resolver.project == nil || resolver.moduleCache == nil || modulePath == "" {
		return "", "", false
	}
	absPath, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", "", false
	}
	resolved, err := resolver.ResolveImport("", modulePath)
	if err != nil {
		return "", "", false
	}
	key := filepath.Clean(resolved.FilePath)
	if key != filepath.Clean(absPath) {
		return "", "", false
	}
	return key, resolved.ModulePath, true
}
