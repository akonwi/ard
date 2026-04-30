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

type AsyncHandle struct {
	wg        sync.WaitGroup
	mu        sync.Mutex
	result    any
	panicVal  any
	hasResult bool
	hasPanic  bool
}

func NewAsyncHandle() *AsyncHandle {
	handle := &AsyncHandle{}
	handle.wg.Add(1)
	return handle
}

func (h *AsyncHandle) Wait() {
	h.wg.Wait()
}

func (h *AsyncHandle) Done() {
	h.wg.Done()
}

func (h *AsyncHandle) SetResult(value any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.result = value
	h.hasResult = true
}

func (h *AsyncHandle) SetPanic(value any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.panicVal = value
	h.hasPanic = true
}

func (h *AsyncHandle) GetResult() any {
	h.wg.Wait()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hasPanic {
		panic(fmt.Errorf("panic in fiber: %v", h.panicVal))
	}
	return h.result
}

// Chrono module FFI functions

// fn now() Int
func Now() int {
	return int(time.Now().Unix())
}

// fn sleep(ns: Int) Void
func Sleep(ns int) {
	time.Sleep(time.Duration(ns))
}

// fn (FiberHandle<$T>) Void
// WaitFor waits for an opaque handle to complete.
func WaitFor(handle any) {
	if waiter, ok := handle.(interface{ Wait() }); ok {
		waiter.Wait()
		return
	}
	panic(fmt.Errorf("wait handle does not implement Wait(): %T", handle))
}

// fn (FiberHandle<$T>) $T
func GetResult(args []*runtime.Object) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("get_result expects 1 argument, got %d", len(args)))
	}
	handle, ok := args[0].Raw().(*AsyncHandle)
	if !ok {
		panic(fmt.Errorf("get_result expects async handle, got %T", args[0].Raw()))
	}
	return runtime.ValueToObject(handle.GetResult(), nil)
}
