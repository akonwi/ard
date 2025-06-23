package vm_test

import (
	"os"
	"testing"
)

func TestSQLiteBasicOperations(t *testing.T) {
	// Clean up any existing test database
	testDB := "test.db"
	defer os.Remove(testDB)

	// Test opening database and creating table
	result := run(t, `
		use ard/sqlite
		let db = sqlite::open("test.db")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)")
	`)

	// Should return None (no error)
	if result != nil {
		t.Errorf("Expected nil (no error), got %v", result)
	}
}

func TestSQLiteInsertStruct(t *testing.T) {
	// Clean up any existing test database  
	testDB := "test_insert.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}
		
		let db = sqlite::open("test_insert.db")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)")
		
		let player = Player{ id: 1, name: "John Doe", number: 2 }
		db.insert("players", player)
	`)

	// Should return None (no error)
	if result != nil {
		t.Errorf("Expected nil (no error), got %v", result)
	}
}

func TestSQLiteInsertMultipleValues(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_multi.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}
		
		let db = sqlite::open("test_multi.db")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)")
		
		let player1 = Player{ id: 1, name: "John Doe", number: 2 }
		let player2 = Player{ id: 2, name: "Jane Smith", number: 5 }
		
		let result1 = db.insert("players", player1)
		let result2 = db.insert("players", player2)
		
		// Both should succeed (return None)
		match result1 {
			error => false,
			_ => match result2 {
				error => false,
				_ => true
			}
		}
	`)

	if result != true {
		t.Errorf("Expected both inserts to succeed, got %v", result)
	}
}

func TestSQLiteInsertError(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_error.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}
		
		let db = sqlite::open("test_error.db")
		// Don't create the table - this should cause an error
		
		let player = Player{ id: 1, name: "John Doe", number: 2 }
		let result = db.insert("players", player)
		
		// Should return Some(error_message) 
		match result {
			error => true,
			_ => false
		}
	`)

	if result != true {
		t.Errorf("Expected insert to fail with error, got %v", result)
	}
}
