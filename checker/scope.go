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

// GenericContext maps generic parameter names to their Any instances
// The Any instances are mutable - binding happens by setting Any.actual and Any.bound
type GenericContext = map[string]*Any

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

	if st.parent != nil {
		got, ok := st.parent.get(name)
		// for isolated scopes, only read-only references are allowed
		if ok && st.isolated && got.mutable {
			return nil, false
		}
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
	gc := make(GenericContext)

	// Create unbound Any instances for each generic parameter
	for _, param := range genericParams {
		gc[param] = &Any{
			name:   param,
			actual: nil,
			bound:  false,
		}
	}

	return &SymbolTable{
		parent:         st,
		symbols:        make(map[string]*Symbol),
		isolated:       false,
		genericContext: &gc,
	}
}

func (st *SymbolTable) bindGeneric(genericName string, concreteType Type) error {
	if st.genericContext == nil {
		return nil // No generic context, ignore
	}

	// Get the Any instance for this generic parameter
	any, exists := (*st.genericContext)[genericName]
	if !exists {
		// Generic not found in this scope - not an error, might be from parent
		return nil
	}

	if any.bound {
		// Already bound - verify consistency
		// Dereference both sides to handle chains
		actual := deref(any.actual)
		concrete := deref(concreteType)
		if !actual.equal(concrete) {
			return fmt.Errorf("generic %s already bound to %s, cannot bind to %s",
				genericName, actual.String(), concrete.String())
		}
		return nil
	}

	// Bind it now - mutate the Any in-place
	any.actual = deref(concreteType)
	any.bound = true

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

	// Collect bindings from the bound Any instances
	bindings := make(map[string]Type)
	for name, any := range *st.genericContext {
		if any.bound && any.actual != nil {
			bindings[name] = any.actual
		}
	}
	return bindings
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

// copyFunctionWithAnyMap recursively copies a function definition, replacing
// Any instances with those from the provided map
func copyFunctionWithAnyMap(fnDef *FunctionDef, anyMap map[string]*Any) *FunctionDef {
	newParams := make([]Parameter, len(fnDef.Parameters))
	for i, param := range fnDef.Parameters {
		newParams[i] = Parameter{
			Name:    param.Name,
			Type:    copyTypeWithAnyMap(param.Type, anyMap),
			Mutable: param.Mutable,
		}
	}

	return &FunctionDef{
		Name:       fnDef.Name,
		Parameters: newParams,
		ReturnType: copyTypeWithAnyMap(fnDef.ReturnType, anyMap),
		Body:       fnDef.Body,
		Mutates:    fnDef.Mutates,
		Private:    fnDef.Private,
	}
}

// copyTypeWithAnyMap deep copies a type, replacing Any instances with fresh ones
func copyTypeWithAnyMap(t Type, anyMap map[string]*Any) Type {
	switch typ := t.(type) {
	case *Any:
		if fresh, exists := anyMap[typ.name]; exists {
			return fresh // Use the fresh Any instance from genericScope
		}
		return typ // Keep as-is if not a generic parameter
	case *List:
		return &List{of: copyTypeWithAnyMap(typ.of, anyMap)}
	case *Map:
		return &Map{
			key:   copyTypeWithAnyMap(typ.key, anyMap),
			value: copyTypeWithAnyMap(typ.value, anyMap),
		}
	case *Maybe:
		return &Maybe{of: copyTypeWithAnyMap(typ.of, anyMap)}
	case *Result:
		return &Result{
			val: copyTypeWithAnyMap(typ.val, anyMap),
			err: copyTypeWithAnyMap(typ.err, anyMap),
		}
	case *Union:
		newTypes := make([]Type, len(typ.Types))
		for i, t := range typ.Types {
			newTypes[i] = copyTypeWithAnyMap(t, anyMap)
		}
		return &Union{
			Name:  typ.Name,
			Types: newTypes,
		}
	case *FunctionDef:
		return copyFunctionWithAnyMap(typ, anyMap)
	default:
		return t
	}
}
