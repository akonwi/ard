package checker

import (
	"fmt"
	"maps"
	"math"
	"math/big"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/akonwi/ard/parse"
)

type Program struct {
	Imports       map[string]Module
	GoImports     map[string]*GoPackage
	Statements    []Statement
	StructMethods map[MethodOwner]map[string]*FunctionDef
}

type Module interface {
	Path() string
	Get(name string) Symbol
	Program() *Program
}

type DiagnosticKind string

const (
	Error DiagnosticKind = "error"
	Warn  DiagnosticKind = "warn"
)

type Diagnostic struct {
	Kind     DiagnosticKind
	Message  string
	filePath string
	location parse.Location
}

func NewDiagnostic(kind DiagnosticKind, message string, filePath string, location parse.Location) Diagnostic {
	return Diagnostic{
		Kind:     kind,
		Message:  message,
		filePath: filePath,
		location: location,
	}
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s %s %s", d.filePath, d.location.Start, d.Message)
}

func (d Diagnostic) FilePath() string {
	return d.filePath
}

func (d Diagnostic) Location() parse.Location {
	return d.location
}

// deref follows TypeVar bindings to find the concrete type.
// Used during type unification to ensure we see resolved types.
// Only dereferences a single type node; for compound types use derefType.
//
// Example: If $T is bound to Int, deref($T) returns Int.
// If $T is bound to [$U], deref($T) returns [$U] (not the resolved contents).
func deref(t Type) Type {
	if typeVar, ok := t.(*TypeVar); ok && typeVar.bound {
		if typeVar.actual == t {
			return t // break self-referential cycle
		}
		return deref(typeVar.actual) // Recursively follow chains
	}
	return t
}

// derefType recursively dereferences a type, including through compound types.
// Walks the entire type tree, dereferencing TypeVar at each level.
// Used when parameter types must reflect bound generics before checking arguments.
// Only creates new instances when inner types actually change.
//
// Example: If $T is bound to Int, derefType([$T]) returns [Int].
// Ensures anonymous function parameters see fully resolved types.
func derefType(t Type) Type {
	return derefTypeSeen(t, map[Type]bool{})
}

// derefTypeSeen guards against cycles in named type graphs, such as a struct
// with an impl method returning the same struct type. Longer term, methods
// should probably live outside StructDef's type identity so value shape and
// method namespace cannot recursively contain each other.
func derefTypeSeen(t Type, seen map[Type]bool) Type {
	t = deref(t) // First dereference at the top level
	if t == nil {
		return nil
	}
	if seen[t] {
		return t
	}
	seen[t] = true
	switch typ := t.(type) {
	case *List:
		derefInner := derefTypeSeen(typ.of, seen)
		if derefInner == typ.of {
			return typ // No change, return original
		}
		return &List{of: derefInner}
	case *Chan:
		derefInner := derefTypeSeen(typ.of, seen)
		if derefInner == typ.of {
			return typ
		}
		return &Chan{of: derefInner}
	case *Receiver:
		derefInner := derefTypeSeen(typ.of, seen)
		if derefInner == typ.of {
			return typ
		}
		return &Receiver{of: derefInner}
	case *Sender:
		derefInner := derefTypeSeen(typ.of, seen)
		if derefInner == typ.of {
			return typ
		}
		return &Sender{of: derefInner}
	case *Map:
		derefKey := derefTypeSeen(typ.key, seen)
		derefVal := derefTypeSeen(typ.value, seen)
		if derefKey == typ.key && derefVal == typ.value {
			return typ // No change, return original
		}
		return &Map{
			key:   derefKey,
			value: derefVal,
		}
	case *Maybe:
		derefInner := derefTypeSeen(typ.of, seen)
		if derefInner == typ.of {
			return typ // No change, return original
		}
		return &Maybe{of: derefInner}
	case *Result:
		derefVal := derefTypeSeen(typ.val, seen)
		derefErr := derefTypeSeen(typ.err, seen)
		if derefVal == typ.val && derefErr == typ.err {
			return typ // No change, return original
		}
		return &Result{
			val: derefVal,
			err: derefErr,
		}
	case *MutableRef:
		derefInner := derefTypeSeen(typ.of, seen)
		if derefInner == typ.of {
			return typ
		}
		return MakeMutableRef(derefInner)
	case *Union:
		newTypes := make([]Type, len(typ.Types))
		changed := false
		for i, t := range typ.Types {
			newTypes[i] = derefTypeSeen(t, seen)
			if newTypes[i] != typ.Types[i] {
				changed = true
			}
		}
		if !changed {
			return typ // No change, return original
		}
		return &Union{
			Name:       typ.Name,
			ModulePath: typ.ModulePath,
			Types:      newTypes,
			Private:    typ.Private,
		}
	case *StructDef:
		fieldsChanged := false
		newFields := make(map[string]Type, len(typ.Fields))
		for name, fieldType := range typ.Fields {
			derefFieldType := derefTypeSeen(fieldType, seen)
			newFields[name] = derefFieldType
			if derefFieldType != fieldType {
				fieldsChanged = true
			}
		}
		newTypeArgs := make([]Type, len(typ.TypeArgs))
		typeArgsChanged := false
		for i, typeArg := range typ.TypeArgs {
			newTypeArgs[i] = derefTypeSeen(typeArg, seen)
			if newTypeArgs[i] != typeArg {
				typeArgsChanged = true
			}
		}
		if !fieldsChanged && !typeArgsChanged {
			return typ
		}
		return &StructDef{
			Name:          typ.Name,
			ModulePath:    typ.ModulePath,
			Fields:        newFields,
			Self:          typ.Self,
			Traits:        typ.Traits,
			GenericParams: append([]string(nil), typ.GenericParams...),
			TypeArgs:      newTypeArgs,
			Private:       typ.Private,
		}
	case *FunctionDef:
		newParams := make([]Parameter, len(typ.Parameters))
		paramsChanged := false
		for i, param := range typ.Parameters {
			derefParamType := derefTypeSeen(param.Type, seen)
			newParams[i] = Parameter{
				Name:    param.Name,
				Type:    derefParamType,
				Mutable: param.Mutable,
			}
			if derefParamType != param.Type {
				paramsChanged = true
			}
		}
		derefReturnType := derefTypeSeen(typ.ReturnType, seen)
		returnChanged := derefReturnType != typ.ReturnType
		if !paramsChanged && !returnChanged {
			return typ // No change, return original
		}
		return &FunctionDef{
			Name:                    typ.Name,
			GenericParams:           append([]string(nil), typ.GenericParams...),
			Parameters:              newParams,
			ReturnType:              derefReturnType,
			InferReturnTypeFromBody: typ.InferReturnTypeFromBody,
			Body:                    typ.Body,
			Mutates:                 typ.Mutates,
			IsTest:                  typ.IsTest,
			Private:                 typ.Private,
			GenericBindings:         cloneTypeMap(typ.GenericBindings),
		}
	default:
		return t
	}
}

// referenceArgType returns the type used when matching an argument against a
// parameter, preserving a mutable-reference field's reference type. Reading a
// `mut T` field deref's to its value type `T` (write-through semantics); this
// recovers the underlying `mut T` so a stored handle (a mutable lvalue) can be
// borrowed back into `mut T` at a call site (ADR 0031).
func referenceArgType(expr Expression) Type {
	if ip, ok := expr.(*InstanceProperty); ok {
		return ip._type
	}
	return expr.Type()
}

func (c Checker) isMutable(expr Expression) bool {
	switch e := expr.(type) {
	case *Variable:
		// A value whose type is a mutable reference is itself mutable through the
		// reference, regardless of whether the binding is reassignable (ADR 0031).
		// Mirrors the InstanceProperty case so a `mut T` value is usable wherever a
		// `mut T` is expected (fields, params, mutating methods).
		if _, ok := mutableRefBase(e.sym.Type); ok {
			return true
		}
		return e.sym.mutable
	case *InstanceProperty:
		if _, ok := mutableRefBase(e._type); ok {
			return true
		}
		return c.isMutable(e.Subject)
	case *ForeignFieldAccess:
		return c.isMutable(e.Subject)
	}
	return false
}

type Checker struct {
	diagnostics                       []Diagnostic
	input                             *parse.Program
	scope                             *SymbolTable
	filePath                          string
	modulePath                        string
	program                           *Program
	halted                            bool
	moduleResolver                    *ModuleResolver
	options                           CheckOptions
	expectedExpr                      Type
	duplicateTopLevelTypeDeclarations map[parse.Statement]bool
	topLevelStructDeclarations        map[string]*parse.StructDefinition
	topLevelTypeAliases               map[string]*parse.TypeDeclaration
	resolvingTopLevelStructs          map[string]bool
	resolvedTopLevelStructs           map[string]bool
	resolvingTopLevelAliases          map[string]bool
	resolvedTopLevelAliases           map[string]bool
	genericContextStack               []map[string]bool
	discardExprContext                bool
	matchArmDiscardContext            bool
	reportedMapKeyErrors              map[parse.Location]bool
}

func New(filePath string, input *parse.Program, moduleResolver *ModuleResolver, options ...CheckOptions) *Checker {
	rootScope := makeScope(nil)
	checkOptions := normalizeCheckOptions(options)
	modulePath := filePath
	if checkOptions.ModulePath != "" {
		modulePath = checkOptions.ModulePath
	}
	c := &Checker{
		diagnostics:    []Diagnostic{},
		input:          input,
		filePath:       filePath,
		modulePath:     modulePath,
		moduleResolver: moduleResolver,
		options:        checkOptions,
		program: &Program{
			Imports:       map[string]Module{},
			GoImports:     map[string]*GoPackage{},
			Statements:    []Statement{},
			StructMethods: map[MethodOwner]map[string]*FunctionDef{},
		},
		scope: &rootScope,
	}

	return c
}

func (c *Checker) typeOwnerPath() string {
	if c.moduleResolver == nil && !strings.HasPrefix(c.modulePath, "ard/") {
		return ""
	}
	return c.modulePath
}

func (c *Checker) HasErrors() bool {
	return len(c.diagnostics) > 0
}

func (c *Checker) Diagnostics() []Diagnostic {
	return c.diagnostics
}

func (c *Checker) Check() {
	seenImportAliases := map[string]struct{}{}
	for _, imp := range c.input.Imports {
		if _, dup := seenImportAliases[imp.Name]; dup {
			c.addWarning(fmt.Sprintf("%s Duplicate import: %s", imp.GetStart(), imp.Name), imp.GetLocation())
			continue
		}
		seenImportAliases[imp.Name] = struct{}{}

		if imp.Kind == parse.ImportKindGo {
			pkg, err := ResolveGoPackage(imp.Path)
			if err != nil {
				c.addError(fmt.Sprintf("Failed to resolve Go import '%s': %v", imp.Path, err), imp.GetLocation())
				continue
			}
			c.program.GoImports[imp.Name] = pkg
			continue
		}

		if strings.HasPrefix(imp.Path, "ard/") {
			// Handle standard library imports
			if mod, ok := findInStdLib(imp.Path); ok {
				c.program.Imports[imp.Name] = mod
			} else {
				c.addError(fmt.Sprintf("Unknown module: %s", imp.Path), imp.GetLocation())
			}
		} else {
			// Handle user module imports
			if c.moduleResolver == nil {
				panic(fmt.Sprintf("No module resolver provided for user import: %s", imp.Path))
			}

			resolved, err := c.moduleResolver.ResolveImport(c.modulePath, imp.Path)
			if err != nil {
				c.addError(fmt.Sprintf("Failed to resolve import '%s': %v", imp.Path, err), imp.GetLocation())
				continue
			}
			filePath := filepath.Clean(resolved.FilePath)

			// Check if module is already cached
			if cachedModule, ok := c.moduleResolver.moduleCache[filePath]; ok {
				c.program.Imports[imp.Name] = cachedModule
				continue
			}
			if slices.Contains(c.moduleResolver.loadingChain, resolved.ModulePath) {
				chain := append(append([]string{}, c.moduleResolver.loadingChain...), resolved.ModulePath)
				c.addError(fmt.Sprintf("circular dependency detected: %s", strings.Join(chain, " -> ")), imp.GetLocation())
				continue
			}
			c.moduleResolver.loadingChain = append(c.moduleResolver.loadingChain, resolved.ModulePath)

			// Load and parse the module file using the resolved package context.
			ast, err := c.moduleResolver.LoadModuleFile(filePath)
			if err != nil {
				c.moduleResolver.loadingChain = c.moduleResolver.loadingChain[:len(c.moduleResolver.loadingChain)-1]
				c.addError(fmt.Sprintf("Failed to load module %s: %v", filePath, err), imp.GetLocation())
				continue
			}

			// Type-check the imported module
			importOptions := c.options
			userModule, diagnostics := check(ast, c.moduleResolver, filePath, resolved.ModulePath, importOptions)
			c.moduleResolver.loadingChain = c.moduleResolver.loadingChain[:len(c.moduleResolver.loadingChain)-1]
			if len(diagnostics) > 0 {
				// Add all diagnostics from the imported module
				for _, diag := range diagnostics {
					c.diagnostics = append(c.diagnostics, diag)
				}
				continue
			}

			// Set the correct module path for the module
			if um, ok := userModule.(*UserModule); ok {
				um.setFilePath(resolved.ModulePath)
			}

			// Cache and add to imports
			c.moduleResolver.moduleCache[filePath] = userModule
			c.program.Imports[imp.Name] = userModule
		}
	}

	// Auto-import prelude modules (only for non-std lib)
	if !strings.HasPrefix(c.filePath, "ard/") {
		if mod, ok := findInStdLib("ard/any"); ok {
			c.program.Imports["Any"] = mod
		}
		if mod, ok := findInStdLib("ard/int"); ok {
			c.program.Imports["Int"] = mod
		}
		if mod, ok := findInStdLib("ard/byte"); ok {
			c.program.Imports["Byte"] = mod
		}
		if mod, ok := findInStdLib("ard/rune"); ok {
			c.program.Imports["Rune"] = mod
		}
		if mod, ok := findInStdLib("ard/list"); ok {
			c.program.Imports["List"] = mod
		}
		if mod, ok := findInStdLib("ard/map"); ok {
			c.program.Imports["Map"] = mod
		}
		if mod, ok := findInStdLib("ard/string"); ok {
			c.program.Imports["Str"] = mod
		}
	}

	c.hoistTopLevelTypeDeclarations()
	c.predeclareTopLevelTypeAliases()
	c.populateTopLevelTypeDefinitions()

	for i := range c.input.Statements {
		if stmt := c.checkedTopLevelTypeStatement(c.input.Statements[i]); stmt != nil {
			c.program.Statements = append(c.program.Statements, *stmt)
			continue
		}
		if isTopLevelTypeDeclaration(c.input.Statements[i]) {
			continue
		}
		if stmt := c.checkStmt(&c.input.Statements[i]); stmt != nil {
			c.program.Statements = append(c.program.Statements, *stmt)
		}
		if c.halted {
			break
		}
	}

	c.validateTopLevelTypeAliases()
	c.checkRecursiveStructLayouts()

	// now that we're done with the aliases, use module paths for the import keys
	for alias, mod := range c.program.Imports {
		delete(c.program.Imports, alias)
		c.program.Imports[mod.Path()] = mod
	}
}

func (c *Checker) scanForUnresolvedGenerics() {
	for _, stmt := range c.program.Statements {
		if stmt.Expr == nil {
			continue
		}

		if typeVar, ok := stmt.Expr.Type().(*TypeVar); ok && typeVar.actual == nil {
			loc := parse.Location{}
			if locatable, ok := stmt.Expr.(interface{ GetLocation() parse.Location }); ok {
				loc = locatable.GetLocation()
			}

			c.addError(fmt.Sprintf("Unresolved generic: %s", typeVar.String()), loc)
			break
		}
	}
}

// This should only be called after .Check()
// The returned module could be problematic if there are diagnostic errors.
func (c *Checker) Module() Module {
	return NewUserModule(c.modulePath, c.program, c.scope)
}

// check is an internal helper for recursive module checking.
// Use New() + Check() + Module() for the public API.
func check(input *parse.Program, moduleResolver *ModuleResolver, filePath string, modulePath string, options CheckOptions) (Module, []Diagnostic) {
	c := New(filePath, input, moduleResolver, options)
	c.modulePath = modulePath

	c.Check()

	return c.Module(), c.diagnostics
}

func (c *Checker) addError(msg string, location parse.Location) {
	c.diagnostics = append(c.diagnostics, Diagnostic{
		Kind:     Error,
		Message:  msg,
		filePath: c.filePath,
		location: location,
	})
}

func (c *Checker) addWarning(msg string, location parse.Location) {
	c.diagnostics = append(c.diagnostics, Diagnostic{
		Kind:     Warn,
		Message:  msg,
		filePath: c.filePath,
		location: location,
	})
}

func (c *Checker) resolveModule(name string) Module {
	if mod, ok := c.program.Imports[name]; ok {
		return mod
	}

	if mod, ok := prelude[name]; ok {
		return mod
	}

	return nil
}

func (c *Checker) findModuleByPath(path string) Module {
	for _, mod := range c.program.Imports {
		if mod.Path() == path {
			return mod
		}
	}

	return nil
}

func namedTypeRequiresTypeArguments(t Type) bool {
	switch typ := t.(type) {
	case *StructDef:
		return (len(typ.GenericParams) > 0 && len(typ.TypeArgs) == 0) || hasGenericsInType(typ)
	default:
		return hasGenericsInType(typ)
	}
}

func collectGenericsFromType(t Type, params *[]string, seen map[string]bool) {
	switch t := t.(type) {
	case *TypeVar:
		if !seen[t.name] {
			*params = append(*params, t.name)
			seen[t.name] = true
		}
	case *List:
		collectGenericsFromType(t.of, params, seen)
	case *Chan:
		collectGenericsFromType(t.of, params, seen)
	case *Receiver:
		collectGenericsFromType(t.of, params, seen)
	case *Sender:
		collectGenericsFromType(t.of, params, seen)
	case *Map:
		collectGenericsFromType(t.key, params, seen)
		collectGenericsFromType(t.value, params, seen)
	case *FunctionDef:
		for _, p := range t.Parameters {
			collectGenericsFromType(p.Type, params, seen)
		}
		if t.ReturnType != nil {
			collectGenericsFromType(t.ReturnType, params, seen)
		}
	case *Maybe:
		collectGenericsFromType(t.of, params, seen)
	case *Result:
		collectGenericsFromType(t.val, params, seen)
		collectGenericsFromType(t.err, params, seen)
	case *StructDef:
		if len(t.TypeArgs) == 0 {
			for _, genericName := range t.GenericParams {
				if !seen[genericName] {
					*params = append(*params, genericName)
					seen[genericName] = true
				}
			}
		}
		for _, typeArg := range t.TypeArgs {
			collectGenericsFromType(typeArg, params, seen)
		}
	}
}

func (c *Checker) specializeAliasedType(originalType Type, typeArgs []parse.DeclaredType, loc parse.Location) Type {
	// 1. Collect generics from the original type
	genericParams := []string{}
	seenGenerics := make(map[string]bool)
	collectGenericsFromType(originalType, &genericParams, seenGenerics)

	if len(genericParams) == 0 {
		c.addError("Type is not generic and cannot be specialized.", loc)
		return originalType
	}

	if len(typeArgs) != len(genericParams) {
		c.addError(fmt.Sprintf("Incorrect number of type arguments: expected %d, got %d", len(genericParams), len(typeArgs)), loc)
		return originalType
	}

	if structDef, ok := originalType.(*StructDef); ok {
		if c.isResolvingStructDefinition(structDef) {
			c.addError(fmt.Sprintf("Recursive generic self-reference %s is not supported yet", structDef.Name), loc)
			return structDef
		}
		c.ensureStructDefinitionResolved(structDef)
		genericParams = []string{}
		seenGenerics = make(map[string]bool)
		collectGenericsFromType(originalType, &genericParams, seenGenerics)
		if len(genericParams) == 0 && len(structDef.GenericParams) > 0 {
			genericParams = append(genericParams, structDef.GenericParams...)
		}
		if len(typeArgs) != len(genericParams) {
			c.addError(fmt.Sprintf("Incorrect number of type arguments: expected %d, got %d", len(genericParams), len(typeArgs)), loc)
			return originalType
		}
		typeVarMap := make(map[string]*TypeVar, len(genericParams))
		for i, typeArg := range typeArgs {
			resolvedArgType := c.resolveType(typeArg)
			typeVarMap[genericParams[i]] = &TypeVar{name: genericParams[i], actual: resolvedArgType, bound: true}
		}
		return copyStructWithTypeVarMap(structDef, typeVarMap)
	}

	// 3. Replace generics
	specializedType := originalType
	for i, typeArg := range typeArgs {
		genericName := genericParams[i]
		resolvedArgType := c.resolveType(typeArg)
		specializedType = replaceGeneric(specializedType, genericName, resolvedArgType)
	}
	return specializedType
}

// validateMapKeyType reports a diagnostic when a map key type is not a valid Go
// map key. Map keys must be Go strictly-comparable so every Ard map lowers to a
// plain Go map (ADR 0031). Unresolved generic parameters are allowed; the
// constraint applies when they are instantiated.
func (c *Checker) validateMapKeyType(key Type, loc parse.Location) {
	if key == nil || isValidMapKeyType(key) {
		return
	}
	// Type annotations are resolved more than once, so dedupe by location to
	// avoid emitting the same map-key diagnostic multiple times.
	if c.reportedMapKeyErrors == nil {
		c.reportedMapKeyErrors = map[parse.Location]bool{}
	}
	if c.reportedMapKeyErrors[loc] {
		return
	}
	c.reportedMapKeyErrors[loc] = true
	c.addError(fmt.Sprintf("Invalid map key type %s: map keys must be comparable (primitives, enums, or structs)", formatTypeForDisplay(key)), loc)
}

// isComparableValueType reports whether a type can be compared with == / != per
// ADR 0031: only primitives and enums (and, via the caller, their nullable
// forms). There is no structural equality over lists, maps, structs, unions, or
// Any.
func isComparableValueType(t Type) bool {
	if t == nil {
		return false
	}
	if t.equal(Int) || t.equal(Float64) || t.equal(Str) || t.equal(Bool) || t.equal(Byte) || t.equal(Rune) || isExplicitScalar(t) {
		return true
	}
	_, isEnum := t.(*Enum)
	return isEnum
}

func isValidMapKeyType(t Type) bool {
	switch ty := t.(type) {
	case *TypeVar:
		if ty.actual != nil {
			return isValidMapKeyType(ty.actual)
		}
		return true
	case *Maybe, *List, *Map, *Result, *Union, *FunctionDef, *Trait, *anyType:
		return false
	default:
		return true
	}
}

func scalarTypeByName(name string) Type {
	switch name {
	case "Int8":
		return Int8
	case "Int16":
		return Int16
	case "Int32":
		return Int32
	case "Int64":
		return Int64
	case "Uint":
		return Uint
	case "Uint8":
		return Uint8
	case "Uint16":
		return Uint16
	case "Uint32":
		return Uint32
	case "Uint64":
		return Uint64
	case "Uintptr":
		return Uintptr
	case "Float32":
		return Float32
	}
	return nil
}

func (c *Checker) resolveType(t parse.DeclaredType) Type {
	var baseType Type
	switch ty := t.(type) {
	case *parse.StringType:
		baseType = Str
	case *parse.IntType:
		baseType = Int
	case *parse.FloatType:
		baseType = Float64
	case *parse.BooleanType:
		baseType = Bool
	case *parse.VoidType:
		baseType = Void

	case *parse.MutableType:
		baseType = MakeMutableRef(c.resolveType(ty.Inner))
	case parse.MutableType:
		baseType = MakeMutableRef(c.resolveType(ty.Inner))
	case *parse.FunctionType:
		// Convert each parameter type and return type
		params := make([]Parameter, len(ty.Params))
		for i, param := range ty.Params {
			mutable := false
			if i < len(ty.ParamMutability) {
				mutable = ty.ParamMutability[i]
			}
			params[i] = Parameter{
				Name:    fmt.Sprintf("arg%d", i),
				Type:    c.resolveType(param),
				Mutable: mutable,
			}
		}
		// If no return type specified, default to Void
		var returnType Type = Void
		if ty.Return != nil {
			returnType = c.resolveType(ty.Return)
		}

		// Create a FunctionDef from the function type syntax
		baseType = &FunctionDef{
			Name:       "<function>",
			Parameters: params,
			ReturnType: returnType,
		}
	case *parse.List:
		of := c.resolveType(ty.Element)
		baseType = MakeList(of)
	case *parse.Map:
		key := c.resolveType(ty.Key)
		value := c.resolveType(ty.Value)
		c.validateMapKeyType(key, ty.Key.GetLocation())
		baseType = MakeMap(key, value)
	case *parse.ResultType:
		val := c.resolveType(ty.Val)
		err := c.resolveType(ty.Err)
		baseType = MakeResult(val, err)
	case *parse.CustomType:
		switch t.GetName() {
		case "Any":
			baseType = Any
			break
		case "Byte":
			baseType = Byte
			break
		case "Rune":
			baseType = Rune
			break
		default:
			baseType = scalarTypeByName(t.GetName())
		}
		if baseType != nil {
			break
		}
		if ty.Type.Target == nil && c.topLevelTypeAliases != nil {
			if _, ok := c.topLevelTypeAliases[t.GetName()]; ok {
				c.resolveTopLevelTypeAlias(t.GetName())
			}
		}

		if sym, ok := c.scope.get(t.GetName()); ok {
			if len(ty.TypeArgs) > 0 {
				baseType = c.specializeAliasedType(sym.Type, ty.TypeArgs, ty.GetLocation())
			} else {
				if namedTypeRequiresTypeArguments(sym.Type) {
					c.addError(fmt.Sprintf("Generic type %s requires type arguments", t.GetName()), ty.GetLocation())
				}
				baseType = sym.Type
			}
			break
		}
		if ty.Type.Target != nil {
			targetName := ty.Type.Target.(*parse.Identifier).Name
			if goPkg := c.program.GoImports[targetName]; goPkg != nil {
				if goType := goPkg.Types[ty.Type.Property.(*parse.Identifier).Name]; goType != nil {
					baseType = goType
					break
				}
			}
			mod := c.resolveModule(targetName)
			if mod != nil {
				// at some point, this will need to unwrap the property down to root for nested paths: `mod::sym::more`
				sym := mod.Get(ty.Type.Property.(*parse.Identifier).Name)
				if !sym.IsZero() {
					if len(ty.TypeArgs) > 0 {
						baseType = c.specializeAliasedType(sym.Type, ty.TypeArgs, ty.GetLocation())
					} else {
						if namedTypeRequiresTypeArguments(sym.Type) {
							c.addError(fmt.Sprintf("Generic type %s::%s requires type arguments", ty.Type.Target, ty.Type.Property), ty.GetLocation())
						}
						baseType = sym.Type
					}
					break
				}
			}
		}
		c.addError(fmt.Sprintf("Unrecognized type: %s", t.GetName()), t.GetLocation())
		return &TypeVar{name: "unknown"}
	case *parse.GenericType:
		if existing := c.scope.findGeneric(ty.Name); existing != nil {
			baseType = existing
		} else {
			baseType = &TypeVar{name: ty.Name}
		}
	default:
		panic(fmt.Errorf("unrecognized type: %s", t.GetName()))
	}

	// If the type is nullable, wrap it in a Maybe
	if t.IsNullable() {
		return &Maybe{of: baseType}
	}

	return baseType
}

func (c *Checker) destructurePath(expr *parse.StaticFunction) (string, string) {
	absolute := expr.Target.String() + "::" + expr.Function.Name
	parts := strings.Split(absolute, "::")

	switch len(parts) {
	case 3:
		return parts[0], strings.Join(parts[1:], "::")
	case 2:
		return parts[0], parts[1]
	default:
		return parts[0], parts[0]
	}
}

func (c *Checker) pushFunctionGenericContext(fnDef *FunctionDef, extraParams ...string) {
	params := genericParamsForFunction(fnDef)
	params = appendUniqueStrings(params, extraParams...)
	if len(params) == 0 {
		c.genericContextStack = append(c.genericContextStack, nil)
		return
	}
	context := make(map[string]bool, len(params))
	for _, param := range params {
		context[param] = true
	}
	c.genericContextStack = append(c.genericContextStack, context)
}

func (c *Checker) popFunctionGenericContext() {
	if len(c.genericContextStack) == 0 {
		return
	}
	c.genericContextStack = c.genericContextStack[:len(c.genericContextStack)-1]
}

func (c *Checker) genericInCurrentContext(name string) bool {
	if len(c.genericContextStack) == 0 {
		return false
	}
	current := c.genericContextStack[len(c.genericContextStack)-1]
	return current[name]
}

func (c *Checker) validateExplicitCallTypeArg(typeArg parse.DeclaredType) {
	params := []string{}
	seen := map[string]bool{}
	collectGenericParamsFromDeclaredType(typeArg, &params, seen)
	for _, param := range params {
		if c.genericInCurrentContext(param) {
			continue
		}
		c.addError(fmt.Sprintf("unbound generic type argument $%s", param), typeArg.GetLocation())
	}
}

func (c *Checker) resolveCallTypeArgs(typeArgs []parse.DeclaredType) []Type {
	if len(typeArgs) == 0 {
		return nil
	}
	resolved := make([]Type, len(typeArgs))
	for i, typeArg := range typeArgs {
		resolved[i] = c.resolveType(typeArg)
		c.validateExplicitCallTypeArg(typeArg)
	}
	return resolved
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values)+len(additions))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}

func genericParamsForType(typ Type) []string {
	params := []string{}
	seen := map[string]bool{}
	collectGenericsFromType(typ, &params, seen)
	return params
}

func genericParamsForFunction(fnDef *FunctionDef) []string {
	if fnDef == nil {
		return nil
	}
	genericParams := []string{}
	seen := map[string]bool{}
	for _, param := range fnDef.GenericParams {
		if !seen[param] {
			genericParams = append(genericParams, param)
			seen[param] = true
		}
	}
	for _, param := range fnDef.Parameters {
		collectGenericsFromType(param.Type, &genericParams, seen)
	}
	if fnDef.ReturnType != nil {
		collectGenericsFromType(fnDef.ReturnType, &genericParams, seen)
	}
	return genericParams
}

func (c *Checker) explicitMethodGenericParams(fnDef *FunctionDef, subject Type) []string {
	if fnDef == nil {
		return nil
	}
	if len(fnDef.GenericParams) > 0 {
		return append([]string(nil), fnDef.GenericParams...)
	}
	params := genericParamsForFunction(fnDef)
	structType, ok := subject.(*StructDef)
	if !ok {
		return params
	}
	originalDef := c.structDefinition(structType)
	if originalDef == nil || len(originalDef.GenericParams) == 0 {
		return params
	}
	receiverGenerics := make(map[string]bool, len(originalDef.GenericParams))
	for _, param := range originalDef.GenericParams {
		receiverGenerics[param] = true
	}
	out := make([]string, 0, len(params))
	for _, param := range params {
		if !receiverGenerics[param] {
			out = append(out, param)
		}
	}
	return out
}

