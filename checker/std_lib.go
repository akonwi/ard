package checker

var prelude = map[string]Module{
	"Result": ResultPkg{},
	"Str":    strMod,
}

func findInStdLib(path string) (Module, bool) {
	// First check for embedded .ard modules
	if mod, ok := findEmbeddedModule(path); ok {
		return mod, true
	}

	switch path {
	case "ard/async":
		return AsyncPkg{}, true
	case "ard/fs":
		return FsPkg{}, true
	case "ard/http":
		return HttpPkg{}, true
	case "ard/json":
		return JsonPkg{}, true
	case "ard/decode":
		return DecodePkg{}, true
	case "ard/maybe":
		return MaybePkg{}, true
	case "ard/result":
		return ResultPkg{}, true
	case "ard/sqlite":
		return SQLitePkg{}, true
	}
	return nil, false
}

/* ard/fs */
type FsPkg struct{}

func (pkg FsPkg) Path() string {
	return "ard/fs"
}

func (pkg FsPkg) Program() *Program {
	return nil
}
func (pkg FsPkg) Get(name string) Symbol {
	switch name {
	case "append":
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{{Name: "path", Type: Str}, {Name: "content", Type: Str}},
				ReturnType: MakeResult(Void, Str),
			},
		}
	case "create_file":
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{{Name: "path", Type: Str}},
				ReturnType: MakeResult(Void, Str),
			},
		}
	case "delete":
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{{Name: "path", Type: Str}},
				ReturnType: MakeResult(Void, Str),
			},
		}
	case "exists":
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{{Name: "path", Type: Str}},
				ReturnType: Bool,
			},
		}
	case "read":
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{{Name: "path", Type: Str}},
				ReturnType: &Maybe{Str},
			},
		}
	case "write":
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{{Name: "path", Type: Str}, {Name: "content", Type: Str}},
				ReturnType: MakeResult(Void, Str),
			},
		}
	default:
		return Symbol{}
	}
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
				ReturnType: &Maybe{&Any{name: "T"}},
			},
		}
	case "some":
		// This function returns Maybe<T> where T is the type of the parameter
		// We use Any as a placeholder, but the type checker should infer
		// the actual type based on the argument type
		any := &Any{name: "T"}
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{{Name: "val", Type: any}},
				ReturnType: &Maybe{any},
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
		valType := &Any{name: "Val"}
		errType := &Any{name: "Err"}
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
		valType := &Any{name: "Val"}
		errType := &Any{name: "Err"}
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

type StrMod struct {
	symbols map[string]Symbol
}

var strMod = StrMod{
	symbols: map[string]Symbol{
		"ToString": Symbol{
			Name: "ToString",
			Type: &Trait{
				Name: "ToString",
				methods: []FunctionDef{
					{
						Name:       "to_str",
						Parameters: []Parameter{},
						ReturnType: Str,
					},
				},
			},
		},
	},
}

func (pkg StrMod) Path() string {
	return "ard/string"
}

func (pkg StrMod) Program() *Program {
	return nil
}

func (pkg StrMod) Get(name string) Symbol {
	return pkg.symbols[name]
}
