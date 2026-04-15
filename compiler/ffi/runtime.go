package ffi

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/akonwi/ard/runtime"
)

// Runtime module FFI functions

func OsArgs() []string {
	return runtime.CurrentOSArgs()
}

// Print prints a value to stdout
func Print(str string) {
	fmt.Println(str)
}

var (
	stdinReaderMu sync.Mutex
	stdinReader   *bufio.Reader
	stdinSource   *os.File
)

// ReadLine reads a line from stdin
func ReadLine() (string, error) {
	stdinReaderMu.Lock()
	defer stdinReaderMu.Unlock()

	if stdinReader == nil || stdinSource != os.Stdin {
		stdinSource = os.Stdin
		stdinReader = bufio.NewReader(os.Stdin)
	}

	line, err := stdinReader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			if line == "" {
				return "", nil
			}
			return strings.TrimRight(line, "\r\n"), nil
		}
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// PanicWithMessage panics with a message
func PanicWithMessage(message string) {
	panic(message)
}

// Environment module FFI functions

// EnvGet retrieves an environment variable
func EnvGet(key string) *string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return nil
	}
	return &value
}

func GetTodayString() string {
	year, month, day := time.Now().Date()
	return fmt.Sprintf("%d-%02d-%02d", year, month, day)
}

// Chrono module FFI functions

// fn now() Int
func Now() int {
	return int(time.Now().Unix())
}

// fn sleep(ms: Int) Void — the Ard param is named "ms" but the
// duration module converts to nanoseconds, so the value is actually ns.
func Sleep(ns int) {
	time.Sleep(time.Duration(ns))
}

// fn (wg: Dynamic) Void
// WaitFor waits for an opaque *sync.WaitGroup handle to complete.
func WaitFor(handle any) {
	wg := handle.(*sync.WaitGroup)
	wg.Wait()
}

// fn (fibers: [Fiber]) Void
func Join(args []*runtime.Object) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("join expects 1 argument, got %d", len(args)))
	}

	fibers := args[0].AsList()
	for _, fiberObj := range fibers {
		fiberFields := fiberObj.Raw().(map[string]*runtime.Object)
		wg := fiberFields["wg"].Raw().(*sync.WaitGroup)
		wg.Wait()
	}
	return runtime.Void()
}
