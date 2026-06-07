package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

type referenceDocument struct {
	filePath string
	source   string
	prog     *parse.Program
}

type referenceResolvedTarget struct {
	kind string
	name string
	def  *definitionTarget
}

func computeReferences(source string, filePath string, position protocol.Position, includeDeclaration bool) []protocol.Location {
	return computeReferencesWithOverlays(source, filePath, position, includeDeclaration, nil)
}

func computeReferencesWithOverlays(source string, filePath string, position protocol.Position, includeDeclaration bool, overlays map[string]string) []protocol.Location {
	target := lspPositionToParsePoint(position)
	prog := parseAndCache(source, filePath)
	if prog == nil {
		return nil
	}

	var def *definitionTarget
	var targetKind string
	var targetName string
	if resolved := findReferenceDeclarationTarget(prog.Statements, target, filePath, source, prog); resolved != nil {
		def = resolved.def
		targetKind = resolved.kind
		targetName = resolved.name
	} else {
		expr := findInStmts(prog.Statements, target)
		if expr == nil {
			return nil
		}

		def = definitionForExpr(expr, prog, filePath)
		if def == nil {
			return nil
		}
		targetKind = referenceTargetKind(expr, prog, filePath, def)
		targetName = referenceExprName(expr)
	}

	refs := []protocol.Location{}
	seen := map[string]bool{}
	add := func(filePath string, loc parse.Location) {
		if loc.Start.Row == 0 && loc.Start.Col == 0 {
			return
		}
		if !includeDeclaration && sameDefinitionTarget(def, &definitionTarget{filePath: filePath, loc: loc}) {
			return
		}
		rng := checkerLocationToLSPRange(loc)
		key := fmt.Sprintf("%s:%d:%d:%d:%d", filePath, rng.Start.Line, rng.Start.Character, rng.End.Line, rng.End.Character)
		if seen[key] {
			return
		}
		seen[key] = true
		refs = append(refs, protocol.Location{URI: uri.File(filePath), Range: rng})
	}

	targetName = referenceNameTail(targetName)

	if includeDeclaration {
		add(def.filePath, def.loc)
	}
	for _, doc := range referenceDocuments(source, filePath, prog, def, overlays) {
		scanReferenceDocument(doc, targetKind, targetName, def, add)
	}
	return refs
}

