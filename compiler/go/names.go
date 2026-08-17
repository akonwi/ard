package gotarget

import (
	"fmt"
	"go/token"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/goname"
)

func moduleFileName(program *air.Program, module air.Module) string {
	name := moduleFileBaseName(program, module.ID)
	if strings.HasSuffix(name, "_test") {
		name += "_ard"
	}
	return name + ".go"
}

func modulePackageName(program *air.Program, module air.ModuleID) string {
	if program == nil || module < 0 || int(module) >= len(program.Modules) {
		return "module"
	}
	name := goPackageNameFromModulePath(program.Modules[module].Path)
	// `main` is reserved for the synthetic entry package, which is generated
	// separately and never a transpiled Ard module (ADR 0031).
	if name == "main" {
		return "main_"
	}
	return name
}

func modulePackageDir(program *air.Program, module air.ModuleID) string {
	return modulePackageDirWithProject(program, module, "")
}

func modulePackageDirWithProject(program *air.Program, module air.ModuleID, projectName string) string {
	if program == nil || module < 0 || int(module) >= len(program.Modules) {
		return "module"
	}
	pathNoExt := strings.TrimSuffix(program.Modules[module].Path, filepath.Ext(program.Modules[module].Path))
	if projectName != "" {
		pathNoExt = strings.TrimPrefix(pathNoExt, projectName+"/")
		pathNoExt = strings.TrimPrefix(pathNoExt, projectName+"\\")
	}
	parts := strings.FieldsFunc(pathNoExt, func(r rune) bool { return r == '/' || r == '\\' })
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		name := sanitizeGoPackageIdentifier(part)
		if name != "" {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return modulePackageName(program, module)
	}
	return filepath.Join(out...)
}

func moduleImportPath(program *air.Program, module air.ModuleID) string {
	return moduleImportPathWithPrefix(program, module, "generated")
}

func moduleImportPathWithPrefix(program *air.Program, module air.ModuleID, modulePath string) string {
	return moduleImportPathForProject(program, module, modulePath, "")
}

func moduleImportPathForProject(program *air.Program, module air.ModuleID, modulePath string, projectName string) string {
	if strings.TrimSpace(modulePath) == "" {
		modulePath = "generated"
	}
	dir := modulePackageDirWithProject(program, module, projectName)
	if dir == "" || dir == "." {
		return modulePath
	}
	return path.Join(modulePath, filepath.ToSlash(dir))
}

func moduleFileBaseName(program *air.Program, module air.ModuleID) string {
	return moduleNameWithPath(program, module, func(path string) string {
		return strings.TrimSuffix(path, filepath.Ext(path))
	})
}

func moduleName(program *air.Program, module air.ModuleID) string {
	return moduleNameWithPath(program, module, func(path string) string { return path })
}

func moduleNameWithPath(program *air.Program, module air.ModuleID, pathName func(string) string) string {
	if program == nil || module < 0 || int(module) >= len(program.Modules) {
		return fmt.Sprintf("module_%d", module)
	}
	base := sanitizeName(pathName(program.Modules[module].Path))
	if base == "" {
		base = fmt.Sprintf("module_%d", module)
	}
	for _, other := range program.Modules {
		if other.ID == module {
			continue
		}
		otherBase := sanitizeName(pathName(other.Path))
		if otherBase == "" {
			otherBase = fmt.Sprintf("module_%d", other.ID)
		}
		if otherBase == base {
			return fmt.Sprintf("%s_m%d", base, module)
		}
	}
	return base
}

func globalName(program *air.Program, global air.Global) string {
	if name, ok := naturalGlobalName(program, global); ok {
		return name
	}
	return legacyGlobalName(program, global)
}

func naturalGlobalName(program *air.Program, global air.Global) (string, bool) {
	if global.Name == "" {
		return "", false
	}
	name := naturalGoIdentifier(global.Name, !global.Private)
	if name == "" || name == "_" {
		return "", false
	}
	return topLevelValueNameAlias(program, topLevelNameGlobal, int(global.ID), name), true
}

func legacyGlobalName(program *air.Program, global air.Global) string {
	moduleName := moduleName(program, global.Module)
	name := sanitizeName(global.Name)
	if name == "" {
		name = fmt.Sprintf("global_%d", global.ID)
	}
	out := moduleName + "__global_" + name
	if !global.Private {
		out = upperFirst(out)
	}
	return out
}

func functionName(program *air.Program, fn air.Function) string {
	// Script roots (top-level statement programs) are exported so the synthetic
	// main package can call them across the package boundary.
	if fn.IsScript {
		return fmt.Sprintf("ArdScript_%d", fn.ID)
	}
	if name, ok := naturalFunctionName(program, fn); ok {
		return name
	}
	return legacyFunctionName(program, fn)
}

