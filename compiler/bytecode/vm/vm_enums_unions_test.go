package vm

import "testing"

func TestBytecodeVMParityEnumsUnions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "enum to int comparison",
			input: `
				enum Status { active, inactive, pending }
				let status = Status::active
				status == 0
			`,
			want: true,
		},
		{
			name: "enum explicit value",
			input: `
				enum HttpStatus {
					Ok = 200,
					Created = 201,
					Not_Found = 404
				}
				HttpStatus::Ok
			`,
			want: 200,
		},
		{
			name: "enum equality",
			input: `
				enum Direction { Up, Down, Left, Right }
				let dir1 = Direction::Up
				let dir2 = Direction::Down
				dir1 == dir2
			`,
			want: false,
		},
		{
			name: "union matching",
			input: `
				type Printable = Str | Int | Bool
				fn print(p: Printable) Str {
				  match p {
					  Str(str) => str,
						Int(int) => int.to_str(),
						_ => "boolean value"
					}
				}
				print(20)
			`,
			want: "20",
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
