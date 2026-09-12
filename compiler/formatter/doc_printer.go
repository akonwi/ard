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
	return p.printDocAtColumn(root, 0)
}

func (p printer) printDocAtColumn(root doc, baseColumn int) string {
	var out strings.Builder
	stack := []printCmd{{indent: 0, mode: modeBreak, doc: root}}
	column := baseColumn
	// Indentation is written lazily, just before the next non-empty text on
	// a line, so that blank lines (consecutive line breaks) never carry
	// trailing whitespace.
	pendingIndent := 0

	newline := func(indent int) {
		out.WriteByte('\n')
		pendingIndent = indent
		column = baseColumn + indent
	}
	// write is the only path that emits visible content, so indentation is
	// guaranteed to precede anything else on a line.
	write := func(value string) {
		if pendingIndent > 0 {
			out.WriteString(strings.Repeat(" ", pendingIndent))
			pendingIndent = 0
		}
		out.WriteString(value)
	}

	for len(stack) > 0 {
		cmd := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch node := cmd.doc.(type) {
		case docText:
			if node.value != "" {
				write(node.value)
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
		case docIfFits:
			selected := node.fallback
			if node.firstLineWidth <= p.maxLineWidth-column {
				selected = node.preferred
			}
			stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: selected})
		case docLine:
			if node.hard {
				newline(cmd.indent)
				continue
			}
			if cmd.mode == modeFlat {
				if !node.soft {
					write(" ")
					column++
				}
				continue
			}
			newline(cmd.indent)
		case docGroup:
			testStack := append([]printCmd(nil), stack...)
			testStack = append(testStack, printCmd{indent: cmd.indent, mode: modeFlat, doc: node.content})
			if fits(p.maxLineWidth-column, testStack) {
				stack = append(stack, printCmd{indent: cmd.indent, mode: modeFlat, doc: node.content})
			} else {
				stack = append(stack, printCmd{indent: cmd.indent, mode: modeBreak, doc: node.content})
			}
		}
	}

	return out.String()
}

func fits(remaining int, stack []printCmd) bool {
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
		case docIfFits:
			selected := node.fallback
			if node.firstLineWidth <= remaining {
				selected = node.preferred
			}
			stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: selected})
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
