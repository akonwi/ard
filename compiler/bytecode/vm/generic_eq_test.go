package vm

import "testing"

func TestGenericValueEquality(t *testing.T) {
	t.Run("find result compared with ==", func(t *testing.T) {
		res := runBytecode(t, `
use ard/list
let found = list::find([1, 2, 3, 4, 5], fn(n) { n == 3 })
match found {
  value => value == 3,
  _ => false
}
`)
		if res != true {
			t.Fatalf("Expected true, got %v (%T)", res, res)
		}
	})

	t.Run("inline maybe wrapping in generic context", func(t *testing.T) {
		res := runBytecode(t, `
use ard/maybe
mut found = maybe::none<Int>()
let list = [1, 2, 3, 4, 5]
for t in list {
  if t == 3 {
    found = maybe::some(t)
    break
  }
}
match found {
  value => value == 3,
  _ => false
}
`)
		if res != true {
			t.Fatalf("Expected true, got %v (%T)", res, res)
		}
	})
}
