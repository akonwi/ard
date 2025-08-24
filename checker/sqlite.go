package checker

var DatabaseDef = &StructDef{
	Name:   "Database",
	Fields: map[string]Type{},
	Methods: map[string]*FunctionDef{
		"close": {
			Name:       "close",
			Parameters: []Parameter{},
			ReturnType: MakeResult(Void, Str),
		},
		"count": {
			Name:       "count",
			Parameters: []Parameter{{Name: "table", Type: Str}, {Name: "where", Type: Str}},
			ReturnType: MakeResult(Int, Str),
		},
		"delete": {
			Name:       "delete",
			Parameters: []Parameter{{Name: "table", Type: Str}, {Name: "where", Type: Str}},
			ReturnType: MakeResult(Bool, Str),
		},
		"exec": {
			Name:       "exec",
			Parameters: []Parameter{{Name: "sql", Type: Str}},
			ReturnType: MakeResult(Void, Str),
		},
		"exists": {
			Name:       "exists",
			Parameters: []Parameter{{Name: "table", Type: Str}, {Name: "where", Type: Str}},
			ReturnType: MakeResult(Bool, Str),
		},
		"insert": {
			Name:       "insert",
			Parameters: []Parameter{{Name: "table", Type: Str}, {Name: "values", Type: MakeMap(Str, Dynamic)}},
			ReturnType: MakeResult(Dynamic, Str),
		},
		"query": {
			Name:       "query",
			Parameters: []Parameter{{Name: "sql", Type: Str}},
			ReturnType: MakeResult(Dynamic, Str),
		},
		"first": {
			Name:       "first",
			Parameters: []Parameter{{Name: "sql", Type: Str}},
			ReturnType: MakeResult(Dynamic, Str),
		},
		"update": {
			Name: "update",
			Parameters: []Parameter{
				{Name: "table", Type: Str},
				{Name: "where", Type: Str},
				{Name: "values", Type: MakeMap(Str, Dynamic)},
			},
			ReturnType: MakeResult(Dynamic, Str),
		},
		"upsert": {
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
