package lsp

import (
	"context"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/lsp/analysis"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// signatureHelpFromSpans resolves signature help through the analysis
// engine: the enclosing call's checked definition provides the label and
// parameters. Returns nil to fall back to legacy heuristics.
func (s *Server) signatureHelpFromSpans(ctx context.Context, docURI uri.URI, source string, position protocol.Position) *protocol.SignatureHelp {
	filePath, err := filePathFromURI(docURI)
	if err != nil {
		return nil
	}

	// Complete calls analyze as-is; in-progress calls (unbalanced parens or
	// a dangling comma) need the legacy paren-patching to parse. Patching a
	// valid call would corrupt it, so try the plain source first.
	ws := s.workspaceFor(filePath)
	scratch := analysis.NewWorkspace(ws.Engine())
	for _, doc := range s.cache.Snapshot() {
		if p, err := filePathFromURI(doc.URI); err == nil && p != filePath {
			scratch.SetOverlay(p, doc.Text)
		}
	}
	analyzeWith := func(text string, allowPartial bool) *analysis.FileAnalysis {
		scratch.SetOverlay(filePath, text)
		fa, err := scratch.Snapshot().AnalyzeEphemeral(ctx, filePath)
		if err != nil || fa == nil || fa.Spans == nil {
			return nil
		}
		if !allowPartial && len(fa.ParseErrors) > 0 {
			return nil
		}
		return fa
	}
	fa := analyzeWith(source, false)
	if fa == nil {
		// In-progress call: paren-patch and best-effort check the partial AST.
		fa = analyzeWith(signatureParseSource(source, position), true)
	}
	if fa == nil {
		return nil
	}

	target := s.docLinesFor(filePath).positionToPoint(position)
	for _, rec := range fa.Spans.At(target) {
		label, params, callLoc, callArgs, ok := signatureFromRecord(rec)
		if !ok {
			continue
		}
		activeParam := activeParameterIndex(source, target, callLoc, callArgs, params)
		return &protocol.SignatureHelp{
			Signatures: []protocol.SignatureInformation{
				{
					Label:           label,
					Parameters:      signatureParameterInformation(params),
					ActiveParameter: activeParam,
				},
			},
			ActiveSignature: 0,
			ActiveParameter: activeParam,
		}
	}
	return nil
}

// signatureFromRecord extracts a call signature from a span record whose
// source is a call expression.
func signatureFromRecord(rec checker.SpanRecord) (string, []hoverParam, parse.Location, []parse.Argument, bool) {
	if rec.Node == nil || rec.Source == nil {
		return "", nil, parse.Location{}, nil, false
	}

	var def *checker.FunctionDef
	var label string
	voidLabel := func(base string, d *checker.FunctionDef) string {
		// Signature help spells out Void returns, matching editor
		// conventions for callable tooltips.
		if ret := checkerTypeString(d.ReturnType); ret == "" || ret == "Void" {
			return base + " Void"
		}
		return base
	}

	switch node := rec.Node.(type) {
	case *checker.FunctionCall:
		def = node.Definition()
		if def != nil {
			label = voidLabel(functionSignatureString(node.Name, def), def)
		}
	case *checker.InstanceMethod:
		if node.Method != nil {
			def = node.Method.Definition()
			if def != nil {
				label = voidLabel(methodSignatureString(instanceMethodOwner(node), def), def)
			}
		}
	case *checker.ForeignFunctionCall:
		if node.Call != nil {
			def = node.Call.Definition()
			if def != nil {
				label = voidLabel(functionSignatureString(node.Qualifier+"::"+node.Symbol, def), def)
			}
		}
	default:
		if recv, name, ok := checker.BuiltinMethodInfo(rec.Node); ok && name != "" {
			def = checker.BuiltinMethodDef(recv, name)
			if def != nil {
				label = voidLabel(methodSignatureString(checkerTypeString(recv), def), def)
			}
		}
	}
	if def == nil || label == "" {
		return "", nil, parse.Location{}, nil, false
	}

	loc, args, ok := callSiteOf(rec.Source)
	if !ok {
		return "", nil, parse.Location{}, nil, false
	}

	params := make([]hoverParam, len(def.Parameters))
	for i, p := range def.Parameters {
		params[i] = hoverParam{Name: p.Name, Type: checkerTypeString(p.Type), Mutable: p.Mutable}
	}
	return label, params, loc, args, true
}

// callSiteOf extracts the call location and arguments from a parse node.
func callSiteOf(source parse.Expression) (parse.Location, []parse.Argument, bool) {
	switch call := source.(type) {
	case *parse.FunctionCall:
		return call.GetLocation(), call.Args, true
	case *parse.FunctionValueCall:
		return call.GetLocation(), call.Args, true
	case *parse.InstanceMethod:
		return call.Method.GetLocation(), call.Method.Args, true
	case *parse.StaticFunction:
		return call.Function.GetLocation(), call.Function.Args, true
	}
	return parse.Location{}, nil, false
}
