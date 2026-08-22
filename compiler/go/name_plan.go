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
	localReserved   map[string]bool
}

type enumVariantKey struct {
	typeID       air.TypeID
	name         string
	discriminant int
}

func newNamePlan(l *lowerer) *namePlan {
	program := l.program
	typeCount, traitCount, functionCount, globalCount, moduleCount := 0, 0, 0, 0, 0
	if program != nil {
		typeCount = len(program.Types)
		traitCount = len(program.Traits)
		functionCount = len(program.Functions)
		globalCount = len(program.Globals)
		moduleCount = len(program.Modules)
	}
	plan := &namePlan{
		program:         program,
		typeNames:       make(map[air.TypeID]string, typeCount),
		traitNames:      make(map[air.TraitID]string, traitCount),
		functionNames:   make(map[air.FunctionID]string, functionCount),
		globalNames:     make(map[air.GlobalID]string, globalCount),
		variantNames:    make(map[enumVariantKey]string, variantCount(program)),
		programTopLevel: make(map[string]bool, topLevelNameCount(program)),
		moduleTopLevel:  make(map[air.ModuleID]map[string]bool, moduleCount),
		localReserved:   make(map[string]bool, typeCount+functionCount+globalCount),
	}
	if l.program == nil {
		return plan
	}

	planner := newTopLevelNamePlanner(l.program)
	naturalTypeNames := map[air.TypeID]string{}
	for _, typ := range l.program.Types {
		plan.typeNames[typ.ID] = planner.typeName(typ)
		if typ.Name != "" {
			plan.localReserved[plan.typeNames[typ.ID]] = true
		}
		if name, ok := planner.naturalTypeName(typ); ok {
			naturalTypeNames[typ.ID] = name
		}
	}
	for _, trait := range l.program.Traits {
		plan.traitNames[trait.ID] = planner.traitName(trait)
	}
	for _, fn := range l.program.Functions {
		plan.functionNames[fn.ID] = planner.functionName(fn)
		if fn.Name != "" && !l.inlineClosures[fn.ID] {
			plan.localReserved[plan.functionNames[fn.ID]] = true
		}
	}
	for _, global := range l.program.Globals {
		plan.globalNames[global.ID] = planner.globalName(global)
		if global.Name != "" {
			plan.localReserved[plan.globalNames[global.ID]] = true
		}
	}

	plan.buildVariantNames(l, naturalTypeNames)
	plan.buildImportCollisionSets(l)
	return plan
}

type topLevelNameKey struct {
	kind topLevelNameKind
	id   int
}

type plannedNaturalName struct {
	key      topLevelNameKey
	name     string
	owner    air.ModuleID
	hasOwner bool
}

// topLevelNamePlanner indexes natural declaration names once. The standalone
// naming helpers intentionally remain simple reference implementations, while
// code generation uses this index to avoid rescanning every AIR declaration
// for every generated name.
type topLevelNamePlanner struct {
	program              *air.Program
	naturalByKey         map[topLevelNameKey]plannedNaturalName
	naturalByName        map[string][]plannedNaturalName
	legacyTypeBase       map[air.TypeID]string
	legacyTypeBaseCounts map[string]int
}

func newTopLevelNamePlanner(program *air.Program) *topLevelNamePlanner {
	p := &topLevelNamePlanner{
		program:              program,
		naturalByKey:         map[topLevelNameKey]plannedNaturalName{},
		naturalByName:        map[string][]plannedNaturalName{},
		legacyTypeBase:       map[air.TypeID]string{},
		legacyTypeBaseCounts: map[string]int{},
	}
	add := func(key topLevelNameKey, name string) {
		owner, hasOwner := topLevelNameModule(program, key.kind, key.id)
		planned := plannedNaturalName{key: key, name: name, owner: owner, hasOwner: hasOwner}
		p.naturalByKey[key] = planned
		p.naturalByName[name] = append(p.naturalByName[name], planned)
	}
	for _, typ := range program.Types {
		base := typeNameBase(program, typ)
		p.legacyTypeBase[typ.ID] = base
		p.legacyTypeBaseCounts[base]++
		if naturalTypeNameEligible(typ) {
			add(topLevelNameKey{kind: topLevelNameType, id: int(typ.ID)}, naturalGoIdentifier(typ.Name, !typ.Private))
		}
	}
	for _, trait := range program.Traits {
		if trait.Name != "" {
			add(topLevelNameKey{kind: topLevelNameTrait, id: int(trait.ID)}, naturalGoIdentifier(trait.Name, !trait.Private))
		}
	}
	for _, fn := range program.Functions {
		if naturalFunctionNameEligible(fn) {
			add(topLevelNameKey{kind: topLevelNameFunction, id: int(fn.ID)}, naturalGoIdentifier(fn.Name, !fn.Private))
		}
	}
	for _, global := range program.Globals {
		if global.Name != "" {
			add(topLevelNameKey{kind: topLevelNameGlobal, id: int(global.ID)}, naturalGoIdentifier(global.Name, !global.Private))
		}
	}
	return p
}

