package checker

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/akonwi/ard/parse"
)

type Program struct {
	Imports    map[string]Module
	Statements []Statement
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

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s %s %s", d.filePath, d.location.Start, d.Message)
}

// deref follows TypeVar bindings to find the concrete type.
// Used during type unification to ensure we see resolved types.
// Only dereferences a single type node; for compound types use derefType.
//
// Example: If $T is bound to Int, deref($T) returns Int.
// If $T is bound to [$U], deref($T) returns [$U] (not the resolved contents).
func deref(t Type) Type {
	if typeVar, ok := t.(*TypeVar); ok && typeVar.bound {
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
	t = deref(t) // First dereference at the top level
	switch typ := t.(type) {
	case *List:
		derefInner := derefType(typ.of)
		if derefInner == typ.of {
			return typ // No change, return original
		}
		return &List{of: derefInner}
	case *Map:
		derefKey := derefType(typ.key)
		derefVal := derefType(typ.value)
		if derefKey == typ.key && derefVal == typ.value {
			return typ // No change, return original
		}
		return &Map{
			key:   derefKey,
			value: derefVal,
		}
	case *Maybe:
		derefInner := derefType(typ.of)
		if derefInner == typ.of {
			return typ // No change, return original
		}
		return &Maybe{of: derefInner}
	case *Result:
		derefVal := derefType(typ.val)
		derefErr := derefType(typ.err)
		if derefVal == typ.val && derefErr == typ.err {
			return typ // No change, return original
		}
		return &Result{
			val: derefVal,
			err: derefErr,
		}
	case *Union:
		newTypes := make([]Type, len(typ.Types))
		changed := false
		for i, t := range typ.Types {
			newTypes[i] = derefType(t)
			if newTypes[i] != typ.Types[i] {
				changed = true
			}
		}
		if !changed {
			return typ // No change, return original
		}
		return &Union{
			Name:  typ.Name,
			Types: newTypes,
		}
	case *FunctionDef:
		newParams := make([]Parameter, len(typ.Parameters))
		paramsChanged := false
		for i, param := range typ.Parameters {
			derefParamType := derefType(param.Type)
			newParams[i] = Parameter{
				Name:    param.Name,
				Type:    derefParamType,
				Mutable: param.Mutable,
			}
			if derefParamType != param.Type {
				paramsChanged = true
			}
		}
		derefReturnType := derefType(typ.ReturnType)
		returnChanged := derefReturnType != typ.ReturnType
		if !paramsChanged && !returnChanged {
			return typ // No change, return original
		}
		return &FunctionDef{
			Name:       typ.Name,
			Parameters: newParams,
			ReturnType: derefReturnType,
			Body:       typ.Body,
			Mutates:    typ.Mutates,
			Private:    typ.Private,
		}
	default:
		return t
	}
}

func (c Checker) isMutable(expr Expression) bool {
	switch e := expr.(type) {
	case *Variable:
		return e.sym.mutable
	case *InstanceProperty:
		return c.isMutable(e.Subject)
	}
	return false
}

// isCopyable returns true if the type can be copied for mut parameters
func (c Checker) isCopyable(_ Type) bool {
	// In Ard, all types are copyable by default
	// Future: might exclude external resources like file handles
	return true
}

// shouldCopyForMutableAssignment determines if we should copy for mut variable assignments
// This is more conservative than isCopyable to avoid unnecessary copying of primitives
func (c Checker) shouldCopyForMutableAssignment(t Type) bool {
	switch t.(type) {
	case *StructDef, *List, *Map:
		// Complex types that benefit from copy semantics
		return true
	default:
		// Primitives (Int, Str, Bool, etc.) are immutable anyway, no need to copy
		return false
	}
}

type Checker struct {
	diagnostics    []Diagnostic
	input          *parse.Program
	scope          *SymbolTable
	filePath       string
	program        *Program
	halted         bool
	moduleResolver *ModuleResolver
}

func New(filePath string, input *parse.Program, moduleResolver *ModuleResolver) *Checker {
	rootScope := makeScope(nil)
	c := &Checker{
		diagnostics:    []Diagnostic{},
		input:          input,
		filePath:       filePath,
		moduleResolver: moduleResolver,
		program: &Program{
			Imports:    map[string]Module{},
			Statements: []Statement{},
		},
		scope: &rootScope,
	}

	return c
}

func (c *Checker) HasErrors() bool {
	return len(c.diagnostics) > 0
}

func (c *Checker) Diagnostics() []Diagnostic {
	return c.diagnostics
}

func (c *Checker) Check() {
	for _, imp := range c.input.Imports {
		if _, dup := c.program.Imports[imp.Name]; dup {
			c.addWarning(fmt.Sprintf("%s Duplicate import: %s", imp.GetStart(), imp.Name), imp.GetLocation())
			continue
		}
		if _, dup := c.program.Imports[imp.Name]; dup {
			c.addWarning(fmt.Sprintf("%s Duplicate import: %s", imp.GetStart(), imp.Name), imp.GetLocation())
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

			filePath, err := c.moduleResolver.ResolveImportPath(imp.Path)
			if err != nil {
				c.addError(fmt.Sprintf("Failed to resolve import '%s': %v", imp.Path, err), imp.GetLocation())
				continue
			}

			// Check if module is already cached
			if cachedModule, ok := c.moduleResolver.moduleCache[filePath]; ok {
				c.program.Imports[imp.Name] = cachedModule
				continue
			}

			// Load and parse the module file using import path
			ast, err := c.moduleResolver.LoadModule(imp.Path)
			if err != nil {
				c.addError(fmt.Sprintf("Failed to load module %s: %v", filePath, err), imp.GetLocation())
				continue
			}

			// Type-check the imported module
			userModule, diagnostics := check(ast, c.moduleResolver, imp.Path+".ard")
			if len(diagnostics) > 0 {
				// Add all diagnostics from the imported module
				for _, diag := range diagnostics {
					c.diagnostics = append(c.diagnostics, diag)
				}
				continue
			}

			// Set the correct file path for the module
			if um, ok := userModule.(*UserModule); ok {
				um.setFilePath(imp.Path)
			}

			// Cache and add to imports
			c.moduleResolver.moduleCache[filePath] = userModule
			c.program.Imports[imp.Name] = userModule
		}
	}

	// Auto-import prelude modules (only for non-std lib)
	if !strings.HasPrefix(c.filePath, "ard/") {
		if mod, ok := findInStdLib("ard/dynamic"); ok {
			c.program.Imports["Dynamic"] = mod
		}
		if mod, ok := findInStdLib("ard/float"); ok {
			c.program.Imports["Float"] = mod
		}
		if mod, ok := findInStdLib("ard/int"); ok {
			c.program.Imports["Int"] = mod
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

	for i := range c.input.Statements {
		if stmt := c.checkStmt(&c.input.Statements[i]); stmt != nil {
			c.program.Statements = append(c.program.Statements, *stmt)
		}
		if c.halted {
			break
		}
	}

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
	return NewUserModule(c.filePath, c.program, c.scope)
}

// check is an internal helper for recursive module checking.
// Use New() + Check() + Module() for the public API.
func check(input *parse.Program, moduleResolver *ModuleResolver, filePath string) (Module, []Diagnostic) {
	c := New(filePath, input, moduleResolver)

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

func collectGenericsFromType(t Type, params *[]string, seen map[string]bool) {
	switch t := t.(type) {
	case *TypeVar:
		if !seen[t.name] {
			*params = append(*params, t.name)
			seen[t.name] = true
		}
	case *List:
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
		// Extract generics from struct fields in a consistent order
		// We need to iterate in a deterministic order; using sorted keys would work
		for _, fieldType := range t.Fields {
			collectGenericsFromType(fieldType, params, seen)
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

	// 3. Replace generics
	specializedType := originalType
	for i, typeArg := range typeArgs {
		genericName := genericParams[i]
		resolvedArgType := c.resolveType(typeArg)
		specializedType = replaceGeneric(specializedType, genericName, resolvedArgType)
	}
	return specializedType
}

func (c *Checker) resolveType(t parse.DeclaredType) Type {
	var baseType Type
	switch ty := t.(type) {
	case *parse.StringType:
		baseType = Str
	case *parse.IntType:
		baseType = Int
	case *parse.FloatType:
		baseType = Float
	case *parse.BooleanType:
		baseType = Bool
	case *parse.VoidType:
		baseType = Void

	case *parse.FunctionType:
		// Convert each parameter type and return type
		params := make([]Parameter, len(ty.Params))
		for i, param := range ty.Params {
			params[i] = Parameter{
				Name: fmt.Sprintf("arg%d", i),
				Type: c.resolveType(param),
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
		baseType = MakeMap(key, value)
	case *parse.ResultType:
		val := c.resolveType(ty.Val)
		err := c.resolveType(ty.Err)
		baseType = MakeResult(val, err)
	case *parse.CustomType:
		if t.GetName() == "Dynamic" {
			baseType = Dynamic
			break
		}

		if sym, ok := c.scope.get(t.GetName()); ok {
			if len(ty.TypeArgs) > 0 {
				baseType = c.specializeAliasedType(sym.Type, ty.TypeArgs, ty.GetLocation())
			} else {
				baseType = sym.Type
			}
			break
		}
		if ty.Type.Target != nil {
			mod := c.resolveModule(ty.Type.Target.(*parse.Identifier).Name)
			if mod != nil {
				// at some point, this will need to unwrap the property down to root for nested paths: `mod::sym::more`
				sym := mod.Get(ty.Type.Property.(*parse.Identifier).Name)
				if !sym.IsZero() {
					if len(ty.TypeArgs) > 0 {
						baseType = c.specializeAliasedType(sym.Type, ty.TypeArgs, ty.GetLocation())
					} else {
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

func formatTypeForDisplay(t Type) string {
	// For StructDef with generic fields, show type parameters
	if structDef, ok := t.(*StructDef); ok {
		if resultType, hasResult := structDef.Fields["result"]; hasResult {
			// The result field's type indicates the generic parameter
			return fmt.Sprintf("%s<%s>", structDef.String(), resultType.String())
		}
	}
	return t.String()
}

func typeMismatch(expected, got Type) string {
	exMsg := formatTypeForDisplay(expected)
	if _, isTrait := expected.(*Trait); isTrait {
		exMsg = "implementation of " + exMsg
	}
	return fmt.Sprintf("Type mismatch: Expected %s, got %s", exMsg, formatTypeForDisplay(got))
}

func (c *Checker) checkStmt(stmt *parse.Statement) *Statement {
	if c.halted {
		return nil
	}
	switch s := (*stmt).(type) {
	case *parse.Comment:
		return nil
	case *parse.Break:
		return &Statement{Break: true}
	case *parse.TraitDefinition:
		{
			methods := make([]FunctionDef, len(s.Methods))
			for i, method := range s.Methods {
				params := make([]Parameter, len(method.Parameters))
				for j, param := range method.Parameters {
					paramType := c.resolveType(param.Type)
					if paramType == nil {
						c.addError(fmt.Sprintf("Unrecognized type: %s", param.Type.GetName()), param.Type.GetLocation())
						continue
					}
					params[j] = Parameter{Name: param.Name, Type: paramType}
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

			trait := Trait{
				private: s.Private,
				Name:    s.Name.Name,
				methods: methods,
			}

			c.scope.add(trait.name(), &trait, false)
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

						params[i] = Parameter{Name: param.Name, Type: paramType}
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
						c.scope.add("@", targetType, method.Mutates)
					})
					fnDef.Mutates = method.Mutates
					// add the method to the struct
					targetType.Methods[method.Name] = fnDef
				}

				// Check if all required methods are implemented
				for _, method := range traitMethods {
					if !implementedMethods[method.Name] {
						c.addError(fmt.Sprintf("Missing method '%s' in trait '%s'", method.Name, trait.name()), s.GetLocation())
					}
				}

				// Add the trait to the struct type's traits list
				targetType.Traits = append(targetType.Traits, trait)

				// Return the struct so VM can register the new trait methods
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

						params[i] = Parameter{Name: param.Name, Type: paramType}
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
						c.scope.add("@", targetType, false) // Enums are immutable, so always false
					})
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

				// Return the enum so VM can register the new trait methods
				return &Statement{Stmt: targetType}

			default:
				c.addError(fmt.Sprintf("%s cannot implement a Trait", s.ForType.Name), s.ForType.GetLocation())
				return nil
			}
		}
	case *parse.TypeDeclaration:
		{
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

			// Create a union type (even if it only contains one type)
			unionType := &Union{
				Name:  s.Name.Name,
				Types: types,
			}

			// Register the type in the scope with the given name
			c.scope.add(unionType.name(), unionType, false)
			return nil
		}
	case *parse.VariableDeclaration:
		{
			var val Expression
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
					return nil
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

			if val == nil {
				return nil
			}

			__type := val.Type()

			if s.Type != nil {
				if expected := c.resolveType(s.Type); expected != nil {
					if !expected.equal(val.Type()) {
						c.addError(typeMismatch(expected, val.Type()), s.Value.GetLocation())
						return nil
					}
					__type = expected
				}
			}

			// Apply copy semantics for mutable variable assignments
			if s.Mutable && c.shouldCopyForMutableAssignment(val.Type()) {
				// Always wrap in copy expression for mutable assignment of copyable types
				val = &CopyExpression{
					Expr:  val,
					Type_: val.Type(),
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

				value := c.checkExpr(s.Value)
				if value == nil {
					return nil
				}

				if !target.Type.equal(value.Type()) {
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
				value := c.checkExpr(s.Value)
				if value == nil {
					return nil
				}

				if !c.isMutable(subject) {
					c.addError(fmt.Sprintf("Immutable: %s", ip), s.Target.GetLocation())
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
					// Add the cursor variable to the scope as a string
					// Each character in a string is also a string
					c.scope.add(s.Cursor.Name, Str, false)
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

				body := c.checkBlock(s.Body, func() {
					// Add the cursor variable to the scope
					c.scope.add(s.Cursor.Name, listType.of, false)
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

				body := c.checkBlock(s.Body, func() {
					// Add the cursors to the scope
					c.scope.add(loop.Key, mapType.Key(), false)
					c.scope.add(loop.Val, mapType.Value(), false)
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

			enum := &Enum{
				Private: s.Private,
				Name:    s.Name,
				Values:  computedValues,
				Methods: make(map[string]*FunctionDef),
			}

			c.scope.add(enum.name(), enum, false)
			return nil
		}
	case *parse.StructDefinition:
		{
			def := &StructDef{
				Name:    s.Name.Name,
				Fields:  make(map[string]Type),
				Methods: make(map[string]*FunctionDef),
				Private: s.Private,
			}
			for _, field := range s.Fields {
				fieldType := c.resolveType(field.Type)
				if fieldType == nil {
					return nil
				}

				if _, dup := def.Fields[field.Name.Name]; dup {
					c.addError(fmt.Sprintf("Duplicate field: %s", field.Name.Name), field.Name.GetLocation())
					return nil
				}
				def.Fields[field.Name.Name] = fieldType
			}
			c.scope.add(def.name(), def, false)
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
						c.scope.add("@", def, method.Mutates)
					})
					if fnDef != nil {
						fnDef.Mutates = method.Mutates
						def.Methods[method.Name] = fnDef
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
						c.scope.add("@", def, false)
					})
					def.Methods[method.Name] = fnDef
				}
				return &Statement{Stmt: def}
			default:
				c.addError(fmt.Sprintf("Can only implement methods on structs and enums, not %s", sym.Type), s.Target.GetLocation())
				return nil
			}
			return nil
		}
	case nil:
		return nil
	default:
		expr := c.checkExpr((parse.Expression)(*stmt))
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
		for i := range expr.Items {
			item := expr.Items[i]
			element := c.checkExpr(item)
			if !expectedElementType.equal(element.Type()) {
				c.addError(typeMismatch(expectedElementType, element.Type()), item.GetLocation())
				return nil
			}
			elements[i] = element
		}

		return &ListLiteral{
			Elements: elements,
			_type:    declaredType.(*List),
		}
	}

	if len(expr.Items) == 0 {
		c.addError("Empty lists need an explicit type", expr.GetLocation())
		c.halted = true
		return &ListLiteral{_type: MakeList(Void), Elements: []Expression{}}
	}

	hasError := false
	var elementType Type
	elements := make([]Expression, len(expr.Items))
	for i := range expr.Items {
		item := expr.Items[i]
		element := c.checkExpr(item)
		if element == nil {
			continue
		}

		if i == 0 {
			elementType = element.Type()
		} else if !elementType.equal(element.Type()) {
			c.addError("Type mismatch: A list can only contain values of single type", item.GetLocation())
			hasError = true
			continue
		}

		elements[i] = element
	}

	if hasError {
		return nil
	}

	return &ListLiteral{
		Elements: elements,
		_type:    MakeList(elementType),
	}
}

func (c *Checker) checkBlock(stmts []parse.Statement, setup func()) *Block {
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

	block := &Block{Stmts: make([]Statement, len(stmts))}
	for i := range stmts {
		if stmt := c.checkStmt(&stmts[i]); stmt != nil {
			block.Stmts[i] = *stmt
		}
		if c.halted {
			break
		}
	}
	return block
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
			if !areCompatible(expectedKeyType, key.Type()) {
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
			if !areCompatible(expectedValueType, value.Type()) {
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

	// For generic structs, infer generics from provided field values
	var structDefCopy *StructDef
	var genericScope *SymbolTable
	if structType.hasGenerics() {
		// Extract generic parameter names from struct fields
		genericNames := make(map[string]bool)
		for _, fieldType := range structType.Fields {
			extractGenericNames(fieldType, genericNames)
		}

		genericParams := make([]string, 0, len(genericNames))
		for name := range genericNames {
			genericParams = append(genericParams, name)
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
			// For generic structs, unify types to resolve generics
			if genericScope != nil {
				// Check expression without type context first (let it infer if possible)
				checkVal := c.checkExpr(property.Value)
				if checkVal == nil {
					continue
				}

				if err := c.unifyTypes(field, checkVal.Type(), genericScope); err != nil {
					c.addError(err.Error(), property.GetLocation())
					continue
				}
				// After unification, dereference to get the actual type
				field = derefType(field)

				fields[fieldName] = checkVal
				fieldTypes[fieldName] = field
			} else {
				// For non-generic structs, use the original checkExprAs which provides type context
				if val := c.checkExprAs(property.Value, field); val != nil {
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
				// todo: distinguish between provided w/ error + missing to avoid creating 2 errors:
				// missing field + invalid expression for field
				missing = append(missing, name)
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
	instance._type = &StructDef{
		Name:    structDefCopy.Name,
		Fields:  fieldTypes,
		Methods: structDefCopy.Methods,
		Self:    structDefCopy.Self,
		Traits:  structDefCopy.Traits,
		Private: structDefCopy.Private,
	}
	return instance
}

// createPrimitiveMethodNode creates type-specific method nodes for primitives and collections
// Falls back to generic InstanceMethod for user-defined types (structs, enums)
func (c *Checker) createPrimitiveMethodNode(subject Expression, methodName string, args []Expression, fnDef *FunctionDef) Expression {
	// Determine subject type - emit specialized nodes for all built-in types
	switch subject.Type() {
	case Str:
		return c.createStrMethod(subject, methodName, args)
	case Int:
		return c.createIntMethod(subject, methodName)
	case Float:
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
	return &InstanceMethod{
		Subject: subject,
		Method: &FunctionCall{
			Name: methodName,
			Args: args,
			fn:   fnDef,
		},
	}
}

func (c *Checker) createStrMethod(subject Expression, methodName string, args []Expression) Expression {
	var kind StrMethodKind
	switch methodName {
	case "size":
		kind = StrSize
	case "is_empty":
		kind = StrIsEmpty
	case "contains":
		kind = StrContains
	case "replace":
		kind = StrReplace
	case "replace_all":
		kind = StrReplaceAll
	case "split":
		kind = StrSplit
	case "starts_with":
		kind = StrStartsWith
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
		panic(fmt.Sprintf("Unknown Float method: %s", methodName))
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
	default:
		panic(fmt.Sprintf("Unknown Maybe method: %s", methodName))
	}
	return &MaybeMethod{
		Subject:   subject,
		Kind:      kind,
		Args:      args,
		InnerType: maybeType.Of(),
		fn:        fnDef,
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
	default:
		panic(fmt.Sprintf("Unknown Result method: %s", methodName))
	}
	return &ResultMethod{
		Subject: subject,
		Kind:    kind,
		Args:    args,
		OkType:  resultType.Val(),
		ErrType: resultType.Err(),
		fn:      fnDef,
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

	// Now compare original field types with specialized field types
	// For each field that contains a generic in the original, extract its resolved type
	for fieldName, originalFieldType := range originalDef.Fields {
		specializedFieldType, ok := specializedDef.Fields[fieldName]
		if !ok {
			continue
		}

		// Extract generic names from the original field type
		genericNames := make(map[string]bool)
		extractGenericNames(originalFieldType, genericNames)

		// For each generic found, bind it to the corresponding specialized type
		for genericName := range genericNames {
			if _, alreadyBound := bindings[genericName]; !alreadyBound {
				// Bind this generic to the specialized field type
				bindings[genericName] = specializedFieldType
			}
		}
	}

	return bindings
}

func (c *Checker) checkExpr(expr parse.Expression) Expression {
	if c.halted {
		return nil
	}
	switch s := (expr).(type) {
	case *parse.StrLiteral:
		return &StrLiteral{s.Value}
	case *parse.BoolLiteral:
		return &BoolLiteral{s.Value}
	case *parse.VoidLiteral:
		return &VoidLiteral{}
	case *parse.NumLiteral:
		{
			stripped := strings.ReplaceAll(s.Value, "_", "")
			if strings.Contains(stripped, ".") {
				value, err := strconv.ParseFloat(s.Value, 64)
				if err != nil {
					c.addError(fmt.Sprintf("Invalid float: %s", s.Value), s.GetLocation())
					return &FloatLiteral{Value: 0.0}
				}
				return &FloatLiteral{Value: value}
			}
			value, err := strconv.Atoi(stripped)
			if err != nil {
				c.addError(fmt.Sprintf("Invalid int: %s", s.Value), s.GetLocation())
			}
			return &IntLiteral{value}
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
					methodNode := c.createPrimitiveMethodNode(cx, toStrMethod.Name, []Expression{}, &toStrMethod)
					chunks[i] = methodNode
				}
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
	case *parse.FunctionCall:
		{
			if s.Name == "panic" {
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
				// Check if it's an external function
				if extFnDef, ok := fnSym.Type.(*ExternalFunctionDef); ok {
					// Convert ExternalFunctionDef to FunctionDef for type checking
					fnDef = &FunctionDef{
						Name:       extFnDef.Name,
						Parameters: extFnDef.Parameters,
						ReturnType: extFnDef.ReturnType,
						Body:       nil, // External functions don't have bodies
						Private:    extFnDef.Private,
					}
				} else {
					//// technically, the below isn't possible anymore
					// Check if it's a variable that holds a function
					// if varDef, ok := fnSym.(*VariableDef); ok {
					// 	// Try to get a FunctionDef directly
					// 	if anon, ok := varDef.Value.(*FunctionDef); ok {
					// 		fnDef = anon
					// 	} else if existingFnDef, ok := varDef._type().(*FunctionDef); ok {
					// 		// FunctionDef can be used directly
					// 		// This handles the case where a variable holds a function
					// 		fnDef = existingFnDef
					// 	} else {
					// 		c.addError(fmt.Sprintf("Not a function: %s", s.Name), s.GetLocation())
					// 		return nil
					// 	}
					// } else {
					c.addError(fmt.Sprintf("Not a function: %s", s.Name), s.GetLocation())
					return nil
					// }
				}
			}

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
				return nil
			}

			// Align mutability information with parameters
			resolvedArgs := c.alignArgumentsWithParameters(s.Args, fnDef.Parameters)

			// Setup generics if function has them
			fnDefCopy, genericScope := c.setupFunctionGenerics(fnDef)

			// Check and process arguments (handles both generics and mutability)
			args, fnToUse := c.checkAndProcessArguments(fnDef, resolvedArgs, resolvedExprs, fnDefCopy, genericScope, numOmittedArgs)
			if args == nil {
				return nil
			}

			// Create and return the function call node
			return &FunctionCall{
				Name: s.Name,
				Args: args,
				fn:   fnToUse,
			}
		}
	case *parse.InstanceProperty:
		{
			subj := c.checkExpr(s.Target)
			if subj == nil {
				// panic(fmt.Errorf("Cannot access %s on nil", s.Property))
				return nil
			}

			propType := subj.Type().get(s.Property.Name)
			if propType == nil {
				c.addError(fmt.Sprintf("Undefined: %s.%s", subj, s.Property.Name), s.Property.GetLocation())
				return nil
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
			sig := subj.Type().get(s.Method.Name)
			if sig == nil {
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
				return nil
			}

			// Align mutability information with parameters
			resolvedArgs := c.alignArgumentsWithParameters(s.Method.Args, fnDef.Parameters)

			// For methods on generic struct instances, bind struct generics to method parameters.
			// When calling a method on a generic struct instance, the method's generic parameters
			// should use the types that were resolved during struct instantiation.
			// Example: For Box{ item: 42 }, calling put(x: $T) should require Int for $T.
			var genericScope *SymbolTable
			var fnDefCopy *FunctionDef

			// Check if the subject is a struct and if the original struct definition has generics
			structType, isStruct := subj.Type().(*StructDef)
			if isStruct {
				// Look up the original struct definition by name to check if it's generic
				scopeSymbol, _ := c.scope.get(structType.Name)
				var originalDef *StructDef
				if scopeSymbol != nil {
					if origType, ok := scopeSymbol.Type.(*StructDef); ok {
						originalDef = origType
					}
				}

				// If the original definition has generics, the current structType might be
				// a specialized version with concrete field types (e.g., item: Int instead of item: $T)
				if originalDef != nil && originalDef.hasGenerics() {
					// Extract generic parameter names used in the function
					genericNames := make(map[string]bool)
					for _, param := range fnDef.Parameters {
						extractGenericNames(param.Type, genericNames)
					}
					extractGenericNames(fnDef.ReturnType, genericNames)

					if len(genericNames) > 0 {
						genericParams := make([]string, 0, len(genericNames))
						for name := range genericNames {
							genericParams = append(genericParams, name)
						}

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

			// Check and process arguments (handles both generics and mutability)
			args, fnToUse := c.checkAndProcessArguments(fnDef, resolvedArgs, resolvedExprs, fnDefCopy, genericScope, numOmittedArgs)
			if args == nil {
				return nil
			}

			// Create function call
			return c.createPrimitiveMethodNode(subj, s.Method.Name, args, fnToUse)
		}
	case *parse.UnaryExpression:
		{
			value := c.checkExpr(s.Operand)
			if value == nil {
				return nil
			}
			if s.Operator == parse.Minus {
				if value.Type() != Int && value.Type() != Float {
					c.addError("Only numbers can be negated with '-'", s.GetLocation())
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
					if left.Type() == Int {
						return &IntAddition{left, right}
					}
					if left.Type() == Float {
						return &FloatAddition{left, right}
					}
					if left.Type() == Str {
						return &StrAddition{left, right}
					}
					c.addError("The '-' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case parse.Minus:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot subtract different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntSubtraction{left, right}
					}
					if left.Type() == Float {
						return &FloatSubtraction{left, right}
					}
					c.addError("The '+' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case parse.Multiply:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot multiply different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntMultiplication{left, right}
					}
					if left.Type() == Float {
						return &FloatMultiplication{left, right}
					}
					c.addError("The '*' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case parse.Divide:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot divide different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntDivision{left, right}
					}
					if left.Type() == Float {
						return &FloatDivision{left, right}
					}
					c.addError("The '/' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case parse.Modulo:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot modulo different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntModulo{left, right}
					}
					c.addError("The '%' operator can only be used for Int", s.GetLocation())
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
						if left.Type() == Int || c.isEnum(left.Type()) {
							return &IntGreater{left, right}
						}
						if left.Type() == Float {
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
						if left.Type() == Int || c.isEnum(left.Type()) {
							return &IntGreaterEqual{left, right}
						}
						if left.Type() == Float {
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
						if left.Type() == Int || c.isEnum(left.Type()) {
							return &IntLess{left, right}
						}
						if left.Type() == Float {
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
						if left.Type() == Int || c.isEnum(left.Type()) {
							return &IntLessEqual{left, right}
						}
						if left.Type() == Float {
							return &FloatLessEqual{left, right}
						}
					}
					c.addError("Cannot compare different types", s.GetLocation())
					return nil
				}
			case parse.Equal:
				{
					left, right := c.checkExpr(s.Left), c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					isMaybe := func(val Type) bool {
						_, ok := val.(*Maybe)
						return ok
					}
					if isMaybe(left.Type()) && isMaybe(right.Type()) {
						return &Equality{left, right}
					}

					// Allow Enum vs Int and Int vs Enum comparisons
					if !c.areTypesComparable(left.Type(), right.Type()) {
						c.addError(fmt.Sprintf("Invalid: %s == %s", left.Type(), right.Type()), s.GetLocation())
						return nil
					}

					// Check if types are allowed for equality (use equal() not pointer equality)
					isAllowedType := func(t Type) bool {
						// Primitives are allowed for equality
						if t.equal(Int) || t.equal(Float) || t.equal(Str) || t.equal(Bool) {
							return true
						}
						// Enums are allowed (they are just integers with semantic meaning)
						_, isEnum := t.(*Enum)
						return isEnum
					}
					if !isAllowedType(left.Type()) || !isAllowedType(right.Type()) {
						c.addError(fmt.Sprintf("Invalid: %s == %s", left.Type(), right.Type()), s.GetLocation())
						return nil
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
					if !areCompatible(paramType, checkedArg.Type()) {
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
					Name: absolutePath,
					Args: args,
					fn:   fnDef,
				}

				// Use new generic resolution system
				if fnDef.hasGenerics() {
					specialized, err := c.resolveGenericFunction(fnDef, args, s.Function.TypeArgs, s.GetLocation())
					if err != nil {
						c.addError(err.Error(), s.GetLocation())
						return nil
					}

					call.fn = specialized
				}

				return call
			}

			// find the function in a module
			modName, name := c.destructurePath(s)
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
			case *ExternalFunctionDef:
				// Convert ExternalFunctionDef to FunctionDef for validation
				fnDef = &FunctionDef{
					Name:       fn.Name,
					Parameters: fn.Parameters,
					ReturnType: fn.ReturnType,
					Private:    fn.Private,
				}
				ok = true
			default:
				ok = false
			}

			if !ok {
				targetName := s.Target.String()
				c.addError(fmt.Sprintf("%s::%s is not a function", targetName, s.Function.Name), s.GetLocation())
				return nil
			}

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
				return nil
			}

			// Align mutability information with parameters
			resolvedArgs := c.alignArgumentsWithParameters(s.Function.Args, fnDef.Parameters)

			// Setup generics if function has them
			fnDefCopy, genericScope := c.setupFunctionGenerics(fnDef)

			// Check and process arguments (handles both generics and mutability)
			args, fnToUse := c.checkAndProcessArguments(fnDef, resolvedArgs, resolvedExprs, fnDefCopy, genericScope, numOmittedArgs)
			if args == nil {
				return nil
			}

			// Create function call
			call := &FunctionCall{
				Name: fnDef.Name,
				Args: args,
				fn:   fnToUse,
			}

			// Special validation for async::start calls
			if mod.Path() == "ard/async" && s.Function.Name == "start" {
				return c.validateFiberFunction(s.Function.Args[0].Value, mod.Get("Fiber").Type)
			}
			// Special validation for async::eval calls
			if mod.Path() == "ard/async" && s.Function.Name == "eval" {
				return c.validateAsyncEval(s.Function.Args[0].Value)
			}

			return &ModuleFunctionCall{
				Module: mod.Path(),
				Call:   call,
			}
		}
	case *parse.IfStatement:
		{
			cond := c.checkExpr(s.Condition)
			if cond == nil {
				return nil
			}
			if cond.Type() != Bool {
				c.addError("If conditions must be boolean expressions", s.GetLocation())
				return nil
			}

			body := c.checkBlock(s.Body, nil)

			var elseIf *If
			var elseBody *Block

			// does not recurse. reach into AST for each level since it's fixed
			if s.Else != nil {
				next := s.Else.(*parse.IfStatement)
				if next.Condition != nil {
					cond := c.checkExpr(next.Condition)
					if cond == nil {
						return nil
					}
					if cond.Type() != Bool {
						c.addError("If conditions must be boolean expressions", next.GetLocation())
						return nil
					}

					elseIfBody := c.checkBlock(next.Body, nil)
					if elseIfBody.Type() != body.Type() {
						c.addError("All branches must have the same result type", next.GetLocation())
						return nil
					}

					elseIf = &If{
						Condition: cond,
						Body:      elseIfBody,
					}

					if next, ok := next.Else.(*parse.IfStatement); ok {
						elseBody = c.checkBlock(next.Body, nil)
					}
				} else {
					b := c.checkBlock(next.Body, nil)
					if b.Type() != body.Type() {
						c.addError("All branches must have the same result type", next.GetLocation())
						return nil
					}
					elseBody = b
				}
			}

			return &If{
				Condition: cond,
				Body:      body,
				ElseIf:    elseIf,
				Else:      elseBody,
			}
		}
	case *parse.FunctionDeclaration:
		return c.checkFunction(s, nil)
	case *parse.ExternalFunction:
		return c.checkExternalFunction(s)
	case *parse.AnonymousFunction:
		{
			// Resolve parameters and return type (no type context for inference)
			params := c.resolveParametersWithContext(s.Parameters, nil)
			returnType := c.resolveReturnTypeWithContext(s.ReturnType, nil)

			// Create function definition
			uniqueName := fmt.Sprintf("anon_func_%p", s)
			fn := &FunctionDef{
				Name:       uniqueName,
				Parameters: params,
				ReturnType: returnType,
				Body:       nil,
			}

			// Check body (anonymous functions use equal, not areCompatible)
			body := c.checkBlock(s.Body, func() {
				c.scope.expectReturn(returnType)
				for _, param := range params {
					c.scope.add(param.Name, param.Type, param.Mutable)
				}
			})

			// Add function to scope after body is checked (for generic resolution support)
			c.scope.add(uniqueName, fn, false)

			// Validate return type using equal (stricter than areCompatible)
			if returnType != Void && !returnType.equal(body.Type()) {
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
		return c.checkList(nil, s)
	case *parse.MapLiteral:
		return c.checkMap(nil, s)
	case *parse.MatchExpression:
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

			// Process the cases
			for _, matchCase := range s.Cases {
				// Check if it's the default case (_)
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok {
					if id.Name == "_" {
						// This is the None case
						noneBody = c.checkBlock(matchCase.Body, nil)
					} else {
						// This is the Some case with a variable binding
						// Create a new scope for the body with the pattern bound to the unwrapped value
						someBody = c.checkBlock(matchCase.Body, func() {
							// Add the pattern name as a variable in the scope with the inner type
							// For example, if the Maybe is Str?, the pattern should be a Str
							c.scope.add(id.Name, maybeType.of, false)
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

			// Create the OptionMatch
			return &OptionMatch{
				Subject:   subject,
				InnerType: maybeType.of,
				Some: &Match{
					Pattern: patternIdent,
					Body:    someBody,
				},
				None: noneBody,
			}
		}

		// For Enum types, generate an EnumMatch
		if enumType, ok := subject.Type().(*Enum); ok {
			// Map to track which variants we've seen
			seenVariants := make(map[string]bool)
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
						catchAllBody = c.checkBlock(matchCase.Body, nil)
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

					// Check for duplicate cases
					if seenVariants[variantName] {
						c.addError(fmt.Sprintf("Duplicate case: %s::%s", enumType.Name, variantName), staticProp.GetLocation())
						continue
					}
					seenVariants[variantName] = true

					// Check the body for this case
					body := c.checkBlock(matchCase.Body, nil)
					cases[variantIndex] = body
				} else {
					c.addError("Pattern in enum match must be an enum variant or wildcard", matchCase.Pattern.GetLocation())
					return nil
				}
			}

			// Check if the match is exhaustive
			if !hasCatchAll {
				for i, value := range enumType.Values {
					if cases[i] == nil {
						c.addError(fmt.Sprintf("Incomplete match: missing case for '%s::%s'", enumType.Name, value.Name), s.GetLocation())
					}
				}
			}

			// Ensure all cases return the same type
			if len(cases) > 0 {
				// Find the first non-nil case to use as reference type
				var referenceType Type
				for _, caseBody := range cases {
					if caseBody != nil {
						referenceType = caseBody.Type()
						break
					}
				}

				if referenceType != nil {
					if catchAllBody != nil && !referenceType.equal(catchAllBody.Type()) {
						c.addError(typeMismatch(referenceType, catchAllBody.Type()), s.GetLocation())
						return nil
					}

					for _, caseBody := range cases {
						if caseBody != nil && !referenceType.equal(caseBody.Type()) {
							c.addError(typeMismatch(referenceType, caseBody.Type()), s.GetLocation())
							return nil
						}
					}
				}
			}

			// Create the EnumMatch
			enumMatch := &EnumMatch{
				Subject:  subject,
				Cases:    cases,
				CatchAll: catchAllBody,
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
					body := c.checkBlock(matchCase.Body, nil)

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

			// Ensure both branches return the same type
			if !trueBody.Type().equal(falseBody.Type()) {
				c.addError(typeMismatch(trueBody.Type(), falseBody.Type()), s.GetLocation())
				return nil
			}

			// Create and return the BoolMatch
			return &BoolMatch{
				Subject: subject,
				True:    trueBody,
				False:   falseBody,
			}
		}

		// For Union types, generate a UnionMatch
		if unionType, ok := subject.Type().(*Union); ok {
			// Track which union types we've seen and their corresponding bodies
			typeCases := make(map[string]*Match)
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
					if p.Name != "_" {
						c.addError("Catch-all case should be matched with '_'", matchCase.Pattern.GetLocation())
					} else {
						if catchAllBody != nil {
							c.addWarning("Duplicate catch-all case", matchCase.Pattern.GetLocation())
						} else {
							catchAllBody = c.checkBlock(matchCase.Body, nil)
						}
					}
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
						body := c.checkBlock(matchCase.Body, func() {
							c.scope.add(varName, matchedType, false)
						})
						typeCases[typeName] = &Match{
							Pattern: &Identifier{Name: varName},
							Body:    body,
						}
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

			// Ensure all cases return the same type
			var referenceType Type
			for _, caseBody := range typeCases {
				if caseBody != nil {
					referenceType = caseBody.Body.Type()
					break
				}
			}

			if referenceType != nil {
				if catchAllBody != nil && !referenceType.equal(catchAllBody.Type()) {
					c.addError(typeMismatch(referenceType, catchAllBody.Type()), s.GetLocation())
					return nil
				}

				for _, caseBody := range typeCases {
					if caseBody != nil && !referenceType.equal(caseBody.Body.Type()) {
						c.addError(typeMismatch(referenceType, caseBody.Body.Type()), s.GetLocation())
						return nil
					}
				}
			}

			// Create and return the UnionMatch
			return &UnionMatch{
				Subject:   subject,
				TypeCases: typeCases,
				CatchAll:  catchAllBody,
			}
		}

		if resultType, ok := subject.Type().(*Result); ok {
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
								Body: c.checkBlock(node.Body, func() {
									c.scope.add("ok", resultType.Val(), false)
								}),
							}
						case "err":
							errCase = &Match{
								Pattern: &Identifier{Name: "err"},
								Body: c.checkBlock(node.Body, func() {
									c.scope.add("err", resultType.Err(), false)
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
								Body: c.checkBlock(node.Body, func() {
									c.scope.add(varName, resultType.Val(), false)
								}),
							}
						case "err":
							errCase = &Match{
								Pattern: &Identifier{Name: varName},
								Body: c.checkBlock(node.Body, func() {
									c.scope.add(varName, resultType.Err(), false)
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

			return &ResultMatch{
				Subject: subject,
				Ok:      okCase,
				Err:     errCase,
			}
		}

		// Check for Int matching
		if subject.Type() == Int {
			intCases := make(map[int]*Block)
			rangeCases := make(map[IntRange]*Block)
			var catchAll *Block

			for _, matchCase := range s.Cases {
				// Check if it's the default case (_)
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok && id.Name == "_" {
					catchAll = c.checkBlock(matchCase.Body, nil)
				} else if literal, ok := matchCase.Pattern.(*parse.NumLiteral); ok {
					// Convert string to int
					value, err := strconv.Atoi(literal.Value)
					if err != nil {
						c.addError(fmt.Sprintf("Invalid integer literal: %s", literal.Value), matchCase.Pattern.GetLocation())
						return nil
					}
					caseBlock := c.checkBlock(matchCase.Body, nil)
					intCases[value] = caseBlock
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
						caseBlock := c.checkBlock(matchCase.Body, nil)
						intCases[negativeValue] = caseBlock
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

					caseBlock := c.checkBlock(matchCase.Body, nil)
					rangeCases[IntRange{Start: startValue, End: endValue}] = caseBlock
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
					caseBlock := c.checkBlock(matchCase.Body, nil)
					intCases[value] = caseBlock
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
			}
		}

		c.addError(fmt.Sprintf("Cannot match on %s", subject.Type()), s.GetLocation())
		return nil
	case *parse.ConditionalMatchExpression:
		var cases []ConditionalCase
		var catchAll *Block
		var referenceType Type

		for _, matchCase := range s.Cases {
			if matchCase.Condition == nil {
				// This is a catch-all case (_)
				if catchAll != nil {
					c.addError("Duplicate catch-all case", matchCase.GetLocation())
				} else {
					catchAll = c.checkBlock(matchCase.Body, nil)
					if referenceType == nil {
						referenceType = catchAll.Type()
					} else if !referenceType.equal(catchAll.Type()) {
						c.addError(fmt.Sprintf("All cases must return the same type. Expected %s, got %s", referenceType.String(), catchAll.Type().String()), matchCase.GetLocation())
					}
				}
			} else {
				// Regular condition case
				if condition := c.checkExpr(matchCase.Condition); condition != nil {
					// Ensure condition is boolean
					if condition.Type() != Bool {
						c.addError(fmt.Sprintf("Condition must be of type Bool, got %s", condition.Type().String()), matchCase.Condition.GetLocation())
					}

					body := c.checkBlock(matchCase.Body, nil)
					if referenceType == nil {
						referenceType = body.Type()
					} else if !referenceType.equal(body.Type()) {
						c.addError(fmt.Sprintf("All cases must return the same type. Expected %s, got %s", referenceType.String(), body.Type().String()), matchCase.GetLocation())
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
			Cases:    cases,
			CatchAll: catchAll,
		}
	case *parse.StaticProperty:
		{
			if id, ok := s.Target.(*parse.Identifier); ok {
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
						for name, t := range structType.Fields {
							fieldTypes[name] = t
						}

						return &ModuleStructInstance{
							Module:     mod.Path(),
							Property:   instance,
							FieldTypes: fieldTypes,
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

				var variant int8 = -1
				for i := range enum.Values {
					if enum.Values[i].Name == s.Property.(*parse.Identifier).Name {
						variant = int8(i)
						break
					}
				}
				if variant == -1 {
					c.addError(fmt.Sprintf("Undefined: %s::%s", sym.Name, s.Property.(*parse.Identifier).Name), id.GetLocation())
					return nil
				}

				return &EnumVariant{enum: enum, Variant: variant}
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
					var variant int8 = -1
					for i := range enum.Values {
						if enum.Values[i].Name == s.Property.(*parse.Identifier).Name {
							variant = int8(i)
							break
						}
					}
					if variant == -1 {
						c.addError(fmt.Sprintf("Undefined: %s::%s", enum.Name, s.Property.(*parse.Identifier).Name), s.Property.GetLocation())
						return nil
					}

					return &EnumVariant{enum: enum, Variant: variant}
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
							expr: expr,
							ok:   _type.val,
							Kind: TryResult,
						}
					}

					// Error types must match for direct propagation
					if !_type.err.equal(fnReturnResult.err) {
						c.addError(fmt.Sprintf("Error type mismatch: Expected %s, got %s", fnReturnResult.err.String(), _type.err.String()), s.Expression.GetLocation())
						// Return a try op with the unwrapped type to avoid cascading errors
						return &TryOp{
							expr: expr,
							ok:   _type.val,
							Kind: TryResult,
						}
					}

					// Success: returns the unwrapped value
					// Error: early returns the error wrapped in the function's Result type
					return &TryOp{
						expr: expr,
						ok:   _type.val,
						Kind: TryResult,
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
							expr: expr,
							ok:   _type.of,
							Kind: TryMaybe,
						}
					}

					// When try fails on Maybe, it early returns none wrapped in the function's Maybe type
					// The inner types don't need to match since we're not directly returning the unwrapped value
					// The unwrapped value (on success) can be any type and will be used in subsequent computations
					_ = fnReturnMaybe // We just need to verify the function returns a Maybe type

					// Success: returns the unwrapped value
					// None: early returns none wrapped in the function's Maybe type
					return &TryOp{
						expr: expr,
						ok:   _type.of,
						Kind: TryMaybe,
					}
				}
			default:
				c.addError("try can only be used on Result or Maybe types, got: "+expr.Type().String(), s.Expression.GetLocation())
				// Return a try op with the expr type to avoid cascading errors
				return &TryOp{
					expr: expr,
					ok:   expr.Type(),
					Kind: TryResult, // Default to Result, though this is an error path
				}
			}
		}
	case *parse.BlockExpression:
		{
			// Check block statements in the current scope
			block := c.checkBlock(s.Statements, nil)
			return block
		}
	default:
		panic(fmt.Errorf("Unexpected expression: %s", reflect.TypeOf(s)))
	}
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
func (c *Checker) checkExprAs(expr parse.Expression, expectedType Type) Expression {
	switch s := (expr).(type) {
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
				Name:       uniqueName,
				Parameters: params,
				ReturnType: returnType,
				Body:       nil,
			}

			// Check body
			body := c.checkBlock(s.Body, func() {
				c.scope.expectReturn(returnType)
				for _, param := range params {
					c.scope.add(param.Name, param.Type, param.Mutable)
				}
			})

			// Add function to scope after checking body
			c.scope.add(uniqueName, fn, false)

			// Validate return type
			if returnType != Void && !returnType.equal(body.Type()) {
				c.addError(typeMismatch(returnType, body.Type()), s.GetLocation())
				return nil
			}

			fn.Body = body
			return fn
		}
	case *parse.StaticFunction:
		{
			resultType, expectResult := expectedType.(*Result)
			if !expectResult &&
				s.Target.(*parse.Identifier).Name != "Result" &&
				(s.Function.Name != "ok" && s.Function.Name != "err") {
				return c.checkExpr(s)
			}

			moduleName := s.Target.(*parse.Identifier).Name
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
			case *ExternalFunctionDef:
				// Convert ExternalFunctionDef to FunctionDef for validation
				fnDef = &FunctionDef{
					Name:       fn.Name,
					Parameters: fn.Parameters,
					ReturnType: fn.ReturnType,
					Private:    fn.Private,
				}
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
				if !resultType.Val().equal(arg.Type()) {
					c.addError(typeMismatch(resultType.Val(), arg.Type()), s.Function.Args[0].Value.GetLocation())
					return nil
				}
			}
			if fnDef.name() == "err" {
				arg = c.checkExpr(s.Function.Args[0].Value)
				if !resultType.Err().equal(arg.Type()) {
					c.addError(typeMismatch(resultType.Err(), arg.Type()), s.Function.Args[0].Value.GetLocation())
					return nil
				}
			}

			fnDef.ReturnType = resultType
			return &ModuleFunctionCall{
				Module: mod.Path(),
				Call: &FunctionCall{
					Name: fnDef.name(),
					Args: []Expression{arg},
					fn:   fnDef,
				},
			}
		}
	}

	checked := c.checkExpr(expr)
	if checked == nil {
		return nil
	}

	if !expectedType.equal(checked.Type()) {
		c.addError(typeMismatch(expectedType, checked.Type()), expr.GetLocation())
		return nil
	}

	return checked
}

func (c *Checker) checkExternalFunction(def *parse.ExternalFunction) *ExternalFunctionDef {
	// Check for duplicate function names
	if _, dup := c.scope.get(def.Name); dup {
		c.addError(fmt.Sprintf("Duplicate declaration: %s", def.Name), def.GetLocation())
		return nil
	}

	// Process parameters
	params := make([]Parameter, len(def.Parameters))
	for i, param := range def.Parameters {
		paramType := c.resolveType(param.Type)
		params[i] = Parameter{
			Name:    param.Name,
			Type:    paramType,
			Mutable: param.Mutable,
		}
	}

	// Resolve return type
	returnType := c.resolveType(def.ReturnType)

	// Validate external binding format and existence
	if def.ExternalBinding == "" {
		c.addError("External binding cannot be empty", def.GetLocation())
		return nil
	}

	// Create external function definition
	extFn := &ExternalFunctionDef{
		Name:            def.Name,
		Parameters:      params,
		ReturnType:      returnType,
		ExternalBinding: def.ExternalBinding,
		Private:         def.Private,
	}

	// Add to scope
	c.scope.add(def.Name, extFn, false)

	return extFn
}

// resolveParametersWithContext resolves parameter types, optionally inferring from an expected function type
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

	// Check function body
	body := c.checkBlock(bodyStmts, func() {
		// Set the expected return type to the scope
		c.scope.expectReturn(returnType)
		// Add parameters to scope
		for _, param := range params {
			c.scope.add(param.Name, param.Type, param.Mutable)
		}
	})

	// Check that the function's return type matches its body's type
	if returnType != Void && !returnType.equal(body.Type()) {
		c.addError(typeMismatch(returnType, body.Type()), location)
	}

	return body
}

func (c *Checker) checkFunction(def *parse.FunctionDeclaration, init func()) *FunctionDef {
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
		Name:       def.Name,
		Parameters: params,
		ReturnType: returnType,
		Body:       nil,
		Private:    def.Private,
	}

	// Add function to scope before checking body (for recursion support)
	// For methods (when init != nil), only add within the body scope
	if init == nil {
		c.scope.add(def.Name, fn, false)
	}

	body := c.checkBlock(def.Body, func() {
		c.scope.expectReturn(returnType)
		for _, param := range params {
			c.scope.add(param.Name, param.Type, param.Mutable)
		}
	})

	// Validate return type - named functions use areCompatible, anonymous use equal
	if returnType != Void && !areCompatible(returnType, body.Type()) {
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
			Name:       typ.Name,
			Parameters: substitutedParams,
			ReturnType: substituteType(typ.ReturnType, typeMap),
			Body:       typ.Body,
			Mutates:    typ.Mutates,
			Private:    typ.Private,
		}
	// Handle other compound types
	default:
		return t
	}
}

// alignArgumentsWithParameters converts a list of (possibly named) arguments to positional form,
// aligned with function parameters and preserving mutability information.
func (c *Checker) alignArgumentsWithParameters(args []parse.Argument, params []Parameter) []parse.Argument {
	resolvedArgs := make([]parse.Argument, len(params))

	if len(args) == 0 {
		return resolvedArgs
	}

	// If arguments have names, reorder them to match parameter positions
	if args[0].Name != "" {
		paramMap := make(map[string]int)
		for i, param := range params {
			paramMap[param.Name] = i
		}
		for _, arg := range args {
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
		// Positional arguments - direct copy
		copy(resolvedArgs, args)
	}

	return resolvedArgs
}

// setupFunctionGenerics sets up generic scope and function copy for generic functions.
// Returns the function copy (with fresh TypeVar instances for generics) and the generic scope.
// For non-generic functions, returns the original function and a nil scope.
func (c *Checker) setupFunctionGenerics(fnDef *FunctionDef) (*FunctionDef, *SymbolTable) {
	if !fnDef.hasGenerics() {
		return fnDef, nil
	}

	// Extract generic parameter names
	genericNames := make(map[string]bool)
	for _, param := range fnDef.Parameters {
		extractGenericNames(param.Type, genericNames)
	}
	extractGenericNames(fnDef.ReturnType, genericNames)

	genericParams := make([]string, 0, len(genericNames))
	for name := range genericNames {
		genericParams = append(genericParams, name)
	}

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
		},
	}
}

// checkAndProcessArguments validates and type-checks function arguments with generic support.
// Returns the processed arguments and the specialized function (with generics resolved if applicable).
// Handles the `mut` keyword for explicit copy semantics on mutable parameters.
// Synthesizes maybe::none() calls for omitted nullable arguments.
// If any error occurs, it's added to the checker's diagnostics.
func (c *Checker) checkAndProcessArguments(fnDef *FunctionDef, resolvedArgs []parse.Argument, resolvedExprs []parse.Expression, fnDefCopy *FunctionDef, genericScope *SymbolTable, numOmittedArgs int) ([]Expression, *FunctionDef) {
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
		switch resolvedExprs[i].(type) {
		case *parse.ListLiteral, *parse.MapLiteral:
			checkedArg = c.checkExprAs(resolvedExprs[i], expectedType)
		case *parse.AnonymousFunction:
			checkedArg = c.checkExprAs(resolvedExprs[i], expectedType)
		default:
			checkedArg = c.checkExpr(resolvedExprs[i])
		}

		if checkedArg == nil {
			return nil, nil
		}

		// Check if we need to wrap the argument in maybe::some() for nullable parameters
		// If parameter is Maybe<T> and argument is T, wrap it
		if maybeParam, isMaybe := paramType.(*Maybe); isMaybe {
			if argType := checkedArg.Type(); !argType.equal(paramType) {
				// Check if argument type matches the inner Maybe type
				if maybeParam.Of().equal(argType) {
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
			if !areCompatible(paramType, checkedArg.Type()) {
				c.addError(typeMismatch(paramType, checkedArg.Type()), resolvedExprs[i].GetLocation())
				return nil, nil
			}
		}

		// Check mutability constraints if needed
		if fnDefCopy.Parameters[i].Mutable {
			if c.isMutable(checkedArg) {
				// Argument is already mutable - use it directly
				allExprs[i] = checkedArg
			} else if i < len(resolvedArgs) && resolvedArgs[i].Mutable {
				// User provided `mut` keyword - create a copy
				// (omitted arguments won't have a resolvedArgs entry)
				allExprs[i] = &CopyExpression{
					Expr:  checkedArg,
					Type_: checkedArg.Type(),
				}
			} else {
				// Argument is not mutable and no `mut` keyword - error
				c.addError(fmt.Sprintf("Type mismatch: Expected a mutable %s", fnDefCopy.Parameters[i].Type.String()), resolvedExprs[i].GetLocation())
				return nil, nil
			}
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
	if fnDef.hasGenerics() && genericScope != nil {
		bindings := genericScope.getGenericBindings()

		if len(bindings) == 0 {
			// No generics were bound, use original function
			fnToUse = fnDef
		} else {
			// Create specialized function with resolved generics
			fnToUse = &FunctionDef{
				Name:       fnDefCopy.Name,
				Parameters: make([]Parameter, len(fnDefCopy.Parameters)),
				ReturnType: substituteType(fnDefCopy.ReturnType, bindings),
				Body:       fnDefCopy.Body,
				Mutates:    fnDefCopy.Mutates,
				Private:    fnDefCopy.Private,
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

	return allExprs, fnToUse
}

// New generic resolution using the enhanced symbol table
func (c *Checker) resolveGenericFunction(fnDef *FunctionDef, args []Expression, typeArgs []parse.DeclaredType, _ parse.Location) (*FunctionDef, error) {
	if !fnDef.hasGenerics() {
		return fnDef, nil
	}

	// Extract generic parameter names (unique)
	genericNames := make(map[string]bool)
	for _, param := range fnDef.Parameters {
		extractGenericNames(param.Type, genericNames)
	}
	extractGenericNames(fnDef.ReturnType, genericNames)

	genericParams := make([]string, 0, len(genericNames))
	for name := range genericNames {
		genericParams = append(genericParams, name)
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

		for i, arg := range typeArgs {
			actual := c.resolveType(arg)
			if actual == nil {
				return nil, fmt.Errorf("Could not resolve type argument")
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
		Name:       fnDefCopy.Name,
		Parameters: make([]Parameter, len(fnDefCopy.Parameters)),
		ReturnType: substituteType(fnDefCopy.ReturnType, bindings),
		Body:       fnDefCopy.Body,
		Mutates:    fnDefCopy.Mutates,
		Private:    fnDefCopy.Private,
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
	switch expectedType := expected.(type) {
	case *TypeVar:
		// Generic type - bind it to the actual type using in-place mutation.
		// This mutates expectedType.bound and expectedType.actual directly.
		return genericScope.bindGeneric(expectedType.name, actual)
	case *FunctionDef:
		// Function type unification - handle both FunctionDef and ExternalFunctionDef
		var actualParams []Parameter
		var actualReturnType Type

		if actualFn, ok := actual.(*FunctionDef); ok {
			actualParams = actualFn.Parameters
			actualReturnType = actualFn.ReturnType
		} else if actualFn, ok := actual.(*ExternalFunctionDef); ok {
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
		if left.Type() == Int || c.isEnum(left.Type()) {
			return &IntGreater{left, right}
		}
		if left.Type() == Float {
			return &FloatGreater{left, right}
		}
		c.addError("The '>' operator can only be used for Int or Float", leftExpr.GetLocation())
		return nil

	case parse.GreaterThanOrEqual:
		if left.Type() == Int || c.isEnum(left.Type()) {
			return &IntGreaterEqual{left, right}
		}
		if left.Type() == Float {
			return &FloatGreaterEqual{left, right}
		}
		c.addError("The '>=' operator can only be used for Int or Float", leftExpr.GetLocation())
		return nil

	case parse.LessThan:
		if left.Type() == Int || c.isEnum(left.Type()) {
			return &IntLess{left, right}
		}
		if left.Type() == Float {
			return &FloatLess{left, right}
		}
		c.addError("The '<' operator can only be used for Int or Float", leftExpr.GetLocation())
		return nil

	case parse.LessThanOrEqual:
		if left.Type() == Int || c.isEnum(left.Type()) {
			return &IntLessEqual{left, right}
		}
		if left.Type() == Float {
			return &FloatLessEqual{left, right}
		}
		c.addError("The '<=' operator can only be used for Int or Float", leftExpr.GetLocation())
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

		sig := innerType.get(p.Method.Name)
		if sig == nil {
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
