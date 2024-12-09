package ast

import (
	"strings"
)

type Document struct {
	indentLevel int
	lines       []string
}

func MakeDoc(content string) Document {
	var lines []string
	if content != "" {
		contentLines := strings.Split(content, "\n")
		lines = make([]string, len(contentLines))
		for i, line := range contentLines {
			lines[i] = line
		}
	} else {
		lines = make([]string, 0)
	}
	return Document{lines: lines, indentLevel: 0}
}

func (d Document) String() string {
	return strings.Join(d.lines, "\n")
}

func (d Document) indentation() string {
	return strings.Repeat(" ", d.indentLevel*2)
}

func (d *Document) Indent() *Document {
	d.indentLevel++
	return d
}

func (d *Document) Dedent() *Document {
	d.indentLevel--
	return d
}

func (d *Document) Line(line string) *Document {
	d.lines = append(d.lines, d.indentation()+line)
	return d
}

func (d *Document) Nest(doc Document) *Document {
	d.Indent()
	for i, line := range doc.lines {
		// skip trailing empties
		if i == len(doc.lines)-1 && line == "" {
			continue
		}
		d.lines = append(d.lines, d.indentation()+line)
	}
	d.Dedent()
	return d
}

func (d *Document) Append(doc Document) *Document {
	for _, line := range doc.lines {
		d.lines = append(d.lines, line)
	}
	return d
}
