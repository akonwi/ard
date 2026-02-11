package vm

import "testing"

func TestBytecodeVMParityTypeAPIs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{name: "Int.to_str", input: `100.to_str()`, want: "100"},
		{name: "Int::from_str", input: `Int::from_str("100")`, want: 100},
		{name: "Float::from_int", input: `Float::from_int(100)`, want: 100.0},
		{name: "Float.to_int", input: `5.9.to_int()`, want: 5},
		{name: "Bool.to_str", input: `true.to_str()`, want: "true"},
		{name: "Str.replace_all", input: `"hello world hello world".replace_all("world", "universe")`, want: "hello universe hello universe"},
		{name: "Map::new", input: `
			mut ages = Map::new<Int>()
			ages.set("Alice", 25)
			ages.get("Alice").or(0)
		`, want: 25},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}
