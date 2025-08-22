package vm

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
	_ "github.com/mattn/go-sqlite3"
)

// Database wrapper for sql.DB with additional metadata
type Database struct {
	conn     *sql.DB
	filePath string
}

// SQLiteModule handles ard/sqlite module functions
type SQLiteModule struct{}

func (m *SQLiteModule) Path() string {
	return "ard/sqlite"
}

func (m *SQLiteModule) Program() *checker.Program {
	return nil
}

func (m *SQLiteModule) Handle(vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "open":
		filePath := args[0].Raw().(string)
		conn, err := sql.Open("sqlite3", filePath)
		if err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}

		// Test the connection
		if err := conn.Ping(); err != nil {
			return runtime.MakeErr(runtime.MakeStr(fmt.Sprintf("Failed to connect to database: %s", err)))
		}

		// Create a Database object with the correct type
		database := runtime.MakeStruct(checker.DatabaseDef, map[string]*runtime.Object{
			"__conn":     runtime.MakeDynamic(conn),
			"__filePath": runtime.MakeStr(filePath),
		})
		return runtime.MakeOk(database)
	default:
		panic(fmt.Errorf("Unimplemented: sqlite::%s()", call.Name))
	}
}

func (m *SQLiteModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch structName {
	case "Database":
		// Handle static methods on Database struct if any
		panic(fmt.Errorf("Unimplemented: sqlite::Database::%s()", call.Name))
	default:
		panic(fmt.Errorf("Unimplemented: sqlite::%s::%s()", structName, call.Name))
	}
}

