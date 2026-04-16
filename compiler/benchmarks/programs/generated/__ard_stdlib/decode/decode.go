package decode

import (
	ardgo "github.com/akonwi/ard/go"
	strconv "strconv"
	strings "strings"
)

type Error struct {
	Expected string
	Found    string
	Path     []string
}

func (self Error) ToStr() string {
	pathStr := ""
	for i, step := range self.Path {
		switch {
		case i == 0:
			pathStr = (pathStr + step)
		default:
			if strings.HasPrefix(step, "[") {
				pathStr = (pathStr + step)
			} else {
				pathStr = (pathStr + ("." + step))
			}
		}
	}
	var __ardBoolMatch0 string
	if len(pathStr) == 0 {
		__ardBoolMatch0 = ("got " + self.Found + (", expected " + self.Expected))
	} else {
		__ardBoolMatch0 = ("" + pathStr + (": got " + self.Found + (", expected " + self.Expected)))
	}
	return __ardBoolMatch0
}

func decodeString(data any) ardgo.Result[string, Error] {
	result, err := ardgo.CallExtern("DecodeString", data)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[string, Error]](result)
}

func decodeInt(data any) ardgo.Result[int, Error] {
	result, err := ardgo.CallExtern("DecodeInt", data)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[int, Error]](result)
}

func decodeFloat(data any) ardgo.Result[float64, Error] {
	result, err := ardgo.CallExtern("DecodeFloat", data)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[float64, Error]](result)
}

func decodeBool(data any) ardgo.Result[bool, Error] {
	result, err := ardgo.CallExtern("DecodeBool", data)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[bool, Error]](result)
}

func IsVoid(data any) bool {
	result, err := ardgo.CallExtern("IsNil", data)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[bool](result)
}

func fromJson(json string) ardgo.Result[any, string] {
	result, err := ardgo.CallExtern("JsonToDynamic", json)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[any, string]](result)
}

func FromJson(input any) ardgo.Result[any, string] {
	var __ardUnionMatch1 string
	switch __ardUnion2 := any(input).(type) {
	case any:
		raw := __ardUnion2
		__ardTry3 := decodeString(raw)
		if __ardTry3.IsErr() {
			err := __ardTry3.UnwrapErr()
			return ardgo.Err[any, string](("Expected a JSON string, got " + err.Found))
		}
		text := __ardTry3.UnwrapOk()
		__ardUnionMatch1 = text
	case string:
		text2 := __ardUnion2
		__ardUnionMatch1 = text2
	default:
		panic("non-exhaustive union match")
	}
	json := __ardUnionMatch1
	return fromJson(json)
}

func String(data any) ardgo.Result[string, []Error] {
	__ardTry4 := decodeString(data)
	if __ardTry4.IsErr() {
		err := __ardTry4.UnwrapErr()
		return ardgo.Err[string, []Error]([]Error{err})
	}
	val := __ardTry4.UnwrapOk()
	return ardgo.Ok[string, []Error](val)
}

func Int(data any) ardgo.Result[int, []Error] {
	__ardTry5 := decodeInt(data)
	if __ardTry5.IsErr() {
		err := __ardTry5.UnwrapErr()
		return ardgo.Err[int, []Error]([]Error{err})
	}
	val := __ardTry5.UnwrapOk()
	return ardgo.Ok[int, []Error](val)
}

func Float(data any) ardgo.Result[float64, []Error] {
	__ardTry6 := decodeFloat(data)
	if __ardTry6.IsErr() {
		err := __ardTry6.UnwrapErr()
		return ardgo.Err[float64, []Error]([]Error{err})
	}
	val := __ardTry6.UnwrapOk()
	return ardgo.Ok[float64, []Error](val)
}

func Bool(data any) ardgo.Result[bool, []Error] {
	__ardTry7 := decodeBool(data)
	if __ardTry7.IsErr() {
		err := __ardTry7.UnwrapErr()
		return ardgo.Err[bool, []Error]([]Error{err})
	}
	val := __ardTry7.UnwrapOk()
	return ardgo.Ok[bool, []Error](val)
}

func Nullable[T any](decoder func(any) ardgo.Result[T, []Error]) func(any) ardgo.Result[ardgo.Maybe[T], []Error] {
	return func(data any) ardgo.Result[ardgo.Maybe[T], []Error] {
		var __ardBoolMatch8 ardgo.Result[ardgo.Maybe[T], []Error]
		if IsVoid(data) {
			__ardBoolMatch8 = ardgo.Ok[ardgo.Maybe[T], []Error](ardgo.None[T]())
		} else {
			__ardTry9 := decoder(data)
			if __ardTry9.IsErr() {
				return ardgo.Err[ardgo.Maybe[T], []Error](__ardTry9.UnwrapErr())
			}
			val := __ardTry9.UnwrapOk()
			__ardBoolMatch8 = ardgo.Ok[ardgo.Maybe[T], []Error](ardgo.Some[T](val))
		}
		return __ardBoolMatch8
	}
}

func Dynamic(data any) ardgo.Result[any, []Error] {
	return ardgo.Ok[any, []Error](data)
}

