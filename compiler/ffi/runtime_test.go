package ffi

import (
	"os"
	"testing"
)

func TestReadLineReadsMultipleLines(t *testing.T) {
	oldStdin := os.Stdin
	defer func() {
		os.Stdin = oldStdin
		stdinReaderMu.Lock()
		stdinReader = nil
		stdinSource = nil
		stdinReaderMu.Unlock()
	}()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	defer reader.Close()

	if _, err := writer.WriteString("first\nsecond\n"); err != nil {
		t.Fatalf("failed to seed stdin: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close stdin writer: %v", err)
	}

	os.Stdin = reader
	stdinReaderMu.Lock()
	stdinReader = nil
	stdinSource = nil
	stdinReaderMu.Unlock()

	first, err := ReadLine()
	if err != nil {
		t.Fatalf("first ReadLine failed: %v", err)
	}
	if first != "first" {
		t.Fatalf("expected first line, got %q", first)
	}

	second, err := ReadLine()
	if err != nil {
		t.Fatalf("second ReadLine failed: %v", err)
	}
	if second != "second" {
		t.Fatalf("expected second line, got %q", second)
	}
}
