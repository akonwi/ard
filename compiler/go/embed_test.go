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

func TestEmbeddedFSReadsFilesAndSubdirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	assetPath := filepath.Join(root, "public", "index.html")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetPath, []byte("<h1>Hello</h1>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public", "binary.bin"), []byte{0xff, 1}, 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	source := `use ard/embed
use go:io/fs as gofs

let assets = embed::fs(["public"])

fn main() {
  let text = assets.read_text("public/index.html").expect("read text")
  if text != "<h1>Hello</h1>\n" { panic("bad text") }
  let bytes = assets.read_file("public/index.html").expect("read bytes")
  if bytes.size() != 15 { panic("bad bytes") }
  let changed = mut assets.read_file("public/index.html").expect("mutable bytes")
  changed.set(0, Byte::from(0))
  if assets.read_file("public/index.html").expect("fresh bytes").at(0).or(Byte::from(0)) != 60 { panic("read bytes shared storage") }
  if gofs::ReadFile(assets, "public/index.html").expect("direct io/fs").size() != 15 { panic("bad io/fs bridge") }
  if not assets.read_file("missing").is_err() { panic("missing file succeeded") }
  if not assets.read_text("public/binary.bin").is_err() { panic("invalid UTF-8 succeeded") }
  if not assets.sub("public/index.html").is_err() { panic("file sub succeeded") }
  let entries = assets.read_dir("public").expect("read dir")
  if entries.size() != 2 { panic("bad directory size") }
  let entry = entries.at(1).expect("directory entry")
  if entry.name != "index.html" or entry.is_dir { panic("bad directory entry") }
  let info = assets.stat("public/index.html").expect("stat")
  if info.name != "index.html" or info.is_dir or info.size.or(0) != 15 { panic("bad file info") }
  if assets.stat(".").expect("root stat").name != "." { panic("bad root name") }
  let public = assets.sub("public").expect("sub")
  if public.stat(".").expect("sub root stat").name != "." { panic("bad sub root name") }
  if public.read_text("index.html").expect("sub read") != text { panic("bad sub") }
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
	if err := os.RemoveAll(filepath.Join(root, "public")); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "embedded-fs-program")
	built, err := BuildProgram(program, output, resolver.GetProjectInfo())
	if err != nil {
		t.Fatalf("BuildProgram: %v", err)
	}
	if output, err := exec.Command(built).CombinedOutput(); err != nil {
		t.Fatalf("embedded FS program failed: %v\n%s", err, output)
	}
}
