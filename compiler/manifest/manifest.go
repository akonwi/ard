package manifest

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"

	"github.com/akonwi/ard/parse"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/mod/module"
)

type BuildValueType string

const (
	BuildValueStr  BuildValueType = "Str"
	BuildValueInt  BuildValueType = "Int"
	BuildValueBool BuildValueType = "Bool"
)

type Document struct {
	Name         string
	Ard          string
	Target       string
	Go           GoConfig
	Dependencies map[string]Dependency
	Build        BuildConfig
}

type GoConfig struct {
	BuildTags []string
	Imports   map[string]string
}

type Dependency struct {
	Path   string
	Git    string
	Tag    string
	Commit string
}

type BuildConfig struct {
	Values map[string]BuildValue
}

type BuildValue struct {
	Type    BuildValueType
	Default any
	Release bool
}

func ParseFile(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Document, error) {
	var root map[string]any
	decoder := toml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("invalid TOML: %w", err)
	}
	doc := &Document{Dependencies: map[string]Dependency{}, Build: BuildConfig{Values: map[string]BuildValue{}}}
	var err error
	if doc.Name, err = optionalString(root, "name", "name"); err != nil {
		return nil, err
	}
	if doc.Ard, err = optionalString(root, "ard", "ard"); err != nil {
		return nil, err
	}
	if doc.Target, err = optionalString(root, "target", "target"); err != nil {
		return nil, err
	}
	if raw, ok := root["go"]; ok {
		if doc.Go, err = parseGo(raw); err != nil {
			return nil, err
		}
		if _, exists := doc.Go.Imports[doc.Name]; doc.Name != "" && exists {
			return nil, fmt.Errorf("Go import alias %q conflicts with the project name", doc.Name)
		}
	}
	if raw, ok := root["dependencies"]; ok {
		if doc.Dependencies, err = parseDependencies(raw); err != nil {
			return nil, err
		}
	}
	if raw, ok := root["build"]; ok {
		if doc.Build, err = parseBuild(raw); err != nil {
			return nil, err
		}
	}
	return doc, nil
}

func parseGo(raw any) (GoConfig, error) {
	table, ok := raw.(map[string]any)
	if !ok {
		return GoConfig{}, fmt.Errorf("go must be a table")
	}
	config := GoConfig{Imports: map[string]string{}}
	if value, ok := table["build_tags"]; ok {
		items, ok := value.([]any)
		if !ok {
			return GoConfig{}, fmt.Errorf("[go].build_tags must be a list of quoted strings")
		}
		validTag := regexp.MustCompile(`^[A-Za-z0-9_.]+$`)
		for _, item := range items {
			tag, ok := item.(string)
			if !ok {
				return GoConfig{}, fmt.Errorf("[go].build_tags must be a list of quoted strings")
			}
			if tag == "" || !validTag.MatchString(tag) {
				return GoConfig{}, fmt.Errorf("invalid Go build tag %q", tag)
			}
			config.BuildTags = append(config.BuildTags, tag)
		}
	}
	if value, ok := table["imports"]; ok {
		imports, ok := value.(map[string]any)
		if !ok {
			return GoConfig{}, fmt.Errorf("[go].imports must be a table")
		}
		for _, alias := range sortedKeys(imports) {
			if !parse.IsValidIdentifier(alias) {
				return GoConfig{}, fmt.Errorf("Go import alias %q is not a valid Ard identifier", alias)
			}
			path, ok := imports[alias].(string)
			if !ok {
				return GoConfig{}, fmt.Errorf("Go import alias %q path must be a string", alias)
			}
			if path == "" {
				return GoConfig{}, fmt.Errorf("Go import alias %q path must not be empty", alias)
			}
			if err := module.CheckImportPath(path); err != nil {
				return GoConfig{}, fmt.Errorf("Go import alias %q has invalid path %q: %w", alias, path, err)
			}
			config.Imports[alias] = path
		}
	}
	return config, nil
}

