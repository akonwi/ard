package fs

import (
	ardgo "github.com/akonwi/ard/go"
)

type Direntry struct {
	IsFile bool
	Name   string
}

func Exists(path string) bool {
	result, err := ardgo.CallExtern("FS_Exists", path)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[bool](result)
}

func IsFile(path string) bool {
	result, err := ardgo.CallExtern("FS_IsFile", path)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[bool](result)
}

func IsDir(path string) bool {
	result, err := ardgo.CallExtern("FS_IsDir", path)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[bool](result)
}

func CreateFile(path string) ardgo.Result[bool, string] {
	result, err := ardgo.CallExtern("FS_CreateFile", path)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[bool, string]](result)
}

func Write(path string, content string) ardgo.Result[struct{}, string] {
	result, err := ardgo.CallExtern("FS_WriteFile", path, content)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[struct{}, string]](result)
}

func Append(path string, content string) ardgo.Result[struct{}, string] {
	result, err := ardgo.CallExtern("FS_AppendFile", path, content)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[struct{}, string]](result)
}

func Read(path string) ardgo.Result[string, string] {
	result, err := ardgo.CallExtern("FS_ReadFile", path)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[string, string]](result)
}

func Delete(path string) ardgo.Result[struct{}, string] {
	result, err := ardgo.CallExtern("FS_DeleteFile", path)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[struct{}, string]](result)
}

func Copy(from string, to string) ardgo.Result[struct{}, string] {
	result, err := ardgo.CallExtern("FS_Copy", from, to)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[struct{}, string]](result)
}

func Rename(from string, to string) ardgo.Result[struct{}, string] {
	result, err := ardgo.CallExtern("FS_Rename", from, to)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[struct{}, string]](result)
}

func Cwd() ardgo.Result[string, string] {
	result, err := ardgo.CallExtern("FS_Cwd")
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[string, string]](result)
}

func Abs(path string) ardgo.Result[string, string] {
	result, err := ardgo.CallExtern("FS_Abs", path)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[string, string]](result)
}

func CreateDir(path string) ardgo.Result[struct{}, string] {
	result, err := ardgo.CallExtern("FS_CreateDir", path)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[struct{}, string]](result)
}

func DeleteDir(path string) ardgo.Result[struct{}, string] {
	result, err := ardgo.CallExtern("FS_DeleteDir", path)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[struct{}, string]](result)
}

func listDirMap(path string) ardgo.Result[map[string]bool, string] {
	result, err := ardgo.CallExtern("FS_ListDir", path)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[map[string]bool, string]](result)
}

func ListDir(path string) ardgo.Result[[]Direntry, string] {
	__ardTry0 := listDirMap(path)
	if __ardTry0.IsErr() {
		return ardgo.Err[[]Direntry, string](__ardTry0.UnwrapErr())
	}
	entriesMap := __ardTry0.UnwrapOk()
	entries := append([]Direntry(nil), []Direntry{}...)
	__ardMap1 := entriesMap
	for _, name := range ardgo.MapKeys(__ardMap1) {
		isFile := __ardMap1[name]
		_ = func() []Direntry { entries = append(entries, Direntry{IsFile: isFile, Name: name}); return entries }()
	}
	return ardgo.Ok[[]Direntry, string](entries)
}
