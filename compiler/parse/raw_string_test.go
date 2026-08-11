package parse

import (
	"strings"
	"testing"
)

func TestRawStringLiterals(t *testing.T) {
	tests := []struct {
		name   string
		source string
		value  string
		form   StringForm
	}{
		{
			name:   "empty",
			source: "``",
			value:  "",
			form:   StringFormRawSingleLine,
		},
		{
			name:   "backslashes quotes and escape spellings are literal",
			source: "`C:\\Users\\name\\n\\t\\u263a \"quoted\"`",
			value:  "C:\\Users\\name\\n\\t\\u263a \"quoted\"",
			form:   StringFormRawSingleLine,
		},
		{
			name:   "doubled braces",
			source: "`SELECT '{{value}}'`",
			value:  "SELECT '{value}'",
			form:   StringFormRawSingleLine,
		},
		{
			name: "multiline closing margin",
			source: "`\n" +
				"  SELECT id\n" +
				"    FROM users\n" +
				"  `",
			value: "SELECT id\n  FROM users",
			form:  StringFormRawMultiline,
		},
		{
			name: "multiline boundary newlines",
			source: "`\n" +
				"\n" +
				"  value\n" +
				"\n" +
				"  `",
			value: "\nvalue\n",
			form:  StringFormRawMultiline,
		},
		{
			name: "multiline blank whitespace is empty",
			source: "`\n" +
				"  first\n" +
				"     \n" +
				"  second\n" +
				"  `",
			value: "first\n\nsecond",
			form:  StringFormRawMultiline,
		},
		{
			name:   "CRLF normalizes to LF",
			source: "`\r\n  first\r\n  second\r\n  `",
			value:  "first\nsecond",
			form:   StringFormRawMultiline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.source), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %#v", result.Errors)
			}
			if len(result.Program.Statements) != 1 {
				t.Fatalf("statements = %d", len(result.Program.Statements))
			}
			literal, ok := result.Program.Statements[0].(*StrLiteral)
			if !ok {
				t.Fatalf("expression = %T, want *StrLiteral", result.Program.Statements[0])
			}
			if literal.Value != tt.value {
				t.Fatalf("value = %q, want %q", literal.Value, tt.value)
			}
			if literal.Form != tt.form {
				t.Fatalf("form = %v, want %v", literal.Form, tt.form)
			}
		})
	}
}

func TestRawInterpolatedStrings(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantChunks []string
		form       StringForm
	}{
		{
			name:       "single line",
			source:     "`Hello, {name}!`",
			wantChunks: []string{"literal:Hello, ", "identifier:name", "literal:!"},
			form:       StringFormRawSingleLine,
		},
		{
			name:       "literal braces around interpolation",
			source:     "`{{{name}}}`",
			wantChunks: []string{"literal:{", "identifier:name", "literal:}"},
			form:       StringFormRawSingleLine,
		},
		{
			name:       "backslash does not escape interpolation",
			source:     "`\\{name}`",
			wantChunks: []string{"literal:\\", "identifier:name"},
			form:       StringFormRawSingleLine,
		},
		{
			name: "multiline margin ignores interpolation value",
			source: "`\n" +
				"  before {name}\n" +
				"    after\n" +
				"  `",
			wantChunks: []string{"literal:before ", "identifier:name", "literal:\n  after"},
			form:       StringFormRawMultiline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.source), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %#v", result.Errors)
			}
			interpolated, ok := result.Program.Statements[0].(*InterpolatedStr)
			if !ok {
				t.Fatalf("expression = %T, want *InterpolatedStr", result.Program.Statements[0])
			}
			if interpolated.Form != tt.form {
				t.Fatalf("form = %v, want %v", interpolated.Form, tt.form)
			}
			got := describeStringChunks(t, interpolated.Chunks)
			if strings.Join(got, "|") != strings.Join(tt.wantChunks, "|") {
				t.Fatalf("chunks = %#v, want %#v", got, tt.wantChunks)
			}
		})
	}
}

func TestRawInterpolationParsesFullValueExpressionGrammar(t *testing.T) {
	source := "`{match true {\ntrue => \"yes\",\nfalse => \"no\",\n}}`"
	result := Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %#v", result.Errors)
	}
	interpolated, ok := result.Program.Statements[0].(*InterpolatedStr)
	if !ok || len(interpolated.Chunks) != 2 {
		t.Fatalf("expression = %#v", result.Program.Statements[0])
	}
	if _, ok := interpolated.Chunks[1].(*MatchExpression); !ok {
		t.Fatalf("interpolation = %T, want *MatchExpression", interpolated.Chunks[1])
	}
}

func TestRawInterpolationRangeRetainsSourceLocation(t *testing.T) {
	result := Parse([]byte("`{1..2}`"), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %#v", result.Errors)
	}
	interpolated := result.Program.Statements[0].(*InterpolatedStr)
	rangeExpr, ok := interpolated.Chunks[1].(*RangeExpression)
	if !ok {
		t.Fatalf("interpolation = %T, want *RangeExpression", interpolated.Chunks[1])
	}
	want := Location{Start: Point{Row: 1, Col: 3}, End: Point{Row: 1, Col: 6}}
	if rangeExpr.Location != want {
		t.Fatalf("location = %#v, want %#v", rangeExpr.Location, want)
	}
}

