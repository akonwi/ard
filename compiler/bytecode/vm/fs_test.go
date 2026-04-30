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
	dirPath := filepath.Join(tmpDir, "a", "b", "c")

	runBytecodeTests(t, []vmTestCase{
		{name: "fs::exists false for missing path", input: `
			use ard/fs
			fs::exists("path/to/file")
		`, want: false},
		{name: "fs::exists true for compiler main.go", input: `
			use ard/fs
			fs::exists("../../main.go")
		`, want: true},
		{name: "fs::create_dir", input: fmt.Sprintf(`
			use ard/fs
			fs::create_dir(%q)
		`, dirPath), want: nil},
		{name: "fs::create_dir created nested dirs", input: fmt.Sprintf(`
			use ard/fs
			fs::is_dir(%q)
		`, dirPath), want: true},
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

func TestBytecodeFS_Copy(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ard_bytecode_fs_copy_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	runBytecodeTests(t, []vmTestCase{
		{name: "setup: create source file", input: fmt.Sprintf(`
			use ard/fs
			fs::write(%q, "copy me")
		`, srcPath), want: nil},
		{name: "fs::copy", input: fmt.Sprintf(`
			use ard/fs
			fs::copy(%q, %q)
		`, srcPath, dstPath), want: nil},
		{name: "fs::copy preserves content", input: fmt.Sprintf(`
			use ard/fs
			fs::read(%q).expect("read failed")
		`, dstPath), want: "copy me"},
		{name: "fs::copy source still exists", input: fmt.Sprintf(`
			use ard/fs
			fs::exists(%q)
		`, srcPath), want: true},
	})
}

func TestBytecodeFS_Rename(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ard_bytecode_fs_rename_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	srcPath := filepath.Join(tmpDir, "before.txt")
	dstPath := filepath.Join(tmpDir, "after.txt")

	runBytecodeTests(t, []vmTestCase{
		{name: "setup: create file", input: fmt.Sprintf(`
			use ard/fs
			fs::write(%q, "move me")
		`, srcPath), want: nil},
		{name: "fs::rename", input: fmt.Sprintf(`
			use ard/fs
			fs::rename(%q, %q)
		`, srcPath, dstPath), want: nil},
		{name: "fs::rename moved content", input: fmt.Sprintf(`
			use ard/fs
			fs::read(%q).expect("read failed")
		`, dstPath), want: "move me"},
		{name: "fs::rename removed source", input: fmt.Sprintf(`
			use ard/fs
			fs::exists(%q)
		`, srcPath), want: false},
	})
}

func TestBytecodeFS_DeleteDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ard_bytecode_fs_deldir_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	dirPath := filepath.Join(tmpDir, "removeme", "nested")
	filePath := filepath.Join(dirPath, "file.txt")

	runBytecodeTests(t, []vmTestCase{
		{name: "setup: create nested dir", input: fmt.Sprintf(`
			use ard/fs
			fs::create_dir(%q)
		`, dirPath), want: nil},
		{name: "setup: create file in dir", input: fmt.Sprintf(`
			use ard/fs
			fs::write(%q, "data")
		`, filePath), want: nil},
		{name: "fs::delete_dir", input: fmt.Sprintf(`
			use ard/fs
			fs::delete_dir(%q)
		`, filepath.Join(tmpDir, "removeme")), want: nil},
		{name: "fs::delete_dir removed everything", input: fmt.Sprintf(`
			use ard/fs
			fs::exists(%q)
		`, filepath.Join(tmpDir, "removeme")), want: false},
	})
}

func TestBytecodeFS_CwdAndAbs(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	runBytecodeTests(t, []vmTestCase{
		{name: "fs::cwd", input: `
			use ard/fs
			fs::cwd().expect("cwd failed")
		`, want: cwd},
		{name: "fs::abs resolves relative path", input: fmt.Sprintf(`
			use ard/fs
			fs::abs(".").expect("abs failed")
		`), want: cwd},
	})
}

func TestBytecodeFS_ListDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ard_bytecode_fs_listdir_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	if err := os.WriteFile(filepath.Join(tmpDir, "note.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	runBytecodeTests(t, []vmTestCase{
		{name: "fs::list_dir returns entries", input: fmt.Sprintf(`
			use ard/fs
			let entries = fs::list_dir(%q).expect("list dir failed")
			mut saw_note = false
			mut saw_nested = false
			for entry in entries {
				if entry.name == "note.txt" {
					saw_note = true
				}
				if entry.name == "nested" {
					saw_nested = true
				}
			}
			saw_note and saw_nested
		`, tmpDir), want: true},
		{name: "fs::list_dir preserves file flag", input: fmt.Sprintf(`
			use ard/fs
			let entries = fs::list_dir(%q).expect("list dir failed")
			mut note_is_file = false
			mut nested_is_file = true
			for entry in entries {
				if entry.name == "note.txt" {
					note_is_file = entry.is_file
				}
				if entry.name == "nested" {
					nested_is_file = entry.is_file
				}
			}
			note_is_file and not nested_is_file
		`, tmpDir), want: true},
	})
}
