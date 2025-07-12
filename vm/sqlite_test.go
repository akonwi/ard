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
		let db = sqlite::open("test.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)").expect("Failed to create table")
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

		let db = sqlite::open("test_insert.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		let player = Player{ id: 1, name: "John Doe", number: 2 }
		db.insert("players", player)
	`)

	// Should return Void!Str
	if result != nil {
		t.Errorf("Expected nil (no error), got %v", result)
	}
}

func TestSQLiteInsertMultipleValues(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_multi.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_multi.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		let player1 = Player{ id: 1, name: "John Doe", number: 2 }
		let player2 = Player{ id: 2, name: "Jane Smith", number: 5 }

		db.insert("players", player1).expect("Failed to insert player 1")
		db.insert("players", player2).expect("Failed to insert player 2")
	`)
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

		let db = sqlite::open("test_error.db").expect("Failed to open database")
		// Don't create the table - this should cause an error

		let player = Player{ id: 1, name: "John Doe", number: 2 }
		db.insert("players", player)
	`)

	if result == nil {
		t.Errorf("Expected insert to fail with error, got %v", result)
	}
}

func TestSQLiteGetBasic(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_get.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_get.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		// Insert test data
		let player1 = Player{ id: 1, name: "John Doe", number: 2 }
		let player2 = Player{ id: 2, name: "Jane Smith", number: 5 }
		db.insert("players", player1).expect("Failed to insert player")
		db.insert("players", player2).expect("Failed to insert player")

		// Get all players
		let players = db.get<Player>("players", "1=1").expect("Failed to get players")
		players.size()
	`)

	if result != 2 {
		t.Errorf("Expected 2 players, got %v", result)
	}
}

func TestSQLiteGetWithCondition(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_get_condition.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_get_condition.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		// Insert test data
		let player1 = Player{ id: 1, name: "John Doe", number: 2 }
		let player2 = Player{ id: 2, name: "Jane Smith", number: 2 }
		let player3 = Player{ id: 3, name: "Bob Wilson", number: 5 }
		db.insert("players", player1).expect("Failed to insert player 1")
		db.insert("players", player2).expect("Failed to insert player 2")
		db.insert("players", player3).expect("Failed to insert player 3")

		// Get players with number = 2
		let twos = db.get<Player>("players", "number = 2").expect("Failed to get players with number=2")
		twos.size()
	`)

	if result != 2 {
		t.Errorf("Expected 2 players with number=2, got %v", result)
	}
}

func TestSQLiteGetFieldAccess(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_get_fields.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_get_fields.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		// Insert test data
		let player = Player{ id: 1, name: "John Doe", number: 2 }
		db.insert("players", player).expect("Failed to insert player")

		// Get player and check field access
		let players = db.get<Player>("players", "id = 1").expect("Failed to get player with id=1")
		let first = players.at(0)
		first.name == "John Doe" and first.number == 2
	`)

	if result != true {
		t.Errorf("Expected field access to work correctly, got %v", result)
	}
}

func TestSQLiteInsertWithMaybeTypes(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_maybe.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sqlite
		use ard/maybe
		struct User {
			id: Int,
			name: Str,
			email: Str?
		}

		let db = sqlite::open("test_maybe.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)").expect("Failed to create table")

		// Insert user with email
		let user1 = User{ id: 1, name: "John Doe", email: maybe::some("john@example.com") }
		db.insert("users", user1).expect("Failed to insert user 1")

		// Insert user without email (none)
		let user2 = User{ id: 2, name: "Jane Smith", email: maybe::none() }
		db.insert("users", user2).expect("Failed to insert user 2")
	`)
}

func TestSQLiteGetWithMaybeTypes(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_maybe_get.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		use ard/maybe
		struct User {
			id: Int,
			name: Str,
			email: Str?
		}

		let db = sqlite::open("test_maybe_get.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)").expect("Failed to create table")

		// Insert users with and without email
		let user1 = User{ id: 1, name: "John Doe", email: maybe::some("john@example.com") }
		let user2 = User{ id: 2, name: "Jane Smith", email: maybe::none() }
		db.insert("users", user1).expect("Failed to insert user 1")
		db.insert("users", user2).expect("Failed to insert user 2")

		// Get all users
		let users = db.get<User>("users", "1=1").expect("Failed to get users")
		let first = users.at(0)
		let second = users.at(1)

		// Check that Maybe fields work correctly
		first.email.or("") == "john@example.com" and second.email.or("") == ""
	`)

	if result != true {
		t.Errorf("Expected Maybe field retrieval to work correctly, got %v", result)
	}
}

