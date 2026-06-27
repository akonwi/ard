package checker

import (
	"fmt"
	"go/constant"
	gotoken "go/token"
	"go/types"
	"math"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/stdlibgo"
	"golang.org/x/tools/go/packages"
)

type GoPackageResolver interface {
	LoadPackage(importPath string) (*GoPackage, error)
}

type GoPackage struct {
	ImportPath string
	Name       string
	Functions  map[string]GoFunction
	Types      map[string]GoType
	Constants  map[string]GoConstant
	Variables  map[string]GoVariable
}

type GoFunction struct {
	Name      string
	Signature GoSignature
}

type GoMethod struct {
	Name      string
	Signature GoSignature
}

type GoSignature struct {
	Receiver *GoValueType
	Params   []GoValueType
	Results  []GoValueType
	Variadic bool
}

type GoValueKind string

const (
	GoValueInvalid GoValueKind = ""
	GoValueBool    GoValueKind = "bool"
	GoValueString  GoValueKind = "string"
	GoValueInt     GoValueKind = "int"
	GoValueUint    GoValueKind = "uint"
	GoValueFloat   GoValueKind = "float"
	GoValueAny     GoValueKind = "any"
	GoValueSlice   GoValueKind = "slice"
	GoValueMap     GoValueKind = "map"
	GoValuePointer GoValueKind = "pointer"
	GoValueError   GoValueKind = "error"
	GoValueFunc    GoValueKind = "func"
	GoValueChan    GoValueKind = "chan"
	GoValueOther   GoValueKind = "other"
)

type GoValueType struct {
	Expr       string
	Kind       GoValueKind
	Bits       int
	Named      bool
	ImportPath string
	Package    string
	Name       string
	ParamName  string
	TypeParams int
	Elem       *GoValueType
	Key        *GoValueType
	Value      *GoValueType
	// Func holds the parameter/result types when Kind == GoValueFunc, i.e. the
	// Go parameter is a `func(...)` value that an Ard closure can satisfy.
	Func *GoSignature
	// ChanDir holds the direction when Kind == GoValueChan.
	ChanDir types.ChanDir
	Type    types.Type
}

type GoType struct {
	Name                 string
	Methods              map[string]GoMethod
	ValueMethods         map[string]GoMethod
	PointerMethods       map[string]GoMethod
	Fields               map[string]GoField
	Struct               bool
	Interface            bool
	HasUnexportedMethods bool
	TypeParams           int
	EnumConstants        []GoConstant
	ClosedEnum           bool
	Type                 types.Type
}

type GoField struct {
	Name string
	Type GoValueType
}

type GoConstant struct {
	Name           string
	Type           GoValueType
	IntValue       int
	HasIntValue    bool
	BoolValue      bool
	HasBoolValue   bool
	StringValue    string
	HasStringValue bool
	FloatValue     float64
	HasFloatValue  bool
}

type GoVariable struct {
	Name string
	Type GoValueType
}

type directGoImport struct {
	alias      string
	importPath string
	pkg        *GoPackage
}

func validDirectGoImportAlias(alias string) bool {
	return alias != "_" && alias != "init" && gotoken.IsIdentifier(alias) && gotoken.Lookup(alias) == gotoken.IDENT
}

type GoPackagesResolver struct {
	Dir   string
	cache map[string]*GoPackage
}

var sharedStdlibGoPackageCache sync.Map

func NewGoPackagesResolver(dir string) *GoPackagesResolver {
	return &GoPackagesResolver{Dir: dir, cache: map[string]*GoPackage{}}
}

func (r *GoPackagesResolver) LoadPackage(importPath string) (*GoPackage, error) {
	if r.cache == nil {
		r.cache = map[string]*GoPackage{}
	}
	if cached, ok := r.cache[importPath]; ok {
		return cached, nil
	}
	cfg := &packages.Config{
		Dir:        r.Dir,
		Mode:       packages.NeedName | packages.NeedTypes,
		BuildFlags: []string{"-mod=readonly"},
	}
	var sharedCacheKey string
	var useSharedCache bool
	if stdlibgo.IsBundledImportPath(importPath) {
		// The bundled standard library Go packages are not on the build's module
		// path, so resolve them against the embedded module materialized to a
		// content-hashed cache directory. Resolve hermetically, independent of any
		// enclosing Go workspace or hostile GOFLAGS, while preserving proxy/sumdb
		// configuration so deps download exactly as the generated program's build
		// would.
		bundledDir, err := stdlibgo.MaterializedDir()
		if err != nil {
			return nil, fmt.Errorf("materialize bundled standard library: %w", err)
		}
		cfg.Dir = bundledDir
		cfg.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
		sharedCacheKey = "bundled:" + stdlibgo.ContentHash() + ":" + goruntime.GOOS + "/" + goruntime.GOARCH + ":" + importPath
		useSharedCache = true
	} else {
		sharedCacheKey, useSharedCache = stdlibGoPackageCacheKey(r.Dir, importPath)
	}
	if useSharedCache {
		if cached, ok := sharedStdlibGoPackageCache.Load(sharedCacheKey); ok {
			pkg := cached.(*GoPackage)
			r.cache[importPath] = pkg
			return pkg, nil
		}
	}
	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("package not found")
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		parts := make([]string, len(pkg.Errors))
		for i, pkgErr := range pkg.Errors {
			parts[i] = pkgErr.Msg
		}
		return nil, fmt.Errorf("%s", strings.Join(parts, "; "))
	}
	if pkg.Types == nil {
		return nil, fmt.Errorf("package has no type information")
	}
	resolved := goPackageFromTypes(importPath, pkg.Name, pkg.Types)
	r.cache[importPath] = resolved
	if useSharedCache {
		sharedStdlibGoPackageCache.Store(sharedCacheKey, resolved)
	}
	return resolved, nil
}

func stdlibGoPackageCacheKey(dir string, importPath string) (string, bool) {
	if filepath.Clean(dir) != "." {
		return "", false
	}
	return importPath, true
}

func goPackageFromTypes(importPath string, name string, pkg *types.Package) *GoPackage {
	out := &GoPackage{
		ImportPath: importPath,
		Name:       name,
		Functions:  map[string]GoFunction{},
		Types:      map[string]GoType{},
		Constants:  map[string]GoConstant{},
		Variables:  map[string]GoVariable{},
	}
	if pkg == nil || pkg.Scope() == nil {
		return out
	}
	for _, name := range pkg.Scope().Names() {
		obj := pkg.Scope().Lookup(name)
		if obj == nil || !obj.Exported() {
			continue
		}
		switch obj := obj.(type) {
		case *types.Func:
			out.Functions[name] = GoFunction{Name: name, Signature: goSignature(obj.Type())}
		case *types.TypeName:
			fields, isStruct := exportedStructFields(obj.Type())
			valueMethods, pointerMethods := exportedMethodSets(obj.Type())
			out.Types[name] = GoType{Name: name, Methods: mergeGoMethods(valueMethods, pointerMethods), ValueMethods: valueMethods, PointerMethods: pointerMethods, Fields: fields, Struct: isStruct, Interface: isGoInterfaceType(obj.Type()), HasUnexportedMethods: hasUnexportedMethods(obj.Type()), TypeParams: goTypeParamCount(obj.Type()), Type: obj.Type()}
		case *types.Const:
			out.Constants[name] = goConstant(obj)
		case *types.Var:
			out.Variables[name] = goVariable(obj)
		}
	}
	attachEnumLikeConstants(out)
	return out
}

func goConstant(obj *types.Const) GoConstant {
	constantType := goValueType(obj.Type())
	intValue, intOK := goConstantIntValue(obj.Val())
	boolValue, boolOK := goConstantBoolValue(obj.Val())
	stringValue, stringOK := goConstantStringValue(obj.Val())
	floatValue, floatOK := goConstantFloatValue(obj.Val())
	return GoConstant{Name: obj.Name(), Type: constantType, IntValue: intValue, HasIntValue: intOK, BoolValue: boolValue, HasBoolValue: boolOK, StringValue: stringValue, HasStringValue: stringOK, FloatValue: floatValue, HasFloatValue: floatOK}
}

func goVariable(obj *types.Var) GoVariable {
	return GoVariable{Name: obj.Name(), Type: goValueType(obj.Type())}
}

func goConstantIntValue(value constant.Value) (int, bool) {
	if value == nil || value.Kind() != constant.Int {
		return 0, false
	}
	if signed, ok := constant.Int64Val(value); ok {
		if strconv.IntSize == 32 && (signed < -1<<31 || signed > 1<<31-1) {
			return 0, false
		}
		return int(signed), true
	}
	if unsigned, ok := constant.Uint64Val(value); ok {
		maxInt := uint64(1<<(strconv.IntSize-1) - 1)
		if unsigned <= maxInt {
			return int(unsigned), true
		}
	}
	return 0, false
}

func goConstantBoolValue(value constant.Value) (bool, bool) {
	if value == nil || value.Kind() != constant.Bool {
		return false, false
	}
	return constant.BoolVal(value), true
}

func goConstantStringValue(value constant.Value) (string, bool) {
	if value == nil || value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value), true
}

func goConstantFloatValue(value constant.Value) (float64, bool) {
	if value == nil || value.Kind() != constant.Float {
		return 0, false
	}
	floatValue, _ := constant.Float64Val(value)
	if math.IsInf(floatValue, 0) || math.IsNaN(floatValue) {
		return 0, false
	}
	return floatValue, true
}

func attachEnumLikeConstants(pkg *GoPackage) {
	if pkg == nil {
		return
	}
	names := make([]string, 0, len(pkg.Constants))
	for name := range pkg.Constants {
		names = append(names, name)
	}
	sort.Strings(names)
	byType := map[string][]GoConstant{}
	for _, name := range names {
		constant := pkg.Constants[name]
		if !goConstantIsEnumCandidate(pkg.ImportPath, constant) {
			continue
		}
		if _, ok := pkg.Types[constant.Type.Name]; !ok {
			continue
		}
		byType[constant.Type.Name] = append(byType[constant.Type.Name], constant)
	}
	for typeName, constants := range byType {
		if !goConstantsLookClosedEnum(constants) {
			continue
		}
		typ := pkg.Types[typeName]
		typ.EnumConstants = constants
		pkg.Types[typeName] = typ
	}
}

func goConstantIsEnumCandidate(importPath string, constant GoConstant) bool {
	if !constant.Type.Named || constant.Type.ImportPath != importPath || constant.Type.Name == "" {
		return false
	}
	return constant.Type.Kind == GoValueInt && constant.Type.Bits == 0
}

func goConstantsLookClosedEnum(constants []GoConstant) bool {
	if len(constants) == 0 {
		return false
	}
	values := map[int]struct{}{}
	for _, constant := range constants {
		if !constant.HasIntValue {
			return false
		}
		values[constant.IntValue] = struct{}{}
	}
	distinct := make([]int, 0, len(values))
	for value := range values {
		distinct = append(distinct, value)
	}
	sort.Ints(distinct)
	min := distinct[0]
	max := distinct[len(distinct)-1]
	return (min == 0 || min == 1) && max-min+1 == len(distinct)
}

func goTypeHasEnumConstant(typ GoType, name string) bool {
	for _, constant := range typ.EnumConstants {
		if constant.Name == name {
			return true
		}
	}
	return false
}

