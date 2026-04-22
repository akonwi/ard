package formatter

import "github.com/akonwi/ard/pretty"

func (p printer) printDoc(root doc) string {
	return pretty.Print(root, p.maxLineWidth, indentWidth)
}
