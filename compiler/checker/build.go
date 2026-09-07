package checker

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/akonwi/ard/manifest"
)

const BuildModulePath = "ard/build"

type BuildOverride struct {
	Name  string
	Value string
}

type BuildOptions struct {
	Release   bool
	Overrides []BuildOverride
}

type effectiveBuildValue struct {
	Type  manifest.BuildValueType
	Value any
}

type buildModule struct {
	program *Program
	symbols map[string]Symbol
}

func (m *buildModule) Path() string               { return BuildModulePath }
func (m *buildModule) Program() *Program          { return m.program }
func (m *buildModule) Get(name string) Symbol     { return m.symbols[name] }
func (m *buildModule) Symbols() map[string]Symbol { return m.symbols }

func resolveBuildValues(config manifest.BuildConfig, options BuildOptions) (map[string]effectiveBuildValue, error) {
	values := make(map[string]effectiveBuildValue, len(config.Values))
	for name, declaration := range config.Values {
		value, err := effectiveValue(name, declaration.Type, declaration.Default)
		if err != nil {
			return nil, err
		}
		values[name] = value
	}

	explicit := make(map[string]bool, len(options.Overrides))
	for _, override := range options.Overrides {
		if explicit[override.Name] {
			return nil, fmt.Errorf("build value %q is defined more than once", override.Name)
		}
		explicit[override.Name] = true
		declaration, ok := config.Values[override.Name]
		if !ok {
			return nil, fmt.Errorf("unknown build value %q", override.Name)
		}
		value, err := parseBuildOverride(override, declaration.Type)
		if err != nil {
			return nil, err
		}
		values[override.Name] = value
	}

	if options.Release {
		missing := make([]string, 0)
		for name, declaration := range config.Values {
			if declaration.Release && !explicit[name] {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			return nil, fmt.Errorf("release build requires explicit --define values for: %s", joinQuoted(missing))
		}
	}
	return values, nil
}

func effectiveValue(name string, valueType manifest.BuildValueType, raw any) (effectiveBuildValue, error) {
	switch valueType {
	case manifest.BuildValueStr:
		return effectiveBuildValue{Type: valueType, Value: raw.(string)}, nil
	case manifest.BuildValueBool:
		return effectiveBuildValue{Type: valueType, Value: raw.(bool)}, nil
	case manifest.BuildValueInt:
		value := raw.(int64)
		parsed, err := strconv.ParseInt(strconv.FormatInt(value, 10), 10, strconv.IntSize)
		if err != nil {
			return effectiveBuildValue{}, fmt.Errorf("build value %q default overflows Int", name)
		}
		return effectiveBuildValue{Type: valueType, Value: int(parsed)}, nil
	default:
		return effectiveBuildValue{}, fmt.Errorf("build value %q has unsupported type %q", name, valueType)
	}
}

func parseBuildOverride(override BuildOverride, valueType manifest.BuildValueType) (effectiveBuildValue, error) {
	switch valueType {
	case manifest.BuildValueStr:
		return effectiveBuildValue{Type: valueType, Value: override.Value}, nil
	case manifest.BuildValueBool:
		if override.Value != "true" && override.Value != "false" {
			return effectiveBuildValue{}, fmt.Errorf("build value %q must be Bool (true or false)", override.Name)
		}
		return effectiveBuildValue{Type: valueType, Value: override.Value == "true"}, nil
	case manifest.BuildValueInt:
		value, err := strconv.ParseInt(override.Value, 10, strconv.IntSize)
		if err != nil {
			return effectiveBuildValue{}, fmt.Errorf("build value %q must be a decimal Int", override.Name)
		}
		return effectiveBuildValue{Type: valueType, Value: int(value)}, nil
	default:
		return effectiveBuildValue{}, fmt.Errorf("build value %q has unsupported type %q", override.Name, valueType)
	}
}

func newBuildModule(values map[string]effectiveBuildValue) Module {
	program := &Program{
		Imports:               map[string]Module{},
		GoImports:             map[string]*GoPackage{},
		StructMethods:         map[MethodOwner]map[string]*FunctionDef{},
		InherentMethods:       map[MethodOwner]map[string]*FunctionDef{},
		TraitMethods:          map[TraitMethodOwner]map[string]*FunctionDef{},
		AmbiguousTraitMethods: map[MethodOwner]map[string]bool{},
		RequiredGoMethods:     map[MethodOwner]map[string]*FunctionDef{},
		ForeignInterfaceImpls: map[MethodOwner][]*ForeignType{},
	}
	symbols := make(map[string]Symbol, len(values))
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := values[name]
		var typ Type
		var expression Expression
		switch value.Type {
		case manifest.BuildValueStr:
			typ = Str
			expression = &StrLiteral{Value: value.Value.(string)}
		case manifest.BuildValueBool:
			typ = Bool
			expression = &BoolLiteral{Value: value.Value.(bool)}
		case manifest.BuildValueInt:
			typ = Int
			expression = &IntLiteral{Value: value.Value.(int)}
		}
		program.Statements = append(program.Statements, Statement{Stmt: &VariableDef{Name: name, __type: typ, Value: expression}})
		symbols[name] = Symbol{Name: name, Type: typ}
	}
	return &buildModule{program: program, symbols: symbols}
}

func joinQuoted(names []string) string {
	result := ""
	for i, name := range names {
		if i > 0 {
			result += ", "
		}
		result += strconv.Quote(name)
	}
	return result
}
