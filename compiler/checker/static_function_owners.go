package checker

import (
	"github.com/akonwi/ard/parse"
)

// Static functions resolved through their owner type (issue #607).
//
// A static function such as `fn Box::wrap(value: Int) Box` is declared under
// the name `Box::wrap` in its module. It can be named through any path that
// denotes its owner type: `Box::wrap` locally, `feature::Box::wrap` from
// another module, and through a type alias, `Alias::wrap`,
// `feature::Alias::wrap`, or a re-exported alias such as
// `barrel::Reexport::wrap`. An alias that applies type arguments, such as
// `type IntCell = Cell<Int>`, binds them to the static function's generics of
// the same name, the way a receiver binds a method's generics.

// staticFunctionRef is a static function resolved from an owner path.
type staticFunctionRef struct {
	// module declares the function; nil when it is the module being checked.
	module Module
	// name is the declared name, such as `Box::wrap`.
	name   string
	symbol Symbol
	// ownerBindings binds the function's generics from the type arguments an
	// alias applies to a generic owner type.
	ownerBindings map[string]Type
}

// lookupStaticFunction resolves `owner::member` to a static function without
// checking or recording anything. owner is a type name (`Box`) or a
// module-qualified type name (`feature::Box`), either of which may be an alias.
// It reports false for any other path, such as a module function or an enum
// variant.
func (c *Checker) lookupStaticFunction(owner parse.Expression, member string) (staticFunctionRef, bool) {
	var ownerType Type
	switch o := owner.(type) {
	case *parse.Identifier:
		if ref, ok := c.localStaticFunction(o.Name, member); ok {
			return ref, true
		}
		if c.program.GoImports[o.Name] != nil || c.resolveModule(o.Name) != nil {
			return staticFunctionRef{}, false
		}
		sym, ok := c.scope.get(o.Name)
		if !ok || !sym.typeDeclaration {
			return staticFunctionRef{}, false
		}
		ownerType = sym.Type
	case *parse.StaticProperty:
		moduleName, ok := o.Target.(*parse.Identifier)
		if !ok {
			return staticFunctionRef{}, false
		}
		typeName, ok := o.Property.(*parse.Identifier)
		if !ok || c.program.GoImports[moduleName.Name] != nil {
			return staticFunctionRef{}, false
		}
		mod := c.resolveModule(moduleName.Name)
		if mod == nil {
			return staticFunctionRef{}, false
		}
		if sym := mod.Get(typeName.Name + "::" + member); isStaticFunctionSymbol(sym) {
			return staticFunctionRef{module: mod, name: typeName.Name + "::" + member, symbol: sym}, true
		}
		sym := mod.Get(typeName.Name)
		if sym.IsZero() || !sym.typeDeclaration {
			return staticFunctionRef{}, false
		}
		ownerType = sym.Type
	default:
		return staticFunctionRef{}, false
	}
	return c.lookupStaticFunctionOfType(ownerType, member)
}

// localStaticFunction finds a static function declared in the module being
// checked under exactly `owner::member`. Like a call, this exact name takes
// precedence over a module or Go package named owner.
func (c *Checker) localStaticFunction(owner, member string) (staticFunctionRef, bool) {
	name := owner + "::" + member
	sym, ok := c.scope.get(name)
	if !ok || !isStaticFunctionSymbol(*sym) {
		return staticFunctionRef{}, false
	}
	return staticFunctionRef{name: name, symbol: *sym}, true
}

