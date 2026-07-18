package checker

import (
	"sort"
	"strings"

	"github.com/akonwi/ard/parse"
)

type genericInstantiationEdge struct {
	from      *StructDef
	to        *StructDef
	typeArgs  []Type
	fieldName string
	location  parse.Location
}

func (c *Checker) checkGenericInstantiationCycles() {
	definitions := []*StructDef{}
	definitionSet := map[*StructDef]bool{}
	declarations := map[*StructDef]*parse.StructDefinition{}
	for _, statement := range c.input.Statements {
		decl, ok := statement.(*parse.StructDefinition)
		if !ok {
			continue
		}
		def, ok := c.hoistedStruct(decl.Name.Name)
		if !ok || def == nil || len(def.GenericParams) == 0 {
			continue
		}
		def = canonicalStructDefinition(def)
		definitions = append(definitions, def)
		definitionSet[def] = true
		declarations[def] = decl
	}
	if len(definitions) == 0 {
		return
	}

	edges := map[*StructDef][]genericInstantiationEdge{}
	for _, from := range definitions {
		decl := declarations[from]
		for _, field := range decl.Fields {
			fieldType, ok := from.Fields[field.Name.Name]
			if !ok {
				continue
			}
			for _, application := range structApplicationsInType(fieldType, map[Type]bool{}) {
				to := canonicalStructDefinition(application)
				if !definitionSet[to] || len(application.TypeArgs) != len(to.GenericParams) {
					continue
				}
				edges[from] = append(edges[from], genericInstantiationEdge{
					from:      from,
					to:        to,
					typeArgs:  application.TypeArgs,
					fieldName: field.Name.Name,
					location:  field.Name.GetLocation(),
				})
			}
		}
	}

	graph := map[genericParamNode][]genericParamEdge{}
	for from, structEdges := range edges {
		paramIndexes := make(map[string]int, len(from.GenericParams))
		paramSet := make(map[string]bool, len(from.GenericParams))
		for i, name := range from.GenericParams {
			paramIndexes[name] = i
			paramSet[name] = true
		}
		for _, structEdge := range structEdges {
			for targetIndex, arg := range structEdge.typeArgs {
				occurrences := map[genericVarEdge]bool{}
				collectGenericVarOccurrences(arg, false, paramSet, occurrences, map[Type]bool{})
				for occurrence := range occurrences {
					sourceIndex, ok := paramIndexes[occurrence.to]
					if !ok {
						continue
					}
					fromNode := genericParamNode{def: structEdge.to, index: targetIndex}
					graph[fromNode] = append(graph[fromNode], genericParamEdge{
						to:      genericParamNode{def: from, index: sourceIndex},
						wrapped: occurrence.wrapped,
						origin:  structEdge,
					})
				}
			}
		}
	}

	reported := map[string]bool{}
	for from, outgoing := range graph {
		for _, edge := range outgoing {
			if !edge.wrapped {
				continue
			}
			path, found := genericParamPath(edge.to, from, graph, map[genericParamNode]bool{})
			if !found {
				continue
			}
			origins := []genericInstantiationEdge{edge.origin}
			for _, transition := range path {
				origins = append(origins, transition.origin)
			}
			key := genericInstantiationCycleKey(origins)
			if reported[key] {
				continue
			}
			reported[key] = true
			references := make([]genericInstantiationCycleReference, 0, len(origins))
			seenOrigins := map[string]bool{}
			for _, origin := range origins {
				originKey := origin.from.ModulePath + "::" + origin.from.Name + "." + origin.fieldName
				if seenOrigins[originKey] {
					continue
				}
				seenOrigins[originKey] = true
				references = append(references, genericInstantiationCycleReference{
					StructName: origin.from.Name,
					FieldName:  origin.fieldName,
					Span:       c.sourceSpan(origin.location),
				})
			}
			c.addDiagnostic(genericInstantiationCycleDiagnostic{Cycle: references}.build())
		}
	}
}

type genericParamNode struct {
	def   *StructDef
	index int
}

type genericParamEdge struct {
	to      genericParamNode
	wrapped bool
	origin  genericInstantiationEdge
}

func genericParamPath(from, to genericParamNode, graph map[genericParamNode][]genericParamEdge, seen map[genericParamNode]bool) ([]genericParamEdge, bool) {
	if from == to {
		return nil, true
	}
	if seen[from] {
		return nil, false
	}
	seen[from] = true
	for _, edge := range graph[from] {
		if path, found := genericParamPath(edge.to, to, graph, seen); found {
			return append([]genericParamEdge{edge}, path...), true
		}
	}
	return nil, false
}

