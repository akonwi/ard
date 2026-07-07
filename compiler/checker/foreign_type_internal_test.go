package checker

import (
	"go/token"
	"go/types"
	"testing"
)

// TestTranslateGoInterfacePreservesMethodPackages pins that structural
// interface translation keeps each method's declaring package. go/types
// matches unexported names by package path, so dropping the package would
// make translated sealed interfaces unsatisfiable (or, in the reverse
// direction, spuriously satisfiable).
func TestTranslateGoInterfacePreservesMethodPackages(t *testing.T) {
	owner := types.NewPackage("example.com/sealed", "sealed")
	universe := types.NewPackage("example.com/universe", "universe")

	sig := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	sealedMethod := types.NewFunc(token.NoPos, owner, "sealed", sig)
	exportedMethod := types.NewFunc(token.NoPos, owner, "Do", sig)
	iface := types.NewInterfaceType([]*types.Func{sealedMethod, exportedMethod}, nil)
	iface.Complete()

	translated, ok := translateGoType(iface, universe)
	if !ok {
		t.Fatal("translateGoType failed for a basic interface")
	}
	translatedIface, ok := translated.(*types.Interface)
	if !ok {
		t.Fatalf("translated type = %T, want *types.Interface", translated)
	}
	for i := 0; i < translatedIface.NumExplicitMethods(); i++ {
		method := translatedIface.ExplicitMethod(i)
		if method.Pkg() != owner {
			t.Fatalf("method %s package = %v, want %v", method.Name(), method.Pkg(), owner)
		}
	}
}

// TestTranslateGoStructPreservesFieldPackages pins the same package
// preservation for structural struct translation.
func TestTranslateGoStructPreservesFieldPackages(t *testing.T) {
	owner := types.NewPackage("example.com/owner", "owner")
	universe := types.NewPackage("example.com/universe", "universe")

	field := types.NewField(token.NoPos, owner, "secret", types.Typ[types.Int], false)
	structType := types.NewStruct([]*types.Var{field}, []string{""})

	translated, ok := translateGoType(structType, universe)
	if !ok {
		t.Fatal("translateGoType failed for a basic struct")
	}
	translatedStruct, ok := translated.(*types.Struct)
	if !ok {
		t.Fatalf("translated type = %T, want *types.Struct", translated)
	}
	if got := translatedStruct.Field(0).Pkg(); got != owner {
		t.Fatalf("field package = %v, want %v", got, owner)
	}
}
