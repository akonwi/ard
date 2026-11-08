package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

// ADR 0073 removed the ADR 0057 reference spellings. Each use is rejected
// once, with its replacement, and the rest of the program checks as if the
// replacement were written.
func TestRemovedPointerSyntaxIsRejected(t *testing.T) {
	const prelude = `struct Box { value: Int }
trait View {
  fn value() Int
}
impl View for Box {
  fn value() Int { self.value }
}
`
	const (
		pointerType = "Removed pointer type syntax: write `&mut T` instead of `mut T`"
		addressOf   = "Removed pointer syntax: write `&mut expression` instead of `mut expression`"
		redundant   = "Removed pointer syntax: `mut` on a pointer"
		redundantTy = "Removed pointer type syntax: `mut` on a pointer type"
		traitBorrow = "Removed pointer syntax: `mut` on a trait value"
	)
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{name: "pointer syntax is current", source: `fn reset(box: &mut Box) { box.value = 0 }
fn main() {
  mut box = Box{value: 1}
  reset(&mut box)
  let copy: Box = (&box).*
}`},
		{name: "mut Trait is current", source: `fn show(view: mut View) Int { view.value() }`},
		{name: "mut Trait function type parameter is current", source: `fn apply(f: fn(mut View) Int) {}`},
		{name: "parameter type", source: `fn reset(box: mut Box) { box.value = 0 }`, want: []string{pointerType}},
		{name: "function type parameter", source: `fn apply(f: fn(mut Box) Void) {}`, want: []string{pointerType}},
		{name: "pointer type", source: `fn reset(box: mut &mut Box) { box.value = 0 }`, want: []string{redundantTy}},
		{name: "binding annotation", source: `fn main() {
  let box: mut Box = &mut Box{value: 1}
}`, want: []string{pointerType}},
		{name: "borrow", source: `fn main() {
  let box = mut Box{value: 1}
}`, want: []string{addressOf}},
		{name: "redundant borrow", source: `fn main() {
  let box = &mut Box{value: 1}
  let again = mut box
}`, want: []string{redundant}},
		{name: "trait place borrow", source: `fn main() {
  mut view: View = Box{value: 1}
  let borrowed = mut view
}`, want: []string{traitBorrow}},
		{name: "repeated resolution reports once", source: `fn reset(box: mut Box) { box.value = 0 }
fn main() {
  reset(&mut Box{value: 1})
  reset(&mut Box{value: 2})
}`, want: []string{pointerType}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(prelude+tt.source), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			c := checker.New("test.ard", result.Program, nil)
			c.Check()
			var got []string
			for _, diagnostic := range c.Diagnostics() {
				if diagnostic.Kind != checker.Error || diagnostic.Code != checker.DiagnosticCodeRemovedPointerSyntax {
					t.Fatalf("unexpected diagnostic: %s %s", diagnostic.Kind, diagnostic.Message)
				}
				got = append(got, diagnostic.Message)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("diagnostics mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
