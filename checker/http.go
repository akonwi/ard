package checker

var HttpRequestDef = &StructDef{
	Name: "Request",
	Fields: map[string]Type{
		"method":  Str,
		"url":     Str,
		"headers": MakeMap(Str, Str),
		"body":    MakeMaybe(Str),
	},
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
}

var HttpSendFn = &FunctionDef{
	Name: "send",
	Parameters: []Parameter{
		{Name: "request", Type: HttpRequestDef},
	},
	ReturnType: &Maybe{of: HttpResponseDef},
}

/* ard/http */
type HttpPkg struct{}

func (pkg HttpPkg) path() string {
	return "ard/http"
}
func (pkg HttpPkg) buildScope(scope *scope) {
	scope.symbols["Request"] = HttpRequestDef
	scope.symbols["Response"] = HttpResponseDef
	scope.symbols["send"] = HttpSendFn
}
func (pkg HttpPkg) get(name string) symbol {
	switch name {
	case "Request":
		return HttpRequestDef
	case "Response":
		return HttpResponseDef
	case "send":
		return HttpSendFn
	default:
		return nil
	}
}
