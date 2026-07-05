package lsp

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/lsp/analysis"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// definitionFromSpans resolves go-to-definition using the checker's span
// table (ADR 0043). It returns nil when the position resolves to nothing,
// letting callers fall back to legacy heuristics during the migration.
func (s *Server) definitionFromSpans(ctx context.Context, docURI uri.URI, position protocol.Position) []protocol.Location {
	fa, err := s.analyzeSnapshot(ctx, docURI)
	if err != nil || fa == nil || fa.Spans == nil {
		return nil
	}
	filePath, err := filePathFromURI(docURI)
	if err != nil {
		return nil
	}

	point := parse.Point{Row: int(position.Line) + 1, Col: int(position.Character) + 1}
	for _, rec := range fa.Spans.At(point) {
		// Same-file identity: def recorded in this file's span table.
		if rec.Key != nil {
			if def, ok := fa.Spans.Def(rec.Key); ok {
				loc := narrowDefLocation(def.Loc)
				return []protocol.Location{{
					URI:   protocol.DocumentURI(docURI),
					Range: parseLocationToLSPRange(loc),
				}}
			}
		}
		// Cross-module target: analyze the target module and find the decl.
		if rec.Target != nil {
			if loc := s.crossModuleDefinition(filePath, rec.Target); loc != nil {
				return loc
			}
		}
	}
	return nil
}

// narrowDefLocation trims a whole-declaration def span down to a point at
// the declaration start when the AST does not carry a precise name location.
func narrowDefLocation(loc parse.Location) parse.Location {
	if loc.Start.Row == loc.End.Row || loc.End.Row == 0 {
		return loc
	}
	// Multi-line span (e.g. a variable declaration with a block initializer):
	// keep the start position, collapse to a point on the first line.
	return parse.Location{Start: loc.Start, End: parse.Point{Row: loc.Start.Row, Col: loc.Start.Col}}
}

// crossModuleDefinition resolves a SpanTarget to a location in its defining
// module's file.
func (s *Server) crossModuleDefinition(fromFile string, target *checker.SpanTarget) []protocol.Location {
	snap := s.workspaceFor(fromFile).Snapshot()

	// The checker records the resolved file path at check time; prefer it
	// over re-resolving the module path.
	depPath := target.File
	if depPath == "" {
		depPath = s.resolveModuleFile(snap, target.Module)
	}
	if depPath == "" {
		return nil
	}
	program, parseErrs, err := snap.Parse(depPath)
	if err != nil || program == nil || len(parseErrs) > 0 {
		return nil
	}

	loc, ok := findDeclLocation(program, target)
	if !ok {
		return nil
	}
	return []protocol.Location{{
		URI:   protocol.DocumentURI(uri.File(depPath)),
		Range: parseLocationToLSPRange(narrowDefLocation(loc)),
	}}
}

// resolveModuleFile maps a canonical module path to a file path as a
// fallback when the span target carries no file.
func (s *Server) resolveModuleFile(snap *analysis.Snapshot, modulePath string) string {
	if modulePath == "" || strings.HasPrefix(modulePath, "ard/") {
		return ""
	}
	root := snap.Engine().ProjectRoot()
	candidate := filepath.Join(root, modulePath+".ard")
	if _, err := snap.Content(candidate); err == nil {
		return candidate
	}
	return ""
}

// findDeclLocation locates a target entity's declaration in a parsed module.
func findDeclLocation(program *parse.Program, target *checker.SpanTarget) (parse.Location, bool) {
	for _, stmt := range program.Statements {
		switch decl := stmt.(type) {
		case *parse.FunctionDeclaration:
			if target.Kind == checker.TargetFunction && decl.Name == target.Symbol {
				return decl.GetLocation(), true
			}
		case *parse.StructDefinition:
			switch target.Kind {
			case checker.TargetType:
				if decl.Name.Name == target.Symbol {
					return decl.Name.GetLocation(), true
				}
			case checker.TargetField:
				if decl.Name.Name == target.Owner {
					for _, field := range decl.Fields {
						if field.Name.Name == target.Symbol {
							return field.Name.GetLocation(), true
						}
					}
				}
			}
		case *parse.EnumDefinition:
			if target.Kind == checker.TargetType && decl.Name == target.Symbol {
				return decl.GetLocation(), true
			}
		case *parse.TraitDefinition:
			if target.Kind == checker.TargetType && decl.Name.Name == target.Symbol {
				return decl.Name.GetLocation(), true
			}
		case *parse.TypeDeclaration:
			if target.Kind == checker.TargetType && decl.Name.Name == target.Symbol {
				return decl.Name.GetLocation(), true
			}
		case *parse.ImplBlock:
			if target.Kind == checker.TargetMethod && decl.Target.Name == target.Owner {
				for i := range decl.Methods {
					if decl.Methods[i].Name == target.Symbol {
						return decl.Methods[i].GetLocation(), true
					}
				}
			}
		case *parse.TraitImplementation:
			if target.Kind == checker.TargetMethod && decl.ForType.Name == target.Owner {
				for i := range decl.Methods {
					if decl.Methods[i].Name == target.Symbol {
						return decl.Methods[i].GetLocation(), true
					}
				}
			}
		}
	}
	return parse.Location{}, false
}

// parseLocationToLSPRange converts a 1-based parse location to a 0-based LSP
// range.
func parseLocationToLSPRange(loc parse.Location) protocol.Range {
	start := protocol.Position{}
	if loc.Start.Row > 0 {
		start.Line = uint32(loc.Start.Row - 1)
	}
	if loc.Start.Col > 0 {
		start.Character = uint32(loc.Start.Col - 1)
	}
	end := start
	if loc.End.Row > 0 {
		end = protocol.Position{Line: uint32(loc.End.Row - 1)}
		if loc.End.Col > 0 {
			end.Character = uint32(loc.End.Col - 1)
		}
	}
	return protocol.Range{Start: start, End: end}
}
