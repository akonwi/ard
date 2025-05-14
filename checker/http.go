package checker

func buildHttpPkgScope(scope *scope) {
	scope.symbols["Request"] = HttpRequestDef
	scope.symbols["Response"] = HttpResponseDef
	scope.symbols["get"] = HttpGetFn
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
			ReturnType: &Maybe{of: &Any{name: "T"}},
		},
	},
}

var HttpRequestDef = &StructDef{
	Name: "Request",
	Fields: map[string]Type{
		"url":     Str,
		"headers": MakeMap(Str, Str),
		"body":    MakeMaybe(Str),
	},
}

var HttpGetFn = &FunctionDef{
	Name: "get",
	Parameters: []Parameter{
		{Name: "request", Type: HttpRequestDef},
	},
	ReturnType: &Maybe{of: HttpResponseDef},
}

var HttpPostFn = &FunctionDef{
	Name: "post",
	Parameters: []Parameter{
		{Name: "request", Type: HttpRequestDef},
	},
	ReturnType: &Maybe{of: HttpResponseDef},
}
var HttpPutFn = &FunctionDef{
	Name: "put",
	Parameters: []Parameter{
		{Name: "request", Type: HttpRequestDef},
	},
	ReturnType: &Maybe{of: HttpResponseDef},
}

func getInHTTP(name string) symbol {
	switch name {
	case "Request":
		return HttpRequestDef
	case "Response":
		return HttpResponseDef
	case "get":
		return HttpGetFn
	case "post":
		return HttpPostFn
	case "put":
		return HttpPutFn
	default:
		return nil
	}
}
