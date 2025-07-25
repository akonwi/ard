package checker

var HttpMethodDef = &Enum{
	Name: "Method",
	Variants: []string{
		"Get",
		"Post",
		"Put",
		"Del",
		"Patch",
	},
	Traits: []*Trait{},
}

var HttpRequestDef = &StructDef{
	Name: "Request",
	Fields: map[string]Type{
		"method":  HttpMethodDef,
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
	Private:    false,
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
	// Add ToString trait to Method enum
	HttpMethodDef.Traits = append(HttpMethodDef.Traits, strMod.symbols["ToString"].Type.(*Trait))
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
func (pkg HttpPkg) Get(name string) Symbol {
	switch name {
	case "Method":
		return Symbol{Name: name, Type: HttpMethodDef}
	case "Request":
		return Symbol{Name: name, Type: HttpRequestDef}
	case "Response":
		return Symbol{Name: name, Type: HttpResponseDef}
	case "send":
		return Symbol{Name: name, Type: HttpSendFn}
	case "serve":
		return Symbol{Name: name, Type: HttpServeFn}
	case "respond":
		return Symbol{Name: name, Type: HttpRespondFn}
	default:
		return Symbol{}
	}
}
