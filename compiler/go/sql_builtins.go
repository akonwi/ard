package ardgo

import "github.com/akonwi/ard/ffi"

func builtinSQLValues(values []any) []any {
	out := make([]any, len(values))
	for i, value := range values {
		switch typed := builtinDynamicValue(value).(type) {
		case struct{}:
			out[i] = nil
		default:
			out[i] = typed
		}
	}
	return out
}

func builtinSqlCreateConnection(connectionString string) Result[any, string] {
	value, err := ffi.SqlCreateConnection(connectionString)
	if err != nil {
		return Err[any, string](err.Error())
	}
	return Ok[any, string](value)
}

func builtinSqlClose(handle any) Result[struct{}, string] {
	if err := ffi.SqlClose(handle); err != nil {
		return errStringResult(err)
	}
	return okVoidResult()
}

func builtinSqlExecute(conn any, sqlStr string, values []any) Result[struct{}, string] {
	if err := ffi.SqlExecute(conn, sqlStr, builtinSQLValues(values)); err != nil {
		return errStringResult(err)
	}
	return okVoidResult()
}

func builtinSqlQuery(conn any, sqlStr string, values []any) Result[[]any, string] {
	rows, err := ffi.SqlQuery(conn, sqlStr, builtinSQLValues(values))
	if err != nil {
		return Err[[]any, string](err.Error())
	}
	return Ok[[]any, string](rows)
}

func builtinSqlBeginTx(handle any) Result[any, string] {
	value, err := ffi.SqlBeginTx(handle)
	if err != nil {
		return Err[any, string](err.Error())
	}
	return Ok[any, string](value)
}

func builtinSqlCommit(handle any) Result[struct{}, string] {
	if err := ffi.SqlCommit(handle); err != nil {
		return errStringResult(err)
	}
	return okVoidResult()
}

func builtinSqlRollback(handle any) Result[struct{}, string] {
	if err := ffi.SqlRollback(handle); err != nil {
		return errStringResult(err)
	}
	return okVoidResult()
}
