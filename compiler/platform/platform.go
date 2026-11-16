// Package platform describes the operating system and architecture a program
// is compiled for, and the platform names `#platform(...)` attributes use to
// select declarations (ADR 0082).
package platform

import (
	"fmt"
	"go/types"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Platform is a target operating system and architecture, named as Go's
// GOOS and GOARCH.
type Platform struct {
	OS   string
	Arch string
}

func (p Platform) String() string {
	if p.IsZero() {
		return ""
	}
	return p.OS + "/" + p.Arch
}

// IsZero reports whether p is unset.
func (p Platform) IsZero() bool {
	return p.OS == "" && p.Arch == ""
}

// IntBits is the width of Go's int on p's architecture, which Ard's Int,
// Uint, and Uintptr follow. It is 0 for an unknown architecture.
func (p Platform) IntBits() int {
	sizes := types.SizesFor("gc", p.Arch)
	if sizes == nil {
		return 0
	}
	return int(sizes.Sizeof(types.Typ[types.Int]) * 8)
}

// Env returns base with GOOS and GOARCH replaced by p, for running the go
// tool so that it resolves and builds for p.
func (p Platform) Env(base []string) []string {
	env := make([]string, 0, len(base)+2)
	for _, entry := range base {
		if strings.HasPrefix(entry, "GOOS=") || strings.HasPrefix(entry, "GOARCH=") {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "GOOS="+p.OS, "GOARCH="+p.Arch)
}

// known is `go tool dist list` for the supported Go release. Update it when
// Go adds or removes a port.
var known = []Platform{
	{"aix", "ppc64"},
	{"android", "386"},
	{"android", "amd64"},
	{"android", "arm"},
	{"android", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"dragonfly", "amd64"},
	{"freebsd", "386"},
	{"freebsd", "amd64"},
	{"freebsd", "arm"},
	{"freebsd", "arm64"},
	{"illumos", "amd64"},
	{"ios", "amd64"},
	{"ios", "arm64"},
	{"js", "wasm"},
	{"linux", "386"},
	{"linux", "amd64"},
	{"linux", "arm"},
	{"linux", "arm64"},
	{"linux", "loong64"},
	{"linux", "mips"},
	{"linux", "mips64"},
	{"linux", "mips64le"},
	{"linux", "mipsle"},
	{"linux", "ppc64"},
	{"linux", "ppc64le"},
	{"linux", "riscv64"},
	{"linux", "s390x"},
	{"netbsd", "386"},
	{"netbsd", "amd64"},
	{"netbsd", "arm"},
	{"netbsd", "arm64"},
	{"openbsd", "386"},
	{"openbsd", "amd64"},
	{"openbsd", "arm"},
	{"openbsd", "arm64"},
	{"openbsd", "ppc64"},
	{"openbsd", "riscv64"},
	{"plan9", "386"},
	{"plan9", "amd64"},
	{"plan9", "arm"},
	{"solaris", "amd64"},
	{"wasip1", "wasm"},
	{"windows", "386"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

// unixOS is Go's `unix` build constraint set.
var unixOS = map[string]bool{
	"aix":       true,
	"android":   true,
	"darwin":    true,
	"dragonfly": true,
	"freebsd":   true,
	"hurd":      true,
	"illumos":   true,
	"ios":       true,
	"linux":     true,
	"netbsd":    true,
	"openbsd":   true,
	"solaris":   true,
}

// Unix is the platform name for Go's unix family of operating systems.
const Unix = "unix"

var (
	knownSet    map[Platform]bool
	knownOS     map[string]bool
	knownArch   map[string]bool
	initIndexes sync.Once
)

func indexes() {
	initIndexes.Do(func() {
		knownSet = map[Platform]bool{}
		knownOS = map[string]bool{}
		knownArch = map[string]bool{}
		for _, p := range known {
			knownSet[p] = true
			knownOS[p.OS] = true
			knownArch[p.Arch] = true
		}
	})
}

// Known returns every platform the compiler knows, sorted by OS then
// architecture.
func Known() []Platform {
	out := append([]Platform(nil), known...)
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// IsKnown reports whether p is a platform the compiler knows.
func IsKnown(p Platform) bool {
	indexes()
	return knownSet[p]
}

// Parse reads an `<os>/<arch>` platform, such as `windows/amd64`. The
// platform must be known.
func Parse(text string) (Platform, error) {
	osName, arch, ok := strings.Cut(text, "/")
	if !ok || osName == "" || arch == "" || strings.Contains(arch, "/") {
		return Platform{}, fmt.Errorf("invalid platform %q: expected <os>/<arch>, such as linux/amd64", text)
	}
	p := Platform{OS: osName, Arch: arch}
	if !IsKnown(p) {
		return Platform{}, fmt.Errorf("unknown platform %q; run `go tool dist list` for supported platforms", text)
	}
	return p, nil
}

// osMatches reports whether an operating system name matches goos. Like Go's
// build constraints, `linux` also matches android, `darwin` also matches ios,
// and `solaris` also matches illumos.
func osMatches(name string, goos string) bool {
	if name == goos {
		return true
	}
	switch name {
	case "linux":
		return goos == "android"
	case "darwin":
		return goos == "ios"
	case "solaris":
		return goos == "illumos"
	}
	return false
}

// Matches reports whether the platform name matches p. Names are an
// operating system (`linux`), an operating system and architecture
// (`linux_amd64`), or `unix`.
func Matches(name string, p Platform) bool {
	if name == Unix {
		return unixOS[p.OS]
	}
	if osName, arch, ok := strings.Cut(name, "_"); ok {
		return arch == p.Arch && osMatches(osName, p.OS)
	}
	return osMatches(name, p.OS)
}

// ValidateName reports an error when name is not a platform name, or names
// no known platform.
func ValidateName(name string) error {
	indexes()
	if name == Unix {
		return nil
	}
	osName, arch, hasArch := strings.Cut(name, "_")
	if !knownOS[osName] {
		return fmt.Errorf("unknown platform %q", name)
	}
	if !hasArch {
		return nil
	}
	if !knownArch[arch] {
		return fmt.Errorf("unknown platform %q: unknown architecture %q", name, arch)
	}
	for _, p := range known {
		if Matches(name, p) {
			return nil
		}
	}
	return fmt.Errorf("unknown platform %q: %s does not run on %s", name, osName, arch)
}

// Preferred returns every known platform, starting with host and then the
// most common platforms (linux/amd64, darwin/arm64, windows/amd64). Tools use
// it to pick a representative platform for a set of equivalent ones.
func Preferred(host Platform) []Platform {
	candidates := append([]Platform{host, {"linux", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}}, Known()...)
	ordered := make([]Platform, 0, len(candidates))
	added := map[Platform]bool{}
	for _, candidate := range candidates {
		if candidate.IsZero() || added[candidate] {
			continue
		}
		added[candidate] = true
		ordered = append(ordered, candidate)
	}
	return ordered
}

// Overlap returns a known platform that both name lists match, if any.
func Overlap(left []string, right []string) (Platform, bool) {
	for _, p := range Known() {
		if MatchesAny(left, p) && MatchesAny(right, p) {
			return p, true
		}
	}
	return Platform{}, false
}

// MatchesAny reports whether any of the names matches p.
func MatchesAny(names []string, p Platform) bool {
	for _, name := range names {
		if Matches(name, p) {
			return true
		}
	}
	return false
}

type detectKey struct {
	goos, goarch, goenv string
}

var (
	detectMu    sync.Mutex
	detectCache = map[detectKey]Platform{}
)

// Detect returns the platform the go tool targets: `go env GOOS GOARCH`,
// which honours the environment and `go env -w`. It falls back to the
// compiler's own platform when the go tool cannot be run. Results are cached
// per process for the same GOOS, GOARCH, and GOENV environment.
func Detect(dir string) Platform {
	key := detectKey{goos: os.Getenv("GOOS"), goarch: os.Getenv("GOARCH"), goenv: os.Getenv("GOENV")}
	detectMu.Lock()
	defer detectMu.Unlock()
	if cached, ok := detectCache[key]; ok {
		return cached
	}
	detected := Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	cmd := exec.Command("go", "env", "GOOS", "GOARCH")
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.Output(); err == nil {
		fields := strings.Fields(string(out))
		if len(fields) == 2 {
			detected = Platform{OS: fields[0], Arch: fields[1]}
		}
	}
	detectCache[key] = detected
	return detected
}
