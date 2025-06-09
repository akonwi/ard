package checker

var prelude = map[string]Module{
	"Float":  FloatPkg{},
	"Int":    IntPkg{},
	"Result": ResultPkg{},
	"Str":    strMod,
}

func findInStdLib(path string) (Module, bool) {
	switch path {
	case "ard/fs":
		return FsPkg{}, true
	case "ard/io":
		return IoPkg{}, true
	case "ard/http":
		return HttpPkg{}, true
	case "ard/json":
		return JsonPkg{}, true
	case "ard/maybe":
		return MaybePkg{}, true
	case "ard/result":
		return ResultPkg{}, true
	}
	return nil, false
}

/* ard/float */
type FloatPkg struct{}

func (pkg FloatPkg) Path() string {
	return "ard/float"
}
func (pkg FloatPkg) Get(name string) symbol {
	switch name {
	case "from_int":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "int", Type: Int}},
			ReturnType: Float,
		}
	case "from_str":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "string", Type: Str}},
			ReturnType: &Maybe{Float},
		}
	default:
		return nil
	}
}

/* ard/fs */
type FsPkg struct{}

func (pkg FsPkg) Path() string {
	return "ard/fs"
}
func (pkg FsPkg) Get(name string) symbol {
	switch name {
	case "append":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "path", Type: Str}, {Name: "content", Type: Str}},
			ReturnType: Bool,
		}
	case "create_file":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "path", Type: Str}},
			ReturnType: Bool,
		}
	case "delete":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "path", Type: Str}},
			ReturnType: Bool,
		}
	case "exists":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "path", Type: Str}},
			ReturnType: Bool,
		}
	case "read":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "path", Type: Str}},
			ReturnType: &Maybe{Str},
		}
	case "write":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "path", Type: Str}, {Name: "content", Type: Str}},
			ReturnType: Bool,
		}
	default:
		return nil
	}
}

/* ard/ints */
type IntPkg struct{}

func (pkg IntPkg) Path() string {
	return "ard/int"
}
func (pkg IntPkg) Get(name string) symbol {
	switch name {
	case "from_str":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "string", Type: Str}},
			ReturnType: &Maybe{Int},
		}
	default:
		return nil
	}
}

/* ard/io */
type IoPkg struct{}

func (pkg IoPkg) Path() string {
	return "ard/io"
}
func (pkg IoPkg) Get(name string) symbol {
	switch name {
	case "print":
		fn := &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "string", Type: strMod.symbols["ToString"].(*Trait)}},
			ReturnType: Void,
		}
		return fn
	case "read_line":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: MakeResult(Str, Str),
		}
	default:
		return nil
	}
}

/* ard/maybe */
type MaybePkg struct{}

func (pkg MaybePkg) Path() string {
	return "ard/maybe"
}
func (pkg MaybePkg) Get(name string) symbol {
	switch name {
	case "none":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: &Maybe{&Any{name: "T"}},
		}
	case "some":
		// This function returns Maybe<T> where T is the type of the parameter
		// We use Any as a placeholder, but the type checker should infer
		// the actual type based on the argument type
		any := &Any{name: "T"}
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "val", Type: any}},
			ReturnType: &Maybe{any},
		}
	default:
		return nil
	}
}

type ResultPkg struct {
}

func (pkg ResultPkg) Path() string {
	return "ard/result"
}
func (pkg ResultPkg) Get(name string) symbol {
	switch name {
	case "ok":
		// This function returns Result<T, E> where T is the type of the parameter
		// and E is a generic type parameter
		valType := &Any{name: "Val"}
		errType := &Any{name: "Err"}
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "val", Type: valType}},
			ReturnType: MakeResult(valType, errType),
		}
	case "err":
		// This function returns Result<T, E> where E is the type of the parameter
		// and T is a generic type parameter
		valType := &Any{name: "Val"}
		errType := &Any{name: "Err"}
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "err", Type: errType}},
			ReturnType: MakeResult(valType, errType),
		}
	default:
		return nil
	}
}

type StrMod struct {
	symbols map[string]symbol
}

var strMod = StrMod{
	symbols: map[string]symbol{
		"ToString": &Trait{
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
}

func (pkg StrMod) Path() string {
	return "ard/string"
}
func (pkg StrMod) Get(name string) symbol {
	return pkg.symbols[name]
}