func (c *Checker) resolveDirectGoType(ty *parse.CustomType) Type {
	if ty == nil || ty.Type.Target == nil {
		return nil
	}
	alias, ok := ty.Type.Target.(*parse.Identifier)
	if !ok {
		return nil
	}
	property, ok := ty.Type.Property.(*parse.Identifier)
	if !ok {
		return nil
	}
	goImport, ok := c.directGoImports[alias.Name]
	if !ok {
		return nil
	}
	var goType GoType
	if goImport.pkg != nil {
		var ok bool
		goType, ok = goImport.pkg.Types[property.Name]
		if !ok {
			c.addError(fmt.Sprintf("Go package %q has no exported type %q", goImport.importPath, property.Name), ty.GetLocation())
			return &TypeVar{name: "unknown"}
		}
		if enum := c.directGoEnumType(goImport, goType, ty.GetLocation()); enum != nil {
			return enum
		}
	}
	binding := canonicalDirectGoBinding(goImport.importPath, goImport.alias, []string{property.Name})
	return directGoExternTypeWithMetadata(ty.GetName(), binding, &goType)
}

func (c *Checker) resolveDirectGoPackageValue(alias string, name string, loc parse.Location) Expression {
	goImport, ok := c.directGoImports[alias]
	if !ok || goImport.pkg == nil {
		return nil
	}
	if constant, ok := goImport.pkg.Constants[name]; ok {
		return c.resolveDirectGoConstantValue(goImport, constant, loc)
	}
	if variable, ok := goImport.pkg.Variables[name]; ok {
		return c.resolveDirectGoVariable(goImport, variable, loc)
	}
	c.addError(fmt.Sprintf("Go package %q has no exported enum-like constant or variable %q", goImport.importPath, name), loc)
	return nil
}

func (c *Checker) resolveDirectGoStructInstance(goImport directGoImport, inst *parse.StructInstance) Expression {
	if inst == nil {
		return nil
	}
	if goImport.pkg == nil {
		c.addError(fmt.Sprintf("Go package %q is not loaded; cannot construct Go struct %q", goImport.importPath, inst.Name.Name), inst.GetLocation())
		return nil
	}
	typeName := inst.Name.Name
	goType, ok := goImport.pkg.Types[typeName]
	if !ok {
		c.addError(fmt.Sprintf("Go package %q has no exported type %q", goImport.importPath, typeName), inst.Name.GetLocation())
		return nil
	}
	if !goType.Struct {
		c.addError(fmt.Sprintf("Go type %q in package %q is not an exported struct type", typeName, goImport.importPath), inst.Name.GetLocation())
		return nil
	}
	if goType.TypeParams > 0 {
		c.addError(fmt.Sprintf("Go generic struct type %q in package %q cannot be constructed directly", typeName, goImport.importPath), inst.Name.GetLocation())
		return nil
	}

	valid := true
	unsupportedFields := map[string]bool{}
	for _, fieldName := range sortedGoFieldNames(goType.Fields) {
		field := goType.Fields[fieldName]
		if !c.directGoAssignableShapeSupported(field.Type, inst.GetLocation(), true) {
			c.addError(fmt.Sprintf("Go field %s.%s has unsupported type %s", pkgQualifiedName(goImport.pkg, []string{typeName}), field.Name, field.Type.String()), inst.GetLocation())
			unsupportedFields[fieldName] = true
			valid = false
		}
	}

	fields := make(map[string]Expression, len(inst.Properties))
	fieldGoTypes := make(map[string]GoValueType, len(goType.Fields))
	providedFields := map[string]bool{}
	for _, property := range inst.Properties {
		fieldName := property.Name.Name
		field, ok := goType.Fields[fieldName]
		if !ok {
			c.addError(fmt.Sprintf("Go type %q in package %q has no exported field %q", typeName, goImport.importPath, fieldName), property.Value.GetLocation())
			valid = false
			continue
		}
		if providedFields[fieldName] {
			c.addError(fmt.Sprintf("Duplicate Go field: %s", fieldName), property.Value.GetLocation())
			valid = false
			continue
		}
		providedFields[fieldName] = true
		if unsupportedFields[fieldName] {
			continue
		}

		var expectedClosedEnum *Enum
		if enum, required := c.directGoClosedEnumAssignmentType(field.Type, property.Value.GetLocation()); required {
			if enum == nil {
				valid = false
				continue
			}
			expectedClosedEnum = enum
		}

		var value Expression
		c.withValueExprContext(func() {
			if expectedClosedEnum != nil {
				value = c.checkExprAs(property.Value, expectedClosedEnum)
			} else {
				value = c.checkExpr(property.Value)
			}
		})
		if value == nil {
			valid = false
			continue
		}
		if expectedClosedEnum != nil {
			if !directGoEnumTypeMatches(value.Type(), field.Type) {
				c.addError(fmt.Sprintf("field %s: %s", field.Name, typeMismatch(expectedClosedEnum, value.Type())), property.Value.GetLocation())
				valid = false
				continue
			}
		} else if ok, reason := c.directGoParamCompatible(value.Type(), field.Type, true); !ok {
			c.addError(fmt.Sprintf("field %s: %s", field.Name, reason), property.Value.GetLocation())
			valid = false
			continue
		}
		fields[fieldName] = value
		fieldGoTypes[fieldName] = field.Type
	}

	missing := []string{}
	for _, fieldName := range sortedGoFieldNames(goType.Fields) {
		if unsupportedFields[fieldName] {
			continue
		}
		if !providedFields[fieldName] {
			missing = append(missing, fieldName)
		}
	}
	if len(missing) > 0 {
		c.addError(fmt.Sprintf("Missing Go field: %s", strings.Join(missing, ", ")), inst.GetLocation())
		valid = false
	}
	if !valid {
		return nil
	}

	binding := canonicalDirectGoBinding(goImport.importPath, goImport.alias, []string{typeName})
	name := typeName
	if goImport.alias != "" {
		name = goImport.alias + "::" + typeName
	}
	valueType := directGoExternTypeWithMetadata(name, binding, &goType)
	return &DirectGoStructInstance{ImportPath: goImport.importPath, Alias: goImport.alias, PackageName: goImport.pkg.Name, Name: typeName, Binding: binding, Fields: fields, FieldGoTypes: fieldGoTypes, ValueType: valueType}
}

func sortedGoFieldNames(fields map[string]GoField) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Checker) directGoAssignableShapeSupported(goType GoValueType, loc parse.Location, topLevel bool) bool {
	if goType.Named && goType.Kind != GoValuePointer {
		return goType.ImportPath != "" && goType.Name != "" && !c.directGoNamedTypeHasTypeParamsOrLoad(goType, loc)
	}
	switch goType.Kind {
	case GoValueBool, GoValueString, GoValueAny:
		return true
	case GoValueInt:
		return topLevel || goType.Bits == 0 || goType.Bits == 32
	case GoValueUint:
		return topLevel || goType.Bits == 8
	case GoValueFloat:
		return topLevel || goType.Bits == 64
	case GoValuePointer:
		if goType.Named || goType.Elem == nil || !goType.Elem.Named {
			return false
		}
		return goType.Elem.ImportPath != "" && goType.Elem.Name != "" && !c.directGoNamedTypeHasTypeParamsOrLoad(*goType.Elem, loc) && !c.directGoNamedTypeIsEnumLike(*goType.Elem, loc)
	case GoValueSlice:
		return goType.Elem != nil && c.directGoAssignableShapeSupported(*goType.Elem, loc, false)
	case GoValueMap:
		return goType.Key != nil && goType.Value != nil && c.directGoAssignableShapeSupported(*goType.Key, loc, false) && c.directGoAssignableShapeSupported(*goType.Value, loc, false)
	default:
		return false
	}
}

func (c *Checker) directGoNamedTypeHasTypeParams(goType GoValueType) bool {
	if goType.TypeParams > 0 {
		return true
	}
	if !goType.Named || goType.ImportPath == "" || goType.Name == "" {
		return false
	}
	goImport, ok := c.directGoImportForPath(goType.ImportPath)
	if !ok || goImport.pkg == nil {
		return false
	}
	typ, ok := goImport.pkg.Types[goType.Name]
	return ok && typ.TypeParams > 0
}

func (c *Checker) directGoNamedTypeHasTypeParamsOrLoad(goType GoValueType, loc parse.Location) bool {
	if goType.TypeParams > 0 {
		return true
	}
	if !goType.Named || goType.ImportPath == "" || goType.Name == "" {
		return false
	}
	goImport, ok := c.directGoImportForPathOrLoad(goType.ImportPath, loc)
	if !ok || goImport.pkg == nil {
		return false
	}
	typ, ok := goImport.pkg.Types[goType.Name]
	return ok && typ.TypeParams > 0
}

func (c *Checker) directGoNamedTypeIsEnumLike(goType GoValueType, loc parse.Location) bool {
	if !goType.Named || goType.ImportPath == "" || goType.Name == "" {
		return false
	}
	goImport, ok := c.directGoImportForPathOrLoad(goType.ImportPath, loc)
	if !ok || goImport.pkg == nil {
		return false
	}
	typ, ok := goImport.pkg.Types[goType.Name]
	return ok && len(typ.EnumConstants) > 0
}

func (c *Checker) resolveDirectGoVariable(goImport directGoImport, variable GoVariable, loc parse.Location) Expression {
	beforeDiagnostics := len(c.diagnostics)
	valueType, ok := c.directGoValueArdType(variable.Type, loc)
	if !ok {
		message := fmt.Sprintf("Go variable %s.%s has unsupported type %s", goImport.pkg.Name, variable.Name, variable.Type.String())
		if len(c.diagnostics) > beforeDiagnostics {
			last := len(c.diagnostics) - 1
			c.diagnostics[last].Message = message + ": " + c.diagnostics[last].Message
		} else {
			c.addError(message, loc)
		}
		return nil
	}
	binding := canonicalDirectGoBinding(goImport.importPath, goImport.alias, []string{variable.Name})
	return &DirectGoPackageValue{ImportPath: goImport.importPath, Alias: goImport.alias, PackageName: goImport.pkg.Name, Name: variable.Name, Binding: binding, ValueType: valueType}
}

func (c *Checker) resolveDirectGoConstant(alias string, name string, loc parse.Location) Expression {
	goImport, ok := c.directGoImports[alias]
	if !ok || goImport.pkg == nil {
		return nil
	}
	constant, ok := goImport.pkg.Constants[name]
	if !ok {
		c.addError(fmt.Sprintf("Go package %q has no exported constant %q", goImport.importPath, name), loc)
		return nil
	}
	return c.resolveDirectGoConstantValue(goImport, constant, loc)
}

func (c *Checker) resolveDirectGoConstantValue(goImport directGoImport, constant GoConstant, loc parse.Location) Expression {
	if goConstantIsEnumCandidate(goImport.importPath, constant) {
		if goType, ok := goImport.pkg.Types[constant.Type.Name]; ok && goTypeHasEnumConstant(goType, constant.Name) {
			enum := c.directGoEnumType(goImport, goType, loc)
			if enum == nil {
				return nil
			}
			for i := range enum.Values {
				if enum.Values[i].Name == constant.Name {
					return &EnumVariant{enum: enum, Variant: i, EnumType: enum, Discriminant: enum.Values[i].Value}
				}
			}
			c.addError(fmt.Sprintf("Go constant %s.%s is not part of enum-like type %s", goImport.pkg.Name, constant.Name, goType.Name), loc)
			return nil
		}
	}
	valueType, ok := c.directGoConstantArdType(goImport, constant, loc)
	if !ok {
		return nil
	}
	binding := canonicalDirectGoBinding(goImport.importPath, goImport.alias, []string{constant.Name})
	return &DirectGoPackageValue{ImportPath: goImport.importPath, Alias: goImport.alias, PackageName: goImport.pkg.Name, Name: constant.Name, Binding: binding, ValueType: valueType}
}

