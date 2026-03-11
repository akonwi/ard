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

func TestBytecodeStructMaybeFieldImplicitWrapping(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "implicit wrapping of non-nullable value for nullable struct field",
			input: `
				struct Config {
					name: Str,
					timeout: Int?,
				}
				let c = Config{name: "app", timeout: 30}
				c.timeout.or(0)
			`,
			want: 30,
		},
		{
			name: "omitting nullable struct field still works",
			input: `
				struct Config {
					name: Str,
					timeout: Int?,
				}
				let c = Config{name: "app"}
				c.timeout.or(0)
			`,
			want: 0,
		},
		{
			name: "explicit maybe::some still works for struct fields",
			input: `
				use ard/maybe
				struct Config {
					name: Str,
					timeout: Int?,
				}
				let c = Config{name: "app", timeout: maybe::some(30)}
				c.timeout.or(0)
			`,
			want: 30,
		},
		{
			name: "implicit wrapping of list literal for nullable struct field",
			input: `
				struct Data {
					items: [Int]?,
				}
				let d = Data{items: [1, 2, 3]}
				match d.items {
					lst => lst.size()
					_ => 0
				}
			`,
			want: 3,
		},
		{
			name: "implicit wrapping of map literal for nullable struct field",
			input: `
				struct Data {
					meta: [Str:Int]?,
				}
				let d = Data{meta: ["count": 42]}
				match d.meta {
					m => true
					_ => false
				}
			`,
			want: true,
		},
		{
			name: "implicit wrapping with multiple nullable fields",
			input: `
				struct Options {
					a: Int?,
					b: Str?,
					c: Bool?,
				}
				let o = Options{a: 1, b: "hi", c: true}
				let av = o.a.or(0)
				let bv = o.b.or("")
				let cv = o.c.or(false)
				"{av},{bv},{cv}"
			`,
			want: "1,hi,true",
		},
		{
			name: "explicit maybe values still infer against nullable dynamic struct fields",
			input: `
				use ard/http
				use ard/maybe
				let req = http::Request{
					method: http::Method::Post,
					url: "https://example.com",
					headers: [:],
					body: maybe::some("payload"),
				}
				req.body.is_some()
			`,
			want: true,
		},
		{
			name: "maybe none still infers against nullable dynamic struct fields",
			input: `
				use ard/http
				use ard/maybe
				let req = http::Request{
					method: http::Method::Del,
					url: "https://example.com",
					headers: [:],
					body: maybe::none(),
				}
				req.body.is_none()
			`,
			want: true,
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
