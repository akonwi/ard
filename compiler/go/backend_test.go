package gotarget

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/frontend"
	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/version"
)

func TestGenerateSourcesFormatsSimpleProgram(t *testing.T) {
	program := lowerSource(t, `
		fn add(a: Int, b: Int) Int {
			a + b
		}

		fn main() Int {
			add(1, 2)
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source, ok := sources["test.go"]
	if !ok {
		t.Fatalf("generated sources missing test.go: %#v", mapsKeys(sources))
	}
	got := string(source)
	if !strings.Contains(got, "package main") {
		t.Fatalf("generated source missing package declaration:\n%s", got)
	}
	if !strings.Contains(got, "func test_ard__add(a int, b int) int") {
		t.Fatalf("generated source missing lowered add function:\n%s", got)
	}
	if !strings.Contains(got, "return a + b") {
		t.Fatalf("generated source missing arithmetic return:\n%s", got)
	}
	if !strings.Contains(got, "func main()") {
		t.Fatalf("generated source missing Go main wrapper:\n%s", got)
	}
}

func TestGenerateSourcesOmitsTestsUnlessIncluded(t *testing.T) {
	result := parse.Parse([]byte(`
		fn main() Int { 1 }
		test fn check() Void!Str { Result::ok(()) }
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.LowerWithTests(c.Module())
	if err != nil {
		t.Fatalf("lower with tests: %v", err)
	}

	productionSources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources production error = %v", err)
	}
	if strings.Contains(string(productionSources["test.go"]), "__check") {
		t.Fatalf("production source includes test function:\n%s", productionSources["test.go"])
	}

	testSources, err := GenerateSources(program, Options{PackageName: "main", IncludeTests: true, SuppressMain: true})
	if err != nil {
		t.Fatalf("GenerateSources tests error = %v", err)
	}
	if !strings.Contains(string(testSources["test.go"]), "__check") {
		t.Fatalf("test source missing test function:\n%s", testSources["test.go"])
	}
}

