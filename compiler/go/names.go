package gotarget

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/akonwi/ard/air"
)

func moduleFileName(module air.Module) string {
	name := strings.TrimSuffix(module.Path, filepath.Ext(module.Path))
	name = sanitizeName(name)
	if name == "" {
		name = fmt.Sprintf("module_%d", module.ID)
	}
	return name + ".go"
}

func functionName(program *air.Program, fn air.Function) string {
	moduleName := sanitizeName(program.Modules[fn.Module].Path)
	suffix := sanitizeName(fn.Name)
	if fn.IsScript {
		suffix = "script"
	}
	if moduleName == "" {
		moduleName = fmt.Sprintf("module_%d", fn.Module)
	}
	if suffix == "" {
		suffix = fmt.Sprintf("fn_%d", fn.ID)
	}
	return moduleName + "__" + suffix
}

func typeName(program *air.Program, typ air.TypeInfo) string {
	moduleName := ""
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
	name := sanitizeName(typ.Name)
	if moduleName == "" {
		moduleName = "type"
	}
	if name == "" {
		name = fmt.Sprintf("type_%d", typ.ID)
	}
	return moduleName + "__" + name
}

func enumVariantName(program *air.Program, typ air.TypeInfo, variant air.VariantInfo) string {
	return typeName(program, typ) + "__" + sanitizeName(variant.Name)
}

func localName(fn air.Function, local air.LocalID) string {
	if int(local) >= 0 && int(local) < len(fn.Locals) {
		name := sanitizeName(fn.Locals[local].Name)
		if name != "" {
			if int(local) < len(fn.Signature.Params) {
				return name
			}
			return fmt.Sprintf("%s_%d", name, local)
		}
	}
	return fmt.Sprintf("local_%d", local)
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
