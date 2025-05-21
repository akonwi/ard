package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestResults(t *testing.T) {
	run(t, []test{
		{
			name: "creating results",
			input: `
				fn divide(a: Int, b: Int) Result<Int, Str> {
				  match b == 0 {
						true => Result::err("not possible"),
						false => Result::ok(a/b),
					}
				}
			`,
			output: &checker.Program{},
		},
	})
}
