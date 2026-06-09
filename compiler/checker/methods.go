package checker

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
	for _, mod := range c.program.Imports {
		if mod == nil || mod.Program() == nil {
			continue
		}
		if method, ok := mod.Program().StructMethod(owner, name); ok {
			return method, true
		}
	}
	return nil, false
}

func (c *Checker) structMethods(def *StructDef) map[string]*FunctionDef {
	if def == nil {
		return nil
	}
	owner := StructMethodOwner(def)
	if methods := c.program.StructMethodsFor(owner); methods != nil {
		return methods
	}
	for _, mod := range c.program.Imports {
		if mod == nil || mod.Program() == nil {
			continue
		}
		if methods := mod.Program().StructMethodsFor(owner); methods != nil {
			return methods
		}
	}
	return nil
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
	for _, mod := range c.program.Imports {
		if mod == nil {
			continue
		}
		if def.ModulePath != "" && mod.Path() != def.ModulePath {
			continue
		}
		if sym := mod.Get(def.Name); !sym.IsZero() {
			if structDef, ok := sym.Type.(*StructDef); ok && structDef.Name == def.Name && !namedTypeOwnersDiffer(structDef.ModulePath, def.ModulePath) {
				return structDef
			}
		}
	}
	return def
}
