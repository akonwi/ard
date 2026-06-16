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
	Name string
}

type GoMethod struct {
	Name string
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
			out.Functions[name] = GoFunction{Name: name}
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
			methods[name] = GoMethod{Name: name}
		}
	}
	addMethods(types.NewMethodSet(typ))
	addMethods(types.NewMethodSet(types.NewPointer(typ)))
	return methods
}
