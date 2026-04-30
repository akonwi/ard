package vm

import (
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

func vmFFIPrint(args []any) any {
	ffi.Print(args[0].(string))
	return runtime.NativeVoid
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
