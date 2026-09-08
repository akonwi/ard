package air

import (
	"errors"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestNominalInternerTracksLifecycle(t *testing.T) {
	lowerer := newLowerer(LowerOptions{}, 0)
	key := declarationNominalKey(TypeStruct, "example", "Node")
	seed := TypeInfo{Kind: TypeStruct, Name: "Node", ModulePath: "example"}

	id, build, err := lowerer.typeInterner.reserveNominal(key, seed)
	if err != nil || !build {
		t.Fatalf("reserve = (%d, %t, %v), want new reservation", id, build, err)
	}
	if got := lowerer.program.Types[id-1].Kind; got != TypeStruct {
		t.Fatalf("reserved kind = %v, want TypeStruct", got)
	}
	if _, available := lowerer.typeInfo(id); available {
		t.Fatal("building nominal metadata was exposed through typeInfo")
	}
	if err := lowerer.typeInterner.validateComplete(); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("validate building reservation = %v, want incomplete error", err)
	}

	info := seed
	info.Fields = []FieldInfo{{Name: "value", Type: lowerer.mustIntern(checker.Int)}}
	if completed, err := lowerer.typeInterner.completeNominal(key, info); err != nil || completed != id {
		t.Fatalf("complete = (%d, %v), want %d", completed, err, id)
	}
	if err := lowerer.typeInterner.validateComplete(); err != nil {
		t.Fatalf("validate complete reservation: %v", err)
	}

	conflict := info
	conflict.Private = true
	if _, err := lowerer.typeInterner.completeNominal(key, conflict); err == nil || !strings.Contains(err.Error(), "conflicting AIR metadata") {
		t.Fatalf("conflicting completion error = %v", err)
	}
}

func TestNominalInternerRemembersFailure(t *testing.T) {
	lowerer := newLowerer(LowerOptions{}, 0)
	key := declarationNominalKey(TypeStruct, "example", "Broken")
	seed := TypeInfo{Kind: TypeStruct, Name: "Broken", ModulePath: "example"}
	id, build, err := lowerer.typeInterner.reserveNominal(key, seed)
	if err != nil || !build {
		t.Fatalf("reserve = (%d, %t, %v)", id, build, err)
	}
	failure := errors.New("field lowering failed")
	lowerer.typeInterner.failNominal(key, failure)

	if got, build, err := lowerer.typeInterner.reserveNominal(key, seed); got != NoType || build || !errors.Is(err, failure) {
		t.Fatalf("repeat reserve = (%d, %t, %v), want remembered failure", got, build, err)
	}
	if err := lowerer.typeInterner.validateComplete(); !errors.Is(err, failure) {
		t.Fatalf("validate failed reservation = %v, want %v", err, failure)
	}
}

func TestNominalStructFailureCannotReturnReservedID(t *testing.T) {
	lowerer := newLowerer(LowerOptions{}, 0)
	broken := &checker.StructDef{
		Name:       "Broken",
		ModulePath: "example",
		Fields:     map[string]checker.Type{"value": &checker.TypeVar{}},
	}
	for attempt := 0; attempt < 2; attempt++ {
		id, err := lowerer.internNominalStruct(broken, lowerer.internType)
		if id != NoType || err == nil {
			t.Fatalf("attempt %d = (%d, %v), want remembered failure", attempt, id, err)
		}
	}
	key := declarationNominalKey(TypeStruct, "example", "Broken")
	if entry := lowerer.typeInterner.nominal[key]; entry == nil || entry.state != nominalFailed {
		t.Fatalf("nominal entry = %#v, want failed", entry)
	}
}

func TestRecursiveGenericApplicationNamesUseReservedNominalMetadata(t *testing.T) {
	program := lowerSource(t, `
		struct A<$T> { b: B<A<$T>>? }
		struct B<$T> { a: $T? }
		fn main() { let value: A<Int>? = Maybe::new() }
	`)
	for _, typ := range program.Types {
		if strings.Contains(typ.Name, "<invalid:") {
			t.Fatalf("completed type %d has incomplete display name %q", typ.ID, typ.Name)
		}
	}
}

func TestGenericApplicationRecoversCanonicalDefinitionOwner(t *testing.T) {
	definition := &checker.StructDef{Name: "Box", ModulePath: "dependency", GenericParams: []string{"T"}, Fields: map[string]checker.Type{}}
	dependency := &countingMethodModule{
		path:    "dependency",
		program: &checker.Program{Statements: []checker.Statement{{Stmt: definition}}},
	}
	lowerer := newLowerer(LowerOptions{}, 1)
	lowerer.moduleByName[dependency.path] = dependency

	ownerless := &checker.StructDef{Name: "Box", TypeArgs: []checker.Type{checker.Int}, Fields: map[string]checker.Type{}}
	canonical := &checker.StructDef{Name: "Box", ModulePath: "dependency", TypeArgs: []checker.Type{checker.Int}, Fields: map[string]checker.Type{}}
	first, err := lowerer.internStructApplicationWithInterner(ownerless, lowerer.internType)
	if err != nil {
		t.Fatal(err)
	}
	second, err := lowerer.internStructApplicationWithInterner(canonical, lowerer.internType)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("ownerless/canonical applications = %d/%d, want one identity", first, second)
	}
	info := lowerer.program.Types[first-1]
	if info.ModulePath != "dependency" || info.Generic == NoType {
		t.Fatalf("application metadata = %#v, want canonical dependency definition", info)
	}
}

