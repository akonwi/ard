package vm_test

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/akonwi/ard/internal/ast"
	"github.com/akonwi/ard/internal/checker"
	"github.com/akonwi/ard/internal/vm"
	ts_ard "github.com/akonwi/tree-sitter-ard/bindings/go"
)

func parse(t *testing.T, input string) ast.Program {
	t.Helper()
	ts, err := ts_ard.MakeParser()
	if err != nil {
		panic(err)
	}
	tree := ts.Parse([]byte(input), nil)
	parser := ast.NewParser([]byte(input), tree)
	program, err := parser.Parse()
	if err != nil {
		t.Fatalf("Program error: %v", err)
	}
	return program
}

func run(t *testing.T, input string) any {
	t.Helper()
	program, diagnostics := checker.Check(parse(t, input))
	if len(diagnostics) > 0 {
		t.Fatalf("Diagnostics found: %v", diagnostics)
	}
	v := vm.New(&program)
	res, err := v.Run()
	if err != nil {
		t.Fatalf("VM error: %v", err)
	}
	return res
}

func TestEmptyProgram(t *testing.T) {
	res := run(t, "")
	if res != nil {
		t.Fatalf("Expected nil, got %v", res)
	}
}

func TestPrinting(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	run(t, strings.Join([]string{
		`use std/io`,
		`io.print("Hello, World!")`,
		`io.print("Hello, {{"Ard"}}!")`,
	}, "\n"))

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	got := buf.String()

	for _, want := range []string{"Hello, World!", "Hello, Ard!"} {
		if strings.Contains(got, want) == false {
			t.Errorf("Expected \"%s\", got %s", want, got)
		}
	}
}

func TestVariables(t *testing.T) {
	for want := range []any{
		"Alice",
		40,
		true,
	} {
		res := run(t, strings.Join([]string{
			fmt.Sprintf(`let val = %v`, want),
			`val`,
		}, "\n"))
		if res != want {
			t.Fatalf("Expected %v, got %v", want, res)
		}
	}
}
