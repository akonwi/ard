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
		{
			name: "from_minutes",
			input: `
				use ard/duration
				duration::from_minutes(5)
			`,
			want: 5 * 60 * 1000,
		},
		{
			name: "from_hours",
			input: `
				use ard/duration
				duration::from_hours(2)
			`,
			want: 2 * 3600 * 1000,
		},
	})
}
