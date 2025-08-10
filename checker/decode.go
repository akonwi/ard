package checker

// Error struct definition for the decode library
// Will be referenced as decode::Error in Ard code
var DecodeErrorDef = &StructDef{
	Name: "Error",
	Fields: map[string]Type{
		"expected": Str,
		"found":    Str,
		"path":     MakeList(Str),
	},
	Methods: map[string]*FunctionDef{
		"to_str": &FunctionDef{
			Name:       "to_str",
			Parameters: []Parameter{},
			ReturnType: Str,
		},
	},
	Traits: []*Trait{
		strMod.symbols["ToString"].Type.(*Trait),
	},
}

// Dynamic type for external/untyped data
type dynamicType struct{}

func (d dynamicType) String() string       { return "Dynamic" }
func (d dynamicType) get(name string) Type { return nil }
func (d dynamicType) equal(other Type) bool {
	_, ok := other.(*dynamicType)
	return ok
}
func (d dynamicType) hasTrait(trait *Trait) bool { return false }

var Dynamic = &dynamicType{}

type DecodePkg struct {
}

func (pkg DecodePkg) Path() string {
	return "ard/decode"
}

func (pkg DecodePkg) Program() *Program {
	return nil
}

func (pkg DecodePkg) Get(name string) Symbol {
	switch name {
	case "Error":
		return Symbol{Name: name, Type: DecodeErrorDef}
	case "Dynamic":
		return Symbol{Name: name, Type: Dynamic}
	case "as_string":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(Str, MakeList(DecodeErrorDef)),
		}}
	case "as_int":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(Int, MakeList(DecodeErrorDef)),
		}}
	case "as_float":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(Float, MakeList(DecodeErrorDef)),
		}}
	case "as_bool":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(Bool, MakeList(DecodeErrorDef)),
		}}
	case "string":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       "Decoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(Str, MakeList(DecodeErrorDef)),
		}}
	case "int":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       "Decoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(Int, MakeList(DecodeErrorDef)),
		}}
	case "float":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       "Decoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(Float, MakeList(DecodeErrorDef)),
		}}
	case "bool":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       "Decoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(Bool, MakeList(DecodeErrorDef)),
		}}
	case "run":
		// Create a generic type parameter for the decoder's return type
		genericT := &Any{name: "T"}
		decoderType := &FunctionDef{
			Name:       "Decoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(genericT, MakeList(DecodeErrorDef)),
		}

		return Symbol{Name: name, Type: &FunctionDef{
			Name: name,
			Parameters: []Parameter{
				{Name: "data", Type: Dynamic},        // Data comes first
				{Name: "decoder", Type: decoderType}, // Decoder comes second
			},
			ReturnType: MakeResult(genericT, MakeList(DecodeErrorDef)), // Same T
		}}
	case "any":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "external_data", Type: Str}},
			ReturnType: Dynamic,
		}}
	case "nullable":
		// Create generic type parameters
		innerT := &Any{name: "T"}
		innerDecoder := &FunctionDef{
			Name:       "Decoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(innerT, MakeList(DecodeErrorDef)),
		}
		// Nullable decoder returns Maybe<T>
		maybeT := MakeMaybe(innerT)
		nullableDecoder := &FunctionDef{
			Name:       "NullableDecoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(maybeT, MakeList(DecodeErrorDef)),
		}

		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "as", Type: innerDecoder}},
			ReturnType: nullableDecoder,
		}}
	case "list":
		// Create generic type parameters
		innerT := &Any{name: "T"}
		innerDecoder := &FunctionDef{
			Name:       "Decoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(innerT, MakeList(DecodeErrorDef)),
		}
		// List decoder returns [T]
		listT := MakeList(innerT)
		listDecoder := &FunctionDef{
			Name:       "ListDecoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(listT, MakeList(DecodeErrorDef)),
		}

		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "as", Type: innerDecoder}},
			ReturnType: listDecoder,
		}}
	case "map":
		// Create generic type parameters
		keyT := &Any{name: "K"}
		valueT := &Any{name: "V"}
		keyDecoder := &FunctionDef{
			Name:       "Decoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(keyT, MakeList(DecodeErrorDef)),
		}
		valueDecoder := &FunctionDef{
			Name:       "Decoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(valueT, MakeList(DecodeErrorDef)),
		}
		// Map decoder returns [K:V] (map with K keys and V values)
		mapT := MakeMap(keyT, valueT)
		mapDecoder := &FunctionDef{
			Name:       "MapDecoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(mapT, MakeList(DecodeErrorDef)),
		}

		return Symbol{Name: name, Type: &FunctionDef{
			Name: name,
			Parameters: []Parameter{
				{Name: "key", Type: keyDecoder},
				{Name: "val", Type: valueDecoder},
			},
			ReturnType: mapDecoder,
		}}
	case "field":
		// Create generic type parameter for field value
		innerT := &Any{name: "T"}
		valueDecoder := &FunctionDef{
			Name:       "Decoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(innerT, MakeList(DecodeErrorDef)),
		}

		// Field decoder returns same type as inner value decoder
		fieldDecoder := &FunctionDef{
			Name:       "FieldDecoder",
			Parameters: []Parameter{{Name: "data", Type: Dynamic}},
			ReturnType: MakeResult(innerT, MakeList(DecodeErrorDef)),
		}

		return Symbol{Name: name, Type: &FunctionDef{
			Name: name,
			Parameters: []Parameter{
				{Name: "key", Type: Str},
				{Name: "as", Type: valueDecoder},
			},
			ReturnType: fieldDecoder,
		}}
	default:
		return Symbol{}
	}
}
