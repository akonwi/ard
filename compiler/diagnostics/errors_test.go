package diagnostics

import (
	"errors"
	"fmt"
	"testing"
)

func TestAlreadyReportedPreservesCause(t *testing.T) {
	cause := errors.New("type errors")
	err := AlreadyReported(cause)
	if !IsAlreadyReported(err) {
		t.Fatal("error was not marked as already reported")
	}
	if !errors.Is(err, cause) {
		t.Fatal("reported error did not preserve its cause")
	}
	if got := err.Error(); got != "type errors" {
		t.Fatalf("Error() = %q, want %q", got, "type errors")
	}
}

func TestIsAlreadyReportedRecognizesWrappedMarker(t *testing.T) {
	err := fmt.Errorf("load module: %w", AlreadyReported(errors.New("parse errors")))
	if !IsAlreadyReported(err) {
		t.Fatal("wrapped marker was not recognized")
	}
}