// lookupStaticFunctionOfType finds member among the static functions of the
// nominal type ownerType, in the module that declares the type.
func (c *Checker) lookupStaticFunctionOfType(ownerType Type, member string) (staticFunctionRef, bool) {
	var modulePath, typeName string
	var ownerBindings map[string]Type
	switch t := ownerType.(type) {
	case *StructDef:
		definition := canonicalStructDefinition(t)
		modulePath, typeName = definition.ModulePath, definition.Name
		if len(t.TypeArgs) > 0 {
			ownerBindings = c.extractGenericBindingsFromSpecializedStruct(definition, t)
		}
	case *Enum:
		modulePath, typeName = t.ModulePath, t.Name
	case *Union:
		modulePath, typeName = t.ModulePath, t.Name
	case *Trait:
		modulePath, typeName = t.ModulePath, t.Name
	default:
		return staticFunctionRef{}, false
	}
	name := typeName + "::" + member

	if modulePath == "" || modulePath == c.typeOwnerPath() {
		sym, ok := c.scope.get(name)
		if !ok || !isStaticFunctionSymbol(*sym) {
			return staticFunctionRef{}, false
		}
		return staticFunctionRef{name: name, symbol: *sym, ownerBindings: ownerBindingsFor(sym.Type, ownerBindings)}, true
	}
	mod := c.findModuleByPathTransitively(modulePath)
	if mod == nil {
		return staticFunctionRef{}, false
	}
	sym := mod.Get(name)
	if !isStaticFunctionSymbol(sym) {
		return staticFunctionRef{}, false
	}
	return staticFunctionRef{module: mod, name: name, symbol: sym, ownerBindings: ownerBindingsFor(sym.Type, ownerBindings)}, true
}

func isStaticFunctionSymbol(sym Symbol) bool {
	if sym.IsZero() || sym.typeDeclaration {
		return false
	}
	_, ok := sym.Type.(*FunctionDef)
	return ok
}

// ownerBindingsFor keeps the owner bindings that name one of the function's
// own generics.
func ownerBindingsFor(typ Type, bindings map[string]Type) map[string]Type {
	fn, ok := typ.(*FunctionDef)
	if !ok || len(bindings) == 0 {
		return nil
	}
	var kept map[string]Type
	for _, param := range callGenericParamsForFunction(fn) {
		if bound, ok := bindings[param]; ok && bound != nil {
			if kept == nil {
				kept = map[string]Type{}
			}
			kept[param] = bound
		}
	}
	return kept
}

// findModuleByPathTransitively finds a module by its path among this module's
// imports and, for types re-exported through another module, their imports.
func (c *Checker) findModuleByPathTransitively(path string) Module {
	seen := acquireModuleSeen()
	defer releaseModuleSeen(seen)
	return findModuleByPathSeen(c.program.Imports, path, seen)
}

func findModuleByPathSeen(modules map[string]Module, path string, seen map[string]bool) Module {
	for _, mod := range modules {
		if mod != nil && mod.Path() == path {
			return mod
		}
	}
	for _, mod := range modules {
		if mod == nil || seen[mod.Path()] {
			continue
		}
		seen[mod.Path()] = true
		program := mod.Program()
		if program == nil {
			continue
		}
		if found := findModuleByPathSeen(program.Imports, path, seen); found != nil {
			return found
		}
	}
	return nil
}

// staticFunctionValue is a static function used as a value. A generic one is
// instantiated from the owner's type arguments and the expected function type.
func (c *Checker) staticFunctionValue(expr parse.Expression, member parse.Expression, ref staticFunctionRef) Expression {
	valueType, ok := c.functionValueType(expr, ref.symbol.Type, ref.ownerBindings, member.GetLocation())
	if !ok {
		return nil
	}
	value := ref.symbol
	value.Type = valueType
	if ref.module == nil {
		return &Variable{value}
	}
	node := &ModuleSymbol{Module: ref.module.Path(), Symbol: value}
	c.recordTarget(member, node, SpanTarget{Kind: TargetFunction, Module: ref.module.Path(), Symbol: ref.name})
	return node
}

// setupStaticFunctionCall sets up a call's generic scope with the owner
// bindings an alias applies to a static function, in addition to explicit
// type arguments.
func (c *Checker) setupStaticFunctionCall(fnDef *FunctionDef, ownerBindings map[string]Type, typeArgs []Type, declaredTypeArgs []parse.DeclaredType) (*FunctionDef, *SymbolTable, error) {
	if len(ownerBindings) == 0 {
		return c.setupFunctionCall(fnDef, typeArgs, declaredTypeArgs)
	}
	genericParams := append([]string(nil), callGenericParamsForFunction(fnDef)...)
	return c.setupFunctionCallWithBindings(fnDef, genericParams, ownerBindings, genericParams, typeArgs, declaredTypeArgs)
}
