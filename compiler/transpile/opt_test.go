package transpile

import "testing"

func TestOptimizeGoFileIR(t *testing.T) {
	optimized := optimizeGoFileIR(goFileIR{
		PackageName: "main",
		Imports: []goImportIR{
			{Alias: helperImportAlias, Path: helperImportPath},
			{Alias: helperImportAlias, Path: helperImportPath},
		},
		Decls: []goDeclIR{
			{Source: ""},
			{Source: " type Person struct{}\n"},
		},
	})

	if len(optimized.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(optimized.Imports))
	}
	if len(optimized.Decls) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(optimized.Decls))
	}
	if optimized.Decls[0].Source != "type Person struct{}" {
		t.Fatalf("expected trimmed declaration, got %q", optimized.Decls[0].Source)
	}
}
