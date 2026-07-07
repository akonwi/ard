package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestBreakLoopContextValidation(t *testing.T) {
	run(t, []test{
		{
			name: "break in a loop is valid",
			input: `
				for i in 1..3 {
					break
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name:  "break outside a loop is rejected",
			input: `break`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "break can only be used inside a loop"},
			},
		},
		{
			name: "break in a match arm outside a loop is rejected",
			input: `
				match 1 {
					1 => {
						break
					},
					_ => {},
				}
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "break can only be used inside a loop"},
			},
		},
		{
			name: "break in a closure inside a loop is rejected",
			input: `
				fn run(cb: fn() Void) {}
				for i in 1..3 {
					run(fn() {
						break
					})
				}
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "break can only be used inside a loop"},
			},
		},
		{
			name: "break in a while loop is valid",
			input: `
				mut going = true
				while going {
					break
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "break in a c-style loop is valid",
			input: `
				for mut i = 0; i < 3; i =+ 1 {
					break
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "break in a for-in loop is valid",
			input: `
				for item in [1, 2, 3] {
					break
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}

func TestBreakInMatchArms(t *testing.T) {
	run(t, []test{
		{
			name: "inline break arm in a subject match inside a loop",
			input: `
				for i in 1..3 {
					match i {
						2 => break,
						_ => {},
					}
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "inline break arm in a conditional match inside a loop",
			input: `
				fn should_skip(n: Int) Bool { n == 2 }
				for i in 1..3 {
					match {
						should_skip(i) => break,
						_ => {},
					}
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "single-line block break arm inside a loop",
			input: `
				for i in 1..3 {
					match i {
						2 => { break },
						_ => {},
					}
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "break arm in a value-producing match is rejected",
			input: `
				for i in 1..3 {
					let label = match i {
						2 => break,
						_ => "keep",
					}
				}
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch in match branches: expected Str, got Void"},
			},
		},
	})
}

func TestBreakInUnsafeBlockReportsOneDiagnostic(t *testing.T) {
	run(t, []test{
		{
			name: "unsafe pre-scan owns the break diagnostic",
			input: `
				fn run() Int {
					for i in 1..3 {
						let x = try unsafe {
							break
							1
						} -> _ { 0 }
					}
					0
				}
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "break is not allowed inside unsafe blocks"},
			},
		},
	})
}
