package formatter

import (
	"strings"
	"testing"
)

func TestFormatKeepsImportsUsedByRawInterpolation(t *testing.T) {
	input := "use go:fmt\n\nfn main() Str { `{fmt::Sprint(1)}` }\n"
	formatted, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(formatted), "use go:fmt") {
		t.Fatalf("format removed interpolation import:\n%s", formatted)
	}
}

func TestFormatPreservesRawStrings(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single line",
			input: "fn main() Str { `C:\\Users\\name \\\\n` }",
			want:  "fn main() Str {\n  `C:\\Users\\name \\\\n`\n}\n",
		},
		{
			name:  "single line literal braces stay doubled",
			input: "fn main() Str { `SELECT '{{name}}'` }",
			want:  "fn main() Str {\n  `SELECT '{{name}}'`\n}\n",
		},
		{
			name:  "quoted interpolation containing backtick remains an expression",
			input: "fn main() Str { `{\"`\"}` }",
			want:  "fn main() Str {\n  `{\"`\"}`\n}\n",
		},
		{
			name:  "quoted interpolation newline is escaped",
			input: "fn main() Str { `{\"a\nb\"}` }",
			want:  "fn main() Str {\n  `{\"a\\nb\"}`\n}\n",
		},
		{
			name: "multiline margin shifts with enclosing block",
			input: "fn main() Str {\n" +
				"      `\n" +
				"        SELECT id\n" +
				"          FROM users\n" +
				"        `\n" +
				"}\n",
			want: "fn main() Str {\n" +
				"  `\n" +
				"    SELECT id\n" +
				"      FROM users\n" +
				"    `\n" +
				"}\n",
		},
		{
			name: "multiline interpolation",
			input: "fn query(table: Str) Str {\n" +
				"  `\n" +
				"    SELECT *\n" +
				"    FROM {table}\n" +
				"    `\n" +
				"}\n",
			want: "fn query(table: Str) Str {\n" +
				"  `\n" +
				"    SELECT *\n" +
				"    FROM {table}\n" +
				"    `\n" +
				"}\n",
		},
		{
			name: "multiline call argument keeps suffix comma",
			input: "fn main() {\n" +
				"  execute(`\n" +
				"    SELECT 1\n" +
				"    `, true)\n" +
				"}\n",
			want: "fn main() {\n" +
				"  execute(\n" +
				"    `\n" +
				"      SELECT 1\n" +
				"      `,\n" +
				"    true,\n" +
				"  )\n" +
				"}\n",
		},
		{
			name: "semantic leading trailing and blank lines",
			input: "fn main() Str {\n" +
				"  `\n" +
				"\n" +
				"    value\n" +
				"\n" +
				"    `\n" +
				"}\n",
			want: "fn main() Str {\n" +
				"  `\n" +
				"    \n" +
				"    value\n" +
				"    \n" +
				"    `\n" +
				"}\n",
		},
		{
			name: "trailing content whitespace is preserved",
			input: "fn main() Str {\n" +
				"  `\n" +
				"    value  \n" +
				"    `\n" +
				"}\n",
			want: "fn main() Str {\n" +
				"  `\n" +
				"    value  \n" +
				"    `\n" +
				"}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted, err := Format([]byte(tt.input), "test.ard")
			if err != nil {
				t.Fatal(err)
			}
			if string(formatted) != tt.want {
				t.Fatalf("format mismatch:\ngot:\n%s\nwant:\n%s", formatted, tt.want)
			}
			again, err := Format(formatted, "test.ard")
			if err != nil {
				t.Fatal(err)
			}
			if string(again) != string(formatted) {
				t.Fatalf("format is not idempotent:\nfirst:\n%s\nsecond:\n%s", formatted, again)
			}
		})
	}
}
