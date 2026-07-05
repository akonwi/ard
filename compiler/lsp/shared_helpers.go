package lsp

import (
	"path/filepath"
	"runtime"
	"strings"
)

// isValidRenameIdentifier reports whether newName is a legal Ard identifier.
func isValidRenameIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || r == '$' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || (i > 0 && '0' <= r && r <= '9') {
			continue
		}
		return false
	}
	return !(name[0] >= '0' && name[0] <= '9')
}

// stdLibSourcePath maps an ard/* import path to the compiler's bundled
// standard library source file, for tooling that shows stdlib sources.
func stdLibSourcePath(importPath string) string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return importPath
	}
	moduleName := strings.TrimPrefix(importPath, "ard/") + ".ard"
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "std_lib", moduleName))
}
