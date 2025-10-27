package vm_test

import (
	"testing"
	"time"
)

func TestDurationFunctions(t *testing.T) {
	runTests(t, []test{
		{
			name: "from_seconds",
			input: `
				use ard/duration
				duration::from_seconds(20)
			`,
			want: int(20 * time.Second),
		},
		{
			name: "from_minutes",
			input: `
				use ard/duration
				duration::from_minutes(5)
			`,
			want: int(5 * time.Minute),
		},
		{
			name: "from_hours",
			input: `
				use ard/duration
				duration::from_hours(2)
			`,
			want: int(2 * time.Hour),
		},
	})
}
