package air

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestLowerEmbeddedExactFilesIntoBlobReferences(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	content := []byte("shared embedded contents\n")
	if err := os.WriteFile(filepath.Join(root, "asset.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	parsed := parse.Parse([]byte("use ard/embed\nlet page = embed::text(\"asset.txt\")\nlet raw = embed::bytes(\"asset.txt\")\n"), mainPath)
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
	if err := os.Remove(filepath.Join(root, "asset.txt")); err != nil {
		t.Fatal(err)
	}

	program, err := Lower(checked.Module())
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	if len(program.EmbeddedBlobs) != 1 {
		t.Fatalf("embedded blobs = %d, want 1", len(program.EmbeddedBlobs))
	}
	if string(program.EmbeddedBlobs[0].Data) != string(content) {
		t.Fatalf("blob data = %q", program.EmbeddedBlobs[0].Data)
	}
	if len(program.Globals) != 2 {
		t.Fatalf("globals = %d, want 2", len(program.Globals))
	}
	text := program.Globals[0].Initializer.Value
	bytes := program.Globals[1].Initializer.Value
	if text.Kind != ExprEmbeddedText || bytes.Kind != ExprEmbeddedBytes {
		t.Fatalf("embedded kinds = %d, %d", text.Kind, bytes.Kind)
	}
	if text.EmbeddedBlobPayload().Blob != 0 || bytes.EmbeddedBlobPayload().Blob != 0 {
		t.Fatalf("embedded blob refs = %d, %d", text.EmbeddedBlobPayload().Blob, bytes.EmbeddedBlobPayload().Blob)
	}

	encoded, err := SerializeProgram(program)
	if err != nil {
		t.Fatalf("SerializeProgram: %v", err)
	}
	roundTrip, err := DeserializeProgram(encoded)
	if err != nil {
		t.Fatalf("DeserializeProgram: %v", err)
	}
	if len(roundTrip.EmbeddedBlobs) != 1 || string(roundTrip.EmbeddedBlobs[0].Data) != string(content) {
		t.Fatalf("round-trip embedded blobs = %#v", roundTrip.EmbeddedBlobs)
	}

	malformed := *program
	duplicate := program.EmbeddedBlobs[0]
	duplicate.ID = 1
	malformed.EmbeddedBlobs = append(append([]EmbeddedBlob(nil), program.EmbeddedBlobs...), duplicate)
	if err := Validate(&malformed); err == nil || !strings.Contains(err.Error(), "duplicates digest") {
		t.Fatalf("duplicate embedded blob validation error = %v", err)
	}
}