func formatTypeForDisplay(t Type) string {
	// For StructDef with generic fields, show type parameters
	if structDef, ok := t.(*StructDef); ok {
		if len(structDef.TypeArgs) > 0 {
			return structDef.String()
		}
		if resultType, hasResult := structDef.Fields["result"]; hasResult {
			// The result field's type indicates the generic parameter
			return fmt.Sprintf("%s<%s>", structDef.String(), resultType.String())
		}
	}
	return t.String()
}

func mergeMatchResultType(c *Checker, current Type, next Type, loc parse.Location, allowMixedVoid bool) (Type, bool) {
	if current == nil {
		return next, true
	}
	if expected, got, ok := mixedVoidMatchTypes(current, next); ok && !allowMixedVoid {
		c.addError(matchBranchTypeMismatch(expected, got), loc)
		return nil, false
	}
	merged, ok := commonResultType(current, next)
	if ok {
		return merged, true
	}
	if c.expectedExpr != nil && c.areCompatible(c.expectedExpr, current) && c.areCompatible(c.expectedExpr, next) {
		return c.expectedExpr, true
	}
	c.addError(typeMismatch(current, next), loc)
	return nil, false
}

func mixedVoidMatchTypes(left Type, right Type) (Type, Type, bool) {
	if left == nil || right == nil || left == right {
		return nil, nil, false
	}
	if left == Void {
		return right, left, true
	}
	if right == Void {
		return left, right, true
	}
	return nil, nil, false
}

func matchBranchTypeMismatch(expected Type, got Type) string {
	return fmt.Sprintf("Type mismatch in match branches: expected %s, got %s", formatTypeForDisplay(expected), formatTypeForDisplay(got))
}

func typeMismatch(expected, got Type) string {
	exMsg := formatTypeForDisplay(expected)
	if _, isTrait := expected.(*Trait); isTrait {
		exMsg = "implementation of " + exMsg
	}
	return fmt.Sprintf("Type mismatch: Expected %s, got %s", exMsg, formatTypeForDisplay(got))
}

func (c *Checker) areCompatible(expected Type, actual Type) bool {
	if _, ok := expected.(*anyType); ok {
		return true
	}
	if trait, ok := expected.(*Trait); ok {
		return actual.hasTrait(trait)
	}
	return expected.equal(actual)
}