func parseDependencies(raw any) (map[string]Dependency, error) {
	table, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("dependencies must be a table")
	}
	dependencies := make(map[string]Dependency, len(table))
	for _, alias := range sortedKeys(table) {
		rawDependency, ok := table[alias].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("dependency %q must be a table", alias)
		}
		dependency := Dependency{}
		var err error
		if dependency.Path, err = optionalString(rawDependency, "path", "dependency path"); err != nil {
			return nil, fmt.Errorf("dependency %q: %w", alias, err)
		}
		if dependency.Git, err = optionalString(rawDependency, "git", "dependency git"); err != nil {
			return nil, fmt.Errorf("dependency %q: %w", alias, err)
		}
		if dependency.Tag, err = optionalString(rawDependency, "tag", "dependency tag"); err != nil {
			return nil, fmt.Errorf("dependency %q: %w", alias, err)
		}
		if dependency.Commit, err = optionalString(rawDependency, "commit", "dependency commit"); err != nil {
			return nil, fmt.Errorf("dependency %q: %w", alias, err)
		}
		dependencies[alias] = dependency
	}
	return dependencies, nil
}

func parseBuild(raw any) (BuildConfig, error) {
	table, ok := raw.(map[string]any)
	if !ok {
		return BuildConfig{}, fmt.Errorf("build must be a table")
	}
	if err := rejectUnknown(table, "build", "values"); err != nil {
		return BuildConfig{}, err
	}
	config := BuildConfig{Values: map[string]BuildValue{}}
	rawValues, ok := table["values"]
	if !ok {
		return config, nil
	}
	values, ok := rawValues.(map[string]any)
	if !ok {
		return BuildConfig{}, fmt.Errorf("build.values must be a table")
	}
	for _, name := range sortedKeys(values) {
		if !parse.IsValidIdentifier(name) {
			return BuildConfig{}, fmt.Errorf("build value %q is not a valid Ard identifier", name)
		}
		declaration, ok := values[name].(map[string]any)
		if !ok {
			return BuildConfig{}, fmt.Errorf("build value %q must be a table", name)
		}
		if err := rejectUnknown(declaration, "build value "+fmt.Sprintf("%q", name), "type", "default", "release"); err != nil {
			return BuildConfig{}, err
		}
		rawType, ok := declaration["type"]
		if !ok {
			return BuildConfig{}, fmt.Errorf("build value %q is missing type", name)
		}
		typeName, ok := rawType.(string)
		if !ok {
			return BuildConfig{}, fmt.Errorf("build value %q type must be a string", name)
		}
		valueType := BuildValueType(typeName)
		if valueType != BuildValueStr && valueType != BuildValueInt && valueType != BuildValueBool {
			return BuildConfig{}, fmt.Errorf("build value %q has unsupported type %q", name, typeName)
		}
		defaultValue, ok := declaration["default"]
		if !ok {
			return BuildConfig{}, fmt.Errorf("build value %q is missing default", name)
		}
		if err := validateDefault(name, valueType, defaultValue); err != nil {
			return BuildConfig{}, err
		}
		release := false
		if rawRelease, ok := declaration["release"]; ok {
			var valid bool
			release, valid = rawRelease.(bool)
			if !valid {
				return BuildConfig{}, fmt.Errorf("build value %q release must be Bool", name)
			}
		}
		config.Values[name] = BuildValue{Type: valueType, Default: defaultValue, Release: release}
	}
	return config, nil
}

func validateDefault(name string, valueType BuildValueType, value any) error {
	valid := false
	switch valueType {
	case BuildValueStr:
		_, valid = value.(string)
	case BuildValueInt:
		_, valid = value.(int64)
	case BuildValueBool:
		_, valid = value.(bool)
	}
	if !valid {
		return fmt.Errorf("build value %q default must be %s", name, valueType)
	}
	return nil
}

func optionalString(table map[string]any, key, label string) (string, error) {
	value, ok := table[key]
	if !ok {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", label)
	}
	return text, nil
}

func rejectUnknown(table map[string]any, label string, allowed ...string) error {
	known := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		known[key] = true
	}
	for _, key := range sortedKeys(table) {
		if !known[key] {
			return fmt.Errorf("%s has unknown field %q", label, key)
		}
	}
	return nil
}

func sortedKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
