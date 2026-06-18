# Standard Library FFI Audit

This audit tracks the remaining Ard standard-library bindings that still use companion Go FFI after the direct-Go import work. The goal is to distinguish real adapters from legacy forwarding wrappers and to identify places that can become more pure Ard.

## Migration order

1. **Easy pure-Ard cleanup**
   - Replace `ard/list::new` with a typed empty-list literal.
   - Replace `ard/dynamic::{from_str, from_int, from_float, from_bool}` with built-in `.to_dyn()` calls.

2. **Useful filesystem migration**
   - Refactor `fs::{write, copy, create_dir, create_file, append}` to Ard functions over direct Go calls.
   - Decide separately whether `fs::read` should preserve today's raw byte-to-string behavior or become UTF-8-validating via `Str::from_bytes`.

3. **Small SQL pure-Ard cleanup**
   - Move `sql::detect_driver` and `sql::extract_params` into Ard.
   - Consider moving pgx placeholder normalization into Ard if the SQL execution adapter remains in Go.

4. **Ongoing direct-Go capability backlog**
   - Add direct-Go support for package variables/globals, conversions, fixed arrays, variadic calls or slice spread, struct fields, interface alias assignability, and callback/interface bridging.
   - This is not required to finish the current stdlib polish branch; it unlocks larger future migrations.

## Module audit

### `ard/argv`

- Current adapter: `OsArgs`.
- Still necessary today.
- Reason: `os.Args` is a Go package global and the stdlib host can inject configured args for tests/embedding through `HostConfig.Args`.
- Pure Ard opportunity: `load()` is already Ard; no further action until package-global binding exists.

### `ard/async`

- Current extern-shaped API: `start`, `eval`, `wait_for`, `get_result`, opaque `WaitGroup`.
- Treat as compiler/runtime intrinsic rather than ordinary companion FFI.
- Reason: fibers are checked and lowered specially by the compiler/runtime. The extern declarations are legacy surface for this special lowering.
- Pure Ard opportunity: none until Ard has first-class runtime concurrency primitives implemented without compiler help.

### `ard/async/channel`

- Current adapters/special lowering: channel type, `new`, `send`, `recv`, `close`.
- Still necessary today.
- Reason: needs Go `chan T`, `make(chan T)`, blocking send/receive, close, receive-to-`Maybe`, and `recover` around send/close panics.

### `ard/base64`

- Current adapters: standard and URL-safe encode/decode with optional padding.
- Still necessary today.
- Reason: Go's API is method calls on package variables such as `base64.StdEncoding`, `RawStdEncoding`, `URLEncoding`, and `RawURLEncoding`; direct-Go imports do not expose package variables/globals yet.
- Future unlock: direct package-global values plus method calls on those values.

### `ard/byte`

- Current adapter: `ByteFromInt`.
- Still necessary today.
- Reason: Ard needs checked `Int -> Byte?`; direct Go scalar coercions are call-boundary conversions, not standalone Ard conversion expressions returning `Maybe`.

### `ard/rune`

- Current adapters: `RuneFromInt`, `RuneFromStr`.
- `RuneFromInt` still needs an adapter for checked `Int -> Rune?`.
- `RuneFromStr` could be pure Ard by checking `value.runes().size() == 1`, assuming Ard `Str` values are always valid UTF-8. The current adapter also rejects invalid UTF-8 Go strings, so changing it would slightly narrow the validation surface.

### `ard/string`

- Current adapters: `StrFromUtf8`, `StrFromRunes`.
- Still necessary today.
- Reason: they validate UTF-8 / Unicode scalar values and perform Go conversions from `[]byte` or `[]rune` to `string`.
- Future unlock: direct conversion support plus explicit validation helpers.

### `ard/crypto`

- Current adapters: MD5, SHA-256, SHA-512, bcrypt hash/verify, scrypt hash/verify, UUID.
- Still necessary today.
- Reasons:
  - Go hash functions return fixed arrays (`[16]byte`, `[32]byte`, `[64]byte`) while Ard exposes `[Byte]`.
  - bcrypt/scrypt wrappers choose defaults, validate parameters, normalize password text, generate salts, format stored hashes, and perform constant-time comparisons.
  - UUID needs random bytes, byte mutation, and formatting.
- Future unlock: fixed-array-to-slice adapters, package globals/conversions, variadic formatting, and possibly richer pure-Ard byte manipulation.

### `ard/dynamic`

- Current adapters: primitive boxing, `Void` boxing, list/map boxing, bytes boxing.
- Can refactor now:
  - `from_str`
  - `from_int`
  - `from_float`
  - `from_bool`
- Still necessary today:
  - `from_void()` because `Void` has no `.to_dyn()` surface.
  - `from_list([Dynamic])` because it needs `[]Dynamic -> []any` boxing.
  - `object([Str: Dynamic])` because it needs map boxing.
  - `from_bytes([Byte])` because list/slice boxing is not available as pure Ard.

### `ard/decode`

