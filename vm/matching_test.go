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
