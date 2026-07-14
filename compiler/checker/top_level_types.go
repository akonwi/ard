package checker

import (
	"fmt"
	"sort"

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
		if original, dup := seen[name]; dup {
			c.addDiagnostic(duplicateDeclarationDiagnostic{
				Name:          name,
				DuplicateSpan: c.sourceSpan(loc),
				OriginalSpan:  c.sourceSpan(original),
			}.build())
			c.markDuplicateTopLevelTypeDeclaration(stmt)
			continue
		}
		seen[name] = loc
		if isReservedBuiltinTypeName(name) {
			c.addDiagnostic(builtInTypeRedeclarationDiagnostic{
				Name: name,
				Span: c.sourceSpan(loc),
			}.build())
			c.markDuplicateTopLevelTypeDeclaration(stmt)
			continue
		}

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
			c.scope.add(name, &Trait{Name: s.Name.Name, ModulePath: c.typeOwnerPath(), private: s.Private}, false)
		case *parse.EnumDefinition:
			c.scope.add(name, &Enum{Name: s.Name, ModulePath: c.typeOwnerPath(), Private: s.Private, Methods: make(map[string]*FunctionDef), Location: s.GetLocation()}, false)
		case *parse.TypeDeclaration:
			if len(s.Type) == 1 {
				if c.topLevelTypeAliases == nil {
					c.topLevelTypeAliases = map[string]*parse.TypeDeclaration{}
				}
				c.topLevelTypeAliases[name] = s
			} else {
				c.scope.add(name, &Union{Name: s.Name.Name, ModulePath: c.typeOwnerPath(), Private: s.Private}, false)
			}
		}
	}
}

// hoistTopLevelFunctionSignatures pre-resolves the signatures of top-level
// function declarations and adds them to module scope so functions can be
// referenced before their declaration within a module.
func (c *Checker) hoistTopLevelFunctionSignatures() {
	c.hoistedTopLevelFunctions = map[*parse.FunctionDeclaration]*FunctionDef{}
	for i := range c.input.Statements {
		def, ok := c.input.Statements[i].(*parse.FunctionDeclaration)
		if !ok {
			continue
		}
		params := c.resolveParametersWithContext(def.Parameters, nil)
		returnType := c.resolveReturnTypeWithContext(def.ReturnType, nil)
		resolved := true
		for i, param := range def.Parameters {
			if param.Type != nil && params[i].Type == nil {
				resolved = false
				break
			}
		}
		if !resolved {
			// Leave unresolvable signatures to the in-order pass so its
			// diagnostics and panics behave as before.
			continue
		}
		fn := &FunctionDef{
			Name:          def.Name,
			GenericParams: append([]string(nil), def.TypeParams...),
			Parameters:    params,
			ReturnType:    returnType,
			Body:          nil,
			Private:       def.Private,
			IsTest:        def.IsTest,
		}
		c.hoistedTopLevelFunctions[def] = fn
		c.scope.add(def.Name, fn, false)
	}
}

