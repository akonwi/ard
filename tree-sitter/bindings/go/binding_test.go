package tree_sitter_ard

import (
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCanLoadGrammar(t *testing.T) {
	language := tree_sitter.NewLanguage(Language())
	if language == nil {
		t.Errorf("Error loading Ard grammar")
	}
}
