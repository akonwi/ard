package gotarget

import (
	"fmt"
	"go/token"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/akonwi/ard/air"
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
	return goPackageNameFromModulePath(program.Modules[module].Path)
}

func modulePackageDir(program *air.Program, module air.ModuleID) string {
	if program == nil || module < 0 || int(module) >= len(program.Modules) {
		return "module"
	}
	pathNoExt := strings.TrimSuffix(program.Modules[module].Path, filepath.Ext(program.Modules[module].Path))
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
	dir := modulePackageDir(program, module)
	if dir == "" || dir == "." {
		return "generated"
	}
	return path.Join("generated", filepath.ToSlash(dir))
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
	return moduleName + "__global_" + name
}

func functionName(program *air.Program, fn air.Function) string {
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
	if duplicate {
		return fmt.Sprintf("%s__%s_%d", moduleName, suffix, fn.ID)
	}
	return moduleName + "__" + suffix
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
		return fmt.Sprintf("%s_%d", base, typ.ID)
	}
	return base
}

func naturalTypeName(program *air.Program, typ air.TypeInfo) (string, bool) {
	if typ.Kind != air.TypeStruct && typ.Kind != air.TypeEnum && typ.Kind != air.TypeUnion {
		return "", false
	}
	if typ.Name == "" || strings.ContainsAny(typ.Name, "<>[]?:!") || typ.ExternBinding != "" {
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
	return typ.Name != "" && !strings.ContainsAny(typ.Name, "<>[]?:!") && typ.ExternBinding == ""
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
	for _, reserved := range predeclaredGoIdentifiers() {
		if name == reserved {
			return true
		}
	}
	for _, reserved := range runtimePreludeTopLevelNames() {
		if name == reserved {
			return true
		}
	}
	for _, reserved := range generatedImportAliases() {
		if name == reserved {
			return true
		}
	}
	return false
}

func generatedImportAliases() []string {
	aliases := make([]string, 0, len(generatedImportAliasPaths()))
	for alias := range generatedImportAliasPaths() {
		aliases = append(aliases, alias)
	}
	return aliases
}

func generatedImportAliasPath(alias string) (string, bool) {
	path, ok := generatedImportAliasPaths()[alias]
	return path, ok
}

func generatedImportAliasPaths() map[string]string {
	return map[string]string{
		"ardmath":    "math",
		"ardruntime": "github.com/akonwi/ard/runtime",
		"ardutf8":    "unicode/utf8",
		"bytes":      "bytes",
		"fmt":        "fmt",
		"json":       "encoding/json/v2",
		"jsontext":   "encoding/json/jsontext",
		"slices":     "slices",
		"sort":       "sort",
		"strconv":    "strconv",
		"stdlibffi":  "github.com/akonwi/ard/std_lib/ffi",
		"strings":    "strings",
	}
}

func topLevelNaturalNameCollides(program *air.Program, selfKind topLevelNameKind, selfID int, name string) bool {
	if program == nil {
		return false
	}
	for _, typ := range program.Types {
		if selfKind == topLevelNameType && int(typ.ID) == selfID {
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
		if naturalFunctionNameEligible(fn) && naturalGoIdentifier(fn.Name, !fn.Private) == name {
			return true
		}
	}
	for _, global := range program.Globals {
		if selfKind == topLevelNameGlobal && int(global.ID) == selfID {
			continue
		}
		if global.Name != "" && naturalGoIdentifier(global.Name, !global.Private) == name {
			return true
		}
	}
	return false
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
	if typ.Kind != air.TypeEnum || variant.Name == "" || len(goIdentifierParts(variant.Name)) == 0 || typ.ExternBinding != "" {
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
	if isSpecialGoTopLevelName(candidate) || topLevelActualNameCollides(program, candidate) {
		return true
	}
	if program == nil {
		return false
	}
	for _, other := range program.Types {
		if other.Kind != air.TypeEnum {
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

func topLevelActualNameCollides(program *air.Program, candidate string) bool {
	if program == nil {
		return false
	}
	for _, typ := range program.Types {
		if typeName(program, typ) == candidate {
			return true
		}
	}
	traitLowerer := &lowerer{program: program}
	for _, trait := range program.Traits {
		if traitLowerer.traitInterfaceTypeName(trait) == candidate {
			return true
		}
	}
	for _, fn := range program.Functions {
		if functionName(program, fn) == candidate {
			return true
		}
	}
	for _, global := range program.Globals {
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

func localName(fn air.Function, local air.LocalID) string {
	for _, capture := range fn.Captures {
		if capture.Local == local {
			name := sanitizeName(capture.Name)
			if name != "" {
				return safeLocalName(name, local)
			}
		}
	}
	if int(local) >= 0 && int(local) < len(fn.Locals) {
		name := sanitizeName(fn.Locals[local].Name)
		if name != "" {
			if int(local) < len(fn.Signature.Params) {
				return paramLocalName(fn, local, name)
			}
			return fmt.Sprintf("%s_%d", name, local)
		}
	}
	return fmt.Sprintf("local_%d", local)
}

func paramLocalName(fn air.Function, local air.LocalID, name string) string {
	base := fmt.Sprintf("%s_%d", name, local)
	candidate := base
	for i := 1; localNameCollidesWithCapture(fn, local, candidate); i++ {
		candidate = fmt.Sprintf("%s_%d", base, i)
	}
	return candidate
}

func localNameCollidesWithCapture(fn air.Function, local air.LocalID, candidate string) bool {
	for _, capture := range fn.Captures {
		if capture.Local == local {
			continue
		}
		name := sanitizeName(capture.Name)
		if name != "" && safeLocalName(name, capture.Local) == candidate {
			return true
		}
	}
	return false
}

func safeLocalName(name string, local air.LocalID) string {
	if isReservedLocalName(name) {
		return fmt.Sprintf("%s_%d", name, local)
	}
	return name
}

func isReservedLocalName(name string) bool {
	if name == "main" {
		return true
	}
	for _, reserved := range predeclaredGoIdentifiers() {
		if name == reserved {
			return true
		}
	}
	for _, reserved := range runtimePreludeTopLevelNames() {
		if name == reserved {
			return true
		}
	}
	return false
}

func sanitizeName(raw string) string {
	if raw == "" {
		return ""
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
	if token.Lookup(name) != token.IDENT {
		name += "_"
	}
	return name
}

func naturalGoIdentifier(raw string, exported bool) string {
	parts := goIdentifierParts(raw)
	if len(parts) == 0 {
		if exported {
			return "Exported"
		}
		return "name"
	}
	var b strings.Builder
	for i, part := range parts {
		if i == 0 && !exported {
			b.WriteString(lowerFirst(part))
			continue
		}
		b.WriteString(upperFirst(part))
	}
	name := b.String()
	if name == "" {
		if exported {
			return "Exported"
		}
		return "name"
	}
	if !exported && token.Lookup(name) != token.IDENT {
		name += "_"
	}
	return name
}

func sanitizeGoIdentifier(raw string) string {
	if raw == "" {
		return ""
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
