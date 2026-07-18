package checker

import "github.com/akonwi/ard/parse"

// checkStructFieldMapKeyTypes revalidates struct-bearing map keys after every
// top-level struct declaration has been populated. During declaration
// resolution a recursive key can still be an incomplete hoisted shell, so the
// ordinary eager validation cannot yet inspect its final fields.
func (c *Checker) checkStructFieldMapKeyTypes() {
	for _, statement := range c.input.Statements {
		decl, ok := statement.(*parse.StructDefinition)
		if !ok {
			continue
		}
		def, ok := c.hoistedStruct(decl.Name.Name)
		if !ok || def == nil {
			continue
		}
		for _, field := range decl.Fields {
			fieldType, ok := def.Fields[field.Name.Name]
			if !ok {
				continue
			}
			c.validateNestedStructMapKeys(fieldType, field.Name.GetLocation(), map[Type]bool{}, &mapKeyTypeContext{
				seen:         map[string]bool{},
				complexities: map[*StructDef][]int{},
			})
		}
	}
}

func (c *Checker) validateNestedStructMapKeys(t Type, loc parse.Location, seen map[Type]bool, context *mapKeyTypeContext) {
	if t == nil {
		return
	}
	if typ, ok := t.(*StructDef); ok {
		definition := canonicalStructDefinition(typ)
		key := definition.ModulePath + "::" + typ.String()
		if context.seen[key] {
			return
		}
		complexity := 0
		for _, arg := range typ.TypeArgs {
			complexity += mapKeyTypeComplexity(arg, map[Type]bool{})
		}
		for _, previous := range context.complexities[definition] {
			if complexity > previous {
				return
			}
		}
		context.seen[key] = true
		context.complexities[definition] = append(context.complexities[definition], complexity)
		defer func() {
			delete(context.seen, key)
			values := context.complexities[definition]
			context.complexities[definition] = values[:len(values)-1]
		}()
		for _, field := range structFields(typ) {
			c.validateNestedStructMapKeys(field, loc, seen, context)
		}
		return
	}
	if seen[t] {
		return
	}
	seen[t] = true
	defer delete(seen, t)

	switch typ := t.(type) {
	case *Map:
		if mapKeyContainsStruct(typ.Key(), map[Type]bool{}) && !isValidMapKeyType(typ.Key()) {
			c.validateMapKeyType(typ.Key(), loc)
		}
		c.validateNestedStructMapKeys(typ.Key(), loc, seen, context)
		c.validateNestedStructMapKeys(typ.Value(), loc, seen, context)
	case *List:
		c.validateNestedStructMapKeys(typ.Of(), loc, seen, context)
	case *FixedArray:
		c.validateNestedStructMapKeys(typ.Of(), loc, seen, context)
	case *Chan:
		c.validateNestedStructMapKeys(typ.Of(), loc, seen, context)
	case *Receiver:
		c.validateNestedStructMapKeys(typ.Of(), loc, seen, context)
	case *Sender:
		c.validateNestedStructMapKeys(typ.Of(), loc, seen, context)
	case *Maybe:
		c.validateNestedStructMapKeys(typ.Of(), loc, seen, context)
	case *Result:
		c.validateNestedStructMapKeys(typ.Val(), loc, seen, context)
		c.validateNestedStructMapKeys(typ.Err(), loc, seen, context)
	case *MutableRef:
		c.validateNestedStructMapKeys(typ.Of(), loc, seen, context)
	case *Union:
		for _, member := range typ.Types {
			c.validateNestedStructMapKeys(member, loc, seen, context)
		}
	case *FunctionDef:
		c.validateNestedStructMapKeys(typ.ReturnType, loc, seen, context)
		for _, param := range typ.Parameters {
			c.validateNestedStructMapKeys(param.Type, loc, seen, context)
		}
	}
}

func mapKeyContainsStruct(t Type, seen map[Type]bool) bool {
	if t == nil || seen[t] {
		return false
	}
	seen[t] = true
	switch typ := t.(type) {
	case *StructDef:
		return true
	case *FixedArray:
		return mapKeyContainsStruct(typ.Of(), seen)
	case *TypeVar:
		return typ.actual != nil && mapKeyContainsStruct(typ.actual, seen)
	default:
		return false
	}
}
