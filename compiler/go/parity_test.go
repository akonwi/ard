package gotarget

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	stdlibffi "github.com/akonwi/ard/std_lib/ffi"
	vmnext "github.com/akonwi/ard/vm_next"
)

type goParityCase struct {
	name  string
	input string
}

func TestGoTargetParityCoreCorpus(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "reassigning variables",
			input: `
				fn main() Int {
					mut val = 1
					val = 2
					val = 3
					val
				}
			`,
		},
		{name: "unary not", input: `fn main() Bool { not true }`},
		{name: "unary negative float", input: `fn main() Float { -20.1 }`},
		{name: "arithmetic precedence", input: `fn main() Int { 30 + (20 * 4) }`},
		{name: "chained comparisons", input: `fn main() Bool { 200 <= 250 <= 300 }`},
		{
			name: "if else if else",
			input: `
				fn main() Str {
					let is_on = false
					mut result = ""
					if is_on { result = "then" }
					else if result.size() == 0 { result = "else if" }
					else { result = "else" }
					result
				}
			`,
		},
		{
			name: "inline block expression",
			input: `
				fn main() Int {
					let value = {
						let x = 10
						let y = 32
						x + y
					}
					value
				}
			`,
		},
		{
			name: "recursive function",
			input: `
				fn fib(n: Int) Int {
					match n <= 1 {
						true => n,
						false => fib(n - 1) + fib(n - 2),
					}
				}
				fn main() Int {
					fib(8)
				}
			`,
		},
		{
			name: "while loop accumulation",
			input: `
				fn main() Int {
					mut i = 0
					mut total = 0
					while i < 5 {
						total = total + i
						i = i + 1
					}
					total
				}
			`,
		},
		{
			name: "first class function value",
			input: `
				fn main() Int {
					let sub = fn(a: Int, b: Int) Int { a - b }
					sub(30, 8)
				}
			`,
		},
		{
			name: "closure lexical scoping",
			input: `
				fn createAdder(base: Int) fn(Int) Int {
					fn(x: Int) Int {
						base + x
					}
				}

				fn main() Int {
					let addFive = createAdder(5)
					addFive(10)
				}
			`,
		},
		{
			name: "list sort with closure",
			input: `
				fn main() [Int] {
					mut values = [5, 1, 3]
					values.sort(fn(a: Int, b: Int) Bool { a < b })
					values
				}
			`,
		},
		{
			name: "map keys use sorted order",
			input: `
				fn main() [Str] {
					let values = ["b": 2, "a": 1, "c": 3]
					values.keys()
				}
			`,
		},
		{
			name: "maybe match some",
			input: `
				use ard/maybe

				fn main() Int {
					match maybe::some(42) {
						s => s,
						_ => 0,
					}
				}
			`,
		},
		{
			name: "result match ok",
			input: `
				fn main() Int {
					match Result::ok(42) {
						ok => ok,
						err => 0,
					}
				}
			`,
		},
		{
			name: "struct field reassignment",
			input: `
				struct Person { name: Str, age: Int }

				fn main() Int {
					mut person = Person{name: "Alice", age: 30}
					person.age = 31
					person.age
				}
			`,
		},
		{
			name: "enum match",
			input: `
				enum Light { Red, Yellow, Green }

				fn main() Str {
					match Light::Yellow {
						Light::Red => "stop",
						Light::Yellow => "wait",
						Light::Green => "go",
					}
				}
			`,
		},
		{
			name: "map literal",
			input: `
				fn main() [Str: Int] {
					["a": 1, "b": 2]
				}
			`,
		},
	})
}

func TestGoTargetParityLoops(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "basic for loop",
			input: `
				fn main() Int {
					mut sum = 0
					for mut even = 0; even <= 10; even =+ 2 {
						sum =+ even
					}
					sum
				}
			`,
		},
		{
			name: "loop over numeric range",
			input: `
				fn main() Int {
					mut sum = 0
					for i in 1..5 {
						sum = sum + i
					}
					sum
				}
			`,
		},
		{
			name: "loop over a number",
			input: `
				fn main() Int {
					mut sum = 0
					for i in 5 {
						sum = sum + i
					}
					sum
				}
			`,
		},
		{
			name: "looping over a string",
			input: `
				fn main() Str {
					mut res = ""
					for c in "hello" {
						res = "{c}{res}"
					}
					res
				}
			`,
		},
		{
			name: "looping over a list",
			input: `
				fn main() Int {
					mut sum = 0
					for n in [1,2,3,4,5] {
						sum = sum + n
					}
					sum
				}
			`,
		},
		{
			name: "looping over a map",
			input: `
				fn main() Int {
					mut sum = 0
					for k,count in ["key":3, "foobar":6] {
						sum =+ count
					}
					sum
				}
			`,
		},
		{
			name: "looping over a map uses sorted keys",
			input: `
				fn main() Str {
					mut out = ""
					for key,val in [3:"c", 1:"a", 2:"b"] {
						out = out + "{key}:{val};"
					}
					out
				}
			`,
		},
		{
			name: "break out of loop",
			input: `
				fn main() Int {
					mut count = 5
					while count > 0 {
						count = count - 1
						if count == 3 {
							break
						}
					}
					count
				}
			`,
		},
	})
}

