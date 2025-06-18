package vm_test

import "testing"

func TestStructs(t *testing.T) {
	runTests(t, []test{
		{
			name: "Struct usage",
			input: `
				struct Point {
					x: Int,
					y: Int,
				}

				impl Point {
					fn print() Str {
						"{@x.to_str()},{@y.to_str()}"
					}
				}

				let p = Point { x: 10, y: 20 }
				p.print()`,
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
				p.x`,
			want: 30,
		},
	})
}

func TestStaticFunctions(t *testing.T) {
	runTests(t, []test{
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
				p.x`,
			want: 10,
		},
	})
}