func ToList(data any) ardgo.Result[[]any, string] {
	result, err := ardgo.CallExtern("DynamicToList", data)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[[]any, string]](result)
}

func List[T any](decoder func(any) ardgo.Result[T, []Error]) func(any) ardgo.Result[[]T, []Error] {
	return func(data any) ardgo.Result[[]T, []Error] {
		__ardTry10 := ToList(data)
		if __ardTry10.IsErr() {
			found := __ardTry10.UnwrapErr()
			decodeErr := Error{Expected: "List", Found: found, Path: []string{}}
			return ardgo.Err[[]T, []Error]([]Error{decodeErr})
		}
		raw := append([]any(nil), __ardTry10.UnwrapOk()...)
		out := append([]T(nil), []T{}...)
		errors := append([]Error(nil), []Error{}...)
		for idx, item := range raw {
			__ardResult11 := decoder(item)
			if __ardResult11.IsOk() {
				ok := __ardResult11.UnwrapOk()
				_ = func() []T { out = append(out, ok); return out }()
			} else {
				errs := __ardResult11.UnwrapErr()
				for _, e := range errs {
					path := append([]string(nil), e.Path...)
					_ = func() []string { path = append([]string{("[" + strconv.Itoa(idx) + "]")}, path...); return path }()
					_ = func() []Error {
						errors = append(errors, Error{Expected: e.Expected, Found: e.Found, Path: path})
						return errors
					}()
				}
			}
		}
		var __ardIntMatch12 ardgo.Result[[]T, []Error]
		switch {
		case len(errors) == 0:
			__ardIntMatch12 = ardgo.Ok[[]T, []Error](out)
		default:
			__ardIntMatch12 = ardgo.Err[[]T, []Error](errors)
		}
		return __ardIntMatch12
	}
}

func At[T any](index int, with func(any) ardgo.Result[T, []Error]) func(any) ardgo.Result[T, []Error] {
	return func(data any) ardgo.Result[T, []Error] {
		__ardTry13 := ToList(data)
		if __ardTry13.IsErr() {
			found := __ardTry13.UnwrapErr()
			return ardgo.Err[T, []Error]([]Error{Error{Expected: "List", Found: found, Path: []string{}}})
		}
		list := append([]any(nil), __ardTry13.UnwrapOk()...)
		var __ardIntMatch14 ardgo.Result[T, []Error]
		switch {
		case len(list) == 0:
			__ardIntMatch14 = ardgo.Err[T, []Error]([]Error{Error{Expected: "Dynamic", Found: "Empty list", Path: []string{}}})
		default:
			__ardIntMatch14 = with(list[0])
		}
		return __ardIntMatch14
	}
}

func ToMap(data any) ardgo.Result[map[any]any, string] {
	result, err := ardgo.CallExtern("DynamicToMap", data)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[map[any]any, string]](result)
}

func Map[Key comparable, Value any](key func(any) ardgo.Result[Key, []Error], value func(any) ardgo.Result[Value, []Error]) func(any) ardgo.Result[map[Key]Value, []Error] {
	return func(data any) ardgo.Result[map[Key]Value, []Error] {
		__ardTry15 := ToMap(data)
		if __ardTry15.IsErr() {
			found := __ardTry15.UnwrapErr()
			decodeErr := Error{Expected: "Map", Found: found, Path: []string{}}
			return ardgo.Err[map[Key]Value, []Error]([]Error{decodeErr})
		}
		raw := __ardTry15.UnwrapOk()
		errors := append([]Error(nil), []Error{}...)
		out := map[Key]Value{}
		__ardMap16 := raw
		for _, k := range ardgo.MapKeys(__ardMap16) {
			v := __ardMap16[k]
			__ardResult17 := key(k)
			if __ardResult17.IsOk() {
				decodedKey := __ardResult17.UnwrapOk()
				__ardResult18 := value(v)
				if __ardResult18.IsOk() {
					decodedVal := __ardResult18.UnwrapOk()
					_ = func() bool { out[decodedKey] = decodedVal; return true }()
				} else {
					errs := __ardResult18.UnwrapErr()
					for _, e := range errs {
						path := append([]string(nil), e.Path...)
						_ = func() []string { path = append(path, "values"); return path }()
						_ = func() []Error {
							errors = append(errors, Error{Expected: e.Expected, Found: e.Found, Path: path})
							return errors
						}()
					}
				}
			} else {
				errs2 := __ardResult17.UnwrapErr()
				for _, e2 := range errs2 {
					path2 := append([]string(nil), e2.Path...)
					_ = func() []string { path2 = append(path2, "keys"); return path2 }()
					_ = func() []Error {
						errors = append(errors, Error{Expected: e2.Expected, Found: e2.Found, Path: path2})
						return errors
					}()
				}
			}
		}
		var __ardIntMatch19 ardgo.Result[map[Key]Value, []Error]
		switch {
		case len(errors) == 0:
			__ardIntMatch19 = ardgo.Ok[map[Key]Value, []Error](out)
		default:
			__ardIntMatch19 = ardgo.Err[map[Key]Value, []Error](errors)
		}
		return __ardIntMatch19
	}
}

