package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// TestRunProgramAdaptedGoFunctionValues pins that imported Go functions are
// first-class values. Their Ard callable types preserve variadicity through
// assignment and typed boundaries while result and descriptor ABI adaptation
// remains transparent.
func TestRunProgramAdaptedGoFunctionValues(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"fnvalues\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module fnvalues\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ffi", "ffi.go"), []byte(`package ffi

import "errors"

func Parse(s string) (int, error) {
	if s == "bad" {
		return 0, errors.New("bad input")
	}
	return len(s), nil
}

func Validate(s string) error {
	if s == "bad" {
		return errors.New("invalid")
	}
	return nil
}

func Find(key string) (string, bool) {
	if key == "hit" {
		return "found", true
	}
	return "", false
}

func EmptyResult(parts ...string) (struct{}, error) { return struct{}{}, nil }
func EmptyMaybe(parts ...string) (struct{}, bool) { return struct{}{}, len(parts) > 0 }

func Join(prefix string, parts ...string) string {
	out := prefix
	for _, p := range parts {
		out += ":" + p
	}
	return out
}

func Mutate(values ...string) {
	if len(values) > 0 { values[0] = "changed" }
}
func VariadicNil(values ...string) bool { return values == nil }
func VariadicAppendReuses(values ...string) bool {
	if len(values) == 0 { return false }
	before := &values[0]
	values = append(values, "appended")
	return before == &values[0]
}
func NilStrings() []string { return nil }
func EmptyStrings() []string { return []string{} }
type Strings []string
func NamedStrings() Strings { return Strings{"named", "slice"} }
func StringsRef() *Strings { values := Strings{"pointer", "slice"}; return &values }
type Item struct { Value string }
func NewItem(value string) *Item { return &Item{Value: value} }
func JoinItems(items ...*Item) string {
	out := ""
	for _, item := range items { out += item.Value }
	return out
}

var spreadOrder string
func ResetSpreadOrder() { spreadOrder = "" }
func MarkFixed(value string) string { spreadOrder += "f"; return value }
func SpreadValues() []string { spreadOrder += "s"; return []string{"spread"} }
func SpreadOrder() string { return spreadOrder }
func OrderedJoiner() *Joiner { spreadOrder += "r"; return &Joiner{prefix: "ordered"} }
func OrderedJoinFunc() func(string, ...string) string { spreadOrder += "c"; return Join }

func JoinResult(prefix string, parts ...string) (string, error) {
	return Join(prefix, parts...), nil
}

func SlicesNil(parts ...[]string) bool { return parts == nil }

func JoinSlices(parts ...[]string) string {
	out := ""
	for _, part := range parts {
		if len(part) > 0 { out += part[0] }
	}
	if len(parts) > 0 && len(parts[0]) > 0 { parts[0][0] = "changed" }
	return out
}

func JoinList(parts []string, extras ...string) string {
	out := ""
	for _, p := range parts {
		out += p
	}
	for _, p := range extras {
		out += p
	}
	return out
}

type Joiner struct {
	prefix string
}

var joinersCreated int

func NewJoiner(prefix string) *Joiner {
	joinersCreated++
	return &Joiner{prefix: prefix}
}

func JoinersCreated() int { return joinersCreated }

func (j *Joiner) Join(parts ...string) string {
	return Join(j.prefix, parts...)
}

func (j *Joiner) JoinResult(parts ...string) (string, error) {
	return Join(j.prefix, parts...), nil
}

func (j *Joiner) EmptyResult(parts ...string) (struct{}, error) { return struct{}{}, nil }
func (j *Joiner) EmptyMaybe(parts ...string) (struct{}, bool) { return struct{}{}, len(parts) > 0 }

type JoinFunc func(prefix string, parts ...string) string

func NamedJoin() JoinFunc { return Join }

func CallJoin(join func(string, ...string) string) string {
	return join("a", "b", "c")
}

func CountStrings(values ...string) string { return Join("", values...) }

func Invoke[T any](call func(...T) string, values ...T) string { return call(values...) }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:fnvalues/ffi

fn apply(f: fn(Str) Int!Str, input: Str) Int {
  f(input).or(-1)
}

fn apply_variadic(f: fn(Str, ...Str) Str) Str {
  f("a", "b", "c")
}

fn pass_variadic(f: fn(Str, ...Str) Str) fn(Str, ...Str) Str {
  f
}

struct Callables {
  join: fn(Str, ...Str) Str,
}

fn main() {
  // (T, error) adaptation
  let parse = ffi::Parse
  if not parse("four").or(-1) == 4 { panic("parse ok case failed") }
  if not parse("bad").or(-1) == -1 { panic("parse err case failed") }
  // adapted value flows as a function argument
  if not apply(ffi::Parse, "12345") == 5 { panic("adapted value as argument failed") }

  // error-only adaptation
  let validate = ffi::Validate
  if validate("ok").is_err() { panic("validate ok case failed") }
  if not validate("bad").is_err() { panic("validate err case failed") }

  // comma-ok adaptation
  let find = ffi::Find
  if not find("hit").or("") == "found" { panic("find hit case failed") }
  if find("miss").is_some() { panic("find miss case failed") }
  let empty_result = ffi::EmptyResult
  if empty_result("x").is_err() { panic("empty variadic result case failed") }
  let empty_maybe = ffi::EmptyMaybe
  if empty_maybe().is_some() { panic("empty variadic maybe none case failed") }
  if empty_maybe("x").is_none() { panic("empty variadic maybe some case failed") }

  // Variadicity survives capture, rebinding, parameters, and returns.
  let join: fn(Str, ...Str) Str = ffi::Join
  if not join("a") == "a" { panic("variadic zero case failed") }
  if not join("a", "b", "c") == "a:b:c" { panic("variadic repeated case failed") }
  let rebound = pass_variadic(join)
  if not rebound("x", "y", "z") == "x:y:z" { panic("variadic rebound case failed") }
  if not apply_variadic(ffi::Join) == "a:b:c" { panic("variadic parameter case failed") }
  let callables = Callables{join: ffi::Join}
  if not callables.join("f", "g", "h") == "f:g:h" { panic("variadic field case failed") }
  let join_result = ffi::JoinResult
  if not join_result("r", "s", "t").or("") == "r:s:t" { panic("variadic result case failed") }
  if not ffi::CallJoin(ffi::Join) == "a:b:c" { panic("variadic callback case failed") }
  if not ffi::Invoke<Str>(ffi::CountStrings, "a", "b") == ":a:b" { panic("generic variadic callback case failed") }
  let named_join = ffi::NamedJoin()
  if not named_join("n", "o", "p") == "n:o:p" { panic("named variadic function case failed") }

  // Referenced lists spread without copying through direct, captured, rebound,
  // field, result-adapted, generic, named-function, and named-slice call paths.
  let spread_values = ["b", "c"]
  let spread_reference = mut spread_values
  if not ffi::Join("a", spread_reference...) == "a:b:c" { panic("direct spread failed") }
  if not join("a", spread_reference...) == "a:b:c" { panic("captured spread failed") }
  if not rebound("a", spread_reference...) == "a:b:c" { panic("rebound spread failed") }
  if not callables.join("a", spread_reference...) == "a:b:c" { panic("field spread failed") }
  if not join_result("a", spread_reference...).or("") == "a:b:c" { panic("result spread failed") }
  if not ffi::Invoke<Str>(ffi::CountStrings, spread_reference...) == ":b:c" { panic("generic spread failed") }
  if not named_join("a", spread_reference...) == "a:b:c" { panic("named function spread failed") }
  let named_values = ffi::NamedStrings()
  if not ffi::Join("a", (mut named_values)...) == "a:named:slice" { panic("named slice spread failed") }
  let named_reference = ffi::StringsRef()
  if not ffi::Join("a", named_reference...) == "a:pointer:slice" { panic("named slice pointer spread failed") }
  let items = [ffi::NewItem("pointer"), ffi::NewItem("s")]
  if not ffi::JoinItems((mut items)...) == "pointers" { panic("pointer element spread failed") }

  // Spread preserves backing storage and nil versus nonnil-empty descriptors.
  let mutable_values = ["before"]
  ffi::Mutate((mut mutable_values)...)
  if not mutable_values.at(0).or("") == "changed" { panic("spread mutation was copied") }
  let nil_values = ffi::NilStrings()
  if not ffi::VariadicNil((mut nil_values)...) { panic("nil spread lost nil descriptor") }
  let empty_values = ffi::EmptyStrings()
  if ffi::VariadicNil((mut empty_values)...) { panic("empty spread became nil") }
  let view_source = ["visible", "hidden"]
  let view = view_source.slice(0, 1).expect("view")
  ffi::Mutate((mut view)...)
  if not view_source.at(0).or("") == "changed" { panic("Slice spread lost shared storage") }
  if ffi::VariadicAppendReuses((mut view)...) { panic("Slice spread exposed hidden capacity") }

  // Fixed arguments and the spread operand retain source evaluation order.
  ffi::ResetSpreadOrder()
  if not ffi::Join(ffi::MarkFixed("ordered"), (mut ffi::SpreadValues())...) == "ordered:spread" { panic("ordered direct spread failed") }
  if not ffi::SpreadOrder() == "fs" { panic("fixed/spread evaluation order failed") }
  ffi::ResetSpreadOrder()
  if not ffi::OrderedJoiner().Join((mut ffi::SpreadValues())...) == "ordered:spread" { panic("ordered receiver spread failed") }
  if not ffi::SpreadOrder() == "rs" { panic("receiver/spread evaluation order failed") }
  ffi::ResetSpreadOrder()
  if not ffi::OrderedJoinFunc()("ordered", (mut ffi::SpreadValues())...) == "ordered:spread" { panic("ordered callee spread failed") }
  if not ffi::SpreadOrder() == "cs" { panic("callee/spread evaluation order failed") }

  // Variadic descriptor elements remain explicit references and are
  // projected into the generated Go forwarding slice.
  if not ffi::SlicesNil() { panic("direct variadic descriptor nil tail lost") }
  let slices_nil = ffi::SlicesNil
  if not slices_nil() { panic("captured variadic descriptor nil tail lost") }
  let direct_first = ["a"]
  let direct_second = ["b"]
  if not ffi::JoinSlices(mut direct_first, mut direct_second) == "ab" { panic("direct variadic descriptor failed") }
  if not direct_first.at(0).or("") == "changed" { panic("direct variadic descriptor mutation lost") }
  let join_slices = ffi::JoinSlices
  let captured_first = ["c"]
  let captured_second = ["d"]
  if not join_slices(mut captured_first, mut captured_second) == "cd" { panic("captured variadic descriptor failed") }
  if not captured_first.at(0).or("") == "changed" { panic("captured variadic descriptor mutation lost") }

  // A fixed descriptor parameter still gets its boundary projection while
  // the variadic tail remains repeated.
  let join_list = ffi::JoinList
  let parts = ["a"]
  if not join_list(mut parts) == "a" { panic("descriptor variadic zero case failed") }
  if not join_list(mut parts, "b", "c") == "abc" { panic("descriptor variadic repeated case failed") }

  // Bound variadic methods have the same call shape and evaluate the receiver once.
  let joiner = ffi::NewJoiner("m")
  let method = joiner.Join
  if not method() == "m" { panic("method variadic zero case failed") }
  if not method("n", "o") == "m:n:o" { panic("method variadic repeated case failed") }
  let method_values = ["n", "o"]
  if not method((mut method_values)...) == "m:n:o" { panic("bound method spread failed") }
  if not joiner.Join((mut method_values)...) == "m:n:o" { panic("direct method spread failed") }
  let method_result = joiner.JoinResult
  if not method_result("p", "q").or("") == "m:p:q" { panic("method variadic result case failed") }
  if not method_result((mut method_values)...).or("") == "m:n:o" { panic("method result spread failed") }
  let empty_method_result = joiner.EmptyResult
  if empty_method_result("x").is_err() { panic("empty method result case failed") }
  let empty_method_maybe = joiner.EmptyMaybe
  if empty_method_maybe("x").is_none() { panic("empty method maybe case failed") }
  let temporary_method = ffi::NewJoiner("once").Join
  if not temporary_method("x") == "once:x" { panic("temporary method failed") }
  if not ffi::JoinersCreated() == 2 { panic("method receiver evaluated more than once") }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
