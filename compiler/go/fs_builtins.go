package ardgo

import "github.com/akonwi/ard/ffi"

func okVoidResult() Result[struct{}, string] {
	return Ok[struct{}, string](struct{}{})
}

func errStringResult(err error) Result[struct{}, string] {
	return Err[struct{}, string](err.Error())
}

func builtinFSCreateFile(path string) Result[bool, string] {
	value, err := ffi.FS_CreateFile(path)
	if err != nil {
		return Err[bool, string](err.Error())
	}
	return Ok[bool, string](value)
}

func builtinFSWriteFile(path, content string) Result[struct{}, string] {
	if err := ffi.FS_WriteFile(path, content); err != nil {
		return errStringResult(err)
	}
	return okVoidResult()
}

func builtinFSAppendFile(path, content string) Result[struct{}, string] {
	if err := ffi.FS_AppendFile(path, content); err != nil {
		return errStringResult(err)
	}
	return okVoidResult()
}

func builtinFSReadFile(path string) Result[string, string] {
	value, err := ffi.FS_ReadFile(path)
	if err != nil {
		return Err[string, string](err.Error())
	}
	return Ok[string, string](value)
}

func builtinFSDeleteFile(path string) Result[struct{}, string] {
	if err := ffi.FS_DeleteFile(path); err != nil {
		return errStringResult(err)
	}
	return okVoidResult()
}

func builtinFSCopy(from, to string) Result[struct{}, string] {
	if err := ffi.FS_Copy(from, to); err != nil {
		return errStringResult(err)
	}
	return okVoidResult()
}

func builtinFSRename(from, to string) Result[struct{}, string] {
	if err := ffi.FS_Rename(from, to); err != nil {
		return errStringResult(err)
	}
	return okVoidResult()
}

func builtinFSCwd() Result[string, string] {
	value, err := ffi.FS_Cwd()
	if err != nil {
		return Err[string, string](err.Error())
	}
	return Ok[string, string](value)
}

func builtinFSAbs(path string) Result[string, string] {
	value, err := ffi.FS_Abs(path)
	if err != nil {
		return Err[string, string](err.Error())
	}
	return Ok[string, string](value)
}

func builtinFSCreateDir(path string) Result[struct{}, string] {
	if err := ffi.FS_CreateDir(path); err != nil {
		return errStringResult(err)
	}
	return okVoidResult()
}

func builtinFSDeleteDir(path string) Result[struct{}, string] {
	if err := ffi.FS_DeleteDir(path); err != nil {
		return errStringResult(err)
	}
	return okVoidResult()
}

func builtinFSListDir(path string) Result[map[string]bool, string] {
	value, err := ffi.FS_ListDir(path)
	if err != nil {
		return Err[map[string]bool, string](err.Error())
	}
	return Ok[map[string]bool, string](value)
}
