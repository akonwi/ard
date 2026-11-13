package gotarget

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The argument of `or` and `expect` on Maybe and Result is evaluated only when
// the value is absent, and the callback of `map`, `and_then`, and `map_err` is
// evaluated on both paths (ADR 0079, #580). Every helper prints its label, so stdout shows
// which operands ran. An argument written with `later` lowers to setup
// statements (the call returns a Maybe, unpacked from Go's `(T, bool)`
// results), which must not run when the value is present either.

const lazyOrExpectHelpers = `
fn now(label: Str, value: Int) Int {
  fmt::Println(label)
  value
}

fn now_str(label: Str, value: Str) Str {
  fmt::Println(label)
  value
}

fn later(label: Str, value: Int) Int? {
  fmt::Println(label)
  Maybe::new(value)
}

fn later_str(label: Str, value: Str) Str? {
  fmt::Println(label)
  Maybe::new(value)
}

fn some(label: Str, value: Int) Int? {
  fmt::Println(label)
  Maybe::new(value)
}

fn none(label: Str) Int? {
  fmt::Println(label)
  Maybe::new()
}

fn ok(label: Str, value: Int) Int!Str {
  fmt::Println(label)
  Result::ok(value)
}

fn err(label: Str, message: Str) Int!Str {
  fmt::Println(label)
  Result::err(message)
}

fn note(label: Str) {
  fmt::Println(label)
}
`

