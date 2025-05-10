package checker

// Define a struct for HTTP Response with a json method
var HttpResponseDef = &StructDef{
	Name: "Response",
	Fields: map[string]Type{
		"status":  Int,
		"headers": MakeMap(Str, Str),
		"body":    Str,
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
		return &FunctionDef{
			Name: name,
			Parameters: []Parameter{
				{Name: "request", Type: HttpRequestDef},
			},
			ReturnType: &Maybe{of: HttpResponseDef},
		}
	default:
		return nil
	}
}