---
title: Database Access with ard/sql
description: Query relational databases using Ard's generic SQL module supporting SQLite, PostgreSQL, MySQL, and more.
---

The `ard/sql` module provides a type-safe, driver-agnostic interface for accessing relational databases. It supports SQLite, PostgreSQL, MySQL, and others.

The SQL module provides:
- **Connection management** with automatic driver detection
- **Parameterized queries** with `@param` syntax to prevent SQL injection
- **Row Objects** row objects are instances of `Dynamic` and decodable with `ard/decode`

```ard
use ard/io
use ard/decode
use ard/sql

fn main() {
  let db = sql::open("postgres://user:pass@localhost/mydb").expect("connection failed")
  
  let query = db.query("SELECT id, name FROM users WHERE age > @min_age")
  let rows = query.all(["min_age": 18]).expect("query failed")
  
  for row in rows {
    let name = decode::run(row, decode::field("name", decode::string)).expect("decode failed")
    io::print(name) 
  }
  
  db.close().expect("close failed")
}
```

## API

### `fn open(path: Str) Database!Str`

This function accepts a path to a database and returns a result of connection object or error message, in case of failure

```ard
use ard/sql

// SQLite
sql::open("test.db")

// PostgreSQL
sql::open("postgres://user:password@localhost:5432/dbname")

// MySQL
sql::open("user:password@tcp(localhost:3306)/dbname")
```

### `fn Database.exec(stmt: Str) Void!Str`

Use `exec()` for operations which may not return rows or to ignore the results.

```ard
use ard/sql

let db = sql::open("test.db").expect("Failed to open")
db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)").expect("Failed to create")
db.exec("INSERT INTO users (name) VALUES ('Alice')").expect("Failed to insert")
```

:::note
The `exec()` function does not support parameter sanitization. Be wary of using templated strings for variables.

For safe usage of external values, use the below methods.
:::

### `fn Database.query(stmt: Str) sql::Query`

Use `query()` to create reusable query objects that can be executed with arguments to prevent SQL injection.

Variables in the sql string are prefixed with `@` and will be expected in a `Map` of arguments when the query is invoked.

```ard
use ard/sql

let db = sql::open("test.db").expect("Failed to open")
let stmt = db.query("INSERT INTO users (name, age) VALUES (@name, @age)")

let insert_values: [Str : sql::Value] = [
  "name": "Bob",
  "age": 25,
]

stmt.run(insert_values).expect("Insert failed")
```

The `Query` struct has the following methods to choose how many rows are returned:


__`run(args: [Str: sql::Value]) Void!Str`__: Similar to `Database.exec()` doesn't return results

__`all(args: [Str: sql::Value]) [Dynamic]!Str`__: Returns all the found rows as a list of `Dynamic`, which can be decoded with the `ard/decode` module

__`first(args: [Str: sql::Value]) Dynamic?!Str`__: Returns the first row as a nullable. Only query issues will result in errors
