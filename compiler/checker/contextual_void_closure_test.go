package checker_test

import "testing"

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
			name: "explicit value return is coerced to Void",
			input: `fn each(callback: fn(Int)) {
  callback(1)
}

each(fn(value) Int {
  value + 1
})`,
		},
	})
}
