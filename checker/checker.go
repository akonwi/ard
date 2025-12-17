package checker

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/akonwi/ard/ast"
)

type Program struct {
	Imports    map[string]Module
	Statements []Statement
}

type Module interface {
	Path() string
	Get(name string) Symbol
	Program() *Program
	TypeRegistry() *TypeRegistry
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
	location ast.Location
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s %s %s", d.filePath, d.location.Start, d.Message)
}

// LookupType retrieves the type of an expression from the registry.
// Phase 4: Registry-based type lookup with fallback to computed types.
// This is the strangler fig transition: we look up from the registry first,
// and fall back to the traditional Type() method if the registry doesn't have it.
func (c *Checker) LookupType(expr Expression) Type {
	if expr == nil {
		return nil
	}
	
	typeID := expr.GetTypeID()
	if typeID != InvalidTypeID {
		if t := c.types.Lookup(typeID); t != nil {
			return t
		}
	}
	
	// Fallback to computed type during transition
	// This happens during initial expression checking when Type() is called
	// before registerExpr() is called
	return expr.Type()
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
	input          *ast.Program
	scope          *SymbolTable
	filePath       string
	program        *Program
	halted         bool
	moduleResolver *ModuleResolver

	types *TypeRegistry
}

func New(filePath string, input *ast.Program, moduleResolver *ModuleResolver) *Checker {
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
		types: NewTypeRegistry(),
	}

	return c
}

func (c *Checker) HasErrors() bool {
	return len(c.diagnostics) > 0
}

func (c *Checker) Diagnostics() []Diagnostic {
	return c.diagnostics
}

// registerType registers a type in the type registry and returns the assigned ID
func (c *Checker) registerType(t Type) TypeID {
	if t == nil {
		return InvalidTypeID
	}
	id := c.types.Next()
	if err := c.types.Register(id, t); err != nil {
		// This should never happen in normal operation, but log it if it does
		c.addError(fmt.Sprintf("Internal error registering type: %v", err), ast.Location{})
		return InvalidTypeID
	}
	return id
}

// registerExpr registers an expression's type in the registry and sets its typeID
// This is a helper to keep registration logic centralized
func (c *Checker) registerExpr(expr Expression) Expression {
	if expr == nil {
		return nil
	}
	typeID := c.registerType(expr.Type())
	// Set typeID on the expression struct if it has that field
	// This is done via reflection since not all expressions may have it initialized yet
	setTypeID(expr, typeID)
	return expr
}

// setTypeID sets the typeID field on an expression via reflection
func setTypeID(expr interface{}, id TypeID) {
	switch e := expr.(type) {
	case *StrLiteral:
		e.typeID = id
	case *TemplateStr:
		e.typeID = id
	case *BoolLiteral:
		e.typeID = id
	case *VoidLiteral:
		e.typeID = id
	case *IntLiteral:
		e.typeID = id
	case *FloatLiteral:
		e.typeID = id
	case *ListLiteral:
		e.typeID = id
	case *MapLiteral:
		e.typeID = id
	case *Variable:
		e.typeID = id
	case *InstanceProperty:
		e.typeID = id
	case *InstanceMethod:
		e.typeID = id
	case *Negation:
		e.typeID = id
	case *Not:
		e.typeID = id
	case *IntAddition:
		e.typeID = id
	case *IntSubtraction:
		e.typeID = id
	case *IntMultiplication:
		e.typeID = id
	case *IntDivision:
		e.typeID = id
	case *IntModulo:
		e.typeID = id
	case *IntGreater:
		e.typeID = id
	case *IntGreaterEqual:
		e.typeID = id
	case *IntLess:
		e.typeID = id
	case *IntLessEqual:
		e.typeID = id
	case *FloatAddition:
		e.typeID = id
	case *FloatSubtraction:
		e.typeID = id
	case *FloatMultiplication:
		e.typeID = id
	case *FloatDivision:
		e.typeID = id
	case *FloatGreater:
		e.typeID = id
	case *FloatGreaterEqual:
		e.typeID = id
	case *FloatLess:
		e.typeID = id
	case *FloatLessEqual:
		e.typeID = id
	case *StrAddition:
		e.typeID = id
	case *Equality:
		e.typeID = id
	case *And:
		e.typeID = id
	case *Or:
		e.typeID = id
	case *OptionMatch:
		e.typeID = id
	case *EnumMatch:
		e.typeID = id
	case *BoolMatch:
		e.typeID = id
	case *IntMatch:
		e.typeID = id
	case *UnionMatch:
		e.typeID = id
	case *ConditionalMatch:
		e.typeID = id
	case *FunctionCall:
		e.typeID = id
	case *ModuleFunctionCall:
		e.typeID = id
	case *StructInstance:
		e.typeID = id
	case *ResultMatch:
		e.typeID = id
	case *Panic:
		e.typeID = id
	case *TryOp:
		e.typeID = id
	case *CopyExpression:
		e.typeID = id
	}
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

// This should only be called after .Check()
// The returned module could be problematic if there are diagnostic errors.
func (c *Checker) Module() Module {
	return NewUserModule(c.filePath, c.program, c.scope, c.types)
}

// check is an internal helper for recursive module checking.
// Use New() + Check() + Module() for the public API.
func check(input *ast.Program, moduleResolver *ModuleResolver, filePath string) (Module, []Diagnostic) {
	c := New(filePath, input, moduleResolver)

	c.Check()

	return c.Module(), c.diagnostics
}

func (c *Checker) addError(msg string, location ast.Location) {
	c.diagnostics = append(c.diagnostics, Diagnostic{
		Kind:     Error,
		Message:  msg,
		filePath: c.filePath,
		location: location,
	})
}

func (c *Checker) addWarning(msg string, location ast.Location) {
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
	case *Any:
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
		// This needs to handle a defined order, which map doesn't provide.
		// For now, this is not supported.
	}
}

