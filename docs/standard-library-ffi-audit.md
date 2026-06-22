# Standard Library FFI Audit

This audit tracks the remaining Ard standard-library bindings that still use companion Go FFI after the direct-Go import work. The goal is to distinguish real adapters from legacy forwarding wrappers and to identify places that can become more pure Ard.

## Migration order

1. **Completed easy pure-Ard cleanup**
   - `ard/list::new` now returns a typed empty-list literal.
   - `ard/dynamic::{from_str, from_int, from_float, from_bool}` now use built-in `.to_dyn()` calls.

2. **Partially completed filesystem migration**
   - `fs::{write, copy, create_dir, create_file, append, read, read_bytes}` now use Ard functions over direct Go calls.
   - `fs::read` is now a UTF-8-validating text reader; `fs::read_bytes` preserves raw bytes for binary data.

3. **Completed small SQL pure-Ard cleanup**
   - `sql::detect_driver` and `sql::extract_params` now live in Ard.
   - Consider moving pgx placeholder normalization into Ard if the SQL execution adapter remains in Go.

4. **Completed first direct-Go struct-field stdlib cleanup**
   - `http::Request::{path,path_param,query_param}` now use direct `*http.Request` methods/fields plus `ffi::is_nil`.
   - `http::send` now reads `http.Response.StatusCode` directly instead of through an FFI wrapper.

5. **Completed direct-Go package value and construction audit**
   - `ard/base64` now uses direct Go package variables plus method calls for the standard encodings.
   - Direct-Go keyed struct construction is available, but this audit did not find an existing stdlib companion wrapper that is only constructing a simple Go struct and can now be deleted.
   - The constructible structs in currently imported Go packages are either not exposed by Ard's stdlib yet (`http.Cookie`, simple Go error structs), require unsafe zero values over unexported state (`time.Time`, `base64.Encoding`, `strings.Builder`), or still depend on unsupported field shapes (`http.Client`, `http.Request`, `http.Response`, `http.Server`, `os.ProcAttr`, `io.LimitedReader`).

6. **Completed unsafe/recovering interop block**
   - `unsafe { ... }` now converts same-goroutine Go/runtime panics into `T!Str` results and can be used with `try` around direct-Go nil/panic risks.
   - `break` remains rejected inside unsafe blocks until explicit control-flow semantics are designed.

7. **Completed final small pure-Ard cleanup**
   - `rune::from_str` now uses Ard string rune views and `Maybe`; only checked `Int -> Rune?` remains in companion FFI.

8. **Ongoing direct-Go capability backlog**
   - Add direct-Go support for explicit conversions, fixed arrays, variadic calls or slice spread, named map/slice alias assignment, zero/nil construction policy, embedded/promoted fields, Ard-defined Go-interface implementations, and callback/interface bridging.
   - This is not required to finish the current stdlib polish branch; it unlocks larger future migrations.

## Module audit

### `ard/argv`

- Current adapter: `OsArgs`.
- Still necessary today.
- Reason: direct Go package variables are available, but `ard/argv` intentionally supports test/embedding injection through `HostConfig.Args`; reading `os::Args` directly would bypass that host configuration.
- Pure Ard opportunity: `load()` is already Ard; no construction-related action.

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

- No companion FFI remains for the public encode/decode helpers.
- Refactored to Ard + direct Go package values:
  - `base64::StdEncoding`, `RawStdEncoding`, `URLEncoding`, and `RawURLEncoding` are direct Go package variables.
  - Encoding/decoding lowers to method calls on those Go values.
- Construction audit note: `encoding/base64.Encoding{}` is syntactically constructible because it has no exported fields, but it relies on unexported state; stdlib should continue using Go's exported package variables/constructors instead of direct zero-value construction.

### Direct-Go-only utility modules

- `ard/dates` and `ard/duration` use `time` functions/constants directly; struct construction does not remove any companion FFI. Zero-field Go structs such as `time.Time`/`time.Location` should not be introduced as a refactor unless the API intentionally wants Go zero values.
- `ard/env` uses `os::LookupEnv` directly; no construction-related opportunity.
- `ard/hex` uses `encoding/hex` functions directly; no construction-related opportunity.
- `ard/string::split` uses `strings::Split` directly. `strings.Builder`, `Reader`, and `Replacer` are zero-exported-field structs with unexported state/invariants; use Go constructors/functions rather than direct zero-value literals.

### `ard/byte`

- Current adapter: `ByteFromInt`.
- Still necessary today.
- Reason: Ard needs checked `Int -> Byte?`; direct Go scalar coercions are call-boundary conversions, not standalone Ard conversion expressions returning `Maybe`.

### `ard/rune`

- Current adapter: `RuneFromInt`.
- `RuneFromInt` still needs an adapter for checked `Int -> Rune?`.
- `RuneFromStr` has been refactored to pure Ard by checking `value.runes().size() == 1` and returning that sole rune. This assumes Ard `Str` values are valid UTF-8 at the source/runtime boundary; invalid byte sequences should be rejected when constructing a `Str`, for example by `Str::from_bytes`.

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
- Refactored to pure Ard:
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
- Interim note: `is_void` still declares a concrete `IsNil` extern so the generated stdlib host contract includes the nil helper while generic host-contract generation remains limited.
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

