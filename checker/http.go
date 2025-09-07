package checker

var HttpRequestDef = &StructDef{
	Name:    "Request",
	Fields:  map[string]Type{},
	Methods: map[string]*FunctionDef{},
}

var HttpResponseDef = &StructDef{
	Name:    "Response",
	Fields:  map[string]Type{},
	Methods: map[string]*FunctionDef{},
	Statics: map[string]*FunctionDef{},
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

/* ard/http */
type HttpPkg struct{}

func (pkg HttpPkg) Path() string {
	return "ard/http"
}

func (pkg HttpPkg) Program() *Program {
	return nil
}
func (pkg HttpPkg) Get(name string) Symbol {
	return Symbol{}
}
