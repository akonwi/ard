package ffi

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

type sqlConnection struct {
	db     *sql.DB
	driver string
}

type sqlTransaction struct {
	tx     *sql.Tx
	driver string
}

// Generic SQL FFI functions supporting multiple database drivers
// (postgres, mysql, sqlite, etc.)

// SqlCreateConnection creates a new database connection from a connection string
// Supports connection strings for various drivers:
// - SQLite: "file:test.db" or "test.db"
// - PostgreSQL: "postgres://user:password@localhost:5432/dbname"
// - MySQL: "user:password@tcp(localhost:3306)/dbname"
func SqlCreateConnection(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("open expects 1 argument, got %d", len(args)))
	}

	connectionString := args[0].Raw().(string)

	// Determine driver from connection string
	driver := detectDriver(connectionString)

	db, err := sql.Open(driver, connectionString)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("Failed to connect to database: %s", err)))
	}

	conn := &sqlConnection{db: db, driver: driver}

	// Return the connection as Dynamic
	return runtime.MakeOk(runtime.MakeDynamic(conn))
}

// detectDriver identifies the SQL driver based on the connection string
func detectDriver(connStr string) string {
	connStr = strings.TrimSpace(connStr)

	// Check for postgres
	if strings.HasPrefix(connStr, "postgres://") || strings.HasPrefix(connStr, "postgresql://") {
		return "pgx"
	}

	// Check for mysql
	if strings.Contains(connStr, "@tcp(") || strings.Contains(connStr, "@unix(") {
		return "mysql"
	}

	// Default to sqlite for any other format (file paths, file: URIs, etc.)
	return "sqlite3"
}

// SqlClose closes a database connection
func SqlClose(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("close expects 1 argument, got %d", len(args)))
	}

	conn, ok := args[0].Raw().(*sqlConnection)
	if !ok {
		panic(fmt.Errorf("SQL Error: invalid connection object"))
	}

	err := conn.db.Close()
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	return runtime.MakeOk(runtime.Void())
}

// Extract parameter names from a sql expression in the order they appear
func SqlExtractParams(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("extract_params expects 1 argument, got %d", len(args)))
	}

	sqlStr := args[0].AsString()

	// Split SQL into tokens by multiple delimiters
	delimiters := []string{" ", "(", ")", ",", ";", "=", "<", ">", "!", "\t", "\n", "\r"}
	tokens := splitSQLByMultipleDelimiters(sqlStr, delimiters)

	var paramNames []string

	for _, token := range tokens {
		if strings.HasPrefix(token, "@") && len(token) > 1 {
			// Extract parameter name, removing @ prefix and any trailing punctuation
			paramName := strings.TrimLeft(token[1:], "@")
			paramName = strings.TrimRight(paramName, ".,;:!?")

			if paramName != "" {
				paramNames = append(paramNames, paramName)
			}
		}
	}

	// Convert to runtime objects
	var result []*runtime.Object
	for _, paramName := range paramNames {
		result = append(result, runtime.MakeStr(paramName))
	}

	return runtime.MakeList(checker.Str, result...)
}

// splitSQLByMultipleDelimiters is a helper function to split string by multiple delimiters
// Used by both SQL and SQLite extract_params implementations
func splitSQLByMultipleDelimiters(s string, delimiters []string) []string {
	// Replace all delimiters with a single delimiter, then split
	result := s
	for _, delimiter := range delimiters {
		result = strings.ReplaceAll(result, delimiter, " ")
	}

	// Split by space and filter out empty strings
	tokens := strings.Split(result, " ")
	var nonEmpty []string
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token != "" {
			nonEmpty = append(nonEmpty, token)
		}
	}

	return nonEmpty
}

