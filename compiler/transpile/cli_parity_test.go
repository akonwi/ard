package transpile

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type cliRunResult struct {
	stdout   string
	stderr   string
	err      error
	exitCode int
}

type cliSnippetCase struct {
	name  string
	env   map[string]string
	args  []string
	stdin string
	files map[string]string
}

func TestCLIRunMatchesVMSnippetParity(t *testing.T) {
	ardPath := ensureArdBinary(t)
	servePort := reserveLocalPort(t)
	serveBaseURL := fmt.Sprintf("http://127.0.0.1:%d", servePort)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		w.Header().Set("X-Echo-Method", r.Method)
		w.Header().Set("X-Echo-Query", r.URL.Query().Get("lang"))
		w.Header().Set("X-Echo-Header", r.Header.Get("X-Demo"))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(bodyBytes)
	}))
	defer httpServer.Close()

	cases := []cliSnippetCase{
		{
			name: "entrypoint_main",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  io::print("entrypoint ok")
}
`,
			},
		},
		{
			name: "else_if_preserves_fallback",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  for num in 1..6 {
    if num % 3 == 0 {
      io::print("Fizz")
    } else if num % 5 == 0 {
      io::print("Buzz")
    } else {
      io::print(num)
    }
  }
}
`,
			},
		},
		{
			name: "float_to_str_matches_vm",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  let celsius = (Float::from_int(0) - 32.0) * 5.0 / 9.0
  io::print(celsius.to_str())
}
`,
			},
		},
		{
			name: "user_module_import",
			files: map[string]string{
				"main.ard": `
use ard/io
use demo/maths

fn main() {
  io::print(maths::add(1, 2).to_str())
}
`,
				"maths.ard": `
fn add(a: Int, b: Int) Int {
  a + b
}
`,
			},
		},
		{
			name: "maybe_match",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/maybe

fn main() {
  let name: Str? = maybe::some("kit")
  match name {
    value => io::print(value),
    _ => io::print("none")
  }
}
`,
			},
		},
		{
			name: "result_match",
			files: map[string]string{
				"main.ard": `
use ard/io

fn divide(num: Int) Int!Str {
  match num == 0 {
    true => Result::err("zero"),
    false => Result::ok(10 / num),
  }
}

fn main() {
  match divide(2) {
    ok(value) => io::print(value),
    err(message) => io::print(message)
  }
}
`,
			},
		},
		{
			name: "try_fallback",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/maybe

fn half(n: Int) Int? {
  match n > 0 {
    true => maybe::some(n / 2),
    false => maybe::none(),
  }
}

fn maybe_fallback(n: Int) Int {
  let value = try half(n) -> _ {
    0
  }
  value + 1
}

fn main() {
  io::print(maybe_fallback(0))
  io::print(maybe_fallback(8))
}
`,
			},
		},
		{
			name: "map_operations",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  mut values: [Str: Int] = ["a": 1]
  values.set("b", 2)
  io::print(values.has("a").to_str())
  io::print(values.get("b").or(0))
  values.drop("a")
  io::print(values.has("a").to_str())
}
`,
			},
		},
		{
			name: "map_iteration_order",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  mut values: [Int: Str] = [3: "c", 1: "a"]
  values.set(2, "b")

  for key, value in values {
    io::print("{key}:{value}")
  }

  for key in values.keys() {
    io::print(key)
  }
}
`,
			},
		},
		{
			name: "struct_methods",
			files: map[string]string{
				"main.ard": `
use ard/io

struct Box {
  value: Int,
}

impl Box {
  fn get() Int {
    self.value
  }

  fn mut set(value: Int) {
    self.value = value
  }
}

fn main() {
  mut box = Box{value: 1}
  box.set(2)
  io::print(box.get())
}
`,
			},
		},
		{
			name: "mutable_function_params",
			files: map[string]string{
				"main.ard": `
use ard/io

struct Box {
  value: Int,
}

fn set_box(mut box: Box) {
  box.value = 2
}

fn bump(mut value: Int) {
  value = value + 1
}

fn append_one(mut values: [Int]) {
  values.push(1)
}

