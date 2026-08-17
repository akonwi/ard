package runtime

import "testing"

type snapshotView interface {
	Value() int
}

type valueSnapshot struct {
	value int
}

func (valueSnapshot valueSnapshot) Value() int { return valueSnapshot.value }

type mutableSnapshotView interface {
	Value() int
	Set(int)
}

type mutableSnapshot struct {
	value int
}

func (value *mutableSnapshot) Value() int   { return value.value }
func (value *mutableSnapshot) Set(next int) { value.value = next }

func TestTraitSnapshotReturnsDereferencedValueWhenItImplementsTrait(t *testing.T) {
	original := &valueSnapshot{value: 1}
	var view snapshotView = original

	snapshot := TraitSnapshot(view)
	original.value = 2

	if _, ok := snapshot.(valueSnapshot); !ok {
		t.Fatalf("snapshot dynamic type = %T, want valueSnapshot", snapshot)
	}
	if got := snapshot.Value(); got != 1 {
		t.Fatalf("snapshot value = %d, want 1", got)
	}
}

func TestTraitSnapshotCopiesPointerReceiverStorage(t *testing.T) {
	original := &mutableSnapshot{value: 1}
	var view mutableSnapshotView = original

	snapshot := TraitSnapshot(view)
	original.Set(2)
	snapshot.Set(3)

	if snapshot == view {
		t.Fatal("snapshot retained original pointer identity")
	}
	if got := original.Value(); got != 2 {
		t.Fatalf("original value = %d, want 2", got)
	}
	if got := snapshot.Value(); got != 3 {
		t.Fatalf("snapshot value = %d, want 3", got)
	}
}

func TestTraitSnapshotPanicsForNilPointer(t *testing.T) {
	var original *mutableSnapshot
	var view mutableSnapshotView = original
	defer func() {
		if recover() == nil {
			t.Fatal("TraitSnapshot did not panic for nil pointer")
		}
	}()
	TraitSnapshot(view)
}