// sqlRunner interface abstracts both *sql.DB and *sql.Tx for query execution
type sqlRunner interface {
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

func normalizePlaceholders(sqlStr, driver string) string {
	if driver != "pgx" {
		return sqlStr
	}

	// PostgreSQL requires positional placeholders like $1, $2, ...
	// Support both Ard's @name placeholders and ? placeholders.
	var out strings.Builder
	out.Grow(len(sqlStr) + 16)
	index := 1
	for i := 0; i < len(sqlStr); i++ {
		ch := sqlStr[i]

		if ch == '@' {
			j := i + 1
			for j < len(sqlStr) {
				c := sqlStr[j]
				isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
				isDigit := c >= '0' && c <= '9'
				if isAlpha || isDigit || c == '_' {
					j++
					continue
				}
				break
			}

			if j > i+1 {
				out.WriteString(fmt.Sprintf("$%d", index))
				index++
				i = j - 1
				continue
			}
		}

		if ch == '?' {
			out.WriteString(fmt.Sprintf("$%d", index))
			index++
			continue
		}
		out.WriteByte(ch)
	}

	return out.String()
}

func resolveRunner(raw any) (sqlRunner, string, bool) {
	if conn, ok := raw.(*sqlConnection); ok {
		return conn.db, conn.driver, true
	}
	if tx, ok := raw.(*sqlTransaction); ok {
		return tx.tx, tx.driver, true
	}
	return nil, "", false
}

func sqlArgValue(valueObj *runtime.Object) any {
	if valueObj == nil {
		return nil
	}

	if nested, ok := valueObj.Raw().(*runtime.Object); ok {
		return sqlArgValue(nested)
	}

	return valueObj.Raw()
}

// executeQuery is a helper that works with both *sql.DB and *sql.Tx
func executeQuery(runner sqlRunner, sqlStr string, values []any) *runtime.Object {
	rows, err := runner.Query(sqlStr, values...)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("failed to execute query: %v", err)))
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("failed to get column names: %v", err)))
	}

	// Build result list - each row is a map[string]interface{}
	var results []*runtime.Object

	for rows.Next() {
		// Create scan targets for each column
		scanValues := make([]any, len(columns))
		scanTargets := make([]any, len(columns))
		for i := range scanValues {
			scanTargets[i] = &scanValues[i]
		}

		// Scan the row
		if err := rows.Scan(scanTargets...); err != nil {
			return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("failed to scan row: %v", err)))
		}

		// Create map for this row
		rowMap := make(map[string]any)
		for i, columnName := range columns {
			rowMap[columnName] = scanValues[i]
		}

		results = append(results, runtime.MakeDynamic(rowMap))
	}

	if err := rows.Err(); err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("row iteration error: %v", err)))
	}

	// Return Dynamic object containing list of row maps
	return runtime.MakeOk(runtime.MakeList(checker.Dynamic, results...))
}

// executes a query and returns the rows
func SqlQuery(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 3 {
		panic(fmt.Errorf("query_run expects 3 arguments, got %d", len(args)))
	}

	connRaw := args[0].Raw()

	runner, driver, ok := resolveRunner(connRaw)
	if !ok {
		return runtime.MakeStr(fmt.Sprintf("SQL Error: invalid connection object: %T", connRaw))
	}

	sqlStr := normalizePlaceholders(args[1].AsString(), driver)
	valuesListObj := args[2]

	// Extract values from the list
	valuesList := valuesListObj.AsList()
	var values []any
	for _, valueObj := range valuesList {
		values = append(values, sqlArgValue(valueObj))
	}

	return executeQuery(runner, sqlStr, values)
}

// executes a query and doesn't return rows
func SqlExecute(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 3 {
		panic(fmt.Errorf("query_run expects 3 arguments, got %d", len(args)))
	}

	connRaw := args[0].Raw()

	runner, driver, ok := resolveRunner(connRaw)
	if !ok {
		return runtime.MakeErr(runtime.MakeStr("SQL Error: invalid connection object"))
	}

	sqlStr := normalizePlaceholders(args[1].AsString(), driver)

	// Extract values from the list
	var values []any
	for _, valueObj := range args[2].AsList() {
		values = append(values, sqlArgValue(valueObj))
	}

	_, err := runner.Exec(sqlStr, values...)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	return runtime.MakeOk(runtime.Void())
}

// SqlBeginTx begins a new transaction
func SqlBeginTx(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("begin expects 1 argument, got %d", len(args)))
	}

	db, ok := args[0].Raw().(*sql.DB)
	if !ok {
		if conn, isConn := args[0].Raw().(*sqlConnection); isConn {
			db = conn.db
			ok = true
		}
	}
	if !ok {
		return runtime.MakeErr(runtime.MakeStr("SQL Error: invalid connection object"))
	}

	tx, err := db.Begin()
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("failed to begin transaction: %v", err)))
	}

	driver := ""
	if conn, ok := args[0].Raw().(*sqlConnection); ok {
		driver = conn.driver
	}

	return runtime.MakeOk(runtime.MakeDynamic(&sqlTransaction{tx: tx, driver: driver}))
}

// SqlCommit commits a transaction
func SqlCommit(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("commit expects 1 argument, got %d", len(args)))
	}

	wrappedTx, ok := args[0].Raw().(*sqlTransaction)
	if !ok {
		return runtime.MakeErr(runtime.MakeStr("SQL Error: invalid transaction object"))
	}

	err := wrappedTx.tx.Commit()
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("failed to commit transaction: %s", err)))
	}

	return runtime.MakeOk(runtime.Void())
}

// SqlRollback rolls back a transaction
func SqlRollback(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("rollback expects 1 argument, got %d", len(args)))
	}

	wrappedTx, ok := args[0].Raw().(*sqlTransaction)
	if !ok {
		return runtime.MakeErr(runtime.MakeStr("SQL Error: invalid transaction object"))
	}

	err := wrappedTx.tx.Rollback()
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("failed to rollback transaction: %s", err.Error())))
	}

	return runtime.MakeOk(runtime.Void())
}
