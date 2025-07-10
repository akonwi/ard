package vm_test

import "testing"

func TestDurationFunctions(t *testing.T) {
	runTests(t, []test{
		{
			name: "from_seconds",
			input: `
				use ard/duration
				duration::from_seconds(20)
			`,
			want: 20 * 1000,
		},
	})
}
