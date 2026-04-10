# FFI Simplification Analysis

Thisdocument analyzes opportunities to simplify FFI functions by using opaque types and moving logic to Ard code.

## Core Pattern

The most effective FFI pattern uses:
1. **Opaque handles** (`extern type`) for Go objects
2. **Small FFI functions** that operate on handles
3. **Ard code** for orchestration and composition

### Success Story: SQL Module

The SQL module demonstrates this pattern well:

```ard
// std_lib/sql.ard
private extern type Db// Opaque handle
private extern type Tx// Opaque handle

// Simple FFI functions
private extern fn connect(connection_string: Str) Db!Str
private extern fn close_db(db: Db) Void!Str
private extern fn execute(conn: Conn, sql: Str, values: [Value]) Void!Str
private extern fn run_query(conn: Conn, sql: Str, values: [Value]) [Dynamic]!Str

// Ard code orchestrates
impl Query {
  fn prepare(mut query_str: Str, mut values: [Value], args: [Str: Value]) Void!Str {
    // Logic in Ard, not FFI
  }
}
```

The FFI functions are small and focused. Complex operations (parameter binding, query building) happen in Ard.

---

## Refactoring Opportunities

### 1. HTTP Module: Break Down `HTTP_Send`

**Current State:** Single 70-line raw FFI function that:
- Extracts 5 arguments from runtime objects
- Converts body types (string, bytes, any)
- Builds HTTP request
- Executes request
- Converts response headers to Ard map
- Builds Ard Response struct

**Proposed Refactoring:**

```ard
// std_lib/http.ard
private extern type HttpResponse// Opaque Go *http.Response// Simple FFI - just make request
private extern fn _http_do(method: Str, url: Str, body: Str?, headers: [Str: Str], timeout: Int?) HttpResponse!Str

// Accessors for response handle
private extern fn _response_status(resp: HttpResponse) Int
private extern fn _response_header(resp: HttpResponse, name: Str) Str?
private extern fn _response_headers(resp: HttpResponse) [Str: Str]
private extern fn _response_body(resp: HttpResponse) Str!Str
private extern fn _response_close(resp: HttpResponse) Void

// Ard orchestrates
fn send(req: Request, timeout: Int?) Response!Str {
  let method = req.method.to_str()
  let body = match req.body {
    s => maybe::some(s),
    _ => maybe::none(),// Wait for proper pattern matching on Dynamic
  }
  
  let resp = try _http_do(method, req.url, body, req.headers, timeout)
  defer _response_close(resp)// hypothetical defer
  
  let status = _response_status(resp)
  let headers = _response_headers(resp)
  let body = try _response_body(resp)
  
  Result::ok(Response{status: status, headers: headers, body: body})
}
```

**Benefits:**
- FFI functions become idiomatic (could be auto-generated)
- Response building logic moves to Ard
- Easier to test individual pieces
- Smaller FFI surface

**FFI Functions (all could be idiomatic):**

| Function | Signature | Type |
|----------|-----------|------|
| `_http_do` | `(string, string, *string, map[string]string, *int) (any, error)` | Idiomatic |
| `_response_status` | `(any) int` | Idiomatic |
| `_response_header` | `(any, string) *string` | Idiomatic |
| `_response_headers` | `(any) map[string]string` | Idiomatic |
| `_response_body` | `(any) (string, error)` | Idiomatic |
| `_response_close` | `(any)` | Idiomatic |

---

### 2. Decode Module: Simplify Error Construction

**Current State:** `DecodeString`, `DecodeInt`, etc. are raw FFI because they construct custom `Error` struct.

**Problem:** The `Error` struct has `expected`, `found`, `path` fields that require embedded module type lookup.

**Option A: Simpler Error Type**

Replace complex `Error` struct with simple string:

```ard
// Current
struct Error {
  expected: Str,
  found: Str,
  path: [Str],
}

// Simplified
type DecodeError = Str// Just a string
```

**Option B: Ard-Based Error Construction**

FFI returns raw value + type info, Ard constructs error:

