package vm

import (
	"fmt"
	"reflect"

	"github.com/akonwi/ard/ffi"
	"github.com/akonwi/ard/runtime"
)

func vmFFIFloatFromInt(args []any) any {
	return ffi.FloatFromInt(args[0].(int))
}

func vmFFIIntFromStr(args []any) any {
	value := ffi.IntFromStr(args[0].(string))
	if value == nil {
		return runtime.NoneValue()
	}
	return runtime.SomeValue(*value)
}

func vmFFIFloatFromStr(args []any) any {
	value := ffi.FloatFromStr(args[0].(string))
	if value == nil {
		return runtime.NoneValue()
	}
	return runtime.SomeValue(*value)
}

func vmFFIFloatFloor(args []any) any {
	return ffi.FloatFloor(args[0].(float64))
}

func vmFFIEnvGet(args []any) any {
	value := ffi.EnvGet(args[0].(string))
	if value == nil {
		return runtime.NoneValue()
	}
	return runtime.SomeValue(*value)
}

func vmFFIStrToDynamic(args []any) any {
	return args[0].(string)
}

func vmFFIIntToDynamic(args []any) any {
	return args[0].(int)
}

func vmFFIFloatToDynamic(args []any) any {
	return args[0].(float64)
}

func vmFFIBoolToDynamic(args []any) any {
	return args[0].(bool)
}

func vmFFIVoidToDynamic(args []any) any {
	return nil
}

func vmFFIListToDynamic(args []any) any {
	return vmToDynamicValue(args[0])
}

func vmFFIMapToDynamic(args []any) any {
	return vmToDynamicValue(args[0])
}

func vmFFIIsNil(args []any) any {
	return args[0] == nil
}

func maybeBoolArg(value any) *bool {
	maybe, ok := value.(runtime.MaybeValue)
	if !ok {
		panic("expected Bool? argument")
	}
	if maybe.None {
		return nil
	}
	boolValue := maybe.Value.(bool)
	return &boolValue
}

func maybeIntArg(value any) *int {
	maybe, ok := value.(runtime.MaybeValue)
	if !ok {
		panic("expected Int? argument")
	}
	if maybe.None {
		return nil
	}
	intValue := maybe.Value.(int)
	return &intValue
}

func maybeStringArg(value any) *string {
	maybe, ok := value.(runtime.MaybeValue)
	if !ok {
		panic("expected Str? argument")
	}
	if maybe.None {
		return nil
	}
	stringValue := maybe.Value.(string)
	return &stringValue
}

func vmToDynamicValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case runtime.VoidValue:
		return nil
	case string, int, float64, bool:
		return typed
	case runtime.ListValue:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = vmToDynamicValue(typed[i])
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = vmToDynamicValue(typed[i])
		}
		return out
	case runtime.MapValue:
		out := make(map[string]any)
		for _, key := range typed.Storage.Keys() {
			out[key.(string)] = vmToDynamicValue(mustGetMapValue(typed.Storage, key))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = vmToDynamicValue(item)
		}
		return out
	case map[string]*runtime.Object:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = vmToDynamicValue(item)
		}
		return out
	case *runtime.Object:
		return vmToDynamicValue(runtime.ObjectToValue(typed, typed.Type()))
	default:
		return typed
	}
}

func mustGetMapValue(storage runtime.VMMap, key any) any {
	value, _ := storage.GetAny(key)
	return value
}

func decodeFoundResult(found any) any {
	if found == nil {
		return runtime.ErrValue("null")
	}
	return runtime.ErrValue(ffi.FormatRawValueForError(found))
}

func vmFFIDecodeString(args []any) any {
	data := args[0]
	if data == nil {
		return decodeFoundResult(nil)
	}
	value, ok := data.(string)
	if !ok {
		return decodeFoundResult(data)
	}
	return runtime.OkValue(value)
}

func vmFFIDecodeInt(args []any) any {
	data := args[0]
	if data == nil {
		return decodeFoundResult(nil)
	}
	switch value := data.(type) {
	case int:
		return runtime.OkValue(value)
	case int64:
		return runtime.OkValue(int(value))
	case float64:
		intValue := int(value)
		if value == float64(intValue) {
			return runtime.OkValue(intValue)
		}
	}
	return decodeFoundResult(data)
}

