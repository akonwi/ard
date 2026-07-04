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
			name: "infer type arg from later argument",
			input: `use go:example.com/app/ffi

fn set(c: mut ffi::StateCtx) {
  ffi::StateSet(c, 42)
}`,
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
			name: "infer value binding from a mut reference argument",
			input: `use go:example.com/app/ffi

struct DemoState {
  ticks: Int,
}

fn copy_state(c: mut ffi::StateCtx, c2: mut ffi::StateCtx) {
  let state = ffi::StateRef<DemoState>(c)
  // T infers to DemoState (a value), not mut DemoState: the callee gets a copy.
  ffi::StateSet(c2, state)
  let echoed = ffi::Identity(state)
  let ticks: Int = echoed.ticks
}`,
		},
		{
			name: "reject mut binding of a Go pointer result",
			input: `use go:example.com/app/ffi

struct DemoState {
  ticks: Int,
}

fn bump(c: mut ffi::StateCtx) {
  mut state = ffi::StateRef<DemoState>(c)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "A mut reference from a Go call must be bound with let; rebinding it is not supported"}},
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

func First[T comparable](values []T) T {
	return values[0]
}

func Touch(c *StateCtx) {}
`
	if err := os.WriteFile(filepath.Join(ffiDir, "generic.go"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
