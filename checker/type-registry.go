package checker

import "fmt"

// TypeID is a handle to a registered type in the registry
type TypeID uint32

const InvalidTypeID TypeID = 0

// TypeRegistry stores all types for a module and assigns them unique IDs
type TypeRegistry struct {
	types  map[TypeID]Type
	nextID TypeID
}

// NewTypeRegistry creates an empty registry with ID allocation starting at 1
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		types:  make(map[TypeID]Type),
		nextID: 1, // 0 is reserved for InvalidTypeID
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
