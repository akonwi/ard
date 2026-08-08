package gotarget

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestADR0057ReferenceCopiesAndSlotRebinding(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }

		fn main() [Int] {
			let first = Box{value: 1}
			let second = Box{value: 2}
			mut pointer = mut first
			let alias = pointer
			let idempotent = mut alias
			pointer = mut second
			alias.value = 10
			idempotent.value = 11
			pointer.value = 20
			[first.value, second.value]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[11,20]` {
		t.Fatalf("result = %s, want [11,20]", got)
	}
}

func TestADR0057CopyProducingAccessorGetsFreshReferenceStorage(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }

		fn main() [Int] {
			let values = [Box{value: 1}]
			let reference = mut values.at(0).expect("item")
			reference.value = 2
			[reference.value, values.at(0).expect("item").value]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[2,1]` {
		t.Fatalf("result = %s, want [2,1]", got)
	}
}

func TestADR0057ReferenceObservesOrdinaryTargetSlotReplacement(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }

		fn main() Int {
			mut target = Box{value: 1}
			let reference = mut target
			target = Box{value: 2}
			reference.value = 3
			target.value
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `3` {
		t.Fatalf("result = %s, want 3", got)
	}
}

func TestADR0057ReferenceValuedFieldCopiesAndRebindsItsOwnSlot(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }
		struct Holder { item: mut Box }

		fn main() [Int] {
			let first = Box{value: 1}
			let second = Box{value: 2}
			let holder = mut Holder{item: mut first}
			let copied = holder.item
			holder.item = mut second
			copied.value = 10
			holder.item.value = 20
			[first.value, second.value, copied.value, holder.item.value]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[10,20,10,20]` {
		t.Fatalf("result = %s, want [10,20,10,20]", got)
	}
}

func TestADR0057EscapingReferencesKeepLocalParameterFieldAndContainerStorageAlive(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }
		struct Owner { inner: Box }

		fn from_local() mut Box {
			let value = Box{value: 1}
			(mut value)
		}

		fn from_parameter(value: Box) mut Box { (mut value) }

		fn from_field() mut Box {
			let owner = Owner{inner: Box{value: 3}}
			(mut owner.inner)
		}

		fn from_list() [mut Box] {
			let value = Box{value: 4}
			[mut value]
		}

		fn from_maybe() (mut Box)? {
			let value = Box{value: 5}
			Maybe::new(mut value)
		}

		fn from_result() (mut Box)!Str {
			let value = Box{value: 6}
			Result::ok(mut value)
		}

		fn retained_closure() fn() Int {
			let value = Box{value: 7}
			let reference = mut value
			fn() Int { reference.value }
		}

		fn identity(value: $T) $T { value }
		fn apply(callback: fn(mut Box) Int, value: mut Box) Int { callback(value) }

		fn main() [Int] {
			let local = from_local()
			let parameter = from_parameter(Box{value: 2})
			let field = from_field()
			let listed = from_list().at(0).expect("list")
			let maybe = from_maybe().expect("maybe")
			let result = from_result().expect("result")
			let generic = identity(local)
			let callback_value = apply(fn(value: mut Box) Int { value.value }, parameter)
			let closure = retained_closure()
			let channel_source = from_local()
			let channel = Chan::new<mut Box>(1)
			select {
				channel.send(channel_source) => {},
				_ => panic("reference channel send would block"),
			}
			let through_channel = select {
				let received = channel.recv() => received.expect("channel"),
				_ => panic("reference channel receive would block"),
			}
			local.value = 10
			parameter.value = 20
			field.value = 30
			listed.value = 40
			maybe.value = 50
			result.value = 60
			through_channel.value = 70
			[generic.value, parameter.value, field.value, listed.value, maybe.value, result.value, through_channel.value, channel_source.value, callback_value, closure()]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[10,20,30,40,50,60,70,70,2,7]` {
		t.Fatalf("result = %s, want escaping reference values", got)
	}
}