func (c *Checker) directGoConstantArdType(goImport directGoImport, constant GoConstant, loc parse.Location) (Type, bool) {
	switch {
	case constant.HasBoolValue:
		return Bool, true
	case constant.HasStringValue:
		return Str, true
	case constant.HasIntValue:
		return Int, true
	case constant.HasFloatValue:
		return Float, true
	}
	c.addError(fmt.Sprintf("Go constant %s.%s has unsupported type %s", goImport.pkg.Name, constant.Name, constant.Type.String()), loc)
	return nil, false
}

func (c *Checker) directGoEnumType(goImport directGoImport, goType GoType, loc parse.Location) *Enum {
	if len(goType.EnumConstants) == 0 {
		return nil
	}
	values := make([]EnumValue, 0, len(goType.EnumConstants))
	seenNames := map[string]struct{}{}
	for _, constant := range goType.EnumConstants {
		if _, dup := seenNames[constant.Name]; dup {
			continue
		}
		seenNames[constant.Name] = struct{}{}
		if !constant.HasIntValue {
			c.addError(fmt.Sprintf("Go constant %s.%s has a value that cannot be represented as an Ard enum discriminant", goImport.pkg.Name, constant.Name), loc)
			continue
		}
		values = append(values, EnumValue{Name: constant.Name, Value: constant.IntValue})
	}
	if len(values) == 0 {
		return nil
	}
	binding := canonicalDirectGoBinding(goImport.importPath, goImport.alias, []string{goType.Name})
	return &Enum{Name: goType.Name, ModulePath: "go:" + goImport.importPath, Values: values, Methods: map[string]*FunctionDef{}, Location: loc, ExternalBinding: binding, ExternalBindings: map[string]string{"go": binding}, Open: !goType.ClosedEnum}
}

func canonicalDirectGoBinding(importPath string, alias string, symbolParts []string) string {
	head := importPath
	if strings.TrimSpace(alias) != "" {
		head += " as " + alias
	}
	return "go:" + head + "::" + strings.Join(symbolParts, "::")
}

func directGoBindingParts(binding string) ([]string, bool) {
	if !strings.Contains(binding, "::") {
		return nil, false
	}
	parts := strings.Split(binding, "::")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return parts, true
		}
	}
	return parts, true
}

type canonicalDirectGoBindingInfo struct {
	ImportPath string
	Alias      string
	Symbols    []string
}

func parseCanonicalDirectGoBinding(binding string) (canonicalDirectGoBindingInfo, bool) {
	if !strings.HasPrefix(binding, "go:") {
		return canonicalDirectGoBindingInfo{}, false
	}
	parts := strings.Split(strings.TrimPrefix(binding, "go:"), "::")
	if len(parts) != 2 && len(parts) != 3 {
		return canonicalDirectGoBindingInfo{}, false
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return canonicalDirectGoBindingInfo{}, false
		}
	}
	importPath, alias := parseCanonicalDirectGoImportHead(parts[0])
	if importPath == "" {
		return canonicalDirectGoBindingInfo{}, false
	}
	return canonicalDirectGoBindingInfo{ImportPath: importPath, Alias: alias, Symbols: parts[1:]}, true
}

func parseCanonicalDirectGoImportHead(head string) (string, string) {
	parts := strings.Split(head, " as ")
	if len(parts) == 1 {
		return strings.TrimSpace(head), ""
	}
	if len(parts) != 2 {
		return "", ""
	}
	importPath := strings.TrimSpace(parts[0])
	alias := strings.TrimSpace(parts[1])
	if importPath == "" || alias == "" {
		return "", ""
	}
	return importPath, alias
}

type directGoSignatureTarget struct {
	Name      string
	Binding   string
	Signature GoSignature
	Method    bool
}

func (c *Checker) isDirectGoStaticFunction(call *parse.StaticFunction) bool {
	parts := strings.Split(call.Target.String()+"::"+call.Function.Name, "::")
	if len(parts) < 2 {
		return false
	}
	_, ok := c.directGoImports[parts[0]]
	return ok
}

func (c *Checker) checkDirectGoStaticFunction(call *parse.StaticFunction) (Expression, bool) {
	return c.checkDirectGoStaticFunctionAs(call, nil)
}

func (c *Checker) checkDirectGoStaticFunctionAs(call *parse.StaticFunction, expected Type) (Expression, bool) {
	parts := strings.Split(call.Target.String()+"::"+call.Function.Name, "::")
	if len(parts) < 2 {
		return nil, false
	}
	goImport, ok := c.directGoImports[parts[0]]
	if !ok {
		return nil, false
	}
	if len(call.Function.TypeArgs) > 0 {
		c.addError(fmt.Sprintf("Go function %s does not take Ard type arguments", strings.Join(parts, "::")), call.GetLocation())
		return nil, true
	}
	target, ok := c.directGoCallTarget(goImport, parts[1:], call.GetLocation())
	if !ok {
		return nil, true
	}
	if target.Signature.Variadic {
		c.addError(fmt.Sprintf("Go function %s is variadic; variadic direct Go calls are not supported yet", target.Name), call.GetLocation())
		return nil, true
	}
	goParams, ok := directGoCallSourceParams(target, call.GetLocation(), func(message string, loc parse.Location) { c.addError(message, loc) })
	if !ok {
		return nil, true
	}
	args, params, ok := c.checkDirectGoCallArguments(call.Function.Args, goParams, call.GetLocation())
	if !ok {
		return nil, true
	}
	if target.Method && target.Signature.Receiver != nil {
		if ok, reason := c.directGoAssignableCompatible(args[0].Type(), *target.Signature.Receiver); !ok {
			c.addError(fmt.Sprintf("receiver for %s: %s", target.Name, reason), call.Function.Args[0].Value.GetLocation())
			return nil, true
		}
	}
	returnType, ok := c.directGoReturnType(target.Signature.Results, call.GetLocation(), expected)
	if !ok {
		return nil, true
	}
	if expected != nil && expected != Void && !areCompatible(expected, returnType) {
		c.addError(typeMismatch(expected, returnType), call.GetLocation())
		return nil, true
	}
	return c.directGoFunctionCall(strings.Join(parts, "::"), args, params, returnType, target.Binding), true
}

func (c *Checker) checkDirectGoInstanceProperty(subject Expression, fieldName string, loc parse.Location) (Expression, bool) {
	goImport, typeName, field, ok, handled := c.directGoStructField(subject, fieldName, loc)
	if !handled {
		return nil, false
	}
	if !ok {
		return nil, true
	}
	beforeDiagnostics := len(c.diagnostics)
	fieldType, ok := c.directGoValueArdType(field.Type, loc)
	if !ok {
		message := fmt.Sprintf("Go field %s.%s has unsupported type %s", pkgQualifiedName(goImport.pkg, []string{typeName}), field.Name, field.Type.String())
		if len(c.diagnostics) > beforeDiagnostics {
			last := len(c.diagnostics) - 1
			c.diagnostics[last].Message = message + ": " + c.diagnostics[last].Message
		} else {
			c.addError(message, loc)
		}
		return nil, true
	}
	return &DirectGoFieldAccess{Subject: subject, Field: field.Name, FieldType: fieldType, FieldGoType: field.Type}, true
}

func (c *Checker) checkDirectGoInstancePropertyAssignmentTarget(subject Expression, fieldName string, loc parse.Location) (*DirectGoFieldAccess, bool) {
	_, _, field, ok, handled := c.directGoStructField(subject, fieldName, loc)
	if !handled {
		return nil, false
	}
	if !ok {
		return nil, true
	}
	return &DirectGoFieldAccess{Subject: subject, Field: field.Name, FieldType: Void, FieldGoType: field.Type}, true
}

func (c *Checker) directGoStructField(subject Expression, fieldName string, loc parse.Location) (directGoImport, string, GoField, bool, bool) {
	importPath, typeName, ok := directGoNamedTypeBinding(subject.Type())
	if !ok {
		return directGoImport{}, "", GoField{}, false, false
	}
	goImport, ok := c.directGoImportForPathOrLoad(importPath, loc)
	if !ok || goImport.pkg == nil {
		return directGoImport{}, "", GoField{}, false, false
	}
	typ, ok := goImport.pkg.Types[typeName]
	if !ok {
		c.addError(fmt.Sprintf("Go package %q has no exported type %q", importPath, typeName), loc)
		return goImport, typeName, GoField{}, false, true
	}
	field, ok := typ.Fields[fieldName]
	if !ok {
		c.addError(fmt.Sprintf("Go type %q in package %q has no exported field %q", typeName, importPath, fieldName), loc)
		return goImport, typeName, GoField{}, false, true
	}
	return goImport, typeName, field, true, true
}

func (c *Checker) directGoClosedEnumAssignmentType(goType GoValueType, loc parse.Location) (*Enum, bool) {
	if !goType.Named || goType.ImportPath == "" || goType.Name == "" {
		return nil, false
	}
	goImport, ok := c.directGoImportForPathOrLoad(goType.ImportPath, loc)
	if !ok || goImport.pkg == nil {
		return nil, false
	}
	typ, ok := goImport.pkg.Types[goType.Name]
	if !ok || len(typ.EnumConstants) == 0 || !typ.ClosedEnum {
		return nil, false
	}
	return c.directGoEnumType(goImport, typ, loc), true
}

func (c *Checker) checkDirectGoInstanceMethod(subject Expression, call parse.FunctionCall, loc parse.Location) (Expression, bool) {
	return c.checkDirectGoInstanceMethodAs(subject, call, loc, nil)
}

func (c *Checker) checkDirectGoInstanceMethodAs(subject Expression, call parse.FunctionCall, loc parse.Location, expected Type) (Expression, bool) {
	importPath, typeName, ok := directGoNamedTypeBinding(subject.Type())
	if !ok {
		return nil, false
	}
	goImport, ok := c.directGoImportForPathOrLoad(importPath, loc)
	if !ok {
		return nil, false
	}
	if len(call.TypeArgs) > 0 {
		c.addError(fmt.Sprintf("Go method %s::%s does not take Ard type arguments", typeName, call.Name), loc)
		return nil, true
	}
	target, ok := c.directGoCallTarget(goImport, []string{typeName, call.Name}, loc)
	if !ok {
		return nil, true
	}
	if target.Signature.Variadic {
		c.addError(fmt.Sprintf("Go method %s is variadic; variadic direct Go calls are not supported yet", target.Name), loc)
		return nil, true
	}
	if target.Signature.Receiver == nil {
		c.addError(fmt.Sprintf("Go method %s has no receiver metadata", target.Name), loc)
		return nil, true
	}
	recvOK, recvReason := c.directGoAssignableCompatible(subject.Type(), *target.Signature.Receiver)
	if !recvOK {
		// A mutable-reference lvalue (a `mut T` field read deref's to its value
		// type) auto-borrows back into `mut T` for a pointer receiver (ADR 0031).
		if refType := referenceArgType(subject); !refType.equal(subject.Type()) {
			if ok2, _ := c.directGoAssignableCompatible(refType, *target.Signature.Receiver); ok2 {
				recvOK = true
			}
		}
	}
	if !recvOK {
		c.addError(fmt.Sprintf("receiver for %s: %s", target.Name, recvReason), loc)
		return nil, true
	}
	args, params, ok := c.checkDirectGoCallArguments(call.Args, target.Signature.Params, loc)
	if !ok {
		return nil, true
	}
	args = append([]Expression{subject}, args...)
	params = append([]Parameter{{Name: "receiver", Type: subject.Type()}}, params...)
	returnType, ok := c.directGoReturnType(target.Signature.Results, loc, expected)
	if !ok {
		return nil, true
	}
	if expected != nil && expected != Void && !areCompatible(expected, returnType) {
		c.addError(typeMismatch(expected, returnType), loc)
		return nil, true
	}
	callName := goImport.alias
	if callName == "" {
		callName = goImport.importPath
	}
	callName += "::" + typeName + "::" + call.Name
	return c.directGoFunctionCall(callName, args, params, returnType, target.Binding), true
}

