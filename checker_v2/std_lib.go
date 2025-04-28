package checker_v2

func getInPackage(pkgPath, name string) symbol {
	switch pkgPath {
	case "ard/io":
		return getInIO(name)
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
