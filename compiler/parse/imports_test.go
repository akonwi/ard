package parse

import "testing"

func TestImportPathLocations(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   Location
	}{
		{
			name:   "Ard module",
			source: "use app/nested/module\n",
			want:   Location{Start: Point{Row: 1, Col: 5}, End: Point{Row: 1, Col: 21}},
		},
		{
			name:   "Go package excludes prefix",
			source: "use go:example.com/acme/pkg\n",
			want:   Location{Start: Point{Row: 1, Col: 8}, End: Point{Row: 1, Col: 27}},
		},
		{
			name:   "Aliased module excludes alias",
			source: "use app/people as users\n",
			want:   Location{Start: Point{Row: 1, Col: 5}, End: Point{Row: 1, Col: 14}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			if len(result.Program.Imports) != 1 {
				t.Fatalf("imports = %#v", result.Program.Imports)
			}
			if got := result.Program.Imports[0].PathLocation; got != tt.want {
				t.Fatalf("path location = %v, want %v", got, tt.want)
			}
		})
	}
}
