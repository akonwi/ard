package checker

import (
	"fmt"

	"github.com/akonwi/ard/parse"
)

func (c *Checker) hoistTopLevelTypeDeclarations() {
	seen := map[string]parse.Location{}
	for i := range c.input.Statements {
		stmt := c.input.Statements[i]
		name, loc, ok := topLevelTypeDeclarationName(stmt)
		if !ok {
			continue
		}
		if _, dup := seen[name]; dup {
			c.addError(fmt.Sprintf("Duplicate declaration: %s", name), loc)
			c.markDuplicateTopLevelTypeDeclaration(stmt)
			continue
		}
		seen[name] = loc

		switch s := stmt.(type) {
		case *parse.StructDefinition:
			if c.topLevelStructDeclarations == nil {
				c.topLevelStructDeclarations = map[string]*parse.StructDefinition{}
			}
			c.topLevelStructDeclarations[name] = s
			genericParams := genericParamsFromStructDeclaration(s)
			c.scope.add(name, &StructDef{
				Name:          s.Name.Name,
				ModulePath:    c.typeOwnerPath(),
				Fields:        make(map[string]Type),
				GenericParams: genericParams,
				Private:       s.Private,
			}, false)
		case *parse.TraitDefinition:
			c.scope.add(name, &Trait{Name: s.Name.Name, private: s.Private}, false)
		case *parse.EnumDefinition:
			c.scope.add(name, &Enum{Name: s.Name, ModulePath: c.typeOwnerPath(), Private: s.Private, Methods: make(map[string]*FunctionDef), Location: s.GetLocation()}, false)
		case *parse.ExternTypeDeclaration:
			typeArgs := make([]Type, len(s.TypeParams))
			for i, param := range s.TypeParams {
				typeArgs[i] = &TypeVar{name: param}
			}
			c.scope.add(name, &ExternType{Name_: s.Name, GenericParams: append([]string(nil), s.TypeParams...), TypeArgs: typeArgs, private: s.Private}, false)
		case *parse.TypeDeclaration:
			if len(s.Type) == 1 {
				if c.topLevelTypeAliases == nil {
					c.topLevelTypeAliases = map[string]*parse.TypeDeclaration{}
				}
				c.topLevelTypeAliases[name] = s
			} else {
				c.scope.add(name, &Union{Name: s.Name.Name, ModulePath: c.typeOwnerPath()}, false)
			}
		}
	}
}

func (c *Checker) populateTopLevelTypeDefinitions() {
	for i := range c.input.Statements {
		switch c.input.Statements[i].(type) {
		case *parse.StructDefinition, *parse.TraitDefinition, *parse.EnumDefinition, *parse.ExternTypeDeclaration, *parse.TypeDeclaration:
			c.checkStmt(&c.input.Statements[i])
		}
	}
}

func isTopLevelTypeDeclaration(stmt parse.Statement) bool {
	_, _, ok := topLevelTypeDeclarationName(stmt)
	return ok
}

func (c *Checker) checkedTopLevelTypeStatement(stmt parse.Statement) *Statement {
	if c.isDuplicateTopLevelTypeDeclaration(stmt) {
		return nil
	}
	switch s := stmt.(type) {
	case *parse.StructDefinition:
		def, ok := c.hoistedStruct(s.Name.Name)
		if !ok {
			return nil
		}
		return &Statement{Stmt: def}
	case *parse.ExternTypeDeclaration:
		externType, ok := c.hoistedExternType(s.Name)
		if !ok {
			return nil
		}
		return &Statement{Stmt: externType}
	case *parse.TypeDeclaration:
		if len(s.Type) <= 1 {
			return nil
		}
		unionType, ok := c.hoistedUnion(s.Name.Name)
		if !ok {
			return nil
		}
		return &Statement{Stmt: unionType}
	default:
		return nil
	}
}

func topLevelTypeDeclarationName(stmt parse.Statement) (string, parse.Location, bool) {
	switch s := stmt.(type) {
	case *parse.StructDefinition:
		return s.Name.Name, s.Name.GetLocation(), true
	case *parse.TraitDefinition:
		return s.Name.Name, s.Name.GetLocation(), true
	case *parse.EnumDefinition:
		return s.Name, s.GetLocation(), true
	case *parse.ExternTypeDeclaration:
		return s.Name, s.GetLocation(), true
	case *parse.TypeDeclaration:
		return s.Name.Name, s.Name.GetLocation(), true
	default:
		return "", parse.Location{}, false
	}
}

func genericParamsFromStructDeclaration(decl *parse.StructDefinition) []string {
	params := []string{}
	seen := map[string]bool{}
	for _, param := range decl.TypeParams {
		if !seen[param] {
			params = append(params, param)
			seen[param] = true
		}
	}
	for _, field := range decl.Fields {
		collectGenericParamsFromDeclaredType(field.Type, &params, seen)
	}
	return params
}