func (c *Checker) checkStmt(stmt *parse.Statement) *Statement {
	if c.halted {
		return nil
	}
	if c.isDuplicateTopLevelTypeDeclaration(*stmt) {
		return nil
	}
	switch s := (*stmt).(type) {
	case *parse.Comment:
		return nil
	case *parse.Break:
		return &Statement{Break: true}
	case *parse.TraitDefinition:
		{
			trait, ok := c.hoistedTrait(s.Name.Name)
			if !ok {
				return nil
			}
			methods := make([]FunctionDef, len(s.Methods))
			for i, method := range s.Methods {
				params := make([]Parameter, len(method.Parameters))
				for j, param := range method.Parameters {
					paramType := c.resolveType(param.Type)
					if paramType == nil {
						c.addError(fmt.Sprintf("Unrecognized type: %s", param.Type.GetName()), param.Type.GetLocation())
						continue
					}
					params[j] = Parameter{Name: param.Name, Type: paramType, Mutable: param.Mutable}
				}

				var returnType Type = Void
				if method.ReturnType != nil {
					returnType = c.resolveType(method.ReturnType)
					if returnType == nil {
						c.addError(fmt.Sprintf("Unrecognized return type: %s", method.ReturnType.GetName()), method.ReturnType.GetLocation())
						continue
					}
				}

				methods[i] = FunctionDef{
					Private:    false,
					Name:       method.Name,
					Parameters: params,
					ReturnType: returnType,
				}
			}

			trait.private = s.Private
			trait.Name = s.Name.Name
			trait.methods = methods
			return nil
		}
	case *parse.TraitImplementation:
		{
			var sym Symbol
			switch name := s.Trait.(type) {
			case parse.Identifier:
				if s, ok := c.scope.get(name.Name); ok {
					sym = *s
				}
			case parse.StaticProperty:
				mod := c.resolveModule(name.Target.(*parse.Identifier).Name)
				if mod != nil {
					if propId, ok := name.Property.(*parse.Identifier); ok {
						sym = mod.Get(propId.Name)
					} else {
						c.addError(fmt.Sprintf("Bad path: %s", name), name.Property.GetLocation())
						return nil
					}
				}
			default:
				panic(fmt.Errorf("Unsupported trait node: %s", name))
			}

			if sym.IsZero() {
				c.addError(fmt.Sprintf("Undefined trait: %s", s.Trait), s.Trait.GetLocation())
				return nil
			}

			trait, ok := sym.Type.(*Trait)
			if !ok {
				c.addError(fmt.Sprintf("%T is not a trait", sym.Type), s.Trait.GetLocation())
				return nil
			}

			// Check that the type exists
			typeSym, ok := c.scope.get(s.ForType.Name)
			if !ok {
				c.addError(fmt.Sprintf("Undefined type: %s", s.ForType.Name), s.ForType.GetLocation())
				return nil
			}

			switch targetType := typeSym.Type.(type) {
			case *StructDef:
				// Verify that all required methods are implemented
				traitMethods := trait.GetMethods()
				implementedMethods := make(map[string]bool)

				// Check each method in the implementation
				for _, method := range s.Methods {
					implementedMethods[method.Name] = true

					// Find the corresponding trait method
					var traitMethod *FunctionDef
					for _, m := range traitMethods {
						if m.Name == method.Name {
							traitMethod = &m
							break
						}
					}

					if traitMethod == nil {
						c.addWarning(fmt.Sprintf("Method %s is not part of trait %s", method.Name, trait.name()), method.GetLocation())
						continue
					}

					// Check parameter count
					if len(method.Parameters) != len(traitMethod.Parameters) {
						c.addError(fmt.Sprintf("Method %s has wrong number of parameters", method.Name), method.GetLocation())
						continue
					}

					params := make([]Parameter, len(method.Parameters))
					for i, param := range method.Parameters {
						paramType := c.resolveType(param.Type)
						expectedType := traitMethod.Parameters[i].Type
						if !paramType.equal(expectedType) {
							c.addError(typeMismatch(expectedType, paramType), param.GetLocation())
						}

						if param.Mutable != traitMethod.Parameters[i].Mutable {
							c.addError(fmt.Sprintf("Trait method '%s' parameter '%s' mutability mismatch", method.Name, param.Name), param.GetLocation())
						}

						params[i] = Parameter{Name: param.Name, Type: paramType, Mutable: param.Mutable}
					}

					// Check return type
					var returnType Type = Void
					if method.ReturnType != nil {
						returnType = c.resolveType(method.ReturnType)
					}
					if !traitMethod.ReturnType.equal(returnType) {
						c.addError(fmt.Sprintf("Trait method '%s' has return type of %s", method.Name, traitMethod.ReturnType), method.GetLocation())
						continue
					}

					// if we made it this far, it's a valid implementation
					fnDef := c.checkFunction(&method, func() {
						c.scope.add(s.Receiver.Name, targetType, method.Mutates)
					}, genericParamsForType(targetType)...)
					fnDef.Receiver = s.Receiver.Name
					fnDef.Mutates = method.Mutates
					// add the method to the struct method table
					c.addStructMethod(targetType, fnDef)
				}

				// Check if all required methods are implemented
				for _, method := range traitMethods {
					if !implementedMethods[method.Name] {
						c.addError(fmt.Sprintf("Missing method '%s' in trait '%s'", method.Name, trait.name()), s.GetLocation())
					}
				}

				// Add the trait to the struct type's traits list
				targetType.Traits = append(targetType.Traits, trait)

				// Return the struct so downstream backends can register the new trait methods
				return &Statement{Stmt: targetType}

			case *Enum:
				// Verify that all required methods are implemented (same logic as structs)
				traitMethods := trait.GetMethods()
				implementedMethods := make(map[string]bool)

				// Check each method in the implementation
				for _, method := range s.Methods {
					implementedMethods[method.Name] = true

					// Find the corresponding trait method
					var traitMethod *FunctionDef
					for _, m := range traitMethods {
						if m.Name == method.Name {
							traitMethod = &m
							break
						}
					}

					if traitMethod == nil {
						c.addWarning(fmt.Sprintf("Method %s is not part of trait %s", method.Name, trait.name()), method.GetLocation())
						continue
					}

					// Check parameter count
					if len(method.Parameters) != len(traitMethod.Parameters) {
						c.addError(fmt.Sprintf("Method %s has wrong number of parameters", method.Name), method.GetLocation())
						continue
					}

					params := make([]Parameter, len(method.Parameters))
					for i, param := range method.Parameters {
						paramType := c.resolveType(param.Type)
						expectedType := traitMethod.Parameters[i].Type
						if !paramType.equal(expectedType) {
							c.addError(typeMismatch(expectedType, paramType), param.GetLocation())
						}

						if param.Mutable != traitMethod.Parameters[i].Mutable {
							c.addError(fmt.Sprintf("Trait method '%s' parameter '%s' mutability mismatch", method.Name, param.Name), param.GetLocation())
						}

						params[i] = Parameter{Name: param.Name, Type: paramType, Mutable: param.Mutable}
					}

					// Check return type
					var returnType Type = Void
					if method.ReturnType != nil {
						returnType = c.resolveType(method.ReturnType)
					}
					if !traitMethod.ReturnType.equal(returnType) {
						c.addError(fmt.Sprintf("Trait method '%s' has return type of %s", method.Name, traitMethod.ReturnType), method.GetLocation())
						continue
					}

					// if we made it this far, it's a valid implementation
					fnDef := c.checkFunction(&method, func() {
						c.scope.add(s.Receiver.Name, targetType, false) // Enums are immutable, so always false
					}, genericParamsForType(targetType)...)
					fnDef.Receiver = s.Receiver.Name
					// Enums cannot have mutating methods
					if method.Mutates {
						c.addError("Enum methods cannot be mutating", method.GetLocation())
					}
					fnDef.Mutates = false // Enums are always immutable

					// Ensure enum has Methods map initialized
					if targetType.Methods == nil {
						targetType.Methods = make(map[string]*FunctionDef)
					}
					// add the method to the enum
					targetType.Methods[method.Name] = fnDef
				}

				// Check if all required methods are implemented
				for _, method := range traitMethods {
					if !implementedMethods[method.Name] {
						c.addError(fmt.Sprintf("Missing method '%s' in trait '%s'", method.Name, trait.name()), s.GetLocation())
					}
				}

				// Add the trait to the enum type's traits list
				targetType.Traits = append(targetType.Traits, trait)

				// Return the enum so downstream backends can register the new trait methods
				return &Statement{Stmt: targetType}

			default:
				c.addError(fmt.Sprintf("%s cannot implement a Trait", s.ForType.Name), s.ForType.GetLocation())
				return nil
			}
		}
	case *parse.TypeDeclaration:
		{
			if len(s.Type) == 1 {
				if _, exists := c.scope.get(s.Name.Name); exists {
					return nil
				}
			}
			// Handle type declaration (type unions/aliases)
			types := make([]Type, len(s.Type))
			for i, declType := range s.Type {
				resolvedType := c.resolveType(declType)
				if resolvedType == nil {
					c.addError(fmt.Sprintf("Unrecognized type: %s", declType.GetName()), declType.GetLocation())
					return nil
				}
				types[i] = resolvedType
			}

			if len(types) == 1 {
				// It's a type alias
				c.scope.add(s.Name.Name, types[0], false)
				return nil
			}

			unionType, ok := c.hoistedUnion(s.Name.Name)
			if !ok {
				return nil
			}
			unionType.Name = s.Name.Name
			unionType.ModulePath = c.typeOwnerPath()
			unionType.Types = types
			unionType.Private = s.Private
			return &Statement{Stmt: unionType}
		}
	case *parse.VariableDeclaration:
		{
			var val Expression
			c.withValueExprContext(func() {
				if s.Type == nil {
					switch literal := s.Value.(type) {
					case *parse.ListLiteral:
						if expr := c.checkList(nil, literal); expr != nil {
							val = expr
						}
					case *parse.MapLiteral:
						if expr := c.checkMap(nil, literal); expr != nil {
							val = expr
						}
					default:
						val = c.checkExpr(s.Value)
					}
				} else {
					expected := c.resolveType(s.Type)
					if expected == Void {
						c.addError("Cannot assign a void value", s.Value.GetLocation())
						return
					}

					switch literal := s.Value.(type) {
					case *parse.ListLiteral:
						if expr := c.checkList(expected, literal); expr != nil {
							val = expr
						}
					case *parse.MapLiteral:
						if expr := c.checkMap(expected, literal); expr != nil {
							val = expr
						}
					default:
						if expected != nil {
							val = c.checkExprAs(s.Value, expected)
						}
					}
				}
			})

			if val == nil {
				return nil
			}

			__type := val.Type()

			if s.Type != nil {
				if expected := c.resolveType(s.Type); expected != nil {
					if !c.areCompatible(expected, val.Type()) {
						c.addError(typeMismatch(expected, val.Type()), s.Value.GetLocation())
						return nil
					}
					__type = expected
				}
			}

			v := &VariableDef{
				Mutable: s.Mutable,
				Name:    s.Name,
				Value:   val,
				__type:  __type,
			}
			c.scope.add(v.Name, v.__type, v.Mutable)
			return &Statement{
				Stmt: v,
			}
		}
	case *parse.VariableAssignment:
		{
			if id, ok := s.Target.(*parse.Identifier); ok {
				target, ok := c.scope.get(id.Name)
				if !ok {
					c.addError(fmt.Sprintf("Undefined: %s", id.Name), s.Target.GetLocation())
					return nil
				}

				if !target.mutable {
					c.addError(fmt.Sprintf("Immutable variable: %s", target.Name), s.Target.GetLocation())
					return nil
				}

				var value Expression
				c.withValueExprContext(func() {
					value = c.checkExpr(s.Value)
				})
				if value == nil {
					return nil
				}

				if !c.areCompatible(target.Type, value.Type()) {
					c.addError(typeMismatch(target.Type, value.Type()), s.Value.GetLocation())
					return nil
				}

				return &Statement{
					Stmt: &Reassignment{Target: &Variable{*target}, Value: value},
				}
			}

			if ip, ok := s.Target.(*parse.InstanceProperty); ok {
				subject := c.checkExpr(ip)
				if subject == nil {
					return nil
				}
				var value Expression
				c.withValueExprContext(func() {
					value = c.checkExpr(s.Value)
				})
				if value == nil {
					return nil
				}

				if !c.isMutable(subject) {
					c.addError(fmt.Sprintf("Immutable: %s", ip), s.Target.GetLocation())
					return nil
				}
				if _, ok := subject.(*ForeignFieldAccess); ok && !c.areCompatible(subject.Type(), value.Type()) {
					c.addError(typeMismatch(subject.Type(), value.Type()), s.Value.GetLocation())
					return nil
				}

				return &Statement{
					Stmt: &Reassignment{Target: subject, Value: value},
				}
			}

			panic(fmt.Sprintf("Unsupported reassignment target: %T", s.Target))
		}
	case *parse.WhileLoop:
		{
			// Check the condition expression
			var condition Expression
			if s.Condition == nil {
				condition = &BoolLiteral{true}
			} else {
				condition = c.checkExpr(s.Condition)
			}

			if condition == nil {
				return nil
			}

			// Condition must be a boolean expression
			if condition.Type() != Bool {
				c.addError("While loop condition must be a boolean expression", s.Condition.GetLocation())
				return nil
			}

			// Check the body of the loop
			body := c.checkBlock(s.Body, nil)

			// Create and return the while loop
			loop := &WhileLoop{
				Condition: condition,
				Body:      body,
			}

			return &Statement{Stmt: loop}
		}
	case *parse.ForLoop:
		{
			// Create a new scope for the loop body and initialization
			parent := c.scope
			scope := makeScope(parent)
			c.scope = &scope
			defer func() {
				c.scope = parent
			}()

			// Check the initialization statement - handle it as a variable declaration
			initDeclStmt := parse.Statement(s.Init)
			initStmt := c.checkStmt(&initDeclStmt)
			if initStmt == nil || initStmt.Stmt == nil {
				c.addError("Invalid for loop initialization", s.Init.GetLocation())
				return nil
			}
			initVarDef, ok := initStmt.Stmt.(*VariableDef)
			if !ok {
				c.addError("For loop initialization must be a variable declaration", s.Init.GetLocation())
				return nil
			}

			// Check the condition expression
			condition := c.checkExpr(s.Condition)
			if condition == nil {
				return nil
			}

			// Condition must be a boolean expression
			if condition.Type() != Bool {
				c.addError("For loop condition must be a boolean expression", s.Condition.GetLocation())
				return nil
			}

			// Check the update statement - handle it as a variable assignment
			incrStmt := parse.Statement(s.Incrementer)
			updateStmt := c.checkStmt(&incrStmt)
			if updateStmt == nil || updateStmt.Stmt == nil {
				c.addError("Invalid for loop update expression", s.Incrementer.GetLocation())
				return nil
			}
			update, ok := updateStmt.Stmt.(*Reassignment)
			if !ok {
				c.addError("For loop update must be a reassignment", s.Incrementer.GetLocation())
				return nil
			}

			// Check the body of the loop
			body := c.checkBlock(s.Body, nil)

			// Create and return the for loop
			loop := &ForLoop{
				Init:      initVarDef,
				Condition: condition,
				Update:    update,
				Body:      body,
			}

			return &Statement{Stmt: loop}
		}
	case *parse.RangeLoop:
		{
			start, end := c.checkExpr(s.Start), c.checkExpr(s.End)
			if start == nil || end == nil {
				return nil
			}
			if start.Type() != end.Type() {
				c.addError(fmt.Sprintf("Invalid range: %s..%s", start.Type(), end.Type()), s.Start.GetLocation())
				return nil
			}

			if start.Type() == Int {
				loop := &ForIntRange{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					Start:  start,
					End:    end,
				}
				body := c.checkBlock(s.Body, func() {
					c.scope.add(s.Cursor.Name, start.Type(), false)
					if loop.Index != "" {
						c.scope.add(loop.Index, Int, false)
					}
				})
				loop.Body = body
				return &Statement{Stmt: loop}
			}

			panic(fmt.Errorf("Cannot create range of %s", start.Type()))
		}
	case *parse.ForInLoop:
		{
			iterValue := c.checkExpr(s.Iterable)
			if iterValue == nil {
				return nil
			}

			// Handle strings specifically
			if iterValue.Type() == Str {
				loop := &ForInStr{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					Value:  iterValue,
				}

				// Create a new scope for the loop body where the cursor is defined
				body := c.checkBlock(s.Body, func() {
					// Direct string iteration yields Unicode scalar values.
					c.scope.add(s.Cursor.Name, Rune, false)
					if loop.Index != "" {
						c.scope.add(loop.Index, Int, false)
					}
				})

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			// Handle integer iteration (for i in n - sugar for 0..n)
			if iterValue.Type() == Int {
				// This is syntax sugar for a range from 0 to n
				loop := &ForIntRange{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					Start:  &IntLiteral{0}, // Start from 0
					End:    iterValue,      // End at the specified number
				}

				// Create a new scope for the loop body where the cursor is defined
				body := c.checkBlock(s.Body, func() {
					// Add the cursor variable to the scope
					c.scope.add(s.Cursor.Name, Int, false)
					if loop.Index != "" {
						c.scope.add(loop.Index, Int, false)
					}
				})

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			if listType, ok := iterValue.Type().(*List); ok {
				// This is syntax sugar for a range from 0 to n
				loop := &ForInList{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					List:   iterValue,
				}
				cursorMutable := c.isMutable(iterValue)

				body := c.checkBlock(s.Body, func() {
					// Add the cursor variable to the scope
					c.scope.add(s.Cursor.Name, listType.of, cursorMutable)
					if loop.Index != "" {
						c.scope.add(loop.Index, Int, false)
					}
				})

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			if mapType, ok := iterValue.Type().(*Map); ok {
				iterable := c.checkExpr(s.Iterable)
				if iterable == nil {
					return nil
				}

				loop := &ForInMap{
					Key: s.Cursor.Name,
					Val: s.Cursor2.Name,
					Map: iterable,
				}

				valueMutable := c.isMutable(iterable)
				body := c.checkBlock(s.Body, func() {
					// Add the cursors to the scope
					c.scope.add(loop.Key, mapType.Key(), false)
					c.scope.add(loop.Val, mapType.Value(), valueMutable)
				})

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			// Currently we only support string, integer, and List iteration
			c.addError(fmt.Sprintf("Cannot iterate over a %s", iterValue.Type()), s.Iterable.GetLocation())
			return nil
		}
	case *parse.EnumDefinition:
		{
			enum, ok := c.hoistedEnum(s.Name)
			if !ok {
				return nil
			}
			if len(s.Variants) == 0 {
				c.addError("Enums must have at least one variant", s.GetLocation())
				return nil
			}

			// Check for duplicate variant names
			seenNames := make(map[string]bool)
			for _, variant := range s.Variants {
				if seenNames[variant.Name] {
					c.addError(fmt.Sprintf("Duplicate variant: %s", variant.Name), s.GetLocation())
					return nil
				}
				seenNames[variant.Name] = true
			}

			// Compute discriminant values
			var computedValues []EnumValue
			var nextValue int = 0
			seenValues := make(map[int]string) // Detect duplicate discriminants

			for _, variant := range s.Variants {
				var value int

				if variant.Value != nil {
					// Parse explicit value
					expr := c.checkExpr(variant.Value)
					if expr == nil {
						continue
					}

					// Value must be an integer literal
					intLit, ok := expr.(*IntLiteral)
					if !ok {
						c.addError("Enum variant value must be an integer literal", variant.Value.GetLocation())
						continue
					}
					value = intLit.Value
					nextValue = value + 1
				} else {
					// Auto-assign
					value = nextValue
					nextValue++
				}

				// Check for duplicate discriminant values
				if existing, found := seenValues[value]; found {
					c.addError(fmt.Sprintf("Duplicate enum value %d (also used by variant %s)", value, existing), s.GetLocation())
					return nil
				}
				seenValues[value] = variant.Name

				computedValues = append(computedValues, EnumValue{
					Name:  variant.Name,
					Value: value,
				})
			}

			enum.Private = s.Private
			enum.Name = s.Name
			enum.ModulePath = c.typeOwnerPath()
			enum.Values = computedValues
			if enum.Methods == nil {
				enum.Methods = make(map[string]*FunctionDef)
			}
			return nil
		}
	case *parse.StructDefinition:
		{
			def, ok := c.hoistedStruct(s.Name.Name)
			if !ok {
				return nil
			}
			c.populateStructDefinition(def, s)
			return &Statement{Stmt: def}
		}
	case *parse.ImplBlock:
		{
			sym, ok := c.scope.get(s.Target.Name)
			if !ok {
				c.addError(fmt.Sprintf("Undefined: %s", s.Target), s.Target.GetLocation())
				return nil
			}

			switch def := sym.Type.(type) {
			case *StructDef:
				for _, method := range s.Methods {
					fnDef := c.checkFunction(&method, func() {
						c.scope.add(s.Receiver.Name, def, method.Mutates)
					}, genericParamsForType(def)...)
					if fnDef != nil {
						fnDef.Receiver = s.Receiver.Name
						fnDef.Mutates = method.Mutates
						c.addStructMethod(def, fnDef)
					}
				}
				return &Statement{Stmt: def}
			case *Enum:
				if def.Methods == nil {
					def.Methods = make(map[string]*FunctionDef)
				}
				for _, method := range s.Methods {
					if method.Mutates {
						c.addError("Enum methods cannot be mutating", method.GetLocation())
					}
					fnDef := c.checkFunction(&method, func() {
						c.scope.add(s.Receiver.Name, def, false)
					}, genericParamsForType(def)...)
					fnDef.Receiver = s.Receiver.Name
					def.Methods[method.Name] = fnDef
				}
				return &Statement{Stmt: def}
			default:
				c.addError(fmt.Sprintf("Can only implement methods on structs and enums, not %s", sym.Type), s.Target.GetLocation())
				return nil
			}
		}
	case nil:
		return nil
	default:
		expr := c.withDiscardExprContext(func() Expression {
			return c.checkExpr((parse.Expression)(*stmt))
		})
		if expr == nil {
			return nil
		}
		return &Statement{Expr: expr}
	}
}

func (c *Checker) checkList(declaredType Type, expr *parse.ListLiteral) *ListLiteral {
	if declaredType != nil {
		expectedElementType := declaredType.(*List).of
		elements := make([]Expression, len(expr.Items))
		hasError := false
		for i := range expr.Items {
			item := expr.Items[i]
			element := c.checkExpr(item)
			if element == nil {
				hasError = true
				continue
			}
			if !c.areCompatible(expectedElementType, element.Type()) {
				c.addError(typeMismatch(expectedElementType, element.Type()), item.GetLocation())
				hasError = true
				continue
			}
			elements[i] = element
		}
		if hasError {
			return nil
		}

		listType := declaredType.(*List)
		return &ListLiteral{
			Elements: elements,
			_type:    listType,
			ListType: listType,
		}
	}

	if len(expr.Items) == 0 {
		c.addError("Empty lists need an explicit type", expr.GetLocation())
		c.halted = true
		listType := MakeList(Void)
		return &ListLiteral{_type: listType, ListType: listType, Elements: []Expression{}}
	}

	hasError := false
	var elementType Type
	elements := make([]Expression, len(expr.Items))
	for i := range expr.Items {
		item := expr.Items[i]
		element := c.checkExpr(item)
		if element == nil {
			hasError = true
			continue
		}

		if elementType == nil {
			elementType = element.Type()
		} else if !elementType.equal(element.Type()) {
			c.addError("Type mismatch: A list can only contain values of single type", item.GetLocation())
			hasError = true
			continue
		}

		elements[i] = element
	}

	if hasError || elementType == nil {
		return nil
	}

	listType := MakeList(elementType)
	return &ListLiteral{
		Elements: elements,
		_type:    listType,
		ListType: listType,
	}
}

func (c *Checker) checkBlock(stmts []parse.Statement, setup func()) *Block {
	return c.checkBlockWithExpected(stmts, setup, nil, false)
}

func (c *Checker) checkBlockWithExpected(stmts []parse.Statement, setup func(), expectedFinal Type, onlyMatchFinal bool) *Block {
	if len(stmts) == 0 {
		return &Block{Stmts: []Statement{}}
	}

	parent := c.scope
	newScope := makeScope(parent)
	c.scope = &newScope
	defer func() {
		c.scope = parent
	}()

	if setup != nil {
		setup()
	}

	lastExprIndex := -1
	if expectedFinal != nil {
		for i := len(stmts) - 1; i >= 0; i-- {
			if _, ok := stmts[i].(*parse.Comment); ok {
				continue
			}
			if c.canCheckStatementAsExpectedExpression(stmts[i], expectedFinal, onlyMatchFinal) {
				lastExprIndex = i
			}
			break
		}
	}

	block := &Block{Stmts: make([]Statement, len(stmts)), DiscardFinalValue: expectedFinal == Void}
	for i := range stmts {
		if i == lastExprIndex {
			expr := c.checkExprAs(stmts[i].(parse.Expression), expectedFinal)
			if expr != nil {
				block.Stmts[i] = Statement{Expr: expr}
			}
		} else if stmt := c.checkStmt(&stmts[i]); stmt != nil {
			block.Stmts[i] = *stmt
		}
		if c.halted {
			break
		}
	}
	return block
}

func (c *Checker) canCheckStatementAsExpectedExpression(stmt parse.Statement, expectedFinal Type, onlyMatchFinal bool) bool {
	if onlyMatchFinal && expectedFinal != Void {
		switch stmt.(type) {
		case *parse.MatchExpression, *parse.ConditionalMatchExpression, *parse.SelectExpression, *parse.InstanceMethod, *parse.UnsafeBlock:
			return true
		default:
			return false
		}
	}
	if expectedFinal == Void {
		switch stmt.(type) {
		case *parse.StrLiteral, *parse.RuneLiteral, *parse.BoolLiteral, *parse.VoidLiteral, *parse.NumLiteral, *parse.InterpolatedStr,
			*parse.Identifier, *parse.FunctionCall, *parse.FunctionValueCall, *parse.InstanceProperty, *parse.InstanceMethod,
			*parse.UnaryExpression, *parse.BinaryExpression, *parse.ChainedComparison, *parse.StaticFunction,
			*parse.IfStatement, *parse.AnonymousFunction, *parse.ListLiteral, *parse.MapLiteral,
			*parse.MatchExpression, *parse.ConditionalMatchExpression, *parse.SelectExpression, *parse.StaticProperty,
			*parse.StructInstance, *parse.Try, *parse.BlockExpression, *parse.UnsafeBlock:
			return true
		default:
			return false
		}
	}

	switch stmt.(type) {
	case *parse.MatchExpression, *parse.ConditionalMatchExpression, *parse.SelectExpression, *parse.IfStatement, *parse.StaticFunction, *parse.FunctionValueCall, *parse.ListLiteral, *parse.MapLiteral, *parse.AnonymousFunction, *parse.UnsafeBlock:
		return true
	default:
		return false
	}
}

func parseStatementsContainBreak(stmts []parse.Statement) bool {
	for _, stmt := range stmts {
		if parseStatementContainsBreak(stmt) {
			return true
		}
	}
	return false
}

func parseStatementContainsBreak(stmt parse.Statement) bool {
	switch s := stmt.(type) {
	case nil:
		return false
	case *parse.Break:
		return true
	case *parse.WhileLoop:
		return parseExpressionContainsBreak(s.Condition) || parseStatementsContainBreak(s.Body)
	case *parse.RangeLoop:
		return parseExpressionContainsBreak(s.Start) || parseExpressionContainsBreak(s.End) || parseStatementsContainBreak(s.Body)
	case *parse.ForInLoop:
		return parseExpressionContainsBreak(s.Iterable) || parseStatementsContainBreak(s.Body)
	case *parse.ForLoop:
		return parseStatementContainsBreak(s.Init) || parseExpressionContainsBreak(s.Condition) || parseStatementContainsBreak(s.Incrementer) || parseStatementsContainBreak(s.Body)
	case *parse.IfStatement:
		return parseExpressionContainsBreak(s.Condition) || parseStatementsContainBreak(s.Body) || parseStatementContainsBreak(s.Else)
	case *parse.MatchExpression, *parse.ConditionalMatchExpression, *parse.Try, *parse.BlockExpression, *parse.UnsafeBlock, *parse.AnonymousFunction:
		return parseExpressionContainsBreak(s.(parse.Expression))
	default:
		if expr, ok := stmt.(parse.Expression); ok {
			return parseExpressionContainsBreak(expr)
		}
		return false
	}
}

func parseExpressionContainsBreak(expr parse.Expression) bool {
	switch e := expr.(type) {
	case nil:
		return false
	case *parse.UnaryExpression:
		return parseExpressionContainsBreak(e.Operand)
	case *parse.BinaryExpression:
		return parseExpressionContainsBreak(e.Left) || parseExpressionContainsBreak(e.Right)
	case *parse.ChainedComparison:
		for _, operand := range e.Operands {
			if parseExpressionContainsBreak(operand) {
				return true
			}
		}
	case *parse.RangeExpression:
		return parseExpressionContainsBreak(e.Start) || parseExpressionContainsBreak(e.End)
	case *parse.InterpolatedStr:
		for _, chunk := range e.Chunks {
			if parseExpressionContainsBreak(chunk) {
				return true
			}
		}
	case *parse.FunctionCall:
		for _, arg := range e.Args {
			if parseExpressionContainsBreak(arg.Value) {
				return true
			}
		}
	case *parse.FunctionValueCall:
		if parseExpressionContainsBreak(e.Callee) {
			return true
		}
		for _, arg := range e.Args {
			if parseExpressionContainsBreak(arg.Value) {
				return true
			}
		}
	case *parse.InstanceProperty:
		return parseExpressionContainsBreak(e.Target) || parseExpressionContainsBreak(e.Property)
	case *parse.InstanceMethod:
		if parseExpressionContainsBreak(e.Target) {
			return true
		}
		for _, arg := range e.Method.Args {
			if parseExpressionContainsBreak(arg.Value) {
				return true
			}
		}
	case *parse.StaticProperty:
		return parseExpressionContainsBreak(e.Target) || parseExpressionContainsBreak(e.Property)
	case *parse.StaticFunction:
		if parseExpressionContainsBreak(e.Target) {
			return true
		}
		for _, arg := range e.Function.Args {
			if parseExpressionContainsBreak(arg.Value) {
				return true
			}
		}
	case *parse.ListLiteral:
		for _, item := range e.Items {
			if parseExpressionContainsBreak(item) {
				return true
			}
		}
	case *parse.MapLiteral:
		for _, entry := range e.Entries {
			if parseExpressionContainsBreak(entry.Key) || parseExpressionContainsBreak(entry.Value) {
				return true
			}
		}
	case *parse.StructInstance:
		for _, prop := range e.Properties {
			if parseExpressionContainsBreak(prop.Value) {
				return true
			}
		}
	case *parse.SelectExpression:
		for _, arm := range e.Cases {
			if arm.Op != nil && parseExpressionContainsBreak(arm.Op) {
				return true
			}
			if parseStatementsContainBreak(arm.Body) {
				return true
			}
		}
	case *parse.MatchExpression:
		if parseExpressionContainsBreak(e.Subject) {
			return true
		}
		for _, matchCase := range e.Cases {
			if parseExpressionContainsBreak(matchCase.Pattern) || parseStatementsContainBreak(matchCase.Body) {
				return true
			}
		}
	case *parse.ConditionalMatchExpression:
		for _, matchCase := range e.Cases {
			if parseExpressionContainsBreak(matchCase.Condition) || parseStatementsContainBreak(matchCase.Body) {
				return true
			}
		}
	case *parse.Try:
		return parseExpressionContainsBreak(e.Expression) || parseStatementsContainBreak(e.CatchBlock)
	case *parse.BlockExpression:
		return parseStatementsContainBreak(e.Statements)
	case *parse.UnsafeBlock:
		return parseStatementsContainBreak(e.Statements)
	case *parse.AnonymousFunction:
		return false
	case *parse.VariableAssignment:
		return parseExpressionContainsBreak(e.Target) || parseExpressionContainsBreak(e.Value)
	case *parse.VariableDeclaration:
		return parseExpressionContainsBreak(e.Value)
	}
	return false
}

func unsafeCatchOkValueTypes(block *Block) []Type {
	return unsafeCatchOkValueTypesInBlock(block, nil)
}

func unsafeCatchOkValueTypesInBlock(block *Block, aliases map[string][]Type) []Type {
	if block == nil {
		return nil
	}
	finalExprIndex := -1
	for i := len(block.Stmts) - 1; i >= 0; i-- {
		if block.Stmts[i].Expr != nil {
			finalExprIndex = i
			break
		}
	}
	if finalExprIndex == -1 {
		return nil
	}
	aliases = cloneUnsafeCatchTypeAliases(aliases)
	for _, stmt := range block.Stmts[:finalExprIndex] {
		if stmt.Expr != nil {
			continue
		}
		if stmt.Break {
			aliases = map[string][]Type{}
			continue
		}
		switch s := stmt.Stmt.(type) {
		case nil:
			continue
		case *VariableDef:
			aliases[s.Name] = unsafeCatchOkValueTypesFromExpression(s.Value, aliases)
		case *Reassignment:
			if target, ok := s.Target.(*Variable); ok {
				aliases[target.Name()] = unsafeCatchOkValueTypesFromExpression(s.Value, aliases)
			} else {
				aliases = map[string][]Type{}
			}
		default:
			aliases = map[string][]Type{}
		}
	}
	return unsafeCatchOkValueTypesFromExpression(block.Stmts[finalExprIndex].Expr, aliases)
}

func unsafeCatchOkValueTypesFromExpression(expr Expression, aliases map[string][]Type) []Type {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *Variable:
		if types, ok := aliases[e.Name()]; ok {
			return types
		}
	case *ModuleFunctionCall:
		if (e.Module == "Result" || e.Module == "ard/result") && e.Call != nil {
			switch e.Call.Name {
			case "ok":
				if len(e.Call.Args) == 1 {
					return []Type{e.Call.Args[0].Type()}
				}
				return nil
			case "err":
				return nil
			}
		}
	case *Block:
		return unsafeCatchOkValueTypesInBlock(e, aliases)
	case *If:
		var out []Type
		for _, branch := range e.Branches {
			out = append(out, unsafeCatchOkValueTypesInBlock(branch.Body, aliases)...)
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.Else, aliases)...)
		return out
	case *BoolMatch:
		out := unsafeCatchOkValueTypesInBlock(e.True, aliases)
		out = append(out, unsafeCatchOkValueTypesInBlock(e.False, aliases)...)
		return out
	case *IntMatch:
		var out []Type
		for _, block := range e.IntCases {
			out = append(out, unsafeCatchOkValueTypesInBlock(block, aliases)...)
		}
		for _, block := range e.RangeCases {
			out = append(out, unsafeCatchOkValueTypesInBlock(block, aliases)...)
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *StrMatch:
		var out []Type
		for _, block := range e.Cases {
			out = append(out, unsafeCatchOkValueTypesInBlock(block, aliases)...)
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *EnumMatch:
		var out []Type
		for _, block := range e.Cases {
			out = append(out, unsafeCatchOkValueTypesInBlock(block, aliases)...)
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *UnionMatch:
		var out []Type
		for _, match := range e.TypeCases {
			if match != nil {
				out = append(out, unsafeCatchOkValueTypesInBlock(match.Body, aliases)...)
			}
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *ConditionalMatch:
		var out []Type
		for _, matchCase := range e.Cases {
			out = append(out, unsafeCatchOkValueTypesInBlock(matchCase.Body, aliases)...)
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *OptionMatch:
		var out []Type
		if e.Some != nil {
			out = append(out, unsafeCatchOkValueTypesInBlock(e.Some.Body, aliases)...)
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.None, aliases)...)
		return out
	case *ResultMatch:
		var out []Type
		if e.Ok != nil {
			out = append(out, unsafeCatchOkValueTypesInBlock(e.Ok.Body, aliases)...)
		}
		if e.Err != nil {
			out = append(out, unsafeCatchOkValueTypesInBlock(e.Err.Body, aliases)...)
		}
		return out
	}
	if result, ok := expr.Type().(*Result); ok {
		return []Type{result.val}
	}
	return nil
}

func unsafeCatchErrValueTypes(block *Block) []Type {
	return unsafeCatchErrValueTypesInBlock(block, nil)
}

func unsafeCatchErrValueTypesInBlock(block *Block, aliases map[string][]Type) []Type {
	if block == nil {
		return nil
	}
	finalExprIndex := -1
	for i := len(block.Stmts) - 1; i >= 0; i-- {
		if block.Stmts[i].Expr != nil {
			finalExprIndex = i
			break
		}
	}
	if finalExprIndex == -1 {
		return nil
	}
	aliases = cloneUnsafeCatchTypeAliases(aliases)
	for _, stmt := range block.Stmts[:finalExprIndex] {
		if stmt.Expr != nil {
			continue
		}
		if stmt.Break {
			aliases = map[string][]Type{}
			continue
		}
		switch s := stmt.Stmt.(type) {
		case nil:
			continue
		case *VariableDef:
			aliases[s.Name] = unsafeCatchErrValueTypesFromExpression(s.Value, aliases)
		case *Reassignment:
			if target, ok := s.Target.(*Variable); ok {
				aliases[target.Name()] = unsafeCatchErrValueTypesFromExpression(s.Value, aliases)
			} else {
				aliases = map[string][]Type{}
			}
		default:
			aliases = map[string][]Type{}
		}
	}
	return unsafeCatchErrValueTypesFromExpression(block.Stmts[finalExprIndex].Expr, aliases)
}

func unsafeCatchErrValueTypesFromExpression(expr Expression, aliases map[string][]Type) []Type {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *Variable:
		if types, ok := aliases[e.Name()]; ok {
			return types
		}
	case *ModuleFunctionCall:
		if (e.Module == "Result" || e.Module == "ard/result") && e.Call != nil {
			switch e.Call.Name {
			case "ok":
				return nil
			case "err":
				if len(e.Call.Args) == 1 {
					return []Type{e.Call.Args[0].Type()}
				}
				return nil
			}
		}
	case *Block:
		return unsafeCatchErrValueTypesInBlock(e, aliases)
	case *If:
		var out []Type
		for _, branch := range e.Branches {
			out = append(out, unsafeCatchErrValueTypesInBlock(branch.Body, aliases)...)
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.Else, aliases)...)
		return out
	case *BoolMatch:
		out := unsafeCatchErrValueTypesInBlock(e.True, aliases)
		out = append(out, unsafeCatchErrValueTypesInBlock(e.False, aliases)...)
		return out
	case *IntMatch:
		var out []Type
		for _, block := range e.IntCases {
			out = append(out, unsafeCatchErrValueTypesInBlock(block, aliases)...)
		}
		for _, block := range e.RangeCases {
			out = append(out, unsafeCatchErrValueTypesInBlock(block, aliases)...)
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *StrMatch:
		var out []Type
		for _, block := range e.Cases {
			out = append(out, unsafeCatchErrValueTypesInBlock(block, aliases)...)
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *EnumMatch:
		var out []Type
		for _, block := range e.Cases {
			out = append(out, unsafeCatchErrValueTypesInBlock(block, aliases)...)
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *UnionMatch:
		var out []Type
		for _, match := range e.TypeCases {
			if match != nil {
				out = append(out, unsafeCatchErrValueTypesInBlock(match.Body, aliases)...)
			}
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *ConditionalMatch:
		var out []Type
		for _, matchCase := range e.Cases {
			out = append(out, unsafeCatchErrValueTypesInBlock(matchCase.Body, aliases)...)
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *OptionMatch:
		var out []Type
		if e.Some != nil {
			out = append(out, unsafeCatchErrValueTypesInBlock(e.Some.Body, aliases)...)
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.None, aliases)...)
		return out
	case *ResultMatch:
		var out []Type
		if e.Ok != nil {
			out = append(out, unsafeCatchErrValueTypesInBlock(e.Ok.Body, aliases)...)
		}
		if e.Err != nil {
			out = append(out, unsafeCatchErrValueTypesInBlock(e.Err.Body, aliases)...)
		}
		return out
	}
	if result, ok := expr.Type().(*Result); ok {
		return []Type{result.err}
	}
	return nil
}

func cloneUnsafeCatchTypeAliases(aliases map[string][]Type) map[string][]Type {
	if len(aliases) == 0 {
		return map[string][]Type{}
	}
	cloned := make(map[string][]Type, len(aliases))
	for name, types := range aliases {
		cloned[name] = types
	}
	return cloned
}

func unsafeResultOkValueCompatible(expected Type, actual Type) bool {
	expected = deref(expected)
	actual = deref(actual)
	if expectedVar, ok := expected.(*TypeVar); ok {
		if expectedVar.bound && expectedVar.actual != nil {
			return unsafeResultOkValueCompatible(expectedVar.actual, actual)
		}
		actualVar, ok := actual.(*TypeVar)
		if !ok {
			return false
		}
		if actualVar.bound && actualVar.actual != nil {
			return unsafeResultOkValueCompatible(expected, actualVar.actual)
		}
		return expectedVar.name == actualVar.name
	}
	if actualVar, ok := actual.(*TypeVar); ok {
		if actualVar.bound && actualVar.actual != nil {
			return unsafeResultOkValueCompatible(expected, actualVar.actual)
		}
		return false
	}
	if !hasGenericsInType(expected) && !hasGenericsInType(actual) {
		return expected.equal(actual)
	}

	switch expectedType := expected.(type) {
	case *List:
		actualType, ok := actual.(*List)
		return ok && unsafeResultOkValueCompatible(expectedType.of, actualType.of)
	case *Chan:
		actualType, ok := actual.(*Chan)
		return ok && unsafeResultOkValueCompatible(expectedType.of, actualType.of)
	case *Receiver:
		actualType, ok := actual.(*Receiver)
		return ok && unsafeResultOkValueCompatible(expectedType.of, actualType.of)
	case *Sender:
		actualType, ok := actual.(*Sender)
		return ok && unsafeResultOkValueCompatible(expectedType.of, actualType.of)
	case *Map:
		actualType, ok := actual.(*Map)
		return ok && unsafeResultOkValueCompatible(expectedType.key, actualType.key) && unsafeResultOkValueCompatible(expectedType.value, actualType.value)
	case *Maybe:
		actualType, ok := actual.(*Maybe)
		return ok && unsafeResultOkValueCompatible(expectedType.of, actualType.of)
	case *Result:
		actualType, ok := actual.(*Result)
		return ok && unsafeResultOkValueCompatible(expectedType.val, actualType.val) && unsafeResultOkValueCompatible(expectedType.err, actualType.err)
	case *MutableRef:
		actualType, ok := actual.(*MutableRef)
		return ok && unsafeResultOkValueCompatible(expectedType.of, actualType.of)
	case *StructDef:
		actualType, ok := actual.(*StructDef)
		if !ok || expectedType.Name != actualType.Name || expectedType.ModulePath != actualType.ModulePath || len(expectedType.TypeArgs) != len(actualType.TypeArgs) {
			return false
		}
		for i := range expectedType.TypeArgs {
			if !unsafeResultOkValueCompatible(expectedType.TypeArgs[i], actualType.TypeArgs[i]) {
				return false
			}
		}
		return true
	case *FunctionDef:
		actualType, ok := actual.(*FunctionDef)
		if !ok || len(expectedType.Parameters) != len(actualType.Parameters) {
			return false
		}
		for i := range expectedType.Parameters {
			eMut, eType := normalizedParamMutability(expectedType.Parameters[i])
			aMut, aType := normalizedParamMutability(actualType.Parameters[i])
			if eMut != aMut || !unsafeResultOkValueCompatible(eType, aType) {
				return false
			}
		}
		return unsafeResultOkValueCompatible(expectedType.ReturnType, actualType.ReturnType)
	}

	if hasGenericsInType(expected) || hasGenericsInType(actual) {
		return false
	}
	return expected.equal(actual)
}

func (c *Checker) validateUnsafeCatchResults(block *Block, resultType *Result, loc parse.Location) {
	if block == nil || resultType == nil {
		return
	}
	for _, stmt := range block.Stmts {
		c.validateUnsafeCatchResultsInStatement(stmt, resultType, loc)
	}
}

func (c *Checker) validateUnsafeCatchResultsInStatement(stmt Statement, resultType *Result, loc parse.Location) {
	if stmt.Expr != nil {
		c.validateUnsafeCatchResultsInExpression(stmt.Expr, resultType, loc)
	}
	if stmt.Stmt == nil {
		return
	}
	switch s := stmt.Stmt.(type) {
	case *VariableDef:
		c.validateUnsafeCatchResultsInExpression(s.Value, resultType, loc)
	case *Reassignment:
		c.validateUnsafeCatchResultsInExpression(s.Target, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(s.Value, resultType, loc)
	case *WhileLoop:
		c.validateUnsafeCatchResultsInExpression(s.Condition, resultType, loc)
		c.validateUnsafeCatchResults(s.Body, resultType, loc)
	case *ForIntRange:
		c.validateUnsafeCatchResultsInExpression(s.Start, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(s.End, resultType, loc)
		c.validateUnsafeCatchResults(s.Body, resultType, loc)
	case *ForInStr:
		c.validateUnsafeCatchResultsInExpression(s.Value, resultType, loc)
		c.validateUnsafeCatchResults(s.Body, resultType, loc)
	case *ForInList:
		c.validateUnsafeCatchResultsInExpression(s.List, resultType, loc)
		c.validateUnsafeCatchResults(s.Body, resultType, loc)
	case *ForInMap:
		c.validateUnsafeCatchResultsInExpression(s.Map, resultType, loc)
		c.validateUnsafeCatchResults(s.Body, resultType, loc)
	case *ForLoop:
		if s.Init != nil {
			c.validateUnsafeCatchResultsInExpression(s.Init.Value, resultType, loc)
		}
		c.validateUnsafeCatchResultsInExpression(s.Condition, resultType, loc)
		if s.Update != nil {
			c.validateUnsafeCatchResultsInExpression(s.Update.Target, resultType, loc)
			c.validateUnsafeCatchResultsInExpression(s.Update.Value, resultType, loc)
		}
		c.validateUnsafeCatchResults(s.Body, resultType, loc)
	}
}

func (c *Checker) validateUnsafeCatchResultsInExpression(expr Expression, resultType *Result, loc parse.Location) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *TryOp:
		c.validateUnsafeCatchResultsInExpression(e.Expr(), resultType, loc)
		if e.CatchBlock != nil {
			if catchResult, ok := e.CatchBlock.Type().(*Result); ok && catchResult.err.equal(resultType.err) {
				for _, okType := range unsafeCatchOkValueTypes(e.CatchBlock) {
					if !unsafeResultOkValueCompatible(resultType.val, okType) {
						c.addError(typeMismatch(resultType, MakeResult(okType, catchResult.err)), loc)
					}
				}
				for _, errType := range unsafeCatchErrValueTypes(e.CatchBlock) {
					if !unsafeResultOkValueCompatible(resultType.err, errType) {
						c.addError(typeMismatch(resultType, MakeResult(catchResult.val, errType)), loc)
					}
				}
			}
			c.validateUnsafeCatchResults(e.CatchBlock, resultType, loc)
		}
	case *Block:
		c.validateUnsafeCatchResults(e, resultType, loc)
	case *UnsafeBlock:
		if nestedResult, ok := e.Type().(*Result); ok {
			c.validateUnsafeCatchResults(e.Body, nestedResult, loc)
		}
	case *If:
		for _, branch := range e.Branches {
			c.validateUnsafeCatchResultsInExpression(branch.Condition, resultType, loc)
			c.validateUnsafeCatchResults(branch.Body, resultType, loc)
		}
		c.validateUnsafeCatchResults(e.Else, resultType, loc)
	case *BoolMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		c.validateUnsafeCatchResults(e.True, resultType, loc)
		c.validateUnsafeCatchResults(e.False, resultType, loc)
	case *IntMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, block := range e.IntCases {
			c.validateUnsafeCatchResults(block, resultType, loc)
		}
		for _, block := range e.RangeCases {
			c.validateUnsafeCatchResults(block, resultType, loc)
		}
		c.validateUnsafeCatchResults(e.CatchAll, resultType, loc)
	case *StrMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, block := range e.Cases {
			c.validateUnsafeCatchResults(block, resultType, loc)
		}
		c.validateUnsafeCatchResults(e.CatchAll, resultType, loc)
	case *EnumMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, block := range e.Cases {
			c.validateUnsafeCatchResults(block, resultType, loc)
		}
		c.validateUnsafeCatchResults(e.CatchAll, resultType, loc)
	case *UnionMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, match := range e.TypeCases {
			if match != nil {
				c.validateUnsafeCatchResults(match.Body, resultType, loc)
			}
		}
		c.validateUnsafeCatchResults(e.CatchAll, resultType, loc)
	case *ConditionalMatch:
		for _, matchCase := range e.Cases {
			c.validateUnsafeCatchResultsInExpression(matchCase.Condition, resultType, loc)
			c.validateUnsafeCatchResults(matchCase.Body, resultType, loc)
		}
		c.validateUnsafeCatchResults(e.CatchAll, resultType, loc)
	case *OptionMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		if e.Some != nil {
			c.validateUnsafeCatchResults(e.Some.Body, resultType, loc)
		}
		c.validateUnsafeCatchResults(e.None, resultType, loc)
	case *ResultMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		if e.Ok != nil {
			c.validateUnsafeCatchResults(e.Ok.Body, resultType, loc)
		}
		if e.Err != nil {
			c.validateUnsafeCatchResults(e.Err.Body, resultType, loc)
		}
	case *Negation:
		c.validateUnsafeCatchResultsInExpression(e.Value, resultType, loc)
	case *Not:
		c.validateUnsafeCatchResultsInExpression(e.Value, resultType, loc)
	case *IntAddition:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntSubtraction:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntMultiplication:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntDivision:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntModulo:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntGreater:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntGreaterEqual:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntLess:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntLessEqual:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatAddition:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatSubtraction:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatMultiplication:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatDivision:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatGreater:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatGreaterEqual:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatLess:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatLessEqual:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *StrAddition:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *Equality:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *Inequality:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *And:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *Or:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *StrMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *ByteMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
	case *RuneMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
	case *IntMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
	case *FloatMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
	case *BoolMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
	case *ListMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *MapMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *MaybeMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *ResultMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *ListLiteral:
		for _, item := range e.Elements {
			c.validateUnsafeCatchResultsInExpression(item, resultType, loc)
		}
	case *MapLiteral:
		for _, key := range e.Keys {
			c.validateUnsafeCatchResultsInExpression(key, resultType, loc)
		}
		for _, value := range e.Values {
			c.validateUnsafeCatchResultsInExpression(value, resultType, loc)
		}
	case *StructInstance:
		for _, field := range e.Fields {
			c.validateUnsafeCatchResultsInExpression(field, resultType, loc)
		}
	case *FunctionCall:
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *FunctionValueCall:
		c.validateUnsafeCatchResultsInExpression(e.Callee, resultType, loc)
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *InstanceProperty:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
	case *InstanceMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		if e.Method != nil {
			for _, arg := range e.Method.Args {
				c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
			}
		}
	case *ModuleFunctionCall:
		if e.Call != nil {
			for _, arg := range e.Call.Args {
				c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
			}
		}
	case *ModuleStructInstance:
		if e.Property != nil {
			for _, field := range e.Property.Fields {
				c.validateUnsafeCatchResultsInExpression(field, resultType, loc)
			}
		}
	case *TemplateStr:
		for _, chunk := range e.Chunks {
			c.validateUnsafeCatchResultsInExpression(chunk, resultType, loc)
		}
	case *Panic:
		c.validateUnsafeCatchResultsInExpression(e.Message, resultType, loc)
	}
}

func (c *Checker) checkMatchArmBlock(stmts []parse.Statement, setup func()) *Block {
	expectedType := c.expectedExpr
	discardFinal := c.matchArmDiscardContext || expectedType == Void
	c.expectedExpr = nil
	var block *Block
	if expectedType == nil || expectedType == Void {
		block = c.checkBlockWithInferredFinalValue(stmts, setup, discardFinal)
	} else {
		block = c.checkBlockWithExpected(stmts, setup, expectedType, false)
	}
	c.expectedExpr = expectedType
	return block
}

func (c *Checker) checkBlockWithInferredFinalValue(stmts []parse.Statement, setup func(), finalDiscardContext bool) *Block {
	if len(stmts) == 0 {
		return &Block{Stmts: []Statement{}}
	}

	parent := c.scope
	newScope := makeScope(parent)
	c.scope = &newScope
	defer func() {
		c.scope = parent
	}()

	if setup != nil {
		setup()
	}

	lastExprIndex := -1
	for i := len(stmts) - 1; i >= 0; i-- {
		if _, ok := stmts[i].(*parse.Comment); ok {
			continue
		}
		if c.canCheckStatementAsExpectedExpression(stmts[i], Void, false) {
			lastExprIndex = i
		}
		break
	}

	block := &Block{Stmts: make([]Statement, len(stmts))}
	for i := range stmts {
		if i == lastExprIndex {
			var expr Expression
			if finalDiscardContext {
				expr = c.withDiscardExprContext(func() Expression {
					return c.checkExpr(stmts[i].(parse.Expression))
				})
			} else {
				expr = c.checkValueExpr(stmts[i].(parse.Expression))
			}
			if expr != nil {
				block.Stmts[i] = Statement{Expr: expr}
			}
		} else if stmt := c.checkStmt(&stmts[i]); stmt != nil {
			block.Stmts[i] = *stmt
		}
		if c.halted {
			break
		}
	}
	return block
}

func (c *Checker) withExpectedExpr(expected Type, check func() Expression) Expression {
	previousExpected := c.expectedExpr
	previousDiscard := c.discardExprContext
	c.expectedExpr = expected
	c.discardExprContext = expected == Void
	defer func() {
		c.expectedExpr = previousExpected
		c.discardExprContext = previousDiscard
	}()
	return check()
}

func (c *Checker) withDiscardExprContext(check func() Expression) Expression {
	previous := c.discardExprContext
	c.discardExprContext = true
	defer func() {
		c.discardExprContext = previous
	}()
	return check()
}

func (c *Checker) withValueExprContext(check func()) {
	previous := c.discardExprContext
	c.discardExprContext = false
	defer func() {
		c.discardExprContext = previous
	}()
	check()
}

func (c *Checker) checkValueExpr(expr parse.Expression) Expression {
	var checked Expression
	c.withValueExprContext(func() {
		checked = c.checkExpr(expr)
	})
	return checked
}

func (c *Checker) checkMap(declaredType Type, expr *parse.MapLiteral) *MapLiteral {
	// Handle empty map with declared type
	if len(expr.Entries) == 0 {
		if declaredType != nil {
			mapType, ok := declaredType.(*Map)
			if !ok {
				c.addError(fmt.Sprintf("Expected map type but got %s", declaredType), expr.GetLocation())
				return nil
			}

			// Return empty map with the declared type
			return &MapLiteral{
				Keys:      []Expression{},
				Values:    []Expression{},
				_type:     mapType,
				KeyType:   mapType.Key(),
				ValueType: mapType.Value(),
			}
		} else {
			// Empty map without a declared type is an error
			c.addError("Empty maps need an explicit type", expr.GetLocation())
			c.halted = true
			mapType := MakeMap(Void, Void)
			return &MapLiteral{
				_type:     mapType,
				Keys:      []Expression{},
				Values:    []Expression{},
				KeyType:   Void,
				ValueType: Void,
			}
		}
	}

	// Handle non-empty map
	if declaredType != nil {
		mapType, ok := declaredType.(*Map)
		if !ok {
			c.addError(fmt.Sprintf("Expected map type but got %s", declaredType), expr.GetLocation())
			return nil
		}

		expectedKeyType := mapType.key
		expectedValueType := mapType.value

		keys := make([]Expression, len(expr.Entries))
		values := make([]Expression, len(expr.Entries))

		hasError := false
		for i, entry := range expr.Entries {
			// Type check the key
			key := c.checkExpr(entry.Key)
			if key == nil {
				hasError = true
				continue
			}
			if !c.areCompatible(expectedKeyType, key.Type()) {
				c.addError(typeMismatch(expectedKeyType, key.Type()), entry.Key.GetLocation())
				hasError = true
				continue
			}
			keys[i] = key

			// Type check the value
			value := c.checkExpr(entry.Value)
			if value == nil {
				hasError = true
				continue
			}
			if !c.areCompatible(expectedValueType, value.Type()) {
				c.addError(typeMismatch(expectedValueType, value.Type()), entry.Value.GetLocation())
				hasError = true
				continue
			}
			values[i] = value
		}

		if hasError {
			return nil
		}

		return &MapLiteral{
			Keys:      keys,
			Values:    values,
			_type:     mapType,
			KeyType:   mapType.Key(),
			ValueType: mapType.Value(),
		}
	}

	// Type inference for non-empty maps without declared type
	keys := make([]Expression, len(expr.Entries))
	values := make([]Expression, len(expr.Entries))

	// Check the first entry to determine key and value types
	firstKey := c.checkExpr(expr.Entries[0].Key)
	firstValue := c.checkExpr(expr.Entries[0].Value)

	if firstKey == nil || firstValue == nil {
		return nil
	}

	keyType := firstKey.Type()
	valueType := firstValue.Type()
	keys[0] = firstKey
	values[0] = firstValue

	// Check that all entries have consistent types
	for i := 1; i < len(expr.Entries); i++ {
		key := c.checkExpr(expr.Entries[i].Key)
		if key == nil {
			keyType = Void
			continue
		}
		if !keyType.equal(key.Type()) {
			c.addError(fmt.Sprintf("Map key type mismatch: Expected %s, got %s", keyType, key.Type()), expr.Entries[i].Key.GetLocation())
			continue
		}
		keys[i] = key

		value := c.checkExpr(expr.Entries[i].Value)
		if value == nil {
			valueType = Void
			continue
		}
		if !valueType.equal(value.Type()) {
			c.addError(fmt.Sprintf("Map value type mismatch: Expected %s, got %s", valueType, value.Type()), expr.Entries[i].Value.GetLocation())
			continue
		}
		values[i] = value
	}

	// Create and return the map
	c.validateMapKeyType(keyType, expr.Entries[0].Key.GetLocation())
	mapType := MakeMap(keyType, valueType)
	return &MapLiteral{
		Keys:      keys,
		Values:    values,
		_type:     mapType,
		KeyType:   keyType,
		ValueType: valueType,
	}
}

// validateStructInstance validates struct instantiation and returns the instance or nil if errors
func (c *Checker) validateStructInstance(structType *StructDef, properties []parse.StructValue, structName string, loc parse.Location) *StructInstance {
	instance := &StructInstance{Name: structName, _type: structType}
	fields := make(map[string]Expression)
	fieldTypes := make(map[string]Type)
	providedFields := make(map[string]bool)

	// For generic structs, infer generics from provided field values
	var structDefCopy *StructDef
	var genericScope *SymbolTable
	if structType.hasGenerics() {
		genericParams := append([]string(nil), structType.GenericParams...)
		if len(genericParams) == 0 {
			genericNames := make(map[string]bool)
			for _, fieldType := range structType.Fields {
				extractGenericNames(fieldType, genericNames)
			}
			for name := range genericNames {
				genericParams = append(genericParams, name)
			}
		}

		// Create generic scope and fresh struct copy
		genericScope = c.scope.createGenericScope(genericParams)
		structDefCopy = copyStructWithTypeVarMap(structType, *genericScope.genericContext)
	} else {
		structDefCopy = structType
	}

	// Check all provided properties
	for _, property := range properties {
		fieldName := property.Name.Name
		var field Type
		var ok bool

		// Get field from the copy (which has fresh TypeVar instances for generics)
		if genericScope != nil {
			field, ok = structDefCopy.Fields[fieldName]
		} else {
			field, ok = structType.Fields[fieldName]
		}

		if !ok {
			c.addError(fmt.Sprintf("Unknown field: %s", fieldName), property.GetLocation())
		} else {
			providedFields[fieldName] = true

			// A `mut T` (MutableRef) field accepts an existing `mut T` reference value
			// directly (a stored handle), in addition to borrowing a mutable base-type
			// lvalue below (ADR 0031). Try the reference value first.
			if _, ok := mutableRefBase(field); ok {
				diagCount := len(c.diagnostics)
				if checked := c.checkExprAs(property.Value, field); checked != nil && checked.Type().equal(field) {
					fields[fieldName] = checked
					fieldTypes[fieldName] = field
					continue
				}
				c.diagnostics = c.diagnostics[:diagCount]
			}

			fieldExpected := field
			fieldIsMutableRef := false
			if base, ok := mutableRefBase(field); ok {
				fieldExpected = base
				fieldIsMutableRef = true
			}

			// For generic structs, unify types to resolve generics
			if genericScope != nil {
				// Check expression without type context first (let it infer if possible)
				checkVal := c.checkExpr(property.Value)
				if checkVal == nil {
					continue
				}

				// Implicit Maybe wrapping: if field is Maybe<T> and value is T, wrap in maybe::some()
				if maybeField, isMaybe := fieldExpected.(*Maybe); isMaybe {
					if valType := checkVal.Type(); !valType.equal(fieldExpected) {
						if c.areCompatible(maybeField.Of(), valType) {
							checkVal = c.synthesizeMaybeSome(checkVal, fieldExpected)
						}
					}
				}

				if fieldIsMutableRef && !c.isMutable(checkVal) {
					c.addError(fmt.Sprintf("Type mismatch: Expected a mutable %s", fieldExpected.String()), property.GetLocation())
					continue
				}

				if err := c.unifyTypes(fieldExpected, checkVal.Type(), genericScope); err != nil {
					c.addError(err.Error(), property.GetLocation())
					continue
				}
				// After unification, dereference to get the actual type
				fieldExpected = derefType(fieldExpected)
				if fieldIsMutableRef {
					field = MakeMutableRef(fieldExpected)
				} else {
					field = fieldExpected
				}

				fields[fieldName] = checkVal
				fieldTypes[fieldName] = field
			} else {
				// For non-generic structs, handle nullable fields with implicit wrapping
				var val Expression
				if maybeField, isMaybe := fieldExpected.(*Maybe); isMaybe {
					// Preserve full Maybe<T> type context for expressions like maybe::some(...)
					// and maybe::none(), but use the inner type for literals and anonymous
					// functions so they can still infer their element/parameter types.
					switch property.Value.(type) {
					case *parse.ListLiteral, *parse.MapLiteral, *parse.AnonymousFunction:
						val = c.checkExprAs(property.Value, maybeField.Of())
						if val != nil {
							val = c.synthesizeMaybeSome(val, field)
						}
					default:
						diagnosticCount := len(c.diagnostics)
						val = c.checkExprAs(property.Value, fieldExpected)
						if val == nil {
							c.diagnostics = c.diagnostics[:diagnosticCount]
							val = c.checkExpr(property.Value)
							if val != nil && !val.Type().equal(fieldExpected) {
								if c.areCompatible(maybeField.Of(), val.Type()) {
									val = c.synthesizeMaybeSome(val, fieldExpected)
								} else {
									c.addError(typeMismatch(fieldExpected, val.Type()), property.GetLocation())
									val = nil
								}
							}
						}
					}
				} else {
					// Non-nullable fields use checkExprAs which provides type context
					diagCount := len(c.diagnostics)
					val = c.checkExprAs(property.Value, fieldExpected)
					if val == nil {
						// A mutable-reference lvalue (e.g. a `mut T` field read that
						// deref's to its value type) auto-borrows back into `mut T` so a
						// stored Go pointer handle can satisfy an interface/pointer field
						// (ADR 0031).
						if borrowed := c.checkExpr(property.Value); borrowed != nil {
							if refType := referenceArgType(borrowed); !refType.equal(borrowed.Type()) && c.areCompatible(fieldExpected, refType) {
								c.diagnostics = c.diagnostics[:diagCount]
								val = borrowed
							}
						}
					}
				}
				if val != nil {
					if fieldIsMutableRef && !c.isMutable(val) {
						c.addError(fmt.Sprintf("Type mismatch: Expected a mutable %s", fieldExpected.String()), property.GetLocation())
						continue
					}
					fields[fieldName] = val
					fieldTypes[fieldName] = field
				}
			}
		}
	}

	// Check for missing required fields
	missing := []string{}
	checkFieldsMap := structDefCopy.Fields
	if structDefCopy == structType {
		checkFieldsMap = structType.Fields
	}

	for name, t := range checkFieldsMap {
		if _, exists := fields[name]; !exists {
			if _, isMaybe := t.(*Maybe); !isMaybe {
				if !providedFields[name] {
					missing = append(missing, name)
				}
			} else {
				// For optional fields, include their type
				fieldTypes[name] = t
			}
		} else if _, exists := fieldTypes[name]; !exists {
			// Pre-compute all field types, not just provided ones
			fieldTypes[name] = t
		}
	}
	if len(missing) > 0 {
		c.addError(fmt.Sprintf("Missing field: %s", strings.Join(missing, ", ")), loc)
	}

	instance.Fields = fields
	instance.FieldTypes = fieldTypes
	// Store the refined struct definition (with resolved generics) as the instance's type
	typeArgs := make([]Type, len(structDefCopy.TypeArgs))
	for i, typeArg := range structDefCopy.TypeArgs {
		typeArgs[i] = derefType(typeArg)
	}
	genericParams := append([]string(nil), structDefCopy.GenericParams...)
	if len(genericParams) == 0 {
		genericParams = nil
	}
	instance._type = &StructDef{
		Name:          structDefCopy.Name,
		ModulePath:    structDefCopy.ModulePath,
		Fields:        fieldTypes,
		Self:          structDefCopy.Self,
		Traits:        structDefCopy.Traits,
		GenericParams: genericParams,
		TypeArgs:      typeArgs,
		Private:       structDefCopy.Private,
	}
	instance.StructType = instance._type
	return instance
}

// createPrimitiveMethodNode creates type-specific method nodes for primitives and collections
// Falls back to generic InstanceMethod for user-defined types (structs, enums)
func (c *Checker) createPrimitiveMethodNode(subject Expression, methodName string, args []Expression, fnDef *FunctionDef, typeArgs []Type) Expression {
	// Determine subject type - emit specialized nodes for all built-in types
	switch subject.Type() {
	case Str:
		return c.createStrMethod(subject, methodName, args)
	case Int:
		return c.createIntMethod(subject, methodName)
	case Byte:
		return c.createByteMethod(subject, methodName)
	case Rune:
		return c.createRuneMethod(subject, methodName)
	case Float64:
		return c.createFloatMethod(subject, methodName)
	case Bool:
		return c.createBoolMethod(subject, methodName)
	}

	// Check for collection types
	if _, isList := subject.Type().(*List); isList {
		return c.createListMethod(subject, methodName, args, fnDef)
	}
	if _, isMap := subject.Type().(*Map); isMap {
		return c.createMapMethod(subject, methodName, args, fnDef)
	}
	if _, isMaybe := subject.Type().(*Maybe); isMaybe {
		return c.createMaybeMethod(subject, methodName, args, fnDef)
	}
	if _, isResult := subject.Type().(*Result); isResult {
		return c.createResultMethod(subject, methodName, args, fnDef)
	}

	// For user-defined types (structs, enums), use generic InstanceMethod
	receiverKind := ReceiverUnknown
	var structType *StructDef
	var enumType *Enum
	var traitType *Trait
	switch receiver := subject.Type().(type) {
	case *StructDef:
		receiverKind = ReceiverStruct
		structType = receiver
	case *Enum:
		receiverKind = ReceiverEnum
		enumType = receiver
	case *Trait:
		receiverKind = ReceiverTrait
		traitType = receiver
	}
	return &InstanceMethod{
		Subject: subject,
		Method: &FunctionCall{
			Name:       methodName,
			Args:       args,
			TypeArgs:   typeArgs,
			fn:         fnDef,
			ReturnType: fnDef.ReturnType,
		},
		ReceiverKind: receiverKind,
		StructType:   structType,
		EnumType:     enumType,
		TraitType:    traitType,
	}
}

func (c *Checker) createStrMethod(subject Expression, methodName string, args []Expression) Expression {
	var kind StrMethodKind
	switch methodName {
	case "at":
		kind = StrAt
	case "size":
		kind = StrSize
	case "bytes":
		kind = StrBytes
	case "runes":
		kind = StrRunes
	case "is_empty":
		kind = StrIsEmpty
	case "contains":
		kind = StrContains
	case "replace":
		kind = StrReplace
	case "replace_all":
		kind = StrReplaceAll
	case "starts_with":
		kind = StrStartsWith
	case "ends_with":
		kind = StrEndsWith
	case "to_str":
		kind = StrToStr
	case "trim":
		kind = StrTrim
	default:
		// Fallback for unknown methods
		panic(fmt.Sprintf("Unknown Str method: %s", methodName))
	}
	return &StrMethod{
		Subject: subject,
		Kind:    kind,
		Args:    args,
	}
}

func (c *Checker) createByteMethod(subject Expression, methodName string) Expression {
	var kind ByteMethodKind
	switch methodName {
	case "to_int":
		kind = ByteToInt
	case "to_str":
		kind = ByteToStr
	default:
		panic(fmt.Sprintf("Unknown Byte method: %s", methodName))
	}
	return &ByteMethod{Subject: subject, Kind: kind}
}

func (c *Checker) createRuneMethod(subject Expression, methodName string) Expression {
	var kind RuneMethodKind
	switch methodName {
	case "to_int":
		kind = RuneToInt
	case "to_str":
		kind = RuneToStr
	default:
		panic(fmt.Sprintf("Unknown Rune method: %s", methodName))
	}
	return &RuneMethod{Subject: subject, Kind: kind}
}

func (c *Checker) createIntMethod(subject Expression, methodName string) Expression {
	var kind IntMethodKind
	switch methodName {
	case "to_str":
		kind = IntToStr
	default:
		panic(fmt.Sprintf("Unknown Int method: %s", methodName))
	}
	return &IntMethod{
		Subject: subject,
		Kind:    kind,
	}
}

func (c *Checker) createFloatMethod(subject Expression, methodName string) Expression {
	var kind FloatMethodKind
	switch methodName {
	case "to_str":
		kind = FloatToStr
	case "to_int":
		kind = FloatToInt
	default:
		panic(fmt.Sprintf("Unknown Float64 method: %s", methodName))
	}
	return &FloatMethod{
		Subject: subject,
		Kind:    kind,
	}
}

func (c *Checker) createBoolMethod(subject Expression, methodName string) Expression {
	var kind BoolMethodKind
	switch methodName {
	case "to_str":
		kind = BoolToStr
	default:
		panic(fmt.Sprintf("Unknown Bool method: %s", methodName))
	}
	return &BoolMethod{
		Subject: subject,
		Kind:    kind,
	}
}

func (c *Checker) createListMethod(subject Expression, methodName string, args []Expression, fnDef *FunctionDef) Expression {
	listType := subject.Type().(*List)
	var kind ListMethodKind
	switch methodName {
	case "at":
		kind = ListAt
	case "prepend":
		kind = ListPrepend
	case "push":
		kind = ListPush
	case "set":
		kind = ListSet
	case "size":
		kind = ListSize
	case "sort":
		kind = ListSort
	case "swap":
		kind = ListSwap
	default:
		panic(fmt.Sprintf("Unknown List method: %s", methodName))
	}
	return &ListMethod{
		Subject:     subject,
		Kind:        kind,
		Args:        args,
		ElementType: listType.of,
		fn:          fnDef,
	}
}

func (c *Checker) createMapMethod(subject Expression, methodName string, args []Expression, fnDef *FunctionDef) Expression {
	mapType := subject.Type().(*Map)
	var kind MapMethodKind
	switch methodName {
	case "keys":
		kind = MapKeys
	case "size":
		kind = MapSize
	case "get":
		kind = MapGet
	case "set":
		kind = MapSet
	case "drop":
		kind = MapDrop
	case "has":
		kind = MapHas
	default:
		panic(fmt.Sprintf("Unknown Map method: %s", methodName))
	}
	return &MapMethod{
		Subject:   subject,
		Kind:      kind,
		Args:      args,
		KeyType:   mapType.Key(),
		ValueType: mapType.Value(),
		fn:        fnDef,
	}
}

func (c *Checker) createMaybeMethod(subject Expression, methodName string, args []Expression, fnDef *FunctionDef) Expression {
	maybeType := subject.Type().(*Maybe)
	var kind MaybeMethodKind
	switch methodName {
	case "expect":
		kind = MaybeExpect
	case "is_none":
		kind = MaybeIsNone
	case "is_some":
		kind = MaybeIsSome
	case "or":
		kind = MaybeOr
	case "map":
		kind = MaybeMap
	case "and_then":
		kind = MaybeAndThen
	default:
		panic(fmt.Sprintf("Unknown Maybe method: %s", methodName))
	}
	return &MaybeMethod{
		Subject:    subject,
		Kind:       kind,
		Args:       args,
		InnerType:  maybeType.Of(),
		fn:         fnDef,
		ReturnType: fnDef.ReturnType,
	}
}

func (c *Checker) createResultMethod(subject Expression, methodName string, args []Expression, fnDef *FunctionDef) Expression {
	resultType := subject.Type().(*Result)
	var kind ResultMethodKind
	switch methodName {
	case "expect":
		kind = ResultExpect
	case "or":
		kind = ResultOr
	case "is_ok":
		kind = ResultIsOk
	case "is_err":
		kind = ResultIsErr
	case "map":
		kind = ResultMap
	case "map_err":
		kind = ResultMapErr
	case "and_then":
		kind = ResultAndThen
	default:
		panic(fmt.Sprintf("Unknown Result method: %s", methodName))
	}
	return &ResultMethod{
		Subject:    subject,
		Kind:       kind,
		Args:       args,
		OkType:     resultType.Val(),
		ErrType:    resultType.Err(),
		fn:         fnDef,
		ReturnType: fnDef.ReturnType,
	}
}

// extractGenericBindingsFromSpecializedStruct builds a mapping from generic parameter names to their resolved types
// by comparing the original struct definition with a specialized version of it.
// For example, if the original struct is `Box { item: $T }` and the specialized has `item: Int`,
// this returns `{$T: Int}`.
func (c *Checker) extractGenericBindingsFromSpecializedStruct(originalDef, specializedDef *StructDef) map[string]Type {
	bindings := make(map[string]Type)

	if originalDef == nil || specializedDef == nil {
		return bindings
	}

	for i, param := range originalDef.GenericParams {
		if i < len(specializedDef.TypeArgs) {
			bindings[param] = derefType(specializedDef.TypeArgs[i])
		}
	}

	// Now compare original field types with specialized field types
	// For each field that contains a generic in the original, extract its resolved type
	for fieldName, originalFieldType := range originalDef.Fields {
		specializedFieldType, ok := specializedDef.Fields[fieldName]
		if !ok {
			continue
		}

		bindGenericTypes(originalFieldType, specializedFieldType, bindings)
	}

	return bindings
}

func bindGenericTypes(original Type, specialized Type, bindings map[string]Type) {
	original = deref(original)
	specialized = deref(specialized)
	switch orig := original.(type) {
	case *TypeVar:
		if orig.name != "" {
			if _, alreadyBound := bindings[orig.name]; !alreadyBound {
				bindings[orig.name] = specialized
			}
		}
	case *List:
		if spec, ok := specialized.(*List); ok {
			bindGenericTypes(orig.of, spec.of, bindings)
		}
	case *Chan:
		if spec, ok := specialized.(*Chan); ok {
			bindGenericTypes(orig.of, spec.of, bindings)
		}
	case *Receiver:
		if spec, ok := specialized.(*Receiver); ok {
			bindGenericTypes(orig.of, spec.of, bindings)
		}
	case *Sender:
		if spec, ok := specialized.(*Sender); ok {
			bindGenericTypes(orig.of, spec.of, bindings)
		}
	case *Map:
		if spec, ok := specialized.(*Map); ok {
			bindGenericTypes(orig.key, spec.key, bindings)
			bindGenericTypes(orig.value, spec.value, bindings)
		}
	case *Maybe:
		if spec, ok := specialized.(*Maybe); ok {
			bindGenericTypes(orig.of, spec.of, bindings)
		}
	case *Result:
		if spec, ok := specialized.(*Result); ok {
			bindGenericTypes(orig.val, spec.val, bindings)
			bindGenericTypes(orig.err, spec.err, bindings)
		}
	case *StructDef:
		if spec, ok := specialized.(*StructDef); ok {
			for i, originalArg := range orig.TypeArgs {
				if i < len(spec.TypeArgs) {
					bindGenericTypes(originalArg, spec.TypeArgs[i], bindings)
				}
			}
			for name, originalField := range orig.Fields {
				if specializedField, ok := spec.Fields[name]; ok {
					bindGenericTypes(originalField, specializedField, bindings)
				}
			}
		}
	case *FunctionDef:
		if spec, ok := specialized.(*FunctionDef); ok {
			for i := range orig.Parameters {
				if i >= len(spec.Parameters) {
					break
				}
				bindGenericTypes(orig.Parameters[i].Type, spec.Parameters[i].Type, bindings)
			}
			bindGenericTypes(orig.ReturnType, spec.ReturnType, bindings)
		}
	}
}

func (c *Checker) checkIfChain(s *parse.IfStatement) Expression {
	if s == nil || s.Condition == nil {
		return nil
	}
	branches := []IfBranch{}
	var elseBlock *Block
	var referenceType Type
	current := s
	for current != nil {
		if current.Condition == nil {
			expectedType := c.expectedExpr
			c.expectedExpr = nil
			block := c.checkBlockWithExpected(current.Body, nil, expectedType, false)
			c.expectedExpr = expectedType
			if referenceType != nil && !block.Type().equal(referenceType) {
				if referenceType == Void || block.Type() == Void {
					if referenceType != Void {
						for i := range branches {
							if branches[i].Body != nil && branches[i].Body.Type() != Void {
								branches[i].Body.DiscardFinalValue = true
							}
						}
					}
					if block.Type() != Void {
						block.DiscardFinalValue = true
					}
					referenceType = Void
				} else {
					c.addError("All branches must have the same result type", current.GetLocation())
					return nil
				}
			}
			elseBlock = block
			break
		}
		condition := c.checkExpr(current.Condition)
		if condition == nil {
			return nil
		}
		if condition.Type() != Bool {
			c.addError("If conditions must be boolean expressions", current.GetLocation())
			return nil
		}
		expectedType := c.expectedExpr
		c.expectedExpr = nil
		body := c.checkBlockWithExpected(current.Body, nil, expectedType, false)
		c.expectedExpr = expectedType
		if referenceType == nil {
			referenceType = body.Type()
		} else if !body.Type().equal(referenceType) {
			if referenceType == Void || body.Type() == Void {
				if referenceType != Void {
					for i := range branches {
						if branches[i].Body != nil && branches[i].Body.Type() != Void {
							branches[i].Body.DiscardFinalValue = true
						}
					}
				}
				if body.Type() != Void {
					body.DiscardFinalValue = true
				}
				referenceType = Void
			} else {
				c.addError("All branches must have the same result type", current.GetLocation())
				return nil
			}
		}
		branches = append(branches, IfBranch{Condition: condition, Body: body})
		next, ok := current.Else.(*parse.IfStatement)
		if !ok {
			break
		}
		current = next
	}
	return &If{Branches: branches, Else: elseBlock}
}

func functionDefForCallableType(typ Type) (*FunctionDef, bool) {
	typ = derefType(typ)
	switch fn := typ.(type) {
	case *FunctionDef:
		return fn, true
	default:
		return nil, false
	}
}

func (c *Checker) checkFunctionValueCall(callee Expression, callArgs []parse.Argument, typeArgs []parse.DeclaredType, location parse.Location, displayName string) Expression {
	fnDef, ok := functionDefForCallableType(callee.Type())
	if !ok {
		c.addError(fmt.Sprintf("%s is not a function", displayName), location)
		return nil
	}

	callTypeArgs := c.resolveCallTypeArgs(typeArgs)
	resolvedExprs, err := c.resolveArguments(callArgs, fnDef.Parameters)
	if err != nil {
		c.addError(err.Error(), location)
		return nil
	}

	numOmittedArgs := 0
	if len(resolvedExprs) < len(fnDef.Parameters) {
		for i := len(resolvedExprs); i < len(fnDef.Parameters); i++ {
			paramType := fnDef.Parameters[i].Type
			if _, isMaybe := paramType.(*Maybe); !isMaybe {
				c.addError(fmt.Sprintf("missing argument for parameter: %s", fnDef.Parameters[i].Name), location)
				return nil
			}
		}
		numOmittedArgs = len(fnDef.Parameters) - len(resolvedExprs)
	} else if len(resolvedExprs) > len(fnDef.Parameters) {
		c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d", len(fnDef.Parameters), len(resolvedExprs)), location)
		resolvedExprs = resolvedExprs[:len(fnDef.Parameters)]
	}

	fnDefCopy, genericScope := c.setupFunctionGenerics(fnDef)
	args, fnToUse := c.checkAndProcessArguments(fnDef, resolvedExprs, fnDefCopy, genericScope, numOmittedArgs)
	if args == nil {
		return nil
	}
	if len(callTypeArgs) > 0 {
		specialized, err := c.resolveGenericFunction(fnDef, args, callTypeArgs, location)
		if err != nil {
			c.addError(err.Error(), location)
			return nil
		}
		fnToUse = specialized
	}

	return &FunctionValueCall{
		Callee:       callee,
		Args:         args,
		FunctionType: fnToUse,
		ReturnType:   fnToUse.ReturnType,
	}
}

func (c *Checker) checkFunctionFieldCall(subject Expression, method parse.FunctionCall, location parse.Location) (Expression, bool) {
	if subject == nil || subject.Type() == nil {
		return nil, false
	}
	fieldType := subject.Type().get(method.Name)
	if fieldType == nil {
		return nil, false
	}
	field := &InstanceProperty{
		Subject:  subject,
		Property: method.Name,
		_type:    fieldType,
	}
	if _, ok := subject.Type().(*StructDef); ok {
		field.Kind = StructSubject
	}
	return c.checkFunctionValueCall(field, method.Args, method.TypeArgs, location, fmt.Sprintf("%s.%s", subject, method.Name)), true
}

func (c *Checker) checkExpr(expr parse.Expression) Expression {
	if c.halted {
		return nil
	}
	discardThisExpr := c.discardExprContext
	previousDiscard := c.discardExprContext
	c.discardExprContext = false
	defer func() {
		c.discardExprContext = previousDiscard
	}()

	switch s := (expr).(type) {
	case *parse.StrLiteral:
		return &StrLiteral{s.Value}
	case *parse.RuneLiteral:
		runes := []rune(s.Value)
		if len(runes) != 1 || !utf8.ValidRune(runes[0]) {
			c.addError("Rune literal must contain exactly one Unicode scalar value", s.GetLocation())
			return &RuneLiteral{Value: 0}
		}
		return &RuneLiteral{Value: runes[0]}
	case *parse.BoolLiteral:
		return &BoolLiteral{s.Value}
	case *parse.VoidLiteral:
		return &VoidLiteral{}
	case *parse.NumLiteral:
		{
			stripped := strings.ReplaceAll(s.Value, "_", "")
			if strings.Contains(stripped, ".") {
				value, err := strconv.ParseFloat(stripped, 64)
				if err != nil {
					c.addError(fmt.Sprintf("Invalid float: %s", s.Value), s.GetLocation())
					return &FloatLiteral{Value: 0.0}
				}
				return &FloatLiteral{Value: value}
			}
			value64, err := strconv.ParseInt(stripped, 0, 64)
			if err != nil {
				c.addError(fmt.Sprintf("Invalid int: %s", s.Value), s.GetLocation())
				return &IntLiteral{0}
			}
			if !c.intLiteralFitsType(value64, Int) {
				c.addError(fmt.Sprintf("Integer literal %s overflows Int", s.Value), s.GetLocation())
			}
			return &IntLiteral{int(value64)}
		}
	case *parse.InterpolatedStr:
		{
			chunks := make([]Expression, len(s.Chunks))
			for i := range s.Chunks {
				cx := c.checkExpr(s.Chunks[i])
				if cx == nil {
					// skip bad expressions
					chunks[i] = &StrLiteral{}
					continue
				}

				// If chunk is a string, use it directly
				if cx.Type() == Str {
					chunks[i] = cx
					continue
				}

				if toStr, ok := cx.Type().get("to_str").(*FunctionDef); ok && toStr.ReturnType == Str && len(toStr.Parameters) == 0 {
					chunks[i] = c.createPrimitiveMethodNode(cx, toStr.Name, []Expression{}, toStr, nil)
					continue
				}

				if strMod := c.findModuleByPath("ard/string"); strMod != nil {
					toStringTrait := strMod.Get("ToString").Type.(*Trait)
					if !cx.Type().hasTrait(toStringTrait) {
						c.addError(typeMismatch(toStringTrait, cx.Type()), s.Chunks[i].GetLocation())
						// a non-stringable chunk stays empty
						chunks[i] = &StrLiteral{}
						continue
					}

					// For non-string types that satisfy ToString trait, wrap with to_str() call
					toStrMethod := toStringTrait.methods[0]
					methodNode := c.createPrimitiveMethodNode(cx, toStrMethod.Name, []Expression{}, &toStrMethod, nil)
					chunks[i] = methodNode
					continue
				}

				c.addError(fmt.Sprintf("Type mismatch: Expected stringable value, got %s", cx.Type()), s.Chunks[i].GetLocation())
				chunks[i] = &StrLiteral{}
			}
			return &TemplateStr{chunks}
		}
	case *parse.Identifier:
		if sym, ok := c.scope.get(s.Name); ok {
			return &Variable{*sym}
		}
		c.addError(fmt.Sprintf("Undefined variable: %s", s.Name), s.GetLocation())
		c.halted = true
		return nil
	case *parse.FunctionValueCall:
		{
			callee := c.checkExpr(s.Callee)
			if callee == nil {
				return nil
			}
			return c.checkFunctionValueCall(callee, s.Args, s.TypeArgs, s.GetLocation(), s.Callee.String())
		}
	case *parse.FunctionCall:
		{
			if s.Name == "panic" {
				if len(s.TypeArgs) > 0 {
					c.addError("function panic does not take type arguments", s.GetLocation())
					return nil
				}
				if len(s.Args) != 1 {
					c.addError("Incorrect number of arguments: 'panic' requires a message", s.GetLocation())
					return nil
				}
				message := c.checkExpr(s.Args[0].Value)
				if message == nil {
					return nil
				}

				return &Panic{
					Message: message,
					node:    s,
				}
			}

			// Find the function in the scope
			fnSym, got := c.scope.get(s.Name)
			if !got {
				c.addError(fmt.Sprintf("Undefined function: %s", s.Name), s.GetLocation())
				return nil
			}

			// Cast to FunctionDef
			var fnDef *FunctionDef
			var ok bool

			// Try different types for the function symbol
			fnDef, ok = fnSym.Type.(*FunctionDef)
			if !ok {
				c.addError(fmt.Sprintf("Not a function: %s", s.Name), s.GetLocation())
				return nil
			}

			callTypeArgs := c.resolveCallTypeArgs(s.TypeArgs)

			// Resolve named arguments to positional arguments (for expressions only)
			resolvedExprs, err := c.resolveArguments(s.Args, fnDef.Parameters)
			if err != nil {
				c.addError(err.Error(), s.GetLocation())
				return nil
			}

			// Check argument count after resolving
			numOmittedArgs := 0
			if len(resolvedExprs) < len(fnDef.Parameters) {
				// Find first non-nullable parameter that's missing
				for i := len(resolvedExprs); i < len(fnDef.Parameters); i++ {
					paramType := fnDef.Parameters[i].Type
					if _, isMaybe := paramType.(*Maybe); !isMaybe {
						c.addError(fmt.Sprintf("missing argument for parameter: %s", fnDef.Parameters[i].Name), s.GetLocation())
						return nil
					}
				}
				numOmittedArgs = len(fnDef.Parameters) - len(resolvedExprs)
			} else if len(resolvedExprs) > len(fnDef.Parameters) {
				c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d",
					len(fnDef.Parameters), len(resolvedExprs)), s.GetLocation())
				resolvedExprs = resolvedExprs[:len(fnDef.Parameters)]
			}

			// Setup generics if function has them
			fnDefCopy, genericScope := c.setupFunctionGenerics(fnDef)

			// Check and process arguments (handles both generics and mutability)
			args, fnToUse := c.checkAndProcessArguments(fnDef, resolvedExprs, fnDefCopy, genericScope, numOmittedArgs)
			if args == nil {
				return nil
			}
			if len(callTypeArgs) > 0 {
				specialized, err := c.resolveGenericFunction(fnDef, args, callTypeArgs, s.GetLocation())
				if err != nil {
					c.addError(err.Error(), s.GetLocation())
					return nil
				}
				fnToUse = specialized
			}

			call := &FunctionCall{
				Name:       s.Name,
				Args:       args,
				TypeArgs:   callTypeArgs,
				fn:         fnToUse,
				ReturnType: fnToUse.ReturnType,
			}
			return call
		}
	case *parse.InstanceProperty:
		{
			subj := c.checkExpr(s.Target)
			if subj == nil {
				// panic(fmt.Errorf("Cannot access %s on nil", s.Property))
				return nil
			}

			if foreign, ok := subj.Type().(*ForeignType); ok {
				if !foreign.FieldsLoaded {
					foreign.Fields, foreign.UnsupportedFields = loadForeignTypeFields(foreign)
					foreign.FieldsLoaded = true
				}
				if reason := foreign.UnsupportedFields[s.Property.Name]; reason != "" {
					c.addError(fmt.Sprintf("Unsupported foreign field %s.%s: %s", foreign, s.Property.Name, reason), s.Property.GetLocation())
					return nil
				}
				if fieldType := foreign.Fields[s.Property.Name]; fieldType != nil {
					return &ForeignFieldAccess{Subject: subj, Target: foreign.Target, Symbol: s.Property.Name, _type: fieldType}
				}
			}

			propType := subj.Type().get(s.Property.Name)
			foreignPointerReceiver := false
			if propType == nil {
				if foreign, ok := subj.Type().(*ForeignType); ok && !foreign.Pointer {
					pointerForeign := *foreign
					pointerForeign.Pointer = true
					pointerForeign.Methods = nil
					pointerForeign.UnsupportedMethods = nil
					pointerForeign.MethodsLoaded = false
					if pointerSig := pointerForeign.get(s.Property.Name); pointerSig != nil {
						if !c.isMutable(subj) {
							c.addError(fmt.Sprintf("Cannot access pointer receiver method %s.%s on immutable value", foreign, s.Property.Name), s.Property.GetLocation())
							return nil
						}
						propType = pointerSig
						foreignPointerReceiver = true
					} else if reason := pointerForeign.UnsupportedMethods[s.Property.Name]; reason != "" {
						c.addError(fmt.Sprintf("Unsupported foreign method %s.%s: %s", foreign, s.Property.Name, reason), s.Property.GetLocation())
						return nil
					}
				}
			}
			if propType == nil {
				if foreign, ok := subj.Type().(*ForeignType); ok {
					if !foreign.MethodsLoaded {
						foreign.Methods, foreign.UnsupportedMethods = loadForeignTypeMethods(foreign)
						foreign.MethodsLoaded = true
					}
					if reason := foreign.UnsupportedMethods[s.Property.Name]; reason != "" {
						c.addError(fmt.Sprintf("Unsupported foreign method %s.%s: %s", foreign, s.Property.Name, reason), s.Property.GetLocation())
						return nil
					}
				}
				c.addError(fmt.Sprintf("Undefined: %s.%s", subj, s.Property.Name), s.Property.GetLocation())
				return nil
			}

			if fnDef, ok := propType.(*FunctionDef); ok {
				if foreign, ok := subj.Type().(*ForeignType); ok {
					pointer := foreign.Pointer || foreignPointerReceiver
					return &ForeignMethodValue{Subject: subj, Target: foreign.Target, Namespace: foreign.Namespace, Qualifier: foreign.Qualifier, Receiver: foreign.Name, Pointer: pointer, Symbol: s.Property.Name, _type: fnDef}
				}
			}

			prop := &InstanceProperty{
				Subject:  subj,
				Property: s.Property.Name,
				_type:    propType,
			}

			// Pre-compute which kind of property this is based on subject type
			switch subj.Type().(type) {
			case *StructDef:
				prop.Kind = StructSubject
			default:
				// unreachable
				c.addError(fmt.Sprintf("Cannot access property on type %s", subj.Type()), s.Property.GetLocation())
			}

			return prop
		}
	case *parse.InstanceMethod:
		{
			subj := c.checkExpr(s.Target)
			if subj == nil {
				c.addError(fmt.Sprintf("Cannot access %s on Void", s.Method.Name), s.Method.GetLocation())
				return nil
			}

			if subj.Type() == nil {
				panic(fmt.Errorf("Cannot access %+v on nil: %s", subj.(*Variable).sym, s.Target))
			}
			var sig Type
			if structDef, ok := subj.Type().(*StructDef); ok {
				if method, ok := c.structMethod(structDef, s.Method.Name); ok {
					sig = method
				}
			} else {
				sig = subj.Type().get(s.Method.Name)
			}
			foreignPointerReceiver := false
			if sig == nil {
				if foreign, ok := subj.Type().(*ForeignType); ok {
					if !foreign.MethodsLoaded {
						foreign.Methods, foreign.UnsupportedMethods = loadForeignTypeMethods(foreign)
						foreign.MethodsLoaded = true
					}
					if reason := foreign.UnsupportedMethods[s.Method.Name]; reason != "" {
						c.addError(fmt.Sprintf("Unsupported foreign method %s.%s: %s", foreign, s.Method.Name, reason), s.Method.GetLocation())
						return nil
					}
					if !foreign.Pointer {
						pointerForeign := *foreign
						pointerForeign.Pointer = true
						pointerForeign.Methods = nil
						pointerForeign.UnsupportedMethods = nil
						pointerForeign.MethodsLoaded = false
						if pointerSig := pointerForeign.get(s.Method.Name); pointerSig != nil {
							if !c.isMutable(subj) {
								c.addError(fmt.Sprintf("Cannot call pointer receiver method %s.%s on immutable value", foreign, s.Method.Name), s.Method.GetLocation())
								return nil
							}
							sig = pointerSig
							foreignPointerReceiver = true
						} else if reason := pointerForeign.UnsupportedMethods[s.Method.Name]; reason != "" {
							c.addError(fmt.Sprintf("Unsupported foreign method %s.%s: %s", foreign, s.Method.Name, reason), s.Method.GetLocation())
							return nil
						}
					}
				}
			}
			if sig == nil {
				if call, ok := c.checkFunctionFieldCall(subj, s.Method, s.GetLocation()); ok {
					return call
				}
				c.addError(fmt.Sprintf("Undefined: %s.%s", subj, s.Method.Name), s.Method.GetLocation())
				return nil
			}

			fnDef, ok := sig.(*FunctionDef)
			if !ok {
				c.addError(fmt.Sprintf("%s.%s is not a function", subj, s.Method.Name), s.Method.GetLocation())
				return nil
			}

			if fnDef.Mutates && !c.isMutable(subj) {
				c.addError(fmt.Sprintf("Cannot mutate immutable '%s' with '.%s()'", subj, s.Method.Name), s.Method.GetLocation())
				return nil
			}

			// Resolve named and positional arguments to match parameters
			resolvedExprs, err := c.resolveArguments(s.Method.Args, fnDef.Parameters)
			if err != nil {
				c.addError(err.Error(), s.GetLocation())
				return nil
			}

			// Check argument count and validate omitted arguments
			numOmittedArgs := 0
			if len(resolvedExprs) < len(fnDef.Parameters) {
				// Find first non-nullable parameter that's missing
				for i := len(resolvedExprs); i < len(fnDef.Parameters); i++ {
					paramType := fnDef.Parameters[i].Type
					if _, isMaybe := paramType.(*Maybe); !isMaybe {
						c.addError(fmt.Sprintf("missing argument for parameter: %s", fnDef.Parameters[i].Name), s.GetLocation())
						return nil
					}
				}
				numOmittedArgs = len(fnDef.Parameters) - len(resolvedExprs)
			} else if len(resolvedExprs) > len(fnDef.Parameters) {
				c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d",
					len(fnDef.Parameters), len(resolvedExprs)), s.GetLocation())
				resolvedExprs = resolvedExprs[:len(fnDef.Parameters)]
			}

			// For methods on generic struct instances, bind struct generics to method parameters.
			// When calling a method on a generic struct instance, the method's generic parameters
			// should use the types that were resolved during struct instantiation.
			// Example: For Box{ item: 42 }, calling put(x: $T) should require Int for $T.
			var genericScope *SymbolTable
			var fnDefCopy *FunctionDef

			// Check if the subject is a struct and if the original struct definition has generics
			structType, isStruct := subj.Type().(*StructDef)
			if isStruct {
				// Look up the original named struct definition by module/name to check if it's generic.
				originalDef := c.structDefinition(structType)

				// If the original definition has generics, the current structType might be
				// a specialized version with concrete field types (e.g., item: Int instead of item: $T)
				if originalDef != nil && originalDef.hasGenerics() {
					genericParams := genericParamsForFunction(fnDef)
					genericParams = appendUniqueStrings(genericParams, originalDef.GenericParams...)
					if len(genericParams) > 0 {
						// Create generic scope with fresh TypeVar instances
						genericScope = c.scope.createGenericScope(genericParams)

						// Build mapping from generic names to their resolved types.
						// Compare original struct fields (which contain generics like $T)
						// with current struct fields (which have concrete types like Int).
						// This tells us what each generic parameter should be bound to.
						genericBindings := c.extractGenericBindingsFromSpecializedStruct(originalDef, structType)

						// Bind the method's generic parameters based on the struct instance's specialization
						for paramName, resolvedType := range genericBindings {
							if gv, exists := (*genericScope.genericContext)[paramName]; exists {
								gv.actual = resolvedType
								gv.bound = true
							}
						}

						// Copy the function with the struct's generic bindings applied
						fnDefCopy = copyFunctionWithTypeVarMap(fnDef, *genericScope.genericContext)
						fnDefCopy.GenericBindings = cloneTypeMap(genericBindings)
					} else {
						fnDefCopy = fnDef
					}
				} else {
					// Not a generic struct, use normal generic setup (for methods with their own generics)
					fnDefCopy, genericScope = c.setupFunctionGenerics(fnDef)
				}
			} else {
				// Not a struct - setup generics if the method itself has them
				fnDefCopy, genericScope = c.setupFunctionGenerics(fnDef)
			}

			callTypeArgs := c.resolveCallTypeArgs(s.Method.TypeArgs)
			if len(callTypeArgs) > 0 {
				methodGenericParams := c.explicitMethodGenericParams(fnDef, subj.Type())
				if len(methodGenericParams) == 0 {
					c.addError(fmt.Sprintf("function %s does not take type arguments", fnDef.Name), s.GetLocation())
					return nil
				}
				if len(callTypeArgs) != len(methodGenericParams) {
					c.addError(fmt.Sprintf("Expected %d type arguments, got %d", len(methodGenericParams), len(callTypeArgs)), s.GetLocation())
					return nil
				}
				if genericScope == nil {
					genericScope = c.scope.createGenericScope(genericParamsForFunction(fnDef))
					fnDefCopy = copyFunctionWithTypeVarMap(fnDef, *genericScope.genericContext)
				}
				for i, actual := range callTypeArgs {
					if err := genericScope.bindGeneric(methodGenericParams[i], actual); err != nil {
						c.addError(err.Error(), s.GetLocation())
						return nil
					}
				}
			}

			// Check and process arguments (handles both generics and mutability)
			args, fnToUse := c.checkAndProcessArguments(fnDef, resolvedExprs, fnDefCopy, genericScope, numOmittedArgs)
			if args == nil {
				return nil
			}
			if foreign, ok := subj.Type().(*ForeignType); ok {
				for _, arg := range s.Method.Args {
					if arg.Name != "" {
						c.addError("Foreign method calls do not support named arguments", arg.GetLocation())
						return nil
					}
				}
				pointer := foreign.Pointer || foreignPointerReceiver
				return &ForeignMethodCall{Subject: subj, Target: foreign.Target, Namespace: foreign.Namespace, Qualifier: foreign.Qualifier, Receiver: foreign.Name, Pointer: pointer, Symbol: s.Method.Name, Call: &FunctionCall{Name: s.Method.Name, Args: args, fn: fnToUse, ReturnType: fnToUse.ReturnType}}
			}
			// Create function call
			return c.createPrimitiveMethodNode(subj, s.Method.Name, args, fnToUse, callTypeArgs)
		}
	case *parse.UnaryExpression:
		{
			value := c.checkExpr(s.Operand)
			if value == nil {
				return nil
			}
			if s.Operator == parse.Minus {
				if !isSignedArithmeticLike(value.Type()) {
					c.addError("Only signed numbers can be negated with '-'", s.GetLocation())
					return nil
				}
				return &Negation{value}
			}

			if value.Type() != Bool {
				c.addError("Only booleans can be negated with 'not'", s.GetLocation())
				return nil
			}
			return &Not{value}
		}
	case *parse.BinaryExpression:
		{
			switch s.Operator {
			case parse.Plus:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if !left.Type().equal(right.Type()) {
						c.addError("Cannot add different types", s.GetLocation())
						return nil
					}
					if isArithmeticIntegerLike(left.Type()) {
						return &IntAddition{left, right}
					}
					if isArithmeticFloatLike(left.Type()) {
						return &FloatAddition{left, right}
					}
					if left.Type() == Str {
						return &StrAddition{left, right}
					}
					c.addError("The '-' operator can only be used for Int or Float64", s.GetLocation())
					return nil
				}
			case parse.Minus:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if !left.Type().equal(right.Type()) {
						c.addError("Cannot subtract different types", s.GetLocation())
						return nil
					}
					if isArithmeticIntegerLike(left.Type()) {
						return &IntSubtraction{left, right}
					}
					if isArithmeticFloatLike(left.Type()) {
						return &FloatSubtraction{left, right}
					}
					c.addError("The '+' operator can only be used for Int or Float64", s.GetLocation())
					return nil
				}
			case parse.Multiply:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if !left.Type().equal(right.Type()) {
						c.addError("Cannot multiply different types", s.GetLocation())
						return nil
					}
					if isArithmeticIntegerLike(left.Type()) {
						return &IntMultiplication{left, right}
					}
					if isArithmeticFloatLike(left.Type()) {
						return &FloatMultiplication{left, right}
					}
					c.addError("The '*' operator can only be used for Int or Float64", s.GetLocation())
					return nil
				}
			case parse.Divide:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if !left.Type().equal(right.Type()) {
						c.addError("Cannot divide different types", s.GetLocation())
						return nil
					}
					if isArithmeticIntegerLike(left.Type()) {
						return &IntDivision{left, right}
					}
					if isArithmeticFloatLike(left.Type()) {
						return &FloatDivision{left, right}
					}
					c.addError("The '/' operator can only be used for Int or Float64", s.GetLocation())
					return nil
				}
			case parse.Modulo:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if !left.Type().equal(right.Type()) {
						c.addError("Cannot modulo different types", s.GetLocation())
						return nil
					}
					if isArithmeticIntegerLike(left.Type()) {
						return &IntModulo{left, right}
					}
					c.addError("The '%' operator can only be used for integer scalars", s.GetLocation())
					return nil
				}
			case parse.GreaterThan:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					// Allow Enum vs Int comparisons
					if c.areTypesComparable(left.Type(), right.Type()) {
						if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
							return &IntGreater{left, right}
						}
						if isRelationalFloatLike(left.Type()) {
							return &FloatGreater{left, right}
						}
					}
					c.addError("Cannot compare different types", s.GetLocation())
					return nil
				}
			case parse.GreaterThanOrEqual:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					// Allow Enum vs Int comparisons
					if c.areTypesComparable(left.Type(), right.Type()) {
						if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
							return &IntGreaterEqual{left, right}
						}
						if isRelationalFloatLike(left.Type()) {
							return &FloatGreaterEqual{left, right}
						}
					}
					c.addError("Cannot compare different types", s.GetLocation())
					return nil
				}
			case parse.LessThan:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					// Allow Enum vs Int comparisons
					if c.areTypesComparable(left.Type(), right.Type()) {
						if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
							return &IntLess{left, right}
						}
						if isRelationalFloatLike(left.Type()) {
							return &FloatLess{left, right}
						}
					}
					c.addError("Cannot compare different types", s.GetLocation())
					return nil
				}
			case parse.LessThanOrEqual:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					// Allow Enum vs Int comparisons
					if c.areTypesComparable(left.Type(), right.Type()) {
						if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
							return &IntLessEqual{left, right}
						}
						if isRelationalFloatLike(left.Type()) {
							return &FloatLessEqual{left, right}
						}
					}
					c.addError("Cannot compare different types", s.GetLocation())
					return nil
				}
			case parse.Equal, parse.NotEqual:
				{
					operator := "=="
					if s.Operator == parse.NotEqual {
						operator = "!="
					}

					left, right := c.checkExpr(s.Left), c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftMaybe, leftIsMaybe := left.Type().(*Maybe)
					rightMaybe, rightIsMaybe := right.Type().(*Maybe)
					if leftIsMaybe && rightIsMaybe {
						leftInner := leftMaybe.Of()
						rightInner := rightMaybe.Of()
						if leftInner != Void && rightInner != Void && !c.areCompatible(leftInner, rightInner) && !c.areCompatible(rightInner, leftInner) {
							c.addError(fmt.Sprintf("Invalid: %s %s %s", left.Type(), operator, right.Type()), s.GetLocation())
							return nil
						}
						// Equality is only supported on nullable primitives. The
						// inner type (the non-Void side when one is `none`) must be a
						// comparable value type.
						inner := leftInner
						if inner == Void {
							inner = rightInner
						}
						if inner != Void && !isComparableValueType(inner) {
							c.addError(fmt.Sprintf("Invalid: %s %s %s", left.Type(), operator, right.Type()), s.GetLocation())
							return nil
						}
						if s.Operator == parse.NotEqual {
							return &Inequality{left, right}
						}
						return &Equality{left, right}
					}

					// Allow Enum vs Int and Int vs Enum comparisons
					if !c.areTypesComparable(left.Type(), right.Type()) {
						c.addError(fmt.Sprintf("Invalid: %s %s %s", left.Type(), operator, right.Type()), s.GetLocation())
						return nil
					}

					if !isComparableValueType(left.Type()) || !isComparableValueType(right.Type()) {
						c.addError(fmt.Sprintf("Invalid: %s %s %s", left.Type(), operator, right.Type()), s.GetLocation())
						return nil
					}
					if s.Operator == parse.NotEqual {
						return &Inequality{left, right}
					}
					return &Equality{left, right}
				}
			case parse.And:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != Bool || right.Type() != Bool {
						c.addError("The 'and' operator can only be used between Bools", s.GetLocation())
						return nil
					}

					return &And{left, right}
				}
			case parse.Or:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != Bool || right.Type() != Bool {
						c.addError("The 'or' operator can only be used with Boolean values", s.GetLocation())
						return nil
					}

					return &Or{left, right}
				}
			default:
				panic(fmt.Errorf("Unexpected operator: %v", s.Operator))
			}
		}

	case *parse.ChainedComparison:
		{
			// Validate that only relative operators are used (not == or !=)
			for _, op := range s.Operators {
				if op == parse.Equal || op == parse.NotEqual {
					c.addError("equality operators cannot be chained", s.GetLocation())
					return nil
				}
			}

			// Transform chained comparison into nested And expressions
			// a op1 b op2 c => (a op1 b) && (b op2 c)
			var result Expression

			// Build the first comparison: operands[0] op operators[0] operands[1]
			firstComparison := c.buildComparison(s.Operands[0], s.Operators[0], s.Operands[1])
			if firstComparison == nil {
				return nil
			}
			result = firstComparison

			// Build remaining comparisons and AND them together
			for i := 1; i < len(s.Operators); i++ {
				nextComparison := c.buildComparison(s.Operands[i], s.Operators[i], s.Operands[i+1])
				if nextComparison == nil {
					return nil
				}

				// AND the previous result with the next comparison
				result = &And{result, nextComparison}
			}

			return result
		}

	// [refactor] a lot of the function call checking can be extracted
	// - validate args and resolve generics
	case *parse.StaticFunction:
		{
			// Handle local functions
			absolutePath := s.Target.String() + "::" + s.Function.Name
			if sym, ok := c.scope.get(absolutePath); ok {
				fnDef := sym.Type.(*FunctionDef)
				callTypeArgs := c.resolveCallTypeArgs(s.Function.TypeArgs)

				// Resolve named and positional arguments to match parameters
				resolvedExprs, err := c.resolveArguments(s.Function.Args, fnDef.Parameters)
				if err != nil {
					c.addError(err.Error(), s.GetLocation())
					return nil
				}

				// We also need argument mutability aligned with parameters
				resolvedArgs := make([]parse.Argument, len(fnDef.Parameters))
				if len(s.Function.Args) > 0 && s.Function.Args[0].Name != "" {
					paramMap := make(map[string]int)
					for i, param := range fnDef.Parameters {
						paramMap[param.Name] = i
					}
					for _, arg := range s.Function.Args {
						if index, exists := paramMap[arg.Name]; exists {
							resolvedArgs[index] = parse.Argument{
								Location: arg.Location,
								Name:     "",
								Value:    arg.Value,
								Mutable:  arg.Mutable,
							}
						}
					}
				} else {
					copy(resolvedArgs, s.Function.Args)
				}

				// Check and process arguments
				args := make([]Expression, len(resolvedArgs))
				for i := range resolvedArgs {
					paramType := fnDef.Parameters[i].Type

					var checkedArg Expression
					switch resolvedExprs[i].(type) {
					case *parse.ListLiteral, *parse.MapLiteral:
						checkedArg = c.checkExprAs(resolvedExprs[i], paramType)
					default:
						checkedArg = c.checkExpr(resolvedExprs[i])
					}

					if checkedArg == nil {
						return nil
					}

					// Type check the argument against the parameter type
					if !c.areCompatible(paramType, checkedArg.Type()) {
						c.addError(typeMismatch(paramType, checkedArg.Type()), resolvedExprs[i].GetLocation())
						return nil
					}

					// Check mutability constraints if needed
					if fnDef.Parameters[i].Mutable && !c.isMutable(checkedArg) {
						c.addError(fmt.Sprintf("Type mismatch: Expected a mutable %s", fnDef.Parameters[i].Type.String()), resolvedExprs[i].GetLocation())
					}

					args[i] = checkedArg
				}

				call := &FunctionCall{
					Name:       absolutePath,
					Args:       args,
					TypeArgs:   callTypeArgs,
					fn:         fnDef,
					ReturnType: fnDef.ReturnType,
				}

				// Use new generic resolution system
				if fnDef.hasGenerics() || len(callTypeArgs) > 0 {
					specialized, err := c.resolveGenericFunction(fnDef, args, callTypeArgs, s.GetLocation())
					if err != nil {
						c.addError(err.Error(), s.GetLocation())
						return nil
					}

					call.fn = specialized
					call.ReturnType = specialized.ReturnType
				}

				return call
			}

			// find the function in a module or Go package namespace
			modName, name := c.destructurePath(s)
			if goPkg := c.program.GoImports[modName]; goPkg != nil {
				fnDef := goPkg.Functions[name]
				if fnDef == nil {
					if reason, ok := goPkg.UnsupportedFunctions[name]; ok {
						c.addError(fmt.Sprintf("Unsupported Go function %s::%s: %s", modName, name, reason), s.GetLocation())
					} else {
						c.addError(fmt.Sprintf("Undefined Go function: %s::%s", modName, name), s.GetLocation())
					}
					return nil
				}
				for _, arg := range s.Function.Args {
					if arg.Name != "" {
						c.addError("Go function calls do not support named arguments", arg.GetLocation())
						return nil
					}
				}
				resolvedExprs, err := c.resolveArguments(s.Function.Args, fnDef.Parameters)
				if err != nil {
					c.addError(err.Error(), s.GetLocation())
					return nil
				}
				if len(resolvedExprs) != len(fnDef.Parameters) {
					c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d", len(fnDef.Parameters), len(resolvedExprs)), s.GetLocation())
					return nil
				}
				args := make([]Expression, len(resolvedExprs))
				for i, expr := range resolvedExprs {
					checkedArg := c.checkExprAs(expr, fnDef.Parameters[i].Type)
					if checkedArg == nil {
						return nil
					}
					if !c.areCompatible(fnDef.Parameters[i].Type, checkedArg.Type()) {
						c.addError(typeMismatch(fnDef.Parameters[i].Type, checkedArg.Type()), expr.GetLocation())
						return nil
					}
					if fnDef.Parameters[i].Mutable && !c.isMutable(checkedArg) {
						c.addError(fmt.Sprintf("Type mismatch: Expected a mutable %s", fnDef.Parameters[i].Type.String()), expr.GetLocation())
						return nil
					}
					args[i] = checkedArg
				}
				return &ForeignFunctionCall{Target: "go", Namespace: goPkg.Path, Qualifier: goPkg.TypesName, Symbol: name, Call: &FunctionCall{Name: name, Args: args, fn: fnDef, ReturnType: fnDef.ReturnType}}
			}

			var fnDef *FunctionDef
			mod := c.resolveModule(modName)
			if mod == nil {
				c.addError(fmt.Sprintf("Undefined module: %s", modName), s.Target.GetLocation())
				return nil
			}

			sym := mod.Get(name)
			if sym.IsZero() {
				targetName := s.Target.String()
				c.addError(fmt.Sprintf("Undefined: %s::%s", targetName, s.Function.Name), s.GetLocation())
				return nil
			}

			// Handle both regular functions and external functions
			var ok bool
			switch fn := sym.Type.(type) {
			case *FunctionDef:
				fnDef = fn
				ok = true
			default:
				ok = false
			}

			if !ok {
				targetName := s.Target.String()
				c.addError(fmt.Sprintf("%s::%s is not a function", targetName, s.Function.Name), s.GetLocation())
				return nil
			}
			callTypeArgs := c.resolveCallTypeArgs(s.Function.TypeArgs)

			// Resolve named and positional arguments to match parameters
			resolvedExprs, err := c.resolveArguments(s.Function.Args, fnDef.Parameters)
			if err != nil {
				c.addError(err.Error(), s.GetLocation())
				return nil
			}

			// Check argument count and validate omitted arguments
			numOmittedArgs := 0
			if len(resolvedExprs) < len(fnDef.Parameters) {
				// Find first non-nullable parameter that's missing
				for i := len(resolvedExprs); i < len(fnDef.Parameters); i++ {
					paramType := fnDef.Parameters[i].Type
					if _, isMaybe := paramType.(*Maybe); !isMaybe {
						c.addError(fmt.Sprintf("missing argument for parameter: %s", fnDef.Parameters[i].Name), s.GetLocation())
						return nil
					}
				}
				numOmittedArgs = len(fnDef.Parameters) - len(resolvedExprs)
			} else if len(resolvedExprs) > len(fnDef.Parameters) {
				c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d",
					len(fnDef.Parameters), len(resolvedExprs)), s.GetLocation())
				resolvedExprs = resolvedExprs[:len(fnDef.Parameters)]
			}

			// Setup generics if function has them
			fnDefCopy, genericScope := c.setupFunctionGenerics(fnDef)

			// Check and process arguments (handles both generics and mutability)
			args, fnToUse := c.checkAndProcessArguments(fnDef, resolvedExprs, fnDefCopy, genericScope, numOmittedArgs)
			if args == nil {
				return nil
			}
			if len(callTypeArgs) > 0 {
				specialized, err := c.resolveGenericFunction(fnDef, args, callTypeArgs, s.GetLocation())
				if err != nil {
					c.addError(err.Error(), s.GetLocation())
					return nil
				}
				fnToUse = specialized
			}

			// Create function call. Keep the source member name so lowering can
			// distinguish real module functions from function-typed module variables.
			callName := name
			call := &FunctionCall{
				Name:       callName,
				Args:       args,
				TypeArgs:   callTypeArgs,
				fn:         fnToUse,
				ReturnType: fnToUse.ReturnType,
			}
			// json::parse decodes into the requested type; parsing into a union is
			// ambiguous, so the checker rejects it (ADR 0031).
			if mod.Path() == "ard/json" && name == "parse" {
				if err := validateJSONParseTarget(call.ReturnType); err != nil {
					c.addError(err.Error(), s.GetLocation())
					return nil
				}
			}
			return &ModuleFunctionCall{
				Module: mod.Path(),
				Call:   call,
			}
		}
	case *parse.IfStatement:
		return c.checkIfChain(s)
	case *parse.FunctionDeclaration:
		return c.checkFunction(s, nil)
	case *parse.AnonymousFunction:
		{
			// Resolve parameters and return type (no type context for inference)
			params := c.resolveParametersWithContext(s.Parameters, nil)
			returnType := c.resolveReturnTypeWithContext(s.ReturnType, nil)

			// Create function definition
			uniqueName := fmt.Sprintf("anon_func_%p", s)
			fn := &FunctionDef{
				Name:                    uniqueName,
				Parameters:              params,
				ReturnType:              returnType,
				InferReturnTypeFromBody: s.ReturnType == nil,
				Body:                    nil,
			}

			// Check body
			c.pushFunctionGenericContext(fn)
			body := c.checkBlockWithExpected(s.Body, func() {
				c.scope.expectReturn(returnType)
				for _, param := range params {
					c.scope.add(param.Name, param.Type, param.Mutable)
				}
			}, returnType, true)
			c.popFunctionGenericContext()

			// Add function to scope after body is checked (for generic resolution support)
			c.scope.add(uniqueName, fn, false)

			// Validate return type
			if returnType != Void && !c.areCompatible(returnType, body.Type()) {
				c.addError(typeMismatch(returnType, body.Type()), s.GetLocation())
				return nil
			}

			fn.Body = body
			return fn
		}
	case *parse.StaticFunctionDeclaration:
		fn := c.checkFunction(&s.FunctionDeclaration, nil)
		if fn != nil {
			fn.Name = s.Path.String()
			c.scope.add(fn.Name, fn, false)
		}

		return fn
	case *parse.ListLiteral:
		// checkList returns a typed-nil *ListLiteral on failure; normalize to an
		// interface nil so callers' `== nil` checks hold and never deref it.
		if result := c.checkList(nil, s); result != nil {
			return result
		}
		return nil
	case *parse.MapLiteral:
		if result := c.checkMap(nil, s); result != nil {
			return result
		}
		return nil
	case *parse.SelectExpression:
		allowMixedVoid := discardThisExpr || c.expectedExpr == Void
		previousArmDiscard := c.matchArmDiscardContext
		c.matchArmDiscardContext = allowMixedVoid
		defer func() {
			c.matchArmDiscardContext = previousArmDiscard
		}()

		sel := &Select{}
		var resultType Type
		hasDefault := false

		for _, arm := range s.Cases {
			// Default arm: the head is the `_` identifier.
			if id, ok := arm.Op.(*parse.Identifier); ok && id.Name == "_" {
				if arm.Binding != nil {
					c.addError("The default select arm cannot bind a value", arm.Op.GetLocation())
				}
				if hasDefault {
					c.addError("Duplicate default (_) arm in select", arm.Op.GetLocation())
				}
				hasDefault = true
				body := c.checkMatchArmBlock(arm.Body, nil)
				sel.Arms = append(sel.Arms, SelectArm{Kind: SelectArmDefault, Body: body})
				if merged, ok := mergeMatchResultType(c, resultType, body.Type(), arm.Op.GetLocation(), allowMixedVoid); ok {
					resultType = merged
				}
				continue
			}

			op, ok := arm.Op.(*parse.InstanceMethod)
			if !ok {
				c.addError("A select arm must be a channel recv() or send() operation", arm.Op.GetLocation())
				continue
			}

			channel := c.checkExpr(op.Target)
			if channel == nil {
				continue
			}
			elem, ok := channelElementType(channel.Type())
			if !ok {
				c.addError(fmt.Sprintf("A select arm operates on a channel, but got %s", channel.Type().String()), op.Target.GetLocation())
				continue
			}

			switch op.Method.Name {
			case "recv":
				if !channelCanRecv(channel.Type()) {
					c.addError(fmt.Sprintf("recv() is not available on %s", channel.Type().String()), op.GetLocation())
					continue
				}
				if len(op.Method.Args) != 0 {
					c.addError("recv() in a select arm takes no arguments", op.GetLocation())
				}
				var binding *Identifier
				if arm.Binding != nil {
					binding = &Identifier{Name: arm.Binding.Name}
				}
				body := c.checkMatchArmBlock(arm.Body, func() {
					if arm.Binding != nil {
						c.scope.add(arm.Binding.Name, &Maybe{elem}, false)
					}
				})
				sel.Arms = append(sel.Arms, SelectArm{Kind: SelectArmRecv, Channel: channel, Binding: binding, ElemType: elem, Body: body})
				if merged, ok := mergeMatchResultType(c, resultType, body.Type(), op.GetLocation(), allowMixedVoid); ok {
					resultType = merged
				}
			case "send":
				if !channelCanSend(channel.Type()) {
					c.addError(fmt.Sprintf("send() is not available on %s", channel.Type().String()), op.GetLocation())
					continue
				}
				if arm.Binding != nil {
					c.addError("A select send arm cannot bind a value", arm.Op.GetLocation())
				}
				if len(op.Method.Args) != 1 {
					c.addError("send() in a select arm takes exactly one argument", op.GetLocation())
					continue
				}
				value := c.checkExprAs(op.Method.Args[0].Value, elem)
				body := c.checkMatchArmBlock(arm.Body, nil)
				sel.Arms = append(sel.Arms, SelectArm{Kind: SelectArmSend, Channel: channel, ElemType: elem, Value: value, Body: body})
				if merged, ok := mergeMatchResultType(c, resultType, body.Type(), op.GetLocation(), allowMixedVoid); ok {
					resultType = merged
				}
			default:
				c.addError(fmt.Sprintf("A select arm must use recv() or send(), got %s()", op.Method.Name), op.GetLocation())
			}
		}

		sel.ResultType = resultType
		return sel

	case *parse.MatchExpression:
		allowMixedVoid := discardThisExpr || c.expectedExpr == Void
		previousArmDiscard := c.matchArmDiscardContext
		c.matchArmDiscardContext = allowMixedVoid
		defer func() {
			c.matchArmDiscardContext = previousArmDiscard
		}()

		// Check the subject
		subject := c.checkExpr(s.Subject)
		if subject == nil {
			return nil
		}

		// For Maybe types, generate an OptionMatch
		if maybeType, ok := subject.Type().(*Maybe); ok {
			var patternIdent *Identifier
			var someBody *Block
			var noneBody *Block
			bindingMutable := c.isMutable(subject)

			// Process the cases
			for _, matchCase := range s.Cases {
				// Check if it's the default case (_)
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok {
					if id.Name == "_" {
						// This is the None case
						noneBody = c.checkMatchArmBlock(matchCase.Body, nil)
					} else {
						// This is the Some case with a variable binding
						// Create a new scope for the body with the pattern bound to the unwrapped value
						someBody = c.checkMatchArmBlock(matchCase.Body, func() {
							// Add the pattern name as a variable in the scope with the inner type
							// For example, if the Maybe is Str?, the pattern should be a Str
							c.scope.add(id.Name, maybeType.of, bindingMutable)
						})

						// Create an identifier to use in the Match struct
						patternIdent = &Identifier{Name: id.Name}
					}
				} else {
					c.addError("Pattern in Maybe match must be an identifier", matchCase.Pattern.GetLocation())
					return nil
				}
			}

			// Ensure we have both some and none cases
			if someBody == nil {
				c.addError("Match on a Maybe type must include a binding case", s.GetLocation())
				return nil
			}

			if noneBody == nil {
				c.addError("Match on a Maybe type must include a wildcard (_) case", s.GetLocation())
				return nil
			}

			resultType, ok := mergeMatchResultType(c, someBody.Type(), noneBody.Type(), s.GetLocation(), allowMixedVoid)
			if !ok {
				return nil
			}

			// Create the OptionMatch
			return &OptionMatch{
				Subject:   subject,
				InnerType: maybeType.of,
				Some: &Match{
					Pattern: patternIdent,
					Body:    someBody,
				},
				None:       noneBody,
				ResultType: resultType,
			}
		}

		// For Enum types, generate an EnumMatch
		if enumType, ok := subject.Type().(*Enum); ok {
			// Map to track which discriminant values we've seen. Imported Go enum-like
			// constants may have multiple exported aliases for the same value.
			seenDiscriminants := make(map[int]string)
			// Track whether we've seen a catch-all case
			hasCatchAll := false
			// Cases in the match statement mapped to enum variants
			cases := make([]*Block, len(enumType.Values))
			var catchAllBody *Block

			// Process the cases
			for _, matchCase := range s.Cases {
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok {
					if id.Name == "_" {
						// This is a catch-all case
						if hasCatchAll {
							c.addError("Duplicate catch-all case", matchCase.Pattern.GetLocation())
							return nil
						}

						hasCatchAll = true
						catchAllBody = c.checkMatchArmBlock(matchCase.Body, nil)
						continue
					}
				}

				// Handle enum variant case - the pattern should be a static property reference like Enum::Variant
				if staticProp, ok := matchCase.Pattern.(*parse.StaticProperty); ok {
					// Resolve the pattern using existing expression resolution logic
					patternExpr := c.checkExpr(staticProp)
					if patternExpr == nil {
						continue // Error already reported by checkExpr
					}

					// Check if the pattern resolves to an enum variant
					enumVariant, ok := patternExpr.(*EnumVariant)
					if !ok {
						c.addError("Pattern in enum match must be an enum variant", staticProp.GetLocation())
						continue
					}

					// Verify that the variant's enum matches the subject's enum
					if !enumVariant.enum.equal(enumType) {
						c.addError(fmt.Sprintf("Cannot match %s variant against %s enum",
							enumVariant.enum.Name, enumType.Name), staticProp.GetLocation())
						continue
					}

					// Get the variant name and index
					variantName := enumType.Values[enumVariant.Variant].Name
					variantIndex := int(enumVariant.Variant)

					// Check for duplicate cases by value, not just by name. This lets Go
					// enum-like constants import aliases while preserving closed enum
					// exhaustiveness over distinct values.
					discriminant := enumType.Values[enumVariant.Variant].Value
					current := fmt.Sprintf("%s::%s", enumType.Name, variantName)
					if previous, found := seenDiscriminants[discriminant]; found {
						if previous == current {
							c.addError(fmt.Sprintf("Duplicate case: %s", current), staticProp.GetLocation())
						} else {
							c.addError(fmt.Sprintf("Duplicate case: %s has same value as %s", current, previous), staticProp.GetLocation())
						}
						continue
					}
					seenDiscriminants[discriminant] = current

					// Check the body for this case
					body := c.checkMatchArmBlock(matchCase.Body, nil)
					cases[variantIndex] = body
				} else {
					c.addError("Pattern in enum match must be an enum variant or wildcard", matchCase.Pattern.GetLocation())
					return nil
				}
			}

			// Check if the match is exhaustive over distinct values. Aliases do not
			// require separate arms. Imported open Go enum-like types always require
			// a wildcard because Go may produce values outside exported constants.
			if !hasCatchAll {
				if enumType.Open {
					c.addError(fmt.Sprintf("Open enum-like Go type %s requires a catch-all (_) match case", enumType.Name), s.GetLocation())
				} else {
					missingValues := map[int]bool{}
					for i, value := range enumType.Values {
						if _, covered := seenDiscriminants[value.Value]; covered {
							continue
						}
						if cases[i] == nil && !missingValues[value.Value] {
							c.addError(fmt.Sprintf("Incomplete match: missing case for '%s::%s'", enumType.Name, value.Name), s.GetLocation())
							missingValues[value.Value] = true
						}
					}
				}
			}

			// Ensure all cases return compatible types
			var enumResultType Type
			for _, caseBody := range cases {
				if caseBody == nil {
					continue
				}
				var ok bool
				enumResultType, ok = mergeMatchResultType(c, enumResultType, caseBody.Type(), s.GetLocation(), allowMixedVoid)
				if !ok {
					return nil
				}
			}
			if catchAllBody != nil {
				var ok bool
				enumResultType, ok = mergeMatchResultType(c, enumResultType, catchAllBody.Type(), s.GetLocation(), allowMixedVoid)
				if !ok {
					return nil
				}
			}

			// Create the EnumMatch
			discriminantToIndex := make(map[int]int, len(enumType.Values))
			for i, value := range enumType.Values {
				if _, ok := discriminantToIndex[value.Value]; !ok {
					discriminantToIndex[value.Value] = i
				}
			}
			enumMatch := &EnumMatch{
				Subject:             subject,
				Cases:               cases,
				CatchAll:            catchAllBody,
				DiscriminantToIndex: discriminantToIndex,
				ResultType:          enumResultType,
			}

			return enumMatch
		}

		// For Bool types, generate a BoolMatch
		if subject.Type() == Bool {
			var trueBody, falseBody *Block
			// Track which cases we've seen
			seenTrue, seenFalse := false, false

			// Process the cases
			for _, matchCase := range s.Cases {
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok {
					if id.Name == "_" {
						// Catch-all cases aren't allowed for boolean matches
						c.addError("Catch-all case is not allowed for boolean matches", matchCase.Pattern.GetLocation())
						return nil
					}
				}

				// Handle boolean literal case
				if boolLit, ok := matchCase.Pattern.(*parse.BoolLiteral); ok {
					// Check for duplicates
					if boolLit.Value && seenTrue {
						c.addError("Duplicate case: 'true'", matchCase.Pattern.GetLocation())
						return nil
					}
					if !boolLit.Value && seenFalse {
						c.addError("Duplicate case: 'false'", matchCase.Pattern.GetLocation())
						return nil
					}

					// Process the body
					body := c.checkMatchArmBlock(matchCase.Body, nil)

					// Store the body in the appropriate field
					if boolLit.Value {
						seenTrue = true
						trueBody = body
					} else {
						seenFalse = true
						falseBody = body
					}
				} else {
					c.addError("Pattern in boolean match must be a boolean literal (true or false)", matchCase.Pattern.GetLocation())
					return nil
				}
			}

			// Check exhaustiveness
			if !seenTrue || !seenFalse {
				if !seenTrue {
					c.addError("Incomplete match: Missing case for 'true'", s.GetLocation())
				} else {
					c.addError("Incomplete match: Missing case for 'false'", s.GetLocation())
				}
				return nil
			}

			// Ensure both branches return compatible types. This allows one branch to
			// return a trait object while the other returns a concrete implementation.
			resultType, ok := mergeMatchResultType(c, trueBody.Type(), falseBody.Type(), s.GetLocation(), allowMixedVoid)
			if !ok {
				return nil
			}

			// Create and return the BoolMatch
			return &BoolMatch{
				Subject:    subject,
				True:       trueBody,
				False:      falseBody,
				ResultType: resultType,
			}
		}

		// For Union types, generate a UnionMatch
		if unionType, ok := subject.Type().(*Union); ok {
			bindingMutable := c.isMutable(subject)
			// Track which union types we've seen and their corresponding bodies
			typeCases := make(map[string]*Match)
			typeCasesByType := make(map[Type]*Match)
			var catchAllBody *Block

			// Record all types in the union
			unionTypeSet := make(map[string]Type)
			for _, t := range unionType.Types {
				unionTypeSet[t.String()] = t
			}

			// Process the cases
			for _, matchCase := range s.Cases {
				switch p := matchCase.Pattern.(type) {
				case *parse.Identifier:
					if p.Name == "_" {
						if catchAllBody != nil {
							c.addWarning("Duplicate catch-all case", matchCase.Pattern.GetLocation())
						} else {
							catchAllBody = c.checkMatchArmBlock(matchCase.Body, nil)
						}
						break
					}
					// Allow union type name as implicit binding to "it"
					matchedType, found := unionTypeSet[p.Name]
					if !found {
						c.addError("Catch-all case should be matched with '_'", matchCase.Pattern.GetLocation())
						break
					}
					if _, exists := typeCases[p.Name]; exists {
						c.addWarning(fmt.Sprintf("Duplicate case: %s", p.Name), matchCase.Pattern.GetLocation())
						break
					}
					body := c.checkMatchArmBlock(matchCase.Body, func() {
						c.scope.add("it", matchedType, bindingMutable)
					})
					matchNode := &Match{
						Pattern: &Identifier{Name: "it"},
						Body:    body,
					}
					typeCases[p.Name] = matchNode
					typeCasesByType[matchedType] = matchNode
				case *parse.FunctionCall:
					varName := p.Args[0].Value.(*parse.Identifier).Name
					typeName := p.Name

					// Check if the type exists in the union
					_, found := unionTypeSet[typeName]
					if !found {
						c.addError(fmt.Sprintf("Type %s is not part of union %s", typeName, unionType),
							matchCase.Pattern.GetLocation())
					}

					// Check for duplicates
					if _, exists := typeCases[typeName]; exists {
						c.addWarning(fmt.Sprintf("Duplicate case: %s", typeName), matchCase.Pattern.GetLocation())
					} else {

						// Get the actual type object
						matchedType := unionTypeSet[typeName]

						// Process the body with the matched type binding
						body := c.checkMatchArmBlock(matchCase.Body, func() {
							c.scope.add(varName, matchedType, bindingMutable)
						})
						matchNode := &Match{
							Pattern: &Identifier{Name: varName},
							Body:    body,
						}
						typeCases[typeName] = matchNode
						typeCasesByType[matchedType] = matchNode
					}
				}
			}

			// Check exhaustiveness if no catch-all is provided
			if catchAllBody == nil {
				for typeName := range unionTypeSet {
					if _, covered := typeCases[typeName]; !covered {
						c.addError(fmt.Sprintf("Incomplete match: missing case for '%s'", typeName),
							s.GetLocation())
					}
				}
			}

			// Ensure all cases return compatible types
			var unionResultType Type
			for _, caseBody := range typeCases {
				if caseBody == nil {
					continue
				}
				var ok bool
				unionResultType, ok = mergeMatchResultType(c, unionResultType, caseBody.Body.Type(), s.GetLocation(), allowMixedVoid)
				if !ok {
					return nil
				}
			}
			if catchAllBody != nil {
				var ok bool
				unionResultType, ok = mergeMatchResultType(c, unionResultType, catchAllBody.Type(), s.GetLocation(), allowMixedVoid)
				if !ok {
					return nil
				}
			}

			// Create and return the UnionMatch
			return &UnionMatch{
				Subject:         subject,
				TypeCases:       typeCases,
				TypeCasesByType: typeCasesByType,
				CatchAll:        catchAllBody,
				ResultType:      unionResultType,
			}
		}

		if resultType, ok := subject.Type().(*Result); ok {
			bindingMutable := c.isMutable(subject)
			if len(s.Cases) > 2 {
				c.addError("Too many cases in match", s.GetLocation())
				return nil
			}

			var okCase *Match
			var errCase *Match
			for _, node := range s.Cases {
				switch p := node.Pattern.(type) {
				case *parse.Identifier:
					{
						switch p.Name {
						case "ok":
							okCase = &Match{
								Pattern: &Identifier{Name: "ok"},
								Body: c.checkMatchArmBlock(node.Body, func() {
									c.scope.add("ok", resultType.Val(), bindingMutable)
								}),
							}
						case "err":
							errCase = &Match{
								Pattern: &Identifier{Name: "err"},
								Body: c.checkMatchArmBlock(node.Body, func() {
									c.scope.add("err", resultType.Err(), bindingMutable)
								}),
							}
						default:
							c.addWarning("Ignored pattern", p.GetLocation())
						}
					}
				case *parse.FunctionCall: // use FunctionCall node as aliasing variable
					{
						varName := p.Args[0].Value.(*parse.Identifier).Name
						switch p.Name {
						case "ok":
							varName := p.Args[0].Value.(*parse.Identifier).Name
							okCase = &Match{
								Pattern: &Identifier{Name: varName},
								Body: c.checkMatchArmBlock(node.Body, func() {
									c.scope.add(varName, resultType.Val(), bindingMutable)
								}),
							}
						case "err":
							errCase = &Match{
								Pattern: &Identifier{Name: varName},
								Body: c.checkMatchArmBlock(node.Body, func() {
									c.scope.add(varName, resultType.Err(), bindingMutable)
								}),
							}
						default:
							c.addWarning("Ignored pattern", p.GetLocation())
						}
					}
				}
			}

			if okCase == nil {
				c.addError("Missing ok case", s.GetLocation())
				return nil
			}
			if errCase == nil {
				c.addError("Missing err case", s.GetLocation())
				return nil
			}

			matchResultType, ok := mergeMatchResultType(c, okCase.Body.Type(), errCase.Body.Type(), s.GetLocation(), allowMixedVoid)
			if !ok {
				return nil
			}
			return &ResultMatch{
				Subject:    subject,
				Ok:         okCase,
				Err:        errCase,
				OkType:     resultType.Val(),
				ErrType:    resultType.Err(),
				ResultType: matchResultType,
			}
		}

		if subject.Type() == Str {
			strCases := make(map[string]*Block)
			var catchAll *Block
			var strResultType Type

			for _, matchCase := range s.Cases {
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok && id.Name == "_" {
					if catchAll != nil {
						c.addError("Duplicate catch-all case", matchCase.Pattern.GetLocation())
						return nil
					}
					catchAll = c.checkMatchArmBlock(matchCase.Body, nil)
					var ok bool
					strResultType, ok = mergeMatchResultType(c, strResultType, catchAll.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}
					continue
				}

				literal, ok := matchCase.Pattern.(*parse.StrLiteral)
				if !ok {
					c.addError("Pattern in Str match must be a string literal or '_'", matchCase.Pattern.GetLocation())
					return nil
				}
				if _, exists := strCases[literal.Value]; exists {
					c.addError(fmt.Sprintf("Duplicate case: %q", literal.Value), matchCase.Pattern.GetLocation())
					return nil
				}
				caseBlock := c.checkMatchArmBlock(matchCase.Body, nil)
				strCases[literal.Value] = caseBlock
				var mergeOK bool
				strResultType, mergeOK = mergeMatchResultType(c, strResultType, caseBlock.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
				if !mergeOK {
					return nil
				}
			}

			if catchAll == nil {
				c.addError("Incomplete match: missing catch-all case for Str match", s.GetLocation())
				return nil
			}

			return &StrMatch{Subject: subject, Cases: strCases, CatchAll: catchAll, ResultType: strResultType}
		}

		if subject.Type() == Rune {
			runeCases := make(map[int]*Block)
			var catchAll *Block
			var runeResultType Type

			for _, matchCase := range s.Cases {
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok && id.Name == "_" {
					if catchAll != nil {
						c.addError("Duplicate catch-all case", matchCase.Pattern.GetLocation())
						return nil
					}
					catchAll = c.checkMatchArmBlock(matchCase.Body, nil)
					var ok bool
					runeResultType, ok = mergeMatchResultType(c, runeResultType, catchAll.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}
					continue
				}

				literal, ok := matchCase.Pattern.(*parse.RuneLiteral)
				if !ok {
					c.addError("Pattern in Rune match must be a rune literal or '_'", matchCase.Pattern.GetLocation())
					return nil
				}
				value, valid := c.parseRuneLiteralValue(literal)
				if !valid {
					return nil
				}
				intValue := int(value)
				if _, exists := runeCases[intValue]; exists {
					c.addError(fmt.Sprintf("Duplicate case: %s", strconv.QuoteRune(value)), matchCase.Pattern.GetLocation())
					return nil
				}
				caseBlock := c.checkMatchArmBlock(matchCase.Body, nil)
				runeCases[intValue] = caseBlock
				var mergeOK bool
				runeResultType, mergeOK = mergeMatchResultType(c, runeResultType, caseBlock.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
				if !mergeOK {
					return nil
				}
			}

			if catchAll == nil {
				c.addError("Incomplete match: missing catch-all case for Rune match", s.GetLocation())
			}

			return &IntMatch{
				Subject:    subject,
				IntCases:   runeCases,
				RangeCases: map[IntRange]*Block{},
				CatchAll:   catchAll,
				ResultType: runeResultType,
			}
		}

		// Check for Int matching
		if subject.Type() == Int {
			intCases := make(map[int]*Block)
			rangeCases := make(map[IntRange]*Block)
			var catchAll *Block
			var intResultType Type

			for _, matchCase := range s.Cases {
				// Check if it's the default case (_)
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok && id.Name == "_" {
					catchAll = c.checkMatchArmBlock(matchCase.Body, nil)
					var ok bool
					intResultType, ok = mergeMatchResultType(c, intResultType, catchAll.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}
				} else if literal, ok := matchCase.Pattern.(*parse.NumLiteral); ok {
					// Convert string to int
					value, err := strconv.Atoi(literal.Value)
					if err != nil {
						c.addError(fmt.Sprintf("Invalid integer literal: %s", literal.Value), matchCase.Pattern.GetLocation())
						return nil
					}
					caseBlock := c.checkMatchArmBlock(matchCase.Body, nil)
					intCases[value] = caseBlock
					var ok bool
					intResultType, ok = mergeMatchResultType(c, intResultType, caseBlock.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}
				} else if unaryExpr, ok := matchCase.Pattern.(*parse.UnaryExpression); ok && unaryExpr.Operator == parse.Minus {
					// Handle negative numbers like -1, -5, etc.
					if literal, ok := unaryExpr.Operand.(*parse.NumLiteral); ok {
						// Convert string to int and negate
						value, err := strconv.Atoi(literal.Value)
						if err != nil {
							c.addError(fmt.Sprintf("Invalid integer literal: %s", literal.Value), literal.GetLocation())
							return nil
						}
						negativeValue := -value
						caseBlock := c.checkMatchArmBlock(matchCase.Body, nil)
						intCases[negativeValue] = caseBlock
						var ok bool
						intResultType, ok = mergeMatchResultType(c, intResultType, caseBlock.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
						if !ok {
							return nil
						}
					} else {
						c.addError(fmt.Sprintf("Invalid pattern for Int match: %T", matchCase.Pattern), matchCase.Pattern.GetLocation())
						return nil
					}
				} else if rangeExpr, ok := matchCase.Pattern.(*parse.RangeExpression); ok {
					// Handle range pattern like 1..10 or -10..5
					startValue, startErr := c.extractIntFromPattern(rangeExpr.Start)
					if startErr != nil {
						c.addError(fmt.Sprintf("Invalid start value in range: %s", startErr.Error()), rangeExpr.Start.GetLocation())
						return nil
					}

					endValue, endErr := c.extractIntFromPattern(rangeExpr.End)
					if endErr != nil {
						c.addError(fmt.Sprintf("Invalid end value in range: %s", endErr.Error()), rangeExpr.End.GetLocation())
						return nil
					}

					if startValue > endValue {
						c.addError("Range start must be less than or equal to end", matchCase.Pattern.GetLocation())
						return nil
					}

					caseBlock := c.checkMatchArmBlock(matchCase.Body, nil)
					rangeCases[IntRange{Start: startValue, End: endValue}] = caseBlock
					var ok bool
					intResultType, ok = mergeMatchResultType(c, intResultType, caseBlock.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}
				} else if staticProp, ok := matchCase.Pattern.(*parse.StaticProperty); ok {
					// Handle enum variant pattern like Status::active
					patternExpr := c.checkExpr(staticProp)
					if patternExpr == nil {
						continue // Error already reported by checkExpr
					}

					// Check if the pattern resolves to an enum variant
					enumVariant, ok := patternExpr.(*EnumVariant)
					if !ok {
						c.addError("Pattern in Int match must be an integer literal, range, or enum variant", staticProp.GetLocation())
						continue
					}

					// Extract the integer value from the enum variant's actual value (supports custom enum values)
					value := enumVariant.enum.Values[enumVariant.Variant].Value
					caseBlock := c.checkMatchArmBlock(matchCase.Body, nil)
					intCases[value] = caseBlock
					var mergeOK bool
					intResultType, mergeOK = mergeMatchResultType(c, intResultType, caseBlock.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
					if !mergeOK {
						return nil
					}
				} else {
					c.addError(fmt.Sprintf("Invalid pattern for Int match: %T", matchCase.Pattern), matchCase.Pattern.GetLocation())
					return nil
				}
			}

			// Validate that there is a catch-all case for Int match
			if catchAll == nil {
				c.addError("Incomplete match: missing catch-all case for Int match", s.GetLocation())
			}

			return &IntMatch{
				Subject:    subject,
				IntCases:   intCases,
				RangeCases: rangeCases,
				CatchAll:   catchAll,
				ResultType: intResultType,
			}
		}

		c.addError(fmt.Sprintf("Cannot match on %s", subject.Type()), s.GetLocation())
		return nil
	case *parse.ConditionalMatchExpression:
		allowMixedVoid := discardThisExpr || c.expectedExpr == Void
		previousArmDiscard := c.matchArmDiscardContext
		c.matchArmDiscardContext = allowMixedVoid
		defer func() {
			c.matchArmDiscardContext = previousArmDiscard
		}()
		var cases []ConditionalCase
		var catchAll *Block
		var conditionalResultType Type

		for _, matchCase := range s.Cases {
			if matchCase.Condition == nil {
				// This is a catch-all case (_)
				if catchAll != nil {
					c.addError("Duplicate catch-all case", matchCase.GetLocation())
				} else {
					catchAll = c.checkMatchArmBlock(matchCase.Body, nil)
					var ok bool
					conditionalResultType, ok = mergeMatchResultType(c, conditionalResultType, catchAll.Type(), matchCase.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}
				}
			} else {
				// Regular condition case
				if condition := c.checkExpr(matchCase.Condition); condition != nil {
					// Ensure condition is boolean
					if condition.Type() != Bool {
						c.addError(fmt.Sprintf("Condition must be of type Bool, got %s", condition.Type().String()), matchCase.Condition.GetLocation())
					}

					body := c.checkMatchArmBlock(matchCase.Body, nil)
					var ok bool
					conditionalResultType, ok = mergeMatchResultType(c, conditionalResultType, body.Type(), matchCase.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}

					cases = append(cases, ConditionalCase{
						Condition: condition,
						Body:      body,
					})
				}
			}
		}

		// Require catch-all case for conditional match to guarantee a return value
		if catchAll == nil {
			c.addError("Conditional match must include a catch-all (_) case", s.GetLocation())
		}

		return &ConditionalMatch{
			Cases:      cases,
			CatchAll:   catchAll,
			ResultType: conditionalResultType,
		}
	case *parse.StaticProperty:
		{
			if id, ok := s.Target.(*parse.Identifier); ok {
				if goPkg := c.program.GoImports[id.Name]; goPkg != nil {
					prop, ok := s.Property.(*parse.Identifier)
					if !ok {
						c.addError(fmt.Sprintf("Unsupported property type in %s::%s", id.Name, s.Property), s.Property.GetLocation())
						return nil
					}
					if typ := goPkg.Constants[prop.Name]; typ != nil {
						return &ForeignValue{Target: "go", Namespace: goPkg.Path, Qualifier: goPkg.TypesName, Symbol: prop.Name, ValueType: typ}
					}
					if reason := goPkg.UnsupportedConstants[prop.Name]; reason != "" {
						c.addError(fmt.Sprintf("Unsupported Go constant %s::%s: %s", id.Name, prop.Name, reason), prop.GetLocation())
						return nil
					}
					c.addError(fmt.Sprintf("Undefined: %s::%s", id.Name, prop.Name), prop.GetLocation())
					return nil
				}

				// Check if this is accessing a module
				if mod := c.resolveModule(id.Name); mod != nil {
					switch prop := s.Property.(type) {
					case *parse.StructInstance:
						// Look up the struct symbol directly from the module
						sym := mod.Get(prop.Name.Name)
						if sym.IsZero() {
							c.addError(fmt.Sprintf("Undefined: %s::%s", id.Name, prop.Name.Name), prop.Name.GetLocation())
							return nil
						}

						structType, ok := sym.Type.(*StructDef)
						if !ok {
							c.addError(fmt.Sprintf("%s::%s is not a struct", id.Name, prop.Name.Name), prop.Name.GetLocation())
							return nil
						}

						// Use helper function for validation
						instance := c.validateStructInstance(structType, prop.Properties, prop.Name.Name, prop.GetLocation())
						if instance == nil {
							return nil
						}

						// Pre-compute field types for the module instance
						fieldTypes := make(map[string]Type)
						maps.Copy(fieldTypes, structType.Fields)

						return &ModuleStructInstance{
							Module:     mod.Path(),
							Property:   instance,
							FieldTypes: fieldTypes,
							StructType: instance._type,
						}
					case *parse.Identifier:
						sym := mod.Get(prop.Name)
						if sym.IsZero() {
							c.addError(fmt.Sprintf("Undefined: %s::%s", id.Name, prop.Name), prop.GetLocation())
							return nil
						}
						return &ModuleSymbol{Module: mod.Path(), Symbol: Symbol{Name: prop.Name, Type: sym.Type}}
					default:
						c.addError(fmt.Sprintf("Unsupported property type in %s::%s", id.Name, prop), s.Property.GetLocation())
						return nil
					}
				}

				// Handle local enum variants or static functions (not from modules)
				sym, ok := c.scope.get(id.Name)
				if !ok {
					c.addError(fmt.Sprintf("Undefined: %s", id.Name), id.GetLocation())
					return nil
				}

				// Check if it's accessing a static function (e.g., Fixture::from_api_entry)
				if propIdent, ok := s.Property.(*parse.Identifier); ok {
					staticFnName := id.Name + "::" + propIdent.Name
					if fnSym, ok := c.scope.get(staticFnName); ok {
						// Found a static function, return it as a function variable
						if fnDef, ok := fnSym.Type.(*FunctionDef); ok {
							return &Variable{Symbol{Name: staticFnName, Type: fnDef}}
						}
					}
				}

				// Check if it's an enum variant
				enum, ok := sym.Type.(*Enum)
				if !ok {
					c.addError(fmt.Sprintf("Undefined: %s::%s", sym.Name, s.Property), id.GetLocation())
					return nil
				}

				variant := -1
				for i := range enum.Values {
					if enum.Values[i].Name == s.Property.(*parse.Identifier).Name {
						variant = i
						break
					}
				}
				if variant == -1 {
					c.addError(fmt.Sprintf("Undefined: %s::%s", sym.Name, s.Property.(*parse.Identifier).Name), id.GetLocation())
					return nil
				}

				return &EnumVariant{
					enum:         enum,
					Variant:      variant,
					EnumType:     enum,
					Discriminant: enum.Values[variant].Value,
				}
			}
			// Handle nested static properties like http::Method::Get
			if _, ok := s.Target.(*parse.StaticProperty); ok {
				// First resolve the nested static property (e.g., http::Method)
				nestedSym := c.checkExpr(s.Target)
				if nestedSym == nil {
					return nil
				}

				// Check if it's an enum type
				if enum, ok := nestedSym.Type().(*Enum); ok {
					// Find the variant
					variant := -1
					for i := range enum.Values {
						if enum.Values[i].Name == s.Property.(*parse.Identifier).Name {
							variant = i
							break
						}
					}
					if variant == -1 {
						c.addError(fmt.Sprintf("Undefined: %s::%s", enum.Name, s.Property.(*parse.Identifier).Name), s.Property.GetLocation())
						return nil
					}

					return &EnumVariant{
						enum:         enum,
						Variant:      variant,
						EnumType:     enum,
						Discriminant: enum.Values[variant].Value,
					}
				}

				c.addError(fmt.Sprintf("Cannot access property on %T", nestedSym.Type()), s.Property.GetLocation())
				return nil
			}
			panic(fmt.Errorf("Unexpected static property target: %T", s.Target))
		}
	case *parse.StructInstance:
		name := s.Name.Name
		sym, ok := c.scope.get(name)
		if !ok {
			c.addError(fmt.Sprintf("Undefined: %s", name), s.GetLocation())
			return nil
		}

		structType, ok := sym.Type.(*StructDef)
		if !ok {
			c.addError(fmt.Sprintf("Undefined: %s", name), s.GetLocation())
			return nil
		}

		// Use helper function for validation
		return c.validateStructInstance(structType, s.Properties, name, s.GetLocation())
	case *parse.Try:
		{
			// Check if this is a property/method accessor chain that might need cascading Maybe handling
			expr := c.tryCheckAccessorChain(s.Expression)
			if expr == nil {
				return nil
			}

			if c.scope.getReturnType() == nil {
				c.addError("The `try` keyword can only be used in a function body", s.GetLocation())
				return nil
			}

			var catchBlock []Statement

			switch _type := expr.Type().(type) {
			case *Result:
				// Handle catch clause if present
				if s.CatchVar != nil && s.CatchBlock != nil {
					// Create new scope for catch block with error variable
					prevScope := c.scope
					newScope := makeScope(prevScope)
					c.scope = &newScope

					// Add error variable to scope with the error type
					c.scope.add(s.CatchVar.Name, _type.err, false)

					// Check catch block statements
					for _, stmt := range s.CatchBlock {
						checkedStmt := c.checkStmt(&stmt)
						if checkedStmt != nil {
							catchBlock = append(catchBlock, *checkedStmt)
						}
					}

					// Restore previous scope
					c.scope = prevScope

					// Create the block
					block := &Block{Stmts: catchBlock}
					blockType := block.Type()
					returnType := c.scope.getReturnType()

					// Validate catch block type compatibility
					// If both are Results, only error types need to match (value types can differ, including generic $Val)
					var typeOk bool
					if fnReturnResult, ok := returnType.(*Result); ok {
						if blockResultType, ok := blockType.(*Result); ok {
							typeOk = fnReturnResult.err.equal(blockResultType.err)
							if !typeOk {
								c.addError(fmt.Sprintf("Error type mismatch: Expected %s, got %s", fnReturnResult.err.String(), blockResultType.err.String()), s.GetLocation())
							}
						} else {
							// Catch block returns non-Result but function expects Result
							typeOk = false
							c.addError(typeMismatch(returnType, blockType), s.GetLocation())
						}
					} else {
						// Function return type is not a Result
						typeOk = returnType.equal(blockType)
						if !typeOk {
							c.addError(typeMismatch(returnType, blockType), s.GetLocation())
						}
					}

					// With catch clause, try returns the unwrapped value type on success
					// On error, it early returns with the catch block result
					return &TryOp{
						expr:       expr,
						ok:         _type.val, // Returns unwrapped value for continued execution
						OkType:     _type.val,
						ErrType:    _type.err,
						CatchBlock: block,
						CatchVar:   s.CatchVar.Name,
						Kind:       TryResult,
					}
				} else {
					// No catch clause: function must return a compatible Result type
					fnReturnResult, ok := c.scope.getReturnType().(*Result)
					if !ok {
						c.addError("try without catch clause requires function to return a Result type", s.GetLocation())
						// Return a try op with the unwrapped type to avoid cascading errors
						return &TryOp{
							expr:    expr,
							ok:      _type.val,
							OkType:  _type.val,
							ErrType: _type.err,
							Kind:    TryResult,
						}
					}

					// Error types must match for direct propagation
					if !_type.err.equal(fnReturnResult.err) {
						c.addError(fmt.Sprintf("Error type mismatch: Expected %s, got %s", fnReturnResult.err.String(), _type.err.String()), s.Expression.GetLocation())
						// Return a try op with the unwrapped type to avoid cascading errors
						return &TryOp{
							expr:    expr,
							ok:      _type.val,
							OkType:  _type.val,
							ErrType: _type.err,
							Kind:    TryResult,
						}
					}

					// Success: returns the unwrapped value
					// Error: early returns the error wrapped in the function's Result type
					return &TryOp{
						expr:    expr,
						ok:      _type.val,
						OkType:  _type.val,
						ErrType: _type.err,
						Kind:    TryResult,
					}
				}
			case *Maybe:
				// Handle catch clause if present
				if s.CatchVar != nil && s.CatchBlock != nil {
					// Create new scope for catch block - no variable binding for Maybe catch
					prevScope := c.scope
					newScope := makeScope(prevScope)
					c.scope = &newScope

					// Check catch block statements
					for _, stmt := range s.CatchBlock {
						checkedStmt := c.checkStmt(&stmt)
						if checkedStmt != nil {
							catchBlock = append(catchBlock, *checkedStmt)
						}
					}

					// Restore previous scope
					c.scope = prevScope

					// Create the block
					block := &Block{Stmts: catchBlock}
					blockType := block.Type()
					returnType := c.scope.getReturnType()

					// Validate catch block type compatibility
					// For Maybe catch blocks, inner types must match (or both have unresolved generics)
					var typeOk bool
					if fnReturnMaybe, ok := returnType.(*Maybe); ok {
						if blockMaybeType, ok := blockType.(*Maybe); ok {
							// Both are Maybe types - inner types should match
							typeOk = fnReturnMaybe.of.equal(blockMaybeType.of)
							if !typeOk {
								c.addError(typeMismatch(returnType, blockType), s.GetLocation())
							}
						} else {
							typeOk = false
							c.addError(typeMismatch(returnType, blockType), s.GetLocation())
						}
					} else {
						// Function return type is not a Maybe
						typeOk = returnType.equal(blockType)
						if !typeOk {
							c.addError(typeMismatch(returnType, blockType), s.GetLocation())
						}
					}

					// With catch clause, try returns the unwrapped value type on success
					// On none, it early returns with the catch block result
					return &TryOp{
						expr:       expr,
						ok:         _type.of, // Returns unwrapped value for continued execution
						OkType:     _type.of,
						CatchBlock: block,
						CatchVar:   "", // No variable binding for Maybe catch
						Kind:       TryMaybe,
					}
				} else {
					// No catch clause: function must return a compatible Maybe type
					fnReturnMaybe, ok := c.scope.getReturnType().(*Maybe)
					if !ok {
						c.addError("try without catch clause on Maybe requires function to return a Maybe type", s.GetLocation())
						// Return a try op with the unwrapped type to avoid cascading errors
						return &TryOp{
							expr:   expr,
							ok:     _type.of,
							OkType: _type.of,
							Kind:   TryMaybe,
						}
					}

					// When try fails on Maybe, it early returns none wrapped in the function's Maybe type
					// The inner types don't need to match since we're not directly returning the unwrapped value
					// The unwrapped value (on success) can be any type and will be used in subsequent computations
					_ = fnReturnMaybe // We just need to verify the function returns a Maybe type

					// Success: returns the unwrapped value
					// None: early returns none wrapped in the function's Maybe type
					return &TryOp{
						expr:   expr,
						ok:     _type.of,
						OkType: _type.of,
						Kind:   TryMaybe,
					}
				}
			default:
				c.addError("try can only be used on Result or Maybe types, got: "+expr.Type().String(), s.Expression.GetLocation())
				// Return a try op with the expr type to avoid cascading errors
				return &TryOp{
					expr:    expr,
					ok:      expr.Type(),
					OkType:  expr.Type(),
					ErrType: Void,
					Kind:    TryResult, // Default to Result, though this is an error path
				}
			}
		}
	case *parse.UnsafeBlock:
		{
			if parseStatementsContainBreak(s.Statements) {
				c.addError("break is not allowed inside unsafe blocks", s.GetLocation())
			}
			var expectedValue Type
			if expectedResult, ok := c.expectedExpr.(*Result); ok {
				expectedValue = expectedResult.val
			}
			unsafeReturnType := MakeResult(&TypeVar{name: "Unsafe"}, Str)
			var block *Block
			if expectedValue != nil {
				block = c.checkBlockWithExpected(s.Statements, func() {
					c.scope.expectReturn(unsafeReturnType)
				}, expectedValue, false)
			} else {
				block = c.checkBlockWithInferredFinalValue(s.Statements, func() {
					c.scope.expectReturn(unsafeReturnType)
				}, discardThisExpr || c.expectedExpr == Void)
			}
			valueType := block.Type()
			resultType := MakeResult(valueType, Str)
			c.validateUnsafeCatchResults(block, resultType, s.GetLocation())
			return &UnsafeBlock{Body: block, ValueType: valueType, ResultType: resultType}
		}
	case *parse.BlockExpression:
		{
			block := c.checkBlockWithInferredFinalValue(s.Statements, nil, discardThisExpr || c.expectedExpr == Void)
			return block
		}
	default:
		panic(fmt.Errorf("Unexpected expression: %s", reflect.TypeOf(s)))
	}
}

func (c *Checker) parseRuneLiteralValue(literal *parse.RuneLiteral) (rune, bool) {
	runes := []rune(literal.Value)
	if len(runes) != 1 || !utf8.ValidRune(runes[0]) {
		c.addError("Rune literal must contain exactly one Unicode scalar value", literal.GetLocation())
		return 0, false
	}
	return runes[0], true
}

// extractIntFromPattern extracts an integer value from a pattern that can be either
// a NumLiteral or a UnaryExpression with minus operator applied to a NumLiteral
func (c *Checker) extractIntFromPattern(expr parse.Expression) (int, error) {
	switch e := expr.(type) {
	case *parse.NumLiteral:
		return strconv.Atoi(e.Value)
	case *parse.UnaryExpression:
		if e.Operator == parse.Minus {
			if literal, ok := e.Operand.(*parse.NumLiteral); ok {
				value, err := strconv.Atoi(literal.Value)
				if err != nil {
					return 0, err
				}
				return -value, nil
			}
		}
		return 0, fmt.Errorf("unsupported unary expression in pattern")
	default:
		return 0, fmt.Errorf("pattern must be an integer literal or negative integer")
	}
}

// isEnum checks if a type is an Enum
func (c *Checker) isEnum(t Type) bool {
	_, ok := t.(*Enum)
	return ok
}

// areTypesComparable checks if two types can be compared together
// This allows Enum vs Int and Int vs Enum comparisons
func (c *Checker) areTypesComparable(left, right Type) bool {
	// Same type is always comparable
	if left.equal(right) {
		return true
	}
	// Allow Enum vs Int comparisons
	leftIsEnum := c.isEnum(left)
	rightIsEnum := c.isEnum(right)

	if (leftIsEnum && right == Int) || (left == Int && rightIsEnum) {
		return true
	}

	return false
}

// use this when we know what the expr's Type should be
func bindInferredTypeVars(expected Type, actual Type) {
	if expected == nil || actual == nil {
		return
	}

	expected = derefType(expected)
	actual = derefType(actual)

	switch exp := expected.(type) {
	case *TypeVar:
		if exp.actual == nil {
			exp.actual = actual
			exp.bound = true
			return
		}
		bindInferredTypeVars(exp.actual, actual)
	case *Maybe:
		if act, ok := actual.(*Maybe); ok {
			bindInferredTypeVars(exp.Of(), act.Of())
		}
	case *Result:
		if act, ok := actual.(*Result); ok {
			bindInferredTypeVars(exp.Val(), act.Val())
			bindInferredTypeVars(exp.Err(), act.Err())
		}
	case *List:
		if act, ok := actual.(*List); ok {
			bindInferredTypeVars(exp.Of(), act.Of())
		}
	case *Chan:
		if act, ok := actual.(*Chan); ok {
			bindInferredTypeVars(exp.Of(), act.Of())
		}
	case *Receiver:
		if act, ok := actual.(*Receiver); ok {
			bindInferredTypeVars(exp.Of(), act.Of())
		}
	case *Sender:
		if act, ok := actual.(*Sender); ok {
			bindInferredTypeVars(exp.Of(), act.Of())
		}
	case *Map:
		if act, ok := actual.(*Map); ok {
			bindInferredTypeVars(exp.Key(), act.Key())
			bindInferredTypeVars(exp.Value(), act.Value())
		}
	case *StructDef:
		if act, ok := actual.(*StructDef); ok && exp.Name == act.Name && !namedTypeOwnersDiffer(exp.ModulePath, act.ModulePath) {
			limit := len(exp.TypeArgs)
			if len(act.TypeArgs) < limit {
				limit = len(act.TypeArgs)
			}
			for i := 0; i < limit; i++ {
				bindInferredTypeVars(exp.TypeArgs[i], act.TypeArgs[i])
			}
			for fieldName, expectedField := range exp.Fields {
				if actualField, ok := act.Fields[fieldName]; ok {
					bindInferredTypeVars(expectedField, actualField)
				}
			}
		}
	case *FunctionDef:
		if act, ok := actual.(*FunctionDef); ok {
			limit := len(exp.Parameters)
			if len(act.Parameters) < limit {
				limit = len(act.Parameters)
			}
			for i := 0; i < limit; i++ {
				bindInferredTypeVars(exp.Parameters[i].Type, act.Parameters[i].Type)
			}
			bindInferredTypeVars(exp.ReturnType, act.ReturnType)
		}
	}
}

func (c *Checker) checkNumericLiteralAs(expr parse.Expression, expected Type) Expression {
	if unary, ok := expr.(*parse.UnaryExpression); ok && unary.Operator == parse.Minus {
		if num, ok := unary.Operand.(*parse.NumLiteral); ok {
			return c.checkSignedNumericLiteralAs(num, expected, true)
		}
	}
	num, ok := expr.(*parse.NumLiteral)
	if !ok || expected == nil {
		return nil
	}
	return c.checkSignedNumericLiteralAs(num, expected, false)
}

func (c *Checker) checkSignedNumericLiteralAs(num *parse.NumLiteral, expected Type, negative bool) Expression {
	literalType := expected
	if foreign, ok := expected.(*ForeignType); ok {
		literalType = foreign.Underlying
	}
	if expected == nil || (!isIntegerScalar(literalType) && literalType != Float32 && literalType != Float64) {
		return nil
	}
	literalText := num.Value
	if negative {
		literalText = "-" + literalText
	}
	if strings.Contains(num.Value, ".") {
		clean := strings.ReplaceAll(literalText, "_", "")
		value, err := strconv.ParseFloat(clean, 64)
		if err != nil {
			c.addError(fmt.Sprintf("Invalid float: %s", num.Value), num.GetLocation())
			return nil
		}
		if literalType == Float64 {
			if expected != literalType {
				return &TypedFloatLiteral{Value: value, Text: clean, Typed: expected}
			}
			return &FloatLiteral{Value: value}
		}
		if literalType == Float32 {
			float32Value, err := strconv.ParseFloat(clean, 32)
			if err != nil {
				c.addError(fmt.Sprintf("Float literal %s overflows Float32", num.Value), num.GetLocation())
				return &TypedFloatLiteral{Value: float32Value, Text: clean, Typed: expected}
			}
			return &TypedFloatLiteral{Value: float32Value, Text: clean, Typed: expected}
		}
		return nil
	}
	clean := strings.ReplaceAll(literalText, "_", "")
	if isUnsignedScalar(literalType) {
		value := new(big.Int)
		if _, ok := value.SetString(clean, 0); !ok || value.Sign() < 0 || !c.uintLiteralFitsType(value, literalType) {
			c.addError(fmt.Sprintf("Integer literal %s overflows %s", literalText, expected), num.GetLocation())
		}
		return &TypedIntLiteral{Value: int(value.Int64()), Text: clean, Typed: expected}
	}
	if literalType == Float32 || literalType == Float64 {
		return nil
	}
	value64, err := strconv.ParseInt(clean, 0, 64)
	if err != nil {
		c.addError(fmt.Sprintf("Invalid int: %s", literalText), num.GetLocation())
		return nil
	}
	if !c.intLiteralFitsType(value64, literalType) {
		c.addError(fmt.Sprintf("Integer literal %s overflows %s", literalText, expected), num.GetLocation())
		if isIntegerScalar(literalType) {
			return &TypedIntLiteral{Value: int(value64), Text: clean, Typed: expected}
		}
		return nil
	}
	if expected == Int {
		return &IntLiteral{Value: int(value64)}
	}
	if isIntegerScalar(literalType) {
		return &TypedIntLiteral{Value: int(value64), Text: clean, Typed: expected}
	}
	return nil
}

func isExplicitScalar(t Type) bool {
	switch t {
	case Int8, Int16, Int32, Int64, Uint, Uint8, Uint16, Uint32, Uint64, Uintptr, Float32:
		return true
	default:
		return false
	}
}

func isIntegerScalar(t Type) bool {
	switch t {
	case Int, Int8, Int16, Int32, Int64, Uint, Uint8, Uint16, Uint32, Uint64, Uintptr, Byte, Rune:
		return true
	default:
		return false
	}
}

func isRelationalIntegerLike(t Type) bool { return isIntegerScalar(t) }

func isRelationalFloatLike(t Type) bool { return t == Float64 || t == Float32 }

func isArithmeticIntegerLike(t Type) bool { return isIntegerScalar(t) }

func isSignedArithmeticLike(t Type) bool {
	switch t {
	case Int, Int8, Int16, Int32, Int64, Float32, Float64:
		return true
	default:
		return false
	}
}

func isArithmeticFloatLike(t Type) bool { return isRelationalFloatLike(t) }

func isUnsignedScalar(t Type) bool {
	switch t {
	case Uint, Uint8, Uint16, Uint32, Uint64, Uintptr:
		return true
	default:
		return false
	}
}

func (c *Checker) targetIntBits() int {
	if c.options.Target.IntBits != 0 {
		return c.options.Target.IntBits
	}
	return strconv.IntSize
}

func (c *Checker) targetUintBits() int {
	if c.options.Target.UintBits != 0 {
		return c.options.Target.UintBits
	}
	return c.targetIntBits()
}

func (c *Checker) targetUintptrBits() int {
	if c.options.Target.UintptrBits != 0 {
		return c.options.Target.UintptrBits
	}
	return c.targetIntBits()
}

func unsignedMax(bits int) *big.Int {
	if bits <= 0 {
		bits = strconv.IntSize
	}
	return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(bits)), big.NewInt(1))
}

