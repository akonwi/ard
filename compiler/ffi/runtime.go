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
	if waiter, ok := handle.(interface{ Wait() }); ok {
		waiter.Wait()
		return
	}
	panic(fmt.Errorf("wait handle does not implement Wait(): %T", handle))
}

// fn (WaitGroup, $T) $T
func GetResult(args []*runtime.Object) *runtime.Object {
	if len(args) != 2 {
		panic(fmt.Errorf("get_result expects 2 arguments, got %d", len(args)))
	}
	WaitFor(args[0].Raw())
	return args[1]
}
