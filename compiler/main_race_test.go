package main

import (
	"bytes"
	"debug/buildinfo"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireRaceDetector skips t when the host Go toolchain cannot build with
// -race (no cgo where it is required, or an unsupported GOOS/GOARCH).
func requireRaceDetector(t *testing.T) {
	t.Helper()
	cmd := exec.Command("go", "list", "-race", "runtime")
	cmd.Dir = t.TempDir()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("race detector unavailable: %s", strings.TrimSpace(string(out)))
	}
}

// racyCounterBody increments a shared counter from main and from a task with no
// synchronization between the two increments, a data race by Go's memory model.
const racyCounterBody = `  mut counter = Counter{value: 0}
  let pointer = &mut counter
  let done = Chan::new<Bool>()
  async::start(fn() {
    pointer.value = pointer.value + 1
    done.send(true)
  })
  pointer.value = pointer.value + 1
  done.recv().expect("done")
`

func writeRacyProject(t *testing.T) string {
	t.Helper()
	projectDir := t.TempDir()
	files := map[string]string{
		"ard.toml": "name = \"racy\"\nard = \">= 0.27.0\"\n",
		"main.ard": "use ard/async\nuse go:fmt\n\nstruct Counter { value: Int }\n\nfn main() {\n" +
			racyCounterBody + "  fmt::Println(counter.value)\n}\n",
		"racy_test.ard": "use ard/async\n\nstruct Counter { value: Int }\n\ntest fn racy_counter() Void!Str {\n" +
			racyCounterBody + "  Result::ok(())\n}\n\ntest fn calm() Void!Str {\n  Result::ok(())\n}\n",
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(projectDir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return projectDir
}

func binaryBuiltWithRace(t *testing.T, path string) bool {
	t.Helper()
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		t.Fatalf("read build info of %s: %v", path, err)
	}
	for _, setting := range info.Settings {
		if setting.Key == "-race" {
			return setting.Value == "true"
		}
	}
	return false
}

func runBinary(t *testing.T, path string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("run %s: %v", path, err)
	}
	code := 0
	if exitErr != nil {
		code = exitErr.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

const raceReportHeader = "WARNING: DATA RACE"

func TestRaceFlag(t *testing.T) {
	requireRaceDetector(t)
	projectDir := writeRacyProject(t)
	mainPath := filepath.Join(projectDir, "main.ard")

	t.Run("run reports the race and fails", func(t *testing.T) {
		stdout, stderr, err := runCLIForTest(t, "run", "--race", mainPath)
		if err == nil {
			t.Fatalf("run --race succeeded; want failure\nstderr:\n%s", stderr)
		}
		if strings.TrimSpace(stdout) != "2" {
			t.Fatalf("stdout = %q, want program output", stdout)
		}
		for _, want := range []string{raceReportHeader, "Found 1 data race(s)", "error: exit status 66"} {
			if !strings.Contains(stderr, want) {
				t.Fatalf("stderr missing %q:\n%s", want, stderr)
			}
		}
	})

	t.Run("run without the flag is unchanged", func(t *testing.T) {
		stdout, stderr, err := runCLIForTest(t, "run", mainPath)
		if err != nil {
			t.Fatalf("run failed: %v\nstderr:\n%s", err, stderr)
		}
		// The CLI helper process appends the test framework's PASS line when
		// main returns normally.
		if strings.TrimSpace(strings.TrimSuffix(stdout, "PASS\n")) != "2" || strings.Contains(stderr, raceReportHeader) {
			t.Fatalf("stdout = %q, stderr = %q; want plain output", stdout, stderr)
		}
	})

	t.Run("build instruments only when asked", func(t *testing.T) {
		binary := filepath.Join(t.TempDir(), "racy-bin")
		if _, stderr, err := runCLIForTest(t, "build", mainPath, "--race", "--out", binary); err != nil {
			t.Fatalf("build --race failed: %v\nstderr:\n%s", err, stderr)
		}
		if !binaryBuiltWithRace(t, binary) {
			t.Fatal("build --race produced a binary without -race")
		}
		if _, stderr, code := runBinary(t, binary); code != 66 || !strings.Contains(stderr, raceReportHeader) {
			t.Fatalf("race binary exit = %d, want 66 with a race report\nstderr:\n%s", code, stderr)
		}

		// Rebuilding to the same path without --race must not reuse the
		// instrumented output.
		if _, stderr, err := runCLIForTest(t, "build", mainPath, "--out", binary); err != nil {
			t.Fatalf("build failed: %v\nstderr:\n%s", err, stderr)
		}
		if binaryBuiltWithRace(t, binary) {
			t.Fatal("build without --race produced a race-instrumented binary")
		}
		if stdout, stderr, code := runBinary(t, binary); code != 0 || strings.TrimSpace(stdout) != "2" {
			t.Fatalf("plain binary exit = %d stdout = %q, want 0 and program output\nstderr:\n%s", code, stdout, stderr)
		}
	})

	t.Run("test reports outcomes then fails on the race", func(t *testing.T) {
		stdout, stderr, err := runCLIForTest(t, "test", "--race", projectDir)
		if err == nil {
			t.Fatalf("test --race succeeded; want failure\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
		}
		for _, want := range []string{"racy_test::racy_counter", "racy_test::calm", "2 passed; 0 failed; 0 panicked"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout missing %q:\n%s", want, stdout)
			}
		}
		for _, want := range []string{raceReportHeader, "error: data race detected"} {
			if !strings.Contains(stderr, want) {
				t.Fatalf("stderr missing %q:\n%s", want, stderr)
			}
		}
	})

	t.Run("test without the flag is unchanged", func(t *testing.T) {
		stdout, stderr, err := runCLIForTest(t, "test", projectDir)
		if err != nil {
			t.Fatalf("test failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		if !strings.Contains(stdout, "2 passed; 0 failed; 0 panicked") || strings.Contains(stderr, raceReportHeader) {
			t.Fatalf("stdout = %q, stderr = %q; want a clean pass", stdout, stderr)
		}
	})
}

func TestRaceFlagExplainsUnavailableDetector(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go tool not found")
	}
	// linux/386 is a Go target without race detector support, so this runs
	// the same on every host.
	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "386")
	projectDir := writeRacyProject(t)
	binary := filepath.Join(t.TempDir(), "racy-bin")
	commands := map[string][]string{
		"run":   {"run", "--race", filepath.Join(projectDir, "main.ard")},
		"build": {"build", filepath.Join(projectDir, "main.ard"), "--race", "--out", binary},
		"test":  {"test", "--race", projectDir},
	}
	for name, args := range commands {
		t.Run(name, func(t *testing.T) {
			_, stderr, err := runCLIForTest(t, args...)
			if err == nil {
				t.Fatalf("%s --race succeeded on linux/386; want failure", name)
			}
			for _, want := range []string{
				"error: --race is unavailable: -race is not supported on linux/386",
				"hint: ",
				"go.dev/doc/articles/race_detector",
			} {
				if !strings.Contains(stderr, want) {
					t.Fatalf("stderr missing %q:\n%s", want, stderr)
				}
			}
		})
	}
}
