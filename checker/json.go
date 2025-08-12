package checker

type JsonPkg struct {
}

func (pkg JsonPkg) Path() string {
	return "ard/json"
}

func (pkg JsonPkg) Program() *Program {
	return nil
}
func (pkg JsonPkg) Get(name string) Symbol {
	switch name {
	case "encode":
		return Symbol{Name: name, Type: &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "value", Type: &Any{name: "In"}}},
			ReturnType: MakeResult(Str, Str),
		}}
	default:
		return Symbol{}
	}
}