func TestGoTargetOrEvaluatesDefaultOnlyWhenAbsent(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
`+lazyOrExpectHelpers+`
fn maybe_or_try(value: Int?, label: Str) Int!Str {
  Result::ok(value.or(try ok(label, 7)))
}

fn maybe_or_failed_try(value: Int?, label: Str) Int!Str {
  Result::ok(value.or(try err(label, "default failed")))
}

fn result_or_try(value: Int!Str, label: Str) Int!Str {
  Result::ok(value.or(try ok(label, 7)))
}

fn main() {
  let present = Maybe::new(1)
  let absent: Int? = Maybe::new()
  let good: Int!Str = Result::ok(1)
  let bad: Int!Str = Result::err("bad")

  fmt::Println("-- maybe or")
  let a = present.or(now("maybe or plain, present", 0))
  let b = absent.or(now("maybe or plain, absent", 2))
  let c = present.or(later("maybe or setup, present", 0).or(0))
  let d = absent.or(later("maybe or setup, absent", 3).or(0))
  let e = absent.or(none("maybe or nested, absent").or(now("maybe or nested default", 4)))
  let f = some("maybe or target, present", 5).or(later("maybe or target setup, present", 0).or(0))
  let g = none("maybe or target, absent").or(later("maybe or target setup, absent", 6).or(0))
  fmt::Println(a, b, c, d, e, f, g)

  fmt::Println("-- maybe or with try")
  fmt::Println(maybe_or_try(present, "maybe or try, present").or(0))
  fmt::Println(maybe_or_try(absent, "maybe or try, absent").or(0))
  fmt::Println(maybe_or_failed_try(present, "maybe or failed try, present").is_ok())
  fmt::Println(maybe_or_failed_try(absent, "maybe or failed try, absent").is_err())

  fmt::Println("-- result or")
  let h = good.or(now("result or plain, ok", 0))
  let i = bad.or(now("result or plain, err", 2))
  let j = good.or(later("result or setup, ok", 0).or(0))
  let k = bad.or(later("result or setup, err", 3).or(0))
  let l = bad.or(err("result or nested, err", "again").or(now("result or nested default", 4)))
  let m = ok("result or target, ok", 5).or(later("result or target setup, ok", 0).or(0))
  let n = err("result or target, err", "bad").or(later("result or target setup, err", 6).or(0))
  fmt::Println(h, i, j, k, l, m, n)

  fmt::Println("-- result or with try")
  fmt::Println(result_or_try(good, "result or try, ok").or(0))
  fmt::Println(result_or_try(bad, "result or try, err").or(0))

  fmt::Println("-- void or")
  let present_void: Void? = Maybe::new(())
  let absent_void: Void? = Maybe::new()
  present_void.or(note("maybe void or, present"))
  absent_void.or(note("maybe void or, absent"))
  let good_void: Void!Str = Result::ok(())
  let bad_void: Void!Str = Result::err("bad")
  good_void.or(note("result void or, ok"))
  bad_void.or(note("result void or, err"))
}
`)
	want := `-- maybe or
maybe or plain, absent
maybe or setup, absent
maybe or nested, absent
maybe or nested default
maybe or target, present
maybe or target, absent
maybe or target setup, absent
1 2 1 3 4 5 6
-- maybe or with try
1
maybe or try, absent
7
true
maybe or failed try, absent
true
-- result or
result or plain, err
result or setup, err
result or nested, err
result or nested default
result or target, ok
result or target, err
result or target setup, err
1 2 1 3 4 5 6
-- result or with try
1
result or try, err
7
-- void or
maybe void or, absent
result void or, err
`
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestGoTargetExpectEvaluatesMessageOnlyWhenAbsent(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
`+lazyOrExpectHelpers+`
fn maybe_expect_try(value: Int?, label: Str) Int!Str {
  Result::ok(value.expect(try ok(label, 7).map(fn(n) { "{n}" })))
}

fn result_expect_try(value: Int!Str, label: Str) Int!Str {
  Result::ok(value.expect(try ok(label, 7).map(fn(n) { "{n}" })))
}

fn main() {
  let present = Maybe::new(1)
  let good: Int!Str = Result::ok(1)
  let a = present.expect(now_str("maybe expect plain, present", "missing"))
  let b = present.expect(later_str("maybe expect setup, present", "missing").or(""))
  let c = some("maybe expect target, present", 2).expect(later_str("maybe expect target setup, present", "missing").or(""))
  let d = maybe_expect_try(present, "maybe expect try, present").or(0)
  let e = good.expect(now_str("result expect plain, ok", "failed"))
  let f = good.expect(later_str("result expect setup, ok", "failed").or(""))
  let g = ok("result expect target, ok", 2).expect(later_str("result expect target setup, ok", "failed").or(""))
  let h = result_expect_try(good, "result expect try, ok").or(0)
  let present_void: Void? = Maybe::new(())
  present_void.expect(later_str("maybe void expect, present", "missing").or(""))
  let good_void: Void!Str = Result::ok(())
  good_void.expect(later_str("result void expect, ok", "failed").or(""))
  fmt::Println(a, b, c, d, e, f, g, h)
}
`)
	want := `maybe expect target, present
result expect target, ok
1 1 2 1 1 1 2 1
`
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestGoTargetExpectPanicsWithMessageWhenAbsent(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStdout string
		wantPanic  string
	}{
		{
			name:       "maybe plain message",
			body:       `let absent: Int? = Maybe::new()` + "\n" + `  absent.expect(now_str("message", "maybe missing"))`,
			wantStdout: "message\n",
			wantPanic:  "panic: maybe missing",
		},
		{
			name:       "maybe message with setup and target with setup",
			body:       `none("target").expect(later_str("message", "maybe missing").or(""))`,
			wantStdout: "target\nmessage\n",
			wantPanic:  "panic: maybe missing",
		},
		{
			name:       "maybe void message with setup",
			body:       `let absent: Void? = Maybe::new()` + "\n" + `  absent.expect(later_str("message", "void missing").or(""))`,
			wantStdout: "message\n",
			wantPanic:  "panic: void missing",
		},
		{
			name:       "result plain message",
			body:       `let bad: Int!Str = Result::err("bad")` + "\n" + `  bad.expect(now_str("message", "result failed"))`,
			wantStdout: "message\n",
			wantPanic:  "panic: result failed: bad",
		},
		{
			name:       "result message with setup and target with setup",
			body:       `err("target", "bad").expect(later_str("message", "result failed").or(""))`,
			wantStdout: "target\nmessage\n",
			wantPanic:  "panic: result failed: bad",
		},
		{
			name:       "result void message with setup",
			body:       `let bad: Void!Str = Result::err("bad")` + "\n" + `  bad.expect(later_str("message", "void failed").or(""))`,
			wantStdout: "message\n",
			wantPanic:  "panic: void failed: bad",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := runGoTargetSourceOutput(t, `use go:fmt
`+lazyOrExpectHelpers+`
fn main() {
  `+tt.body+`
  fmt::Println("unreachable")
}
`)
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("run error = %v, want a non-zero exit\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
			}
			if stdout != tt.wantStdout {
				t.Fatalf("stdout = %q, want %q", stdout, tt.wantStdout)
			}
			if !strings.Contains(stderr, tt.wantPanic+"\n") {
				t.Fatalf("stderr = %q, want it to contain %q", stderr, tt.wantPanic)
			}
		})
	}
}

