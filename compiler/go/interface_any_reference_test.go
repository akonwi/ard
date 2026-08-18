package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestRunProgramPreservesMutableReferencesConvertedToAny(t *testing.T) {
	program := lowerSource(t, `
		use go:encoding/json
		use go:fmt

		struct User { name: Str }
		struct Box { value: Any }

		fn dynamic_type(value: Any) Str {
			fmt::Sprintf("%T", value)
		}

		fn generic_reference_type(value: mut $T) Str {
			dynamic_type(value)
		}

		fn concrete_reference_type(value: mut User) Str {
			dynamic_type(value)
		}

		fn value_type(value: User) Str {
			dynamic_type(value)
		}

		fn maybe_type(value: Any?) Str {
			match value {
				some => dynamic_type(some),
				_ => "none",
			}
		}

		fn maybe_user_name(value: User?) Str {
			match value {
				some => some.name,
				_ => "none",
			}
		}

		fn inferred_maybe_type(value: $T?) Str {
			match value {
				some => dynamic_type(some),
				_ => "none",
			}
		}

		fn identity_maybe(value: $T?) $T? {
			value
		}

		fn maybe_reference_type(value: mut $T) Str {
			maybe_type(value)
		}

		fn explicit_maybe_reference(value: mut $T) Any? {
			Maybe::new<Any>(value)
		}

		fn contextual_maybe_reference(value: mut $T) Any? {
			Maybe::new(value)
		}

		fn normalize_maybe(value: Any?) Any? {
			Maybe::new(value)
		}

		fn maybe_with_witness(value: Any?, witness: $T) Str {
			maybe_type(value)
		}

		fn generic_optional_reference(value: mut $T) Str {
			maybe_with_witness(value, 1)
		}

		fn return_reference(value: mut $T) Any {
			value
		}

		fn box_reference(value: mut $T) Box {
			Box{value: value}
		}

		fn assign_reference(value: mut $T) Any {
			mut boxed: Any = "initial"
			boxed = value
			boxed
		}

		fn unmarshal(data: [Byte], target: mut $T) Void!Error {
			let input = mut data
			json::Unmarshal(input, target)
		}

		fn main() {
			let user = User{name: "Joe"}
			let user_reference = mut user
			if not generic_reference_type(user_reference) == "*test.User" {
				panic("generic mutable reference lost identity")
			}
			if not concrete_reference_type(user_reference) == "*test.User" {
				panic("concrete mutable reference lost identity")
			}
			if not value_type(user) == "test.User" {
				panic("ordinary value should remain a value")
			}
			if not maybe_reference_type(user_reference) == "*test.User" {
				panic("Maybe<Any> conversion lost reference identity")
			}
			if not maybe_user_name(user_reference.@) == "Joe" {
				panic("explicit reference was not snapshotted before Maybe wrapping")
			}
			if not inferred_maybe_type(user_reference) == "*test.User" {
				panic("unresolved Maybe generic did not infer mutable reference")
			}
			let reference = identity_maybe(user_reference).expect("some")
			reference.name = "Grace"
			if not user.name == "Grace" {
				panic("Maybe.expect lost mutable reference identity")
			}
			let fallback = User{name: "Fallback"}
			let selected = identity_maybe(user_reference).or(mut fallback)
			selected.name = "Selected"
			if not user.name == "Selected" {
				panic("Maybe.or lost mutable reference identity")
			}
			if not maybe_type(explicit_maybe_reference(user_reference)) == "*test.User" {
				panic("explicit Maybe::new<Any> lost reference identity")
			}
			if not maybe_type(contextual_maybe_reference(user_reference)) == "*test.User" {
				panic("contextual Maybe::new lost reference identity")
			}
			if not maybe_type(normalize_maybe(contextual_maybe_reference(user_reference))) == "*test.User" {
				panic("Maybe::new double-wrapped an existing Maybe")
			}
			if not generic_optional_reference(user_reference) == "*test.User" {
				panic("generic call rejected optional interface conversion")
			}
			if not dynamic_type(return_reference(user_reference)) == "*test.User" {
				panic("return conversion lost reference identity")
			}
			if not dynamic_type(box_reference(user_reference).value) == "*test.User" {
				panic("field conversion lost reference identity")
			}
			if not dynamic_type(assign_reference(user_reference)) == "*test.User" {
				panic("assignment conversion lost reference identity")
			}
			let _ = try unmarshal("{{\"name\":\"Ada\"}}".bytes(), user_reference) -> err { panic(err) }
			if not user.name == "Ada" {
				panic("json.Unmarshal did not mutate caller storage")
			}
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramPreservesMutableReferencesConvertedToNamedEmptyInterface(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"emptyiface\"\nard = \">= 0.32.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module emptyiface\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ffi", "ffi.go"), []byte(`package ffi

import "reflect"

type Empty interface{}

func IsPointer(value Empty) bool {
	typ := reflect.TypeOf(value)
	return typ != nil && typ.Kind() == reflect.Pointer
}

func GenericIsPointer[T any](value T) bool {
	typ := reflect.TypeOf(value)
	return typ != nil && typ.Kind() == reflect.Pointer
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:emptyiface/ffi

struct User { name: Str }

fn named_empty_reference(value: mut $T) Bool {
  ffi::IsPointer(value)
}

fn generic_inferred_reference(value: mut $T) Bool {
  ffi::GenericIsPointer(value)
}

fn explicit_any_reference(value: mut $T) Bool {
  ffi::GenericIsPointer<Any>(value)
}

fn main() {
  let user = User{name: "Joe"}
  let reference = mut user
  if not named_empty_reference(reference) {
    panic("named empty interface lost reference identity")
  }
  if not generic_inferred_reference(reference) {
    panic("inferred Go generic parameter lost reference identity")
  }
  if not ffi::GenericIsPointer(reference) {
    panic("explicit mutable reference lost identity for an inferred Go generic parameter")
  }
  if not explicit_any_reference(reference) {
    panic("explicit Go generic Any destination lost reference identity")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
