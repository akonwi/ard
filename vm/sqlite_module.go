package vm

import (
	"database/sql"
	"fmt"

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
	default:
		panic(fmt.Errorf("Unimplemented: Database.%s()", method.Name))
	}
}
