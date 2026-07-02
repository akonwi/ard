package checker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestStructMethodsAreStoredInProgramSideTable(t *testing.T) {
	result := parse.Parse([]byte(`
		struct Frame {}

		impl Frame {
			fn sub() Frame {
				self
			}
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}

	sym, ok := c.scope.get("Frame")
	if !ok {
		t.Fatal("Frame missing from scope")
	}
	frame, ok := sym.Type.(*StructDef)
	if !ok {
		t.Fatalf("Frame symbol type = %T, want *StructDef", sym.Type)
	}
	if got := frame.get("sub"); got != nil {
		t.Fatalf("StructDef.get returned method %T; methods should not be in value-shape lookup", got)
	}
	if _, ok := c.program.StructMethod(StructMethodOwner(frame), "sub"); !ok {
		t.Fatal("method sub missing from Program.StructMethods side table")
	}
}
func TestStructSideTableMethodUsesGenericBindingsFromNestedFields(t *testing.T) {
	result := parse.Parse([]byte(`
		struct Chan {
			value: $T
		}

		struct Channel {
			chan: Chan<$T>
		}

		impl Channel {
			fn send(value: $T) Bool {
				true
			}
		}

		fn new() Channel<Int> { Channel{ chan: Chan{ value: 0 } } }

		let ch = new()
		ch.send(42)
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
}

func TestTransitiveGenericStructMethodUsesOwnerDefinition(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.1.0\""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "box.ard"), []byte(`
		struct Box {
			item: $T
		}

		impl Box {
			fn put(value: $T) Bool {
				true
			}
		}
	`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "aliases.ard"), []byte(`
		use test_project/box

		type IntBox = box::Box<Int>
	`), 0o644); err != nil {
		t.Fatal(err)
	}

	result := parse.Parse([]byte(`
		use test_project/aliases

		fn save(box: aliases::IntBox) Bool {
			box.put("oops")
		}
	`), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	resolver, err := NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	c := New(filepath.Join(tempDir, "main.ard"), result.Program, resolver)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected generic method argument error")
	}
	if got := c.Diagnostics()[0].Message; got != "type mismatch: expected Int, got Str" {
		t.Fatalf("first diagnostic = %q, want generic method argument mismatch", got)
	}
}
func TestStructEqualityIndependentOfMethodSideTable(t *testing.T) {
	left := &StructDef{Name: "Frame", Fields: map[string]Type{}}
	right := &StructDef{Name: "Frame", Fields: map[string]Type{}}
	program := &Program{}
	program.AddStructMethod(StructMethodOwner(right), "sub", &FunctionDef{Name: "sub", ReturnType: right})

	if !left.equal(right) {
		t.Fatal("struct equality should ignore side-table method signatures")
	}
}