// runGoTargetSourceOutput builds input like runGoTargetSourceStdout but
// returns the run's stdout, stderr, and error instead of failing on a
// non-zero exit, so a test can assert on a panic.
func runGoTargetSourceOutput(t *testing.T, input string) (string, string, error) {
	t.Helper()
	program := lowerParitySource(t, input)
	tempDir := t.TempDir()
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate sources: %v", err)
	}
	writeGeneratedSourcesForTest(t, tempDir, sources)
	if err := writeGeneratedRuntimePackage(tempDir); err != nil {
		t.Fatalf("write generated runtime: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module generated\n\ngo 1.27.0\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	binaryPath := filepath.Join(tempDir, "output-bin")
	if err := buildGeneratedProgram(tempDir, binaryPath, goBuildOptions{}); err != nil {
		t.Fatalf("build generated program: %v", err)
	}
	cmd := exec.Command(binaryPath)
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

// The callback of `map`, `and_then`, and `map_err` is an ordinary argument:
// the expression that produces it is evaluated once, after the target and
// before the call, whether or not the callback is then called (ADR 0079).
func TestGoTargetCallbackArgumentsAreEvaluatedEagerly(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt
`+lazyOrExpectHelpers+`
fn inc(label: Str) fn(Int) Int {
  fmt::Println(label)
  fn(x: Int) Int { x + 1 }
}

fn inc_later(label: Str) (fn(Int) Int)? {
  fmt::Println(label)
  Maybe::new(fn(x: Int) Int { x + 1 })
}

fn keep(label: Str) fn(Int) Int? {
  fmt::Println(label)
  fn(x: Int) Int? { Maybe::new(x) }
}

fn keep_ok(label: Str) fn(Int) Int!Str {
  fmt::Println(label)
  fn(x: Int) Int!Str { Result::ok(x) }
}

fn shout(label: Str) fn(Str) Str {
  fmt::Println(label)
  fn(s: Str) Str { "{s}!" }
}

fn ident(label: Str) fn($T) $T {
  fmt::Println(label)
  fn(x: $T) $T { x }
}

fn main() {
  let absent: Int? = Maybe::new()
  let bad: Int!Str = Result::err("bad")
  let good: Int!Str = Result::ok(1)

  fmt::Println("-- maybe map")
  let a = some("maybe map target, present", 1).map(inc("maybe map plain, present"))
  let b = absent.map(inc("maybe map plain, absent"))
  let c = none("maybe map target, absent").map(inc_later("maybe map setup, absent").expect(""))
  let d = absent.map(ident<Int>("maybe map generic, absent"))
  fmt::Println(a.or(0), b.is_none(), c.is_none(), d.is_none())

  fmt::Println("-- maybe and_then")
  let e = some("maybe and_then target, present", 1).and_then(keep("maybe and_then plain, present"))
  let f = absent.and_then(keep("maybe and_then plain, absent"))
  fmt::Println(e.or(0), f.is_none())

  fmt::Println("-- result map")
  let g = good.map(inc("result map plain, ok"))
  let h = err("result map target, err", "bad").map(inc("result map plain, err"))
  let i = bad.map(inc_later("result map setup, err").expect(""))
  fmt::Println(g.or(0), h.is_err(), i.is_err())

  fmt::Println("-- result and_then")
  let j = bad.and_then(keep_ok("result and_then plain, err"))
  fmt::Println(j.is_err())

  fmt::Println("-- result map_err")
  let k = ok("result map_err target, ok", 1).map_err(shout("result map_err plain, ok"))
  let l = bad.map_err(shout("result map_err plain, err"))
  fmt::Println(k.or(0), l.is_err())
}
`)
	want := `-- maybe map
maybe map target, present
maybe map plain, present
maybe map plain, absent
maybe map target, absent
maybe map setup, absent
maybe map generic, absent
2 true true true
-- maybe and_then
maybe and_then target, present
maybe and_then plain, present
maybe and_then plain, absent
1 true
-- result map
result map plain, ok
result map target, err
result map plain, err
result map setup, err
2 true true
-- result and_then
result and_then plain, err
true
-- result map_err
result map_err target, ok
result map_err plain, ok
result map_err plain, err
1 true
`
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// A read in the target of `or`, `expect`, or `map` is evaluated before the
// argument, even when a call in the argument writes what the target reads
// (#575), and a callback read sees what a call in the target wrote.
func TestGoTargetOrExpectAndCallbackReadsFollowSourceOrder(t *testing.T) {
	got := runGoTargetSourceStdout(t, `use go:fmt

struct MaybeBox {
  value: Int?,
}

struct ResultBox {
  value: Int!Str,
}

struct Holder {
  f: fn(Int) Int,
}

fn fill(box: &mut MaybeBox, label: Str) Int {
  fmt::Println(label)
  box.value = Maybe::new(5)
  7
}

fn fill_later(box: &mut MaybeBox, label: Str) Int? {
  fmt::Println(label)
  box.value = Maybe::new(5)
  Maybe::new(7)
}

fn fill_message(box: &mut MaybeBox, label: Str) Str {
  fmt::Println(label)
  box.value = Maybe::new(5)
  "missing"
}

fn repair(box: &mut ResultBox, label: Str) Int {
  fmt::Println(label)
  box.value = Result::ok(5)
  7
}

fn adder(box: &mut MaybeBox, label: Str) fn(Int) Int {
  fmt::Println(label)
  box.value = Maybe::new(5)
  fn(n: Int) Int { n + 1 }
}

fn retarget(holder: &mut Holder) Int? {
  holder.f = fn(n: Int) Int { n + 100 }
  Maybe::new(1)
}

fn main() {
  let box = &mut MaybeBox{value: Maybe::new()}
  let a = box.value.or(fill(box, "maybe or, absent"))
  let b = box.value.or(fill(box, "maybe or, present"))
  box.value = Maybe::new()
  let c = box.value.or(fill_later(box, "maybe or setup, absent").or(0))
  box.value = Maybe::new(2)
  let d = box.value.expect(fill_message(box, "maybe expect, present"))
  fmt::Println(a, b, c, d)

  let rbox = &mut ResultBox{value: Result::err("bad")}
  let e = rbox.value.or(repair(rbox, "result or, err"))
  let f = rbox.value.or(repair(rbox, "result or, ok"))
  fmt::Println(e, f)

  box.value = Maybe::new()
  let g = box.value.map(adder(box, "map callback, absent"))
  fmt::Println(g.is_none())

  let holder = &mut Holder{f: fn(n: Int) Int { n + 1 }}
  let h = retarget(holder).map(holder.f)
  fmt::Println(h.or(0))
}
`)
	want := `maybe or, absent
maybe or setup, absent
7 5 7 2
result or, err
7 5
map callback, absent
true
101
`
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