func (c *Checker) uintLiteralFitsType(value *big.Int, t Type) bool {
	if value.Sign() < 0 {
		return false
	}
	switch t {
	case Uint:
		return value.Cmp(unsignedMax(c.targetUintBits())) <= 0
	case Uintptr:
		return value.Cmp(unsignedMax(c.targetUintptrBits())) <= 0
	case Uint64:
		return value.Cmp(unsignedMax(64)) <= 0
	case Uint8:
		return value.Cmp(big.NewInt(math.MaxUint8)) <= 0
	case Uint16:
		return value.Cmp(big.NewInt(math.MaxUint16)) <= 0
	case Uint32:
		return value.Cmp(big.NewInt(math.MaxUint32)) <= 0
	default:
		return false
	}
}

func (c *Checker) intLiteralFitsType(value int64, t Type) bool {
	switch t {
	case Int:
		bits := c.targetIntBits()
		if bits <= 0 {
			bits = strconv.IntSize
		}
		if bits >= 64 {
			return true
		}
		min := -(int64(1) << (bits - 1))
		max := (int64(1) << (bits - 1)) - 1
		return value >= min && value <= max
	case Int64:
		return true
	case Int8:
		return value >= math.MinInt8 && value <= math.MaxInt8
	case Int16:
		return value >= math.MinInt16 && value <= math.MaxInt16
	case Int32, Rune:
		return value >= math.MinInt32 && value <= math.MaxInt32
	case Uint, Uint64, Uintptr:
		return value >= 0
	case Uint8, Byte:
		return value >= 0 && value <= math.MaxUint8
	case Uint16:
		return value >= 0 && value <= math.MaxUint16
	case Uint32:
		return value >= 0 && value <= math.MaxUint32
	default:
		return false
	}
}

