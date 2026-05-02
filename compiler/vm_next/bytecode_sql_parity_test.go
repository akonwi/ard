package vm_next

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestVMNextBytecodeParitySQLExtractParams(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "extracts params in query order",
			input: `
				use ard/sql

				let names = sql::extract_params("SELECT f.*, fs.* FROM fixtures f
					JOIN fixture_stats fs ON f.id = fs.fixture_id
					WHERE f.league_id = @league_id AND f.season = @season
					AND (f.home_id = @team_id OR f.away_id = @team_id)
					AND f.finished = true"
				)
				names.size()
			`,
			want: 4,
		},
		{
			name: "keeps overlapping param names in appearance order",
			input: `
				use ard/sql

				let names = sql::extract_params("INSERT INTO teams (team, team_id) VALUES (@team, @team_id)")
				names.at(0) + "," + names.at(1)
			`,
			want: "team,team_id",
		},
	})
}

func TestVMNextBytecodeParitySQLQueryRunAllAndFirst(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "query.db")

	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "query all decodes row fields",
			input: fmt.Sprintf(`
				use ard/sql
				use ard/decode

				let db = sql::open(%q).expect("Failed to open database")
				db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")
				db.exec("INSERT INTO players (name, number) VALUES ('John Doe', 2)").expect("Failed to insert player 1")
				db.exec("INSERT INTO players (name, number) VALUES ('Jane Smith', 5)").expect("Failed to insert player 2")

				let query = db.query("SELECT id, name, number FROM players WHERE number = @number")
				let rows = query.all(["number": 5]).expect("Failed to query players")
				let decode_name = decode::field("name", decode::string)
				decode_name(rows.at(0)).expect("Unable to decode row")
			`, dbPath),
			want: "Jane Smith",
		},
		{
			name: "query first returns maybe row",
			input: fmt.Sprintf(`
				use ard/sql
				use ard/decode

				let db = sql::open(%q).expect("Failed to open database")
				let query = db.query("SELECT id FROM players WHERE number = @number")
				let maybe_row = query.first(["number": 5]).expect("Failed to query players")
				let row = maybe_row.expect("Found none")
				decode::run(row, decode::field("id", decode::int)).expect("Failed to decode id")
			`, dbPath),
			want: 2,
		},
	})
}

func TestVMNextBytecodeParitySQLInsertingNull(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "maybe.db")

	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "void sql value round-trips as nullable field",
			input: fmt.Sprintf(`
				use ard/sql
				use ard/decode

				let db = sql::open(%q).expect("Failed to open database")
				let create_table = db.query("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)")
				create_table.run([:]).expect("Failed to create table")

				let stmt = db.query("INSERT INTO users (name, email) VALUES (@name, @email)")
				let values: [Str:sql::Value] = [
					"name": "John Doe",
					"email": ()
				]
				stmt.run(values).expect("Failed to insert row")

				let query = db.query("SELECT email FROM users WHERE id = 1")
				let rows = query.all(values).expect("Failed to find users with id 1")
				let email = decode::run(rows.at(0), decode::field("email", decode::nullable(decode::string))).expect("Failed to decode email")
				email.is_none()
			`, dbPath),
			want: true,
		},
	})
}

func TestVMNextBytecodeParitySQLMissingParameters(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")

	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "query prepare reports missing parameter",
			input: fmt.Sprintf(`
		use ard/sql

		let db = sql::open(%q).expect("Failed to open database")
		db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")

		let stmt = db.query("INSERT INTO players (name, number) VALUES (@name, @number)")
		match stmt.run([ "name": "John Doe", "int": 2]) {
			err(msg) => msg,
			ok(_) => "unexpected success",
		}
	`, dbPath),
			want: "Missing parameter: number",
		},
	})
}

func TestVMNextBytecodeParitySQLTransactions(t *testing.T) {
	rollbackDB := filepath.Join(t.TempDir(), "rollback.db")
	commitDB := filepath.Join(t.TempDir(), "commit.db")

	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "rollback discards inserted rows",
			input: fmt.Sprintf(`
				use ard/sql

				let db = sql::open(%q).expect("Failed to open database")
				db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)").expect("Failed to create table")

				let tx = db.begin().expect("Failed to begin transaction")
				tx.exec("INSERT INTO users (id, name) VALUES (1, 'joe')").expect("Failed to insert")
				tx.rollback().expect("Failed to rollback transaction")

				let rows = db.query("SELECT * FROM users").all([:]).expect("Failed to get all")
				rows.size()
			`, rollbackDB),
			want: 0,
		},
		{
			name: "commit persists inserted rows and query params work in tx",
			input: fmt.Sprintf(`
				use ard/sql
				use ard/decode

				let db = sql::open(%q).expect("Failed to open database")
				db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)").expect("Failed to create table")

				let tx = db.begin().expect("Failed to begin transaction")
				tx.exec("INSERT INTO users (name, age) VALUES ('User1', 25)").expect("Failed to insert User1")
				tx.exec("INSERT INTO users (name, age) VALUES ('User2', 30)").expect("Failed to insert User2")
				let rows = tx.query("SELECT * FROM users WHERE age = @age").all(["age": 30]).expect("Failed to query in transaction")
				tx.commit().expect("Failed to commit transaction")

				decode::run(rows.at(0), decode::field("name", decode::string)).expect("Failed to decode name")
			`, commitDB),
			want: "User2",
		},
	})
}