func collectGenericParamsFromDeclaredType(t parse.DeclaredType, params *[]string, seen map[string]bool) {
	if t == nil {
		return
	}
	switch typ := t.(type) {
	case *parse.GenericType:
		if !seen[typ.Name] {
			*params = append(*params, typ.Name)
			seen[typ.Name] = true
		}
	case parse.GenericType:
		if !seen[typ.Name] {
			*params = append(*params, typ.Name)
			seen[typ.Name] = true
		}
	case *parse.MutableType:
		collectGenericParamsFromDeclaredType(typ.Inner, params, seen)
	case parse.MutableType:
		collectGenericParamsFromDeclaredType(typ.Inner, params, seen)
	case *parse.List:
		collectGenericParamsFromDeclaredType(typ.Element, params, seen)
	case parse.List:
		collectGenericParamsFromDeclaredType(typ.Element, params, seen)
	case *parse.Map:
		collectGenericParamsFromDeclaredType(typ.Key, params, seen)
		collectGenericParamsFromDeclaredType(typ.Value, params, seen)
	case parse.Map:
		collectGenericParamsFromDeclaredType(typ.Key, params, seen)
		collectGenericParamsFromDeclaredType(typ.Value, params, seen)
	case *parse.ResultType:
		collectGenericParamsFromDeclaredType(typ.Val, params, seen)
		collectGenericParamsFromDeclaredType(typ.Err, params, seen)
	case parse.ResultType:
		collectGenericParamsFromDeclaredType(typ.Val, params, seen)
		collectGenericParamsFromDeclaredType(typ.Err, params, seen)
	case *parse.FunctionType:
		for _, param := range typ.Params {
			collectGenericParamsFromDeclaredType(param, params, seen)
		}
		collectGenericParamsFromDeclaredType(typ.Return, params, seen)
	case parse.FunctionType:
		for _, param := range typ.Params {
			collectGenericParamsFromDeclaredType(param, params, seen)
		}
		collectGenericParamsFromDeclaredType(typ.Return, params, seen)
	case *parse.CustomType:
		for _, arg := range typ.TypeArgs {
			collectGenericParamsFromDeclaredType(arg, params, seen)
		}
	case parse.CustomType:
		for _, arg := range typ.TypeArgs {
			collectGenericParamsFromDeclaredType(arg, params, seen)
		}
	}
}

func (c *Checker) markDuplicateTopLevelTypeDeclaration(stmt parse.Statement) {
	if c.duplicateTopLevelTypeDeclarations == nil {
		c.duplicateTopLevelTypeDeclarations = map[parse.Statement]bool{}
	}
	c.duplicateTopLevelTypeDeclarations[stmt] = true
}

func (c *Checker) isDuplicateTopLevelTypeDeclaration(stmt parse.Statement) bool {
	return c.duplicateTopLevelTypeDeclarations != nil && c.duplicateTopLevelTypeDeclarations[stmt]
}

func (c *Checker) isResolvingStructDefinition(def *StructDef) bool {
	if def == nil || def.ModulePath != c.typeOwnerPath() || c.resolvingTopLevelStructs == nil || !c.resolvingTopLevelStructs[def.Name] {
		return false
	}
	sym, ok := c.scope.get(def.Name)
	return ok && sym.Type == def
}

func (c *Checker) ensureStructDefinitionResolved(def *StructDef) {
	if def == nil || c.topLevelStructDeclarations == nil {
		return
	}
	if def.ModulePath != c.typeOwnerPath() {
		return
	}
	sym, ok := c.scope.get(def.Name)
	if !ok || sym.Type != def {
		return
	}
	decl := c.topLevelStructDeclarations[def.Name]
	if decl == nil {
		return
	}
	c.populateStructDefinition(def, decl)
}

func (c *Checker) populateStructDefinition(def *StructDef, decl *parse.StructDefinition) {
	if def == nil || decl == nil {
		return
	}
	name := decl.Name.Name
	if c.resolvedTopLevelStructs != nil && c.resolvedTopLevelStructs[name] {
		return
	}
	if c.resolvingTopLevelStructs != nil && c.resolvingTopLevelStructs[name] {
		return
	}
	if c.resolvingTopLevelStructs == nil {
		c.resolvingTopLevelStructs = map[string]bool{}
	}
	c.resolvingTopLevelStructs[name] = true
	defer delete(c.resolvingTopLevelStructs, name)

	declaredGenericParams := append([]string(nil), def.GenericParams...)
	if len(declaredGenericParams) == 0 {
		declaredGenericParams = genericParamsFromStructDeclaration(decl)
	}

	def.Name = decl.Name.Name
	def.ModulePath = c.typeOwnerPath()
	def.Fields = make(map[string]Type)
	def.GenericParams = declaredGenericParams
	def.Private = decl.Private
	resolvedGenericParams := []string{}
	seenGenerics := make(map[string]bool)
	for _, field := range decl.Fields {
		fieldType := c.resolveType(field.Type)
		if fieldType == nil {
			continue
		}

		if _, dup := def.Fields[field.Name.Name]; dup {
			c.addError(fmt.Sprintf("Duplicate field: %s", field.Name.Name), field.Name.GetLocation())
			continue
		}
		def.Fields[field.Name.Name] = fieldType
		collectGenericsFromType(fieldType, &resolvedGenericParams, seenGenerics)
	}
	def.GenericParams = appendUniqueStrings(declaredGenericParams, resolvedGenericParams...)
	if len(def.GenericParams) == 0 {
		def.GenericParams = nil
	}
	if c.resolvedTopLevelStructs == nil {
		c.resolvedTopLevelStructs = map[string]bool{}
	}
	c.resolvedTopLevelStructs[name] = true
}

