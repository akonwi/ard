package ffi

import (
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

var (
	fsDirEntryType     checker.Type
	fsDirEntryTypeOnce sync.Once
)

func getFSDirEntryType() checker.Type {
	fsDirEntryTypeOnce.Do(func() {
		mod, ok := checker.FindEmbeddedModule("ard/fs")
		if !ok {
			panic("failed to load ard/fs embedded module")
		}
		sym := mod.Get("DirEntry")
		if sym.Type == nil {
			panic("DirEntry type not found in ard/fs module")
		}
		fsDirEntryType = sym.Type
	})
	return fsDirEntryType
}

func FS_Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func FS_CreateFile(path string) (bool, error) {
	file, err := os.Create(path)
	if err != nil {
		return false, err
	}
	file.Close()
	return true, nil
}

func FS_WriteFile(path, content string) error {
	/* file permissions:
	- `6` (owner): read (4) + write (2) = 6
	- `4` (group): read only
	- `4` (others): read only
	*/
	return os.WriteFile(path, []byte(content), 0644)
}

func FS_AppendFile(path, content string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(content); err != nil {
		file.Close()
		return err
	}
	file.Close()
	return nil
}

func FS_ReadFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func FS_DeleteFile(path string) error {
	return os.Remove(path)
}

func FS_IsFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func FS_IsDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func FS_Copy(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(to)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func FS_Rename(from, to string) error {
	return os.Rename(from, to)
}

func FS_Cwd() (string, error) {
	return os.Getwd()
}

func FS_Abs(path string) (string, error) {
	return filepath.Abs(path)
}

func FS_CreateDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func FS_DeleteDir(path string) error {
	return os.RemoveAll(path)
}

func FS_ListDir(args []*runtime.Object) *runtime.Object {
	path := args[0].Raw().(string)
	entries, err := os.ReadDir(path)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	dirEntryType := getFSDirEntryType()

	var dirEntries []*runtime.Object
	for _, entry := range entries {
		dirEntryObj := runtime.MakeStruct(dirEntryType, map[string]*runtime.Object{
			"name":    runtime.MakeStr(entry.Name()),
			"is_file": runtime.MakeBool(!entry.IsDir()),
		})
		dirEntries = append(dirEntries, dirEntryObj)
	}

	return runtime.MakeOk(runtime.MakeList(dirEntryType, dirEntries...))
}