func findReferenceDeclarationTarget(stmts []parse.Statement, target parse.Point, filePath string, source string, prog *parse.Program) *referenceResolvedTarget {
	for _, stmt := range stmts {
		if stmt == nil || !pointInRange(target, stmt.GetLocation()) {
			continue
		}
		switch s := stmt.(type) {
		case *parse.StructDefinition:
			nameLoc := typeDefinitionNameLocation(stmt, s.Name.Location, s.Name.Name, len("struct "))
			if pointInRange(target, nameLoc) {
				return &referenceResolvedTarget{kind: "type", name: s.Name.Name, def: &definitionTarget{filePath: filePath, loc: nameLoc}}
			}
			for _, field := range s.Fields {
				if pointInRange(target, field.Name.Location) {
					return &referenceResolvedTarget{kind: "instanceProperty", name: field.Name.Name, def: &definitionTarget{filePath: filePath, loc: field.Name.Location}}
				}
				if resolved := findReferenceDeclaredTypeTarget(field.Type, target, filePath, prog); resolved != nil {
					return resolved
				}
			}
		case *parse.TypeDeclaration:
			nameLoc := typeDefinitionNameLocation(stmt, s.Name.Location, s.Name.Name, len("type "))
			if pointInRange(target, nameLoc) {
				return &referenceResolvedTarget{kind: "type", name: s.Name.Name, def: &definitionTarget{filePath: filePath, loc: nameLoc}}
			}
			for _, declared := range s.Type {
				if resolved := findReferenceDeclaredTypeTarget(declared, target, filePath, prog); resolved != nil {
					return resolved
				}
			}
		case *parse.TraitDefinition:
			nameLoc := typeDefinitionNameLocation(stmt, s.Name.Location, s.Name.Name, len("trait "))
			if pointInRange(target, nameLoc) {
				return &referenceResolvedTarget{kind: "type", name: s.Name.Name, def: &definitionTarget{filePath: filePath, loc: nameLoc}}
			}
			for i := range s.Methods {
				if resolved := findReferenceDeclarationTarget([]parse.Statement{&s.Methods[i]}, target, filePath, source, prog); resolved != nil {
					return resolved
				}
			}
		case *parse.ImplBlock:
			if pointInRange(target, s.Target.Location) {
				if def := definitionForTypeName(s.Target.Name, prog, filePath); def != nil {
					return &referenceResolvedTarget{kind: "type", name: s.Target.Name, def: def}
				}
			}
			for i := range s.Methods {
				if resolved := findReferenceDeclarationTarget([]parse.Statement{&s.Methods[i]}, target, filePath, source, prog); resolved != nil {
					return resolved
				}
			}
		case *parse.TraitImplementation:
			if pointInRange(target, s.ForType.Location) {
				if def := definitionForTypeName(s.ForType.Name, prog, filePath); def != nil {
					return &referenceResolvedTarget{kind: "type", name: s.ForType.Name, def: def}
				}
			}
			for i := range s.Methods {
				if resolved := findReferenceDeclarationTarget([]parse.Statement{&s.Methods[i]}, target, filePath, source, prog); resolved != nil {
					return resolved
				}
			}
		case *parse.FunctionDeclaration:
			for _, param := range s.Parameters {
				if resolved := findReferenceDeclaredTypeTarget(param.Type, target, filePath, prog); resolved != nil {
					return resolved
				}
			}
			if resolved := findReferenceDeclaredTypeTarget(s.ReturnType, target, filePath, prog); resolved != nil {
				return resolved
			}
			if resolved := findReferenceDeclarationTarget(s.Body, target, filePath, source, prog); resolved != nil {
				return resolved
			}
		case *parse.StaticFunctionDeclaration:
			if s.Path.Target != nil && pointInRange(target, s.Path.Target.GetLocation()) {
				name := simpleExprName(s.Path.Target)
				if def := definitionForTypeName(name, prog, filePath); def != nil {
					return &referenceResolvedTarget{kind: "type", name: name, def: def}
				}
			}
			if resolved := findReferenceDeclarationTarget([]parse.Statement{&s.FunctionDeclaration}, target, filePath, source, prog); resolved != nil {
				return resolved
			}
		case *parse.VariableDeclaration:
			nameLoc := documentHighlightNameLocation(source, s.Location, s.Name)
			if pointInRange(target, nameLoc) {
				return &referenceResolvedTarget{kind: "name", name: s.Name, def: &definitionTarget{filePath: filePath, loc: s.Location}}
			}
			if resolved := findReferenceDeclaredTypeTarget(s.Type, target, filePath, prog); resolved != nil {
				return resolved
			}
		case *parse.ExternalFunction:
			for _, param := range s.Parameters {
				if resolved := findReferenceDeclaredTypeTarget(param.Type, target, filePath, prog); resolved != nil {
					return resolved
				}
			}
			if resolved := findReferenceDeclaredTypeTarget(s.ReturnType, target, filePath, prog); resolved != nil {
				return resolved
			}
		case *parse.IfStatement:
			if resolved := findReferenceDeclarationTarget(s.Body, target, filePath, source, prog); resolved != nil {
				return resolved
			}
			if s.Else != nil {
				if resolved := findReferenceDeclarationTarget([]parse.Statement{s.Else}, target, filePath, source, prog); resolved != nil {
					return resolved
				}
			}
		case *parse.WhileLoop:
			if resolved := findReferenceDeclarationTarget(s.Body, target, filePath, source, prog); resolved != nil {
				return resolved
			}
		case *parse.ForInLoop:
			if resolved := findReferenceDeclarationTarget(s.Body, target, filePath, source, prog); resolved != nil {
				return resolved
			}
		case *parse.RangeLoop:
			if resolved := findReferenceDeclarationTarget(s.Body, target, filePath, source, prog); resolved != nil {
				return resolved
			}
		case *parse.ForLoop:
			if resolved := findReferenceDeclarationTarget(s.Body, target, filePath, source, prog); resolved != nil {
				return resolved
			}
		case *parse.BlockExpression:
			if resolved := findReferenceDeclarationTarget(s.Statements, target, filePath, source, prog); resolved != nil {
				return resolved
			}
		}
	}
	return nil
}

func findReferenceDeclaredTypeTarget(declared parse.DeclaredType, target parse.Point, filePath string, prog *parse.Program) *referenceResolvedTarget {
	if declared == nil || !pointInRange(target, declared.GetLocation()) {
		return nil
	}
	for _, child := range declaredTypeChildren(declared) {
		if resolved := findReferenceDeclaredTypeTarget(child, target, filePath, prog); resolved != nil {
			return resolved
		}
	}
	if def := definitionForDeclaredType(declared, prog, filePath); def != nil {
		return &referenceResolvedTarget{kind: "type", name: declaredTypeReferenceName(declared), def: def}
	}
	return nil
}

