package formatter

import "strings"

type printMode int

const (
	modeFlat printMode = iota
	modeBreak
)

type printCmd struct {
	indent int
	mode   printMode
	doc    doc
}

func (p printer) printDoc(root doc) string {
	var out strings.Builder
	stack := []printCmd{{indent: 0, mode: modeBreak, doc: root}}
	column := 0

	for len(stack) > 0 {
		cmd := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch node := cmd.doc.(type) {
		case docText:
			if node.value != "" {
				out.WriteString(node.value)
				column += len(node.value)
			}
		case docConcat:
			for i := len(node.parts) - 1; i >= 0; i-- {
				stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: node.parts[i]})
			}
		case docIndent:
			stack = append(stack, printCmd{indent: cmd.indent + indentWidth, mode: cmd.mode, doc: node.content})
		case docIfBreak:
			if cmd.mode == modeBreak {
				stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: node.broken})
			} else {
				stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: node.flat})
			}
		case docLine:
			if node.hard {
				out.WriteByte('\n')
				if cmd.indent > 0 {
					out.WriteString(strings.Repeat(" ", cmd.indent))
				}
				column = cmd.indent
				continue
			}
			if cmd.mode == modeFlat {
				if !node.soft {
					out.WriteByte(' ')
					column++
				}
				continue
			}
			out.WriteByte('\n')
			if cmd.indent > 0 {
				out.WriteString(strings.Repeat(" ", cmd.indent))
			}
			column = cmd.indent
		case docGroup:
			if p.fits(p.maxLineWidth-column, append(stack, printCmd{indent: cmd.indent, mode: modeFlat, doc: node.content})) {
				stack = append(stack, printCmd{indent: cmd.indent, mode: modeFlat, doc: node.content})
			} else {
				stack = append(stack, printCmd{indent: cmd.indent, mode: modeBreak, doc: node.content})
			}
		}
	}

	return out.String()
}

func (p printer) fits(remaining int, stack []printCmd) bool {
	for remaining >= 0 && len(stack) > 0 {
		cmd := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch node := cmd.doc.(type) {
		case docText:
			remaining -= len(node.value)
		case docConcat:
			for i := len(node.parts) - 1; i >= 0; i-- {
				stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: node.parts[i]})
			}
		case docIndent:
			stack = append(stack, printCmd{indent: cmd.indent + indentWidth, mode: cmd.mode, doc: node.content})
		case docIfBreak:
			if cmd.mode == modeBreak {
				stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: node.broken})
			} else {
				stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: node.flat})
			}
		case docLine:
			if node.hard {
				if cmd.mode == modeFlat {
					return false
				}
				return true
			}
			if cmd.mode == modeFlat {
				if !node.soft {
					remaining--
				}
				continue
			}
			return true
		case docGroup:
			stack = append(stack, printCmd{indent: cmd.indent, mode: modeFlat, doc: node.content})
		}
	}
	return remaining >= 0
}
