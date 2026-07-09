package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestFixedArrayTypes(t *testing.T) {
	run(t, []test{
		{
			name:  "fixed array annotation contextually types literal",
			input: `let rgb: [Byte; 3] = [255, 0, 0]`,
		},
		{
			name: "fixed arrays can be wrapped as Any",
			input: `let xs: [Int; 2] = [1, 2]
let value: Any = xs`,
		},
		{
			name: "generic fixed array element types resolve",
			input: `fn first(arr: [$T; 2]) $T {
  arr.at(0).expect("first")
}

let value: Int = first<Int>([1, 2])`,
		},
		{
			name: "length participates in type identity",
			input: `let a: [Byte; 3] = [1, 2, 3]
let b: [Byte; 2] = a`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected [Byte; 2], got [Byte; 3]"},
			},
		},
		{
			name:  "zero length fixed array literal",
			input: `let empty: [Byte; 0] = []`,
		},
		{
			name: "fixed arrays can be iterated",
			input: `let xs: [Int; 3] = [1, 2, 3]
for x in xs {
  let y: Int = x
}`,
		},
		{
			name: "fixed array iteration cursor is immutable",
			input: `mut xs: [Int; 3] = [1, 2, 3]
for x in xs {
  x = 4
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Immutable variable: x"},
			},
		},
		{
			name:  "nested fixed array literals",
			input: `let grid: [[Byte; 2]; 3] = [[1, 2], [3, 4], [5, 6]]`,
		},
		{
			name:  "literal length must match expected fixed array",
			input: `let rgb: [Byte; 3] = [255, 0]`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected 3 elements, got 2"},
			},
		},
		{
			name:  "zero length fixed array rejects elements",
			input: `let empty: [Byte; 0] = [1]`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected 0 elements, got 1"},
			},
		},
	})
}