func TestSQLiteMaybeTypesRoundTrip(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_maybe_roundtrip.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		use ard/maybe
		struct Product {
			id: Int,
			name: Str,
			description: Str?,
			price: Float?,
			in_stock: Bool?
		}

		let db = sqlite::open("test_maybe_roundtrip.db").expect("Failed to open database")
		db.exec("CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL, description TEXT, price REAL, in_stock INTEGER)").expect("Failed to create table")

		// Insert products with various Maybe field combinations
		let product1 = Product{
			id: 1,
			name: "Widget",
			description: maybe::some("A useful widget"),
			price: maybe::some(19.99),
			in_stock: maybe::some(true)
		}
		let product2 = Product{
			id: 2,
			name: "Gadget",
			description: maybe::none(),
			price: maybe::none(),
			in_stock: maybe::some(false)
		}
		let product3 = Product{
			id: 3,
			name: "Thing",
			description: maybe::some("Another thing"),
			price: maybe::some(5.0),
			in_stock: maybe::none()
		}

		db.insert("products", product1).expect("Failed to insert product 1")
		db.insert("products", product2).expect("Failed to insert product 2")
		db.insert("products", product3).expect("Failed to insert product 3")

		// Retrieve and verify
		let products = db.get<Product>("products", "1=1").expect("Failed to get products")
		let p1 = products.at(0)
		let p2 = products.at(1)
		let p3 = products.at(2)

		// Test all combinations
		let p1_desc_ok = p1.description.or("") == "A useful widget"
		let p1_price_ok = p1.price.or(0.0) == 19.99
		let p1_stock_ok = p1.in_stock.or(false) == true

		let p2_desc_ok = p2.description.or("default") == "default"
		let p2_price_ok = p2.price.or(-1.0) == -1.0
		let p2_stock_ok = p2.in_stock.or(true) == false

		let p3_desc_ok = p3.description.or("") == "Another thing"
		let p3_price_ok = p3.price.or(0.0) == 5.0
		let p3_stock_ok = p3.in_stock.or(true) == true

		(p1_desc_ok and p1_price_ok and p1_stock_ok and p2_desc_ok and p2_price_ok and p2_stock_ok and p3_desc_ok and p3_price_ok and p3_stock_ok)
	`)

	if result != true {
		t.Errorf("Expected comprehensive Maybe types round-trip to work correctly, got %v", result)
	}
}

func TestSQLiteUpdate(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_update.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_update.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		// Insert initial record
		let player = Player{ id: 1, name: "John Doe", number: 2 }
		db.insert("players", player).expect("Failed to insert player")

		// Update the record
		let updated_player = Player{ id: 1, name: "John Smith", number: 10 }
		let result = db.update("players", "id = 1", updated_player)

		// Should succeed
		match result {
			ok => true,
			err => false
		}
	`)

	if result != true {
		t.Errorf("Expected update to succeed, got %v", result)
	}
}

