package gotarget

import (
	"fmt"
	"sort"

	"github.com/akonwi/ard/air"
)

// namePlan caches generated top-level names and import-collision sets for one
// lowering invocation. Declaration names use the existing naming rules;
// variants are resolved once in the same TypeID/source order as the reference
// implementation instead of recursively recomputing every predecessor.
type namePlan struct {
	program *air.Program

	typeNames     map[air.TypeID]string
	traitNames    map[air.TraitID]string
	functionNames map[air.FunctionID]string
	globalNames   map[air.GlobalID]string
	variantNames  map[enumVariantKey]string

	programTopLevel map[string]bool
	moduleTopLevel  map[air.ModuleID]map[string]bool
}

type enumVariantKey struct {
	typeID       air.TypeID
	name         string
	discriminant int
}

func newNamePlan(l *lowerer) *namePlan {
	plan := &namePlan{
		program:         l.program,
		typeNames:       map[air.TypeID]string{},
		traitNames:      map[air.TraitID]string{},
		functionNames:   map[air.FunctionID]string{},
		globalNames:     map[air.GlobalID]string{},
		variantNames:    map[enumVariantKey]string{},
		programTopLevel: map[string]bool{},
		moduleTopLevel:  map[air.ModuleID]map[string]bool{},
	}
	if l.program == nil {
		return plan
	}

	naturalTypeNames := map[air.TypeID]string{}
	for _, typ := range l.program.Types {
		plan.typeNames[typ.ID] = typeName(l.program, typ)
		if name, ok := naturalTypeName(l.program, typ); ok {
			naturalTypeNames[typ.ID] = name
		}
	}
	for _, trait := range l.program.Traits {
		plan.traitNames[trait.ID] = l.traitInterfaceTypeName(trait)
	}
	for _, fn := range l.program.Functions {
		plan.functionNames[fn.ID] = functionName(l.program, fn)
	}
	for _, global := range l.program.Globals {
		plan.globalNames[global.ID] = globalName(l.program, global)
	}

	plan.buildVariantNames(naturalTypeNames)
	plan.buildImportCollisionSets(l)
	return plan
}

type plannedTopLevelName struct {
	name     string
	owner    air.ModuleID
	hasOwner bool
}

func (p *namePlan) buildVariantNames(naturalTypeNames map[air.TypeID]string) {
	actualNames := make([]plannedTopLevelName, 0, len(p.program.Types)+len(p.program.Traits)+len(p.program.Functions)+len(p.program.Globals))
	for _, typ := range p.program.Types {
		owner, ok := topLevelNameModule(p.program, topLevelNameType, int(typ.ID))
		actualNames = append(actualNames, plannedTopLevelName{name: p.typeName(typ), owner: owner, hasOwner: ok})
	}
	for _, trait := range p.program.Traits {
		owner, ok := topLevelNameModule(p.program, topLevelNameTrait, int(trait.ID))
		actualNames = append(actualNames, plannedTopLevelName{name: p.traitName(trait), owner: owner, hasOwner: ok})
	}
	for _, fn := range p.program.Functions {
		owner, ok := topLevelNameModule(p.program, topLevelNameFunction, int(fn.ID))
		actualNames = append(actualNames, plannedTopLevelName{name: p.functionName(fn), owner: owner, hasOwner: ok})
	}
	for _, global := range p.program.Globals {
		owner, ok := topLevelNameModule(p.program, topLevelNameGlobal, int(global.ID))
		actualNames = append(actualNames, plannedTopLevelName{name: p.globalName(global), owner: owner, hasOwner: ok})
	}

	typesByID := append([]air.TypeInfo(nil), p.program.Types...)
	sort.SliceStable(typesByID, func(i, j int) bool { return typesByID[i].ID < typesByID[j].ID })
	preceding := []plannedTopLevelName{}
	for _, typ := range typesByID {
		if typ.Kind != air.TypeEnum {
			continue
		}
		owner, hasOwner := topLevelNameModule(p.program, topLevelNameType, int(typ.ID))
		for _, variant := range typ.Variants {
			key := variantPlanKey(typ, variant)
			if _, exists := p.variantNames[key]; exists {
				continue
			}
			name := p.plannedVariantName(typ, variant, naturalTypeNames[typ.ID], owner, hasOwner, actualNames, preceding)
			p.variantNames[key] = name
			preceding = append(preceding, plannedTopLevelName{name: name, owner: owner, hasOwner: hasOwner})
		}
	}
}

func (p *namePlan) plannedVariantName(typ air.TypeInfo, variant air.VariantInfo, naturalType string, owner air.ModuleID, hasOwner bool, actualNames, preceding []plannedTopLevelName) string {
	if naturalType == "" || variant.Name == "" || len(goIdentifierParts(variant.Name)) == 0 {
		return p.legacyVariantName(typ, variant)
	}
	variantPart := naturalGoIdentifier(variant.Name, true)
	if variantPart == "" || variantPart == "_" {
		return p.legacyVariantName(typ, variant)
	}
	base := naturalType + variantPart
	candidate := base
	for suffix := 1; p.variantNameCollides(owner, hasOwner, candidate, actualNames, preceding); suffix++ {
		candidate = fmt.Sprintf("%s_%d", base, suffix)
	}
	return candidate
}

func (p *namePlan) legacyVariantName(typ air.TypeInfo, variant air.VariantInfo) string {
	name := sanitizeName(variant.Name)
	if name == "" {
		name = fmt.Sprintf("variant_%d", variant.Discriminant)
	}
	return p.typeName(typ) + "__" + name
}

