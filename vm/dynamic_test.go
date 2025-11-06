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

func TestDynamicObject(t *testing.T) {
	run(t, `
		use ard/decode

		let data = Dynamic::object([
			"foo": Dynamic::from_int(0),
			"baz": Dynamic::from_int(1),
		])

		let map = decode::run(data, decode::map(decode::string, decode::int)).expect("Couldn't decode data")
		let foo = map.get("foo")
		if not foo.or(-1) == 0 {
			panic("foo key should be 0")
		}
		`)
}
