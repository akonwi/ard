package checker_v2

var preludePkgs = map[string]*StdPackage{
	"Int": {Name: "Int", Path: "ard/ints"},
}

func getInPackage(pkgPath, name string) symbol {
	switch pkgPath {
	case "ard/ints":
		return getInInts(name)
	case "ard/io":
		return getInIO(name)
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
			ReturnType: Int,
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
