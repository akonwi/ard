package checker

var preludePkgs = map[string]Package{
	"Float":  FloatPkg{},
	"Int":    IntPkg{},
	"Result": ResultPkg{},
}

func findInStdLib(path string) (Package, bool) {
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

func (pkg FloatPkg) path() string {
	return "ard/float"
}
func (pkg FloatPkg) buildScope(scope *scope) {}
func (pkg FloatPkg) get(name string) symbol {
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

func (pkg FsPkg) path() string {
	return "ard/fs"
}
func (pkg FsPkg) buildScope(scope *scope) {}
func (pkg FsPkg) get(name string) symbol {
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

func (pkg IntPkg) path() string {
	return "ard/ints"
}
func (pkg IntPkg) buildScope(scope *scope) {}
func (pkg IntPkg) get(name string) symbol {
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

func (pkg IoPkg) path() string {
	return "ard/io"
}
func (pkg IoPkg) buildScope(scope *scope) {
}
func (pkg IoPkg) get(name string) symbol {
	switch name {
	case "print":
		fn := &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "string", Type: Str}},
			ReturnType: Void,
		}
		return fn
	case "read_line":
		return &FunctionDef{
			Name:       name,
			Parameters: []Parameter{},
			ReturnType: Str,
		}
	default:
		return nil
	}
}

/* ard/maybe */
type MaybePkg struct{}

func (pkg MaybePkg) path() string {
	return "ard/maybe"
}
func (pkg MaybePkg) buildScope(scope *scope) {
}
func (pkg MaybePkg) get(name string) symbol {
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

func (pkg ResultPkg) path() string {
	return "ard/result"
}
func (pkg ResultPkg) buildScope(scope *scope) {}
func (pkg ResultPkg) get(name string) symbol {
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
