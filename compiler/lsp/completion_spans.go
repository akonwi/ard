package lsp

import (
	"sort"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/lsp/analysis"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// completionFromSpans resolves member completions through the analysis
// engine: the receiver expression's checked type enumerates fields and
// methods. Returns nil to fall back to legacy heuristics (builtin receivers,
// static/import completion) during the migration.
func (s *Server) completionFromSpans(docURI uri.URI, source string, position protocol.Position) []protocol.CompletionItem {
	ctx, ok := completionContextAt(source, position)
	if !ok || ctx.kind != completionInstance || ctx.sepEnd < 2 {
		return nil
	}
	filePath, err := filePathFromURI(docURI)
	if err != nil {
		return nil
	}

	// Patch a placeholder identifier after the dot so the file parses, then
	// analyze it in a scratch workspace sharing the engine's caches.
	patched := source[:ctx.sepEnd] + completionPlaceholder + source[ctx.offset:]
	ws := s.workspaceFor(filePath)
	scratch := analysis.NewWorkspace(ws.Engine())
	// Carry over sibling overlays so imports resolve against editor state.
	for _, doc := range s.cache.Snapshot() {
		if p, err := filePathFromURI(doc.URI); err == nil && p != filePath {
			scratch.SetOverlay(p, doc.Text)
		}
	}
	scratch.SetOverlay(filePath, patched)
	fa, err := scratch.Snapshot().AnalyzeEphemeral(filePath)
	if err != nil || fa == nil || fa.Spans == nil {
		return nil
	}

	// The receiver is the expression immediately before the dot.
	receiverPoint := offsetToParsePoint(patched, ctx.sepEnd-2)
	var receiverType checker.Type
	for _, rec := range fa.Spans.At(receiverPoint) {
		if rec.Node != nil && rec.Node.Type() != nil {
			receiverType = rec.Node.Type()
			break
		}
		if sym, ok := rec.Key.(*checker.Symbol); ok && sym.Type != nil {
			receiverType = sym.Type
			break
		}
	}
	if receiverType == nil {
		return nil
	}
	if ref, ok := receiverType.(*checker.MutableRef); ok {
		receiverType = ref.Of()
	}

	items := memberCompletionItems(receiverType, fa)
	if len(items) == 0 {
		return nil
	}
	return withCompletionTextEdits(items, ctx, position)
}

// memberCompletionItems enumerates fields and methods for a checked type.
func memberCompletionItems(t checker.Type, fa *analysis.FileAnalysis) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	addMethod := func(def *checker.FunctionDef) {
		if def == nil {
			return
		}
		items = append(items, protocol.CompletionItem{
			Label:  def.Name,
			Kind:   protocol.CompletionItemKindMethod,
			Detail: methodDetailString(def),
		})
	}

	switch owner := t.(type) {
	case *checker.Trait:
		for _, method := range owner.GetMethods() {
			m := method
			addMethod(&m)
		}
	case *checker.StructDef:
		fieldNames := make([]string, 0, len(owner.Fields))
		for name := range owner.Fields {
			fieldNames = append(fieldNames, name)
		}
		sort.Strings(fieldNames)
		for _, name := range fieldNames {
			items = append(items, protocol.CompletionItem{
				Label:  name,
				Kind:   protocol.CompletionItemKindField,
				Detail: checkerTypeString(owner.Fields[name]),
			})
		}
		if fa.Checker != nil {
			program := fa.Checker.Module().Program()
			methods := program.StructMethodsFor(checker.StructMethodOwner(owner))
			// Imported structs keep their methods in the defining module's
			// program; merge the cross-module view.
			imported := checker.StructMethodsInModules(program.Imports, checker.StructMethodOwner(owner))
			merged := make(map[string]*checker.FunctionDef, len(methods)+len(imported))
			for name, def := range imported {
				merged[name] = def
			}
			for name, def := range methods {
				merged[name] = def
			}
			methodNames := make([]string, 0, len(merged))
			for name := range merged {
				methodNames = append(methodNames, name)
			}
			sort.Strings(methodNames)
			for _, name := range methodNames {
				addMethod(merged[name])
			}
		}
	case *checker.Enum:
		methodNames := make([]string, 0, len(owner.Methods))
		for name := range owner.Methods {
			methodNames = append(methodNames, name)
		}
		sort.Strings(methodNames)
		for _, name := range methodNames {
			addMethod(owner.Methods[name])
		}
	default:
		return nil
	}
	return items
}

// methodDetailString renders "fn (params) Return" for completion detail.
func methodDetailString(def *checker.FunctionDef) string {
	detail := "fn " + paramListString(def)
	if ret := checkerTypeString(def.ReturnType); ret != "" && ret != "Void" {
		detail += " " + ret
	}
	return detail
}