func TestGoTargetParityNullableArguments(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "omitting nullable parameters",
			input: `
				use ard/maybe
				fn add(a: Int, b: Int?) Int {
					a + b.or(0)
				}
				fn main() Int {
					add(1)
				}
			`,
		},
		{
			name: "omitting nullable parameters with explicit value",
			input: `
				use ard/maybe
				fn add(a: Int, b: Int?) Int {
					a + b.or(0)
				}
				fn main() Int {
					add(1, maybe::some(5))
				}
			`,
		},
		{
			name: "automatic wrapping of non nullable values for nullable parameters",
			input: `
				fn add(a: Int, b: Int?) Int {
					a + b.or(0)
				}
				fn main() Int {
					add(1, 5)
				}
			`,
		},
		{
			name: "automatic wrapping works with omitted args",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				fn main() Str {
					test(42)
				}
			`,
		},
		{
			name: "automatic wrapping with all arguments provided",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				fn main() Str {
					test(42, 7, "hello")
				}
			`,
		},
		{
			name: "automatic wrapping of list literals for nullable parameters",
			input: `
				fn process(items: [Int]?) Bool {
					match items {
						lst => true
						_ => false
					}
				}
				fn main() Bool {
					process([10, 20, 30])
				}
			`,
		},
		{
			name: "automatic wrapping of map literals for nullable parameters",
			input: `
				fn process(data: [Str:Int]?) Bool {
					match data {
						m => true
						_ => false
					}
				}
				fn main() Bool {
					process(["count": 42])
				}
			`,
		},
		{
			name: "automatic wrapping with labeled arguments in different order",
			input: `
				fn test(a: Int, b: Int?, c: Str?) Str {
					let bval = b.or(0)
					let cval = c.or("default")
					"{a},{bval},{cval}"
				}
				fn main() Str {
					test(c: "reorder", b: 99, a: 5)
				}
			`,
		},
	})
}

func TestGoTargetParityAnonymousFunctionInference(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "callback with inferred Str parameter",
			input: `
				fn process(f: fn(Str) Bool) Bool {
					f("hello")
				}
				fn main() Bool {
					process(fn(x) { x.size() > 0 })
				}
			`,
		},
		{
			name: "callback with inferred Bool return type",
			input: `
				fn check(f: fn(Str) Bool) Bool {
					f("test")
				}
				fn main() Bool {
					check(fn(s) { true })
				}
			`,
		},
	})
}

func TestGoTargetParityNullableStructFields(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "implicit wrapping of non nullable value for nullable struct field",
			input: `
				struct Config {
					name: Str,
					timeout: Int?,
				}
				fn main() Int {
					let c = Config{name: "app", timeout: 30}
					c.timeout.or(0)
				}
			`,
		},
		{
			name: "omitting nullable struct field still works",
			input: `
				struct Config {
					name: Str,
					timeout: Int?,
				}
				fn main() Int {
					let c = Config{name: "app"}
					c.timeout.or(0)
				}
			`,
		},
		{
			name: "explicit maybe some still works for struct fields",
			input: `
				use ard/maybe
				struct Config {
					name: Str,
					timeout: Int?,
				}
				fn main() Int {
					let c = Config{name: "app", timeout: maybe::some(30)}
					c.timeout.or(0)
				}
			`,
		},
		{
			name: "implicit wrapping of list literal for nullable struct field",
			input: `
				struct Data {
					items: [Int]?,
				}
				fn main() Int {
					let d = Data{items: [1, 2, 3]}
					match d.items {
						lst => lst.size()
						_ => 0
					}
				}
			`,
		},
		{
			name: "implicit wrapping of map literal for nullable struct field",
			input: `
				struct Data {
					meta: [Str:Int]?,
				}
				fn main() Bool {
					let d = Data{meta: ["count": 42]}
					match d.meta {
						m => true
						_ => false
					}
				}
			`,
		},
		{
			name: "implicit wrapping with multiple nullable fields",
			input: `
				struct Options {
					a: Int?,
					b: Str?,
					c: Bool?,
				}
				fn main() Str {
					let o = Options{a: 1, b: "hi", c: true}
					let av = o.a.or(0)
					let bv = o.b.or("")
					let cv = o.c.or(false)
					"{av},{bv},{cv}"
				}
			`,
		},
	})
}

