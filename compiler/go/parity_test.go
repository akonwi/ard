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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

type goParityCase struct {
	name  string
	input string
}

func TestGoTargetParityRecursiveStructFields(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "list self reference",
			input: `
				struct Node { value: Int, children: [Node] }
				fn main() Int {
					let root = Node{value: 1, children: [Node{value: 2, children: []}]}
					root.children.at(0).value
				}
			`,
		},
		{
			name: "map value self reference",
			input: `
				struct Node { value: Int, children: [Str:Node] }
				fn main() Int {
					let root = Node{value: 1, children: ["leaf": Node{value: 3, children: [:]}]}
					root.children.get("leaf").expect("").value
				}
			`,
		},
		{
			name: "nullable self reference",
			input: `
				use ard/maybe
				struct Node { value: Int, parent: Node? }
				fn main() Bool {
					let missing: Node? = maybe::none()
					let root = Node{value: 1, parent: missing}
					root.parent.is_none()
				}
			`,
		},
		{
			name: "nullable self reference some value",
			input: `
				use ard/maybe
				struct Node { value: Int, parent: Node? }
				fn main() Int {
					let root = Node{value: 1, parent: maybe::none()}
					let child = Node{value: 2, parent: maybe::some(root)}
					child.parent.expect("").value
				}
			`,
		},
		{
			name: "mutual nullable reference",
			input: `
				use ard/maybe
				struct A { b: B? }
				struct B { a: A }
				fn main() Int {
					let a = A{b: maybe::none()}
					let b = B{a: a}
					if b.a.b.is_none() { 1 } else { 0 }
				}
			`,
		},
		{
			name: "mutual nullable references",
			input: `
				use ard/maybe
				struct A { b: B? }
				struct B { a: A? }
				fn main() Int {
					let a = A{b: maybe::none()}
					if a.b.is_none() { 1 } else { 0 }
				}
			`,
		},
		{
			name: "generic nullable reference",
			input: `
				struct A { box: Box<A>? }
				struct Box { value: $T }
				fn main() Int { 1 }
			`,
		},
		{
			name: "nullable union reference",
			input: `
				use ard/maybe
				type U = A | Int
				struct A { u: U? }
				fn main() Int {
					let a = A{u: maybe::none()}
					if a.u.is_none() { 1 } else { 0 }
				}
			`,
		},
		{
			name: "function field self reference",
			input: `
				struct A { make: fn() A }
				fn main() Int { 1 }
			`,
		},
		{
			name: "same module retained tree recursive type group",
			input: `
				struct Context {
					tree: ViewTree,
					node_id: Int,
				}

				struct ViewTree {
					nodes: [TreeNode],
				}

				struct TreeNode {
					view: View,
					children: [Int],
				}

				trait View {
					fn init(ctx: Context)
					fn id() Int
				}

				struct Leaf {
					value: Int,
				}

				impl View for Leaf {
					fn init(ctx: Context) {}
					fn id() Int { self.value }
				}

				fn main() Int {
					let tree = ViewTree{nodes: []}
					let ctx = Context{tree: tree, node_id: 1}
					let node = TreeNode{view: Leaf{value: 41}, children: []}
					node.view.id() + ctx.node_id
				}
			`,
		},
	})
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
			name: "if else if chain preserves all conditions",
			input: `
				fn classify(key: Str) Int {
					mut result = 0
					if key == "q" {
						result = 1
					} else if key == "up" {
						result = 2
					} else if key == "down" {
						result = 3
					} else if key == "r" {
						result = 4
					} else {
						result = 5
					}
					result
				}

				fn main() Bool {
					classify("q") == 1 and classify("up") == 2 and classify("down") == 3 and classify("r") == 4 and classify("?") == 5
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
			name: "map set through struct field",
			input: `
				struct Response { headers: [Str: Str] }

				fn main() Str {
					mut res = Response{headers: [:]}
					let _ = res.headers.set("Content-Type", "application/json")
					match res.headers.get("Content-Type") {
						v => v,
						_ => "missing",
					}
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

func TestGoTargetParityFunctionValuedStructFields(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "direct function field call",
			input: `
				struct EventContext {}
				struct Option {}

				struct Props {
					on_confirm: fn(EventContext, Option) Int,
				}

				fn confirm(ctx: EventContext, opt: Option) Int { 42 }

				fn main() Int {
					let p = Props{on_confirm: confirm}
					p.on_confirm(EventContext{}, Option{})
				}
			`,
		},
		{
			name: "parenthesized function field call",
			input: `
				struct EventContext {}
				struct Option {}

				struct Props {
					on_confirm: fn(EventContext, Option) Int,
				}

				fn confirm(ctx: EventContext, opt: Option) Int { 42 }

				fn main() Int {
					let p = Props{on_confirm: confirm}
					(p.on_confirm)(EventContext{}, Option{})
				}
			`,
		},
		{
			name: "try direct result-returning function field call",
			input: `
				struct Props {
					cb: fn() Int!Str,
				}

				fn ok() Int!Str { Result::ok(41) }

				fn run() Int!Str {
					let p = Props{cb: ok}
					let x = try p.cb()
					Result::ok(x + 1)
				}

				fn main() Int {
					run().or(0)
				}
			`,
		},
		{
			name: "generic function field call",
			input: `
				struct Props {
					cb: $F,
				}

				fn ok() Int!Str { Result::ok(41) }

				fn run() Int!Str {
					let p: Props<fn() Int!Str> = Props{cb: ok}
					let x = try p.cb()
					Result::ok(x + 1)
				}

				fn main() Int {
					run().or(0)
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

func TestGoTargetParityTryOnMaybe(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "try on maybe some returns unwrapped value",
			input: `
				use ard/maybe
				fn get_value() Int? {
					maybe::some(42)
				}
				fn test() Int? {
					let value = try get_value()
					maybe::some(value + 1)
				}
				fn main() Int {
					let result = test()
					match result {
						value => value,
						_ => -1
					}
				}
			`,
		},
		{
			name: "try on maybe none propagates none",
			input: `
				use ard/maybe
				fn get_value() Int? {
					maybe::none()
				}
				fn test() Int? {
					let value = try get_value()
					maybe::some(value + 1)
				}
				fn main() Int {
					let result = test()
					match result {
						value => value,
						_ => -999
					}
				}
			`,
		},
		{
			name: "try on maybe with catch block transforms none",
			input: `
				use ard/maybe
				fn get_value() Int? {
					maybe::none()
				}
				fn main() Int {
					let value = try get_value() -> _ { 42 }
					value
				}
			`,
		},
		{
			name: "try on maybe chained fallback",
			input: `
				use ard/maybe
				struct Profile { name: Str? }
				struct User { profile: Profile? }
				fn get_user() User? {
					let profile = maybe::some(Profile{name: maybe::none()})
					maybe::some(User{profile: profile})
				}
				fn main() Str {
					let name = try get_user().profile.name -> _ { "Sample" }
					name
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

func TestGoTargetParityCryptoHashes(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "md5 hashes hello",
			input: `
				use ard/crypto
				use ard/hex
				fn main() Str {
					hex::encode(crypto::md5("hello".bytes()))
				}
			`,
		},
		{
			name: "sha256 returns raw bytes",
			input: `
				use ard/crypto
				fn main() Int {
					crypto::sha256("".bytes()).size()
				}
			`,
		},
		{
			name: "sha256 can be hex encoded",
			input: `
				use ard/crypto
				use ard/hex
				fn main() Str {
					hex::encode(crypto::sha256("".bytes()))
				}
			`,
		},
		{
			name: "sha512 returns raw bytes",
			input: `
				use ard/crypto
				fn main() Int {
					crypto::sha512("hello".bytes()).size()
				}
			`,
		},
		{
			name: "crypto password hashing",
			input: `
				use ard/crypto
				fn check() Bool!Str {
					let hashed = try crypto::hash("password123", 4)
					let verified = try crypto::verify("password123", hashed)
					let wrong = try crypto::verify("wrong-password", hashed)
					Result::ok(verified and not wrong and crypto::hash("password123", 32).is_err())
				}
				fn main() Bool {
					check().expect("crypto password check failed")
				}
			`,
		},
		{
			name: "crypto scrypt",
			input: `
				use ard/crypto
				fn check() Bool!Str {
					let hashed = try crypto::scrypt_hash("password", "73616c74", 16, 1, 1, 16)
					let verified = try crypto::scrypt_verify("password", hashed, 16, 1, 1, 16)
					let deterministic = hashed == "73616c74:d360147c2a2db7903186e387bb385547"
					let malformed = crypto::scrypt_verify("password123", "bad-hash").is_err()
					Result::ok(deterministic and verified and malformed)
				}
				fn main() Bool {
					check().expect("scrypt check failed")
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
			name: "primitive enum and int inequality",
			input: `
				enum Direction { Up, Down, Left, Right }
				fn main() Bool {
					let dir1 = Direction::Up
					let dir2 = Direction::Down
					let code: Int = 1
					1 != 2 and "a" != "b" and dir1 != dir2 and dir1 != code and code != dir1
				}
			`,
		},
		{
			name: "enum and int relational comparison",
			input: `
				enum Direction { Up, Down, Left, Right }
				fn main() Bool {
					let dir = Direction::Down
					let code: Int = 2
					dir < code and code >= dir
				}
			`,
		},
		{
			name: "enum and int comparison with int parameter",
			input: `
				enum Direction { Up, Down, Left, Right }
				fn differs(int: Int) Bool {
					Direction::Up != int
				}
				fn main() Bool {
					differs(1)
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

func TestGoTargetParityConcurrentMethodAccess(t *testing.T) {
	const workers = 20
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("panic: %v", r)
				}
			}()
			input := `
				fn main() Int {
					mut list = [1,2,3]
					list.push(4)
					list.size()
				}
			`
			if id%2 == 1 {
				input = `
					fn main() Str {
						"hello world".replace_all("world", "ard")
					}
				`
			}
			program := lowerParitySource(t, input)
			_ = runGoTargetParityJSON(t, program)
			errCh <- nil
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent go parity failed: %v", err)
		}
	}
}

func TestGoTargetParityConcurrentModuleAccess(t *testing.T) {
	const workers = 20
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("panic: %v", r)
				}
			}()
			program := lowerParitySource(t, `
				use ard/decode
				fn main() Int {
					let d = decode::from_json("[1,2,3]").expect("")
					decode::run(d, decode::list(decode::int)).expect("").size()
				}
			`)
			if got := runGoTargetParityJSON(t, program); got != "3" {
				errCh <- fmt.Errorf("got %s, want 3", got)
				return
			}
			errCh <- nil
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent go parity failed: %v", err)
		}
	}
}

