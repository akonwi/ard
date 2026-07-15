package gotarget

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestRunProgramExecutesVoidCallsAsMatchArmResults(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"voidmatch\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:fmt

fn called(label: Str) {
  fmt::Println(label)
}

fn main() {
  match true {
    true => called("BOOL_CALLED"),
    false => fmt::Println("false"),
  }
  match "FT" {
    "FT" => called("STR_CALLED"),
    _ => fmt::Println("other"),
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outputCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		output, err := io.ReadAll(reader)
		outputCh <- output
		errCh <- err
	}()

	originalStdout := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
		_ = writer.Close()
		_ = reader.Close()
	}()
	runErr := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo)
	os.Stdout = originalStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output := <-outputCh
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if runErr != nil {
		t.Fatalf("RunProgram error = %v", runErr)
	}
	if got, want := string(output), "BOOL_CALLED\nSTR_CALLED\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