```ard
// FFI returns what it found, Ard builds error
private extern fn _try_decode_string(data: Dynamic) Str?// Returns None if not string
private extern fn _try_decode_int(data: Dynamic) Int?
// etc.

// Ard handles errors
fn string(data: Dynamic) Str![Error] {
  match _try_decode_string(data) {
    s => Result::ok(s),
    _ => {
      let found = _describe_dynamic(data)// Get type description
      Result::err([Error{expected: "Str", found: found, path: []}])
    }
  }
}
```

**Recommendation:** Option A (simpler error) for initial refactoring. The current error type provides nice messages but adds significant complexity.

---

### 3. FS Module: Keep As-Is

The `FS_ListDir` function returns a list of structs:

```ard
struct DirEntry {
  name: Str,
  is_file: Bool,}
```

This is already well-structured:
- Simple scalar fields in the struct
- FFI builds the struct
- Would require struct return support in generator to simplify

**Future:** If generator supports struct returns with scalar fields, this could become idiomatic.

---

### 4. HTTP Serve: Keep As-Is

`HTTP_Serve` is inherently complex because:
- It registers Ard closures as HTTP handlers
- Converts Go `*http.Request` to Ard `Request` on each request
- Must handle concurrent requests safely

This pattern (Go calling Ard closures) is the inverse of normal FFI and requires VM/runtime integration.

**No simplification recommended.**

---

### 5. Dynamic Conversions: Use `[]any` Support

Once generator supports `[]any`:

| Current | Simplified |
|---------|------------|
| `ListToDynamic(args []*runtime.Object)` | `ListToDynamic(items []any) any` |
| `MapToDynamic(args []*runtime.Object)` | `MapToDynamic(m map[string]any) any` |
| `DynamicToList(args []*runtime.Object)` | `DynamicToList(data any) ([]any, error)` |

---

## New Opaque Types to Add

### HTTP Response Handle

```ard
// std_lib/http.ard
private extern type HttpResponse// *http.Response in Go

// Accessors
private extern fn _response_status(resp: HttpResponse) Int
private extern fn _response_headers(resp: HttpResponse) [Str: Str]
private extern fn _response_body(resp: HttpResponse) Str!Str
```

### Potential Future: SQL Rows Handle

```ard
// For streaming large result sets
private extern type Rows// *sql.Rows in Go
private extern fn _rows_next(rows: Rows) Bool
private extern fn _rows_scan(rows: Rows) Dynamic!Str
private extern fn _rows_close(rows: Rows) Void
```

---

## Implementation Priority

### Phase 1: Low-Hanging Fruit
1. Add `[]any` and `map[string]any` generator support
2. Convert `ListToDynamic`, `MapToDynamic` to idiomatic
3. Add `GetStdType()` helper

### Phase 2: HTTP Simplification
1. Create `HttpResponse` opaque type
2. Create accessor FFI functions (all idiomatic)
3. Refactor `send()` in Ard to use new functions
4. Keep `HTTP_Serve` as-is

### Phase 3: Decode Simplification (Optional)
1. Evaluate if simpler error type suffices
2. If complex errors needed, move construction to Ard

---

## Testing Strategy

For each refactoring:

```bash
# 1. Make FFI changes
cd compiler
go generate ./bytecode/vm

# 2. Run all tests
go test ./...# 3. Run stdlib tests via samples
go run main.go run std_lib/fs.ard
go run main.go run std_lib/http.ard

# 4. Format check
go run main.go format std_lib
```

---

## Summary

| Module | Current | Recommended |
|--------|---------|-------------|
| SQL | Well-structured | Keep as-is |
| HTTP Send | Single large FFI | Break into handle + accessors |
| HTTP Serve | Complex callback | Keep as-is (inherently complex) |
| Decode | Complex error struct | Simplify error OR move to Ard |
| FS ListDir | Returns struct list | Keep as-is (needs struct support) |
| Dynamic | Raw FFI | Convert to idiomatic with `[]any` |

The key insight: **FFI should be thin wrappers around Go operations. Logic belongs in Ard.**