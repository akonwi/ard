package checker

import (
	"fmt"
	"go/constant"
	gotoken "go/token"
	"go/types"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/akonwi/ard/parse"
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
	Elem       *GoValueType
	Key        *GoValueType
	Value      *GoValueType
}

type GoType struct {
	Name          string
	Methods       map[string]GoMethod
	Fields        map[string]GoField
	EnumConstants []GoConstant
	ClosedEnum    bool
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
	sharedCacheKey, useSharedCache := stdlibGoPackageCacheKey(r.Dir, importPath)
	if useSharedCache {
		if cached, ok := sharedStdlibGoPackageCache.Load(sharedCacheKey); ok {
			pkg := cached.(*GoPackage)
			r.cache[importPath] = pkg
			return pkg, nil
		}
	}
	cfg := &packages.Config{
		Dir:        r.Dir,
		Mode:       packages.NeedName | packages.NeedTypes,
		BuildFlags: []string{"-mod=readonly"},
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
			out.Types[name] = GoType{Name: name, Methods: exportedMethods(obj.Type()), Fields: exportedStructFields(obj.Type())}
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
	return &ExternType{Name_: ty.GetName(), ExternalBinding: binding, ExternalBindings: map[string]string{"go": binding}}
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

func (c *Checker) resolveDirectGoExternBinding(binding string, loc parse.Location) string {
	parts, ok := directGoBindingParts(binding)
	if !ok {
		return binding
	}
	if len(parts) != 2 && len(parts) != 3 {
		c.addError(fmt.Sprintf("Direct Go extern binding %q must be package::Function or package::Type::Method", binding), loc)
		return binding
	}
	goImport, ok := c.directGoImports[parts[0]]
	if !ok {
		c.addError(fmt.Sprintf("Unknown Go import alias %q in extern binding %q", parts[0], binding), loc)
		return binding
	}
	if goImport.pkg != nil {
		if len(parts) == 2 {
			if _, ok := goImport.pkg.Functions[parts[1]]; !ok {
				c.addError(fmt.Sprintf("Go package %q has no exported function %q", goImport.importPath, parts[1]), loc)
			}
		} else {
			typ, ok := goImport.pkg.Types[parts[1]]
			if !ok {
				c.addError(fmt.Sprintf("Go package %q has no exported type %q", goImport.importPath, parts[1]), loc)
			} else if _, ok := typ.Methods[parts[2]]; !ok {
				c.addError(fmt.Sprintf("Go type %q in package %q has no exported method %q", parts[1], goImport.importPath, parts[2]), loc)
			}
		}
	}
	return canonicalDirectGoBinding(goImport.importPath, goImport.alias, parts[1:])
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
	if ok, reason := c.directGoAssignableCompatible(subject.Type(), *target.Signature.Receiver); !ok {
		c.addError(fmt.Sprintf("receiver for %s: %s", target.Name, reason), loc)
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
		if ok, reason := c.directGoParamCompatible(checkedArg.Type(), goParams[i], true); !ok {
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
	case GoValueError:
		c.addError("Go error values require a Result adapter", loc)
		return nil, false
	}
	c.addError(fmt.Sprintf("Go type %s cannot be represented as an inferred Ard direct Go value type", goType.String()), loc)
	return nil, false
}

func (c *Checker) directGoNamedArdType(goType GoValueType, loc parse.Location) (Type, bool) {
	if goType.ImportPath == "" || goType.Name == "" {
		c.addError(fmt.Sprintf("Go named type %s is missing package metadata", goType.String()), loc)
		return nil, false
	}
	if goImport, ok := c.directGoImportForPath(goType.ImportPath); ok && goImport.pkg != nil {
		if typ, ok := goImport.pkg.Types[goType.Name]; ok {
			if enum := c.directGoEnumType(goImport, typ, loc); enum != nil {
				return enum, true
			}
		}
	}
	alias := ""
	if goImport, ok := c.directGoImportForPath(goType.ImportPath); ok {
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
	return &ExternType{Name_: name, ExternalBinding: binding, ExternalBindings: map[string]string{"go": binding}}, true
}

func (c *Checker) validateDirectGoExternSignature(name string, params []Parameter, returnType Type, binding string, loc parse.Location) {
	target, ok := c.directGoSignatureTarget(binding)
	if !ok {
		return
	}
	if target.Signature.Variadic {
		c.addError(fmt.Sprintf("Go function %s is variadic; variadic direct Go externs are not supported yet", target.Name), loc)
		return
	}
	expectedParams := len(target.Signature.Params)
	paramOffset := 0
	if target.Method {
		expectedParams++
		paramOffset = 1
	}
	if len(params) != expectedParams {
		c.addError(fmt.Sprintf("Go function %s expects %d parameter(s), extern %s declares %d", target.Name, expectedParams, name, len(params)), loc)
		return
	}
	if target.Method && target.Signature.Receiver != nil {
		if ok, reason := c.directGoAssignableCompatible(params[0].Type, *target.Signature.Receiver); !ok {
			c.addError(fmt.Sprintf("receiver for %s: %s", target.Name, reason), loc)
		}
	}
	for i, goParam := range target.Signature.Params {
		ardParam := params[i+paramOffset]
		if ok, reason := c.directGoParamCompatible(ardParam.Type, goParam, true); !ok {
			c.addError(fmt.Sprintf("parameter %d for %s: %s", i+1+paramOffset, target.Name, reason), loc)
		}
	}
	c.validateDirectGoExternReturn(name, returnType, target, loc)
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
	if directGoNamedTypeMatches(ard, goType) {
		return true, ""
	}
	if goType.Kind == GoValuePointer {
		if goType.Named {
			return false, fmt.Sprintf("Go named pointer type %s is not supported by direct Go pointer bindings yet", goType.String())
		}
		if directGoPointerToEnumLike(ard, goType) {
			return false, fmt.Sprintf("Go pointer to enum-like type %s is not supported by direct Go pointer bindings yet", goType.String())
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
	case GoValueError:
		return false, "Go error values require an adapter; direct Go error adapters are not supported yet"
	}
	return false, fmt.Sprintf("Ard type %s is not compatible with Go type %s", typeSyntaxString(ard), goType.String())
}

func (c *Checker) directGoAssignableCompatible(ard Type, goType GoValueType) (bool, string) {
	ard = derefType(ard)
	if directGoNamedTypeMatches(ard, goType) {
		return true, ""
	}
	if goType.Kind == GoValuePointer {
		if goType.Named {
			return false, fmt.Sprintf("Go named pointer type %s is not supported by direct Go pointer bindings yet", goType.String())
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
			if compatible, reason := c.directGoAssignableCompatible(list.Of(), *goType.Elem); compatible {
				return true, ""
			} else {
				return false, "list element: " + reason
			}
		}
	case GoValueMap:
		m, ok := ard.(*Map)
		if ok && goType.Key != nil && goType.Value != nil {
			if compatible, reason := c.directGoAssignableCompatible(m.Key(), *goType.Key); !compatible {
				return false, "map key: " + reason
			}
			if compatible, reason := c.directGoAssignableCompatible(m.Value(), *goType.Value); !compatible {
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

	out := GoValueType{Expr: goTypeExpr(typ), Kind: GoValueOther}
	if out.Expr == "error" {
		out.Kind = GoValueError
		return out
	}
	if named, ok := typ.(*types.Named); ok {
		underlying := goValueTypeSeen(named.Underlying(), seen)
		underlying.Expr = goTypeExpr(typ)
		underlying.Named = true
		underlying.Name = named.Obj().Name()
		if pkg := named.Obj().Pkg(); pkg != nil {
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
	}
	return out
}

func goOpaqueType(typ types.Type) GoValueType {
	out := GoValueType{Expr: goTypeExpr(typ), Kind: GoValueOther}
	if named, ok := typ.(*types.Named); ok {
		out.Named = true
		out.Name = named.Obj().Name()
		if pkg := named.Obj().Pkg(); pkg != nil {
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

func exportedStructFields(typ types.Type) map[string]GoField {
	fields := map[string]GoField{}
	if typ == nil {
		return fields
	}
	underlying := typ.Underlying()
	structType, ok := underlying.(*types.Struct)
	if !ok {
		return fields
	}
	for i := 0; i < structType.NumFields(); i++ {
		field := structType.Field(i)
		if field == nil || !field.Exported() || field.Embedded() {
			continue
		}
		name := field.Name()
		fields[name] = GoField{Name: name, Type: goValueType(field.Type())}
	}
	return fields
}

func exportedMethods(typ types.Type) map[string]GoMethod {
	methods := map[string]GoMethod{}
	addMethods := func(methodSet *types.MethodSet) {
		if methodSet == nil {
			return
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
	}
	addMethods(types.NewMethodSet(typ))
	addMethods(types.NewMethodSet(types.NewPointer(typ)))
	return methods
}
