package vm_test

import "testing"

func TestCopySemantics(t *testing.T) {
	runTests(t, []test{
		{
			name: "mut assignment creates a copy - modifying copy doesn't affect original",
			input: `
				struct Person {
					name: Str,
					age: Int
				}
				let alice = Person { name: "Alice", age: 30 }
				mut bob = alice
				bob.age = 31
				"{alice.age} -- {bob.age}`, // alice.age should remain unchanged after copy semantics
			want: "30 -- 31",
		},
		{
			name: "mut assignment from mut variable still creates copy",
			input: `
				struct Person {
					name: Str,
					age: Int
				}
				mut alice = Person { name: "Alice", age: 30 }
				alice.age = 28
				mut bob = alice
				bob.age =+ 1
				"{alice.age} - {bob.age}`, // alice.age should remain unchanged after copy semantics
			want: "28 - 29",
		},
		{
			name: "`mut` can be used to copy and make a mutable argument",
			input: `
			struct Person { age: Int }
			fn grow(mut p: Person) {
			  p.age =+ 1
			}

			let alice = Person{age: 30}
			grow(mut alice)
			alice.age == 30
			`,
			want: true,
		},
	})
}
