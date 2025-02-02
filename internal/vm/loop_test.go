package vm_test

import "testing"

func TestForLoop(t *testing.T) {
	runTests(t, []test{
		{
			name: "Basic for loop",
			input: `
				use ard/io
				// sum even numbers
				mut sum = 0
				for mut even = 0; even <= 10; even =+ 2 {
					io.print(even.to_str())
				  sum =+ even
				}
				sum`,
			want: 30,
		},
	})
}

func TestForInLoops(t *testing.T) {
	runTests(t, []test{
		{
			name: "loop over numeric range",
			input: `
					mut sum = 0
					for i in 1..5 {
						sum = sum + i
					}
					sum`,
			want: 15,
		},
		{
			name: "loop over a number",
			input: `
				mut sum = 0
				for i in 5 {
					sum = sum + i
				}
				sum`,
			want: 15,
		},
		{
			name: "looping over a string",
			input: `
				mut res = ""
				for c in "hello" {
					res = "{{c}}{{res}}"
				}
				res`,
			want: "olleh",
		},
		{
			name: "looping over a list",
			input: `
				mut sum = 0
				for n in [1,2,3,4,5] {
					sum = sum + n
				}
				sum
			`,
			want: 15,
		},
	})
}

func TestWhileLoops(t *testing.T) {
	input := `
		mut count = 5
		while count > 0 {
			count = count - 1
		}
		count == 0`

	if res := run(t, input); res != true {
		t.Errorf("Expected true but got %v", res)
	}
}

func TestBreakStatement(t *testing.T) {
	runTests(t, []test{
		{
			name: "break out of loop",
			input: `
			  mut count = 5
				while count > 0 {
				  count = count - 1
					if count == 3 {
					  break
					}
				}
				count`,
			want: 3,
		},
	})
}
