package gotarget

import "testing"

// #282: a try-catch early return inside a generic Result-returning function
// must convert the caught Result into the function's (T, error) tuple ABI
// (ADR 0038), not return the Result value directly.
func TestGoTargetGenericTryCatchResultABI(t *testing.T) {
	src := `fn walk(items: [Str], index: Int, decoder: fn(Str) $T!Str) $T!Str {
  match index >= items.size() {
    true => decoder("end"),
    false => {
      let name = items.at(index).expect("bounds")
      let next = try walk(items, index + 1, decoder) -> e {
        Result::err("{name}: {e}")
      }
      Result::ok(next)
    },
  }
}

fn main() Bool {
  let ok_result = walk(["a"], 0, fn(s: Str) Int!Str { Result::ok(s.size()) })
  let err_result = walk(["a"], 0, fn(s: Str) Int!Str { Result::err("boom") })
  let ok_val = ok_result.or(0 - 1)
  let err_msg = match err_result {
    ok(v) => "",
    err(e) => e,
  }
  ok_val == 3 and err_msg == "a: boom"
}`
	program := lowerParitySource(t, src)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}

// A try-catch over a Maybe inside a generic Result-returning function must
// likewise pack the caught value into the (T, error) tuple ABI. (#282)
func TestGoTargetGenericTryMaybeCatchResultABI(t *testing.T) {
	src := `fn first(items: [$T]) $T!Str {
  let head = try items.at(0) -> _ {
    Result::err("empty")
  }
  Result::ok(head)
}

fn main() Bool {
  let ok_result = first([10, 20])
  let empty: [Int] = []
  let err_result = first(empty)
  let err_msg = match err_result {
    ok(v) => "",
    err(e) => e,
  }
  ok_result.or(0) == 10 and err_msg == "empty"
}`
	program := lowerParitySource(t, src)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}
