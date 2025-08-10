package main

// go_add adds two integers
// Used by extern fn add(a: Int, b: Int) Int = "math.go_add"
func go_add(a, b int) int {
	return a + b
}

// go_multiply multiplies two integers
// Used by extern fn multiply(a: Int, b: Int) Int = "math.go_multiply"
func go_multiply(a, b int) int {
	return a * b
}

// go_max returns the maximum of two integers
// Used by extern fn max(a: Int, b: Int) Int = "math.go_max"
func go_max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
