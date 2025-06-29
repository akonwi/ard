package checker

var HttpRequestDef = &StructDef{
	Name: "Request",
	Fields: map[string]Type{
		"method":  Str,
		"url":     Str,
		"headers": MakeMap(Str, Str),
		"body":    MakeMaybe(Str),
		"path": &FunctionDef{
			Name:       "path",
			Parameters: []Parameter{},
			ReturnType: MakeResult(Str, Str),
		},
	},
}

var ResponseNew = &FunctionDef{
	Name:       "new",
	Parameters: []Parameter{{Name: "status", Type: Int}, {Name: "body", Type: Str}},
	ReturnType: HttpResponseDef,
	Public:     true,
}

var HttpResponseDef = &StructDef{
	Name: "Response",
	Fields: map[string]Type{
		"status":  Int,
		"headers": MakeMap(Str, Str),
		"body":    Str,
		"is_ok": &FunctionDef{
			Name:       "is_ok",
			Parameters: []Parameter{},
			ReturnType: Bool,
		},
		"json": &FunctionDef{
			Name:       "json",
			Parameters: []Parameter{},
			ReturnType: MakeResult(&Any{name: "T"}, Str),
		},
	},
	Statics: map[string]*FunctionDef{},
}

// workaround circular references to HttpResponseDef
func init() {
	HttpResponseDef.Statics["new"] = ResponseNew
}

var HttpSendFn = &FunctionDef{
	Name: "send",
	Parameters: []Parameter{
		{Name: "request", Type: HttpRequestDef},
	},
	ReturnType: MakeResult(HttpResponseDef, Str),
}

var HttpServeFn = &FunctionDef{
	Name: "serve",
	Parameters: []Parameter{
		{Name: "port", Type: Int},
		{
			Name: "handlers",
			Type: MakeMap(
				Str,
				&FunctionDef{
					Parameters: []Parameter{{Name: "req", Type: HttpRequestDef}},
					ReturnType: HttpResponseDef,
				}),
		},
	},
	ReturnType: Void,
}

var HttpRespondFn = &FunctionDef{
	Name: "respond",
	Parameters: []Parameter{
		{Name: "status", Type: Int},
		{Name: "body", Type: Str},
	},
	ReturnType: HttpResponseDef,
}

/* ard/http */
type HttpPkg struct{}

func (pkg HttpPkg) Path() string {
	return "ard/http"
}

func (pkg HttpPkg) Program() *Program {
	return nil
}
func (pkg HttpPkg) Get(name string) symbol {
	switch name {
	case "Request":
		return HttpRequestDef
	case "Response":
		return HttpResponseDef
	case "send":
		return HttpSendFn
	case "serve":
		return HttpServeFn
	case "respond":
		return HttpRespondFn
	default:
		return nil
	}
}
