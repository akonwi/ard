package checker_test

import (
	"fmt"
	"go/build"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/std_lib"
)

func TestStdLibModules(t *testing.T) {
	// Get the ard package path
	pkg, err := build.Default.Import("github.com/akonwi/ard", "", build.FindOnly)
	if err != nil {
		t.Fatalf("Failed to find ard package: %v", err)
	}

	// Get all .ard files in the std_lib directory
	files, err := filepath.Glob(filepath.Join(pkg.Dir, "std_lib", "*.ard"))
	if err != nil {
		t.Fatalf("Failed to get std_lib files: %v", err)
	}

	for _, file := range files {
		moduleName := strings.TrimSuffix(filepath.Base(file), ".ard")
		modulePath := fmt.Sprintf("ard/%s", moduleName)

		t.Run(modulePath, func(t *testing.T) {
			// Read the embedded file using std_lib.Find
			content, err := std_lib.Find(modulePath)
			if err != nil {
				t.Fatalf("Failed to read embedded file: %v", err)
			}

			// Parse the .ard file
			result := ast.Parse(content, modulePath)
			if len(result.Errors) > 0 {
				var errorMessages []string
				for _, err := range result.Errors {
					errorMessages = append(errorMessages, err.Message)
				}
				t.Fatalf("Parse errors: %s", strings.Join(errorMessages, "\n"))
			}

			// Type check the program to create a Program with symbols
			c := checker.New(modulePath, result.Program, nil)
			c.Check()
			if c.HasErrors() {
				var errorMessages []string
				for _, d := range c.Diagnostics() {
					errorMessages = append(errorMessages, d.String())
				}
				t.Fatalf("Checker diagnostics: %s", strings.Join(errorMessages, "\n"))
			}
		})
	}
}