func TestEnumNominalIdentityRejectsConflictingMetadata(t *testing.T) {
	lowerer := newLowerer(LowerOptions{}, 0)
	first := &checker.Enum{Name: "Status", ModulePath: "example", Values: []checker.EnumValue{{Name: "Ready", Value: 1}}}
	copy := &checker.Enum{Name: "Status", ModulePath: "example", Values: []checker.EnumValue{{Name: "Ready", Value: 1}}}
	conflict := &checker.Enum{Name: "Status", ModulePath: "example", Values: []checker.EnumValue{{Name: "Ready", Value: 2}}}

	firstID, err := lowerer.internType(first)
	if err != nil {
		t.Fatal(err)
	}
	copyID, err := lowerer.internType(copy)
	if err != nil {
		t.Fatal(err)
	}
	if firstID != copyID {
		t.Fatalf("enum declaration copies = %d/%d, want one identity", firstID, copyID)
	}
	if _, err := lowerer.internType(conflict); err == nil || !strings.Contains(err.Error(), "conflicting AIR metadata") {
		t.Fatalf("conflicting enum metadata error = %v", err)
	}
}

func TestForeignNominalIdentityUsesCanonicalNamespaceNotQualifier(t *testing.T) {
	lowerer := newLowerer(LowerOptions{}, 0)
	first := &checker.ForeignType{Target: "go", Namespace: "example.com/project/pkg", Qualifier: "first", Name: "Token"}
	alias := &checker.ForeignType{Target: "go", Namespace: "example.com/project/pkg", Qualifier: "alias", Name: "Token"}
	other := &checker.ForeignType{Target: "go", Namespace: "example.com/other/pkg", Qualifier: "first", Name: "Token"}

	firstID, err := lowerer.internType(first)
	if err != nil {
		t.Fatal(err)
	}
	aliasID, err := lowerer.internType(alias)
	if err != nil {
		t.Fatal(err)
	}
	otherID, err := lowerer.internType(other)
	if err != nil {
		t.Fatal(err)
	}
	if firstID != aliasID {
		t.Fatalf("foreign aliases = %d/%d, want one canonical identity", firstID, aliasID)
	}
	if firstID == otherID {
		t.Fatalf("foreign types in distinct namespaces share AIR type %d", firstID)
	}
}

func TestValidateRejectsDuplicateNominalAndTypeParameterIdentity(t *testing.T) {
	tests := []struct {
		name  string
		types []TypeInfo
	}{
		{
			name: "enum",
			types: []TypeInfo{
				{ID: 1, Kind: TypeEnum, Name: "Status", ModulePath: "example"},
				{ID: 2, Kind: TypeEnum, Name: "Status", ModulePath: "example"},
			},
		},
		{
			name: "union",
			types: []TypeInfo{
				{ID: 1, Kind: TypeUnion, Name: "Choice", ModulePath: "example"},
				{ID: 2, Kind: TypeUnion, Name: "Choice", ModulePath: "example"},
			},
		},
		{
			name: "type parameter",
			types: []TypeInfo{
				{ID: 1, Kind: TypeParam, Name: "T", ParamOwner: "owner", ParamIndex: 0},
				{ID: 2, Kind: TypeParam, Name: "Renamed", ParamOwner: "owner", ParamIndex: 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Validate(&Program{Types: tt.types}); err == nil || !strings.Contains(err.Error(), "duplicate") {
				t.Fatalf("Validate error = %v, want duplicate identity rejection", err)
			}
		})
	}
}

func TestTypeParameterEquivalenceIncludesOwner(t *testing.T) {
	program := Program{Types: []TypeInfo{
		{ID: 1, Kind: TypeParam, Name: "T", ParamOwner: "first", ParamIndex: 0},
		{ID: 2, Kind: TypeParam, Name: "T", ParamOwner: "second", ParamIndex: 0},
		{ID: 3, Kind: TypeParam, Name: "Renamed", ParamOwner: "first", ParamIndex: 0},
	}}
	if typesStructurallyEquivalent(&program, 1, 2, map[[2]TypeID]bool{}) {
		t.Fatal("type parameters from different owners compare equivalent")
	}
	if !typesStructurallyEquivalent(&program, 1, 3, map[[2]TypeID]bool{}) {
		t.Fatal("type parameter display name changed owner-scoped identity")
	}
}
