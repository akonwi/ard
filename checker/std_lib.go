package checker

func findStdLib(path string, alias string) Package {
	switch path {
	case "ard/fs":
		return newFileSystem(alias)
	case "ard/io":
		return newIO(alias)
	case "ard/json":
		return newJSON(alias)
	case "ard/maybe":
		return newMaybe(alias)
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

type FileSystem struct {
	alias string
}

func newFileSystem(alias string) FileSystem {
	return FileSystem{alias: alias}
}
func (fs FileSystem) GetName() string {
	if fs.alias != "" {
		return fs.alias
	}
	return "fs"
}
func (fs FileSystem) String() string {
	return fs.GetPath()
}
func (fs FileSystem) GetPath() string {
	return "ard/fs"
}
func (fs FileSystem) GetType() Type {
	return fs
}
func (fs FileSystem) asFunction() (function, bool) {
	return function{}, false
}
func (fs FileSystem) GetProperty(name string) Type {
	switch name {
	case "exists":
		return function{
			name:       name,
			parameters: []variable{{name: "path", mut: false, _type: Str{}}},
			returns:    Bool{},
		}
	case "read":
		return function{
			name:       name,
			parameters: []variable{{name: "path", mut: false, _type: Str{}}},
			returns:    MakeMaybe(Str{}),
		}

	case "create_file":
		return function{
			name:       name,
			parameters: []variable{{name: "path", mut: false, _type: Str{}}},
			returns:    Bool{},
		}
	case "delete":
		return function{
			name:       name,
			parameters: []variable{{name: "path", mut: false, _type: Str{}}},
			returns:    Bool{},
		}
	case "write":
		return function{
			name:       name,
			parameters: []variable{{name: "path", mut: false, _type: Str{}}, {name: "content", mut: false, _type: Str{}}},
			returns:    Void{},
		}
	case "append":
		return function{
			name:       name,
			parameters: []variable{{name: "path", mut: false, _type: Str{}}, {name: "content", mut: false, _type: Str{}}},
			returns:    Void{},
		}
	default:
		return nil
	}
}

type Options struct{ name string }

func newMaybe(alias string) Options {
	return Options{name: alias}
}
func (pkg Options) GetPath() string {
	return "ard/maybe"
}
func (pkg Options) GetName() string {
	if pkg.name != "" {
		return pkg.name
	}
	return "maybe"
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
			returns:    MakeMaybe(MakeAny("Any")),
		}
	case "some":
		Value := MakeAny("Value")
		return function{
			name:       name,
			parameters: []variable{{name: "value", mut: false, _type: Value}},
			returns:    MakeMaybe(Value),
		}
	default:
		return nil
	}
}

type JSON struct{ name string }

func newJSON(alias string) JSON {
	return JSON{name: alias}
}

func (j JSON) GetName() string {
	if j.name != "" {
		return j.name
	}
	return "json"
}
func (j JSON) GetPath() string {
	return "ard/json"
}
func (j JSON) GetType() Type {
	return j
}
func (j JSON) asFunction() (function, bool) {
	return function{}, false
}
func (j JSON) String() string {
	return j.GetPath()
}
func (j JSON) GetProperty(name string) Type {
	switch name {
	case "encode":
		return function{
			name:       name,
			parameters: []variable{{name: "val", mut: false, _type: MakeAny("Value")}},
			returns:    MakeMaybe(Str{}),
		}
	case "decode":
		return function{
			name: name,
			parameters: []variable{
				{name: "string", mut: false, _type: Str{}},
			},
			returns: MakeMaybe(MakeAny("Out")),
		}
	default:
		return nil
	}
}
