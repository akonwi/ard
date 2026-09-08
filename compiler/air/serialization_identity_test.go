package air_test

import (
	"reflect"
	"testing"

	"github.com/akonwi/ard/air"
)

func TestSerializeProgramPreservesCanonicalTypeIdentityMetadata(t *testing.T) {
	program := &air.Program{
		Types: []air.TypeInfo{
			{ID: 1, Kind: air.TypeInt, Name: "Int"},
			{ID: 2, Kind: air.TypeParam, Name: "T", ParamOwner: "genericdef:example:Box", ParamIndex: 0},
			{
				ID: 3, Kind: air.TypeForeignType, Name: "pkg::Box<Int>",
				ForeignTarget: "go", ForeignNamespace: "example/pkg", ForeignQualifier: "pkg", ForeignSymbol: "Box",
				GenericArgs: []air.TypeID{1}, GenericComparable: []bool{true},
			},
		},
		Functions: []air.Function{{ID: 0, Name: "identity", TypeParams: []string{"T"}, TypeParamOwner: "genericfunction:0:identity"}},
	}
	data, err := air.SerializeProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := air.DeserializeProgram(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Types, program.Types) || !reflect.DeepEqual(decoded.Functions, program.Functions) {
		t.Fatalf("identity metadata changed after round trip:\n got %#v\nwant %#v", decoded, program)
	}
}
