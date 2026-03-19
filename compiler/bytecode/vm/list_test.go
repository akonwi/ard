package vm

import "testing"

func TestBytecodeListApi(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "List::new",
			input: `mut nums = List::new<Int>()
			nums.push(1)
			nums.push(2)
			nums.size()`,
			want: 2,
		},
		{
			name:  "List.size",
			input: "[1,2,3].size()",
			want:  3,
		},
		{
			name: "List::prepend",
			input: `
				mut list = [1,2,3]
				list.prepend(4)
			  list.size()`,
			want: 4,
		},
		{
			name: "List::push",
			input: `
				mut list = [1,2,3]
				list.push(4)
			  list.size()`,
			want: 4,
		},
		{
			name: "List::at",
			input: `
				mut list = [1,2,3]
				list.push(4)
			  list.at(3)`,
			want: 4,
		},
		{
			name: "List::set updates the list at the specified index",
			input: `
				mut list = [1,2,3]
				list.set(1, 10)
				list.at(1)`,
			want: 10,
		},
		{
			name: "List.sort()",
			input: `
				mut list = [3,7,8,5,2,9,5,4]
				list.sort(fn(a: Int, b: Int) Bool { a < b })
				list.at(0) + list.at(7)
			`,
			want: 11,
		},
		{
			name: "List.swap swaps values at the given indexes",
			input: `
				mut list = [1,2,3]
				list.swap(0,2)
				list.at(0)`,
			want: 3,
		},
		{
			name: "List::keep infers parameter types in closures",
			input: `
				struct User { name: Str, age: Int }

				let users = [
					User{name: "Alice", age: 25},
					User{name: "Bob", age: 30},
					User{name: "Andrew", age: 35},
				]

				let adults = List::keep(users, fn(u) { u.age >= 30 })
				adults.size()
			`,
			want: 2,
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
