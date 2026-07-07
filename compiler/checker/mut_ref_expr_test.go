package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestMutRefExpressions(t *testing.T) {
	run(t, []test{
		{
			name: "mut ref of a mutable binding",
			input: `mut counter = 0
let r = mut counter`,
		},
		{
			name: "mut ref binding has the reference type",
			input: `mut counter = 0
let r: mut Int = mut counter`,
		},
		{
			name:  "mut ref of an immutable binding is rejected",
			input: `let counter = 0
let r = mut counter`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Cannot take a mutable reference to immutable 'counter'"}},
		},
		{
			name: "mut ref of a mut ref aliases the same referent",
			input: `mut counter = 0
let r = mut counter
let again: mut Int = mut r`,
		},
		{
			name: "binding a reference without mut copies the referent",
			input: `mut counter = 0
let r = mut counter
let copy: Int = r`,
		},
		{
			name: "explicit mut argument satisfies a mut parameter",
			input: `struct Person { age: Int }
fn update(person: mut Person) {
  person.age = 99
}
mut alice = Person{age: 30}
update(mut alice)`,
		},
		{
			name: "implicit passing to a mut parameter remains valid",
			input: `struct Person { age: Int }
fn update(person: mut Person) {
  person.age = 99
}
mut alice = Person{age: 30}
update(alice)`,
		},
		{
			name: "explicit mut argument of an immutable binding is rejected",
			input: `struct Person { age: Int }
fn update(person: mut Person) {
  person.age = 99
}
let alice = Person{age: 30}
update(mut alice)`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Cannot take a mutable reference to immutable 'alice'"}},
		},
		{
			name: "mut ref of a struct literal creates fresh storage",
			input: `struct Person { age: Int }
fn update(person: mut Person) {
  person.age = 99
}
update(mut Person{age: 30})`,
		},
		{
			name: "fresh storage can be bound",
			input: `struct Person { age: Int }
let r = mut Person{age: 30}
r.age = 99`,
		},
		{
			name: "mut ref of a field through a mutable binding",
			input: `struct Inner { n: Int }
struct Outer { inner: Inner }
fn bump(inner: mut Inner) {
  inner.n =+ 1
}
mut o = Outer{inner: Inner{n: 0}}
bump(mut o.inner)`,
		},
		{
			name: "mut ref of a field through an immutable binding is rejected",
			input: `struct Inner { n: Int }
struct Outer { inner: Inner }
let o = Outer{inner: Inner{n: 0}}
let r = mut o.inner`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Cannot take a mutable reference to immutable 'o.inner'"}},
		},
		{
			name: "mut ref of a mut field aliases the same referent",
			input: `struct Tree {}
struct Ctx { tree: mut Tree }
mut tree = Tree{}
let ctx = Ctx{tree: tree}
let r: mut Tree = mut ctx.tree`,
		},
		{
			name: "alias writes are visible through the original binding",
			input: `mut counter = 0
let r = mut counter
r =+ 1
let snapshot: Int = r`,
		},
		{
			name: "references cannot be rebound",
			input: `mut counter = 0
mut other = 10
let r = mut counter
r = mut other`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "References cannot be rebound; assign the value directly"}},
		},
		{
			name: "whole-value writes through a descriptor reference are rejected",
			input: `mut items = [1, 2]
let r = mut items
r = [9, 9]`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Cannot assign a new value through 'r': element writes share storage, but the referent binding is not reachable. Assign to the original binding instead"}},
		},
		{
			name: "immutable place diagnostic stays readable for complex subjects",
			input: `struct Inner { n: Int }
struct Outer { inner: Inner }
fn make() Outer {
  Outer{inner: Inner{n: 0}}
}
let r = mut make().inner`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Cannot take a mutable reference to immutable 'value.inner'"}},
		},
	})
}
