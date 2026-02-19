package formatter

import "testing"

func TestFormat(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name:   "empty source",
			input:  "",
			output: "",
		},
		{
			name:   "normalizes windows line endings",
			input:  "let x = 1\r\nlet y = 2\r\n",
			output: "let x = 1\nlet y = 2\n",
		},
		{
			name:   "trims trailing spaces and tabs",
			input:  "let x = 1  \nlet y = 2\t\n",
			output: "let x = 1\nlet y = 2\n",
		},
		{
			name:   "adds trailing newline",
			input:  "let x = 1",
			output: "let x = 1\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format([]byte(tt.input))
			if string(got) != tt.output {
				t.Fatalf("expected %q, got %q", tt.output, string(got))
			}
		})
	}
}
