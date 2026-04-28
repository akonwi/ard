package ardgo

import (
	"encoding/json"
	"os"
	"runtime/debug"
	"sync"

	"github.com/akonwi/ard/ffi"
)

const osArgsEnvVar = "ARDGO_OS_ARGS_JSON"

var registerBuiltinExternsOnce sync.Once

func builtinOSArgs() []string {
	raw, ok := os.LookupEnv(osArgsEnvVar)
	if ok && raw != "" {
		var args []string
		if err := json.Unmarshal([]byte(raw), &args); err == nil {
			return args
		}
	}
	return ffi.OsArgs()
}

func maybeBoolPointer(value Maybe[bool]) *bool {
	if value.IsNone() {
		return nil
	}
	boolValue := value.Or(false)
	return &boolValue
}

func maybeIntPointer(value Maybe[int]) *int {
	if value.IsNone() {
		return nil
	}
	intValue := value.Or(0)
	return &intValue
}

func maybeStringPointer(value Maybe[string]) *string {
	if value.IsNone() {
		return nil
	}
	stringValue := value.Or("")
	return &stringValue
}

func builtinDynamicValue(value any) any {
	switch v := value.(type) {
	case nil, string, int, int64, float64, bool, []any, map[string]any, map[any]any:
		return value
	case jsonDynamic:
		parsed, err := ffi.JsonToDynamic(string(v))
		if err == nil {
			return parsed
		}
		return string(v)
	case jsonObjectDynamic:
		parsed, err := ffi.JsonToDynamic(string(v.raw))
		if err == nil {
			return parsed
		}
		return string(v.raw)
	}
	if encodable, ok := value.(Encodable); ok {
		return encodable.ToDyn()
	}
	return value
}