func referenceTargetKind(expr parse.Expression, prog *parse.Program, filePath string, def *definitionTarget) string {
	kind := referenceExprKind(expr, prog)
	if kind == "moduleAlias" {
		return kind
	}
	if definitionTargetIsType(def, prog, filePath) {
		return "type"
	}
	return kind
}

func definitionTargetIsType(def *definitionTarget, currentProg *parse.Program, currentFile string) bool {
	if def == nil {
		return false
	}
	prog := currentProg
	if cleanReferencePath(def.filePath) != cleanReferencePath(currentFile) {
		doc, ok := readReferenceDocument(def.filePath, nil)
		if !ok {
			return false
		}
		prog = doc.prog
	}
	if prog == nil {
		return false
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *parse.StructDefinition:
			if sameParseLocation(def.loc, typeDefinitionNameLocation(stmt, s.Name.Location, s.Name.Name, len("struct "))) {
				return true
			}
		case *parse.TypeDeclaration:
			if sameParseLocation(def.loc, typeDefinitionNameLocation(stmt, s.Name.Location, s.Name.Name, len("type "))) {
				return true
			}
		case *parse.TraitDefinition:
			if sameParseLocation(def.loc, typeDefinitionNameLocation(stmt, s.Name.Location, s.Name.Name, len("trait "))) {
				return true
			}
		case *parse.EnumDefinition:
			if sameParseLocation(def.loc, typeDefinitionNameLocation(stmt, parse.Location{}, s.Name, len("enum "))) {
				return true
			}
		case *parse.ExternTypeDeclaration:
			if sameParseLocation(def.loc, typeDefinitionNameLocation(stmt, parse.Location{}, s.Name, len("extern type "))) {
				return true
			}
		}
	}
	return false
}

func referenceDocuments(currentSource string, currentFile string, currentProg *parse.Program, targetDef *definitionTarget, overlays map[string]string) []referenceDocument {
	docs := []referenceDocument{{filePath: currentFile, source: currentSource, prog: currentProg}}
	seen := map[string]bool{cleanReferencePath(currentFile): true}

	for _, doc := range projectReferenceDocuments(currentFile, overlays) {
		key := cleanReferencePath(doc.filePath)
		if seen[key] {
			continue
		}
		seen[key] = true
		docs = append(docs, doc)
	}

	if targetDef != nil && targetDef.filePath != "" {
		key := cleanReferencePath(targetDef.filePath)
		if !seen[key] {
			if doc, ok := readReferenceDocument(targetDef.filePath, overlays); ok {
				docs = append(docs, doc)
			}
		}
	}

	return docs
}

func projectReferenceDocuments(currentFile string, overlays map[string]string) []referenceDocument {
	if currentFile == "" {
		return nil
	}
	project, err := checker.FindProjectRoot(filepath.Dir(currentFile))
	if err != nil || project == nil || project.RootPath == "" {
		return nil
	}

	paths := []string{}
	_ = filepath.WalkDir(project.RootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".ard", "ard-out", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".ard" {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)

	docs := make([]referenceDocument, 0, len(paths))
	pathSet := map[string]bool{}
	for _, path := range paths {
		pathSet[cleanReferencePath(path)] = true
	}
	for path := range normalizedReferenceOverlays(overlays) {
		if pathSet[path] || filepath.Ext(path) != ".ard" || !referencePathInRoot(path, project.RootPath) {
			continue
		}
		paths = append(paths, path)
		pathSet[path] = true
	}
	sort.Strings(paths)

	for _, path := range paths {
		if doc, ok := readReferenceDocument(path, overlays); ok {
			docs = append(docs, doc)
		}
	}
	return docs
}

func readReferenceDocument(filePath string, overlays map[string]string) (referenceDocument, bool) {
	if source, ok := normalizedReferenceOverlays(overlays)[cleanReferencePath(filePath)]; ok {
		result := parse.Parse([]byte(source), filePath)
		if result.Program == nil {
			return referenceDocument{}, false
		}
		return referenceDocument{filePath: filePath, source: source, prog: result.Program}, true
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return referenceDocument{}, false
	}
	result := parse.Parse(data, filePath)
	if result.Program == nil {
		return referenceDocument{}, false
	}
	return referenceDocument{filePath: filePath, source: string(data), prog: result.Program}, true
}

func cleanReferencePath(filePath string) string {
	if filePath == "" {
		return ""
	}
	if abs, err := filepath.Abs(filePath); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(filePath)
}

func normalizedReferenceOverlays(overlays map[string]string) map[string]string {
	if len(overlays) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(overlays))
	for path, source := range overlays {
		normalized[cleanReferencePath(path)] = source
	}
	return normalized
}

