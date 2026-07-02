package checker

import (
	"fmt"
	"strings"

	"golang.org/x/tools/go/packages"
)

type GoPackagesResolver struct {
	ProjectRoot string
	BuildTags   []string
	cache       map[string]goPackageResolveResult
}

type goPackageResolveResult struct {
	pkg *GoPackage
	err error
}

func NewGoPackagesResolver(projectRoot string, buildTags []string) *GoPackagesResolver {
	return &GoPackagesResolver{ProjectRoot: projectRoot, BuildTags: append([]string(nil), buildTags...), cache: map[string]goPackageResolveResult{}}
}

func (r *GoPackagesResolver) ResolveGoPackage(path string) (*GoPackage, error) {
	if r.cache == nil {
		r.cache = map[string]goPackageResolveResult{}
	}
	if cached, ok := r.cache[path]; ok {
		return cached.pkg, cached.err
	}
	pkg, err := r.load(path)
	r.cache[path] = goPackageResolveResult{pkg: pkg, err: err}
	return pkg, err
}

func (r *GoPackagesResolver) load(path string) (*GoPackage, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps,
		Dir:   r.ProjectRoot,
		Tests: false,
	}
	if len(r.BuildTags) > 0 {
		cfg.BuildFlags = []string{"-tags=" + strings.Join(r.BuildTags, ",")}
	}
	loaded, err := packages.Load(cfg, path)
	if err != nil {
		return nil, err
	}
	if len(loaded) == 0 {
		return nil, fmt.Errorf("package not found")
	}
	pkg := loaded[0]
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("resolve Go package %q: %s", path, pkg.Errors[0].Msg)
	}
	if pkg.Types == nil {
		return nil, fmt.Errorf("package has no type information")
	}
	return goPackageFromTypesPackage(path, pkg.Types), nil
}
