package ffi

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

func FS_Exists(args []*runtime.Object, _ checker.Type) *runtime.Object {
	path := args[0].Raw().(string)
	if _, err := os.Stat(path); err == nil {
		return runtime.MakeBool(true)
	}
	return runtime.MakeBool(false)
}

func FS_CreateFile(args []*runtime.Object, _ checker.Type) *runtime.Object {
	path := args[0].Raw().(string)
	if file, err := os.Create(path); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	} else {
		file.Close()
	}
	return runtime.MakeOk(runtime.MakeBool(true))
}

func FS_WriteFile(args []*runtime.Object, _ checker.Type) *runtime.Object {
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

func FS_AppendFile(args []*runtime.Object, _ checker.Type) *runtime.Object {
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

func FS_ReadFile(args []*runtime.Object, _ checker.Type) *runtime.Object {
	path := args[0].Raw().(string)
	content, err := os.ReadFile(path)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.MakeStr(string(content)))
}

func FS_DeleteFile(args []*runtime.Object, _ checker.Type) *runtime.Object {
	path := args[0].Raw().(string)
	if err := os.Remove(path); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.Void())
}

func FS_IsFile(args []*runtime.Object, _ checker.Type) *runtime.Object {
	path := args[0].Raw().(string)
	info, err := os.Stat(path)
	if err != nil {
		return runtime.MakeBool(false)
	}
	return runtime.MakeBool(!info.IsDir())
}

func FS_IsDir(args []*runtime.Object, _ checker.Type) *runtime.Object {
	path := args[0].Raw().(string)
	info, err := os.Stat(path)
	if err != nil {
		return runtime.MakeBool(false)
	}
	return runtime.MakeBool(info.IsDir())
}

func FS_Copy(args []*runtime.Object, _ checker.Type) *runtime.Object {
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

func FS_Rename(args []*runtime.Object, _ checker.Type) *runtime.Object {
	from := args[0].Raw().(string)
	to := args[1].Raw().(string)
	if err := os.Rename(from, to); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.Void())
}

func FS_FileSize(args []*runtime.Object, _ checker.Type) *runtime.Object {
	path := args[0].Raw().(string)
	info, err := os.Stat(path)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.MakeInt(int(info.Size())))
}

func FS_Cwd(_ []*runtime.Object, _ checker.Type) *runtime.Object {
	dir, err := os.Getwd()
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.MakeStr(dir))
}

func FS_Abs(args []*runtime.Object, _ checker.Type) *runtime.Object {
	path := args[0].Raw().(string)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.MakeStr(absPath))
}

func FS_CreateDir(args []*runtime.Object, _ checker.Type) *runtime.Object {
	path := args[0].Raw().(string)
	if err := os.MkdirAll(path, 0755); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.Void())
}

func FS_DeleteDir(args []*runtime.Object, _ checker.Type) *runtime.Object {
	path := args[0].Raw().(string)
	if err := os.RemoveAll(path); err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.Void())
}

func FS_ListDir(args []*runtime.Object, outType checker.Type) *runtime.Object {
	path := args[0].Raw().(string)
	entries, err := os.ReadDir(path)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	// Extract the DirEntry struct type from the result type
	resultType, ok := outType.(*checker.Result)
	if !ok {
		panic(fmt.Sprintf("Unexpected return type of list_dir(): %s", outType))
	}
	listType, ok := resultType.Val().(*checker.List)
	if !ok {
		panic(fmt.Sprintf("Unexpected return Result::ok type of list_dir(): %s", resultType.Val()))
	}
	dirEntryType := listType.Of()

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
