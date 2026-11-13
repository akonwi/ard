package gotarget

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
)

// requireRaceDetector skips t when the host Go toolchain cannot build with
// -race (no cgo where it is required, or an unsupported GOOS/GOARCH).
func requireRaceDetector(t *testing.T) {
	t.Helper()
	if err := checkRaceDetector(t.TempDir()); err != nil {
		t.Skipf("race detector unavailable: %v", err)
	}
}

func TestGoBuildOptionsForCarriesRaceAndRelease(t *testing.T) {
	info := &checker.ProjectInfo{}
	info.Go.BuildTags = []string{"special"}
	tests := []struct {
		name    string
		options BuildOptions
		want    goBuildOptions
	}{
		{name: "defaults", options: BuildOptions{}, want: goBuildOptions{tags: []string{"special"}}},
		{name: "race", options: BuildOptions{Race: true}, want: goBuildOptions{tags: []string{"special"}, race: true}},
		{name: "release", options: BuildOptions{Release: true}, want: goBuildOptions{tags: []string{"special"}, release: true}},
		{name: "race release", options: BuildOptions{Race: true, Release: true}, want: goBuildOptions{tags: []string{"special"}, race: true, release: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := goBuildOptionsFor(info, tt.options)
			if got.race != tt.want.race || got.release != tt.want.release || strings.Join(got.tags, ",") != strings.Join(tt.want.tags, ",") {
				t.Fatalf("goBuildOptionsFor() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCheckRaceDetectorExplainsUnsupportedConfigurations(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go tool not found")
	}
	tests := []struct {
		name   string
		env    map[string]string
		reason string
	}{
		{
			name:   "unsupported platform",
			env:    map[string]string{"GOOS": "linux", "GOARCH": "386"},
			reason: "linux/386",
		},
		{
			name:   "cgo disabled where the race runtime needs it",
			env:    map[string]string{"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0"},
			reason: "requires cgo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			err := checkRaceDetector(t.TempDir())
			var unavailable *RaceUnavailableError
			if !errors.As(err, &unavailable) {
				t.Fatalf("checkRaceDetector() error = %v, want *RaceUnavailableError", err)
			}
			if !strings.Contains(err.Error(), tt.reason) {
				t.Fatalf("error %q does not mention %q", err.Error(), tt.reason)
			}
			if !strings.HasPrefix(err.Error(), "--race is unavailable: ") {
				t.Fatalf("error %q should name the --race flag", err.Error())
			}
			if !strings.Contains(unavailable.Hint(), "go.dev/doc/articles/race_detector") {
				t.Fatalf("hint %q should link the race detector requirements", unavailable.Hint())
			}
		})
	}
}

func TestGoTargetParityAsyncStartIsRaceFree(t *testing.T) {
	requireRaceDetector(t)
	for _, tc := range asyncStartParityCases {
		t.Run(tc.name, func(t *testing.T) {
			program := lowerParitySource(t, tc.input)
			// The helper fails the test on a non-zero exit, which is how a race
			// report surfaces (exit status 66).
			if got := strings.TrimSpace(runGoTargetParityJSONWithOptions(t, program, goBuildOptions{race: true})); got != tc.want {
				t.Fatalf("go output = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRenderTestRunnerChecksRacesPerTestOnlyInRaceMode(t *testing.T) {
	program := &air.Program{
		Modules:   []air.Module{{ID: 0, Path: "math.ard", Functions: []air.FunctionID{0}}},
		Functions: []air.Function{{ID: 0, Module: 0, Name: "adds", IsTest: true}},
	}
	tests := []TestCase{{Name: "adds", DisplayName: "math::adds", Function: 0}}

	plain := renderTestRunner(program, tests, TestOptions{}, nil)
	if strings.Contains(plain, "RaceErrors") || strings.Contains(plain, "\"runtime\"") || strings.Contains(plain, "Race bool") {
		t.Fatalf("runner without --race must not reference the race runtime:\n%s", plain)
	}

	raced := renderTestRunner(program, tests, TestOptions{Race: true}, nil)
	for _, want := range []string{"\"runtime\"", "runtime.RaceErrors()", "out.Race = true", "data race detected"} {
		if !strings.Contains(raced, want) {
			t.Fatalf("race runner missing %q:\n%s", want, raced)
		}
	}
}

func TestTestRunnerReservesRuntimeAliasOnlyInRaceMode(t *testing.T) {
	program := &air.Program{
		Modules:   []air.Module{{ID: 0, Path: "runtime.ard", Functions: []air.FunctionID{0}}},
		Functions: []air.Function{{ID: 0, Module: 0, Name: "enum_test", IsTest: true}},
	}
	if got := testRunnerImportAliases(program, []TestCase{{Function: 0}}, false).modules[0]; got != "runtime" {
		t.Fatalf("module alias without --race = %q, want runtime (unchanged default output)", got)
	}
	if got := testRunnerImportAliases(program, []TestCase{{Function: 0}}, true).modules[0]; got != "runtime_1" {
		t.Fatalf("module alias with --race = %q, want runtime_1", got)
	}
}
