package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestGoGenericFunctionCalls(t *testing.T) {
	root := t.TempDir()
	writeGoGenericFuncPackage(t, root)
	resolver := checker.NewGoPackagesResolver(root, nil)

	tests := []test{
		{
			name: "explicit type arg instantiates generic result",
			input: `use go:example.com/app/ffi

struct DemoState {
  ticks: Int,
}

fn read(c: mut ffi::StateCtx) Int {
  let state = ffi::StateValue<DemoState>(c)
  state.ticks
}`,
		},
		{
			name: "generic pointer result gives mutable access",
			input: `use go:example.com/app/ffi

struct DemoState {
  ticks: Int,
}

fn bump(c: mut ffi::StateCtx) {
  let state = ffi::StateRef<DemoState>(c)
  state.ticks = state.ticks + 1
}`,
		},
		{
			name: "explicit primitive type arg",
			input: `use go:example.com/app/ffi

fn f(value: Any) Str {
  ffi::Depend<Str>(value)
}`,
		},
		{
			name: "infer type arg from argument",
			input: `use go:example.com/app/ffi

let doubled: Str = ffi::Identity("hello")`,
		},
		{
			name: "declaration generic remains rigid across repeated Go type parameter",
			input: `use go:example.com/app/ffi

fn forward(value: $W) {
  let paired = ffi::Pair(value, 1)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Conflicting inferred type arguments for T: $W and Int"}},
		},
		{
			name: "infer type arg from later argument",
			input: `use go:example.com/app/ffi

fn set(c: mut ffi::StateCtx) {
  ffi::StateSet(c, 42)
}`,
		},
		{
			name: "infer variadic type arg from spread element",
			input: `use go:example.com/app/ffi

fn first_present() Int {
  let values = [0, 4, 0]
  ffi::Or((mut values)...)
}`,
		},
		{
			name: "explicit variadic type arg contexts empty spread",
			input: `use go:example.com/app/ffi

fn empty() Int {
  ffi::Or<Int>([]...)
}`,
		},
		{
			name: "fixed argument contexts empty spread after generic inference",
			input: `use go:example.com/app/ffi

fn prefixed() Int {
  ffi::WithPrefix(4, []...)
}`,
		},
		{
			name: "generic spread infers from ordinary list",
			input: `use go:example.com/app/ffi

fn inferred() Int {
  let values = [1, 2]
  ffi::Or(values...)
}`,
		},
		{
			name: "context-free empty spread cannot infer generic element",
			input: `use go:example.com/app/ffi

fn empty() {
  let value = ffi::Or([]...)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Could not infer type argument T for Go function ffi::Or"}},
		},
		{
			name: "explicit generic element still requires whole-slice identity",
			input: `use go:example.com/app/ffi

fn mismatch() {
  let values = [1, 2]
  let value: Any = ffi::Or<Any>((mut values)...)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Variadic spread type mismatch: expected [Any], got [Int]"}},
		},
		{
			name: "foreign pointer to named slice spreads its referenced descriptor",
			input: `use go:example.com/app/ffi

fn join() Str {
  let reference = ffi::StringsRef()
  ffi::JoinStrings(reference...)
}`,
		},
		{
			name: "named Go slices with channel elements spread exactly",
			input: `use go:example.com/app/ffi

fn count() Int {
  let values = ffi::Channels()
  ffi::CountChannels((mut values)...)
}`,
		},
		{
			name: "named Go slices with function elements spread exactly",
			input: `use go:example.com/app/ffi

fn count() Int {
  let values = ffi::CallbacksValue()
  ffi::CountCallbacks((mut values)...)
}`,
		},
		{
			name: "named Go slices with fixed-array elements spread exactly",
			input: `use go:example.com/app/ffi

fn count() Int {
  let values = ffi::ArraysValue()
  ffi::CountArrays((mut values)...)
}`,
		},
		{
			name: "descriptor-value variadic element spread is rejected",
			input: `use go:example.com/app/ffi

fn join() {
  let first = ["a"]
  let values = [mut first]
  ffi::JoinSlices((mut values)...)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Variadic spread does not support descriptor-reference element type mut [Str]"}},
		},
		{
			name: "descriptor-value element remains rejected after callable assignment",
			input: `use go:example.com/app/ffi

fn join() {
  let first = ["a"]
  let values = [mut first]
  let join: fn(...mut [Str]) = ffi::JoinSlices
  join((mut values)...)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Variadic spread does not support descriptor-reference element type mut [Str]"}},
		},
		{
			name: "pointer-to-named-slice variadic element is conservatively rejected",
			input: `use go:example.com/app/ffi

fn join() {
  let first = ffi::StringsRef()
  let values = [first]
  ffi::NamedSlicePointers((mut values)...)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Variadic spread does not support descriptor-reference element type mut ffi::Strings"}},
		},
		{
			name: "exact pointer-to-descriptor variadic element spread is conservatively rejected",
			input: `use go:example.com/app/ffi

fn join() {
  let first = ["a"]
  let values = [mut first]
  ffi::SlicePointers((mut values)...)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Variadic spread does not support descriptor-reference element type mut [Str]"}},
		},
		{
			name: "reject uninferable call without type args",
			input: `use go:example.com/app/ffi

fn read(c: mut ffi::StateCtx) {
  let state = ffi::StateValue(c)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Could not infer type argument T for Go function ffi::StateValue"}},
		},
		{
			name: "reject type args on non-generic Go function",
			input: `use go:example.com/app/ffi

fn touch(c: mut ffi::StateCtx) {
  ffi::Touch<Int>(c)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Go function ffi::Touch is not generic"}},
		},
		{
			name: "reject wrong number of type args",
			input: `use go:example.com/app/ffi

fn read(c: mut ffi::StateCtx) {
  let state = ffi::StateValue<Str, Int>(c)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Go function ffi::StateValue expects 1 type argument(s), got 2"}},
		},
		{
			name: "inferred generic preserves reference and explicit value generic dereferences",
			input: `use go:example.com/app/ffi

struct DemoState {
  ticks: Int,
}

fn copy_state(c: mut ffi::StateCtx, c2: mut ffi::StateCtx) {
  let state = ffi::StateRef<DemoState>(c)
  let echoed: mut DemoState = ffi::Identity(state)
  let snapshot: DemoState = ffi::Identity<DemoState>(state.@)
  ffi::StateSet(c2, snapshot)
  let ticks: Int = echoed.ticks
}`,
		},
		{
			name: "mutable binding may rebind a Go pointer result",
			input: `use go:example.com/app/ffi

struct DemoState {
  ticks: Int,
}

fn bump(c: mut ffi::StateCtx, other: mut ffi::StateCtx) {
  mut state = ffi::StateRef<DemoState>(c)
  state = ffi::StateRef<DemoState>(other)
}`,
		},
		{
			name: "enforce Go constraints on type args",
			input: `use go:example.com/app/ffi

let first = ffi::First<[Int]>([[1], [2]])`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type argument [Int] does not satisfy Go constraint comparable"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse error: %s", result.Errors[0].Message)
			}
			c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
			c.Check()
			if len(tt.diagnostics) > 0 || c.HasErrors() {
				if diff := cmp.Diff(tt.diagnostics, c.Diagnostics(), compareOptions); diff != "" {
					t.Fatalf("diagnostics mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func writeGoGenericFuncPackage(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := `package ffi

type StateCtx struct {
	Value any
}

func StateRef[T any](c *StateCtx) *T {
	if p, ok := c.Value.(*T); ok {
		return p
	}
	v := c.Value.(T)
	p := &v
	c.Value = p
	return p
}

func StateValue[T any](c *StateCtx) T {
	if p, ok := c.Value.(*T); ok {
		return *p
	}
	return c.Value.(T)
}

func StateSet[T any](c *StateCtx, v T) {
	c.Value = v
}

func Depend[T any](value any) T {
	return value.(T)
}

func Identity[T any](value T) T {
	return value
}

func Pair[T any](first T, second T) T {
	return first
}

func SlicePair[T any](first []T, second []T) T {
	return first[0]
}

func First[T comparable](values []T) T {
	return values[0]
}

func Or[T comparable](values ...T) T {
	var zero T
	for _, value := range values {
		if value != zero {
			return value
		}
	}
	return zero
}

func WithPrefix[T comparable](prefix T, values ...T) T { return prefix }

type Strings []string
func StringsRef() *Strings { values := Strings{"a", "b"}; return &values }
func JoinStrings(values ...string) string { out := ""; for _, value := range values { out += value }; return out }
func NamedSlicePointers(values ...*Strings) {}

type Chans []chan int
func Channels() Chans { return Chans{make(chan int), make(chan int)} }
func CountChannels(values ...chan int) int { return len(values) }
type Callbacks []func(int) int
func CallbacksValue() Callbacks { return Callbacks{func(v int) int { return v }} }
func CountCallbacks(values ...func(int) int) int { return len(values) }
type Arrays [][2]int
func ArraysValue() Arrays { return Arrays{{1, 2}} }
func CountArrays(values ...[2]int) int { return len(values) }

func JoinSlices(values ...[]string) {}
func SlicePointers(values ...*[]string) {}

func Touch(c *StateCtx) {}
`
	if err := os.WriteFile(filepath.Join(ffiDir, "generic.go"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
