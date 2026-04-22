package pretty

import "strings"

type printMode int

const (
	modeFlat printMode = iota
	modeBreak
)

type printCmd struct {
	indent int
	mode   printMode
	doc    Doc
}

func Print(root Doc, maxLineWidth int, indentWidth int) string {
	var out strings.Builder
	stack := []printCmd{{indent: 0, mode: modeBreak, doc: root}}
	column := 0

	for len(stack) > 0 {
		cmd := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch node := cmd.doc.(type) {
		case Text:
			if node.Value != "" {
				out.WriteString(node.Value)
				column += len(node.Value)
			}
		case Concat:
			for i := len(node.Parts) - 1; i >= 0; i-- {
				stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: node.Parts[i]})
			}
		case Indent:
			stack = append(stack, printCmd{indent: cmd.indent + indentWidth, mode: cmd.mode, doc: node.Content})
		case IfBreak:
			if cmd.mode == modeBreak {
				stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: node.Broken})
			} else {
				stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: node.Flat})
			}
		case Line:
			if node.Hard {
				out.WriteByte('\n')
				if cmd.indent > 0 {
					out.WriteString(strings.Repeat(" ", cmd.indent))
				}
				column = cmd.indent
				continue
			}
			if cmd.mode == modeFlat {
				if !node.Soft {
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
		case Group:
			testStack := append([]printCmd(nil), stack...)
			testStack = append(testStack, printCmd{indent: cmd.indent, mode: modeFlat, doc: node.Content})
			if fits(maxLineWidth-column, indentWidth, testStack) {
				stack = append(stack, printCmd{indent: cmd.indent, mode: modeFlat, doc: node.Content})
			} else {
				stack = append(stack, printCmd{indent: cmd.indent, mode: modeBreak, doc: node.Content})
			}
		}
	}

	return out.String()
}

func fits(remaining int, indentWidth int, stack []printCmd) bool {
	for remaining >= 0 && len(stack) > 0 {
		cmd := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch node := cmd.doc.(type) {
		case Text:
			remaining -= len(node.Value)
		case Concat:
			for i := len(node.Parts) - 1; i >= 0; i-- {
				stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: node.Parts[i]})
			}
		case Indent:
			stack = append(stack, printCmd{indent: cmd.indent + indentWidth, mode: cmd.mode, doc: node.Content})
		case IfBreak:
			if cmd.mode == modeBreak {
				stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: node.Broken})
			} else {
				stack = append(stack, printCmd{indent: cmd.indent, mode: cmd.mode, doc: node.Flat})
			}
		case Line:
			if node.Hard {
				if cmd.mode == modeFlat {
					return false
				}
				return true
			}
			if cmd.mode == modeFlat {
				if !node.Soft {
					remaining--
				}
				continue
			}
			return true
		case Group:
			stack = append(stack, printCmd{indent: cmd.indent, mode: modeFlat, doc: node.Content})
		}
	}
	return remaining >= 0
}
