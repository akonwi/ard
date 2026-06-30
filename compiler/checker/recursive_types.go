package checker

import (
	"sort"
	"strings"

	"github.com/akonwi/ard/parse"
)

const recursiveLayoutDiagnostic = "Put the recursive reference behind mut, list, map, nullable, trait, or function indirection."

type recursiveStructEdge struct {
	from      *StructDef
	to        *StructDef
	fieldName string
	location  parse.Location
}

func (c *Checker) checkRecursiveStructLayouts() {
	structs := []*StructDef{}
	structSet := map[*StructDef]bool{}
	for i := range c.input.Statements {
		decl, ok := c.input.Statements[i].(*parse.StructDefinition)
		if !ok {
			continue
		}
		def, ok := c.hoistedStruct(decl.Name.Name)
		if !ok || def == nil {
			continue
		}
		structs = append(structs, def)
		structSet[def] = true
	}
	if len(structs) == 0 {
		return
	}

	edges := map[*StructDef][]recursiveStructEdge{}
	for i := range c.input.Statements {
		decl, ok := c.input.Statements[i].(*parse.StructDefinition)
		if !ok {
			continue
		}
		owner, ok := c.hoistedStruct(decl.Name.Name)
		if !ok || owner == nil {
			continue
		}
		for _, field := range decl.Fields {
			fieldType, ok := owner.Fields[field.Name.Name]
			if !ok {
				continue
			}
			refs := inlineStructReferences(fieldType, map[Type]bool{})
			refs = append(refs, recursiveMapKeyReferences(fieldType, map[Type]bool{})...)
			for _, ref := range refs {
				if ref == nil || !structSet[ref] {
					continue
				}
				edges[owner] = append(edges[owner], recursiveStructEdge{
					from:      owner,
					to:        ref,
					fieldName: field.Name.Name,
					location:  field.Name.GetLocation(),
				})
			}
		}
	}

	reported := map[string]bool{}
	for _, owner := range structs {
		for _, edge := range edges[owner] {
			if !recursivePathExists(edge.to, owner, edges, map[*StructDef]bool{}) {
				continue
			}
			key := recursiveCycleKey(owner, structs, edges)
			if reported[key] {
				continue
			}
			reported[key] = true
			c.addError("Recursive field "+edge.from.Name+"."+edge.fieldName+" has infinite size. "+recursiveLayoutDiagnostic, edge.location)
		}
	}
}

func inlineStructReferences(t Type, seen map[Type]bool) []*StructDef {
	return inlineStructReferencesWithNullable(t, seen, true)
}

func inlineStructReferencesWithNullable(t Type, seen map[Type]bool, nullableIsBoundary bool) []*StructDef {
	t = deref(t)
	if t == nil {
		return nil
	}
	if _, ok := seen[t]; ok {
		return nil
	}
	seen[t] = true

	switch typ := t.(type) {
	case *StructDef:
		refs := []*StructDef{typ}
		for _, field := range typ.Fields {
			refs = append(refs, inlineStructReferencesWithNullable(field, seen, true)...)
		}
		return refs
	case *Map:
		return inlineStructReferencesWithNullable(typ.Key(), seen, false)
	case *Maybe:
		if nullableIsBoundary {
			return nil
		}
		return inlineStructReferencesWithNullable(typ.Of(), seen, false)
	case *Result:
		refs := inlineStructReferencesWithNullable(typ.Val(), seen, false)
		refs = append(refs, inlineStructReferencesWithNullable(typ.Err(), seen, false)...)
		return refs
	case *Union:
		refs := []*StructDef{}
		for _, member := range typ.Types {
			refs = append(refs, inlineStructReferencesWithNullable(member, seen, false)...)
		}
		return refs
	case *MutableRef, *List, *Trait, *FunctionDef:
		return nil
	default:
		return nil
	}
}

func recursiveMapKeyReferences(t Type, seen map[Type]bool) []*StructDef {
	t = deref(t)
	if t == nil {
		return nil
	}
	if _, ok := seen[t]; ok {
		return nil
	}
	seen[t] = true
	switch typ := t.(type) {
	case *Map:
		refs := inlineStructReferencesWithNullable(typ.Key(), map[Type]bool{}, false)
		refs = append(refs, recursiveMapKeyReferences(typ.Key(), seen)...)
		refs = append(refs, recursiveMapKeyReferences(typ.Value(), seen)...)
		return refs
	case *List:
		return recursiveMapKeyReferences(typ.Of(), seen)
	case *Maybe:
		return recursiveMapKeyReferences(typ.Of(), seen)
	case *Result:
		refs := recursiveMapKeyReferences(typ.Val(), seen)
		refs = append(refs, recursiveMapKeyReferences(typ.Err(), seen)...)
		return refs
	case *MutableRef:
		return nil
	case *Union:
		refs := []*StructDef{}
		for _, member := range typ.Types {
			refs = append(refs, recursiveMapKeyReferences(member, seen)...)
		}
		return refs
	case *StructDef:
		refs := []*StructDef{}
		for _, field := range typ.Fields {
			refs = append(refs, recursiveMapKeyReferences(field, seen)...)
		}
		return refs
	case *FunctionDef:
		refs := recursiveMapKeyReferences(typ.ReturnType, seen)
		for _, param := range typ.Parameters {
			refs = append(refs, recursiveMapKeyReferences(param.Type, seen)...)
		}
		return refs
	default:
		return nil
	}
}

func recursivePathExists(from *StructDef, to *StructDef, edges map[*StructDef][]recursiveStructEdge, seen map[*StructDef]bool) bool {
	if from == nil || to == nil {
		return false
	}
	if from == to {
		return true
	}
	if seen[from] {
		return false
	}
	seen[from] = true
	for _, edge := range edges[from] {
		if recursivePathExists(edge.to, to, edges, seen) {
			return true
		}
	}
	return false
}

func recursiveCycleKey(owner *StructDef, structs []*StructDef, edges map[*StructDef][]recursiveStructEdge) string {
	parts := []string{}
	for _, candidate := range structs {
		if recursivePathExists(owner, candidate, edges, map[*StructDef]bool{}) && recursivePathExists(candidate, owner, edges, map[*StructDef]bool{}) {
			parts = append(parts, candidate.ModulePath+"::"+candidate.Name)
		}
	}
	if len(parts) == 0 {
		return owner.ModulePath + "::" + owner.Name
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}