fn main() {
  mut box = Box{value: 1}
  set_box(box)
  io::print(box.value)

  mut value = 1
  bump(value)
  io::print(value)

  mut values = [1]
  append_one(values)
  io::print(values.size())
  io::print(values.at(1))
}
`,
			},
		},
		{
			name: "trait_dispatch",
			files: map[string]string{
				"main.ard": `
use ard/io

struct Book {
  title: Str,
  author: Str,
}

impl Str::ToString for Book {
  fn to_str() Str {
    "Book: " + self.title + " by " + self.author
  }
}

fn show(item: Str::ToString) {
  io::print(item)
}

fn main() {
  let book = Book{title: "The Hobbit", author: "J.R.R. Tolkien"}
  show(book)
}
`,
			},
		},
		{
			name: "env_lookup",
			env: map[string]string{
				"ARD_PARITY_VALUE": "parity-ok",
			},
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/env

fn main() {
  match env::get("ARD_PARITY_VALUE") {
    value => io::print(value),
    _ => io::print("missing")
  }
}
`,
			},
		},
		{
			name: "argv_load",
			args: []string{"alpha", "beta"},
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/argv

fn main() {
  let args = argv::load()
  io::print(args.program)
  io::print(args.arguments.size())
  io::print(args.arguments.at(0))
  io::print(args.arguments.at(1))
}
`,
			},
		},
		{
			name:  "stdin_read_line",
			stdin: "kit\n",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  match io::read_line() {
    ok(line) => io::print(line.trim()),
    err(message) => io::print(message),
  }
}
`,
			},
		},
		{
			name: "int_from_str",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  io::print(Int::from_str("42").or(-1))
  io::print(Int::from_str("oops").or(-1))
}
`,
			},
		},
		{
			name: "float_helpers",
			files: map[string]string{
				"main.ard": `
use ard/io