func directGoNamedTypeBinding(typ Type) (string, string, bool) {
	typ = deref(typ)
	if ref, ok := typ.(*MutableRef); ok {
		typ = deref(ref.Of())
	}
	var binding string
	switch typed := typ.(type) {
	case *ExternType:
		binding = typed.ExternalBinding
	case *Enum:
		binding = typed.ExternalBinding
	default:
		return "", "", false
	}
	info, ok := parseCanonicalDirectGoBinding(binding)
	if !ok || len(info.Symbols) != 1 {
		return "", "", false
	}
	return info.ImportPath, info.Symbols[0], true
}

func (c *Checker) directGoImportForPath(importPath string) (directGoImport, bool) {
	for _, goImport := range c.directGoImports {
		if goImport.importPath == importPath {
			return goImport, true
		}
	}
	return directGoImport{}, false
}

func (c *Checker) directGoImportForPathOrLoad(importPath string, loc parse.Location) (directGoImport, bool) {
	if goImport, ok := c.directGoImportForPath(importPath); ok {
		return goImport, true
	}
	if c.options.GoResolver == nil {
		return directGoImport{}, false
	}
	pkg, err := c.options.GoResolver.LoadPackage(importPath)
	if err != nil {
		c.addError(fmt.Sprintf("Failed to load Go package '%s': %v", importPath, err), loc)
		return directGoImport{}, false
	}
	alias := ""
	if pkg != nil && validDirectGoImportAlias(pkg.Name) {
		alias = pkg.Name
	}
	goImport := directGoImport{alias: alias, importPath: importPath, pkg: pkg}
	c.directGoImports["go:"+importPath] = goImport
	return goImport, true
}

func (c *Checker) directGoCallTarget(goImport directGoImport, symbols []string, loc parse.Location) (directGoSignatureTarget, bool) {
	binding := canonicalDirectGoBinding(goImport.importPath, goImport.alias, symbols)
	if goImport.pkg == nil {
		return directGoSignatureTarget{Binding: binding}, false
	}
	switch len(symbols) {
	case 1:
		fn, ok := goImport.pkg.Functions[symbols[0]]
		if !ok {
			c.addError(fmt.Sprintf("Go package %q has no exported function %q", goImport.importPath, symbols[0]), loc)
			return directGoSignatureTarget{}, false
		}
		return directGoSignatureTarget{Name: pkgQualifiedName(goImport.pkg, symbols), Binding: binding, Signature: fn.Signature}, true
	case 2:
		typ, ok := goImport.pkg.Types[symbols[0]]
		if !ok {
			c.addError(fmt.Sprintf("Go package %q has no exported type %q", goImport.importPath, symbols[0]), loc)
			return directGoSignatureTarget{}, false
		}
		method, ok := typ.Methods[symbols[1]]
		if !ok {
			c.addError(fmt.Sprintf("Go type %q in package %q has no exported method %q", symbols[0], goImport.importPath, symbols[1]), loc)
			return directGoSignatureTarget{}, false
		}
		return directGoSignatureTarget{Name: pkgQualifiedName(goImport.pkg, symbols), Binding: binding, Signature: method.Signature, Method: true}, true
	default:
		c.addError(fmt.Sprintf("Direct Go function call %q must be package::Function or package::Type::Method", strings.Join(append([]string{goImport.alias}, symbols...), "::")), loc)
		return directGoSignatureTarget{}, false
	}
}

func directGoCallSourceParams(target directGoSignatureTarget, loc parse.Location, addError func(string, parse.Location)) ([]GoValueType, bool) {
	if !target.Method {
		return target.Signature.Params, true
	}
	if target.Signature.Receiver == nil {
		addError(fmt.Sprintf("Go method %s has no receiver metadata", target.Name), loc)
		return nil, false
	}
	params := make([]GoValueType, 0, len(target.Signature.Params)+1)
	params = append(params, *target.Signature.Receiver)
	params = append(params, target.Signature.Params...)
	return params, true
}

func (c *Checker) checkDirectGoCallArguments(rawArgs []parse.Argument, goParams []GoValueType, loc parse.Location) ([]Expression, []Parameter, bool) {
	for _, arg := range rawArgs {
		if arg.Name != "" {
			c.addError("Direct Go calls do not support named arguments", arg.GetLocation())
			return nil, nil, false
		}
	}
	if len(rawArgs) != len(goParams) {
		c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d", len(goParams), len(rawArgs)), loc)
		return nil, nil, false
	}
	args := make([]Expression, len(rawArgs))
	params := make([]Parameter, len(rawArgs))
	for i, arg := range rawArgs {
		checkedArg := c.checkExpr(arg.Value)
		if checkedArg == nil {
			return nil, nil, false
		}
		ok, reason := c.directGoParamCompatible(checkedArg.Type(), goParams[i], true)
		if !ok {
			// A mutable-reference lvalue (e.g. a `mut T` field whose read deref's to
			// its value type) auto-borrows back into `mut T` at the call site, so a
			// stored Go pointer handle can be re-passed (ADR 0031).
			if refType := referenceArgType(checkedArg); refType != checkedArg.Type() {
				if ok2, _ := c.directGoParamCompatible(refType, goParams[i], true); ok2 {
					ok = true
				}
			}
		}
		if !ok {
			c.addError(fmt.Sprintf("parameter %d: %s", i+1, reason), arg.Value.GetLocation())
			return nil, nil, false
		}
		args[i] = checkedArg
		params[i] = Parameter{Name: fmt.Sprintf("arg%d", i), Type: checkedArg.Type()}
	}
	return args, params, true
}

func (c *Checker) directGoFunctionCall(name string, args []Expression, params []Parameter, returnType Type, binding string) *FunctionCall {
	fn := &FunctionDef{Name: name, Parameters: params, ReturnType: returnType}
	return &FunctionCall{Name: name, Args: args, fn: fn, ReturnType: returnType, ExternalBinding: binding}
}

func (c *Checker) directGoReturnType(results []GoValueType, loc parse.Location, expected Type) (Type, bool) {
	if expected != nil && expected != Void && c.directGoResultAdapterCompatible(expected, results) {
		return expected, true
	}
	switch len(results) {
	case 0:
		return Void, true
	case 1:
		if results[0].Kind == GoValueError {
			return MakeResult(Void, Str), true
		}
		return c.directGoValueArdType(results[0], loc)
	case 2:
		valueType, ok := c.directGoValueArdType(results[0], loc)
		if !ok {
			return nil, false
		}
		switch results[1].Kind {
		case GoValueError:
			return MakeResult(valueType, Str), true
		case GoValueBool:
			if results[1].Named {
				c.addError(fmt.Sprintf("direct Go maybe adapter requires bool, got named bool %s", results[1].String()), loc)
				return nil, false
			}
			return MakeMaybe(valueType), true
		}
	}
	c.addError(fmt.Sprintf("Go returns %s; no supported direct call adapter matches", goSignatureResultsString(results)), loc)
	return nil, false
}

func (c *Checker) directGoValueArdType(goType GoValueType, loc parse.Location) (Type, bool) {
	if goType.Kind == GoValuePointer {
		if goType.Named {
			c.addError(fmt.Sprintf("Go named pointer type %s is not supported by direct Go pointer bindings yet", goType.String()), loc)
			return nil, false
		}
		if goType.Elem == nil {
			c.addError("Go pointer type is missing element metadata", loc)
			return nil, false
		}
		elem, ok := c.directGoValueArdType(*goType.Elem, loc)
		if !ok {
			return nil, false
		}
		if _, ok := elem.(*Enum); ok {
			c.addError(fmt.Sprintf("Go pointer to enum-like type %s is not supported by direct Go pointer bindings yet", goType.String()), loc)
			return nil, false
		}
		if _, ok := elem.(*ExternType); !ok {
			c.addError(fmt.Sprintf("Go pointer type %s is not supported by direct Go pointer bindings yet", goType.String()), loc)
			return nil, false
		}
		return MakeMutableRef(elem), true
	}
	if goType.Named {
		return c.directGoNamedArdType(goType, loc)
	}
	switch goType.Kind {
	case GoValueFunc:
		if goType.Func == nil {
			c.addError("Go func type is missing signature metadata", loc)
			return nil, false
		}
		return c.directGoFuncArdType(goType, loc)
	case GoValueBool:
		return Bool, true
	case GoValueString:
		return Str, true
	case GoValueInt:
		if goType.Bits == 0 {
			return Int, true
		}
		if goType.Bits == 32 {
			return Rune, true
		}
	case GoValueUint:
		if goType.Bits == 8 {
			return Byte, true
		}
	case GoValueFloat:
		if goType.Bits == 64 {
			return Float, true
		}
	case GoValueAny:
		return Dynamic, true
	case GoValueSlice:
		if goType.Elem != nil {
			elem, ok := c.directGoValueArdType(*goType.Elem, loc)
			if ok {
				return MakeList(elem), true
			}
			return nil, false
		}
	case GoValueMap:
		if goType.Key != nil && goType.Value != nil {
			key, ok := c.directGoValueArdType(*goType.Key, loc)
			if !ok {
				return nil, false
			}
			value, ok := c.directGoValueArdType(*goType.Value, loc)
			if !ok {
				return nil, false
			}
			return MakeMap(key, value), true
		}
	case GoValueChan:
		if goType.Elem == nil {
			c.addError("Go channel type is missing element metadata", loc)
			return nil, false
		}
		elem, ok := c.directGoValueArdType(*goType.Elem, loc)
		if !ok {
			return nil, false
		}
		switch goType.ChanDir {
		case types.RecvOnly:
			return MakeReceiver(elem), true
		case types.SendOnly:
			return MakeSender(elem), true
		default:
			return MakeChan(elem), true
		}
	case GoValueError:
		c.addError("Go error values require a Result adapter", loc)
		return nil, false
	}
	c.addError(fmt.Sprintf("Go type %s cannot be represented as an inferred Ard direct Go value type", goType.String()), loc)
	return nil, false
}

// directGoFuncArdType maps a Go callback signature to the Ard function type a
// closure must have. Parameters and a single result map recursively; a Go
// pointer parameter (which maps to a mutable ref) becomes a `mut` Ard
// parameter. Signatures that cannot map without a boundary adapter (multiple
// results, a bare error, an optional pointer) are reported as unsupported.
func (c *Checker) directGoFuncArdType(goType GoValueType, loc parse.Location) (Type, bool) {
	if len(goType.Func.Results) > 1 {
		c.addError(fmt.Sprintf("Go func type %s is not supported by direct Go bindings yet: callbacks with multiple results require an adapter", goType.String()), loc)
		return nil, false
	}
	params := make([]Parameter, len(goType.Func.Params))
	for i := range goType.Func.Params {
		pt, ok := c.directGoValueArdType(goType.Func.Params[i], loc)
		if !ok {
			return nil, false
		}
		mutable := false
		if ref, isRef := pt.(*MutableRef); isRef {
			mutable = true
			pt = ref.Of()
		}
		params[i] = Parameter{Name: fmt.Sprintf("arg%d", i), Type: pt, Mutable: mutable}
	}
	returnType := Type(Void)
	if len(goType.Func.Results) == 1 {
		rt, ok := c.directGoValueArdType(goType.Func.Results[0], loc)
		if !ok {
			return nil, false
		}
		returnType = rt
	}
	return &FunctionDef{Parameters: params, ReturnType: returnType}, true
}