func TestGoTargetParityTry(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "try early return with catch block",
			input: `
				fn test_early_return() Str {
					try Result::err("test") -> err {
						"caught: {err}"
					}
					"should not reach here"
				}
				fn main() Str {
					test_early_return()
				}
			`,
		},
		{
			name: "try success with catch block",
			input: `
				fn foobar() Int!Str {
					Result::ok(42)
				}
				fn do_thing() Str {
					let result = try foobar() -> err {
						"error: {err}"
					}
					"success: {result}"
				}
				fn main() Str {
					do_thing()
				}
			`,
		},
		{
			name: "try without catch propagates error",
			input: `
				fn foobar() Int!Str {
					Result::err("error")
				}
				fn do_thing() Int!Str {
					let res = try foobar()
					Result::ok(res * 2)
				}
				fn main() Int!Str {
					do_thing()
				}
			`,
		},
		{
			name: "try without catch success",
			input: `
				fn foobar() Int!Str {
					Result::ok(21)
				}
				fn do_thing() Int!Str {
					let res = try foobar()
					Result::ok(res * 2)
				}
				fn main() Int!Str {
					do_thing()
				}
			`,
		},
		{
			name: "try in enum match success case",
			input: `
				enum Status { active, inactive }
				fn get_result() Int!Str {
					Result::ok(42)
				}
				fn process_status(status: Status) Int!Str {
					match status {
						Status::active => {
							let value = try get_result()
							Result::ok(value + 1)
						}
						Status::inactive => Result::err("inactive")
					}
				}
				fn main() Int {
					process_status(Status::active).expect("")
				}
			`,
		},
		{
			name: "try in maybe match success",
			input: `
				use ard/maybe
				fn get_result() Int!Str {
					Result::ok(100)
				}
				fn process_maybe(maybe_val: Int?) Int!Str {
					match maybe_val {
						val => {
							let result = try get_result()
							Result::ok(result + val)
						}
						_ => Result::err("no value")
					}
				}
				fn main() Int {
					process_maybe(maybe::some(5)).expect("")
				}
			`,
		},
		{
			name: "try with catch in match block",
			input: `
				fn risky_operation() Str!Str {
					Result::err("operation failed")
				}
				fn process_with_catch(flag: Bool) Str {
					match flag {
						true => {
							try risky_operation() -> err {
								"caught: {err}"
							}
						}
						false => "no operation"
					}
				}
				fn main() Str {
					process_with_catch(true)
				}
			`,
		},
	})
}

func TestGoTargetParityEnumsUnionsAndGenericEquality(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "enum to int comparison",
			input: `
				enum Status { active, inactive, pending }
				fn main() Bool {
					let status = Status::active
					status == 0
				}
			`,
		},
		{
			name: "enum explicit value",
			input: `
				enum HttpStatus {
					Ok = 200,
					Created = 201,
					Not_Found = 404
				}
				fn main() HttpStatus {
					HttpStatus::Ok
				}
			`,
		},
		{
			name: "enum equality",
			input: `
				enum Direction { Up, Down, Left, Right }
				fn main() Bool {
					let dir1 = Direction::Up
					let dir2 = Direction::Down
					dir1 == dir2
				}
			`,
		},
		{
			name: "union matching",
			input: `
				type Printable = Str | Int | Bool
				fn print(p: Printable) Str {
					match p {
						Str(str) => str,
						Int(int) => int.to_str(),
						_ => "boolean value"
					}
				}
				fn main() Str {
					print(20)
				}
			`,
		},
		{
			name: "direct generic return compared with equals",
			input: `
				fn id<$T>(value: $T) $T {
					value
				}
				fn main() Bool {
					id(3) == 3
				}
			`,
		},
		{
			name: "inline maybe wrapping in generic context",
			input: `
				use ard/maybe
				fn main() Bool {
					mut found = maybe::none<Int>()
					let list = [1, 2, 3, 4, 5]
					for t in list {
						if t == 3 {
							found = maybe::some(t)
							break
						}
					}
					match found {
						value => value == 3,
						_ => false
					}
				}
			`,
		},
	})
}