func (p *namePlan) variantNameCollides(owner air.ModuleID, hasOwner bool, candidate string, actualNames, preceding []plannedTopLevelName) bool {
	if isSpecialGoTopLevelName(candidate) {
		return true
	}
	for _, existing := range actualNames {
		if nameScopesOverlap(owner, hasOwner, existing.owner, existing.hasOwner) && existing.name == candidate {
			return true
		}
	}
	for _, existing := range preceding {
		if nameScopesOverlap(owner, hasOwner, existing.owner, existing.hasOwner) && existing.name == candidate {
			return true
		}
	}
	return false
}

func nameScopesOverlap(left air.ModuleID, leftKnown bool, right air.ModuleID, rightKnown bool) bool {
	return !leftKnown || !rightKnown || left == right
}

func (p *namePlan) buildImportCollisionSets(l *lowerer) {
	for _, typ := range p.program.Types {
		p.addTypeNames(p.programTopLevel, typ)
	}
	for _, trait := range p.program.Traits {
		p.programTopLevel[p.traitName(trait)] = true
	}
	for _, global := range p.program.Globals {
		p.programTopLevel[p.globalName(global)] = true
	}
	for _, fn := range p.program.Functions {
		p.programTopLevel[p.functionName(fn)] = true
	}

	for _, module := range p.program.Modules {
		occupied := map[string]bool{}
		p.moduleTopLevel[module.ID] = occupied
		for _, typ := range l.typesForModule(module.ID, module.ID) {
			p.addTypeNames(occupied, typ)
		}
		for _, globalID := range module.Globals {
			if globalID >= 0 && int(globalID) < len(p.program.Globals) {
				occupied[p.globalName(p.program.Globals[globalID])] = true
			}
		}
		for _, functionID := range l.functionsForModule(module.ID) {
			if validFunctionID(p.program, functionID) {
				occupied[p.functionName(p.program.Functions[functionID])] = true
			}
		}
		for _, trait := range p.program.Traits {
			owner, ok := l.ownerModuleForTrait(trait.ID)
			if ok && owner == module.ID {
				occupied[p.traitName(trait)] = true
			}
		}
	}
}

func (p *namePlan) addTypeNames(occupied map[string]bool, typ air.TypeInfo) {
	occupied[p.typeName(typ)] = true
	for _, variant := range typ.Variants {
		occupied[p.enumVariantName(typ, variant)] = true
	}
}

func (p *namePlan) addTypeDuringLowering(l *lowerer, typ air.TypeInfo) {
	if p == nil {
		return
	}
	p.typeNames[typ.ID] = typeName(p.program, typ)
	p.addTypeNames(p.programTopLevel, typ)
	for _, module := range p.program.Modules {
		for _, moduleType := range l.typesForModule(module.ID, module.ID) {
			if moduleType.ID == typ.ID {
				p.addTypeNames(p.moduleTopLevel[module.ID], typ)
				break
			}
		}
	}
}

func (p *namePlan) typeName(typ air.TypeInfo) string {
	if p == nil {
		return typeName(nil, typ)
	}
	if name, ok := p.typeNames[typ.ID]; ok {
		return name
	}
	return typeName(p.program, typ)
}

func (p *namePlan) traitName(trait air.Trait) string {
	if p == nil {
		return (&lowerer{}).traitInterfaceTypeName(trait)
	}
	if name, ok := p.traitNames[trait.ID]; ok {
		return name
	}
	return (&lowerer{program: p.program}).traitInterfaceTypeName(trait)
}

func (p *namePlan) functionName(fn air.Function) string {
	if p == nil {
		return functionName(nil, fn)
	}
	if name, ok := p.functionNames[fn.ID]; ok {
		return name
	}
	return functionName(p.program, fn)
}

func (p *namePlan) globalName(global air.Global) string {
	if p == nil {
		return globalName(nil, global)
	}
	if name, ok := p.globalNames[global.ID]; ok {
		return name
	}
	return globalName(p.program, global)
}

func (p *namePlan) enumVariantName(typ air.TypeInfo, variant air.VariantInfo) string {
	if p == nil {
		return enumVariantName(nil, typ, variant)
	}
	if name, ok := p.variantNames[variantPlanKey(typ, variant)]; ok {
		return name
	}
	return enumVariantName(p.program, typ, variant)
}

func variantPlanKey(typ air.TypeInfo, variant air.VariantInfo) enumVariantKey {
	return enumVariantKey{typeID: typ.ID, name: variant.Name, discriminant: variant.Discriminant}
}

func (l *lowerer) typeName(typ air.TypeInfo) string {
	if l.namePlan != nil {
		return l.namePlan.typeName(typ)
	}
	return typeName(l.program, typ)
}

func (l *lowerer) functionName(fn air.Function) string {
	if l.namePlan != nil {
		return l.namePlan.functionName(fn)
	}
	return functionName(l.program, fn)
}

func (l *lowerer) globalName(global air.Global) string {
	if l.namePlan != nil {
		return l.namePlan.globalName(global)
	}
	return globalName(l.program, global)
}

func (l *lowerer) enumVariantName(typ air.TypeInfo, variant air.VariantInfo) string {
	if l.namePlan != nil {
		return l.namePlan.enumVariantName(typ, variant)
	}
	return enumVariantName(l.program, typ, variant)
}

func (p *namePlan) importAliasCollides(useModulePackages bool, module air.ModuleID, alias string) bool {
	if p == nil {
		return false
	}
	if !useModulePackages {
		return p.programTopLevel[alias]
	}
	return p.moduleTopLevel[module][alias]
}