func TestGoTargetParityUnsafeBlocks(t *testing.T) {
	t.Run("success result can be unwrapped", func(t *testing.T) {
		program := lowerParitySource(t, `
fn main() Int {
  unsafe {
    42
  }.expect("unsafe")
}`)
		if got := runGoTargetParityJSON(t, program); got != "42" {
			t.Fatalf("got %s, want 42", got)
		}
	})

	t.Run("panic is recovered as result error", func(t *testing.T) {
		program := lowerParitySource(t, `
fn main() Str {
  match unsafe {
    panic("boom")
    "ok"
  } {
    ok(value) => value,
    err(message) => message,
  }
}`)
		if got := runGoTargetParityJSON(t, program); got != `"boom"` {
			t.Fatalf("got %s, want boom", got)
		}
	})

	t.Run("try catches unsafe panic", func(t *testing.T) {
		program := lowerParitySource(t, `
fn main() Int {
  try unsafe {
    panic("boom")
    1
  } -> err { 7 }
  0
}`)
		if got := runGoTargetParityJSON(t, program); got != "7" {
			t.Fatalf("got %s, want 7", got)
		}
	})

	t.Run("try inside unsafe returns unsafe error", func(t *testing.T) {
		program := lowerParitySource(t, `
fn inner() Int!Str {
  Result::err("inner")
}

fn main() Str {
  match unsafe {
    let value = try inner()
    value.to_str()
  } {
    ok(value) => value,
    err(message) => message,
  }
}`)
		if got := runGoTargetParityJSON(t, program); got != `"inner"` {
			t.Fatalf("got %s, want inner", got)
		}
	})

	t.Run("try catch inside unsafe uses unsafe return type", func(t *testing.T) {
		program := lowerParitySource(t, `
fn inner() Int!Str {
  Result::err("inner")
}

fn main() Str {
  match unsafe {
    let value = try inner() -> err { Result::err("caught: {err}") }
    value.to_str()
  } {
    ok(value) => value,
    err(message) => message,
  }
}`)
		if got := runGoTargetParityJSON(t, program); got != `"caught: inner"` {
			t.Fatalf("got %s, want caught: inner", got)
		}
	})
}

func TestGoTargetParityMapClosureCapturesOuterLocal(t *testing.T) {
	t.Run("maybe map", func(t *testing.T) {
		program := lowerParitySource(t, `
			use ard/maybe

			fn main() Int {
				let offset = 2
				let result = maybe::some(40).map(fn(value) { value + offset })
				result.or(0)
			}
		`)
		if got := runGoTargetParityJSON(t, program); got != "42" {
			t.Fatalf("got %s, want 42", got)
		}
	})

	t.Run("result map", func(t *testing.T) {
		program := lowerParitySource(t, `
			fn main() Int {
				let multiplier = 2
				let res: Int!Str = Result::ok(21)
				let mapped = res.map(fn(value) { value * multiplier })
				mapped.or(0)
			}
		`)
		if got := runGoTargetParityJSON(t, program); got != "42" {
			t.Fatalf("got %s, want 42", got)
		}
	})
}

