//go:build integration

package go_backend

import (
	"testing"
)

// TestBuildBinaryTryLoopAndMatchRuntimeParity asserts behavior-level runtime
// parity between the VM and Go targets for `try` usage inside loop bodies and
// match-arm statement flows. This upgrades prior compile-only success checks
// for these statement contexts into observable runtime/output assertions.
//
// Each subtest exercises a representative scenario and compares VM vs Go
// target stdout/stderr/exit code byte-for-byte to ensure semantic parity.
func TestBuildBinaryTryLoopAndMatchRuntimeParity(t *testing.T) {
	ardPath := requireIntegrationArdBinary(t)

	cases := []cliSnippetCase{
		{
			name: "try_stmt_in_for_loop_success",
			files: map[string]string{
				"main.ard": `
use ard/io

fn check(v: Int) Int!Str {
  match v < 0 {
    true => Result::err("negative: {v}"),
    false => Result::ok(v),
  }
}

fn run_loop(values: [Int]) Int!Str {
  for v in values {
    try check(v)
    io::print("ok: {v}")
  }
  Result::ok(values.size())
}

fn main() {
  match run_loop([1, 2, 3]) {
    ok(n) => io::print("count: {n}"),
    err(msg) => io::print(msg),
  }
}
`,
			},
		},
		{
			name: "try_stmt_in_for_loop_propagates_error",
			files: map[string]string{
				"main.ard": `
use ard/io

fn check(v: Int) Int!Str {
  match v < 0 {
    true => Result::err("negative: {v}"),
    false => Result::ok(v),
  }
}

fn run_loop(values: [Int]) Int!Str {
  for v in values {
    try check(v)
    io::print("ok: {v}")
  }
  Result::ok(values.size())
}

fn main() {
  match run_loop([1, -2, 3]) {
    ok(n) => io::print("count: {n}"),
    err(msg) => io::print(msg),
  }
}
`,
			},
		},
		{
			name: "try_value_in_for_loop_propagates",
			files: map[string]string{
				"main.ard": `
use ard/io

fn double(v: Int) Int!Str {
  match v < 0 {
    true => Result::err("negative: {v}"),
    false => Result::ok(v * 2),
  }
}

fn sum_doubled(values: [Int]) Int!Str {
  mut total = 0
  for v in values {
    let d = try double(v)
    total = total + d
  }
  Result::ok(total)
}

fn main() {
  match sum_doubled([1, 2, 3]) {
    ok(n) => io::print("total: {n}"),
    err(msg) => io::print(msg),
  }
  match sum_doubled([1, -2, 3]) {
    ok(n) => io::print("total: {n}"),
    err(msg) => io::print(msg),
  }
}
`,
			},
		},
		{
			name: "try_stmt_in_while_loop",
			files: map[string]string{
				"main.ard": `
use ard/io

fn check(v: Int) Int!Str {
  match v < 0 {
    true => Result::err("negative: {v}"),
    false => Result::ok(v),
  }
}

fn run_loop(values: [Int]) Int!Str {
  mut i = 0
  while i < values.size() {
    try check(values.at(i))
    io::print("ok: {values.at(i)}")
    i = i + 1
  }
  Result::ok(i)
}

fn main() {
  match run_loop([1, 2]) {
    ok(n) => io::print("count: {n}"),
    err(msg) => io::print(msg),
  }
  match run_loop([1, -2, 3]) {
    ok(n) => io::print("count: {n}"),
    err(msg) => io::print(msg),
  }
}
`,
			},
		},
		{
			name: "try_with_maybe_in_for_loop_propagates",
			files: map[string]string{
				"main.ard": `
use ard/io
use ard/maybe

fn half(v: Int) Int? {
  match v == 0 {
    true => maybe::none(),
    false => maybe::some(v / 2),
  }
}

fn sum_halves(values: [Int]) Int? {
  mut total = 0
  for v in values {
    let h = try half(v)
    total = total + h
  }
  maybe::some(total)
}

fn main() {
  match sum_halves([2, 4, 6]) {
    n => io::print("total: {n}"),
    _ => io::print("none"),
  }
  match sum_halves([2, 0, 6]) {
    n => io::print("total: {n}"),
    _ => io::print("none"),
  }
}
`,
			},
		},
		{
			name: "try_stmt_in_match_arm_block_propagates",
			files: map[string]string{
				"main.ard": `
use ard/io

fn check(v: Int) Int!Str {
  match v < 0 {
    true => Result::err("negative: {v}"),
    false => Result::ok(v),
  }
}

fn handle(flag: Bool, value: Int) Int!Str {
  match flag {
    true => {
      try check(value)
      io::print("checked: {value}")
      Result::ok(value + 1)
    },
    false => Result::ok(0),
  }
}

fn main() {
  match handle(true, 4) {
    ok(n) => io::print("ok: {n}"),
    err(msg) => io::print("err: {msg}"),
  }
  match handle(true, -5) {
    ok(n) => io::print("ok: {n}"),
    err(msg) => io::print("err: {msg}"),
  }
  match handle(false, 0) {
    ok(n) => io::print("ok: {n}"),
    err(msg) => io::print("err: {msg}"),
  }
}
`,
			},
		},
		{
			name: "try_value_in_match_arm_block_uses_unwrapped",
			files: map[string]string{
				"main.ard": `
use ard/io

fn double(v: Int) Int!Str {
  match v < 0 {
    true => Result::err("negative: {v}"),
    false => Result::ok(v * 2),
  }
}

fn handle(flag: Bool, value: Int) Int!Str {
  match flag {
    true => {
      let d = try double(value)
      Result::ok(d + 1)
    },
    false => Result::ok(0),
  }
}

fn main() {
  match handle(true, 5) {
    ok(n) => io::print("ok: {n}"),
    err(msg) => io::print("err: {msg}"),
  }
  match handle(true, -7) {
    ok(n) => io::print("ok: {n}"),
    err(msg) => io::print("err: {msg}"),
  }
  match handle(false, 0) {
    ok(n) => io::print("ok: {n}"),
    err(msg) => io::print("err: {msg}"),
  }
}
`,
			},
		},
		{
			name: "try_stmt_in_for_loop_inside_match_arm",
			files: map[string]string{
				"main.ard": `
use ard/io

fn check(v: Int) Int!Str {
  match v < 0 {
    true => Result::err("negative: {v}"),
    false => Result::ok(v),
  }
}

fn process(active: Bool, values: [Int]) Int!Str {
  match active {
    true => {
      for v in values {
        try check(v)
        io::print("ok: {v}")
      }
      Result::ok(values.size())
    },
    false => Result::ok(-1),
  }
}

fn main() {
  match process(true, [1, 2]) {
    ok(n) => io::print("count: {n}"),
    err(msg) => io::print("err: {msg}"),
  }
  match process(true, [1, -2, 3]) {
    ok(n) => io::print("count: {n}"),
    err(msg) => io::print("err: {msg}"),
  }
  match process(false, [1, 2, 3]) {
    ok(n) => io::print("count: {n}"),
    err(msg) => io::print("err: {msg}"),
  }
}
`,
			},
		},
		{
			name: "try_value_in_match_arm_inside_for_loop",
			files: map[string]string{
				"main.ard": `
use ard/io

fn double(v: Int) Int!Str {
  match v < 0 {
    true => Result::err("negative: {v}"),
    false => Result::ok(v * 2),
  }
}

fn run(values: [Int]) Int!Str {
  mut total = 0
  for v in values {
    let part = match v == 0 {
      true => 0,
      false => try double(v),
    }
    total = total + part
    io::print("partial: {total}")
  }
  Result::ok(total)
}

fn main() {
  match run([1, 0, 2]) {
    ok(n) => io::print("total: {n}"),
    err(msg) => io::print("err: {msg}"),
  }
  match run([1, -3, 2]) {
    ok(n) => io::print("total: {n}"),
    err(msg) => io::print("err: {msg}"),
  }
}
`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot := writeSnippetProject(t, tc.files)

			vmArgs := append([]string{"run", "main.ard"}, tc.args...)
			vmResult := runArdCLI(t, ardPath, projectRoot, tc.env, tc.stdin, vmArgs...)
			if vmResult.err != nil {
				t.Fatalf("vm snippet run failed: %s", formatCLIRunFailure(vmResult))
			}

			goArgs := append([]string{"run", "--target", "go", "main.ard"}, tc.args...)
			goResult := runArdCLI(t, ardPath, projectRoot, tc.env, tc.stdin, goArgs...)
			if goResult.err != nil {
				t.Fatalf("go snippet run failed: %s", formatCLIRunFailure(goResult))
			}

			if vmResult.exitCode != goResult.exitCode || vmResult.stdout != goResult.stdout || vmResult.stderr != goResult.stderr {
				t.Fatalf("try loop/match runtime parity mismatch\nvm: %s\ngo: %s", formatCLIRunFailure(vmResult), formatCLIRunFailure(goResult))
			}

			if vmResult.stdout == "" {
				t.Fatalf("expected non-empty observable stdout for runtime parity scenario %q; got empty\nvm: %s", tc.name, formatCLIRunFailure(vmResult))
			}
		})
	}
}
