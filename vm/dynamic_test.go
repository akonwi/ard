package vm_test

import "testing"

func TestDynamicList(t *testing.T) {
	run(t, `
		use ard/decode

		let foo = [1,2,3]
		let data = Dynamic::list(from: foo, of: Dynamic::from_int)

		let list = decode::run(data, decode::list(decode::int)).expect("Couldn't decode data")
		if not list.size() == 3 {
			panic("List size is not 3")
		}

		if not list.at(1) == 2 {
			panic("Element at index 1 is not 2")
		}
		`)
}