func (c *Checker) hoistedStruct(name string) (*StructDef, bool) {
	sym, ok := c.scope.get(name)
	if !ok {
		return nil, false
	}
	def, ok := sym.Type.(*StructDef)
	if !ok {
		return nil, false
	}
	return def, true
}

func (c *Checker) hoistedTrait(name string) (*Trait, bool) {
	sym, ok := c.scope.get(name)
	if !ok {
		return nil, false
	}
	trait, ok := sym.Type.(*Trait)
	if !ok {
		return nil, false
	}
	return trait, true
}

func (c *Checker) hoistedEnum(name string) (*Enum, bool) {
	sym, ok := c.scope.get(name)
	if !ok {
		return nil, false
	}
	enum, ok := sym.Type.(*Enum)
	if !ok {
		return nil, false
	}
	return enum, true
}

func (c *Checker) hoistedExternType(name string) (*ExternType, bool) {
	sym, ok := c.scope.get(name)
	if !ok {
		return nil, false
	}
	externType, ok := sym.Type.(*ExternType)
	if !ok {
		return nil, false
	}
	return externType, true
}

func (c *Checker) hoistedUnion(name string) (*Union, bool) {
	sym, ok := c.scope.get(name)
	if !ok {
		return nil, false
	}
	unionType, ok := sym.Type.(*Union)
	if !ok {
		return nil, false
	}
	return unionType, true
}

func (c *Checker) predeclareTopLevelTypeAliases() {
	for name := range c.topLevelTypeAliases {
		c.resolveTopLevelTypeAlias(name)
	}
}

func (c *Checker) resolveTopLevelTypeAlias(name string) Type {
	if c.resolvedTopLevelAliases != nil && c.resolvedTopLevelAliases[name] {
		if sym, ok := c.scope.get(name); ok {
			return sym.Type
		}
		return nil
	}
	decl := c.topLevelTypeAliases[name]
	if decl == nil {
		if sym, ok := c.scope.get(name); ok {
			return sym.Type
		}
		return nil
	}
	if c.resolvingTopLevelAliases != nil && c.resolvingTopLevelAliases[name] {
		c.addError(fmt.Sprintf("Recursive type alias: %s", name), decl.Name.GetLocation())
		return &TypeVar{name: "unknown"}
	}
	if c.resolvingTopLevelAliases == nil {
		c.resolvingTopLevelAliases = map[string]bool{}
	}
	c.resolvingTopLevelAliases[name] = true
	defer delete(c.resolvingTopLevelAliases, name)

	if custom, ok := decl.Type[0].(*parse.CustomType); ok && custom.Type.Target == nil {
		if _, alias := c.topLevelTypeAliases[custom.GetName()]; alias {
			c.resolveTopLevelTypeAlias(custom.GetName())
		}
	}
	resolvedType := c.resolveType(decl.Type[0])
	if resolvedType == nil {
		c.addError(fmt.Sprintf("Unrecognized type: %s", decl.Type[0].GetName()), decl.Type[0].GetLocation())
		return nil
	}
	c.scope.add(decl.Name.Name, resolvedType, false)
	if c.resolvedTopLevelAliases == nil {
		c.resolvedTopLevelAliases = map[string]bool{}
	}
	c.resolvedTopLevelAliases[name] = true
	return resolvedType
}

func (c *Checker) validateTopLevelTypeAliases() {
	for i := range c.input.Statements {
		decl, ok := c.input.Statements[i].(*parse.TypeDeclaration)
		if !ok || len(decl.Type) != 1 {
			continue
		}
		custom, ok := decl.Type[0].(*parse.CustomType)
		if !ok || len(custom.TypeArgs) > 0 {
			continue
		}
		sym, ok := c.scope.get(custom.GetName())
		if !ok {
			continue
		}
		if namedTypeRequiresTypeArguments(sym.Type) {
			c.addError(fmt.Sprintf("Generic type %s requires type arguments", custom.GetName()), custom.GetLocation())
		}
	}
}