func TestRawInterpolationSupportsOrdinaryAndNestedRawStrings(t *testing.T) {
	tests := []string{
		"`{wrap(\"arg\")}`",
		"`{wrap(`inner`)}`",
	}
	for _, source := range tests {
		result := Parse([]byte(source), "test.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("source %q parse errors: %#v", source, result.Errors)
		}
		interpolated, ok := result.Program.Statements[0].(*InterpolatedStr)
		if !ok || len(interpolated.Chunks) != 2 {
			t.Fatalf("source %q expression = %#v", source, result.Program.Statements[0])
		}
		call, ok := interpolated.Chunks[1].(*FunctionCall)
		if !ok || len(call.Args) != 1 {
			t.Fatalf("source %q interpolation = %#v", source, interpolated.Chunks[1])
		}
		arg, ok := call.Args[0].Value.(*StrLiteral)
		if !ok || arg.Value != map[bool]string{true: "arg", false: "inner"}[strings.Contains(source, "arg")] {
			t.Fatalf("source %q argument = %#v", source, call.Args[0].Value)
		}
	}
}

func TestMultilineRawStringAllowsExpressionSuffix(t *testing.T) {
	source := "[`\n  value\n  `, `next`]"
	result := Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %#v", result.Errors)
	}
	list, ok := result.Program.Statements[0].(*ListLiteral)
	if !ok || len(list.Items) != 2 {
		t.Fatalf("expression = %#v", result.Program.Statements[0])
	}
}

func TestMultilineRawStringReportsUnderIndentedContent(t *testing.T) {
	source := "`\n  first\n second\n  `"
	result := Parse([]byte(source), "test.ard")
	if len(result.Errors) != 1 {
		t.Fatalf("errors = %#v, want one", result.Errors)
	}
	if !strings.Contains(result.Errors[0].Message, "closing delimiter margin") {
		t.Fatalf("error = %q", result.Errors[0].Message)
	}
	if result.Errors[0].Location.Start != (Point{Row: 3, Col: 1}) {
		t.Fatalf("location = %#v, want row 3 column 1", result.Errors[0].Location)
	}
}

func TestMultilineRawStringReportsUnderIndentedInterpolation(t *testing.T) {
	source := "`\n {value}\n  `"
	result := Parse([]byte(source), "test.ard")
	if len(result.Errors) != 1 {
		t.Fatalf("errors = %#v, want one", result.Errors)
	}
	if !strings.Contains(result.Errors[0].Message, "closing delimiter margin") {
		t.Fatalf("error = %q", result.Errors[0].Message)
	}
	if result.Errors[0].Location.Start != (Point{Row: 2, Col: 1}) {
		t.Fatalf("location = %#v, want row 2 column 1", result.Errors[0].Location)
	}
}

func TestRawInterpolationRequiresExactlyOneExpression(t *testing.T) {
	for _, source := range []string{"`{}`", "`{first second}`"} {
		result := Parse([]byte(source), "test.ard")
		if len(result.Errors) == 0 {
			t.Fatalf("source %q: expected parse error", source)
		}
		location := result.Errors[0].Location
		if location.Start.Row < 1 || location.End.Row < location.Start.Row ||
			(location.End.Row == location.Start.Row && location.End.Col < location.Start.Col) {
			t.Fatalf("source %q: invalid diagnostic location %#v", source, location)
		}
	}
}

func TestUnterminatedRawInterpolationStartsDiagnosticAtOpeningBrace(t *testing.T) {
	result := Parse([]byte("`{"), "test.ard")
	if len(result.Errors) != 1 {
		t.Fatalf("errors = %#v, want one", result.Errors)
	}
	location := result.Errors[0].Location
	if location.Start != (Point{Row: 1, Col: 2}) || location.End != (Point{Row: 1, Col: 2}) {
		t.Fatalf("location = %#v, want opening brace at 1:2", location)
	}
}

func TestUnterminatedRawStringAfterInterpolationEndsAtLastSourceByte(t *testing.T) {
	result := Parse([]byte("`{x}"), "test.ard")
	if len(result.Errors) != 1 {
		t.Fatalf("errors = %#v, want one", result.Errors)
	}
	location := result.Errors[0].Location
	if location.Start != (Point{Row: 1, Col: 1}) || location.End != (Point{Row: 1, Col: 4}) {
		t.Fatalf("location = %#v, want source extent 1:1-1:4", location)
	}
}

func TestUnterminatedRawStringReportsDiagnostic(t *testing.T) {
	result := Parse([]byte("`unterminated"), "test.ard")
	if len(result.Errors) == 0 {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(result.Errors[0].Message, "Unterminated raw string") {
		t.Fatalf("error = %q", result.Errors[0].Message)
	}
}

func TestRawStringLocationsUseSourceExtent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		end    Point
	}{
		{name: "single line", source: "`abc`", end: Point{Row: 1, Col: 5}},
		{name: "multiline", source: "`\n  abc\n  `", end: Point{Row: 3, Col: 3}},
		{name: "interpolated", source: "`a{value}b`", end: Point{Row: 1, Col: 11}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.source), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %#v", result.Errors)
			}
			location := result.Program.Statements[0].GetLocation()
			if location.Start != (Point{Row: 1, Col: 1}) || location.End != tt.end {
				t.Fatalf("location = %#v, want 1:1-%v", location, tt.end)
			}
		})
	}
}

func describeStringChunks(t *testing.T, chunks []Expression) []string {
	t.Helper()
	result := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		switch chunk := chunk.(type) {
		case *StrLiteral:
			result = append(result, "literal:"+chunk.Value)
		case *Identifier:
			result = append(result, "identifier:"+chunk.Name)
		default:
			t.Fatalf("unexpected chunk %T", chunk)
		}
	}
	return result
}