func (c *Checker) checkExprAs(expr parse.Expression, expectedType Type) Expression {
	if literal := c.checkNumericLiteralAs(expr, expectedType); literal != nil {
		return literal
	}
	switch s := (expr).(type) {
	case *parse.MatchExpression:
		return c.withExpectedExpr(expectedType, func() Expression {
			return c.checkExpr(s)
		})
	case *parse.SelectExpression:
		return c.withExpectedExpr(expectedType, func() Expression {
			return c.checkExpr(s)
		})
	case *parse.IfStatement:
		return c.withExpectedExpr(expectedType, func() Expression {
			return c.checkExpr(s)
		})
	case *parse.ConditionalMatchExpression:
		return c.withExpectedExpr(expectedType, func() Expression {
			return c.checkExpr(s)
		})
	case *parse.BlockExpression:
		return c.withExpectedExpr(expectedType, func() Expression {
			return c.checkExpr(s)
		})
	case *parse.UnsafeBlock:
		return c.withExpectedExpr(expectedType, func() Expression {
			return c.checkExpr(s)
		})
	case *parse.ListLiteral:
		// Only use collection-specific inference when the expected type is a list.
		if _, ok := expectedType.(*List); ok {
			if result := c.checkList(expectedType, s); result != nil {
				return result
			}
			return nil
		}
	case *parse.MapLiteral:
		// Only use collection-specific inference when the expected type is a map.
		if _, ok := expectedType.(*Map); ok {
			if result := c.checkMap(expectedType, s); result != nil {
				return result
			}
			return nil
		}
	case *parse.AnonymousFunction:
		{
			// Try to infer types from the expected type
			expectedFnType, ok := expectedType.(*FunctionDef)
			if !ok || expectedFnType.Name != "<function>" {
				// Not a function type (or not a type signature), check normally
				return c.checkExpr(s)
			}

			// Check parameter count
			if len(s.Parameters) != len(expectedFnType.Parameters) {
				c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d",
					len(expectedFnType.Parameters), len(s.Parameters)), s.GetLocation())
				return nil
			}

			// Resolve parameters with type inference from expected function
			params := c.resolveParametersWithContext(s.Parameters, expectedFnType)
			returnType := c.resolveReturnTypeWithContext(s.ReturnType, expectedFnType)

			// Create function definition
			uniqueName := fmt.Sprintf("anon_func_%p", s)
			fn := &FunctionDef{
				Name:                    uniqueName,
				Parameters:              params,
				ReturnType:              returnType,
				InferReturnTypeFromBody: false,
				Body:                    nil,
			}

			// Check body
			c.pushFunctionGenericContext(fn)
			body := c.checkBlockWithExpected(s.Body, func() {
				c.scope.expectReturn(returnType)
				for _, param := range params {
					c.scope.add(param.Name, param.Type, param.Mutable)
				}
			}, returnType, true)
			c.popFunctionGenericContext()

			// Add function to scope after checking body
			c.scope.add(uniqueName, fn, false)

			// Nuance: in context-inferred callbacks (map/map_err/and_then, etc), the expected return
			// type can contain unbound generics (for example Result<$Mapped, E>). If these remain
			// unresolved, later method/property lookup can panic with
			// "Cannot look up symbols in unrefined $T". Bind generics from the checked body type here.
			//
			// This is a shared anonymous-function inference path, so changes here affect closure typing
			// beyond Result/Maybe combinators.
			bindInferredTypeVars(returnType, body.Type())

			// Validate return type
			if returnType != Void && !c.areCompatible(returnType, body.Type()) {
				c.addError(typeMismatch(returnType, body.Type()), s.GetLocation())
				return nil
			}

			fn.Body = body
			return fn
		}
	case *parse.InstanceMethod:
		{
			subj := c.checkExpr(s.Target)
			if subj == nil {
				c.addError(fmt.Sprintf("Cannot access %s on Void", s.Method.Name), s.Method.GetLocation())
				return nil
			}
		}
	case *parse.StaticFunction:
		{
			resultType, expectResult := expectedType.(*Result)
			target, ok := s.Target.(*parse.Identifier)
			if !expectResult || resultType == nil || !ok || target.Name != "Result" || (s.Function.Name != "ok" && s.Function.Name != "err") {
				break
			}

			moduleName := target.Name
			mod := c.resolveModule(moduleName)
			if mod == nil {
				c.addError(fmt.Sprintf("Undefined: %s", moduleName), s.GetLocation())
				return nil
			}

			sym := mod.Get(s.Function.Name)
			if sym.IsZero() {
				c.addError(fmt.Sprintf("Undefined: %s::%s", moduleName, s.Function.Name), s.GetLocation())
				return nil
			}

			// Handle both regular functions and external functions
			var fnDef *FunctionDef
			var isFunc bool
			switch fn := sym.Type.(type) {
			case *FunctionDef:
				fnDef = fn
				isFunc = true
			default:
				isFunc = false
			}

			if !isFunc {
				c.addError(fmt.Sprintf("%s::%s is not a function", moduleName, s.Function.Name), s.GetLocation())
				return nil
			}

			if len(s.Function.Args) != len(fnDef.Parameters) {
				c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d",
					len(fnDef.Parameters), len(s.Function.Args)), s.GetLocation())
				return nil
			}

			var arg Expression = nil
			if fnDef.name() == "ok" {
				arg = c.checkExpr(s.Function.Args[0].Value)
				if arg == nil {
					return nil
				}
				if !resultType.Val().equal(arg.Type()) {
					c.addError(typeMismatch(resultType.Val(), arg.Type()), s.Function.Args[0].Value.GetLocation())
					return nil
				}
				bindInferredTypeVars(resultType.Val(), arg.Type())
			}
			if fnDef.name() == "err" {
				arg = c.checkExpr(s.Function.Args[0].Value)
				if arg == nil {
					return nil
				}
				if !resultType.Err().equal(arg.Type()) {
					c.addError(typeMismatch(resultType.Err(), arg.Type()), s.Function.Args[0].Value.GetLocation())
					return nil
				}
				bindInferredTypeVars(resultType.Err(), arg.Type())
			}

			fnDef.ReturnType = resultType
			return &ModuleFunctionCall{
				Module: mod.Path(),
				Call: &FunctionCall{
					Name:       fnDef.name(),
					Args:       []Expression{arg},
					fn:         fnDef,
					ReturnType: fnDef.ReturnType,
				},
			}
		}
	}

	checked := c.checkExpr(expr)
	if checked == nil {
		return nil
	}

	if expectedType == Void {
		return checked
	}

	if !c.areCompatible(expectedType, checked.Type()) {
		c.addError(typeMismatch(expectedType, checked.Type()), expr.GetLocation())
		return nil
	}

	return checked
}

