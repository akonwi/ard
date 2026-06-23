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
	moduleName := moduleName(program, global.Module)
	name := sanitizeName(global.Name)
	if name == "" {
		name = fmt.Sprintf("global_%d", global.ID)
	}
	return moduleName + "__global_" + name
}

func functionName(program *air.Program, fn air.Function) string {
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
	if typ.Kind != air.TypeStruct && typ.Kind != air.TypeEnum {
		return "", false
	}
	if typ.Name == "" || strings.ContainsAny(typ.Name, "<>[]?:!") || strings.HasPrefix(typ.ModulePath, "ard/") || typ.ExternBinding != "" {
		return "", false
	}
	name := naturalGoIdentifier(typ.Name, !typ.Private)
	if name == "" || name == "_" {
		return "", false
	}
	for _, other := range program.Types {
		if other.ID == typ.ID || !naturalTypeNameEligible(other) {
			continue
		}
		if naturalGoIdentifier(other.Name, !other.Private) == name {
			return "", false
		}
	}
	return name, true
}

func naturalTypeNameEligible(typ air.TypeInfo) bool {
	if typ.Kind != air.TypeStruct && typ.Kind != air.TypeEnum {
		return false
	}
	return typ.Name != "" && !strings.ContainsAny(typ.Name, "<>[]?:!") && !strings.HasPrefix(typ.ModulePath, "ard/") && typ.ExternBinding == ""
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
	return typeName(program, typ) + "__" + sanitizeName(variant.Name)
}

func unionMemberFieldName(member air.UnionMember) string {
	name := sanitizeName(member.Name)
	if name == "" {
		return fmt.Sprintf("member_%d", member.Tag)
	}
	return name
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