fn main() {
  let parsed = Float::from_str("3.75").or(0.0)
  io::print(Float::floor(parsed))
  io::print(Float::from_str("oops").or(1.25))
}
`,
			},
		},
		{
			name: "hex_codec",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/hex

fn main() {
  let encoded = hex::encode("abc")
  io::print(encoded)
  match hex::decode(encoded) {
    ok(decoded) => io::print(decoded),
    err(message) => io::print(message),
  }
  io::print(hex::decode("zz").is_err().to_str())
}
`,
			},
		},
		{
			name: "base64_codec",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/base64

fn main() {
  let encoded = base64::encode("hello")
  io::print(encoded)
  match base64::decode(encoded) {
    ok(decoded) => io::print(decoded),
    err(message) => io::print(message),
  }
  let encoded_url = base64::encode_url("hello world", true)
  io::print(encoded_url)
  match base64::decode_url(encoded_url, true) {
    ok(decoded) => io::print(decoded),
    err(message) => io::print(message),
  }
  io::print(base64::decode("not!valid!base64").is_err().to_str())
}
`,
			},
		},
		{
			name: "json_encode",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/encode

fn main() {
  io::print(encode::json("hello").or("err"))
  io::print(encode::json(42).or("err"))
  io::print(encode::json(true).or("err"))
}
`,
			},
		},
		{
			name: "json_decode_dynamic_roundtrip",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/decode

fn main() {
  io::print(decode::from_json("[1,2,3]").is_ok().to_str())
  io::print(decode::from_json("not json").is_err().to_str())
}
`,
			},
		},
		{
			name: "decode_primitives",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/decode

fn main() {
  match decode::from_json("\"hello\"") {
    ok(raw) => io::print(decode::string(raw).or("bad")),
    err(message) => io::print(message),
  }

  match decode::from_json("1") {
    ok(raw) => io::print(decode::string(raw).is_err().to_str()),
    err(message) => io::print(message),
  }

  match decode::from_json("42") {
    ok(raw) => io::print(decode::int(raw).or(-1)),
    err(message) => io::print(message),
  }
}
`,
			},
		},
		{
			name: "decode_collections_and_field",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/decode

fn main() {
  match decode::from_json("\{\"name\":\"kit\",\"nums\":[1,2,3],\"counts\":\{\"a\":1}\}") {
    ok(raw) => {
      let name_decoder = decode::field("name", decode::string)
      let nums_decoder = decode::field("nums", decode::list(decode::int))
      let counts_decoder = decode::field("counts", decode::map(decode::string, decode::int))
      io::print(name_decoder(raw).or("bad"))
      io::print(nums_decoder(raw).is_ok().to_str())
      io::print(counts_decoder(raw).is_ok().to_str())
    },
    err(message) => io::print(message),
  }
}
`,
			},
		},
		{
			name: "fs_roundtrip",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/fs

fn main() {
  io::print(fs::exists("note.txt").to_str())
  io::print(fs::create_file("note.txt").is_ok().to_str())
  io::print(fs::is_file("note.txt").to_str())
  io::print(fs::write("note.txt", "hello").is_ok().to_str())
  io::print(fs::append("note.txt", " world").is_ok().to_str())
  io::print(fs::read("note.txt").or("bad"))
  io::print(fs::delete("note.txt").is_ok().to_str())
  io::print(fs::exists("note.txt").to_str())
}
`,
			},
		},
		{
			name: "crypto_hashes",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/crypto
use ard/hex

fn main() {
  io::print(crypto::md5("hello"))
  io::print(hex::encode(crypto::sha256("")))
  io::print(hex::encode(crypto::sha512("hello")))
}
`,
			},
		},
		{
			name: "crypto_passwords",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/crypto

fn main() {
  let hashed = crypto::hash("password123", 4).or("")
  io::print(crypto::verify("password123", hashed).or(false).to_str())
  io::print(crypto::verify("wrong-password", hashed).or(true).to_str())

  let scrypt_hashed = crypto::scrypt_hash("password", "73616c74", 16, 1, 1, 16).or("")
  io::print(scrypt_hashed)
  io::print(crypto::scrypt_verify("password", scrypt_hashed, 16, 1, 1, 16).or(false).to_str())
}
`,
			},
		},
		{
			name: "sql_sqlite_roundtrip",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/sql
use ard/decode

fn main() {
  match sql::open("test.db") {
    ok(db) => {
      let no_args: [Str: sql::Value] = [:]
      io::print(db.exec("CREATE TABLE items(name TEXT, age INTEGER)").is_ok().to_str())
      let insert = db.query("INSERT INTO items(name, age) VALUES (@name, @age)")
      io::print(insert.run(["name":"kit", "age": 3]).is_ok().to_str())
      let select = db.query("SELECT name, age FROM items")
      match select.all(no_args) {
        ok(rows) => {
          let name_decoder = decode::field("name", decode::string)
          let age_decoder = decode::field("age", decode::int)
          io::print(rows.size())
          io::print(name_decoder(rows.at(0)).or("bad"))
          io::print(age_decoder(rows.at(0)).or(-1))
        },
        err(message) => io::print(message),
      }
      io::print(db.close().is_ok().to_str())
    },
    err(message) => io::print(message),
  }
}
`,
			},
		},
		{
			name: "async_eval",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/async

fn main() {
  let fiber = async::eval(fn() Int {
    41 + 1
  })
  io::print(fiber.get())
}
`,
			},
		},
		{
			name: "async_start_join",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/async

fn main() {
  let fiber = async::start(fn() {
    io::print("hi")
  })
  fiber.join()
  io::print("done")
}
`,
			},
		},
		{
			name: "async_sleep",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/async

fn main() {
  io::print("before")
  async::sleep(0)
  io::print("after")
}
`,
			},
		},
		{
			name: "async_join_list",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/async

fn main() {
  let a = async::eval(fn() Int {
    1
  })
  let b = async::eval(fn() Int {
    2
  })
  async::join([a, b])
  io::print(a.get() + b.get())
}
`,
			},
		},
		{
			name: "dynamic_builders",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/dynamic
use ard/decode

fn main() {
  let numbers = dynamic::list([1, 2, 3], dynamic::from_int)
  let value = dynamic::object([
    "name": dynamic::from_str("kit"),
    "ok": dynamic::from_bool(true),
    "nums": numbers,
  ])

  match decode::extract_field(value, "name") {
    ok(name) => io::print(decode::string(name).or("bad")),
    err(message) => io::print(message),
  }

  match decode::extract_field(value, "nums") {
    ok(items) => match decode::to_list(items) {
      ok(list) => io::print(list.size()),
      err(message) => io::print(message),
    },
    err(message) => io::print(message),
  }

  io::print(decode::is_void(dynamic::from_void()).to_str())
}
`,
			},
		},
		{
			name: "sql_transactions",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/sql
use ard/decode

fn main() {
  match sql::open("test.db") {
    ok(db) => {
      let no_args: [Str: sql::Value] = [:]
      let name_decoder = decode::field("name", decode::string)

      io::print(db.exec("CREATE TABLE items(name TEXT)").is_ok().to_str())

      match db.begin() {
        ok(tx) => {
          let insert = tx.query("INSERT INTO items(name) VALUES (@name)")
          io::print(insert.run(["name": "rolled-back"]).is_ok().to_str())
          io::print(tx.rollback().is_ok().to_str())
        },
        err(message) => io::print(message),
      }

      match db.query("SELECT name FROM items").all(no_args) {
        ok(rows) => io::print(rows.size()),
        err(message) => io::print(message),
      }

      match db.begin() {
        ok(tx) => {
          let insert = tx.query("INSERT INTO items(name) VALUES (@name)")
          io::print(insert.run(["name": "committed"]).is_ok().to_str())
          io::print(tx.commit().is_ok().to_str())
        },
        err(message) => io::print(message),
      }

      match db.query("SELECT name FROM items").all(no_args) {
        ok(rows) => {
          io::print(rows.size())
          io::print(name_decoder(rows.at(0)).or("bad"))
        },
        err(message) => io::print(message),
      }

      io::print(db.close().is_ok().to_str())
    },
    err(message) => io::print(message),
  }
}
`,
			},
		},
		{
			name: "time_and_uuid_properties",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/chrono
use ard/dates
use ard/crypto

fn main() {
  let now = chrono::now()
  let today = dates::get_today()
  let id = crypto::uuid()

  io::print((now > 0).to_str())
  io::print(today.size())
  io::print(today.contains("-").to_str())
  io::print(today.split("-").size())
  io::print(id.size())
  io::print(id.contains("-").to_str())
  io::print(id.split("-").size())
}
`,
			},
		},
		{
			name: "http_client_roundtrip",
			env: map[string]string{
				"ARD_HTTP_BASE_URL": httpServer.URL,
			},
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/http
use ard/env
use ard/maybe
use ard/dynamic

fn main() {
  let base = env::get("ARD_HTTP_BASE_URL").or("")
  let req = http::Request{
    method: http::Method::Post,
    url: base + "/echo?lang=ard",
    headers: ["content-type": "text/plain", "x-demo": "kit"],
    body: maybe::some(dynamic::from_str("hello")),
  }

  match http::send(req, 5) {
    ok(res) => {
      io::print(res.status)
      io::print(res.is_ok().to_str())
      io::print(res.headers.get("X-Echo-Method").or("missing"))
      io::print(res.headers.get("X-Echo-Query").or("missing"))
      io::print(res.headers.get("X-Echo-Header").or("missing"))
      io::print(res.body)
    },
    err(message) => io::print(message),
  }
}
`,
			},
		},
		{
			name: "http_server_roundtrip",
			env: map[string]string{
				"ARD_HTTP_SERVE_PORT":     fmt.Sprintf("%d", servePort),
				"ARD_HTTP_SERVE_BASE_URL": serveBaseURL,
			},
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/http
use ard/env
use ard/async

fn main() {
  let port = Int::from_str(env::get("ARD_HTTP_SERVE_PORT").or("0")).or(0)
  let base = env::get("ARD_HTTP_SERVE_BASE_URL").or("")

  async::start(fn() {
    let routes: [Str: http::HandlerFn] = [
      "/": fn(req: http::Request, mut res: http::Response) Void {
        res.status = 201
        res.headers.set("x-path", req.path().or(""))
        res.headers.set("x-query", req.query_param("lang"))
        res.body = "hello"
      }
    ]
    http::serve(port, routes).expect("serve failed")
  })

  mut ok = false
  mut status = 0
  mut path = ""
  mut query = ""
  mut body = ""
  mut attempts = 0

  while ok == false and attempts < 200 {
    let req = http::Request{
      method: http::Method::Get,
      url: base + "/?lang=ard",
      headers: [:],
    }
    match http::send(req, 1) {
      ok(res) => {
        ok = true
        status = res.status
        path = res.headers.get("X-Path").or("missing")
        query = res.headers.get("X-Query").or("missing")
        body = res.body
      },
      err(_) => {
        attempts = attempts + 1
      },
    }
  }

  io::print(ok.to_str())
  io::print(status)
  io::print(path)
  io::print(query)
  io::print(body)
}
`,
			},
		},
		{
			name: "enum_match",
			files: map[string]string{
				"main.ard": `
use ard/io

enum Color {
  Red,
  Yellow,
}

fn label(color: Color) Str {
  match color {
    Color::Red => "stop",
    Color::Yellow => "wait"
  }
}

fn main() {
  io::print(label(Color::Yellow))
}
`,
			},
		},
		{
			name: "union_match",
			files: map[string]string{
				"main.ard": `
use ard/io

struct Square {
  size: Int,
}

struct Circle {
  radius: Int,
}

type Shape = Square | Circle

fn label(shape: Shape) Str {
  match shape {
    Square => "square {it.size}",
    Circle => "circle {it.radius}"
  }
}

fn main() {
  let shapes: [Shape] = [Square { size: 2 }, Circle { radius: 3 }]
  for shape in shapes {
    io::print(label(shape))
  }
}
`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot := writeSnippetProject(t, tc.files)

			vmArgs := append([]string{"run", "main.ard"}, tc.args...)
			vmResult := runArdCLI(t, ardPath, projectRoot, tc.env, tc.stdin, vmArgs...)
			if vmResult.err != nil {
				t.Fatalf("vm snippet run failed: %s", formatCLIRunFailure(vmResult))
			}

			goArgs := append([]string{"run", "--target", "go", "main.ard"}, tc.args...)
			goResult := runArdCLI(t, ardPath, projectRoot, tc.env, tc.stdin, goArgs...)
			if goResult.err != nil {
				t.Fatalf("go snippet run failed: %s", formatCLIRunFailure(goResult))
			}

			if vmResult.exitCode != goResult.exitCode || vmResult.stdout != goResult.stdout || vmResult.stderr != goResult.stderr {
				t.Fatalf("snippet parity mismatch\nvm: %s\ngo: %s", formatCLIRunFailure(vmResult), formatCLIRunFailure(goResult))
			}
		})
	}
}

func reserveLocalPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve local port: %v", err)
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected TCP listener address, got %T", listener.Addr())
	}
	return addr.Port
}

func ensureArdBinary(t *testing.T) string {
	t.Helper()
	compilerRoot, err := compilerModuleRoot()
	if err != nil {
		t.Fatalf("failed to determine compiler root: %v", err)
	}
	ardPath := filepath.Join(t.TempDir(), "ard")
	cmd := exec.Command("go", "build", "-o", ardPath, ".")
	cmd.Dir = compilerRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build ard CLI: %v\n%s", err, string(output))
	}
	return ardPath
}

func writeSnippetProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\ntarget = \"bytecode\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	for path, content := range files {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("failed to create directory for %s: %v", path, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}
	return root
}

func runArdCLI(t *testing.T, ardPath, dir string, env map[string]string, stdin string, args ...string) cliRunResult {
	t.Helper()
	cmd := exec.Command(ardPath, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := cliRunResult{
		stdout: normalizeOutput(stdout.String()),
		stderr: normalizeOutput(stderr.String()),
		err:    err,
	}
	if err == nil {
		return result
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.exitCode = exitErr.ExitCode()
		return result
	}
	result.exitCode = -1
	return result
}

func formatCLIRunFailure(result cliRunResult) string {
	return fmt.Sprintf("exit=%d err=%v\nstdout:\n%s\nstderr:\n%s", result.exitCode, result.err, result.stdout, result.stderr)
}
