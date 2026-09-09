package air

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	parsed := parse.Parse([]byte("use ard/embed\nlet page = embed::text(\"asset.txt\")\nlet raw = embed::bytes(\"asset.txt\")\nlet files = embed::fs([\"asset.txt\"])\n"), mainPath)
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
	if len(program.Globals) != 3 {
		t.Fatalf("globals = %d, want 3", len(program.Globals))
	}
	text := program.Globals[0].Initializer.Value
	bytes := program.Globals[1].Initializer.Value
	if text.Kind != ExprEmbeddedText || bytes.Kind != ExprEmbeddedBytes {
		t.Fatalf("embedded kinds = %d, %d", text.Kind, bytes.Kind)
	}
	if text.EmbeddedBlobPayload().Blob != 0 || bytes.EmbeddedBlobPayload().Blob != 0 {
		t.Fatalf("embedded blob refs = %d, %d", text.EmbeddedBlobPayload().Blob, bytes.EmbeddedBlobPayload().Blob)
	}
	files := program.Globals[2].Initializer.Value
	if files.Kind != ExprMakeEmbeddedFS || files.EmbeddedSetPayload().Set != 0 {
		t.Fatalf("embedded filesystem expression = %#v", files)
	}
	if len(program.EmbeddedSets) != 1 || len(program.EmbeddedSets[0].Entries) != 1 || program.EmbeddedSets[0].Entries[0].Path != "asset.txt" {
		t.Fatalf("embedded sets = %#v", program.EmbeddedSets)
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
	if len(roundTrip.EmbeddedSets) != 1 || roundTrip.EmbeddedSets[0].Digest != program.EmbeddedSets[0].Digest {
		t.Fatalf("round-trip embedded sets = %#v", roundTrip.EmbeddedSets)
	}

	malformed := *program
	duplicate := program.EmbeddedBlobs[0]
	duplicate.ID = 1
	malformed.EmbeddedBlobs = append(append([]EmbeddedBlob(nil), program.EmbeddedBlobs...), duplicate)
	if err := Validate(&malformed); err == nil || !strings.Contains(err.Error(), "duplicates digest") {
		t.Fatalf("duplicate embedded blob validation error = %v", err)
	}

	invalidPath := *program
	invalidPath.EmbeddedSets = append([]EmbeddedSet(nil), program.EmbeddedSets...)
	invalidPath.EmbeddedSets[0].Entries = append([]EmbeddedEntry(nil), program.EmbeddedSets[0].Entries...)
	invalidPath.EmbeddedSets[0].Entries[0].Path = "../escape"
	if err := Validate(&invalidPath); err == nil || !strings.Contains(err.Error(), "invalid path") {
		t.Fatalf("invalid embedded set path validation error = %v", err)
	}
}

func embeddedSetTestDigest(set EmbeddedSet, blobs []EmbeddedBlob) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(set.OwnerPackageIdentity))
	_, _ = hash.Write([]byte{0})
	for _, entry := range set.Entries {
		_, _ = hash.Write([]byte(entry.Path))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(blobs[entry.Blob].Digest))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func TestValidateEmbeddedResourceLimits(t *testing.T) {
	blobData := make([]byte, checker.MaxEmbeddedFileBytes)
	blobSum := sha256.Sum256(blobData)
	blob := EmbeddedBlob{ID: 0, Data: blobData, Digest: hex.EncodeToString(blobSum[:])}

	oversizedSet := EmbeddedSet{ID: 0, OwnerPackageIdentity: "app"}
	for index := 0; index < 5; index++ {
		oversizedSet.Entries = append(oversizedSet.Entries, EmbeddedEntry{Path: fmt.Sprintf("%05d.bin", index), Blob: 0})
	}
	oversizedSet.Digest = embeddedSetTestDigest(oversizedSet, []EmbeddedBlob{blob})
	if err := Validate(&Program{EmbeddedBlobs: []EmbeddedBlob{blob}, EmbeddedSets: []EmbeddedSet{oversizedSet}}); err == nil || !strings.Contains(err.Error(), "embedded set") {
		t.Fatalf("oversized embedded set validation error = %v", err)
	}

	zeroSum := sha256.Sum256(nil)
	zeroBlob := EmbeddedBlob{ID: 0, Digest: hex.EncodeToString(zeroSum[:])}
	tooMany := EmbeddedSet{ID: 0, OwnerPackageIdentity: "app"}
	for index := 0; index <= checker.MaxEmbeddedProgramFileCount; index++ {
		tooMany.Entries = append(tooMany.Entries, EmbeddedEntry{Path: fmt.Sprintf("%05d.txt", index), Blob: 0})
	}
	tooMany.Digest = embeddedSetTestDigest(tooMany, []EmbeddedBlob{zeroBlob})
	if err := Validate(&Program{EmbeddedBlobs: []EmbeddedBlob{zeroBlob}, EmbeddedSets: []EmbeddedSet{tooMany}}); err == nil || !strings.Contains(err.Error(), "program limit") {
		t.Fatalf("embedded file-count validation error = %v", err)
	}
}