- Current adapters: exists/is-file/is-dir/list-dir.
- Refactored to Ard + direct Go:
  - `write`: `os::WriteFile(path, content.bytes(), FILE_MODE)` using the current `0644` permission value.
  - `copy`: `os::ReadFile` then `os::WriteFile` with the current `0644` permission value.
  - `create_dir`: `os::MkdirAll(path, DIR_MODE)` using the current `0755` permission value.
  - `create_file`: `os::Create` then `os::File::Close`.
  - `append`: create-if-missing, `os::OpenFile` write-only, seek-to-end, `os::File::WriteString`, and `Close`.
  - `read`: `read_bytes` plus `Str::from_bytes`, making `read` a UTF-8-validating text reader.
  - `read_bytes`: `os::ReadFile` for raw bytes and binary data.
- Retained for now, but direct-Go interface assignability/method access now unblocks pure Ard replacements:
  - `exists`, `is_file`, and `is_dir` can be expressed over `os::Stat` plus `os.FileInfo` methods.
  - `list_dir` can be expressed over `os::ReadDir` plus `os.DirEntry` methods while preserving the stable Ard `DirEntry` struct.
- Construction audit note: `os.Process{Pid: ...}` is constructible, and some Go error structs are close except for `error` fields, but none map to current `ard/fs` APIs. `os.ProcAttr` still has pointer/slice fields that are awkward without nil/zero construction policy and process-start APIs.

### `ard/http`

- Current adapters: client request execution, response header/body extraction/close, server callback bridge.
- Refactored to Ard + direct Go:
  - raw request/response host FFI signatures now use direct `mut gohttp::{Request,Response}` types without private aliases;
  - inbound `Request::{path,path_param,query_param}` use direct `*http.Request` field/method access;
  - client response status uses direct `http.Response.StatusCode` access.
- Still necessary today.
- Reasons:
  - `http_do` constructs `http.Client` with timeout and adapts `Dynamic` request bodies to `io.Reader`, including JSON encoding;
  - response body/header helpers convert Go headers/bodies to Ard values and manage response body lifecycle;
  - `serve` bridges Ard handlers to `http.HandlerFunc` and `http.ResponseWriter`.
- Construction audit note:
  - `http.Cookie` is the best new direct-construction candidate (`Cookie{...}` with scalar/list/time fields), but `ard/http` does not expose cookies yet, so this is a new API opportunity rather than a refactor.
  - `http.Client`, `Request`, `Response`, `Server`, and `Transport` remain poor direct-construction targets because their exported fields include interfaces, function callbacks, typed nil pointers, named map aliases, and lifecycle-sensitive bodies.
  - `http.MaxBytesError` and `http.ProtocolError` are constructible/simple, but not currently used by stdlib APIs.
- Future unlock: callback/interface bridging, named `http.Header` map adaptation, nil/zero construction policy, and lifecycle helpers.

### `ard/io`

- Current adapters: `_print`, `read_line`.
- Still necessary today.
- Reasons:
  - `fmt.Println` is variadic.
  - `read_line` keeps buffered stdin state and trims EOF/newline behavior.

### `ard/list`

- `new` has been refactored to pure Ard using a typed empty-list literal.
- No companion FFI remains for this module.

### `ard/sql`

- Current adapters: open/close, exec/query, begin/commit/rollback.
- Refactored to pure Ard:
  - `detect_driver`
  - `extract_params`
- Could later move to direct Go after type cleanup:
  - `connect` mostly maps to `sql.Open` plus `Ping`, but driver blank imports still need Go-side registration.
  - `close_db`, `begin_tx`, `commit_tx`, and `rollback_tx` can use direct Go pointer method calls once the Ard aliases are changed to direct Go types.
- Construction audit note: the SQL stdlib does not currently wrap construction of Go config structs. The blockers are method/variadic/dynamic row adaptation and driver registration, not struct literals.
- Still necessary today:
  - `execute` and `run_query` because `Exec`/`Query` are variadic, values need `[]Value -> []any`, rows need dynamic scanning, and SQL byte values are normalized to strings.
  - blank imports for sqlite/mysql/postgres drivers.
  - connection/transaction union dispatch to a common runner.

## Direct-Go feature backlog exposed by this audit

These capabilities would let future branches remove more companion FFI:

- Explicit Go conversions in Ard or at direct-call boundaries (`[]byte -> string`, `int -> byte`, `int -> rune`, `int -> float64`).
- Fixed array to slice/list adaptation for crypto hashes.
- Variadic Go calls and/or Ard slice spread for APIs like `fmt.Println`, `fmt.Sprintf`, SQL `Exec`/`Query`.
- Named Go map/slice alias adaptation, especially `http.Header` and `url.Values`.
- Direct nil/zero-value construction policy for Go pointer/interface/function fields, where safe.
- Embedded/promoted fields.
- Ard-defined type/function adapters for implementing Go interfaces such as `http.HandlerFunc` and `http.ResponseWriter`.
- Callback/interface bridging for HTTP server handlers and other Go callback-shaped APIs.
- Controlled blank-import/dependency registration for database drivers.