func naturalFunctionName(program *air.Program, fn air.Function) (string, bool) {
	if !naturalFunctionNameEligible(fn) {
		return "", false
	}
	name := naturalGoIdentifier(fn.Name, !fn.Private)
	if name == "" || name == "_" {
		return "", false
	}
	return topLevelValueNameAlias(program, topLevelNameFunction, int(fn.ID), name), true
}

func naturalFunctionNameEligible(fn air.Function) bool {
	return fn.Name != "" && !fn.IsScript && fn.Receiver == air.NoType && len(fn.Captures) == 0 && !strings.HasPrefix(fn.Name, "anon_func_")
}

func legacyFunctionName(program *air.Program, fn air.Function) string {
	moduleName := moduleName(program, fn.Module)
	suffix := sanitizeName(fn.Name)
	if fn.IsScript {
		suffix = "script"
	}
	if suffix == "" {
		suffix = fmt.Sprintf("fn_%d", fn.ID)
	}
	duplicate := false
	for _, other := range program.Functions {
		if other.ID != fn.ID && other.Module == fn.Module && other.Name == fn.Name {
			duplicate = true
			break
		}
	}
	out := moduleName + "__" + suffix
	if duplicate {
		out = fmt.Sprintf("%s__%s_%d", moduleName, suffix, fn.ID)
	}
	if !fn.Private && fn.Receiver != air.NoType {
		out = upperFirst(out)
	}
	return out
}

type topLevelNameKind int

const (
	topLevelNameType topLevelNameKind = iota
	topLevelNameTrait
	topLevelNameFunction
	topLevelNameGlobal
)

func typeName(program *air.Program, typ air.TypeInfo) string {
	if name, ok := naturalTypeName(program, typ); ok {
		return name
	}
	base := typeNameBase(program, typ)
	if typeNameCollides(program, typ, base) {
		base = fmt.Sprintf("%s_%d", base, typ.ID)
	}
	if !typ.Private {
		base = upperFirst(base)
	}
	return base
}

func naturalTypeName(program *air.Program, typ air.TypeInfo) (string, bool) {
	if typ.Kind != air.TypeStruct && typ.Kind != air.TypeEnum && typ.Kind != air.TypeUnion {
		return "", false
	}
	if typ.Name == "" || strings.ContainsAny(typ.Name, "<>[]?:!") {
		return "", false
	}
	name := naturalGoIdentifier(typ.Name, !typ.Private)
	if name == "" || name == "_" || isReservedTopLevelName(name) {
		return "", false
	}
	if topLevelNaturalNameCollides(program, topLevelNameType, int(typ.ID), name) {
		return "", false
	}
	return name, true
}

func naturalTypeNameEligible(typ air.TypeInfo) bool {
	if typ.Kind != air.TypeStruct && typ.Kind != air.TypeEnum && typ.Kind != air.TypeUnion {
		return false
	}
	return typ.Name != "" && !strings.ContainsAny(typ.Name, "<>[]?:!")
}

func topLevelValueNameAlias(program *air.Program, selfKind topLevelNameKind, selfID int, base string) string {
	suffix := 0
	if isSpecialGoTopLevelName(base) || topLevelNaturalNameCollides(program, selfKind, selfID, base) {
		suffix = 1 + earlierAliasedValueCount(program, selfKind, selfID, base)
	}
	for {
		name := base
		if suffix > 0 {
			name = fmt.Sprintf("%s_%d", base, suffix)
		}
		if !isSpecialGoTopLevelName(name) && !topLevelNaturalNameCollides(program, selfKind, selfID, name) {
			return name
		}
		suffix++
	}
}

func earlierAliasedValueCount(program *air.Program, selfKind topLevelNameKind, selfID int, base string) int {
	if program == nil {
		return 0
	}
	count := 0
	for _, fn := range program.Functions {
		if selfKind == topLevelNameFunction && int(fn.ID) == selfID {
			continue
		}
		if naturalFunctionNameEligible(fn) && naturalGoIdentifier(fn.Name, !fn.Private) == base && topLevelValuePrecedes(topLevelNameFunction, int(fn.ID), selfKind, selfID) && (isSpecialGoTopLevelName(base) || topLevelNaturalNameCollides(program, topLevelNameFunction, int(fn.ID), base)) {
			count++
		}
	}
	for _, global := range program.Globals {
		if selfKind == topLevelNameGlobal && int(global.ID) == selfID {
			continue
		}
		if global.Name != "" && naturalGoIdentifier(global.Name, !global.Private) == base && topLevelValuePrecedes(topLevelNameGlobal, int(global.ID), selfKind, selfID) && (isSpecialGoTopLevelName(base) || topLevelNaturalNameCollides(program, topLevelNameGlobal, int(global.ID), base)) {
			count++
		}
	}
	return count
}

func topLevelValuePrecedes(leftKind topLevelNameKind, leftID int, rightKind topLevelNameKind, rightID int) bool {
	if leftKind != rightKind {
		return leftKind < rightKind
	}
	return leftID < rightID
}

func isReservedTopLevelName(name string) bool {
	return isSpecialGoTopLevelName(name)
}

