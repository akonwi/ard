package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestContextualVoidClosureDiscardsUnannotatedReturnValue(t *testing.T) {
	run(t, []test{
		{
			name: "unannotated anonymous callback may produce value for Void callback",
			input: `fn each(callback: fn(Int)) {
  callback(1)
}

each(fn(value) {
  value + 1
})`,
		},
		{
			name: "explicit return annotation is not coerced to Void",
			input: `fn each(callback: fn(Int)) {
  callback(1)
}

each(fn(value) Int {
  value + 1
})`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected fn(Int), got fn(Int) Int"},
			},
		},
	})
}
