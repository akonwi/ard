package gotarget

import (
	"reflect"
	"testing"

	"github.com/akonwi/ard/air"
)

func TestIndexedTypesForModulePreservesUncachedOrder(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{{ID: 0, Path: "main.ard", Types: []air.TypeID{4}}},
		Types: []air.TypeInfo{
			{ID: 1, Kind: air.TypeStruct, Name: "FirstOwned", ModulePath: "main.ard"},
			{ID: 2, Kind: air.TypeStruct, Name: "Ownerless"},
			{ID: 3, Kind: air.TypeStruct, Name: "SecondOwned", ModulePath: "main.ard"},
			{ID: 4, Kind: air.TypeStruct, Name: "Declared"},
		},
	}

	uncached := (&lowerer{program: program}).typesForModule(0, 0)
	indexedLowerer := &lowerer{program: program}
	indexedLowerer.indexModuleOwnership()
	indexed := indexedLowerer.typesForModule(0, 0)

	ids := func(types []*air.TypeInfo) []air.TypeID {
		out := make([]air.TypeID, len(types))
		for i, typ := range types {
			out[i] = typ.ID
		}
		return out
	}
	if got, want := ids(indexed), ids(uncached); !reflect.DeepEqual(got, want) {
		t.Fatalf("indexed type order = %v, want uncached order %v", got, want)
	}
}
