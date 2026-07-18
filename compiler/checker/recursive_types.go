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
			refs := inlineStructReferences(fieldType, map[Type]bool{}, map[string]bool{})
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
			path, found := recursivePath(edge.to, owner, edges, map[*StructDef]bool{})
			if !found {
				continue
			}
			key := recursiveCycleKey(owner, structs, edges)
			if reported[key] {
				continue
			}
			reported[key] = true
			cycle := append([]recursiveStructEdge{edge}, path...)
			references := make([]recursiveStructLayoutReference, 0, len(cycle))
			for _, cycleEdge := range cycle {
				references = append(references, recursiveStructLayoutReference{
					StructName: cycleEdge.from.Name,
					FieldName:  cycleEdge.fieldName,
					Span:       c.sourceSpan(cycleEdge.location),
				})
			}
			c.addDiagnostic(recursiveStructLayoutDiagnostic{Cycle: references}.build())
		}
	}
}

func inlineStructReferences(t Type, seen map[Type]bool, seenStructs map[string]bool) []*StructDef {
	return inlineStructReferencesWithNullable(t, seen, seenStructs)
}

func inlineStructReferencesWithNullable(t Type, seen map[Type]bool, seenStructs map[string]bool) []*StructDef {
	t = deref(t)
	if t == nil {
		return nil
	}
	if typ, ok := t.(*StructDef); ok {
		definition := canonicalStructDefinition(typ)
		key := recursiveStructApplicationKey(typ)
		if seenStructs[key] {
			return []*StructDef{definition}
		}
		seenStructs[key] = true
		defer delete(seenStructs, key)
		refs := []*StructDef{definition}
		for _, field := range structFields(typ) {
			refs = append(refs, inlineStructReferencesWithNullable(field, seen, seenStructs)...)
		}
		return refs
	}
	if seen[t] {
		return nil
	}
	seen[t] = true
	defer delete(seen, t)

	switch typ := t.(type) {
	case *FixedArray:
		return inlineStructReferencesWithNullable(typ.Of(), seen, seenStructs)
	case *Map, *Maybe:
		return nil
	case *Result:
		refs := inlineStructReferencesWithNullable(typ.Val(), seen, seenStructs)
		refs = append(refs, inlineStructReferencesWithNullable(typ.Err(), seen, seenStructs)...)
		return refs
	case *Union:
		refs := []*StructDef{}
		for _, member := range typ.Types {
			refs = append(refs, inlineStructReferencesWithNullable(member, seen, seenStructs)...)
		}
		return refs
	case *MutableRef, *List, *Trait, *FunctionDef:
		return nil
	default:
		return nil
	}
}

func recursiveStructApplicationKey(typ *StructDef) string {
	definition := canonicalStructDefinition(typ)
	return definition.ModulePath + "::" + definition.Name
}

func recursivePath(from *StructDef, to *StructDef, edges map[*StructDef][]recursiveStructEdge, seen map[*StructDef]bool) ([]recursiveStructEdge, bool) {
	if from == nil || to == nil {
		return nil, false
	}
	if from == to {
		return nil, true
	}
	if seen[from] {
		return nil, false
	}
	seen[from] = true
	for _, edge := range edges[from] {
		if path, found := recursivePath(edge.to, to, edges, seen); found {
			return append([]recursiveStructEdge{edge}, path...), true
		}
	}
	return nil, false
}

func recursivePathExists(from *StructDef, to *StructDef, edges map[*StructDef][]recursiveStructEdge, seen map[*StructDef]bool) bool {
	_, found := recursivePath(from, to, edges, seen)
	return found
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
