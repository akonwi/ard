package checker_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestADR0057GoReferenceBoundaryClassification(t *testing.T) {
	root := t.TempDir()
	writeADR0057GoBoundaryPackage(t, root)
	resolver := checker.NewGoPackagesResolver(root, nil)

	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "pointer rejects ordinary let value", source: `let value = ffi::Item{N: 1}
ffi::TakePtr(value)`, wantError: true},
		{name: "pointer rejects ordinary mut value", source: `mut value = ffi::Item{N: 1}
ffi::TakePtr(value)`, wantError: true},
		{name: "pointer accepts explicit reference to let", source: `let value = ffi::Item{N: 1}
ffi::TakePtr(mut value)`},
		{name: "slice rejects ordinary mut binding", source: `mut values = [1, 2]
ffi::TakeSlice(values)`, wantError: true},
		{name: "slice accepts actual reference", source: `let values = [1, 2]
ffi::TakeSlice(mut values)`},
		{name: "fresh slice requires explicit reference", source: `ffi::TakeSlice(mut [1, 2])`},
		{name: "bare fresh slice is rejected", source: `ffi::TakeSlice([1, 2])`, wantError: true},
		{name: "map rejects ordinary mut binding", source: `mut values = ["a": 1]
ffi::TakeMap(values)`, wantError: true},
		{name: "map accepts actual reference", source: `let values = ["a": 1]
ffi::TakeMap(mut values)`},
		{name: "fresh map requires explicit reference", source: `ffi::TakeMap(mut ["a": 1])`},
		{name: "bare fresh map is rejected", source: `ffi::TakeMap(["a": 1])`, wantError: true},
		{name: "named slice rejects bare value", source: `let values = [1, 2]
ffi::TakeNumbers(values)`, wantError: true},
		{name: "named slice accepts actual reference", source: `let values = [1, 2]
ffi::TakeNumbers(mut values)`},
		{name: "named slice accepts fresh literal reference", source: `ffi::TakeNumbers(mut [1, 2])`},
		{name: "named slice types fresh empty literal", source: `ffi::TakeNumbers(mut [])`},
		{name: "named slice binding borrow keeps descriptor reference", source: `let values: ffi::Numbers = [1, 2]
ffi::TakeNumbers(mut values)`},
		{name: "named map rejects bare value", source: `let values = ["a": 1]
ffi::TakeScores(values)`, wantError: true},
		{name: "named map accepts actual reference", source: `let values = ["a": 1]
ffi::TakeScores(mut values)`},
		{name: "named map types fresh empty literal", source: `ffi::TakeScores(mut [:])`},
		{name: "channel remains an ordinary handle", source: `let channel = Chan::new<Int>()
ffi::TakeChan(channel)`},
		{name: "pointer to descriptor accepts actual reference", source: `let values = [1, 2]
ffi::TakeSlicePtr(mut values)`},
		{name: "Go slice accepts Slice reference", source: `let view = [1, 2].slice().expect("bounds")
ffi::TakeSlice(mut view)`},
		{name: "Go slice function value accepts Slice reference", source: `let view = [1, 2].slice().expect("bounds")
let take = ffi::TakeSlice
take(mut view)`},
		{name: "Go slice method accepts Slice reference", source: `let view = [1, 2].slice().expect("bounds")
let sink = ffi::Sink{}
sink.Take(mut view)`},
		{name: "named Go slice accepts Slice reference", source: `let view = [1, 2].slice().expect("bounds")
ffi::TakeNumbers(mut view)`},
		{name: "Go pointer to slice rejects Slice reference", source: `let view = [1, 2].slice().expect("bounds")
ffi::TakeSlicePtr(mut view)`, wantError: true},
		{name: "bare generic preserves concrete reference", source: `let value = ffi::Item{N: 1}
let reference = mut value
let echoed = ffi::Identity(reference)
ffi::TakePtr(echoed)`},
		{name: "bare generic preserves slice descriptor reference", source: `let values = [1, 2]
let reference = mut values
let echoed = ffi::Identity(reference)
ffi::TakeSlicePtr(echoed)`},
		{name: "bare generic preserves map descriptor reference", source: `let values = ["a": 1]
let reference = mut values
let echoed = ffi::Identity(reference)
ffi::TakeMapPtr(echoed)`},
		{name: "explicit value generic rejects bare reference", source: `let value = ffi::Item{N: 1}
let reference = mut value
let echoed: ffi::Item = ffi::Identity<ffi::Item>(reference)`, wantError: true},
		{name: "explicit value generic accepts deref", source: `let value = ffi::Item{N: 1}
let reference = mut value
let echoed: ffi::Item = ffi::Identity<ffi::Item>(reference.@)`},
		{name: "comparable generic accepts reference identity", source: `let value = ffi::Item{N: 1}
let reference = mut value
let ok = ffi::IsComparable(reference)`},
		{name: "comparable generic accepts Ard struct reference", source: `struct Native { N: Int }
let value = Native{N: 1}
let ok = ffi::IsComparable(mut value)`},
		{name: "unrepresentable sibling does not skip constraint validation", source: `struct Native { N: Int }
let values = [1, 2]
let native = Native{N: 1}
ffi::Two<[Int], Native>(mut values, native)`, wantError: true},
		{name: "foreign pointer equality uses pointer identity", source: `let a = ffi::ItemPtr()
let b = a
let same = a == b
let different = a != b`},
		{name: "foreign struct references compare by pointer identity", source: `let value = ffi::Item{N: 1}
let left = mut value
let right = mut value
let same = left == right`},
		{name: "method constraint validates pointer representation", source: `let value = ffi::Item{N: 1}
let reference = mut value
ffi::UseBumper(reference)`},
		{name: "slice shaped generic rejects bare value", source: `let values = [1, 2]
let size = ffi::SliceSize(values)`, wantError: true},
		{name: "slice shaped generic infers from referent", source: `let values = [1, 2]
let reference = mut values
let size = ffi::SliceSize(reference)`},
		{name: "slice shaped generic infers from Slice referent", source: `let view = [1, 2].slice().expect("bounds")
let size = ffi::SliceSize(mut view)`},
		{name: "generic Go callback Slice parameter is rejected", source: `let consume = ffi::SliceConsumer<Slice<Int>>()`, wantError: true},
		{name: "map shaped generic rejects bare value", source: `let values = ["a": 1]
let size = ffi::MapSize(values)`, wantError: true},
		{name: "map shaped generic infers from referent", source: `let values = ["a": 1]
let reference = mut values
let size = ffi::MapSize(reference)`},
		{name: "explicit descriptor generic rejects bare value", source: `let values = [1, 2]
let size = ffi::MixedSize<[Int], Int>(values)`, wantError: true},
		{name: "mixed generic uses explicit descriptor instantiation", source: `let values = [1, 2]
let reference = mut values
let size = ffi::MixedSize<[Int], Int>(reference)`},
		{name: "mixed generic does not implicitly project descriptor", source: `let values = [1, 2]
let reference = mut values
let size = ffi::MixedSize(reference)`, wantError: true},
		{name: "foreign pointer result flows as reference", source: `let pointer = ffi::ItemPtr()
ffi::TakePtr(pointer)`},
		{name: "foreign pointer satisfies Ard reference destination", source: `fn use_reference(value: mut ffi::Item) Int { value.N }
let pointer = ffi::ItemPtr()
let n = use_reference(pointer)`},

		{name: "foreign pointer explicitly dereferences", source: `let copy: ffi::Item = ffi::ItemPtr().@`},
		{name: "imported global is explicitly addressable", source: `let reference = mut ffi::Global
reference.N = 2`},
		{name: "ordinary mut Go value rejects field mutation", source: `mut value = ffi::Item{N: 1}
value.N = 2`, wantError: true},
		{name: "ordinary mut Go value rejects pointer receiver", source: `mut value = ffi::Item{N: 1}
value.Bump()`, wantError: true},
		{name: "Go reference permits field and pointer receiver mutation", source: `let value = ffi::Item{N: 1}
let reference = mut value
reference.N = 2
reference.Bump()`},
		{name: "named Go interface accepts concrete reference", source: `let value = ffi::Item{N: 1}
ffi::TakeBumper(mut value)`},
		{name: "immediate Ard concrete to trait provenance satisfies named Go interface", source: `trait View {
  fn value() Int
}
struct Native { N: Int }
impl View for Native {
  fn value() Int { self.N }
}
impl ffi::Reader for Native {
  fn read() Int { self.N }
}
let value = Native{N: 1}
ffi::TakeReader(mut value)`},
		{name: "flowed mutable trait is rejected at named Go interface", source: `trait View {
  fn value() Int
}
struct Native { N: Int }
impl View for Native {
  fn value() Int { self.N }
}
impl ffi::Reader for Native {
  fn read() Int { self.N }
}
let value = Native{N: 1}
let view: mut View = mut value
ffi::TakeReader(view)`, wantError: true},
		{name: "bare generic preserves mutable trait wrapper", source: `trait View {
  fn value() Int
}
struct Native { N: Int }
impl View for Native {
  fn value() Int { self.N }
}
fn take(value: mut View) {}
let value = Native{N: 1}
let view: mut View = mut value
let echoed = ffi::Identity(view)
take(echoed)`},
		{name: "mutable trait wrapper satisfies Go method constraint", source: `trait Bumpable {
  fn bump()
}
struct Native { N: Int }
impl Bumpable for Native {
  fn mut bump() { self.N = self.N + 1 }
}
let value = Native{N: 1}
let reference: mut Bumpable = mut value
ffi::UseBumper(reference)`},
		{name: "mutable trait wrapper satisfies Result ABI method constraint", source: `trait Parser {
  fn parse() Int!Str
}
struct Native { N: Int }
impl Parser for Native {
  fn parse() Int!Str { Result::ok(self.N) }
}
let value = Native{N: 1}
let reference: mut Parser = mut value
let result = ffi::UseParser(reference)`},
		{name: "mutable trait wrapper satisfies Maybe ABI method constraint", source: `trait Finder {
  fn find() Int?
}
struct Native { N: Int }
impl Finder for Native {
  fn find() Int? { Maybe::new(self.N) }
}
let value = Native{N: 1}
let reference: mut Finder = mut value
let result = ffi::UseFinder(reference)`},
		{name: "recursive mutable trait wrapper satisfies Go method constraint", source: `trait Link {
  fn next(value: mut Link) mut Link
}
struct Node { N: Int }
impl Link for Node {
  fn next(value: mut Link) mut Link { value }
}
let value = Node{N: 1}
let reference: mut Link = mut value
let returned = ffi::UseLink(reference)`},
		{name: "related recursive mutable trait wrapper Go struct arguments stay canonical", source: `trait Link {
  fn next(value: mut Link) mut Link
}
struct Node { N: Int }
impl Link for Node {
  fn next(value: mut Link) mut Link { value }
}
let value = Node{N: 1}
let reference: mut Link = mut value
let pair = ffi::LinkPair<mut Link, mut Link>{First: reference, Second: reference}`},
		{name: "inferred related recursive mutable trait wrapper Go struct arguments stay canonical", source: `trait Link {
  fn next(value: mut Link) mut Link
}
struct Node { N: Int }
impl Link for Node {
  fn next(value: mut Link) mut Link { value }
}
let value = Node{N: 1}
let reference: mut Link = mut value
let pair = ffi::LinkPair{First: reference, Second: reference}`},
		{name: "nested generic Go struct fields preserve mutable trait binding", source: `trait Link {
  fn next(value: mut Link) mut Link
}
struct Node { N: Int }
impl Link for Node {
  fn next(value: mut Link) mut Link { value }
}
let value = Node{N: 1}
let reference: mut Link = mut value
let inner = ffi::GenericBox<mut Link>{Value: reference}
let outer = ffi::GenericOuter<mut Link>{Box: inner}`},
		{name: "pointer nested generic Go struct fields preserve mutable trait binding", source: `trait Link {
  fn next(value: mut Link) mut Link
}
struct Node { N: Int }
impl Link for Node {
  fn next(value: mut Link) mut Link { value }
}
let value = Node{N: 1}
let reference: mut Link = mut value
let inner = ffi::GenericBox<mut Link>{Value: reference}
let outer = ffi::GenericPointerOuter<mut Link>{Box: mut inner}
let rebound: mut Link = outer.Box.Value
let linked = rebound.next(reference)`},
		{name: "representable wrapper method satisfies constraint beside Ard-only method", source: `struct Payload { N: Int }
trait View {
  fn read() Int
  fn payload() Payload
}
struct Native { N: Int }
impl View for Native {
  fn read() Int { self.N }
  fn payload() Payload { Payload{N: self.N} }
}
let value = Native{N: 1}
let reference: mut View = mut value
let result = ffi::UseReader(reference)`},
		{name: "foreign interface storage rejects bare reference at value destination", source: `let value = ffi::BumperValue()
let reference = mut value
ffi::TakeBumper(reference)`, wantError: true},
		{name: "foreign interface storage requires deref at value destination", source: `let value = ffi::BumperValue()
let reference = mut value
let boxed: Any = reference
ffi::TakeBumper(reference.@)`},
		{name: "pure Ard cannot construct double pointer", source: `let value = ffi::Item{N: 1}
let reference = mut value
ffi::TakeDoublePtr(mut reference)`, wantError: true},
		{name: "compatible foreign double pointer flows", source: `let pointer = ffi::DoublePtr()
ffi::TakeDoublePtr(pointer)
let value = ffi::ReadDoublePtr(pointer)`},
		{name: "deref removes one foreign pointer layer", source: `let pointer = ffi::DoublePtr()
let single: mut ffi::Item = pointer.@
ffi::TakePtr(single)`},
		{name: "exact pointer to interface remains unsupported", source: `fn pass(value: ffi::Bumper) { ffi::TakeInterfacePtr(mut value) }`, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := "use go:example.com/app/ffi\n\n" + tt.source
			assertGoReferenceCheckerResult(t, source, resolver, tt.wantError)
		})
	}
}

