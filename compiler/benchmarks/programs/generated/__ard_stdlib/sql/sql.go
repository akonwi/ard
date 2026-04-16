package sql

import (
	ardgo "github.com/akonwi/ard/go"
	list "programs/__ard_stdlib/list"
	strings "strings"
)

type Transaction struct {
	Tx any
}

func (self Transaction) Commit() ardgo.Result[struct{}, string] {
	return commitTx(self.Tx)
}

func (self Transaction) Exec(sql string) ardgo.Result[struct{}, string] {
	return execute(self.Tx, sql, list.New[any]())
}

func (self Transaction) Query(sql string) Query {
	return Query{Conn: self.Tx, Params: ExtractParams(sql), String: sql}
}

func (self Transaction) Rollback() ardgo.Result[struct{}, string] {
	return rollbackTx(self.Tx)
}

type Preparedquery struct {
	String string
	Values []any
}

type Query struct {
	Conn   any
	Params []string
	String string
}

func (self Query) All(args map[string]any) ardgo.Result[[]any, string] {
	__ardTry0 := self.Prepare(args)
	if __ardTry0.IsErr() {
		return ardgo.Err[[]any, string](__ardTry0.UnwrapErr())
	}
	prepared := __ardTry0.UnwrapOk()
	return runQuery(self.Conn, prepared.String, prepared.Values)
}

func (self Query) First(args map[string]any) ardgo.Result[ardgo.Maybe[any], string] {
	__ardTry1 := self.All(args)
	if __ardTry1.IsErr() {
		return ardgo.Err[ardgo.Maybe[any], string](__ardTry1.UnwrapErr())
	}
	rows := append([]any(nil), __ardTry1.UnwrapOk()...)
	var __ardIntMatch2 ardgo.Result[ardgo.Maybe[any], string]
	switch {
	case len(rows) == 0:
		__ardIntMatch2 = ardgo.Ok[ardgo.Maybe[any], string](ardgo.None[any]())
	default:
		__ardIntMatch2 = ardgo.Ok[ardgo.Maybe[any], string](ardgo.Some[any](rows[0]))
	}
	return __ardIntMatch2
}

func (self Query) Prepare(args map[string]any) ardgo.Result[Preparedquery, string] {
	queryStr := self.String
	values := append([]any(nil), []any{}...)
	for _, param := range self.Params {
		queryStr = strings.Replace(queryStr, ("@" + param), "?", 1)
		__ardTry3 := func() ardgo.Maybe[any] {
			if value, ok := args[param]; ok {
				return ardgo.Some(value)
			}
			return ardgo.None[any]()
		}()
		if __ardTry3.IsNone() {
			ardgo.Err[any, string](("Missing parameter: " + param))
		}
		value := __ardTry3.Expect("unreachable none in try success path")
		_ = func() []any { values = append(values, value); return values }()
	}
	return ardgo.Ok[Preparedquery, string](Preparedquery{String: queryStr, Values: values})
}

func (self Query) Run(args map[string]any) ardgo.Result[struct{}, string] {
	__ardTry4 := self.Prepare(args)
	if __ardTry4.IsErr() {
		return ardgo.Err[struct{}, string](__ardTry4.UnwrapErr())
	}
	prepared := __ardTry4.UnwrapOk()
	return execute(self.Conn, prepared.String, prepared.Values)
}

type Database struct {
	Db   any
	Path string
}

func (self Database) Begin() ardgo.Result[Transaction, string] {
	__ardTry5 := beginTx(self.Db)
	if __ardTry5.IsErr() {
		return ardgo.Err[Transaction, string](__ardTry5.UnwrapErr())
	}
	tx := __ardTry5.UnwrapOk()
	return ardgo.Ok[Transaction, string](Transaction{Tx: tx})
}

func (self Database) Close() ardgo.Result[struct{}, string] {
	return closeDb(self.Db)
}

func (self Database) Exec(sql string) ardgo.Result[struct{}, string] {
	return execute(self.Db, sql, list.New[any]())
}

func (self Database) Query(sql string) Query {
	return Query{Conn: self.Db, Params: ExtractParams(sql), String: sql}
}

func connect(connectionString string) ardgo.Result[any, string] {
	result, err := ardgo.CallExtern("SqlCreateConnection", connectionString)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[any, string]](result)
}

func closeDb(db any) ardgo.Result[struct{}, string] {
	result, err := ardgo.CallExtern("SqlClose", db)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[struct{}, string]](result)
}

func execute(conn any, sql string, values []any) ardgo.Result[struct{}, string] {
	result, err := ardgo.CallExtern("SqlExecute", conn, sql, values)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[struct{}, string]](result)
}

func runQuery(conn any, sql string, values []any) ardgo.Result[[]any, string] {
	result, err := ardgo.CallExtern("SqlQuery", conn, sql, values)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[[]any, string]](result)
}

func beginTx(db any) ardgo.Result[any, string] {
	result, err := ardgo.CallExtern("SqlBeginTx", db)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[any, string]](result)
}

func commitTx(tx any) ardgo.Result[struct{}, string] {
	result, err := ardgo.CallExtern("SqlCommit", tx)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[struct{}, string]](result)
}

func rollbackTx(tx any) ardgo.Result[struct{}, string] {
	result, err := ardgo.CallExtern("SqlRollback", tx)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[struct{}, string]](result)
}

func ExtractParams(sql string) []string {
	result, err := ardgo.CallExtern("SqlExtractParams", sql)
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[[]string](result)
}

func Open(path string) ardgo.Result[Database, string] {
	__ardTry6 := connect(path)
	if __ardTry6.IsErr() {
		return ardgo.Err[Database, string](__ardTry6.UnwrapErr())
	}
	conn := __ardTry6.UnwrapOk()
	return ardgo.Ok[Database, string](Database{Db: conn, Path: path})
}
