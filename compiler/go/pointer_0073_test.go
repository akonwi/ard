package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// ADR 0073 runtime behavior for explicit pointer syntax.

func TestADR0073ReadOnlyAndWritablePointersShareStorage(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }

		fn read(box: *Box) Int { box.value }
		fn bump(box: *mut Box) { box.value = box.value + 1 }

		fn main() [Int] {
			mut box = Box{value: 1}
			let writer = &mut box
			let reader: *Box = writer
			bump(writer)
			bump(&mut box)
			[read(reader), read(&box), box.value]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[3,3,3]` {
		t.Fatalf("result = %s, want [3,3,3]", got)
	}
}

func TestADR0073ReadOnlyPointerToLetBinding(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }

		fn main() Int {
			let box = Box{value: 7}
			let reader = &box
			reader.value
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `7` {
		t.Fatalf("result = %s, want 7", got)
	}
}

func TestADR0073PointeeReplacementAndDeref(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }

		fn main() [Int] {
			mut box = Box{value: 1}
			let writer = &mut box
			let before = writer.*
			writer.* = Box{value: 5}
			writer.*.value = writer.*.value + 1
			[before.value, box.value, writer.*.value]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[1,6,6]` {
		t.Fatalf("result = %s, want [1,6,6]", got)
	}
}

