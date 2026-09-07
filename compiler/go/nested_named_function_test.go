package gotarget

import "testing"

func TestGoTargetParityNestedNamedFunctionsAreLexicalClosures(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "direct call captures parameter",
			source: `
				fn outer(value: Int) Int {
					fn inner() Int { value }
					inner()
				}
				fn main() Int { outer(42) }
			`,
			want: "42",
		},
		{
			name: "function reference passed as argument",
			source: `
				fn apply(callback: fn() Int) Int { callback() }
				fn outer(value: Int) Int {
					fn inner() Int { value }
					apply(inner)
				}
				fn main() Int { outer(42) }
			`,
			want: "42",
		},
		{
			name: "escaped function reference retains capture",
			source: `
				fn outer(value: Int) fn() Int {
					fn inner() Int { value }
					inner
				}
				fn main() Int { outer(42)() }
			`,
			want: "42",
		},
		{
			name: "same named declarations keep distinct bodies",
			source: `
				fn first() Int {
					fn inner() Int { 1 }
					inner()
				}
				fn second() Int {
					fn inner() Int { 2 }
					inner()
				}
				fn main() Bool { first() == 1 and second() == 2 }
			`,
			want: "true",
		},
		{
			name: "same named declarations in sibling blocks keep distinct bodies",
			source: `
				fn main() Bool {
					let first = {
						fn inner() Int { 1 }
						inner()
					}
					let second = {
						fn inner() Int { 2 }
						inner()
					}
					first == 1 and second == 2
				}
			`,
			want: "true",
		},
		{
			name: "local declaration shadows top level",
			source: `
				fn inner() Int { 1 }
				fn outer() Int {
					fn inner() Int { 2 }
					inner()
				}
				fn main() Int { outer() }
			`,
			want: "2",
		},
		{
			name: "recursive closure captures parameter",
			source: `
				fn outer(base: Int) Int {
					fn add(value: Int) Int {
						match value == 0 {
							true => base,
							false => 1 + add(value - 1),
						}
					}
					add(2)
				}
				fn main() Int { outer(40) }
			`,
			want: "42",
		},
		{
			name: "recursive closure escapes",
			source: `
				fn outer(base: Int) fn(Int) Int {
					fn add(value: Int) Int {
						match value == 0 {
							true => base,
							false => 1 + add(value - 1),
						}
					}
					add
				}
				fn main() Int { outer(40)(2) }
			`,
			want: "42",
		},
		{
			name: "value capture snapshots at declaration",
			source: `
				fn main() Int {
					mut value = 1
					fn read() Int { value }
					value = 2
					read()
				}
			`,
			want: "1",
		},
		{
			name: "mutable capture uses binding slot",
			source: `
				fn main() Int {
					mut value = 1
					fn bump() { value = value + 1 }
					bump()
					value
				}
			`,
			want: "2",
		},
		{
			name: "existing reference capture retains pointee",
			source: `
				struct Box { value: Int }
				fn outer(box: mut Box) fn() {
					fn bump() { box.value = box.value + 1 }
					bump
				}
				fn main() Int {
					let box = mut Box{value: 1}
					outer(box)()
					box.value
				}
			`,
			want: "2",
		},
		{
			name: "declaration inside loop captures iteration environment",
			source: `
				fn main() Int {
					mut total = 0
					for value in [1, 2] {
						fn add() { total = total + value }
						add()
					}
					total
				}
			`,
			want: "3",
		},
		{
			name: "nested declaration captures transitively",
			source: `
				fn outer(value: Int) Int {
					fn first() Int {
						fn second() Int { value }
						second()
					}
					first()
				}
				fn main() Int { outer(42) }
			`,
			want: "42",
		},
		{
			name: "local closure inherits enclosing generic definition",
			source: `
				fn apply_value(value: $T) Int {
					fn local() Int {
						let _ = value
						42
					}
					local()
				}
				fn main() Bool { apply_value(1) == 42 and apply_value("one") == 42 }
			`,
			want: "true",
		},
		{
			name: "local closure inherits generic receiver definition",
			source: `
				struct Box { value: $T }
				impl Box {
					fn consume() Int {
						fn local() Int {
							let _ = self.value
							42
						}
						local()
					}
				}
				fn main() Bool {
					let int_box = Box<Int>{value: 1}
					let str_box = Box<Str>{value: "one"}
					int_box.consume() == 42 and str_box.consume() == 42
				}
			`,
			want: "true",
		},
		{
			name: "method local captures receiver",
			source: `
				struct Box { value: Int }
				impl Box {
					fn read() Int {
						fn inner() Int { self.value }
						inner()
					}
				}
				fn main() Int {
					let box = Box{value: 42}
					box.read()
				}
			`,
			want: "42",
		},
		{
			name: "final declaration remains a function value",
			source: `
				fn outer(value: Int) fn() Int {
					fn inner() Int { value }
				}
				fn main() Int { outer(42)() }
			`,
			want: "42",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := lowerParitySource(t, test.source)
			if got := runGoTargetParityJSON(t, program); got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}
		})
	}
}

func TestRunProgramNestedNamedFunctionAdditionalContexts(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "final declaration converted to Any",
			source: `
				fn outer(value: Int) Any {
					fn inner() Int { value }
				}
				fn main() {
					let _ = outer(42)
				}
			`,
		},
		{
			name: "top-level script block",
			source: `
				mut observed = 0
				if true {
					let local = 42
					fn inner() { observed = local }
					inner()
				}
				if observed != 42 { panic("bad script local function") }
			`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := lowerParitySource(t, test.source)
			if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
				t.Fatalf("RunProgram error = %v", err)
			}
		})
	}
}
