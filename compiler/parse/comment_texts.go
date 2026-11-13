package parse

import "strings"

// CommentTexts returns the text of every `//` comment in source, in source
// order, with the leading slashes and surrounding whitespace removed. The
// formatter uses it to check that formatting kept every comment.
func CommentTexts(source []byte) []string {
	var texts []string
	for _, tok := range NewLexer(source).Scan() {
		if tok.kind != comment {
			continue
		}
		texts = append(texts, strings.TrimSpace(strings.TrimLeft(tok.text, "/ \t")))
	}
	return texts
}