func (p *topLevelNamePlanner) naturalName(key topLevelNameKey) (string, bool) {
	planned, ok := p.naturalByKey[key]
	if !ok || planned.name == "" || planned.name == "_" {
		return "", false
	}
	return planned.name, true
}

func (p *topLevelNamePlanner) collides(key topLevelNameKey, name string, typeOrTraitOnly bool) bool {
	self := p.naturalByKey[key]
	for _, other := range p.naturalByName[name] {
		if other.key == key {
			continue
		}
		if typeOrTraitOnly && other.key.kind != topLevelNameType && other.key.kind != topLevelNameTrait {
			continue
		}
		if nameScopesOverlap(self.owner, self.hasOwner, other.owner, other.hasOwner) {
			return true
		}
	}
	return false
}

func (p *topLevelNamePlanner) naturalTypeName(typ air.TypeInfo) (string, bool) {
	key := topLevelNameKey{kind: topLevelNameType, id: int(typ.ID)}
	name, ok := p.naturalName(key)
	if !ok || isReservedTopLevelName(name) || p.collides(key, name, true) {
		return "", false
	}
	return name, true
}

func (p *topLevelNamePlanner) typeName(typ air.TypeInfo) string {
	if name, ok := p.naturalTypeName(typ); ok {
		return name
	}
	base := p.legacyTypeBase[typ.ID]
	if p.legacyTypeBaseCounts[base] > 1 {
		base = fmt.Sprintf("%s_%d", base, typ.ID)
	}
	if !typ.Private {
		base = upperFirst(base)
	}
	return base
}

func (p *topLevelNamePlanner) traitName(trait air.Trait) string {
	key := topLevelNameKey{kind: topLevelNameTrait, id: int(trait.ID)}
	name, ok := p.naturalName(key)
	if !ok || isReservedTopLevelName(name) || p.collides(key, name, true) {
		return legacyTraitInterfaceTypeName(trait)
	}
	return name
}

func (p *topLevelNamePlanner) functionName(fn air.Function) string {
	if fn.IsScript {
		return fmt.Sprintf("ArdScript_%d", fn.ID)
	}
	key := topLevelNameKey{kind: topLevelNameFunction, id: int(fn.ID)}
	name, ok := p.naturalName(key)
	if !ok {
		return legacyFunctionName(p.program, fn)
	}
	return p.valueNameAlias(key, name)
}

func (p *topLevelNamePlanner) globalName(global air.Global) string {
	key := topLevelNameKey{kind: topLevelNameGlobal, id: int(global.ID)}
	name, ok := p.naturalName(key)
	if !ok {
		return legacyGlobalName(p.program, global)
	}
	return p.valueNameAlias(key, name)
}

func (p *topLevelNamePlanner) valueNameAlias(key topLevelNameKey, base string) string {
	suffix := 0
	if isSpecialGoTopLevelName(base) || p.collides(key, base, false) {
		suffix = 1 + p.earlierAliasedValueCount(key, base)
	}
	for {
		name := base
		if suffix > 0 {
			name = fmt.Sprintf("%s_%d", base, suffix)
		}
		if !isSpecialGoTopLevelName(name) && !p.collides(key, name, false) {
			return name
		}
		suffix++
	}
}

