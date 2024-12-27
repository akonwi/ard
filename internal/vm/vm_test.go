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

func TestBindingVariables(t *testing.T) {
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

func TestReassigningVariables(t *testing.T) {
	res := run(t, strings.Join([]string{
		`mut val = 1`,
		`val = 2`,
		`val = 3`,
		`val`,
	}, "\n"))
	if res != 3 {
		t.Fatalf("Expected 3, got %v", res)
	}
}

func TestMemberAccess(t *testing.T) {
	res := run(t, `"foobar".size`)
	if res != 6 {
		t.Fatalf("Expected 6, got %v", res)
	}
}

func TestUnaryExpressions(t *testing.T) {
	for _, test := range []struct {
		input string
		want  any
	}{
		{`!true`, false},
		{`!false`, true},
		{`-10`, -10},
	} {
		res := run(t, test.input)
		if res != test.want {
			t.Fatalf("Expected %v, got %v", test.want, res)
		}
	}
}

func TestNumberOperations(t *testing.T) {
	tests := []struct {
		input string
		want  any
	}{
		{input: `30 + 12`, want: 42},
		{input: `30 - 2`, want: 28},
		{input: `30 * 2`, want: 60},
		{input: `30 / 2`, want: 15},
		{input: `30 % 2`, want: 30 % 2},
		{input: `30 > 2`, want: true},
		{input: `30 >= 2`, want: true},
		{input: `30 < 2`, want: false},
		{input: `30 <= 2`, want: false},
		{input: `30 <= 30`, want: true},
	}

	for _, test := range tests {
		if res := run(t, test.input); res != test.want {
			t.Errorf("%s = %v but got %v", test.input, test.want, res)
		}
	}
}

func TestEquality(t *testing.T) {
	tests := []struct {
		input string
		want  any
	}{
		{input: `30 == 30`, want: true},
		{input: `1 == 10`, want: false},
		{input: `30 != 30`, want: false},
		{input: `1 != 10`, want: true},
		{input: `true == false`, want: false},
		{input: `true != false`, want: true},
		{input: `"hello" == "world"`, want: false},
		{input: `"hello" != "world"`, want: true},
	}

	for _, test := range tests {
		if res := run(t, test.input); res != test.want {
			t.Errorf("%s = %v but got %v", test.input, test.want, res)
		}
	}
}
