package checker

import "fmt"

// TypeID is a handle to a registered type in the registry
type TypeID uint32

const InvalidTypeID TypeID = 0

// TypeRegistry stores all types for a module and assigns them unique IDs
type TypeRegistry struct {
	types        map[TypeID]Type
	nextID       TypeID
	canonicalIDs CanonicalTypeIDs
}

// CanonicalTypeIDs caches TypeIDs for built-in types for O(1) comparisons
// Phase 6: Optimization - avoid Type() calls in hot paths
type CanonicalTypeIDs struct {
	Int   TypeID
	Float TypeID
	Str   TypeID
	Bool  TypeID
	Void  TypeID
}

// NewTypeRegistry creates an empty registry with ID allocation starting at 1
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		types:  make(map[TypeID]Type),
		nextID: 1, // 0 is reserved for InvalidTypeID
		canonicalIDs: CanonicalTypeIDs{
			Int:   InvalidTypeID,
			Float: InvalidTypeID,
			Str:   InvalidTypeID,
			Bool:  InvalidTypeID,
			Void:  InvalidTypeID,
		},
	}
}

// Next returns the next available TypeID and increments the counter
func (tr *TypeRegistry) Next() TypeID {
	id := tr.nextID
	tr.nextID++
	return id
}

// Register stores a type with the given ID
// Returns an error if the ID is already registered
// Phase 6: Cache canonical type IDs for O(1) comparisons
func (tr *TypeRegistry) Register(id TypeID, t Type) error {
	if id == InvalidTypeID {
		return fmt.Errorf("cannot register with InvalidTypeID")
	}
	if _, exists := tr.types[id]; exists {
		return fmt.Errorf("type ID %d already registered", id)
	}
	if t == nil {
		return fmt.Errorf("cannot register nil type")
	}
	tr.types[id] = t
	
	// Cache canonical type IDs for built-in types
	// This enables O(1) type comparisons without Type() calls
	if tr.canonicalIDs.Int == InvalidTypeID && t == Int {
		tr.canonicalIDs.Int = id
	}
	if tr.canonicalIDs.Float == InvalidTypeID && t == Float {
		tr.canonicalIDs.Float = id
	}
	if tr.canonicalIDs.Str == InvalidTypeID && t == Str {
		tr.canonicalIDs.Str = id
	}
	if tr.canonicalIDs.Bool == InvalidTypeID && t == Bool {
		tr.canonicalIDs.Bool = id
	}
	if tr.canonicalIDs.Void == InvalidTypeID && t == Void {
		tr.canonicalIDs.Void = id
	}
	
	return nil
}

// Lookup retrieves a type by ID
// Returns nil if the ID is not found
func (tr *TypeRegistry) Lookup(id TypeID) Type {
	if id == InvalidTypeID {
		return nil
	}
	return tr.types[id]
}

// All returns all registered types (for testing/debugging)
func (tr *TypeRegistry) All() map[TypeID]Type {
	return tr.types
}

// CanonicalIds returns the cached TypeIDs for built-in types
// Phase 6: Used for fast type comparisons in hot paths
func (tr *TypeRegistry) CanonicalIds() CanonicalTypeIDs {
	return tr.canonicalIDs
}
