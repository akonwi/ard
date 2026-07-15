package parse

import "testing"

func TestEmptyInterpolationContinuationRetainsLastConsumedEndpoint(t *testing.T) {
	result := Parse([]byte("\"a{1}"), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %#v", result.Errors)
	}
	location := result.Program.Statements[0].GetLocation()
	if location.End != (Point{Row: 1, Col: 5}) {
		t.Fatalf("location = %#v, want source EOF at 1:5", location)
	}
}

func TestMultilineInterpolationChunkEndsAtLastConsumedByte(t *testing.T) {
	result := Parse([]byte("\"a\n{1}b\""), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %#v", result.Errors)
	}
	interpolated, ok := result.Program.Statements[0].(*InterpolatedStr)
	if !ok || len(interpolated.Chunks) == 0 {
		t.Fatalf("expression = %#v", result.Program.Statements[0])
	}
	location := interpolated.Chunks[0].GetLocation()
	if location.End != (Point{Row: 1, Col: 3}) {
		t.Fatalf("first chunk location = %#v, want end 1:3 before delimiter on line 2", location)
	}
}

func TestStringLiteralLocationsUseRawSourceExtent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		end    Point
	}{
		{name: "ordinary", source: `"abc"`, end: Point{Row: 1, Col: 5}},
		{name: "escaped", source: `"a\n"`, end: Point{Row: 1, Col: 5}},
		{name: "unicode", source: `"é"`, end: Point{Row: 1, Col: 4}},
		{name: "interpolated", source: `"age = {age}"`, end: Point{Row: 1, Col: 13}},
		{name: "multiline", source: "\"a\nb\"", end: Point{Row: 2, Col: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %#v", result.Errors)
			}
			if len(result.Program.Statements) != 1 {
				t.Fatalf("statements = %d", len(result.Program.Statements))
			}
			location := result.Program.Statements[0].GetLocation()
			if location.Start != (Point{Row: 1, Col: 1}) || location.End != tt.end {
				t.Fatalf("location = %#v, want start 1:1 end %#v", location, tt.end)
			}
		})
	}
}
