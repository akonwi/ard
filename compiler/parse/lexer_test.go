package parse

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

type lexTest struct {
	name  string
	input string
	want  []token
}

func runLexing(t *testing.T, tests []lexTest) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lexer := NewLexer([]byte(test.input))
			diff := cmp.Diff(test.want, lexer.Scan(), compareOptions)
			if diff != "" {
				t.Errorf("Tokens do not match (-want +got):\n%s", diff)
			}
		})
	}
}
