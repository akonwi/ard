package air

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestAirTraitIdentityIncludesModulePath(t *testing.T) {
	left := &checker.Trait{Name: "Drawable", ModulePath: "ui/drawable.ard"}
	right := &checker.Trait{Name: "Drawable", ModulePath: "svg/drawable.ard"}
	l := newLowerer(LowerOptions{}, 0)
	leftID, err := l.internType(left)
	if err != nil {
		t.Fatal(err)
	}
	rightID, err := l.internType(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftID == rightID {
		t.Fatalf("distinct same-named traits from different modules collapsed to id %d", leftID)
	}
}
