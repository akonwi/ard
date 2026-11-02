package formatter

import "strings"

type doc interface{ isDoc() }

type docText struct{ value string }
type docConcat struct{ parts []doc }
type docGroup struct{ content doc }
type docIndent struct{ content doc }
type docLine struct {
	hard bool
	soft bool
}

// docForceBreak wraps content that must be printed in break mode, even when
// it would fit on one line. Unlike a hard line it emits nothing itself, so it
// can force multi-line output without introducing a blank line.
type docForceBreak struct {
	content doc
}

func (docForceBreak) isDoc() {}

type docIfBreak struct {
	broken doc
	flat   doc
}
type docIfFits struct {
	firstLineWidth int
	preferred      doc
	fallback       doc
}

func (docText) isDoc()    {}
func (docConcat) isDoc()  {}
func (docGroup) isDoc()   {}
func (docIndent) isDoc()  {}
func (docLine) isDoc()    {}
func (docIfBreak) isDoc() {}
func (docIfFits) isDoc()  {}

func dText(value string) doc {
	if value == "" {
		return docText{value: ""}
	}
	if !strings.Contains(value, "\n") {
		return docText{value: value}
	}
	parts := strings.Split(value, "\n")
	docs := make([]doc, 0, len(parts)*2)
	for i, part := range parts {
		docs = append(docs, docText{value: part})
		if i < len(parts)-1 {
			docs = append(docs, dHardLine())
		}
	}
	return dConcat(docs...)
}

func dConcat(parts ...doc) doc {
	flat := make([]doc, 0, len(parts))
	for _, part := range parts {
		if part == nil {
			continue
		}
		if concat, ok := part.(docConcat); ok {
			flat = append(flat, concat.parts...)
			continue
		}
		flat = append(flat, part)
	}
	if len(flat) == 0 {
		return docText{value: ""}
	}
	if len(flat) == 1 {
		return flat[0]
	}
	return docConcat{parts: flat}
}

func dGroup(content doc) doc            { return docGroup{content: content} }
func dIndent(content doc) doc           { return docIndent{content: content} }
func dLine() doc                        { return docLine{} }
func dSoftLine() doc                    { return docLine{soft: true} }
func dHardLine() doc                    { return docLine{hard: true} }
func dIfBreak(broken doc, flat doc) doc { return docIfBreak{broken: broken, flat: flat} }

// dForceBreak forces the enclosing group to break. Use it for content that is
// only ever valid across several lines, such as a comment-only sequence.
func dForceBreak(content doc) doc { return docForceBreak{content: content} }
func dIfFits(firstLineWidth int, preferred doc, fallback doc) doc {
	return docIfFits{firstLineWidth: firstLineWidth, preferred: preferred, fallback: fallback}
}

func dJoin(separator doc, docs []doc) doc {
	if len(docs) == 0 {
		return docText{value: ""}
	}
	parts := make([]doc, 0, len(docs)*2)
	for i, item := range docs {
		if i > 0 {
			parts = append(parts, separator)
		}
		parts = append(parts, item)
	}
	return dConcat(parts...)
}
