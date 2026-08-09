package gotarget

import "testing"

func TestNativeTraitInterfaceCallPacksResultABI(t *testing.T) {
	program := lowerParitySource(t, `
trait Runner {
  fn run() Int!Str
}

struct Task {}

impl Runner for Task {
  fn run() Int!Str {
    Result::ok(1)
  }
}

fn execute(runner: Runner) Int!Str {
  runner.run()
}

fn execute_try(runner: Runner) Int!Str {
  let value = try runner.run()
  Result::ok(value + 1)
}

fn main() Int {
  let runner: Runner = Task{}
  let direct = runner.run().expect("direct")
  let delegated = execute(runner).expect("delegated")
  let propagated = execute_try(runner).expect("propagated")
  direct + delegated + propagated
}
`)
	if got := runGoTargetParityJSON(t, program); got != "4" {
		t.Fatalf("got %s, want 4", got)
	}
}

func TestNativeTraitInterfaceCallPacksABIResultVariants(t *testing.T) {
	program := lowerParitySource(t, `
trait Outcome {
  fn value(ok: Bool) Int!Str
  fn notify(ok: Bool) Void!Str
  fn lookup(ok: Bool) Int?
}

struct Service {}

impl Outcome for Service {
  fn value(ok: Bool) Int!Str {
    match ok {
      true => Result::ok(7),
      false => Result::err("value"),
    }
  }

  fn notify(ok: Bool) Void!Str {
    match ok {
      true => Result::ok(()),
      false => Result::err("notify"),
    }
  }

  fn lookup(ok: Bool) Int? {
    match ok {
      true => Maybe::new(9),
      false => Maybe::new(),
    }
  }
}

fn main() Bool {
  let outcome: Outcome = Service{}
  let value_ok = outcome.value(true).or(0) == 7
  let value_err = outcome.value(false).is_err()
  let void_ok = outcome.notify(true).is_ok()
  let void_err = outcome.notify(false).is_err()
  let maybe_some = outcome.lookup(true).or(0) == 9
  let maybe_none = outcome.lookup(false).is_none()
  value_ok and value_err and void_ok and void_err and maybe_some and maybe_none
}
`)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}
