package checker_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

const referenceBoxPrelude = `
struct Box {
  value: Int,
}

impl Box {
  fn get() Int { self.value }
  fn mut set(value: Int) { self.value = value }
}
`

func TestADR0057BindingAndReferenceCapabilityMatrix(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "let value rejects slot assignment", source: `let box = Box{value: 1}
box = Box{value: 2}`, wantError: true},
		{name: "let value rejects interior field assignment", source: `let box = Box{value: 1}
box.value = 2`, wantError: true},
		{name: "let value rejects mutating method", source: `let box = Box{value: 1}
box.set(2)`, wantError: true},
		{name: "let value is explicitly borrowable", source: `let box = Box{value: 1}
let reference: mut Box = mut box`},
		{name: "mut value accepts slot assignment", source: `mut box = Box{value: 1}
box = Box{value: 2}`},
		{name: "mut scalar accepts compound slot assignment", source: `mut count = 1
count =+ 1`},
		{name: "mut value rejects interior field assignment", source: `mut box = Box{value: 1}
box.value = 2`, wantError: true},
		{name: "mut value rejects mutating method", source: `mut box = Box{value: 1}
box.set(2)`, wantError: true},
		{name: "mut value is explicitly borrowable", source: `mut box = Box{value: 1}
let reference: mut Box = mut box`},
		{name: "let reference permits interior field assignment", source: `let box = Box{value: 1}
let reference = mut box
reference.value = 2`},
		{name: "let reference permits mutating method", source: `let box = Box{value: 1}
let reference = mut box
reference.set(2)`},
		{name: "let reference rejects reference slot rebinding", source: `let first = Box{value: 1}
let second = Box{value: 2}
let reference = mut first
reference = mut second`, wantError: true},
		{name: "mut reference permits reference slot rebinding", source: `let first = Box{value: 1}
let second = Box{value: 2}
mut reference = mut first
reference = mut second`},
		{name: "mut reference permits interior mutation", source: `let box = Box{value: 1}
mut reference = mut box
reference.value = 2`},
		{name: "scalar reference rejects direct referent assignment", source: `let count = 1
mut reference = mut count
reference = 2`, wantError: true},
		{name: "scalar reference rejects compound referent assignment", source: `let count = 1
mut reference = mut count
reference =+ 1`, wantError: true},
		{name: "reference rejects ordinary whole referent assignment", source: `let box = Box{value: 1}
mut reference = mut box
reference = Box{value: 2}`, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, referenceBoxPrelude+tt.source, tt.wantError)
		})
	}
}

