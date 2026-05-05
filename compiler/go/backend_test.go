package gotarget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestGenerateSourcesFormatsSimpleProgram(t *testing.T) {
	program := lowerSource(t, `
		fn add(a: Int, b: Int) Int {
			a + b
		}

		fn main() Int {
			add(1, 2)
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source, ok := sources["test.go"]
	if !ok {
		t.Fatalf("generated sources missing test.go: %#v", mapsKeys(sources))
	}
	got := string(source)
	if !strings.Contains(got, "package main") {
		t.Fatalf("generated source missing package declaration:\n%s", got)
	}
	if !strings.Contains(got, "func test_ard__add(a int, b int) int") {
		t.Fatalf("generated source missing lowered add function:\n%s", got)
	}
	if !strings.Contains(got, "return a + b") {
		t.Fatalf("generated source missing arithmetic return:\n%s", got)
	}
	if !strings.Contains(got, "func main()") {
		t.Fatalf("generated source missing Go main wrapper:\n%s", got)
	}
}

func TestRunProgramExecutesSimpleMain(t *testing.T) {
	program := lowerSource(t, `
		fn main() Void {
			()
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestGenerateSourcesSupportsStructsAndEnums(t *testing.T) {
	program := lowerSource(t, `
		enum Direction {
			Up, Down
		}

		struct User {
			name: Str,
			age: Int,
		}

		fn direction() Direction {
			Direction::Down
		}

		fn next_age() Int {
			let user = User{name: "Ada", age: 41}
			user.age + 1
		}

		fn main() Int {
			next_age()
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	combined := ""
	for _, source := range sources {
		combined += string(source)
	}
	if !strings.Contains(combined, "type type__Direction int") {
		t.Fatalf("generated source missing enum type:\n%s", combined)
	}
	if !strings.Contains(combined, "type__Direction__Down") {
		t.Fatalf("generated source missing enum constants:\n%s", combined)
	}
	if !strings.Contains(combined, "type type__User struct") {
		t.Fatalf("generated source missing struct type:\n%s", combined)
	}
	if !strings.Contains(combined, "type__User{age: 41, name: \"Ada\"}") {
		t.Fatalf("generated source missing struct literal lowering:\n%s", combined)
	}
	if !strings.Contains(combined, ".age + 1") {
		t.Fatalf("generated source missing field access lowering:\n%s", combined)
	}
}

func TestGenerateSourcesSupportsResultExpectAndStringPredicates(t *testing.T) {
	program := lowerSource(t, `
		use ard/io

		fn main() Bool {
			let line = io::read_line().expect("no line")
			line.is_empty()
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	combined := ""
	for _, source := range sources {
		combined += string(source)
	}
	if !strings.Contains(combined, "type ardResult") {
		t.Fatalf("generated source missing result helper:\n%s", combined)
	}
	if !strings.Contains(combined, "ardReadLine()") {
		t.Fatalf("generated source missing ReadLine lowering:\n%s", combined)
	}
	if !strings.Contains(combined, "panic(\"no line\"") {
		t.Fatalf("generated source missing Result.expect lowering:\n%s", combined)
	}
	if !strings.Contains(combined, "len(line") {
		t.Fatalf("generated source missing is_empty lowering:\n%s", combined)
	}
}

func TestGenerateSourcesSupportsIfAndWhile(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			mut count = 0
			while count < 3 {
				count = count + 1
			}
			if count == 3 {
				count
			} else {
				0
			}
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "< 3 {") {
		t.Fatalf("generated source missing while lowering:\n%s", source)
	}
	if !strings.Contains(source, "== 3 {") {
		t.Fatalf("generated source missing if lowering:\n%s", source)
	}
	if !strings.Contains(source, "var _tmp_0 int") {
		t.Fatalf("generated source missing expression temp lowering:\n%s", source)
	}
}

func TestBuildProgramProducesBinary(t *testing.T) {
	program := lowerSource(t, `
		fn main() Void {
			()
		}
	`)

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "ard-bin")
	builtPath, err := BuildProgram(program, outputPath)
	if err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
	if builtPath != outputPath {
		t.Fatalf("built path = %q, want %q", builtPath, outputPath)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("built binary stat error = %v", err)
	}
}

func mapsKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func lowerSource(t *testing.T, input string) *air.Program {
	t.Helper()
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	return program
}
