package checker

import "github.com/akonwi/ard/parse"

type preparedInherentMethod struct {
	Signature *FunctionDef
}

func (c *Checker) prepareInherentImplSignatures() {
	firstDeclarations := map[MethodOwner]map[string]SourceSpan{}
	for _, statement := range c.input.Statements {
		impl, ok := statement.(*parse.ImplBlock)
		if !ok {
			continue
		}
		sym, ok := c.scope.get(impl.Target.Name)
		if !ok {
			continue
		}

		var owner MethodOwner
		var receiverGenerics []string
		var addMethod func(*FunctionDef)
		switch target := sym.Type.(type) {
		case *StructDef:
			owner = StructMethodOwner(target)
			receiverGenerics = genericParamsForType(target)
			addMethod = func(method *FunctionDef) { c.addStructMethod(target, method) }
		case *Enum:
			owner = MethodOwner{ModulePath: target.ModulePath, TypeName: target.Name}
			receiverGenerics = genericParamsForType(target)
			if target.Methods == nil {
				target.Methods = map[string]*FunctionDef{}
			}
			addMethod = func(method *FunctionDef) { target.Methods[method.Name] = method }
		default:
			continue
		}

		if firstDeclarations[owner] == nil {
			firstDeclarations[owner] = map[string]SourceSpan{}
		}
		for i := range impl.Methods {
			method := &impl.Methods[i]
			if len(method.TypeParams) > 0 {
				continue
			}
			c.pushMethodGenericAllowlist(receiverGenerics)
			signature := c.resolveMethodSignature(method)
			c.popMethodGenericAllowlist()
			signature.Receiver = impl.Receiver.Name
			if _, isEnum := sym.Type.(*Enum); !isEnum {
				signature.Mutates = method.Mutates
			}

			prepared := preparedInherentMethod{Signature: signature}
			if original, duplicate := firstDeclarations[owner][method.Name]; duplicate {
				c.addDiagnostic(duplicateMethodDiagnostic{
					Method:       method.Name,
					Span:         c.sourceSpan(method.GetLocation()),
					OriginalSpan: &original,
				}.build())
			} else {
				firstDeclarations[owner][method.Name] = c.sourceSpan(method.GetLocation())
				addMethod(signature)
			}
			c.preparedInherentMethods[method] = prepared
		}
	}
}

// MethodOwner identifies the named type whose method namespace a method belongs to.
type MethodOwner struct {
	ModulePath string
	TypeName   string
}

func StructMethodOwner(def *StructDef) MethodOwner {
	if def == nil {
		return MethodOwner{}
	}
	return MethodOwner{ModulePath: def.ModulePath, TypeName: def.Name}
}

func (p *Program) AddStructMethod(owner MethodOwner, name string, method *FunctionDef) {
	if p == nil || owner.TypeName == "" || name == "" || method == nil {
		return
	}
	if p.StructMethods == nil {
		p.StructMethods = map[MethodOwner]map[string]*FunctionDef{}
	}
	methods := p.StructMethods[owner]
	if methods == nil {
		methods = map[string]*FunctionDef{}
		p.StructMethods[owner] = methods
	}
	methods[name] = method
}

func (p *Program) StructMethod(owner MethodOwner, name string) (*FunctionDef, bool) {
	if p == nil || p.StructMethods == nil {
		return nil, false
	}
	methods := p.StructMethods[owner]
	if methods == nil {
		return nil, false
	}
	method, ok := methods[name]
	return method, ok
}

func (p *Program) StructMethodsFor(owner MethodOwner) map[string]*FunctionDef {
	if p == nil || p.StructMethods == nil {
		return nil
	}
	return p.StructMethods[owner]
}

func (p *Program) implementsForeignInterface(owner MethodOwner, iface *ForeignType) bool {
	if p == nil || iface == nil {
		return false
	}
	for _, implemented := range p.ForeignInterfaceImpls[owner] {
		if implemented != nil && implemented.equal(iface) {
			return true
		}
	}
	return false
}

func StructMethodInModules(modules map[string]Module, owner MethodOwner, name string) (*FunctionDef, bool) {
	return structMethodInModulesSeen(modules, owner, name, map[string]bool{})
}

func StructMethodsInModules(modules map[string]Module, owner MethodOwner) map[string]*FunctionDef {
	return structMethodsInModulesSeen(modules, owner, map[string]bool{})
}

func StructDefinitionInModules(modules map[string]Module, owner MethodOwner) (*StructDef, bool) {
	return structDefinitionInModulesSeen(modules, owner, map[string]bool{})
}

func foreignInterfaceImplementationInModules(modules map[string]Module, owner MethodOwner, iface *ForeignType) bool {
	return foreignInterfaceImplementationInModulesSeen(modules, owner, iface, map[string]bool{})
}

