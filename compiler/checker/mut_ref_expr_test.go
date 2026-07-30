package checker_test

import "testing"

// TestMutRefExpressions keeps the original ADR 0045 regression surface aligned
// with ADR 0057. The exhaustive capability matrix lives in
// reference_semantics_0057_test.go.
func TestMutRefExpressions(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantError bool
	}{
		{
			name: "let storage can be explicitly referenced",
			source: `let counter = 0
let reference: mut Int = mut counter`,
		},
		{
			name: "mut on an existing reference is idempotent",
			source: `let counter = 0
let reference = mut counter
let again: mut Int = mut reference`,
		},
		{
			name: "unannotated binding preserves the reference",
			source: `let counter = 0
let reference = mut counter
let alias: mut Int = reference`,
		},
		{
			name: "ordinary mut binding does not implicitly satisfy reference parameter",
			source: `fn take(value: mut Int) {}
mut counter = 0
take(counter)`,
			wantError: true,
		},
		{
			name: "explicit reference satisfies reference parameter",
			source: `fn take(value: mut Int) {}
let counter = 0
take(mut counter)`,
		},
		{
			name: "reference does not implicitly materialize a value",
			source: `let counter = 0
let reference = mut counter
let copy: Int = reference`,
			wantError: true,
		},
		{
			name: "explicit deref materializes a value",
			source: `let counter = 0
let reference = mut counter
let copy: Int = deref reference`,
		},
		{
			name: "fresh literal storage remains supported",
			source: `struct Person { age: Int }
let reference = mut Person{age: 30}
reference.age = 99`,
		},
		{
			name: "writable reference slot can rebind",
			source: `let first = 0
let second = 1
mut reference = mut first
reference = mut second`,
		},
		{
			name: "whole list write through reference is rejected",
			source: `let items = [1, 2]
let reference = mut items
reference = [9, 9]`,
			wantError: true,
		},
		{
			name: "temporary selector remains nonaddressable",
			source: `struct Inner { n: Int }
struct Outer { inner: Inner }
fn make() Outer { Outer{inner: Inner{n: 0}} }
let reference = mut make().inner`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertReferenceCheckerResult(t, tt.source, tt.wantError)
		})
	}
}
