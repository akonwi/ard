package formatter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStdLibFormattingIsIdempotent(t *testing.T) {
	paths, err := filepath.Glob("../std_lib/*.ard")
	if err != nil {
		t.Fatalf("failed to list std_lib files: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("no std_lib files found")
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			input, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("failed to read file: %v", readErr)
			}

			first, formatErr := Format(input, path)
			if formatErr != nil {
				t.Fatalf("first format failed: %v", formatErr)
			}

			second, secondErr := Format(first, path)
			if secondErr != nil {
				t.Fatalf("second format failed: %v", secondErr)
			}

			if string(first) != string(second) {
				t.Fatalf("format is not idempotent for %s", path)
			}
		})
	}
}
