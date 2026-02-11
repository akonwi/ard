package vm

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestBytecodeFS(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ard_bytecode_fs_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	filePath := filepath.Join(tmpDir, "fake.file")

	runBytecodeTests(t, []vmTestCase{
		{name: "fs::exists false for missing path", input: `
			use ard/fs
			fs::exists("path/to/file")
		`, want: false},
		{name: "fs::exists true for compiler main.go", input: `
			use ard/fs
			fs::exists("../../main.go")
		`, want: true},
		{name: "fs::create_file", input: fmt.Sprintf(`
			use ard/fs
			fs::create_file(%q).expect("Failed to create file")
		`, filePath), want: true},
		{name: "fs::write", input: fmt.Sprintf(`
			use ard/fs
			fs::write(%q, "content")
		`, filePath), want: nil},
		{name: "fs::append", input: fmt.Sprintf(`
			use ard/fs
			fs::append(%q, "-appended")
		`, filePath), want: nil},
		{name: "fs::read", input: fmt.Sprintf(`
			use ard/fs
			match fs::read(%q) {
				ok(s) => s,
				err => err,
			}
		`, filePath), want: "content-appended"},
		{name: "fs::delete", input: fmt.Sprintf(`
			use ard/fs
			fs::delete(%q)
		`, filePath), want: nil},
	})
}
