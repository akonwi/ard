package checker

func buildHttpPkgScope(scope *scope) {
	scope.symbols["Request"] = HttpRequestDef
	scope.symbols["Response"] = HttpResponseDef
	scope.symbols["get"] = HttpGetFn
}

// Define a struct for HTTP Response with a json method
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
	},
}

// Define HTTP Request structure
var HttpRequestDef = &StructDef{
	Name: "Request",
	Fields: map[string]Type{
		"url":     Str,
		"headers": MakeMap(Str, Str),
	},
}

var HttpGetFn = &FunctionDef{
	Name: "get",
	Parameters: []Parameter{
		{Name: "request", Type: HttpRequestDef},
	},
	ReturnType: &Maybe{of: HttpResponseDef},
}

// Add the json method to the Response struct
func init() {
	HttpResponseDef.Fields["json"] = &FunctionDef{
		Name:       "json",
		Parameters: []Parameter{},
		ReturnType: &Maybe{of: &Any{name: "T"}},
	}
}

func getInHTTP(name string) symbol {
	switch name {
	case "Request":
		// Return the Request struct definition
		return HttpRequestDef
	case "Response":
		// Return the Response struct with the json method
		return HttpResponseDef
	case "get":
		// Define the get function which returns Maybe<Response>
		return HttpGetFn
	default:
		return nil
	}
}