func ExtractField(data any, name string) ardgo.Result[any, string] {
	result, err := ardgo.CallExtern("ExtractField", data, name)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[any, string]](result)
}

func Field[T any](name string, with func(any) ardgo.Result[T, []Error]) func(any) ardgo.Result[T, []Error] {
	return func(data any) ardgo.Result[T, []Error] {
		__ardTry20 := ExtractField(data, name)
		if __ardTry20.IsErr() {
			found := __ardTry20.UnwrapErr()
			return ardgo.Err[T, []Error]([]Error{Error{Expected: "Field", Found: found, Path: []string{name}}})
		}
		raw := __ardTry20.UnwrapOk()
		__ardTry21 := with(raw)
		if __ardTry21.IsErr() {
			errs := __ardTry21.UnwrapErr()
			out := append([]Error(nil), []Error{}...)
			for _, e := range errs {
				path := append([]string(nil), e.Path...)
				_ = func() []string { path = append([]string{name}, path...); return path }()
				_ = func() []Error { out = append(out, Error{Expected: e.Expected, Found: e.Found, Path: path}); return out }()
			}
			return ardgo.Err[T, []Error](out)
		}
		val := __ardTry21.UnwrapOk()
		return ardgo.Ok[T, []Error](val)
	}
}

func Path[T any](subpath []any, with func(any) ardgo.Result[T, []Error]) func(any) ardgo.Result[T, []Error] {
	return func(data any) ardgo.Result[T, []Error] {
		var __ardIntMatch22 ardgo.Result[T, []Error]
		switch {
		case len(subpath) == 0:
			__ardIntMatch22 = with(data)
		default:
			currentData := data
			traveled := append([]string(nil), []string{}...)
			for _, seg := range subpath {
				switch __ardUnion23 := any(seg).(type) {
				case int:
					i := __ardUnion23
					__ardTry24 := ToList(currentData)
					if __ardTry24.IsErr() {
						found := __ardTry24.UnwrapErr()
						errPath := append([]string(nil), traveled...)
						_ = func() []string { errPath = append(errPath, ("[" + strconv.Itoa(i) + "]")); return errPath }()
						ardgo.Err[any, []Error]([]Error{Error{Expected: "List", Found: found, Path: errPath}})
					}
					list := append([]any(nil), __ardTry24.UnwrapOk()...)
					currentData = list[i]
					_ = func() []string { traveled = append(traveled, ("[" + strconv.Itoa(i) + "]")); return traveled }()
				case string:
					s := __ardUnion23
					__ardTry25 := ExtractField(currentData, s)
					if __ardTry25.IsErr() {
						found2 := __ardTry25.UnwrapErr()
						errPath2 := append([]string(nil), traveled...)
						_ = func() []string { errPath2 = append(errPath2, s); return errPath2 }()
						ardgo.Err[any, []Error]([]Error{Error{Expected: "Field", Found: found2, Path: errPath2}})
					}
					currentData = __ardTry25.UnwrapOk()
					_ = func() []string { traveled = append(traveled, s); return traveled }()
				}
			}
			__ardTry26 := with(currentData)
			if __ardTry26.IsErr() {
				errs := __ardTry26.UnwrapErr()
				out2 := append([]Error(nil), []Error{}...)
				for _, err := range errs {
					_ = func() []Error {
						out2 = append(out2, Error{Expected: err.Expected, Found: err.Found, Path: func() []string { out := append([]string(nil), traveled...); return append(out, err.Path...) }()})
						return out2
					}()
				}
				return ardgo.Err[T, []Error](out2)
			}
			out := __ardTry26.UnwrapOk()
			__ardIntMatch22 = ardgo.Ok[T, []Error](out)
		}
		return __ardIntMatch22
	}
}

func OneOf[T any](first func(any) ardgo.Result[T, []Error], others []func(any) ardgo.Result[T, []Error]) func(any) ardgo.Result[T, []Error] {
	return func(data any) ardgo.Result[T, []Error] {
		res := first(data)
		if res.IsErr() {
			for _, decoder := range others {
				__ardResult27 := decoder(data)
				if __ardResult27.IsOk() {
					ok := __ardResult27.UnwrapOk()
					res = ardgo.Ok[T, []Error](ok)
					break
				} else {
					value := __ardResult27.UnwrapErr()
					_ = value
					_ = struct{}{}
				}
			}
		} else {
			_ = struct{}{}
		}
		return res
	}
}

func Run[T any](data any, decoder func(any) ardgo.Result[T, []Error]) ardgo.Result[T, []Error] {
	return decoder(data)
}

func Flatten(errors []Error) string {
	var __ardIntMatch28 string
	switch {
	case len(errors) == 0:
		__ardIntMatch28 = ""
	case len(errors) == 1:
		__ardIntMatch28 = errors[0].ToStr()
	default:
		result := ""
		for idx, err := range errors {
			if idx > 0 {
				result = (result + "\n")
			}
			result = (result + err.ToStr())
		}
		__ardIntMatch28 = result
	}
	return __ardIntMatch28
}
