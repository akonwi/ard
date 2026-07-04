package checker

import (
	"go/token"
	"go/types"
	"testing"
)

func TestConstTypeFromGoRejectsUnsupportedConstantType(t *testing.T) {
	if _, reason := constTypeFromGo(types.Typ[types.UntypedComplex]); reason == "" {
		t.Fatal("expected unsupported untyped complex constant reason")
	}
}

// A Go alias of a named type is the same type as its target, so it must
// resolve through the aliased type to keep type identity across packages
// (for example `ui.Style = vaxis.Style`).
func TestTypeFromGoResolvesNamedTypeAliases(t *testing.T) {
	source := types.NewPackage("example.com/vaxis", "vaxis")
	structType := types.NewStruct(nil, nil)
	namedObj := types.NewTypeName(token.NoPos, source, "Style", nil)
	named := types.NewNamed(namedObj, structType, nil)

	aliasing := types.NewPackage("example.com/vaxis/ui", "ui")
	aliasObj := types.NewTypeName(token.NoPos, aliasing, "Style", nil)
	alias := types.NewAlias(aliasObj, named)

	resolved, reason := typeFromGo(alias)
	if reason != "" {
		t.Fatalf("typeFromGo(alias) reason = %q", reason)
	}
	foreign, ok := resolved.(*ForeignType)
	if !ok {
		t.Fatalf("typeFromGo(alias) = %T, want *ForeignType", resolved)
	}
	if foreign.Namespace != "example.com/vaxis" || foreign.Qualifier != "vaxis" || foreign.Name != "Style" {
		t.Fatalf("alias resolved to %s::%s (namespace %s), want vaxis::Style", foreign.Qualifier, foreign.Name, foreign.Namespace)
	}

	direct, reason := typeFromGo(named)
	if reason != "" {
		t.Fatalf("typeFromGo(named) reason = %q", reason)
	}
	if !resolved.equal(direct) {
		t.Fatal("alias and aliased type should be the same Ard type")
	}
}

// An alias of a bare basic type keeps its own named identity, matching the
// pre-existing scalar alias behavior.
func TestTypeFromGoKeepsBasicAliasIdentity(t *testing.T) {
	source := types.NewPackage("example.com/pkg", "pkg")
	aliasObj := types.NewTypeName(token.NoPos, source, "Name", nil)
	alias := types.NewAlias(aliasObj, types.Typ[types.String])

	resolved, reason := typeFromGo(alias)
	if reason != "" {
		t.Fatalf("typeFromGo(alias) reason = %q", reason)
	}
	foreign, ok := resolved.(*ForeignType)
	if !ok {
		t.Fatalf("typeFromGo(alias) = %T, want *ForeignType", resolved)
	}
	if foreign.Qualifier != "pkg" || foreign.Name != "Name" {
		t.Fatalf("basic alias resolved to %s::%s, want pkg::Name", foreign.Qualifier, foreign.Name)
	}
	if !foreign.Underlying.equal(Str) {
		t.Fatalf("basic alias underlying = %s, want Str", foreign.Underlying)
	}
}
