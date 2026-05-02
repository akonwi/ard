package vm_next

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	stdlibffi "github.com/akonwi/ard/std_lib/ffi"
)

func TestVMNextBytecodeParityDurationFunctions(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "from_seconds",
			input: `
				use ard/duration
				duration::from_seconds(20)
			`,
			want: 20_000_000_000,
		},
		{
			name: "from_minutes",
			input: `
				use ard/duration
				duration::from_minutes(5)
			`,
			want: 300_000_000_000,
		},
		{
			name: "from_hours",
			input: `
				use ard/duration
				duration::from_hours(2)
			`,
			want: 7_200_000_000_000,
		},
	})
}

func TestVMNextBytecodeParityDynamicDecode(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "Dynamic::list can decode back to a list",
			input: `
				use ard/decode

				let foo = [1,2,3]
				let data = Dynamic::list(from: foo, of: Dynamic::from_int)
				let list = decode::run(data, decode::list(decode::int)).expect("Couldn't decode data")
				list.at(1)
			`,
			want: 2,
		},
		{
			name: "Dynamic::object can decode back to a map",
			input: `
				use ard/decode

				let data = Dynamic::object([
					"foo": Dynamic::from_int(0),
					"baz": Dynamic::from_int(1),
				])

				let map = decode::run(data, decode::map(decode::string, decode::int)).expect("Couldn't decode data")
				map.get("foo").or(-1)
			`,
			want: 0,
		},
	})
}

func TestVMNextBytecodeParityDecodePrimitives(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "decode string from external data",
			input: `
				use ard/decode
				let data = Dynamic::from("hello")
				decode::run(data, decode::string).expect("")
			`,
			want: "hello",
		},
		{
			name: "decode int from external data",
			input: `
				use ard/decode
				let data = Dynamic::from(42)
				decode::run(data, decode::int).expect("Failed to decode")
			`,
			want: 42,
		},
		{
			name: "decode float from external data",
			input: `
				use ard/decode
				let data = Dynamic::from(3.14)
				decode::run(data, decode::float).expect("")
			`,
			want: 3.14,
		},
		{
			name: "decode bool from external data",
			input: `
				use ard/decode
				let data = Dynamic::from(true)
				decode::run(data, decode::bool).expect("")
			`,
			want: true,
		},
	})
}

func TestVMNextBytecodeParityDecodeErrors(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "string decoder fails on int - returns error list",
			input: `
				use ard/decode
				let data = Dynamic::from(42)
				let result = decode::run(data, decode::string)
				match result {
					err(errs) => {
						if not errs.size() == 1 { panic("Expected 1 error. Got {errs.size()}") }
						errs.at(0).to_str()
					},
					ok(_) => ""
				}
			`,
			want: "got 42, expected Str",
		},
		{
			name: "int decoder fails on string",
			input: `
				use ard/decode
				let data = Dynamic::from("hello")
				let result = decode::run(data, decode::int)
				result.is_err()
			`,
			want: true,
		},
		{
			name: "list item errors include the failed index",
			input: `
				use ard/decode
				let data = decode::from_json("[1, false, 3]").expect("Failed to parse json")
				let result = decode::run(data, decode::list(decode::int))
				match result {
					err(errs) => {
						if not errs.size() == 1 { panic("Expected 1 error: got {errs.size()}") }
						errs.at(0).to_str()
					}
					ok(_) => ""
				}
			`,
			want: "[1]: got false, expected Int",
		},
		{
			name: "error path with nested field and list",
			input: `
				use ard/decode
				let data = decode::from_json("[\{\"value\": \"not_a_number\"\}]").expect("Unable to parse json")
				let result = decode::run(data, decode::list(decode::field("value", decode::int)))
				match result {
					ok => "unexpected success",
					err(errs) => errs.at(0).to_str()
				}
			`,
			want: "[0].value: got \"not_a_number\", expected Int",
		},
	})
}

func TestVMNextBytecodeParityFromJSONInputTypes(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "from_json accepts Str input",
			input: `
				use ard/decode
				let data = decode::from_json("42").expect("parse")
				decode::run(data, decode::int).expect("decode")
			`,
			want: 42,
		},
		{
			name: "from_json accepts Dynamic string input",
			input: `
				use ard/decode
				let raw = Dynamic::from("\{\"count\": 7\}")
				let data = decode::from_json(raw).expect("parse")
				decode::run(data, decode::field("count", decode::int)).expect("decode")
			`,
			want: 7,
		},
		{
			name: "from_json rejects non-string Dynamic input",
			input: `
				use ard/decode
				let raw = Dynamic::from(42)
				match decode::from_json(raw) {
					err(msg) => msg,
					ok(_) => "unexpected success",
				}
			`,
			want: "Expected a JSON string, got 42",
		},
	})
}

