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
}

// Dynamic type for external/untyped data
type dynamicType struct{}

func (d dynamicType) String() string { return "Dynamic" }
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
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: &FunctionDef{
				Name:       "Decoder",
				Parameters: []Parameter{{Name: "data", Type: Dynamic}},
				ReturnType: MakeResult(Str, MakeList(DecodeErrorDef)),
			},
		}}
	case "int":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: &FunctionDef{
				Name:       "Decoder",
				Parameters: []Parameter{{Name: "data", Type: Dynamic}},
				ReturnType: MakeResult(Int, MakeList(DecodeErrorDef)),
			},
		}}
	case "float":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: &FunctionDef{
				Name:       "Decoder",
				Parameters: []Parameter{{Name: "data", Type: Dynamic}},
				ReturnType: MakeResult(Float, MakeList(DecodeErrorDef)),
			},
		}}
	case "bool":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: &FunctionDef{
				Name:       "Decoder",
				Parameters: []Parameter{{Name: "data", Type: Dynamic}},
				ReturnType: MakeResult(Bool, MakeList(DecodeErrorDef)),
			},
		}}
	case "decode":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{
				{Name: "decoder", Type: &Any{name: "Decoder"}},
				{Name: "data", Type: Dynamic},
			},
			ReturnType: MakeResult(&Any{name: "T"}, MakeList(DecodeErrorDef)),
		}}
	case "any":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "external_data", Type: Str}},
			ReturnType: Dynamic,
		}}
	case "nullable":
		return Symbol{Name: name, Type: &FunctionDef{
			Name: name,
			Parameters: []Parameter{{Name: "as", Type: &Any{name: "Decoder"}}},
			ReturnType: &Any{name: "MaybeDecoder"},
		}}
	default:
		return Symbol{}
	}
}