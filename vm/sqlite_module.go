package vm

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/akonwi/ard/checker"
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

func (m *SQLiteModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "open":
		resultType := call.Type().(*checker.Result)
		filePath := args[0].raw.(string)
		conn, err := sql.Open("sqlite3", filePath)
		if err != nil {
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}

		// Test the connection
		if err := conn.Ping(); err != nil {
			return makeErr(&object{fmt.Sprintf("Failed to connect to database: %s", err), resultType.Err()}, resultType)
		}

		// Create our Database wrapper
		database := &Database{
			conn:     conn,
			filePath: filePath,
		}

		// Create a Database object with the correct type
		dbObject := &object{
			raw:   database,
			_type: resultType.Val(),
		}

		return makeOk(dbObject, resultType)
	default:
		panic(fmt.Errorf("Unimplemented: sqlite::%s()", call.Name))
	}
}

func (m *SQLiteModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch structName {
	case "Database":
		// Handle static methods on Database struct if any
		panic(fmt.Errorf("Unimplemented: sqlite::Database::%s()", call.Name))
	default:
		panic(fmt.Errorf("Unimplemented: sqlite::%s::%s()", structName, call.Name))
	}
}

// evalDatabaseMethod handles instance methods on Database objects
func (m *SQLiteModule) evalDatabaseMethod(database *object, method *checker.FunctionCall, args []*object) *object {
	// Get the Database struct from the object
	db, ok := database.raw.(*Database)
	if !ok {
		panic(fmt.Errorf("SQLite Error: Database object is not correctly formatted"))
	}

	switch method.Name {
	case "exec":
		// fn exec(sql: Str) Void!Str
		resultType := method.Type().(*checker.Result)
		sql := args[0].raw.(string)

		// Execute the SQL
		_, err := db.conn.Exec(sql)
		if err != nil {
			// Return Err(Str)
			errorMsg := &object{err.Error(), resultType.Err()}
			return makeErr(errorMsg, resultType)
		}

		// Return Ok(Void)
		return makeOk(void, resultType)
	case "insert":
		// fn insert(table: Str, values: $V) Void!Str
		resultType := method.Type().(*checker.Result)
		tableName := args[0].raw.(string)
		structObj := args[1]

		// Extract fields from the struct
		structFields, ok := structObj.raw.(map[string]*object)
		if !ok {
			errorMsg := &object{"SQLite Error: insert expects a struct object", resultType.Err()}
			return makeErr(errorMsg, resultType)
		}

		// Build INSERT statement
		var columns []string
		var placeholders []string
		var values []any

		// Sort column names for consistent ordering
		for columnName := range structFields {
			columns = append(columns, columnName)
		}
		sort.Strings(columns)

		// Build values in same order as columns
		for _, columnName := range columns {
			fieldObj := structFields[columnName]
			placeholders = append(placeholders, "?")
			values = append(values, fieldObj.raw)
		}

		// Construct SQL
		sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			tableName,
			strings.Join(columns, ", "),
			strings.Join(placeholders, ", "),
		)

		// Execute the INSERT
		_, err := db.conn.Exec(sql, values...)
		if err != nil {
			// Return Err(Str)
			errorMsg := &object{err.Error(), resultType.Err()}
			return makeErr(errorMsg, resultType)
		}

		// Return Ok(Void)
		return makeOk(void, resultType)
	case "update":
		// fn update(table: Str, where: Str, record: $T) Result<Void, Str>
		tableName := args[0].raw.(string)
		whereClause := args[1].raw.(string)
		structObj := args[2]

		// Extract fields from the struct
		structFields, ok := structObj.raw.(map[string]*object)
		if !ok {
			return makeErr(&object{"Update expects a struct object", checker.Str}, method.Type().(*checker.Result))
		}

		// Build UPDATE statement
		var setPairs []string
		var values []interface{}

		// Sort column names for consistent ordering
		var columns []string
		for columnName := range structFields {
			columns = append(columns, columnName)
		}
		sort.Strings(columns)

		// Build SET clauses
		for _, columnName := range columns {
			fieldObj := structFields[columnName]
			setPairs = append(setPairs, fmt.Sprintf("%s = ?", columnName))

			// Handle Maybe types for update
			if _, isMaybe := fieldObj._type.(*checker.Maybe); isMaybe {
				if fieldObj.raw == nil {
					values = append(values, nil)
				} else {
					values = append(values, fieldObj.raw)
				}
			} else {
				values = append(values, fieldObj.raw)
			}
		}

		// Construct SQL
		sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
			tableName,
			strings.Join(setPairs, ", "),
			whereClause,
		)

		// Execute the UPDATE
		result, err := db.conn.Exec(sql, values...)
		if err != nil {
			return makeErr(&object{err.Error(), checker.Str}, method.Type().(*checker.Result))
		}

		// Check if any rows were affected
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return makeErr(&object{err.Error(), checker.Str}, method.Type().(*checker.Result))
		}

		if rowsAffected == 0 {
			return makeErr(&object{"No records found matching the where clause", checker.Str}, method.Type().(*checker.Result))
		}

		// Return Ok(void)
		return makeOk(&object{nil, checker.Void}, method.Type().(*checker.Result))
	case "get":
		// fn get(table: Str, where: Str) [T]
		tableName := args[0].raw.(string)
		whereClause := args[1].raw.(string)

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
			err := fmt.Errorf("SQLite Error: get method expects a struct type, got %T", inner)
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}

		// Build SELECT statement
		var sql string
		if whereClause == "" {
			sql = fmt.Sprintf("SELECT * FROM %s", tableName)
		} else {
			sql = fmt.Sprintf("SELECT * FROM %s WHERE %s", tableName, whereClause)
		}

		// Execute the query
		rows, err := db.conn.Query(sql)
		if err != nil {
			err := fmt.Errorf("SQLite Error: failed to execute query: %v", err)
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}
		defer rows.Close()

		// Get column names
		columns, err := rows.Columns()
		if err != nil {
			err := fmt.Errorf("SQLite Error: failed to get column names: %v", err)
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}

		// Build result list
		var results []*object

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
				return makeErr(&object{err.Error(), resultType.Err()}, resultType)
			}

			// Map columns to struct fields
			structFields := make(map[string]*object)
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

				structFields[columnName] = &object{fieldValue, fieldType}
			}

			// Create the struct object
			structObj := &object{structFields, targetStructType}
			results = append(results, structObj)
		}

		if err := rows.Err(); err != nil {
			err := fmt.Errorf("SQLite Error: row iteration error: %v", err)
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}

		// Return list of structs
		return makeOk(&object{results, listType}, resultType)
	case "delete":
		// fn delete(table: Str, where: Str) Result<Bool, Str>
		tableName := args[0].raw.(string)
		whereClause := args[1].raw.(string)

		// Construct SQL
		var sql string
		if whereClause == "" {
			sql = fmt.Sprintf("DELETE FROM %s", tableName)
		} else {
			sql = fmt.Sprintf("DELETE FROM %s WHERE %s", tableName, whereClause)
		}

		// Execute the DELETE
		result, err := db.conn.Exec(sql)
		if err != nil {
			return makeErr(&object{err.Error(), checker.Str}, method.Type().(*checker.Result))
		}

		// Check if any rows were affected
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return makeErr(&object{err.Error(), checker.Str}, method.Type().(*checker.Result))
		}

		// Return Ok(true) if rows were deleted, Ok(false) if no rows matched
		deleted := rowsAffected > 0
		return makeOk(&object{deleted, checker.Bool}, method.Type().(*checker.Result))
	case "close":
		// fn close() Result<Void, Str>
		// Close the database connection
		err := db.conn.Close()
		if err != nil {
			return makeErr(&object{err.Error(), checker.Str}, method.Type().(*checker.Result))
		}

		// Return Ok(void)
		return makeOk(void, method.Type().(*checker.Result))
	case "count":
		// fn count(table: Str, where: Str) Result<Int, Str>
		tableName := args[0].raw.(string)
		whereClause := args[1].raw.(string)

		// Construct SQL
		var sql string
		if whereClause == "" {
			sql = fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
		} else {
			sql = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", tableName, whereClause)
		}

		// Execute the COUNT query
		var count int64
		err := db.conn.QueryRow(sql).Scan(&count)
		if err != nil {
			return makeErr(&object{err.Error(), checker.Str}, method.Type().(*checker.Result))
		}

		// Return Ok(count)
		return makeOk(&object{int(count), checker.Int}, method.Type().(*checker.Result))
	case "exists":
		// fn exists(table: Str, where: Str) Result<Bool, Str>
		tableName := args[0].raw.(string)
		whereClause := args[1].raw.(string)

		// Construct SQL - using EXISTS for efficiency
		var sql string
		if whereClause == "" {
			sql = fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s)", tableName)
		} else {
			sql = fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE %s)", tableName, whereClause)
		}

		// Execute the EXISTS query
		var exists bool
		err := db.conn.QueryRow(sql).Scan(&exists)
		if err != nil {
			return makeErr(&object{err.Error(), checker.Str}, method.Type().(*checker.Result))
		}

		// Return Ok(exists)
		return makeOk(&object{exists, checker.Bool}, method.Type().(*checker.Result))
	case "upsert":
		// fn upsert(table: Str, where: Str, record: $T) Result<Bool, Str>
		tableName := args[0].raw.(string)
		whereClause := args[1].raw.(string)
		structObj := args[2]

		resultType := method.Type().(*checker.Result)

		// Extract fields from the struct
		structFields, ok := structObj.raw.(map[string]*object)
		if !ok {
			errorMsg := &object{"Upsert expects a struct object", resultType.Err()}
			return makeErr(errorMsg, resultType)
		}

		// Check if record exists
		checkSQL := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE %s)", tableName, whereClause)
		var exists bool
		err := db.conn.QueryRow(checkSQL).Scan(&exists)
		if err != nil {
			errorMsg := &object{err.Error(), resultType.Err()}
			return makeErr(errorMsg, resultType)
		}

		if exists {
			// Record exists - perform UPDATE
			var setPairs []string
			var values []interface{}

			// Sort column names for consistent ordering
			var columns []string
			for columnName := range structFields {
				columns = append(columns, columnName)
			}
			sort.Strings(columns)

			// Build SET clauses
			for _, columnName := range columns {
				fieldObj := structFields[columnName]
				setPairs = append(setPairs, fmt.Sprintf("%s = ?", columnName))

				// Handle Maybe types for update
				if _, isMaybe := fieldObj._type.(*checker.Maybe); isMaybe {
					if fieldObj.raw == nil {
						values = append(values, nil)
					} else {
						values = append(values, fieldObj.raw)
					}
				} else {
					values = append(values, fieldObj.raw)
				}
			}

			// Construct UPDATE SQL
			sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
				tableName,
				strings.Join(setPairs, ", "),
				whereClause,
			)

			// Execute the UPDATE
			_, err := db.conn.Exec(sql, values...)
			if err != nil {
				errorMsg := &object{err.Error(), resultType.Err()}
				return makeErr(errorMsg, resultType)
			}
		} else {
			// Record doesn't exist - perform INSERT
			var columns []string
			var placeholders []string
			var values []any

			// Sort column names for consistent ordering
			for columnName := range structFields {
				columns = append(columns, columnName)
			}
			sort.Strings(columns)

			// Build values in same order as columns
			for _, columnName := range columns {
				fieldObj := structFields[columnName]
				placeholders = append(placeholders, "?")
				
				// Handle Maybe types
				if _, isMaybe := fieldObj._type.(*checker.Maybe); isMaybe {
					if fieldObj.raw == nil {
						values = append(values, nil)
					} else {
						values = append(values, fieldObj.raw)
					}
				} else {
					values = append(values, fieldObj.raw)
				}
			}

			// Construct INSERT SQL
			sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
				tableName,
				strings.Join(columns, ", "),
				strings.Join(placeholders, ", "),
			)

			// Execute the INSERT
			_, err := db.conn.Exec(sql, values...)
			if err != nil {
				errorMsg := &object{err.Error(), resultType.Err()}
				return makeErr(errorMsg, resultType)
			}
		}

		// Return Ok(true) - upsert succeeded
		return makeOk(&object{true, checker.Bool}, resultType)
	case "query":
		// fn query(sql: Str) Dynamic!Str
		resultType := method.Type().(*checker.Result)
		sql := args[0].raw.(string)

		// Execute the query
		rows, err := db.conn.Query(sql)
		if err != nil {
			err := fmt.Errorf("SQLite Error: failed to execute query: %v", err)
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}
		defer rows.Close()

		// Get column names
		columns, err := rows.Columns()
		if err != nil {
			err := fmt.Errorf("SQLite Error: failed to get column names: %v", err)
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
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
				return makeErr(&object{err.Error(), resultType.Err()}, resultType)
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
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}

		// Return Dynamic object containing list of row maps
		return makeOk(&object{results, checker.Dynamic}, resultType)
	case "first":
		// fn first(sql: Str) Dynamic!Str
		resultType := method.Type().(*checker.Result)
		sql := args[0].raw.(string)

		// Execute the query
		rows, err := db.conn.Query(sql)
		if err != nil {
			err := fmt.Errorf("SQLite Error: failed to execute query: %v", err)
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}
		defer rows.Close()

		// Get column names
		columns, err := rows.Columns()
		if err != nil {
			err := fmt.Errorf("SQLite Error: failed to get column names: %v", err)
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
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
				return makeErr(&object{err.Error(), resultType.Err()}, resultType)
			}

			// Create map for this row
			rowMap := make(map[string]interface{})
			for i, columnName := range columns {
				rowMap[columnName] = values[i]
			}

			// Return Dynamic object containing the first row map
			return makeOk(&object{rowMap, checker.Dynamic}, resultType)
		}

		if err := rows.Err(); err != nil {
			err := fmt.Errorf("SQLite Error: row iteration error: %v", err)
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}

		// No rows found - return nil as Dynamic
		return makeOk(&object{nil, checker.Dynamic}, resultType)
	default:
		panic(fmt.Errorf("Unimplemented: Database.%s()", method.Name))
	}
}