func TestVMNextBytecodeParityDecodeNullableListMapAndField(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "nullable string decoder with valid string returns some",
			input: `
				use ard/decode
				let data = Dynamic::from("hello")
				let maybe_str = decode::run(data, decode::nullable(decode::string)).expect("Decoding failed")
				match maybe_str {
					str => str == "hello",
					_ => false
				}
			`,
			want: true,
		},
		{
			name: "nullable string on null returns none",
			input: `
				use ard/decode
				let data = decode::from_json("null").expect("parse")
				let result = decode::run(data, decode::nullable(decode::string)).expect("decode")
				result.is_none()
			`,
			want: true,
		},
		{
			name: "can decode a list",
			input: `
				use ard/decode
				let data = decode::from_json("[1, 2, 3, 4, 5]").expect("parse")
				let list = decode::run(data, decode::list(decode::int)).expect("decode")
				list.at(4)
			`,
			want: 5,
		},
		{
			name: "map of string keys to integers",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"age\": 30, \"score\": 95\}").expect("parse")
				let decode_map = decode::map(decode::string, decode::int)
				let m = decode_map(data).expect("decode")
				m.get("age").or(0)
			`,
			want: 30,
		},
		{
			name: "decode field string",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"name\": \"John Doe\"\}").expect("parse")
				decode::run(data, decode::field("name", decode::string)).expect("decode")
			`,
			want: "John Doe",
		},
	})
}

