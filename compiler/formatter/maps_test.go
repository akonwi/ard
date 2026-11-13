package formatter

import "testing"

// TestFormatMapLiteralExpressionKeys pins that map keys that are full
// expressions round-trip without parentheses, and that a parenthesized key
// loses its now-redundant parentheses (#581).
func TestFormatMapLiteralExpressionKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "call, member access, and operator keys",
			input: "let m = [key():\"one\",user.id:\"two\",a+1:\"three\"]\n",
			want:  "let m = [key(): \"one\", user.id: \"two\", a + 1: \"three\"]\n",
		},
		{
			name:  "parenthesized keys drop their parentheses",
			input: "let m = [(key()): \"one\", (user.id): \"two\", (a + 1): \"three\"]\n",
			want:  "let m = [key(): \"one\", user.id: \"two\", a + 1: \"three\"]\n",
		},
		{
			name:  "static paths, struct literals, and unary keys",
			input: "let m = [Color::red: 1, Point{x: 1}: 2, -1: 3, not done: 4]\n",
			want:  "let m = [Color::red: 1, Point{x: 1}: 2, -1: 3, not done: 4]\n",
		},
		{
			name:  "multi-line map with comments",
			input: "let m = [\n  // first\n  key(): 1,\n  // second\n  user.id: 2,\n  a + 1: 3,\n]\n",
			want:  "let m = [\n  // first\n  key(): 1,\n  // second\n  user.id: 2,\n  a + 1: 3,\n]\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertDerefFormat(t, tt.input, tt.want)
		})
	}
}