func TestGoTargetParityStringHelpers(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{name: "int to str", input: `fn main() Str { 100.to_str() }`},
		{name: "bool to str", input: `fn main() Str { true.to_str() }`},
		{name: "str replace all", input: `fn main() Str { "hello world hello world".replace_all("world", "universe") }`},
		{name: "str contains", input: `fn main() Bool { "hello".contains("ell") }`},
		{name: "str starts with", input: `fn main() Bool { "hello".starts_with("he") }`},
		{name: "str split", input: `fn main() [Str] { "a,b,c".split(",") }`},
		{name: "str trim", input: `fn main() Str { "  hello \n".trim() }`},
		{name: "str is empty", input: `fn main() Bool { "".is_empty() }`},
	})
}

func TestGoTargetParityMatching(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "matching on booleans",
			input: `
				fn main() Str {
					let is_on = true
					match is_on {
						true => "on",
						false => "off"
					}
				}
			`,
		},
		{
			name: "enum catch all",
			input: `
				enum Direction {
					Up, Down, Left, Right
				}
				fn main() Str {
					let dir: Direction = Direction::Right
					match dir {
						Direction::Up => "North",
						Direction::Down => "South",
						_ => "skip"
					}
				}
			`,
		},
		{
			name: "matching on an int",
			input: `
				fn main() Str {
					let value = 0
					match value {
						-1 => "less",
						0 => "equal",
						1 => "greater",
						_ => "panic"
					}
				}
			`,
		},
		{
			name: "matching with ranges",
			input: `
				fn main() Str {
					let value = 80
					match value {
						-100..0 => "how?",
						0..60 => "F",
						_ => "pass"
					}
				}
			`,
		},
		{
			name: "matching on int with custom enum values",
			input: `
				enum HttpStatus {
					Ok = 200,
					Created = 201,
					NotFound = 404,
					ServerError = 500
				}
				fn main() Str {
					let code: Int = 404
					match code {
						HttpStatus::Ok => "Success",
						HttpStatus::Created => "Created",
						HttpStatus::NotFound => "Not Found",
						HttpStatus::ServerError => "Server Error",
						_ => "Unknown"
					}
				}
			`,
		},
		{
			name: "matching on int with mixed custom enum values and ranges",
			input: `
				enum Status {
					Pending = 0,
					Active = 100,
					Inactive = 101,
					Deleted = 999
				}
				fn main() Str {
					let code: Int = 150
					match code {
						Status::Pending => "Pending",
						Status::Active => "Active",
						Status::Inactive => "Inactive",
						100..199 => "In range 100-199",
						Status::Deleted => "Deleted",
						_ => "Unknown"
					}
				}
			`,
		},
		{
			name: "conditional matching with catch all",
			input: `
				fn main() Str {
					let score = 85
					match {
						score >= 90 => "A",
						score >= 80 => "B",
						score >= 70 => "C",
						score >= 60 => "D",
						_ => "F"
					}
				}
			`,
		},
		{
			name: "conditional matching with boolean conditions",
			input: `
				fn main() Str {
					let a = true
					let b = false
					match {
						a and b => "both true",
						a or b => "at least one true",
						_ => "both false"
					}
				}
			`,
		},
	})
}

func TestGoTargetParityCollectionsMutation(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "list prepend grows list",
			input: `
				fn main() Int {
					mut list = [1,2,3]
					list.prepend(4)
					list.size()
				}
			`,
		},
		{
			name: "list push grows list",
			input: `
				fn main() Int {
					mut list = [1,2,3]
					list.push(4)
					list.size()
				}
			`,
		},
		{
			name: "list at after push",
			input: `
				fn main() Int {
					mut list = [1,2,3]
					list.push(4)
					list.at(3)
				}
			`,
		},
		{
			name: "list set updates item",
			input: `
				fn main() Int {
					mut list = [1,2,3]
					list.set(1, 10)
					list.at(1)
				}
			`,
		},
		{
			name: "list swap swaps values",
			input: `
				fn main() Int {
					mut list = [1,2,3]
					list.swap(0,2)
					list.at(0)
				}
			`,
		},
		{
			name: "map size",
			input: `
				fn main() Int {
					let items = ["a": 1, "b": 2]
					items.size()
				}
			`,
		},
		{
			name: "map has existing key",
			input: `
				fn main() Bool {
					let items = ["a": 1, "b": 2]
					items.has("a")
				}
			`,
		},
		{
			name: "map get existing key",
			input: `
				fn main() Int {
					let items = ["a": 1, "b": 2]
					items.get("a").or(0)
				}
			`,
		},
		{
			name: "map drop removes key",
			input: `
				fn main() Bool {
					mut items = ["a": 1, "b": 2]
					items.drop("a")
					not items.has("a")
				}
			`,
		},
	})
}

