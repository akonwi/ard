package checker

var prelude = map[string]Module{
	"Result": ResultPkg{},
}

func findInStdLib(path string) (Module, bool) {
	// Provide minimal hardcoded definitions for special modules
	// These provide the function signatures for type checking
	switch path {
	case "ard/async":
		return AsyncPkg{}, true
	case "ard/maybe":
		return MaybePkg{}, true
	case "ard/result":
		return ResultPkg{}, true
	}
	
	// Check for embedded .ard modules for other modules
	if mod, ok := FindEmbeddedModule(path); ok {
		return mod, true
	}

	return nil, false
}

/* ard/async */
type AsyncPkg struct{}

func (pkg AsyncPkg) Path() string {
	return "ard/async"
}

func (pkg AsyncPkg) Program() *Program {
	// Return the embedded module's program so Fiber methods have access to wait_for
	if embeddedMod, ok := FindEmbeddedModule("ard/async"); ok {
		return embeddedMod.Program()
	}
	return nil
}

func (pkg AsyncPkg) Get(name string) Symbol {
	switch name {
	case "sleep":
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name:       name,
				Parameters: []Parameter{{Name: "ms", Type: Int}},
				ReturnType: Void,
			},
		}
	case "start":
		// Get the Fiber struct from the embedded module for consistency
		var fiberType Type = &StructDef{Name: "Fiber"}
		if embeddedMod, ok := FindEmbeddedModule("ard/async"); ok {
			if fiberSym := embeddedMod.Get("Fiber"); fiberSym.Type != nil {
				fiberType = fiberSym.Type
			}
		}
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name: name,
				Parameters: []Parameter{{
					Name: "do",
					Type: &FunctionDef{
						Name:       "",
						Parameters: []Parameter{},
						ReturnType: Void,
					},
				}},
				ReturnType: fiberType,
			},
		}
	case "eval":
		// Get the Fiber struct from the embedded module for consistency
		var fiberType Type = &StructDef{Name: "Fiber"}
		if embeddedMod, ok := FindEmbeddedModule("ard/async"); ok {
			if fiberSym := embeddedMod.Get("Fiber"); fiberSym.Type != nil {
				fiberType = fiberSym.Type
			}
		}
		typeVar := &TypeVar{name: "T"}
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name: name,
				Parameters: []Parameter{{
					Name: "do",
					Type: &FunctionDef{
						Name:       "",
						Parameters: []Parameter{},
						ReturnType: typeVar,
					},
				}},
				ReturnType: fiberType,
			},
		}
	case "join":
		// Get the Fiber struct from the embedded module for consistency
		var fiberType Type = &StructDef{Name: "Fiber"}
		if embeddedMod, ok := FindEmbeddedModule("ard/async"); ok {
			if fiberSym := embeddedMod.Get("Fiber"); fiberSym.Type != nil {
				fiberType = fiberSym.Type
			}
		}
		return Symbol{
			Name: name,
			Type: &FunctionDef{
				Name: name,
				Parameters: []Parameter{{
					Name: "fibers",
					Type: &List{of: fiberType},
				}},
				ReturnType: Void,
			},
		}
	case "Fiber":
		// Return the Fiber struct from the embedded module
		if embeddedMod, ok := FindEmbeddedModule("ard/async"); ok {
			return embeddedMod.Get("Fiber")
		}
		return Symbol{}
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
