package gotarget

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
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
