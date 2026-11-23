package checker

import (
	"sort"
	"strings"
)

// ImportPath returns the path the root package writes in a `use` declaration
// to import the module with the given canonical module path, the inverse of
// import resolution from the root package.
//
// Modules of a dependency package have canonical paths of the form
// `_pkg_<hash>/<module path in package>` (see PackageModulePrefix), which are
// independent of the alias any importer chooses. ImportPath maps such a path
// to the root's alias for the package, e.g. `ui/shapes`, and the package's
// main module, whose path is the package name, to the bare alias. When the
// root declares several aliases for one package, the alphabetically first is
// used. A dependency package the root does not declare directly, such as a
// transitive one, cannot be imported from the root, and ImportPath reports
// false.
//
// Other paths, such as root package and standard library modules, are already
// importable and are returned unchanged.
func (p *ProjectInfo) ImportPath(modulePath string) (string, bool) {
	if p == nil {
		return modulePath, true
	}
	first, rest, _ := strings.Cut(modulePath, "/")
	aliases := make([]string, 0, len(p.Dependencies))
	for alias := range p.Dependencies {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		dep := p.Dependencies[alias]
		packageID := dep.PackageID
		if packageID == "" {
			packageID = dep.Alias
		}
		if PackageModulePrefix(packageID) != first {
			continue
		}
		if rest == "" || rest == dependencyMainModule(dep) {
			return alias, true
		}
		return alias + "/" + rest, true
	}
	for packageID := range p.Packages {
		if packageID != p.RootPackageID && PackageModulePrefix(packageID) == first {
			return "", false
		}
	}
	return modulePath, true
}

// dependencyMainModule is the module path a bare `use <alias>` resolves to
// within the dependency, matching ModuleResolver.ResolveImport.
func dependencyMainModule(dep DependencyInfo) string {
	if dep.Name != "" {
		return dep.Name
	}
	return dep.Alias
}