func TestVMNextBytecodeParityDecodePathOneOfAndFlatten(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "path with only string segments",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"foo\": \{\"bar\": 42\}\}").expect("parse")
				let result = decode::run(data, decode::path(["foo", "bar"], decode::int))
				result.expect("decode")
			`,
			want: 42,
		},
		{
			name: "path with array index",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"items\": [10, 20, 30]\}").expect("parse")
				let result = decode::run(data, decode::path(["items", 1], decode::int))
				result.expect("decode")
			`,
			want: 20,
		},
		{
			name: "decode nested path with mixed segments",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"response\": [\{\"name\": \"Alice\"\}, \{\"name\": \"Bob\"\}]\}").expect("parse")
				decode::run(data, decode::path(["response", 0, "name"], decode::string)).expect("decode")
			`,
			want: "Alice",
		},
		{
			name: "path error includes array index",
			input: `
				use ard/decode
				let data = decode::from_json("\{\"items\": [\"a\", \"b\"]\}").expect("parse")
				let result = decode::run(data, decode::path(["items", 1], decode::int))
				match result {
					ok => "unexpected success",
					err(errs) => errs.at(0).to_str(),
				}
			`,
			want: "items[1]: got \"b\", expected Int",
		},
		{
			name: "one_of falls back to alternate decoder",
			input: `
				use ard/decode
				fn int_to_string(data: Dynamic) Str![decode::Error] {
					let int = try decode::int(data)
					Result::ok(int.to_str())
				}
				let data = decode::from_json("20").expect("parse")
				let take_string = decode::one_of(decode::string, [int_to_string])
				take_string(data).expect("decode")
			`,
			want: "20",
		},
		{
			name: "flatten multiple errors with newlines",
			input: `
				use ard/decode
				let errors = [
					decode::Error{expected: "Int", found: "false", path: ["[1]"]},
					decode::Error{expected: "Str", found: "42", path: ["[2]"]},
				]
				decode::flatten(errors)
			`,
			want: "[1]: got false, expected Int\n[2]: got 42, expected Str",
		},
	})
}

func TestVMNextBytecodeParityEnvGet(t *testing.T) {
	t.Setenv("ARD_VM_NEXT_ENV_TEST", "present")

	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "env::get returns Some string for set variables",
			input: `
				use ard/env
				env::get("ARD_VM_NEXT_ENV_TEST").or("")
			`,
			want: "present",
		},
		{
			name: "env::get returns None for non-existent variable",
			input: `
				use ard/env
				env::get("ARD_VM_NEXT_MISSING_ENV_TEST").is_none()
			`,
			want: true,
		},
	})
}

func TestVMNextBytecodeParityPrinting(t *testing.T) {
	got := captureVMNextStdout(t, `
		use ard/io
		io::print("Hello, World!")
	`)

	if want := "Hello, World!"; strings.TrimSpace(got) != want {
		t.Fatalf("Expected %q, got %q", want, got)
	}
}

func TestVMNextBytecodeParityEscapeSequences(t *testing.T) {
	got := captureVMNextStdout(t, `
		use ard/io
		io::print("Line 1\nLine 2")
		io::print("Tab\tTest")
		io::print("Quote \"Test\"")
	`)

	expectedOutputs := []string{
		"Line 1",
		"Line 2",
		"Tab\tTest",
		"Quote \"Test\"",
	}
	for _, want := range expectedOutputs {
		if !strings.Contains(got, want) {
			t.Fatalf("Expected output to contain %q, got %q", want, got)
		}
	}
}

func TestVMNextBytecodeParityFS(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "fake.file")
	dirPath := filepath.Join(tmpDir, "a", "b", "c")

	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "fs::exists false for missing path",
			input: fmt.Sprintf(`
				use ard/fs
				fs::exists(%q)
			`, filepath.Join(tmpDir, "missing.file")),
			want: false,
		},
		{
			name: "fs::exists true for existing path",
			input: fmt.Sprintf(`
				use ard/fs
				fs::exists(%q)
			`, tmpDir),
			want: true,
		},
		{
			name: "fs::create_dir",
			input: fmt.Sprintf(`
				use ard/fs
				fs::create_dir(%q)
			`, dirPath),
			want: nil,
		},
		{
			name: "fs::create_dir created nested dirs",
			input: fmt.Sprintf(`
				use ard/fs
				fs::is_dir(%q)
			`, dirPath),
			want: true,
		},
		{
			name: "fs::create_file",
			input: fmt.Sprintf(`
				use ard/fs
				fs::create_file(%q).expect("Failed to create file")
			`, filePath),
			want: true,
		},
		{
			name: "fs::is_file true for created file",
			input: fmt.Sprintf(`
				use ard/fs
				fs::is_file(%q)
			`, filePath),
			want: true,
		},
		{
			name: "fs::write",
			input: fmt.Sprintf(`
				use ard/fs
				fs::write(%q, "content")
			`, filePath),
			want: nil,
		},
		{
			name: "fs::append",
			input: fmt.Sprintf(`
				use ard/fs
				fs::append(%q, "-appended")
			`, filePath),
			want: nil,
		},
		{
			name: "fs::read",
			input: fmt.Sprintf(`
				use ard/fs
				match fs::read(%q) {
					ok(s) => s,
					err => err,
				}
			`, filePath),
			want: "content-appended",
		},
		{
			name: "fs::delete",
			input: fmt.Sprintf(`
				use ard/fs
				fs::delete(%q)
			`, filePath),
			want: nil,
		},
		{
			name: "fs::delete removed file",
			input: fmt.Sprintf(`
				use ard/fs
				fs::exists(%q)
			`, filePath),
			want: false,
		},
	})
}

func TestVMNextBytecodeParityFSCopyRenameCwdAndAbs(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	copyPath := filepath.Join(tmpDir, "copy.txt")
	renamedPath := filepath.Join(tmpDir, "renamed.txt")

	if err := os.WriteFile(srcPath, []byte("copy me"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "fs::copy",
			input: fmt.Sprintf(`
				use ard/fs
				fs::copy(%q, %q)
			`, srcPath, copyPath),
			want: nil,
		},
		{
			name: "fs::copy preserves content",
			input: fmt.Sprintf(`
				use ard/fs
				fs::read(%q).expect("read failed")
			`, copyPath),
			want: "copy me",
		},
		{
			name: "fs::copy source still exists",
			input: fmt.Sprintf(`
				use ard/fs
				fs::exists(%q)
			`, srcPath),
			want: true,
		},
		{
			name: "fs::rename",
			input: fmt.Sprintf(`
				use ard/fs
				fs::rename(%q, %q)
			`, copyPath, renamedPath),
			want: nil,
		},
		{
			name: "fs::rename moved content",
			input: fmt.Sprintf(`
				use ard/fs
				fs::read(%q).expect("read failed")
			`, renamedPath),
			want: "copy me",
		},
		{
			name: "fs::rename removed source",
			input: fmt.Sprintf(`
				use ard/fs
				fs::exists(%q)
			`, copyPath),
			want: false,
		},
		{
			name: "fs::cwd",
			input: `
				use ard/fs
				fs::cwd().expect("cwd failed")
			`,
			want: cwd,
		},
		{
			name: "fs::abs resolves relative path",
			input: `
				use ard/fs
				fs::abs(".").expect("abs failed")
			`,
			want: cwd,
		},
		{
			name: "fs::abs resolves absolute path",
			input: fmt.Sprintf(`
				use ard/fs
				fs::abs(%q).expect("abs failed")
			`, tmpDir),
			want: tmpDir,
		},
	})
}

func TestVMNextBytecodeParityFSDeleteDirAndListDir(t *testing.T) {
	tmpDir := t.TempDir()
	dirPath := filepath.Join(tmpDir, "removeme", "nested")
	filePath := filepath.Join(dirPath, "file.txt")
	listDir := filepath.Join(tmpDir, "entries")
	listFile := filepath.Join(listDir, "a.txt")
	listNested := filepath.Join(listDir, "nested")

	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "setup: create nested dir",
			input: fmt.Sprintf(`
				use ard/fs
				fs::create_dir(%q)
			`, dirPath),
			want: nil,
		},
		{
			name: "setup: create file in dir",
			input: fmt.Sprintf(`
				use ard/fs
				fs::write(%q, "data")
			`, filePath),
			want: nil,
		},
		{
			name: "fs::delete_dir",
			input: fmt.Sprintf(`
				use ard/fs
				fs::delete_dir(%q)
			`, filepath.Join(tmpDir, "removeme")),
			want: nil,
		},
		{
			name: "fs::delete_dir removed everything",
			input: fmt.Sprintf(`
				use ard/fs
				fs::exists(%q)
			`, filepath.Join(tmpDir, "removeme")),
			want: false,
		},
		{
			name: "setup: create list_dir entries",
			input: fmt.Sprintf(`
				use ard/fs
				fs::create_dir(%q).expect("create list dir failed")
				fs::write(%q, "data").expect("write list file failed")
				fs::create_dir(%q).expect("create nested list dir failed")
			`, listDir, listFile, listNested),
			want: nil,
		},
		{
			name: "fs::list_dir returns files and directories",
			input: fmt.Sprintf(`
				use ard/fs
				let entries = fs::list_dir(%q).expect("list_dir failed")
				mut saw_file = false
				mut saw_dir = false
				for entry in entries {
					if entry.name == "a.txt" and entry.is_file {
						saw_file = true
					}
					if entry.name == "nested" and not entry.is_file {
						saw_dir = true
					}
				}
				saw_file and saw_dir
			`, listDir),
			want: true,
		},
	})
}

func TestVMNextBytecodeParityCryptoUUID(t *testing.T) {
	out := runSourceGoValue(t, `
		use ard/crypto
		crypto::uuid()
	`)

	uuid, ok := out.(string)
	if !ok {
		t.Fatalf("Expected UUID string, got %T", out)
	}

	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(uuid) {
		t.Fatalf("Expected valid UUID v4 format, got %q", uuid)
	}
}

func TestVMNextBytecodeParityFFIPanicRecovery(t *testing.T) {
	got := runSourceWithExterns(t, `
		extern fn panic_test_ffi(message: Str) Str!Str = "PanicTestFFI"

		match panic_test_ffi("test message") {
			err(message) => message,
			ok(_) => "unexpected success",
		}
	`, HostFunctionRegistry{
		"PanicTestFFI": func(message string) (string, error) {
			panic("test panic: " + message)
		},
	})

	if got.Kind != ValueStr {
		t.Fatalf("got %#v, want panic message string", got)
	}
	if !strings.Contains(got.Str, "panic in FFI function 'PanicTestFFI'") ||
		!strings.Contains(got.Str, "test panic: test message") {
		t.Fatalf("panic message = %q", got.Str)
	}
}

func TestVMNextBytecodeParityHttpMethod(t *testing.T) {
	runBytecodeParityCases(t, []bytecodeParityCase{
		{
			name: "Method implements ToString",
			input: `
				use ard/http
				let method = http::Method::Post
				"{method}"
			`,
			want: "POST",
		},
	})
}

func TestVMNextBytecodeParityHttpServerCallbacks(t *testing.T) {
	got := runSourceWithExterns(t, `
		use ard/decode
		use ard/http

		let routes: [Str:http::HandlerFn] = [
			"/users/:id": fn(req: http::Request, mut res: http::Response) {
				let raw_body = try req.body -> _ {
					res.status = 400
					res.body = "missing body"
				}
				let body_text = try decode::run(raw_body, decode::string) -> errs {
					res.status = 400
					res.body = decode::flatten(errs)
				}
				let payload = try decode::from_json(body_text) -> err {
					res.status = 400
					res.body = err
				}
				let email = try decode::run(payload, decode::field("email", decode::string)) -> errs {
					res.status = 400
					res.body = decode::flatten(errs)
				}

				res.status = 201
				res.headers = ["X-User": req.path_param("id")]
				res.body = "{req.path().or("")}|{req.query_param("debug")}|{email}"
			},
		]

		http::serve(9999, routes).expect("serve failed")
	`, HostFunctionRegistry{
		"HTTP_Serve": func(port int, handlers map[string]stdlibffi.Callback2[stdlibffi.Request, *stdlibffi.Response, struct{}]) error {
			if port != 9999 {
				return fmt.Errorf("port = %d, want 9999", port)
			}
			handler, ok := handlers["/users/:id"]
			if !ok {
				return fmt.Errorf("missing /users/:id handler")
			}
			req := httptest.NewRequest(http.MethodPost, "http://example.test/users/42?debug=true", nil)
			req.SetPathValue("id", "42")
			res := &stdlibffi.Response{Status: 200, Headers: map[string]string{}}
			_, err := handler.Call(stdlibffi.Request{
				Method:  stdlibffi.Method(1),
				Url:     req.URL.String(),
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    stdlibffi.Some[any](`{"email":"ada@example.com"}`),
				Raw:     stdlibffi.Some(stdlibffi.RawRequest{Handle: req}),
			}, res)
			if err != nil {
				return err
			}
			if res.Status != 201 {
				return fmt.Errorf("status = %d, want 201", res.Status)
			}
			if res.Headers["X-User"] != "42" {
				return fmt.Errorf("X-User header = %q, want 42", res.Headers["X-User"])
			}
			if res.Body != "/users/42|true|ada@example.com" {
				return fmt.Errorf("body = %q", res.Body)
			}
			return nil
		},
	})

	if got.Kind != ValueVoid {
		t.Fatalf("got %#v, want void", got)
	}
}

func TestVMNextBytecodeParityHttpSendUsesRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	got := runSourceGoValue(t, fmt.Sprintf(`
		use ard/http
		use ard/maybe

		http::send(http::Request{
			method: http::Method::Get,
			url: %q,
			headers: [:],
			timeout: maybe::some(1),
		}).or(http::Response::new(-1, "")).status
	`, server.URL))

	if got != -1 {
		t.Fatalf("Expected request timeout fallback status -1, got %v", got)
	}
}

func TestVMNextBytecodeParityHttpSendCallSiteTimeoutOverridesRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1100 * time.Millisecond)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer server.Close()

	got := runSourceGoValue(t, fmt.Sprintf(`
		use ard/http
		use ard/maybe

		let req = http::Request{
			method: http::Method::Get,
			url: %q,
			headers: [:],
			timeout: maybe::some(1),
		}

		http::send(req, 2).or(http::Response::new(-1, "")).status
	`, server.URL))

	if got != http.StatusCreated {
		t.Fatalf("Expected override timeout to succeed with %d, got %v", http.StatusCreated, got)
	}
}

func TestVMNextBytecodeParityAsyncTiming(t *testing.T) {
	t.Run("async::sleep waits at least requested duration", func(t *testing.T) {
		start := time.Now()
		runSource(t, `
			use ard/async
			async::sleep(1000000)
		`)
		if elapsed := time.Since(start); elapsed < time.Millisecond {
			t.Fatalf("Expected script to take >= 1ms, took %v", elapsed)
		}
	})

	t.Run("joining fibers waits for concurrent work", func(t *testing.T) {
		start := time.Now()
		runSource(t, `
			use ard/async
			let fiber1 = async::start(fn() { async::sleep(2000000) })
			let fiber2 = async::start(fn() { async::sleep(1000000) })
			let fiber3 = async::start(fn() { async::sleep(1000000) })
			fiber1.join()
			fiber2.join()
			fiber3.join()
		`)
		if elapsed := time.Since(start); elapsed < 2*time.Millisecond {
			t.Fatalf("Expected concurrent execution >= 2ms, got %v", elapsed)
		}
	})

	t.Run("async::join waits for the longest fiber", func(t *testing.T) {
		start := time.Now()
		runSource(t, `
			use ard/async
			use ard/duration
			async::join([
				async::start(fn() { async::sleep(duration::from_millis(20)) }),
				async::start(fn() { async::sleep(duration::from_millis(20)) }),
				async::start(fn() { async::sleep(duration::from_millis(40)) }),
			])
		`)
		elapsed := time.Since(start)
		if elapsed < 40*time.Millisecond {
			t.Fatalf("Expected concurrent execution >= 40ms, got %v", elapsed)
		}
		if elapsed > 200*time.Millisecond {
			t.Fatalf("Expected concurrent execution <= 200ms, got %v", elapsed)
		}
	})
}

func captureVMNextStdout(t *testing.T, input string) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	runSource(t, input)

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
