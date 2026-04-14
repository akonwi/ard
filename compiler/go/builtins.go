package ardgo

import (
	"encoding/json"
	"os"
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

func RegisterBuiltinExterns() {
	registerBuiltinExternsOnce.Do(func() {
		RegisterExtern("Print", func(args ...any) (any, error) {
			ffi.Print(args[0].(string))
			return nil, nil
		})
		RegisterExtern("FloatFromInt", func(args ...any) (any, error) {
			return ffi.FloatFromInt(args[0].(int)), nil
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
	})
}