func isSpecialGoTopLevelName(name string) bool {
	if name == "main" || name == "ardRunTest" || name == "ardTestOutcome" {
		return true
	}
	if isPredeclaredGoIdentifier(name) {
		return true
	}
	if isRuntimePreludeTopLevelName(name) {
		return true
	}
	_, ok := generatedImportAliasPath(name)
	return ok
}

var generatedImportPaths = map[string]string{
	"ardmath":  "math",
	"ardutf8":  "unicode/utf8",
	"bytes":    "bytes",
	"fmt":      "fmt",
	"json":     "encoding/json/v2",
	"jsontext": "encoding/json/jsontext",
	"slices":   "slices",
	"sort":     "sort",
	"strconv":  "strconv",
	"strings":  "strings",
}

func generatedImportAliasPath(alias string) (string, bool) {
	path, ok := generatedImportPaths[alias]
	return path, ok
}

func topLevelNaturalNameCollides(program *air.Program, selfKind topLevelNameKind, selfID int, name string) bool {
	if program == nil {
		return false
	}
	selfModule, selfHasModule := topLevelNameModule(program, selfKind, selfID)
	for _, typ := range program.Types {
		if selfKind == topLevelNameType && int(typ.ID) == selfID {
			continue
		}
		if !sameTopLevelPackage(program, selfModule, selfHasModule, topLevelNameType, int(typ.ID)) {
			continue
		}
		if naturalTypeNameEligible(typ) && naturalGoIdentifier(typ.Name, !typ.Private) == name {
			return true
		}
	}
	for _, trait := range program.Traits {
		if selfKind == topLevelNameTrait && int(trait.ID) == selfID {
			continue
		}
		if !sameTopLevelPackage(program, selfModule, selfHasModule, topLevelNameTrait, int(trait.ID)) {
			continue
		}
		if trait.Name != "" && naturalGoIdentifier(trait.Name, !trait.Private) == name {
			return true
		}
	}
	if selfKind == topLevelNameType || selfKind == topLevelNameTrait {
		return false
	}
	for _, fn := range program.Functions {
		if selfKind == topLevelNameFunction && int(fn.ID) == selfID {
			continue
		}
		if !sameTopLevelPackage(program, selfModule, selfHasModule, topLevelNameFunction, int(fn.ID)) {
			continue
		}
		if naturalFunctionNameEligible(fn) && naturalGoIdentifier(fn.Name, !fn.Private) == name {
			return true
		}
	}
	for _, global := range program.Globals {
		if selfKind == topLevelNameGlobal && int(global.ID) == selfID {
			continue
		}
		if !sameTopLevelPackage(program, selfModule, selfHasModule, topLevelNameGlobal, int(global.ID)) {
			continue
		}
		if global.Name != "" && naturalGoIdentifier(global.Name, !global.Private) == name {
			return true
		}
	}
	return false
}

func sameTopLevelPackage(program *air.Program, selfModule air.ModuleID, selfHasModule bool, otherKind topLevelNameKind, otherID int) bool {
	otherModule, otherHasModule := topLevelNameModule(program, otherKind, otherID)
	if selfHasModule && otherHasModule && selfModule != otherModule {
		return false
	}
	return true
}

func topLevelNameModule(program *air.Program, kind topLevelNameKind, id int) (air.ModuleID, bool) {
	if program == nil {
		return 0, false
	}
	switch kind {
	case topLevelNameType:
		if id > 0 && id <= len(program.Types) && int(program.Types[id-1].ID) == id {
			typ := program.Types[id-1]
			if typ.ModulePath != "" {
				return moduleIDForPath(program, typ.ModulePath)
			}
			return moduleIDForDeclaredType(program, typ.ID)
		}
		for _, typ := range program.Types {
			if int(typ.ID) != id {
				continue
			}
			if typ.ModulePath != "" {
				return moduleIDForPath(program, typ.ModulePath)
			}
			return moduleIDForDeclaredType(program, typ.ID)
		}
	case topLevelNameTrait:
		if id >= 0 && id < len(program.Traits) && int(program.Traits[id].ID) == id {
			if modulePath := program.Traits[id].ModulePath; modulePath != "" {
				return moduleIDForPath(program, modulePath)
			}
			return 0, false
		}
		for _, trait := range program.Traits {
			if int(trait.ID) == id && trait.ModulePath != "" {
				return moduleIDForPath(program, trait.ModulePath)
			}
		}
	case topLevelNameFunction:
		if id >= 0 && id < len(program.Functions) && int(program.Functions[id].ID) == id {
			return program.Functions[id].Module, true
		}
		for _, fn := range program.Functions {
			if int(fn.ID) == id {
				return fn.Module, true
			}
		}
	case topLevelNameGlobal:
		if id >= 0 && id < len(program.Globals) && int(program.Globals[id].ID) == id {
			return program.Globals[id].Module, true
		}
		for _, global := range program.Globals {
			if int(global.ID) == id {
				return global.Module, true
			}
		}
	}
	return 0, false
}

