package checker

func findStdLib(path string, alias string) Package {
	switch path {
	case "ard/io":
		return newIO(alias)
	case "ard/option":
		return newOptions(alias)
	default:
		return nil
	}
}

type IO struct {
	name string
}

func newIO(name string) IO {
	_name := "io"
	if name != "" {
		_name = name
	}
	return IO{name: _name}
}
func (io IO) GetName() string {
	return io.name
}
func (io IO) String() string {
	return io.GetPath()
}
func (io IO) GetPath() string {
	return "ard/io"
}
func (io IO) GetType() Type {
	return io
}
func (io IO) GetProperty(name string) Type {
	switch name {
	case "print":
		return function{
			name:       name,
			parameters: []variable{{name: "string", mut: false, _type: Str{}}},
			returns:    Void{},
		}

	case "read_line":
		return function{
			name:       name,
			parameters: []variable{},
			returns:    Str{},
		}

	default:
		return nil
	}
}
func (io IO) asFunction() (function, bool) {
	return function{}, false
}

type Options struct{ name string }

func newOptions(alias string) Options {
	return Options{name: alias}
}
func (pkg Options) GetPath() string {
	return "ard/option"
}
func (pkg Options) GetName() string {
	if pkg.name != "" {
		return pkg.name
	}
	return "option"
}
func (pkg Options) String() string {
	return pkg.GetPath()
}
func (pkg Options) GetType() Type {
	return pkg
}
func (pkg Options) asFunction() (function, bool) {
	return function{}, false
}
func (pkg Options) GetProperty(name string) Type {
	switch name {
	case "none":
		return function{
			name:       name,
			parameters: []variable{},
			returns:    Option{},
		}
	case "some":
		any := Any{}
		return function{
			name:       name,
			parameters: []variable{{name: "value", mut: false, _type: any}},
			returns:    Option{any},
		}
	default:
		return nil
	}
}
