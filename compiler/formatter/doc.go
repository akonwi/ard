package formatter

import "github.com/akonwi/ard/pretty"

type doc = pretty.Doc

type docText = pretty.Text
type docConcat = pretty.Concat
type docGroup = pretty.Group
type docIndent = pretty.Indent
type docLine = pretty.Line
type docIfBreak = pretty.IfBreak

func dText(value string) doc              { return pretty.TextDoc(value) }
func dConcat(parts ...doc) doc            { return pretty.ConcatDocs(parts...) }
func dGroup(content doc) doc              { return pretty.GroupDoc(content) }
func dIndent(content doc) doc             { return pretty.IndentDoc(content) }
func dLine() doc                          { return pretty.LineDoc() }
func dSoftLine() doc                      { return pretty.SoftLine() }
func dHardLine() doc                      { return pretty.HardLine() }
func dIfBreak(broken doc, flat doc) doc   { return pretty.IfBreakDoc(broken, flat) }
func dJoin(separator doc, docs []doc) doc { return pretty.Join(separator, docs) }