func moduleIDForDeclaredType(program *air.Program, typeID air.TypeID) (air.ModuleID, bool) {
	for _, module := range program.Modules {
		for _, declaredID := range module.Types {
			if declaredID == typeID {
				return module.ID, true
			}
		}
	}
	return 0, false
}

func moduleIDForPath(program *air.Program, modulePath string) (air.ModuleID, bool) {
	for _, module := range program.Modules {
		if module.Path == modulePath {
			return module.ID, true
		}
	}
	return 0, false
}

func typeNameBase(program *air.Program, typ air.TypeInfo) string {
	moduleName := sanitizeName(typ.ModulePath)
	if moduleName == "" {
		for _, module := range program.Modules {
			for _, typeID := range module.Types {
				if typeID == typ.ID {
					moduleName = sanitizeName(module.Path)
					break
				}
			}
			if moduleName != "" {
				break
			}
		}
	}
	name := sanitizeName(typ.Name)
	if moduleName == "" {
		moduleName = fmt.Sprintf("type_%d", typ.ID)
	}
	if name == "" {
		name = fmt.Sprintf("type_%d", typ.ID)
	}
	return moduleName + "__" + name
}

func typeNameCollides(program *air.Program, typ air.TypeInfo, base string) bool {
	for _, other := range program.Types {
		if other.ID != typ.ID && typeNameBase(program, other) == base {
			return true
		}
	}
	return false
}

func enumVariantName(program *air.Program, typ air.TypeInfo, variant air.VariantInfo) string {
	if name, ok := naturalEnumVariantName(program, typ, variant); ok {
		return name
	}
	return legacyEnumVariantName(program, typ, variant)
}

func naturalEnumVariantName(program *air.Program, typ air.TypeInfo, variant air.VariantInfo) (string, bool) {
	if typ.Kind != air.TypeEnum || variant.Name == "" || len(goIdentifierParts(variant.Name)) == 0 {
		return "", false
	}
	typePart, ok := naturalTypeName(program, typ)
	if !ok {
		return "", false
	}
	variantPart := naturalGoIdentifier(variant.Name, true)
	if variantPart == "" || variantPart == "_" {
		return "", false
	}
	base := typePart + variantPart
	return enumVariantNameAlias(program, typ, variant, base), true
}

func enumVariantNameAlias(program *air.Program, typ air.TypeInfo, variant air.VariantInfo, base string) string {
	candidate := base
	for i := 1; enumVariantNameCollides(program, typ, variant, candidate); i++ {
		candidate = fmt.Sprintf("%s_%d", base, i)
	}
	return candidate
}

func enumVariantNameCollides(program *air.Program, typ air.TypeInfo, variant air.VariantInfo, candidate string) bool {
	owner, hasOwner := topLevelNameModule(program, topLevelNameType, int(typ.ID))
	if isSpecialGoTopLevelName(candidate) || topLevelActualNameCollides(program, owner, hasOwner, candidate) {
		return true
	}
	if program == nil {
		return false
	}
	for _, other := range program.Types {
		if other.Kind != air.TypeEnum || !sameTopLevelPackage(program, owner, hasOwner, topLevelNameType, int(other.ID)) {
			continue
		}
		for _, otherVariant := range other.Variants {
			if !enumVariantPrecedes(other, otherVariant, typ, variant) {
				continue
			}
			if enumVariantName(program, other, otherVariant) == candidate {
				return true
			}
		}
	}
	return false
}

func topLevelActualNameCollides(program *air.Program, owner air.ModuleID, hasOwner bool, candidate string) bool {
	if program == nil {
		return false
	}
	for _, typ := range program.Types {
		if !sameTopLevelPackage(program, owner, hasOwner, topLevelNameType, int(typ.ID)) {
			continue
		}
		if typeName(program, typ) == candidate {
			return true
		}
	}
	traitLowerer := &lowerer{program: program}
	for _, trait := range program.Traits {
		if !sameTopLevelPackage(program, owner, hasOwner, topLevelNameTrait, int(trait.ID)) {
			continue
		}
		if traitLowerer.traitInterfaceTypeName(trait) == candidate {
			return true
		}
	}
	for _, fn := range program.Functions {
		if !sameTopLevelPackage(program, owner, hasOwner, topLevelNameFunction, int(fn.ID)) {
			continue
		}
		if functionName(program, fn) == candidate {
			return true
		}
	}
	for _, global := range program.Globals {
		if !sameTopLevelPackage(program, owner, hasOwner, topLevelNameGlobal, int(global.ID)) {
			continue
		}
		if globalName(program, global) == candidate {
			return true
		}
	}
	return false
}

func enumVariantPrecedes(leftType air.TypeInfo, left air.VariantInfo, rightType air.TypeInfo, right air.VariantInfo) bool {
	if leftType.ID != rightType.ID {
		return leftType.ID < rightType.ID
	}
	return enumVariantIndex(leftType, left) < enumVariantIndex(rightType, right)
}