func TestSQLiteUpdateVerification(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_update_verify.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_update_verify.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		// Insert initial record
		let player = Player{ id: 1, name: "John Doe", number: 2 }
		db.insert("players", player).expect("Failed to insert player")

		// Update the record
		let updated_player = Player{ id: 1, name: "John Smith", number: 10 }
		db.update("players", "id = 1", updated_player)

		// Verify the update worked
		match db.get<Player>("players", "id = 1") {
			ok => {
				let players = ok
				let retrieved = players.at(0)
				retrieved.name == "John Smith" and retrieved.number == 10
			},
			err => false
		}
	`)

	if result != true {
		t.Errorf("Expected update verification to pass, got %v", result)
	}
}

func TestSQLiteUpdateNonExistentRecord(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_update_missing.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_update_missing.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		// Try to update non-existent record
		let player = Player{ id: 999, name: "Ghost Player", number: 99 }
		let result = db.update("players", "id = 999", player)

		// Should fail
		match result {
			ok => false,
			err => true
		}
	`)

	if result != true {
		t.Errorf("Expected update of non-existent record to fail, got %v", result)
	}
}

func TestSQLiteUpdateWithMaybeTypes(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_update_maybe.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		use ard/maybe
		struct User {
			id: Int,
			name: Str,
			email: Str?
		}

		let db = sqlite::open("test_update_maybe.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)").expect("Failed to create table")

		// Insert initial record with email
		let user = User{ id: 1, name: "John Doe", email: maybe::some("john@example.com") }
		db.insert("users", user).expect("Failed to insert user")

		// Update to remove email (set to none)
		let updated_user = User{ id: 1, name: "John Smith", email: maybe::none() }
		let result = db.update("users", "id = 1", updated_user)

		match result {
					ok => {
						// Verify the update worked
						match db.get<User>("users", "id = 1") {
							ok => {
								let users = ok
								let retrieved = users.at(0)
								retrieved.name == "John Smith" and retrieved.email.or("default") == "default"
							},
							err => false
						}
					},
					err => false
		}
	`)

	if result != true {
		t.Errorf("Expected update with Maybe types to work correctly, got %v", result)
	}
}

func TestSQLiteDelete(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_delete.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_delete.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		// Insert test records
		db.insert("players", Player{ id: 1, name: "John Doe", number: 2 }).expect("Failed to insert player")
		db.insert("players", Player{ id: 2, name: "Jane Smith", number: 10 }).expect("Failed to insert player")
		db.insert("players", Player{ id: 3, name: "Bob Wilson", number: 23 }).expect("Failed to insert player")

		// Delete one record
		let result = db.delete("players", "id = 2")

		// Should succeed - expect Ok(true) since record should be deleted
		match result {
			ok => {
				ok == true
			},
			err => false
		}
	`)

	if result != true {
		t.Errorf("Expected delete to succeed, got %v", result)
	}
}

func TestSQLiteDeleteNonExistent(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_delete_missing.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_delete_missing.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		// Try to delete non-existent record
		db.delete("players", "id = 999")
	`)

	if result != false {
		t.Errorf("Expected delete of non-existent record to return Ok(false), got %v", result)
	}
}

func TestSQLiteDeleteMultiple(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_delete_multiple.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_delete_multiple.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		// Insert test records
		db.insert("players", Player{ id: 1, name: "John Doe", number: 2 }).expect("Failed to insert player")
		db.insert("players", Player{ id: 2, name: "Jane Smith", number: 2 }).expect("Failed to insert player")
		db.insert("players", Player{ id: 3, name: "Bob Wilson", number: 23 }).expect("Failed to insert player")

		// Delete multiple records with same number
		db.delete("players", "number = 2")
	`)

	if result != true {
		t.Errorf("Expected delete multiple to succeed, got %v", result)
	}
}

func TestSQLiteClose(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_close.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_close.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		// Insert a test record
		db.insert("players", Player{ id: 1, name: "John Doe", number: 2 }).expect("Failed to insert player")

		// Close the database
		let result = db.close()

		// Should succeed
		match result {
			ok => true,
			err => false
		}
	`)

	if result != true {
		t.Errorf("Expected close to succeed, got %v", result)
	}
}

func TestSQLiteCloseMultipleTimes(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_close_multiple.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		let db = sqlite::open("test_close_multiple.db").expect("Failed to open database")

		// Close the database once
		let first_close = db.close()

		// Try to close again - SQLite should handle this gracefully
		let second_close = db.close()

		// Both should succeed (SQLite allows multiple closes)
		match first_close {
			ok => {
				match second_close {
					ok => true,
					err => false
				}
			},
			err => false
		}
	`)

	if result != true {
		t.Errorf("Expected multiple closes to succeed, got %v", result)
	}
}