func (c *Checker) directGoNamedArdType(goType GoValueType, loc parse.Location) (Type, bool) {
	if goType.ImportPath == "" || goType.Name == "" {
		c.addError(fmt.Sprintf("Go named type %s is missing package metadata", goType.String()), loc)
		return nil, false
	}
	if c.directGoNamedTypeHasTypeParams(goType) {
		c.addError(fmt.Sprintf("Go generic type %s is not supported by direct Go bindings yet", goType.String()), loc)
		return nil, false
	}
	var goImport directGoImport
	var goImportOK bool
	var metadata *GoType
	if explicit, ok := c.directGoImportForPath(goType.ImportPath); ok {
		goImport = explicit
		goImportOK = true
		if explicit.pkg != nil {
			if typ, ok := explicit.pkg.Types[goType.Name]; ok {
				metadata = &typ
				if enum := c.directGoEnumType(explicit, typ, loc); enum != nil {
					return enum, true
				}
			}
		}
	}
	if metadata == nil {
		if loaded, ok := c.directGoImportForPathOrLoad(goType.ImportPath, loc); ok && loaded.pkg != nil {
			if typ, ok := loaded.pkg.Types[goType.Name]; ok {
				metadata = &typ
			}
		}
	}
	alias := ""
	if goImportOK {
		alias = goImport.alias
	} else if validDirectGoImportAlias(goType.Package) {
		alias = goType.Package
	}
	binding := canonicalDirectGoBinding(goType.ImportPath, alias, []string{goType.Name})
	name := goType.Name
	qualifier := alias
	if qualifier == "" {
		qualifier = goType.Package
	}
	if qualifier != "" {
		name = qualifier + "::" + goType.Name
	}
	return directGoExternTypeWithMetadata(name, binding, metadata), true
}

func (c *Checker) directGoSignatureTarget(binding string) (directGoSignatureTarget, bool) {
	info, ok := parseCanonicalDirectGoBinding(binding)
	if !ok {
		return directGoSignatureTarget{}, false
	}
	var pkg *GoPackage
	for _, imp := range c.directGoImports {
		if imp.importPath == info.ImportPath {
			pkg = imp.pkg
			break
		}
	}
	if pkg == nil {
		return directGoSignatureTarget{}, false
	}
	switch len(info.Symbols) {
	case 1:
		fn, ok := pkg.Functions[info.Symbols[0]]
		if !ok {
			return directGoSignatureTarget{}, false
		}
		return directGoSignatureTarget{Name: pkgQualifiedName(pkg, info.Symbols), Signature: fn.Signature}, true
	case 2:
		typ, ok := pkg.Types[info.Symbols[0]]
		if !ok {
			return directGoSignatureTarget{}, false
		}
		method, ok := typ.Methods[info.Symbols[1]]
		if !ok {
			return directGoSignatureTarget{}, false
		}
		return directGoSignatureTarget{Name: pkgQualifiedName(pkg, info.Symbols), Signature: method.Signature, Method: true}, true
	default:
		return directGoSignatureTarget{}, false
	}
}

func pkgQualifiedName(pkg *GoPackage, symbols []string) string {
	qualifier := ""
	if pkg != nil {
		qualifier = pkg.Name
		if qualifier == "" {
			qualifier = pkg.ImportPath
		}
	}
	if qualifier == "" {
		return strings.Join(symbols, ".")
	}
	return qualifier + "." + strings.Join(symbols, ".")
}

func (c *Checker) validateDirectGoExternReturn(name string, returnType Type, target directGoSignatureTarget, loc parse.Location) {
	results := target.Signature.Results
	if len(results) == 0 {
		if !equalTypes(returnType, Void) {
			c.addError(fmt.Sprintf("return for %s: Go returns nothing, extern %s declares %s", target.Name, name, returnType.String()), loc)
		}
		return
	}
	if c.directGoResultAdapterCompatible(returnType, results) {
		return
	}
	if len(results) == 2 && results[1].Kind == GoValueError {
		if result, ok := derefType(returnType).(*Result); ok && equalTypes(result.Err(), Str) {
			if ok, reason := c.directGoReturnValueCompatible(result.Val(), results[0]); !ok {
				c.addError(fmt.Sprintf("return value for %s: %s", target.Name, reason), loc)
				return
			}
		}
	}
	if len(results) != 1 {
		c.addError(fmt.Sprintf("return for %s: Go returns %s; no supported adapter matches extern %s return %s", target.Name, goSignatureResultsString(results), name, typeSyntaxString(returnType)), loc)
		return
	}
	if results[0].Kind == GoValueError {
		c.addError(fmt.Sprintf("return for %s: Go return error can only adapt to Void!Str", target.Name), loc)
		return
	}
	if ok, reason := c.directGoReturnValueCompatible(returnType, results[0]); !ok {
		c.addError(fmt.Sprintf("return for %s: %s", target.Name, reason), loc)
	}
}

func (c *Checker) directGoResultAdapterCompatible(returnType Type, results []GoValueType) bool {
	returnType = derefType(returnType)
	if len(results) == 1 {
		if results[0].Kind == GoValueError {
			return directGoVoidStrResult(returnType)
		}
		ok, _ := c.directGoReturnValueCompatible(returnType, results[0])
		return ok
	}
	if len(results) != 2 {
		return false
	}
	if results[1].Kind == GoValueError {
		result, ok := returnType.(*Result)
		if !ok || !equalTypes(result.Err(), Str) {
			return false
		}
		ok, _ = c.directGoReturnValueCompatible(result.Val(), results[0])
		return ok
	}
	if results[1].Kind == GoValueBool && !results[1].Named {
		maybe, ok := returnType.(*Maybe)
		if !ok {
			return false
		}
		ok, _ = c.directGoReturnValueCompatible(maybe.Of(), results[0])
		return ok
	}
	return false
}

func directGoVoidStrResult(returnType Type) bool {
	result, ok := returnType.(*Result)
	return ok && equalTypes(result.Val(), Void) && equalTypes(result.Err(), Str)
}