func RegisterBuiltinExterns() {
	registerBuiltinExternsOnce.Do(func() {
		if _, ok := os.LookupEnv("GOGC"); !ok {
			debug.SetGCPercent(300)
		}
		RegisterExtern("Print", func(args ...any) (any, error) {
			ffi.Print(args[0].(string))
			return nil, nil
		})
		RegisterExtern("FloatFromInt", func(args ...any) (any, error) {
			return ffi.FloatFromInt(args[0].(int)), nil
		})
		RegisterExtern("IntFromStr", func(args ...any) (any, error) {
			value := ffi.IntFromStr(args[0].(string))
			if value == nil {
				return None[int](), nil
			}
			return Some(*value), nil
		})
		RegisterExtern("FloatFromStr", func(args ...any) (any, error) {
			value := ffi.FloatFromStr(args[0].(string))
			if value == nil {
				return None[float64](), nil
			}
			return Some(*value), nil
		})
		RegisterExtern("FloatFloor", func(args ...any) (any, error) {
			return ffi.FloatFloor(args[0].(float64)), nil
		})
		RegisterExtern("OsArgs", func(args ...any) (any, error) {
			return builtinOSArgs(), nil
		})
		RegisterExtern("EnvGet", func(args ...any) (any, error) {
			value := ffi.EnvGet(args[0].(string))
			if value == nil {
				return None[string](), nil
			}
			return Some(*value), nil
		})
		RegisterExtern("Now", func(args ...any) (any, error) {
			return ffi.Now(), nil
		})
		RegisterExtern("GetTodayString", func(args ...any) (any, error) {
			return ffi.GetTodayString(), nil
		})
		RegisterExtern("ReadLine", func(args ...any) (any, error) {
			value, err := ffi.ReadLine()
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
		RegisterExtern("Sleep", func(args ...any) (any, error) {
			ffi.Sleep(args[0].(int))
			return nil, nil
		})
		RegisterExtern("WaitFor", func(args ...any) (any, error) {
			asyncWaitFor(args[0])
			return nil, nil
		})
		RegisterExtern("AsyncStart", func(args ...any) (any, error) {
			return asyncStartFiber(args[0]), nil
		})
		RegisterExtern("AsyncEval", func(args ...any) (any, error) {
			return asyncEvalFiber(args[0]), nil
		})
		RegisterExtern("GetResult", func(args ...any) (any, error) {
			return asyncGetResult(args[0], args[1]), nil
		})
		RegisterExtern("HexEncode", func(args ...any) (any, error) {
			return ffi.HexEncode(args[0].(string)), nil
		})
		RegisterExtern("HexDecode", func(args ...any) (any, error) {
			value, err := ffi.HexDecode(args[0].(string))
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
		RegisterExtern("Base64Encode", func(args ...any) (any, error) {
			return ffi.Base64Encode(args[0].(string), maybeBoolPointer(args[1].(Maybe[bool]))), nil
		})
		RegisterExtern("Base64Decode", func(args ...any) (any, error) {
			value, err := ffi.Base64Decode(args[0].(string), maybeBoolPointer(args[1].(Maybe[bool])))
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
		RegisterExtern("Base64EncodeURL", func(args ...any) (any, error) {
			return ffi.Base64EncodeURL(args[0].(string), maybeBoolPointer(args[1].(Maybe[bool]))), nil
		})
		RegisterExtern("Base64DecodeURL", func(args ...any) (any, error) {
			value, err := ffi.Base64DecodeURL(args[0].(string), maybeBoolPointer(args[1].(Maybe[bool])))
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
		RegisterExtern("JsonEncode", func(args ...any) (any, error) {
			value, err := ffi.JsonEncode(builtinDynamicValue(args[0]))
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
		RegisterExtern("StrToDynamic", func(args ...any) (any, error) {
			return args[0].(string), nil
		})
		RegisterExtern("IntToDynamic", func(args ...any) (any, error) {
			return args[0].(int), nil
		})
		RegisterExtern("FloatToDynamic", func(args ...any) (any, error) {
			return args[0].(float64), nil
		})
		RegisterExtern("BoolToDynamic", func(args ...any) (any, error) {
			return args[0].(bool), nil
		})
		RegisterExtern("VoidToDynamic", func(args ...any) (any, error) {
			return nil, nil
		})
		RegisterExtern("ListToDynamic", func(args ...any) (any, error) {
			return ffi.ListToDynamic(args[0].([]any)), nil
		})
		RegisterExtern("MapToDynamic", func(args ...any) (any, error) {
			return ffi.MapToDynamic(args[0].(map[string]any)), nil
		})
		RegisterExtern("DecodeString", func(args ...any) (any, error) {
			return builtinDecodeString(args[0]), nil
		})
		RegisterExtern("DecodeInt", func(args ...any) (any, error) {
			return builtinDecodeInt(args[0]), nil
		})
		RegisterExtern("DecodeFloat", func(args ...any) (any, error) {
			return builtinDecodeFloat(args[0]), nil
		})
		RegisterExtern("DecodeBool", func(args ...any) (any, error) {
			return builtinDecodeBool(args[0]), nil
		})
		RegisterExtern("IsNil", func(args ...any) (any, error) {
			return builtinDynamicValue(args[0]) == nil, nil
		})
		RegisterExtern("JsonToDynamic", func(args ...any) (any, error) {
			return builtinJsonToDynamic(args[0].(string)), nil
		})
		RegisterExtern("DynamicToList", func(args ...any) (any, error) {
			return builtinDynamicToList(args[0]), nil
		})
		RegisterExtern("DynamicToMap", func(args ...any) (any, error) {
			return builtinDynamicToMap(args[0]), nil
		})
		RegisterExtern("DynamicToStringMap", func(args ...any) (any, error) {
			return builtinDynamicToStringMap(args[0]), nil
		})
		RegisterExtern("ExtractField", func(args ...any) (any, error) {
			return builtinExtractField(args[0], args[1].(string)), nil
		})
		RegisterExtern("GetReqPath", func(args ...any) (any, error) {
			return ffi.GetReqPath(args[0]), nil
		})
		RegisterExtern("GetPathValue", func(args ...any) (any, error) {
			return ffi.GetPathValue(args[0], args[1].(string)), nil
		})
		RegisterExtern("GetQueryParam", func(args ...any) (any, error) {
			return ffi.GetQueryParam(args[0], args[1].(string)), nil
		})
		RegisterExtern("HTTP_Do", func(args ...any) (any, error) {
			value, err := ffi.HTTP_Do(
				args[0].(string),
				args[1].(string),
				builtinDynamicValue(args[2]),
				args[3].(map[string]string),
				maybeIntPointer(args[4].(Maybe[int])),
			)
			if err != nil {
				return Err[any, string](err.Error()), nil
			}
			return Ok[any, string](value), nil
		})
		RegisterExtern("HTTP_ResponseStatus", func(args ...any) (any, error) {
			return ffi.HTTP_ResponseStatus(args[0]), nil
		})
		RegisterExtern("HTTP_ResponseHeaders", func(args ...any) (any, error) {
			return ffi.HTTP_ResponseHeaders(args[0]), nil
		})
		RegisterExtern("HTTP_ResponseBody", func(args ...any) (any, error) {
			value, err := ffi.HTTP_ResponseBody(args[0])
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
		RegisterExtern("HTTP_ResponseClose", func(args ...any) (any, error) {
			ffi.HTTP_ResponseClose(args[0])
			return nil, nil
		})
		RegisterExtern("HTTP_Serve", func(args ...any) (any, error) {
			return builtinHTTPServe(args[0].(int), args[1]), nil
		})
		RegisterExtern("FS_Exists", func(args ...any) (any, error) {
			return ffi.FS_Exists(args[0].(string)), nil
		})
		RegisterExtern("FS_IsFile", func(args ...any) (any, error) {
			return ffi.FS_IsFile(args[0].(string)), nil
		})
		RegisterExtern("FS_IsDir", func(args ...any) (any, error) {
			return ffi.FS_IsDir(args[0].(string)), nil
		})
		RegisterExtern("FS_CreateFile", func(args ...any) (any, error) {
			return builtinFSCreateFile(args[0].(string)), nil
		})
		RegisterExtern("FS_WriteFile", func(args ...any) (any, error) {
			return builtinFSWriteFile(args[0].(string), args[1].(string)), nil
		})
		RegisterExtern("FS_AppendFile", func(args ...any) (any, error) {
			return builtinFSAppendFile(args[0].(string), args[1].(string)), nil
		})
		RegisterExtern("FS_ReadFile", func(args ...any) (any, error) {
			return builtinFSReadFile(args[0].(string)), nil
		})
		RegisterExtern("FS_DeleteFile", func(args ...any) (any, error) {
			return builtinFSDeleteFile(args[0].(string)), nil
		})
		RegisterExtern("FS_Copy", func(args ...any) (any, error) {
			return builtinFSCopy(args[0].(string), args[1].(string)), nil
		})
		RegisterExtern("FS_Rename", func(args ...any) (any, error) {
			return builtinFSRename(args[0].(string), args[1].(string)), nil
		})
		RegisterExtern("FS_Cwd", func(args ...any) (any, error) {
			return builtinFSCwd(), nil
		})
		RegisterExtern("FS_Abs", func(args ...any) (any, error) {
			return builtinFSAbs(args[0].(string)), nil
		})
		RegisterExtern("FS_CreateDir", func(args ...any) (any, error) {
			return builtinFSCreateDir(args[0].(string)), nil
		})
		RegisterExtern("FS_DeleteDir", func(args ...any) (any, error) {
			return builtinFSDeleteDir(args[0].(string)), nil
		})
		RegisterExtern("FS_ListDir", func(args ...any) (any, error) {
			return builtinFSListDir(args[0].(string)), nil
		})
		RegisterExtern("CryptoMd5", func(args ...any) (any, error) {
			return ffi.CryptoMd5(args[0].(string)), nil
		})
		RegisterExtern("CryptoSha256", func(args ...any) (any, error) {
			return ffi.CryptoSha256(args[0].(string)), nil
		})
		RegisterExtern("CryptoSha512", func(args ...any) (any, error) {
			return ffi.CryptoSha512(args[0].(string)), nil
		})
		RegisterExtern("CryptoHashPassword", func(args ...any) (any, error) {
			value, err := ffi.CryptoHashPassword(args[0].(string), maybeIntPointer(args[1].(Maybe[int])))
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
		RegisterExtern("CryptoVerifyPassword", func(args ...any) (any, error) {
			value, err := ffi.CryptoVerifyPassword(args[0].(string), args[1].(string))
			if err != nil {
				return Err[bool, string](err.Error()), nil
			}
			return Ok[bool, string](value), nil
		})
		RegisterExtern("CryptoScryptHash", func(args ...any) (any, error) {
			value, err := ffi.CryptoScryptHash(
				args[0].(string),
				maybeStringPointer(args[1].(Maybe[string])),
				maybeIntPointer(args[2].(Maybe[int])),
				maybeIntPointer(args[3].(Maybe[int])),
				maybeIntPointer(args[4].(Maybe[int])),
				maybeIntPointer(args[5].(Maybe[int])),
			)
			if err != nil {
				return Err[string, string](err.Error()), nil
			}
			return Ok[string, string](value), nil
		})
		RegisterExtern("CryptoScryptVerify", func(args ...any) (any, error) {
			value, err := ffi.CryptoScryptVerify(
				args[0].(string),
				args[1].(string),
				maybeIntPointer(args[2].(Maybe[int])),
				maybeIntPointer(args[3].(Maybe[int])),
				maybeIntPointer(args[4].(Maybe[int])),
				maybeIntPointer(args[5].(Maybe[int])),
			)
			if err != nil {
				return Err[bool, string](err.Error()), nil
			}
			return Ok[bool, string](value), nil
		})
		RegisterExtern("CryptoUUID", func(args ...any) (any, error) {
			return ffi.CryptoUUID(), nil
		})
		RegisterExtern("SqlCreateConnection", func(args ...any) (any, error) {
			return builtinSqlCreateConnection(args[0].(string)), nil
		})
		RegisterExtern("SqlClose", func(args ...any) (any, error) {
			return builtinSqlClose(args[0]), nil
		})
		RegisterExtern("SqlExecute", func(args ...any) (any, error) {
			return builtinSqlExecute(args[0], args[1].(string), args[2].([]any)), nil
		})
		RegisterExtern("SqlQuery", func(args ...any) (any, error) {
			return builtinSqlQuery(args[0], args[1].(string), args[2].([]any)), nil
		})
		RegisterExtern("SqlBeginTx", func(args ...any) (any, error) {
			return builtinSqlBeginTx(args[0]), nil
		})
		RegisterExtern("SqlCommit", func(args ...any) (any, error) {
			return builtinSqlCommit(args[0]), nil
		})
		RegisterExtern("SqlRollback", func(args ...any) (any, error) {
			return builtinSqlRollback(args[0]), nil
		})
		RegisterExtern("SqlExtractParams", func(args ...any) (any, error) {
			return ffi.SqlExtractParams(args[0].(string)), nil
		})
		RegisterExtern("NewList", func(args ...any) (any, error) {
			return []any{}, nil
		})
	})
}