func assertGoReferenceCheckerResult(t *testing.T, source string, resolver *checker.GoPackagesResolver, wantError bool) {
	t.Helper()
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	if (strings.Contains(source, "deref ") || strings.Contains(source, ".@")) && !containsParsedDeref(reflect.ValueOf(result.Program)) {
		t.Fatal("parser did not produce a dereference expression")
	}
	checked := checker.New("test.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
	checked.Check()
	if wantError {
		targetRow := lastSourceRow(source)
		foundTarget := false
		for _, diagnostic := range checked.Diagnostics() {
			if diagnostic.Kind != checker.Error {
				continue
			}
			location := diagnostic.Primary.Span.Location
			if diagnostic.Code == checker.DiagnosticCodeUndefinedName || location.Start.Row > targetRow || location.End.Row < targetRow {
				t.Fatalf("unexpected setup/cascade error before target row %d: %#v", targetRow, checked.Diagnostics())
			}
			foundTarget = true
		}
		if !foundTarget {
			t.Fatalf("expected a semantic error on row %d, got %#v", targetRow, checked.Diagnostics())
		}
		return
	}
	for _, diagnostic := range checked.Diagnostics() {
		if diagnostic.Kind == checker.Error {
			t.Fatalf("expected no checker errors, got %#v", checked.Diagnostics())
		}
	}
}

func writeADR0057GoBoundaryPackage(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := `package ffi

type Item struct { N int }

func (i *Item) Bump() { i.N++ }

type Bumper interface { Bump() }
type Reader interface { Read() int }
type Numbers []int
type Scores map[string]int
type Sink struct{}
type LinkPair[T any, U interface{ Next(T) T }] struct { First T; Second U }
type GenericBox[T any] struct { Value T }
type GenericOuter[T any] struct { Box GenericBox[T] }
type GenericPointerOuter[T any] struct { Box *GenericBox[T] }
func (Sink) Take(values []int) {}

var Global Item
var item = Item{N: 1}
var itemPointer = &item

func ItemPtr() *Item { return &item }
func BumperValue() Bumper { return &item }
func DoublePtr() **Item { return &itemPointer }
func ReadDoublePtr(value **Item) int { return (**value).N }
func TakePtr(value *Item) {}
func TakeBumper(value Bumper) {}
func TakeReader(value Reader) {}
func TakeSlice(values []int) {}
func TakeMap(values map[string]int) {}
func TakeNumbers(values Numbers) {}
func TakeScores(values Scores) {}
func TakeChan(channel chan int) {}
func TakeSlicePtr(values *[]int) {}
func TakeMapPtr(values *map[string]int) {}
func TakeDoublePtr(value **Item) {}
func TakeInterfacePtr(value *Bumper) {}

func Identity[T any](value T) T { return value }
func IsComparable[T comparable](value T) bool { return value == value }
func Two[T comparable, U any](t T, u U) {}
func UseBumper[T interface{ Bump() }](value T) { value.Bump() }
func UseReader[T interface{ Read() int }](value T) int { return value.Read() }
func UseParser[T interface{ Parse() (int, error) }](value T) (int, error) { return value.Parse() }
func UseFinder[T interface{ Find() (int, bool) }](value T) (int, bool) { return value.Find() }
func UseLink[T interface{ Next(T) T }](value T) T { return value.Next(value) }
func SliceSize[S ~[]E, E any](value S) int { return len(value) }
func SliceConsumer[S ~[]int]() func(S) { return func(value S) {} }
func MapSize[M ~map[K]V, K comparable, V any](value M) int { return len(value) }
func MixedSize[S ~[]E | string, E any](value S) int { return len(value) }
`
	if err := os.WriteFile(filepath.Join(ffiDir, "reference.go"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