func enumVariantIndex(typ air.TypeInfo, variant air.VariantInfo) int {
	for i, candidate := range typ.Variants {
		if candidate.Name == variant.Name && candidate.Discriminant == variant.Discriminant {
			return i
		}
	}
	return len(typ.Variants)
}

func legacyEnumVariantName(program *air.Program, typ air.TypeInfo, variant air.VariantInfo) string {
	name := sanitizeName(variant.Name)
	if name == "" {
		name = fmt.Sprintf("variant_%d", variant.Discriminant)
	}
	return typeName(program, typ) + "__" + name
}

func unionTagFieldName(typ air.TypeInfo) string {
	base := "ArdTag"
	candidate := base
	for i := 1; unionTagFieldNameCollides(typ, candidate); i++ {
		candidate = fmt.Sprintf("%s%d", base, i)
	}
	return candidate
}

func unionTagFieldNameCollides(typ air.TypeInfo, candidate string) bool {
	for _, member := range typ.Members {
		if unionMemberFieldName(typ, member) == candidate {
			return true
		}
	}
	return false
}

func unionMemberFieldName(typ air.TypeInfo, member air.UnionMember) string {
	base := unionMemberFieldNameBase(member)
	candidate := base
	for i := 1; unionMemberFieldNameCollidesEarlier(typ, member, candidate); i++ {
		candidate = fmt.Sprintf("%s%d", base, i)
	}
	return candidate
}

func unionMemberFieldNameBase(member air.UnionMember) string {
	if len(goIdentifierParts(member.Name)) == 0 {
		return fmt.Sprintf("Member%d", member.Tag)
	}
	return naturalGoIdentifier(member.Name, true)
}

func unionMemberFieldNameCollidesEarlier(typ air.TypeInfo, member air.UnionMember, candidate string) bool {
	self := unionMemberIndex(typ, member)
	for i, other := range typ.Members {
		if i >= self {
			break
		}
		if unionMemberFieldName(typ, other) == candidate {
			return true
		}
	}
	return false
}

func unionMemberIndex(typ air.TypeInfo, member air.UnionMember) int {
	for i, candidate := range typ.Members {
		if candidate.Type == member.Type && candidate.Tag == member.Tag && candidate.Name == member.Name {
			return i
		}
	}
	return len(typ.Members)
}

// rawLocalName returns the Ard-derived source name for a local or capture.
func rawLocalName(fn air.Function, local air.LocalID) string {
	for _, capture := range fn.Captures {
		if capture.Local == local {
			return capture.Name
		}
	}
	if int(local) >= 0 && int(local) < len(fn.Locals) {
		return fn.Locals[local].Name
	}
	return ""
}

// localName renders a local or parameter as its Ard name, kept bare when that is
// unambiguous within the function. A numeric suffix (the AIR local id) is added
// only when the bare name is already claimed by an earlier local, or collides
// with a Go keyword, a predeclared identifier, a generated top-level name, or an
// generated import alias.
func (l *lowerer) localName(fn air.Function, local air.LocalID) string {
	if name, ok := l.allocateLocalNames(fn)[local]; ok {
		return name
	}
	return fmt.Sprintf("local_%d", local)
}

// allocateLocalNames assigns every local and capture a Go identifier for the
// function, keeping each as close to its Ard name as possible. It walks the
// block tree tracking the names currently in scope; a local claims its bare Ard
// name unless that name is already visible in an enclosing scope (a shadow), is
// already taken in the same scope, or is a Go keyword / predeclared identifier /
// generated top-level name. Names in sibling scopes are independent, so two
// disjoint `x`s both stay `x` and a suffix appears only where it is genuinely
// needed to avoid a collision.
func (l *lowerer) allocateLocalNames(fn air.Function) map[air.LocalID]string {
	if l.localNameCache == nil {
		l.localNameCache = map[air.FunctionID]map[air.LocalID]string{}
	}
	if cached, ok := l.localNameCache[fn.ID]; ok {
		return cached
	}
	n := &localNamer{l: l, fn: fn, names: map[air.LocalID]string{}}
	n.push()
	// Params and captures live in the outermost scope and cannot shadow anything,
	// so they take their bare name unless it clashes with a sibling or a reserved
	// name (scopeRefs is irrelevant: there is no enclosing scope to shadow).
	for _, capture := range fn.Captures {
		n.assign(capture.Local, nil)
	}
	for i := 0; i < len(fn.Signature.Params); i++ {
		n.assign(air.LocalID(i), nil)
	}
	n.walkBlock(fn.Body)
	n.pop()
	// Insurance for any local the walk did not reach: name it uniquely against
	// every name already assigned, so a missed binder can never collide.
	n.push()
	for id, name := range n.names {
		n.frames[len(n.frames)-1][name] = id
	}
	for _, loc := range fn.Locals {
		n.assign(loc.ID, nil)
	}
	n.pop()
	l.localNameCache[fn.ID] = n.names
	return n.names
}