func (c *Checker) populateTopLevelTypeDefinitions() {
	for i := range c.input.Statements {
		switch c.input.Statements[i].(type) {
		case *parse.StructDefinition, *parse.TraitDefinition, *parse.EnumDefinition, *parse.TypeDeclaration:
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
		return s.Name, s.NameLocation, true
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
	case *parse.FixedArray:
		collectGenericParamsFromDeclaredType(typ.Element, params, seen)
	case parse.FixedArray:
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
	def.DeclaredGenerics = len(decl.TypeParams) > 0
	def.Private = decl.Private
	resolvedGenericParams := []string{}
	seenGenerics := make(map[string]bool)
	fieldLocations := make(map[string]parse.Location)
	for _, field := range decl.Fields {
		fieldType := c.resolveType(field.Type)
		if fieldType == nil {
			continue
		}

		if original, dup := fieldLocations[field.Name.Name]; dup {
			c.addDiagnostic(duplicateFieldDeclarationDiagnostic{
				Name:          field.Name.Name,
				DuplicateSpan: c.sourceSpan(field.Name.GetLocation()),
				OriginalSpan:  c.sourceSpan(original),
			}.build())
			continue
		}
		fieldLocations[field.Name.Name] = field.Name.GetLocation()
		def.Fields[field.Name.Name] = fieldType
		if c.spans != nil {
			c.spans.add(SpanRecord{
				Loc:   field.Name.GetLocation(),
				Key:   MemberKey(TargetField, def.ModulePath, def.Name, field.Name.Name),
				IsDef: true,
			})
		}
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

type typeAliasResolutionEdge struct {
	from     string
	to       string
	location parse.Location
}

func (c *Checker) predeclareTopLevelTypeAliases() {
	names := make([]string, 0, len(c.topLevelTypeAliases))
	for name := range c.topLevelTypeAliases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c.resolveTopLevelTypeAlias(name)
	}
}

func (c *Checker) resolveTopLevelTypeAlias(name string) Type {
	if c.recursiveTopLevelAliases[name] {
		return &TypeVar{name: "unknown"}
	}
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
		references := make([]recursiveTypeAliasReference, 0, len(c.resolvingTopLevelAliasEdges))
		cycleStart := 0
		for i, edge := range c.resolvingTopLevelAliasEdges {
			if edge.from == name {
				cycleStart = i
				break
			}
		}
		if c.recursiveTopLevelAliases == nil {
			c.recursiveTopLevelAliases = map[string]bool{}
		}
		for _, edge := range c.resolvingTopLevelAliasEdges[cycleStart:] {
			c.recursiveTopLevelAliases[edge.from] = true
			c.recursiveTopLevelAliases[edge.to] = true
			references = append(references, recursiveTypeAliasReference{
				From: edge.from,
				To:   edge.to,
				Span: c.sourceSpan(edge.location),
			})
		}
		c.addDiagnostic(recursiveTypeAliasDiagnostic{
			Name:         name,
			FallbackSpan: c.sourceSpan(decl.Name.GetLocation()),
			References:   references,
		}.build())
		return &TypeVar{name: "unknown"}
	}
	if c.resolvingTopLevelAliases == nil {
		c.resolvingTopLevelAliases = map[string]bool{}
	}
	c.resolvingTopLevelAliases[name] = true
	c.resolvingTopLevelAliasNames = append(c.resolvingTopLevelAliasNames, name)
	defer func() {
		delete(c.resolvingTopLevelAliases, name)
		c.resolvingTopLevelAliasNames = c.resolvingTopLevelAliasNames[:len(c.resolvingTopLevelAliasNames)-1]
	}()

	resolvedType := c.resolveType(decl.Type[0])
	if c.recursiveTopLevelAliases[name] {
		return &TypeVar{name: "unknown"}
	}
	if resolvedType == nil {
		c.addUnresolvedReference(unrecognizedType, decl.Type[0].GetName(), decl.Type[0].GetLocation())
		return nil
	}
	c.scope.add(decl.Name.Name, resolvedType, false)
	if c.resolvedTopLevelAliases == nil {
		c.resolvedTopLevelAliases = map[string]bool{}
	}
	c.resolvedTopLevelAliases[name] = true
	return resolvedType
}

func (c *Checker) resolveTopLevelTypeAliasReference(name string, location parse.Location) Type {
	if len(c.resolvingTopLevelAliasNames) == 0 {
		return c.resolveTopLevelTypeAlias(name)
	}
	from := c.resolvingTopLevelAliasNames[len(c.resolvingTopLevelAliasNames)-1]
	c.resolvingTopLevelAliasEdges = append(c.resolvingTopLevelAliasEdges, typeAliasResolutionEdge{
		from:     from,
		to:       name,
		location: location,
	})
	defer func() {
		c.resolvingTopLevelAliasEdges = c.resolvingTopLevelAliasEdges[:len(c.resolvingTopLevelAliasEdges)-1]
	}()
	return c.resolveTopLevelTypeAlias(name)
}

func (c *Checker) validateTopLevelTypeAliases() {
	for i := range c.input.Statements {
		decl, ok := c.input.Statements[i].(*parse.TypeDeclaration)
		if !ok || len(decl.Type) != 1 || c.recursiveTopLevelAliases[decl.Name.Name] {
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
