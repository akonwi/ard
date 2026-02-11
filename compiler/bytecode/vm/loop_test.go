package vm

import "testing"

func TestBytecodeForLoop(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "Basic for loop",
			input: `
				use ard/io
				mut sum = 0
				for mut even = 0; even <= 10; even =+ 2 {
					io::print(even.to_str())
				  sum =+ even
				}
				sum
			`,
			want: 30,
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

func TestBytecodeForInLoops(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "loop over numeric range",
			input: `
				mut sum = 0
				for i in 1..5 {
					sum = sum + i
				}
				sum
			`,
			want: 15,
		},
		{
			name: "loop over a number",
			input: `
				mut sum = 0
				for i in 5 {
					sum = sum + i
				}
				sum
			`,
			want: 15,
		},
		{
			name: "looping over a string",
			input: `
				mut res = ""
				for c in "hello" {
					res = "{c}{res}"
				}
				res
			`,
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
		{
			name: "looping over a map",
			input: `
				mut sum = 0
				for k,count in ["key":3, "foobar":6] {
					sum =+ count
				}
				sum
			`,
			want: 9,
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

func TestBytecodeWhileLoops(t *testing.T) {
	input := `
		mut count = 5
		while count > 0 {
			count = count - 1
		}
		count == 0
	`

	if res := runBytecode(t, input); res != true {
		t.Fatalf("Expected true but got %v", res)
	}
}

func TestBytecodeBreakStatement(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
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
				count
			`,
			want: 3,
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
