package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// Legacy ADR 0057 reference syntax keeps working during the ADR 0073
// migration and reports one deprecation per use.
func TestLegacyPointerSyntaxIsDeprecated(t *testing.T) {
	const prelude = `struct Box { value: Int }
trait View {
  fn value() Int
}
impl View for Box {
  fn value() Int { self.value }
}
`
	tests := []struct {
		name         string
		source       string
		deprecations int
	}{
		{name: "pointer syntax is current", source: `fn reset(box: *mut Box) { box.value = 0 }
fn main() {
  mut box = Box{value: 1}
  reset(&mut box)
  let copy: Box = (&box).*
}`},
		{name: "mut Trait is current", source: `fn show(view: mut View) Int { view.value() }`},
		{name: "legacy parameter type", source: `fn reset(box: mut Box) { box.value = 0 }`, deprecations: 1},
		{name: "legacy function type parameter", source: `fn apply(f: fn(mut Box) Void) {}`, deprecations: 1},
		{name: "legacy borrow", source: `fn main() {
  let box = mut Box{value: 1}
}`, deprecations: 1},
		{name: "legacy redundant borrow", source: `fn main() {
  let box = &mut Box{value: 1}
  let again = mut box
}`, deprecations: 1},
		{name: "legacy dereference", source: `fn main() {
  let box = &mut Box{value: 1}
  let copy = box.@
}`, deprecations: 1},
		{name: "legacy trait snapshot", source: `fn snap(view: mut View) View { view.@ }`, deprecations: 1},
		{name: "repeated resolution reports once", source: `fn reset(box: mut Box) { box.value = 0 }
fn main() {
  reset(&mut Box{value: 1})
  reset(&mut Box{value: 2})
}`, deprecations: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(prelude+tt.source), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			c := checker.New("test.ard", result.Program, nil)
			c.Check()
			deprecations := 0
			for _, diagnostic := range c.Diagnostics() {
				switch {
				case diagnostic.Kind == checker.Error:
					t.Fatalf("unexpected error: %s", diagnostic.Message)
				case diagnostic.Code == checker.DiagnosticCodeDeprecatedPointerSyntax:
					if diagnostic.Kind != checker.Warn {
						t.Fatalf("deprecation kind = %s, want warn", diagnostic.Kind)
					}
					deprecations++
				}
			}
			if deprecations != tt.deprecations {
				t.Fatalf("deprecations = %d, want %d: %v", deprecations, tt.deprecations, c.Diagnostics())
			}
		})
	}
}