func TestADR0057ReferenceDestinationsRequireActualReferences(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "annotated binding rejects ordinary value", source: `let value = Box{value: 1}
let reference: mut Box = value`, wantError: true},
		{name: "annotated binding accepts explicit reference", source: `let value = Box{value: 1}
let reference: mut Box = mut value`},
		{name: "parameter rejects ordinary let value", source: `fn take(value: mut Box) {}
let value = Box{value: 1}
take(value)`, wantError: true},
		{name: "parameter rejects ordinary mut value", source: `fn take(value: mut Box) {}
mut value = Box{value: 1}
take(value)`, wantError: true},
		{name: "parameter accepts explicit reference", source: `fn take(value: mut Box) {}
let value = Box{value: 1}
take(mut value)`},
		{name: "parameter accepts existing reference", source: `fn take(value: mut Box) {}
let value = Box{value: 1}
let reference = mut value
take(reference)`},
		{name: "unannotated binding preserves reference type", source: `fn take(value: mut Box) {}
let value = Box{value: 1}
let reference = mut value
let alias = reference
take(alias)`},
		{name: "return rejects ordinary value", source: `fn take(value: Box) mut Box { value }`, wantError: true},
		{name: "return accepts explicit reference", source: `fn take(value: Box) mut Box { (mut value) }`},
		{name: "field rejects ordinary value", source: `struct Holder { item: mut Box }
let value = Box{value: 1}
let holder = Holder{item: value}`, wantError: true},
		{name: "field accepts explicit reference", source: `struct Holder { item: mut Box }
let value = Box{value: 1}
let holder = Holder{item: mut value}`},
		{name: "list rejects ordinary value", source: `let value = Box{value: 1}
let values: [mut Box] = [value]`, wantError: true},
		{name: "list preserves reference", source: `let value = Box{value: 1}
let reference = mut value
let values: [mut Box] = [reference]`},
		{name: "map rejects ordinary value", source: `let value = Box{value: 1}
let values: [Str: mut Box] = ["box": value]`, wantError: true},
		{name: "map preserves reference", source: `let value = Box{value: 1}
let reference = mut value
let values: [Str: mut Box] = ["box": reference]`},
		{name: "Maybe rejects ordinary value", source: `let value = Box{value: 1}
let maybe: (mut Box)? = Maybe::new(value)`, wantError: true},
		{name: "Maybe preserves reference", source: `let value = Box{value: 1}
let reference = mut value
let maybe: (mut Box)? = Maybe::new(reference)`},
		{name: "Result rejects ordinary value", source: `let value = Box{value: 1}
let result: (mut Box)!Str = Result::ok(value)`, wantError: true},
		{name: "Result preserves reference", source: `let value = Box{value: 1}
let reference = mut value
let result: (mut Box)!Str = Result::ok(reference)`},
		{name: "channel rejects ordinary value at reference element", source: `let value = Box{value: 1}
let channel = Chan::new<mut Box>(1)
channel.send(value)`, wantError: true},
		{name: "channel preserves reference element", source: `let value = Box{value: 1}
let reference = mut value
let channel = Chan::new<mut Box>(1)
channel.send(reference)`},
		{name: "generic inference preserves reference", source: `fn identity(value: $T) $T { value }
fn take(value: mut Box) {}
let value = Box{value: 1}
let reference = mut value
let same = identity(reference)
take(same)`},
		{name: "function value rejects ordinary argument", source: `let callback: fn(mut Box) = fn(value: mut Box) { value.set(2) }
let value = Box{value: 1}
callback(value)`, wantError: true},
		{name: "function value accepts reference argument", source: `let callback: fn(mut Box) = fn(value: mut Box) { value.set(2) }
let value = Box{value: 1}
let reference = mut value
callback(reference)`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, referenceBoxPrelude+tt.source, tt.wantError)
		})
	}
}

func TestADR0057BorrowClassification(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "borrow let local", source: `let value = Box{value: 1}
let reference = mut value`},
		{name: "borrow mut local", source: `mut value = Box{value: 1}
let reference = mut value`},
		{name: "borrow module let", source: `let value = Box{value: 1}
fn borrow() mut Box { (mut value) }`},
		{name: "borrow field through let storage", source: `struct Outer { inner: Box }
let outer = Outer{inner: Box{value: 1}}
let reference = mut outer.inner`},
		{name: "fresh literal", source: `let reference = mut Box{value: 1}`},
		{name: "fresh call result", source: `fn make() Box { Box{value: 1} }
let reference = mut make()`},
		{name: "copy accessor result gets fresh storage", source: `let values = [Box{value: 1}]
let reference = mut values.at(0).expect("item")`},
		{name: "temporary selector is rejected", source: `struct Outer { inner: Box }
fn make() Outer { Outer{inner: Box{value: 1}} }
let reference = mut make().inner`, wantError: true},
		{name: "mut existing reference is idempotent", source: `let value = Box{value: 1}
let reference = mut value
let same: mut Box = mut reference`},
		{name: "Ard owned nested reference type is rejected", source: `let value = Box{value: 1}
let reference = mut value
let nested: mut mut Box = mut reference`, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, referenceBoxPrelude+tt.source, tt.wantError)
		})
	}
}