func TestADR0073ScalarPointeeReplacement(t *testing.T) {
	program := lowerParitySource(t, `
		fn increment(count: *mut Int) {
			count.* = count.* + 1
			count.* =+ 1
		}

		fn main() Int {
			mut count = 1
			increment(&mut count)
			increment(&mut count)
			count
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `5` {
		t.Fatalf("result = %s, want 5", got)
	}
}

func TestADR0073MutableBindingFieldWritesAreLocal(t *testing.T) {
	program := lowerParitySource(t, `
		struct Profile { name: Str }
		struct User { name: Str, profile: Profile }

		fn renamed(user: User) User {
			mut user = user
			user.name = "Grace"
			user.profile.name = "Grace"
			user
		}

		fn main() [Str] {
			let original = User{name: "Ada", profile: Profile{name: "Ada"}}
			let changed = renamed(original)
			mut copy = original
			copy.name = "Lin"
			[original.name, original.profile.name, changed.name, changed.profile.name, copy.name]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `["Ada","Ada","Grace","Grace","Lin"]` {
		t.Fatalf("result = %s", got)
	}
}

func TestADR0073MutableBindingFieldWriteIsVisibleThroughPointer(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }

		fn main() [Int] {
			mut box = Box{value: 1}
			let reader = &box
			box.value = 2
			[reader.value, box.value]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[2,2]` {
		t.Fatalf("result = %s, want [2,2]", got)
	}
}

func TestADR0073CapturedMutableBindingFieldWrite(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }

		fn main() Int {
			mut box = Box{value: 1}
			let set = fn(value: Int) {
				box.value = value
			}
			set(4)
			box.value
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `4` {
		t.Fatalf("result = %s, want 4", got)
	}
}

func TestADR0073PointerListOperations(t *testing.T) {
	program := lowerParitySource(t, `
		fn add(items: *mut [Int], value: Int) {
			items.push(value)
		}

		fn count(items: *[Int]) Int { items.size() }

		fn main() [Int] {
			mut items = [1]
			add(&mut items, 2)
			let fresh = &mut [10]
			add(fresh, 20)
			[count(&items), items.size(), fresh.size()]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[2,2,2]` {
		t.Fatalf("result = %s, want [2,2,2]", got)
	}
}

func TestADR0073WritablePointerWidensToMutableTrait(t *testing.T) {
	program := lowerParitySource(t, `
		struct Counter { value: Int }

		trait Bump {
			fn mut bump()
			fn current() Int
		}

		impl Bump for Counter {
			fn mut bump() { self.value = self.value + 1 }
			fn current() Int { self.value }
		}

		fn bump_twice(item: mut Bump) {
			item.bump()
			item.bump()
		}

		fn main() Int {
			let counter = &mut Counter{value: 0}
			bump_twice(counter)
			counter.value
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `2` {
		t.Fatalf("result = %s, want 2", got)
	}
}

func TestADR0073PointerIdentityAcrossMutability(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }

		fn main() [Bool] {
			mut box = Box{value: 1}
			let writer = &mut box
			let reader: *Box = writer
			let other = &Box{value: 1}
			[writer == reader, reader == other]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[true,false]` {
		t.Fatalf("result = %s, want [true,false]", got)
	}
}

func TestADR0073ForeignPointers(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "ard.toml"), "name = \"pointers\"\nard = \">= 0.1.0\"\n")
	writeTestFile(t, filepath.Join(projectDir, "go.mod"), "module pointers\n\ngo 1.27\n")
	writeTestFile(t, filepath.Join(projectDir, "ffi", "ffi.go"), `package ffi

type Item struct { N int }

func (value Item) Read() int { return value.N }
func (value *Item) Bump() { value.N++ }

func New(n int) *Item { return &Item{N: n} }
func Bump(value *Item) { value.N++ }
`)
	mainPath := filepath.Join(projectDir, "main.ard")
	writeTestFile(t, mainPath, `use go:pointers/ffi

fn read(item: *ffi::Item) Int { item.N + item.Read() }

fn main() {
  mut item = ffi::Item{N: 1}
  let writer = &mut item
  ffi::Bump(writer)
  writer.Bump()
  let reader: *ffi::Item = writer
  if not read(reader) == 6 { panic("read-only foreign pointer read failed") }
  if not read(&item) == 6 { panic("read-only address of foreign value failed") }

  let created = ffi::New(10)
  if not read(created) == 20 { panic("Go pointer result did not coerce to read-only") }
  let snapshot = created.*
  created.* = ffi::Item{N: 4}
  if not snapshot.N == 10 { panic("foreign dereference did not copy") }
  if not created.N == 4 { panic("foreign pointee replacement failed") }
  if not (reader == writer) { panic("foreign pointer identity across mutability failed") }
}
`)

	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestADR0073GoDescriptorParametersAcceptValues(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "ard.toml"), "name = \"descriptors\"\nard = \">= 0.1.0\"\n")
	writeTestFile(t, filepath.Join(projectDir, "go.mod"), "module descriptors\n\ngo 1.27\n")
	writeTestFile(t, filepath.Join(projectDir, "ffi", "ffi.go"), `package ffi

type Numbers []int
type Sink struct{}

func (Sink) Mutate(values []int) { values[0] = 7 }

func MutateSlice(values []int) { values[0] = 9 }
func MutateNumbers(values Numbers) { values[0] = 8 }
func MutateMap(values map[string]int) { values["b"] = 2 }
func Size(values []int) int { return len(values) }
func ReplaceFirst[S ~[]E, E any](values S, replacement E) { values[0] = replacement }
`)
	mainPath := filepath.Join(projectDir, "main.ard")
	writeTestFile(t, mainPath, `use go:descriptors/ffi

fn main() {
  let values = [1, 2]
  ffi::MutateSlice(values)
  if not values.at(0).or(0) == 9 { panic("Go slice write was not shared") }
  ffi::MutateNumbers(values)
  if not values.at(0).or(0) == 8 { panic("named Go slice write was not shared") }
  let mutate = ffi::MutateSlice
  mutate(values)
  if not values.at(0).or(0) == 9 { panic("function value write was not shared") }
  let sink = ffi::Sink{}
  sink.Mutate(values)
  if not values.at(0).or(0) == 7 { panic("method write was not shared") }
  ffi::ReplaceFirst(values, 5)
  if not values.at(0).or(0) == 5 { panic("generic write was not shared") }
  if not ffi::Size([1, 2, 3]) == 3 { panic("list literal was not passed") }
  if not ffi::Size(&values) == 2 { panic("read-only pointer was not projected") }

  let mapping = ["a": 1]
  ffi::MutateMap(mapping)
  if not mapping.get("b").or(0) == 2 { panic("Go map write was not shared") }
}
`)

	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
}