func structApplicationsInType(t Type, seen map[Type]bool) []*StructDef {
	if t == nil || seen[t] {
		return nil
	}
	seen[t] = true
	switch typ := t.(type) {
	case *StructDef:
		applications := []*StructDef{}
		if typ.Definition != nil && len(typ.TypeArgs) > 0 {
			applications = append(applications, typ)
		}
		for _, arg := range typ.TypeArgs {
			applications = append(applications, structApplicationsInType(arg, seen)...)
		}
		return applications
	case *List:
		return structApplicationsInType(typ.Of(), seen)
	case *FixedArray:
		return structApplicationsInType(typ.Of(), seen)
	case *Chan:
		return structApplicationsInType(typ.Of(), seen)
	case *Receiver:
		return structApplicationsInType(typ.Of(), seen)
	case *Sender:
		return structApplicationsInType(typ.Of(), seen)
	case *Map:
		applications := structApplicationsInType(typ.Key(), seen)
		return append(applications, structApplicationsInType(typ.Value(), seen)...)
	case *Maybe:
		return structApplicationsInType(typ.Of(), seen)
	case *Result:
		applications := structApplicationsInType(typ.Val(), seen)
		return append(applications, structApplicationsInType(typ.Err(), seen)...)
	case *MutableRef:
		return structApplicationsInType(typ.Of(), seen)
	case *Union:
		applications := []*StructDef{}
		for _, member := range typ.Types {
			applications = append(applications, structApplicationsInType(member, seen)...)
		}
		return applications
	case *FunctionDef:
		applications := structApplicationsInType(typ.ReturnType, seen)
		for _, param := range typ.Parameters {
			applications = append(applications, structApplicationsInType(param.Type, seen)...)
		}
		return applications
	case *ForeignType:
		applications := []*StructDef{}
		for _, arg := range typ.TypeArgs {
			applications = append(applications, structApplicationsInType(arg, seen)...)
		}
		return applications
	default:
		return nil
	}
}

type genericVarEdge struct {
	to      string
	wrapped bool
}

func collectGenericVarOccurrences(t Type, wrapped bool, params map[string]bool, out map[genericVarEdge]bool, seen map[Type]bool) {
	if t == nil {
		return
	}
	if typ, ok := t.(*TypeVar); ok {
		if typ.bound && typ.actual != nil {
			collectGenericVarOccurrences(typ.actual, wrapped, params, out, seen)
		} else if params[typ.name] {
			out[genericVarEdge{to: typ.name, wrapped: wrapped}] = true
		}
		return
	}
	if seen[t] {
		return
	}
	seen[t] = true
	switch typ := t.(type) {
	case *List:
		collectGenericVarOccurrences(typ.Of(), true, params, out, seen)
	case *FixedArray:
		collectGenericVarOccurrences(typ.Of(), true, params, out, seen)
	case *Chan:
		collectGenericVarOccurrences(typ.Of(), true, params, out, seen)
	case *Receiver:
		collectGenericVarOccurrences(typ.Of(), true, params, out, seen)
	case *Sender:
		collectGenericVarOccurrences(typ.Of(), true, params, out, seen)
	case *Map:
		collectGenericVarOccurrences(typ.Key(), true, params, out, seen)
		collectGenericVarOccurrences(typ.Value(), true, params, out, seen)
	case *Maybe:
		collectGenericVarOccurrences(typ.Of(), true, params, out, seen)
	case *Result:
		collectGenericVarOccurrences(typ.Val(), true, params, out, seen)
		collectGenericVarOccurrences(typ.Err(), true, params, out, seen)
	case *MutableRef:
		collectGenericVarOccurrences(typ.Of(), true, params, out, seen)
	case *Union:
		for _, member := range typ.Types {
			collectGenericVarOccurrences(member, true, params, out, seen)
		}
	case *FunctionDef:
		collectGenericVarOccurrences(typ.ReturnType, true, params, out, seen)
		for _, param := range typ.Parameters {
			collectGenericVarOccurrences(param.Type, true, params, out, seen)
		}
	case *StructDef:
		for _, arg := range typ.TypeArgs {
			collectGenericVarOccurrences(arg, true, params, out, seen)
		}
	case *ForeignType:
		for _, arg := range typ.TypeArgs {
			collectGenericVarOccurrences(arg, true, params, out, seen)
		}
	}
}

func genericInstantiationCycleKey(cycle []genericInstantiationEdge) string {
	parts := make([]string, len(cycle))
	for i, edge := range cycle {
		parts[i] = edge.from.ModulePath + "::" + edge.from.Name + "." + edge.fieldName
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}