func TestADR0057DerefIsSingleEvaluationAndShallow(t *testing.T) {
	program := lowerParitySource(t, `
		mut calls = 0

		struct Child { value: Int }
		struct Payload {
			child: mut Child,
			values: [Int],
		}

		fn load(reference: mut Payload) mut Payload {
			calls =+ 1
			reference
		}

		fn main() [Int] {
			let child = Child{value: 1}
			let payload = Payload{child: mut child, values: [1, 2]}
			let reference = mut payload
			let copy = deref load(reference)
			let copied_values = mut copy.values
			copied_values.set(0, 9)
			copied_values.push(3)
			copy.child.value = 7
			[
				calls,
				payload.values.at(0).or(0),
				payload.values.size(),
				copy.values.size(),
				child.value,
			]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[1,9,2,3,7]` {
		t.Fatalf("result = %s, want [1,9,2,3,7]", got)
	}
}

func TestADR0057DerefCopiesFixedArraysAndSharesMapDescriptors(t *testing.T) {
	program := lowerParitySource(t, `
		fn main() [Int] {
			mut fixed: [Int; 2] = [1, 2]
			let fixed_reference = mut fixed
			let fixed_copy = deref fixed_reference
			fixed = [3, 4]

			let mapping = ["a": 1]
			let map_reference = mut mapping
			let map_copy = deref map_reference
			let map_copy_reference = mut map_copy
			map_copy_reference.set("b", 2)

			[fixed_copy.at(0).or(0), fixed.at(0).or(0), mapping.get("b").or(0)]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[1,3,2]` {
		t.Fatalf("result = %s, want [1,3,2]", got)
	}
}

func TestADR0057ClosureCaptureDistinguishesHandleCopyAndSlotRebind(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }

		fn main() [Int] {
			let first = Box{value: 1}
			let second = Box{value: 2}
			mut pointer = mut first
			let observe = fn() Int { pointer.value }
			let mutate = fn() { pointer.value = 10 }
			let make_rebinder = fn() { fn() { pointer = mut second } }
			let rebind = make_rebinder()
			mutate()
			rebind()
			[observe(), pointer.value, first.value, second.value]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[10,2,10,2]` {
		t.Fatalf("result = %s, want [10,2,10,2]", got)
	}
}

func TestADR0057InteriorBorrowOfCapturedStorageUsesOuterSlot(t *testing.T) {
	program := lowerParitySource(t, `
		struct Inner { value: Int }
		struct Outer { inner: Inner }

		fn main() Int {
			mut outer = Outer{inner: Inner{value: 1}}
			let update = fn() {
				let reference = mut outer.inner
				reference.value = 42
			}
			update()
			outer.inner.value
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `42` {
		t.Fatalf("result = %s, want captured outer storage mutation", got)
	}
}

func TestADR0057ReferencesUsePointerIdentityForEqualityAndMapKeys(t *testing.T) {
	program := lowerParitySource(t, `
		struct Box { value: Int }

		fn main() Str {
			let first = Box{value: 1}
			let second = Box{value: 1}
			let copied_from = mut first
			let copied = copied_from
			let borrowed_again = mut first
			let other = mut second
			let copied_equal = copied_from == copied
			let borrowed_equal = copied_from == borrowed_again
			let distinct_equal_values = copied_from != other
			let table: [mut Box: Str] = [copied_from: "first", other: "second"]
			copied.value = 9
			let stable_key = table.get(borrowed_again).or("missing")
			"{copied_equal}:{borrowed_equal}:{distinct_equal_values}:{stable_key}"
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `"true:true:true:first"` {
		t.Fatalf("result = %s, want pointer identity result", got)
	}
}

func TestADR0057UnsafeCastMaterializesOrRecoversReferencesExplicitly(t *testing.T) {
	program := lowerParitySource(t, `
		use ard/unsafe

		struct Box { value: Int }

		fn main() [Int] {
			let value = Box{value: 1}
			let reference = mut value
			let boxed: Any = reference
			let recovered = unsafe::cast<mut Box>(boxed).expect("reference")
			let copy = unsafe::cast<Box>(boxed).expect("value")
			recovered.value = 2
			[value.value, copy.value]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[2,1]` {
		t.Fatalf("result = %s, want [2,1]", got)
	}
}

func TestADR0057DescriptorReferencesUseStorageIdentity(t *testing.T) {
	program := lowerParitySource(t, `
		fn main() Str {
			let values = [1, 2]
			let first = mut values
			let second = mut values
			let copied_values = deref first
			let copied_reference = mut copied_values
			let table: [mut [Int]: Str] = [first: "values"]
			second.set(0, 9)
			"{first == second}:{first != copied_reference}:{table.get(copied_reference).or(\"missing\")}:{copied_values.at(0).or(0)}"
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `"true:true:missing:9"` {
		t.Fatalf("result = %s, want descriptor slot identity result", got)
	}
}

func TestADR0057SanctionedReferenceAndChannelOperations(t *testing.T) {
	program := lowerParitySource(t, `
		fn main() [Int] {
			let list = mut [1]
			list.push(2)
			list.set(0, 9)

			let mapping = mut ["a": 1]
			mapping.set("b", 2)
			mapping.delete("a")

			let maybe = mut Maybe::new<Int>()
			maybe.set(7)

			let channel = Chan::new<Int>(1)
			select {
				channel.send(5) => {},
				_ => panic("channel send would block"),
			}
			let received = select {
				let value = channel.recv() => value.or(0),
				_ => panic("channel receive would block"),
			}
			channel.close()

			[list.size(), list.at(0).or(0), mapping.size(), mapping.keys().size(), maybe.or(0), received]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[2,9,1,1,7,5]` {
		t.Fatalf("result = %s, want [2,9,1,1,7,5]", got)
	}
}

func TestADR0057GoAndFFIBoundariesUseCurrentReferenceValues(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"boundary\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module boundary\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(projectDir, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "boundary.go"), []byte(`package ffi

type Item struct { N int }
type Empty struct{}
var emptyA Empty
var emptyB Empty

func EmptyA() *Empty { return &emptyA }
func EmptyB() *Empty { return &emptyB }
func EqualEmpty(left, right *Empty) bool { return left == right }

func (value Item) Read() int { return value.N }
type Reader interface { Read() int }
type Numbers []int
type Scores map[string]int
var saved Reader

func Bump(value *Item) { value.N++ }
func ReadAny(value any) int { return value.(*Item).N }
func ReadValueAny(value any) int { return value.(Item).N }
func SaveReader(value Reader) { saved = value }
func SavedN() int { return saved.Read() }
func ReaderValue() Reader { return &Item{N: 11} }
func ReadReaderPointer(value any) int { return (*value.(*Reader)).Read() }
func MutateSlice(values []int) { values[0] = 9 }
func MutateNumbers(values Numbers) { values[0] = 8 }
func ReplaceSlice(values *[]int) { *values = append(*values, 3) }
func MutateMap(values map[string]int) { values["b"] = 2 }
func MutateScores(values Scores) { values["c"] = 3 }
func Increment(value *int) { (*value)++ }
func Send(channel chan int) bool {
	select {
	case channel <- 5:
		return true
	default:
		return false
	}
}
func Identity[T any](value T) T { return value }
func IsComparable[T comparable](value T) bool { return value == value }
func ReadGeneric[T interface{ Read() int }](value T) int { return value.Read() }
func SliceSize[S ~[]E, E any](value S) int { return len(value) }
func MapSize[M ~map[K]V, K comparable, V any](value M) int { return len(value) }
func MixedSize[S ~[]E | string, E any](value S) int { return len(value) }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:boundary/ffi

fn main() {
  let first = ffi::Item{N: 1}
  let second = ffi::Item{N: 20}
  let first_pointer = mut first
  mut pointer = first_pointer
  ffi::Bump(pointer)

  let boxed: Any = pointer
  pointer = mut second
  if not ffi::ReadAny(boxed) == 2 { panic("interface did not snapshot pointer value") }
  if not pointer.N == 20 { panic("reference slot did not rebind") }

  let echoed = ffi::Identity(first_pointer)
  ffi::Bump(echoed)
  if not first.N == 3 { panic("generic pointer identity lost") }

  let copied: ffi::Item = ffi::Identity<ffi::Item>(deref first_pointer)
  ffi::Bump(first_pointer)
  if not copied.N == 3 { panic("explicit value generic did not copy") }

  ffi::SaveReader(deref first_pointer)
  ffi::Bump(first_pointer)
  if not ffi::SavedN() == 4 { panic("value interface did not own its shallow copy") }
  ffi::SaveReader(first_pointer)
  ffi::Bump(first_pointer)
  if not ffi::SavedN() == 6 { panic("reference interface lost shared pointee") }

  let value_boxed: Any = deref first_pointer
  ffi::Bump(first_pointer)
  if not ffi::ReadValueAny(value_boxed) == 6 { panic("value Any did not copy") }
  if not ffi::ReadAny(boxed) == 7 { panic("reference Any copied the pointee instead of the pointer") }
  if not ffi::IsComparable(first_pointer) { panic("reference did not satisfy comparable") }
  if not ffi::ReadGeneric(first_pointer) == 7 { panic("method-constrained reference failed") }

  let interface_value = ffi::ReaderValue()
  let interface_reference = mut interface_value
  let interface_pointer: Any = interface_reference
  if not ffi::ReadReaderPointer(interface_pointer) == 11 { panic("foreign interface pointer Any failed") }
  ffi::SaveReader(deref interface_reference)
  if not ffi::SavedN() == 11 { panic("foreign interface deref failed") }

  let count = 1
  ffi::Increment(mut count)
  if not count == 2 { panic("scalar pointee FFI mutation failed") }

  let empty_a = ffi::EmptyA()
  let empty_b = ffi::EmptyB()
  if (empty_a == empty_b) != ffi::EqualEmpty(empty_a, empty_b) {
    panic("zero-sized pointer equality diverged from Go")
  }

  let values = [1, 2]
  let mutate_slice = ffi::MutateSlice
  mutate_slice(mut values)
  ffi::ReplaceSlice(mut values)
  if not values.at(0).or(0) == 9 { panic("slice element mutation lost") }
  if not values.size() == 3 { panic("pointer-to-slice replacement lost") }
  if not ffi::SliceSize(mut values) == 3 { panic("slice generic projection failed") }
  if not ffi::MixedSize<[Int], Int>(mut values) == 3 { panic("mixed generic projection failed") }
  ffi::MutateNumbers(mut values)
  if not values.at(0).or(0) == 8 { panic("named slice projection failed") }

  let mapping = ["a": 1]
  ffi::MutateMap(mut mapping)
  if not mapping.get("b").or(0) == 2 { panic("map mutation lost") }
  if not ffi::MapSize(mut mapping) == 2 { panic("map generic projection failed") }
  ffi::MutateScores(mut mapping)
  if not mapping.get("c").or(0) == 3 { panic("named map projection failed") }

  let channel = Chan::new<Int>(1)
  if not ffi::Send(channel) { panic("channel send would block") }
  let received = select {
    let value = channel.recv() => value.or(0),
    _ => panic("channel receive would block"),
  }
  if not received == 5 { panic("channel value boundary failed") }
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
		t.Fatalf("RunProgram: %v", err)
	}
}

func TestADR0057ReferencesAddressModuleAndImportedGlobalStorage(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"globals\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "state.ard"), []byte(`struct Box { value: Int }
let imported = Box{value: 1}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use globals/state

struct Box { value: Int }
let local = Box{value: 1}

fn main() {
  let local_reference = mut local
  let imported_reference = mut state::imported
  local_reference.value = 2
  imported_reference.value = 3
  if not local.value == 2 { panic("module global reference lost") }
  if not state::imported.value == 3 { panic("imported global reference lost") }
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
		t.Fatalf("RunProgram: %v", err)
	}
}

func TestADR0057DereferencingNilForeignPointerPanics(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"nilderef\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module nilderef\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(projectDir, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "nil.go"), []byte(`package ffi

type Item struct { N int }
func NilItem() *Item { return nil }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:nilderef/ffi

fn main() {
  let value = deref ffi::NilItem()
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
	binaryPath := filepath.Join(projectDir, "nil-deref-test")
	built, err := BuildProgram(program, binaryPath, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("BuildProgram: %v", err)
	}
	if err := exec.Command(built).Run(); err == nil {
		t.Fatal("expected nil foreign pointer dereference to panic")
	}
}

func TestADR0057TraitTypedStorageReferencesDispatchThroughCurrentValue(t *testing.T) {
	program := lowerParitySource(t, `
		use ard/unsafe

		trait View {
			fn value() Int
		}
		struct Box { number: Int }
		struct Other { number: Int }
		impl View for Box {
			fn value() Int { self.number }
		}
		impl View for Other {
			fn value() Int { self.number }
		}

		fn main() [Int] {
			mut current: View = Box{number: 4}
			let reference = mut current
			let boxed: Any = reference
			let recovered = unsafe::cast<mut Box>(boxed).expect("box pointer")
			recovered.number = 6
			let before = reference.value()
			current = Other{number: 9}
			[before, reference.value()]
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `[6,9]` {
		t.Fatalf("result = %s, want trait storage forwarding and pointer projection", got)
	}
}

func TestADR0057MixedConcreteAndTraitReferencesCompareTargetIdentity(t *testing.T) {
	program := lowerParitySource(t, `
		trait View {
			fn value() Int
		}
		struct Box { number: Int }
		impl View for Box {
			fn value() Int { self.number }
		}

		fn main() Bool {
			let box = Box{number: 1}
			let concrete = mut box
			let widened: mut View = concrete
			concrete == widened
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `true` {
		t.Fatalf("result = %s, want shared target identity", got)
	}
}

func TestADR0057MutableTraitReferencesCopyComparableForwardingHandles(t *testing.T) {
	program := lowerParitySource(t, `
		use ard/unsafe

		trait View {
			fn value() Int
		}

		struct Box { number: Int }
		struct Other { number: Int }

		impl View for Box {
			fn value() Int { self.number }
		}

		impl View for Other {
			fn value() Int { self.number }
		}

		fn project(value: mut Box) mut View { value }

		fn main() Str {
			let box = Box{number: 1}
			let other = Other{number: 2}
			let source_replacement = Box{number: 9}
			mut concrete = mut box
			let view: mut View = concrete
			let independent: mut View = mut box
			let escaped = project(concrete)
			let copied = view
			concrete = mut source_replacement
			let canonical = view == independent and view == escaped
			let table: [mut View: Str] = [view: "box"]

			mut rebound = view
			rebound = mut other
			let independently_rebound = rebound != view

			let boxed: Any = view
			let recovered = unsafe::cast<mut Box>(boxed).expect("concrete pointer")
			let trait_copy: View = deref view
			recovered.number = 3

			"{canonical}:{independently_rebound}:{table.get(independent).or(\"missing\")}:{copied.value()}:{trait_copy.value()}"
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != `"true:true:box:3:1"` {
		t.Fatalf("result = %s, want canonical comparable forwarding handle result", got)
	}
}
