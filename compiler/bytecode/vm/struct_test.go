package vm

import "testing"

func TestBytecodeStructs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "Struct usage",
			input: `
				struct Point {
					x: Int,
					y: Int,
				}

				impl Point {
					fn print() Str {
						"{self.x.to_str()},{self.y.to_str()}"
					}
				}

				let p = Point { x: 10, y: 20 }
				p.print()
			`,
			want: "10,20",
		},
		{
			name: "Reassigning struct properties",
			input: `
				struct Point {
					x: Int,
					y: Int,
				}
				mut p = Point { x: 10, y: 20 }
				p.x = 30
				p.x
			`,
			want: 30,
		},
		{
			name: "Nesting structs",
			input: `
				struct Point {
					x: Int,
					y: Int,
				}
				struct Line {
					start: Point,
					end: Point,
				}
				let line = Line{
					start: Point { x: 10, y: 20 },
					end: Point { x: 10, y: 0 },
				}
				line.start.x + line.end.y
			`,
			want: 10,
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

func TestBytecodeStaticFunctions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "Struct usage",
			input: `
				struct Point {
					x: Int,
					y: Int,
				}
				fn Point::make(x: Int, y: Int) Point {
					Point { x: x, y: y }
				}
				let p = Point::make(10, 20)
				p.x
			`,
			want: 10,
		},
		{
			name: "deeply nested",
			input: `
				use ard/http
				let res = http::Response::new(200, "ok")
				res.status
			`,
			want: 200,
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
