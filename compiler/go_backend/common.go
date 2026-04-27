package go_backend

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/akonwi/ard/checker"
)

const (
	generatedGoVersion = "1.26.0"
	ardModulePath      = "github.com/akonwi/ard"
	helperImportPath   = ardModulePath + "/go"
	helperImportAlias  = "ardgo"
	osArgsEnvVar       = "ARDGO_OS_ARGS_JSON"
)

func CompileEntrypoint(module checker.Module) ([]byte, error) {
	return compileModuleSource(module, "main", true, "")
}

func compilePackageSource(module checker.Module, projectName string) ([]byte, error) {
	return compileModuleSource(module, packageNameForModulePath(module.Path()), false, projectName)
}

func compileModuleSource(module checker.Module, packageName string, entrypoint bool, projectName string) ([]byte, error) {
	return compileModuleSourceViaBackendIR(module, packageName, entrypoint, projectName)
}

func sortedImportPaths(imports map[string]string) []string {
	paths := make([]string, 0, len(imports))
	for path := range imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func packageNameForModulePath(modulePath string) string {
	base := filepath.Base(strings.TrimSuffix(modulePath, ".ard"))
	name := goName(base, false)
	if isGoPredeclaredIdentifier(name) {
		return name + "_"
	}
	return name
}

func stdlibGeneratedRelativePath(modulePath string) string {
	return path.Join("__ard_stdlib", strings.TrimPrefix(modulePath, "ard/"))
}

func moduleImportPath(projectName, modulePath string) string {
	if strings.HasPrefix(modulePath, "ard/") {
		relative := stdlibGeneratedRelativePath(modulePath)
		if projectName == "" {
			return relative
		}
		return path.Join(projectName, relative)
	}
	return modulePath
}

func generatedPathForModule(generatedDir, projectName, modulePath string) (string, error) {
	var relative string
	if strings.HasPrefix(modulePath, "ard/") {
		relative = stdlibGeneratedRelativePath(modulePath)
	} else {
		prefix := projectName + "/"
		if !strings.HasPrefix(modulePath, prefix) {
			return "", fmt.Errorf("module path %q does not match project %q", modulePath, projectName)
		}
		relative = strings.TrimPrefix(modulePath, prefix)
	}
	dir := filepath.Join(generatedDir, filepath.FromSlash(relative))
	base := filepath.Base(relative)
	return filepath.Join(dir, base+".go"), nil
}

func uniquePackageName(base string, used map[string]struct{}) string {
	name := base
	if _, ok := used[name]; !ok {
		used[name] = struct{}{}
		return name
	}
	name = base + "Fn"
	if _, ok := used[name]; !ok {
		used[name] = struct{}{}
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%sFn%d", base, i)
		if _, ok := used[candidate]; !ok {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

func goName(name string, exported bool) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == ' ' || r == ':'
	})
	if len(parts) == 0 {
		return "value"
	}
	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
	}
	for i := range parts {
		if i == 0 && !exported {
			continue
		}
		parts[i] = upperFirst(parts[i])
	}
	result := strings.Join(parts, "")
	if !exported {
		result = lowerFirst(result)
	}
	if result == "" {
		result = "value"
	}
	if isGoKeyword(result) {
		return result + "_"
	}
	return result
}

func upperFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func isGoKeyword(value string) bool {
	switch value {
	case "break", "default", "func", "interface", "select",
		"case", "defer", "go", "map", "struct",
		"chan", "else", "goto", "package", "switch",
		"const", "fallthrough", "if", "range", "type",
		"continue", "for", "import", "return", "var":
		return true
	default:
		return false
	}
}

func isGoPredeclaredIdentifier(value string) bool {
	switch value {
	case "any", "bool", "byte", "comparable", "complex64", "complex128",
		"error", "false", "float32", "float64", "iota", "int", "int8",
		"int16", "int32", "int64", "nil", "rune", "string", "true",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	default:
		return false
	}
}
