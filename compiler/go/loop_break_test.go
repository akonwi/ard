package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// TestRunProgramBreakInsideSelectExitsLoop pins that Ard's `break` exits the
// nearest enclosing loop even when it lowers inside an emitted Go select or
// switch, where a bare Go `break` would bind to that construct instead
// (issue #270). The backend must emit a labeled loop and `break <label>`.
func TestRunProgramBreakInsideSelectExitsLoop(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"selectbreak\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`fn main() {
  let jobs = Chan::new<Int>(3)
  jobs.send(1)
  jobs.send(2)
  jobs.send(3)

  mut iterations = 0
  while iterations < 3 {
    iterations =+ 1
    select {
      let v = jobs.recv() => {
        break
      },
    }
  }
  if not iterations == 1 {
    panic("break exited the select instead of the loop: {iterations} iterations")
  }

  // Nested loops: the intercepted break binds to the inner loop only.
  let signals = Chan::new<Int>(2)
  signals.send(1)
  signals.send(2)

  mut outer = 0
  mut inner_total = 0
  while outer < 2 {
    outer =+ 1
    mut spins = 0
    while spins < 3 {
      spins =+ 1
      select {
        let v = signals.recv() => {
          break
        },
      }
    }
    inner_total =+ spins
  }
  if not outer == 2 {
    panic("inner break must not exit the outer loop: {outer} outer iterations")
  }
  if not inner_total == 2 {
    panic("inner loops should each break after one spin: {inner_total}")
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
	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
