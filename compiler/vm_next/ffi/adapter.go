package ffi

import (
	"reflect"

	"github.com/akonwi/ard/air"
)

type Bridge interface {
	HostArg(args any, index int, target reflect.Type) (any, error)
	HostArgInt(args any, index int) (int, error)
	HostArgFloat64(args any, index int) (float64, error)
	HostArgBool(args any, index int) (bool, error)
	HostArgString(args any, index int) (string, error)
	HostArgMaybeBool(args any, index int) (any, error)
	HostArgMaybeInt(args any, index int) (any, error)
	HostArgMaybeFloat64(args any, index int) (any, error)
	HostArgMaybeString(args any, index int) (any, error)
	HostArgAny(args any, index int) (any, error)
	HostArgAnySlice(args any, index int) ([]any, error)
	HostArgStringSlice(args any, index int) ([]string, error)
	HostArgStringMap(args any, index int) (map[string]string, error)
	HostReturnVoid(returnType air.TypeID) (any, error)
	HostReturnValue(returnType air.TypeID, value any) (any, error)
	HostReturnError(returnType air.TypeID, err error) (any, error)
	HostReturnValueError(returnType air.TypeID, value any, err error) (any, error)
	HostReturnResult(returnType air.TypeID, value any, errValue any, ok bool) (any, error)
}

type ExternAdapter func(bridge Bridge, extern air.Extern, binding string, args any) (any, error)

type AdapterLookup func(binding string, fn any) (ExternAdapter, bool)

var adapterLookups []AdapterLookup

func RegisterAdapterLookup(lookup AdapterLookup) {
	adapterLookups = append(adapterLookups, lookup)
}

func Adapter(binding string, fn any) (ExternAdapter, bool) {
	for i := len(adapterLookups) - 1; i >= 0; i-- {
		if adapter, ok := adapterLookups[i](binding, fn); ok {
			return adapter, true
		}
	}
	return nil, false
}