func foreignInterfaceImplementationInModulesSeen(modules map[string]Module, owner MethodOwner, iface *ForeignType, seen map[string]bool) bool {
	for _, mod := range modules {
		if mod == nil || seen[mod.Path()] {
			continue
		}
		seen[mod.Path()] = true
		program := mod.Program()
		if program == nil {
			continue
		}
		if program.implementsForeignInterface(owner, iface) || foreignInterfaceImplementationInModulesSeen(program.Imports, owner, iface, seen) {
			return true
		}
	}
	return false
}

func structMethodInModulesSeen(modules map[string]Module, owner MethodOwner, name string, seen map[string]bool) (*FunctionDef, bool) {
	for _, mod := range modules {
		if method, ok := structMethodInModuleSeen(mod, owner, name, seen); ok {
			return method, true
		}
	}
	return nil, false
}

func structMethodInModuleSeen(mod Module, owner MethodOwner, name string, seen map[string]bool) (*FunctionDef, bool) {
	if mod == nil {
		return nil, false
	}
	path := mod.Path()
	if seen[path] {
		return nil, false
	}
	seen[path] = true
	program := mod.Program()
	if program == nil {
		return nil, false
	}
	if method, ok := program.StructMethod(owner, name); ok {
		return method, true
	}
	return structMethodInModulesSeen(program.Imports, owner, name, seen)
}

func structMethodsInModulesSeen(modules map[string]Module, owner MethodOwner, seen map[string]bool) map[string]*FunctionDef {
	for _, mod := range modules {
		if methods := structMethodsInModuleSeen(mod, owner, seen); methods != nil {
			return methods
		}
	}
	return nil
}

func structMethodsInModuleSeen(mod Module, owner MethodOwner, seen map[string]bool) map[string]*FunctionDef {
	if mod == nil {
		return nil
	}
	path := mod.Path()
	if seen[path] {
		return nil
	}
	seen[path] = true
	program := mod.Program()
	if program == nil {
		return nil
	}
	if methods := program.StructMethodsFor(owner); methods != nil {
		return methods
	}
	return structMethodsInModulesSeen(program.Imports, owner, seen)
}

func structDefinitionInModulesSeen(modules map[string]Module, owner MethodOwner, seen map[string]bool) (*StructDef, bool) {
	for _, mod := range modules {
		if def, ok := structDefinitionInModuleSeen(mod, owner, seen); ok {
			return def, true
		}
	}
	return nil, false
}

func structDefinitionInModuleSeen(mod Module, owner MethodOwner, seen map[string]bool) (*StructDef, bool) {
	if mod == nil {
		return nil, false
	}
	path := mod.Path()
	if seen[path] {
		return nil, false
	}
	seen[path] = true
	if owner.ModulePath == "" || path == owner.ModulePath {
		if sym := mod.Get(owner.TypeName); !sym.IsZero() {
			if def, ok := sym.Type.(*StructDef); ok && def.Name == owner.TypeName && !namedTypeOwnersDiffer(def.ModulePath, owner.ModulePath) {
				return def, true
			}
		}
	}
	program := mod.Program()
	if program == nil {
		return nil, false
	}
	return structDefinitionInModulesSeen(program.Imports, owner, seen)
}

func (c *Checker) addStructMethod(def *StructDef, method *FunctionDef) {
	if def == nil || method == nil {
		return
	}
	c.program.AddStructMethod(StructMethodOwner(def), method.Name, method)
}

func (c *Checker) structMethod(def *StructDef, name string) (*FunctionDef, bool) {
	if def == nil {
		return nil, false
	}
	owner := StructMethodOwner(def)
	if method, ok := c.program.StructMethod(owner, name); ok {
		return method, true
	}
	method, ok := StructMethodInModules(c.program.Imports, owner, name)
	if !ok || !c.canAccessStructMethod(owner, method) {
		return nil, false
	}
	return method, true
}

func (c *Checker) canAccessStructMethod(owner MethodOwner, method *FunctionDef) bool {
	if method == nil || !method.Private {
		return true
	}
	return owner.ModulePath == c.typeOwnerPath()
}

func (c *Checker) structMethods(def *StructDef) map[string]*FunctionDef {
	if def == nil {
		return nil
	}
	owner := StructMethodOwner(def)
	if methods := c.program.StructMethodsFor(owner); methods != nil {
		return methods
	}
	return StructMethodsInModules(c.program.Imports, owner)
}

func (c *Checker) structDefinition(def *StructDef) *StructDef {
	if def == nil {
		return nil
	}
	if sym, ok := c.scope.get(def.Name); ok {
		if structDef, ok := sym.Type.(*StructDef); ok && structDef.Name == def.Name && !namedTypeOwnersDiffer(structDef.ModulePath, def.ModulePath) {
			return structDef
		}
	}
	if structDef, ok := StructDefinitionInModules(c.program.Imports, StructMethodOwner(def)); ok {
		return structDef
	}
	return def
}
