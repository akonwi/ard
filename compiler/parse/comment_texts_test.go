package parse

import (
	"slices"
	"testing"
)

func TestCommentTexts(t *testing.T) {
	source := "// header\nlet x = [ // open\n  1, ///  doc  \n  // // nested\n]\nlet s = \"// not a comment\"\n//\n"
	got := CommentTexts([]byte(source))
	want := []string{"header", "open", "/  doc", "// nested", ""}
	if !slices.Equal(got, want) {
		t.Fatalf("CommentTexts = %q, want %q", got, want)
	}
}