func TestGoTargetParityMaybeResultCombinators(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "maybe or fallback",
			input: `
				use ard/maybe
				fn main() Str {
					let a: Str? = maybe::none()
					a.or("foo")
				}
			`,
		},
		{
			name: "maybe is none",
			input: `
				use ard/maybe
				fn main() Bool {
					maybe::none().is_none()
				}
			`,
		},
		{
			name: "maybe is some",
			input: `
				use ard/maybe
				fn main() Bool {
					maybe::some(1).is_some()
				}
			`,
		},
		{
			name: "maybe expect returns value",
			input: `
				use ard/maybe
				fn main() Int {
					maybe::some(42).expect("Should not panic")
				}
			`,
		},
		{
			name: "result or fallback",
			input: `
				fn divide(a: Int, b: Int) Int!Str {
					match b == 0 {
						true => Result::err("cannot divide by 0"),
						false => Result::ok(a / b),
					}
				}
				fn main() Int {
					let res = divide(100, 0)
					res.or(-1)
				}
			`,
		},
		{
			name: "result is ok",
			input: `
				fn main() Bool {
					Result::ok(42).is_ok()
				}
			`,
		},
		{
			name: "result is err",
			input: `
				fn main() Bool {
					Result::err("bad").is_err()
				}
			`,
		},
		{
			name: "maybe map transforms some",
			input: `
				use ard/maybe
				fn main() Int {
					let result = maybe::some(41).map(fn(value) { value + 1 })
					result.or(0)
				}
			`,
		},
		{
			name: "maybe map keeps none",
			input: `
				use ard/maybe
				fn main() Bool {
					let result: Int? = maybe::none()
					result.map(fn(value) { value + 1 }).is_none()
				}
			`,
		},
		{
			name: "maybe and then transforms some",
			input: `
				use ard/maybe
				fn main() Int {
					let result = maybe::some(21).and_then(fn(value) { maybe::some(value * 2) })
					result.or(0)
				}
			`,
		},
		{
			name: "maybe and then keeps none",
			input: `
				use ard/maybe
				fn main() Bool {
					let result: Int? = maybe::none()
					result.and_then(fn(value) { maybe::some(value + 1) }).is_none()
				}
			`,
		},
		{
			name: "result map transforms ok",
			input: `
				fn main() Int {
					let res: Int!Str = Result::ok(21)
					let mapped = res.map(fn(value) { value * 2 })
					mapped.or(0)
				}
			`,
		},
		{
			name: "result map leaves err unchanged",
			input: `
				fn main() Str {
					let res: Int!Str = Result::err("bad")
					let mapped = res.map(fn(value) { value * 2 })
					match mapped {
						err(msg) => msg,
						ok(value) => value.to_str(),
					}
				}
			`,
		},
		{
			name: "result map err transforms err",
			input: `
				fn main() Int {
					let res: Int!Str = Result::err("bad")
					let mapped = res.map_err(fn(err) { err.size() })
					match mapped {
						err(size) => size,
						ok(value) => value,
					}
				}
			`,
		},
		{
			name: "result map err leaves ok unchanged",
			input: `
				fn main() Int {
					let res: Int!Str = Result::ok(42)
					let mapped = res.map_err(fn(err) { err.size() })
					mapped.or(0)
				}
			`,
		},
		{
			name: "result and then chains ok",
			input: `
				fn main() Int {
					let res: Int!Str = Result::ok(21)
					let chained = res.and_then(fn(value) { Result::ok(value * 2) })
					chained.or(0)
				}
			`,
		},
		{
			name: "result and then propagates callback errors",
			input: `
				fn main() Bool {
					let res: Int!Str = Result::ok(21)
					let chained = res.and_then(fn(value) { Result::err("bad") })
					chained.is_err()
				}
			`,
		},
		{
			name: "result and then leaves err unchanged",
			input: `
				fn main() Bool {
					let res: Int!Str = Result::err("bad")
					let chained = res.and_then(fn(value) { Result::ok(value * 2) })
					chained.is_err()
				}
			`,
		},
	})
}

