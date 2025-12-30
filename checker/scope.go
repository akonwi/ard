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

// GenericContext maps generic parameter names to their TypeVar instances
// The TypeVar instances are mutable - binding happens by setting TypeVar.actual and TypeVar.bound
type GenericContext = map[string]*TypeVar

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
func (st *SymbolTable) findGeneric(genericName string) *TypeVar {
	// Check current scope
	for _, symbol := range st.symbols {
		if typeVar, ok := symbol.Type.(*TypeVar); ok && typeVar.name == genericName {
			return typeVar
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

	// Create unbound TypeVar instances for each generic parameter
	for _, param := range genericParams {
		gc[param] = &TypeVar{
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

	// Get the TypeVar instance for this generic parameter
	typeVar, exists := (*st.genericContext)[genericName]
	if !exists {
		// Generic not found in this scope - not an error, might be from parent
		return nil
	}

	if typeVar.bound {
		// Already bound - verify consistency
		// Dereference both sides to handle chains
		actual := deref(typeVar.actual)
		concrete := deref(concreteType)
		if !actual.equal(concrete) {
			return fmt.Errorf("generic %s already bound to %s, cannot bind to %s",
				genericName, actual.String(), concrete.String())
		}
		return nil
	}

	// Bind it now - mutate the TypeVar in-place
	typeVar.actual = deref(concreteType)
	typeVar.bound = true

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

	// Collect bindings from the bound TypeVar instances
	bindings := make(map[string]Type)
	for name, typeVar := range *st.genericContext {
		if typeVar.bound && typeVar.actual != nil {
			bindings[name] = typeVar.actual
		}
	}
	return bindings
}

// extractGenericNames recursively collects all generic parameter names from a type.
// Walks the type tree and adds any $T, $U, etc. to the names map.
func extractGenericNames(t Type, names map[string]bool) {
	switch t := t.(type) {
	case *TypeVar:
		names[t.name] = true
	case *List:
		extractGenericNames(t.of, names)
	case *Map:
		extractGenericNames(t.key, names)
		extractGenericNames(t.value, names)
	case *Maybe:
		extractGenericNames(t.of, names)
	case *Result:
		extractGenericNames(t.val, names)
		extractGenericNames(t.err, names)
	case *Union:
		for _, t := range t.Types {
			extractGenericNames(t, names)
		}
	case *FunctionDef:
		// Extract generics from function parameters and return type
		for _, param := range t.Parameters {
			extractGenericNames(param.Type, names)
		}
		extractGenericNames(t.ReturnType, names)
	}
}

// hasGenericsInType checks if a type contains any generic parameters.
// Used for quick detection before generic handling.
func hasGenericsInType(t Type) bool {
	switch t := t.(type) {
	case *TypeVar:
		return true
	case *List:
		return hasGenericsInType(t.of)
	case *Map:
		return hasGenericsInType(t.key) || hasGenericsInType(t.value)
	case *Maybe:
		return hasGenericsInType(t.of)
	case *Result:
		return hasGenericsInType(t.val) || hasGenericsInType(t.err)
	case *Union:
		for _, t := range t.Types {
			if hasGenericsInType(t) {
				return true
			}
		}
		return false
	case *FunctionDef:
		for _, param := range t.Parameters {
			if hasGenericsInType(param.Type) {
				return true
			}
		}
		return hasGenericsInType(t.ReturnType)
	default:
		return false
	}
}

// Type replacement functions
func replaceGeneric(t Type, genericName string, concreteType Type) Type {
	switch t := t.(type) {
	case *TypeVar:
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
	case *TypeVar:
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

// copyFunctionWithTypeVarMap recursively copies a function definition, replacing
// TypeVar instances with those from the provided map
func copyFunctionWithTypeVarMap(fnDef *FunctionDef, typeVarMap map[string]*TypeVar) *FunctionDef {
	newParams := make([]Parameter, len(fnDef.Parameters))
	for i, param := range fnDef.Parameters {
		newParams[i] = Parameter{
			Name:    param.Name,
			Type:    copyTypeWithTypeVarMap(param.Type, typeVarMap),
			Mutable: param.Mutable,
		}
	}

	return &FunctionDef{
		Name:       fnDef.Name,
		Parameters: newParams,
		ReturnType: copyTypeWithTypeVarMap(fnDef.ReturnType, typeVarMap),
		Body:       fnDef.Body,
		Mutates:    fnDef.Mutates,
		Private:    fnDef.Private,
	}
}

// copyStructWithTypeVarMap creates a shallow copy of a StructDef with fresh TypeVar instances
// for generic type parameters. This is used to create call-site-specific copies of generic structs.
func copyStructWithTypeVarMap(structDef *StructDef, typeVarMap map[string]*TypeVar) *StructDef {
	newFields := make(map[string]Type)
	for name, fieldType := range structDef.Fields {
		newFields[name] = copyTypeWithTypeVarMap(fieldType, typeVarMap)
	}

	return &StructDef{
		Name:    structDef.Name,
		Fields:  newFields,
		Methods: structDef.Methods, // Methods are not copied; they're shared
		Self:    structDef.Self,
		Traits:  structDef.Traits,
		Private: structDef.Private,
	}
}

// copyTypeWithTypeVarMap deep copies a type, replacing TypeVar instances with fresh ones
func copyTypeWithTypeVarMap(t Type, typeVarMap map[string]*TypeVar) Type {
	switch typ := t.(type) {
	case *TypeVar:
		if fresh, exists := typeVarMap[typ.name]; exists {
			return fresh // Use the fresh TypeVar instance from genericScope
		}
		return typ // Keep as-is if not a generic parameter
	case *List:
		return &List{of: copyTypeWithTypeVarMap(typ.of, typeVarMap)}
	case *Map:
		return &Map{
			key:   copyTypeWithTypeVarMap(typ.key, typeVarMap),
			value: copyTypeWithTypeVarMap(typ.value, typeVarMap),
		}
	case *Maybe:
		return &Maybe{of: copyTypeWithTypeVarMap(typ.of, typeVarMap)}
	case *Result:
		return &Result{
			val: copyTypeWithTypeVarMap(typ.val, typeVarMap),
			err: copyTypeWithTypeVarMap(typ.err, typeVarMap),
		}
	case *Union:
		newTypes := make([]Type, len(typ.Types))
		for i, t := range typ.Types {
			newTypes[i] = copyTypeWithTypeVarMap(t, typeVarMap)
		}
		return &Union{
			Name:  typ.Name,
			Types: newTypes,
		}
	case *FunctionDef:
		return copyFunctionWithTypeVarMap(typ, typeVarMap)
	default:
		return t
	}
}
