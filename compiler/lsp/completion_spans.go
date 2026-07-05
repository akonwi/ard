package lsp

import (
	"context"
	"sort"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/lsp/analysis"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// completionFromSpans resolves member completions through the analysis
// engine: the receiver expression's checked type enumerates fields and
// methods. Returns nil to fall back to legacy heuristics (builtin receivers,
// static/import completion) during the migration.
func (s *Server) completionFromSpans(ctx context.Context, docURI uri.URI, source string, position protocol.Position) []protocol.CompletionItem {
	cctx, ok := completionContextAt(source, position)
	if !ok || cctx.sepEnd < 2 {
		return nil
	}
	if cctx.kind != completionInstance && cctx.kind != completionStatic {
		return nil
	}
	filePath, err := filePathFromURI(docURI)
	if err != nil {
		return nil
	}

	// Patch a placeholder identifier after the dot so the file parses, then
	// analyze it in a scratch workspace sharing the engine's caches.
	patched := source[:cctx.sepEnd] + completionPlaceholder + source[cctx.offset:]
	ws := s.workspaceFor(filePath)
	scratch := analysis.NewWorkspace(ws.Engine())
	// Carry over sibling overlays so imports resolve against editor state.
	for _, doc := range s.cache.Snapshot() {
		if p, err := filePathFromURI(doc.URI); err == nil && p != filePath {
			scratch.SetOverlay(p, doc.Text)
		}
	}
	scratch.SetOverlay(filePath, patched)
	fa, err := scratch.Snapshot().AnalyzeEphemeral(ctx, filePath)
	if err != nil || fa == nil || fa.Spans == nil {
		return nil
	}

	if cctx.kind == completionStatic {
		items := s.staticCompletionFromSpans(fa, source, cctx)
		if len(items) == 0 {
			return nil
		}
		return withCompletionTextEdits(items, cctx, position)
	}

	// The receiver is the expression immediately before the dot.
	receiverPoint := offsetToParsePoint(patched, cctx.sepEnd-2)
	var receiverType checker.Type
	for _, rec := range fa.Spans.At(receiverPoint) {
		if sym, ok := rec.Key.(*checker.Symbol); ok && sym.Type != nil {
			if isTypeNameSymbol(sym) {
				// `TypeName.` is not a value receiver; statics use `::`.
				return nil
			}
			receiverType = sym.Type
			break
		}
		if rec.Node != nil && rec.Node.Type() != nil {
			if v, ok := rec.Node.(*checker.Variable); ok && isTypeNameSymbol(v.Symbol()) {
				return nil
			}
			receiverType = rec.Node.Type()
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
	return withCompletionTextEdits(items, cctx, position)
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
		if fa.Checked != nil {
			program := fa.Checked
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
		// Builtin receivers (Str, Int, lists, maps, Maybe, Result, ...)
		// enumerate through the checker's kind tables so completion and
		// hover render from the same source of truth.
		for _, name := range checker.BuiltinMemberNames(t) {
			addMethod(checker.BuiltinMethodDef(t, name))
		}
		if len(items) == 0 {
			return nil
		}
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

// staticCompletionFromSpans enumerates `target::` members from the checked
// program: imported module symbols, local type statics (enum variants,
// Type::fn declarations), and builtin static packages.
func (s *Server) staticCompletionFromSpans(fa *analysis.FileAnalysis, source string, cctx completionContext) []protocol.CompletionItem {
	target := staticCompletionTarget(source, cctx)
	if target == "" || fa.Checked == nil {
		return nil
	}

	items := map[string]protocol.CompletionItem{}
	add := func(item protocol.CompletionItem) {
		if item.Label != "" {
			if _, exists := items[item.Label]; !exists {
				items[item.Label] = item
			}
		}
	}

	// Prelude statics: auto-imported modules are keyed by their surface
	// alias (Int, Str, List, ...), and builtin static packages (Result,
	// Chan) resolve through the prelude table.
	if mod := fa.Checked.Imports[target]; mod != nil {
		for name, sym := range mod.Symbols() {
			if !strings.Contains(name, "::") {
				add(staticSymbolCompletionItem(name, sym))
			}
		}
	}
	if mod, ok := checker.PreludeModule(target); ok {
		for name, sym := range mod.Symbols() {
			add(staticSymbolCompletionItem(name, sym))
		}
	}

	// Imported module members. The checked Imports map is keyed by import
	// path; resolve the alias through the parse tree's import list.
	if fa.Program != nil {
		for _, imp := range fa.Program.Imports {
			if imp.Alias() != target {
				continue
			}
			if mod := fa.Checked.Imports[imp.Path]; mod != nil {
				for name, sym := range mod.Symbols() {
					// Compound (Type::fn) statics and test functions are not
					// module member completions, matching legacy behavior.
					if strings.Contains(name, "::") {
						continue
					}
					if def, ok := sym.Type.(*checker.FunctionDef); ok && def.IsTest {
						continue
					}
					add(staticSymbolCompletionItem(name, sym))
				}
			}
		}
	}

	// Imported types: `Color::` variants and `Type::fn` statics where the
	// type comes from an imported module.
	if fa.Program != nil {
		for _, imp := range fa.Program.Imports {
			mod := fa.Checked.Imports[imp.Path]
			if mod == nil {
				continue
			}
			if sym := mod.Get(target); !sym.IsZero() {
				if enum, ok := sym.Type.(*checker.Enum); ok {
					for _, value := range enum.Values {
						add(protocol.CompletionItem{Label: value.Name, Kind: protocol.CompletionItemKindEnumMember, Detail: target})
					}
				}
			}
			importPrefix := target + "::"
			for name, memberSym := range mod.Symbols() {
				if strings.HasPrefix(name, importPrefix) {
					if def, ok := memberSym.Type.(*checker.FunctionDef); ok {
						add(protocol.CompletionItem{
							Label:  strings.TrimPrefix(name, importPrefix),
							Kind:   protocol.CompletionItemKindFunction,
							Detail: methodDetailString(def),
						})
					}
				}
			}
		}
	}

	// Local declarations from the module's public symbol table: enum
	// variants for `Target::`, and `fn Target::name` statics.
	prefix := target + "::"
	if fa.Module != nil {
		for name, sym := range fa.Module.Symbols() {
			if name == target {
				if enum, ok := sym.Type.(*checker.Enum); ok {
					for _, value := range enum.Values {
						add(protocol.CompletionItem{Label: value.Name, Kind: protocol.CompletionItemKindEnumMember, Detail: target})
					}
				}
			}
			if strings.HasPrefix(name, prefix) {
				if def, ok := sym.Type.(*checker.FunctionDef); ok {
					add(protocol.CompletionItem{
						Label:  strings.TrimPrefix(name, prefix),
						Kind:   protocol.CompletionItemKindFunction,
						Detail: methodDetailString(def),
					})
				}
			}
		}
	}

	out := make([]protocol.CompletionItem, 0, len(items))
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, items[name])
	}
	return out
}

// staticCompletionTarget extracts the identifier before `::`.
func staticCompletionTarget(source string, cctx completionContext) string {
	end := cctx.sepEnd - 2
	if end <= 0 || end > len(source) {
		return ""
	}
	start := end
	for start > 0 && isIdentByte(source[start-1]) {
		start--
	}
	return source[start:end]
}

// staticSymbolCompletionItem renders one module member.
func staticSymbolCompletionItem(name string, sym checker.Symbol) protocol.CompletionItem {
	if def, ok := sym.Type.(*checker.FunctionDef); ok {
		return protocol.CompletionItem{
			Label:  name,
			Kind:   protocol.CompletionItemKindFunction,
			Detail: methodDetailString(def),
		}
	}
	kind := protocol.CompletionItemKindValue
	switch sym.Type.(type) {
	case *checker.StructDef:
		kind = protocol.CompletionItemKindStruct
	case *checker.Enum:
		kind = protocol.CompletionItemKindEnum
	case *checker.Trait:
		kind = protocol.CompletionItemKindInterface
	}
	return protocol.CompletionItem{Label: name, Kind: kind, Detail: checkerTypeString(sym.Type)}
}

// isTypeNameSymbol reports whether a scope symbol is a nominal type's own
// name (the hoisted type symbol) rather than a value binding.
func isTypeNameSymbol(sym *checker.Symbol) bool {
	if sym == nil || sym.Type == nil {
		return false
	}
	switch owner := sym.Type.(type) {
	case *checker.StructDef:
		return owner.Name == sym.Name
	case *checker.Enum:
		return owner.Name == sym.Name
	case *checker.Trait:
		return owner.Name == sym.Name
	}
	return false
}

// computeImportCompletions serves `use ` path completion, which is
// filesystem/parse based rather than semantic.
func computeImportCompletions(source string, filePath string, position protocol.Position) []protocol.CompletionItem {
	cctx, ok := completionContextAt(source, position)
	if !ok || cctx.kind != completionImport {
		return []protocol.CompletionItem{}
	}
	return withCompletionTextEdits(importPathCompletionItems(cctx.importPath, filePath), cctx, position)
}
