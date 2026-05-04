package vm_next

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestRunEntryEvaluatesFunctionCallsAndArithmetic(t *testing.T) {
	got := runSource(t, `
		fn add(a: Int, b: Int) Int {
			a + b
		}

		add(20, 22)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesMainFunction(t *testing.T) {
	vm := newVMFromSource(t, `
		fn main() Int {
			42
		}
	`)
	got, err := vm.RunEntry()
	if err != nil {
		t.Fatalf("run entry: %v", err)
	}

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesLocalsAndAssignment(t *testing.T) {
	got := runSource(t, `
		mut count = 40
		count = count + 2
		count
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesStructLayoutAndFieldAccess(t *testing.T) {
	got := runSource(t, `
		struct User {
			name: Str,
			age: Int,
		}

		fn next_age(user: User) Int {
			user.age + 1
		}

		let user = User{name: "Ada", age: 41}
		next_age(user)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesStructInstanceMethod(t *testing.T) {
	got := runSource(t, `
		struct Todo {
			title: Str,
			completed: Bool,
		}

		impl Todo {
			fn label() Str {
				if self.completed {
					"[x] {self.title}"
				} else {
					"[ ] {self.title}"
				}
			}
		}

		let todo = Todo{ title: "Buy milk", completed: true }
		todo.label()
	`)

	if got.Kind != ValueStr || got.Str != "[x] Buy milk" {
		t.Fatalf("got %#v, want todo label", got)
	}
}

func TestRunEntryAssignsStructField(t *testing.T) {
	got := runSource(t, `
		struct Response {
			status: Int,
			body: Str,
		}

		mut response = Response{status: 200, body: ""}
		response.status = 201
		response.body = "created"
		response.status.to_str() + " " + response.body
	`)

	if got.Kind != ValueStr || got.Str != "201 created" {
		t.Fatalf("got %#v, want 201 created", got)
	}
}

func TestRunEntryEvaluatesModuleStructInstance(t *testing.T) {
	got := runSource(t, `
		use ard/http

		let req = http::Request{
			method: http::Method::Get,
			url: "https://example.com",
			headers: [:],
		}
		req.url
	`)

	if got.Kind != ValueStr || got.Str != "https://example.com" {
		t.Fatalf("got %#v, want request url", got)
	}
}

func TestRunEntryEvaluatesStringAndBoolExpressions(t *testing.T) {
	got := runSource(t, `
		let label = "ard" + "lang"
		(label == "ardlang") and (3 < 4)
	`)

	if got.Kind != ValueBool || !got.Bool {
		t.Fatalf("got %#v, want true", got)
	}
}

func TestRunEntryEvaluatesPrimitiveToStr(t *testing.T) {
	got := runSource(t, `
		40.to_str() + " " + true.to_str()
	`)

	if got.Kind != ValueStr || got.Str != "40 true" {
		t.Fatalf("got %#v, want 40 true", got)
	}
}

func TestRunEntryEvaluatesTemplateString(t *testing.T) {
	got := runSource(t, `
		let name = "Ada"
		let age = 42
		"{name} is {age}"
	`)

	if got.Kind != ValueStr || got.Str != "Ada is 42" {
		t.Fatalf("got %#v, want Ada is 42", got)
	}
}

func TestRunEntryEvaluatesStringMethods(t *testing.T) {
	got := runSource(t, `
		let parts = "  ard lang  ".trim().split(" ")
		parts.size() + parts.at(0).size() + parts.at(1).size()
	`)

	if got.Kind != ValueInt || got.Int != 9 {
		t.Fatalf("got %#v, want int 9", got)
	}
}

func TestRunEntryEvaluatesStdlibPrint(t *testing.T) {
	var printed []string
	got := runSourceWithExterns(t, `
		use ard/io

		io::print(42)
	`, HostFunctionRegistry{
		"Print": func(value string) {
			printed = append(printed, value)
		},
	})

	if got.Kind != ValueVoid {
		t.Fatalf("got %#v, want void", got)
	}
	if len(printed) != 1 || printed[0] != "42" {
		t.Fatalf("printed = %#v, want [42]", printed)
	}
}

func TestRunEntryEvaluatesFizzBuzzSampleShape(t *testing.T) {
	var printed []string
	vm := newVMFromSourceWithExterns(t, `
		use ard/io

		fn main() {
			for num in 1..10 {
				if num % 3 == 0 {
					io::print("Fizz")
				} else if num % 5 == 0 {
					io::print("Buzz")
				} else {
					io::print(num)
				}
			}
		}
	`, HostFunctionRegistry{
		"Print": func(value string) {
			printed = append(printed, value)
		},
	})
	got, err := vm.RunEntry()
	if err != nil {
		t.Fatalf("run entry: %v", err)
	}
	if got.Kind != ValueVoid {
		t.Fatalf("got %#v, want void", got)
	}

	want := []string{"1", "2", "Fizz", "4", "Buzz", "Fizz", "7", "8", "Fizz", "Buzz"}
	if strings.Join(printed, ",") != strings.Join(want, ",") {
		t.Fatalf("printed = %#v, want %#v", printed, want)
	}
}

func TestRunEntryEvaluatesScalarExtern(t *testing.T) {
	got := runSource(t, `
		extern fn floor(value: Float) Float = "FloatFloor"

		floor(42.9)
	`)

	if got.Kind != ValueFloat || got.Float != 42 {
		t.Fatalf("got %#v, want float 42", got)
	}
}

func TestRunEntryEvaluatesImportedModuleExtern(t *testing.T) {
	got := runSource(t, `
		use ard/float as Float

		Float::from_int(42)
	`)

	if got.Kind != ValueFloat || got.Float != 42 {
		t.Fatalf("got %#v, want float 42", got)
	}
}

func TestRunEntryEvaluatesMaybeExtern(t *testing.T) {
	got := runSource(t, `
		use ard/maybe

		extern fn parse_int(value: Str) Int? = "IntFromStr"

		match parse_int("42") {
			value => value,
			_ => 0,
		}
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesResultExtern(t *testing.T) {
	got := runSource(t, `
		use ard/maybe

		extern fn decode(input: Str, no_pad: Bool?) Str!Str = "Base64Decode"

		match decode("YQ", maybe::some(true)) {
			ok(value) => value,
			err(message) => message,
		}
	`)

	if got.Kind != ValueStr || got.Str != "a" {
		t.Fatalf("got %#v, want a", got)
	}
}

func TestRunEntryEvaluatesClosureCallWithCapture(t *testing.T) {
	got := runSource(t, `
		let offset = 2
		let add_offset = fn(value: Int) Int {
			value + offset
		}

		add_offset(40)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesMaybeMapClosure(t *testing.T) {
	got := runSource(t, `
		use ard/maybe

		let offset = 1
		let value = maybe::some(41)
		value.map(fn(v) { v + offset }).or(0)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesMaybeAndThenClosure(t *testing.T) {
	got := runSource(t, `
		use ard/maybe

		let value = maybe::some(40)
		value.and_then(fn(v) { maybe::some(v + 2) }).or(0)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesResultMapClosure(t *testing.T) {
	got := runSource(t, `
		let offset = 1
		let result: Int!Str = Result::ok(41)
		result.map(fn(v) { v + offset }).or(0)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesResultMapErrClosure(t *testing.T) {
	got := runSource(t, `
		let result: Int!Str = Result::err("boom")
		match result.map_err(fn(err) { err + "!" }) {
			ok(value) => "bad",
			err(message) => message,
		}
	`)

	if got.Kind != ValueStr || got.Str != "boom!" {
		t.Fatalf("got %#v, want boom!", got)
	}
}

func TestRunEntryEvaluatesResultAndThenClosure(t *testing.T) {
	got := runSource(t, `
		let result: Int!Str = Result::ok(40)
		result.and_then(fn(v) { Result::ok(v + 2) }).or(0)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesAsyncEvalFiberGet(t *testing.T) {
	got := runSource(t, `
		use ard/async

		let offset = 2
		let fiber = async::eval(fn() Int { 40 + offset })
		fiber.get()
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesAsyncStartFiberJoin(t *testing.T) {
	got := runSource(t, `
		use ard/async

		let fiber = async::start(fn() {})
		fiber.join()
		42
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesAsyncSleep(t *testing.T) {
	got := runSource(t, `
		use ard/async

		async::sleep(0)
		42
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesAsyncJoinFiberList(t *testing.T) {
	got := runSource(t, `
		use ard/async

		let a = async::eval(fn() Int { 40 })
		let b = async::eval(fn() Int { 2 })
		async::join([a, b])
		a.get() + b.get()
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryAllowsAsyncCaptureOfRangeCursor(t *testing.T) {
	got := runSource(t, `
		use ard/async

		mut fibers: [async::Fiber<Int>] = []
		for i in 0..3 {
			fibers.push(async::eval(fn() Int { i }))
		}

		async::join(fibers)

		mut total = 0
		for fiber in fibers {
			total =+ fiber.get()
		}
		total
	`)

	if got.Kind != ValueInt || got.Int != 6 {
		t.Fatalf("got %#v, want int 6", got)
	}
}

func TestRunEntryAllowsAsyncCaptureOfListCursorAndIndex(t *testing.T) {
	got := runSource(t, `
		use ard/async

		let values = [10, 20, 30]
		mut fibers: [async::Fiber<Int>] = []
		for value, index in values {
			fibers.push(async::eval(fn() Int { value + index }))
		}

		async::join(fibers)

		mut total = 0
		for fiber in fibers {
			total =+ fiber.get()
		}
		total
	`)

	if got.Kind != ValueInt || got.Int != 63 {
		t.Fatalf("got %#v, want int 63", got)
	}
}

func TestRunEntryEvaluatesGenericFunctionSpecialization(t *testing.T) {
	got := runSource(t, `
		fn id(value: $T) $T {
			value
		}

		id(40) + id(2)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesGenericFunctionTypedLocalCall(t *testing.T) {
	got := runSource(t, `
		fn apply(value: Int, f: fn(Int) $T) $T {
			f(value)
		}

		apply(40, fn(value) { value + 2 })
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesGenericFunctionReturningClosure(t *testing.T) {
	got := runSource(t, `
		fn constant(value: $T) fn(Int) $T {
			fn(input: Int) $T {
				value
			}
		}

		let answer = constant(42)
		answer(0)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesModuleFunctionReference(t *testing.T) {
	got := runSource(t, `
		use ard/testing

		let pass = testing::pass
		match pass() {
			ok => 42,
			err(message) => 0,
		}
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesIfExpressions(t *testing.T) {
	got := runSource(t, `
		fn choose(value: Int) Int {
			if value > 10 {
				value
			} else {
				10
			}
		}

		choose(42)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesBoolMatch(t *testing.T) {
	got := runSource(t, `
		fn assert(condition: Bool, message: Str) Void!Str {
			match condition {
				true => Result::ok(()),
				false => Result::err(message),
			}
		}

		fn check() Void!Str {
			try assert(true, "should pass")
			Result::ok(())
		}

		match check() {
			ok => "pass",
			err(message) => message,
		}
	`)

	if got.Kind != ValueStr || got.Str != "pass" {
		t.Fatalf("got %#v, want pass", got)
	}
}

func TestRunEntryEvaluatesIntMatch(t *testing.T) {
	got := runSource(t, `
		fn describe(value: Int) Str {
			match value {
				0 => "zero",
				1..9 => "small",
				_ => "large",
			}
		}

		describe(7)
	`)

	if got.Kind != ValueStr || got.Str != "small" {
		t.Fatalf("got %#v, want small", got)
	}
}

func TestRunEntryEvaluatesWhileAndBreak(t *testing.T) {
	got := runSource(t, `
		mut count = 5
		while count > 0 {
			count = count - 1
			if count == 3 {
				break
			}
		}
		count
	`)

	if got.Kind != ValueInt || got.Int != 3 {
		t.Fatalf("got %#v, want int 3", got)
	}
}

func TestRunEntryEvaluatesIntRangeLoop(t *testing.T) {
	got := runSource(t, `
		mut total = 0
		for num in 1..4 {
			total = total + num
		}
		total
	`)

	if got.Kind != ValueInt || got.Int != 10 {
		t.Fatalf("got %#v, want int 10", got)
	}
}

func TestRunEntryEvaluatesListMethodsAndIteration(t *testing.T) {
	got := runSource(t, `
		mut numbers: [Int] = []
		numbers.push(1)
		numbers.push(2)
		numbers.prepend(3)
		numbers.set(1, 4)

		mut total = numbers.size()
		for number in numbers {
			total = total + number
		}
		total + numbers.at(1)
	`)

	if got.Kind != ValueInt || got.Int != 16 {
		t.Fatalf("got %#v, want int 16", got)
	}
}

func TestRunEntryCopiesMutableListInitializers(t *testing.T) {
	got := runSource(t, `
		let original = [1]
		mut copied = original
		copied.push(2)
		original.size() + copied.size()
	`)

	if got.Kind != ValueInt || got.Int != 3 {
		t.Fatalf("got %#v, want int 3", got)
	}
}

func TestRunEntryEvaluatesImportedTestingAssert(t *testing.T) {
	got := runSource(t, `
		use ard/testing

		fn check() Void!Str {
			try testing::assert(true, "should pass")
			testing::pass()
		}

		match check() {
			ok => "pass",
			err(message) => message,
		}
	`)

	if got.Kind != ValueStr || got.Str != "pass" {
		t.Fatalf("got %#v, want pass", got)
	}
}

func TestRunEntryEvaluatesEnums(t *testing.T) {
	got := runSource(t, `
		enum Direction {
			Up, Down, Left, Right
		}

		fn name(direction: Direction) Str {
			match direction {
				Direction::Up => "North",
				Direction::Down => "South",
				Direction::Left => "West",
				Direction::Right => "East",
			}
		}

		let dir = Direction::Right
		let is_right = dir == Direction::Right
		if is_right {
			name(dir)
		} else {
			"bad"
		}
	`)

	if got.Kind != ValueStr || got.Str != "East" {
		t.Fatalf("got %#v, want East", got)
	}
}

func TestRunEntryEvaluatesEnumCatchAllWithCustomDiscriminants(t *testing.T) {
	got := runSource(t, `
		enum HttpStatus {
			Ok = 200,
			Created = 201,
			NotFound = 404,
			ServerError = 500,
		}

		let status = HttpStatus::NotFound
		match status {
			HttpStatus::Ok => "ok",
			HttpStatus::Created => "created",
			_ => "other",
		}
	`)

	if got.Kind != ValueStr || got.Str != "other" {
		t.Fatalf("got %#v, want other", got)
	}
}

func TestRunEntryEvaluatesUnionMatch(t *testing.T) {
	got := runSource(t, `
		type Printable = Int | Str | Bool
		let value: Printable = "ard"

		match value {
			Int(i) => "number",
			Str(s) => s,
			_ => "other",
		}
	`)

	if got.Kind != ValueStr || got.Str != "ard" {
		t.Fatalf("got %#v, want ard", got)
	}
}

func TestRunEntryWrapsUnionFunctionArguments(t *testing.T) {
	got := runSource(t, `
		type Printable = Int | Str

		fn render(value: Printable) Str {
			match value {
				Int(i) => "number",
				Str(s) => s,
			}
		}

		render("ard")
	`)

	if got.Kind != ValueStr || got.Str != "ard" {
		t.Fatalf("got %#v, want ard", got)
	}
}

func TestRunEntryWrapsUnionResultValues(t *testing.T) {
	got := runSource(t, `
		struct InvalidField {
			name: Str,
			message: Str,
		}

		type Error = InvalidField | Str

		fn validate() Bool!Error {
			Result::err(InvalidField{name: "age", message: "bad"})
		}

		match validate() {
			ok(value) => "ok",
			err(error) => match error {
				InvalidField(field) => field.name + ": " + field.message,
				Str(message) => message,
			},
		}
	`)

	if got.Kind != ValueStr || got.Str != "age: bad" {
		t.Fatalf("got %#v, want age: bad", got)
	}
}

func TestRunEntryEvaluatesTraitObjectDispatch(t *testing.T) {
	got := runSource(t, `
		trait Speaks {
			fn speak() Str
		}

		struct Dog {
			name: Str,
		}

		impl Speaks for Dog {
			fn speak() Str {
				self.name + " says hi"
			}
		}

		fn describe(speaker: Speaks) Str {
			speaker.speak()
		}

		describe(Dog{name: "Ada"})
	`)

	if got.Kind != ValueStr || got.Str != "Ada says hi" {
		t.Fatalf("got %#v, want Ada says hi", got)
	}
}

func TestRunEntryEvaluatesMaybes(t *testing.T) {
	got := runSource(t, `
		use ard/maybe

		fn pick(value: Int?) Int {
			match value {
				v => v,
				_ => 0,
			}
		}

		let some = maybe::some(41)
		let none: Int? = maybe::none()
		if some.is_some() and none.is_none() {
			pick(some) + none.or(1)
		} else {
			0
		}
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesMaybeEqualityAndExpect(t *testing.T) {
	got := runSource(t, `
		use ard/maybe

		let left = maybe::some("ard")
		let right = maybe::some("ard")
		let empty: Str? = maybe::none()
		if (left == right) and (not (left == empty)) {
			left.expect("missing")
		} else {
			"bad"
		}
	`)

	if got.Kind != ValueStr || got.Str != "ard" {
		t.Fatalf("got %#v, want ard", got)
	}
}

func TestRunEntryEvaluatesResults(t *testing.T) {
	got := runSource(t, `
		fn pick(result: Int!Str) Int {
			match result {
				ok(value) => value,
				err(msg) => 0,
			}
		}

		let ok: Int!Str = Result::ok(41)
		let err: Int!Str = Result::err("bad")
		if ok.is_ok() and err.is_err() {
			pick(ok) + err.or(1)
		} else {
			0
		}
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesResultExpect(t *testing.T) {
	got := runSource(t, `
		let result: Str!Str = Result::ok("ard")
		result.expect("missing")
	`)

	if got.Kind != ValueStr || got.Str != "ard" {
		t.Fatalf("got %#v, want ard", got)
	}
}

func TestRunEntryEvaluatesTryResult(t *testing.T) {
	got := runSource(t, `
		fn answer() Int!Str {
			Result::ok(42)
		}

		fn compute() Int!Str {
			let value = try answer()
			Result::ok(value)
		}

		compute().expect("missing")
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryPropagatesTryResultFailure(t *testing.T) {
	got := runSource(t, `
		fn fail() Int!Str {
			Result::err("boom")
		}

		fn compute() Int!Str {
			let value = try fail()
			Result::ok(value + 1)
		}

		compute().or(42)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryEvaluatesTryCatchAsEarlyReturn(t *testing.T) {
	got := runSource(t, `
		fn fail() Int!Str {
			Result::err("boom")
		}

		fn compute() Str {
			let value = try fail() -> err {
				"caught: " + err
			}
			"success"
		}

		compute()
	`)

	if got.Kind != ValueStr || got.Str != "caught: boom" {
		t.Fatalf("got %#v, want caught: boom", got)
	}
}

func TestRunEntryEvaluatesTryCatchFunctionHandler(t *testing.T) {
	got := runSource(t, `
		fn format(err: Str) Str {
			"caught: " + err
		}

		fn fail() Int!Str {
			Result::err("boom")
		}

		fn compute() Str {
			let value = try fail() -> format
			"success"
		}

		compute()
	`)

	if got.Kind != ValueStr || got.Str != "caught: boom" {
		t.Fatalf("got %#v, want caught: boom", got)
	}
}

func TestRunEntryPropagatesTryMaybeFailure(t *testing.T) {
	got := runSource(t, `
		use ard/maybe

		fn missing() Int? {
			maybe::none()
		}

		fn compute() Str? {
			let value = try missing()
			maybe::some("present")
		}

		match compute() {
			value => value,
			_ => "none",
		}
	`)

	if got.Kind != ValueStr || got.Str != "none" {
		t.Fatalf("got %#v, want none", got)
	}
}

func TestRunEntryPropagatesTryThroughMatchArm(t *testing.T) {
	got := runSource(t, `
		enum Status {
			Active, Inactive
		}

		fn fail() Int!Str {
			Result::err("boom")
		}

		fn compute(status: Status) Int!Str {
			match status {
				Status::Active => {
					let value = try fail()
					let result: Int!Str = Result::ok(value + 1)
					result
				}
				Status::Inactive => {
					let result: Int!Str = Result::ok(0)
					result
				}
			}
		}

		compute(Status::Active).or(42)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunEntryPropagatesTryThroughWhileLoop(t *testing.T) {
	got := runSource(t, `
		fn fail() Int!Str {
			Result::err("boom")
		}

		fn compute() Int!Str {
			mut index = 0
			while index < 4 {
				if index == 2 {
					let value = try fail()
					index = value
				} else {
					index = index + 1
				}
			}
			Result::ok(index)
		}

		compute().or(42)
	`)

	if got.Kind != ValueInt || got.Int != 42 {
		t.Fatalf("got %#v, want int 42", got)
	}
}

func TestRunTestsEvaluatesResultOutcomes(t *testing.T) {
	vm := newVMFromSource(t, `
		test fn passes() Void!Str {
			Result::ok(())
		}

		test fn fails() Void!Str {
			Result::err("boom")
		}
	`)

	outcomes := vm.RunTests()
	if len(outcomes) != 2 {
		t.Fatalf("outcome count = %d, want 2", len(outcomes))
	}
	if outcomes[0].Name != "passes" || outcomes[0].Status != TestPass || outcomes[0].Message != "" {
		t.Fatalf("first outcome = %#v, want pass", outcomes[0])
	}
	if outcomes[1].Name != "fails" || outcomes[1].Status != TestFail || outcomes[1].Message != "boom" {
		t.Fatalf("second outcome = %#v, want fail boom", outcomes[1])
	}
}

func runSource(t *testing.T, input string) Value {
	t.Helper()
	vm := newVMFromSourceWithExterns(t, input, nil)
	got, err := vm.RunScript()
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	return got
}

func runSourceWithExterns(t *testing.T, input string, externs HostFunctionRegistry) Value {
	t.Helper()
	vm := newVMFromSourceWithExterns(t, input, externs)
	got, err := vm.RunScript()
	if err != nil {
		t.Fatalf("run script: %v", err)
	}
	return got
}

func newVMFromSource(t *testing.T, input string) *VM {
	t.Helper()
	return newVMFromSourceWithExterns(t, input, nil)
}

func newVMFromSourceWithExterns(t *testing.T, input string, externs HostFunctionRegistry) *VM {
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
	vm, err := NewWithExterns(program, externs)
	if err != nil {
		t.Fatalf("new vm: %v", err)
	}
	return vm
}