func TestGoTargetParityMutableParameterClosureInFunctionTypedMap(t *testing.T) {
	// A `mut T` parameter is represented two ways: as the `Mutable` flag (the
	// `fn(mut T)` function-type form used by the map's value type) and as a
	// `MutableRef` baked into the type (the `name: mut T` closure form). The two
	// must reconcile so the closure is assignable to the map and lowers to the
	// same Go signature.
	program := lowerParitySource(t, `
		struct Box { n: Int }

		fn main() Int {
			let base = 41
			mut handlers: [Str: fn(mut Box)] = [:]
			handlers.set("a", fn(b: mut Box) {})
			handlers.size() + base
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != "42" {
		t.Fatalf("got %s, want 42", got)
	}
}

func TestGoTargetParityNestedClosureCaptures(t *testing.T) {
	t.Run("returned closure captures two outer scopes", func(t *testing.T) {
		program := lowerParitySource(t, `
			fn make_nested(a: Int) fn(Int) fn(Int) Int {
				fn(b: Int) fn(Int) Int {
					fn(c: Int) Int {
						a + b + c
					}
				}
			}

			fn main() Int {
				let add = make_nested(10)
				let add_more = add(20)
				add_more(12)
			}
		`)
		if got := runGoTargetParityJSON(t, program); got != "42" {
			t.Fatalf("got %s, want 42", got)
		}
	})

	t.Run("callback captures variable from returned closure", func(t *testing.T) {
		program := lowerParitySource(t, `
			use ard/maybe

			fn make_mapper(offset: Int) fn(Int) Int {
				let bonus = 1
				fn(value: Int) Int {
					maybe::some(value).map(fn(inner) { inner + offset + bonus }).or(0)
				}
			}

			fn main() Int {
				let mapper = make_mapper(10)
				mapper(31)
			}
		`)
		if got := runGoTargetParityJSON(t, program); got != "42" {
			t.Fatalf("got %s, want 42", got)
		}
	})

	t.Run("nested callback captures local and parent closure variables", func(t *testing.T) {
		program := lowerParitySource(t, `
			use ard/maybe

			fn make_calc(base: Int) fn(Int) Int {
				fn(seed: Int) Int {
					let local = 2
					maybe::some(seed).map(fn(value) {
						value + base + local
					}).or(0)
				}
			}

			fn main() Int {
				let calc = make_calc(10)
				calc(30)
			}
		`)
		if got := runGoTargetParityJSON(t, program); got != "42" {
			t.Fatalf("got %s, want 42", got)
		}
	})
}

func TestGoTargetParityMutatingTraitImplClosureCapturesSelf(t *testing.T) {
	program := lowerParitySource(t, `
		trait Initializer {
			fn init()
		}

		struct Box {
			value: Int,
		}

		impl Initializer for Box {
			fn mut init() {
				let bump = fn() {
					self.value = self.value + 1
				}
				bump()
			}
		}

		fn main() Int {
			mut box = Box{value: 0}
			box.init()
			box.value
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != "1" {
		t.Fatalf("got %s, want 1", got)
	}
}

func TestGoTargetParityNativeTraitObjectMutableParameterFromTraitLocal(t *testing.T) {
	program := lowerParitySource(t, `
		trait Draw {
			fn draw() Int
		}

		struct Box {
			value: Int,
		}

		impl Draw for Box {
			fn draw() Int {
				self.value
			}
		}

		fn apply(mut d: Draw) Int {
			d.draw()
		}

		fn main() Int {
			mut d: Draw = Box{value: 1}
			apply(d)
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != "1" {
		t.Fatalf("got %s, want 1", got)
	}
}

func TestGoTargetParityMutableTraitObjectParameterFromConcrete(t *testing.T) {
	program := lowerParitySource(t, `
		struct Context {
			node_id: Int,
		}

		trait View {
			fn init(ctx: Context)
			fn node_id() Int
		}

		fn add_child(ctx: Context, mut child: View) {
			child.init(Context{node_id: ctx.node_id + 1})
		}

		struct Leaf {
			initialized: Bool,
			node_id: Int,
		}

		impl View for Leaf {
			fn mut init(ctx: Context) {
				self.initialized = true
				self.node_id = ctx.node_id
			}

			fn node_id() Int {
				self.node_id
			}
		}

		fn main() Int {
			mut leaf = Leaf{initialized: false, node_id: 0}
			add_child(Context{node_id: 41}, leaf)
			if leaf.initialized { leaf.node_id } else { 0 }
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != "42" {
		t.Fatalf("got %s, want 42", got)
	}
}

func TestGoTargetParityEscapedMutableTraitObjectUpcastAliasesConcrete(t *testing.T) {
	program := lowerParitySource(t, `
		use ard/maybe

		trait View {
			fn set(value: Int)
			fn value() Int
		}

		struct Leaf {
			n: Int,
		}

		impl View for Leaf {
			fn mut set(value: Int) {
				self.n = value
			}

			fn value() Int {
				self.n
			}
		}

		struct Branch {
			n: Int,
		}

		impl View for Branch {
			fn mut set(value: Int) {
				self.n = value
			}

			fn value() Int {
				self.n
			}
		}

		trait Sink {
			fn take(mut child: View) Int
		}

		struct Holder {}

		struct SinkNode {
			sink: mut Sink,
		}

		impl Sink for Holder {
			fn take(mut child: View) Int {
				child.set(41)
				child.value()
			}
		}

		struct Node {
			view: mut View,
		}

		struct Snapshot {
			view: View,
		}

		type ViewOrInt = View | Int

		fn store(mut child: View) Node {
			Node{view: child}
		}

		fn make(value: Int) View {
			Leaf{n: value}
		}

		struct Root {
			view: mut View,
		}

		impl Root {
			fn mut replace(value: Int) {
				self.view = Leaf{n: value}
			}
		}

		fn replace_param(mut child: View, value: Int) {
			child = Leaf{n: value}
		}

		fn assign_param(mut child: View, next: View) {
			child = next
		}

		fn make_reader(mut child: View) fn() Int {
			fn() Int {
				child.value()
			}
		}

		fn make_setter(mut child: View) fn(Int) {
			fn(value: Int) {
				child = Leaf{n: value}
			}
		}

		fn read(child: View) Int {
			child.value()
		}

		fn main() Int {
			mut stored_leaf = Leaf{n: 0}
			let stored_node = store(stored_leaf)
			stored_leaf.n = 7
			let stored_observed = stored_node.view.value()
			stored_node.view.set(11)

			mut direct_leaf = Leaf{n: 0}
			let direct_node = Node{view: direct_leaf}
			direct_leaf.n = 5
			let direct_observed = direct_node.view.value()
			direct_node.view.set(13)
			let immutable_observed = read(direct_node.view)
			let via_ref: View = direct_node.view
			let via_ref_observed = via_ref.value()
			let snapshot = Snapshot{view: direct_node.view}
			snapshot.view.set(59)
			let snapshot_observed = direct_leaf.n
			let view_list: [View] = [direct_node.view]
			view_list.at(0).set(61)
			let list_observed = direct_leaf.n
			let view_map: [Str:View] = ["x": direct_node.view]
			view_map.get("x").expect("missing").set(67)
			let map_observed = direct_leaf.n
			let maybe_view: View? = maybe::some(direct_node.view)
			maybe_view.expect("missing").set(69)
			let maybe_observed = direct_leaf.n
			let result_view: View!Str = Result::ok(direct_node.view)
			result_view.expect("missing").set(71)
			let result_observed = direct_leaf.n
			let union_view: ViewOrInt = direct_node.view
			match union_view {
				View(view) => view.set(73),
				_ => (),
			}
			let union_observed = direct_leaf.n
			mut push_views: [View] = []
			push_views.push(direct_node.view)
			push_views.at(0).set(75)
			let push_observed = direct_leaf.n
			mut prepend_views: [View] = []
			prepend_views.prepend(direct_node.view)
			prepend_views.at(0).set(77)
			let prepend_observed = direct_leaf.n
			mut set_views: [View] = [Leaf{n: 1}]
			set_views.set(0, direct_node.view)
			set_views.at(0).set(79)
			let set_observed = direct_leaf.n
			mut set_map: [Str:View] = [:]
			set_map.set("x", direct_node.view)
			set_map.get("x").expect("missing").set(81)
			let set_map_observed = direct_leaf.n

			mut dynamic_slot = make(2)
			let dynamic_node = Node{view: dynamic_slot}
			dynamic_slot = Branch{n: 17}
			let dynamic_observed = dynamic_node.view.value()
			dynamic_node.view.set(19)
			let dynamic_slot_observed = read(dynamic_slot)

			mut replace_slot = make(1)
			mut root = Root{view: replace_slot}
			root.replace(23)
			let replaced_observed = read(replace_slot)

			mut param_slot = make(3)
			let param_node = store(param_slot)
			param_slot = Leaf{n: 29}
			let param_observed = param_node.view.value()

			mut replaced_leaf = Leaf{n: 0}
			replace_param(replaced_leaf, 31)

			mut replaced_slot = make(4)
			replace_param(replaced_slot, 37)
			let replaced_slot_observed = read(replaced_slot)

			assign_param(replaced_leaf, direct_node.view)
			let forwarded_assign_observed = replaced_leaf.n

			mut closure_leaf = Leaf{n: 43}
			let reader = make_reader(closure_leaf)
			closure_leaf.n = 47
			let closure_observed = reader()

			let sink: Sink = Holder{}
			mut sink_leaf = Leaf{n: 0}
			let sink_result = sink.take(sink_leaf)

			mut holder = Holder{}
			let sink_node = SinkNode{sink: holder}
			mut node_sink_leaf = Leaf{n: 0}
			let node_sink_result = sink_node.sink.take(node_sink_leaf)

			mut setter_leaf = Leaf{n: 0}
			let setter = make_setter(setter_leaf)
			setter(53)
			let setter_observed = setter_leaf.n

			stored_observed + stored_leaf.n + direct_observed + direct_leaf.n + immutable_observed + via_ref_observed + snapshot_observed + list_observed + map_observed + maybe_observed + result_observed + union_observed + push_observed + prepend_observed + set_observed + set_map_observed + dynamic_observed + dynamic_slot_observed + replaced_observed + param_observed + replaced_leaf.n + replaced_slot_observed + forwarded_assign_observed + closure_observed + sink_result + sink_leaf.n + node_sink_result + node_sink_leaf.n + setter_observed
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != "607" {
		t.Fatalf("got %s, want 607", got)
	}
}

func TestGoTargetParityMutableTraitMethodNamesDoNotCollideWithForwarderHooks(t *testing.T) {
	program := lowerParitySource(t, `
		trait Weird {
			fn ardMutTraitLoad_0() Int
		}

		struct Box {
			n: Int,
		}

		impl Weird for Box {
			fn ardMutTraitLoad_0() Int {
				self.n
			}
		}

		struct Holder {
			weird: mut Weird,
		}

		fn main() Int {
			mut box = Box{n: 42}
			let holder = Holder{weird: box}
			holder.weird.ardMutTraitLoad_0()
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != "42" {
		t.Fatalf("got %s, want 42", got)
	}
}

func TestGoTargetParityMutatingTraitDispatchUpdatesStoredTraitObject(t *testing.T) {
	program := lowerParitySource(t, `
		trait View {
			fn handle_event()
			fn value() Int
		}

		struct CounterView {
			count: Int,
		}

		impl View for CounterView {
			fn mut handle_event() {
				self.count = self.count + 1
			}

			fn value() Int {
				self.count
			}
		}

		struct AppRoot {
			view: View,
		}

		impl AppRoot {
			fn mut dispatch() {
				self.view.handle_event()
			}

			fn current() Int {
				self.view.value()
			}
		}

		fn run_typed(mut typed: AppRoot) Int {
			typed.view.handle_event()
			typed.view.value()
		}

		fn run_any(mut any: AppRoot) Int {
			any.view.handle_event()
			any.view.value()
		}

		fn main() Int {
			mut app = AppRoot{view: CounterView{count: 0}}
			app.dispatch()
			let field_result = app.current()

			mut typed_app = AppRoot{view: CounterView{count: 0}}
			let typed_result = run_typed(typed_app)

			mut any_app = AppRoot{view: CounterView{count: 0}}
			let any_result = run_any(any_app)

			field_result + typed_result + any_result
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != "3" {
		t.Fatalf("got %s, want 3", got)
	}
}

func TestGoTargetParityMutableReferenceFieldUpdatesSharedStorage(t *testing.T) {
	program := lowerParitySource(t, `
		struct Tree {
			count: Int,
		}

		struct Context {
			tree: mut Tree,
		}

		fn bump(mut tree: Tree) {
			tree.count = tree.count + 1
		}

		struct Box {
			value: mut Int,
		}

		fn set(mut value: Int) {
			value = 3
		}

		fn main() Int {
			mut tree = Tree{count: 0}
			let ctx = Context{tree: tree}
			bump(ctx.tree)
			ctx.tree.count = 2

			mut count = 0
			let box = Box{value: count}
			set(box.value)

			ctx.tree.count + tree.count + box.value + count
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != "10" {
		t.Fatalf("got %s, want 10", got)
	}
}

func TestGoTargetParityMutableReferenceParameterUpdatesCaller(t *testing.T) {
	t.Run("struct", func(t *testing.T) {
		program := lowerParitySource(t, `
			struct Counter {
				value: Int,
			}

			fn bump(mut c: Counter) {
				c.value = c.value + 1
			}

			fn main() Int {
				mut counter = Counter{value: 0}
				bump(counter)
				counter.value
			}
		`)
		if got := runGoTargetParityJSON(t, program); got != "1" {
			t.Fatalf("got %s, want 1", got)
		}
	})

	t.Run("list", func(t *testing.T) {
		program := lowerParitySource(t, `
			fn append_one(mut values: [Int]) {
				values.push(1)
			}

			fn main() Int {
				mut values: [Int] = []
				append_one(values)
				values.size()
			}
		`)
		if got := runGoTargetParityJSON(t, program); got != "1" {
			t.Fatalf("got %s, want 1", got)
		}
	})

	t.Run("primitive", func(t *testing.T) {
		program := lowerParitySource(t, `
			fn bump(mut count: Int) {
				count = count + 1
			}

			fn main() Int {
				mut count = 0
				bump(count)
				count
			}
		`)
		if got := runGoTargetParityJSON(t, program); got != "1" {
			t.Fatalf("got %s, want 1", got)
		}
	})

	t.Run("closure function type", func(t *testing.T) {
		program := lowerParitySource(t, `
			type MutIntFn = fn(mut Int)

			fn bump(mut count: Int) {
				count = count + 1
			}

			fn apply(f: MutIntFn, mut count: Int) {
				f(count)
			}

			fn main() Int {
				mut count = 0
				apply(bump, count)
				count
			}
		`)
		if got := runGoTargetParityJSON(t, program); got != "1" {
			t.Fatalf("got %s, want 1", got)
		}
	})
}

func TestGoTargetParityMutMethodClosureCapturesSelf(t *testing.T) {
	program := lowerParitySource(t, `
		use ard/io

		struct Box {
			value: Int,
		}

		impl Box {
			fn mut bump_with_closure() {
				let bump = fn() {
					self.value = self.value + 1
				}
				bump()
			}
		}

		fn main() Int {
			mut box = Box{value: 0}
			box.bump_with_closure()
			box.value
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != "1" {
		t.Fatalf("got %s, want 1", got)
	}
}

func TestGoTargetParityMethodClosureCapturesSelf(t *testing.T) {
	program := lowerParitySource(t, `
		struct Counter {
			base: Int,
		}

		impl Counter {
			fn make_adder() fn(Int) Int {
				fn(value: Int) Int {
					self.base + value
				}
			}
		}

		fn main() Int {
			let counter = Counter{base: 32}
			let add = counter.make_adder()
			add(10)
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != "42" {
		t.Fatalf("got %s, want 42", got)
	}
}

func TestGoTargetParityPrinting(t *testing.T) {
	got := runGoTargetSourceStdout(t, `
		use ard/io
		fn main() {
			io::print("Hello, World!")
		}
	`)
	if got != "Hello, World!\n" {
		t.Fatalf("stdout = %q, want %q", got, "Hello, World!\n")
	}
}

func TestGoTargetParityEscapeSequences(t *testing.T) {
	got := runGoTargetSourceStdout(t, `
		use ard/io
		fn main() {
			io::print("Line 1\nLine 2")
			io::print("Tab:\tText")
		}
	`)
	want := "Line 1\nLine 2\nTab:\tText\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestGoTargetParityDurationFunctions(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{name: "from seconds", input: `use ard/duration
fn main() Int { duration::from_seconds(20) }`},
		{name: "from minutes", input: `use ard/duration
fn main() Int { duration::from_minutes(5) }`},
		{name: "from hours", input: `use ard/duration
fn main() Int { duration::from_hours(2) }`},
	})
}

func TestGoTargetParityFS(t *testing.T) {
	root := filepath.Join(t.TempDir(), "go-target-fs-parity")
	dir := filepath.Join(root, "workspace")
	file := filepath.Join(dir, "sample.txt")
	copyFile := filepath.Join(dir, "copy.txt")
	renamedFile := filepath.Join(dir, "renamed.txt")
	runGoParityCasesSerial(t, []goParityCase{
		{
			name: "fs create dir exists and is dir",
			input: fmt.Sprintf(`
				use ard/fs
				fn main() Bool {
					fs::delete_dir(%q).expect("")
					fs::create_dir(%q).expect("")
					fs::exists(%q) and fs::is_dir(%q)
				}
			`, root, dir, dir, dir),
		},
		{
			name: "fs write append and read",
			input: fmt.Sprintf(`
				use ard/fs
				fn main() Str {
					fs::delete_dir(%q).expect("")
					fs::create_dir(%q).expect("")
					fs::create_file(%q).expect("")
					fs::write(%q, "hello").expect("")
					fs::append(%q, " world").expect("")
					fs::read(%q).expect("")
				}
			`, root, dir, file, file, file, file),
		},
		{
			name: "fs copy rename delete and is file",
			input: fmt.Sprintf(`
				use ard/fs
				fn main() Bool {
					fs::delete_dir(%q).expect("")
					fs::create_dir(%q).expect("")
					fs::write(%q, "hello!").expect("")
					fs::copy(%q, %q).expect("")
					fs::rename(%q, %q).expect("")
					let renamed_ok = fs::is_file(%q) and fs::read(%q).expect("") == "hello!"
					fs::delete(%q).expect("")
					fs::delete(%q).expect("")
					renamed_ok and not fs::exists(%q) and not fs::exists(%q)
				}
			`, root, dir, file, file, copyFile, copyFile, renamedFile, renamedFile, renamedFile, file, renamedFile, file, renamedFile),
		},
		{
			name: "fs cwd abs and list dir",
			input: fmt.Sprintf(`
				use ard/fs
				fn main() Bool {
					fs::delete_dir(%q).expect("")
					fs::create_dir(%q).expect("")
					fs::write(%q, "a").expect("")
					fs::write(%q, "b").expect("")
					let entries = fs::list_dir(%q).expect("")
					let cwd = fs::cwd().expect("")
					let abs = fs::abs(%q).expect("")
					entries.size() == 2 and cwd.size() > 0 and abs.size() >= %d
				}
			`, root, dir, file, copyFile, dir, dir, len(dir)),
		},
		{
			name: "fs delete dir removes tree",
			input: fmt.Sprintf(`
				use ard/fs
				fn main() Bool {
					fs::delete_dir(%q).expect("")
					fs::create_dir(%q).expect("")
					fs::write(%q, "bye").expect("")
					fs::delete_dir(%q).expect("")
					not fs::exists(%q)
				}
			`, root, dir, file, dir, dir),
		},
	})
}

func TestGoTargetParityCryptoUUID(t *testing.T) {
	program := lowerParitySource(t, `
		use ard/crypto
		fn main() Str { crypto::uuid() }
	`)
	got := runGoTargetParityJSON(t, program)
	uuid, err := strconv.Unquote(got)
	if err != nil {
		t.Fatalf("unquote uuid %q: %v", got, err)
	}
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(uuid) {
		t.Fatalf("uuid = %q", uuid)
	}
}

func TestGoTargetParitySQL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "query.db")
	runGoParityCasesSerial(t, []goParityCase{
		{
			name: "sql extract params in query order",
			input: `
				use ard/sql
				fn main() Int {
					let names = sql::extract_params("SELECT * FROM players WHERE league_id = @league_id AND season = @season AND (home_id = @team_id OR away_id = @team_id)")
					names.size()
				}
			`,
		},
		{
			name: "sql query all decodes row fields",
			input: fmt.Sprintf(`
				use ard/sql
				use ard/decode
				fn main() Str {
					let db = sql::open(%q).expect("Failed to open database")
					db.exec("DROP TABLE IF EXISTS players").expect("Failed to drop table")
					db.exec("CREATE TABLE players (id INTEGER PRIMARY KEY, name TEXT, number INTEGER)").expect("Failed to create table")
					db.exec("INSERT INTO players (name, number) VALUES ('John Doe', 2)").expect("Failed to insert player 1")
					db.exec("INSERT INTO players (name, number) VALUES ('Jane Smith', 5)").expect("Failed to insert player 2")
					let query = db.query("SELECT id, name, number FROM players WHERE number = @number")
					let rows = query.all(["number": 5]).expect("Failed to query players")
					let decode_name = decode::field("name", decode::string)
					decode_name(rows.at(0)).expect("Unable to decode row")
				}
			`, dbPath),
		},
		{
			name: "sql query first returns maybe row",
			input: fmt.Sprintf(`
				use ard/sql
				use ard/decode
				fn main() Int {
					let db = sql::open(%q).expect("Failed to open database")
					let query = db.query("SELECT id FROM players WHERE number = @number")
					let maybe_row = query.first(["number": 5]).expect("Failed to query players")
					let row = maybe_row.expect("Found none")
					decode::run(row, decode::field("id", decode::int)).expect("Failed to decode id")
				}
			`, dbPath),
		},
		{
			name: "sql missing parameter reports error",
			input: fmt.Sprintf(`
				use ard/sql
				fn main() Str {
					let db = sql::open(%q).expect("Failed to open database")
					let stmt = db.query("INSERT INTO players (name, number) VALUES (@name, @number)")
					match stmt.run(["name": "John Doe", "int": 2]) {
						err(msg) => msg,
						ok(_) => "unexpected success",
					}
				}
			`, dbPath),
		},
		{
			name: "sql rollback discards inserted rows",
			input: fmt.Sprintf(`
				use ard/sql
				fn main() Int {
					let db = sql::open(%q).expect("Failed to open database")
					db.exec("DROP TABLE IF EXISTS users").expect("Failed to drop table")
					db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)").expect("Failed to create table")
					let tx = db.begin().expect("Failed to begin transaction")
					tx.exec("INSERT INTO users (id, name) VALUES (1, 'joe')").expect("Failed to insert")
					tx.rollback().expect("Failed to rollback transaction")
					let rows = db.query("SELECT * FROM users").all([:]).expect("Failed to get all")
					rows.size()
				}
			`, filepath.Join(filepath.Dir(dbPath), "rollback.db")),
		},
		{
			name: "sql inserting null round trips as nullable field",
			input: fmt.Sprintf(`
				use ard/sql
				use ard/decode
				fn main() Bool {
					let db = sql::open(%q).expect("Failed to open database")
					db.exec("DROP TABLE IF EXISTS users").expect("Failed to drop table")
					let create_table = db.query("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)")
					create_table.run([:]).expect("Failed to create table")
					let stmt = db.query("INSERT INTO users (name, email) VALUES (@name, @email)")
					let values: [Str:sql::Value] = ["name": "John Doe", "email": ()]
					stmt.run(values).expect("Failed to insert row")
					let query = db.query("SELECT email FROM users WHERE id = 1")
					let rows = query.all(values).expect("Failed to find users with id 1")
					let email = decode::run(rows.at(0), decode::field("email", decode::nullable(decode::string))).expect("Failed to decode email")
					email.is_none()
				}
			`, filepath.Join(filepath.Dir(dbPath), "maybe.db")),
		},
		{
			name: "sql commit persists inserted rows and query params work in tx",
			input: fmt.Sprintf(`
				use ard/sql
				use ard/decode
				fn main() Str {
					let db = sql::open(%q).expect("Failed to open database")
					db.exec("DROP TABLE IF EXISTS users").expect("Failed to drop table")
					db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)").expect("Failed to create table")
					let tx = db.begin().expect("Failed to begin transaction")
					tx.exec("INSERT INTO users (name, age) VALUES ('User1', 25)").expect("Failed to insert User1")
					tx.exec("INSERT INTO users (name, age) VALUES ('User2', 30)").expect("Failed to insert User2")
					let rows = tx.query("SELECT * FROM users WHERE age = @age").all(["age": 30]).expect("Failed to query in transaction")
					tx.commit().expect("Failed to commit transaction")
					decode::run(rows.at(0), decode::field("name", decode::string)).expect("Failed to decode name")
				}
			`, filepath.Join(filepath.Dir(dbPath), "commit.db")),
		},
	})
}

func TestGoTargetParityEnvGet(t *testing.T) {
	t.Setenv("ARD_ENV_TEST", "present")
	runGoParityCases(t, []goParityCase{
		{
			name: "env get returns some string for set variable",
			input: `
				use ard/env
				fn main() Str {
					env::get("ARD_ENV_TEST").or("")
				}
			`,
		},
		{
			name: "env get returns none for missing variable",
			input: `
				use ard/env
				fn main() Bool {
					env::get("ARD_MISSING_ENV_TEST").is_none()
				}
			`,
		},
	})
}

func TestGoTargetParityDecodeHostFlows(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{
			name: "dynamic list decodes back to list",
			input: `
				use ard/decode
				fn main() Int {
					let foo = [1,2,3]
					let data = Dynamic::list(from: foo, of: Dynamic::from_int)
					let list = decode::run(data, decode::list(decode::int)).expect("Couldn't decode data")
					list.at(1)
				}
			`,
		},
		{
			name: "dynamic object decodes back to map",
			input: `
				use ard/decode
				fn main() Int {
					let data = Dynamic::object([
						"foo": Dynamic::from_int(0),
						"baz": Dynamic::from_int(1),
					])
					let m = decode::run(data, decode::map(decode::string, decode::int)).expect("Couldn't decode data")
					m.get("foo").or(-1)
				}
			`,
		},
		{
			name: "from json accepts string input",
			input: `
				use ard/decode
				fn main() Int {
					let data = decode::from_json("42").expect("parse")
					decode::run(data, decode::int).expect("decode")
				}
			`,
		},
		{
			name: "from json accepts dynamic string input",
			input: `
				use ard/decode
				fn main() Int {
					let raw = Dynamic::from("\{\"count\": 7\}")
					let data = decode::from_json(raw).expect("parse")
					decode::run(data, decode::field("count", decode::int)).expect("decode")
				}
			`,
		},
		{
			name: "from json rejects non string dynamic input",
			input: `
				use ard/decode
				fn main() Str {
					let raw = Dynamic::from(42)
					match decode::from_json(raw) {
						err(msg) => msg,
						ok(_) => "unexpected success",
					}
				}
			`,
		},
		{
			name: "decode field string",
			input: `
				use ard/decode
				fn main() Str {
					let data = decode::from_json("\{\"name\": \"John Doe\"\}").expect("parse")
					decode::run(data, decode::field("name", decode::string)).expect("decode")
				}
			`,
		},
		{
			name: "path with array index",
			input: `
				use ard/decode
				fn main() Int {
					let data = decode::from_json("\{\"items\": [10, 20, 30]\}").expect("parse")
					let result = decode::run(data, decode::path(["items", 1], decode::int))
					result.expect("decode")
				}
			`,
		},
		{
			name: "string decoder fails on int returns error list",
			input: `
				use ard/decode
				fn main() Str {
					let data = Dynamic::from(42)
					let result = decode::run(data, decode::string)
					match result {
						err(errs) => {
							if not errs.size() == 1 { panic("Expected 1 error. Got {errs.size()}") }
							errs.at(0).to_str()
						},
						ok(_) => ""
					}
				}
			`,
		},
		{
			name: "nullable string on null returns none",
			input: `
				use ard/decode
				fn main() Bool {
					let data = decode::from_json("null").expect("parse")
					let result = decode::run(data, decode::nullable(decode::string)).expect("decode")
					result.is_none()
				}
			`,
		},
		{
			name: "can decode a list",
			input: `
				use ard/decode
				fn main() Int {
					let data = decode::from_json("[1, 2, 3, 4, 5]").expect("parse")
					let list = decode::run(data, decode::list(decode::int)).expect("decode")
					list.at(4)
				}
			`,
		},
		{
			name: "map of string keys to integers",
			input: `
				use ard/decode
				fn main() Int {
					let data = decode::from_json("\{\"age\": 30, \"score\": 95\}").expect("parse")
					let decode_map = decode::map(decode::string, decode::int)
					let m = decode_map(data).expect("decode")
					m.get("age").or(0)
				}
			`,
		},
		{
			name: "decode nested path with mixed segments",
			input: `
				use ard/decode
				fn main() Str {
					let data = decode::from_json("\{\"response\": [\{\"name\": \"Alice\"\}, \{\"name\": \"Bob\"\}]\}").expect("parse")
					decode::run(data, decode::path(["response", 0, "name"], decode::string)).expect("decode")
				}
			`,
		},
		{
			name: "path error includes array index",
			input: `
				use ard/decode
				fn main() Str {
					let data = decode::from_json("\{\"items\": [\"a\", \"b\"]\}").expect("parse")
					let result = decode::run(data, decode::path(["items", 1], decode::int))
					match result {
						ok => "unexpected success",
						err(errs) => errs.at(0).to_str(),
					}
				}
			`,
		},
		{
			name: "one of falls back to alternate decoder",
			input: `
				use ard/decode
				fn int_to_string(data: Dynamic) Str![decode::Error] {
					let int = try decode::int(data)
					Result::ok(int.to_str())
				}
				fn main() Str {
					let data = decode::from_json("20").expect("parse")
					let take_string = decode::one_of(decode::string, [int_to_string])
					take_string(data).expect("decode")
				}
			`,
		},
		{
			name: "flatten multiple errors with newlines",
			input: `
				use ard/decode
				fn main() Str {
					let errors = [
						decode::Error{expected: "Int", found: "false", path: ["[1]"]},
						decode::Error{expected: "Str", found: "42", path: ["[2]"]},
					]
					decode::flatten(errors)
				}
			`,
		},
	})
}

// JSON encoding of primitives is exercised through ard/json. The legacy
// ard/encode module and its Encodable trait were removed (ADR 0031).

func TestGoTargetParityStringHelpers(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{name: "int to str", input: `fn main() Str { 100.to_str() }`},
		{name: "bool to str", input: `fn main() Str { true.to_str() }`},
		{name: "str replace all", input: `fn main() Str { "hello world hello world".replace_all("world", "universe") }`},
		{name: "str contains", input: `fn main() Bool { "hello".contains("ell") }`},
		{name: "str starts with", input: `fn main() Bool { "hello".starts_with("he") }`},
		{name: "str ends with", input: `fn main() Bool { "hello".ends_with("lo") }`},
		{name: "str at returns character", input: `fn main() Str { "hello".at(1).expect("missing").to_str() }`},
		{name: "str at uses rune index", input: `fn main() Str { "hé".at(1).expect("missing").to_str() }`},
		{name: "str at out of bounds returns none", input: `fn main() Bool { "hello".at(5).is_none() }`},
		{name: "str at negative returns none", input: `fn main() Bool { "hello".at(-1).is_none() }`},
		{name: "str split", input: `fn main() [Str] { Str::split("a,b,c", ",") }`},
		{name: "str trim", input: `fn main() Str { "  hello \n".trim() }`},
		{name: "str is empty", input: `fn main() Bool { "".is_empty() }`},
	})
}

func TestGoTargetParityStringMatching(t *testing.T) {
	runGoParityCases(t, []goParityCase{
		{name: "str match first case", input: `fn main() Str {
  match "md" {
    "md" => "markdown",
    "html" => "html",
    _ => "unknown",
  }
}`},
		{name: "str match later case", input: `fn main() Str {
  match "html" {
    "md" => "markdown",
    "html" => "html",
    _ => "unknown",
  }
}`},
		{name: "str match fallback", input: `fn main() Str {
  match "txt" {
    "md" => "markdown",
    "html" => "html",
    _ => "unknown",
  }
}`},
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

func TestGoTargetParityDirectGoReturnAdapters(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "value error to result",
			input: `use go:strconv
fn main() Int { strconv::Atoi("42").expect("parse") }`,
			want: "42",
		},
		{
			name: "error to void result",
			input: `use go:os
fn main() Bool { os::Chdir(".").is_ok() }`,
			want: "true",
		},
		{
			name: "value bool to maybe",
			input: `use go:os
fn main() Bool { os::LookupEnv("__ARD_MISSING_DIRECT_GO_ADAPTER__").is_none() }`,
			want: "true",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := lowerParitySource(t, tc.input)
			if got := strings.TrimSpace(runGoTargetParityJSON(t, program)); got != tc.want {
				t.Fatalf("go output = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestGoTargetParityDirectGoFuncCallbacks(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "predicate callback finds matching rune",
			input: `use go:strings
fn main() Int { strings::IndexFunc("ab3c", fn(r: Rune) Bool { r == '3' }) }`,
			want: "2",
		},
		{
			name: "predicate callback returns -1 when none match",
			input: `use go:strings
fn main() Int { strings::IndexFunc("abc", fn(r: Rune) Bool { r == '9' }) }`,
			want: "-1",
		},
		{
			name: "mapping callback transforms runes",
			input: `use go:strings
fn main() Bool { strings::Map(fn(r: Rune) Rune { r }, "hi") == "hi" }`,
			want: "true",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := lowerParitySource(t, tc.input)
			if got := strings.TrimSpace(runGoTargetParityJSON(t, program)); got != tc.want {
				t.Fatalf("go output = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestGoTargetParityJSON(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "encode list preserves order",
			input: `use ard/json
fn main() Bool { json::encode([1, 2, 3]).expect("e") == "[1,2,3]" }`,
			want: "true",
		},
		{
			name: "parse struct field",
			input: `use ard/json
struct Todo { id: Int, title: Str, note: Str? }
fn main() Bool { json::parse<Todo>("\{\"id\":1,\"title\":\"ship\"\}").expect("p").id == 1 }`,
			want: "true",
		},
		{
			name: "round trips a struct through encode and parse",
			input: `use ard/json
use ard/maybe
struct Todo { id: Int, title: Str, note: Str? }
fn main() Bool {
  let raw = json::encode(Todo{id: 7, title: "x", note: maybe::none()}).expect("e")
  json::parse<Todo>(raw).expect("p").id == 7
}`,
			want: "true",
		},
		{
			name: "parse reports an error for invalid json",
			input: `use ard/json
struct Todo { id: Int, title: Str, note: Str? }
fn main() Bool { json::parse<Todo>("not json").is_err() }`,
			want: "true",
		},
		{
			name: "parse a list of primitives",
			input: `use ard/json
fn main() Bool { json::parse<[Int]>("[1,2,3]").expect("p").size() == 3 }`,
			want: "true",
		},
		{
			name: "missing optional field decodes as none",
			input: `use ard/json
struct Todo { id: Int, title: Str, note: Str? }
fn main() Bool { json::parse<Todo>("\{\"id\":1,\"title\":\"x\"\}").expect("p").note.is_none() }`,
			want: "true",
		},
		{
			name: "present optional field decodes as some",
			input: `use ard/json
struct Todo { id: Int, title: Str, note: Str? }
fn main() Bool { json::parse<Todo>("\{\"id\":1,\"title\":\"x\",\"note\":\"hi\"\}").expect("p").note.is_some() }`,
			want: "true",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := lowerParitySource(t, tc.input)
			if got := strings.TrimSpace(runGoTargetParityJSON(t, program)); got != tc.want {
				t.Fatalf("go output = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestGoTargetParityChannel(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "buffered send and receive",
			input: `use ard/channel
fn main() Bool {
  let ch = channel::new<Int>(1)
  ch.send(42)
  ch.recv().expect("recv") == 42
}`,
			want: "true",
		},
		{
			name: "receive on closed empty channel returns none",
			input: `use ard/channel
fn main() Bool {
  let ch = channel::new<Int>(1)
  ch.close()
  ch.recv().is_none()
}`,
			want: "true",
		},
		{
			name: "buffered drain preserves order then closes",
			input: `use ard/channel
fn main() Bool {
  let ch = channel::new<Str>(2)
  ch.send("a")
  ch.send("b")
  ch.close()
  let a = ch.recv().expect("a")
  let b = ch.recv().expect("b")
  let drained = ch.recv().is_none()
  a == "a" and b == "b" and drained
}`,
			want: "true",
		},
		{
			name: "communicates across a goroutine over an unbuffered channel",
			input: `use ard/channel
use ard/async
fn main() Bool {
  let ch = channel::new<Int>(0)
  let fiber = async::start(fn() { ch.send(7) })
  let v = ch.recv().expect("recv")
  fiber.Wait()
  v == 7
}`,
			want: "true",
		},
		{
			name: "sends contextual optional values",
			input: `use ard/channel
use ard/maybe
fn main() Bool {
  let ch = channel::new<Int?>(2)
  ch.send(maybe::none())
  ch.send(maybe::some(5))
  ch.close()
  let first = ch.recv().expect("first")
  let second = ch.recv().expect("second")
  first.is_none() and second.expect("s") == 5
}`,
			want: "true",
		},
		{
			name: "generic function inferring the channel element type",
			input: `use ard/channel
fn first<$T>(ch: channel::Chan<$T>) $T? {
  ch.recv()
}
fn main() Bool {
  let ch = channel::new<Int>(1)
  ch.send(99)
  first(ch).expect("first") == 99
}`,
			want: "true",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := lowerParitySource(t, tc.input)
			if got := strings.TrimSpace(runGoTargetParityJSON(t, program)); got != tc.want {
				t.Fatalf("go output = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestGoTargetParityDirectionalChannels(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "sender and receiver views round-trip a value",
			input: `use ard/channel
fn main() Bool {
  let ch = channel::new<Int>(1)
  let tx = channel::sender(ch)
  let rx = channel::receiver(ch)
  tx.send(42)
  rx.recv().expect("v") == 42
}`,
			want: "true",
		},
		{
			name: "select receives on a receiver view",
			input: `use ard/channel
fn main() Bool {
  let ch = channel::new<Int>(1)
  let rx = channel::receiver(ch)
  ch.send(7)
  select {
    let v = rx.recv() => v.expect("v") == 7,
    _ => false,
  }
}`,
			want: "true",
		},
		{
			name: "sender can close the channel",
			input: `use ard/channel
fn main() Bool {
  let ch = channel::new<Int>(1)
  let tx = channel::sender(ch)
  tx.close()
  ch.recv().is_none()
}`,
			want: "true",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := lowerParitySource(t, tc.input)
			if got := strings.TrimSpace(runGoTargetParityJSON(t, program)); got != tc.want {
				t.Fatalf("go output = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestGoTargetParitySelect(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "receives from the ready channel and binds the value",
			input: `use ard/channel
fn main() Bool {
  let a = channel::new<Int>(1)
  a.send(42)
  let got = select {
    let v = a.recv() => v.expect("a"),
    _ => 0,
  }
  got == 42
}`,
			want: "true",
		},
		{
			name: "non-blocking send falls to default when the buffer is full",
			input: `use ard/channel
fn main() Bool {
  let full = channel::new<Int>(1)
  full.send(1)
  select {
    full.send(2) => false,
    _ => true,
  }
}`,
			want: "true",
		},
		{
			name: "closed channel receive binds none",
			input: `use ard/channel
fn main() Bool {
  let c = channel::new<Int>(1)
  c.close()
  select {
    let v = c.recv() => v.is_none(),
    _ => false,
  }
}`,
			want: "true",
		},
		{
			name: "default fires when nothing is ready",
			input: `use ard/channel
fn main() Bool {
  let empty = channel::new<Int>(1)
  select {
    let v = empty.recv() => v.is_some(),
    _ => true,
  }
}`,
			want: "true",
		},
		{
			name: "discard receive fires and runs its body",
			input: `use ard/channel
fn main() Bool {
  let b = channel::new<Int>(1)
  b.send(1)
  select {
    b.recv() => true,
    _ => false,
  }
}`,
			want: "true",
		},
		{
			name: "blocks on a single recv arm until a goroutine sends",
			input: `use ard/channel
use ard/async
fn main() Bool {
  let ch = channel::new<Int>(0)
  let f = async::start(fn() { ch.send(7) })
  let got = select {
    let v = ch.recv() => v.expect("v"),
  }
  f.Wait()
  got == 7
}`,
			want: "true",
		},
		{
			name: "lowers a contextual none send value",
			input: `use ard/channel
use ard/maybe
fn main() Bool {
  let ch = channel::new<Int?>(1)
  let sent = select {
    ch.send(maybe::none()) => true,
    _ => false,
  }
  sent and ch.recv().expect("v").is_none()
}`,
			want: "true",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := lowerParitySource(t, tc.input)
			if got := strings.TrimSpace(runGoTargetParityJSON(t, program)); got != tc.want {
				t.Fatalf("go output = %s, want %s", got, tc.want)
			}
		})
	}
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
			name: "maybe equality compares presence and value",
			input: `
				use ard/maybe
				fn main() Bool {
					maybe::some(1) == maybe::some(1) and not maybe::some(1) == maybe::some(2)
				}
			`,
		},
		{
			name: "maybe equality distinguishes none from some",
			input: `
				use ard/maybe
				fn main() Bool {
					let none: Int? = maybe::none()
					not maybe::some(0) == none and none == maybe::none()
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
			name: "void local can be passed as value",
			input: `
				fn take(value: Void) Int { 1 }
				fn main() Int {
					let value = ()
					take(value)
				}
			`,
		},
		{
			name: "result ok preserves void side effects",
			input: `
				struct Counter { value: Int }
				impl Counter {
					fn mut bump() { self.value = self.value + 1 }
				}
				fn main() Int {
					mut counter = Counter{value: 0}
					let result: Void!Str = Result::ok(counter.bump())
					counter.value
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
			name: "maybe map supports void callback result",
			input: `
				use ard/maybe
				fn main() Bool {
					let mapped = maybe::some(1).map(fn(value) { () })
					mapped.is_some()
				}
			`,
		},
		{
			name: "maybe or supports void values",
			input: `
				use ard/maybe
				fn main() Bool {
					let value: Void? = maybe::none()
					let fallback = value.or(())
					true
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
			name: "result err preserves void side effects",
			input: `
				struct Counter { value: Int }
				impl Counter {
					fn mut bump() { self.value = self.value + 1 }
				}
				fn main() Int {
					mut counter = Counter{value: 0}
					let result: Int!Void = Result::err(counter.bump())
					counter.value
				}
			`,
		},
		{
			name: "result map supports void callback result",
			input: `
				fn main() Bool {
					let res: Int!Str = Result::ok(21)
					let mapped = res.map(fn(value) { () })
					mapped.is_ok()
				}
			`,
		},
		{
			name: "result or supports void values",
			input: `
				fn main() Bool {
					let res: Void!Str = Result::err("bad")
					let fallback = res.or(())
					true
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
			name: "result map err supports void callback result",
			input: `
				fn main() Bool {
					let res: Int!Str = Result::err("bad")
					let mapped = res.map_err(fn(err) { () })
					mapped.is_err()
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
	runGoParityCasesWithParallel(t, cases, true)
}

func runGoParityCasesSerial(t *testing.T, cases []goParityCase) {
	t.Helper()
	runGoParityCasesWithParallel(t, cases, false)
}

func runGoParityCasesWithParallel(t *testing.T, cases []goParityCase, parallel bool) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			program := lowerParitySource(t, tc.input)
			gotGo := runGoTargetParityJSON(t, program)
			if gotGo == "" {
				t.Fatalf("go target returned empty JSON output")
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

func runGoTargetParityJSON(t *testing.T, program *air.Program) string {
	t.Helper()
	tempDir := t.TempDir()
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate sources: %v", err)
	}
	trimmedSources := make(map[string][]byte, len(sources))
	for name, source := range sources {
		// The synthetic main package is replaced by the parity runner below.
		if name == "main.go" {
			continue
		}
		trimmedSources[name] = source
	}
	writeGeneratedSourcesForTest(t, tempDir, trimmedSources)
	rootID, err := rootFunction(program)
	if err != nil {
		t.Fatalf("root function: %v", err)
	}
	entryModuleID := program.Functions[rootID].Module
	entryAlias := modulePackageName(program, entryModuleID)
	entryImportPath := moduleImportPath(program, entryModuleID)
	scriptFn := entryAlias + "." + functionName(program, program.Functions[rootID])
	runner := fmt.Sprintf(`package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	%s %q
)

func main() {
	encoded, err := json.Marshal(normalizeParityValue(%s()))
	if err != nil {
		panic(err)
	}
	fmt.Print(string(encoded))
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
`, entryAlias, entryImportPath, scriptFn)
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

func runGoTargetSourceStdout(t *testing.T, input string) string {
	t.Helper()
	program := lowerParitySource(t, input)
	tempDir := t.TempDir()
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate sources: %v", err)
	}
	writeGeneratedSourcesForTest(t, tempDir, sources)
	goMod := "module generated\n\ngo 1.26.0\n"
	if moduleRoot, ok := compilerModuleRoot(); ok {
		goMod += "\nrequire github.com/akonwi/ard v0.0.0\n"
		goMod += fmt.Sprintf("replace github.com/akonwi/ard => %s\n", moduleRoot)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	binaryPath := filepath.Join(tempDir, "stdout-bin")
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
