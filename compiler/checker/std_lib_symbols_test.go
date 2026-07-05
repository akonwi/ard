package checker

import "testing"

// TestBuiltinPkgSymbolsMatchGet asserts every name listed in BuiltinPkgNames
// resolves through the package's Get, and every Symbols entry is non-zero —
// guarding drift between the Get switches and the shared name lists.
func TestBuiltinPkgSymbolsMatchGet(t *testing.T) {
	pkgs := []Module{MaybePkg{}, ResultPkg{}, AsyncPkg{}, UnsafePkg{}, ChannelStaticPkg{}}
	for _, pkg := range pkgs {
		names, ok := BuiltinPkgNames[pkg.Path()]
		if !ok {
			t.Fatalf("%s missing from BuiltinPkgNames", pkg.Path())
		}
		symbols := pkg.Symbols()
		if len(symbols) != len(names) {
			t.Fatalf("%s: Symbols has %d entries, name list has %d", pkg.Path(), len(symbols), len(names))
		}
		for _, name := range names {
			if pkg.Get(name).IsZero() {
				t.Fatalf("%s: %q listed but Get returns zero symbol", pkg.Path(), name)
			}
			if _, ok := symbols[name]; !ok {
				t.Fatalf("%s: %q missing from Symbols", pkg.Path(), name)
			}
		}
	}
}
