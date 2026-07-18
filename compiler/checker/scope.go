package checker

import (
	"fmt"
	gotypes "go/types"
	"slices"
)

type SymbolTable struct {
	parent  *SymbolTable
	symbols map[string]*Symbol

	// for scopes that expect a return value
	returnType Type

	// isolated means only read-only references in outer scopes are allowed
	isolated bool

	// inLoop marks a loop body scope; break statements bind to the nearest
	// enclosing loop within the current function
	inLoop bool

	// inUnsafe marks an unsafe block's scope. Break statements inside it are
	// reported by the unsafe pre-scan, so the generic loop-context check
	// stays quiet to avoid a duplicate diagnostic.
	inUnsafe bool

	// inScript marks top-level executable statements that will lower into the
	// synthesized script function.
	inScript bool

	// Generic context for this scope
	genericContext *GenericContext
}

// GenericContext maps generic parameter names to their TypeVar instances
// The TypeVar instances are mutable - binding happens by setting TypeVar.actual and TypeVar.bound
type GenericContext = map[string]*TypeVar

type Symbol struct {
	Name       string
	Type       Type
	declaredAt SourceSpan
	mutable    bool
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

func (st *SymbolTable) add(name string, type_ Type, mutable bool) *Symbol {
	sym := Symbol{
		Name:    name,
		Type:    type_,
		mutable: mutable,
	}
	st.symbols[name] = &sym
	return &sym
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

// breakAllowed reports whether a break statement is valid in this scope: a
// loop body scope must be reachable without crossing a function boundary
// (marked by a scope that expects a return value).
func (st *SymbolTable) breakAllowed() bool {
	if st.inLoop {
		return true
	}
	if st.returnType != nil {
		// Function (or unsafe-block) boundary: an enclosing loop belongs to
		// the outer function and cannot be broken from here.
		return false
	}
	if st.parent != nil {
		return st.parent.breakAllowed()
	}
	return false
}

// insideUnsafeBlock reports whether this scope sits inside an unsafe block
// within the current function.
func (st *SymbolTable) insideUnsafeBlock() bool {
	if st.inUnsafe {
		return true
	}
	if st.returnType != nil {
		return false
	}
	if st.parent != nil {
		return st.parent.insideUnsafeBlock()
	}
	return false
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

func (st *SymbolTable) insideScript() bool {
	if st.inScript {
		return true
	}
	if st.returnType != nil {
		return false
	}
	if st.parent != nil {
		return st.parent.insideScript()
	}
	return false
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

type genericBindingConflictError struct {
	Name               string
	Existing, Incoming Type
}

func (e *genericBindingConflictError) Error() string {
	return fmt.Sprintf("generic %s already bound to %s, cannot bind to %s", e.Name, e.Existing, e.Incoming)
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
			return &genericBindingConflictError{Name: genericName, Existing: actual, Incoming: concrete}
		}
		return nil
	}

	// Avoid self-referential binding (TypeVar bound to itself)
	resolved := deref(concreteType)
	if resolved == typeVar {
		return nil
	}

	// Bind it now - mutate the TypeVar in-place
	typeVar.actual = resolved
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
	case *MutableRef:
		extractGenericNames(t.of, names)
	case *Union:
		for _, t := range t.Types {
			extractGenericNames(t, names)
		}
	case *StructDef:
		for _, fieldType := range t.Fields {
			extractGenericNames(fieldType, names)
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
	return hasGenericsInTypeSeen(t, map[Type]struct{}{})
}

func hasGenericsInTypeSeen(t Type, seen map[Type]struct{}) bool {
	if t == nil {
		return false
	}
	if _, ok := seen[t]; ok {
		return false
	}
	seen[t] = struct{}{}
	switch t := t.(type) {
	case *TypeVar:
		if t.bound && t.actual != nil {
			return hasGenericsInTypeSeen(t.actual, seen)
		}
		return true
	case *List:
		return hasGenericsInTypeSeen(t.of, seen)
	case *Map:
		return hasGenericsInTypeSeen(t.key, seen) || hasGenericsInTypeSeen(t.value, seen)
	case *Maybe:
		return hasGenericsInTypeSeen(t.of, seen)
	case *Result:
		return hasGenericsInTypeSeen(t.val, seen) || hasGenericsInTypeSeen(t.err, seen)
	case *MutableRef:
		return hasGenericsInTypeSeen(t.of, seen)
	case *Union:
		return slices.ContainsFunc(t.Types, func(member Type) bool { return hasGenericsInTypeSeen(member, seen) })
	case *ForeignType:
		for _, typeArg := range t.TypeArgs {
			if hasGenericsInTypeSeen(typeArg, seen) {
				return true
			}
		}
		return false
	case *StructDef:
		for _, typeArg := range t.TypeArgs {
			if hasGenericsInTypeSeen(typeArg, seen) {
				return true
			}
		}
		for _, fieldType := range t.Fields {
			if hasGenericsInTypeSeen(fieldType, seen) {
				return true
			}
		}
		return false
	case *FunctionDef:
		for _, param := range t.Parameters {
			if hasGenericsInTypeSeen(param.Type, seen) {
				return true
			}
		}
		return hasGenericsInTypeSeen(t.ReturnType, seen)
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
		newOf := replaceGeneric(t.of, genericName, concreteType)
		if newOf == t.of {
			return t
		}
		return &List{of: newOf}
	case *Chan:
		newOf := replaceGeneric(t.of, genericName, concreteType)
		if newOf == t.of {
			return t
		}
		return &Chan{of: newOf}
	case *Receiver:
		newOf := replaceGeneric(t.of, genericName, concreteType)
		if newOf == t.of {
			return t
		}
		return &Receiver{of: newOf}
	case *Sender:
		newOf := replaceGeneric(t.of, genericName, concreteType)
		if newOf == t.of {
			return t
		}
		return &Sender{of: newOf}
	case *Map:
		newKey := replaceGeneric(t.key, genericName, concreteType)
		newValue := replaceGeneric(t.value, genericName, concreteType)
		if newKey == t.key && newValue == t.value {
			return t
		}
		return &Map{
			key:   newKey,
			value: newValue,
		}
	case *Maybe:
		newOf := replaceGeneric(t.of, genericName, concreteType)
		if newOf == t.of {
			return t
		}
		return &Maybe{of: newOf}
	case *Result:
		newVal := replaceGeneric(t.val, genericName, concreteType)
		newErr := replaceGeneric(t.err, genericName, concreteType)
		// Only create a new Result if something actually changed
		if newVal == t.val && newErr == t.err {
			return t
		}
		return &Result{
			val: newVal,
			err: newErr,
		}
	case *MutableRef:
		newOf := replaceGeneric(t.of, genericName, concreteType)
		if newOf == t.of {
			return t
		}
		return MakeMutableRef(newOf)
	case *FunctionDef:
		newParams := make([]Parameter, len(t.Parameters))
		for i, p := range t.Parameters {
			newParams[i] = p
			newParams[i].Type = replaceGeneric(p.Type, genericName, concreteType)
		}
		newReturnType := replaceGeneric(t.ReturnType, genericName, concreteType)
		// Create a new FunctionDef, don't modify the original
		return &FunctionDef{
			Name:                    t.Name,
			GenericParams:           append([]string(nil), t.GenericParams...),
			Parameters:              newParams,
			ReturnType:              newReturnType,
			InferReturnTypeFromBody: t.InferReturnTypeFromBody,
			Mutates:                 t.Mutates,
			Body:                    t.Body,
			Private:                 t.Private,
		}
	case *ForeignType:
		args := make([]Type, len(t.TypeArgs))
		for i, arg := range t.TypeArgs {
			args[i] = replaceGeneric(arg, genericName, concreteType)
		}
		return foreignTypeWithArgs(t, args)
	case *StructDef:
		if t.Definition != nil {
			newTypeArgs := make([]Type, len(t.TypeArgs))
			for i, typeArg := range t.TypeArgs {
				newTypeArgs[i] = replaceGeneric(typeArg, genericName, concreteType)
			}
			return newStructApplication(t, newTypeArgs)
		}
		// Canonical declarations are still copied for inference scratch values;
		// ordinary named specialization uses nominal applications above.
		anyChanged := false
		newFields := make(map[string]Type)
		for fieldName, fieldType := range t.Fields {
			newFieldType := replaceGeneric(fieldType, genericName, concreteType)
			newFields[fieldName] = newFieldType
			if newFieldType != fieldType {
				anyChanged = true
			}
		}
		// If nothing changed, return the original struct
		if !anyChanged {
			return t
		}
		newTypeArgs := make([]Type, len(t.TypeArgs))
		for i, typeArg := range t.TypeArgs {
			newTypeArgs[i] = replaceGeneric(typeArg, genericName, concreteType)
		}
		return &StructDef{
			Name:             t.Name,
			ModulePath:       t.ModulePath,
			Fields:           newFields,
			Self:             t.Self,
			Traits:           t.Traits,
			GenericParams:    append([]string(nil), t.GenericParams...),
			DeclaredGenerics: t.DeclaredGenerics,
			TypeArgs:         newTypeArgs,
			Definition:       t.Definition,
			Private:          t.Private,
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
	case *MutableRef:
		return hasGeneric(t.of, genericName)
	case *ForeignType:
		for _, typeArg := range t.TypeArgs {
			if hasGeneric(typeArg, genericName) {
				return true
			}
		}
		return false
	case *StructDef:
		for _, typeArg := range t.TypeArgs {
			if hasGeneric(typeArg, genericName) {
				return true
			}
		}
		for _, fieldType := range t.Fields {
			if hasGeneric(fieldType, genericName) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// StructDefinition returns the canonical declaration for a struct type.
func StructDefinition(def *StructDef) *StructDef {
	return canonicalStructDefinition(def)
}

// StructFields returns declaration fields specialized for the struct type's
// ordered arguments. Callers must treat the returned map as read-only.
func StructFields(def *StructDef) map[string]Type {
	return structFields(def)
}

// StructField returns one declaration field specialized for the struct type's
// ordered arguments.
func StructField(def *StructDef, name string) (Type, bool) {
	return structField(def, name)
}

func canonicalStructDefinition(def *StructDef) *StructDef {
	if def == nil {
		return nil
	}
	for def.Definition != nil && def.Definition != def {
		def = def.Definition
	}
	return def
}

func newStructApplication(def *StructDef, typeArgs []Type) *StructDef {
	definition := canonicalStructDefinition(def)
	if definition == nil {
		return nil
	}
	return &StructDef{
		Name:             definition.Name,
		ModulePath:       definition.ModulePath,
		Self:             definition.Self,
		Traits:           definition.Traits,
		GenericParams:    append([]string(nil), definition.GenericParams...),
		DeclaredGenerics: definition.DeclaredGenerics,
		TypeArgs:         append([]Type(nil), typeArgs...),
		Definition:       definition,
		Private:          definition.Private,
	}
}

func structTypeBindings(def *StructDef) map[string]Type {
	definition := canonicalStructDefinition(def)
	if definition == nil || len(def.TypeArgs) == 0 {
		return nil
	}
	bindings := make(map[string]Type, len(definition.GenericParams))
	for i, name := range definition.GenericParams {
		if i < len(def.TypeArgs) {
			bindings[name] = def.TypeArgs[i]
		}
	}
	return bindings
}

func structFields(def *StructDef) map[string]Type {
	definition := canonicalStructDefinition(def)
	if definition == nil {
		return nil
	}
	if def == definition || len(def.TypeArgs) == 0 {
		return definition.Fields
	}
	bindings := structTypeBindings(def)
	fields := make(map[string]Type, len(definition.Fields))
	for name, fieldType := range definition.Fields {
		fields[name] = substituteTypeBindings(fieldType, bindings)
	}
	return fields
}

func structField(def *StructDef, name string) (Type, bool) {
	definition := canonicalStructDefinition(def)
	if definition == nil {
		return nil, false
	}
	field, ok := definition.Fields[name]
	if !ok {
		return nil, false
	}
	if def == definition || len(def.TypeArgs) == 0 {
		return field, true
	}
	return substituteTypeBindings(field, structTypeBindings(def)), true
}

func foreignTypeWithArgs(typ *ForeignType, args []Type) *ForeignType {
	copy := *typ
	copy.TypeArgs = append([]Type(nil), args...)
	if len(args) == 0 {
		return &copy
	}
	goType := typ.GoType
	pointer := typ.Pointer
	if ptr, ok := goType.(*gotypes.Pointer); ok {
		goType = ptr.Elem()
		pointer = true
	}
	named, ok := goType.(*gotypes.Named)
	if !ok || named.TypeParams() == nil || named.TypeParams().Len() != len(args) {
		return &copy
	}
	goArgs := make([]gotypes.Type, len(args))
	for i, arg := range args {
		converted, ok := checkerTypeToGoType(arg)
		if !ok {
			return &copy
		}
		goArgs[i] = converted
	}
	instantiated, err := gotypes.Instantiate(gotypes.NewContext(), named.Origin(), goArgs, true)
	if err != nil {
		return &copy
	}
	instNamed, ok := instantiated.(*gotypes.Named)
	if !ok {
		return &copy
	}
	refreshed, ok := foreignNamedTypeFromGo(instNamed, pointer, true).(*ForeignType)
	if !ok {
		return &copy
	}
	refreshed.TypeArgs = append([]Type(nil), args...)
	return refreshed
}

func substituteTypeBindings(t Type, bindings map[string]Type) Type {
	switch typ := t.(type) {
	case *TypeVar:
		if replacement, ok := bindings[typ.name]; ok {
			return replacement
		}
		if typ.bound && typ.actual != nil {
			return substituteTypeBindings(typ.actual, bindings)
		}
		return typ
	case *List:
		return MakeList(substituteTypeBindings(typ.of, bindings))
	case *FixedArray:
		return MakeFixedArray(substituteTypeBindings(typ.of, bindings), typ.length)
	case *Chan:
		return MakeChan(substituteTypeBindings(typ.of, bindings))
	case *Receiver:
		return MakeReceiver(substituteTypeBindings(typ.of, bindings))
	case *Sender:
		return MakeSender(substituteTypeBindings(typ.of, bindings))
	case *Map:
		return MakeMap(substituteTypeBindings(typ.key, bindings), substituteTypeBindings(typ.value, bindings))
	case *Maybe:
		return MakeMaybe(substituteTypeBindings(typ.of, bindings))
	case *Result:
		return MakeResult(substituteTypeBindings(typ.val, bindings), substituteTypeBindings(typ.err, bindings))
	case *MutableRef:
		return MakeMutableRef(substituteTypeBindings(typ.of, bindings))
	case *Union:
		members := make([]Type, len(typ.Types))
		for i, member := range typ.Types {
			members[i] = substituteTypeBindings(member, bindings)
		}
		return &Union{Name: typ.Name, ModulePath: typ.ModulePath, Types: members, Private: typ.Private}
	case *FunctionDef:
		params := make([]Parameter, len(typ.Parameters))
		for i, param := range typ.Parameters {
			param.Type = substituteTypeBindings(param.Type, bindings)
			params[i] = param
		}
		copy := *typ
		copy.Parameters = params
		copy.ReturnType = substituteTypeBindings(typ.ReturnType, bindings)
		copy.GenericBindings = cloneTypeMap(typ.GenericBindings)
		return &copy
	case *ForeignType:
		args := make([]Type, len(typ.TypeArgs))
		for i, arg := range typ.TypeArgs {
			args[i] = substituteTypeBindings(arg, bindings)
		}
		return foreignTypeWithArgs(typ, args)
	case *StructDef:
		if len(typ.TypeArgs) == 0 {
			return typ
		}
		args := make([]Type, len(typ.TypeArgs))
		for i, arg := range typ.TypeArgs {
			args[i] = substituteTypeBindings(arg, bindings)
		}
		return newStructApplication(typ, args)
	default:
		return t
	}
}

// copyFunctionWithTypeVarMap recursively copies a function definition, replacing
// TypeVar instances with those from the provided map
func copyFunctionWithTypeVarMap(fnDef *FunctionDef, typeVarMap map[string]*TypeVar) *FunctionDef {
	newParams := make([]Parameter, len(fnDef.Parameters))
	for i, param := range fnDef.Parameters {
		newParams[i] = Parameter{
			Name:       param.Name,
			Type:       copyTypeWithTypeVarMap(param.Type, typeVarMap),
			Mutable:    param.Mutable,
			Loc:        param.Loc,
			declaredAt: param.declaredAt,
			Variadic:   param.Variadic,
		}
	}

	copy := &FunctionDef{
		Name:                    fnDef.Name,
		GenericParams:           append([]string(nil), fnDef.GenericParams...),
		Parameters:              newParams,
		ReturnType:              copyTypeWithTypeVarMap(fnDef.ReturnType, typeVarMap),
		InferReturnTypeFromBody: fnDef.InferReturnTypeFromBody,
		Body:                    fnDef.Body,
		Mutates:                 fnDef.Mutates,
		Private:                 fnDef.Private,
		GenericBindings:         cloneTypeMap(fnDef.GenericBindings),
	}
	if bindings := concreteTypeVarBindings(typeVarMap); bindings != nil {
		copy.GenericBindings = bindings
	}
	return copy
}

// copyStructWithTypeVarMap creates a shallow copy of a StructDef with fresh TypeVar instances
// for generic type parameters. This is used to create call-site-specific copies of generic structs.
func copyStructWithTypeVarMap(structDef *StructDef, typeVarMap map[string]*TypeVar) *StructDef {
	return copyStructWithTypeVarMapSeen(structDef, typeVarMap, map[*StructDef]*StructDef{})
}

func copyStructWithTypeVarMapSeen(structDef *StructDef, typeVarMap map[string]*TypeVar, seen map[*StructDef]*StructDef) *StructDef {
	if structDef == nil {
		return nil
	}
	if structDef.Definition != nil {
		args := make([]Type, len(structDef.TypeArgs))
		for i, arg := range structDef.TypeArgs {
			args[i] = copyTypeWithTypeVarMapSeen(arg, typeVarMap, seen)
		}
		return newStructApplication(structDef, args)
	}
	if existing, ok := seen[structDef]; ok {
		return existing
	}
	newFields := make(map[string]Type)
	structCopy := &StructDef{
		Name:             structDef.Name,
		ModulePath:       structDef.ModulePath,
		Fields:           newFields,
		Self:             structDef.Self,
		Traits:           structDef.Traits,
		GenericParams:    append([]string(nil), structDef.GenericParams...),
		DeclaredGenerics: structDef.DeclaredGenerics,
		Definition:       structDef.Definition,
		Private:          structDef.Private,
	}
	seen[structDef] = structCopy
	for name, fieldType := range structDef.Fields {
		newFields[name] = copyTypeWithTypeVarMapSeen(fieldType, typeVarMap, seen)
	}
	structCopy.TypeArgs = copyStructTypeArgsWithTypeVarMap(structDef, typeVarMap, seen)
	return structCopy
}

func copyStructTypeArgsWithTypeVarMap(structDef *StructDef, typeVarMap map[string]*TypeVar, seenStructs map[*StructDef]*StructDef) []Type {
	if len(structDef.GenericParams) == 0 {
		return nil
	}
	out := make([]Type, len(structDef.GenericParams))
	for i, param := range structDef.GenericParams {
		if i < len(structDef.TypeArgs) {
			out[i] = copyTypeWithTypeVarMapSeen(structDef.TypeArgs[i], typeVarMap, seenStructs)
			continue
		}
		if typeVar, ok := typeVarMap[param]; ok {
			out[i] = derefType(typeVar)
		} else {
			out[i] = &TypeVar{name: param}
		}
	}
	return out
}

func collectUnboundGenericsFromType(t Type, params *[]string, seenGenerics map[string]bool, seenTypes map[Type]bool) {
	if t == nil {
		return
	}
	if typeVar, ok := t.(*TypeVar); ok && typeVar.bound && typeVar.actual != nil {
		t = typeVar.actual
	}
	if _, ok := seenTypes[t]; ok {
		return
	}
	seenTypes[t] = true
	switch typ := t.(type) {
	case *TypeVar:
		if !seenGenerics[typ.name] {
			*params = append(*params, typ.name)
			seenGenerics[typ.name] = true
		}
	case *List:
		collectUnboundGenericsFromType(typ.of, params, seenGenerics, seenTypes)
	case *Map:
		collectUnboundGenericsFromType(typ.key, params, seenGenerics, seenTypes)
		collectUnboundGenericsFromType(typ.value, params, seenGenerics, seenTypes)
	case *Maybe:
		collectUnboundGenericsFromType(typ.of, params, seenGenerics, seenTypes)
	case *Result:
		collectUnboundGenericsFromType(typ.val, params, seenGenerics, seenTypes)
		collectUnboundGenericsFromType(typ.err, params, seenGenerics, seenTypes)
	case *MutableRef:
		collectUnboundGenericsFromType(typ.of, params, seenGenerics, seenTypes)
	case *Union:
		for _, member := range typ.Types {
			collectUnboundGenericsFromType(member, params, seenGenerics, seenTypes)
		}
	case *StructDef:
		for _, typeArg := range typ.TypeArgs {
			collectUnboundGenericsFromType(typeArg, params, seenGenerics, seenTypes)
		}
		for _, fieldType := range typ.Fields {
			collectUnboundGenericsFromType(fieldType, params, seenGenerics, seenTypes)
		}
	case *ForeignType:
		for _, typeArg := range typ.TypeArgs {
			collectUnboundGenericsFromType(typeArg, params, seenGenerics, seenTypes)
		}
	case *FunctionDef:
		for _, param := range typ.Parameters {
			collectUnboundGenericsFromType(param.Type, params, seenGenerics, seenTypes)
		}
		collectUnboundGenericsFromType(typ.ReturnType, params, seenGenerics, seenTypes)
	}
}

// copyTypeWithTypeVarMap deep copies a type, replacing TypeVar instances with fresh ones
func copyTypeWithTypeVarMap(t Type, typeVarMap map[string]*TypeVar) Type {
	return copyTypeWithTypeVarMapSeen(t, typeVarMap, map[*StructDef]*StructDef{})
}

func copyTypeWithTypeVarMapSeen(t Type, typeVarMap map[string]*TypeVar, seenStructs map[*StructDef]*StructDef) Type {
	switch typ := t.(type) {
	case *TypeVar:
		if fresh, exists := typeVarMap[typ.name]; exists {
			return fresh // Use the fresh TypeVar instance from genericScope
		}
		return typ // Keep as-is if not a generic parameter
	case *List:
		return &List{of: copyTypeWithTypeVarMapSeen(typ.of, typeVarMap, seenStructs)}
	case *Map:
		return &Map{
			key:   copyTypeWithTypeVarMapSeen(typ.key, typeVarMap, seenStructs),
			value: copyTypeWithTypeVarMapSeen(typ.value, typeVarMap, seenStructs),
		}
	case *Maybe:
		return &Maybe{of: copyTypeWithTypeVarMapSeen(typ.of, typeVarMap, seenStructs)}
	case *Result:
		return &Result{
			val: copyTypeWithTypeVarMapSeen(typ.val, typeVarMap, seenStructs),
			err: copyTypeWithTypeVarMapSeen(typ.err, typeVarMap, seenStructs),
		}
	case *MutableRef:
		return MakeMutableRef(copyTypeWithTypeVarMapSeen(typ.of, typeVarMap, seenStructs))
	case *Union:
		newTypes := make([]Type, len(typ.Types))
		for i, t := range typ.Types {
			newTypes[i] = copyTypeWithTypeVarMapSeen(t, typeVarMap, seenStructs)
		}
		return &Union{
			Name:       typ.Name,
			ModulePath: typ.ModulePath,
			Types:      newTypes,
			Private:    typ.Private,
		}
	case *ForeignType:
		args := make([]Type, len(typ.TypeArgs))
		for i, arg := range typ.TypeArgs {
			args[i] = copyTypeWithTypeVarMapSeen(arg, typeVarMap, seenStructs)
		}
		return foreignTypeWithArgs(typ, args)
	case *StructDef:
		return copyStructWithTypeVarMapSeen(typ, typeVarMap, seenStructs)
	case *FunctionDef:
		return copyFunctionWithTypeVarMap(typ, typeVarMap)
	default:
		return t
	}
}
