package lsp

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestLocationToRangeUsesUTF16AndExclusiveEnd(t *testing.T) {
	lines := newDocLines("é😀x\n")

	t.Run("astral rune", func(t *testing.T) {
		range_ := diagnosticLocationToRange(lines, parse.Location{
			Start: parse.Point{Row: 1, Col: 3},
			End:   parse.Point{Row: 1, Col: 6},
		})
		if range_.Start.Character != 1 || range_.End.Character != 3 {
			t.Fatalf("range = %#v, want UTF-16 characters 1..3", range_)
		}
	})

	t.Run("after BMP and astral runes", func(t *testing.T) {
		range_ := diagnosticLocationToRange(lines, parse.Location{
			Start: parse.Point{Row: 1, Col: 7},
			End:   parse.Point{Row: 1, Col: 7},
		})
		if range_.Start.Character != 3 || range_.End.Character != 4 {
			t.Fatalf("range = %#v, want UTF-16 characters 3..4", range_)
		}
	})
}