func TestADR0057ExplicitDereferenceContexts(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "ordinary value operand is rejected", source: `let value = Box{value: 1}
let copy = deref value`, wantError: true},
		{name: "value binding requires deref", source: `let value = Box{value: 1}
let reference = mut value
let copy: Box = reference`, wantError: true},
		{name: "value binding accepts deref", source: `let value = Box{value: 1}
let reference = mut value
let copy: Box = deref reference`},
		{name: "value argument rejects bare reference", source: `fn take(value: Box) {}
let value = Box{value: 1}
let reference = mut value
take(reference)`, wantError: true},
		{name: "value argument accepts deref", source: `fn take(value: Box) {}
let value = Box{value: 1}
let reference = mut value
take(deref reference)`},
		{name: "value return rejects bare reference", source: `fn copy(reference: mut Box) Box { reference }`, wantError: true},
		{name: "value return accepts deref", source: `fn copy(reference: mut Box) Box { deref reference }`},
		{name: "value field rejects bare reference", source: `struct Holder { item: Box }
let value = Box{value: 1}
let reference = mut value
let holder = Holder{item: reference}`, wantError: true},
		{name: "value field accepts deref", source: `struct Holder { item: Box }
let value = Box{value: 1}
let reference = mut value
let holder = Holder{item: (deref reference)}`},
		{name: "value list rejects bare reference", source: `let value = Box{value: 1}
let reference = mut value
let list: [Box] = [reference]`, wantError: true},
		{name: "value Maybe rejects bare reference", source: `let value = Box{value: 1}
let reference = mut value
let maybe: Box? = Maybe::new(reference)`, wantError: true},
		{name: "value container accepts deref", source: `let value = Box{value: 1}
let reference = mut value
let list: [Box] = [(deref reference)]
let maybe: Box? = Maybe::new((deref reference))`},
		{name: "deref is not an assignment place", source: `let value = Box{value: 1}
let reference = mut value
deref reference = Box{value: 2}`, wantError: true},
		{name: "field of deref temporary is not mutable", source: `let value = Box{value: 1}
let reference = mut value
(deref reference).value = 2`, wantError: true},
		{name: "mut deref creates independent top level storage", source: `let value = Box{value: 1}
let reference = mut value
let independent: mut Box = mut deref reference`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, referenceBoxPrelude+tt.source, tt.wantError)
		})
	}
}

func TestADR0057ReferenceObservationAssignmentAndComparability(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "observational reads remain implicit", source: `let box = Box{value: 1}
let reference = mut box
let field = reference.value
let method = reference.get()
let number = 1
let number_reference = mut number
let arithmetic = number_reference + 1
let text = "{number_reference}"
let matched = match number_reference {
  1 => true,
  _ => false,
}`},
		{name: "reference equality and inequality are accepted", source: `let value = 1
let left = mut value
let right = mut value
let equal = left == right
let different = left != right`},
		{name: "reference relational comparison is rejected", source: `let value = 1
let left = mut value
let right = mut value
let ordered = left < right`, wantError: true},
		{name: "reference to noncomparable referent is a map key", source: `let values = [1, 2]
let key = mut values
let table: [mut [Int]: Str] = [key: "values"]`},
		{name: "mutable trait reference supports identity and map keys", source: `trait View { fn get() Int }
impl View for Box { fn get() Int { self.value } }
let value = Box{value: 1}
let first: mut View = mut value
let second = first
let equal = first == second
let table: [mut View: Str] = [first: "value"]
let found = table.get(second)`},
		{name: "ordinary let holder rejects reference field rebinding", source: `struct Holder { item: mut Box }
let first = Box{value: 1}
let second = Box{value: 2}
let holder = Holder{item: mut first}
holder.item = mut second`, wantError: true},
		{name: "ordinary mut holder rejects reference field rebinding", source: `struct Holder { item: mut Box }
let first = Box{value: 1}
let second = Box{value: 2}
mut holder = Holder{item: mut first}
holder.item = mut second`, wantError: true},
		{name: "reference valued field rebinds through referenced holder", source: `struct Holder { item: mut Box }
let first = Box{value: 1}
let second = Box{value: 2}
let holder = mut Holder{item: mut first}
holder.item = mut second`},
		{name: "reference valued field rejects value rhs", source: `struct Holder { item: mut Box }
let first = Box{value: 1}
let holder = mut Holder{item: mut first}
holder.item = Box{value: 2}`, wantError: true},
		{name: "whole list replacement through reference is rejected", source: `let values = [1, 2]
mut reference = mut values
reference = [9]`, wantError: true},
		{name: "sanctioned list mutation through reference", source: `let values = [1, 2]
let reference = mut values
reference.push(3)
reference.set(0, 9)`},
		{name: "sanctioned map mutation through reference", source: `let values = ["a": 1]
let reference = mut values
reference.set("b", 2)
reference.delete("a")`},
		{name: "sanctioned Maybe mutation through reference", source: `let value: Int? = Maybe::new()
let reference = mut value
reference.set(1)
reference.clear()`},
		{name: "channel handle mutates through let", source: `let channel = Chan::new<Int>(1)
channel.send(1)
let value = channel.recv()
channel.close()`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, referenceBoxPrelude+tt.source, tt.wantError)
		})
	}
}

