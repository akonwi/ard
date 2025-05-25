package checker

func getInJson(name string) symbol {
	switch name {
	case "encode":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "value", Type: &Any{name: "In"}}},
			ReturnType: &Maybe{Str},
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
