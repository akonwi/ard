package checker

var prelude = map[string]Module{
	"Result":   ResultPkg{},
	"Chan":     ChannelStaticPkg{},
	"Receiver": EmptyBuiltinPkg{name: "Receiver"},
	"Sender":   EmptyBuiltinPkg{name: "Sender"},
}

func findInStdLib(path string) (Module, bool) {
	// Provide minimal hardcoded definitions for special modules
	// These provide the function signatures for type checking
	switch path {
	case "ard/maybe":
		return MaybePkg{}, true
	case "ard/result":
		return ResultPkg{}, true
	case "ard/async":
		return AsyncPkg{}, true
	case "ard/unsafe":
		return UnsafePkg{}, true
	}

	return FindEmbeddedModule(path)
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

/* ard/async */
type AsyncPkg struct{}

func (pkg AsyncPkg) Path() string { return "ard/async" }

func (pkg AsyncPkg) Program() *Program { return nil }

func (pkg AsyncPkg) Get(name string) Symbol {
	switch name {
	case "start":
		return Symbol{Name: name, Type: &FunctionDef{Name: name, Parameters: []Parameter{{Name: "task", Type: &FunctionDef{Name: "<function>", ReturnType: Void}}}, ReturnType: Void}}
	default:
		return Symbol{}
	}
}

/* ard/unsafe */
type UnsafePkg struct{}

func (pkg UnsafePkg) Path() string { return "ard/unsafe" }

func (pkg UnsafePkg) Program() *Program { return nil }

func (pkg UnsafePkg) Get(name string) Symbol {
	switch name {
	case "cast":
		return Symbol{Name: name, Type: &FunctionDef{Name: name, GenericParams: []string{"T"}, Parameters: []Parameter{{Name: "value", Type: Any}}, ReturnType: MakeMaybe(&TypeVar{name: "T"})}}
	case "is_nil":
		return Symbol{Name: name, Type: &FunctionDef{Name: name, Parameters: []Parameter{{Name: "value", Type: Any}}, ReturnType: Bool}}
	default:
		return Symbol{}
	}
}

type EmptyBuiltinPkg struct{ name string }

func (pkg EmptyBuiltinPkg) Path() string { return "builtin/" + pkg.name }

func (pkg EmptyBuiltinPkg) Program() *Program { return nil }

func (pkg EmptyBuiltinPkg) Get(name string) Symbol { return Symbol{} }

/* Chan static functions — typed channels lowering to native Go `chan T` */
type ChannelStaticPkg struct{}

func (pkg ChannelStaticPkg) Path() string { return "builtin/Chan" }

func (pkg ChannelStaticPkg) Program() *Program { return nil }

func (pkg ChannelStaticPkg) Get(name string) Symbol {
	switch name {
	case "new":
		// Chan::new<$T>(capacity: Int?) Chan<$T>; send/recv/close are methods on Chan.
		t := &TypeVar{name: "T"}
		return Symbol{Name: name, Type: &FunctionDef{
			Name:          name,
			GenericParams: []string{"T"},
			Parameters:    []Parameter{{Name: "capacity", Type: MakeMaybe(Int)}},
			ReturnType:    MakeChan(t),
		}}
	default:
		return Symbol{}
	}
}

// Builtin package symbol names: the single source of truth for each Get
// switch above. BuiltinPkgNames is exported so tests can assert Get and
// Symbols stay in sync.
var BuiltinPkgNames = map[string][]string{
	"ard/maybe":    {"none", "some"},
	"ard/result":   {"ok", "err"},
	"ard/async":    {"start"},
	"ard/unsafe":   {"cast", "is_nil"},
	"builtin/Chan": {"new"},
}

func (pkg MaybePkg) Symbols() map[string]Symbol {
	return symbolsByName(pkg, BuiltinPkgNames[pkg.Path()]...)
}

func (pkg ResultPkg) Symbols() map[string]Symbol {
	return symbolsByName(pkg, BuiltinPkgNames[pkg.Path()]...)
}

func (pkg AsyncPkg) Symbols() map[string]Symbol {
	return symbolsByName(pkg, BuiltinPkgNames[pkg.Path()]...)
}

func (pkg UnsafePkg) Symbols() map[string]Symbol {
	return symbolsByName(pkg, BuiltinPkgNames[pkg.Path()]...)
}

func (pkg EmptyBuiltinPkg) Symbols() map[string]Symbol { return map[string]Symbol{} }

func (pkg ChannelStaticPkg) Symbols() map[string]Symbol {
	return symbolsByName(pkg, BuiltinPkgNames[pkg.Path()]...)
}

func symbolsByName(mod Module, names ...string) map[string]Symbol {
	out := make(map[string]Symbol, len(names))
	for _, name := range names {
		if sym := mod.Get(name); !sym.IsZero() {
			out[name] = sym
		}
	}
	return out
}

// PreludeModule returns a built-in prelude static package (Result, Chan,
// Receiver, Sender) by its surface name. Tooling uses this for completion.
func PreludeModule(name string) (Module, bool) {
	mod, ok := prelude[name]
	return mod, ok
}