func vmFFIDecodeFloat(args []any) any {
	data := args[0]
	if data == nil {
		return decodeFoundResult(nil)
	}
	switch value := data.(type) {
	case float64:
		return runtime.OkValue(value)
	case int:
		return runtime.OkValue(float64(value))
	case int64:
		return runtime.OkValue(float64(value))
	}
	return decodeFoundResult(data)
}

func vmFFIDecodeBool(args []any) any {
	data := args[0]
	if data == nil {
		return decodeFoundResult(nil)
	}
	value, ok := data.(bool)
	if !ok {
		return decodeFoundResult(data)
	}
	return runtime.OkValue(value)
}

func vmFFIJsonEncode(args []any) any {
	value := args[0]
	if obj, ok := value.(*runtime.Object); ok {
		value = obj.Raw()
	} else {
		value = vmToDynamicValue(value)
	}
	encoded, err := ffi.JsonEncode(value)
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(encoded)
}

func vmFFIJsonToDynamic(args []any) any {
	value, err := ffi.JsonToDynamic(args[0].(string))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFIDynamicToList(args []any) any {
	data := args[0]
	if data == nil {
		return runtime.ErrValue("null")
	}
	switch values := data.(type) {
	case runtime.ListValue:
		out := make(runtime.ListValue, len(values))
		copy(out, values)
		return runtime.OkValue(out)
	case []any:
		out := make(runtime.ListValue, len(values))
		copy(out, values)
		return runtime.OkValue(out)
	}
	value := reflect.ValueOf(data)
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return runtime.ErrValue("null")
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
		return runtime.ErrValue(ffi.FormatRawValueForError(data))
	}
	out := make(runtime.ListValue, value.Len())
	for i := 0; i < value.Len(); i++ {
		out[i] = value.Index(i).Interface()
	}
	return runtime.OkValue(out)
}

func vmFFIDynamicToMap(args []any) any {
	data := args[0]
	if data == nil {
		return runtime.ErrValue("null")
	}
	if mapped, ok := data.(runtime.MapValue); ok {
		return runtime.OkValue(runtime.MapValue{Storage: mapped.Storage.Copy()})
	}
	if mapped, ok := data.(map[string]any); ok {
		storage := runtime.NewMap[string]()
		for key, value := range mapped {
			storage.Entries[key] = value
		}
		return runtime.OkValue(runtime.MapValue{Storage: storage})
	}
	if mapped, ok := data.(map[string]*runtime.Object); ok {
		storage := runtime.NewMap[string]()
		for key, value := range mapped {
			storage.Entries[key] = runtime.ObjectToValue(value, value.Type())
		}
		return runtime.OkValue(runtime.MapValue{Storage: storage})
	}
	value := reflect.ValueOf(data)
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return runtime.ErrValue("null")
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Map {
		return runtime.ErrValue(ffi.FormatRawValueForError(data))
	}
	storage := runtime.NewMap[string]()
	iter := value.MapRange()
	for iter.Next() {
		keyValue := iter.Key()
		for keyValue.Kind() == reflect.Interface {
			if keyValue.IsNil() {
				return runtime.ErrValue(ffi.FormatRawValueForError(data))
			}
			keyValue = keyValue.Elem()
		}
		if keyValue.Kind() != reflect.String {
			return runtime.ErrValue(ffi.FormatRawValueForError(data))
		}
		storage.Entries[keyValue.String()] = iter.Value().Interface()
	}
	return runtime.OkValue(runtime.MapValue{Storage: storage})
}

func vmFFIExtractField(args []any) any {
	data := args[0]
	name := args[1].(string)
	if data == nil {
		return runtime.ErrValue("null")
	}
	if mapped, ok := data.(runtime.MapValue); ok {
		value, _ := mapped.Storage.GetAny(name)
		return runtime.OkValue(value)
	}
	if mapped, ok := data.(map[string]any); ok {
		value, _ := mapped[name]
		return runtime.OkValue(value)
	}
	if mapped, ok := data.(map[string]*runtime.Object); ok {
		if value, ok := mapped[name]; ok {
			return runtime.OkValue(runtime.ObjectToValue(value, value.Type()))
		}
		return runtime.OkValue(nil)
	}
	value := reflect.ValueOf(data)
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return runtime.ErrValue("null")
		}
		value = value.Elem()
	}
	if value.Kind() == reflect.Map && value.Type().Key().Kind() == reflect.String {
		key := reflect.ValueOf(name)
		entry := value.MapIndex(key)
		if !entry.IsValid() {
			return runtime.OkValue(nil)
		}
		return runtime.OkValue(entry.Interface())
	}
	return runtime.ErrValue(ffi.FormatRawValueForError(data))
}

