# SQLite Query Result Decoding

This document outlines the design for integrating SQLite query results with Ard's decode system.

## API Design

The goal is to separate database querying from type decoding:

```ard
// Execute query, return all rows as Dynamic data
let rows = db.query("SELECT id, name, email FROM users WHERE active = 1").expect("Query failed")

// Decode the raw data using existing decoder infrastructure  
let users = decode::run(rows, decode::list(user_decoder)).expect("Decode failed")
```

### Key Design Principles

1. **Separation of concerns** - Database operations vs type conversion
2. **Eager evaluation** - All rows scanned into memory immediately
3. **Reuse existing decoders** - No new decoder types needed
4. **Consistent error handling** - `Result` types throughout

## Function Signatures

### Database Module

```ard
// New method on Database struct
fn query(sql: Str) Dynamic!Str
```

- **Input**: SQL query string
- **Output**: `Result<Dynamic, Str>` containing all rows or error message
- **Behavior**: Executes query, scans all rows, closes statement, returns data

### Decode Module  

```ard
// Existing function, works with SQL data
fn run(data: Dynamic, decoder: $Decoder) $T![$Error]
```

- **Input**: Dynamic data (from any source) and decoder function
- **Output**: Decoded result or list of decode errors
- **Behavior**: Applies decoder to data, same as current implementation

## Data Representation

### SQL Query Results → Dynamic Format

SQL query results are converted to the same format as JSON data:

```go
// For query: "SELECT id, name, email FROM users"  
// Each row becomes a map, all rows become a slice
rowData := []interface{}{
    map[string]interface{}{
        "id":    1,                    // INTEGER → int
        "name":  "John",               // TEXT → string  
        "email": "john@example.com",   // TEXT → string
    },
    map[string]interface{}{
        "id":    2,
        "name":  "Jane", 
        "email": nil,                  // NULL → Go nil
    },
}

// Wrapped as Dynamic object
return &object{
    raw:   rowData,
    _type: checker.Dynamic,
}
```

### SQL Type → Go Type Mapping

| SQL Type | Go Type | Notes |
|----------|---------|-------|
| INTEGER | `int` | Direct mapping |
| REAL | `float64` | Direct mapping |
| TEXT | `string` | Direct mapping |
| BLOB | `[]byte` | Direct mapping |
| NULL | `nil` | Becomes Go nil |

### Column Name Mapping

- Column names become map keys exactly as returned by SQL
- Case-sensitive matching with struct field names
- Handles `SELECT *` and explicit column lists identically

## Decoder Usage Patterns

### Basic Field Extraction

```ard
let rows = db.query("SELECT id, name FROM users").expect("Query failed")

// Extract single field from first row
let first_id = decode::run(rows, 
    decode::index(0, decode::field("id", decode::int()))
).expect("Decode failed")

// Extract all names
let names = decode::run(rows, 
    decode::list(decode::field("name", decode::string()))
).expect("Decode failed")
```

### Struct-like Decoding

```ard
// Define decoder for User-like structure
let user_decoder = /* composition of field decoders */

let users = decode::run(rows, decode::list(user_decoder)).expect("Decode failed")
```

### Handling NULL Values

```ard
let rows = db.query("SELECT id, email FROM users").expect("Query failed")

let users = decode::run(rows, decode::list(
    // Combine required and optional fields
    decode::combine(
        decode::field("id", decode::int()),
        decode::field("email", decode::nullable(decode::string()))
    )
)).expect("Decode failed")
```

## Implementation Details

### SQLite Module Changes

1. **Add `query` method to Database struct**:
   ```go
   case "query":
       return Symbol{Name: name, Type: &FunctionDef{
           Name:       name,
           Parameters: []Parameter{{Name: "sql", Type: Str}},
           ReturnType: MakeResult(Dynamic, Str),
       }}
   ```

2. **VM implementation**:
   ```go
   func (m *SQLiteModule) handleQuery(db *sql.DB, query string) *object {
       rows, err := db.Query(query)
       if err != nil {
           return makeErr(&object{err.Error(), Str}, resultType)
       }
       defer rows.Close()
       
       // Get column names
       columns, err := rows.Columns()
       // ... scan all rows into []interface{} of map[string]interface{}
       
       return makeOk(&object{raw: rowData, _type: Dynamic}, resultType)
   }
   ```

### No Decode Module Changes

The existing decode infrastructure handles this data format automatically:
- `decode::field()` works with `map[string]interface{}`
- `decode::list()` works with `[]interface{}`
- `decode::nullable()` handles `nil` values
- All primitive decoders handle the Go types correctly

## Error Handling

### Query Errors

```ard
let result = db.query("INVALID SQL")
match result {
    ok(rows) => /* process rows */,
    err(message) => io::print("SQL Error: {message}")
}
```

### Decode Errors

```ard
let result = decode::run(rows, bad_decoder)
match result {
    ok(data) => /* use data */,
    err(errors) => {
        for error in errors {
            io::print("Decode error at {error.path}: expected {error.expected}, found {error.found}")
        }
    }
}
```

## Benefits

1. **Familiar API** - Developers already know `decode::run()` and decoder composition
2. **Composable** - Any decoder that works with objects/maps works with SQL data
3. **Type safe** - Full type checking on decoded results
4. **Error rich** - Detailed error messages with field paths
5. **Memory efficient** - Eager loading allows connection cleanup
6. **No magic** - Explicit, predictable data flow

## Future Considerations

### Streaming Support

If needed later, could add streaming API:
```ard
let stream = db.query_stream("SELECT * FROM large_table").expect("Query failed")
for row in stream {
    let user = decode::run(row, user_decoder).expect("Decode failed")
    // process user
}
```

### Prepared Statements

Could extend with parameterized queries:
```ard
let stmt = db.prepare("SELECT * FROM users WHERE age > ?").expect("Prepare failed")
let rows = stmt.query([25]).expect("Query failed")
```

### Schema Validation

Could add optional schema validation:
```ard
let rows = db.query("SELECT * FROM users").expect("Query failed")
let users = decode::run(rows, decode::schema(UserSchema)).expect("Decode failed")
```
