package tree_sitter_ard

// #cgo CFLAGS: -std=c11 -fPIC
// #include "../../src/parser.c"
// // NOTE: if your language has an external scanner, add it here.
import "C"

import (
	"unsafe"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// 0.0.31
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_ard())
}

func MakeParser() (*tree_sitter.Parser, error) {
	parser := tree_sitter.NewParser()
	err := parser.SetLanguage(tree_sitter.NewLanguage(Language()))
	if err != nil {
		return nil, err
	}
	return parser, nil
}

func Parse(source []byte) (*tree_sitter.Tree, error) {
	parser, err := MakeParser()
	if err != nil {
		return nil, err
	}
	return parser.Parse(source, nil), nil
}
