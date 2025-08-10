package checker

var DatabaseDef = &StructDef{
	Name:    "Database",
	Fields:  map[string]Type{},
	Methods: map[string]*FunctionDef{
		"close": &FunctionDef{
			Name:       "close",
			Parameters: []Parameter{},
			ReturnType: MakeResult(Void, Str),
		},
		"count": &FunctionDef{
			Name:       "count",
			Parameters: []Parameter{{Name: "table", Type: Str}, {Name: "where", Type: Str}},
			ReturnType: MakeResult(Int, Str),
		},
		"delete": &FunctionDef{
			Name:       "delete",
			Parameters: []Parameter{{Name: "table", Type: Str}, {Name: "where", Type: Str}},
			ReturnType: MakeResult(Bool, Str),
		},
		"exec": &FunctionDef{
			Name:       "exec",
			Parameters: []Parameter{{Name: "sql", Type: Str}},
			ReturnType: MakeResult(Void, Str),
		},
		"exists": &FunctionDef{
			Name:       "exists",
			Parameters: []Parameter{{Name: "table", Type: Str}, {Name: "where", Type: Str}},
			ReturnType: MakeResult(Bool, Str),
		},
		"get": &FunctionDef{
			Name:       "get",
			Parameters: []Parameter{{Name: "table", Type: Str}, {Name: "where", Type: Str}},
			ReturnType: MakeResult(MakeList(&Any{name: "T"}), Str),
		},
		"insert": &FunctionDef{
			Name: "insert",
			// these singleton types are problematic because it means the Any gets refined to a single type
			// instead of allowing multiple types
			Parameters: []Parameter{{Name: "table", Type: Str}, {Name: "values", Type: &Any{name: "V"}}},
			ReturnType: MakeResult(Void, Str),
		},
		"query": &FunctionDef{
			Name:       "query",
			Parameters: []Parameter{{Name: "sql", Type: Str}},
			ReturnType: MakeResult(Dynamic, Str),
		},
		"first": &FunctionDef{
			Name:       "first",
			Parameters: []Parameter{{Name: "sql", Type: Str}},
			ReturnType: MakeResult(Dynamic, Str),
		},
		"update": &FunctionDef{
			Name: "update",
			Parameters: []Parameter{
				{Name: "table", Type: Str},
				{Name: "where", Type: Str},
				{Name: "record", Type: &Any{name: "T"}},
			},
			ReturnType: MakeResult(Void, Str),
		},
		"upsert": &FunctionDef{
			Name: "upsert",
			Parameters: []Parameter{
				{Name: "table", Type: Str},
				{Name: "where", Type: Str},
				{Name: "record", Type: &Any{name: "T"}},
			},
			ReturnType: MakeResult(Bool, Str),
		},
	},
	Statics: map[string]*FunctionDef{},
}

var SQLiteOpenFn = &FunctionDef{
	Name: "open",
	Parameters: []Parameter{
		{Name: "file_path", Type: Str},
	},
	ReturnType: MakeResult(DatabaseDef, Str),
}

/* ard/sqlite */
type SQLitePkg struct{}

func (pkg SQLitePkg) Path() string {
	return "ard/sqlite"
}

func (pkg SQLitePkg) Program() *Program {
	return nil
}

func (pkg SQLitePkg) Get(name string) Symbol {
	switch name {
	case "Database":
		return Symbol{Name: name, Type: DatabaseDef}
	case "open":
		return Symbol{Name: name, Type: SQLiteOpenFn}
	default:
		return Symbol{}
	}
}
