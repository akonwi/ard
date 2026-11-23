package checker_test

import (
	"fmt"
	"strings"
	"testing"

	checker "github.com/akonwi/ard/compiler/checker"
	"github.com/akonwi/ard/compiler/parse"
	"github.com/akonwi/ard/compiler/platform"
)

var (
	linuxAMD64   = platform.Platform{OS: "linux", Arch: "amd64"}
	darwinARM64  = platform.Platform{OS: "darwin", Arch: "arm64"}
	windowsAMD64 = platform.Platform{OS: "windows", Arch: "amd64"}
	plan9AMD64   = platform.Platform{OS: "plan9", Arch: "amd64"}
)

func checkForPlatform(t *testing.T, source string, target platform.Platform) *checker.Checker {
	t.Helper()
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil, checker.CheckOptions{Platform: target})
	c.Check()
	return c
}

func diagnosticSummary(c *checker.Checker) string {
	lines := make([]string, 0, len(c.Diagnostics()))
	for _, diagnostic := range c.Diagnostics() {
		lines = append(lines, fmt.Sprintf("%s %s: %s", diagnostic.Location().Start, diagnostic.Code, diagnostic.Message))
	}
	return strings.Join(lines, "\n")
}

func TestPlatformSelectsTheMatchingDeclaration(t *testing.T) {
	// Each variant's body has a type error only that variant's platform
	// reports, so the error shows which declaration was checked.
	source := `fn value() Int { "fallback" }

#platform(unix)
fn value() Int { "unix" }

#platform(windows)
fn value() Int { "windows" }

fn main() {
  let v = value()
}
`
	tests := []struct {
		target platform.Platform
		row    int
	}{
		{linuxAMD64, 4},
		{darwinARM64, 4},
		{windowsAMD64, 7},
		{plan9AMD64, 1},
	}
	for _, tt := range tests {
		t.Run(tt.target.String(), func(t *testing.T) {
			c := checkForPlatform(t, source, tt.target)
			diagnostics := c.Diagnostics()
			if len(diagnostics) != 1 || diagnostics[0].Location().Start.Row != tt.row || diagnostics[0].Code != checker.DiagnosticCodeTypeMismatch {
				t.Fatalf("diagnostics:\n%s\nwant one type mismatch on row %d", diagnosticSummary(c), tt.row)
			}
		})
	}
}

func TestPlatformDeclarationIsAbsentElsewhere(t *testing.T) {
	source := `#platform(windows)
fn console_mode() Int { 1 }

fn main() {
  let mode = console_mode()
}
`
	if c := checkForPlatform(t, source, windowsAMD64); len(c.Diagnostics()) != 0 {
		t.Fatalf("windows diagnostics:\n%s", diagnosticSummary(c))
	}
	c := checkForPlatform(t, source, linuxAMD64)
	if !c.HasErrors() || !strings.Contains(diagnosticSummary(c), "console_mode") {
		t.Fatalf("linux diagnostics:\n%s\nwant an undefined console_mode", diagnosticSummary(c))
	}
}

func TestPlatformSelectsImplBlocksAndInactiveImportsAreSkipped(t *testing.T) {
	source := `#platform(windows)
use go:example.com/does/not/exist

struct Point {}

impl Point {
  fn size() Int { 1 }
}

#platform(unix)
impl Point {
  fn size() Int { "unix" }
}
`
	if c := checkForPlatform(t, source, plan9AMD64); len(c.Diagnostics()) != 0 {
		t.Fatalf("plan9 diagnostics:\n%s", diagnosticSummary(c))
	}
	c := checkForPlatform(t, source, linuxAMD64)
	if len(c.Diagnostics()) != 1 || c.Diagnostics()[0].Location().Start.Row != 12 {
		t.Fatalf("linux diagnostics:\n%s\nwant one error in the unix impl", diagnosticSummary(c))
	}
}

func TestPlatformLoneDeclarationsAndAttributedTypes(t *testing.T) {
	source := `#platform(unix)
struct Fd { value: Int }

#platform(unix)
type Handle = Int

#platform(windows)
type Handle = Str

#platform(unix)
fn open() Fd { Fd{value: 1} }

#platform(unix)
test fn opens() Void!Str { Result::ok(()) }
`
	for _, target := range []platform.Platform{linuxAMD64, windowsAMD64} {
		if c := checkForPlatform(t, source, target); len(c.Diagnostics()) != 0 {
			t.Fatalf("%s diagnostics:\n%s", target, diagnosticSummary(c))
		}
	}
}

