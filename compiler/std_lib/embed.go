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

// Find returns the content of an embedded .ard file by path
func Find(path string) ([]byte, error) {
	// Convert "ard/duration" to "duration.ard"
	if !strings.HasPrefix(path, "ard/") {
		return nil, fmt.Errorf("invalid std_lib path: %s", path)
	}

	moduleName := strings.TrimPrefix(path, "ard/")
	fileName := fmt.Sprintf("%s.ard", moduleName)

	return embeddedFS.ReadFile(fileName)
}