func goSignatureResultsString(results []GoValueType) string {
	parts := make([]string, len(results))
	for i, result := range results {
		parts[i] = result.String()
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func (c *Checker) directGoReturnValueCompatible(ard Type, goType GoValueType) (bool, string) {
	return c.directGoParamCompatible(ard, goType, true)
}

func (c *Checker) directGoParamCompatible(ard Type, goType GoValueType, topLevel bool) (bool, string) {
	ard = derefType(ard)
	if goType.Named && c.directGoNamedTypeHasTypeParams(goType) {
		return false, fmt.Sprintf("Go generic type %s is not supported by direct Go bindings yet", goType.String())
	}
	if directGoNamedTypeMatches(ard, goType) {
		return true, ""
	}
	if topLevel && c.directGoInterfaceAssignable(ard, goType) {
		return true, ""
	}
	if goType.Kind == GoValuePointer {
		if goType.Named {
			return false, fmt.Sprintf("Go named pointer type %s is not supported by direct Go pointer bindings yet", goType.String())
		}
		if goType.Elem != nil && goType.Elem.Named && c.directGoNamedTypeHasTypeParams(*goType.Elem) {
			return false, fmt.Sprintf("Go generic pointer element type %s is not supported by direct Go bindings yet", goType.Elem.String())
		}
		if directGoPointerToEnumLike(ard, goType) {
			return false, fmt.Sprintf("Go pointer to enum-like type %s is not supported by direct Go pointer bindings yet", goType.String())
		}
		// An optional Ard value passes to a Go pointer parameter: some(x) -> &x,
		// none() -> nil (ADR 0031).
		if directGoMaybePointerCompatible(ard, goType) {
			return true, ""
		}
		if directGoPointerCompatible(ard, goType) {
			return true, ""
		}
		return false, fmt.Sprintf("Go type %s requires Ard type %s", goType.String(), directGoPointerArdTypeString(goType))
	}
	if topLevel && directGoScalarCompatible(ard, goType) {
		return true, ""
	}
	if goType.Named {
		return false, fmt.Sprintf("Ard type %s is not compatible with Go named type %s", typeSyntaxString(ard), goType.String())
	}
	switch goType.Kind {
	case GoValueBool, GoValueString, GoValueInt, GoValueUint, GoValueFloat:
		if directGoAssignableScalarCompatible(ard, goType) {
			return true, ""
		}
	case GoValueAny:
		if equalTypes(ard, Dynamic) {
			return true, ""
		}
	case GoValueSlice:
		list, ok := ard.(*List)
		if ok && goType.Elem != nil {
			if compatible, reason := c.directGoParamCompatible(list.Of(), *goType.Elem, false); compatible {
				return true, ""
			} else {
				return false, "list element: " + reason
			}
		}
	case GoValueMap:
		m, ok := ard.(*Map)
		if ok && goType.Key != nil && goType.Value != nil {
			if compatible, reason := c.directGoParamCompatible(m.Key(), *goType.Key, false); !compatible {
				return false, "map key: " + reason
			}
			if compatible, reason := c.directGoParamCompatible(m.Value(), *goType.Value, false); !compatible {
				return false, "map value: " + reason
			}
			return true, ""
		}
	case GoValueFunc:
		fn, ok := ard.(*FunctionDef)
		if !ok {
			return false, fmt.Sprintf("Ard type %s is not a function compatible with Go type %s", typeSyntaxString(ard), goType.String())
		}
		if goType.Func == nil {
			return false, "Go func type is missing signature metadata"
		}
		if len(goType.Func.Results) > 1 {
			return false, fmt.Sprintf("Go func type %s is not supported by direct Go bindings yet: callbacks with multiple results require an adapter", goType.String())
		}
		if len(fn.Parameters) != len(goType.Func.Params) {
			return false, fmt.Sprintf("Go type %s expects a callback with %d parameter(s), got %d", goType.String(), len(goType.Func.Params), len(fn.Parameters))
		}
		// The closure passes through with no boundary adapter, so its lowered Go
		// signature must match the target func type exactly.
		for i := range fn.Parameters {
			// A `mut` parameter (whether written `mut x: T` or `x: mut T`) lowers
			// to a Go pointer, so normalize either form before mapping.
			paramType := fn.Parameters[i].Type
			mutable := fn.Parameters[i].Mutable
			if ref, isRef := paramType.(*MutableRef); isRef {
				mutable = true
				paramType = ref.Of()
			}
			got, ok := c.goValueTypeForArdType(paramType, mutable)
			if !ok || !goValueTypesMatch(got, goType.Func.Params[i]) {
				return false, fmt.Sprintf("Go type %s callback parameter %d expects a value of Go type %s", goType.String(), i+1, goType.Func.Params[i].String())
			}
		}
		retIsVoid := fn.ReturnType == nil || fn.ReturnType == Void
		if len(goType.Func.Results) == 0 {
			if !retIsVoid {
				return false, fmt.Sprintf("Go type %s expects a callback returning Void, got a callback returning %s", goType.String(), typeSyntaxString(fn.ReturnType))
			}
		} else {
			if retIsVoid {
				return false, fmt.Sprintf("Go type %s expects a callback returning a value of Go type %s, got Void", goType.String(), goType.Func.Results[0].String())
			}
			got, ok := c.goValueTypeForArdType(fn.ReturnType, false)
			if !ok || !goValueTypesMatch(got, goType.Func.Results[0]) {
				return false, fmt.Sprintf("Go type %s expects a callback returning a value of Go type %s", goType.String(), goType.Func.Results[0].String())
			}
		}
		return true, ""
	case GoValueError:
		return false, "Go error values require an adapter; direct Go error adapters are not supported yet"
	}
	return false, fmt.Sprintf("Ard type %s is not compatible with Go type %s", typeSyntaxString(ard), goType.String())
}

func (c *Checker) directGoAssignableCompatible(ard Type, goType GoValueType) (bool, string) {
	return c.directGoAssignableCompatibleAt(ard, goType, true)
}

func (c *Checker) directGoAssignableCompatibleAt(ard Type, goType GoValueType, topLevel bool) (bool, string) {
	ard = derefType(ard)
	if goType.Named && c.directGoNamedTypeHasTypeParams(goType) {
		return false, fmt.Sprintf("Go generic type %s is not supported by direct Go bindings yet", goType.String())
	}
	if directGoNamedTypeMatches(ard, goType) {
		return true, ""
	}
	if topLevel && c.directGoInterfaceAssignable(ard, goType) {
		return true, ""
	}
	if goType.Kind == GoValuePointer {
		if goType.Named {
			return false, fmt.Sprintf("Go named pointer type %s is not supported by direct Go pointer bindings yet", goType.String())
		}
		if goType.Elem != nil && goType.Elem.Named && c.directGoNamedTypeHasTypeParams(*goType.Elem) {
			return false, fmt.Sprintf("Go generic pointer element type %s is not supported by direct Go bindings yet", goType.Elem.String())
		}
		if directGoPointerToEnumLike(ard, goType) {
			return false, fmt.Sprintf("Go pointer to enum-like type %s is not supported by direct Go pointer bindings yet", goType.String())
		}
		if directGoPointerCompatible(ard, goType) {
			return true, ""
		}
		return false, fmt.Sprintf("Go type %s requires Ard type %s", goType.String(), directGoPointerArdTypeString(goType))
	}
	if goType.Named {
		return false, fmt.Sprintf("Ard type %s is not compatible with Go named type %s", typeSyntaxString(ard), goType.String())
	}
	switch goType.Kind {
	case GoValueBool, GoValueString, GoValueInt, GoValueUint, GoValueFloat:
		if directGoAssignableScalarCompatible(ard, goType) {
			return true, ""
		}
	case GoValueAny:
		if equalTypes(ard, Dynamic) {
			return true, ""
		}
	case GoValueSlice:
		list, ok := ard.(*List)
		if ok && goType.Elem != nil {
			if compatible, reason := c.directGoAssignableCompatibleAt(list.Of(), *goType.Elem, false); compatible {
				return true, ""
			} else {
				return false, "list element: " + reason
			}
		}
	case GoValueMap:
		m, ok := ard.(*Map)
		if ok && goType.Key != nil && goType.Value != nil {
			if compatible, reason := c.directGoAssignableCompatibleAt(m.Key(), *goType.Key, false); !compatible {
				return false, "map key: " + reason
			}
			if compatible, reason := c.directGoAssignableCompatibleAt(m.Value(), *goType.Value, false); !compatible {
				return false, "map value: " + reason
			}
			return true, ""
		}
	case GoValueError:
		return false, "Go error values require an adapter; direct Go error adapters are not supported yet"
	}
	return false, fmt.Sprintf("Ard type %s is not compatible with Go type %s", typeSyntaxString(ard), goType.String())
}

func directGoScalarCompatible(ard Type, goType GoValueType) bool {
	switch goType.Kind {
	case GoValueBool:
		return equalTypes(ard, Bool)
	case GoValueString:
		return equalTypes(ard, Str)
	case GoValueInt:
		return equalTypes(ard, Int) || (goType.Bits == 32 && equalTypes(ard, Rune))
	case GoValueUint:
		return equalTypes(ard, Int) || (goType.Bits == 8 && equalTypes(ard, Byte))
	case GoValueFloat:
		return equalTypes(ard, Float)
	default:
		return false
	}
}

func directGoAssignableScalarCompatible(ard Type, goType GoValueType) bool {
	switch goType.Kind {
	case GoValueBool:
		return equalTypes(ard, Bool)
	case GoValueString:
		return equalTypes(ard, Str)
	case GoValueInt:
		return equalTypes(ard, Int) && goType.Bits == 0 || equalTypes(ard, Rune) && goType.Bits == 32
	case GoValueUint:
		return equalTypes(ard, Byte) && goType.Bits == 8
	case GoValueFloat:
		return equalTypes(ard, Float) && goType.Bits == 64
	default:
		return false
	}
}

func directGoExternTypeWithMetadata(name string, binding string, metadata *GoType) *ExternType {
	ext := &ExternType{Name_: name, ExternalBinding: binding, ExternalBindings: map[string]string{"go": binding}}
	if metadata == nil {
		return ext
	}
	ext.DirectGoInterface = metadata.Interface
	ext.DirectGoHasUnexportedMethods = metadata.HasUnexportedMethods
	ext.DirectGoMethods = cloneGoMethodMap(metadata.Methods)
	ext.DirectGoValueMethods = cloneGoMethodMap(metadata.ValueMethods)
	ext.DirectGoPointerMethods = cloneGoMethodMap(metadata.PointerMethods)
	ext.DirectGoType = metadata.Type
	return ext
}

func cloneGoMethodMap(methods map[string]GoMethod) map[string]GoMethod {
	if methods == nil {
		return nil
	}
	out := make(map[string]GoMethod, len(methods))
	for name, method := range methods {
		out[name] = method
	}
	return out
}

func directGoNamedTypeMatches(ard Type, goType GoValueType) bool {
	if !goType.Named || goType.ImportPath == "" || goType.Name == "" {
		return false
	}
	if extern, ok := ard.(*ExternType); ok {
		return directGoBindingMatchesNamedType(extern.ExternalBinding, goType)
	}
	if enum, ok := ard.(*Enum); ok {
		return directGoBindingMatchesNamedType(enum.ExternalBinding, goType)
	}
	return false
}

func directGoEnumTypeMatches(ard Type, goType GoValueType) bool {
	if !goType.Named || goType.ImportPath == "" || goType.Name == "" {
		return false
	}
	enum, ok := derefType(ard).(*Enum)
	return ok && directGoBindingMatchesNamedType(enum.ExternalBinding, goType)
}

func directGoBindingMatchesNamedType(binding string, goType GoValueType) bool {
	info, ok := parseCanonicalDirectGoBinding(binding)
	return ok && info.ImportPath == goType.ImportPath && len(info.Symbols) == 1 && info.Symbols[0] == goType.Name
}

func directGoPointerToEnumLike(ard Type, goType GoValueType) bool {
	if goType.Kind != GoValuePointer || goType.Elem == nil {
		return false
	}
	ref, ok := ard.(*MutableRef)
	if !ok {
		return false
	}
	if _, ok := ref.Of().(*Enum); !ok {
		return false
	}
	return directGoNamedTypeMatches(ref.Of(), *goType.Elem)
}

// directGoMaybePointerCompatible reports whether an Ard Maybe over a primitive
// value maps to a Go pointer to the matching primitive (`Int?` <-> `*int`).
func directGoMaybePointerCompatible(ard Type, goType GoValueType) bool {
	if goType.Kind != GoValuePointer || goType.Elem == nil || goType.Elem.Named {
		return false
	}
	maybe, ok := derefType(ard).(*Maybe)
	if !ok {
		return false
	}
	return directGoScalarCompatible(maybe.Of(), *goType.Elem)
}

func directGoPointerCompatible(ard Type, goType GoValueType) bool {
	if goType.Kind != GoValuePointer || goType.Elem == nil {
		return false
	}
	ref, ok := ard.(*MutableRef)
	if !ok {
		return false
	}
	if _, ok := ref.Of().(*ExternType); !ok {
		return false
	}
	return directGoNamedTypeMatches(ref.Of(), *goType.Elem)
}

func directGoInterfaceCompatible(expected Type, actual Type) bool {
	expected = derefType(expected)
	expectedExtern, ok := expected.(*ExternType)
	if !ok || !expectedExtern.DirectGoInterface {
		return false
	}
	if expectedExtern.DirectGoType != nil {
		if actualGoType, ok := directGoExternGoTypeForType(actual); ok && types.AssignableTo(actualGoType, expectedExtern.DirectGoType) {
			return true
		}
		// AssignableTo compares by type identity, which fails across separate
		// go/packages loads; fall through to the structural method-set check.
	}
	required := directGoExternInterfaceMethods(expectedExtern)
	if required == nil {
		return false
	}
	actualMethods, ok := directGoExternMethodSetForType(actual)
	return ok && goMethodSetImplements(actualMethods, required)
}

func (c *Checker) directGoInterfaceCompatible(expected Type, actual Type) bool {
	expected = derefType(expected)
	expectedExtern, ok := expected.(*ExternType)
	if !ok || !expectedExtern.DirectGoInterface {
		return false
	}
	if expectedExtern.DirectGoType != nil {
		if actualGoType, ok := c.directGoGoTypeForArdType(actual); ok && types.AssignableTo(actualGoType, expectedExtern.DirectGoType) {
			return true
		}
		// AssignableTo compares by type identity, which fails across separate
		// go/packages loads; fall through to the structural method-set check
		// unless the interface has unexported methods an external type can't have.
		if expectedExtern.DirectGoHasUnexportedMethods {
			return false
		}
	}
	required := directGoExternInterfaceMethods(expectedExtern)
	if required == nil {
		return false
	}
	actualMethods, ok := c.directGoMethodSetForArdType(actual)
	return ok && goMethodSetImplements(actualMethods, required)
}

func directGoExternGoTypeForType(typ Type) (types.Type, bool) {
	typ = derefType(typ)
	pointer := false
	if ref, ok := typ.(*MutableRef); ok {
		pointer = true
		typ = derefType(ref.Of())
	}
	ext, ok := typ.(*ExternType)
	if !ok || ext.DirectGoType == nil {
		return nil, false
	}
	if pointer {
		return types.NewPointer(ext.DirectGoType), true
	}
	return ext.DirectGoType, true
}

func directGoExternInterfaceMethods(ext *ExternType) map[string]GoMethod {
	if ext == nil || !ext.DirectGoInterface {
		return nil
	}
	if ext.DirectGoMethods != nil {
		return ext.DirectGoMethods
	}
	if ext.DirectGoValueMethods != nil {
		return ext.DirectGoValueMethods
	}
	return nil
}

func directGoExternMethodSetForType(typ Type) (map[string]GoMethod, bool) {
	typ = derefType(typ)
	pointer := false
	if ref, ok := typ.(*MutableRef); ok {
		pointer = true
		typ = derefType(ref.Of())
	}
	ext, ok := typ.(*ExternType)
	if !ok {
		return nil, false
	}
	if pointer {
		if ext.DirectGoInterface {
			return nil, false
		}
		if ext.DirectGoPointerMethods != nil {
			return ext.DirectGoPointerMethods, true
		}
		if ext.DirectGoMethods != nil {
			return ext.DirectGoMethods, true
		}
		return nil, false
	}
	if ext.DirectGoInterface {
		methods := directGoExternInterfaceMethods(ext)
		return methods, methods != nil
	}
	if ext.DirectGoValueMethods != nil {
		return ext.DirectGoValueMethods, true
	}
	if ext.DirectGoMethods != nil {
		return ext.DirectGoMethods, true
	}
	return nil, false
}

func (c *Checker) directGoInterfaceAssignable(ard Type, goType GoValueType) bool {
	expected, ok := c.directGoNamedGoType(goType)
	if !ok || !expected.Interface {
		return false
	}
	if expected.Type != nil {
		if actualType, ok := c.directGoGoTypeForArdType(ard); ok && types.AssignableTo(actualType, expected.Type) {
			return true
		}
		// AssignableTo compares by type identity, which fails when the concrete
		// type and the interface come from separate go/packages loads. Fall
		// through to the structural method-set comparison below, unless the
		// interface has unexported methods, which an external type cannot satisfy.
		if expected.HasUnexportedMethods {
			return false
		}
	}
	required := expected.Methods
	if required == nil {
		return false
	}
	actualMethods, ok := c.directGoMethodSetForArdType(ard)
	return ok && goMethodSetImplements(actualMethods, required)
}

func (c *Checker) directGoNamedGoType(goType GoValueType) (GoType, bool) {
	if !goType.Named || goType.ImportPath == "" || goType.Name == "" {
		return GoType{}, false
	}
	goImport, ok := c.directGoImportForPathOrLoad(goType.ImportPath, parse.Location{})
	if !ok || goImport.pkg == nil {
		return GoType{}, false
	}
	typ, ok := goImport.pkg.Types[goType.Name]
	return typ, ok
}

func (c *Checker) directGoGoTypeForArdType(typ Type) (types.Type, bool) {
	if goType, ok := directGoExternGoTypeForType(typ); ok {
		return goType, true
	}
	typ = derefType(typ)
	pointer := false
	if ref, ok := typ.(*MutableRef); ok {
		pointer = true
		typ = derefType(ref.Of())
	}
	importPath, typeName, ok := directGoNamedTypeBinding(typ)
	if !ok {
		return nil, false
	}
	goImport, ok := c.directGoImportForPathOrLoad(importPath, parse.Location{})
	if !ok || goImport.pkg == nil {
		return nil, false
	}
	goType, ok := goImport.pkg.Types[typeName]
	if !ok || goType.Type == nil {
		return nil, false
	}
	if pointer {
		return types.NewPointer(goType.Type), true
	}
	return goType.Type, true
}

func (c *Checker) directGoMethodSetForArdType(typ Type) (map[string]GoMethod, bool) {
	if methods, ok := directGoExternMethodSetForType(typ); ok {
		return methods, true
	}
	original := typ
	typ = derefType(typ)
	pointer := false
	if ref, ok := typ.(*MutableRef); ok {
		pointer = true
		typ = derefType(ref.Of())
	}
	if methods, ok := c.ardDefinedGoMethodSetForType(original, pointer); ok {
		return methods, true
	}
	importPath, typeName, ok := directGoNamedTypeBinding(typ)
	if !ok {
		return nil, false
	}
	goImport, ok := c.directGoImportForPathOrLoad(importPath, parse.Location{})
	if !ok || goImport.pkg == nil {
		return nil, false
	}
	goType, ok := goImport.pkg.Types[typeName]
	if !ok {
		return nil, false
	}
	if pointer {
		if goType.Interface {
			return nil, false
		}
		if goType.PointerMethods != nil {
			return goType.PointerMethods, true
		}
		return goType.Methods, true
	}
	if goType.Interface {
		return goType.Methods, true
	}
	if goType.ValueMethods != nil {
		return goType.ValueMethods, true
	}
	return goType.Methods, true
}

func (c *Checker) ardDefinedGoMethodSetForType(typ Type, pointer bool) (map[string]GoMethod, bool) {
	if ref, ok := typ.(*MutableRef); ok {
		pointer = true
		typ = derefType(ref.Of())
	} else {
		typ = derefType(typ)
	}
	def, ok := typ.(*StructDef)
	if !ok {
		return nil, false
	}
	methods := c.structMethods(def)
	if methods == nil {
		return nil, true
	}
	goNameCounts := map[string]int{}
	for name := range methods {
		if goName, ok := ardGoMethodName(name); ok {
			goNameCounts[goName]++
		}
	}
	out := map[string]GoMethod{}
	for name, method := range methods {
		if method == nil || method.hasGenerics() {
			continue
		}
		goName, ok := ardGoMethodName(name)
		if !ok || goNameCounts[goName] > 1 || ardGoMethodNameUnavailableOnStruct(def, goName) {
			continue
		}
		if method.Mutates && !pointer {
			continue
		}
		signature, ok := c.goSignatureForArdMethod(method)
		if !ok {
			continue
		}
		out[goName] = GoMethod{Name: goName, Signature: signature}
	}
	return out, true
}

func (c *Checker) goSignatureForArdMethod(method *FunctionDef) (GoSignature, bool) {
	params := make([]GoValueType, 0, len(method.Parameters))
	for _, param := range method.Parameters {
		goType, ok := c.goValueTypeForArdType(param.Type, param.Mutable)
		if !ok {
			return GoSignature{}, false
		}
		params = append(params, goType)
	}
	results := []GoValueType{}
	if !equalTypes(derefType(method.ReturnType), Void) {
		goType, ok := c.goValueTypeForArdType(method.ReturnType, false)
		if !ok {
			return GoSignature{}, false
		}
		results = append(results, goType)
	}
	return GoSignature{Params: params, Results: results}, true
}

func (c *Checker) goValueTypeForArdType(typ Type, mutable bool) (GoValueType, bool) {
	if mutable {
		inner, ok := c.goValueTypeForArdType(typ, false)
		if !ok {
			return GoValueType{}, false
		}
		return GoValueType{Kind: GoValuePointer, Expr: "*" + inner.String(), Elem: &inner}, true
	}
	typ = derefType(typ)
	if ext, ok := typ.(*ExternType); ok {
		if ext.DirectGoType != nil {
			return goValueType(ext.DirectGoType), true
		}
		if ext.ExternalBinding != "" {
			if binding, ok := parseCanonicalDirectGoBinding(ext.ExternalBinding); ok && len(binding.Symbols) == 1 {
				if goImport, ok := c.directGoImportForPathOrLoad(binding.ImportPath, parse.Location{}); ok && goImport.pkg != nil {
					if goType, ok := goImport.pkg.Types[binding.Symbols[0]]; ok {
						if goType.Type != nil {
							return goValueType(goType.Type), true
						}
						return GoValueType{Expr: goType.Name, Kind: GoValueOther, Named: true, ImportPath: binding.ImportPath, Package: goImport.pkg.Name, Name: goType.Name}, true
					}
				}
			}
		}
	}
	switch typ {
	case Int:
		return GoValueType{Kind: GoValueInt, Expr: "int"}, true
	case Float:
		return GoValueType{Kind: GoValueFloat, Expr: "float64", Bits: 64}, true
	case Bool:
		return GoValueType{Kind: GoValueBool, Expr: "bool"}, true
	case Byte:
		return GoValueType{Kind: GoValueUint, Expr: "byte", Bits: 8}, true
	case Rune:
		return GoValueType{Kind: GoValueInt, Expr: "rune", Bits: 32}, true
	case Str:
		return GoValueType{Kind: GoValueString, Expr: "string"}, true
	}
	switch typed := typ.(type) {
	case *List:
		elem, ok := c.goValueTypeForArdType(typed.Of(), false)
		if !ok {
			return GoValueType{}, false
		}
		return GoValueType{Kind: GoValueSlice, Expr: "[]" + elem.String(), Elem: &elem}, true
	case *Map:
		key, ok := c.goValueTypeForArdType(typed.Key(), false)
		if !ok {
			return GoValueType{}, false
		}
		value, ok := c.goValueTypeForArdType(typed.Value(), false)
		if !ok {
			return GoValueType{}, false
		}
		return GoValueType{Kind: GoValueMap, Expr: "map[" + key.String() + "]" + value.String(), Key: &key, Value: &value}, true
	}
	return GoValueType{}, false
}

func ardGoMethodName(raw string) (string, bool) {
	name := ardSanitizeGoName(raw)
	if name == "" || name == "_" {
		return "", false
	}
	if gotoken.Lookup(name).IsKeyword() {
		name += "_"
	}
	if !gotoken.IsIdentifier(name) {
		return "", false
	}
	return name, true
}

func ardGoMethodNameUnavailableOnStruct(def *StructDef, methodName string) bool {
	if def == nil {
		return false
	}
	for field := range def.Fields {
		if ardGoFieldName(field) == methodName {
			return true
		}
	}
	switch methodName {
	case "MarshalJSONTo", "UnmarshalJSONFrom":
		return true
	default:
		return false
	}
}

func ardGoFieldName(raw string) string {
	name := ardSanitizeGoName(raw)
	if name == "" {
		return "field"
	}
	if gotoken.Lookup(name).IsKeyword() {
		return name + "_"
	}
	return name
}

func ardSanitizeGoName(raw string) string {
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

func goMethodSetImplements(actual map[string]GoMethod, required map[string]GoMethod) bool {
	if required == nil || actual == nil {
		return false
	}
	for name, requiredMethod := range required {
		actualMethod, ok := actual[name]
		if !ok || !goMethodSignaturesMatch(actualMethod.Signature, requiredMethod.Signature) {
			return false
		}
	}
	return true
}

func goMethodSignaturesMatch(actual GoSignature, required GoSignature) bool {
	if actual.Variadic != required.Variadic || len(actual.Params) != len(required.Params) || len(actual.Results) != len(required.Results) {
		return false
	}
	for i := range required.Params {
		if !goValueTypesMatch(actual.Params[i], required.Params[i]) {
			return false
		}
	}
	for i := range required.Results {
		if !goValueTypesMatch(actual.Results[i], required.Results[i]) {
			return false
		}
	}
	return true
}

func goValueTypesMatch(left GoValueType, right GoValueType) bool {
	// Prefer exact identity, but fall through to a structural (name + shape)
	// comparison when it fails: types loaded in separate go/packages sessions are
	// distinct instances even when they denote the same Go type.
	if left.Type != nil && right.Type != nil && types.Identical(left.Type, right.Type) {
		return true
	}
	if left.Named || right.Named {
		return left.Named == right.Named && left.ImportPath == right.ImportPath && left.Name == right.Name
	}
	if left.Kind != right.Kind || left.Bits != right.Bits {
		return false
	}
	switch left.Kind {
	case GoValuePointer, GoValueSlice:
		if left.Elem == nil || right.Elem == nil {
			return left.Elem == right.Elem
		}
		return goValueTypesMatch(*left.Elem, *right.Elem)
	case GoValueMap:
		if left.Key == nil || right.Key == nil || left.Value == nil || right.Value == nil {
			return left.Key == right.Key && left.Value == right.Value
		}
		return goValueTypesMatch(*left.Key, *right.Key) && goValueTypesMatch(*left.Value, *right.Value)
	case GoValueOther, GoValueInvalid:
		return left.Expr != "" && left.Expr == right.Expr
	default:
		return true
	}
}

func directGoPointerArdTypeString(goType GoValueType) string {
	if goType.Elem == nil {
		return "mut ?"
	}
	return "mut " + directGoArdTypeString(*goType.Elem)
}

func directGoArdTypeString(goType GoValueType) string {
	if goType.Named && goType.Name != "" {
		if strings.Contains(goType.Expr, ".") {
			return strings.ReplaceAll(goType.Expr, ".", "::")
		}
		qualifier := goType.Package
		if qualifier == "" || strings.Contains(qualifier, "/") {
			qualifier = pathBase(goType.ImportPath)
		}
		if qualifier != "" {
			return qualifier + "::" + goType.Name
		}
		return goType.Name
	}
	return goType.String()
}

func pathBase(value string) string {
	idx := strings.LastIndex(value, "/")
	if idx >= 0 {
		return value[idx+1:]
	}
	return value
}

func (t GoValueType) String() string {
	if t.Expr != "" {
		return t.Expr
	}
	switch t.Kind {
	case GoValueBool:
		return "bool"
	case GoValueString:
		return "string"
	case GoValueInt:
		if t.Bits > 0 {
			return fmt.Sprintf("int%d", t.Bits)
		}
		return "int"
	case GoValueUint:
		if t.Bits > 0 {
			return fmt.Sprintf("uint%d", t.Bits)
		}
		return "uint"
	case GoValueFloat:
		if t.Bits == 32 {
			return "float32"
		}
		return "float64"
	case GoValueAny:
		return "any"
	case GoValueSlice:
		if t.Elem != nil {
			return "[]" + t.Elem.String()
		}
		return "[]?"
	case GoValueMap:
		if t.Key != nil && t.Value != nil {
			return "map[" + t.Key.String() + "]" + t.Value.String()
		}
		return "map[?]?"
	case GoValuePointer:
		if t.Elem != nil {
			return "*" + t.Elem.String()
		}
		return "*?"
	case GoValueError:
		return "error"
	default:
		return "?"
	}
}

func goSignature(typ types.Type) GoSignature {
	sig, ok := typ.(*types.Signature)
	if !ok || sig == nil {
		return GoSignature{}
	}
	out := GoSignature{Params: goTuple(sig.Params()), Results: goTuple(sig.Results()), Variadic: sig.Variadic()}
	if recv := sig.Recv(); recv != nil {
		receiver := goValueType(recv.Type())
		out.Receiver = &receiver
	}
	return out
}

func goTuple(tuple *types.Tuple) []GoValueType {
	if tuple == nil || tuple.Len() == 0 {
		return nil
	}
	out := make([]GoValueType, tuple.Len())
	for i := 0; i < tuple.Len(); i++ {
		out[i] = goValueType(tuple.At(i).Type())
		out[i].ParamName = tuple.At(i).Name()
	}
	return out
}

func goValueType(typ types.Type) GoValueType {
	return goValueTypeSeen(typ, map[types.Type]bool{})
}

func goValueTypeSeen(typ types.Type, seen map[types.Type]bool) GoValueType {
	if typ == nil {
		return GoValueType{Kind: GoValueInvalid}
	}
	if seen[typ] {
		return goOpaqueType(typ)
	}
	seen[typ] = true
	defer delete(seen, typ)

	out := GoValueType{Expr: goTypeExpr(typ), Kind: GoValueOther, Type: typ}
	if out.Expr == "error" {
		out.Kind = GoValueError
		return out
	}
	if named, ok := typ.(*types.Named); ok {
		underlying := goValueTypeSeen(named.Underlying(), seen)
		underlying.Expr = goTypeExpr(typ)
		underlying.Named = true
		underlying.Name = named.Obj().Name()
		underlying.TypeParams = goTypeParamCount(typ)
		underlying.Type = typ
		if pkg := named.Obj().Pkg(); pkg != nil {
			underlying.ImportPath = pkg.Path()
			underlying.Package = pkg.Name()
		}
		return underlying
	}
	if alias, ok := typ.(*types.Alias); ok {
		underlying := goValueTypeSeen(alias.Rhs(), seen)
		aliasTypeParams := goTypeParamCount(typ)
		if alias.Obj().Pkg() == nil || aliasTypeParams == 0 && underlying.TypeParams == 0 {
			return underlying
		}
		underlying.Expr = goTypeExpr(typ)
		underlying.Named = true
		underlying.Name = alias.Obj().Name()
		underlying.TypeParams = max(aliasTypeParams, underlying.TypeParams)
		underlying.Type = typ
		if pkg := alias.Obj().Pkg(); pkg != nil {
			underlying.ImportPath = pkg.Path()
			underlying.Package = pkg.Name()
		}
		return underlying
	}
	switch underlying := typ.Underlying().(type) {
	case *types.Basic:
		switch underlying.Kind() {
		case types.Bool, types.UntypedBool:
			out.Kind = GoValueBool
		case types.String, types.UntypedString:
			out.Kind = GoValueString
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
			out.Kind = GoValueInt
			out.Bits = basicIntBits(underlying.Kind())
		case types.UntypedInt:
			out.Kind = GoValueInt
		case types.UntypedRune:
			out.Kind = GoValueInt
			out.Bits = 32
		case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
			out.Kind = GoValueUint
			out.Bits = basicIntBits(underlying.Kind())
		case types.Float32:
			out.Kind = GoValueFloat
			out.Bits = 32
		case types.Float64, types.UntypedFloat:
			out.Kind = GoValueFloat
			out.Bits = 64
		}
	case *types.Pointer:
		elem := goValueTypeSeen(underlying.Elem(), seen)
		out.Kind = GoValuePointer
		out.Elem = &elem
	case *types.Slice:
		elem := goValueTypeSeen(underlying.Elem(), seen)
		out.Kind = GoValueSlice
		out.Elem = &elem
	case *types.Map:
		key := goValueTypeSeen(underlying.Key(), seen)
		value := goValueTypeSeen(underlying.Elem(), seen)
		out.Kind = GoValueMap
		out.Key = &key
		out.Value = &value
	case *types.Interface:
		if underlying.Empty() {
			out.Kind = GoValueAny
		}
	case *types.Signature:
		out.Kind = GoValueFunc
		sig := goSignature(underlying)
		out.Func = &sig
	case *types.Chan:
		elem := goValueTypeSeen(underlying.Elem(), seen)
		out.Kind = GoValueChan
		out.Elem = &elem
		out.ChanDir = underlying.Dir()
	}
	return out
}

func goOpaqueType(typ types.Type) GoValueType {
	out := GoValueType{Expr: goTypeExpr(typ), Kind: GoValueOther, Type: typ}
	switch typ := typ.(type) {
	case *types.Named:
		out.Named = true
		out.Name = typ.Obj().Name()
		out.TypeParams = goTypeParamCount(typ)
		if pkg := typ.Obj().Pkg(); pkg != nil {
			out.ImportPath = pkg.Path()
			out.Package = pkg.Name()
		}
	case *types.Alias:
		typeParams := goTypeParamCount(typ)
		if typ.Obj().Pkg() == nil || typeParams == 0 {
			return out
		}
		out.Named = true
		out.Name = typ.Obj().Name()
		out.TypeParams = typeParams
		if pkg := typ.Obj().Pkg(); pkg != nil {
			out.ImportPath = pkg.Path()
			out.Package = pkg.Name()
		}
	}
	return out
}

func goTypeExpr(typ types.Type) string {
	if named, ok := typ.(*types.Named); ok && named.Obj() != nil {
		name := named.Obj().Name()
		if pkg := named.Obj().Pkg(); pkg != nil && pkg.Name() != "" {
			return pkg.Name() + "." + name
		}
		return name
	}
	if alias, ok := typ.(*types.Alias); ok && alias.Obj() != nil {
		name := alias.Obj().Name()
		if pkg := alias.Obj().Pkg(); pkg != nil && pkg.Name() != "" {
			return pkg.Name() + "." + name
		}
		return name
	}
	return types.TypeString(typ, packageQualifier)
}

func packageQualifier(pkg *types.Package) string {
	if pkg == nil {
		return ""
	}
	return pkg.Name()
}

func basicIntBits(kind types.BasicKind) int {
	switch kind {
	case types.Int8, types.Uint8:
		return 8
	case types.Int16, types.Uint16:
		return 16
	case types.Int32, types.Uint32:
		return 32
	case types.Int64, types.Uint64:
		return 64
	default:
		return 0
	}
}

func exportedStructFields(typ types.Type) (map[string]GoField, bool) {
	if typ == nil {
		return nil, false
	}
	underlying := typ.Underlying()
	structType, ok := underlying.(*types.Struct)
	if !ok {
		return nil, false
	}
	fields := map[string]GoField{}
	for i := 0; i < structType.NumFields(); i++ {
		field := structType.Field(i)
		if field == nil || !field.Exported() || field.Embedded() {
			continue
		}
		name := field.Name()
		fields[name] = GoField{Name: name, Type: goValueType(field.Type())}
	}
	return fields, true
}

func goTypeParamCount(typ types.Type) int {
	switch typ := typ.(type) {
	case *types.Named:
		if typ.TypeParams() != nil && typ.TypeParams().Len() > 0 {
			return typ.TypeParams().Len()
		}
		if typ.TypeArgs() != nil {
			return typ.TypeArgs().Len()
		}
	case *types.Alias:
		if typ.TypeParams() != nil && typ.TypeParams().Len() > 0 {
			return typ.TypeParams().Len()
		}
		if typ.TypeArgs() != nil {
			return typ.TypeArgs().Len()
		}
	}
	return 0
}

func exportedMethods(typ types.Type) map[string]GoMethod {
	valueMethods, pointerMethods := exportedMethodSets(typ)
	return mergeGoMethods(valueMethods, pointerMethods)
}

func exportedMethodSets(typ types.Type) (map[string]GoMethod, map[string]GoMethod) {
	return exportedMethodSet(types.NewMethodSet(typ)), exportedMethodSet(types.NewMethodSet(types.NewPointer(typ)))
}

func hasUnexportedMethods(typ types.Type) bool {
	methodSet := types.NewMethodSet(typ)
	for i := 0; i < methodSet.Len(); i++ {
		selection := methodSet.At(i)
		if selection == nil {
			continue
		}
		fn, ok := selection.Obj().(*types.Func)
		if ok && !fn.Exported() {
			return true
		}
	}
	return false
}

func exportedMethodSet(methodSet *types.MethodSet) map[string]GoMethod {
	methods := map[string]GoMethod{}
	if methodSet == nil {
		return methods
	}
	for i := 0; i < methodSet.Len(); i++ {
		selection := methodSet.At(i)
		if selection == nil || selection.Obj() == nil || !selection.Obj().Exported() {
			continue
		}
		// A promoted method's signature keeps the embedded type as its receiver,
		// while Ard names the outer type in bindings like pkg::Outer::Method.
		// Skip promoted methods until direct Go FFI can model promotion explicitly.
		if len(selection.Index()) != 1 {
			continue
		}
		name := selection.Obj().Name()
		methods[name] = GoMethod{Name: name, Signature: goSignature(selection.Obj().Type())}
	}
	return methods
}

func mergeGoMethods(valueMethods map[string]GoMethod, pointerMethods map[string]GoMethod) map[string]GoMethod {
	methods := map[string]GoMethod{}
	for name, method := range valueMethods {
		methods[name] = method
	}
	for name, method := range pointerMethods {
		methods[name] = method
	}
	return methods
}

func isGoInterfaceType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	_, ok := typ.Underlying().(*types.Interface)
	return ok
}
