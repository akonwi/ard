package checker

import (
	"fmt"
	"go/types"
	"strings"

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
	Params  []GoValueType
	Results []GoValueType
}

type GoValueKind string

const (
	GoValueInvalid GoValueKind = ""
	GoValueBool    GoValueKind = "bool"
	GoValueString  GoValueKind = "string"
	GoValueInt     GoValueKind = "int"
	GoValueUint    GoValueKind = "uint"
	GoValueFloat   GoValueKind = "float"
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
}

type GoType struct {
	Name    string
	Methods map[string]GoMethod
}

type GoConstant struct {
	Name string
}

type directGoImport struct {
	alias      string
	importPath string
	pkg        *GoPackage
}

type GoPackagesResolver struct {
	Dir   string
	cache map[string]*GoPackage
}

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
	return resolved, nil
}

func goPackageFromTypes(importPath string, name string, pkg *types.Package) *GoPackage {
	out := &GoPackage{
		ImportPath: importPath,
		Name:       name,
		Functions:  map[string]GoFunction{},
		Types:      map[string]GoType{},
		Constants:  map[string]GoConstant{},
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
			out.Types[name] = GoType{Name: name, Methods: exportedMethods(obj.Type())}
		case *types.Const:
			out.Constants[name] = GoConstant{Name: name}
		}
	}
	return out
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
	return canonicalDirectGoBinding(goImport.importPath, parts[1:])
}

func canonicalDirectGoBinding(importPath string, symbolParts []string) string {
	return "go:" + importPath + "::" + strings.Join(symbolParts, "::")
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

func goSignature(typ types.Type) GoSignature {
	sig, ok := typ.(*types.Signature)
	if !ok || sig == nil {
		return GoSignature{}
	}
	return GoSignature{Params: goTuple(sig.Params()), Results: goTuple(sig.Results())}
}

func goTuple(tuple *types.Tuple) []GoValueType {
	if tuple == nil || tuple.Len() == 0 {
		return nil
	}
	out := make([]GoValueType, tuple.Len())
	for i := 0; i < tuple.Len(); i++ {
		out[i] = goValueType(tuple.At(i).Type())
	}
	return out
}

func goValueType(typ types.Type) GoValueType {
	if typ == nil {
		return GoValueType{Kind: GoValueInvalid}
	}
	out := GoValueType{Expr: types.TypeString(typ, packageQualifier), Kind: GoValueOther}
	if out.Expr == "error" {
		out.Kind = GoValueError
		return out
	}
	if named, ok := typ.(*types.Named); ok {
		underlying := goValueType(named.Underlying())
		underlying.Expr = types.TypeString(typ, packageQualifier)
		underlying.Named = true
		underlying.Name = named.Obj().Name()
		if pkg := named.Obj().Pkg(); pkg != nil {
			underlying.ImportPath = pkg.Path()
			underlying.Package = pkg.Name()
		}
		return underlying
	}
	basic, ok := typ.Underlying().(*types.Basic)
	if !ok {
		return out
	}
	switch basic.Kind() {
	case types.Bool:
		out.Kind = GoValueBool
	case types.String:
		out.Kind = GoValueString
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
		out.Kind = GoValueInt
		out.Bits = basicIntBits(basic.Kind())
	case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
		out.Kind = GoValueUint
		out.Bits = basicIntBits(basic.Kind())
	case types.Float32:
		out.Kind = GoValueFloat
		out.Bits = 32
	case types.Float64:
		out.Kind = GoValueFloat
		out.Bits = 64
	}
	return out
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
			name := selection.Obj().Name()
			methods[name] = GoMethod{Name: name, Signature: goSignature(selection.Obj().Type())}
		}
	}
	addMethods(types.NewMethodSet(typ))
	addMethods(types.NewMethodSet(types.NewPointer(typ)))
	return methods
}
