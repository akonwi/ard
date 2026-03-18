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

func FS_Exists(args []*runtime.Object) *runtime.Object {
	path := args[0].Raw().(string)
	if _, err := os.Stat(path); err == nil {
		return runtime.MakeBool(true)
	}
	return runtime.MakeBool(false)
}

func FS_CreateFile(args []*runtime.Object) *runtime.Object {
	path := args[0].Raw().(string)
	if file, err := os.Create(path); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	} else {
		file.Close()
	}
	return runtime.MakeOk(runtime.MakeBool(true))
}

func FS_WriteFile(args []*runtime.Object) *runtime.Object {
	path := args[0].AsString()
	content := args[1].AsString()
	/* file permissions:
	- `6` (owner): read (4) + write (2) = 6
	- `4` (group): read only
	- `4` (others): read only
	*/
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.Void())
}

func FS_AppendFile(args []*runtime.Object) *runtime.Object {
	path := args[0].Raw().(string)
	content := args[1].Raw().(string)
	if file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	} else {
		if _, err := file.WriteString(content); err != nil {
			file.Close()
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}
		file.Close()
	}
	return runtime.MakeOk(runtime.Void())
}

func FS_ReadFile(args []*runtime.Object) *runtime.Object {
	path := args[0].Raw().(string)
	content, err := os.ReadFile(path)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.MakeStr(string(content)))
}

func FS_DeleteFile(args []*runtime.Object) *runtime.Object {
	path := args[0].Raw().(string)
	if err := os.Remove(path); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.Void())
}

func FS_IsFile(args []*runtime.Object) *runtime.Object {
	path := args[0].Raw().(string)
	info, err := os.Stat(path)
	if err != nil {
		return runtime.MakeBool(false)
	}
	return runtime.MakeBool(!info.IsDir())
}

func FS_IsDir(args []*runtime.Object) *runtime.Object {
	path := args[0].Raw().(string)
	info, err := os.Stat(path)
	if err != nil {
		return runtime.MakeBool(false)
	}
	return runtime.MakeBool(info.IsDir())
}

func FS_Copy(args []*runtime.Object) *runtime.Object {
	from := args[0].Raw().(string)
	to := args[1].Raw().(string)

	src, err := os.Open(from)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	defer src.Close()

	dst, err := os.Create(to)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.Void())
}

func FS_Rename(args []*runtime.Object) *runtime.Object {
	from := args[0].Raw().(string)
	to := args[1].Raw().(string)
	if err := os.Rename(from, to); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.Void())
}

func FS_Cwd(_ []*runtime.Object) *runtime.Object {
	dir, err := os.Getwd()
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.MakeStr(dir))
}

func FS_Abs(args []*runtime.Object) *runtime.Object {
	path := args[0].Raw().(string)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.MakeStr(absPath))
}

func FS_CreateDir(args []*runtime.Object) *runtime.Object {
	path := args[0].Raw().(string)
	if err := os.MkdirAll(path, 0755); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.Void())
}

func FS_DeleteDir(args []*runtime.Object) *runtime.Object {
	path := args[0].Raw().(string)
	if err := os.RemoveAll(path); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.Void())
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
