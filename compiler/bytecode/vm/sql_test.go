package vm

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func runBytecodeWithRuntimeError(input string) error {
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		return fmt.Errorf("parse errors: %v", result.Errors[0].Message)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	resolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		return fmt.Errorf("failed to init module resolver: %w", err)
	}

	c := checker.New("test.ard", result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		return fmt.Errorf("diagnostics found: %v", c.Diagnostics())
	}

	emitter := bytecode.NewEmitter()
	program, err := emitter.EmitProgram(c.Module())
	if err != nil {
		return fmt.Errorf("emit error: %w", err)
	}
	if err := bytecode.VerifyProgram(program); err != nil {
		return fmt.Errorf("verify error: %w", err)
	}

	_, err = New(program).Run("main")
	return err
}

func expectBytecodeRuntimeError(t *testing.T, expected, input string) {
	t.Helper()
	err := runBytecodeWithRuntimeError(input)
	if err == nil {
		t.Fatalf("expected runtime error containing %q, got nil", expected)
	}
	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected runtime error containing %q, got %q", expected, err.Error())
	}
}

func TestBytecodeSQLOpen(t *testing.T) {
	testDB := "test.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
		use ard/sql
		let db = sql::open("test.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)").expect("Failed to create table")
	`)
}

func TestBytecodeSQLClose(t *testing.T) {
	testDB := "test_close.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
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

func TestBytecodeSqlExtractParams(t *testing.T) {
	runBytecodeRaw(t, `
		use ard/sql

	  let names = sql::extract_params("SELECT f.*, fs.* FROM fixtures f
	    JOIN fixture_stats fs ON f.id = fs.fixture_id
	    WHERE f.league_id = @league_id AND f.season = @season
	    AND (f.home_id = @team_id OR f.away_id = @team_id)
	    AND f.finished = true"
	  )
		if not names.size() == 4 {
			panic("Expected a list of 4 names - got {names.size()}")
		}
	`)
}

func TestBytecodeSQLParameterSubstringOverlap(t *testing.T) {
	testDB := "test_substring_overlap.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
		use ard/sql

		let db = sql::open("test_substring_overlap.db").expect("Failed to open database")
		db.exec("CREATE TABLE teams (id INTEGER PRIMARY KEY, team_id TEXT, team TEXT)").expect("Failed to create table")

		let stmt = db.query("INSERT INTO teams (team, team_id) VALUES (@team, @team_id)")
		stmt.run([
			"team_id": "T123",
			"team": "Rockets",
		]).expect("Insert failed")

		let query = db.query("SELECT team_id, team FROM teams WHERE team_id = @team_id AND team = @team")
		let rows = query.all([
			"team_id": "T123",
			"team": "Rockets",
		]).expect("Query failed")

		if not rows.size() == 1 {
			panic("Expected 1 result, got {rows.size()}")
		}
	`)
}

func TestBytecodeSQLMissingParameters(t *testing.T) {
	testDB := "test_insert.db"
	defer os.Remove(testDB)

	expectBytecodeRuntimeError(t, "Missing parameter: number", `
		use ard/sql
		use ard/decode

		let db = sql::open("test_insert.db").expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		let stmt = db.query("INSERT INTO players (name, number) VALUES (@name, @number)")
	 	stmt.run([ "name": "John Doe", "int": 2]).expect("Failed to insert")
	`)
}

func TestBytecodeMissingParameters(t *testing.T) {
	TestBytecodeSQLMissingParameters(t)
}