func (c *Checker) resolveParametersWithContext(params []parse.Parameter, expectedFnType *FunctionDef) []Parameter {
	result := make([]Parameter, len(params))
	for i, param := range params {
		var paramType Type = Void

		if param.Type != nil {
			// Explicit type provided
			paramType = c.resolveType(param.Type)
		} else if expectedFnType != nil && i < len(expectedFnType.Parameters) {
			// Infer from expected function type
			paramType = expectedFnType.Parameters[i].Type
		}
		// Otherwise defaults to Void

		result[i] = Parameter{
			Name:    param.Name,
			Type:    paramType,
			Mutable: param.Mutable,
		}
	}
	return result
}

// resolveReturnTypeWithContext resolves return type, optionally inferring from an expected function type
func (c *Checker) resolveReturnTypeWithContext(returnTypeNode parse.DeclaredType, expectedFnType *FunctionDef) Type {
	if returnTypeNode != nil {
		// Explicit type provided
		return c.resolveType(returnTypeNode)
	} else if expectedFnType != nil {
		// Infer from expected function type
		return expectedFnType.ReturnType
	}
	// Default to Void
	return Void
}

// checkFunctionBody validates the function body and returns the checked block
func (c *Checker) checkFunctionBody(fn *FunctionDef, bodyStmts []parse.Statement, params []Parameter, returnType Type, location parse.Location) *Block {
	// Add function to scope BEFORE checking body
	c.scope.add(fn.Name, fn, false)

	previousDiscard := c.discardExprContext
	c.discardExprContext = false
	defer func() {
		c.discardExprContext = previousDiscard
	}()

	// Check function body
	c.pushFunctionGenericContext(fn)
	body := c.checkBlockWithExpected(bodyStmts, func() {
		// Set the expected return type to the scope
		c.scope.expectReturn(returnType)
		// Add parameters to scope
		for _, param := range params {
			c.scope.add(param.Name, param.Type, param.Mutable)
		}
	}, returnType, true)
	c.popFunctionGenericContext()

	// Check that the function's return type matches its body's type
	if returnType != Void && !c.areCompatible(returnType, body.Type()) {
		c.addError(typeMismatch(returnType, body.Type()), location)
	}

	return body
}

