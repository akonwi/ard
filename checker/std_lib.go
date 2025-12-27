package checker

var prelude = map[string]Module{
	"Result": ResultPkg{},
}

func findInStdLib(path string) (Module, bool) {
	// First check for embedded .ard modules
	if mod, ok := FindEmbeddedModule(path); ok {
		return mod, true
	}

	switch path {
	case "ard/maybe":
		return MaybePkg{}, true
	case "ard/result":
		return ResultPkg{}, true
	}
	return nil, false
}

/* ard/maybe */
type MaybePkg struct{}

func (pkg MaybePkg) Path() string {
	return "ard/maybe"
}

func (pkg MaybePkg) Program() *Program {
	return nil
}
func (pkg MaybePkg) Get(name string) Symbol {
	switch name {
	case "none":
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{},
				ReturnType: &Maybe{&TypeVar{name: "T"}},
			},
		}
	case "some":
		// This function returns Maybe<T> where T is the type of the parameter
		// We use TypeVar as a placeholder, but the type checker should infer
		// the actual type based on the argument type
		typeVar := &TypeVar{name: "T"}
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{{Name: "val", Type: typeVar}},
				ReturnType: &Maybe{typeVar},
			},
		}
	default:
		return Symbol{}
	}
}

type ResultPkg struct {
}

func (pkg ResultPkg) Path() string {
	return "ard/result"
}

func (pkg ResultPkg) Program() *Program {
	return nil
}
func (pkg ResultPkg) Get(name string) Symbol {
	switch name {
	case "ok":
		// This function returns Result<T, E> where T is the type of the parameter
		// and E is a generic type parameter
		valType := &TypeVar{name: "Val"}
		errType := &TypeVar{name: "Err"}
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{{Name: "val", Type: valType}},
				ReturnType: MakeResult(valType, errType),
			},
		}
	case "err":
		// This function returns Result<T, E> where E is the type of the parameter
		// and T is a generic type parameter
		valType := &TypeVar{name: "Val"}
		errType := &TypeVar{name: "Err"}
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{{Name: "err", Type: errType}},
				ReturnType: MakeResult(valType, errType),
			},
		}
	default:
		return Symbol{}
	}
}
