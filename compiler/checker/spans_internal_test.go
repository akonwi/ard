package checker

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestCompletionSpanCanBeRecordedAfterSpeculativeRollback(t *testing.T) {
	c := &Checker{spans: &SpanIndex{}}
	location := parse.Location{
		Start: parse.Point{Row: 1, Col: 5},
		End:   parse.Point{Row: 1, Col: 10},
	}
	owner := &StructDef{Name: "User", Fields: map[string]Type{"name": Str}}

	mark := c.spansMark()
	c.recordCompletionType(location, owner)
	c.spansTruncate(mark)
	c.recordCompletionType(location, owner)

	records := c.spans.At(parse.Point{Row: 1, Col: 6})
	if len(records) != 1 || records[0].CompletionType != owner {
		t.Fatalf("completion records after rollback = %#v, want one User hint", records)
	}
}
