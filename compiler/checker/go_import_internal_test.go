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

// A named empty Go interface (`type Event interface{}`) keeps its type
// identity instead of collapsing to Any, so signatures naming it lower to the
// exact Go type. Only the unnamed empty interface maps to Any.
func TestTypeFromGoKeepsNamedEmptyInterfaceIdentity(t *testing.T) {
	source := types.NewPackage("example.com/vaxis", "vaxis")
	ifaceObj := types.NewTypeName(token.NoPos, source, "Event", nil)
	named := types.NewNamed(ifaceObj, types.NewInterfaceType(nil, nil), nil)

	resolved, reason := typeFromGo(named)
	if reason != "" {
		t.Fatalf("typeFromGo(named empty interface) reason = %q", reason)
	}
	foreign, ok := resolved.(*ForeignType)
	if !ok {
		t.Fatalf("typeFromGo = %T, want *ForeignType", resolved)
	}
	if !foreign.Interface || foreign.Name != "Event" || foreign.Qualifier != "vaxis" {
		t.Fatalf("named empty interface resolved to %#v, want vaxis::Event interface", foreign)
	}
	if !foreign.EmptyInterface() {
		t.Fatal("EmptyInterface() = false, want true")
	}

	if unnamed, reason := typeFromGo(types.NewInterfaceType(nil, nil)); reason != "" || unnamed != Any {
		t.Fatalf("unnamed empty interface = %v (%q), want Any", unnamed, reason)
	}
}

// A named Go func type (`type VoidCallback func(...)`) keeps its identity so
// generated Go names the exact type; its Underlying carries the signature.
func TestTypeFromGoKeepsNamedFuncIdentity(t *testing.T) {
	source := types.NewPackage("example.com/ui", "ui")
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewParam(token.NoPos, source, "v", types.Typ[types.String])),
		nil, false)
	fnObj := types.NewTypeName(token.NoPos, source, "Callback", nil)
	named := types.NewNamed(fnObj, sig, nil)

	resolved, reason := typeFromGo(named)
	if reason != "" {
		t.Fatalf("typeFromGo(named func) reason = %q", reason)
	}
	foreign, ok := resolved.(*ForeignType)
	if !ok {
		t.Fatalf("typeFromGo = %T, want *ForeignType", resolved)
	}
	if foreign.Name != "Callback" || foreign.Qualifier != "ui" {
		t.Fatalf("named func resolved to %s::%s, want ui::Callback", foreign.Qualifier, foreign.Name)
	}
	fn, ok := foreign.Underlying.(*FunctionDef)
	if !ok {
		t.Fatalf("Underlying = %T, want *FunctionDef", foreign.Underlying)
	}
	if len(fn.Parameters) != 1 || !fn.Parameters[0].Type.equal(Str) {
		t.Fatalf("signature parameters = %#v, want (Str)", fn.Parameters)
	}
}

func TestGoFieldsIncludeEmbeddedFieldWithoutPromotingChildren(t *testing.T) {
	baseFields := []*types.Var{
		types.NewField(token.NoPos, nil, "Name", types.Typ[types.String], false),
	}
	base := types.NewNamed(
		types.NewTypeName(token.NoPos, nil, "Base", nil),
		types.NewStruct(baseFields, nil),
		nil,
	)
	outer := types.NewStruct(
		[]*types.Var{types.NewField(token.NoPos, nil, "Base", base, true)},
		nil,
	)

	fields, unsupported := goFieldsForStruct(outer)
	if len(unsupported) != 0 {
		t.Fatalf("unsupported fields = %v, want none", unsupported)
	}
	if fields["Base"] == nil {
		t.Fatal("embedded field Base was not exposed")
	}
	if fields["Name"] != nil {
		t.Fatal("embedded child field Name was promoted")
	}
}
