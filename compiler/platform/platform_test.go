package platform

import (
	"os/exec"
	"strings"
	"testing"
)

func TestNameMatches(t *testing.T) {
	tests := []struct {
		name     string
		platform Platform
		want     bool
	}{
		{"linux", Platform{"linux", "amd64"}, true},
		{"linux", Platform{"windows", "amd64"}, false},
		{"linux", Platform{"android", "arm64"}, true},
		{"darwin", Platform{"ios", "arm64"}, true},
		{"solaris", Platform{"illumos", "amd64"}, true},
		{"android", Platform{"linux", "arm64"}, false},
		{"unix", Platform{"darwin", "arm64"}, true},
		{"unix", Platform{"illumos", "amd64"}, true},
		{"unix", Platform{"windows", "amd64"}, false},
		{"unix", Platform{"js", "wasm"}, false},
		{"unix", Platform{"wasip1", "wasm"}, false},
		{"unix", Platform{"plan9", "amd64"}, false},
		{"linux_amd64", Platform{"linux", "amd64"}, true},
		{"linux_amd64", Platform{"linux", "arm64"}, false},
		{"linux_arm64", Platform{"android", "arm64"}, true},
		{"windows_arm64", Platform{"windows", "arm64"}, true},
	}
	for _, tt := range tests {
		if got := Matches(tt.name, tt.platform); got != tt.want {
			t.Errorf("Matches(%q, %s) = %v, want %v", tt.name, tt.platform, got, tt.want)
		}
	}
}

func TestValidName(t *testing.T) {
	for _, name := range []string{"linux", "darwin", "windows", "wasip1", "js", "unix", "linux_amd64", "linux_386", "darwin_arm64", "linux_arm64", "android_arm64"} {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", name, err)
		}
	}
	for _, name := range []string{"linx", "Linux", "amd64", "unix_amd64", "linux_wasm", "windows_riscv64", "linux_", "_amd64", "default", "linux_amd64_v3"} {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", name)
		}
	}
}

func TestParse(t *testing.T) {
	got, err := Parse("windows/amd64")
	if err != nil || got != (Platform{OS: "windows", Arch: "amd64"}) {
		t.Fatalf("Parse(windows/amd64) = %v, %v", got, err)
	}
	for _, input := range []string{"", "windows", "windows/", "/amd64", "windows/amd64/x", "linux/wasm", "linx/amd64"} {
		if _, err := Parse(input); err == nil {
			t.Errorf("Parse(%q) = nil error", input)
		}
	}
}

func TestKnownIsSortedAndUnique(t *testing.T) {
	known := Known()
	if len(known) < 40 {
		t.Fatalf("expected the Go port list, got %d platforms", len(known))
	}
	seen := map[Platform]bool{}
	for index, p := range known {
		if seen[p] {
			t.Fatalf("duplicate platform %s", p)
		}
		seen[p] = true
		if index > 0 && known[index-1].String() >= p.String() {
			t.Fatalf("platforms not sorted at %s", p)
		}
	}
	for _, p := range []Platform{{"linux", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}} {
		if !IsKnown(p) {
			t.Errorf("%s should be known", p)
		}
	}
}

func TestEnvSetsTarget(t *testing.T) {
	env := Platform{OS: "windows", Arch: "arm64"}.Env([]string{"PATH=/bin", "GOOS=linux", "GOARCH=amd64"})
	joined := strings.Join(env, " ")
	if strings.Contains(joined, "GOOS=linux") || strings.Contains(joined, "GOARCH=amd64") {
		t.Fatalf("Env kept the old target: %v", env)
	}
	if !strings.Contains(joined, "GOOS=windows") || !strings.Contains(joined, "GOARCH=arm64") || !strings.Contains(joined, "PATH=/bin") {
		t.Fatalf("Env = %v", env)
	}
}

func TestDetectReportsAKnownPlatform(t *testing.T) {
	t.Setenv("GOOS", "windows")
	t.Setenv("GOARCH", "arm64")
	if got := Detect(""); got != (Platform{OS: "windows", Arch: "arm64"}) {
		t.Fatalf("Detect with GOOS/GOARCH set = %s", got)
	}
}

func TestPreferredStartsWithHostThenCommonPlatforms(t *testing.T) {
	host := Platform{OS: "freebsd", Arch: "arm64"}
	preferred := Preferred(host)
	want := []Platform{host, {"linux", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}}
	for index, p := range want {
		if preferred[index] != p {
			t.Fatalf("Preferred[%d] = %s, want %s", index, preferred[index], p)
		}
	}
	if len(preferred) != len(Known()) {
		t.Fatalf("Preferred lists %d platforms, want each known platform once (%d)", len(preferred), len(Known()))
	}
}

func TestIntBits(t *testing.T) {
	tests := map[Platform]int{
		{"linux", "amd64"}:   64,
		{"linux", "386"}:     32,
		{"linux", "arm"}:     32,
		{"linux", "mipsle"}:  32,
		{"js", "wasm"}:       64,
		{"darwin", "arm64"}:  64,
		{"linux", "unknown"}: 0,
	}
	for p, want := range tests {
		if got := p.IntBits(); got != want {
			t.Errorf("%s IntBits = %d, want %d", p, got, want)
		}
	}
}

func TestKnownMatchesTheGoToolchain(t *testing.T) {
	out, err := exec.Command("go", "tool", "dist", "list").Output()
	if err != nil {
		t.Skipf("go tool dist list: %v", err)
	}
	var want []string
	for _, line := range strings.Fields(string(out)) {
		want = append(want, line)
	}
	var got []string
	for _, p := range Known() {
		got = append(got, p.String())
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("Known() = %v\ngo tool dist list = %v", got, want)
	}
}