func (c *Checker) checkFunction(def *parse.FunctionDeclaration, init func(), extraGenericParams ...string) *FunctionDef {
	if init != nil {
		init()
	}

	// Resolve parameters and return type
	params := c.resolveParametersWithContext(def.Parameters, nil)
	returnType := c.resolveReturnTypeWithContext(def.ReturnType, nil)

	// Validate parameters resolved correctly (for named functions, types must be explicit)
	for i, param := range def.Parameters {
		if param.Type != nil && params[i].Type == nil {
			panic(fmt.Errorf("Cannot resolve type for parameter %s", param.Name))
		}
	}

	// Create function definition
	fn := &FunctionDef{
		Name:          def.Name,
		GenericParams: append([]string(nil), def.TypeParams...),
		Parameters:    params,
		ReturnType:    returnType,
		Body:          nil,
		Private:       def.Private,
		IsTest:        def.IsTest,
	}

	if def.IsTest {
		if init != nil {
			c.addError("test functions must be top-level declarations", def.GetLocation())
		}
		if len(def.Parameters) > 0 {
			c.addError("test functions must not take parameters", def.GetLocation())
		}
		if len(def.TypeParams) > 0 {
			c.addError("test functions must not be generic", def.GetLocation())
		}
		expectedReturnType := MakeResult(Void, Str)
		if !returnType.equal(expectedReturnType) {
			c.addError("test functions must return Void!Str", def.GetLocation())
		}
	}

	// Add function to scope before checking body (for recursion support)
	// For methods (when init != nil), only add within the body scope
	if init == nil {
		c.scope.add(def.Name, fn, false)
	}

	c.pushFunctionGenericContext(fn, extraGenericParams...)
	body := c.checkBlockWithExpected(def.Body, func() {
		c.scope.expectReturn(returnType)
		for _, param := range params {
			c.scope.add(param.Name, param.Type, param.Mutable)
		}
	}, returnType, true)
	c.popFunctionGenericContext()

	// Validate return type
	if returnType != Void && !c.areCompatible(returnType, body.Type()) {
		c.addError(typeMismatch(returnType, body.Type()), def.GetLocation())
	}

	fn.Body = body
	return fn
}

// Substitute generic parameters in a type
func substituteType(t Type, typeMap map[string]Type) Type {
	switch typ := t.(type) {
	case *TypeVar:
		if concrete, exists := typeMap[typ.name]; exists {
			return concrete
			// typ.actual = concrete
		}
		return typ
	case *Maybe:
		return &Maybe{of: substituteType(typ.of, typeMap)}
	case *Result:
		return MakeResult(
			substituteType(typ.val, typeMap),
			substituteType(typ.err, typeMap),
		)
	case *List:
		return &List{of: substituteType(typ.of, typeMap)}
	case *Chan:
		return &Chan{of: substituteType(typ.of, typeMap)}
	case *Receiver:
		return &Receiver{of: substituteType(typ.of, typeMap)}
	case *Sender:
		return &Sender{of: substituteType(typ.of, typeMap)}
	case *Map:
		return &Map{key: substituteType(typ.key, typeMap), value: substituteType(typ.value, typeMap)}
	case *Union:
		types := make([]Type, len(typ.Types))
		for i, member := range typ.Types {
			types[i] = substituteType(member, typeMap)
		}
		return &Union{Name: typ.Name, ModulePath: typ.ModulePath, Types: types, Private: typ.Private}
	case *StructDef:
		var out Type = typ
		for genericName, concrete := range typeMap {
			out = replaceGeneric(out, genericName, concrete)
		}
		return out
	case *FunctionDef:
		// Substitute generics in function parameters and return type
		substitutedParams := make([]Parameter, len(typ.Parameters))
		for i, param := range typ.Parameters {
			substitutedParams[i] = Parameter{
				Name:    param.Name,
				Type:    substituteType(param.Type, typeMap),
				Mutable: param.Mutable,
			}
		}
		return &FunctionDef{
			Name:                    typ.Name,
			GenericParams:           append([]string(nil), typ.GenericParams...),
			Parameters:              substitutedParams,
			ReturnType:              substituteType(typ.ReturnType, typeMap),
			InferReturnTypeFromBody: typ.InferReturnTypeFromBody,
			Body:                    typ.Body,
			Mutates:                 typ.Mutates,
			IsTest:                  typ.IsTest,
			Private:                 typ.Private,
			GenericBindings:         cloneTypeMap(typ.GenericBindings),
		}
	// Handle other compound types
	default:
		return t
	}
}