func TestADR0057ClosureReferenceCaptureModes(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "read-only closure copies current reference handle", source: `let value = Box{value: 1}
let reference = mut value
let observe = fn() Int { reference.value }
let result = observe()`},
		{name: "nested closure propagates writable reference slot capture", source: `let first = Box{value: 1}
let second = Box{value: 2}
mut reference = mut first
let make_rebinder = fn() { fn() { reference = mut second } }
let rebind = make_rebinder()
rebind()`},
		{name: "ordinary mutable slot capture remains distinct", source: `mut value = Box{value: 1}
let replace = fn() { value = Box{value: 2} }
replace()`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, referenceBoxPrelude+tt.source, false)
		})
	}
}

func TestADR0057UnsafeCastReferencePolicy(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "concrete reference can be recovered", source: `let value = Box{value: 1}
let reference = mut value
let boxed: Any = reference
let recovered: mut Box = unsafe::cast<mut Box>(boxed).expect("reference")`},
		{name: "concrete pointee can be explicitly materialized", source: `let value = Box{value: 1}
let reference = mut value
let boxed: Any = reference
let copy: Box = unsafe::cast<Box>(boxed).expect("value")`},
		{name: "mutable trait reconstruction is rejected", source: `trait View { fn get() Int }
impl View for Box { fn get() Int { self.value } }
let view: mut View = mut Box{value: 1}
let boxed: Any = view
let recovered = unsafe::cast<mut View>(boxed)`, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, "use ard/unsafe\n"+referenceBoxPrelude+tt.source, tt.wantError)
		})
	}
}

func TestADR0057AsyncReferenceBoundaryIsIntentionallyShallow(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "direct mutating reference capture is rejected", source: `let value = Box{value: 1}
let reference = mut value
async::start(fn() { reference.value = 2 })`, wantError: true},
		{name: "direct read-only reference capture is rejected", source: `let value = Box{value: 1}
let reference = mut value
async::start(fn() { let observed = reference.value })`, wantError: true},
		{name: "explicit borrow of outer storage is rejected", source: `let value = Box{value: 1}
async::start(fn() { let reference = mut value })`, wantError: true},
		{name: "reference hidden in a container is allowed", source: `let value = Box{value: 1}
let references = [mut value]
async::start(fn() { let observed = references.at(0).expect("reference").value })`},
		{name: "reference created inside fiber is allowed", source: `async::start(fn() {
  let value = Box{value: 1}
  let reference = mut value
  reference.value = 2
})`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, "use ard/async\n"+referenceBoxPrelude+tt.source, tt.wantError)
		})
	}
}

func assertReferenceCheckerResult(t *testing.T, source string, wantError bool) {
	t.Helper()
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	if strings.Contains(source, "deref ") && !containsParsedDeref(reflect.ValueOf(result.Program)) {
		t.Fatal("parser did not produce a dereference expression")
	}
	checked := checker.New("test.ard", result.Program, nil)
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

func lastSourceRow(source string) int {
	lines := strings.Split(source, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return len(lines)
}

func containsParsedDeref(value reflect.Value) bool {
	if !value.IsValid() {
		return false
	}
	if value.CanInterface() {
		if expression, ok := value.Interface().(parse.Expression); ok {
			text := fmt.Sprint(expression)
			if strings.HasPrefix(text, "deref ") || strings.HasPrefix(text, "(deref ") {
				return true
			}
		}
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false
		}
		return containsParsedDeref(value.Elem())
	}
	switch value.Kind() {
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			field := value.Field(index)
			if field.CanInterface() && containsParsedDeref(field) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if containsParsedDeref(value.Index(index)) {
				return true
			}
		}
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			if containsParsedDeref(iterator.Key()) || containsParsedDeref(iterator.Value()) {
				return true
			}
		}
	}
	return false
}
