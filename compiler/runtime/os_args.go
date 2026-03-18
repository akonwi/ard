package runtime

import (
	"os"
	"sync"
)

var (
	osArgsMu       sync.RWMutex
	osArgsOverride []string
)

func SetOSArgs(args []string) {
	osArgsMu.Lock()
	defer osArgsMu.Unlock()

	if args == nil {
		osArgsOverride = nil
		return
	}

	osArgsOverride = append([]string(nil), args...)
}

func CurrentOSArgs() []string {
	osArgsMu.RLock()
	override := append([]string(nil), osArgsOverride...)
	osArgsMu.RUnlock()
	if override != nil {
		return override
	}
	return append([]string(nil), os.Args...)
}
