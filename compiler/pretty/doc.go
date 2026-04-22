package pretty

import "strings"

type Doc interface{ isDoc() }

type Text struct{ Value string }
type Concat struct{ Parts []Doc }
type Group struct{ Content Doc }
type Indent struct{ Content Doc }
type Line struct {
	Hard bool
	Soft bool
}
type IfBreak struct {
	Broken Doc
	Flat   Doc
}

func (Text) isDoc()    {}
func (Concat) isDoc()  {}
func (Group) isDoc()   {}
func (Indent) isDoc()  {}
func (Line) isDoc()    {}
func (IfBreak) isDoc() {}

func TextDoc(value string) Doc {
	if value == "" {
		return Text{Value: ""}
	}
	if !strings.Contains(value, "\n") {
		return Text{Value: value}
	}
	parts := strings.Split(value, "\n")
	docs := make([]Doc, 0, len(parts)*2)
	for i, part := range parts {
		docs = append(docs, Text{Value: part})
		if i < len(parts)-1 {
			docs = append(docs, HardLine())
		}
	}
	return Concat{Parts: docs}
}

func ConcatDocs(parts ...Doc) Doc {
	flat := make([]Doc, 0, len(parts))
	for _, part := range parts {
		if part == nil {
			continue
		}
		if concat, ok := part.(Concat); ok {
			flat = append(flat, concat.Parts...)
			continue
		}
		flat = append(flat, part)
	}
	if len(flat) == 0 {
		return Text{Value: ""}
	}
	if len(flat) == 1 {
		return flat[0]
	}
	return Concat{Parts: flat}
}

func GroupDoc(content Doc) Doc            { return Group{Content: content} }
func IndentDoc(content Doc) Doc           { return Indent{Content: content} }
func LineDoc() Doc                        { return Line{} }
func SoftLine() Doc                       { return Line{Soft: true} }
func HardLine() Doc                       { return Line{Hard: true} }
func IfBreakDoc(broken Doc, flat Doc) Doc { return IfBreak{Broken: broken, Flat: flat} }

func Join(separator Doc, docs []Doc) Doc {
	if len(docs) == 0 {
		return Text{Value: ""}
	}
	parts := make([]Doc, 0, len(docs)*2)
	for i, item := range docs {
		if i > 0 {
			parts = append(parts, separator)
		}
		parts = append(parts, item)
	}
	return ConcatDocs(parts...)
}
