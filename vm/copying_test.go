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
			fn grow(mut p: Person) {}

			let alice = Person{age: 30}
			grow(mut alice)
			alice.age == 30
			`,
			want: true,
		},
	})
}

// func TestCopySemanticsAllTypes(t *testing.T) {
// 	runTests(t, []test{
// 		{
// 			name: "copy semantics work with primitive types",
// 			input: `
// 				fn modify_int(mut x: Int) {
// 					x = 999
// 				}
// 				let original = 42
// 				modify_int(original)
// 				original`,
// 			want: 42, // original should remain unchanged
// 		},
// 		{
// 			name: "copy semantics work with string types",
// 			input: `
// 				fn modify_str(mut s: Str) {
// 					s = "modified"
// 				}
// 				let original = "original"
// 				modify_str(original)
// 				original`,
// 			want: "original", // original should remain unchanged
// 		},
// 		{
// 			name: "copy semantics work with lists",
// 			input: `
// 				fn modify_list(mut lst: [Int]) {
// 					lst.push(999)
// 				}
// 				let original = [1, 2, 3]
// 				modify_list(original)
// 				original.size()`,
// 			want: 3, // original list size should remain unchanged
// 		},
// 	})
// }
