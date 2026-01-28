package ffi

import (
	"fmt"
	"os"

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