func TestPlatformAttributeErrors(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		code    checker.DiagnosticCode
		row     int
		message string
	}{
		{
			name:    "unknown platform",
			source:  "#platform(linx)\nfn f() {}\n",
			code:    checker.DiagnosticCodeUnknownPlatform,
			row:     1,
			message: `unknown platform "linx"`,
		},
		{
			name:    "string argument",
			source:  "#platform(\"linux\")\nfn f() {}\n",
			code:    checker.DiagnosticCodeInvalidAttributeArgument,
			row:     1,
			message: "#platform arguments must be platform names",
		},
		{
			name:    "named argument",
			source:  "#platform(os: linux)\nfn f() {}\n",
			code:    checker.DiagnosticCodeInvalidAttributeArgument,
			row:     1,
			message: "#platform arguments must be platform names",
		},
		{
			name:    "no arguments",
			source:  "#platform()\nfn f() {}\n",
			code:    checker.DiagnosticCodeInvalidAttributeArgument,
			row:     1,
			message: "#platform requires at least one platform name",
		},
		{
			name:    "unknown attribute",
			source:  "#json(name: \"f\")\nfn f() {}\n",
			code:    checker.DiagnosticCodeUnknownAttribute,
			row:     1,
			message: "Unknown attribute: #json",
		},
		{
			name:    "repeated attribute",
			source:  "#platform(unix)\n#platform(linux)\nfn f() {}\n",
			code:    checker.DiagnosticCodeDuplicateAttribute,
			row:     2,
			message: "Duplicate attribute: #platform",
		},
		{
			name:    "overlapping siblings",
			source:  "#platform(unix)\nfn f() {}\n\n#platform(darwin)\nfn f() {}\n",
			code:    checker.DiagnosticCodePlatformOverlap,
			row:     4,
			message: "#platform(darwin) overlaps #platform(unix) on darwin/amd64",
		},
		{
			name:    "different return type",
			source:  "fn f() Int { 1 }\n\n#platform(unix)\nfn f() Str { \"a\" }\n",
			code:    checker.DiagnosticCodePlatformSignatureMismatch,
			row:     4,
			message: "`fn f() Str` does not match `fn f() Int`",
		},
		{
			name:    "different parameter name",
			source:  "fn f(a: Int) {}\n\n#platform(unix)\nfn f(b: Int) {}\n",
			code:    checker.DiagnosticCodePlatformSignatureMismatch,
			row:     4,
			message: "`fn f(b: Int)` does not match `fn f(a: Int)`",
		},
		{
			name:    "different kind",
			source:  "let f: Int = 1\n\n#platform(unix)\nfn f() Int { 1 }\n",
			code:    checker.DiagnosticCodePlatformSignatureMismatch,
			row:     4,
			message: "`fn f() Int` does not match `let f: Int`",
		},
		{
			name:    "let without type",
			source:  "let size = 1\n\n#platform(unix)\nlet size: Int = 2\n",
			code:    checker.DiagnosticCodePlatformSignatureMismatch,
			row:     1,
			message: "Platform-specific `size` needs an explicit type",
		},
		{
			name:    "struct sibling",
			source:  "struct S {}\n\n#platform(unix)\nstruct S {}\n",
			code:    checker.DiagnosticCodePlatformSibling,
			row:     4,
			message: "Structs, enums, and traits cannot have platform-specific siblings",
		},
		{
			name:    "two fallbacks",
			source:  "fn f() {}\nfn f() {}\n\n#platform(unix)\nfn f() {}\n",
			code:    checker.DiagnosticCodePlatformSibling,
			row:     2,
			message: "`f` already has a declaration without #platform",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Attribute and sibling errors do not depend on the platform.
			for _, target := range []platform.Platform{linuxAMD64, windowsAMD64} {
				c := checkForPlatform(t, tt.source, target)
				found := false
				for _, diagnostic := range c.Diagnostics() {
					if diagnostic.Code == tt.code && diagnostic.Location().Start.Row == tt.row && strings.Contains(diagnostic.Message, tt.message) {
						found = true
					}
				}
				if !found {
					t.Fatalf("%s diagnostics:\n%s\nwant %s on row %d containing %q", target, diagnosticSummary(c), tt.code, tt.row, tt.message)
				}
			}
		})
	}
}

func TestPlatformAttributeOnStructFieldIsUnknown(t *testing.T) {
	c := checkForPlatform(t, "struct S {\n  #platform(unix)\n  fd: Int,\n}\n", linuxAMD64)
	if len(c.Diagnostics()) != 1 || c.Diagnostics()[0].Code != checker.DiagnosticCodeUnknownAttribute {
		t.Fatalf("diagnostics:\n%s", diagnosticSummary(c))
	}
}

func TestPlatformInactiveRegions(t *testing.T) {
	source := `use go:os
#platform(windows)
use go:golang.org/x/sys/windows

fn value() Int { 1 }

#platform(unix)
fn value() Int { 2 }

#platform(windows)
fn console() {}

#platform(linx)
fn broken() {}
`
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	analysis := checker.AnalyzePlatforms(result.Program)
	regions := analysis.InactiveRegions(linuxAMD64, platform.Preferred(linuxAMD64))
	type region struct {
		start, end int
		platform   platform.Platform
	}
	var got []region
	for _, r := range regions {
		got = append(got, region{r.Location.Start.Row, r.Location.End.Row, r.Platform})
	}
	want := []region{
		{2, 3, windowsAMD64},
		{5, 5, windowsAMD64},
		{10, 11, windowsAMD64},
		{13, 14, platform.Platform{}},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("regions = %v, want %v", got, want)
	}
}

func TestPlatformArchitectureSetsIntWidth(t *testing.T) {
	source := "let big: Int = 2147483648\n"
	if c := checkForPlatform(t, source, linuxAMD64); len(c.Diagnostics()) != 0 {
		t.Fatalf("linux/amd64 diagnostics:\n%s", diagnosticSummary(c))
	}
	c := checkForPlatform(t, source, platform.Platform{OS: "linux", Arch: "386"})
	if !c.HasErrors() || !strings.Contains(diagnosticSummary(c), "overflow") {
		t.Fatalf("linux/386 diagnostics:\n%s\nwant an Int overflow", diagnosticSummary(c))
	}
}
