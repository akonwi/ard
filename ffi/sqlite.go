package ffi

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
	_ "github.com/mattn/go-sqlite3"
)

// SQLite FFI functions

// Note: SqliteOpen is not needed - the open function is implemented in sqlite.ard
// Only the core _create_connection function is implemented here

// SqliteCreateConnection creates a new SQLite database connection
func SqliteCreateConnection(vm runtime.VM, args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("create_connection expects 1 argument, got %d", len(args)))
	}

	filePath := args[0].Raw().(string)
	conn, err := sql.Open("sqlite3", filePath)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	// Test the connection
	if err := conn.Ping(); err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("Failed to connect to database: %s", err)))
	}

	// Return the raw connection as Dynamic
	return runtime.MakeOk(runtime.MakeDynamic(conn))
}

// SqliteClose closes a database connection
func SqliteClose(vm runtime.VM, args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("close expects 1 argument, got %d", len(args)))
	}

	conn, ok := args[0].Raw().(*sql.DB)
	if !ok {
		panic(fmt.Errorf("SQLite Error: invalid connection object"))
	}

	err := conn.Close()
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	return runtime.MakeOk(runtime.Void())
}

// SqliteExec executes a SQL statement
func SqliteExec(vm runtime.VM, args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 2 {
		panic(fmt.Errorf("exec expects 2 arguments, got %d", len(args)))
	}

	conn, ok := args[0].Raw().(*sql.DB)
	if !ok {
		panic(fmt.Errorf("SQLite Error: invalid connection object"))
	}

	sqlStr := args[1].AsString()
	_, err := conn.Exec(sqlStr)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	return runtime.MakeOk(runtime.Void())
}

// SqliteQuery executes a query and returns all rows
func SqliteQuery(vm runtime.VM, args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 2 {
		panic(fmt.Errorf("query expects 2 arguments, got %d", len(args)))
	}

	conn, ok := args[0].Raw().(*sql.DB)
	if !ok {
		panic(fmt.Errorf("SQLite Error: invalid connection object"))
	}

	sqlStr := args[1].AsString()
	rows, err := conn.Query(sqlStr)
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
	var results []interface{}

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
		rowMap := make(map[string]interface{})
		for i, columnName := range columns {
			rowMap[columnName] = values[i]
		}

		results = append(results, rowMap)
	}

	if err := rows.Err(); err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("row iteration error: %v", err)))
	}

	// Return Dynamic object containing list of row maps
	return runtime.MakeOk(runtime.MakeDynamic(results))
}

// SqliteFirst executes a query and returns the first row
func SqliteFirst(vm runtime.VM, args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 2 {
		panic(fmt.Errorf("first expects 2 arguments, got %d", len(args)))
	}

	conn, ok := args[0].Raw().(*sql.DB)
	if !ok {
		panic(fmt.Errorf("SQLite Error: invalid connection object"))
	}

	sqlStr := args[1].AsString()
	rows, err := conn.Query(sqlStr)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("failed to execute query: %v", err)))
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("failed to get column names: %v", err)))
	}

	// Process only the first row
	if rows.Next() {
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

		// Return Dynamic object containing the first row map
		return runtime.MakeOk(runtime.MakeDynamic(rowMap))
	}

	if err := rows.Err(); err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("row iteration error: %v", err)))
	}

	// No rows found - return nil as Dynamic
	return runtime.MakeOk(runtime.MakeDynamic(nil))
}

// SqliteInsert inserts a record and returns the inserted row
func SqliteInsert(vm runtime.VM, args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 3 {
		panic(fmt.Errorf("insert expects 3 arguments, got %d", len(args)))
	}

	conn, ok := args[0].Raw().(*sql.DB)
	if !ok {
		panic(fmt.Errorf("SQLite Error: invalid connection object"))
	}

	tableName := args[1].AsString()
	valuesMap := args[2]

	// Extract fields from the map
	mapData, ok := valuesMap.Raw().(map[string]*runtime.Object)
	if !ok {
		return runtime.MakeErr(runtime.MakeStr("SQLite Error: insert expects a map object"))
	}

	// Build INSERT statement
	var columns []string
	var placeholders []string
	var values []any

	// Sort column names for consistent ordering
	for columnName := range mapData {
		columns = append(columns, columnName)
	}
	sort.Strings(columns)

	// Build values in same order as columns
	for _, columnName := range columns {
		fieldObj := mapData[columnName]
		placeholders = append(placeholders, "?")
		values = append(values, fieldObj.GoValue())
	}

	// Construct SQL
	sqlStr := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	// Execute the INSERT
	_, err := conn.Exec(sqlStr, values...)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	// Build SELECT query to get the inserted row back
	var whereConditions []string
	var selectValues []any
	for _, columnName := range columns {
		whereConditions = append(whereConditions, fmt.Sprintf("%s = ?", columnName))
		selectValues = append(selectValues, mapData[columnName].GoValue())
	}

	selectSQL := fmt.Sprintf("SELECT * FROM %s WHERE %s LIMIT 1",
		tableName,
		strings.Join(whereConditions, " AND "),
	)

	// Execute the SELECT to get the inserted row
	rows, err := conn.Query(selectSQL, selectValues...)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("Failed to retrieve inserted row: %v", err)))
	}
	defer rows.Close()

	// Get column names from the result
	resultColumns, err := rows.Columns()
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("Failed to get result columns: %v", err)))
	}

	if !rows.Next() {
		return runtime.MakeErr(runtime.MakeStr("Failed to find inserted row"))
	}

	// Scan the row data
	scanValues := make([]any, len(resultColumns))
	scanTargets := make([]any, len(resultColumns))
	for i := range scanValues {
		scanTargets[i] = &scanValues[i]
	}

	if err := rows.Scan(scanTargets...); err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("Failed to scan inserted row: %v", err)))
	}

	// Build result map
	resultMap := make(map[string]any)
	for i, columnName := range resultColumns {
		resultMap[columnName] = scanValues[i]
	}

	// Return Ok(Dynamic) with the full row data
	return runtime.MakeOk(runtime.MakeDynamic(resultMap))
}