func referencePathInRoot(path string, root string) bool {
	path = cleanReferencePath(path)
	root = cleanReferencePath(root)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func scanReferenceDocument(doc referenceDocument, targetKind string, targetName string, targetDef *definitionTarget, add func(string, parse.Location)) {
	if doc.prog == nil {
		return
	}
	visitReferenceStatements(doc.prog.Statements, doc.filePath, doc.source, doc.prog, doc.filePath, targetKind, targetName, targetDef, add)
}

func visitReferenceStatements(stmts []parse.Statement, currentFile string, source string, prog *parse.Program, rootFile string, targetKind string, targetName string, targetDef *definitionTarget, add func(string, parse.Location)) {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		visitReferenceStatement(stmt, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	}
}

func visitReferenceStatement(stmt parse.Statement, currentFile string, source string, prog *parse.Program, rootFile string, targetKind string, targetName string, targetDef *definitionTarget, add func(string, parse.Location)) {
	if expr, ok := stmt.(parse.Expression); ok {
		visitReferenceExpr(expr, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	}

	switch s := stmt.(type) {
	case *parse.VariableDeclaration:
		visitReferenceDeclaredType(s.Type, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceExpr(s.Value, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.VariableAssignment:
		visitReferenceExpr(s.Target, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceExpr(s.Value, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.FunctionDeclaration:
		for i := range s.Parameters {
			visitReferenceExpr(&s.Parameters[i], currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
			visitReferenceDeclaredType(s.Parameters[i].Type, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
		visitReferenceDeclaredType(s.ReturnType, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceStatements(s.Body, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.StaticFunctionDeclaration:
		visitReferenceExpr(s.Path.Target, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceStatement(&s.FunctionDeclaration, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.ImplBlock:
		visitReferenceExpr(&s.Target, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceExpr(&s.Receiver, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		for i := range s.Methods {
			visitReferenceStatement(&s.Methods[i], currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.TraitImplementation:
		visitReferenceExpr(s.Trait, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceExpr(&s.ForType, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceExpr(&s.Receiver, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		for i := range s.Methods {
			visitReferenceStatement(&s.Methods[i], currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.TraitDefinition:
		visitReferenceExpr(&s.Name, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		for i := range s.Methods {
			visitReferenceStatement(&s.Methods[i], currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.StructDefinition:
		visitReferenceExpr(&s.Name, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		for _, field := range s.Fields {
			visitReferenceDeclaredType(field.Type, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.TypeDeclaration:
		visitReferenceExpr(&s.Name, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		for _, declared := range s.Type {
			visitReferenceDeclaredType(declared, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.ExternalFunction:
		for i := range s.Parameters {
			visitReferenceExpr(&s.Parameters[i], currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
			visitReferenceDeclaredType(s.Parameters[i].Type, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
		visitReferenceDeclaredType(s.ReturnType, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.IfStatement:
		visitReferenceExpr(s.Condition, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceStatements(s.Body, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		if s.Else != nil {
			visitReferenceStatement(s.Else, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.WhileLoop:
		visitReferenceExpr(s.Condition, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceStatements(s.Body, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.ForInLoop:
		visitReferenceExpr(&s.Cursor, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceExpr(&s.Cursor2, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceExpr(s.Iterable, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceStatements(s.Body, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.RangeLoop:
		visitReferenceExpr(&s.Cursor, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceExpr(&s.Cursor2, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceExpr(s.Start, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceExpr(s.End, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceStatements(s.Body, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.ForLoop:
		if s.Init != nil {
			visitReferenceStatement(s.Init, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
		visitReferenceExpr(s.Condition, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		if s.Incrementer != nil {
			visitReferenceStatement(s.Incrementer, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
		visitReferenceStatements(s.Body, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.MatchExpression:
		visitReferenceExpr(s.Subject, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		for _, matchCase := range s.Cases {
			visitReferenceExpr(matchCase.Pattern, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
			visitReferenceStatements(matchCase.Body, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.ConditionalMatchExpression:
		for _, matchCase := range s.Cases {
			visitReferenceExpr(matchCase.Condition, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
			visitReferenceStatements(matchCase.Body, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.BlockExpression:
		visitReferenceStatements(s.Statements, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.Try:
		visitReferenceExpr(s.Expression, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		if s.CatchVar != nil {
			visitReferenceExpr(s.CatchVar, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
		visitReferenceStatements(s.CatchBlock, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.StructInstance:
		visitReferenceExpr(&s.Name, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		for _, prop := range s.Properties {
			visitReferenceExpr(prop.Value, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	}
}

func visitReferenceExpr(expr parse.Expression, currentFile string, source string, prog *parse.Program, rootFile string, targetKind string, targetName string, targetDef *definitionTarget, add func(string, parse.Location)) {
	if expr == nil {
		return
	}
	candidateName := referenceExprName(expr)
	if referenceNamesMatch(targetName, candidateName) && referenceExprCompatible(targetKind, expr, currentFile, prog, targetDef) {
		if def := definitionForReferenceCandidate(expr, targetKind, targetName, targetDef, prog, rootFile); sameDefinitionTarget(def, targetDef) {
			add(currentFile, referenceExprLocation(expr))
		}
	}

	switch e := expr.(type) {
	case *parse.BinaryExpression:
		visitReferenceExpr(e.Left, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceExpr(e.Right, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.UnaryExpression:
		visitReferenceExpr(e.Operand, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.FunctionCall:
		for _, arg := range e.Args {
			visitReferenceExpr(arg.Value, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.InstanceProperty:
		visitReferenceExpr(e.Target, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.InstanceMethod:
		visitReferenceExpr(e.Target, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		for _, arg := range e.Method.Args {
			visitReferenceExpr(arg.Value, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.StaticProperty:
		visitReferenceExpr(e.Target, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceExpr(e.Property, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.StaticFunction:
		visitReferenceExpr(e.Target, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		for _, arg := range e.Function.Args {
			visitReferenceExpr(arg.Value, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.ListLiteral:
		for _, item := range e.Items {
			visitReferenceExpr(item, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.MapLiteral:
		for _, entry := range e.Entries {
			visitReferenceExpr(entry.Key, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
			visitReferenceExpr(entry.Value, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.MatchExpression:
		visitReferenceExpr(e.Subject, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		for _, matchCase := range e.Cases {
			visitReferenceExpr(matchCase.Pattern, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
			visitReferenceStatements(matchCase.Body, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.ConditionalMatchExpression:
		for _, matchCase := range e.Cases {
			visitReferenceExpr(matchCase.Condition, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
			visitReferenceStatements(matchCase.Body, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.Try:
		visitReferenceExpr(e.Expression, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		if e.CatchVar != nil {
			visitReferenceExpr(e.CatchVar, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
		visitReferenceStatements(e.CatchBlock, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.BlockExpression:
		visitReferenceStatements(e.Statements, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	case *parse.InterpolatedStr:
		for _, chunk := range e.Chunks {
			visitReferenceExpr(chunk, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.IfStatement:
		visitReferenceExpr(e.Condition, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceStatements(e.Body, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		if e.Else != nil {
			visitReferenceStatement(e.Else, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.StructInstance:
		visitReferenceExpr(&e.Name, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		for _, prop := range e.Properties {
			if targetKind == "instanceProperty" && referenceNamesMatch(targetName, prop.Name.Name) {
				if def := definitionForStructValueField(e, prop.Name.Name, prog, rootFile); sameDefinitionTarget(def, targetDef) {
					add(currentFile, structValueNameLocation(prop, source))
				}
			}
			visitReferenceExpr(prop.Value, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
	case *parse.AnonymousFunction:
		for i := range e.Parameters {
			visitReferenceExpr(&e.Parameters[i], currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
			visitReferenceDeclaredType(e.Parameters[i].Type, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		}
		visitReferenceDeclaredType(e.ReturnType, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
		visitReferenceStatements(e.Body, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	}
}

func definitionForReferenceCandidate(expr parse.Expression, targetKind string, targetName string, targetDef *definitionTarget, prog *parse.Program, filePath string) *definitionTarget {
	if targetKind == "type" {
		switch e := expr.(type) {
		case *parse.Identifier:
			if !referenceNamesMatch(targetName, e.Name) {
				return nil
			}
			return definitionForTypeName(e.Name, prog, filePath)
		case *parse.StaticProperty:
			if !referenceNamesMatch(targetName, referenceExprName(e.Property)) {
				return nil
			}
			if staticPropertyReferencesTargetFile(e, targetDef, prog, filePath) {
				return targetDef
			}
			return definitionForTypeName(simpleExprName(e), prog, filePath)
		case *parse.StructInstance:
			if !referenceNamesMatch(targetName, e.Name.Name) {
				return nil
			}
			return definitionForTypeName(e.Name.Name, prog, filePath)
		}
	}
	return definitionForExpr(expr, prog, filePath)
}

func staticPropertyReferencesTargetFile(sp *parse.StaticProperty, targetDef *definitionTarget, prog *parse.Program, filePath string) bool {
	if sp == nil || targetDef == nil {
		return false
	}
	alias := simpleExprName(sp.Target)
	if alias == "" {
		return false
	}
	modulePath, ok := modulePathForAlias(alias, prog, filePath)
	return ok && cleanReferencePath(modulePath) == cleanReferencePath(targetDef.filePath)
}

func modulePathForAlias(alias string, prog *parse.Program, filePath string) (string, bool) {
	if prog == nil {
		return "", false
	}
	for _, imp := range prog.Imports {
		if imp.Name != alias {
			continue
		}
		return modulePathForImport(imp, filePath)
	}
	if path := preludeModulePath(alias); path != "" {
		return stdLibSourcePath(path), true
	}
	return "", false
}

func modulePathForImport(imp parse.Import, filePath string) (string, bool) {
	return resolveModulePathForImport(imp, filePath)
}

func resolveModulePathForImport(imp parse.Import, filePath string) (string, bool) {
	if strings.HasPrefix(imp.Path, "ard/") {
		return stdLibSourcePath(imp.Path), true
	}
	if filePath == "" {
		return "", false
	}
	moduleResolver, err := checker.NewModuleResolver(filepath.Dir(filePath))
	if err != nil {
		return "", false
	}
	moduleFile, err := moduleResolver.ResolveImportPath(imp.Path)
	if err != nil {
		return "", false
	}
	return moduleFile, true
}

func referenceExprName(expr parse.Expression) string {
	switch e := expr.(type) {
	case *parse.Identifier:
		return e.Name
	case *parse.Parameter:
		return e.Name
	case *parse.VariableDeclaration:
		return e.Name
	case *parse.FunctionDeclaration:
		return e.Name
	case *parse.FunctionCall:
		return e.Name
	case *parse.ExternalFunction:
		return e.Name
	case *parse.StaticFunctionDeclaration:
		return e.Name
	case *parse.StaticFunction:
		return e.Function.Name
	case *parse.StaticProperty:
		return referenceExprName(e.Property)
	case *parse.InstanceProperty:
		return e.Property.Name
	case *parse.InstanceMethod:
		return e.Method.Name
	case *parse.StructInstance:
		return e.Name.Name
	case *parse.StructDefinition:
		return e.Name.Name
	case *parse.EnumDefinition:
		return e.Name
	case *parse.TraitDefinition:
		return e.Name.Name
	case *parse.TypeDeclaration:
		return e.Name.Name
	case *parse.ExternTypeDeclaration:
		return e.Name
	}
	return ""
}

func declaredTypeReferenceName(declared parse.DeclaredType) string {
	switch t := declared.(type) {
	case *parse.MutableType:
		return declaredTypeReferenceName(t.Inner)
	case *parse.CustomType:
		return t.Name
	case *parse.List:
		return declaredTypeReferenceName(t.Element)
	case *parse.Map:
		return declaredTypeReferenceName(t.Value)
	case *parse.ResultType:
		if name := declaredTypeReferenceName(t.Val); name != "" {
			return name
		}
		return declaredTypeReferenceName(t.Err)
	case *parse.FunctionType:
		if name := declaredTypeReferenceName(t.Return); name != "" {
			return name
		}
		for _, param := range t.Params {
			if name := declaredTypeReferenceName(param); name != "" {
				return name
			}
		}
	}
	return ""
}

func referenceNamesMatch(targetName string, candidateName string) bool {
	if targetName == "" {
		return true
	}
	if candidateName == "" {
		return false
	}
	if candidateName == targetName {
		return true
	}
	if !strings.ContainsAny(candidateName, ":?<> ") {
		return false
	}
	return referenceNameTail(candidateName) == targetName
}

func referenceNameTail(name string) string {
	name = normalizeDisplayType(strings.TrimSpace(name))
	name = strings.TrimSuffix(name, "?")
	if genericStart := strings.Index(name, "<"); genericStart >= 0 {
		name = name[:genericStart]
	}
	if idx := strings.LastIndex(name, "::"); idx >= 0 {
		name = name[idx+2:]
	}
	return name
}

func referenceExprCompatible(targetKind string, candidate parse.Expression, currentFile string, prog *parse.Program, targetDef *definitionTarget) bool {
	candidateKind := referenceExprKind(candidate, prog)
	switch targetKind {
	case "moduleAlias":
		return candidateKind == "moduleAlias"
	case "name":
		// Top-level variables can be referenced as module::name from importers.
		return candidateKind == "name" || candidateKind == "staticProperty"
	case "function":
		// Functions may be plain calls, module::function calls, or methods when
		// the declaration is inside an impl/trait block.
		return candidateKind == "function" || candidateKind == "staticFunction" || candidateKind == "instanceMethod"
	case "staticFunction":
		return candidateKind == "staticFunction" || (candidateKind == "function" && targetDef != nil && currentFile == targetDef.filePath)
	case "staticProperty":
		return candidateKind == "staticProperty" || (candidateKind == "name" && targetDef != nil && currentFile == targetDef.filePath)
	case "instanceProperty":
		return candidateKind == "instanceProperty"
	case "instanceMethod":
		return candidateKind == "instanceMethod"
	case "type":
		return candidateKind == "type" || candidateKind == "name" || candidateKind == "staticProperty"
	default:
		return targetKind == candidateKind
	}
}

func referenceExprKind(expr parse.Expression, prog *parse.Program) string {
	switch e := expr.(type) {
	case *parse.Identifier:
		if isModuleAliasName(e.Name, prog) {
			return "moduleAlias"
		}
		return "name"
	case *parse.Parameter, *parse.VariableDeclaration:
		return "name"
	case *parse.FunctionDeclaration, *parse.FunctionCall, *parse.StaticFunctionDeclaration, *parse.ExternalFunction:
		return "function"
	case *parse.StaticFunction:
		return "staticFunction"
	case *parse.StaticProperty:
		return "staticProperty"
	case *parse.InstanceProperty:
		return "instanceProperty"
	case *parse.InstanceMethod:
		return "instanceMethod"
	case *parse.StructInstance, *parse.StructDefinition, *parse.EnumDefinition, *parse.TraitDefinition, *parse.TypeDeclaration, *parse.ExternTypeDeclaration:
		return "type"
	}
	return "other"
}

func isModuleAliasName(name string, prog *parse.Program) bool {
	if name == "" || prog == nil {
		return false
	}
	for _, imp := range prog.Imports {
		if imp.Name == name {
			return true
		}
	}
	return preludeModulePath(name) != ""
}

func structValueNameLocation(prop parse.StructValue, source string) parse.Location {
	if prop.Name.Location.Start.Row != 0 || prop.Name.Location.Start.Col != 0 {
		return prop.Name.Location
	}
	if prop.Value == nil || prop.Name.Name == "" || source == "" {
		return prop.Name.Location
	}
	valueLoc := prop.Value.GetLocation()
	if valueLoc.Start.Row <= 0 || valueLoc.Start.Col <= 0 {
		return prop.Name.Location
	}
	line := sourceLine(source, valueLoc.Start.Row)
	limit := valueLoc.Start.Col - 1
	if limit < 0 {
		return prop.Name.Location
	}
	if limit > len(line) {
		limit = len(line)
	}
	prefix := line[:limit]
	colon := strings.LastIndex(prefix, ":")
	if colon < 0 {
		return prop.Name.Location
	}
	idx := strings.LastIndex(prefix[:colon], prop.Name.Name)
	if idx < 0 {
		return prop.Name.Location
	}
	start := parse.Point{Row: valueLoc.Start.Row, Col: idx + 1}
	return parse.Location{Start: start, End: parse.Point{Row: start.Row, Col: start.Col + len(prop.Name.Name)}}
}

func sourceLine(source string, row int) string {
	if row <= 0 {
		return ""
	}
	current := 1
	start := 0
	for i, ch := range source {
		if current == row {
			if ch == '\n' || ch == '\r' {
				return source[start:i]
			}
			continue
		}
		if ch == '\n' {
			current++
			start = i + 1
		}
	}
	if current == row {
		return source[start:]
	}
	return ""
}

func visitReferenceDeclaredType(declared parse.DeclaredType, currentFile string, source string, prog *parse.Program, rootFile string, targetKind string, targetName string, targetDef *definitionTarget, add func(string, parse.Location)) {
	if declared == nil {
		return
	}
	if targetKind == "type" && referenceNamesMatch(targetName, declaredTypeReferenceName(declared)) {
		if def := definitionForReferenceDeclaredType(declared, targetDef, prog, rootFile); sameDefinitionTarget(def, targetDef) {
			add(currentFile, declared.GetLocation())
		}
	}
	for _, child := range declaredTypeChildren(declared) {
		visitReferenceDeclaredType(child, currentFile, source, prog, rootFile, targetKind, targetName, targetDef, add)
	}
}

func definitionForReferenceDeclaredType(declared parse.DeclaredType, targetDef *definitionTarget, prog *parse.Program, filePath string) *definitionTarget {
	switch t := declared.(type) {
	case *parse.CustomType:
		if customTypeReferencesTargetFile(t, targetDef, prog, filePath) {
			return targetDef
		}
	}
	return definitionForDeclaredType(declared, prog, filePath)
}

func customTypeReferencesTargetFile(t *parse.CustomType, targetDef *definitionTarget, prog *parse.Program, filePath string) bool {
	if t == nil || targetDef == nil {
		return false
	}
	alias, _, ok := importedTypeDisplayParts(t.Name)
	if !ok {
		return false
	}
	modulePath, ok := modulePathForAlias(alias, prog, filePath)
	return ok && cleanReferencePath(modulePath) == cleanReferencePath(targetDef.filePath)
}

func definitionForDeclaredType(declared parse.DeclaredType, prog *parse.Program, filePath string) *definitionTarget {
	switch t := declared.(type) {
	case *parse.MutableType:
		return definitionForDeclaredType(t.Inner, prog, filePath)
	case *parse.CustomType:
		if t.Name == "" {
			return nil
		}
		return definitionForTypeName(t.Name, prog, filePath)
	}
	return nil
}

func declaredTypeChildren(declared parse.DeclaredType) []parse.DeclaredType {
	switch t := declared.(type) {
	case *parse.MutableType:
		return []parse.DeclaredType{t.Inner}
	case *parse.List:
		return []parse.DeclaredType{t.Element}
	case *parse.Map:
		return []parse.DeclaredType{t.Key, t.Value}
	case *parse.ResultType:
		return []parse.DeclaredType{t.Val, t.Err}
	case *parse.FunctionType:
		children := append([]parse.DeclaredType{}, t.Params...)
		children = append(children, t.Return)
		return children
	case *parse.CustomType:
		return t.TypeArgs
	}
	return nil
}

func definitionForStructValueField(instance *parse.StructInstance, fieldName string, prog *parse.Program, filePath string) *definitionTarget {
	if instance == nil || fieldName == "" {
		return nil
	}
	ownerName := instance.Name.Name
	if ownerName == "" {
		return nil
	}
	if alias, memberName, ok := importedTypeDisplayParts(ownerName); ok {
		modulePath, moduleProg, ok := moduleSourceForAlias(alias, prog, filePath)
		if !ok {
			return nil
		}
		return findStructFieldDefinition(memberName, fieldName, moduleProg.Statements, modulePath)
	}
	return findStructFieldDefinition(ownerName, fieldName, prog.Statements, filePath)
}

func referenceExprLocation(expr parse.Expression) parse.Location {
	switch e := expr.(type) {
	case *parse.InstanceProperty:
		return e.Property.Location
	case *parse.InstanceMethod:
		return e.Method.Location
	case *parse.StaticProperty:
		return e.Property.GetLocation()
	case *parse.StaticFunction:
		return e.Function.Location
	case *parse.StructInstance:
		return e.Name.Location
	}
	return expr.GetLocation()
}

func sameDefinitionTarget(a *definitionTarget, b *definitionTarget) bool {
	if a == nil || b == nil {
		return false
	}
	return a.filePath == b.filePath && sameParseLocation(a.loc, b.loc)
}

func sameParseLocation(a parse.Location, b parse.Location) bool {
	return a.Start.Row == b.Start.Row && a.Start.Col == b.Start.Col && a.End.Row == b.End.Row && a.End.Col == b.End.Col
}
