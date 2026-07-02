package checker

var prelude = map[string]Module{
	"Result": ResultPkg{},
}

func findInStdLib(path string) (Module, bool) {
	// Provide minimal hardcoded definitions for special modules
	// These provide the function signatures for type checking
	switch path {
	case "ard/maybe":
		return MaybePkg{}, true
	case "ard/result":
		return ResultPkg{}, true
	case "ard/channel":
		return ChannelPkg{}, true
	case "ard/any":
		return AnyPkg{}, true
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

/* ard/any */
type AnyPkg struct{}

func (pkg AnyPkg) Path() string { return "ard/any" }

func (pkg AnyPkg) Program() *Program { return nil }

func (pkg AnyPkg) Get(name string) Symbol {
	switch name {
	case "cast":
		return Symbol{Name: name, Type: &FunctionDef{Name: name, GenericParams: []string{"T"}, Parameters: []Parameter{{Name: "value", Type: Any}}, ReturnType: MakeMaybe(&TypeVar{name: "T"})}}
	default:
		return Symbol{}
	}
}

/* ard/channel — typed channels lowering to native Go `chan T` */
type ChannelPkg struct{}

func (pkg ChannelPkg) Path() string { return "ard/channel" }

func (pkg ChannelPkg) Program() *Program { return nil }

func (pkg ChannelPkg) Get(name string) Symbol {
	switch name {
	case "Chan":
		// The Chan<$T> type, resolvable in annotations as channel::Chan<T>.
		return Symbol{Name: name, Type: MakeChan(&TypeVar{name: "T"})}
	case "Receiver":
		// The receive-only Receiver<$T> type.
		return Symbol{Name: name, Type: MakeReceiver(&TypeVar{name: "T"})}
	case "Sender":
		// The send-only Sender<$T> type.
		return Symbol{Name: name, Type: MakeSender(&TypeVar{name: "T"})}
	case "new":
		// new<$T>(capacity: Int) Chan<$T>; send/recv/close are methods on Chan.
		t := &TypeVar{name: "T"}
		return Symbol{Name: name, Type: &FunctionDef{
			Name:          name,
			GenericParams: []string{"T"},
			Parameters:    []Parameter{{Name: "capacity", Type: Int}},
			ReturnType:    MakeChan(t),
		}}
	case "receiver":
		// receiver<$T>(ch: Chan<$T>) Receiver<$T> narrows a bidirectional channel
		// to a receive-only view.
		t := &TypeVar{name: "T"}
		return Symbol{Name: name, Type: &FunctionDef{
			Name:          name,
			GenericParams: []string{"T"},
			Parameters:    []Parameter{{Name: "ch", Type: MakeChan(t)}},
			ReturnType:    MakeReceiver(t),
		}}
	case "sender":
		// sender<$T>(ch: Chan<$T>) Sender<$T> narrows a bidirectional channel to a
		// send-only view.
		t := &TypeVar{name: "T"}
		return Symbol{Name: name, Type: &FunctionDef{
			Name:          name,
			GenericParams: []string{"T"},
			Parameters:    []Parameter{{Name: "ch", Type: MakeChan(t)}},
			ReturnType:    MakeSender(t),
		}}
	default:
		return Symbol{}
	}
}
