package ardgo

import (
	"sync"

	"github.com/akonwi/ard/ffi"
)

var registerBuiltinExternsOnce sync.Once

func RegisterBuiltinExterns() {
	registerBuiltinExternsOnce.Do(func() {
		RegisterExtern("Print", func(args ...any) (any, error) {
			ffi.Print(args[0].(string))
			return nil, nil
		})
		RegisterExtern("FloatFromInt", func(args ...any) (any, error) {
			return ffi.FloatFromInt(args[0].(int)), nil
		})
	})
}
