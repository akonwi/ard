package vm_test

import "testing"

func TestStructs(t *testing.T) {
	runTests(t, []test{
		{
			name: "Struct usage",
			input: `
				struct Point {
					x: Int,
					y: Int,
				}

				impl Point {
					fn print() Str {
						"{@x.to_str()},{@y.to_str()}"
					}
				}

				let p = Point { x: 10, y: 20 }
				p.print()`,
			want: "10,20",
		},
		{
			name: "Reassigning struct properties",
			input: `
				struct Point {
					x: Int,
					y: Int,
				}
				mut p = Point { x: 10, y: 20 }
				p.x = 30
				p.x`,
			want: 30,
		},
	})
}

func TestStaticFunctions(t *testing.T) {
	runTests(t, []test{
		{
			name: "Struct usage",
			input: `
				struct Point {
					x: Int,
					y: Int,
				}
				fn Point::make(x: Int, y: Int) Point {
					Point { x: x, y: y }
				}
				let p = Point::make(10, 20)
				p.x`,
			want: 10,
		},
		{
			name: "deeply nested",
			input: `
				use ard/http
				let res = http::Response::new(200, "ok")
				res.status`,
			want: 200,
		},
	})
}

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
				"{alice.age} - {bob.age}`, // alice.age should remain unchanged after copy semantics
			want: "30 - 31",
		},
		{
			name: "mut assignment from mut variable still creates copy",
			input: `
				struct Person {
					name: Str,
					age: Int
				}
				mut alice = Person { name: "Alice", age: 30 }
				mut bob = alice
				bob.age = 31
				"{alice.age} - {bob.age}`, // alice.age should remain unchanged after copy semantics
			want: "30 - 31",
		},
	})
}

func TestCopySemanticsAllTypes(t *testing.T) {
	runTests(t, []test{
		{
			name: "copy semantics work with primitive types",
			input: `
				fn modify_int(mut x: Int) {
					x = 999
				}
				let original = 42
				modify_int(original)
				original`,
			want: 42, // original should remain unchanged
		},
		{
			name: "copy semantics work with string types",
			input: `
				fn modify_str(mut s: Str) {
					s = "modified"
				}
				let original = "original"
				modify_str(original)
				original`,
			want: "original", // original should remain unchanged
		},
		{
			name: "copy semantics work with lists",
			input: `
				fn modify_list(mut lst: [Int]) {
					lst.push(999)
				}
				let original = [1, 2, 3]
				modify_list(original)
				original.size()`,
			want: 3, // original list size should remain unchanged
		},
	})
}
