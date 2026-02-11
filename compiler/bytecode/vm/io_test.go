package vm

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestBytecodePrinting(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runBytecode(t, strings.Join([]string{
		`use ard/io`,
		`io::print("Hello, World!")`,
	}, "\n"))

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	got := strings.TrimSpace(buf.String())
	want := "Hello, World!"

	if want != got {
		t.Fatalf("Expected %q, got %q", want, got)
	}
}

func TestBytecodeEscapeSequences(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runBytecode(t, strings.Join([]string{
		`use ard/io`,
		`io::print("Line 1\nLine 2")`,
		`io::print("Tab\tTest")`,
		`io::print("Quote \"Test\"")`,
	}, "\n"))

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	got := buf.String()

	expectedOutputs := []string{
		"Line 1",
		"Line 2",
		"Tab\tTest",
		"Quote \"Test\"",
	}

	for _, want := range expectedOutputs {
		if !strings.Contains(got, want) {
			t.Fatalf("Expected output to contain %q, got %q", want, got)
		}
	}
}