// evalDatabaseMethod handles instance methods on Database objects
func (m *SQLiteModule) evalDatabaseMethod(database *runtime.Object, method *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	// Get the Database struct from the object
	conn, ok := database.Struct_Get("__conn").Raw().(*sql.DB)
	if !ok {
		panic(fmt.Errorf("SQLite Error: Database object is not correctly formatted"))
	}

	switch method.Name {
	case "exec":
		// fn exec(sql: Str) Void!Str
		sql := args[0].AsString()

		// Execute the SQL
		_, err := conn.Exec(sql)
		if err != nil {
			// Return Err(Str)
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}

		// Return Ok(Void)
		return runtime.MakeOk(runtime.Void())
	case "insert":
		// fn insert(table: Str, values: [Str: Dynamic]) Result<Dynamic, Str>
		tableName := args[0].AsString()
		valuesMap := args[1]

		// Extract fields from the map
		mapData, ok := valuesMap.Raw().(map[string]*runtime.Object)
		if !ok {
			errorMsg := runtime.MakeStr("SQLite Error: ins expects a map object")
			return runtime.MakeErr(errorMsg)
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
		sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			tableName,
			strings.Join(columns, ", "),
			strings.Join(placeholders, ", "),
		)

		// Execute the INSERT
		_, err := conn.Exec(sql, values...)
		if err != nil {
			// Return Err(Str)
			errorMsg := runtime.MakeStr(err.Error())
			return runtime.MakeErr(errorMsg)
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
			errorMsg := runtime.MakeStr(fmt.Sprintf("Failed to retrieve inserted row: %v", err))
			return runtime.MakeErr(errorMsg)
		}
		defer rows.Close()

		// Get column names from the result
		resultColumns, err := rows.Columns()
		if err != nil {
			errorMsg := runtime.MakeStr(fmt.Sprintf("Failed to get result columns: %v", err))
			return runtime.MakeErr(errorMsg)
		}

		if !rows.Next() {
			errorMsg := runtime.MakeStr("Failed to find inserted row")
			return runtime.MakeErr(errorMsg)
		}

		// Scan the row data
		scanValues := make([]any, len(resultColumns))
		scanTargets := make([]any, len(resultColumns))
		for i := range scanValues {
			scanTargets[i] = &scanValues[i]
		}

		if err := rows.Scan(scanTargets...); err != nil {
			errorMsg := runtime.MakeStr(fmt.Sprintf("Failed to scan inserted row: %v", err))
			return runtime.MakeErr(errorMsg)
		}

		// Build result map
		resultMap := make(map[string]any)
		for i, columnName := range resultColumns {
			resultMap[columnName] = scanValues[i]
		}

		// Return Ok(Dynamic) with the full row data
		dynamicResult := runtime.MakeDynamic(resultMap)
		return runtime.MakeOk(dynamicResult)
	case "update":
		// fn update(table: Str, where: Str, values: [Str:Dynamic]) Result<Dynamic, Str>
		tableName := args[0].AsString()
		whereClause := args[1].AsString()
		valuesMap := args[2]

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
		sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
			tableName,
			strings.Join(setPairs, ", "),
			whereClause,
		)

		// Execute the UPDATE
		_, err := conn.Exec(sql, values...)
		if err != nil {
			errorMsg := runtime.MakeStr(fmt.Sprintf("Failed to update row: %s", err))
			return runtime.MakeErr(errorMsg)
		}

		// Build SELECT query to get the updated row(s) back
		selectSQL := fmt.Sprintf("SELECT * FROM %s WHERE %s",
			tableName,
			whereClause,
		)

		// Execute the SELECT to get the updated row
		rows, err := conn.Query(selectSQL)
		if err != nil {
			errorMsg := runtime.MakeStr(fmt.Sprintf("Failed to retrieve updated row: %s", err))
			return runtime.MakeErr(errorMsg)
		}
		defer rows.Close()

		// Get column names from the result
		resultColumns, err := rows.Columns()
		if err != nil {
			errorMsg := runtime.MakeStr(fmt.Sprintf("Failed to get result columns: %s", err))
			return runtime.MakeErr(errorMsg)
		}

		if !rows.Next() {
			errorMsg := runtime.MakeStr("No records found matching the where clause")
			return runtime.MakeErr(errorMsg)
		}

		// Scan the row data
		scanValues := make([]any, len(resultColumns))
		scanTargets := make([]any, len(resultColumns))
		for i := range scanValues {
			scanTargets[i] = &scanValues[i]
		}

		if err := rows.Scan(scanTargets...); err != nil {
			errorMsg := runtime.MakeStr(fmt.Sprintf("Failed to scan updated row: %s", err))
			return runtime.MakeErr(errorMsg)
		}

		// Build result map
		resultMap := make(map[string]any)
		for i, columnName := range resultColumns {
			resultMap[columnName] = scanValues[i]
		}

		// Return Ok(Dynamic) with the updated row data
		return runtime.MakeOk(runtime.MakeDynamic(resultMap))
	case "get":
		// fn get(table: Str, where: Str) [T]
		tableName := args[0].AsString()
		whereClause := args[1].AsString()

		resultType := method.Type().(*checker.Result)
		// Get the target struct type from the method's return type
		listType := resultType.Val().(*checker.List)
		inner := listType.Of()

		// Handle generic types by resolving to actual type
		for anyType, isAny := inner.(*checker.Any); isAny; anyType, isAny = inner.(*checker.Any) {
			if anyType.Actual() == nil {
				break
			}
			inner = anyType.Actual()
		}

		targetStructType, ok := inner.(*checker.StructDef)
		if !ok {
			err := runtime.MakeStr(fmt.Errorf("SQLite Error: get method expects a struct type, got %T", inner).Error())
			return runtime.MakeErr(err)
		}

		// Build SELECT statement
		var sql string
		if whereClause == "" {
			sql = fmt.Sprintf("SELECT * FROM %s", tableName)
		} else {
			sql = fmt.Sprintf("SELECT * FROM %s WHERE %s", tableName, whereClause)
		}

		// Execute the query
		rows, err := conn.Query(sql)
		if err != nil {
			err := fmt.Errorf("SQLite Error: failed to execute query: %v", err)
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}
		defer rows.Close()

		// Get column names
		columns, err := rows.Columns()
		if err != nil {
			err := fmt.Errorf("SQLite Error: failed to get column names: %v", err)
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}

		// Build result list
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
				err := fmt.Errorf("SQLite Error: failed to scan row: %v", err)
				return runtime.MakeErr(runtime.MakeStr(err.Error()))
			}

			// Map columns to struct fields
			structFields := make(map[string]*runtime.Object)
			for i, columnName := range columns {
				// Get the field type from the target struct
				fieldType, exists := targetStructType.Fields[columnName]
				if !exists {
					// Skip columns that don't exist in the struct
					continue
				}

				// Convert the raw value to the appropriate type
				var fieldValue any
				var actualFieldType checker.Type = fieldType
				rawValue := values[i]

				// Check if field type is Maybe
				if maybeType, isMaybe := fieldType.(*checker.Maybe); isMaybe {
					actualFieldType = maybeType.Of()
					if rawValue == nil {
						// NULL value -> create none
						fieldValue = nil
					} else {
						// Non-NULL value -> create some(value) after conversion
						var convertedValue any
						switch actualFieldType {
						case checker.Int:
							if v, ok := rawValue.(int64); ok {
								convertedValue = int(v)
							} else {
								convertedValue = rawValue
							}
						case checker.Str:
							if v, ok := rawValue.([]byte); ok {
								convertedValue = string(v)
							} else {
								convertedValue = rawValue
							}
						case checker.Bool:
							if v, ok := rawValue.(int64); ok {
								convertedValue = v != 0
							} else {
								convertedValue = rawValue
							}
						case checker.Float:
							convertedValue = rawValue
						default:
							convertedValue = rawValue
						}
						fieldValue = convertedValue
					}
				} else {
					// Non-Maybe field
					if rawValue == nil {
						fieldValue = nil
					} else {
						switch fieldType {
						case checker.Int:
							if v, ok := rawValue.(int64); ok {
								fieldValue = int(v)
							} else {
								fieldValue = rawValue
							}
						case checker.Str:
							if v, ok := rawValue.([]byte); ok {
								fieldValue = string(v)
							} else {
								fieldValue = rawValue
							}
						case checker.Bool:
							if v, ok := rawValue.(int64); ok {
								fieldValue = v != 0
							} else {
								fieldValue = rawValue
							}
						case checker.Float:
							fieldValue = rawValue
						default:
							fieldValue = rawValue
						}
					}
				}

				structFields[columnName] = runtime.Make(fieldValue, fieldType)
			}

			// Create the struct object
			structObj := runtime.MakeStruct(targetStructType, structFields)
			results = append(results, structObj)
		}

		if err := rows.Err(); err != nil {
			err := fmt.Errorf("SQLite Error: row iteration error: %v", err)
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}

		// Return list of structs
		return runtime.MakeOk(runtime.MakeList(listType.Of(), results...))
	case "delete":
		// fn delete(table: Str, where: Str) Result<Bool, Str>
		tableName := args[0].AsString()
		whereClause := args[1].AsString()

		// Construct SQL
		var sql string
		if whereClause == "" {
			sql = fmt.Sprintf("DELETE FROM %s", tableName)
		} else {
			sql = fmt.Sprintf("DELETE FROM %s WHERE %s", tableName, whereClause)
		}

		// Execute the DELETE
		result, err := conn.Exec(sql)
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
	case "close":
		// fn close() Result<Void, Str>
		// Close the database connection
		err := conn.Close()
		if err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}

		// Return Ok(void)
		return runtime.MakeOk(runtime.Void())
	case "count":
		// fn count(table: Str, where: Str) Result<Int, Str>
		tableName := args[0].AsString()
		whereClause := args[1].AsString()

		// Construct SQL
		var sql string
		if whereClause == "" {
			sql = fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
		} else {
			sql = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", tableName, whereClause)
		}

		// Execute the COUNT query
		var count int64
		err := conn.QueryRow(sql).Scan(&count)
		if err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}

		// Return Ok(count)
		return runtime.MakeOk(runtime.MakeInt(int(count)))
	case "exists":
		// fn exists(table: Str, where: Str) Result<Bool, Str>
		tableName := args[0].AsString()
		whereClause := args[1].AsString()

		// Construct SQL - using EXISTS for efficiency
		var sql string
		if whereClause == "" {
			sql = fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s)", tableName)
		} else {
			sql = fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE %s)", tableName, whereClause)
		}

		// Execute the EXISTS query
		var exists bool
		err := conn.QueryRow(sql).Scan(&exists)
		if err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}

		// Return Ok(exists)
		return runtime.MakeOk(runtime.MakeBool(exists))
	// case "upsert":
	// 	// fn upsert(table: Str, where: Str, record: $T) Result<Bool, Str>
	// 	tableName := args[0].AsString()
	// 	whereClause := args[1].AsString()
	// 	structObj := args[2]

	// 	resultType := method.Type().(*checker.Result)

	// 	// Extract fields from the struct
	// 	structFields, ok := structObj.raw.(map[string]*runtime.Object)
	// 	if !ok {
	// 		errorMsg := &object{"Upsert expects a struct object", resultType.Err()}
	// 		return runtime.MakeErr(errorMsg, resultType)
	// 	}

	// 	// Check if record exists
	// 	checkSQL := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE %s)", tableName, whereClause)
	// 	var exists bool
	// 	err := db.conn.QueryRow(checkSQL).Scan(&exists)
	// 	if err != nil {
	// 		errorMsg := &object{err.Error(), resultType.Err()}
	// 		return runtime.MakeErr(errorMsg, resultType)
	// 	}

	// 	if exists {
	// 		// Record exists - perform UPDATE
	// 		var setPairs []string
	// 		var values []interface{}

	// 		// Sort column names for consistent ordering
	// 		var columns []string
	// 		for columnName := range structFields {
	// 			columns = append(columns, columnName)
	// 		}
	// 		sort.Strings(columns)

	// 		// Build SET clauses
	// 		for _, columnName := range columns {
	// 			fieldObj := structFields[columnName]
	// 			setPairs = append(setPairs, fmt.Sprintf("%s = ?", columnName))

	// 			// Handle Maybe types for update
	// 			if _, isMaybe := fieldObj._type.(*checker.Maybe); isMaybe {
	// 				if fieldObj.raw == nil {
	// 					values = append(values, nil)
	// 				} else {
	// 					values = append(values, fieldObj.raw)
	// 				}
	// 			} else {
	// 				values = append(values, fieldObj.raw)
	// 			}
	// 		}

	// 		// Construct UPDATE SQL
	// 		sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
	// 			tableName,
	// 			strings.Join(setPairs, ", "),
	// 			whereClause,
	// 		)

	// 		// Execute the UPDATE
	// 		_, err := db.conn.Exec(sql, values...)
	// 		if err != nil {
	// 			errorMsg := &object{err.Error(), resultType.Err()}
	// 			return runtime.MakeErr(errorMsg, resultType)
	// 		}
	// 	} else {
	// 		// Record doesn't exist - perform INSERT
	// 		var columns []string
	// 		var placeholders []string
	// 		var values []any

	// 		// Sort column names for consistent ordering
	// 		for columnName := range structFields {
	// 			columns = append(columns, columnName)
	// 		}
	// 		sort.Strings(columns)

	// 		// Build values in same order as columns
	// 		for _, columnName := range columns {
	// 			fieldObj := structFields[columnName]
	// 			placeholders = append(placeholders, "?")

	// 			// Handle Maybe types
	// 			if _, isMaybe := fieldObj._type.(*checker.Maybe); isMaybe {
	// 				if fieldObj.raw == nil {
	// 					values = append(values, nil)
	// 				} else {
	// 					values = append(values, fieldObj.raw)
	// 				}
	// 			} else {
	// 				values = append(values, fieldObj.raw)
	// 			}
	// 		}

	// 		// Construct INSERT SQL
	// 		sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
	// 			tableName,
	// 			strings.Join(columns, ", "),
	// 			strings.Join(placeholders, ", "),
	// 		)

	// 		// Execute the INSERT
	// 		_, err := db.conn.Exec(sql, values...)
	// 		if err != nil {
	// 			errorMsg := &object{err.Error(), resultType.Err()}
	// 			return runtime.MakeErr(errorMsg, resultType)
	// 		}
	// 	}

	// 	// Return Ok(true) - upsert succeeded
	// 	return runtime.MakeOk(&object{true, checker.Bool}, resultType)
	case "query":
		// fn query(sql: Str) Dynamic!Str
		sql := args[0].AsString()

		// Execute the query
		rows, err := conn.Query(sql)
		if err != nil {
			err := fmt.Errorf("SQLite Error: failed to execute query: %v", err)
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}
		defer rows.Close()

		// Get column names
		columns, err := rows.Columns()
		if err != nil {
			err := fmt.Errorf("SQLite Error: failed to get column names: %v", err)
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
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
				err := fmt.Errorf("SQLite Error: failed to scan row: %v", err)
				return runtime.MakeErr(runtime.MakeStr(err.Error()))
			}

			// Create map for this row
			rowMap := make(map[string]interface{})
			for i, columnName := range columns {
				rowMap[columnName] = values[i]
			}

			results = append(results, rowMap)
		}

		if err := rows.Err(); err != nil {
			err := fmt.Errorf("SQLite Error: row iteration error: %v", err)
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}

		// Return Dynamic object containing list of row maps
		return runtime.MakeOk(runtime.MakeDynamic(results))
	case "first":
		// fn first(sql: Str) Dynamic!Str
		sql := args[0].AsString()

		// Execute the query
		rows, err := conn.Query(sql)
		if err != nil {
			err := fmt.Errorf("SQLite Error: failed to execute query: %v", err)
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}
		defer rows.Close()

		// Get column names
		columns, err := rows.Columns()
		if err != nil {
			err := fmt.Errorf("SQLite Error: failed to get column names: %v", err)
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
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
				err := fmt.Errorf("SQLite Error: failed to scan row: %v", err)
				return runtime.MakeErr(runtime.MakeStr(err.Error()))
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
			err := fmt.Errorf("SQLite Error: row iteration error: %v", err)
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}

		// No rows found - return nil as Dynamic
		return runtime.MakeOk(runtime.MakeDynamic(nil))
	default:
		panic(fmt.Errorf("Unimplemented: Database.%s()", method.Name))
	}
}
