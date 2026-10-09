package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

const pointerPrelude = `
struct Profile {
  name: Str,
}

struct User {
  name: Str,
  profile: Profile,
  tags: [Str],
}

struct Box {
  value: Int,
}

impl Box {
  fn get() Int { self.value }
  fn mut set(value: Int) { self.value = value }
}

trait View {
  fn value() Int
  fn mut bump()
}

impl View for Box {
  fn value() Int { self.value }
  fn mut bump() { self.value = self.value + 1 }
}

fn new_user() User { User{name: "Ada", profile: Profile{name: "Ada"}, tags: []} }
`

func TestADR0073PointerTypes(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "read-only pointer parameter reads through", source: `fn show(user: *User) Str { user.name }`},
		{name: "writable pointer parameter writes through", source: `fn rename(user: *mut User) { user.name = "Grace" }`},
		{name: "read-only pointer parameter rejects field write", source: `fn rename(user: *User) { user.name = "Grace" }`, wantError: true},
		{name: "read-only pointer rejects nested field write", source: `fn rename(user: *User) { user.profile.name = "Grace" }`, wantError: true},
		{name: "read-only pointer parameter rejects mutating method", source: `fn bump(box: *Box) { box.set(2) }`, wantError: true},
		{name: "read-only pointer parameter allows read method", source: `fn read(box: *Box) Int { box.get() }`},
		{name: "read-only list pointer rejects push", source: `fn add(xs: *[Int]) { xs.push(1) }`, wantError: true},
		{name: "read-only list pointer allows size", source: `fn count(xs: *[Int]) Int { xs.size() }`},
		{name: "writable list pointer allows push", source: `fn add(xs: *mut [Int]) { xs.push(1) }`},
		{name: "pointer to trait is rejected", source: `fn show(view: *View) {}`, wantError: true},
		{name: "writable pointer to trait is rejected", source: `fn show(view: *mut View) {}`, wantError: true},
		{name: "mutable trait value is accepted", source: `fn show(view: mut View) { view.bump() }`},
		{name: "pointer field in struct", source: `struct Node {
  value: Int,
  parent: *Node?,
}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, pointerPrelude+tt.source, tt.wantError)
		})
	}
}

func TestADR0073AddressOf(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "read-only address of let binding", source: `let user = new_user()
let reader: *User = &user`},
		{name: "read-only address of mut binding", source: `mut user = new_user()
let reader: *User = &user`},
		{name: "writable address of mut binding", source: `mut user = new_user()
let writer: *mut User = &mut user`},
		{name: "writable address of let binding is rejected", source: `let user = new_user()
let writer = &mut user`, wantError: true},
		{name: "read-only address is not writable", source: `let user = new_user()
let writer: *mut User = &user`, wantError: true},
		{name: "writable address of fresh value", source: `let writer: *mut User = &mut new_user()`},
		{name: "writable address of struct literal", source: `let writer: *mut Box = &mut Box{value: 1}`},
		{name: "writable address of field of mut binding", source: `mut user = new_user()
let writer: *mut Profile = &mut user.profile`},
		{name: "writable address of field of let binding is rejected", source: `let user = new_user()
let writer = &mut user.profile`, wantError: true},
		{name: "writable address of field through writable pointer", source: `let user = &mut new_user()
let writer: *mut Profile = &mut user.profile`},
		{name: "writable address of field through read-only pointer is rejected", source: `let user = &new_user()
let writer = &mut user.profile`, wantError: true},
		{name: "read-only address of field through read-only pointer", source: `let user = &new_user()
let reader: *Profile = &user.profile`},
		{name: "writable pointer coerces to read-only", source: `let writer = &mut new_user()
let reader: *User = writer`},
		{name: "read-only pointer does not coerce to writable", source: `let reader = &new_user()
let writer: *mut User = reader`, wantError: true},
		{name: "address of pointer is rejected", source: `mut writer = &mut new_user()
let nested = &writer`, wantError: true},
		{name: "writable address of pointer is rejected", source: `mut writer = &mut new_user()
let nested = &mut writer`, wantError: true},
		{name: "writable address of trait-typed place is rejected", source: `mut view: View = Box{value: 1}
let pointer = &mut view`, wantError: true},
		{name: "writable pointer widens to mutable trait", source: `let box = &mut Box{value: 1}
let view: mut View = box
view.bump()`},
		{name: "pointers compare by identity", source: `let box = &mut Box{value: 1}
let reader: *Box = box
let same = box == reader`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, pointerPrelude+tt.source, tt.wantError)
		})
	}
}

func TestADR0073PointerArguments(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "read-only parameter accepts read-only pointer", source: `fn show(user: *User) {}
let user = new_user()
show(&user)`},
		{name: "read-only parameter accepts writable pointer", source: `fn show(user: *User) {}
mut user = new_user()
show(&mut user)`},
		{name: "read-only parameter rejects value", source: `fn show(user: *User) {}
let user = new_user()
show(user)`, wantError: true},
		{name: "writable parameter rejects read-only pointer", source: `fn rename(user: *mut User) {}
let user = new_user()
rename(&user)`, wantError: true},
		{name: "writable parameter accepts writable pointer", source: `fn rename(user: *mut User) {}
mut user = new_user()
rename(&mut user)`},
		{name: "generic pointer parameter infers from writable pointer", source: `fn read(pointer: *$T) $T { pointer.* }
let box: Box = read(&mut Box{value: 1})`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, pointerPrelude+tt.source, tt.wantError)
		})
	}
}

func TestADR0073Dereference(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "dereference read-only pointer", source: `let box = Box{value: 1}
let copy: Box = (&box).*`},
		{name: "dereference writable pointer", source: `let box = &mut Box{value: 1}
let copy: Box = box.*`},
		{name: "dereference value is rejected", source: `let box = Box{value: 1}
let copy = box.*`, wantError: true},
		{name: "dereference mutable trait is rejected", source: `let view: mut View = &mut Box{value: 1}
let copy = view.*`, wantError: true},
		{name: "replace whole pointee through writable pointer", source: `let box = &mut Box{value: 1}
box.* = Box{value: 2}`},
		{name: "replace whole pointee through read-only pointer is rejected", source: `let box = &Box{value: 1}
box.* = Box{value: 2}`, wantError: true},
		{name: "write field through dereferenced writable pointer", source: `let box = &mut Box{value: 1}
box.*.value = 2`},
		{name: "legacy dereference is not assignable", source: `let box = &mut Box{value: 1}
box.@ = Box{value: 2}`, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, pointerPrelude+tt.source, tt.wantError)
		})
	}
}

func TestADR0073BindingInlineStorage(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "mut binding allows field write", source: `mut box = Box{value: 1}
box.value = 2`},
		{name: "mut binding allows nested field write", source: `mut user = new_user()
user.profile.name = "Grace"`},
		{name: "let binding rejects field write", source: `let box = Box{value: 1}
box.value = 2`, wantError: true},
		{name: "mut binding rejects mutating method", source: `mut box = Box{value: 1}
box.set(2)`, wantError: true},
		{name: "mut binding mutating method through explicit pointer", source: `mut box = Box{value: 1}
(&mut box).set(2)`},
		{name: "mut binding rejects list push", source: `mut xs = [1, 2]
xs.push(3)`, wantError: true},
		{name: "mut binding allows list replacement", source: `mut xs = [1, 2]
xs = [3]`},
		{name: "mut binding allows list field replacement", source: `mut user = new_user()
user.tags = ["a"]`},
		{name: "mut binding rejects list field push", source: `mut user = new_user()
user.tags.push("a")`, wantError: true},
		{name: "pointer field writes through let binding", source: `struct Holder {
  box: *mut Box,
}
let holder = Holder{box: &mut Box{value: 1}}
holder.box.value = 2`},
		{name: "pointer field slot is not writable through let binding", source: `struct Holder {
  box: *mut Box,
}
let holder = Holder{box: &mut Box{value: 1}}
holder.box = &mut Box{value: 2}`, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, pointerPrelude+tt.source, tt.wantError)
		})
	}
}

func TestADR0073PointerTypeDisplay(t *testing.T) {
	source := pointerPrelude + `let reader = &new_user()
let value: Int = reader`
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	checked := checker.New("test.ard", result.Program, nil)
	checked.Check()
	for _, diagnostic := range checked.Diagnostics() {
		for _, label := range append([]checker.DiagnosticLabel{diagnostic.Primary}, diagnostic.Secondary...) {
			if label.Message == "this expression has type `*User`" {
				return
			}
		}
	}
	t.Fatalf("expected the read-only pointer type to display as `*User`, got %#v", checked.Diagnostics())
}

func TestADR0073GoPointerBoundaries(t *testing.T) {
	root := t.TempDir()
	writeADR0057GoBoundaryPackage(t, root)
	resolver := checker.NewGoPackagesResolver(root, nil)

	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "Go pointer parameter accepts writable address of mut binding", source: `mut item = ffi::Item{N: 1}
ffi::TakePtr(&mut item)`},
		{name: "Go pointer parameter rejects read-only address", source: `let item = ffi::Item{N: 1}
ffi::TakePtr(&item)`, wantError: true},
		{name: "writable address of let foreign value is rejected", source: `let item = ffi::Item{N: 1}
ffi::TakePtr(&mut item)`, wantError: true},
		{name: "Go pointer result coerces to read-only pointer", source: `let reader: *ffi::Item = ffi::ItemPtr()
let n = reader.N`},
		{name: "read-only foreign pointer rejects pointer receiver method", source: `let reader: *ffi::Item = ffi::ItemPtr()
reader.Bump()`, wantError: true},
		{name: "read-only foreign pointer rejects pointer receiver method value", source: `let reader: *ffi::Item = ffi::ItemPtr()
let bump = reader.Bump`, wantError: true},
		{name: "read-only foreign pointer rejects field write", source: `let reader: *ffi::Item = ffi::ItemPtr()
reader.N = 2`, wantError: true},
		{name: "read-only foreign pointer does not satisfy writable parameter", source: `let reader: *ffi::Item = ffi::ItemPtr()
ffi::TakePtr(reader)`, wantError: true},
		{name: "writable foreign pointer calls pointer receiver method", source: `let writer = ffi::ItemPtr()
writer.Bump()`},
		{name: "mut binding writes foreign value field", source: `mut item = ffi::Item{N: 1}
item.N = 2`},
		{name: "Go slice parameter accepts let list", source: `let values = [1, 2]
ffi::TakeSlice(values)`},
		{name: "Go slice parameter accepts list literal", source: `ffi::TakeSlice([1, 2])`},
		{name: "Go slice parameter types empty literal", source: `ffi::TakeSlice([])`},
		{name: "Go slice parameter accepts writable pointer", source: `mut values = [1, 2]
ffi::TakeSlice(&mut values)`},
		{name: "Go slice parameter accepts read-only pointer", source: `let values = [1, 2]
ffi::TakeSlice(&values)`},
		{name: "Go slice parameter accepts Slice view", source: `let view = [1, 2].slice().expect("bounds")
ffi::TakeSlice(view)`},
		{name: "Go map parameter accepts let map", source: `let values = ["a": 1]
ffi::TakeMap(values)`},
		{name: "Go map parameter accepts map literal", source: `ffi::TakeMap(["a": 1])`},
		{name: "named Go slice parameter accepts list", source: `let values = [1, 2]
ffi::TakeNumbers(values)`},
		{name: "named Go slice parameter types empty literal", source: `ffi::TakeNumbers([])`},
		{name: "named Go map parameter accepts map", source: `let values = ["a": 1]
ffi::TakeScores(values)`},
		{name: "Go slice function value accepts list", source: `let values = [1, 2]
let take = ffi::TakeSlice
take(values)`},
		{name: "Go slice method accepts list", source: `let values = [1, 2]
let sink = ffi::Sink{}
sink.Take(values)`},
		{name: "slice shaped generic accepts list", source: `let values = [1, 2]
let size = ffi::SliceSize(values)`},
		{name: "map shaped generic accepts map", source: `let values = ["a": 1]
let size = ffi::MapSize(values)`},
		{name: "explicit descriptor generic accepts list", source: `let values = [1, 2]
let size = ffi::MixedSize<[Int], Int>(values)`},
		{name: "Go pointer to slice still requires a pointer", source: `let values = [1, 2]
ffi::TakeSlicePtr(values)`, wantError: true},
		{name: "Go pointer to slice accepts writable pointer", source: `mut values = [1, 2]
ffi::TakeSlicePtr(&mut values)`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := "use go:example.com/app/ffi\n\n" + tt.source
			assertGoReferenceCheckerResult(t, source, resolver, tt.wantError)
		})
	}
}