type localNamer struct {
	l     *lowerer
	fn    air.Function
	names map[air.LocalID]string
	// frames is the stack of in-scope names; each maps a claimed Go name to the
	// local that holds it in that scope.
	frames []map[string]air.LocalID
}

func (n *localNamer) push() { n.frames = append(n.frames, map[string]air.LocalID{}) }

func (n *localNamer) pop() { n.frames = n.frames[:len(n.frames)-1] }

func (n *localNamer) reservedName(name string) bool {
	return token.IsKeyword(name) || isReservedLocalName(name) || n.l.topLevelReservedName(name)
}

// mustSuffix reports whether name cannot be used bare for a local whose scope
// references the locals in scopeRefs. A reserved name or a same-scope clash is
// always rejected. Shadowing a local from an enclosing scope is rejected only
// when that enclosing local is actually referenced within the new local's
// scope; an unused outer binding can be harmlessly shadowed. A nil scopeRefs is
// conservative and rejects any shadow.
func (n *localNamer) mustSuffix(name string, scopeRefs []int) bool {
	if n.reservedName(name) {
		return true
	}
	if _, ok := n.frames[len(n.frames)-1][name]; ok {
		return true
	}
	for i := len(n.frames) - 2; i >= 0; i-- {
		if outer, ok := n.frames[i][name]; ok {
			// Only the innermost enclosing holder matters: any further-out holder
			// of this name is already shadowed by it within this scope.
			return scopeRefs == nil || int(outer) < len(scopeRefs) && scopeRefs[outer] > 0
		}
	}
	return false
}

func (n *localNamer) assign(id air.LocalID, scopeRefs []int) {
	if _, ok := n.names[id]; ok {
		return
	}
	base := sanitizeName(rawLocalName(n.fn, id))
	if base == "" {
		base = "local"
	}
	cand := base
	if n.mustSuffix(cand, scopeRefs) {
		cand = fmt.Sprintf("%s_%d", base, id)
		for k := 1; n.mustSuffix(cand, scopeRefs); k++ {
			cand = fmt.Sprintf("%s_%d_%d", base, id, k)
		}
	}
	n.names[id] = cand
	n.frames[len(n.frames)-1][cand] = id
}

func (n *localNamer) walkBlock(b air.Block) {
	if len(b.Stmts) == 0 {
		if b.Result != nil {
			n.walkExpr(*b.Result)
		}
		return
	}
	n.push()
	n.walkStmts(b)
	n.pop()
}

// walkStmts assigns names within an already-pushed scope. Each let binds after
// its initializer, and its scope is the remainder of the block, so a let only
// shadows an enclosing local that the remaining statements actually reference.
func (n *localNamer) walkStmts(b air.Block) {
	if len(b.Stmts) == 0 {
		if b.Result != nil {
			n.walkExpr(*b.Result)
		}
		return
	}
	remainingRefs := make([]int, len(n.fn.Locals))
	for i := range b.Stmts {
		collectStmtRefCounts(b.Stmts[i], remainingRefs, 1)
	}
	if b.Result != nil {
		collectExprRefCounts(*b.Result, remainingRefs, 1)
	}
	for i := range b.Stmts {
		collectStmtRefCounts(b.Stmts[i], remainingRefs, -1)
		n.walkStmt(b.Stmts[i], remainingRefs)
	}
	if b.Result != nil {
		n.walkExpr(*b.Result)
	}
}

// walkBindingBlock walks a block that binds a pattern local (Some/Ok/Err/Catch/
// union case) scoped to that block. The local is bound only when the owning
// construct actually has one, since the *Local fields default to id 0.
func (n *localNamer) walkBindingBlock(bind bool, local air.LocalID, b air.Block) {
	if !bind && len(b.Stmts) == 0 {
		if b.Result != nil {
			n.walkExpr(*b.Result)
		}
		return
	}
	n.push()
	if bind {
		// A pattern-bound local is scoped to the whole block, so it shadows an
		// enclosing local only if that local is referenced anywhere in the block.
		scope := make([]int, len(n.fn.Locals))
		collectBlockRefCounts(b, scope, 1)
		n.assign(local, scope)
	}
	n.walkStmts(b)
	n.pop()
}

func (n *localNamer) walkStmt(s air.Stmt, scopeRefs []int) {
	switch s.Kind {
	case air.StmtLet:
		if s.Value != nil {
			n.walkExpr(*s.Value) // initializer is evaluated before the local binds
		}
		n.assign(s.Local, scopeRefs)
	case air.StmtWhile:
		if s.Condition != nil {
			n.walkExpr(*s.Condition)
		}
		n.walkBlock(s.Body)
	case air.StmtForMap:
		if s.Target != nil {
			n.walkExpr(*s.Target)
		}
		n.push()
		scope := make([]int, len(n.fn.Locals))
		collectBlockRefCounts(s.Body, scope, 1)
		n.assign(s.Local, scope)
		n.assign(s.ValueLocal, scope)
		n.walkStmts(s.Body)
		n.pop()
	default:
		if s.Value != nil {
			n.walkExpr(*s.Value)
		}
		if s.Target != nil {
			n.walkExpr(*s.Target)
		}
		if s.Expr != nil {
			n.walkExpr(*s.Expr)
		}
		if s.Condition != nil {
			n.walkExpr(*s.Condition)
		}
	}
}

