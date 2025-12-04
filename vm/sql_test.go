package vm_test

import (
	"os"
	"testing"
)

func TestSQLOpen(t *testing.T) {
	// Clean up any existing test database
	testDB := "test.db"
	defer os.Remove(testDB)

	// Test opening database and creating table using generic sql module
	run(t, `
		use ard/sql
		let db = sql::open("test.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)").expect("Failed to create table")
	`)
}

func TestSQLClose(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_close.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sql
		use ard/decode
		struct Player {
			id: Int,
			name: Str,
			number: Int,
		}

		let db = sql::open("test_close.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		db.close().expect("should succeed")
	`)
}

func TestSqlExtractParams(t *testing.T) {
	run(t, `
		use ard/sql

		let params = sql::extract_params("INSERT INTO players (name, number) VALUES (@name, @number)")
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
				_ => panic("Unexpected: {p}")
			}
		}
	`)
}

func TestMissingParameters(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_insert.db"
	defer os.Remove(testDB)

	expectPanic(t, "Missing parameter: number", `
		use ard/sql
		use ard/decode

		let db = sql::open("test_insert.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		let stmt = db.query("INSERT INTO players (name, number) VALUES (@name, @number)")
 		stmt.run([ "name": "John Doe", "int": 2]).expect("Failed to insert")
	`)
}

func TestSQLQueryRun(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_insert.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sql
		use ard/decode

		let db = sql::open("test_insert.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		let stmt = db.query("INSERT INTO players (name, number) VALUES (@name, @number)")
  	stmt.run([
   		"name": "John Doe",
			"number": 2,
   	]).expect("Insert failed")
	`)
}

func TestSQLQueryError(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_error.db"
	defer os.Remove(testDB)

	expectPanic(t, "Insert should fail", `
		use ard/sql
		use ard/decode

		let db = sql::open("test_error.db").expect("Failed to open database")
		// Don't create the table - this should cause an error

		let stmt = db.query("INSERT INTO players (name, number) VALUES (@name, @number)")
		stmt.run([
		  "name": "John Doe",
			"number": 2,
		]).expect("Insert should fail")
`)
}

func TestSQLQueryAll(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_query_decode.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sql
		use ard/decode
		use ard/maybe

		let db = sql::open("test_query_decode.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")
		db.exec("INSERT INTO players (name, number) VALUES ('John Doe', 2)").expect("Failed to insert player 1")
		db.exec("INSERT INTO players (name, number) VALUES ('Jane Smith', 5)").expect("Failed to insert player 2")

		let query = db.query("SELECT id, name, number FROM players WHERE number = @number")
		let rows = query.all(["number": 5]).expect("Failed to query players")
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

func TestSQLQueryFirst(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_query_first.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sql
		use ard/decode
		use ard/maybe

		let db = sql::open("test_query_first.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")
		db.exec("INSERT INTO players (name, number) VALUES ('John Doe', 2)").expect("Failed to insert player 1")
		db.exec("INSERT INTO players (name, number) VALUES ('Jane Smith', 5)").expect("Failed to insert player 2")

		let query = db.query("SELECT id FROM players WHERE number = @number")
		let maybe_row = query.first(["number": 5]).expect("Failed to query players")
		let row = maybe_row.expect("Found none")

		let decode_name = decode::field("name", decode::string)
		let id = decode::run(row, decode::field("id", decode::int)).expect("Failed to decode id")
		if not id == 2 {
			panic("Expected id 2, got {id}")
		}
	`)
}

func TestSQLInsertingNull(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_maybe.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sql
		use ard/decode

		let db = sql::open("test_maybe.db").expect("Failed to open database")
		let create_table = db.query("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)")
		create_table.run([:])

		let stmt = db.query("INSERT INTO users (name, email) VALUES (@name, @email)")
		let values: [Str:sql::Value] = [
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

func TestTransactionRollback(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_tx_rollback.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sql

		let db = sql::open("test_tx_rollback.db").expect("Failed to open database")
		db.query("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)").run([:])

		let tx = db.begin().expect("Failed to begin transaction")
		tx.exec("INSERT INTO users (id, name) VALUES (1, 'joe');").expect("Failed to insert")
		tx.rollback().expect("Failed to rollback transaction")

		let rows = db.query("SELECT * FROM users;").all([:]).expect("Failed to get all")
		if not rows.size() == 0 {
			panic("Expected no rows after rollback")
		}
	`)
}

