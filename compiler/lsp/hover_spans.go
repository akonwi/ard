package lsp

import (
	"context"
	"fmt"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/lsp/analysis"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// hoverFromSpans renders hover content from the checker's span table
// (ADR 0043). Returns nil when the position resolves to nothing so callers
// can fall back to legacy heuristics during the migration.
func (s *Server) hoverFromSpans(ctx context.Context, docURI uri.URI, position protocol.Position) *hoverInfo {
	fa, err := s.analyzeSnapshot(ctx, docURI)
	if err != nil || fa == nil || fa.Spans == nil {
		return nil
	}

	docURIPath, err := filePathFromURI(docURI)
	if err != nil {
		return nil
	}
	point := s.docLinesFor(docURIPath).positionToPoint(position)
	records := fa.Spans.At(point)
	for _, rec := range records {
		if content := renderSpanHover(rec); content != "" {
			return simpleHover(content)
		}
		// Binding records carry no node but know the symbol's type.
		if sym, ok := rec.Key.(*checker.Symbol); ok && sym.Type != nil {
			return simpleHover(checkerTypeString(sym.Type))
		}
		// Function and method definition records render their signature when
		// the cursor sits on the declaration line itself.
		if key, ok := rec.Key.(string); ok && rec.IsDef && point.Row == rec.Loc.Start.Row {
			switch {
			case strings.HasPrefix(key, "fn:"):
				for _, other := range records {
					if def, ok := other.Node.(*checker.FunctionDef); ok && other.Loc == rec.Loc {
						return simpleHover(functionSignatureString(def.Name, def))
					}
				}
			case strings.HasPrefix(key, "method:"):
				// key = method:<module>:<Owner>.<name>
				tail := key[strings.LastIndex(key, ":")+1:]
				owner, name, found := strings.Cut(tail, ".")
				if !found {
					break
				}
				if def := methodDefFromAnalysis(fa, owner, name); def != nil {
					return simpleHover(methodSignatureString(owner, def))
				}
			}
		}
	}
	return nil
}

// renderSpanHover renders one span record, or "" when it has nothing to say.
func renderSpanHover(rec checker.SpanRecord) string {
	if rec.Node == nil {
		return ""
	}

	// Builtin methods: `fn mut [Str].set(index: Int, value: Str) Bool`.
	if recv, name, ok := checker.BuiltinMethodInfo(rec.Node); ok && name != "" {
		if def := checker.BuiltinMethodDef(recv, name); def != nil {
			return methodSignatureString(checkerTypeString(recv), def)
		}
		return fmt.Sprintf("fn %s.%s(...)", checkerTypeString(recv), name)
	}

	switch node := rec.Node.(type) {
	case *checker.FunctionDef:
		// The enclosing function declaration's span covers the whole body;
		// rendering it here would shadow every inner hover. Function
		// signature hovers resolve through call records instead.
		if _, isDecl := rec.Source.(*parse.FunctionDeclaration); isDecl {
			return ""
		}
	case *checker.InstanceMethod:
		if node.Method != nil {
			owner := instanceMethodOwner(node)
			if def := node.Method.Definition(); def != nil {
				return methodSignatureString(owner, def)
			}
		}
	case *checker.InstanceProperty:
		ownerType := node.Subject.Type()
		if ref, ok := ownerType.(*checker.MutableRef); ok {
			ownerType = ref.Of()
		}
		return fmt.Sprintf("%s.%s: %s", checkerTypeString(ownerType), node.Property, checkerTypeString(node.Type()))
	case *checker.FunctionCall:
		if def := node.Definition(); def != nil {
			return functionSignatureString(node.Name, def)
		}
	case *checker.Variable:
		return checkerTypeString(node.Type())
	}

	// Fallback: render the expression's type.
	if t := rec.Node.Type(); t != nil {
		text := checkerTypeString(t)
		if text != "" && text != "?" {
			return text
		}
	}
	return ""
}

func instanceMethodOwner(node *checker.InstanceMethod) string {
	subjType := node.Subject.Type()
	if ref, ok := subjType.(*checker.MutableRef); ok {
		subjType = ref.Of()
	}
	return checkerTypeString(subjType)
}

// methodSignatureString renders `fn [mut ]Owner.name(params) Return`.
func methodSignatureString(owner string, def *checker.FunctionDef) string {
	var b strings.Builder
	b.WriteString("fn ")
	if def.Mutates {
		b.WriteString("mut ")
	}
	b.WriteString(owner)
	b.WriteString(".")
	b.WriteString(def.Name)
	b.WriteString(paramListString(def))
	if ret := checkerTypeString(def.ReturnType); ret != "" && ret != "Void" {
		b.WriteString(" ")
		b.WriteString(ret)
	}
	return b.String()
}

// functionSignatureString renders `fn name(params) Return`.
func functionSignatureString(name string, def *checker.FunctionDef) string {
	display := def.Name
	if display == "" || strings.Contains(name, "::") {
		display = name
	}
	var b strings.Builder
	b.WriteString("fn ")
	b.WriteString(display)
	b.WriteString(paramListString(def))
	if ret := checkerTypeString(def.ReturnType); ret != "" && ret != "Void" {
		b.WriteString(" ")
		b.WriteString(ret)
	}
	return b.String()
}

func paramListString(def *checker.FunctionDef) string {
	parts := make([]string, 0, len(def.Parameters))
	for _, p := range def.Parameters {
		text := mutParamTypeString(checkerTypeString(p.Type), p.Mutable)
		if p.Name != "" && !strings.HasPrefix(p.Name, "arg") {
			text = p.Name + ": " + text
		}
		parts = append(parts, text)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// methodDefFromAnalysis resolves a method definition by owner and name from
// the checked module view.
func methodDefFromAnalysis(fa *analysis.FileAnalysis, owner string, name string) *checker.FunctionDef {
	if fa.Module == nil {
		return nil
	}
	sym := fa.Module.Get(owner)
	if sym.IsZero() {
		return nil
	}
	switch ownerType := sym.Type.(type) {
	case *checker.StructDef:
		if fa.Checked != nil {
			if def, ok := fa.Checked.StructMethod(checker.StructMethodOwner(ownerType), name); ok {
				return def
			}
			return checker.StructMethodsInModules(fa.Checked.Imports, checker.StructMethodOwner(ownerType))[name]
		}
	case *checker.Enum:
		return ownerType.Methods[name]
	}
	return nil
}