func TestBytecodeSQLQueryRun(t *testing.T) {
	testDB := "test_insert.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
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

func TestBytecodeSQLQueryError(t *testing.T) {
	testDB := "test_error.db"
	defer os.Remove(testDB)

	expectBytecodeRuntimeError(t, "Insert should fail", `
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

func TestBytecodeSQLQueryAll(t *testing.T) {
	testDB := "test_query_decode.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
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

func TestBytecodeSQLQueryFirst(t *testing.T) {
	testDB := "test_query_first.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
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

		let id = decode::run(row, decode::field("id", decode::int)).expect("Failed to decode id")
		if not id == 2 {
			panic("Expected id 2, got {id}")
		}
	`)
}

func TestBytecodeSQLInsertingNull(t *testing.T) {
	testDB := "test_maybe.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
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

func TestBytecodeTransactionRollback(t *testing.T) {
	testDB := "test_tx_rollback.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
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

func TestBytecodeTransactionCommitInsert(t *testing.T) {
	testDB := "test_tx_insert.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
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

func TestBytecodeTransactionRollbackInsert(t *testing.T) {
	testDB := "test_tx_rollback_insert.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
		use ard/sql

		let db = sql::open("test_tx_rollback_insert.db").expect("Failed to open database")
		db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)").expect("Failed to create table")

		let tx = db.begin().expect("Failed to begin transaction")
		tx.exec("INSERT INTO users (name) VALUES ('Bob')").expect("Failed to insert in transaction")
		tx.rollback().expect("Failed to rollback transaction")

		let rows = db.query("SELECT * FROM users").all([:]).expect("Failed to query")
		if not rows.size() == 0 {
			panic("Expected 0 rows after rollback, got {rows.size()}")
		}
	`)
}

func TestBytecodeTransactionQueryRead(t *testing.T) {
	testDB := "test_tx_read.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
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

func TestBytecodeTransactionMultipleOperations(t *testing.T) {
	testDB := "test_tx_multiple.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
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

		let final_rows = db.query("SELECT * FROM items").all([:]).expect("Failed to query after commit")
		if not final_rows.size() == 2 {
			panic("Expected 2 rows after commit, got {final_rows.size()}")
		}
	`)
}

func TestBytecodeTransactionRollbackMultipleOperations(t *testing.T) {
	testDB := "test_tx_rollback_multiple.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
		use ard/sql

		let db = sql::open("test_tx_rollback_multiple.db").expect("Failed to open database")
		db.exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)").expect("Failed to create table")

		let tx = db.begin().expect("Failed to begin transaction")
		tx.exec("INSERT INTO items (name) VALUES ('Item1')").expect("Failed to insert Item1")
		tx.exec("INSERT INTO items (name) VALUES ('Item2')").expect("Failed to insert Item2")
		tx.rollback().expect("Failed to rollback transaction")

		let rows = db.query("SELECT * FROM items").all([:]).expect("Failed to query")
		if not rows.size() == 0 {
			panic("Expected 0 rows after rollback, got {rows.size()}")
		}
	`)
}

func TestBytecodeTransactionQueryWithParams(t *testing.T) {
	testDB := "test_tx_params.db"
	defer os.Remove(testDB)

	runBytecodeRaw(t, `
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

func TestBytecodeSQLPostgresNamedParams(t *testing.T) {
	dsn := os.Getenv("ARD_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set ARD_TEST_POSTGRES_DSN to run postgres integration tests")
	}

	runBytecodeRaw(t, fmt.Sprintf(`
		use ard/sql
		use ard/decode

		let db = sql::open(%q).expect("Failed to open postgres database")
		db.exec("CREATE TEMP TABLE ard_named_params (id TEXT PRIMARY KEY, name TEXT NOT NULL)").expect("Failed to create temp table")

		let insert = db.query("INSERT INTO ard_named_params (id, name) VALUES (@id, @name)")
		insert.run(["id": "abc", "name": "test"]).expect("Insert failed")

		let query = db.query("SELECT name FROM ard_named_params WHERE id = @id OR id = @id")
		let rows = query.all(["id": "abc"]).expect("Query failed")
		if not rows.size() == 1 {
			panic("Expected 1 result, got {rows.size()}")
		}

		let name = decode::run(rows.at(0), decode::field("name", decode::string)).expect("Failed to decode name")
		if not name == "test" {
			panic("Expected name 'test', got {name}")
		}
	`, dsn))
}
