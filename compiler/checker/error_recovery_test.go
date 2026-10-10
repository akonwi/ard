package checker_test

import (
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
)

// An undefined variable must not stop the checker from reporting later
// errors (#523).
func TestUndefinedVariableDoesNotHaltChecking(t *testing.T) {
	run(t, []test{
		{
			name: "later top-level declarations are still checked",
			input: strings.Join([]string{
				`fn one() {`,
				`  let a = missing`,
				`}`,
				`fn two() {`,
				`  let b: Int = "two"`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
			},
		},
		{
			name: "later statements in the same body are still checked",
			input: strings.Join([]string{
				`fn main() {`,
				`  let a = missing`,
				`  let b: Int = "two"`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
			},
		},
		{
			name: "later top-level statements are still checked",
			input: strings.Join([]string{
				`let a = missing`,
				`let b: Int = "two"`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
			},
		},
		{
			name: "every undefined variable is reported",
			input: strings.Join([]string{
				`fn one() {`,
				`  let a = first`,
				`}`,
				`fn two() {`,
				`  let b = second`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: first"},
				{Kind: checker.Error, Message: "Undefined variable: second"},
			},
		},
	})
}

// A struct field value that fails to check is reported once, even though the
// checker retries it for mutable-reference auto-borrowing (#523).
func TestStructFieldUndefinedValueReportedOnce(t *testing.T) {
	run(t, []test{
		{
			name: "explicit field value",
			input: strings.Join([]string{
				`struct Person {`,
				`  name: Str,`,
				`}`,
				`let p = Person{name: missing}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "undefined function call as field value",
			input: strings.Join([]string{
				`struct Person {`,
				`  name: Str,`,
				`}`,
				`let p = Person{name: missing_fn()}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined function: missing_fn"},
			},
		},
		{
			name: "field value of a later struct literal is still checked",
			input: strings.Join([]string{
				`struct Person {`,
				`  name: Str,`,
				`}`,
				`let p = Person{name: missing}`,
				`let q = Person{name: 1}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
			},
		},
	})
}

// A block whose final value expression failed to check has no meaningful
// type, so it must not produce a follow-on return or branch mismatch (#522).
func TestFailedFinalExpressionDoesNotCascade(t *testing.T) {
	run(t, []test{
		{
			name: "undefined variable as function result",
			input: strings.Join([]string{
				`fn f() Int {`,
				`  missing`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "chained equality as function result",
			input: strings.Join([]string{
				`fn f(a: Int, b: Int) Bool {`,
				`  a > 0 != (b > 0)`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "equality operators cannot be chained"},
			},
		},
		{
			name: "undefined function as function result",
			input: strings.Join([]string{
				`fn f() Str {`,
				`  missing_fn(1)`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined function: missing_fn"},
			},
		},
		{
			name: "failed argument in a Result constructor result",
			input: strings.Join([]string{
				`fn f() Int!Str {`,
				`  Result::ok(missing)`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "failed final expression after an earlier expression statement",
			input: strings.Join([]string{
				`fn f() Int {`,
				`  "unused"`,
				`  missing`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "closure result",
			input: strings.Join([]string{
				`fn main() {`,
				`  let g = fn() Int { missing }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "method result",
			input: strings.Join([]string{
				`struct Counter {`,
				`  count: Int,`,
				`}`,
				`impl Counter {`,
				`  fn next() Int {`,
				`    missing`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "match arm result",
			input: strings.Join([]string{
				`fn f(flag: Bool) Int {`,
				`  match flag {`,
				`    true => missing,`,
				`    false => 1,`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "real result mismatch is still reported",
			input: strings.Join([]string{
				`fn f() Int {`,
				`  "str"`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
			},
		},
	})
}

// A binding whose initializer failed still introduces its name, so later uses
// do not report it as undefined (#525).
func TestFailedBindingDoesNotCascade(t *testing.T) {
	run(t, []test{
		{
			name: "uses of an unannotated failed binding are silent",
			input: strings.Join([]string{
				`fn f(n: Int) Int {`,
				`  let x = missing`,
				`  let y: Int = x`,
				`  let z = x + 1`,
				`  let s = x.size()`,
				`  x.field`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "failed mutable binding can still be assigned",
			input: strings.Join([]string{
				`fn main() {`,
				`  mut x = missing`,
				`  x = 1`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "failed binding called as a function is silent",
			input: strings.Join([]string{
				`fn main() {`,
				`  let g = missing`,
				`  g(1)`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "try on a failed value",
			input: strings.Join([]string{
				`fn f() Int!Str {`,
				`  let x = try missing`,
				`  Result::ok(x)`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "annotated failed binding keeps its declared type",
			input: strings.Join([]string{
				`fn main() {`,
				`  let b: Int = "two"`,
				`  let c: Str = b`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
			},
		},
		{
			name: "failed top-level binding",
			input: strings.Join([]string{
				`let x = missing`,
				`fn f() Int {`,
				`  x`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "failed generic struct field does not report an unresolved generic",
			input: strings.Join([]string{
				`struct Box<$T> {`,
				`  value: $T,`,
				`}`,
				`fn main() {`,
				`  let b = Box{value: missing}`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
	})
}

// An untyped empty collection is an ordinary recoverable failure: it does not
// halt checking and its binding does not leak a placeholder type (#525).
func TestUntypedEmptyCollectionRecovery(t *testing.T) {
	run(t, []test{
		{
			name: "empty list does not halt checking",
			input: strings.Join([]string{
				`fn main() {`,
				`  let xs = []`,
				`  let n: Int = "two"`,
				`}`,
				`fn later() {`,
				`  let m: Str = 1`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Empty lists need an explicit type"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
			},
		},
		{
			name: "empty map does not halt checking",
			input: strings.Join([]string{
				`fn main() {`,
				`  let m = [:]`,
				`  let n: Int = "two"`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Empty maps need an explicit type"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"},
			},
		},
		{
			name: "uses of an untyped empty list are silent",
			input: strings.Join([]string{
				`fn total(xs: [Int]) Int {`,
				`  xs.size()`,
				`}`,
				`fn main() {`,
				`  let xs = []`,
				`  total(xs)`,
				`  let first: Int = xs.at(0).or(0)`,
				`  for x in xs {`,
				`    let y: Int = x`,
				`  }`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Empty lists need an explicit type"},
			},
		},
	})
}

// The error type never escapes into a composite type, such as a closure whose
// return type is inferred from a failed body (#525).
func TestErrorTypeDoesNotLeakIntoCompositeTypes(t *testing.T) {
	run(t, []test{
		{
			name: "closure with a failed body passed to a generic method",
			input: strings.Join([]string{
				`fn f(res: Int!Str) {`,
				`  let x = res.map(fn(value) { missing })`,
				`  let y: Str = x`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "closure with a failed body bound without an annotation",
			input: strings.Join([]string{
				`fn main() {`,
				`  let g = fn() { missing }`,
				`  let n: Int = g()`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
		{
			name: "list of a failed binding",
			input: strings.Join([]string{
				`fn main() {`,
				`  let x = missing`,
				`  let xs = [x]`,
				`  let n: Str = xs`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: missing"},
			},
		},
	})
}
