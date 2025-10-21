package checker

import "fmt"

type SymbolTable struct {
	parent  *SymbolTable
	symbols map[string]*Symbol

	// for scopes that expect a return value
	returnType Type

	// isolated means only read-only references in outer scopes are allowed
	isolated bool

	// Generic context for this scope
	genericContext *GenericContext
}

type GenericContext struct {
	// Map from generic parameter name to concrete type
	bindings map[string]Type

	// Track which generics are still unresolved
	unresolved map[string]bool
}

type Symbol struct {
	Name    string
	Type    Type
	mutable bool
}

func (s Symbol) IsZero() bool {
	return s == Symbol{}
}

func makeScope(parent *SymbolTable) SymbolTable {
	return SymbolTable{
		parent:  parent,
		symbols: map[string]*Symbol{},
	}
}

func (st *SymbolTable) add(name string, type_ Type, mutable bool) {
	sym := Symbol{
		Name:    name,
		Type:    type_,
		mutable: mutable,
	}
	st.symbols[name] = &sym
}

func (st SymbolTable) get(name string) (*Symbol, bool) {
	if sym, ok := st.symbols[name]; ok {
		return sym, true
	}

	if st.parent != nil && !st.isolated {
		got, ok := st.parent.get(name)
		return got, ok
	}
	return nil, false
}

// findGeneric looks for an existing generic type with the given name in the scope chain
func (st *SymbolTable) findGeneric(genericName string) *Any {
	// Check current scope
	for _, symbol := range st.symbols {
		if anyType, ok := symbol.Type.(*Any); ok && anyType.name == genericName {
			return anyType
		}
	}

	// Check parent scopes
	if st.parent != nil {
		return st.parent.findGeneric(genericName)
	}

	return nil
}

func (st *SymbolTable) expectReturn(returnType Type) {
	st.returnType = returnType
}

// getReturnType traverses up the scope hierarchy to find the first non-nil returnType
func (st *SymbolTable) getReturnType() Type {
	if st.returnType != nil {
		return st.returnType
	}
	if st.parent != nil {
		return st.parent.getReturnType()
	}
	return nil
}

func (st *SymbolTable) isolate() {
	st.isolated = true
}

// Generic context methods
func (st *SymbolTable) createGenericScope(genericParams []string) *SymbolTable {
	genericContext := &GenericContext{
		bindings:   make(map[string]Type),
		unresolved: make(map[string]bool),
	}

	// Mark all generics as unresolved initially
	for _, param := range genericParams {
		genericContext.unresolved[param] = true
	}

	return &SymbolTable{
		parent:         st,
		symbols:        make(map[string]*Symbol),
		isolated:       false,
		genericContext: genericContext,
	}
}

func (st *SymbolTable) bindGeneric(genericName string, concreteType Type) error {
	if st.genericContext == nil {
		return nil // No generic context, ignore
	}

	// Check if already bound to a different type
	if existing, exists := st.genericContext.bindings[genericName]; exists {
		if !existing.equal(concreteType) {
			return fmt.Errorf("generic %s already bound to %s, cannot bind to %s",
				genericName, existing.String(), concreteType.String())
		}
		return nil
	}

	// Bind the generic
	st.genericContext.bindings[genericName] = concreteType
	st.genericContext.unresolved[genericName] = false

	// Update all symbols that use this generic
	st.updateSymbolsWithGeneric(genericName, concreteType)

	return nil
}

func (st *SymbolTable) updateSymbolsWithGeneric(genericName string, concreteType Type) {
	for _, symbol := range st.symbols {
		if hasGeneric(symbol.Type, genericName) {
			symbol.Type = replaceGeneric(symbol.Type, genericName, concreteType)
		}
	}
}

func (st *SymbolTable) getGenericBindings() map[string]Type {
	if st.genericContext == nil {
		return nil
	}
	return st.genericContext.bindings
}

// Type replacement functions
func replaceGeneric(t Type, genericName string, concreteType Type) Type {
	switch t := t.(type) {
	case *Any:
		if t.name == genericName {
			return concreteType
		}
		return t
	case *List:
		return &List{of: replaceGeneric(t.of, genericName, concreteType)}
	case *Map:
		return &Map{
			key:   replaceGeneric(t.key, genericName, concreteType),
			value: replaceGeneric(t.value, genericName, concreteType),
		}
	case *Maybe:
		return &Maybe{of: replaceGeneric(t.of, genericName, concreteType)}
	case *Result:
		return &Result{
			val: replaceGeneric(t.val, genericName, concreteType),
			err: replaceGeneric(t.err, genericName, concreteType),
		}
	case *FunctionDef:
		newParams := make([]Parameter, len(t.Parameters))
		for i, p := range t.Parameters {
			newParams[i] = Parameter{
				Name:    p.Name,
				Type:    replaceGeneric(p.Type, genericName, concreteType),
				Mutable: p.Mutable,
			}
		}
		newReturnType := replaceGeneric(t.ReturnType, genericName, concreteType)
		// Create a new FunctionDef, don't modify the original
		return &FunctionDef{
			Name:       t.Name,
			Parameters: newParams,
			ReturnType: newReturnType,
			Mutates:    t.Mutates,
			Body:       t.Body,
			Private:    t.Private,
		}
	default:
		return t
	}
}

func hasGeneric(t Type, genericName string) bool {
	switch t := t.(type) {
	case *Any:
		return t.name == genericName
	case *List:
		return hasGeneric(t.of, genericName)
	case *Map:
		return hasGeneric(t.key, genericName) || hasGeneric(t.value, genericName)
	case *Maybe:
		return hasGeneric(t.of, genericName)
	case *Result:
		return hasGeneric(t.val, genericName) || hasGeneric(t.err, genericName)
	default:
		return false
	}
}
