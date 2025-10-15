package vm_test

import (
	"os"
	"testing"
)

func TestSQLiteOpen(t *testing.T) {
	// Clean up any existing test database
	testDB := "test.db"
	defer os.Remove(testDB)

	// Test opening database and creating table
	run(t, `
		use ard/sqlite
		let db = sqlite::open("test.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)").expect("Failed to create table")
	`)
}

func TestSQLiteClose(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_close.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sqlite
		use ard/decode
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sqlite::open("test_close.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		db.close().expect("should succeed")
	`)
}

func TestSqliteExtractParams(t *testing.T) {
	run(t, `
		use ard/sqlite

		let params = sqlite::extract_params("INSERT INTO players (name, number) VALUES (@name, @number)")
		if not params.size() == 2 {
			panic("Expected a list of 2 elements")
		}
		for p, idx in params {
			match idx {
				0 => {
					if not p == "name" {
						panic("Expected 'name' at {idx} got {p}")
					}
				},
				1 => {
					if not p == "number" {
						panic("Expected 'number' at {idx} got {p}")
					}
				},
			}
		}
	`)
}

func TestSQLiteQueryRun(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_insert.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sqlite
		use ard/decode

		let db = sqlite::open("test_insert.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		let stmt = db.query("INSERT INTO players (name, number) VALUES (@name, @number)")
		let values: [Str:sqlite::Value] = [
		  "name": "John Doe",
			"number": 2,
		]
  	stmt.run(values).expect("Insert failed")
	`)
}

func TestSQLiteQueryError(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_error.db"
	defer os.Remove(testDB)

	expectPanic(t, "Insert should fail", `
		use ard/sqlite
		use ard/decode

		let db = sqlite::open("test_error.db").expect("Failed to open database")
		// Don't create the table - this should cause an error

		let stmt = db.query("INSERT INTO players (name, number) VALUES (@name, @number)")
		let values: [Str:sqlite::Value] = [
		  "name": "John Doe",
			"number": 2,
		]
		stmt.run(values).expect("Insert should fail")
`)
}

func TestSQLiteQueryAll(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_query_decode.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sqlite
		use ard/decode
		use ard/maybe

		let db = sqlite::open("test_query_decode.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")
		db.exec("INSERT INTO players (name, number) VALUES ('John Doe', 2)").expect("Failed to insert player 1")
		db.exec("INSERT INTO players (name, number) VALUES ('Jane Smith', 5)").expect("Failed to insert player 2")

		let query = db.query("SELECT id, name, number FROM players WHERE number = @number")
		let vals: [Str:sqlite::Value] = ["number": 5]
		let rows = query.all(vals).expect("Failed to query players")
		if not rows.size() == 1 {
			panic("Expected 1 result, got {rows.size()}")
		}

		let decode_name = decode::field("name", decode::string)
		let first_name = decode_name(rows.at(0)).expect("Unable to decode row")
		if not first_name == "Jane Smith" {
			panic("Expected Jane Smith, got {first_name}")
		}
	`)
}

func TestSQLiteQueryFirst(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_query_first.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sqlite
		use ard/decode
		use ard/maybe

		let db = sqlite::open("test_query_first.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")
		db.exec("INSERT INTO players (name, number) VALUES ('John Doe', 2)").expect("Failed to insert player 1")
		db.exec("INSERT INTO players (name, number) VALUES ('Jane Smith', 5)").expect("Failed to insert player 2")

		let query = db.query("SELECT id FROM players WHERE number = @number")
		let vals: [Str:sqlite::Value] = ["number": 5]
		let maybe_row = query.first(vals).expect("Failed to query players")
		let row = maybe_row.expect("Found none")

		let decode_name = decode::field("name", decode::string)
		let id = decode::run(row, decode::field("id", decode::int)).expect("Failed to decode id")
		if not id == 2 {
			panic("Expected id 2, got {id}")
		}
	`)
}

func TestSQLiteInsertingNull(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_maybe.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sqlite
		use ard/decode

		let db = sqlite::open("test_maybe.db").expect("Failed to open database")
		let create_table = db.query("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)")
		let values: [Str:sqlite::Value] = [:]
		create_table.run(values)

		let stmt = db.query("INSERT INTO users (name, email) VALUES (@name, @email)")
		let values: [Str:sqlite::Value] = [
		  "name": "John Doe",
			"email": ()
		]
		stmt.run(values).expect("Failed to insert row")

		let query = db.query("SELECT email FROM users WHERE id = 1")
		let rows = query.all(values).expect("Failed to find users with id 1")
		if not rows.size() == 1 {
			panic("Expected 1 result, got {rows.size()}")
		}

		let email = decode::run(rows.at(0), decode::field("email", decode::nullable(decode::string))).expect("Failed to decode email")
		if email.is_some() {
			panic("Expected the email to be maybe::none")
		}
	`)
}
