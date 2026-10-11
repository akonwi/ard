package gotarget

import "testing"

// Named Go slice and map types flow into Ard lists and maps and iterate with
// `for ... in`, mirroring Go's symmetric assignability and `range` (#511).
func TestRunProgramNamedGoCollectionsBehaveLikeArdCollections(t *testing.T) {
	program := lowerSource(t, `
		use go:net/http
		use go:sort

		fn total(values: [Int]) Int {
			mut sum = 0
			for value in values {
				sum = sum + value
			}
			sum
		}

		fn count(values: [Str: [Str]]) Int {
			values.size()
		}

		fn main() {
			mut nums: sort::IntSlice = [3, 1, 2]
			if total(nums) != 6 { panic("named slice as list: {total(nums)}") }

			mut indexed = 0
			for n, i in nums {
				indexed = indexed + n * (i + 1)
			}
			if indexed != 11 { panic("named slice iteration: {indexed}") }

			let sorted = &mut nums
			sorted.Sort()
			mut seen = ""
			for n in sorted {
				seen = seen + n.to_str()
			}
			if seen != "123" { panic("named slice reference iteration: {seen}") }

			let header: http::Header = ["Accept": ["a", "b"], "Host": ["h"]]
			if count(header) != 2 { panic("named map as map: {count(header)}") }
			mut values = 0
			for key, entries in header {
				values = values + key.size() + entries.size()
			}
			if values != 13 { panic("named map iteration: {values}") }
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
