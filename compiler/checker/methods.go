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
			addMethod = func(method *FunctionDef) { c.addInherentStructMethod(target, method) }
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

func MethodOwnerForType(typ Type) (MethodOwner, bool) {
	switch typed := typ.(type) {
	case *StructDef:
		return StructMethodOwner(typed), true
	case *Enum:
		return MethodOwner{ModulePath: typed.ModulePath, TypeName: typed.Name}, true
	default:
		return MethodOwner{}, false
	}
}

type TraitMethodOwner struct {
	MethodOwner
	TraitModulePath string
	TraitName       string
}

func traitMethodOwner(owner MethodOwner, trait *Trait) TraitMethodOwner {
	if trait == nil {
		return TraitMethodOwner{MethodOwner: owner}
	}
	return TraitMethodOwner{MethodOwner: owner, TraitModulePath: trait.ModulePath, TraitName: trait.Name}
}

func (p *Program) AddInherentMethod(owner MethodOwner, name string, method *FunctionDef) {
	if p == nil || owner.TypeName == "" || name == "" || method == nil {
		return
	}
	if p.InherentMethods == nil {
		p.InherentMethods = map[MethodOwner]map[string]*FunctionDef{}
	}
	methods := p.InherentMethods[owner]
	if methods == nil {
		methods = map[string]*FunctionDef{}
		p.InherentMethods[owner] = methods
	}
	methods[name] = method
}

func (p *Program) InherentMethod(owner MethodOwner, name string) (*FunctionDef, bool) {
	if p == nil || p.InherentMethods == nil {
		return nil, false
	}
	method, ok := p.InherentMethods[owner][name]
	return method, ok
}

func (p *Program) InherentMethodsFor(owner MethodOwner) map[string]*FunctionDef {
	if p == nil || p.InherentMethods == nil {
		return nil
	}
	return p.InherentMethods[owner]
}

func (p *Program) AddTraitMethod(owner MethodOwner, trait *Trait, name string, method *FunctionDef) {
	if p == nil || owner.TypeName == "" || trait == nil || name == "" || method == nil {
		return
	}
	if p.TraitMethods == nil {
		p.TraitMethods = map[TraitMethodOwner]map[string]*FunctionDef{}
	}
	key := traitMethodOwner(owner, trait)
	methods := p.TraitMethods[key]
	if methods == nil {
		methods = map[string]*FunctionDef{}
		p.TraitMethods[key] = methods
	}
	methods[name] = method
}

func (p *Program) TraitMethodsFor(owner MethodOwner, trait *Trait) map[string]*FunctionDef {
	if p == nil || p.TraitMethods == nil || trait == nil {
		return nil
	}
	return p.TraitMethods[traitMethodOwner(owner, trait)]
}

func (p *Program) HasTraitMethodNamed(owner MethodOwner, name string) bool {
	if p == nil {
		return false
	}
	for key, methods := range p.TraitMethods {
		if key.MethodOwner == owner && methods[name] != nil {
			return true
		}
	}
	return false
}

func (p *Program) MarkAmbiguousTraitMethod(owner MethodOwner, name string) {
	if p == nil || owner.TypeName == "" || name == "" {
		return
	}
	if p.AmbiguousTraitMethods == nil {
		p.AmbiguousTraitMethods = map[MethodOwner]map[string]bool{}
	}
	methods := p.AmbiguousTraitMethods[owner]
	if methods == nil {
		methods = map[string]bool{}
		p.AmbiguousTraitMethods[owner] = methods
	}
	methods[name] = true
}

