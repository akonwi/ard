package std_lib

import (
	"embed"
	"fmt"
	"strings"
)

// embeddedFS contains Ard standard-library source files used by the checker and
// targets to resolve imports such as ard/io.
//
//go:embed *.ard
var embeddedFS embed.FS

// Modules lists the import paths of every embedded standard-library module.
func Modules() []string {
	entries, err := embeddedFS.ReadDir(".")
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".ard") {
			continue
		}
		paths = append(paths, "ard/"+strings.TrimSuffix(name, ".ard"))
	}
	return paths
}

// Find returns the content of an embedded .ard file by path
func Find(path string) ([]byte, error) {
	// Convert "ard/list" to "list.ard"
	if !strings.HasPrefix(path, "ard/") {
		return nil, fmt.Errorf("invalid std_lib path: %s", path)
	}

	moduleName := strings.TrimPrefix(path, "ard/")
	fileName := fmt.Sprintf("%s.ard", moduleName)

	return embeddedFS.ReadFile(fileName)
}