func (n *localNamer) walkExpr(e air.Expr) {
	for i := range e.Args {
		n.walkExpr(e.Args[i])
	}
	for i := range e.Entries {
		n.walkExpr(e.Entries[i].Key)
		n.walkExpr(e.Entries[i].Value)
	}
	for i := range e.Fields {
		n.walkExpr(e.Fields[i].Value)
	}
	if e.Target != nil {
		n.walkExpr(*e.Target)
	}
	if e.Left != nil {
		n.walkExpr(*e.Left)
	}
	if e.Right != nil {
		n.walkExpr(*e.Right)
	}
	if e.Condition != nil {
		n.walkExpr(*e.Condition)
	}
	n.walkBlock(e.Body)
	n.walkBlock(e.Then)
	n.walkBlock(e.Else)
	n.walkBlock(e.None)
	n.walkBlock(e.CatchAll)
	n.walkBindingBlock(e.Kind == air.ExprMatchMaybe, e.SomeLocal, e.Some)
	n.walkBindingBlock(e.Kind == air.ExprMatchResult, e.OkLocal, e.Ok)
	n.walkBindingBlock(e.Kind == air.ExprMatchResult, e.ErrLocal, e.Err)
	n.walkBindingBlock(e.HasCatch, e.CatchLocal, e.Catch)
	for i := range e.EnumCases {
		n.walkBlock(e.EnumCases[i].Body)
	}
	for i := range e.IntCases {
		n.walkBlock(e.IntCases[i].Body)
	}
	for i := range e.StrCases {
		n.walkBlock(e.StrCases[i].Body)
	}
	for i := range e.RangeCases {
		n.walkBlock(e.RangeCases[i].Body)
	}
	for i := range e.UnionCases {
		n.walkBindingBlock(true, e.UnionCases[i].Local, e.UnionCases[i].Body)
	}
	for i := range e.SelectCases {
		arm := e.SelectCases[i]
		if arm.Channel != nil {
			n.walkExpr(*arm.Channel)
		}
		if arm.Value != nil {
			n.walkExpr(*arm.Value)
		}
		n.walkBindingBlock(arm.HasBind, arm.BindLocal, arm.Body)
	}
}

// collectBlockRefCounts/collectStmtRefCounts/collectExprRefCounts count every
// local that is referenced (not bound) within a subtree. The local namer adds
// all remaining statement references once, then subtracts each statement as it
// advances, avoiding a copied suffix map for every statement. The reference
// sites are loads (ExprLoadLocal), assignment targets (StmtAssign), and closure
// captures (ExprMakeClosure.CaptureLocals); field-sets reference their local
// through an ExprLoadLocal target. This must stay complete: a missed reference
// could let a shadowing local keep a bare name that then captures the reference.
func collectBlockRefCounts(b air.Block, into []int, delta int) {
	for i := range b.Stmts {
		collectStmtRefCounts(b.Stmts[i], into, delta)
	}
	if b.Result != nil {
		collectExprRefCounts(*b.Result, into, delta)
	}
}

func collectStmtRefCounts(s air.Stmt, into []int, delta int) {
	if s.Kind == air.StmtAssign {
		adjustLocalRefCount(into, s.Local, delta)
	}
	if s.Value != nil {
		collectExprRefCounts(*s.Value, into, delta)
	}
	if s.Expr != nil {
		collectExprRefCounts(*s.Expr, into, delta)
	}
	if s.Target != nil {
		collectExprRefCounts(*s.Target, into, delta)
	}
	if s.Condition != nil {
		collectExprRefCounts(*s.Condition, into, delta)
	}
	collectBlockRefCounts(s.Body, into, delta)
}