func (p *topLevelNamePlanner) earlierAliasedValueCount(key topLevelNameKey, base string) int {
	count := 0
	for _, other := range p.naturalByName[base] {
		if other.key == key || (other.key.kind != topLevelNameFunction && other.key.kind != topLevelNameGlobal) {
			continue
		}
		if topLevelValuePrecedes(other.key.kind, other.key.id, key.kind, key.id) && (isSpecialGoTopLevelName(base) || p.collides(other.key, base, false)) {
			count++
		}
	}
	return count
}

type plannedTopLevelName struct {
	name     string
	owner    air.ModuleID
	hasOwner bool
}

func (p *namePlan) buildVariantNames(l *lowerer, naturalTypeNames map[air.TypeID]string) {
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
		if l.inlineClosures[fn.ID] {
			continue
		}
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
	localNames := p.plannedFunctionLocalNames(l)
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
		if !l.inlineClosures[fn.ID] {
			p.programTopLevel[p.functionName(fn)] = true
		}
		for _, name := range fn.TypeParams {
			p.programTopLevel[name] = true
		}
		for _, name := range localNames[fn.ID] {
			p.programTopLevel[name] = true
		}
	}

	for _, module := range p.program.Modules {
		types := l.typesByModule[module.ID]
		ownerless := l.ownerlessTypes
		if l.typesByModule == nil {
			types = l.typesForModule(module.ID, module.ID)
			ownerless = nil
		}
		capacity := len(types) + len(ownerless) + len(module.Globals) + len(module.Functions)
		for _, typ := range types {
			capacity += len(typ.Variants)
		}
		for _, typ := range ownerless {
			capacity += len(typ.Variants)
		}
		occupied := make(map[string]bool, capacity)
		p.moduleTopLevel[module.ID] = occupied
		for _, typ := range types {
			p.addTypeNames(occupied, *typ)
		}
		for _, typ := range ownerless {
			p.addTypeNames(occupied, *typ)
		}
		for _, globalID := range module.Globals {
			if globalID >= 0 && int(globalID) < len(p.program.Globals) {
				occupied[p.globalName(p.program.Globals[globalID])] = true
			}
		}
		for _, functionID := range l.functionsForModule(module.ID) {
			if !validFunctionID(p.program, functionID) {
				continue
			}
			fn := p.program.Functions[functionID]
			if !l.inlineClosures[functionID] {
				occupied[p.functionName(fn)] = true
			}
			for _, name := range fn.TypeParams {
				occupied[name] = true
			}
			for _, name := range localNames[functionID] {
				occupied[name] = true
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

// plannedFunctionLocalNames computes the exact generated local identifiers
// before import aliases are selected. Imports are package-scoped in Go, so an
// alias must avoid every local in the generated module even when the import is
// first discovered while lowering a function body.
func (p *namePlan) plannedFunctionLocalNames(l *lowerer) map[air.FunctionID][]string {
	planner := *l
	planner.namePlan = p
	planner.topLevelReserved = p.localReserved
	planner.localNameCache = nil

	result := make(map[air.FunctionID][]string, len(p.program.Functions))
	for _, fn := range p.program.Functions {
		allocated := planner.allocateLocalNames(fn)
		names := make([]string, 0, len(allocated))
		for _, name := range allocated {
			names = append(names, name)
		}
		result[fn.ID] = names
	}
	return result
}

func (p *namePlan) addTypeNames(occupied map[string]bool, typ air.TypeInfo) {
	occupied[p.typeName(typ)] = true
	for _, name := range typ.TypeParams {
		occupied[name] = true
	}
	for _, variant := range typ.Variants {
		occupied[p.enumVariantName(typ, variant)] = true
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

func variantCount(program *air.Program) int {
	if program == nil {
		return 0
	}
	count := 0
	for _, typ := range program.Types {
		count += len(typ.Variants)
	}
	return count
}

func topLevelNameCount(program *air.Program) int {
	if program == nil {
		return 0
	}
	return len(program.Types) + variantCount(program) + len(program.Traits) + len(program.Functions) + len(program.Globals)
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
