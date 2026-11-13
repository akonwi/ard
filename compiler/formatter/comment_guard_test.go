package formatter

import (
	"strings"
	"testing"
)

// Formatting must never lose or duplicate a comment. The guard compares the
// comments before and after, and refuses the output when they differ.
func TestCheckFormattedOutputKeepsComments(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		formatted string
		wantErr   string
	}{
		{
			name:      "comments kept and moved",
			input:     "use b // second\n// first\nuse a\n",
			formatted: "// first\nuse a\nuse b // second\n",
		},
		{
			name:      "comment marker normalized",
			input:     "let x = 1 //one\n",
			formatted: "let x = 1 // one\n",
		},
		{
			name:      "comment lost",
			input:     "let x = ( // why\n  1 + 2) * 3\n",
			formatted: "let x = (1 + 2) * 3\n",
			wantErr:   `formatting would lose the comment "// why"`,
		},
		{
			name:      "comment duplicated",
			input:     "let x = 1 // one\n",
			formatted: "// one\nlet x = 1 // one\n",
			wantErr:   `formatting would duplicate the comment "// one"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkFormattedOutputKeepsComments([]byte(test.input), []byte(test.formatted), "test.ard")
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, test.wantErr)
			}
		})
	}
}
