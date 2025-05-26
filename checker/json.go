package checker

type JsonPkg struct {
}

func (pkg JsonPkg) Path() string {
	return "ard/json"
}
func (pkg JsonPkg) buildScope(scope *scope) {
}
func (pkg JsonPkg) get(name string) symbol {
	return getInJson(name)
}

func getInJson(name string) symbol {
	switch name {
	case "encode":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "value", Type: &Any{name: "In"}}},
			ReturnType: MakeResult(Str, Str),
		}
	case "decode":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "string", Type: Str}},
			ReturnType: MakeResult(&Any{name: "Out"}, Str),
		}
	default:
		return nil
	}
}
