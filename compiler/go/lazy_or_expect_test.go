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
// the value is absent (#580). Every helper prints its label, so stdout shows
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
