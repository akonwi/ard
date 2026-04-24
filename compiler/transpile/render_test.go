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

func TestRenderGoFile(t *testing.T) {
	fileIR := lowerGoFileIR("main", map[string]string{helperImportPath: helperImportAlias})
	fileIR.Decls = append(fileIR.Decls,
		goDeclIR{Source: "type Person struct{}"},
		goDeclIR{Source: "func greet() string { return \"hi\" }"},
	)

	got, err := renderGoFile(fileIR)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	want := "package main\n\nimport ardgo \"github.com/akonwi/ard/go\"\n\ntype Person struct{}\n\nfunc greet() string {\n\treturn \"hi\"\n}\n"
	if string(got) != want {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", want, string(got))
	}
}
