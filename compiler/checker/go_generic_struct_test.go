package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestGoGenericStructLiterals(t *testing.T) {
	root := t.TempDir()
	writeGoGenericStructPackage(t, root)
	resolver := checker.NewGoPackagesResolver(root, nil)

	tests := []test{
		{
			name: "explicit type arg on Go generic struct literal",
			input: `use go:example.com/app/ffi

let box = ffi::Box<Str>{Value: "hello"}`,
		},
		{
			name: "infer type arg from supplied Go struct field",
			input: `use go:example.com/app/ffi

let box = ffi::Box{Value: "hello"}`,
		},
		{
			name: "generic method keeps substituted Error as an ordinary value",
			input: `use go:example.com/app/ffi

let box = ffi::Box<Error>{Value: Error::new("boom")}
let error: Error = box.Get()
let result: Error!Error = box.Load()`,
		},
		{
			name: "generic result methods preserve Ard type arguments",
			input: `use go:example.com/app/ffi

struct User { name: Str }

let box = ffi::Make(User{name: "Ada"})
let user: User = box.Get()
let nested = box.Nested()
let nested_user: User = nested.Get()`,
		},
		{
			name: "generic method keeps nested callback Error results as ordinary values",
			input: `use go:example.com/app/ffi

let holder = ffi::Holder<Error>{}
let called: Error = holder.Call(fn() Error { Error::new("called") })
let callback: fn() Error = holder.Callback(Error::new("callback"))
let named = holder.Named(Error::new("named"))
let named_value: Error = named()
let callbacks = [fn() Error { Error::new("list") }]
let mapped_callbacks: [fn() Error] = holder.Callbacks(mut callbacks)
let maybe_callback: (fn() Error)? = holder.Lookup(Error::new("maybe"))
let variadic_value: Error = holder.Variadic(fn() Error { Error::new("variadic") })
let wrapped = holder.Wrap(Error::new("wrapped"))
let wrapped_value: Error = wrapped.Produce()
let producers = holder.Array(Error::new("array"))
let producer: fn() Error = producers.at(0).expect("producer")`,
		},
		{
			name: "generic interface method keeps substituted Error as an ordinary value",
			input: `use go:example.com/app/ffi

let getter = ffi::AsGetter(Error::new("boom"))
let error: Error = getter.Get()`,
		},
		{
			name: "generic interface argument does not accept an unconstrained declaration generic",
			input: `use go:example.com/app/ffi

fn consume(value: ffi::Getter<Int>) {}

fn forward(value: ffi::Getter<$T>) {
  consume(value)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected ffi::Getter<Int>, got ffi::Getter<$T>"}},
		},
		{
			name: "promoted generic method keeps substituted Error as an ordinary value",
			input: `use go:example.com/app/ffi

let inner = ffi::Inner<Str, Error>{Value: Error::new("boom")}
let outer = ffi::Outer<Error>{Inner: inner}
let error: Error = outer.Read()`,
		},
		{
			name: "infer slice element type arg from supplied field",
			input: `use go:example.com/app/ffi

let list_box = ffi::ListBox{Values: [1, 2]}`,
		},
		{
			name: "infer shared type arg from multiple fields",
			input: `use go:example.com/app/ffi

let radio = ffi::Radio{Value: "compact", GroupValue: "cozy"}`,
		},
		{
			name: "generic named callback field accepts matching closure",
			input: `use go:example.com/app/ffi

let radio = ffi::Radio<Str>{
  Value: "compact",
  GroupValue: "cozy",
  OnChanged: fn(event: ffi::EventContext, value: Str) {},
}`,
		},
		{
			name: "generic named callback fields preserve result ABI adaptation",
			input: `use go:example.com/app/ffi

let callbacks = ffi::Callbacks<Str>{
  Error: fn(value: Str) Void!Error { Result::ok(()) },
  Result: fn(value: Str) Str!Error { Result::ok(value) },
  Maybe: fn(value: Str) Str? { Maybe::new(value) },
}`,
		},
		{
			name: "generic named callback keeps substituted Error as an ordinary value",
			input: `use go:example.com/app/ffi

let returned = ffi::ReturnCallbacks<Str, Error>{
  Return: fn(value: Str) Error { Error::new(value) },
}
let paired = ffi::PairCallbacks<Str, Bool>{
  Pair: fn(value: Str) Str? { Maybe::new(value) },
}`,
		},
		{
			name: "generic named callback does not reinterpret substituted Error as Result",
			input: `use go:example.com/app/ffi

let returned = ffi::ReturnCallbacks<Str, Error>{
  Return: fn(value: Str) Void!Error { Result::ok(()) },
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected ffi::ReturnCallback<Str, Error>, got fn(Str) Void!Error"}},
		},
		{
			name: "generic named callback rejects substituted empty struct result",
			input: `use go:example.com/app/ffi

let callbacks = ffi::ReturnCallbacks<Str, Void>{
  Return: fn(value: Str) {},
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Unsupported foreign field ffi::ReturnCallbacks<Str, Void>.Return: callback result struct{} requires an ABI adapter"}},
		},
		{
			name: "generic named callback rejects value-position empty struct result",
			input: `use go:example.com/app/ffi

let callbacks = ffi::Callbacks<Str>{
  Empty: fn(value: Str) {},
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Unsupported foreign field ffi::Callbacks<Str>.Empty: callback result struct{} requires an ABI adapter"}},
		},
		{
			name: "reject conflicting inferred type args",
			input: `use go:example.com/app/ffi

let radio = ffi::Radio{Value: "compact", GroupValue: 1}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Conflicting inferred type arguments for T: Str and Int"}},
		},
		{
			name: "require explicit args when fields do not constrain type param",
			input: `use go:example.com/app/ffi

let box = ffi::Box{Label: "empty"}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Could not infer type argument T for Go type ffi::Box"}},
		},
		{
			name: "does not duplicate diagnostics from non-inference fields",
			input: `use go:example.com/app/ffi

let box = ffi::Box{Value: "x", Label: "a" + 1}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Cannot add different types"}},
		},
		{
			name: "enforce comparable constraint",
			input: `use go:example.com/app/ffi

let radio = ffi::Radio<[Int]>{Value: [1], GroupValue: [2]}`,
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

func writeGoGenericStructPackage(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := `package ffi

type Box[T any] struct {
	Value T
	Label string
}

func (b Box[T]) Get() T { return b.Value }

func (b Box[T]) Load() (T, error) { return b.Value, nil }

func Make[T any](value T) Box[T] { return Box[T]{Value: value} }

type Container[T any] struct {
	Value T
}

func (container Container[T]) Get() T { return container.Value }

func (b Box[T]) Nested() Container[T] { return Container[T]{Value: b.Value} }

type Holder[T any] struct{}

func (Holder[T]) Call(callback func() T) T { return callback() }

func (Holder[T]) Callback(value T) func() T { return func() T { return value } }

type Producer[T any] func() T

func (Holder[T]) Named(value T) Producer[T] { return func() T { return value } }

func (Holder[T]) Callbacks(callbacks []func() T) []func() T { return callbacks }

func (Holder[T]) Lookup(value T) (func() T, bool) {
	return func() T { return value }, true
}

func (Holder[T]) Variadic(callbacks ...func() T) T { return callbacks[0]() }

type CallbackBox[T any] struct {
	Produce func() T
}

func (Holder[T]) Wrap(value T) CallbackBox[T] {
	return CallbackBox[T]{Produce: func() T { return value }}
}

type Producers[T any] [1]func() T

func (Holder[T]) Array(value T) Producers[T] {
	return Producers[T]{func() T { return value }}
}

type Getter[T any] interface {
	Get() T
}

func AsGetter[T any](value T) Getter[T] { return Box[T]{Value: value} }

type Inner[A, B any] struct {
	Value B
}

func (inner Inner[A, B]) Read() B { return inner.Value }

type Outer[T any] struct {
	Inner[string, T]
}

type EventContext struct{}

type ValueChangedCallback[T any] func(EventContext, T)

type Radio[T comparable] struct {
	Value T
	GroupValue T
	OnChanged ValueChangedCallback[T]
}

type ErrorCallback[T any] func(T) error

type ResultCallback[T any] func(T) (T, error)

type MaybeCallback[T any] func(T) (T, bool)

type EmptyCallback[T any] func(T) struct{}

type ReturnCallback[T, R any] func(T) R

type PairCallback[T, R any] func(T) (T, R)

type ReturnCallbacks[T, R any] struct {
	Return ReturnCallback[T, R]
}

type PairCallbacks[T, R any] struct {
	Pair PairCallback[T, R]
}

type Callbacks[T any] struct {
	Error ErrorCallback[T]
	Result ResultCallback[T]
	Maybe MaybeCallback[T]
	Empty EmptyCallback[T]
}

type ListBox[T any] struct {
	Values []T
}
`
	if err := os.WriteFile(filepath.Join(ffiDir, "generic.go"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