func runGoParityCases(t *testing.T, cases []goParityCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := lowerParitySource(t, tc.input)
			gotVM := runVMNextParityJSON(t, program)
			gotGo := runGoTargetParityJSON(t, program)
			if gotGo != gotVM {
				t.Fatalf("json mismatch\nvm_next: %s\ngo:      %s", gotVM, gotGo)
			}
		})
	}
}

func lowerParitySource(t *testing.T, input string) *air.Program {
	t.Helper()
	result := parse.Parse([]byte(input), "parity.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("parity.ard", result.Program, nil)
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

func runVMNextParityJSON(t *testing.T, program *air.Program) string {
	t.Helper()
	vm, err := vmnext.New(program)
	if err != nil {
		t.Fatalf("new vm: %v", err)
	}
	got, err := vm.RunEntry()
	if err != nil {
		t.Fatalf("run vm: %v", err)
	}
	encoded, err := stdlibffi.JsonEncode(normalizeJSONValue(got.GoValue()))
	if err != nil {
		t.Fatalf("encode vm result: %v", err)
	}
	return encoded
}

func runGoTargetParityJSON(t *testing.T, program *air.Program) string {
	t.Helper()
	tempDir := t.TempDir()
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate sources: %v", err)
	}
	for name, source := range sources {
		trimmed, err := stripGeneratedMain(source)
		if err != nil {
			t.Fatalf("strip main from %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(tempDir, name), trimmed, 0o644); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
	}
	rootID, err := rootFunction(program)
	if err != nil {
		t.Fatalf("root function: %v", err)
	}
	scriptFn := functionName(program, program.Functions[rootID])
	runner := fmt.Sprintf(`package main

import (
	"fmt"
	"reflect"
	stdlibffi "github.com/akonwi/ard/std_lib/ffi"
)

func main() {
	encoded, err := stdlibffi.JsonEncode(normalizeParityValue(%s()))
	if err != nil {
		panic(err)
	}
	fmt.Print(encoded)
}

func normalizeParityValue(value any) any {
	if value == nil {
		return nil
	}
	v := reflect.ValueOf(value)
	return normalizeReflectValue(v)
}

func normalizeReflectValue(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		if some := v.FieldByName("Some"); some.IsValid() && some.Kind() == reflect.Bool {
			if !some.Bool() {
				return nil
			}
			return normalizeReflectValue(v.FieldByName("Value"))
		}
		if ok := v.FieldByName("Ok"); ok.IsValid() && ok.Kind() == reflect.Bool {
			if ok.Bool() {
				return normalizeReflectValue(v.FieldByName("Value"))
			}
			return normalizeReflectValue(v.FieldByName("Err"))
		}
	}
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := range out {
			out[i] = normalizeReflectValue(v.Index(i))
		}
		return out
	case reflect.Map:
		out := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out[fmt.Sprint(iter.Key().Interface())] = normalizeReflectValue(iter.Value())
		}
		return out
	default:
		return v.Interface()
	}
}
`, scriptFn)
	if err := os.WriteFile(filepath.Join(tempDir, "runner.go"), []byte(runner), 0o644); err != nil {
		t.Fatalf("write runner: %v", err)
	}
	goMod := "module generated\n\ngo 1.26.0\n"
	if moduleRoot, ok := compilerModuleRoot(); ok {
		goMod += "\nrequire github.com/akonwi/ard v0.0.0\n"
		goMod += fmt.Sprintf("replace github.com/akonwi/ard => %s\n", moduleRoot)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	binaryPath := filepath.Join(tempDir, "parity-bin")
	if err := buildGeneratedProgram(tempDir, binaryPath); err != nil {
		t.Fatalf("build generated program: %v", err)
	}
	cmd := exec.Command(binaryPath)
	cmd.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run generated program: %v\nstderr:\n%s", err, stderr.String())
	}
	return stdout.String()
}

func stripGeneratedMain(source []byte) ([]byte, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "generated.go", source, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	filtered := file.Decls[:0]
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name != nil && fn.Name.Name == "main" {
			continue
		}
		filtered = append(filtered, decl)
	}
	file.Decls = filtered
	var out bytes.Buffer
	if err := format.Node(&out, fileSet, file); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func normalizeJSONValue(value any) any {
	switch v := value.(type) {
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[fmt.Sprint(key)] = normalizeJSONValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = normalizeJSONValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeJSONValue(item)
		}
		return out
	default:
		return value
	}
}
