package vm_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/checker_v2"
	"github.com/akonwi/ard/vm"
)

func run2(t *testing.T, input string) any {
	t.Helper()
	tree, err := ast.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Error parsing program: %v", err)
	}
	program, diagnostics := checker_v2.Check(tree)
	if len(diagnostics) > 0 {
		t.Fatalf("Diagnostics found: %v", diagnostics)
	}
	res, err := vm.Run2(program)
	if err != nil {
		t.Fatalf("VM error: %v", err)
	}
	return res
}

func runTests2(t *testing.T, tests []test) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if res := run(t, test.input); test.want != res {
				t.Logf("Expected %v, got %v", test.want, res)
				t.Fail()
			}
		})
	}
}

func TestEmptyProgram(t *testing.T) {
	res := run2(t, "")
	if res != nil {
		t.Fatalf("Expected nil, got %v", res)
	}
}

func TestPrinting(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	run2(t, strings.Join([]string{
		`use ard/io`,
		`io::print("Hello, World!")`,
		// `io::print("Hello, {{"Ard"}}!")`,
	}, "\n"))

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	got := buf.String()

	for _, want := range []string{
		"Hello, World!",
		// "Hello, Ard!",
	} {
		if strings.Contains(got, want) == false {
			t.Errorf("Expected \"%s\", got %s", want, got)
		}
	}
}