func cloneTypeMap(in map[string]Type) map[string]Type {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]Type, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func concreteTypeVarBindings(in map[string]*TypeVar) map[string]Type {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]Type, len(in))
	for key, value := range in {
		if value != nil && value.Actual() != nil {
			out[key] = value.Actual()
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func inferGenericBindingsFromFunction(original, specialized *FunctionDef) map[string]Type {
	if original == nil || specialized == nil {
		return nil
	}
	bindings := map[string]Type{}
	for i, param := range original.Parameters {
		if i < len(specialized.Parameters) {
			inferGenericBindingsFromTypes(param.Type, specialized.Parameters[i].Type, bindings)
		}
	}
	inferGenericBindingsFromTypes(original.ReturnType, specialized.ReturnType, bindings)
	if len(bindings) == 0 {
		return nil
	}
	return bindings
}

func inferGenericBindingsFromTypes(original, specialized Type, bindings map[string]Type) {
	specialized = derefType(specialized)
	switch orig := original.(type) {
	case *TypeVar:
		if specialized != nil && !hasGenericsInType(specialized) {
			bindings[orig.Name()] = specialized
		}
	case *Maybe:
		if spec, ok := specialized.(*Maybe); ok {
			inferGenericBindingsFromTypes(orig.Of(), spec.Of(), bindings)
		}
	case *Result:
		if spec, ok := specialized.(*Result); ok {
			inferGenericBindingsFromTypes(orig.Val(), spec.Val(), bindings)
			inferGenericBindingsFromTypes(orig.Err(), spec.Err(), bindings)
		}
	case *Chan:
		if spec, ok := specialized.(*Chan); ok {
			inferGenericBindingsFromTypes(orig.Of(), spec.Of(), bindings)
		}
	case *Receiver:
		if spec, ok := specialized.(*Receiver); ok {
			inferGenericBindingsFromTypes(orig.Of(), spec.Of(), bindings)
		}
	case *Sender:
		if spec, ok := specialized.(*Sender); ok {
			inferGenericBindingsFromTypes(orig.Of(), spec.Of(), bindings)
		}
	case *List:
		if spec, ok := specialized.(*List); ok {
			inferGenericBindingsFromTypes(orig.Of(), spec.Of(), bindings)
		}
	case *Map:
		if spec, ok := specialized.(*Map); ok {
			inferGenericBindingsFromTypes(orig.Key(), spec.Key(), bindings)
			inferGenericBindingsFromTypes(orig.Value(), spec.Value(), bindings)
		}
	case *StructDef:
		if spec, ok := specialized.(*StructDef); ok && orig.Name == spec.Name && !namedTypeOwnersDiffer(orig.ModulePath, spec.ModulePath) {
			for i, typeArg := range orig.TypeArgs {
				if i < len(spec.TypeArgs) {
					inferGenericBindingsFromTypes(typeArg, spec.TypeArgs[i], bindings)
				}
			}
			for fieldName, fieldType := range orig.Fields {
				if specializedField, ok := spec.Fields[fieldName]; ok {
					inferGenericBindingsFromTypes(fieldType, specializedField, bindings)
				}
			}
		}
	case *FunctionDef:
		if spec, ok := specialized.(*FunctionDef); ok {
			for i, param := range orig.Parameters {
				if i < len(spec.Parameters) {
					inferGenericBindingsFromTypes(param.Type, spec.Parameters[i].Type, bindings)
				}
			}
			inferGenericBindingsFromTypes(orig.ReturnType, spec.ReturnType, bindings)
		}
	}
}

// setupFunctionGenerics sets up generic scope and function copy for generic functions.
// Returns the function copy (with fresh TypeVar instances for generics) and the generic scope.
// For non-generic functions, returns the original function and a nil scope.
func (c *Checker) setupFunctionGenerics(fnDef *FunctionDef) (*FunctionDef, *SymbolTable) {
	if !fnDef.hasGenerics() {
		return fnDef, nil
	}

	genericParams := genericParamsForFunction(fnDef)

	// Create generic scope and fresh function copy
	genericScope := c.scope.createGenericScope(genericParams)
	fnDefCopy := copyFunctionWithTypeVarMap(fnDef, *genericScope.genericContext)

	return fnDefCopy, genericScope
}

// synthesizeMaybeNone creates a synthetic maybe::none() call for an omitted nullable argument.
// This transforms the omitted argument into an explicit function call, allowing backends
// to treat all arguments uniformly without special OmittedArg handling.
func (c *Checker) synthesizeMaybeNone(paramType Type) Expression {
	// paramType should be a Maybe type for omitted arguments
	_, ok := paramType.(*Maybe)
	if !ok {
		// Defensive: if somehow we got here with non-Maybe, return a VoidLiteral
		// (this shouldn't happen due to validation earlier)
		return &VoidLiteral{}
	}

	// Create a module function call: maybe::none()
	// The return type of maybe::none() depends on its context, which will be the Maybe type
	return &ModuleFunctionCall{
		Module: "ard/maybe",
		Call: &FunctionCall{
			Name: "none",
			Args: []Expression{},
			fn: &FunctionDef{
				Name:       "none",
				Parameters: []Parameter{},
				ReturnType: paramType, // The return type is the Maybe type we're filling in
				Body:       nil,       // No body for synthesized calls
			},
			ReturnType: paramType,
		},
	}
}

// synthesizeMaybeSome wraps a value in maybe::some() for automatic coercion of T to Maybe<T>.
// This allows calling functions with nullable parameters using unwrapped values:
// instead of add(1, maybe::some(5)), you can write add(1, 5).
func (c *Checker) synthesizeMaybeSome(value Expression, maybeType Type) Expression {
	return &ModuleFunctionCall{
		Module: "ard/maybe",
		Call: &FunctionCall{
			Name: "some",
			Args: []Expression{value},
			fn: &FunctionDef{
				Name: "some",
				Parameters: []Parameter{
					{
						Name:    "value",
						Type:    value.Type(),
						Mutable: false,
					},
				},
				ReturnType: maybeType,
				Body:       nil, // No body for synthesized calls
			},
			ReturnType: maybeType,
		},
	}
}

// checkAndProcessArguments validates and type-checks function arguments with generic support.
// Returns the processed arguments and the specialized function (with generics resolved if applicable).
// Mutable parameters require addressable mutable arguments.
// Synthesizes maybe::none() calls for omitted nullable arguments.
// If any error occurs, it's added to the checker's diagnostics.
func (c *Checker) checkAndProcessArguments(fnDef *FunctionDef, resolvedExprs []parse.Expression, fnDefCopy *FunctionDef, genericScope *SymbolTable, numOmittedArgs int) ([]Expression, *FunctionDef) {
	// Create the full argument list including synthesized maybe::none() calls for omitted arguments
	// Need to maintain parameter order, so use indexed assignment instead of appending
	totalArgs := len(fnDefCopy.Parameters)
	allExprs := make([]Expression, totalArgs)

	// Process provided arguments
	for i := range resolvedExprs {
		// Skip omitted arguments (nil entries) - handle them separately below
		if resolvedExprs[i] == nil {
			continue
		}

		// Get the expected parameter type from the copy (which has fresh TypeVar instances for generics).
		// For generic functions, dereference to see bound generics from previous arguments.
		// derefType walks the type tree so List($T) becomes List(Int) if $T was bound to Int.
		paramType := fnDefCopy.Parameters[i].Type
		if fnDef.hasGenerics() && genericScope != nil {
			paramType = derefType(paramType)
		}

		// For list and map literals, use checkExprAs to infer type from context.
		// Anonymous functions also use checkExprAs so parameter types are inferred from
		// the (possibly bound) paramType.
		// For nullable parameters, use the inner type since literals can't be Maybe
		expectedType := paramType
		if maybeParam, isMaybe := paramType.(*Maybe); isMaybe {
			_, isLiteralOrFunc := resolvedExprs[i].(*parse.ListLiteral)
			if !isLiteralOrFunc {
				_, isLiteralOrFunc = resolvedExprs[i].(*parse.MapLiteral)
			}
			if !isLiteralOrFunc {
				_, isLiteralOrFunc = resolvedExprs[i].(*parse.AnonymousFunction)
			}
			// For literals and anonymous functions in Maybe parameters, use inner type
			if isLiteralOrFunc {
				expectedType = maybeParam.Of()
			}
		}

		var checkedArg Expression
		c.withValueExprContext(func() {
			switch resolvedExprs[i].(type) {
			case *parse.ListLiteral, *parse.MapLiteral:
				if hasGenericsInType(expectedType) {
					checkedArg = c.checkExpr(resolvedExprs[i])
				} else {
					checkedArg = c.checkExprAs(resolvedExprs[i], expectedType)
				}
			case *parse.AnonymousFunction:
				checkedArg = c.checkExprAs(resolvedExprs[i], expectedType)
			default:
				checkedArg = c.checkExpr(resolvedExprs[i])
			}
		})

		if checkedArg == nil {
			return nil, nil
		}

		// Check if we need to wrap the argument in maybe::some() for nullable parameters
		// If parameter is Maybe<T> and argument is T, wrap it
		if maybeParam, isMaybe := paramType.(*Maybe); isMaybe {
			if argType := checkedArg.Type(); !argType.equal(paramType) {
				// Check if argument type matches the inner Maybe type
				if c.areCompatible(maybeParam.Of(), argType) {
					// Wrap non-Maybe value in maybe::some()
					checkedArg = c.synthesizeMaybeSome(checkedArg, paramType)
				}
			}
		}

		// For generic functions, unify the argument type with the parameter type.
		// unifyTypes uses deref() to see bound generics and calls bindGeneric()
		// to mutate TypeVar instances in-place. This binds generics so that
		// subsequent arguments see bound types.
		if fnDef.hasGenerics() && genericScope != nil {
			if err := c.unifyTypes(paramType, checkedArg.Type(), genericScope); err != nil {
				c.addError(err.Error(), resolvedExprs[i].GetLocation())
				return nil, nil
			}
		} else {
			// For non-generic functions, do regular type compatibility check
			if !c.areCompatible(paramType, checkedArg.Type()) {
				c.addError(typeMismatch(paramType, checkedArg.Type()), resolvedExprs[i].GetLocation())
				return nil, nil
			}
		}

		// Check mutable-reference constraints if needed. A mutable parameter
		// requires an addressable mutable place; a call-site `mut` marker no longer
		// requests a defensive copy.
		if fnDefCopy.Parameters[i].Mutable {
			if !c.isMutable(checkedArg) {
				c.addError(fmt.Sprintf("Type mismatch: Expected a mutable %s", fnDefCopy.Parameters[i].Type.String()), resolvedExprs[i].GetLocation())
				return nil, nil
			}
			allExprs[i] = checkedArg
		} else {
			allExprs[i] = checkedArg
		}
	}

	// Fill in synthesized maybe::none() calls for omitted arguments
	for i := range allExprs {
		if allExprs[i] == nil {
			paramType := fnDefCopy.Parameters[i].Type
			if fnDef.hasGenerics() && genericScope != nil {
				paramType = derefType(paramType)
			}
			allExprs[i] = c.synthesizeMaybeNone(paramType)
		}
	}

	// Finalize generic resolution by creating the specialized function
	var fnToUse *FunctionDef
	if genericScope != nil {
		bindings := genericScope.getGenericBindings()

		if len(bindings) == 0 {
			// No generics were bound from arguments. A receiver specialization
			// may still have pre-bound generics, as with Box<T>.get().
			if len(fnDefCopy.GenericBindings) == 0 {
				fnDefCopy.GenericBindings = inferGenericBindingsFromFunction(fnDef, fnDefCopy)
			}
			if len(fnDefCopy.GenericBindings) > 0 {
				fnToUse = fnDefCopy
			} else {
				fnToUse = fnDef
			}
		} else {
			// Create specialized function with resolved generics
			fnToUse = &FunctionDef{
				Name:                    fnDefCopy.Name,
				GenericParams:           append([]string(nil), fnDefCopy.GenericParams...),
				Parameters:              make([]Parameter, len(fnDefCopy.Parameters)),
				ReturnType:              substituteType(fnDefCopy.ReturnType, bindings),
				InferReturnTypeFromBody: fnDefCopy.InferReturnTypeFromBody,
				Body:                    fnDefCopy.Body,
				Mutates:                 fnDefCopy.Mutates,
				Private:                 fnDefCopy.Private,
				GenericBindings:         cloneTypeMap(bindings),
			}

			// Replace generics in parameters
			for i, param := range fnDefCopy.Parameters {
				fnToUse.Parameters[i] = Parameter{
					Name:    param.Name,
					Type:    substituteType(param.Type, bindings),
					Mutable: param.Mutable,
				}
			}
		}
	} else {
		fnToUse = fnDef
	}

	if fnToUse != nil {
		if derefFn, ok := derefType(fnToUse).(*FunctionDef); ok {
			fnToUse = derefFn
		}
	}

	return allExprs, fnToUse
}

// New generic resolution using the enhanced symbol table
func (c *Checker) resolveGenericFunction(fnDef *FunctionDef, args []Expression, typeArgs []Type, _ parse.Location) (*FunctionDef, error) {
	genericParams := genericParamsForFunction(fnDef)
	if !fnDef.hasGenerics() || len(genericParams) == 0 {
		if len(typeArgs) > 0 {
			return nil, fmt.Errorf("function %s does not take type arguments", fnDef.Name)
		}
		return fnDef, nil
	}

	// Create a call-site-specific generic context scope
	genericScope := c.scope.createGenericScope(genericParams)

	// Create a call-site-specific copy of the function with fresh TypeVar instances
	// This copy is isolated to this call and its TypeVar instances will be mutated during unification
	fnDefCopy := copyFunctionWithTypeVarMap(fnDef, *genericScope.genericContext)

	// Handle explicit type arguments
	if len(typeArgs) > 0 {
		if len(typeArgs) != len(genericParams) {
			return nil, fmt.Errorf("Expected %d type arguments, got %d", len(genericParams), len(typeArgs))
		}

		for i, actual := range typeArgs {
			if actual == nil {
				return nil, fmt.Errorf("could not resolve type argument")
			}

			if err := genericScope.bindGeneric(genericParams[i], actual); err != nil {
				return nil, err
			}
		}
	}

	// Infer types from arguments using the copied function definition
	// The fresh TypeVar instances in fnDefCopy will be mutated as generics are bound
	for i, param := range fnDefCopy.Parameters {
		if err := c.unifyTypes(param.Type, args[i].Type(), genericScope); err != nil {
			return nil, err
		}
	}

	// Allow unresolved generics - they will be resolved through context
	// (e.g., variable assignment, return type inference)
	// Don't require all generics to be resolved at function call time

	// Get bindings to determine if we need to create a specialized version
	bindings := genericScope.getGenericBindings()

	// If no generics were bound, return the original function
	if len(bindings) == 0 {
		return fnDef, nil
	}

	// The function copy already has fresh TypeVar instances that have been bound.
	// We now need to substitute the bindings to create the final specialized function.
	specialized := &FunctionDef{
		Name:                    fnDefCopy.Name,
		GenericParams:           append([]string(nil), fnDefCopy.GenericParams...),
		Parameters:              make([]Parameter, len(fnDefCopy.Parameters)),
		ReturnType:              substituteType(fnDefCopy.ReturnType, bindings),
		InferReturnTypeFromBody: fnDefCopy.InferReturnTypeFromBody,
		Body:                    fnDefCopy.Body,
		Mutates:                 fnDefCopy.Mutates,
		Private:                 fnDefCopy.Private,
		GenericBindings:         cloneTypeMap(bindings),
	}

	// Replace generics in parameters
	for i, param := range fnDefCopy.Parameters {
		specialized.Parameters[i] = Parameter{
			Name:    param.Name,
			Type:    substituteType(param.Type, bindings),
			Mutable: param.Mutable,
		}
	}

	return specialized, nil
}

// unifyTypes performs type unification for generic function arguments.
// It recursively walks the type tree, binding generics to concrete types.
// When expected is a TypeVar (generic parameter), bindGeneric mutates the TypeVar in-place
// with the concrete type, making the binding immediately visible to all callers.
// This enables single-pass argument checking where later arguments see bindings from earlier ones.
func (c *Checker) unifyTypes(expected Type, actual Type, genericScope *SymbolTable) error {
	expected = deref(expected)
	actual = deref(actual)

	switch expectedType := expected.(type) {
	case *TypeVar:
		// Generic type - bind it to the actual type using in-place mutation.
		// This mutates expectedType.bound and expectedType.actual directly.
		return genericScope.bindGeneric(expectedType.name, actual)
	case *FunctionDef:
		// Function type unification
		var actualParams []Parameter
		var actualReturnType Type

		if actualFn, ok := actual.(*FunctionDef); ok {
			actualParams = actualFn.Parameters
			actualReturnType = actualFn.ReturnType
		} else {
			return fmt.Errorf("expected function, got %s", actual.String())
		}

		// Check parameter count
		if len(expectedType.Parameters) != len(actualParams) {
			return fmt.Errorf("parameter count mismatch")
		}

		// Unify parameters
		for i, expectedParam := range expectedType.Parameters {
			if err := c.unifyTypes(expectedParam.Type, actualParams[i].Type, genericScope); err != nil {
				return err
			}
		}

		// Unify return types
		return c.unifyTypes(expectedType.ReturnType, actualReturnType, genericScope)
	case *Result:
		if actualResult, ok := actual.(*Result); ok {
			if err := c.unifyTypes(expectedType.val, actualResult.val, genericScope); err != nil {
				return err
			}
			return c.unifyTypes(expectedType.err, actualResult.err, genericScope)
		}
		return fmt.Errorf("expected result type, got %T", actual)
	case *Maybe:
		if actualMaybe, ok := actual.(*Maybe); ok {
			return c.unifyTypes(expectedType.of, actualMaybe.of, genericScope)
		}
		return fmt.Errorf("expected maybe type, got %T", actual)
	case *List:
		if actualList, ok := actual.(*List); ok {
			return c.unifyTypes(expectedType.of, actualList.of, genericScope)
		}
		return fmt.Errorf("expected list type, got %T", actual)
	case *Chan:
		if actualChannel, ok := actual.(*Chan); ok {
			return c.unifyTypes(expectedType.of, actualChannel.of, genericScope)
		}
		return fmt.Errorf("expected channel type, got %T", actual)
	case *Receiver:
		if act, ok := actual.(*Receiver); ok {
			return c.unifyTypes(expectedType.of, act.of, genericScope)
		}
		return fmt.Errorf("expected receiver type, got %T", actual)
	case *Sender:
		if act, ok := actual.(*Sender); ok {
			return c.unifyTypes(expectedType.of, act.of, genericScope)
		}
		return fmt.Errorf("expected sender type, got %T", actual)
	case *StructDef:
		actualStruct, ok := actual.(*StructDef)
		if !ok || expectedType.Name != actualStruct.Name || namedTypeOwnersDiffer(expectedType.ModulePath, actualStruct.ModulePath) || len(expectedType.TypeArgs) != len(actualStruct.TypeArgs) {
			return fmt.Errorf("type mismatch: expected %s, got %s", expected.String(), actual.String())
		}
		for i := range expectedType.TypeArgs {
			if err := c.unifyTypes(expectedType.TypeArgs[i], actualStruct.TypeArgs[i], genericScope); err != nil {
				return err
			}
		}
		for fieldName, expectedField := range expectedType.Fields {
			actualField, ok := actualStruct.Fields[fieldName]
			if !ok {
				return fmt.Errorf("type mismatch: expected %s, got %s", expected.String(), actual.String())
			}
			if err := c.unifyTypes(expectedField, actualField, genericScope); err != nil {
				return err
			}
		}
		return nil
	default:
		// Concrete types - must match exactly
		if !expected.equal(actual) {
			return fmt.Errorf("type mismatch: expected %s, got %s", expected.String(), actual.String())
		}
		return nil
	}
}

// resolveArguments converts unified argument list to positional arguments
func (c *Checker) resolveArguments(args []parse.Argument, params []Parameter) ([]parse.Expression, error) {
	// Separate positional and named arguments
	var positionalArgs []parse.Expression
	var namedArgs []parse.Argument

	for _, arg := range args {
		if arg.Name == "" {
			// Positional argument
			positionalArgs = append(positionalArgs, arg.Value)
		} else {
			// Named argument
			namedArgs = append(namedArgs, arg)
		}
	}

	// If no named arguments, check if we can omit nullable parameters
	if len(namedArgs) == 0 {
		// Check if all provided positional arguments are present
		if len(positionalArgs) <= len(params) {
			// Check if remaining parameters are all nullable
			allNullableOrProvidedMatches := true
			for i := len(positionalArgs); i < len(params); i++ {
				paramType := params[i].Type
				// Check if parameter type is nullable (Maybe)
				if _, isMaybe := paramType.(*Maybe); !isMaybe {
					allNullableOrProvidedMatches = false
					break
				}
			}

			// If all remaining parameters are nullable, allow omitting them
			if allNullableOrProvidedMatches {
				return positionalArgs, nil
			}
		}
		// Otherwise, return the positional arguments and let the count check handle the error
		return positionalArgs, nil
	}

	// Create a map of parameter names to indices
	paramMap := make(map[string]int)
	for i, param := range params {
		paramMap[param.Name] = i
	}

	// Create result array
	result := make([]parse.Expression, len(params))
	used := make([]bool, len(params))

	// Fill in positional arguments first
	for i, arg := range positionalArgs {
		if i >= len(params) {
			return nil, fmt.Errorf("too many positional arguments")
		}
		result[i] = arg
		used[i] = true
	}

	// Fill in named arguments
	for _, namedArg := range namedArgs {
		paramIndex, exists := paramMap[namedArg.Name]
		if !exists {
			return nil, fmt.Errorf("unknown parameter name: %s", namedArg.Name)
		}

		if used[paramIndex] {
			return nil, fmt.Errorf("parameter %s specified multiple times", namedArg.Name)
		}

		result[paramIndex] = namedArg.Value
		used[paramIndex] = true
	}

	// Check that all parameters are provided (allow missing nullable parameters)
	for i, param := range params {
		if !used[i] {
			// Allow omitting nullable parameters
			paramType := params[i].Type
			if _, isMaybe := paramType.(*Maybe); !isMaybe {
				return nil, fmt.Errorf("missing argument for parameter: %s", param.Name)
			}
		}
	}

	return result, nil
}

// buildComparison builds a comparison expression node from two operands and an operator
// It returns the appropriate typed comparison node (IntGreater, FloatLess, etc.)
func (c *Checker) buildComparison(leftExpr parse.Expression, op parse.Operator, rightExpr parse.Expression) Expression {
	left := c.checkExpr(leftExpr)
	right := c.checkExpr(rightExpr)
	if left == nil || right == nil {
		return nil
	}

	// Allow Enum vs Int comparisons
	if !c.areTypesComparable(left.Type(), right.Type()) {
		c.addError("Cannot compare different types", leftExpr.GetLocation())
		return nil
	}

	// Build the appropriate comparison node based on operator and type
	switch op {
	case parse.GreaterThan:
		if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
			return &IntGreater{left, right}
		}
		if isRelationalFloatLike(left.Type()) {
			return &FloatGreater{left, right}
		}
		c.addError("The '>' operator can only be used for Int or Float64", leftExpr.GetLocation())
		return nil

	case parse.GreaterThanOrEqual:
		if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
			return &IntGreaterEqual{left, right}
		}
		if isRelationalFloatLike(left.Type()) {
			return &FloatGreaterEqual{left, right}
		}
		c.addError("The '>=' operator can only be used for Int or Float64", leftExpr.GetLocation())
		return nil

	case parse.LessThan:
		if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
			return &IntLess{left, right}
		}
		if isRelationalFloatLike(left.Type()) {
			return &FloatLess{left, right}
		}
		c.addError("The '<' operator can only be used for Int or Float64", leftExpr.GetLocation())
		return nil

	case parse.LessThanOrEqual:
		if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
			return &IntLessEqual{left, right}
		}
		if isRelationalFloatLike(left.Type()) {
			return &FloatLessEqual{left, right}
		}
		c.addError("The '<=' operator can only be used for Int or Float64", leftExpr.GetLocation())
		return nil

	default:
		c.addError(fmt.Sprintf("Unsupported operator in comparison: %v", op), leftExpr.GetLocation())
		return nil
	}
}

// tryCheckAccessorChain checks if the expression is a property/method accessor chain with Maybes
// and handles cascading None propagation via OptionMatch expressions
func (c *Checker) tryCheckAccessorChain(parseExpr parse.Expression) Expression {
	// Detect if this is a property/method accessor chain
	if !c.isAccessorChain(parseExpr) {
		// Not an accessor chain, check normally
		return c.checkExpr(parseExpr)
	}

	// Try to build the accessor chain with Maybe handling
	return c.checkAccessorChainWithMaybes(parseExpr)
}

// isAccessorChain checks if an expression is a property or method accessor (possibly chained)
func (c *Checker) isAccessorChain(parseExpr parse.Expression) bool {
	switch parseExpr.(type) {
	case *parse.InstanceProperty, *parse.InstanceMethod:
		return true
	default:
		return false
	}
}

// checkAccessorChainWithMaybes checks an accessor chain and wraps Maybe property accesses in OptionMatch
func (c *Checker) checkAccessorChainWithMaybes(parseExpr parse.Expression) Expression {
	switch p := parseExpr.(type) {
	case *parse.InstanceProperty:
		// First check the target
		target := c.checkAccessorChainWithMaybes(p.Target)
		if target == nil {
			return nil
		}

		// Try to get the property type
		innerType := target.Type()
		var isMaybe bool
		if maybeType, ok := innerType.(*Maybe); ok {
			innerType = maybeType.of
			isMaybe = true
		}

		propType := innerType.get(p.Property.Name)
		if propType == nil {
			c.addError(fmt.Sprintf("Undefined: %s.%s", innerType, p.Property.Name), p.Property.GetLocation())
			return nil
		}

		prop := &InstanceProperty{
			Subject:  target,
			Property: p.Property.Name,
			_type:    propType,
			Kind:     StructSubject,
		}

		// If the target is Maybe, wrap in OptionMatch
		if isMaybe {
			return c.wrapAccessorInMatch(target, prop, innerType, propType)
		}

		return prop

	case *parse.InstanceMethod:
		// Similar logic for methods
		target := c.checkAccessorChainWithMaybes(p.Target)
		if target == nil {
			return nil
		}

		// Try to get the method signature
		innerType := target.Type()
		var isMaybe bool
		if maybeType, ok := innerType.(*Maybe); ok {
			innerType = maybeType.of
			isMaybe = true
		}

		var sig Type
		if structDef, ok := innerType.(*StructDef); ok {
			if method, ok := c.structMethod(structDef, p.Method.Name); ok {
				sig = method
			}
		} else {
			sig = innerType.get(p.Method.Name)
		}
		if sig == nil {
			// This accessor-chain path only speculatively verifies that a method
			// exists before falling back to normal expression checking below.
			// Pointer-receiver mutability/addressability is enforced by the normal
			// InstanceMethod checker, not here.
			if foreign, ok := innerType.(*ForeignType); ok && !foreign.Pointer {
				pointerForeign := *foreign
				pointerForeign.Pointer = true
				pointerForeign.Methods = nil
				pointerForeign.MethodsLoaded = false
				if pointerSig := pointerForeign.get(p.Method.Name); pointerSig != nil {
					sig = pointerSig
				}
			}
		}
		if sig == nil {
			if !isMaybe {
				if call, ok := c.checkFunctionFieldCall(target, p.Method, p.GetLocation()); ok {
					return call
				}
			}
			c.addError(fmt.Sprintf("Undefined: %s.%s", innerType, p.Method.Name), p.Method.GetLocation())
			return nil
		}

		_, ok := sig.(*FunctionDef)
		if !ok {
			c.addError(fmt.Sprintf("%s.%s is not a function", innerType, p.Method.Name), p.Method.GetLocation())
			return nil
		}

		// For now, just check the method normally and return if not Maybe
		// (full method call handling is complex, only handle property accessor chains for now)
		if !isMaybe {
			// Fall back to normal checking for non-Maybe methods
			return c.checkExpr(parseExpr)
		}

		// If the target is Maybe, we'd need to wrap the entire method call
		// For simplicity, just check normally - the user should use property access if they want cascading
		return c.checkExpr(parseExpr)

	default:
		// Not an accessor, check normally
		return c.checkExpr(parseExpr)
	}
}

// wrapAccessorInMatch wraps a property access on a Maybe type in an OptionMatch expression
func (c *Checker) wrapAccessorInMatch(subject Expression, prop *InstanceProperty, innerType Type, propType Type) Expression {
	// Generate a pattern variable name
	patternVar := "_maybe_prop"

	// Create a symbol for the pattern variable
	patternSym := Symbol{
		Name:    patternVar,
		Type:    innerType,
		mutable: false,
	}

	// Create an identifier for the pattern variable with the symbol
	patternIdent := &Identifier{Name: patternVar}
	patternIdent.sym = patternSym

	// The Some block accesses the property on the unwrapped value
	// We create a new InstanceProperty with the pattern variable as subject
	propOnUnwrapped := &InstanceProperty{
		Subject:  patternIdent,
		Property: prop.Property,
		_type:    propType,
		Kind:     StructSubject,
	}

	// Create the Some block containing the property access
	someBlock := &Block{
		Stmts: []Statement{
			{Expr: propOnUnwrapped},
		},
	}

	// The None block just returns the subject (which is None)
	// The subject's type is Maybe<innerType>, so it will propagate as None of type propType
	noneBlock := &Block{
		Stmts: []Statement{
			{Expr: subject},
		},
	}

	// Create and return the OptionMatch
	return &OptionMatch{
		Subject: subject,
		Some: &Match{
			Pattern: patternIdent,
			Body:    someBlock,
		},
		None:      noneBlock,
		InnerType: innerType,
	}
}

// validateJSONParseTarget rejects json::parse into shapes it cannot decode
// unambiguously. It is parse-specific: parsing into a union is ambiguous, and
// JSON object keys are always strings (ADR 0031). Encoding has no such
// restriction (a union marshals to its active member), so it is an ordinary
// function and is not validated here.
func validateJSONParseTarget(typ Type) error {
	if result, ok := derefType(typ).(*Result); ok {
		typ = result.Val()
	}
	return validateJSONParseShape(derefType(typ), map[string]bool{})
}

func validateJSONParseShape(typ Type, seen map[string]bool) error {
	typ = derefType(typ)
	if typ == Str || typ == Int || typ == Float64 || typ == Bool || typ == Byte || typ == Rune || typ == Any {
		return nil
	}
	switch t := typ.(type) {
	case *List:
		return validateJSONParseShape(t.Of(), seen)
	case *Map:
		if derefType(t.Key()) != Str {
			return fmt.Errorf("json::parse only supports Str map keys, got %s", t.Key().String())
		}
		return validateJSONParseShape(t.Value(), seen)
	case *Maybe:
		return validateJSONParseShape(t.Of(), seen)
	case *Enum:
		return nil
	case *Result:
		if err := validateJSONParseShape(t.Val(), seen); err != nil {
			return err
		}
		return validateJSONParseShape(t.Err(), seen)
	case *Union:
		return fmt.Errorf("json::parse does not support %s: parsing into a union is ambiguous", typ.String())
	case *StructDef:
		name := t.String()
		if seen[name] {
			return nil
		}
		seen[name] = true
		for fieldName, fieldType := range t.Fields {
			if err := validateJSONParseShape(fieldType, seen); err != nil {
				return fmt.Errorf("json::parse field %s: %w", fieldName, err)
			}
		}
		return nil
	case *TypeVar:
		if t.Actual() != nil {
			return validateJSONParseShape(t.Actual(), seen)
		}
	}
	return fmt.Errorf("json::parse does not support %s", typ.String())
}
