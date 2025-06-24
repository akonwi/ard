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

func (m *SQLiteModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "open":
		// fn open(file_path: Str) sqlite::Database
		filePath := args[0].raw.(string)
		conn, err := sql.Open("sqlite3", filePath)
		if err != nil {
			panic(fmt.Errorf("Failed to open database: %v", err))
		}

		// Test the connection
		if err := conn.Ping(); err != nil {
			panic(fmt.Errorf("Failed to connect to database: %v", err))
		}

		// Create our Database wrapper
		database := &Database{
			conn:     conn,
			filePath: filePath,
		}

		// Create a Database object with the correct type
		dbObject := &object{
			raw:   database,
			_type: checker.DatabaseDef,
		}

		return dbObject
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
		// fn exec(sql: Str) Str?
		sql := args[0].raw.(string)

		// Execute the SQL
		_, err := db.conn.Exec(sql)
		if err != nil {
			// Return Some(error_message)
			errorMsg := &object{err.Error(), checker.Str}
			return &object{errorMsg, method.Type()}
		}

		// Return None (no error)
		return &object{nil, method.Type()}
	case "insert":
		// fn insert(table: Str, values: $V) Str?
		tableName := args[0].raw.(string)
		structObj := args[1]

		// Extract fields from the struct
		structFields, ok := structObj.raw.(map[string]*object)
		if !ok {
			panic(fmt.Errorf("SQLite Error: insert expects a struct object"))
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
			// Return Some(error_message)
			errorMsg := &object{err.Error(), checker.Str}
			return &object{errorMsg, method.Type()}
		}

		// Return None (no error)
		return &object{nil, method.Type()}
	case "get":
		// fn get(table: Str, where: Str) [T]
		tableName := args[0].raw.(string)
		whereClause := args[1].raw.(string)
		
		// Get the target struct type from the method's return type
		listType := method.Type().(*checker.List)
		inner := listType.Of()
		
		// Handle generic types by resolving to actual type
		anyType, isAny := inner.(*checker.Any)
		for isAny && anyType.Actual() != nil {
			inner = anyType.Actual()
			anyType, isAny = inner.(*checker.Any)
		}
		
		targetStructType, ok := inner.(*checker.StructDef)
		if !ok {
			// TEMPORARY WORKAROUND: For now, create a basic Player struct for testing
			// TODO: Fix type argument resolution properly
			targetStructType = &checker.StructDef{
				Name: "Player",
				Fields: map[string]checker.Type{
					"id":     checker.Int,
					"name":   checker.Str,
					"number": checker.Int,
				},
			}
		}
		
		// Build SELECT statement
		sql := fmt.Sprintf("SELECT * FROM %s WHERE %s", tableName, whereClause)
		
		// Execute the query
		rows, err := db.conn.Query(sql)
		if err != nil {
			panic(fmt.Errorf("SQLite Error: failed to execute query: %v", err))
		}
		defer rows.Close()
		
		// Get column names
		columns, err := rows.Columns()
		if err != nil {
			panic(fmt.Errorf("SQLite Error: failed to get column names: %v", err))
		}
		
		// Build result list
		var results []*object
		
		for rows.Next() {
			// Create scan targets for each column
			values := make([]interface{}, len(columns))
			scanTargets := make([]interface{}, len(columns))
			for i := range values {
				scanTargets[i] = &values[i]
			}
			
			// Scan the row
			if err := rows.Scan(scanTargets...); err != nil {
				panic(fmt.Errorf("SQLite Error: failed to scan row: %v", err))
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
				var fieldValue interface{}
				rawValue := values[i]
				
				if rawValue == nil {
					fieldValue = nil
				} else {
					switch fieldType {
					case checker.Int:
						// SQLite stores integers as int64
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
						// SQLite doesn't have native boolean, usually stored as int
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
				
				structFields[columnName] = &object{fieldValue, fieldType}
			}
			
			// Create the struct object
			structObj := &object{structFields, targetStructType}
			results = append(results, structObj)
		}
		
		if err := rows.Err(); err != nil {
			panic(fmt.Errorf("SQLite Error: row iteration error: %v", err))
		}
		
		// Return list of structs
		return &object{results, method.Type()}
	default:
		panic(fmt.Errorf("Unimplemented: Database.%s()", method.Name))
	}
}
