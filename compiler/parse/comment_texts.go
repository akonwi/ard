package parse

import "strings"

// CommentTexts returns the text of every `//` comment in source, in source
// order, without the `//` marker and surrounding whitespace. The formatter
// uses it to check that formatting kept every comment.
func CommentTexts(source []byte) []string {
	var texts []string
	for _, tok := range NewLexer(source).Scan() {
		if tok.kind != comment {
			continue
		}
		texts = append(texts, strings.TrimSpace(strings.TrimPrefix(tok.text, "//")))
	}
	return texts
}