func TestExplicitTypeArgsCannotOverrideReceiverGenericMethod(t *testing.T) {
	result := parse.Parse([]byte(`
		struct Box {
			item: $T
		}

		impl Box {
			fn get() $T {
				self.item
			}
		}

		let b = Box{item: 1}
		let x = b.get<Str>()
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected explicit receiver-generic method type arg error")
	}
	if got := c.Diagnostics()[0].Message; got != "function get does not take type arguments" {
		t.Fatalf("first diagnostic = %q, want explicit method type arg rejection", got)
	}
}
func TestMethodsCannotIntroduceGenericParams(t *testing.T) {
	result := parse.Parse([]byte(`
		struct Box {
			item: Int
		}

		impl Box {
			fn get() Int {
				if true {
					let x: $U = 1
				}
				self.item
			}
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected method generic parameter error")
	}
	if got := c.Diagnostics()[0].Message; got != "methods cannot introduce generic type parameters; use the receiver type's generics" {
		t.Fatalf("first diagnostic = %q, want method generic parameter rejection", got)
	}
}
func TestUnboundGenericExplicitCallTypeArgIsRejected(t *testing.T) {
	result := parse.Parse([]byte(`
		use ard/maybe
		fn get_raw<$T>(key: Str) $T? { maybe::none() }
		get_raw<$U>("count")
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected unbound explicit type arg error")
	}
	if got := c.Diagnostics()[0].Message; got != "unbound generic type argument $U" {
		t.Fatalf("first diagnostic = %q, want unbound generic type arg", got)
	}
}
func TestNestedFunctionCannotUseOuterGenericAsExplicitTypeArg(t *testing.T) {
	result := parse.Parse([]byte(`
		use ard/maybe
		fn raw<$T>(key: Str) $T? { maybe::none() }

		fn outer<$T>() Bool {
			fn inner() Bool {
				raw<$T>("x").is_some()
			}
			inner()
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected outer generic type arg to be rejected in nested function")
	}
	if got := c.Diagnostics()[0].Message; got != "unbound generic type argument $T" {
		t.Fatalf("first diagnostic = %q, want unbound generic type arg", got)
	}
}
func TestClosureCannotUseOuterGenericAsExplicitTypeArg(t *testing.T) {
	result := parse.Parse([]byte(`
		use ard/maybe
		fn raw<$T>(key: Str) $T? { maybe::none() }

		fn outer<$T>() Bool {
			let inner = fn() Bool {
				raw<$T>("x").is_some()
			}
			true
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected outer generic type arg to be rejected in closure")
	}
	if got := c.Diagnostics()[0].Message; got != "unbound generic type argument $T" {
		t.Fatalf("first diagnostic = %q, want unbound generic type arg", got)
	}
}
func TestGenericStructReceiverBindingInExplicitCallbackParameter(t *testing.T) {
	result := parse.Parse([]byte(`
		struct State {
			_type: fn($T) Void
		}

		struct Widget {}
		struct Ctx {}

		impl State {
			fn value() $T {
				panic("x")
			}
		}

		fn make<$T>(init: fn(Ctx, State<$T>) $T, build: fn(Ctx, State<$T>) Widget) Widget {
			Widget{}
		}

		struct Model { n: Int }

		fn main() {
			let _ = make<Model>(
				init: fn(_ctx: Ctx, _state: State<Model>) Model { Model{n: 0} },
				build: fn(_ctx: Ctx, state: State<Model>) Widget {
					let m: Model = state.value()
					Widget{}
				},
			)
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
}
func TestGenericStructReceiverBindingInInferredCallbackParameter(t *testing.T) {
	result := parse.Parse([]byte(`
		struct State {
			_type: fn($T) Void
		}

		struct Widget {}
		struct Ctx {}

		impl State {
			fn value() $T {
				panic("x")
			}
		}

		fn make<$T>(init: fn(Ctx, State<$T>) $T, build: fn(Ctx, State<$T>) Widget) Widget {
			Widget{}
		}

		struct Model { n: Int }

		fn main() {
			let _ = make<Model>(
				init: fn(_ctx, _state) { Model{n: 0} },
				build: fn(_ctx, state) {
					let m: Model = state.value()
					Widget{}
				},
			)
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
}
func TestGenericStructReceiverBindingInCallbackParameterStillRejectsMismatch(t *testing.T) {
	result := parse.Parse([]byte(`
		struct State {
			_type: fn($T) Void
		}

		struct Widget {}
		struct Ctx {}

		impl State {
			fn value() $T {
				panic("x")
			}
		}

		fn make<$T>(init: fn(Ctx, State<$T>) $T, build: fn(Ctx, State<$T>) Widget) Widget {
			Widget{}
		}

		struct Model { n: Int }
		struct Other { s: Str }

		fn main() {
			let _ = make<Model>(
				init: fn(_ctx: Ctx, _state: State<Model>) Model { Model{n: 0} },
				build: fn(_ctx: Ctx, state: State<Other>) Widget {
					Widget{}
				},
			)
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected callback state type mismatch")
	}
	if got := c.Diagnostics()[0].Message; got != "type mismatch: expected Model, got Other" {
		t.Fatalf("first diagnostic = %q, want callback state type mismatch", got)
	}
}
func TestExplicitGenericStructCanUseTypeParamOnlyInMethods(t *testing.T) {
	result := parse.Parse([]byte(`
		struct State<$T> {
			handle: Int
		}

		struct Widget {}
		struct Ctx {}

		impl State {
			fn value() $T {
				panic("x")
			}

			fn set(mutate: fn(mut $T)) {
			}
		}

		fn make<$T>(init: fn(Ctx, State<$T>) $T, build: fn(Ctx, State<$T>) Widget) Widget {
			Widget{}
		}

		struct Model { n: Int }

		fn main() {
			let _ = make<Model>(
				init: fn(_ctx: Ctx, _state: State<Model>) Model { Model{n: 0} },
				build: fn(_ctx: Ctx, state: State<Model>) Widget {
					let model = state.value()
					state.set(fn(mut next: Model) {
						next.n = model.n + 1
					})
					Widget{}
				},
			)
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
}
func TestExplicitGenericStructTypeArgumentsRemainDistinctWithoutGenericFields(t *testing.T) {
	result := parse.Parse([]byte(`
		struct State<$T> {
			handle: Int
		}

		struct Model { n: Int }
		struct Other { s: Str }

		fn consume(state: State<Model>) {}

		fn main() {
			let other: State<Other> = State{handle: 1}
			consume(other)
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected distinct explicit struct type arguments to mismatch")
	}
	if got := c.Diagnostics()[0].Message; got != "Type mismatch: Expected State<Model>, got State<Other>" {
		t.Fatalf("first diagnostic = %q, want State<Model>/State<Other> mismatch", got)
	}
}
