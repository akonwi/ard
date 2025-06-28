package checker

var DatabaseDef = &StructDef{
	Name: "Database",
	Fields: map[string]Type{
		"exec": &FunctionDef{
			Name:       "exec",
			Parameters: []Parameter{{Name: "sql", Type: Str}},
			ReturnType: MakeMaybe(Str),
		},
		"insert": &FunctionDef{
			Name: "insert",
			// these singleton types are problematic because it means the Any gets refined to a single type
			// instead of allowing multiple types
			Parameters: []Parameter{{Name: "table", Type: Str}, {Name: "values", Type: &Any{name: "V"}}},
			ReturnType: MakeMaybe(Str),
		},
		"update": &FunctionDef{
			Name:       "update",
			Parameters: []Parameter{{Name: "where", Type: Str}, {Name: "record", Type: &Any{name: "T"}}},
			ReturnType: MakeResult(Void, Str),
		},
		"get": &FunctionDef{
			Name:       "get",
			Parameters: []Parameter{{Name: "table", Type: Str}, {Name: "where", Type: Str}},
			ReturnType: MakeResult(MakeList(&Any{name: "T"}), Str),
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

func (pkg SQLitePkg) Get(name string) symbol {
	switch name {
	case "Database":
		return DatabaseDef
	case "open":
		return SQLiteOpenFn
	default:
		return nil
	}
}
