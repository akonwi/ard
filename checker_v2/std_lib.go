package checker_v2

var preludePkgs = map[string]*StdPackage{
	"Float": {Name: "Int", Path: "ard/float"},
	"Int":   {Name: "Int", Path: "ard/ints"},
}

func getInPackage(pkgPath, name string) symbol {
	switch pkgPath {
	case "ard/float":
		return getInFloat(name)
	case "ard/fs":
		return getInFS(name)
	case "ard/ints":
		return getInInts(name)
	case "ard/io":
		return getInIO(name)
	case "ard/maybe":
		return getInMaybe(name)
	default:
		return nil
	}
}

func getInFloat(name string) symbol {
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

func getInFS(name string) symbol {
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

func getInInts(name string) symbol {
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

func getInIO(name string) symbol {
	switch name {
	case "print":
		fn := &FunctionDef{
			Name:       name,
			Parameters: []Parameter{{Name: "string", Type: Str}},
			ReturnType: Void,
		}
		return fn
	default:
		return nil
	}
}

func getInMaybe(name string) symbol {
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
