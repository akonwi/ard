package gotarget

import (
	jsonv1 "encoding/json"
	jsonv2 "encoding/json/v2"
	"reflect"
	"strconv"
	"testing"

	"github.com/akonwi/ard/compiler/air"
)

func TestSupportedJSONFieldNamesRoundTripThroughGoTags(t *testing.T) {
	names := []string{"displayName", "with space", "snow☃", "emoji💡", "!#$%&()*+-./:;<=>?@[]^_{|}~ "}
	for _, name := range names {
		t.Run(strconv.Quote(name), func(t *testing.T) {
			literal := goStructFieldTag(air.FieldInfo{Name: "field", JSON: air.JSONFieldInfo{Name: name, HasName: true}})
			tag, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatalf("invalid generated Go tag literal %q: %v", literal.Value, err)
			}
			typ := reflect.StructOf([]reflect.StructField{{Name: "Field", Type: reflect.TypeFor[int](), Tag: reflect.StructTag(tag)}})
			value := reflect.New(typ).Elem()
			value.Field(0).SetInt(1)
			for encoderName, marshal := range map[string]func(any) ([]byte, error){
				"v1": jsonv1.Marshal,
				"v2": func(value any) ([]byte, error) { return jsonv2.Marshal(value) },
			} {
				data, err := marshal(value.Interface())
				if err != nil {
					t.Fatalf("%s marshal with tag %q: %v", encoderName, tag, err)
				}
				var object map[string]int
				if err := jsonv1.Unmarshal(data, &object); err != nil {
					t.Fatalf("decode %s output %q: %v", encoderName, data, err)
				}
				if object[name] != 1 || len(object) != 1 {
					t.Fatalf("%s output = %q, want sole key %q", encoderName, data, name)
				}
			}
		})
	}
}

func TestJSONFieldAttributesLowerToGoTags(t *testing.T) {
	program := lowerSource(t, `
		struct User {
			#json(name: "displayName", omit: none)
			display_name: Str?,
			#json(skip: true)
			password_hash: Str,
		}
	`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveStructFieldTag(files, "User", "DisplayName", "`json:\"displayName,omitzero\"`") {
		t.Fatal("generated User.DisplayName missing renamed omitzero JSON tag")
	}
	if !astFilesHaveStructFieldTag(files, "User", "PasswordHash", "`json:\"-\"`") {
		t.Fatal("generated User.PasswordHash missing skipped JSON tag")
	}
}

func TestRunProgramHonorsJSONFieldAttributes(t *testing.T) {
	program := lowerSource(t, `
		use go:encoding/json
		use go:fmt

		struct User {
			#json(name: "displayName")
			display_name: Str,
			#json(omit: none)
			nickname: Str?,
			#json(skip: true)
			password_hash: Str,
		}

		fn encode(user: User) Str {
			let bytes = try json::Marshal(user) -> err { panic(err) }
			fmt::Sprintf("%s", bytes)
		}

		fn main() {
			let present = encode(User{display_name: "Ada", nickname: "", password_hash: "secret"})
			if not present.contains("\"displayName\":\"Ada\"") {
				panic("renamed field missing: {present}")
			}
			if not present.contains("\"nickname\":\"\"") {
				panic("present empty Maybe was omitted: {present}")
			}
			if present.contains("password") {
				panic("skipped field was encoded: {present}")
			}

			let absent = encode(User{display_name: "Ada", password_hash: "secret"})
			if absent.contains("nickname") {
				panic("none field was encoded: {absent}")
			}
		}
	`)
	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramSupportsUnicodeJSONFieldNames(t *testing.T) {
	program := lowerSource(t, `
		use go:encoding/json
		use go:fmt

		struct Labels {
			#json(name: "snow☃")
			snow: Int,
			#json(name: "emoji💡")
			emoji: Int,
		}

		fn main() {
			let bytes = try json::Marshal(Labels{snow: 1, emoji: 2}) -> err { panic(err) }
			let encoded = fmt::Sprintf("%s", bytes)
			if not encoded.contains("\"snow☃\":1") or not encoded.contains("\"emoji💡\":2") {
				panic("Unicode JSON names missing: {encoded}")
			}
		}
	`)
	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

// TestRunProgramStrictGoJSONDecodingReachesMaybeFields pins issue #634: a
// strict Go decoder (DisallowUnknownFields) must reject unknown members inside
// a struct nested in a Maybe field, exactly as it does for a non-Maybe field.
func TestRunProgramStrictGoJSONDecodingReachesMaybeFields(t *testing.T) {
	program := lowerSource(t, `
		use go:encoding/json
		use go:strings

		struct Inner {
			known: Int,
		}

		struct Req {
			inner: Inner?,
		}

		fn decode_strict(input: Str, target: &mut Req) Void!Error {
			let decoder = json::NewDecoder(strings::NewReader(input))
			decoder.DisallowUnknownFields()
			decoder.Decode(target)
		}

		fn main() {
			mut req = Req{}
			let _ = try decode_strict("{{\"inner\": {{\"known\": 1}}}", &mut req) -> err { panic(err) }
			match req.inner {
				inner => {
					if inner.known != 1 {
						panic("strict decode lost the Maybe payload")
					}
				},
				_ => panic("strict decode produced none for a present payload"),
			}

			mut absent = Req{}
			let _ = try decode_strict("{{\"inner\": null}}", &mut absent) -> err { panic(err) }
			if absent.inner.is_some() {
				panic("strict decode of null produced some")
			}

			mut bad = Req{}
			match decode_strict("{{\"inner\": {{\"known\": 1, \"bogus\": 2}}}", &mut bad) {
				ok => panic("strict decode accepted an unknown field inside a Maybe"),
				err(e) => {
					if not e.error().contains("bogus") {
						panic("unexpected strict decode error: {e.error()}")
					}
				},
			}
		}
	`)
	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
