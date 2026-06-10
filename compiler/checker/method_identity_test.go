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
		extern type Chan<$T>

		struct Channel {
			chan: Chan<$T>
		}

		impl Channel {
			fn send(value: $T) Bool {
				true
			}
		}

		extern fn new() Channel<Int> = "NewChannel"

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

func TestStructMethodLookupUsesOwnerModuleBeyondDirectImports(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.1.0\""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "db.ard"), []byte(`
		use ard/sql

		fn init() sql::Database {
			sql::open("postgres://example").expect("connect")
		}
	`), 0o644); err != nil {
		t.Fatal(err)
	}

	result := parse.Parse([]byte(`
		use test_project/db

		let conn = db::init()
		let query = conn.query("SELECT 1")
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

func TestExplicitMethodTypeArgsPreserveReceiverGenericBindings(t *testing.T) {
	result := parse.Parse([]byte(`
		struct Box {
			item: $T
		}

		impl Box {
			fn pick<$U>(value: $U) $T {
				self.item
			}
		}

		let b = Box{item: 1}
		let x: Int = b.pick<Str>("ok")
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

func TestUnboundGenericExplicitCallTypeArgIsRejected(t *testing.T) {
	result := parse.Parse([]byte(`
		extern fn get_raw<$T>(key: Str) $T? = "GetRaw"
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
		extern fn raw<$T>(key: Str) $T? = "Raw"

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
		extern fn raw<$T>(key: Str) $T? = "Raw"

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
