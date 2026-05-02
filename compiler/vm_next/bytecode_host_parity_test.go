package vm_next

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
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
			name: "fs::abs resolves relative path",
			input: fmt.Sprintf(`
				use ard/fs
				fs::abs(%q).expect("abs failed")
			`, tmpDir),
			want: tmpDir,
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
