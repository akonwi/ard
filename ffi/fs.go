package ffi

import (
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
