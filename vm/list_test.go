package vm_test

import "testing"

func TestListApi(t *testing.T) {
	runTests(t, []test{
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
				list.at(0) + list.at(7) // 2 + 9 = 11
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
			name: "List::concat a combined list",
			input: `
				let a = [1,2,3]
				let b = [4,5,6]
				let list = List::concat(a, b)
				list.at(3) == 4`,
			want: true,
		},
		{
			name: "List::keep with inferred function parameter type",
			input: `
				struct User { name: Str }

				let users = [
					User{name: "Alice"},
					User{name: "Bob"},
					User{name: "Andrew"},
					User{name: "Charlie"},
				]

				let a_people = List::keep(users, fn(u) { u.name.starts_with("A") })
				a_people.size()
			`,
			want: 2,
		},
		{
			name: "List::keep with inferred parameter accessing struct fields",
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
		{
			name: "List::drop removes elements before index",
			input: `
				let list = [1,2,3,4,5]
				let dropped = List::drop(list, 2)
				dropped.size()
			`,
			want: 3,
		},
		{
			name: "List::drop returns correct elements",
			input: `
				let list = [1,2,3,4,5]
				let dropped = List::drop(list, 2)
				dropped.at(0) == 3 && dropped.at(1) == 4 && dropped.at(2) == 5
			`,
			want: true,
		},
		{
			name: "List::drop with index 0 returns all elements",
			input: `
				let list = [1,2,3]
				let dropped = List::drop(list, 0)
				dropped.size()
			`,
			want: 3,
		},
		{
			name: "List::map transforms list",
			input: `
				let nums = [1, 2, 3]
				let doubled: [Int] = List::map(nums, fn(n: Int) Int { n * 2 })
				for n, i in doubled {
				  let expected = nums.at(i) * 2
					if not n == expected {
						panic("At {i}: Expected {expected} and got {n}")
					}
				}
			`,
			want: nil,
		},
		{
			name: "List::find returns Some when item matches",
			input: `
				let list = [1,2,3,4,5]
				let found = List::find(list, fn(n) { n == 3 })
				match found {
					val => val == 3,
					_ => false
				}
			`,
			want: true,
		},
		{
			name: "List::partition splits list correctly",
			input: `
				let list = [1,2,3,4,5]
				let parts = List::partition(list, fn(n) { n > 2 })
				parts.selected.size() == 3 and parts.others.size() == 2
			`,
			want: true,
		},
	})
}
