package vm_test

import "testing"

func TestHttpMethodExample(t *testing.T) {
	input := `
	use ard/http

	let method = http::Method::Post
	match method.to_str() == "POST" {
		true => true,
		false => {
			panic("Expected 'POST', got '{method.to_str()}'")
			false
		}
	}`

	run(t, input)
}
