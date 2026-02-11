package vm

import "testing"

func TestBytecodeMatchingOnBooleans(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "Matching on booleans",
			input: `
				let is_on = true
				match is_on {
					true => "on",
					false => "off"
				}
			`,
			want: "on",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBytecodeMatchingOnEnums(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
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
				}
			`,
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
				}
			`,
			want: "skip",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBytecodeMatchingOnInt(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
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
		{
			name: "matching on int with enum variant patterns",
			input: `
				enum Status {
					active,
					inactive,
					pending
				}
				let code: Int = 0
				match code {
					Status::active => "Active",
					Status::inactive => "Inactive",
					Status::pending => "Pending",
					_ => "Unknown"
				}
			`,
			want: "Active",
		},
		{
			name: "matching on int with mixed enum and literal patterns",
			input: `
				enum HttpStatus {
					ok,
					created,
					notFound,
					serverError
				}
				let code: Int = 2
				match code {
					HttpStatus::ok => "200 OK",
					HttpStatus::created => "201 Created",
					HttpStatus::notFound => "404 Not Found",
					500..599 => "Server Error",
					_ => "Unknown"
				}
			`,
			want: "404 Not Found",
		},
		{
			name: "matching on int with custom enum values",
			input: `
				enum HttpStatus {
					Ok = 200,
					Created = 201,
					NotFound = 404,
					ServerError = 500
				}
				let code: Int = 404
				match code {
					HttpStatus::Ok => "Success",
					HttpStatus::Created => "Created",
					HttpStatus::NotFound => "Not Found",
					HttpStatus::ServerError => "Server Error",
					_ => "Unknown"
				}
			`,
			want: "Not Found",
		},
		{
			name: "matching on int with mixed custom enum values and ranges",
			input: `
				enum Status {
					Pending = 0,
					Active = 100,
					Inactive = 101,
					Deleted = 999
				}
				let code: Int = 150
				match code {
					Status::Pending => "Pending",
					Status::Active => "Active",
					Status::Inactive => "Inactive",
					100..199 => "In range 100-199",
					Status::Deleted => "Deleted",
					_ => "Unknown"
				}
			`,
			want: "In range 100-199",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBytecodeConditionalMatch(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}
