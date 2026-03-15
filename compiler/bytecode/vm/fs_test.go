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

func TestBytecodeFS_FileSize(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ard_bytecode_fs_size_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	filePath := filepath.Join(tmpDir, "sized.txt")

	runBytecodeTests(t, []vmTestCase{
		{name: "setup: create file", input: fmt.Sprintf(`
			use ard/fs
			fs::write(%q, "hello")
		`, filePath), want: nil},
		{name: "fs::file_size", input: fmt.Sprintf(`
			use ard/fs
			fs::file_size(%q).expect("stat failed")
		`, filePath), want: 5},
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
