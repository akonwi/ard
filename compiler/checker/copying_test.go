package checker_test

import "testing"

func TestMutableReferenceFields(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{
			name: "immutable struct can expose actual reference field",
			source: `
struct Tree { count: Int }
struct Context { tree: mut Tree }
fn bump(tree: mut Tree) { tree.count =+ 1 }
let tree = Tree{count: 0}
let ctx = Context{tree: mut tree}
bump(ctx.tree)
`,
		},
		{
			name: "reference field rejects ordinary value",
			source: `
struct Tree { count: Int }
struct Context { tree: mut Tree }
let tree = Tree{count: 0}
let ctx = Context{tree: tree}
`,
			wantError: true,
		},
		{
			name: "immutable holder cannot rebind reference field",
			source: `
struct Tree { count: Int }
struct Context { tree: mut Tree }
let tree = Tree{count: 0}
let other = Tree{count: 1}
let ctx = Context{tree: mut tree}
ctx.tree = mut other
`,
			wantError: true,
		},
		{
			name: "reference to holder can rebind reference field",
			source: `
struct Tree { count: Int }
struct Context { tree: mut Tree }
let tree = Tree{count: 0}
let other = Tree{count: 1}
let ctx = mut Context{tree: mut tree}
ctx.tree = mut other
`,
		},
		{
			name: "reference field permits nested pointee mutation",
			source: `
struct Tree { count: Int }
struct Context { tree: mut Tree }
let tree = Tree{count: 0}
let ctx = Context{tree: mut tree}
ctx.tree.count = 2
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, tt.source, tt.wantError)
		})
	}
}

func TestMutableReferenceParameters(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{
			name: "ordinary mut binding is rejected without explicit reference",
			source: `
struct Person { age: Int }
fn grow(person: mut Person) { person.age =+ 1 }
mut joe = Person{age: 20}
grow(joe)
`,
			wantError: true,
		},
		{
			name: "ordinary let binding is rejected without explicit reference",
			source: `
struct Person { age: Int }
fn grow(person: mut Person) { person.age =+ 1 }
let joe = Person{age: 20}
grow(joe)
`,
			wantError: true,
		},
		{
			name: "explicit reference to let storage is accepted",
			source: `
struct Person { age: Int }
fn grow(person: mut Person) { person.age =+ 1 }
let joe = Person{age: 20}
grow(mut joe)
`,
		},
		{
			name: "existing reference is accepted",
			source: `
struct Person { age: Int }
fn grow(person: mut Person) { person.age =+ 1 }
let joe = Person{age: 20}
let reference = mut joe
grow(reference)
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, tt.source, tt.wantError)
		})
	}
}