func (p *Program) HasAmbiguousTraitMethod(owner MethodOwner, name string) bool {
	return p != nil && p.AmbiguousTraitMethods[owner][name]
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

func (p *Program) AddRequiredGoMethod(owner MethodOwner, name string, method *FunctionDef) {
	if p == nil || owner.TypeName == "" || name == "" || method == nil {
		return
	}
	if p.RequiredGoMethods == nil {
		p.RequiredGoMethods = map[MethodOwner]map[string]*FunctionDef{}
	}
	methods := p.RequiredGoMethods[owner]
	if methods == nil {
		methods = map[string]*FunctionDef{}
		p.RequiredGoMethods[owner] = methods
	}
	methods[name] = method
}

func (p *Program) RequiredGoMethod(owner MethodOwner, name string) (*FunctionDef, bool) {
	if p == nil || p.RequiredGoMethods == nil {
		return nil, false
	}
	method, ok := p.RequiredGoMethods[owner][name]
	return method, ok
}

func (p *Program) RequiredGoMethodsFor(owner MethodOwner) map[string]*FunctionDef {
	if p == nil || p.RequiredGoMethods == nil {
		return nil
	}
	return p.RequiredGoMethods[owner]
}

func (p *Program) HasRequiredGoMethodNamed(owner MethodOwner, name string) bool {
	for _, method := range p.RequiredGoMethodsFor(owner) {
		if method != nil && method.Name == name {
			return true
		}
	}
	return false
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

func InherentMethodsInModules(modules map[string]Module, owner MethodOwner) map[string]*FunctionDef {
	return inherentMethodsInModulesSeen(modules, owner, map[string]bool{})
}

func TraitMethodsInModules(modules map[string]Module, owner MethodOwner, trait *Trait) map[string]*FunctionDef {
	return traitMethodsInModulesSeen(modules, owner, trait, map[string]bool{})
}

func AmbiguousTraitMethodInModules(modules map[string]Module, owner MethodOwner, name string) bool {
	return ambiguousTraitMethodInModulesSeen(modules, owner, name, map[string]bool{})
}

func RequiredGoMethodsInModules(modules map[string]Module, owner MethodOwner) map[string]*FunctionDef {
	return requiredGoMethodsInModulesSeen(modules, owner, map[string]bool{})
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

func ambiguousTraitMethodInModulesSeen(modules map[string]Module, owner MethodOwner, name string, seen map[string]bool) bool {
	for _, mod := range modules {
		if mod == nil || seen[mod.Path()] {
			continue
		}
		seen[mod.Path()] = true
		program := mod.Program()
		if program == nil {
			continue
		}
		if program.HasAmbiguousTraitMethod(owner, name) || ambiguousTraitMethodInModulesSeen(program.Imports, owner, name, seen) {
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

func inherentMethodsInModulesSeen(modules map[string]Module, owner MethodOwner, seen map[string]bool) map[string]*FunctionDef {
	for _, mod := range modules {
		if methods := inherentMethodsInModuleSeen(mod, owner, seen); methods != nil {
			return methods
		}
	}
	return nil
}

func inherentMethodsInModuleSeen(mod Module, owner MethodOwner, seen map[string]bool) map[string]*FunctionDef {
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
	if methods := program.InherentMethodsFor(owner); methods != nil {
		return methods
	}
	return inherentMethodsInModulesSeen(program.Imports, owner, seen)
}

func traitMethodsInModulesSeen(modules map[string]Module, owner MethodOwner, trait *Trait, seen map[string]bool) map[string]*FunctionDef {
	for _, mod := range modules {
		if methods := traitMethodsInModuleSeen(mod, owner, trait, seen); methods != nil {
			return methods
		}
	}
	return nil
}

func requiredGoMethodsInModulesSeen(modules map[string]Module, owner MethodOwner, seen map[string]bool) map[string]*FunctionDef {
	for _, mod := range modules {
		if methods := requiredGoMethodsInModuleSeen(mod, owner, seen); methods != nil {
			return methods
		}
	}
	return nil
}

func requiredGoMethodsInModuleSeen(mod Module, owner MethodOwner, seen map[string]bool) map[string]*FunctionDef {
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
	if methods := program.RequiredGoMethodsFor(owner); methods != nil {
		return methods
	}
	return requiredGoMethodsInModulesSeen(program.Imports, owner, seen)
}

func traitMethodsInModuleSeen(mod Module, owner MethodOwner, trait *Trait, seen map[string]bool) map[string]*FunctionDef {
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
	if methods := program.TraitMethodsFor(owner, trait); methods != nil {
		return methods
	}
	return traitMethodsInModulesSeen(program.Imports, owner, trait, seen)
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

func (c *Checker) addInherentStructMethod(def *StructDef, method *FunctionDef) {
	if def == nil || method == nil {
		return
	}
	owner := StructMethodOwner(def)
	c.program.AddInherentMethod(owner, method.Name, method)
	c.program.AddStructMethod(owner, method.Name, method)
}

func (c *Checker) addStructMethod(def *StructDef, method *FunctionDef) {
	if def == nil || method == nil {
		return
	}
	owner := StructMethodOwner(def)
	c.program.AddStructMethod(owner, method.Name, method)
	if method.RequiredGoMethodName != "" {
		c.program.AddRequiredGoMethod(owner, method.RequiredGoMethodName, method)
	}
}

func (c *Checker) hasAmbiguousTraitMethod(typ Type, name string) bool {
	owner, ok := MethodOwnerForType(typ)
	return ok && (c.program.HasAmbiguousTraitMethod(owner, name) || AmbiguousTraitMethodInModules(c.program.Imports, owner, name))
}

func (c *Checker) structMethod(def *StructDef, name string) (*FunctionDef, bool) {
	if def == nil {
		return nil, false
	}
	owner := StructMethodOwner(def)
	if method, ok := c.program.InherentMethod(owner, name); ok {
		return method, true
	}
	if methods := InherentMethodsInModules(c.program.Imports, owner); methods != nil {
		if method := methods[name]; method != nil && c.canAccessStructMethod(owner, method) {
			return method, true
		}
	}
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