func (c *Checker) specializeAliasedType(originalType Type, typeArgs []ast.DeclaredType, loc ast.Location) Type {
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

func (c *Checker) resolveType(t ast.DeclaredType) Type {
	var baseType Type
	switch ty := t.(type) {
	case *ast.StringType:
		baseType = Str
	case *ast.IntType:
		baseType = Int
	case *ast.FloatType:
		baseType = Float
	case *ast.BooleanType:
		baseType = Bool
	case *ast.VoidType:
		baseType = Void

	case *ast.FunctionType:
		// Convert each parameter type and return type
		params := make([]Parameter, len(ty.Params))
		for i, param := range ty.Params {
			params[i] = Parameter{
				Name: fmt.Sprintf("arg%d", i),
				Type: c.resolveType(param),
			}
		}
		returnType := c.resolveType(ty.Return)

		// Create a FunctionDef from the function type syntax
		baseType = &FunctionDef{
			Name:       "<function>",
			Parameters: params,
			ReturnType: returnType,
		}
	case *ast.List:
		of := c.resolveType(ty.Element)
		baseType = MakeList(of)
	case *ast.Map:
		key := c.resolveType(ty.Key)
		value := c.resolveType(ty.Value)
		baseType = MakeMap(key, value)
	case *ast.ResultType:
		val := c.resolveType(ty.Val)
		err := c.resolveType(ty.Err)
		baseType = MakeResult(val, err)
	case *ast.CustomType:
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
			mod := c.resolveModule(ty.Type.Target.(*ast.Identifier).Name)
			if mod != nil {
				// at some point, this will need to unwrap the property down to root for nested paths: `mod::sym::more`
				sym := mod.Get(ty.Type.Property.(*ast.Identifier).Name)
				if !sym.IsZero() {
					baseType = sym.Type
					break
				}
			}
		}
		c.addError(fmt.Sprintf("Unrecognized type: %s", t.GetName()), t.GetLocation())
		return &Any{name: "unknown"}
	case *ast.GenericType:
		if existing := c.scope.findGeneric(ty.Name); existing != nil {
			baseType = existing
		} else {
			baseType = &Any{name: ty.Name}
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

func (c *Checker) destructurePath(expr *ast.StaticFunction) (string, string) {
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

func typeMismatch(expected, got Type) string {
	exMsg := expected.String()
	if _, isTrait := expected.(*Trait); isTrait {
		exMsg = "implementation of " + exMsg
	}
	return fmt.Sprintf("Type mismatch: Expected %s, got %s", exMsg, got)
}

func (c *Checker) checkStmt(stmt *ast.Statement) *Statement {
	if c.halted {
		return nil
	}
	switch s := (*stmt).(type) {
	case *ast.Comment:
		return nil
	case *ast.Break:
		return &Statement{Break: true}
	case *ast.TraitDefinition:
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
	case *ast.TraitImplementation:
		{
			var sym Symbol
			switch name := s.Trait.(type) {
			case ast.Identifier:
				if s, ok := c.scope.get(name.Name); ok {
					sym = *s
				}
			case ast.StaticProperty:
				mod := c.resolveModule(name.Target.(*ast.Identifier).Name)
				if mod != nil {
					if propId, ok := name.Property.(*ast.Identifier); ok {
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
	case *ast.TypeDeclaration:
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
	case *ast.VariableDeclaration:
		{
			var val Expression
			if s.Type == nil {
				switch literal := s.Value.(type) {
				case *ast.ListLiteral:
					if expr := c.checkList(nil, literal); expr != nil {
						val = expr
					}
				case *ast.MapLiteral:
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
				case *ast.ListLiteral:
					if expr := c.checkList(expected, literal); expr != nil {
						val = expr
					}
				case *ast.MapLiteral:
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
			valType := c.LookupType(val)
			if s.Mutable && c.shouldCopyForMutableAssignment(valType) {
				// Always wrap in copy expression for mutable assignment of copyable types
				val = &CopyExpression{
					Expr:  val,
					Type_: valType,
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
	case *ast.VariableAssignment:
		{
			if id, ok := s.Target.(*ast.Identifier); ok {
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

				valueType := c.LookupType(value)
				if !target.Type.equal(valueType) {
					c.addError(typeMismatch(target.Type, valueType), s.Value.GetLocation())
					return nil
				}

				return &Statement{
					Stmt: &Reassignment{Target: &Variable{sym: *target}, Value: value},
				}
			}

			if ip, ok := s.Target.(*ast.InstanceProperty); ok {
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
	case *ast.WhileLoop:
		{
			// Check the condition expression
			var condition Expression
			if s.Condition == nil {
				condition = c.registerExpr(&BoolLiteral{Value: true})
			} else {
				condition = c.checkExpr(s.Condition)
			}

			// Condition must be a boolean expression
			if c.LookupType(condition) != Bool {
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
	case *ast.ForLoop:
		{
			// Create a new scope for the loop body and initialization
			parent := c.scope
			scope := makeScope(parent)
			c.scope = &scope
			defer func() {
				c.scope = parent
			}()

			// Check the initialization statement - handle it as a variable declaration
			initDeclStmt := ast.Statement(s.Init)
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
			if c.LookupType(condition) != Bool {
				c.addError("For loop condition must be a boolean expression", s.Condition.GetLocation())
				return nil
			}

			// Check the update statement - handle it as a variable assignment
			incrStmt := ast.Statement(s.Incrementer)
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
	case *ast.RangeLoop:
		{
			start, end := c.checkExpr(s.Start), c.checkExpr(s.End)
			if start == nil || end == nil {
				return nil
			}
			startType := c.LookupType(start)
			endType := c.LookupType(end)
			if startType != endType {
				c.addError(fmt.Sprintf("Invalid range: %s..%s", startType, endType), s.Start.GetLocation())
				return nil
			}

			if startType == Int {
				loop := &ForIntRange{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					Start:  start,
					End:    end,
				}
				body := c.checkBlock(s.Body, func() {
					c.scope.add(s.Cursor.Name, startType, false)
					if loop.Index != "" {
						c.scope.add(loop.Index, Int, false)
					}
				})
				loop.Body = body
				return &Statement{Stmt: loop}
			}

			panic(fmt.Errorf("Cannot create range of %s", startType))
		}
	case *ast.ForInLoop:
		{
			iterValue := c.checkExpr(s.Iterable)
			if iterValue == nil {
				return nil
			}

			iterType := c.LookupType(iterValue)
			// Handle strings specifically
			if iterType == Str {
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
			if iterType == Int {
				// This is syntax sugar for a range from 0 to n
				loop := &ForIntRange{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					Start:  c.registerExpr(&IntLiteral{Value: 0}).(*IntLiteral), // Start from 0
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

			if listType, ok := iterType.(*List); ok {
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

			if mapType, ok := iterType.(*Map); ok {
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
			c.addError(fmt.Sprintf("Cannot iterate over a %s", iterType), s.Iterable.GetLocation())
			return nil
		}
	case *ast.EnumDefinition:
		{
			if len(s.Variants) == 0 {
				c.addError("Enums must have at least one variant", s.GetLocation())
				return nil
			}

			// Check for duplicate variant names
			seenVariants := make(map[string]bool)
			for _, variant := range s.Variants {
				if seenVariants[variant] {
					c.addError(fmt.Sprintf("Duplicate variant: %s", variant), s.GetLocation())
					return nil
				}
				seenVariants[variant] = true
			}

			enum := &Enum{
				Private:  s.Private,
				Name:     s.Name,
				Variants: s.Variants,
				Methods:  make(map[string]*FunctionDef),
			}

			c.scope.add(enum.name(), enum, false)
			return nil
		}
	case *ast.StructDefinition:
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
	case *ast.ImplBlock:
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
		expr := c.checkExpr((ast.Expression)(*stmt))
		if expr == nil {
			return nil
		}
		return &Statement{Expr: expr}
	}
}

func (c *Checker) checkList(declaredType Type, expr *ast.ListLiteral) *ListLiteral {
	if declaredType != nil {
		expectedElementType := declaredType.(*List).of
		elements := make([]Expression, len(expr.Items))
		for i := range expr.Items {
			item := expr.Items[i]
			element := c.checkExpr(item)
			elementType := c.LookupType(element)
			if !expectedElementType.equal(elementType) {
				c.addError(typeMismatch(expectedElementType, elementType), item.GetLocation())
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

		elemType := c.LookupType(element)
		if i == 0 {
			elementType = elemType
		} else if !elementType.equal(elemType) {
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

func (c *Checker) checkBlock(stmts []ast.Statement, setup func()) *Block {
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

func (c *Checker) checkMap(declaredType Type, expr *ast.MapLiteral) *MapLiteral {
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
				Keys:   []Expression{},
				Values: []Expression{},
				_type:  mapType,
			}
		} else {
			// Empty map without a declared type is an error
			c.addError("Empty maps need an explicit type", expr.GetLocation())
			c.halted = true
			return &MapLiteral{_type: MakeMap(Void, Void), Keys: []Expression{}, Values: []Expression{}}
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
			keyType := c.LookupType(key)
			if !areCompatible(expectedKeyType, keyType) {
				c.addError(typeMismatch(expectedKeyType, keyType), entry.Key.GetLocation())
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
			valueType := c.LookupType(value)
			if !areCompatible(expectedValueType, valueType) {
				c.addError(typeMismatch(expectedValueType, valueType), entry.Value.GetLocation())
				hasError = true
				continue
			}
			values[i] = value
		}

		if hasError {
			return nil
		}

		return &MapLiteral{
			Keys:   keys,
			Values: values,
			_type:  mapType,
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

	keyType := c.LookupType(firstKey)
	valueType := c.LookupType(firstValue)
	keys[0] = firstKey
	values[0] = firstValue

	// Check that all entries have consistent types
	for i := 1; i < len(expr.Entries); i++ {
		key := c.checkExpr(expr.Entries[i].Key)
		if key == nil {
			keyType = Void
			continue
		}
		currentKeyType := c.LookupType(key)
		if !keyType.equal(currentKeyType) {
			c.addError(fmt.Sprintf("Map key type mismatch: Expected %s, got %s", keyType, currentKeyType), expr.Entries[i].Key.GetLocation())
			continue
		}
		keys[i] = key

		value := c.checkExpr(expr.Entries[i].Value)
		if value == nil {
			valueType = Void
			continue
		}
		currentValueType := c.LookupType(value)
		if !valueType.equal(currentValueType) {
			c.addError(fmt.Sprintf("Map value type mismatch: Expected %s, got %s", valueType, currentValueType), expr.Entries[i].Value.GetLocation())
			continue
		}
		values[i] = value
	}

	// Create and return the map
	return &MapLiteral{
		Keys:   keys,
		Values: values,
		_type:  MakeMap(keyType, valueType),
	}
}

// validateStructInstance validates struct instantiation and returns the instance or nil if errors
func (c *Checker) validateStructInstance(structType *StructDef, properties []ast.StructValue, structName string, loc ast.Location) *StructInstance {
	instance := &StructInstance{Name: structName, _type: structType}
	fields := make(map[string]Expression)

	// Check all provided properties
	for _, property := range properties {
		if field, ok := structType.Fields[property.Name.Name]; !ok {
			c.addError(fmt.Sprintf("Unknown field: %s", property.Name.Name), property.GetLocation())
		} else {
			if val := c.checkExprAs(property.Value, field); val != nil {
				fields[property.Name.Name] = val
			}
		}
	}

	// Check for missing required fields
	missing := []string{}
	for name, t := range structType.Fields {
		if _, exists := fields[name]; !exists {
			if _, isMaybe := t.(*Maybe); !isMaybe {
				// todo: distinguish between provided w/ error + missing to avoid creating 2 errors:
				// missing field + invalid expression for field
				missing = append(missing, name)
			}
		}
	}
	if len(missing) > 0 {
		c.addError(fmt.Sprintf("Missing field: %s", strings.Join(missing, ", ")), loc)
	}

	instance.Fields = fields
	return instance
}

func (c *Checker) checkExpr(expr ast.Expression) Expression {
	if c.halted {
		return nil
	}
	switch s := (expr).(type) {
	case *ast.StrLiteral:
		return c.registerExpr(&StrLiteral{Value: s.Value})
	case *ast.BoolLiteral:
		return c.registerExpr(&BoolLiteral{Value: s.Value})
	case *ast.VoidLiteral:
		return c.registerExpr(&VoidLiteral{})
	case *ast.NumLiteral:
		{
			stripped := strings.ReplaceAll(s.Value, "_", "")
			if strings.Contains(stripped, ".") {
				value, err := strconv.ParseFloat(s.Value, 64)
				if err != nil {
					c.addError(fmt.Sprintf("Invalid float: %s", s.Value), s.GetLocation())
					return c.registerExpr(&FloatLiteral{Value: 0.0})
				}
				return c.registerExpr(&FloatLiteral{Value: value})
			}
			value, err := strconv.Atoi(stripped)
			if err != nil {
				c.addError(fmt.Sprintf("Invalid int: %s", s.Value), s.GetLocation())
			}
			return c.registerExpr(&IntLiteral{Value: value})
		}
	case *ast.InterpolatedStr:
		{
			chunks := make([]Expression, len(s.Chunks))
			for i := range s.Chunks {
				cx := c.checkExpr(s.Chunks[i])
				if cx == nil {
					// Replace failed chunk with placeholder
					chunks[i] = c.registerExpr(&StrLiteral{Value: "<error>"})
					continue
				}
				if strMod := c.findModuleByPath("ard/string"); strMod != nil {
					toStringTrait := strMod.Get("ToString").Type.(*Trait)
					cxType := c.LookupType(cx)
					if !cxType.hasTrait(toStringTrait) {
						c.addError(typeMismatch(toStringTrait, cxType), s.Chunks[i].GetLocation())
						// Replace chunk that can't be converted to string with placeholder
						chunks[i] = c.registerExpr(&StrLiteral{Value: "<error>"})
						continue
					}
					chunks[i] = cx
				}
			}
			return c.registerExpr(&TemplateStr{Chunks: chunks})
		}
	case *ast.Identifier:
		if sym, ok := c.scope.get(s.Name); ok {
			return c.registerExpr(&Variable{sym: *sym})
		}
		c.addError(fmt.Sprintf("Undefined variable: %s", s.Name), s.GetLocation())
		c.halted = true
		return nil
	case *ast.FunctionCall:
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

				return c.registerExpr(&Panic{
					Message: message,
					node:    s,
				})
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
			if len(resolvedExprs) != len(fnDef.Parameters) {
				c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d",
					len(fnDef.Parameters), len(resolvedExprs)), s.GetLocation())
				return nil
			}

			// We need to also resolve the arguments with mutability info
			resolvedArgs := make([]ast.Argument, len(fnDef.Parameters))
			if len(s.Args) > 0 && s.Args[0].Name != "" {
				// Handle named arguments - need to reorder them
				paramMap := make(map[string]int)
				for i, param := range fnDef.Parameters {
					paramMap[param.Name] = i
				}
				for _, arg := range s.Args {
					if index, exists := paramMap[arg.Name]; exists {
						resolvedArgs[index] = ast.Argument{
							Location: arg.Location,
							Name:     "",
							Value:    arg.Value,
							Mutable:  arg.Mutable,
						}
					}
				}
			} else {
				// Positional arguments - direct copy
				copy(resolvedArgs, s.Args)
			}

			// Check and process arguments
			args := make([]Expression, len(resolvedArgs))
			for i, arg := range resolvedArgs {
				// Get the expected parameter type
				paramType := fnDef.Parameters[i].Type

				// For list and map literals, use checkExprAs to infer type from context
				var checkedArg Expression
				switch resolvedExprs[i].(type) {
				case *ast.ListLiteral, *ast.MapLiteral:
					checkedArg = c.checkExprAs(resolvedExprs[i], paramType)
				default:
					checkedArg = c.checkExpr(resolvedExprs[i])
				}

				if checkedArg == nil {
					return nil
				}

				// Type check the argument against the parameter type
				checkedArgType := c.LookupType(checkedArg)
				if !areCompatible(paramType, checkedArgType) {
					c.addError(typeMismatch(paramType, checkedArgType), resolvedExprs[i].GetLocation())
					return nil
				}

				// Check mutability constraints if needed
				if fnDef.Parameters[i].Mutable {
					if arg.Mutable {
						// User provided `mut` - create a copy
						args[i] = c.registerExpr(&CopyExpression{
							Expr:  checkedArg,
							Type_: checkedArgType,
						})
					} else if c.isMutable(checkedArg) {
						// Argument is already mutable
						args[i] = checkedArg
					} else {
						// Not mutable and no `mut` keyword - error
						c.addError(fmt.Sprintf("Type mismatch: Expected a mutable %s", fnDef.Parameters[i].Type.String()), resolvedExprs[i].GetLocation())
						return nil
					}
				} else {
					args[i] = checkedArg
				}
			}

			// Use new generic resolution system
			if fnDef.hasGenerics() {
				specialized, err := c.resolveGenericFunction(fnDef, args, s.TypeArgs, s.GetLocation())
				if err != nil {
					c.addError(err.Error(), s.GetLocation())
					return nil
				}

				return c.registerExpr(&FunctionCall{
					Name: s.Name,
					Args: args,
					fn:   specialized,
				})
			}

			// Create and return the function call node
			return c.registerExpr(&FunctionCall{
				Name: s.Name,
				Args: args,
				fn:   fnDef,
			})
		}
	case *ast.InstanceProperty:
		{
			subj := c.checkExpr(s.Target)
			if subj == nil {
				// panic(fmt.Errorf("Cannot access %s on nil", s.Property))
				return nil
			}

			subjType := c.LookupType(subj)
			propType := subjType.get(s.Property.Name)
			if propType == nil {
				c.addError(fmt.Sprintf("Undefined: %s.%s", subj, s.Property.Name), s.Property.GetLocation())
				return nil
			}
			return c.registerExpr(&InstanceProperty{
				Subject:  subj,
				Property: s.Property.Name,
				_type:    propType,
			})
		}
	case *ast.InstanceMethod:
		{
			subj := c.checkExpr(s.Target)
			if subj == nil {
				c.addError(fmt.Sprintf("Cannot access %s on Void", s.Method.Name), s.Method.GetLocation())
				return nil
			}

			subjType := c.LookupType(subj)
			if subjType == nil {
				panic(fmt.Errorf("Cannot access %+v on nil: %s", subj.(*Variable).sym, s.Target))
			}
			sig := subjType.get(s.Method.Name)
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

			// Align mutability information with parameters
			resolvedArgs := make([]ast.Argument, len(fnDef.Parameters))
			if len(s.Method.Args) > 0 && s.Method.Args[0].Name != "" {
				paramMap := make(map[string]int)
				for i, param := range fnDef.Parameters {
					paramMap[param.Name] = i
				}
				for _, arg := range s.Method.Args {
					if index, exists := paramMap[arg.Name]; exists {
						resolvedArgs[index] = ast.Argument{
							Location: arg.Location,
							Name:     "",
							Value:    arg.Value,
							Mutable:  arg.Mutable,
						}
					}
				}
			} else {
				copy(resolvedArgs, s.Method.Args)
			}

			// Check and process arguments
			args := make([]Expression, len(resolvedArgs))
			for i := range resolvedArgs {
				// Get the expected parameter type
				paramType := fnDef.Parameters[i].Type

				// For list and map literals, use checkExprAs to infer type from context
				var checkedArg Expression
				switch resolvedExprs[i].(type) {
				case *ast.ListLiteral, *ast.MapLiteral:
					checkedArg = c.checkExprAs(resolvedExprs[i], paramType)
				default:
					checkedArg = c.checkExpr(resolvedExprs[i])
				}

				if checkedArg == nil {
					return nil
				}

				// Type check the argument against the parameter type
				checkedArgType := c.LookupType(checkedArg)
				if !areCompatible(paramType, checkedArgType) {
					c.addError(typeMismatch(paramType, checkedArgType), resolvedExprs[i].GetLocation())
					return nil
				}

				// Check mutability constraints if needed
				if fnDef.Parameters[i].Mutable && !c.isMutable(checkedArg) {
					// Check if the type is copyable (structs for now)
					if c.isCopyable(checkedArgType) {
						// Wrap in copy expression instead of erroring
						args[i] = c.registerExpr(&CopyExpression{
							Expr:  checkedArg,
							Type_: checkedArgType,
						})
					} else {
						c.addError(fmt.Sprintf("Type mismatch: Expected a mutable %s", fnDef.Parameters[i].Type.String()), resolvedExprs[i].GetLocation())
					}
				} else {
					args[i] = checkedArg
				}
			}

			// Use new generic resolution system
			if fnDef.hasGenerics() {
				specialized, err := c.resolveGenericFunction(fnDef, args, s.Method.TypeArgs, s.GetLocation())
				if err != nil {
					c.addError(err.Error(), s.GetLocation())
					return nil
				}

				methodCall := &FunctionCall{
					Name: s.Method.Name,
					Args: args,
					fn:   specialized,
				}
				return c.registerExpr(&InstanceMethod{
					Subject: subj,
					Method:  c.registerExpr(methodCall).(*FunctionCall),
				})
			}

			// Create function call
			call := &FunctionCall{
				Name: s.Method.Name,
				Args: args,
				fn:   fnDef,
			}

			return c.registerExpr(&InstanceMethod{
				Subject: subj,
				Method:  c.registerExpr(call).(*FunctionCall),
			})
		}
	case *ast.UnaryExpression:
		{
			value := c.checkExpr(s.Operand)
			if value == nil {
				return nil
			}
			valueType := c.LookupType(value)
			if s.Operator == ast.Minus {
				if valueType != Int && valueType != Float {
					c.addError("Only numbers can be negated with '-'", s.GetLocation())
					return nil
				}
				return c.registerExpr(&Negation{Value: value})
			}

			if valueType != Bool {
				c.addError("Only booleans can be negated with 'not'", s.GetLocation())
				return nil
			}
			return c.registerExpr(&Not{Value: value})
		}
	case *ast.BinaryExpression:
		{
			switch s.Operator {
			case ast.Plus:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftType := c.LookupType(left)
					rightType := c.LookupType(right)
					if !leftType.equal(rightType) {
						c.addError("Cannot add different types", s.GetLocation())
						return nil
					}
					if leftType == Int {
						return c.registerExpr(&IntAddition{Left: left, Right: right})
					}
					if leftType == Float {
						return c.registerExpr(&FloatAddition{Left: left, Right: right})
					}
					if leftType == Str {
						return c.registerExpr(&StrAddition{Left: left, Right: right})
					}
					c.addError("The '-' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case ast.Minus:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftType := c.LookupType(left)
					rightType := c.LookupType(right)
					if leftType != rightType {
						c.addError("Cannot subtract different types", s.GetLocation())
						return nil
					}
					if leftType == Int {
						return c.registerExpr(&IntSubtraction{Left: left, Right: right})
					}
					if leftType == Float {
						return c.registerExpr(&FloatSubtraction{Left: left, Right: right})
					}
					c.addError("The '+' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case ast.Multiply:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftType := c.LookupType(left)
					rightType := c.LookupType(right)
					if leftType != rightType {
						c.addError("Cannot multiply different types", s.GetLocation())
						return nil
					}
					if leftType == Int {
						return c.registerExpr(&IntMultiplication{Left: left, Right: right})
					}
					if leftType == Float {
						return c.registerExpr(&FloatMultiplication{Left: left, Right: right})
					}
					c.addError("The '*' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case ast.Divide:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftType := c.LookupType(left)
					rightType := c.LookupType(right)
					if leftType != rightType {
						c.addError("Cannot divide different types", s.GetLocation())
						return nil
					}
					if leftType == Int {
						return c.registerExpr(&IntDivision{Left: left, Right: right})
					}
					if leftType == Float {
						return c.registerExpr(&FloatDivision{Left: left, Right: right})
					}
					c.addError("The '/' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case ast.Modulo:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftType := c.LookupType(left)
					rightType := c.LookupType(right)
					if leftType != rightType {
						c.addError("Cannot modulo different types", s.GetLocation())
						return nil
					}
					if leftType == Int {
						return c.registerExpr(&IntModulo{Left: left, Right: right})
					}
					c.addError("The '%' operator can only be used for Int", s.GetLocation())
					return nil
				}
			case ast.GreaterThan:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftType := c.LookupType(left)
					rightType := c.LookupType(right)
					if leftType != rightType {
						c.addError("Cannot compare different types", s.GetLocation())
						return nil
					}
					if leftType == Int {
						return c.registerExpr(&IntGreater{Left: left, Right: right})
					}
					if leftType == Float {
						return c.registerExpr(&FloatGreater{Left: left, Right: right})
					}
					c.addError("The '>' operator can only be used for Int", s.GetLocation())
					return nil
				}
			case ast.GreaterThanOrEqual:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftType := c.LookupType(left)
					rightType := c.LookupType(right)
					if leftType != rightType {
						c.addError("Cannot compare different types", s.GetLocation())
						return nil
					}
					if leftType == Int {
						return c.registerExpr(&IntGreaterEqual{Left: left, Right: right})
					}
					if leftType == Float {
						return c.registerExpr(&FloatGreaterEqual{Left: left, Right: right})
					}
					c.addError("The '>=' operator can only be used for Int", s.GetLocation())
					return nil
				}
			case ast.LessThan:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftType := c.LookupType(left)
					rightType := c.LookupType(right)
					if leftType != rightType {
						c.addError("Cannot compare different types", s.GetLocation())
						return nil
					}
					if leftType == Int {
						return c.registerExpr(&IntLess{Left: left, Right: right})
					}
					if leftType == Float {
						return c.registerExpr(&FloatLess{Left: left, Right: right})
					}
					c.addError("The '<' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case ast.LessThanOrEqual:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftType := c.LookupType(left)
					rightType := c.LookupType(right)
					if leftType != rightType {
						c.addError("Cannot compare different types", s.GetLocation())
						return nil
					}
					if leftType == Int {
						return c.registerExpr(&IntLessEqual{Left: left, Right: right})
					}
					if leftType == Float {
						return c.registerExpr(&FloatLessEqual{Left: left, Right: right})
					}
					c.addError("The '<=' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case ast.Equal:
				{
					left, right := c.checkExpr(s.Left), c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftType := c.LookupType(left)
					rightType := c.LookupType(right)
					if !leftType.equal(rightType) {
						c.addError(fmt.Sprintf("Invalid: %s == %s", leftType, rightType), s.GetLocation())
						return nil
					}

					isMaybe := func(val Type) bool {
						_, ok := val.(*Maybe)
						return ok
					}
					if isMaybe(leftType) {
						return c.registerExpr(&Equality{Left: left, Right: right})
					}
					allowedTypes := []Type{Int, Float, Str, Bool}
					if !slices.Contains(allowedTypes, leftType) || !slices.Contains(allowedTypes, rightType) {
						c.addError(fmt.Sprintf("Invalid: %s == %s", leftType, rightType), s.GetLocation())
						return nil
					}
					return c.registerExpr(&Equality{Left: left, Right: right})
				}
			case ast.And:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftType := c.LookupType(left)
					rightType := c.LookupType(right)
					if leftType != Bool || rightType != Bool {
						c.addError("The 'and' operator can only be used between Bools", s.GetLocation())
						return nil
					}

					return c.registerExpr(&And{Left: left, Right: right})
				}
			case ast.Or:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftType := c.LookupType(left)
					rightType := c.LookupType(right)
					if leftType != Bool || rightType != Bool {
						c.addError("The 'or' operator can only be used with Boolean values", s.GetLocation())
						return nil
					}

					return c.registerExpr(&Or{Left: left, Right: right})
				}
			default:
				panic(fmt.Errorf("Unexpected operator: %v", s.Operator))
			}
		}

	// [refactor] a lot of the function call checking can be extracted
	// - validate args and resolve generics
	case *ast.StaticFunction:
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
				resolvedArgs := make([]ast.Argument, len(fnDef.Parameters))
				if len(s.Function.Args) > 0 && s.Function.Args[0].Name != "" {
					paramMap := make(map[string]int)
					for i, param := range fnDef.Parameters {
						paramMap[param.Name] = i
					}
					for _, arg := range s.Function.Args {
						if index, exists := paramMap[arg.Name]; exists {
							resolvedArgs[index] = ast.Argument{
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
					case *ast.ListLiteral, *ast.MapLiteral:
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

			// Align mutability information with parameters
			resolvedArgs := make([]ast.Argument, len(fnDef.Parameters))
			if len(s.Function.Args) > 0 && s.Function.Args[0].Name != "" {
				paramMap := make(map[string]int)
				for i, param := range fnDef.Parameters {
					paramMap[param.Name] = i
				}
				for _, arg := range s.Function.Args {
					if index, exists := paramMap[arg.Name]; exists {
						resolvedArgs[index] = ast.Argument{
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
				case *ast.ListLiteral, *ast.MapLiteral:
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

			// Create function call
			call := &FunctionCall{
				Name: fnDef.Name,
				Args: args,
				fn:   fnDef,
			}

			// resolve generics
			if fnDef.hasGenerics() {
				specialized, err := c.resolveGenericFunction(fnDef, args, s.Function.TypeArgs, s.GetLocation())
				if err != nil {
					c.addError(err.Error(), s.GetLocation())
					return nil
				}

				call.fn = specialized
			}

			// Special validation for async::start calls
			if mod.Path() == "ard/async" && s.Function.Name == "start" {
				return c.validateFiberFunction(s.Function.Args[0].Value, mod.Get("Fiber").Type)
			}

			return &ModuleFunctionCall{
				Module: mod.Path(),
				Call:   call,
			}
		}
	case *ast.IfStatement:
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
				next := s.Else.(*ast.IfStatement)
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

					if next, ok := next.Else.(*ast.IfStatement); ok {
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
	case *ast.FunctionDeclaration:
		return c.checkFunction(s, nil)
	case *ast.ExternalFunction:
		return c.checkExternalFunction(s)
	case *ast.AnonymousFunction:
		{
			// Process parameters
			params := make([]Parameter, len(s.Parameters))
			for i, param := range s.Parameters {
				var paramType Type = Void
				if param.Type != nil {
					paramType = c.resolveType(param.Type)
				}

				params[i] = Parameter{
					Name:    param.Name,
					Type:    paramType,
					Mutable: param.Mutable,
				}
			}

			// Determine return type
			var returnType Type = Void
			if s.ReturnType != nil {
				returnType = c.resolveType(s.ReturnType)
			}

			// Create function definition early (before checking body)
			// Generate a unique name for the anonymous function
			uniqueName := fmt.Sprintf("anon_func_%p", s)

			fn := &FunctionDef{
				Name:       uniqueName,
				Parameters: params,
				ReturnType: returnType,
				Body:       nil, // Will be set after checking
			}

			// Add function to scope BEFORE checking body to support generic resolution
			c.scope.add(uniqueName, fn, false)

			// Check function body with a setup function that adds parameters to scope
			body := c.checkBlock(s.Body, func() {
				// set the expected return type to the scope
				c.scope.expectReturn(returnType)
				for _, param := range params {
					c.scope.add(param.Name, param.Type, param.Mutable)
				}
			})

			// Check that the function's return type matches its body's type
			if returnType != Void && !returnType.equal(body.Type()) {
				c.addError(typeMismatch(returnType, body.Type()), s.GetLocation())
				return nil
			}

			// Set the body now that it's been checked
			fn.Body = body

			return fn
		}
	case *ast.StaticFunctionDeclaration:
		fn := c.checkFunction(&s.FunctionDeclaration, nil)
		if fn != nil {
			fn.Name = s.Path.String()
			c.scope.add(fn.Name, fn, false)
		}

		return fn
	case *ast.ListLiteral:
		return c.checkList(nil, s)
	case *ast.MapLiteral:
		return c.checkMap(nil, s)
	case *ast.MatchExpression:
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
				if id, ok := matchCase.Pattern.(*ast.Identifier); ok {
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
				Subject: subject,
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
			cases := make([]*Block, len(enumType.Variants))
			var catchAllBody *Block

			// Process the cases
			for _, matchCase := range s.Cases {
				if id, ok := matchCase.Pattern.(*ast.Identifier); ok {
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
				if staticProp, ok := matchCase.Pattern.(*ast.StaticProperty); ok {
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
					variantName := enumType.Variants[enumVariant.Variant]
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
				for i, variant := range enumType.Variants {
					if cases[i] == nil {
						c.addError(fmt.Sprintf("Incomplete match: missing case for '%s::%s'", enumType.Name, variant), s.GetLocation())
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
				if id, ok := matchCase.Pattern.(*ast.Identifier); ok {
					if id.Name == "_" {
						// Catch-all cases aren't allowed for boolean matches
						c.addError("Catch-all case is not allowed for boolean matches", matchCase.Pattern.GetLocation())
						return nil
					}
				}

				// Handle boolean literal case
				if boolLit, ok := matchCase.Pattern.(*ast.BoolLiteral); ok {
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
				case *ast.Identifier:
					if p.Name != "_" {
						c.addError("Catch-all case should be matched with '_'", matchCase.Pattern.GetLocation())
					} else {
						if catchAllBody != nil {
							c.addWarning("Duplicate catch-all case", matchCase.Pattern.GetLocation())
						} else {
							catchAllBody = c.checkBlock(matchCase.Body, nil)
						}
					}
				case *ast.FunctionCall:
					varName := p.Args[0].Value.(*ast.Identifier).Name
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
				case *ast.Identifier:
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
				case *ast.FunctionCall: // use FunctionCall node as aliasing variable
					{
						varName := p.Args[0].Value.(*ast.Identifier).Name
						switch p.Name {
						case "ok":
							varName := p.Args[0].Value.(*ast.Identifier).Name
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
				if id, ok := matchCase.Pattern.(*ast.Identifier); ok && id.Name == "_" {
					catchAll = c.checkBlock(matchCase.Body, nil)
				} else if literal, ok := matchCase.Pattern.(*ast.NumLiteral); ok {
					// Convert string to int
					value, err := strconv.Atoi(literal.Value)
					if err != nil {
						c.addError(fmt.Sprintf("Invalid integer literal: %s", literal.Value), matchCase.Pattern.GetLocation())
						return nil
					}
					caseBlock := c.checkBlock(matchCase.Body, nil)
					intCases[value] = caseBlock
				} else if unaryExpr, ok := matchCase.Pattern.(*ast.UnaryExpression); ok && unaryExpr.Operator == ast.Minus {
					// Handle negative numbers like -1, -5, etc.
					if literal, ok := unaryExpr.Operand.(*ast.NumLiteral); ok {
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
				} else if rangeExpr, ok := matchCase.Pattern.(*ast.RangeExpression); ok {
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
	case *ast.ConditionalMatchExpression:
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
	case *ast.StaticProperty:
		{
			if id, ok := s.Target.(*ast.Identifier); ok {
				// Check if this is accessing a module
				if mod := c.resolveModule(id.Name); mod != nil {
					switch prop := s.Property.(type) {
					case *ast.StructInstance:
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

						return &ModuleStructInstance{
							Module:   mod.Path(),
							Property: instance,
						}
					case *ast.Identifier:
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

				// Handle local enum variants (not from modules)
				sym, ok := c.scope.get(id.Name)
				if !ok {
					c.addError(fmt.Sprintf("Undefined: %s", id.Name), id.GetLocation())
					return nil
				}
				enum, ok := sym.Type.(*Enum)
				if !ok {
					c.addError(fmt.Sprintf("Undefined: %s::%s", sym.Name, s.Property), id.GetLocation())
					return nil
				}

				var variant int8 = -1
				for i := range enum.Variants {
					if enum.Variants[i] == s.Property.(*ast.Identifier).Name {
						variant = int8(i)
						break
					}
				}
				if variant == -1 {
					c.addError(fmt.Sprintf("Undefined: %s::%s", sym.Name, s.Property.(*ast.Identifier).Name), id.GetLocation())
					return nil
				}

				return &EnumVariant{enum: enum, Variant: variant}
			}
			// Handle nested static properties like http::Method::Get
			if _, ok := s.Target.(*ast.StaticProperty); ok {
				// First resolve the nested static property (e.g., http::Method)
				nestedSym := c.checkExpr(s.Target)
				if nestedSym == nil {
					return nil
				}

				// Check if it's an enum type
				if enum, ok := nestedSym.Type().(*Enum); ok {
					// Find the variant
					var variant int8 = -1
					for i := range enum.Variants {
						if enum.Variants[i] == s.Property.(*ast.Identifier).Name {
							variant = int8(i)
							break
						}
					}
					if variant == -1 {
						c.addError(fmt.Sprintf("Undefined: %s::%s", enum.Name, s.Property.(*ast.Identifier).Name), s.Property.GetLocation())
						return nil
					}

					return &EnumVariant{enum: enum, Variant: variant}
				}

				c.addError(fmt.Sprintf("Cannot access property on %T", nestedSym.Type()), s.Property.GetLocation())
				return nil
			}
			panic(fmt.Errorf("Unexpected static property target: %T", s.Target))
		}
	case *ast.StructInstance:
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
	case *ast.Try:
		{
			expr := c.checkExpr(s.Expression)
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
						}
					}

					// Error types must match for direct propagation
					if !_type.err.equal(fnReturnResult.err) {
						c.addError(fmt.Sprintf("Error type mismatch: Expected %s, got %s", fnReturnResult.err.String(), _type.err.String()), s.Expression.GetLocation())
						// Return a try op with the unwrapped type to avoid cascading errors
						return &TryOp{
							expr: expr,
							ok:   _type.val,
						}
					}

					// Success: returns the unwrapped value
					// Error: early returns the error wrapped in the function's Result type
					return &TryOp{
						expr: expr,
						ok:   _type.val,
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
					}
				}
			default:
				c.addError("try can only be used on Result or Maybe types, got: "+expr.Type().String(), s.Expression.GetLocation())
				// Return a try op with the expr type to avoid cascading errors
				return &TryOp{
					expr: expr,
					ok:   expr.Type(),
				}
			}
		}
	default:
		panic(fmt.Errorf("Unexpected expression: %s", reflect.TypeOf(s)))
	}
}

// extractIntFromPattern extracts an integer value from a pattern that can be either
// a NumLiteral or a UnaryExpression with minus operator applied to a NumLiteral
func (c *Checker) extractIntFromPattern(expr ast.Expression) (int, error) {
	switch e := expr.(type) {
	case *ast.NumLiteral:
		return strconv.Atoi(e.Value)
	case *ast.UnaryExpression:
		if e.Operator == ast.Minus {
			if literal, ok := e.Operand.(*ast.NumLiteral); ok {
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

// use this when we know what the expr's Type should be
func (c *Checker) checkExprAs(expr ast.Expression, expectedType Type) Expression {
	switch s := (expr).(type) {
	case *ast.ListLiteral:
		// Only use collection-specific inference when the expected type is a list.
		if _, ok := expectedType.(*List); ok {
			if result := c.checkList(expectedType, s); result != nil {
				return result
			}
			return nil
		}
	case *ast.MapLiteral:
		// Only use collection-specific inference when the expected type is a map.
		if _, ok := expectedType.(*Map); ok {
			if result := c.checkMap(expectedType, s); result != nil {
				return result
			}
			return nil
		}
	case *ast.StaticFunction:
		{
			resultType, expectResult := expectedType.(*Result)
			if !expectResult &&
				s.Target.(*ast.Identifier).Name != "Result" &&
				(s.Function.Name != "ok" && s.Function.Name != "err") {
				return c.checkExpr(s)
			}

			moduleName := s.Target.(*ast.Identifier).Name
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

func (c *Checker) checkExternalFunction(def *ast.ExternalFunction) *ExternalFunctionDef {
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

func (c *Checker) checkFunction(def *ast.FunctionDeclaration, init func()) *FunctionDef {
	if init != nil {
		init()
	}

	// Process parameters
	params := make([]Parameter, len(def.Parameters))
	for i, param := range def.Parameters {
		var paramType Type = Void
		if param.Type != nil {
			paramType = c.resolveType(param.Type)
			if paramType == nil {
				panic(fmt.Errorf("Cannot resolve type for parameter %s", param.Name))
			}
		}

		params[i] = Parameter{
			Name:    param.Name,
			Type:    paramType,
			Mutable: param.Mutable,
		}
	}

	// Determine return type
	var returnType Type = Void
	if def.ReturnType != nil {
		returnType = c.resolveType(def.ReturnType)
	}

	// Create function definition early (before checking body)
	fn := &FunctionDef{
		Name:       def.Name,
		Parameters: params,
		ReturnType: returnType,
		Body:       nil, // Will be set after checking
		Private:    def.Private,
	}

	// Add function to scope BEFORE checking body to support recursion
	// But don't add methods (when init != nil) to outer scope - they should only be accessible via receiver
	if init == nil {
		c.scope.add(def.Name, fn, false)
	}

	// Check function body
	body := c.checkBlock(def.Body, func() {
		// set the expected return type to the scope
		c.scope.expectReturn(returnType)
		// add parameters to scope
		for _, param := range params {
			c.scope.add(param.Name, param.Type, param.Mutable)
		}
	})

	// Check that the function's return type matches its body's type
	if returnType != Void && !areCompatible(returnType, body.Type()) {
		c.addError(typeMismatch(returnType, body.Type()), def.GetLocation())
	}

	// Set the body now that it's been checked
	fn.Body = body

	return fn
}

// Substitute generic parameters in a type
func substituteType(t Type, typeMap map[string]Type) Type {
	switch typ := t.(type) {
	case *Any:
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

// New generic resolution using the enhanced symbol table
func (c *Checker) resolveGenericFunction(fnDef *FunctionDef, args []Expression, typeArgs []ast.DeclaredType, _ ast.Location) (*FunctionDef, error) {
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

	// Create generic context scope
	genericScope := c.scope.createGenericScope(genericParams)

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

	// Infer types from arguments
	for i, param := range fnDef.Parameters {
		if err := c.unifyTypes(param.Type, args[i].Type(), genericScope); err != nil {
			return nil, err
		}
	}

	// Allow unresolved generics - they will be resolved through context
	// (e.g., variable assignment, return type inference)
	// Don't require all generics to be resolved at function call time

	// Create specialized function with resolved generics
	bindings := genericScope.getGenericBindings()

	// Only specialize if we have resolved some generics
	if len(bindings) == 0 {
		return fnDef, nil
	}

	specialized := &FunctionDef{
		Name:       fnDef.Name,
		Parameters: make([]Parameter, len(fnDef.Parameters)),
		ReturnType: substituteType(fnDef.ReturnType, bindings),
		Body:       fnDef.Body,
		Mutates:    fnDef.Mutates,
		Private:    fnDef.Private,
	}

	// Replace generics in parameters
	for i, param := range fnDef.Parameters {
		specialized.Parameters[i] = Parameter{
			Name:    param.Name,
			Type:    substituteType(param.Type, bindings),
			Mutable: param.Mutable,
		}
	}

	return specialized, nil
}

// unifyTypes attempts to unify two types by binding generics in the scope
func (c *Checker) unifyTypes(expected Type, actual Type, genericScope *SymbolTable) error {
	switch expectedType := expected.(type) {
	case *Any:
		// Generic type - bind it to the actual type
		return genericScope.bindGeneric(expectedType.name, actual)
	case *FunctionDef:
		// Function type unification
		if actualFn, ok := actual.(*FunctionDef); ok {
			// Check parameter count
			if len(expectedType.Parameters) != len(actualFn.Parameters) {
				return fmt.Errorf("parameter count mismatch")
			}

			// Unify parameters
			for i, expectedParam := range expectedType.Parameters {
				if err := c.unifyTypes(expectedParam.Type, actualFn.Parameters[i].Type, genericScope); err != nil {
					return err
				}
			}

			// Unify return types
			return c.unifyTypes(expectedType.ReturnType, actualFn.ReturnType, genericScope)
		}
		return fmt.Errorf("expected function, got %T", actual)
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

// Helper function to extract generic names from a type
func extractGenericNames(t Type, names map[string]bool) {
	switch t := t.(type) {
	case *Any:
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
	case *FunctionDef:
		// Extract generics from function parameters and return type
		for _, param := range t.Parameters {
			extractGenericNames(param.Type, names)
		}
		extractGenericNames(t.ReturnType, names)
	}
}

// resolveArguments converts unified argument list to positional arguments
func (c *Checker) resolveArguments(args []ast.Argument, params []Parameter) ([]ast.Expression, error) {
	// Separate positional and named arguments
	var positionalArgs []ast.Expression
	var namedArgs []ast.Argument

	for _, arg := range args {
		if arg.Name == "" {
			// Positional argument
			positionalArgs = append(positionalArgs, arg.Value)
		} else {
			// Named argument
			namedArgs = append(namedArgs, arg)
		}
	}

	// If no named arguments, just return positional arguments
	if len(namedArgs) == 0 {
		return positionalArgs, nil
	}

	// Create a map of parameter names to indices
	paramMap := make(map[string]int)
	for i, param := range params {
		paramMap[param.Name] = i
	}

	// Create result array
	result := make([]ast.Expression, len(params))
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

	// Check that all parameters are provided
	for i, param := range params {
		if !used[i] {
			return nil, fmt.Errorf("missing argument for parameter: %s", param.Name)
		}
	}

	return result, nil
}