func TestSQLiteCount(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_count.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_count.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)")

		// Insert test records
		db.insert("players", Player{ id: 1, name: "John Doe", number: 2 }).expect("Failed to insert player")
		db.insert("players", Player{ id: 2, name: "Jane Smith", number: 2 }).expect("Failed to insert player")
		db.insert("players", Player{ id: 3, name: "Bob Wilson", number: 23 }).expect("Failed to insert player")

		// Count all players
		let all_count = db.count("players", "").expect("Failed to count all players")

		// Count players with number 2
		let twos_count = db.count("players", "number = 2").expect("Failed to count players with number 2")

		// Count players with non-existent condition
		let none_count = db.count("players", "number = 999").expect("Failed to count non-existent players")

		all_count == 3 and twos_count == 2 and none_count == 0
	`)

	if result != true {
		t.Errorf("Expected count operations to return correct values, got %v", result)
	}
}

func TestSQLiteCountInvalidTable(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_count_invalid.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		let db = sqlite::open("test_count_invalid.db").expect("Failed to open database")

		// Try to count from non-existent table
		let result = db.count("non_existent_table", "")

		// Should fail
		match result {
			ok => false,
			err => true
		}
	`)

	if result != true {
		t.Errorf("Expected count on invalid table to fail, got %v", result)
	}
}

func TestSQLiteExists(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_exists.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_exists.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)")

		// Insert test records
		db.insert("players", Player{ id: 1, name: "John Doe", number: 2 }).expect("Failed to insert player")
		db.insert("players", Player{ id: 2, name: "Jane Smith", number: 2 }).expect("Failed to insert player")
		db.insert("players", Player{ id: 3, name: "Bob Wilson", number: 23 }).expect("Failed to insert player")

		// Check if any players exist
		let any_exist = db.exists("players", "").expect("Failed to check if any players exist")

		// Check if players with number 2 exist
		let twos_exist = db.exists("players", "number = 2").expect("Failed to check if players with number 2 exist")

		// Check if players with non-existent condition exist
		let none_exist = db.exists("players", "number = 999").expect("Failed to check if players with number 999 exist")

		any_exist == true and twos_exist == true and none_exist == false
	`)

	if result != true {
		t.Errorf("Expected exists operations to return correct values, got %v", result)
	}
}

func TestSQLiteExistsInvalidTable(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_exists_invalid.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		let db = sqlite::open("test_exists_invalid.db").expect("Failed to open database")

		// Try to check existence in non-existent table
		let result = db.exists("non_existent_table", "")

		// Should fail
		match result {
			ok => false,
			err => true
		}
	`)

	if result != true {
		t.Errorf("Expected exists on invalid table to fail, got %v", result)
	}
}

func TestSQLiteExistsEmptyTable(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_exists_empty.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		let db = sqlite::open("test_exists_empty.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)")

		// Check if any players exist in empty table
		let exists = db.exists("players", "").expect("Failed to check existence in empty table")

		exists == false
	`)

	if result != true {
		t.Errorf("Expected exists in empty table to return false, got %v", result)
	}
}

func TestSQLiteEmptyWhereClause(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_empty_where.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_empty_where.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		// Insert test records
		db.insert("players", Player{ id: 1, name: "John Doe", number: 2 }).expect("Failed to insert player")
		db.insert("players", Player{ id: 2, name: "Jane Smith", number: 10 }).expect("Failed to insert player")

		// Get all players using empty where clause
		let all_players = db.get<Player>("players", "").expect("Failed to get all players")

		// Count all players using empty where clause
		let count = db.count("players", "").expect("Failed to count all players")

		// Check if any players exist using empty where clause
		let exists = db.exists("players", "").expect("Failed to check if any players exist")

		count == 2 and exists == true and all_players.at(0).name == "John Doe" and all_players.at(1).name == "Jane Smith"
	`)

	if result != true {
		t.Errorf("Expected empty where clauses to work correctly, got %v", result)
	}
}

