package checker

import (
	"testing"

	"github.com/akonwi/ard/parse"
	std_lib "github.com/akonwi/ard/std_lib"
)

// TestEmbeddedStdLibModulesCheckCleanly gates the shipped standard library on
// the current checker rules. FindEmbeddedModule intentionally tolerates
// diagnostics at import time (issue #350), so without this gate an invalid
// stdlib body would silently flow into every importing program.
func TestEmbeddedStdLibModulesCheckCleanly(t *testing.T) {
	modules := std_lib.Modules()
	if len(modules) == 0 {
		t.Fatal("no embedded std_lib modules found")
	}
	for _, path := range modules {
		t.Run(path, func(t *testing.T) {
			content, err := std_lib.Find(path)
			if err != nil {
				t.Fatalf("read embedded module: %v", err)
			}
			result := parse.Parse(content, path)
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			_, diagnostics := check(result.Program, nil, path, path, CheckOptions{})
			for _, diagnostic := range diagnostics {
				if diagnostic.Kind == Error {
					t.Errorf("%s: %s", diagnostic.Primary.Span.Location.Start, diagnostic.Message)
				}
			}
		})
	}
}
