package vm_test

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestPrinting(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	run(t, strings.Join([]string{
		`use ard/io`,
		`io::print("Hello, World!")`,
	}, "\n"))

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	got := strings.TrimSpace(buf.String())
	want := "Hello, World!"

	if want != got {
		t.Errorf("Expected \"%s\", got \"%s\"", want, got)
	}
}

func TestEscapeSequences(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	run(t, strings.Join([]string{
		`use ard/io`,
		`io::print("Line 1\nLine 2")`,
		`io::print("Tab\tTest")`,
		`io::print("Quote \"Test\"")`,
	}, "\n"))

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	got := buf.String()

	expectedOutputs := []string{
		"Line 1",
		"Line 2",
		"Tab\tTest",
		"Quote \"Test\"",
	}

	for _, want := range expectedOutputs {
		if strings.Contains(got, want) == false {
			t.Errorf("Expected output to contain \"%s\", got %s", want, got)
		}
	}
}
