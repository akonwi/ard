package vm_test

import (
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
			if res := run2(t, test.input); test.want != res {
				t.Logf("Expected %v, got %v", test.want, res)
				t.Fail()
			}
		})
	}
}
