package ffi

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
	_ "github.com/mattn/go-sqlite3"
)

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

	// Return the raw db as Dynamic
	return runtime.MakeOk(runtime.MakeDynamic(db))
}

// detectDriver identifies the SQL driver based on the connection string
func detectDriver(connStr string) string {
	connStr = strings.TrimSpace(connStr)

	// Check for postgres
	if strings.HasPrefix(connStr, "postgres://") || strings.HasPrefix(connStr, "postgresql://") {
		return "postgres"
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

	conn, ok := args[0].Raw().(*sql.DB)
	if !ok {
		panic(fmt.Errorf("SQL Error: invalid connection object"))
	}

	err := conn.Close()
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
	seen := make(map[string]bool)

	for _, token := range tokens {
		if strings.HasPrefix(token, "@") && len(token) > 1 {
			// Extract parameter name, removing @ prefix and any trailing punctuation
			paramName := strings.TrimLeft(token[1:], "@")
			paramName = strings.TrimRight(paramName, ".,;:!?")

			if paramName != "" && !seen[paramName] {
				paramNames = append(paramNames, paramName)
				seen[paramName] = true
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

// executes a query and returns the rows
func SqlQuery(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 3 {
		panic(fmt.Errorf("query_run expects 3 arguments, got %d", len(args)))
	}

	conn, ok := args[0].Raw().(*sql.DB)
	if !ok {
		panic(fmt.Errorf("SQL Error: invalid connection object"))
	}

	sqlStr := args[1].AsString()
	valuesListObj := args[2]

	// Extract values from the list
	valuesList := valuesListObj.AsList()
	var values []any
	for _, valueObj := range valuesList {
		// Convert Ard Value union type to Go value
		values = append(values, valueObj.GoValue())
	}

	rows, err := conn.Query(sqlStr, values...)
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
		values := make([]any, len(columns))
		scanTargets := make([]any, len(columns))
		for i := range values {
			scanTargets[i] = &values[i]
		}

		// Scan the row
		if err := rows.Scan(scanTargets...); err != nil {
			return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("failed to scan row: %v", err)))
		}

		// Create map for this row
		rowMap := make(map[string]any)
		for i, columnName := range columns {
			rowMap[columnName] = values[i]
		}

		results = append(results, runtime.MakeDynamic(rowMap))
	}

	if err := rows.Err(); err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("row iteration error: %v", err)))
	}

	// Return Dynamic object containing list of row maps
	return runtime.MakeOk(runtime.MakeList(checker.Dynamic, results...))
}

// executes a query and doesn't return rows
func SqlExecute(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 3 {
		panic(fmt.Errorf("query_run expects 3 arguments, got %d", len(args)))
	}

	conn, ok := args[0].Raw().(*sql.DB)
	if !ok {
		return runtime.MakeErr(runtime.MakeStr("SQL Error: invalid connection object"))
	}

	sqlStr := args[1].AsString()

	// Extract values from the list
	var values []any
	for _, valueObj := range args[2].AsList() {
		// Convert Ard Value union type to Go value
		values = append(values, valueObj.GoValue())
	}

	_, err := conn.Exec(sqlStr, values...)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	return runtime.MakeOk(runtime.Void())
}
