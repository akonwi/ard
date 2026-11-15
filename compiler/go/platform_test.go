package gotarget

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/frontend"
	"github.com/akonwi/ard/platform"
)

// signalWatchModule is the platform-specific part of Cooper's signal
// watcher (#569): unix platforms also watch SIGWINCH, which Windows lacks.
const signalWatchModule = `use go:os
use go:syscall

fn watched() [os::Signal] {
  [os::Interrupt]
}

#platform(unix)
fn watched() [os::Signal] {
  let signals = &mut List::new<os::Signal>()
  signals.push(os::Interrupt)
  signals.push(syscall::SIGHUP)
  signals.push(syscall::SIGQUIT)
  signals.push(syscall::SIGTERM)
  signals.push(syscall::SIGWINCH)
  signals.*
}

fn is_resize(signal: os::Signal) Bool {
  false
}

#platform(unix)
fn is_resize(signal: os::Signal) Bool {
  signal.String() == syscall::SIGWINCH.String()
}
`

const signalWatchMain = `use go:fmt
use go:os
use signals/watch

fn main() {
  fmt::Println(watch::watched().size(), watch::is_resize(os::Interrupt))
}
`

func writeSignalWatchProject(t *testing.T, watch string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"ard.toml":  "name = \"signals\"\nard = \">= 0.1.0\"\n",
		"watch.ard": watch,
		"main.ard":  signalWatchMain,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "main.ard")
}

func loadForPlatform(t *testing.T, mainPath string, target platform.Platform) (*frontend.LoadResult, error) {
	t.Helper()
	return frontend.LoadModuleWithOptions(mainPath, frontend.LoadOptions{Build: checker.BuildOptions{Platform: target}})
}

func TestPlatformSpecificSignalsBuildForUnixAndWindows(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles for Windows")
	}
	mainPath := writeSignalWatchProject(t, signalWatchModule)

	windows := platform.Platform{OS: "windows", Arch: "amd64"}
	loaded, err := loadForPlatform(t, mainPath, windows)
	if err != nil {
		t.Fatalf("load for windows: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower for windows: %v", err)
	}
	output := filepath.Join(t.TempDir(), "signals.exe")
	if _, err := BuildProgram(program, output, loaded.ProjectInfo); err != nil {
		t.Fatalf("build for windows: %v", err)
	}
	binary, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(binary, []byte("MZ")) {
		t.Fatalf("windows build is not a PE executable")
	}

	host := platform.Detect(filepath.Dir(mainPath))
	loaded, err = loadForPlatform(t, mainPath, host)
	if err != nil {
		t.Fatalf("load for %s: %v", host, err)
	}
	program, err = air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower for %s: %v", host, err)
	}
	hostBinary := filepath.Join(t.TempDir(), "signals")
	if _, err := BuildProgram(program, hostBinary, loaded.ProjectInfo); err != nil {
		t.Fatalf("build for %s: %v", host, err)
	}
	out, err := exec.Command(hostBinary).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "1 false"
	if platform.Matches("unix", host) {
		want = "5 false"
	}
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestUnixOnlySymbolFailsForWindowsWithoutPlatform(t *testing.T) {
	if testing.Short() {
		t.Skip("resolves Go packages for Windows")
	}
	unguarded := strings.Replace(signalWatchModule, "#platform(unix)\nfn watched()", "fn watched_all()", 1)
	mainPath := writeSignalWatchProject(t, unguarded)
	if _, err := loadForPlatform(t, mainPath, platform.Platform{OS: "windows", Arch: "amd64"}); err == nil {
		t.Fatal("expected syscall::SIGWINCH to fail for windows without #platform")
	}
}
