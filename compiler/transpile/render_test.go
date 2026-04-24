package transpile

import "testing"

func TestRenderGoFilePrelude(t *testing.T) {
	got, err := renderGoFilePrelude(lowerGoFileIR("main", map[string]string{
		helperImportPath: helperImportAlias,
		"sync":           "sync",
	}))
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	want := "package main\n\nimport (\n\tardgo \"github.com/akonwi/ard/go\"\n\tsync \"sync\"\n)\n"
	if string(got) != want {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", want, string(got))
	}
}