func TestSQLiteUpsert(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_upsert.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_upsert.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		// Test insert (new record)
		let player1 = Player{ id: 1, name: "John Doe", number: 2 }
		let insert_result = db.upsert("players", "id = 1", player1).expect("Failed to upsert new player")

		// Test update (existing record)
		let updated_player1 = Player{ id: 1, name: "John Smith", number: 10 }
		let update_result = db.upsert("players", "id = 1", updated_player1).expect("Failed to upsert existing player")

		// Verify the update worked
		let players = db.get<Player>("players", "id = 1").expect("Failed to get player")
		let retrieved = players.at(0)
		
		insert_result == true and update_result == true and retrieved.name == "John Smith" and retrieved.number == 10
	`)

	if result != true {
		t.Errorf("Expected upsert operations to work correctly, got %v", result)
	}
}

func TestSQLiteUpsertMultipleKeys(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_upsert_multi.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Score {
			player_id: Int,
			game_id: Int,
			score: Int,
		}

		let db = sqlite::open("test_upsert_multi.db").expect("Failed to open database")
		db.exec("CREATE TABLE scores (player_id INTEGER, game_id INTEGER, score INTEGER, PRIMARY KEY (player_id, game_id))").expect("Failed to create table")

		// Test insert with composite key
		let score1 = Score{ player_id: 1, game_id: 1, score: 100 }
		let insert_result = db.upsert("scores", "player_id = 1 AND game_id = 1", score1).expect("Failed to upsert new score")

		// Test update with composite key
		let updated_score1 = Score{ player_id: 1, game_id: 1, score: 150 }
		let update_result = db.upsert("scores", "player_id = 1 AND game_id = 1", updated_score1).expect("Failed to upsert existing score")

		// Verify the update worked
		let scores = db.get<Score>("scores", "player_id = 1 AND game_id = 1").expect("Failed to get score")
		let retrieved = scores.at(0)
		
		insert_result == true and update_result == true and retrieved.score == 150
	`)

	if result != true {
		t.Errorf("Expected upsert with multiple keys to work correctly, got %v", result)
	}
}

func TestSQLiteUpsertWithMaybeTypes(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_upsert_maybe.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		use ard/maybe
		struct User {
			id: Int,
			name: Str,
			email: Str?
		}

		let db = sqlite::open("test_upsert_maybe.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)").expect("Failed to create table")

		// Insert user with email
		let user1 = User{ id: 1, name: "John Doe", email: maybe::some("john@example.com") }
		db.upsert("users", "id = 1", user1).expect("Failed to upsert user with email")

		// Update user to remove email
		let user1_no_email = User{ id: 1, name: "John Doe", email: maybe::none() }
		db.upsert("users", "id = 1", user1_no_email).expect("Failed to upsert user without email")

		// Verify the update worked
		let users = db.get<User>("users", "id = 1").expect("Failed to get user")
		let retrieved = users.at(0)
		
		retrieved.name == "John Doe" and retrieved.email.or("default") == "default"
	`)

	if result != true {
		t.Errorf("Expected upsert with Maybe types to work correctly, got %v", result)
	}
}

func TestSQLiteUpsertError(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_upsert_error.db"
	defer os.Remove(testDB)

	result := run(t, `
		use ard/sqlite
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_upsert_error.db").expect("Failed to open database")
		// Don't create the table - this should cause an error

		let player = Player{ id: 1, name: "John Doe", number: 2 }
		let result = db.upsert("players", "id = 1", player)

		// Should fail
		match result {
			ok => false,
			err => true
		}
	`)

	if result != true {
		t.Errorf("Expected upsert to fail with missing table, got %v", result)
	}
}