func TestRunProgramExecutesSimpleMain(t *testing.T) {
	program := lowerSource(t, `
		fn main() Void {
			()
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSpecializesGenericEmptyListLocal(t *testing.T) {
	program := lowerSource(t, `
		fn drop(from: [$T], till: Int) [$T] {
			mut out: [$T] = []
			for item, idx in from {
				if idx >= till {
					out.push(item)
				}
			}
			out
		}

		fn main() Bool {
			let dropped = drop([1, 2, 3], 1)
			dropped.size() == 2 and dropped.at(0) == 2
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramAllowsModuleWithoutEntry(t *testing.T) {
	program := lowerSource(t, `
		fn add(a: Int, b: Int) Int {
			a + b
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestGenerateSourcesSupportsStructsAndEnums(t *testing.T) {
	program := lowerSource(t, `
		enum Direction {
			Up, Down
		}

		struct User {
			name: Str,
			age: Int,
		}

		fn direction() Direction {
			Direction::Down
		}

		fn next_age() Int {
			let user = User{name: "Ada", age: 41}
			user.age + 1
		}

		fn main() Int {
			next_age()
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	combined := ""
	for _, source := range sources {
		combined += string(source)
	}
	if !regexp.MustCompile(`type type_\d+__Direction int`).MatchString(combined) {
		t.Fatalf("generated source missing enum type:\n%s", combined)
	}
	if !regexp.MustCompile(`type_\d+__Direction__Down`).MatchString(combined) {
		t.Fatalf("generated source missing enum constants:\n%s", combined)
	}
	if !regexp.MustCompile(`type type_\d+__User struct`).MatchString(combined) {
		t.Fatalf("generated source missing struct type:\n%s", combined)
	}
	if !regexp.MustCompile(`type_\d+__User\{age: 41, name: "Ada"\}`).MatchString(combined) {
		t.Fatalf("generated source missing struct literal lowering:\n%s", combined)
	}
	if !strings.Contains(combined, ".age + 1") {
		t.Fatalf("generated source missing field access lowering:\n%s", combined)
	}
}

func TestGenerateSourcesSupportsTryMaybeCatchAndEarlyReturn(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn missing() Int? {
			maybe::none()
		}

		fn with_default() Int {
			let value = try missing() -> _ { 42 }
			value
		}

		fn passthrough() Int? {
			let value = try missing()
			maybe::some(value)
		}

		fn main() Int {
			with_default()
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "return _tmp_") {
		t.Fatalf("generated source missing try early return lowering:\n%s", source)
	}
	if !strings.Contains(source, "= 42") {
		t.Fatalf("generated source missing try catch lowering:\n%s", source)
	}
}

func TestGenerateSourcesPropagatesTryResultAcrossDifferentResultValueTypes(t *testing.T) {
	program := lowerSource(t, `
		fn read_text() Str!Str {
			Result::err("bad")
		}

		fn parse() Int!Str {
			let text = try read_text()
			let _ignore = text
			Result::ok(1)
		}

		fn main() Int!Str {
			parse()
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "return runtime.Result[int, string]{Err: _tmp_") {
		t.Fatalf("generated source missing result error propagation conversion:\n%s", source)
	}
}

func TestRunProgramSupportsCommonStdlibExterns(t *testing.T) {
	program := lowerSource(t, `
		use ard/argv
		use ard/base64
		use ard/dynamic
		use ard/env
		use ard/float
		use ard/hex

		fn main() Bool {
			let encoded = base64::encode("hi", true)
			let decoded = base64::decode(encoded, true).expect("decode")
			let hexed = hex::encode(decoded)
			let unhex = hex::decode(hexed).expect("hex")
			let args = argv::os_args()
			let _path = env::get("PATH")
			let parsed = float::from_str("3.5").or(0.0)
			let floored = float::floor(parsed)
			let _dyn_list = dynamic::from_list([dynamic::from_str(unhex)])
			let _dyn_map = dynamic::object(["value": dynamic::from_int(args.size())])
			unhex == "hi" and floored == 3.0 and args.size() >= 0
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestBuildProgramCompilesJSONPreludeForStdlibBackedHTTPTypes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/http
use ard/json

struct App {
  routes: [Str: fn(http::Request, mut http::Response)]
}

fn main() Str {
  json::encode(Dynamic::from("ok")).or("")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(dir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestBuildProgramLowersTransitiveStdlibExternFromSubmodule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"sleep-repro\"\nard = \">= 0.13.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib.ard"), []byte(`use ard/async

fn tick() Void {
  async::sleep(1000000)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use sleep-repro/lib

fn main() Void {
  lib::tick()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := frontend.LoadModule(mainPath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(dir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestBuildProgramLowersOptionMatchArmModuleExternCall(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use ard/decode

fn nested_name(obj: Dynamic, field: Str) Str {
  let nested = decode::run(obj, decode::field(field, decode::nullable(decode::dynamic)))
    .expect("Missing nested field")
  match nested {
    n => decode::run(n, decode::field("name", decode::string)).expect("Missing nested name"),
    _ => "",
  }
}

fn main() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := frontend.LoadModule(mainPath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(dir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestBuildProgramImportsProjectFFIForExternTypesOnlyUsedAsTypes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(dir, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "host.go"), []byte(`package ffi

type Handle struct {
	Name string
}

func MakeHandle(name string) (*Handle, error) {
	return &Handle{Name: name}, nil
}

func HandleName(h *Handle) string {
	return h.Name
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib.ard"), []byte(`extern type Handle = "*Handle"

extern fn make_handle_raw(name: Str) Handle!Str = "MakeHandle"
extern fn handle_name(h: Handle) Str = "HandleName"

struct KeyEvent { name: Str }
struct QuitEvent {}

type Event = KeyEvent | QuitEvent

fn next_event(name: Str) Event!Str {
  let h = try make_handle_raw(name)
  let ev: Event = KeyEvent{name: handle_name(h)}
  Result::ok(ev)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use ard/io
use ard/json
use demo/lib

fn main() {
  match lib::next_event("hello").expect("ev") {
    KeyEvent(k) => {
      let s = json::encode(k).expect("enc")
      io::print(s)
    },
    QuitEvent(_) => io::print("quit"),
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := frontend.LoadModule(mainPath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	builtPath, err := BuildProgram(program, filepath.Join(dir, "app"), loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramSupportsProjectGoFFIWithTypedExternType(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
extern type Buffer = "*bytes.Buffer"

extern fn new_buffer() Buffer!Str = "NewBuffer"
extern fn buffer_len(buffer: Buffer) Int = "BufferLen"

fn main() Bool {
	let buffer = new_buffer().expect("buffer")
	buffer_len(buffer) == 0
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

import "bytes"

func NewBuffer() (*bytes.Buffer, error) {
	return &bytes.Buffer{}, nil
}

func BufferLen(buffer *bytes.Buffer) int {
	return buffer.Len()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	binaryPath := filepath.Join(dir, "app")
	builtPath, err := BuildProgram(program, binaryPath, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramSupportsProjectGoFFIWithNativeChannel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/async/channel

extern fn observe(ch: channel::Chan<Int>) Int = "Observe"

fn main() Bool {
	let ch = channel::new<Int>(size: 1)
	ch.send(7) and observe(ch.chan) == 7
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

func Observe(ch chan int) int {
	return <-ch
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	binaryPath := filepath.Join(dir, "app")
	builtPath, err := BuildProgram(program, binaryPath, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramSupportsProjectGoFFI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
extern fn lookup(flag: Bool) Str? = {
	go = "Lookup"
}

extern fn read_value() Str!Str = {
	go = "ReadValue"
}

extern fn mark() Void!Str = {
	go = "Mark"
}

extern fn select(input: Str?) Str = {
	go = "Select"
}

fn main() Bool {
	let found = lookup(true)
	let name = found.or("missing")
	let value = read_value().expect("read")
	mark().expect("mark")
	name == "yes" and value == "ok" and select(found) == "yes"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffi.go"), []byte(`package ffi

func Lookup(flag bool) *string {
	if !flag {
		return nil
	}
	value := "yes"
	return &value
}

func ReadValue() (string, error) {
	return "ok", nil
}

func Mark() error {
	return nil
}

func Select(input *string) string {
	if input == nil {
		return "missing"
	}
	return *input
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	binaryPath := filepath.Join(dir, "app")
	builtPath, err := BuildProgram(program, binaryPath, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := exec.Command(builtPath).Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestBuildProgramSupportsDependencyGoFFI(t *testing.T) {
	workspace := t.TempDir()
	depDir := filepath.Join(workspace, "dep")
	appDir := filepath.Join(workspace, "app")
	if err := os.MkdirAll(filepath.Join(depDir, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "dep.ard"), []byte(`extern fn answer() Int = "Answer"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "ffi", "host.go"), []byte(`package ffi

func Answer() int { return 42 }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use dep

fn main() Int {
	dep::answer()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := checker.FetchDependency(appDir, "dep"); err != nil {
		t.Fatalf("fetch dependency: %v", err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	binaryPath := filepath.Join(appDir, "app")
	builtPath, err := BuildProgram(program, binaryPath, loaded.ProjectInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	cmd := exec.Command(builtPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("run built binary: %v", err)
	}
}

func TestWriteProgramDoesNotRequireProjectFFIForStdlibExternMethods(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use ard/sql

fn close(db: sql::Database) Void!Str {
	db.close()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	workspace := filepath.Join(dir, "workspace")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeProgram(workspace, program, Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo}); err != nil {
		t.Fatalf("write program: %v", err)
	}
}

func TestGenerateSourcesUsesExpectedLocalTypeForMaybeNone(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn main() Bool {
			let found: Int? = maybe::none()
			found.is_none()
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "runtime.Maybe[int]{}") {
		t.Fatalf("generated source missing typed maybe none:\n%s", source)
	}
	if strings.Contains(source, "runtime.Maybe[struct {") {
		t.Fatalf("generated source used untyped maybe none:\n%s", source)
	}
}

func TestGenerateSourcesUsesExpectedDefaultTypeForResultOr(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn fetch() Int?!Str {
			let empty: Int? = maybe::none()
			Result::ok(empty)
		}

		fn main() Bool {
			let value = fetch().or(maybe::none())
			value.is_none()
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if strings.Contains(source, "runtime.Maybe[struct {") {
		t.Fatalf("generated source used untyped maybe default:\n%s", source)
	}
}

func TestGenerateSourcesSkipsVoidAssignmentForStatementMatchBranches(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn main() Bool {
			match maybe::some(1) {
				value => value == 1,
				_ => (),
			}
			false
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if strings.Contains(source, "= nil") {
		t.Fatalf("generated source assigned nil in statement match lowering:\n%s", source)
	}
}

func TestRunProgramSupportsVoidFiberFunctions(t *testing.T) {
	program := lowerSource(t, `
		use ard/async

		fn job() Void {
			()
		}

		fn main() Void {
			async::start(job)
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestTypeNameUsesUniqueFallbackWhenModuleOwnershipIsMissing(t *testing.T) {
	program := &air.Program{}
	left := typeName(program, air.TypeInfo{ID: 1, Name: "Request"})
	right := typeName(program, air.TypeInfo{ID: 2, Name: "Request"})
	if left == right {
		t.Fatalf("fallback type names should be unique, got %q", left)
	}
}

func TestGenerateSourcesSupportsResultExpectAndStringPredicates(t *testing.T) {
	program := lowerSource(t, `
		use ard/io

		fn main() Bool {
			let line = io::read_line().expect("no line")
			line.is_empty()
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	combined := ""
	for _, source := range sources {
		combined += string(source)
	}
	if !strings.Contains(combined, "runtime.Result[string, string]") {
		t.Fatalf("generated source missing runtime.Result usage:\n%s", combined)
	}
	if !strings.Contains(combined, "stdlibffi.ReadLine()") {
		t.Fatalf("generated source missing ReadLine lowering:\n%s", combined)
	}
	if strings.Contains(combined, "ardReadLine") {
		t.Fatalf("generated source should not use legacy ReadLine helper:\n%s", combined)
	}
	if !strings.Contains(combined, "panic(\"no line\"") {
		t.Fatalf("generated source missing Result.expect lowering:\n%s", combined)
	}
	if !strings.Contains(combined, "len(line") {
		t.Fatalf("generated source missing is_empty lowering:\n%s", combined)
	}
}

func TestGenerateSourcesUsesDirectStdlibMaybeCalls(t *testing.T) {
	program := lowerSource(t, `
		use ard/dynamic
		use ard/env
		use ard/float
		use ard/int

		fn main() Bool {
			let _a = env::get("PATH")
			let _b = float::from_str("1.5")
			let _c = int::from_str("2")
			let _d = dynamic::object(["a": dynamic::from_int(1)])
			true
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "stdlibffi.EnvGet(") || !strings.Contains(source, "stdlibffi.FloatFromStr(") || !strings.Contains(source, "stdlibffi.IntFromStr(") {
		t.Fatalf("generated source missing direct stdlib maybe calls:\n%s", source)
	}
	if strings.Contains(source, "ardIntFromStr") {
		t.Fatalf("generated source should not use legacy IntFromStr helper:\n%s", source)
	}
	if strings.Contains(source, "ardMapToDynamic") {
		t.Fatalf("generated source should not use legacy MapToDynamic helper:\n%s", source)
	}
}

func TestGenerateSourcesUsesPointersForMutableStructParams(t *testing.T) {
	program := lowerSource(t, `
		struct Response {
			body: Str,
		}

		fn set_body(mut res: Response) Void {
			res.body = "ok"
		}

		fn main() Void {
			mut res = Response{body: ""}
			set_body(res)
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !regexp.MustCompile(`func test_ard__set_body\(res \*type_\d+__Response\)`).MatchString(source) {
		t.Fatalf("generated source missing pointer mutable param lowering:\n%s", source)
	}
	if !strings.Contains(source, "test_ard__set_body(&res_0)") {
		t.Fatalf("generated source missing pointer call lowering:\n%s", source)
	}
}

func TestGenerateSourcesSupportsCapturedClosureSort(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			mut items = [3, 1, 2]
			let bias = 0
			items.sort(fn(a: Int, b: Int) Bool {
				a + bias < b + bias
			})
			items.at(0)
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "sort.SliceStable") {
		t.Fatalf("generated source missing list sort lowering:\n%s", source)
	}
	if !strings.Contains(source, "func(a int, b int) bool") {
		t.Fatalf("generated source missing closure literal lowering:\n%s", source)
	}
	if !strings.Contains(source, "bias") {
		t.Fatalf("generated source missing closure capture usage:\n%s", source)
	}
}

func TestGenerateSourcesSupportsUserTraitObjectDispatch(t *testing.T) {
	program := lowerSource(t, `
		trait Renderable {
			fn render() Str
		}

		struct Block {
			title: Str,
		}

		struct Para {
			body: Str,
		}

		impl Renderable for Block {
			fn render() Str {
				"[block:" + self.title + "]"
			}
		}

		impl Renderable for Para {
			fn render() Str {
				"[para:" + self.body + "]"
			}
		}

		fn draw(r: Renderable) Str {
			r.render()
		}

		fn main() Str {
			draw(Block{title: "hi"}) + draw(Para{body: "there"})
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "switch typed := r.(type)") {
		t.Fatalf("generated source missing trait object dispatch lowering:\n%s", source)
	}
	if !strings.Contains(source, "Block_Renderable_render(typed)") {
		t.Fatalf("generated source missing Block trait dispatch call:\n%s", source)
	}
	if !strings.Contains(source, "Para_Renderable_render(typed)") {
		t.Fatalf("generated source missing Para trait dispatch call:\n%s", source)
	}
	if !strings.Contains(source, "panic(") {
		t.Fatalf("generated source missing trait dispatch fallback panic:\n%s", source)
	}
}

func TestGenerateSourcesSupportsCrossModuleTraitObjectDispatch(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"checkprobe\"\nard = \">= 0.13.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "widget.ard"), []byte(`
struct Frame { size: Int }

trait Widget {
  fn render(frame: Frame)
}

struct Text { content: Str }

impl Widget for Text {
  fn render(frame: Frame) { () }
}

fn plain(content: Str) Widget {
  Text{content: content}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	result := parse.Parse([]byte(`
use checkprobe/widget

fn main() {
  let f = widget::Frame{size: 10}
  let t = widget::plain("hi")
  t.render(f)
}
`), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New(filepath.Join(tempDir, "main.ard"), result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := ""
	for _, data := range sources {
		if strings.Contains(string(data), "switch typed := t_1.(type)") {
			source = string(data)
			break
		}
	}
	if source == "" {
		t.Fatalf("generated sources missing trait dispatch: %#v", mapsKeys(sources))
	}
	if !strings.Contains(source, "case checkprobe_widget__Text:") {
		t.Fatalf("generated source missing cross-module trait dispatch case:\n%s", source)
	}
	if !strings.Contains(source, "checkprobe_widget__Text_Widget_render(typed, f_0)") {
		t.Fatalf("generated source missing cross-module trait dispatch call:\n%s", source)
	}
}

func TestGenerateSourcesUsesCallSiteImportsForCrossModuleTraitObjectDispatch(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"nestprobe\"\nard = \">= 0.13.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(tempDir, "commands"), filepath.Join(tempDir, "tui", "core")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"tui/core/widget.ard": `
struct Frame { size: Int }

trait Widget {
  fn render(frame: Frame)
}
`,
		"tui/core/text.ard": `
use nestprobe/tui/core/widget

struct Text { content: Str }

impl widget::Widget for Text {
  fn render(frame: widget::Frame) { () }
}

fn plain(content: Str) widget::Widget {
  Text{content: content}
}
`,
		"tui/core/box.ard": `
use nestprobe/tui/core/widget

struct Box { child: widget::Widget }

impl widget::Widget for Box {
  fn render(frame: widget::Frame) {
    self.child.render(frame)
  }
}

fn wrap(child: widget::Widget) widget::Widget {
  Box{child: child}
}
`,
		"commands/demo.ard": `
use nestprobe/tui/core/widget
use nestprobe/tui/core/text as textw
use nestprobe/tui/core/box as boxw

fn run() {
  let f = widget::Frame{size: 10}
  let demo = boxw::wrap(textw::plain("hi"))
  demo.render(f)
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tempDir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	result := parse.Parse([]byte(`
use nestprobe/commands/demo

fn main() {
  demo::run()
}
`), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New(filepath.Join(tempDir, "main.ard"), result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := ""
	for _, data := range sources {
		if strings.Contains(string(data), "switch typed := demo_1.(type)") {
			source = string(data)
			break
		}
	}
	if source == "" {
		t.Fatalf("generated sources missing call-site trait dispatch: %#v", mapsKeys(sources))
	}
	if !strings.Contains(source, "case nestprobe_tui_core_box__Box:") {
		t.Fatalf("generated source missing Box dispatch case from call-site imports:\n%s", source)
	}
	if !strings.Contains(source, "case nestprobe_tui_core_text__Text:") {
		t.Fatalf("generated source missing Text dispatch case from call-site imports:\n%s", source)
	}
}

func TestGenerateSourcesUsesAliasOriginImportsForTraitObjectDispatch(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"aliasprobe\"\nard = \">= 0.13.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(tempDir, "commands"), filepath.Join(tempDir, "widgets")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"widget.ard": `
struct Frame { size: Int }

trait Widget {
  fn render(frame: Frame)
}
`,
		"widgets/text.ard": `
use aliasprobe/widget

struct Text { content: Str }

impl widget::Widget for Text {
  fn render(frame: widget::Frame) { () }
}

fn new(content: Str) widget::Widget { Text{content: content} }
`,
		"widgets/box.ard": `
use aliasprobe/widget

struct Box { child: widget::Widget }

impl widget::Widget for Box {
  fn render(frame: widget::Frame) { self.child.render(frame) }
}

fn new(child: widget::Widget) widget::Widget { Box{child: child} }
`,
		"facade_let.ard": `
use aliasprobe/widgets/text
use aliasprobe/widgets/box

let make_text = text::new
let make_box = box::new
`,
		"commands/demo.ard": `
use aliasprobe/widget
use aliasprobe/facade_let as facade

fn run() {
  let f = widget::Frame{size: 10}
  let w = facade::make_box(facade::make_text("hi"))
  w.render(f)
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tempDir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	result := parse.Parse([]byte(`
use aliasprobe/widgets/text
use aliasprobe/widgets/box
use aliasprobe/commands/demo

fn main() { demo::run() }
`), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New(filepath.Join(tempDir, "main.ard"), result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := ""
	for _, data := range sources {
		if strings.Contains(string(data), "switch typed := w_1.(type)") {
			source = string(data)
			break
		}
	}
	if source == "" {
		t.Fatalf("generated sources missing aliased-constructor trait dispatch: %#v", mapsKeys(sources))
	}
	if !strings.Contains(source, "case aliasprobe_widgets_box__Box:") {
		t.Fatalf("generated source missing Box dispatch case through let alias:\n%s", source)
	}
	if !strings.Contains(source, "case aliasprobe_widgets_text__Text:") {
		t.Fatalf("generated source missing Text dispatch case through let alias:\n%s", source)
	}
}

func TestGenerateSourcesSupportsVoidTraitObjectDispatch(t *testing.T) {
	program := lowerSource(t, `
		use ard/io

		trait Greet {
			fn say()
		}

		struct Cat {
			name: Str,
		}

		impl Greet for Cat {
			fn say() {
				io::print("meow from {self.name}")
			}
		}

		fn invoke(g: Greet) {
			g.say()
		}

		fn main() {
			invoke(Cat{name: "milo"})
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "switch typed := g.(type)") {
		t.Fatalf("generated source missing void trait object dispatch lowering:\n%s", source)
	}
	if !strings.Contains(source, "Cat_Greet_say(typed)") {
		t.Fatalf("generated source missing void trait dispatch call:\n%s", source)
	}
	if strings.Contains(source, "= test_ard__Cat_Greet_say(typed)") || strings.Contains(source, "= Cat_Greet_say(typed)") {
		t.Fatalf("generated source assigns void trait dispatch result:\n%s", source)
	}
	if !strings.Contains(source, "invoke(any(") {
		t.Fatalf("generated source missing trait upcast for call argument:\n%s", source)
	}
}

func TestGenerateSourcesSupportsStoredTraitObjectDispatch(t *testing.T) {
	program := lowerSource(t, `
		use ard/io

		trait Drawable {
			fn draw() Str
		}

		struct Box {
			w: Int,
		}

		impl Drawable for Box {
			fn draw() Str {
				"box[{self.w}]"
			}
		}

		struct Container {
			child: Drawable,
		}

		fn show(d: Drawable) {
			io::print(d.draw())
		}

		fn main() {
			let d: Drawable = Box{w: 1}
			io::print(d.draw())

			let c = Container{child: Box{w: 2}}
			io::print(c.child.draw())

			let items: [Drawable] = [Box{w: 3}]
			show(items.at(0))
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "d_0 := any(") {
		t.Fatalf("generated source missing local trait-object upcast:\n%s", source)
	}
	if !strings.Contains(source, "child: any(") {
		t.Fatalf("generated source missing struct field trait-object upcast:\n%s", source)
	}
	if !strings.Contains(source, "[]any{any(") {
		t.Fatalf("generated source missing list element trait-object upcast:\n%s", source)
	}
	if !strings.Contains(source, "switch typed := d_0.(type)") {
		t.Fatalf("generated source missing local trait-object dispatch:\n%s", source)
	}
	if !strings.Contains(source, "switch typed := c_1.child.(type)") {
		t.Fatalf("generated source missing struct field trait-object dispatch:\n%s", source)
	}
	if !strings.Contains(source, "show(items_2[0])") {
		t.Fatalf("generated source missing list element trait-object use:\n%s", source)
	}
}

func TestGenerateSourcesSupportsTraitObjectDispatch(t *testing.T) {
	program := lowerSource(t, `
		use ard/io

		struct Book {
			title: Str,
		}

		impl Str::ToString for Book {
			fn to_str() Str {
				self.title
			}
		}

		fn show(item: Str::ToString) Str {
			item.to_str()
		}

		fn main() Str {
			show(Book{title: "The Hobbit"})
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "type switch") && !strings.Contains(source, "switch typed := item.(type)") {
		t.Fatalf("generated source missing trait object dispatch lowering:\n%s", source)
	}
	if !strings.Contains(source, "Book_ToString_to_str(typed)") {
		t.Fatalf("generated source missing concrete trait dispatch call:\n%s", source)
	}
}

func TestGenerateSourcesSupportsListSwapAndMapKeys(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			mut items = [1, 2, 3]
			items.swap(0, 2)
			let values = ["b": 2, "a": 1]
			let keys = values.keys()
			items.at(0) + keys.size()
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "items_0[local_") && !strings.Contains(source, "items_0[_tmp_") {
		t.Fatalf("generated source missing list swap lowering:\n%s", source)
	}
	if !strings.Contains(source, "ardSortedStringKeys(values_1)") {
		t.Fatalf("generated source missing map keys lowering:\n%s", source)
	}
}

func TestGenerateSourcesEmitsOnlyUsedImports(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			1
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if strings.Contains(source, "bufio \"bufio\"") || strings.Contains(source, "strconv \"strconv\"") || strings.Contains(source, "strings \"strings\"") {
		t.Fatalf("generated source included unused runtime imports:\n%s", source)
	}
}

func TestGenerateSourcesSupportsFieldMutation(t *testing.T) {
	program := lowerSource(t, `
		struct Counter {
			value: Int,
		}

		fn bump(counter: Counter) Int {
			mut current = counter
			current.value = current.value + 1
			current.value
		}

		fn main() Int {
			bump(Counter{value: 1})
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "current_1.value = current_1.value + 1") {
		t.Fatalf("generated source missing field mutation lowering:\n%s", source)
	}
}

func TestGenerateSourcesSupportsIfAndWhile(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			mut count = 0
			while count < 3 {
				count = count + 1
			}
			if count == 3 {
				count
			} else {
				0
			}
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source := string(sources["test.go"])
	if !strings.Contains(source, "< 3 {") {
		t.Fatalf("generated source missing while lowering:\n%s", source)
	}
	if !strings.Contains(source, "== 3 {") {
		t.Fatalf("generated source missing if lowering:\n%s", source)
	}
	if !strings.Contains(source, "var _tmp_0 int") {
		t.Fatalf("generated source missing expression temp lowering:\n%s", source)
	}
}

func TestCollectFFIGoImportsIncludesStdlibImportsWithoutSourceCheckout(t *testing.T) {
	imports := collectGoImportsFromEmbeddedArdModule()
	if imports["sql"] != "database/sql" {
		t.Fatalf("embedded stdlib FFI imports missing sql: %#v", imports)
	}
	if imports["http"] != "net/http" {
		t.Fatalf("embedded stdlib FFI imports missing http: %#v", imports)
	}
}

func TestWriteProgramUsesEmbeddedArdModuleForReleaseVersion(t *testing.T) {
	original := version.Version
	version.Version = "v0.19.1"
	t.Cleanup(func() { version.Version = original })

	program := lowerSource(t, `
		fn main() Void {
		}
	`)
	dir := t.TempDir()
	if err := writeProgram(dir, program, Options{PackageName: "main"}); err != nil {
		t.Fatalf("writeProgram error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	goMod := string(data)
	if !strings.Contains(goMod, "require github.com/akonwi/ard v0.0.0") {
		t.Fatalf("go.mod missing Ard module requirement:\n%s", goMod)
	}
	if !strings.Contains(goMod, "replace github.com/akonwi/ard => ./.ard/ard-module") {
		t.Fatalf("release go.mod missing embedded module replace:\n%s", goMod)
	}
	if strings.Contains(goMod, "/home/runner") {
		t.Fatalf("release go.mod must not contain CI source path:\n%s", goMod)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ard", "ard-module", "runtime", "maybe.go")); err != nil {
		t.Fatalf("embedded runtime module not written: %v", err)
	}
}

func TestWriteProgramUsesLocalReplaceForDevVersion(t *testing.T) {
	original := version.Version
	version.Version = "dev"
	t.Cleanup(func() { version.Version = original })

	program := lowerSource(t, `
		fn main() Void {
		}
	`)
	dir := t.TempDir()
	if err := writeProgram(dir, program, Options{PackageName: "main"}); err != nil {
		t.Fatalf("writeProgram error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	goMod := string(data)
	if !strings.Contains(goMod, "require github.com/akonwi/ard v0.0.0") || !strings.Contains(goMod, "replace github.com/akonwi/ard =>") {
		t.Fatalf("dev go.mod missing local replace:\n%s", goMod)
	}
}

func TestBuildProgramProducesBinary(t *testing.T) {
	program := lowerSource(t, `
		fn main() Void {
			()
		}
	`)

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "ard-bin")
	builtPath, err := BuildProgram(program, outputPath)
	if err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
	if builtPath != outputPath {
		t.Fatalf("built path = %q, want %q", builtPath, outputPath)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("built binary stat error = %v", err)
	}
}

func TestRunProgramPreservesArtifactsUnderArdOut(t *testing.T) {
	program := lowerSource(t, `
		fn main() Void {
			()
		}
	`)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	projectDir := t.TempDir()
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	if err := RunProgram(program, []string{"ard", "run", "main.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(projectDir, "ard-out", "go", "run", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected generated sources under %s", filepath.Join(projectDir, "ard-out", "go", "run"))
	}
}

func TestArtifactWorkspaceUsesProjectLocalArdOut(t *testing.T) {
	projectDir := t.TempDir()
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte("fn main() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := artifactRootDir(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if root != projectDir {
		t.Fatalf("artifact root = %q, want %q", root, projectDir)
	}
	workspace, err := artifactWorkspace(mainPath, "build")
	if err != nil {
		t.Fatal(err)
	}
	if workspace != filepath.Join(projectDir, "ard-out", "go", "build") {
		t.Fatalf("workspace = %q, want %q", workspace, filepath.Join(projectDir, "ard-out", "go", "build"))
	}
}

func mapsKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func lowerSource(t *testing.T, input string) *air.Program {
	t.Helper()
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	return program
}
