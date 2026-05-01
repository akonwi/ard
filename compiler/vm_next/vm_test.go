package vm_next

import (
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

func TestRunEntryEvaluatesStringAndBoolExpressions(t *testing.T) {
	got := runSource(t, `
		let label = "ard" + "lang"
		(label == "ardlang") and (3 < 4)
	`)

	if got.Kind != ValueBool || !got.Bool {
		t.Fatalf("got %#v, want true", got)
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
	vm := newVMFromSource(t, input)
	got, err := vm.RunEntry()
	if err != nil {
		t.Fatalf("run entry: %v", err)
	}
	return got
}

func newVMFromSource(t *testing.T, input string) *VM {
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
	vm, err := New(program)
	if err != nil {
		t.Fatalf("new vm: %v", err)
	}
	return vm
}