func TestTransactionCommitInsert(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_tx_insert.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sql

		let db = sql::open("test_tx_insert.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)").expect("Failed to create table")

		let tx = db.begin().expect("Failed to begin transaction")
		let insert = tx.query("INSERT INTO users (name) VALUES (@name)")

		let names = ["Alice", "Bob"]
		for name in names {
			insert.run(["name":name]).expect("Failed to insert {name}")
		}
		tx.commit().expect("Failed to commit transaction")

		let rows = db.query("SELECT * FROM users").all([:]).expect("Failed to query")
		if not rows.size() == names.size() {
			panic("Expected {names.size()} rows after commit, got {rows.size()}")
		}
	`)
}

func TestTransactionRollbackInsert(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_tx_rollback_insert.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sql

		let db = sql::open("test_tx_rollback_insert.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)").expect("Failed to create table")

		let tx = db.begin().expect("Failed to begin transaction")
		tx.exec("INSERT INTO users (name) VALUES ('Bob')").expect("Failed to insert in transaction")
		tx.rollback().expect("Failed to rollback transaction")

		// Verify insert was rolled back
		let rows = db.query("SELECT * FROM users").all([:]).expect("Failed to query")
		if not rows.size() == 0 {
			panic("Expected 0 rows after rollback, got {rows.size()}")
		}
	`)
}

func TestTransactionQueryRead(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_tx_read.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sql
		use ard/decode

		let db = sql::open("test_tx_read.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)").expect("Failed to create table")
		db.exec("INSERT INTO users (name) VALUES ('Charlie')").expect("Failed to insert")

		let tx = db.begin().expect("Failed to begin transaction")
		let rows = tx.query("SELECT * FROM users").all([:]).expect("Failed to query in transaction")
		tx.commit().expect("Failed to commit transaction")

		if not rows.size() == 1 {
			panic("Expected 1 row in transaction, got {rows.size()}")
		}

		let name = decode::run(rows.at(0), decode::field("name", decode::string)).expect("Failed to decode name")
		if not name == "Charlie" {
			panic("Expected name 'Charlie', got {name}")
		}
	`)
}

func TestTransactionMultipleOperations(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_tx_multiple.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sql

		let db = sql::open("test_tx_multiple.db").expect("Failed to open database")
		db.exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)").expect("Failed to create table")

		let tx = db.begin().expect("Failed to begin transaction")
		tx.exec("INSERT INTO items (name) VALUES ('Item1')").expect("Failed to insert Item1")
		tx.exec("INSERT INTO items (name) VALUES ('Item2')").expect("Failed to insert Item2")
		let rows = tx.query("SELECT * FROM items").all([:]).expect("Failed to query in transaction")
		tx.commit().expect("Failed to commit transaction")

		if not rows.size() == 2 {
			panic("Expected 2 rows in transaction, got {rows.size()}")
		}

		// Verify all inserts persisted
		let final_rows = db.query("SELECT * FROM items").all([:]).expect("Failed to query after commit")
		if not final_rows.size() == 2 {
			panic("Expected 2 rows after commit, got {final_rows.size()}")
		}
	`)
}

func TestTransactionRollbackMultipleOperations(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_tx_rollback_multiple.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sql

		let db = sql::open("test_tx_rollback_multiple.db").expect("Failed to open database")
		db.exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)").expect("Failed to create table")

		let tx = db.begin().expect("Failed to begin transaction")
		tx.exec("INSERT INTO items (name) VALUES ('Item1')").expect("Failed to insert Item1")
		tx.exec("INSERT INTO items (name) VALUES ('Item2')").expect("Failed to insert Item2")
		tx.rollback().expect("Failed to rollback transaction")

		// Verify all inserts were rolled back
		let rows = db.query("SELECT * FROM items").all([:]).expect("Failed to query")
		if not rows.size() == 0 {
			panic("Expected 0 rows after rollback, got {rows.size()}")
		}
	`)
}

func TestTransactionQueryWithParams(t *testing.T) {
	// Clean up any existing test database
	testDB := "test_tx_params.db"
	defer os.Remove(testDB)

	run(t, `
		use ard/sql
		use ard/decode

		let db = sql::open("test_tx_params.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)").expect("Failed to create table")
		db.exec("INSERT INTO users (name, age) VALUES ('User1', 25)").expect("Failed to insert User1")
		db.exec("INSERT INTO users (name, age) VALUES ('User2', 30)").expect("Failed to insert User2")

		let tx = db.begin().expect("Failed to begin transaction")
		let rows = tx.query("SELECT * FROM users WHERE age = @age").all(["age": 30]).expect("Failed to query in transaction")
		tx.commit().expect("Failed to commit transaction")

		if not rows.size() == 1 {
			panic("Expected 1 row matching age 30, got {rows.size()}")
		}

		let name = decode::run(rows.at(0), decode::field("name", decode::string)).expect("Failed to decode name")
		if not name == "User2" {
			panic("Expected name 'User2', got {name}")
		}
	`)
}
