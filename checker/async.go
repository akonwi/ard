package checker

var Fiber = &StructDef{
	Name: "Fiber",
	Fields: map[string]Type{
		"join": &FunctionDef{
			Name:       "join",
			Parameters: []Parameter{},
			ReturnType: Void,
		},
	},
	Statics: map[string]*FunctionDef{},
	Traits:  []*Trait{},
}

var AsyncStart = &FunctionDef{
	Name: "start",
	Parameters: []Parameter{
		Parameter{
			Name: "worker",
			Type: &FunctionDef{
				Parameters: []Parameter{},
				ReturnType: Void,
			},
		},
	},
	ReturnType: Fiber,
}

/* ard/async */
type AsyncPkg struct{}

func (pkg AsyncPkg) Path() string {
	return "ard/async"
}

func (pkg AsyncPkg) Program() *Program {
	return nil
}
func (pkg AsyncPkg) Get(name string) Symbol {
	switch name {
	case "sleep":
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{{Name: "duration", Type: Int}},
				ReturnType: Void,
			},
		}
	case "start":
		return Symbol{Name: name, Type: AsyncStart}
	default:
		return Symbol{}
	}
}