func collectExprRefCounts(e air.Expr, into []int, delta int) {
	if e.Kind == air.ExprLoadLocal {
		adjustLocalRefCount(into, e.Local, delta)
	}
	for _, c := range e.CaptureLocals {
		adjustLocalRefCount(into, c, delta)
	}
	for i := range e.Args {
		collectExprRefCounts(e.Args[i], into, delta)
	}
	for i := range e.Entries {
		collectExprRefCounts(e.Entries[i].Key, into, delta)
		collectExprRefCounts(e.Entries[i].Value, into, delta)
	}
	for i := range e.Fields {
		collectExprRefCounts(e.Fields[i].Value, into, delta)
	}
	if e.Target != nil {
		collectExprRefCounts(*e.Target, into, delta)
	}
	if e.Left != nil {
		collectExprRefCounts(*e.Left, into, delta)
	}
	if e.Right != nil {
		collectExprRefCounts(*e.Right, into, delta)
	}
	if e.Condition != nil {
		collectExprRefCounts(*e.Condition, into, delta)
	}
	collectBlockRefCounts(e.Body, into, delta)
	collectBlockRefCounts(e.Then, into, delta)
	collectBlockRefCounts(e.Else, into, delta)
	collectBlockRefCounts(e.None, into, delta)
	collectBlockRefCounts(e.CatchAll, into, delta)
	collectBlockRefCounts(e.Some, into, delta)
	collectBlockRefCounts(e.Ok, into, delta)
	collectBlockRefCounts(e.Err, into, delta)
	collectBlockRefCounts(e.Catch, into, delta)
	for i := range e.EnumCases {
		collectBlockRefCounts(e.EnumCases[i].Body, into, delta)
	}
	for i := range e.IntCases {
		collectBlockRefCounts(e.IntCases[i].Body, into, delta)
	}
	for i := range e.StrCases {
		collectBlockRefCounts(e.StrCases[i].Body, into, delta)
	}
	for i := range e.RangeCases {
		collectBlockRefCounts(e.RangeCases[i].Body, into, delta)
	}
	for i := range e.UnionCases {
		collectBlockRefCounts(e.UnionCases[i].Body, into, delta)
	}
	for i := range e.SelectCases {
		arm := e.SelectCases[i]
		if arm.Channel != nil {
			collectExprRefCounts(*arm.Channel, into, delta)
		}
		if arm.Value != nil {
			collectExprRefCounts(*arm.Value, into, delta)
		}
		collectBlockRefCounts(arm.Body, into, delta)
	}
}

func adjustLocalRefCount(counts []int, id air.LocalID, delta int) {
	if id >= 0 && int(id) < len(counts) {
		counts[id] += delta
	}
}

func (l *lowerer) topLevelReservedName(name string) bool {
	if l.topLevelReserved == nil {
		l.topLevelReserved = collectTopLevelReservedNames(l.program)
	}
	return l.topLevelReserved[name]
}

func isReservedLocalName(name string) bool {
	if name == "main" {
		return true
	}
	if isPredeclaredGoIdentifier(name) {
		return true
	}
	return isRuntimePreludeTopLevelName(name)
}

func sanitizeName(raw string) string {
	if raw == "" {
		return ""
	}
	if asciiSanitizedName(raw) {
		return raw
	}
	var out []rune
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, r)
			continue
		}
		out = append(out, '_')
	}
	name := strings.Trim(string(out), "_")
	if name == "" {
		return ""
	}
	runes := []rune(name)
	if len(runes) > 0 && unicode.IsDigit(runes[0]) {
		return "_" + name
	}
	return name
}

func goPackageNameFromModulePath(modulePath string) string {
	base := strings.TrimSuffix(filepath.Base(modulePath), filepath.Ext(modulePath))
	return sanitizeGoPackageIdentifier(base)
}

func sanitizeGoPackageIdentifier(raw string) string {
	name := sanitizeGoIdentifier(raw)
	if name == "" || name == "_" {
		return "module"
	}
	if token.Lookup(name) != token.IDENT || name == "main" {
		name += "_"
	}
	return name
}

func naturalGoIdentifier(raw string, exported bool) string {
	return goname.NaturalIdentifier(raw, exported)
}

func sanitizeGoIdentifier(raw string) string {
	if raw == "" {
		return ""
	}
	if asciiGoIdentifier(raw) {
		return raw
	}
	var out []rune
	lastUnderscore := false
	for _, r := range raw {
		valid := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
		if !valid {
			r = '_'
		}
		if r == '_' {
			if lastUnderscore {
				continue
			}
			lastUnderscore = true
		} else {
			lastUnderscore = false
		}
		out = append(out, r)
	}
	name := string(out)
	if name == "" {
		return ""
	}
	runes := []rune(name)
	if unicode.IsDigit(runes[0]) {
		name = "_" + name
	}
	return name
}

func asciiSanitizedName(raw string) bool {
	if raw[0] == '_' || raw[len(raw)-1] == '_' || raw[0] >= '0' && raw[0] <= '9' {
		return false
	}
	for i := range len(raw) {
		char := raw[i]
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func asciiGoIdentifier(raw string) bool {
	if raw[0] >= '0' && raw[0] <= '9' {
		return false
	}
	lastUnderscore := false
	for i := range len(raw) {
		char := raw[i]
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			lastUnderscore = false
			continue
		}
		if char == '_' && !lastUnderscore {
			lastUnderscore = true
			continue
		}
		return false
	}
	return true
}

func goIdentifierParts(raw string) []string {
	sanitized := sanitizeGoIdentifier(raw)
	if sanitized == "" {
		return nil
	}
	chunks := strings.Split(sanitized, "_")
	parts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk != "" {
			parts = append(parts, chunk)
		}
	}
	return parts
}

func upperFirst(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}
