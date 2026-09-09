package gotarget

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func embeddedTestProgram(t *testing.T) (*air.Program, *checker.ProjectInfo, []byte) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	content := []byte{'h', 'e', 'l', 'l', 'o', 0, 0xff}
	if err := os.WriteFile(filepath.Join(root, "asset.bin"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	parsed := parse.Parse([]byte("use ard/embed\nlet raw = embed::bytes(\"asset.bin\")\nfn main() Int { raw.size() }\n"), mainPath)
	if len(parsed.Errors) > 0 {
		t.Fatalf("parse errors: %v", parsed.Errors)
	}
	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	checked := checker.New(mainPath, parsed.Program, resolver)
	checked.Check()
	if checked.HasErrors() {
		t.Fatalf("checker diagnostics: %#v", checked.Diagnostics())
	}
	program, err := air.Lower(checked.Module())
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	return program, resolver.GetProjectInfo(), content
}

func TestGenerateSourcesUsesEmbeddedResourceAccessors(t *testing.T) {
	program, project, _ := embeddedTestProgram(t)
	sources, err := GenerateSources(program, Options{PackageName: "main", ProjectInfo: project})
	if err != nil {
		t.Fatalf("GenerateSources: %v", err)
	}
	resourceSource := string(sources["internal/ardembed/embed.go"])
	if !strings.Contains(resourceSource, "//go:embed data/") || !strings.Contains(resourceSource, "func Bytes") {
		t.Fatalf("generated resource source:\n%s", resourceSource)
	}
	foundAccessor := false
	for name, source := range sources {
		if name == "internal/ardembed/embed.go" {
			continue
		}
		if strings.Contains(string(source), "ardembed.Bytes") {
			foundAccessor = true
			break
		}
	}
	if !foundAccessor {
		t.Fatalf("generated sources do not call embedded bytes accessor: %#v", sources)
	}
}

func TestEmbeddedExactFilesRunFromCapturedBytesAndReturnFreshLists(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binaryContent := []byte{'h', 'e', 'l', 'l', 'o', 0, 0xff}
	binaryPath := filepath.Join(root, "asset.bin")
	textPath := filepath.Join(root, "page.txt")
	if err := os.WriteFile(binaryPath, binaryContent, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(textPath, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	source := `use ard/embed

fn embedded_bytes() [Byte] { embed::bytes("asset.bin") }

fn main() {
  if embed::text("page.txt") != "hello\n" { panic("bad embedded text") }
  let first = mut embedded_bytes()
  first.set(0, Byte::from(0))
  let second = embedded_bytes()
  if second.at(0).or(Byte::from(0)) != 104 { panic("embedded bytes shared mutable storage") }
  if second.size() != 7 { panic("bad embedded byte length") }
}
`
	parsed := parse.Parse([]byte(source), mainPath)
	if len(parsed.Errors) > 0 {
		t.Fatalf("parse errors: %v", parsed.Errors)
	}
	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	checked := checker.New(mainPath, parsed.Program, resolver)
	checked.Check()
	if checked.HasErrors() {
		t.Fatalf("checker diagnostics: %#v", checked.Diagnostics())
	}
	program, err := air.Lower(checked.Module())
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(textPath); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(t.TempDir(), "embedded-program")
	built, err := BuildProgram(program, output, resolver.GetProjectInfo())
	if err != nil {
		t.Fatalf("BuildProgram: %v", err)
	}
	if output, err := exec.Command(built).CombinedOutput(); err != nil {
		t.Fatalf("embedded program failed: %v\n%s", err, output)
	}
}

func TestWriteProgramStagesEmbeddedBlobs(t *testing.T) {
	program, project, content := embeddedTestProgram(t)
	out := t.TempDir()
	if err := writeProgram(out, program, Options{PackageName: "main", ProjectInfo: project}); err != nil {
		t.Fatalf("writeProgram: %v", err)
	}
	if len(program.EmbeddedBlobs) != 1 {
		t.Fatalf("embedded blobs = %d", len(program.EmbeddedBlobs))
	}
	path := filepath.Join(out, "internal", "ardembed", "data", program.EmbeddedBlobs[0].Digest)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read staged blob: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("staged blob = %v, want %v", got, content)
	}
	if err := buildGeneratedProgram(out, filepath.Join(out, "embedded-test")); err != nil {
		t.Fatalf("build generated embedded program: %v", err)
	}
}
