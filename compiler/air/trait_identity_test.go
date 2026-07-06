package air

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestAirTraitIdentityIncludesModulePath(t *testing.T) {
	left := &checker.Trait{Name: "Drawable", ModulePath: "ui/drawable.ard"}
	right := &checker.Trait{Name: "Drawable", ModulePath: "svg/drawable.ard"}
	if airTypeKey(left) == airTypeKey(right) {
		t.Fatalf("trait AIR type keys should include module path, got %q", airTypeKey(left))
	}

	l := &lowerer{program: Program{}, traits: map[string]TraitID{}}
	leftID, err := l.internTrait(left)
	if err != nil {
		t.Fatal(err)
	}
	rightID, err := l.internTrait(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftID == rightID {
		t.Fatalf("distinct same-named traits from different modules collapsed to id %d", leftID)
	}
}
