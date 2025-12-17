package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestTypeRegistry_NewRegistry(t *testing.T) {
	registry := checker.NewTypeRegistry()
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(registry.All()) != 0 {
		t.Error("expected empty registry initially")
	}
}

func TestTypeRegistry_Next(t *testing.T) {
	registry := checker.NewTypeRegistry()
	id1 := registry.Next()
	id2 := registry.Next()
	id3 := registry.Next()

	if id1 != 1 {
		t.Errorf("expected first ID to be 1, got %d", id1)
	}
	if id2 != 2 {
		t.Errorf("expected second ID to be 2, got %d", id2)
	}
	if id3 != 3 {
		t.Errorf("expected third ID to be 3, got %d", id3)
	}
}

func TestTypeRegistry_RegisterAndLookup(t *testing.T) {
	registry := checker.NewTypeRegistry()
	strType := checker.Str

	id := registry.Next()
	err := registry.Register(id, strType)
	if err != nil {
		t.Fatalf("failed to register type: %v", err)
	}

	lookedUp := registry.Lookup(id)
	if lookedUp != strType {
		t.Errorf("expected to look up %v, got %v", strType, lookedUp)
	}
}

func TestTypeRegistry_DuplicateRegisterError(t *testing.T) {
	registry := checker.NewTypeRegistry()
	id := registry.Next()

	err1 := registry.Register(id, checker.Int)
	if err1 != nil {
		t.Fatalf("first register should succeed: %v", err1)
	}

	err2 := registry.Register(id, checker.Str)
	if err2 == nil {
		t.Error("expected error when registering duplicate ID")
	}
}

func TestTypeRegistry_InvalidTypeIDError(t *testing.T) {
	registry := checker.NewTypeRegistry()
	err := registry.Register(checker.InvalidTypeID, checker.Int)
	if err == nil {
		t.Error("expected error when registering with InvalidTypeID")
	}
}

func TestTypeRegistry_NilTypeError(t *testing.T) {
	registry := checker.NewTypeRegistry()
	id := registry.Next()
	err := registry.Register(id, nil)
	if err == nil {
		t.Error("expected error when registering nil type")
	}
}

func TestTypeRegistry_LookupInvalidID(t *testing.T) {
	registry := checker.NewTypeRegistry()
	result := registry.Lookup(checker.InvalidTypeID)
	if result != nil {
		t.Error("expected nil when looking up InvalidTypeID")
	}
}

func TestTypeRegistry_LookupNonexistent(t *testing.T) {
	registry := checker.NewTypeRegistry()
	result := registry.Lookup(999)
	if result != nil {
		t.Error("expected nil when looking up non-existent ID")
	}
}

func TestTypeRegistry_MultipleTypes(t *testing.T) {
	registry := checker.NewTypeRegistry()

	types := []checker.Type{
		checker.Int,
		checker.Str,
		checker.Bool,
		checker.Float,
	}

	ids := make(map[checker.TypeID]checker.Type)
	for _, typ := range types {
		id := registry.Next()
		err := registry.Register(id, typ)
		if err != nil {
			t.Fatalf("failed to register type: %v", err)
		}
		ids[id] = typ
	}

	if len(registry.All()) != len(types) {
		t.Errorf("expected %d types, got %d", len(types), len(registry.All()))
	}

	for id, expectedType := range ids {
		lookedUp := registry.Lookup(id)
		if lookedType := lookedUp; lookedType != expectedType {
			t.Errorf("ID %d: expected %v, got %v", id, expectedType, lookedType)
		}
	}
}

func TestTypeRegistry_All(t *testing.T) {
	registry := checker.NewTypeRegistry()

	id1 := registry.Next()
	registry.Register(id1, checker.Int)

	id2 := registry.Next()
	registry.Register(id2, checker.Str)

	all := registry.All()
	if len(all) != 2 {
		t.Errorf("expected 2 types in All(), got %d", len(all))
	}

	if all[id1] != checker.Int {
		t.Error("expected Int type in All()")
	}
	if all[id2] != checker.Str {
		t.Error("expected Str type in All()")
	}
}
