package vm_test

import "testing"

func TestMatchingOnBooleans(t *testing.T) {
	runTests(t, []test{
		{
			name: "Matching on booleans",
			input: `
				let is_on = true
				match is_on {
					true => "on",
					false => "off"
				}`,
			want: "on",
		},
	})
}

func TestMatchingOnEnums(t *testing.T) {
	runTests(t, []test{
		{
			name: "Matching on enum",
			input: `
				enum Direction {
					Up, Down, Left, Right
				}
				let dir: Direction = Direction::Right
				match dir {
					Direction::Up => "North",
					Direction::Down => {
						"South"
					},
					Direction::Left => "West",
					Direction::Right => "East"
				}`,
			want: "East",
		},
		{
			name: "Catch all",
			input: `
				enum Direction {
					Up, Down, Left, Right
				}
				let dir: Direction = Direction::Right
				match dir {
					Direction::Up => "North",
					Direction::Down => "South",
					_ => "skip"
				}`,
			want: "skip",
		},
	})
}

func TestMatchingOnInt(t *testing.T) {
	runTests(t, []test{
		{
			name: "matching on an int",
			input: `
			  let int = 0
				match int {
				  -1 => "less",
				  0 => "equal",
					1 => "greater",
					_ => "panic"
				}
			`,
			want: "equal",
		},
		{
			name: "matching with ranges",
			input: `
			  let int = 80
				match int {
				  -100..0 => "how?",
				  0..60 => "F",
					_ => "pass"
				}
				`,
			want: "pass",
		},
	})
}

func TestConditionalMatch(t *testing.T) {
	runTests(t, []test{
		{
			name: "basic conditional matching with catch-all",
			input: `
				let score = 85
				match {
					score >= 90 => "A",
					score >= 80 => "B",
					score >= 70 => "C",
					score >= 60 => "D",
					_ => "F"
				}
			`,
			want: "B",
		},
		{
			name: "conditional matching with complex conditions",
			input: `
				let age = 25
				let hasLicense = true
				match {
					age < 16 => "Too young",
					not hasLicense => "Need license",
					age >= 65 => "Senior driver",
					_ => "Regular driver"
				}
			`,
			want: "Regular driver",
		},
		{
			name: "conditional matching with first condition true",
			input: `
				let x = 15
				match {
					x > 10 => "big",
					x > 5 => "medium",
					_ => "small"
				}
			`,
			want: "big",
		},
		{
			name: "conditional matching falls through to catch-all",
			input: `
				let x = 3
				match {
					x > 10 => "big",
					x > 5 => "medium",
					_ => "small"
				}
			`,
			want: "small",
		},
		{
			name: "conditional matching with integer return types",
			input: `
				let condition = true
				match {
					condition => 42,
					_ => 0
				}
			`,
			want: 42,
		},
		{
			name: "conditional matching with boolean conditions",
			input: `
				let a = true
				let b = false
				match {
					a and b => "both true",
					a or b => "at least one true",
					_ => "both false"
				}
			`,
			want: "at least one true",
		},
	})
}