- Current adapters: primitive dynamic decoders, `is_void`, JSON-to-dynamic, dynamic list/map extraction, field extraction.
- Still necessary today.
- Reason: this module is mostly runtime `any` introspection and type assertions. Ard cannot inspect `Dynamic` payload shapes directly without Go helpers.
- Pure Ard opportunity: the higher-level decoder composition (`nullable`, `list`, `map`, `field`, `path`, `one_of`, `flatten`) is already Ard.

### `ard/encode` and `ard/json`

- Current externs: `encode::json`, `json::parse`, `json::encode`.
- Treat `json::parse` and `json::encode` as compiler-backed intrinsics rather than ordinary companion FFI; the Go backend special-lowers them with type-specific JSON helpers.
- `encode::json` still uses `JsonEncode` over `Encodable` and remains tied to dynamic/runtime encoding.
- Pure Ard opportunity: none without reflection/deriving or broader compile-time code generation support.

### `ard/float`

- Current remaining adapter: `FloatFromInt`.
- Still necessary today.
- Reason: Ard does not currently have a pure expression for checked/intended `Int -> Float`; direct Go scalar coercions only happen at Go call boundaries.
- Other helpers already use direct Go (`math`, `strconv`) or Ard control flow.

### `ard/fs`

- Current adapters: exists/is-file/is-dir/create-file/write/append/read/copy/create-dir/list-dir.
- Can refactor now with Ard + direct Go:
  - `write`: `os::WriteFile(path, content.bytes(), 420)` using the current `0644` permission value.
  - `copy`: `os::ReadFile` then `os::WriteFile` with the current `0644` permission value.
  - `create_dir`: `os::MkdirAll(path, 493)` using the current `0755` permission value.
  - `create_file`: `os::Create` then `os::File::Close`
  - `append`: `os::OpenFile` with flags plus `os::File::WriteString` and `Close`
- Needs decision:
  - `read` can become `os::ReadFile` plus `Str::from_bytes`, but that changes semantics from raw byte-to-string conversion to UTF-8 validation.
- Still necessary today:
  - `exists`, `is_file`, `is_dir` because `os.Stat` returns `os.FileInfo`, an interface type not currently representable as direct Ard type.
  - `list_dir` because `os.ReadDir` returns `[]os.DirEntry`, also an interface type surface, and the stdlib returns a stable Ard `DirEntry` struct.

### `ard/http`

- Current adapters: opaque raw request/response handles, request path/query helpers, client request execution, response extraction/close, server callback bridge.
- Still necessary today.
- Reasons:
  - needs Go struct field access (`req.URL`, `resp.Body`, `resp.StatusCode`, `resp.Header`);
  - constructs `http.Client` with timeout;
  - adapts `Dynamic` request bodies to `io.Reader`, including JSON encoding;
  - reads and closes response bodies;
  - converts Go headers to `[Str: Str]`;
  - bridges Ard handlers to `http.HandlerFunc` and `http.ResponseWriter`.
- Future unlock: struct fields, package globals/functions with interface params, callback/interface bridging, and lifecycle helpers.

### `ard/io`

- Current adapters: `_print`, `read_line`.
- Still necessary today.
- Reasons:
  - `fmt.Println` is variadic.
  - `read_line` keeps buffered stdin state and trims EOF/newline behavior.

### `ard/sql`

- Current adapters: driver detection, open/close, exec/query, begin/commit/rollback, parameter extraction.
- Can refactor now to pure Ard:
  - `detect_driver`
  - `extract_params`
- Could later move to direct Go after type cleanup:
  - `connect` mostly maps to `sql.Open` plus `Ping`, but driver blank imports still need Go-side registration.
  - `close_db`, `begin_tx`, `commit_tx`, and `rollback_tx` can use direct Go pointer method calls once the Ard aliases are changed to direct Go types.
- Still necessary today:
  - `execute` and `run_query` because `Exec`/`Query` are variadic, values need `[]Value -> []any`, rows need dynamic scanning, and SQL byte values are normalized to strings.
  - blank imports for sqlite/mysql/postgres drivers.
  - connection/transaction union dispatch to a common runner.

## Direct-Go feature backlog exposed by this audit

These capabilities would let future branches remove more companion FFI:

- Package variables/globals as values (`os.Args`, `base64.StdEncoding`).
- Explicit Go conversions in Ard or at direct-call boundaries (`[]byte -> string`, `int -> byte`, `int -> rune`, `int -> float64`).
- Fixed array to slice/list adaptation for crypto hashes.
- Variadic Go calls and/or Ard slice spread for APIs like `fmt.Println`, `fmt.Sprintf`, SQL `Exec`/`Query`.
- Struct field access for Go values (`http.Response.StatusCode`, `Request.URL`, headers/body fields).
- Interface alias assignability and method access for Go interface surfaces such as `os.FileInfo` and `os.DirEntry`.
- Callback/interface bridging for HTTP server handlers and `http.ResponseWriter`.
- Controlled blank-import/dependency registration for database drivers.