func vmFFIBase64Encode(args []any) any {
	return ffi.Base64Encode(args[0].(string), maybeBoolArg(args[1]))
}

func vmFFIBase64Decode(args []any) any {
	value, err := ffi.Base64Decode(args[0].(string), maybeBoolArg(args[1]))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFIBase64EncodeURL(args []any) any {
	return ffi.Base64EncodeURL(args[0].(string), maybeBoolArg(args[1]))
}

func vmFFIBase64DecodeURL(args []any) any {
	value, err := ffi.Base64DecodeURL(args[0].(string), maybeBoolArg(args[1]))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFINow(args []any) any {
	return ffi.Now()
}

func vmFFIGetTodayString(args []any) any {
	return ffi.GetTodayString()
}

func vmFFIOsArgs(args []any) any {
	raw := ffi.OsArgs()
	values := make(runtime.ListValue, len(raw))
	for i, value := range raw {
		values[i] = value
	}
	return values
}

func resolveAsyncHandle(value any) *ffi.AsyncHandle {
	switch typed := value.(type) {
	case *ffi.AsyncHandle:
		return typed
	case runtime.StructValue:
		if len(typed.Fields) == 0 {
			panic("async fiber handle struct is missing handle field")
		}
		return resolveAsyncHandle(typed.Fields[0])
	case *runtime.Object:
		if raw, ok := typed.Raw().(*ffi.AsyncHandle); ok {
			return raw
		}
		if typed.IsStruct() {
			if field := typed.Struct_Get("handle"); field != nil {
				return resolveAsyncHandle(field)
			}
		}
		return resolveAsyncHandle(runtime.ObjectToValue(typed, typed.Type()))
	default:
		panic(fmt.Errorf("expected async handle, got %T", value))
	}
}

func vmFFIPrint(args []any) any {
	ffi.Print(args[0].(string))
	return runtime.NativeVoid
}

func vmFFISleep(args []any) any {
	ffi.Sleep(args[0].(int))
	return runtime.NativeVoid
}

func vmFFIWaitFor(args []any) any {
	ffi.WaitFor(resolveAsyncHandle(args[0]))
	return runtime.NativeVoid
}

func vmFFIGetResult(args []any) any {
	return resolveAsyncHandle(args[0]).GetResult()
}

func httpStringArg(value any) string {
	if obj, ok := value.(*runtime.Object); ok {
		return obj.AsString()
	}
	return value.(string)
}

func httpHandleArg(value any) any {
	if obj, ok := value.(*runtime.Object); ok {
		return obj.Raw()
	}
	return value
}

func httpHeadersArg(value any) map[string]string {
	if obj, ok := value.(*runtime.Object); ok {
		mapped := obj.AsMap()
		headers := make(map[string]string, len(mapped))
		for key, item := range mapped {
			headers[key] = item.AsString()
		}
		return headers
	}
	switch mapped := value.(type) {
	case runtime.MapValue:
		headers := make(map[string]string, len(mapped.Storage.Keys()))
		for _, key := range mapped.Storage.Keys() {
			entry, _ := mapped.Storage.GetAny(key)
			headers[key.(string)] = entry.(string)
		}
		return headers
	case map[string]string:
		return mapped
	case map[string]any:
		headers := make(map[string]string, len(mapped))
		for key, item := range mapped {
			headers[key] = item.(string)
		}
		return headers
	default:
		panic(fmt.Errorf("expected http headers map, got %T", value))
	}
}

func httpMaybeTimeoutArg(value any) *int {
	if obj, ok := value.(*runtime.Object); ok {
		if obj.IsNone() {
			return nil
		}
		resolved := obj.AsInt()
		return &resolved
	}
	return maybeIntArg(value)
}

func vmFFIGetReqPath(args []any) any {
	return ffi.GetReqPath(httpHandleArg(args[0]))
}

func vmFFIGetPathValue(args []any) any {
	return ffi.GetPathValue(httpHandleArg(args[0]), httpStringArg(args[1]))
}

func vmFFIGetQueryParam(args []any) any {
	return ffi.GetQueryParam(httpHandleArg(args[0]), httpStringArg(args[1]))
}

func vmFFIHTTPDo(args []any) any {
	value, err := ffi.HTTP_Do(
		httpStringArg(args[0]),
		httpStringArg(args[1]),
		vmToDynamicValue(httpHandleArg(args[2])),
		httpHeadersArg(args[3]),
		httpMaybeTimeoutArg(args[4]),
	)
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFIHTTPResponseStatus(args []any) any {
	return ffi.HTTP_ResponseStatus(httpHandleArg(args[0]))
}

func vmFFIHTTPResponseHeader(args []any) any {
	value := ffi.HTTP_ResponseHeader(httpHandleArg(args[0]), httpStringArg(args[1]))
	if value == nil {
		return runtime.NoneValue()
	}
	return runtime.SomeValue(*value)
}

func vmFFIHTTPResponseHeaders(args []any) any {
	value := ffi.HTTP_ResponseHeaders(httpHandleArg(args[0]))
	storage := runtime.NewMap[string]()
	for key, item := range value {
		storage.Entries[key] = item
	}
	return runtime.MapValue{Storage: storage}
}

func vmFFIHTTPResponseBody(args []any) any {
	value, err := ffi.HTTP_ResponseBody(httpHandleArg(args[0]))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFIHTTPResponseClose(args []any) any {
	ffi.HTTP_ResponseClose(httpHandleArg(args[0]))
	return runtime.NativeVoid
}

func sqlStringArg(value any) string {
	if obj, ok := value.(*runtime.Object); ok {
		return obj.AsString()
	}
	return value.(string)
}

func sqlHandleArg(value any) any {
	if obj, ok := value.(*runtime.Object); ok {
		return obj.Raw()
	}
	return value
}

func sqlValueArg(value any) any {
	if obj, ok := value.(*runtime.Object); ok {
		return vmToDynamicValue(obj.Raw())
	}
	return vmToDynamicValue(value)
}

func vmFFISqlCreateConnection(args []any) any {
	value, err := ffi.SqlCreateConnection(sqlStringArg(args[0]))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFISqlClose(args []any) any {
	if err := ffi.SqlClose(sqlHandleArg(args[0])); err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(runtime.NativeVoid)
}

func vmFFISqlExtractParams(args []any) any {
	params := ffi.SqlExtractParams(sqlStringArg(args[0]))
	items := make(runtime.ListValue, len(params))
	for i := range params {
		items[i] = params[i]
	}
	return items
}

func sqlArgValues(value any) []any {
	if obj, ok := value.(*runtime.Object); ok {
		items := obj.AsList()
		out := make([]any, len(items))
		for i := range items {
			out[i] = sqlValueArg(items[i])
		}
		return out
	}
	var items []any
	switch typed := value.(type) {
	case runtime.ListValue:
		items = []any(typed)
	case []any:
		items = typed
	default:
		panic(fmt.Errorf("expected list of sql values, got %T", value))
	}
	out := make([]any, len(items))
	for i := range items {
		out[i] = sqlValueArg(items[i])
	}
	return out
}

func sqlRowValue(row any) any {
	mapped, ok := row.(map[string]any)
	if !ok {
		return vmToDynamicValue(row)
	}
	storage := runtime.NewMap[string]()
	for key, value := range mapped {
		storage.Entries[key] = vmToDynamicValue(value)
	}
	return runtime.MapValue{Storage: storage}
}

func vmFFISqlQuery(args []any) any {
	rows, err := ffi.SqlQuery(sqlHandleArg(args[0]), sqlStringArg(args[1]), sqlArgValues(args[2]))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	items := make(runtime.ListValue, len(rows))
	for i := range rows {
		items[i] = sqlRowValue(rows[i])
	}
	return runtime.OkValue(items)
}

func vmFFISqlExecute(args []any) any {
	if err := ffi.SqlExecute(sqlHandleArg(args[0]), sqlStringArg(args[1]), sqlArgValues(args[2])); err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(runtime.NativeVoid)
}

func vmFFISqlBeginTx(args []any) any {
	value, err := ffi.SqlBeginTx(sqlHandleArg(args[0]))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFISqlCommit(args []any) any {
	if err := ffi.SqlCommit(sqlHandleArg(args[0])); err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(runtime.NativeVoid)
}

func vmFFISqlRollback(args []any) any {
	if err := ffi.SqlRollback(sqlHandleArg(args[0])); err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(runtime.NativeVoid)
}

func vmFFIFSExists(args []any) any {
	return ffi.FS_Exists(args[0].(string))
}

func vmFFIFSCreateFile(args []any) any {
	value, err := ffi.FS_CreateFile(args[0].(string))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFIFSWriteFile(args []any) any {
	if err := ffi.FS_WriteFile(args[0].(string), args[1].(string)); err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(runtime.NativeVoid)
}

func vmFFIFSAppendFile(args []any) any {
	if err := ffi.FS_AppendFile(args[0].(string), args[1].(string)); err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(runtime.NativeVoid)
}

func vmFFIFSReadFile(args []any) any {
	value, err := ffi.FS_ReadFile(args[0].(string))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFIFSDeleteFile(args []any) any {
	if err := ffi.FS_DeleteFile(args[0].(string)); err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(runtime.NativeVoid)
}

func vmFFIFSIsFile(args []any) any {
	return ffi.FS_IsFile(args[0].(string))
}

func vmFFIFSIsDir(args []any) any {
	return ffi.FS_IsDir(args[0].(string))
}

func vmFFIFSCopy(args []any) any {
	if err := ffi.FS_Copy(args[0].(string), args[1].(string)); err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(runtime.NativeVoid)
}

func vmFFIFSRename(args []any) any {
	if err := ffi.FS_Rename(args[0].(string), args[1].(string)); err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(runtime.NativeVoid)
}

func vmFFIFSCwd(args []any) any {
	value, err := ffi.FS_Cwd()
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFIFSAbs(args []any) any {
	value, err := ffi.FS_Abs(args[0].(string))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFIFSCreateDir(args []any) any {
	if err := ffi.FS_CreateDir(args[0].(string)); err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(runtime.NativeVoid)
}

func vmFFIFSDeleteDir(args []any) any {
	if err := ffi.FS_DeleteDir(args[0].(string)); err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(runtime.NativeVoid)
}

func vmFFIFSListDir(args []any) any {
	value, err := ffi.FS_ListDir(args[0].(string))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	storage := runtime.NewMap[string]()
	for key, item := range value {
		storage.Entries[key] = item
	}
	return runtime.OkValue(runtime.MapValue{Storage: storage})
}

func vmFFIReadLine(args []any) any {
	value, err := ffi.ReadLine()
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFICryptoMd5(args []any) any {
	return ffi.CryptoMd5(args[0].(string))
}

func vmFFICryptoSha256(args []any) any {
	return ffi.CryptoSha256(args[0].(string))
}

func vmFFICryptoSha512(args []any) any {
	return ffi.CryptoSha512(args[0].(string))
}

func vmFFICryptoHashPassword(args []any) any {
	value, err := ffi.CryptoHashPassword(args[0].(string), maybeIntArg(args[1]))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFICryptoVerifyPassword(args []any) any {
	value, err := ffi.CryptoVerifyPassword(args[0].(string), args[1].(string))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFICryptoScryptHash(args []any) any {
	value, err := ffi.CryptoScryptHash(
		args[0].(string),
		maybeStringArg(args[1]),
		maybeIntArg(args[2]),
		maybeIntArg(args[3]),
		maybeIntArg(args[4]),
		maybeIntArg(args[5]),
	)
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFICryptoScryptVerify(args []any) any {
	value, err := ffi.CryptoScryptVerify(
		args[0].(string),
		args[1].(string),
		maybeIntArg(args[2]),
		maybeIntArg(args[3]),
		maybeIntArg(args[4]),
		maybeIntArg(args[5]),
	)
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}

func vmFFICryptoUUID(args []any) any {
	return ffi.CryptoUUID()
}

func vmFFIHexEncode(args []any) any {
	return ffi.HexEncode(args[0].(string))
}

func vmFFIHexDecode(args []any) any {
	value, err := ffi.HexDecode(args[0].(string))
	if err != nil {
		return runtime.ErrValue(err.Error())
	}
	return runtime.OkValue(value)
}