// SqliteUpdate updates records and returns the updated row
func SqliteUpdate(vm runtime.VM, args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 4 {
		panic(fmt.Errorf("update expects 4 arguments, got %d", len(args)))
	}

	conn, ok := args[0].Raw().(*sql.DB)
	if !ok {
		panic(fmt.Errorf("SQLite Error: invalid connection object"))
	}

	tableName := args[1].AsString()
	whereClause := args[2].AsString()
	valuesMap := args[3]

	// Extract fields from the map
	mapData := valuesMap.AsMap()

	// Build UPDATE statement
	var setPairs []string
	var values []interface{}

	// Sort column names for consistent ordering
	var columns []string
	for columnName := range mapData {
		columns = append(columns, columnName)
	}
	sort.Strings(columns)

	// Build SET clauses
	for _, columnName := range columns {
		fieldObj := mapData[columnName]
		setPairs = append(setPairs, fmt.Sprintf("%s = ?", columnName))
		values = append(values, fieldObj.GoValue())
	}

	// Construct SQL
	sqlStr := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		tableName,
		strings.Join(setPairs, ", "),
		whereClause,
	)

	// Execute the UPDATE
	_, err := conn.Exec(sqlStr, values...)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("Failed to update row: %s", err)))
	}

	// Build SELECT query to get the updated row(s) back
	selectSQL := fmt.Sprintf("SELECT * FROM %s WHERE %s",
		tableName,
		whereClause,
	)

	// Execute the SELECT to get the updated row
	rows, err := conn.Query(selectSQL)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("Failed to retrieve updated row: %s", err)))
	}
	defer rows.Close()

	// Get column names from the result
	resultColumns, err := rows.Columns()
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("Failed to get result columns: %s", err)))
	}

	if !rows.Next() {
		return runtime.MakeErr(runtime.MakeStr("No records found matching the where clause"))
	}

	// Scan the row data
	scanValues := make([]any, len(resultColumns))
	scanTargets := make([]any, len(resultColumns))
	for i := range scanValues {
		scanTargets[i] = &scanValues[i]
	}

	if err := rows.Scan(scanTargets...); err != nil {
		return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("Failed to scan updated row: %s", err)))
	}

	// Build result map
	resultMap := make(map[string]any)
	for i, columnName := range resultColumns {
		resultMap[columnName] = scanValues[i]
	}

	// Return Ok(Dynamic) with the updated row data
	return runtime.MakeOk(runtime.MakeDynamic(resultMap))
}

// SqliteDelete deletes records and returns whether any were deleted
func SqliteDelete(vm runtime.VM, args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 3 {
		panic(fmt.Errorf("delete expects 3 arguments, got %d", len(args)))
	}

	conn, ok := args[0].Raw().(*sql.DB)
	if !ok {
		panic(fmt.Errorf("SQLite Error: invalid connection object"))
	}

	tableName := args[1].AsString()
	whereClause := args[2].AsString()

	// Construct SQL
	var sqlStr string
	if whereClause == "" {
		sqlStr = fmt.Sprintf("DELETE FROM %s", tableName)
	} else {
		sqlStr = fmt.Sprintf("DELETE FROM %s WHERE %s", tableName, whereClause)
	}

	// Execute the DELETE
	result, err := conn.Exec(sqlStr)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	// Check if any rows were affected
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	// Return Ok(true) if rows were deleted, Ok(false) if no rows matched
	deleted := rowsAffected > 0
	return runtime.MakeOk(runtime.MakeBool(deleted))
}

// SqliteCount counts records matching the where clause
func SqliteCount(vm runtime.VM, args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 3 {
		panic(fmt.Errorf("count expects 3 arguments, got %d", len(args)))
	}

	conn, ok := args[0].Raw().(*sql.DB)
	if !ok {
		panic(fmt.Errorf("SQLite Error: invalid connection object"))
	}

	tableName := args[1].AsString()
	whereClause := args[2].AsString()

	// Construct SQL
	var sqlStr string
	if whereClause == "" {
		sqlStr = fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	} else {
		sqlStr = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", tableName, whereClause)
	}

	// Execute the COUNT query
	var count int64
	err := conn.QueryRow(sqlStr).Scan(&count)
	if err != nil {
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}

	// Return Ok(count)
	return runtime.MakeOk(runtime.MakeInt(int(count)))
}
