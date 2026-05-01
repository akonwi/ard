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
