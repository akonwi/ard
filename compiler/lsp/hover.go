package lsp

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
)

// hoverInfo holds the content to display in the hover popup.
type hoverInfo struct {
	content string // Markdown content
}

type hoverParam struct {
	Name    string
	Type    string
	Mutable bool
}

type hoverMethodSignature struct {
	OwnerType  string
	Name       string
	Params     []hoverParam
	ReturnType string
	Mutates    bool
}

type hoverStaticFunctionSignature struct {
	Qualifier  string
	Name       string
	Params     []hoverParam
	ReturnType string
}

// computeHover finds the expression at the given position and returns hover info.
func computeHover(source string, filePath string, position protocol.Position) *hoverInfo {
	target := lspPositionToParsePoint(position)

	prog := parseAndCache(source, filePath)
	if prog == nil {
		return nil
	}

	expr := findInStmts(prog.Statements, target)
	if expr == nil {
		return nil
	}

	return describeExpr(expr, source, filePath, prog)
}

// parseAndCache parses source and returns the parsed program.
func parseAndCache(source string, filePath string) *parse.Program {
	result := parse.Parse([]byte(source), filePath)
	return result.Program
}

// lspPositionToParsePoint converts an LSP position back to a parse Point.
// LSP uses 0-based; parser uses 1-based.
func lspPositionToParsePoint(pos protocol.Position) parse.Point {
	return parse.Point{
		Row: int(pos.Line) + 1,
		Col: int(pos.Character) + 1,
	}
}

// pointInRange checks if a point falls within a location range.
// Returns false if the location has zero-value (unset by parser).
func pointInRange(p parse.Point, loc parse.Location) bool {
	if loc.Start.Row == 0 && loc.Start.Col == 0 && loc.End.Row == 0 && loc.End.Col == 0 {
		return false
	}
	if p.Row < loc.Start.Row {
		return false
	}
	if p.Row == loc.Start.Row && p.Col < loc.Start.Col {
		return false
	}
	if loc.End.Row > 0 && p.Row > loc.End.Row {
		return false
	}
	if loc.End.Row > 0 && p.Row == loc.End.Row && p.Col > loc.End.Col {
		return false
	}
	return true
}

// typeDeclString converts a parse.DeclaredType to a readable string.
func typeDeclString(t parse.DeclaredType) string {
	if t == nil {
		return "?"
	}

	var s string
	switch tt := t.(type) {
	case *parse.MutableType:
		s = "mut " + typeDeclString(tt.Inner)
	case *parse.List:
		s = "[" + typeDeclString(tt.Element) + "]"
	case *parse.Map:
		s = "[" + typeDeclString(tt.Key) + ":" + typeDeclString(tt.Value) + "]"
	case *parse.CustomType:
		s = tt.Name
		if len(tt.TypeArgs) > 0 {
			args := make([]string, len(tt.TypeArgs))
			for i, arg := range tt.TypeArgs {
				args[i] = typeDeclString(arg)
			}
			s += "<" + strings.Join(args, ", ") + ">"
		}
	default:
		s = t.GetName()
		// Map parser's canonical names to Ard surface names
		switch s {
		case "String":
			s = "Str"
		case "Boolean":
			s = "Bool"
		}
	}

	if t.IsNullable() {
		s = "?" + s
	}
	return s
}

// checkerTypeString converts a checker.Type to a readable Ard type string.
func checkerTypeString(t checker.Type) string {
	if t == nil {
		return "?"
	}
	// Use the checker's String() which produces canonical type names.
	s := t.String()
	// Map canonical names to Ard surface names
	switch s {
	case "String":
		return "Str"
	case "Boolean":
		return "Bool"
	}
	return s
}

// simpleHover builds a hoverInfo from a label string.
func simpleHover(label string) *hoverInfo {
	return &hoverInfo{content: fmt.Sprintf("```ard\n%s\n```", label)}
}

func simpleExprName(expr parse.Expression) string {
	switch e := expr.(type) {
	case *parse.Identifier:
		return e.Name
	case *parse.StaticProperty:
		left := simpleExprName(e.Target)
		right := simpleExprName(e.Property)
		if left != "" && right != "" {
			return left + "::" + right
		}
		return right
	}
	return ""
}

func formatHoverParams(params []hoverParam) string {
	parts := make([]string, len(params))
	for i, p := range params {
		mut := ""
		if p.Mutable {
			mut = "mut "
		}
		if p.Name == "" {
			parts[i] = mut + normalizeDisplayType(p.Type)
			continue
		}
		parts[i] = fmt.Sprintf("%s%s: %s", mut, p.Name, normalizeDisplayType(p.Type))
	}
	return strings.Join(parts, ", ")
}

func formatMethodSignature(sig *hoverMethodSignature) string {
	prefix := "fn "
	if sig.Mutates {
		prefix += "mut "
	}
	owner := normalizeDisplayType(sig.OwnerType)
	if owner != "" {
		owner += "."
	}
	ret := normalizeDisplayType(sig.ReturnType)
	if ret == "" {
		ret = "Void"
	}
	return fmt.Sprintf("%s%s%s(%s) %s", prefix, owner, sig.Name, formatHoverParams(sig.Params), ret)
}

func formatStaticFunctionSignature(sig *hoverStaticFunctionSignature) string {
	qualifier := sig.Qualifier
	if qualifier != "" {
		qualifier += "::"
	}
	ret := normalizeDisplayType(sig.ReturnType)
	if ret == "" {
		ret = "Void"
	}
	return fmt.Sprintf("fn %s%s(%s) %s", qualifier, sig.Name, formatHoverParams(sig.Params), ret)
}

func normalizeDisplayType(t string) string {
	if strings.HasPrefix(t, "?") && len(t) > 1 {
		return strings.TrimPrefix(t, "?") + "?"
	}
	return t
}

func qualifyStaticFunctionSignature(sig *hoverStaticFunctionSignature, prog *parse.Program, filePath string) {
	for i := range sig.Params {
		sig.Params[i].Type = qualifyTypeDisplay(sig.Params[i].Type, prog, filePath)
	}
	sig.ReturnType = qualifyTypeDisplay(sig.ReturnType, prog, filePath)
}

func qualifyMethodSignature(sig *hoverMethodSignature, prog *parse.Program, filePath string) {
	sig.OwnerType = qualifyTypeDisplay(sig.OwnerType, prog, filePath)
	for i := range sig.Params {
		sig.Params[i].Type = qualifyTypeDisplay(sig.Params[i].Type, prog, filePath)
	}
	sig.ReturnType = qualifyTypeDisplay(sig.ReturnType, prog, filePath)
}

func qualifyTypeDisplay(typeName string, prog *parse.Program, filePath string) string {
	if typeName == "" || prog == nil {
		return typeName
	}
	aliases := importedTypeAliases(prog, filePath)
	if len(aliases) == 0 {
		return normalizeDisplayType(typeName)
	}
	return qualifyTypeNames(normalizeDisplayType(typeName), aliases)
}

func qualifyTypeNames(typeName string, aliases map[string]string) string {
	var out strings.Builder
	for i := 0; i < len(typeName); {
		ch := typeName[i]
		if isTypeIdentStart(ch) {
			start := i
			i++
			for i < len(typeName) && isTypeIdentPart(typeName[i]) {
				i++
			}
			ident := typeName[start:i]
			if qualified, ok := aliases[ident]; ok && !isAlreadyQualified(typeName, start) && !isGenericName(typeName, start) {
				out.WriteString(qualified)
			} else {
				out.WriteString(ident)
			}
			continue
		}
		out.WriteByte(ch)
		i++
	}
	return out.String()
}

func isTypeIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isTypeIdentPart(ch byte) bool {
	return isTypeIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func isAlreadyQualified(s string, start int) bool {
	return start >= 2 && s[start-2:start] == "::"
}

func isGenericName(s string, start int) bool {
	return start > 0 && s[start-1] == '$'
}

func importedTypeAliases(prog *parse.Program, filePath string) map[string]string {
	aliases := map[string]string{}
	ambiguous := map[string]bool{}
	localTypes := localTypeNames(prog.Statements)

	for _, imp := range prog.Imports {
		for _, name := range importedPublicTypeNames(imp, filePath) {
			if localTypes[name] || ambiguous[name] {
				continue
			}
			qualified := imp.Name + "::" + name
			if existing, ok := aliases[name]; ok && existing != qualified {
				delete(aliases, name)
				ambiguous[name] = true
				continue
			}
			aliases[name] = qualified
		}
	}

	return aliases
}

func localTypeNames(stmts []parse.Statement) map[string]bool {
	names := map[string]bool{}
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *parse.StructDefinition:
			names[s.Name.Name] = true
		case *parse.EnumDefinition:
			names[s.Name] = true
		case *parse.TypeDeclaration:
			names[s.Name.Name] = true
		case *parse.ExternTypeDeclaration:
			names[s.Name] = true
		case *parse.TraitDefinition:
			names[s.Name.Name] = true
		}
	}
	return names
}

func importedPublicTypeNames(imp parse.Import, filePath string) []string {
	mod, ok := moduleForImport(imp, filePath)
	if !ok {
		return nil
	}
	return checkerModuleTypeNames(mod)
}

func importedTypeForDisplay(typeName string, prog *parse.Program, filePath string) (checker.Type, bool) {
	importedType, _, _, ok := importedTypeAndModuleForDisplay(typeName, prog, filePath)
	return importedType, ok
}

func importedTypeAndModuleForDisplay(typeName string, prog *parse.Program, filePath string) (checker.Type, checker.Module, string, bool) {
	alias, memberName, ok := importedTypeDisplayParts(typeName)
	if !ok {
		return nil, nil, "", false
	}
	mod, ok := importedModuleForAlias(alias, prog, filePath)
	if !ok {
		return nil, nil, "", false
	}
	sym := mod.Get(memberName)
	if sym.IsZero() {
		return nil, nil, "", false
	}
	return sym.Type, mod, memberName, true
}

func importedStructMethodsForDisplay(ownerType string, def *checker.StructDef, prog *parse.Program, filePath string) map[string]*checker.FunctionDef {
	if def == nil {
		return nil
	}
	_, mod, memberName, ok := importedTypeAndModuleForDisplay(ownerType, prog, filePath)
	if !ok || mod == nil || mod.Program() == nil {
		return nil
	}
	owner := checker.StructMethodOwner(def)
	if owner.ModulePath == "" {
		owner.ModulePath = mod.Path()
	}
	if owner.TypeName == "" {
		owner.TypeName = memberName
	}
	return checker.StructMethodsInModules(map[string]checker.Module{mod.Path(): mod}, owner)
}

func importedTypeDisplayParts(typeName string) (alias string, memberName string, ok bool) {
	typeName = strings.TrimSpace(normalizeDisplayType(typeName))
	typeName = strings.TrimSuffix(typeName, "?")
	if genericStart := strings.Index(typeName, "<"); genericStart >= 0 {
		typeName = typeName[:genericStart]
	}
	parts := strings.SplitN(typeName, "::", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func importedTypeGenericArgs(typeName string) []string {
	typeName = strings.TrimSpace(normalizeDisplayType(typeName))
	typeName = strings.TrimSuffix(typeName, "?")
	start := strings.Index(typeName, "<")
	if start < 0 || !strings.HasSuffix(typeName, ">") {
		return nil
	}
	return splitTopLevel(typeName[start+1:len(typeName)-1], ',')
}

func splitTopLevel(s string, sep rune) []string {
	parts := []string{}
	start := 0
	bracketDepth := 0
	parenDepth := 0
	angleDepth := 0
	for i, r := range s {
		switch r {
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '<':
			angleDepth++
		case '>':
			angleDepth--
		default:
			if r == sep && bracketDepth == 0 && parenDepth == 0 && angleDepth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + len(string(r))
			}
		}
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts
}

func genericParamNames(t checker.Type) []string {
	switch def := t.(type) {
	case *checker.StructDef:
		return def.GenericParams
	case *checker.ExternType:
		return def.GenericParams
	}
	return nil
}

func substituteImportedGenericDisplay(typeDisplay string, importedType checker.Type, ownerType string) string {
	args := importedTypeGenericArgs(ownerType)
	params := genericParamNames(importedType)
	if len(args) == 0 || len(args) != len(params) {
		return typeDisplay
	}
	out := typeDisplay
	for i, param := range params {
		out = strings.ReplaceAll(out, "$"+param, args[i])
	}
	return out
}

func substituteImportedGenericSignature(sig *hoverMethodSignature, importedType checker.Type, ownerType string) {
	sig.OwnerType = ownerType
	for i := range sig.Params {
		sig.Params[i].Type = substituteImportedGenericDisplay(sig.Params[i].Type, importedType, ownerType)
	}
	sig.ReturnType = substituteImportedGenericDisplay(sig.ReturnType, importedType, ownerType)
}

func importedModuleForAlias(alias string, prog *parse.Program, filePath string) (checker.Module, bool) {
	if prog == nil {
		return nil, false
	}
	for _, imp := range prog.Imports {
		if imp.Name != alias {
			continue
		}
		return moduleForImport(imp, filePath)
	}
	if path := preludeModulePath(alias); path != "" {
		return checker.FindEmbeddedModule(path)
	}
	return nil, false
}

func moduleForImport(imp parse.Import, filePath string) (checker.Module, bool) {
	if imp.Kind == parse.ImportKindGo {
		return nil, false
	}
	if strings.HasPrefix(imp.Path, "ard/") {
		return checker.FindEmbeddedModule(imp.Path)
	}

	if filePath == "" {
		return nil, false
	}
	moduleResolver, err := checker.NewModuleResolver(filepath.Dir(filePath))
	if err != nil {
		return nil, false
	}
	importPath, err := moduleResolver.ResolveImportPath(imp.Path)
	if err != nil {
		return nil, false
	}
	ast, err := moduleResolver.LoadModule(imp.Path)
	if err != nil {
		return nil, false
	}
	c := checker.New(importPath, ast, moduleResolver, checker.CheckOptions{})
	c.Check()
	return c.Module(), true
}

func checkerModuleTypeNames(mod checker.Module) []string {
	if mod == nil || mod.Program() == nil {
		return nil
	}
	names := []string{}
	for _, stmt := range mod.Program().Statements {
		name := checkerStatementTypeName(stmt)
		if name == "" {
			continue
		}
		if sym := mod.Get(name); sym.IsZero() {
			continue
		}
		names = append(names, name)
	}
	return names
}

func checkerStatementTypeName(stmt checker.Statement) string {
	switch s := stmt.Stmt.(type) {
	case *checker.StructDef:
		return s.Name
	case *checker.Enum:
		return s.Name
	case *checker.Union:
		return s.Name
	case *checker.ExternType:
		return s.Name_
	}
	return ""
}

// findInStmts walks a list of statements to find the expression at target.
func findInStmts(stmts []parse.Statement, target parse.Point) (best parse.Expression) {
	defer func() {
		if r := recover(); r != nil {
			panic(fmt.Errorf("findInStmts panic at target %v: %v", target, r))
		}
	}()

	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		if !pointInRange(target, stmt.GetLocation()) {
			continue
		}

		// Walk into structured statements first (they also satisfy Expression
		// but need special handling to recurse into their bodies).
		switch s := stmt.(type) {
		case *parse.VariableDeclaration:
			if s.Value != nil {
				if inner := walkExpr(s.Value, target); inner != nil {
					best = inner
					continue
				}
			}
			best = s
			continue
		case *parse.VariableAssignment:
			if s.Target != nil {
				if inner := walkExpr(s.Target, target); inner != nil {
					best = inner
				}
			}
			if s.Value != nil {
				if inner := walkExpr(s.Value, target); inner != nil {
					best = inner
				}
			}
			continue
		case *parse.FunctionDeclaration:
			if target.Row == s.Location.Start.Row {
				best = s
			}
			for _, p := range s.Parameters {
				if inner := walkExpr(&p, target); inner != nil {
					best = inner
				}
			}
			if inner := findInStmts(s.Body, target); inner != nil {
				best = inner
			}
			continue
		case *parse.IfStatement:
			if s.Condition != nil {
				if inner := walkExpr(s.Condition, target); inner != nil {
					best = inner
				}
			}
			if inner := findInStmts(s.Body, target); inner != nil {
				best = inner
			}
			if s.Else != nil {
				if inner := findInStmts([]parse.Statement{s.Else}, target); inner != nil {
					best = inner
				}
			}
			continue
		case *parse.WhileLoop:
			if s.Condition != nil {
				if inner := walkExpr(s.Condition, target); inner != nil {
					best = inner
				}
			}
			if inner := findInStmts(s.Body, target); inner != nil {
				best = inner
			}
			continue
		case *parse.ForInLoop:
			if pointInRange(target, s.Cursor.Location) {
				best = &s.Cursor
			}
			if pointInRange(target, s.Cursor2.Location) {
				best = &s.Cursor2
			}
			if inner := walkExpr(s.Iterable, target); inner != nil {
				best = inner
			}
			if inner := findInStmts(s.Body, target); inner != nil {
				best = inner
			}
			continue
		case *parse.RangeLoop:
			if pointInRange(target, s.Cursor.Location) {
				best = &s.Cursor
			}
			if pointInRange(target, s.Cursor2.Location) {
				best = &s.Cursor2
			}
			if inner := walkExpr(s.Start, target); inner != nil {
				best = inner
			}
			if inner := walkExpr(s.End, target); inner != nil {
				best = inner
			}
			if inner := findInStmts(s.Body, target); inner != nil {
				best = inner
			}
			continue
		case *parse.ForLoop:
			if s.Init != nil {
				if inner := findInStmts([]parse.Statement{s.Init}, target); inner != nil {
					best = inner
				}
			}
			if s.Condition != nil {
				if inner := walkExpr(s.Condition, target); inner != nil {
					best = inner
				}
			}
			if s.Incrementer != nil {
				if inner := findInStmts([]parse.Statement{s.Incrementer}, target); inner != nil {
					best = inner
				}
			}
			if inner := findInStmts(s.Body, target); inner != nil {
				best = inner
			}
			continue
		case *parse.MatchExpression:
			if s.Subject != nil {
				if inner := walkExpr(s.Subject, target); inner != nil {
					best = inner
				}
			}
			for _, matchCase := range s.Cases {
				if matchCase.Pattern != nil {
					if inner := walkExpr(matchCase.Pattern, target); inner != nil {
						best = inner
					}
				}
				if inner := findInStmts(matchCase.Body, target); inner != nil {
					best = inner
				}
			}
			continue
		case *parse.ConditionalMatchExpression:
			for _, matchCase := range s.Cases {
				if matchCase.Condition != nil {
					if inner := walkExpr(matchCase.Condition, target); inner != nil {
						best = inner
					}
				}
				if inner := findInStmts(matchCase.Body, target); inner != nil {
					best = inner
				}
			}
			continue
		case *parse.Try:
			if s.Expression != nil {
				if inner := walkExpr(s.Expression, target); inner != nil {
					best = inner
				}
			}
			continue
		case *parse.BlockExpression:
			if inner := findInStmts(s.Statements, target); inner != nil {
				best = inner
			}
			continue
		case *parse.StructInstance:
			for _, prop := range s.Properties {
				if inner := walkExpr(prop.Value, target); inner != nil {
					best = inner
				}
			}
			continue
		case *parse.ImplBlock:
			if pointInRange(target, s.Target.Location) {
				best = &s.Target
			}
			if pointInRange(target, s.Receiver.Location) {
				best = &s.Receiver
			}
			for i := range s.Methods {
				if inner := findInStmts([]parse.Statement{&s.Methods[i]}, target); inner != nil {
					best = inner
				}
			}
			continue
		case *parse.TraitImplementation:
			if inner := walkExpr(s.Trait, target); inner != nil {
				best = inner
			}
			if pointInRange(target, s.ForType.Location) {
				best = &s.ForType
			}
			if pointInRange(target, s.Receiver.Location) {
				best = &s.Receiver
			}
			for i := range s.Methods {
				if inner := findInStmts([]parse.Statement{&s.Methods[i]}, target); inner != nil {
					best = inner
				}
			}
			continue
		case *parse.TraitDefinition:
			if pointInRange(target, s.Name.Location) {
				best = &s.Name
			}
			for i := range s.Methods {
				if inner := findInStmts([]parse.Statement{&s.Methods[i]}, target); inner != nil {
					best = inner
				}
			}
			continue
		case *parse.StructDefinition, *parse.EnumDefinition,
			*parse.ExternTypeDeclaration, *parse.ExternalFunction,
			*parse.TypeDeclaration, *parse.Comment,
			*parse.StaticFunctionDeclaration:
			continue
		}

		// Fall through to expression-only handling for types not matched above.
		if expr, ok := stmt.(parse.Expression); ok {
			if inner := walkExpr(expr, target); inner != nil {
				best = inner
			}
		}
	}

	return best
}

// walkExpr recursively descends into an expression tree looking for the
// innermost expression containing target. Returns the deepest match.
func walkExpr(expr parse.Expression, target parse.Point) parse.Expression {
	if expr == nil {
		return nil
	}
	if !pointInRange(target, expr.GetLocation()) {
		return nil
	}

	// Default: this expression contains the target
	best := expr

	recurse := func(child parse.Expression) {
		if inner := walkExpr(child, target); inner != nil {
			best = inner
		}
	}

	switch e := expr.(type) {
	case *parse.BinaryExpression:
		recurse(e.Left)
		recurse(e.Right)
	case *parse.UnaryExpression:
		recurse(e.Operand)
	case *parse.FunctionCall:
		for _, arg := range e.Args {
			recurse(arg.Value)
		}
	case *parse.InstanceProperty:
		recurse(e.Target)
		if pointInRange(target, e.Property.GetLocation()) {
			best = e
		}
	case *parse.InstanceMethod:
		recurse(e.Target)
		for _, arg := range e.Method.Args {
			recurse(arg.Value)
		}
	case *parse.StaticProperty:
		recurse(e.Target)
		if pointInRange(target, e.Property.GetLocation()) {
			if _, ok := e.Property.(*parse.Identifier); ok {
				best = e
			} else if inner := walkExpr(e.Property, target); inner != nil {
				best = inner
			} else {
				best = e
			}
		}
	case *parse.StaticFunction:
		recurse(e.Target)
		for _, arg := range e.Function.Args {
			recurse(arg.Value)
		}
	case *parse.ListLiteral:
		for _, item := range e.Items {
			recurse(item)
		}
	case *parse.MapLiteral:
		for _, entry := range e.Entries {
			recurse(entry.Key)
			recurse(entry.Value)
		}
	case *parse.MatchExpression:
		recurse(e.Subject)
		for _, matchCase := range e.Cases {
			recurse(matchCase.Pattern)
			if inner := findInStmts(matchCase.Body, target); inner != nil {
				best = inner
			}
		}
	case *parse.ConditionalMatchExpression:
		for _, matchCase := range e.Cases {
			recurse(matchCase.Condition)
			if inner := findInStmts(matchCase.Body, target); inner != nil {
				best = inner
			}
		}
	case *parse.Try:
		recurse(e.Expression)
	case *parse.BlockExpression:
		if inner := findInStmts(e.Statements, target); inner != nil {
			best = inner
		}
	case *parse.InterpolatedStr:
		for _, chunk := range e.Chunks {
			recurse(chunk)
		}
	case *parse.IfStatement:
		recurse(e.Condition)
	case *parse.StructInstance:
		for _, prop := range e.Properties {
			recurse(prop.Value)
		}
	case *parse.AnonymousFunction:
		for _, bodyStmt := range e.Body {
			if inner := findInStmts([]parse.Statement{bodyStmt}, target); inner != nil {
				best = inner
			}
		}
	case *parse.Identifier, *parse.StrLiteral, *parse.NumLiteral,
		*parse.BoolLiteral, *parse.VoidLiteral:
		// Leaf nodes — return as-is
	}

	return best
}

// describeExpr returns hover text describing the expression's type/signature.
func describeExpr(expr parse.Expression, source string, filePath string, prog *parse.Program) *hoverInfo {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *parse.Identifier:
		return describeIdentifier(e, source, filePath, prog)
	case *parse.StrLiteral:
		return simpleHover("Str")
	case *parse.NumLiteral:
		if strings.ContainsRune(e.Value, '.') {
			return simpleHover("Float")
		}
		return simpleHover("Int")
	case *parse.BoolLiteral:
		return simpleHover("Bool")
	case *parse.VoidLiteral:
		return simpleHover("Void")
	case *parse.Parameter:
		if e.Type != nil {
			return simpleHover(typeDeclString(e.Type))
		}
		return simpleHover("?")
	case *parse.FunctionCall:
		return describeFunctionCall(e, source, filePath, prog)
	case *parse.InstanceProperty:
		return describeInstanceProperty(e, source, filePath, prog)
	case *parse.InstanceMethod:
		return describeInstanceMethod(e, source, filePath, prog)
	case *parse.StaticProperty:
		return describeStaticProperty(e, source, filePath, prog)
	case *parse.StaticFunction:
		return describeStaticFunction(e, source, filePath, prog)
	case *parse.VariableDeclaration:
		return describeVariableDecl(e, source, filePath, prog)
	case *parse.FunctionDeclaration:
		return describeFunctionDecl(e)
	case *parse.ListLiteral:
		if len(e.Items) == 0 {
			return simpleHover("[?]")
		}
		itemType := inferExprType(e.Items[0], prog, filePath)
		if itemType == "" || itemType == "?" {
			return simpleHover("List")
		}
		return simpleHover("[" + itemType + "]")
	case *parse.MapLiteral:
		return simpleHover("Map")
	case *parse.MatchExpression:
		return simpleHover("match")
	case *parse.Try:
		return simpleHover("try")
	case *parse.BinaryExpression:
		return simpleHover("Bool")
	case *parse.UnaryExpression:
		if e.Operator == parse.Bang || e.Operator == parse.Not {
			return simpleHover("Bool")
		}
		return simpleHover("Int")
	case *parse.AnonymousFunction:
		return describeAnonFunction(e)
	case *parse.StructInstance:
		return simpleHover(e.Name.Name)
	case *parse.IfStatement:
		if len(e.Body) > 0 {
			if lastExpr, ok := e.Body[len(e.Body)-1].(parse.Expression); ok {
				return describeExpr(lastExpr, source, filePath, prog)
			}
		}
		return simpleHover("Void")
	}

	return nil
}

// describeIdentifier looks up an identifier's type using the checker first,
// then falls back to parse-tree scanning.
func describeIdentifier(id *parse.Identifier, source string, filePath string, prog *parse.Program) *hoverInfo {
	if id == nil {
		return nil
	}

	// Check builtins first (they're known without needing checker)
	if info := builtinType(id.Name); info != nil {
		return info
	}

	// Try the checker — it has fully resolved types.
	if t := resolveFromChecker(id.Name, prog, filePath); t != "" && t != "?" {
		return simpleHover(t)
	}

	// Fall back to parse-tree scanning
	info := scanForType(id.Name, prog.Statements, prog, filePath)
	if info != nil {
		return info
	}

	if info := describeModuleAlias(id.Name, prog); info != nil {
		return info
	}

	return nil
}

// resolveFromChecker runs the checker on the parsed program and returns the
// resolved type string for the given identifier name, or "" if not found.
func resolveFromChecker(name string, prog *parse.Program, filePath string) string {
	workingDir := filepath.Dir(filePath)
	moduleResolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		log.Printf("hover: module resolver error for %s: %v", filePath, err)
		return ""
	}

	relPath, err := filepath.Rel(workingDir, filePath)
	if err != nil {
		relPath = filePath
	}

	c := checker.New(relPath, prog, moduleResolver, checker.CheckOptions{})
	c.Check()

	// Try module public symbols first
	if sym := c.Module().Get(name); !sym.IsZero() {
		return qualifyTypeDisplay(checkerTypeString(sym.Type), prog, filePath)
	}

	// Walk the checker's program tree for local variables
	checkerProg := c.Module().Program()
	if checkerProg == nil {
		return ""
	}
	return qualifyTypeDisplay(findTypeInCheckerProg(name, checkerProg), prog, filePath)
}

// findTypeInCheckerProg walks checker statements to find a variable by name.
func findTypeInCheckerProg(name string, prog *checker.Program) string {
	if prog == nil {
		return ""
	}
	for _, stmt := range prog.Statements {
		// Check NonProducing variants
		if t := findTypeInNonProducing(name, stmt.Stmt); t != "" {
			return t
		}
		if def, ok := stmt.Stmt.(*checker.StructDef); ok {
			for _, method := range prog.StructMethodsFor(checker.StructMethodOwner(def)) {
				if t := findTypeInExpr(name, method); t != "" {
					return t
				}
			}
		}
		// Check Expression variants (includes FunctionDef, If, Block, etc.)
		if t := findTypeInExpr(name, stmt.Expr); t != "" {
			return t
		}
	}
	return ""
}

// findTypeInNonProducing handles the NonProducing side of checker.Statement.
func findTypeInNonProducing(name string, stmt checker.NonProducing) string {
	if stmt == nil {
		return ""
	}
	switch s := stmt.(type) {
	case *checker.VariableDef:
		if s.Name == name {
			return checkerTypeString(s.Type())
		}
	case *checker.Reassignment:
		// Reassignment doesn't declare a new variable, skip
	case *checker.ForIntRange:
		if s.Cursor == name || s.Index == name {
			return "Int"
		}
		if s.Body != nil {
			if t := findTypeInCheckerBlock(name, s.Body); t != "" {
				return t
			}
		}
	case *checker.ForInStr:
		if s.Cursor == name || s.Index == name {
			return "Str"
		}
		if s.Body != nil {
			if t := findTypeInCheckerBlock(name, s.Body); t != "" {
				return t
			}
		}
	case *checker.ForInList:
		if s.Cursor == name || s.Index == name {
			if lt, ok := s.List.Type().(*checker.List); ok {
				return checkerTypeString(lt.Of())
			}
			return "?"
		}
		if s.Body != nil {
			if t := findTypeInCheckerBlock(name, s.Body); t != "" {
				return t
			}
		}
	case *checker.ForInMap:
		if s.Key == name || s.Val == name {
			if mt, ok := s.Map.Type().(*checker.Map); ok {
				if s.Key == name {
					return checkerTypeString(mt.Key())
				}
				return checkerTypeString(mt.Value())
			}
			return "?"
		}
		if s.Body != nil {
			if t := findTypeInCheckerBlock(name, s.Body); t != "" {
				return t
			}
		}
	case *checker.ForLoop:
		if s.Init != nil && s.Init.Name == name {
			return checkerTypeString(s.Init.Type())
		}
		if s.Body != nil {
			if t := findTypeInCheckerBlock(name, s.Body); t != "" {
				return t
			}
		}
	case *checker.WhileLoop:
		if s.Body != nil {
			if t := findTypeInCheckerBlock(name, s.Body); t != "" {
				return t
			}
		}
	case *checker.Enum:
		if s.Name == name {
			return checkerTypeString(s.Type())
		}
		for _, method := range s.Methods {
			if t := findTypeInExpr(name, method); t != "" {
				return t
			}
		}
	case *checker.StructDef:
		if s.Name == name {
			return checkerTypeString(s)
		}
	case *checker.Union:
		// Type definitions without method bodies — skip
	}
	return ""
}

// findTypeInExpr handles the Expression side of checker.Statement.
func findTypeInExpr(name string, expr checker.Expression) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *checker.FunctionDef:
		for _, p := range e.Parameters {
			if p.Name == name {
				return checkerTypeString(p.Type)
			}
		}
		if e.Body != nil {
			if t := findTypeInCheckerBlock(name, e.Body); t != "" {
				return t
			}
		}
	case *checker.Block:
		return findTypeInCheckerBlock(name, e)
	case *checker.If:
		return findTypeInCheckerIf(name, e)
	case *checker.BoolMatch:
		if e.True != nil {
			if t := findTypeInCheckerBlock(name, e.True); t != "" {
				return t
			}
		}
		if e.False != nil {
			if t := findTypeInCheckerBlock(name, e.False); t != "" {
				return t
			}
		}
	case *checker.OptionMatch:
		if e.Some != nil && e.Some.Body != nil {
			if t := findTypeInCheckerBlock(name, e.Some.Body); t != "" {
				return t
			}
		}
		if e.None != nil {
			if t := findTypeInCheckerBlock(name, e.None); t != "" {
				return t
			}
		}
	case *checker.EnumMatch:
		for _, c := range e.Cases {
			if c != nil {
				if t := findTypeInCheckerBlock(name, c); t != "" {
					return t
				}
			}
		}
		if e.CatchAll != nil {
			if t := findTypeInCheckerBlock(name, e.CatchAll); t != "" {
				return t
			}
		}
	case *checker.IntMatch:
		for _, b := range e.IntCases {
			if t := findTypeInCheckerBlock(name, b); t != "" {
				return t
			}
		}
		for _, b := range e.RangeCases {
			if t := findTypeInCheckerBlock(name, b); t != "" {
				return t
			}
		}
		if e.CatchAll != nil {
			if t := findTypeInCheckerBlock(name, e.CatchAll); t != "" {
				return t
			}
		}
	case *checker.StrMatch:
		for _, b := range e.Cases {
			if t := findTypeInCheckerBlock(name, b); t != "" {
				return t
			}
		}
		if e.CatchAll != nil {
			if t := findTypeInCheckerBlock(name, e.CatchAll); t != "" {
				return t
			}
		}
	case *checker.UnionMatch:
		for _, m := range e.TypeCases {
			if m != nil && m.Body != nil {
				if t := findTypeInCheckerBlock(name, m.Body); t != "" {
					return t
				}
			}
		}
		if e.CatchAll != nil {
			if t := findTypeInCheckerBlock(name, e.CatchAll); t != "" {
				return t
			}
		}
	case *checker.ResultMatch:
		if e.Ok != nil && e.Ok.Body != nil {
			if t := findTypeInCheckerBlock(name, e.Ok.Body); t != "" {
				return t
			}
		}
		if e.Err != nil && e.Err.Body != nil {
			if t := findTypeInCheckerBlock(name, e.Err.Body); t != "" {
				return t
			}
		}
	case *checker.ConditionalMatch:
	case *checker.TryOp:
		if e.CatchVar == name {
			// Return the error type if known
			if e.ErrType != nil {
				return checkerTypeString(e.ErrType)
			}
			return "?"
		}
		if e.CatchBlock != nil {
			if t := findTypeInCheckerBlock(name, e.CatchBlock); t != "" {
				return t
			}
		}
	}
	return ""
}

// findTypeInCheckerBlock walks a checker Block for variable declarations.
func findTypeInCheckerBlock(name string, block *checker.Block) string {
	if block == nil {
		return ""
	}
	for _, stmt := range block.Stmts {
		if t := findTypeInNonProducing(name, stmt.Stmt); t != "" {
			return t
		}
		if t := findTypeInExpr(name, stmt.Expr); t != "" {
			return t
		}
	}
	return ""
}

// findTypeInCheckerIf walks an If expression's branches.
func findTypeInCheckerIf(name string, e *checker.If) string {
	for _, branch := range e.Branches {
		if branch.Body != nil {
			if t := findTypeInCheckerBlock(name, branch.Body); t != "" {
				return t
			}
		}
	}
	if e.Else != nil {
		return findTypeInCheckerBlock(name, e.Else)
	}
	return ""
}

// scanForType searches parse-tree statements for a declaration matching name.
func scanForType(name string, stmts []parse.Statement, prog *parse.Program, filePath string) *hoverInfo {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parse.VariableDeclaration:
			if s.Name == name {
				return describeVariableDecl(s, "", filePath, prog)
			}
		case *parse.FunctionDeclaration:
			if s.Name == name {
				return describeFunctionDecl(s)
			}
			for _, p := range s.Parameters {
				if p.Name == name {
					return describeParam(&p)
				}
			}
			if info := scanForType(name, s.Body, prog, filePath); info != nil {
				return info
			}
		case *parse.ImplBlock:
			if s.Target.Name == name || s.Receiver.Name == name {
				return simpleHover(s.Target.Name)
			}
			for i := range s.Methods {
				if info := scanForType(name, []parse.Statement{&s.Methods[i]}, prog, filePath); info != nil {
					return info
				}
			}
		case *parse.TraitImplementation:
			if traitName := simpleExprName(s.Trait); traitName == name {
				return simpleHover(traitName)
			}
			if s.ForType.Name == name || s.Receiver.Name == name {
				return simpleHover(s.ForType.Name)
			}
			for i := range s.Methods {
				if info := scanForType(name, []parse.Statement{&s.Methods[i]}, prog, filePath); info != nil {
					return info
				}
			}
		case *parse.TraitDefinition:
			if s.Name.Name == name {
				return simpleHover(s.Name.Name)
			}
			for i := range s.Methods {
				if info := scanForType(name, []parse.Statement{&s.Methods[i]}, prog, filePath); info != nil {
					return info
				}
			}
		case *parse.IfStatement:
			if info := scanForType(name, s.Body, prog, filePath); info != nil {
				return info
			}
			if s.Else != nil {
				if info := scanForType(name, []parse.Statement{s.Else}, prog, filePath); info != nil {
					return info
				}
			}
		case *parse.WhileLoop:
			if info := scanForType(name, s.Body, prog, filePath); info != nil {
				return info
			}
		case *parse.ForInLoop:
			if s.Cursor.Name == name {
				return simpleHover(inferLoopCursorType(s.Iterable, false, prog, filePath))
			}
			if s.Cursor2.Name == name {
				return simpleHover(inferLoopCursorType(s.Iterable, true, prog, filePath))
			}
			if info := scanForType(name, s.Body, prog, filePath); info != nil {
				return info
			}
		case *parse.RangeLoop:
			if s.Cursor.Name == name || s.Cursor2.Name == name {
				return simpleHover("Int")
			}
			if info := scanForType(name, s.Body, prog, filePath); info != nil {
				return info
			}
		case *parse.ForLoop:
			if info := scanForType(name, s.Body, prog, filePath); info != nil {
				return info
			}
		case *parse.BlockExpression:
			if info := scanForType(name, s.Statements, prog, filePath); info != nil {
				return info
			}
		case *parse.Try:
			if s.CatchVar != nil && s.CatchVar.Name == name {
				return simpleHover("?")
			}
		}
	}
	return nil
}

func describeModuleAlias(name string, prog *parse.Program) *hoverInfo {
	if prog == nil {
		return nil
	}
	for _, imp := range prog.Imports {
		if imp.Name == name {
			return simpleHover(fmt.Sprintf("module %s: %s", imp.Name, imp.Path))
		}
	}
	return nil
}

// builtinType returns hover info for known built-in identifiers.
func builtinType(name string) *hoverInfo {
	switch name {
	case "true", "false":
		return simpleHover("Bool")
	case "print", "println":
		return simpleHover("fn (Str) Void")
	case "panic":
		return simpleHover("fn (Str) Void")
	}
	return nil
}

// describeVariableDecl returns hover info for a variable declaration.
func describeVariableDecl(vd *parse.VariableDeclaration, source string, filePath string, prog *parse.Program) *hoverInfo {
	if vd.Type != nil {
		return simpleHover(typeDeclString(vd.Type))
	}

	// Try the checker for resolved types (handles complex initializers)
	if prog != nil {
		if t := resolveFromChecker(vd.Name, prog, filePath); t != "" && t != "?" {
			return simpleHover(t)
		}
	}

	// Fall back to parse-tree inference
	if vd.Value != nil {
		inferred := inferExprType(vd.Value, prog, filePath)
		if inferred != "" && inferred != "?" {
			return simpleHover(inferred)
		}
	}
	return simpleHover("?")
}

// describeInstanceProperty returns hover info for an instance field access.
func describeInstanceProperty(ip *parse.InstanceProperty, source string, filePath string, prog *parse.Program) *hoverInfo {
	ownerType, fieldType := resolveInstancePropertyType(ip, prog, filePath)
	ownerType = qualifyTypeDisplay(ownerType, prog, filePath)
	fieldType = qualifyTypeDisplay(fieldType, prog, filePath)
	if ownerType != "" && fieldType != "" && fieldType != "?" {
		return simpleHover(fmt.Sprintf("%s.%s: %s", ownerType, ip.Property.Name, fieldType))
	}
	if fieldType != "" && fieldType != "?" {
		return simpleHover(fmt.Sprintf(".%s: %s", ip.Property.Name, fieldType))
	}
	return simpleHover(fmt.Sprintf(".%s", ip.Property.Name))
}

// resolveInstancePropertyType returns the receiver type and field type for a field access.
func resolveInstancePropertyType(ip *parse.InstanceProperty, prog *parse.Program, filePath string) (string, string) {
	if ip == nil || prog == nil {
		return "", ""
	}

	ownerType := normalizeDisplayType(inferExprType(ip.Target, prog, filePath))
	ownerType = strings.TrimSuffix(ownerType, "?")
	if ownerType == "" || ownerType == "?" {
		return "", ""
	}

	if fieldType := findStructFieldType(ownerType, ip.Property.Name, prog.Statements); fieldType != "" {
		return ownerType, fieldType
	}
	if fieldType := findImportedStructFieldType(ownerType, ip.Property.Name, prog, filePath); fieldType != "" {
		return ownerType, fieldType
	}
	return ownerType, ""
}

func findStructFieldType(structName string, fieldName string, stmts []parse.Statement) string {
	for _, stmt := range stmts {
		s, ok := stmt.(*parse.StructDefinition)
		if !ok || s.Name.Name != structName {
			continue
		}
		for _, field := range s.Fields {
			if field.Name.Name == fieldName {
				return typeDeclString(field.Type)
			}
		}
	}
	return ""
}

func findImportedStructFieldType(ownerType string, fieldName string, prog *parse.Program, filePath string) string {
	importedType, ok := importedTypeForDisplay(ownerType, prog, filePath)
	if !ok {
		return ""
	}
	structDef, ok := importedType.(*checker.StructDef)
	if !ok {
		return ""
	}
	fieldType := structDef.Fields[fieldName]
	if fieldType == nil {
		return ""
	}
	return substituteImportedGenericDisplay(checkerTypeString(fieldType), importedType, ownerType)
}

// describeInstanceMethod returns hover info for an instance method call.
func describeInstanceMethod(im *parse.InstanceMethod, source string, filePath string, prog *parse.Program) *hoverInfo {
	if sig := resolveInstanceMethodSignature(im, prog, filePath); sig != nil {
		qualifyMethodSignature(sig, prog, filePath)
		return simpleHover(formatMethodSignature(sig))
	}
	return simpleHover(fmt.Sprintf(".%s(...)", im.Method.Name))
}

func resolveInstanceMethodSignature(im *parse.InstanceMethod, prog *parse.Program, filePath string) *hoverMethodSignature {
	if im == nil || prog == nil {
		return nil
	}

	ownerType := inferExprType(im.Target, prog, filePath)
	if ownerType == "" || ownerType == "?" {
		return nil
	}
	ownerType = normalizeDisplayType(ownerType)

	if sig := builtinMethodSignature(ownerType, im.Method.Name); sig != nil {
		return sig
	}
	if sig := findInstanceMethodSignature(ownerType, im.Method.Name, prog.Statements); sig != nil {
		return sig
	}
	return findImportedInstanceMethodSignature(ownerType, im.Method.Name, prog, filePath)
}

func findInstanceMethodSignature(ownerType string, methodName string, stmts []parse.Statement) *hoverMethodSignature {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parse.ImplBlock:
			if s.Target.Name != ownerType {
				continue
			}
			for i := range s.Methods {
				if s.Methods[i].Name == methodName {
					return methodDeclSignature(ownerType, &s.Methods[i])
				}
			}
		case *parse.TraitImplementation:
			if s.ForType.Name != ownerType {
				continue
			}
			for i := range s.Methods {
				if s.Methods[i].Name == methodName {
					return methodDeclSignature(ownerType, &s.Methods[i])
				}
			}
		}
	}
	return nil
}

func findImportedInstanceMethodSignature(ownerType string, methodName string, prog *parse.Program, filePath string) *hoverMethodSignature {
	if sig := directGoInstanceMethodSignature(ownerType, methodName, prog, filePath); sig != nil {
		return sig
	}
	importedType, ok := importedTypeForDisplay(ownerType, prog, filePath)
	if !ok {
		return nil
	}

	var method *checker.FunctionDef
	switch def := importedType.(type) {
	case *checker.StructDef:
		method = importedStructMethodsForDisplay(ownerType, def, prog, filePath)[methodName]
	case *checker.Enum:
		method = def.Methods[methodName]
	case *checker.Trait:
		for _, traitMethod := range def.GetMethods() {
			if traitMethod.Name == methodName {
				m := traitMethod
				method = &m
				break
			}
		}
	}
	if method == nil {
		return nil
	}
	sig := checkerMethodSignature(ownerType, method)
	substituteImportedGenericSignature(sig, importedType, ownerType)
	return sig
}

func directGoInstanceMethodSignature(ownerType string, methodName string, prog *parse.Program, filePath string) *hoverMethodSignature {
	normalizedOwner := normalizeDisplayType(ownerType)
	mutableOwner := strings.HasPrefix(normalizedOwner, "mut ")
	displayOwner := strings.TrimPrefix(normalizedOwner, "mut ")
	alias, typeName, ok := importedTypeDisplayParts(displayOwner)
	if !ok {
		return nil
	}
	imp, ok := directGoImportForAlias(alias, prog)
	if !ok {
		return nil
	}
	pkg, ok := loadDirectGoPackage(imp, filePath)
	if !ok {
		return nil
	}
	typ, ok := pkg.Types[typeName]
	if !ok {
		return nil
	}
	method, ok := typ.Methods[methodName]
	if !ok || method.Signature.Variadic {
		return nil
	}
	return directGoInstanceMethodSignatureFromGoSignature(displayOwner, mutableOwner, methodName, method.Signature, directGoHoverContextForImport(imp, prog))
}

func directGoInstanceMethodSignatureFromGoSignature(displayOwner string, mutableOwner bool, methodName string, signature checker.GoSignature, ctx directGoHoverContext) *hoverMethodSignature {
	if signature.Variadic {
		return nil
	}
	mutates := false
	if signature.Receiver != nil {
		receiverType, ok := directGoHoverReturnType(*signature.Receiver, ctx)
		if !ok {
			return nil
		}
		mutates = strings.HasPrefix(receiverType, "mut ")
		if mutates && !mutableOwner {
			return nil
		}
	}
	params := make([]hoverParam, len(signature.Params))
	for i, param := range signature.Params {
		paramType, ok := directGoHoverParamType(param, ctx, true)
		if !ok {
			return nil
		}
		paramName := param.ParamName
		if paramName == "" || paramName == "_" {
			paramName = fmt.Sprintf("arg%d", i)
		}
		params[i] = hoverParam{Name: paramName, Type: paramType}
	}
	returnType, ok := directGoHoverReturn(signature.Results, ctx)
	if !ok {
		return nil
	}
	return &hoverMethodSignature{OwnerType: displayOwner, Name: methodName, Params: params, ReturnType: returnType, Mutates: mutates}
}

func checkerMethodSignature(ownerType string, fd *checker.FunctionDef) *hoverMethodSignature {
	return &hoverMethodSignature{
		OwnerType:  ownerType,
		Name:       fd.Name,
		Params:     checkerHoverParams(fd.Parameters),
		ReturnType: checkerTypeString(fd.ReturnType),
		Mutates:    fd.Mutates,
	}
}

func methodDeclSignature(ownerType string, fd *parse.FunctionDeclaration) *hoverMethodSignature {
	params := make([]hoverParam, len(fd.Parameters))
	for i, p := range fd.Parameters {
		paramType := "?"
		if p.Type != nil {
			paramType = normalizeDisplayType(typeDeclString(p.Type))
		}
		params[i] = hoverParam{Name: p.Name, Type: paramType, Mutable: p.Mutable}
	}

	retType := "Void"
	if fd.ReturnType != nil {
		retType = normalizeDisplayType(typeDeclString(fd.ReturnType))
	}

	return &hoverMethodSignature{
		OwnerType:  ownerType,
		Name:       fd.Name,
		Params:     params,
		ReturnType: retType,
		Mutates:    fd.Mutates,
	}
}

func builtinMethodSignature(ownerType string, methodName string) *hoverMethodSignature {
	ownerType = normalizeDisplayType(ownerType)
	if sig := primitiveMethodSignature(ownerType, methodName); sig != nil {
		return sig
	}
	if sig := listMethodSignature(ownerType, methodName); sig != nil {
		return sig
	}
	if sig := mapMethodSignature(ownerType, methodName); sig != nil {
		return sig
	}
	if sig := maybeMethodSignature(ownerType, methodName); sig != nil {
		return sig
	}
	if sig := resultMethodSignature(ownerType, methodName); sig != nil {
		return sig
	}
	return nil
}

func primitiveMethodSignature(ownerType string, methodName string) *hoverMethodSignature {
	mk := func(ret string, params ...hoverParam) *hoverMethodSignature {
		return &hoverMethodSignature{OwnerType: ownerType, Name: methodName, Params: params, ReturnType: ret}
	}

	switch ownerType {
	case "Str":
		switch methodName {
		case "at":
			return mk("Str?", hoverParam{Name: "index", Type: "Int"})
		case "contains":
			return mk("Bool", hoverParam{Name: "sub", Type: "Str"})
		case "is_empty":
			return mk("Bool")
		case "replace", "replace_all":
			return mk("Str", hoverParam{Name: "old", Type: "Str"}, hoverParam{Name: "new", Type: "Str"})
		case "size":
			return mk("Int")
		case "split":
			return mk("[Str]", hoverParam{Name: "delimeter", Type: "Str"})
		case "starts_with", "ends_with":
			return mk("Bool", hoverParam{Name: "str", Type: "Str"})
		case "to_str":
			return mk("Str")
		case "to_dyn":
			return mk("Dynamic")
		case "trim":
			return mk("Str")
		}
	case "Int":
		switch methodName {
		case "to_str":
			return mk("Str")
		case "to_dyn":
			return mk("Dynamic")
		}
	case "Float":
		switch methodName {
		case "to_str":
			return mk("Str")
		case "to_dyn":
			return mk("Dynamic")
		case "to_int":
			return mk("Int")
		}
	case "Bool":
		switch methodName {
		case "to_str":
			return mk("Str")
		case "to_dyn":
			return mk("Dynamic")
		}
	}
	return nil
}

func listMethodSignature(ownerType string, methodName string) *hoverMethodSignature {
	elementType, ok := listElementType(ownerType)
	if !ok {
		return nil
	}

	mk := func(ret string, mutates bool, params ...hoverParam) *hoverMethodSignature {
		return &hoverMethodSignature{OwnerType: ownerType, Name: methodName, Params: params, ReturnType: ret, Mutates: mutates}
	}

	switch methodName {
	case "at":
		return mk(elementType, false, hoverParam{Name: "index", Type: "Int"})
	case "prepend", "push":
		return mk("Int", true, hoverParam{Name: "value", Type: elementType})
	case "set":
		return mk("Bool", true, hoverParam{Name: "index", Type: "Int"}, hoverParam{Name: "value", Type: elementType})
	case "size":
		return mk("Int", false)
	case "sort":
		return mk("Void", true, hoverParam{Name: "cmp", Type: fmt.Sprintf("fn(%s, %s) Bool", elementType, elementType)})
	case "swap":
		return mk("Void", true, hoverParam{Name: "l", Type: "Int"}, hoverParam{Name: "r", Type: "Int"})
	}
	return nil
}

func mapMethodSignature(ownerType string, methodName string) *hoverMethodSignature {
	keyType, valueType, ok := mapEntryTypes(ownerType)
	if !ok {
		return nil
	}

	mk := func(ret string, mutates bool, params ...hoverParam) *hoverMethodSignature {
		return &hoverMethodSignature{OwnerType: ownerType, Name: methodName, Params: params, ReturnType: ret, Mutates: mutates}
	}

	switch methodName {
	case "get":
		return mk(valueType+"?", false, hoverParam{Name: "key", Type: keyType})
	case "keys":
		return mk("["+keyType+"]", false)
	case "set":
		return mk("Bool", true, hoverParam{Name: "key", Type: keyType}, hoverParam{Name: "value", Type: valueType})
	case "drop":
		return mk("Void", true, hoverParam{Name: "key", Type: keyType})
	case "has":
		return mk("Bool", false, hoverParam{Name: "key", Type: keyType})
	case "size":
		return mk("Int", false)
	}
	return nil
}

func maybeMethodSignature(ownerType string, methodName string) *hoverMethodSignature {
	innerType, ok := maybeInnerType(ownerType)
	if !ok {
		return nil
	}

	ownerType = innerType + "?"
	mk := func(ret string, params ...hoverParam) *hoverMethodSignature {
		return &hoverMethodSignature{OwnerType: ownerType, Name: methodName, Params: params, ReturnType: ret}
	}

	switch methodName {
	case "expect":
		return mk(innerType, hoverParam{Name: "message", Type: "Str"})
	case "is_none":
		return mk("Bool")
	case "is_some":
		return mk("Bool")
	case "or":
		return mk(innerType, hoverParam{Name: "default", Type: innerType})
	case "map":
		return mk("$Mapped?", hoverParam{Name: "with", Type: fmt.Sprintf("fn(%s) $Mapped", innerType)})
	case "and_then":
		return mk("$Mapped?", hoverParam{Name: "with", Type: fmt.Sprintf("fn(%s) $Mapped?", innerType)})
	}
	return nil
}

func resultMethodSignature(ownerType string, methodName string) *hoverMethodSignature {
	valType, errType, ok := resultTypes(ownerType)
	if !ok {
		return nil
	}

	mk := func(ret string, params ...hoverParam) *hoverMethodSignature {
		return &hoverMethodSignature{OwnerType: ownerType, Name: methodName, Params: params, ReturnType: ret}
	}

	switch methodName {
	case "expect":
		return mk(valType, hoverParam{Name: "message", Type: "Str"})
	case "or":
		return mk(valType, hoverParam{Name: "default", Type: valType})
	case "is_ok", "is_err":
		return mk("Bool")
	case "map":
		return mk("$MappedVal!"+errType, hoverParam{Name: "with", Type: fmt.Sprintf("fn(%s) $MappedVal", valType)})
	case "map_err":
		return mk(valType+"!$MappedErr", hoverParam{Name: "with", Type: fmt.Sprintf("fn(%s) $MappedErr", errType)})
	case "and_then":
		return mk("$MappedVal!"+errType, hoverParam{Name: "with", Type: fmt.Sprintf("fn(%s) $MappedVal!%s", valType, errType)})
	}
	return nil
}

func listElementType(ownerType string) (string, bool) {
	inner, ok := bracketedType(ownerType)
	if !ok || topLevelIndex(inner, ':') >= 0 {
		return "", false
	}
	return inner, true
}

func mapEntryTypes(ownerType string) (string, string, bool) {
	inner, ok := bracketedType(ownerType)
	if !ok {
		return "", "", false
	}
	idx := topLevelIndex(inner, ':')
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(inner[:idx]), strings.TrimSpace(inner[idx+1:]), true
}

func bracketedType(ownerType string) (string, bool) {
	ownerType = strings.TrimSpace(ownerType)
	if !strings.HasPrefix(ownerType, "[") || !strings.HasSuffix(ownerType, "]") {
		return "", false
	}
	return strings.TrimSpace(ownerType[1 : len(ownerType)-1]), true
}

func maybeInnerType(ownerType string) (string, bool) {
	ownerType = strings.TrimSpace(ownerType)
	if strings.HasPrefix(ownerType, "?") && len(ownerType) > 1 {
		return strings.TrimSpace(ownerType[1:]), true
	}
	if strings.HasSuffix(ownerType, "?") && len(ownerType) > 1 {
		return strings.TrimSpace(ownerType[:len(ownerType)-1]), true
	}
	return "", false
}

func resultTypes(ownerType string) (string, string, bool) {
	idx := topLevelIndex(ownerType, '!')
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(ownerType[:idx]), strings.TrimSpace(ownerType[idx+1:]), true
}

func topLevelIndex(s string, sep rune) int {
	bracketDepth := 0
	parenDepth := 0
	angleDepth := 0
	for i, r := range s {
		switch r {
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '<':
			angleDepth++
		case '>':
			angleDepth--
		default:
			if r == sep && bracketDepth == 0 && parenDepth == 0 && angleDepth == 0 {
				return i
			}
		}
	}
	return -1
}

// describeFunctionDecl returns hover info for a function declaration.
func describeFunctionDecl(fd *parse.FunctionDeclaration) *hoverInfo {
	params := make([]string, len(fd.Parameters))
	for i, p := range fd.Parameters {
		t := "?"
		if p.Type != nil {
			t = typeDeclString(p.Type)
		}
		params[i] = fmt.Sprintf("%s: %s", p.Name, t)
	}

	retType := "Void"
	if fd.ReturnType != nil {
		retType = typeDeclString(fd.ReturnType)
	}

	return simpleHover(fmt.Sprintf("fn %s(%s) %s", fd.Name, strings.Join(params, ", "), retType))
}

// describeParam returns hover info for a function parameter.
func describeParam(p *parse.Parameter) *hoverInfo {
	t := "?"
	if p.Type != nil {
		t = typeDeclString(p.Type)
	}
	return simpleHover(t)
}

// describeAnonFunction returns hover info for an anonymous function.
func describeAnonFunction(af *parse.AnonymousFunction) *hoverInfo {
	params := make([]string, len(af.Parameters))
	for i, p := range af.Parameters {
		t := "?"
		if p.Type != nil {
			t = typeDeclString(p.Type)
		}
		params[i] = fmt.Sprintf("%s: %s", p.Name, t)
	}

	retType := "Void"
	if af.ReturnType != nil {
		retType = typeDeclString(af.ReturnType)
	}

	return simpleHover(fmt.Sprintf("fn(%s) %s", strings.Join(params, ", "), retType))
}

// describeFunctionCall returns hover info for a function call expression.
func describeFunctionCall(fc *parse.FunctionCall, source string, filePath string, prog *parse.Program) *hoverInfo {
	// First try to get the resolved signature from the checker.
	if info := resolveCallFromChecker(fc.Name, prog, filePath); info != nil {
		return info
	}

	// Fall back to parse-tree scanning
	if info := findFunctionDecl(fc.Name, prog.Statements); info != nil {
		return info
	}

	return simpleHover(fmt.Sprintf("fn %s(...)", fc.Name))
}

func describeStaticProperty(sp *parse.StaticProperty, source string, filePath string, prog *parse.Program) *hoverInfo {
	target := simpleExprName(sp.Target)
	property := simpleExprName(sp.Property)
	if target == "" || property == "" {
		return simpleHover(fmt.Sprintf("%v::?", sp.Target))
	}
	if info := directGoStaticPropertyHover(target, property, prog, filePath); info != nil {
		return info
	}
	if info := importedStaticPropertyHover(target, property, prog, filePath); info != nil {
		return info
	}
	return simpleHover(fmt.Sprintf("%s::%s", target, property))
}

func directGoStaticPropertyHover(target string, property string, prog *parse.Program, filePath string) *hoverInfo {
	if varType, ok := directGoPackageVariableDisplayType(target, property, prog, filePath); ok {
		return simpleHover(fmt.Sprintf("%s::%s: %s", target, property, varType))
	}
	return nil
}

func importedStaticPropertyHover(target string, property string, prog *parse.Program, filePath string) *hoverInfo {
	mod, lookupName, ok := importedStaticLookup(target, property, prog, filePath)
	if !ok {
		return nil
	}
	if varType := checkerModuleVariableType(mod, lookupName); varType != nil {
		return simpleHover(fmt.Sprintf("%s::%s: %s", target, property, qualifyTypeDisplay(checkerTypeString(varType), prog, filePath)))
	}
	return nil
}

func checkerModuleVariableType(mod checker.Module, name string) checker.Type {
	if mod == nil || mod.Program() == nil {
		return nil
	}
	for _, stmt := range mod.Program().Statements {
		if v, ok := stmt.Stmt.(*checker.VariableDef); ok && v.Name == name {
			return v.Type()
		}
	}
	return nil
}

func describeStaticFunction(sf *parse.StaticFunction, source string, filePath string, prog *parse.Program) *hoverInfo {
	if sig := resolveStaticFunctionSignature(sf, prog, filePath); sig != nil {
		qualifyStaticFunctionSignature(sig, prog, filePath)
		return simpleHover(formatStaticFunctionSignature(sig))
	}
	return simpleHover(fmt.Sprintf("%s::%s(...)", sf.Target, sf.Function.Name))
}

// resolveCallFromChecker finds a function's signature via the checker.
func resolveCallFromChecker(name string, prog *parse.Program, filePath string) *hoverInfo {
	workingDir := filepath.Dir(filePath)
	moduleResolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		return nil // silent — diagnostics handles this
	}

	relPath, err := filepath.Rel(workingDir, filePath)
	if err != nil {
		relPath = filePath
	}

	c := checker.New(relPath, prog, moduleResolver, checker.CheckOptions{})
	c.Check()

	if sym := c.Module().Get(name); !sym.IsZero() {
		return simpleHover(checkerTypeString(sym.Type))
	}
	return nil
}

// findFunctionDecl searches for a function declaration with the given name.
func findFunctionDecl(name string, stmts []parse.Statement) *hoverInfo {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parse.FunctionDeclaration:
			if s.Name == name {
				return describeFunctionDecl(s)
			}
			if info := findFunctionDecl(name, s.Body); info != nil {
				return info
			}
		case *parse.ImplBlock:
			for i := range s.Methods {
				if info := findFunctionDecl(name, []parse.Statement{&s.Methods[i]}); info != nil {
					return info
				}
			}
		case *parse.TraitImplementation:
			for i := range s.Methods {
				if info := findFunctionDecl(name, []parse.Statement{&s.Methods[i]}); info != nil {
					return info
				}
			}
		case *parse.TraitDefinition:
			for i := range s.Methods {
				if info := findFunctionDecl(name, []parse.Statement{&s.Methods[i]}); info != nil {
					return info
				}
			}
		}
	}
	return nil
}

// inferExprType returns a type name for an expression, scanning the AST for declarations.
func inferExprType(expr parse.Expression, prog *parse.Program, filePath string) string {
	if expr == nil {
		return ""
	}

	switch e := expr.(type) {
	case *parse.StrLiteral:
		return "Str"
	case *parse.NumLiteral:
		if strings.ContainsRune(e.Value, '.') {
			return "Float"
		}
		return "Int"
	case *parse.BoolLiteral:
		return "Bool"
	case *parse.VoidLiteral:
		return "Void"
	case *parse.ListLiteral:
		if len(e.Items) == 0 {
			return "[?]"
		}
		itemType := inferExprType(e.Items[0], prog, filePath)
		if itemType == "" || itemType == "?" {
			return "List"
		}
		return "[" + itemType + "]"
	case *parse.MapLiteral:
		return "Map"
	case *parse.Identifier:
		return resolveIdentType(e.Name, prog, filePath)
	case *parse.FunctionCall:
		return resolveFunctionReturnType(e.Name, prog)
	case *parse.InstanceProperty:
		_, fieldType := resolveInstancePropertyType(e, prog, filePath)
		if fieldType != "" {
			return qualifyTypeDisplay(fieldType, prog, filePath)
		}
		return resolveIdentType(e.Property.Name, prog, filePath)
	case *parse.InstanceMethod:
		if sig := resolveInstanceMethodSignature(e, prog, filePath); sig != nil && sig.ReturnType != "" {
			return qualifyTypeDisplay(sig.ReturnType, prog, filePath)
		}
		return resolveFunctionReturnType(e.Method.Name, prog)
	case *parse.StaticFunction:
		if ret := resolveStaticFunctionReturnType(e, prog, filePath); ret != "" {
			return ret
		}
		return "?"
	case *parse.StaticProperty:
		if id, ok := e.Property.(*parse.Identifier); ok {
			if target := simpleExprName(e.Target); target != "" {
				if varType, ok := directGoPackageVariableDisplayType(target, id.Name, prog, filePath); ok {
					return qualifyTypeDisplay(varType, prog, filePath)
				}
			}
			return resolveIdentType(id.Name, prog, filePath)
		}
		return "?"
	case *parse.UnaryExpression:
		if e.Operator == parse.Bang || e.Operator == parse.Not {
			return "Bool"
		}
		return inferExprType(e.Operand, prog, filePath)
	case *parse.BinaryExpression:
		return inferBinaryExprType(e, prog, filePath)
	case *parse.IfStatement:
		if len(e.Body) > 0 {
			if last, ok := e.Body[len(e.Body)-1].(parse.Expression); ok {
				return inferExprType(last, prog, filePath)
			}
		}
		if e.Else != nil {
			if last, ok := e.Else.(parse.Expression); ok {
				return inferExprType(last, prog, filePath)
			}
		}
		return "Void"
	case *parse.StructInstance:
		return e.Name.Name
	}
	return "?"
}

// resolveIdentType resolves an identifier's type by scanning the parse tree.
func resolveIdentType(name string, prog *parse.Program, filePath string) string {
	if prog == nil {
		return "?"
	}
	switch name {
	case "true", "false":
		return "Bool"
	case "print", "println":
		return "Void"
	case "panic":
		return "?"
	}
	return scanIdentType(name, prog.Statements, prog, filePath)
}

// scanIdentType searches parse-tree statements for a declaration matching name.
func scanIdentType(name string, stmts []parse.Statement, prog *parse.Program, filePath string) string {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parse.VariableDeclaration:
			if s.Name == name {
				return typeFromDecl(s, prog, filePath)
			}
		case *parse.FunctionDeclaration:
			if s.Name == name {
				return funcReturnTypeString(s)
			}
			for _, p := range s.Parameters {
				if p.Name == name && p.Type != nil {
					return typeDeclString(p.Type)
				}
			}
			if t := scanIdentType(name, s.Body, prog, filePath); t != "" {
				return t
			}
		case *parse.ImplBlock:
			if s.Receiver.Name == name {
				return s.Target.Name
			}
			for i := range s.Methods {
				if t := scanIdentType(name, []parse.Statement{&s.Methods[i]}, prog, filePath); t != "" {
					return t
				}
			}
		case *parse.TraitImplementation:
			if traitName := simpleExprName(s.Trait); traitName == name {
				return traitName
			}
			if s.Receiver.Name == name {
				return s.ForType.Name
			}
			for i := range s.Methods {
				if t := scanIdentType(name, []parse.Statement{&s.Methods[i]}, prog, filePath); t != "" {
					return t
				}
			}
		case *parse.TraitDefinition:
			if s.Name.Name == name {
				return s.Name.Name
			}
			for i := range s.Methods {
				if t := scanIdentType(name, []parse.Statement{&s.Methods[i]}, prog, filePath); t != "" {
					return t
				}
			}
		case *parse.IfStatement:
			if t := scanIdentType(name, s.Body, prog, filePath); t != "" {
				return t
			}
			if s.Else != nil {
				if t := scanIdentType(name, []parse.Statement{s.Else}, prog, filePath); t != "" {
					return t
				}
			}
		case *parse.WhileLoop:
			if t := scanIdentType(name, s.Body, prog, filePath); t != "" {
				return t
			}
		case *parse.ForInLoop:
			if s.Cursor.Name == name {
				return inferLoopCursorType(s.Iterable, false, prog, filePath)
			}
			if s.Cursor2.Name == name {
				return inferLoopCursorType(s.Iterable, true, prog, filePath)
			}
			if t := scanIdentType(name, s.Body, prog, filePath); t != "" {
				return t
			}
		case *parse.RangeLoop:
			if s.Cursor.Name == name || s.Cursor2.Name == name {
				return "Int"
			}
			if t := scanIdentType(name, s.Body, prog, filePath); t != "" {
				return t
			}
		case *parse.ForLoop:
			if t := scanIdentType(name, s.Body, prog, filePath); t != "" {
				return t
			}
		case *parse.BlockExpression:
			if t := scanIdentType(name, s.Statements, prog, filePath); t != "" {
				return t
			}
		case *parse.Try:
			if s.CatchVar != nil && s.CatchVar.Name == name {
				return "?"
			}
		}
	}
	return ""
}

// inferLoopCursorType returns the parse-inferred item type for a for-in cursor.
func inferLoopCursorType(iterable parse.Expression, index bool, prog *parse.Program, filePath string) string {
	iterableType := inferExprType(iterable, prog, filePath)
	if index {
		if iterableType == "Map" {
			return "?"
		}
		return "Int"
	}
	if iterableType == "Str" {
		return "Str"
	}
	if strings.HasPrefix(iterableType, "[") && strings.HasSuffix(iterableType, "]") && len(iterableType) > 2 {
		return strings.TrimSuffix(strings.TrimPrefix(iterableType, "["), "]")
	}
	return "?"
}

// typeFromDecl returns the type string for a variable declaration.
func typeFromDecl(vd *parse.VariableDeclaration, prog *parse.Program, filePath string) string {
	if vd.Type != nil {
		return typeDeclString(vd.Type)
	}
	if vd.Value != nil {
		return inferExprType(vd.Value, prog, filePath)
	}
	return ""
}

func resolveStaticFunctionReturnType(sf *parse.StaticFunction, prog *parse.Program, filePath string) string {
	if sig := resolveStaticFunctionSignature(sf, prog, filePath); sig != nil {
		return qualifyTypeDisplay(sig.ReturnType, prog, filePath)
	}
	return ""
}

func resolveStaticFunctionSignature(sf *parse.StaticFunction, prog *parse.Program, filePath string) *hoverStaticFunctionSignature {
	if sf == nil || prog == nil {
		return nil
	}
	target := simpleExprName(sf.Target)
	if sig := importedStaticFunctionSignature(target, sf.Function.Name, prog, filePath); sig != nil {
		return sig
	}
	return scanStaticFunctionSignature(target, sf.Function.Name, prog.Statements)
}

func importedStaticFunctionSignature(target string, name string, prog *parse.Program, filePath string) *hoverStaticFunctionSignature {
	if sig := importedDirectGoFunctionSignature(target, name, prog, filePath); sig != nil {
		return sig
	}
	mod, lookupName, ok := importedStaticLookup(target, name, prog, filePath)
	if !ok {
		return nil
	}
	sym := mod.Get(lookupName)
	if sym.IsZero() {
		return nil
	}
	return checkerStaticFunctionSignature(target, name, sym.Type)
}

func directGoPackageVariableDisplayType(target string, name string, prog *parse.Program, filePath string) (string, bool) {
	alias, memberPrefix := splitStaticTarget(target)
	if memberPrefix != "" {
		return "", false
	}
	imp, ok := directGoImportForAlias(alias, prog)
	if !ok {
		return "", false
	}
	pkg, ok := loadDirectGoPackage(imp, filePath)
	if !ok {
		return "", false
	}
	variable, ok := pkg.Variables[name]
	if !ok {
		return "", false
	}
	return directGoHoverReturnType(variable.Type, directGoHoverContextForImport(imp, prog))
}

func importedDirectGoFunctionSignature(target string, name string, prog *parse.Program, filePath string) *hoverStaticFunctionSignature {
	alias, memberPrefix := splitStaticTarget(target)
	imp, ok := directGoImportForAlias(alias, prog)
	if !ok {
		return nil
	}
	pkg, ok := loadDirectGoPackage(imp, filePath)
	if !ok {
		return nil
	}
	ctx := directGoHoverContextForImport(imp, prog)
	if memberPrefix == "" {
		fn, ok := pkg.Functions[name]
		if !ok || fn.Signature.Variadic {
			return nil
		}
		return directGoFunctionHoverSignature(target, name, fn.Signature, false, ctx)
	}
	if strings.Contains(memberPrefix, "::") {
		return nil
	}
	typ, ok := pkg.Types[memberPrefix]
	if !ok {
		return nil
	}
	method, ok := typ.Methods[name]
	if !ok || method.Signature.Variadic {
		return nil
	}
	return directGoFunctionHoverSignature(target, name, method.Signature, true, ctx)
}

func directGoImportForAlias(alias string, prog *parse.Program) (parse.Import, bool) {
	if prog == nil {
		return parse.Import{}, false
	}
	for _, imp := range prog.Imports {
		if imp.Kind == parse.ImportKindGo && imp.Name == alias {
			return imp, true
		}
	}
	return parse.Import{}, false
}

func loadDirectGoPackage(imp parse.Import, filePath string) (*checker.GoPackage, bool) {
	dir := "."
	if filePath != "" {
		dir = filepath.Dir(filePath)
	}
	pkg, err := checker.NewGoPackagesResolver(dir).LoadPackage(imp.Path)
	if err != nil {
		log.Printf("hover: direct Go package %s load error: %v", imp.Path, err)
		return nil, false
	}
	return pkg, true
}

type directGoHoverContext struct {
	currentImportPath string
	currentAlias      string
	aliasesByPath     map[string]string
}

func directGoHoverContextForImport(imp parse.Import, prog *parse.Program) directGoHoverContext {
	ctx := directGoHoverContext{currentImportPath: imp.Path, currentAlias: imp.Name, aliasesByPath: map[string]string{}}
	if prog != nil {
		for _, imported := range prog.Imports {
			if imported.Kind == parse.ImportKindGo {
				ctx.aliasesByPath[imported.Path] = imported.Name
			}
		}
	}
	return ctx
}

func directGoFunctionHoverSignature(qualifier string, name string, signature checker.GoSignature, includeReceiver bool, ctx directGoHoverContext) *hoverStaticFunctionSignature {
	params := make([]hoverParam, 0, len(signature.Params)+1)
	if includeReceiver && signature.Receiver != nil {
		receiverType, ok := directGoHoverReturnType(*signature.Receiver, ctx)
		if !ok {
			return nil
		}
		params = append(params, hoverParam{Name: "receiver", Type: receiverType})
	}
	for i, param := range signature.Params {
		paramType, ok := directGoHoverParamType(param, ctx, true)
		if !ok {
			return nil
		}
		paramName := param.ParamName
		if paramName == "" || paramName == "_" {
			paramName = fmt.Sprintf("arg%d", i)
		}
		params = append(params, hoverParam{Name: paramName, Type: paramType})
	}
	returnType, ok := directGoHoverReturn(signature.Results, ctx)
	if !ok {
		return nil
	}
	return &hoverStaticFunctionSignature{Qualifier: qualifier, Name: name, Params: params, ReturnType: returnType}
}

func directGoHoverReturn(results []checker.GoValueType, ctx directGoHoverContext) (string, bool) {
	switch len(results) {
	case 0:
		return "Void", true
	case 1:
		if results[0].Kind == checker.GoValueError {
			return "Void!Str", true
		}
		return directGoHoverReturnType(results[0], ctx)
	case 2:
		value, ok := directGoHoverReturnType(results[0], ctx)
		if !ok {
			return "", false
		}
		switch results[1].Kind {
		case checker.GoValueError:
			return directGoHoverPostfixBase(value) + "!Str", true
		case checker.GoValueBool:
			if !results[1].Named {
				return directGoHoverPostfixBase(value) + "?", true
			}
		}
	}
	return "", false
}

func directGoHoverPostfixBase(value string) string {
	if strings.HasPrefix(value, "mut ") {
		return "(" + value + ")"
	}
	return value
}

func directGoHoverParamType(goType checker.GoValueType, ctx directGoHoverContext, topLevel bool) (string, bool) {
	if goType.Kind == checker.GoValuePointer {
		return directGoHoverPointerType(goType, ctx)
	}
	if goType.Named && goType.Name != "" {
		return directGoHoverNamedType(goType, ctx), true
	}
	switch goType.Kind {
	case checker.GoValueBool:
		return "Bool", true
	case checker.GoValueString:
		return "Str", true
	case checker.GoValueInt:
		if goType.Bits == 32 {
			return "Rune", true
		}
		if topLevel || goType.Bits == 0 {
			return "Int", true
		}
	case checker.GoValueUint:
		if goType.Bits == 8 {
			return "Byte", true
		}
		if topLevel {
			return "Int", true
		}
	case checker.GoValueFloat:
		if topLevel || goType.Bits == 64 {
			return "Float", true
		}
	case checker.GoValueAny:
		return "Dynamic", true
	case checker.GoValueSlice:
		if goType.Elem == nil {
			return "", false
		}
		elem, ok := directGoHoverParamType(*goType.Elem, ctx, false)
		if !ok {
			return "", false
		}
		return "[" + elem + "]", true
	case checker.GoValueMap:
		if goType.Key == nil || goType.Value == nil {
			return "", false
		}
		key, keyOK := directGoHoverParamType(*goType.Key, ctx, false)
		value, valueOK := directGoHoverParamType(*goType.Value, ctx, false)
		if !keyOK || !valueOK {
			return "", false
		}
		return "[" + key + ": " + value + "]", true
	}
	return "", false
}

func directGoHoverReturnType(goType checker.GoValueType, ctx directGoHoverContext) (string, bool) {
	if goType.Kind == checker.GoValuePointer {
		return directGoHoverPointerType(goType, ctx)
	}
	if goType.Named && goType.Name != "" {
		return directGoHoverNamedType(goType, ctx), true
	}
	switch goType.Kind {
	case checker.GoValueBool:
		return "Bool", true
	case checker.GoValueString:
		return "Str", true
	case checker.GoValueInt:
		if goType.Bits == 0 {
			return "Int", true
		}
		if goType.Bits == 32 {
			return "Rune", true
		}
	case checker.GoValueUint:
		if goType.Bits == 8 {
			return "Byte", true
		}
	case checker.GoValueFloat:
		if goType.Bits == 64 {
			return "Float", true
		}
	case checker.GoValueAny:
		return "Dynamic", true
	case checker.GoValueSlice:
		if goType.Elem == nil {
			return "", false
		}
		elem, ok := directGoHoverReturnType(*goType.Elem, ctx)
		if !ok {
			return "", false
		}
		return "[" + elem + "]", true
	case checker.GoValueMap:
		if goType.Key == nil || goType.Value == nil {
			return "", false
		}
		key, keyOK := directGoHoverReturnType(*goType.Key, ctx)
		value, valueOK := directGoHoverReturnType(*goType.Value, ctx)
		if !keyOK || !valueOK {
			return "", false
		}
		return "[" + key + ": " + value + "]", true
	}
	return "", false
}

func directGoHoverPointerType(goType checker.GoValueType, ctx directGoHoverContext) (string, bool) {
	if goType.Named || goType.Elem == nil || !goType.Elem.Named || goType.Elem.Kind != checker.GoValueOther {
		return "", false
	}
	elem := directGoHoverNamedType(*goType.Elem, ctx)
	if elem == "" {
		return "", false
	}
	return "mut " + elem, true
}

func directGoHoverNamedType(goType checker.GoValueType, ctx directGoHoverContext) string {
	qualifier := ""
	if goType.ImportPath == ctx.currentImportPath {
		qualifier = ctx.currentAlias
	} else if alias := ctx.aliasesByPath[goType.ImportPath]; alias != "" {
		qualifier = alias
	} else if goType.Package != "" {
		qualifier = goType.Package
	} else {
		qualifier = directGoImportPathBase(goType.ImportPath)
	}
	if qualifier != "" {
		return qualifier + "::" + goType.Name
	}
	return goType.Name
}

func directGoImportPathBase(importPath string) string {
	if idx := strings.LastIndex(importPath, "/"); idx >= 0 {
		return importPath[idx+1:]
	}
	return importPath
}

func importedStaticLookup(target string, name string, prog *parse.Program, filePath string) (checker.Module, string, bool) {
	alias, memberPrefix := splitStaticTarget(target)
	mod, ok := importedModuleForAlias(alias, prog, filePath)
	if !ok {
		return nil, "", false
	}

	lookupName := name
	if memberPrefix != "" {
		lookupName = memberPrefix + "::" + name
	}
	return mod, lookupName, true
}

func splitStaticTarget(target string) (alias string, memberPrefix string) {
	parts := strings.SplitN(target, "::", 2)
	if len(parts) == 1 {
		return target, ""
	}
	return parts[0], parts[1]
}

func preludeModulePath(alias string) string {
	switch alias {
	case "Dynamic":
		return "ard/dynamic"
	case "Float":
		return "ard/float"
	case "Int":
		return "ard/int"
	case "List":
		return "ard/list"
	case "Map":
		return "ard/map"
	case "Str":
		return "ard/string"
	}
	return ""
}

func checkerStaticFunctionSignature(alias string, name string, t checker.Type) *hoverStaticFunctionSignature {
	sig := &hoverStaticFunctionSignature{Qualifier: alias, Name: name}
	switch fn := t.(type) {
	case *checker.FunctionDef:
		sig.Params = checkerHoverParams(fn.Parameters)
		sig.ReturnType = checkerTypeString(fn.ReturnType)
		return sig
	case *checker.ExternalFunctionDef:
		sig.Params = checkerHoverParams(fn.Parameters)
		sig.ReturnType = checkerTypeString(fn.ReturnType)
		return sig
	}
	return nil
}

func checkerHoverParams(params []checker.Parameter) []hoverParam {
	out := make([]hoverParam, len(params))
	for i, p := range params {
		out[i] = hoverParam{Name: p.Name, Type: checkerTypeString(p.Type), Mutable: p.Mutable}
	}
	return out
}

func scanStaticFunctionSignature(target string, name string, stmts []parse.Statement) *hoverStaticFunctionSignature {
	path := target + "::" + name
	for _, stmt := range stmts {
		s, ok := stmt.(*parse.StaticFunctionDeclaration)
		if !ok {
			continue
		}
		if simpleExprName(&s.Path) == path {
			return parseStaticFunctionSignature(target, &s.FunctionDeclaration)
		}
	}
	return nil
}

func parseStaticFunctionSignature(target string, fd *parse.FunctionDeclaration) *hoverStaticFunctionSignature {
	params := make([]hoverParam, len(fd.Parameters))
	for i, p := range fd.Parameters {
		paramType := "?"
		if p.Type != nil {
			paramType = typeDeclString(p.Type)
		}
		params[i] = hoverParam{Name: p.Name, Type: paramType, Mutable: p.Mutable}
	}

	retType := "Void"
	if fd.ReturnType != nil {
		retType = typeDeclString(fd.ReturnType)
	}

	return &hoverStaticFunctionSignature{
		Qualifier:  target,
		Name:       fd.Name,
		Params:     params,
		ReturnType: retType,
	}
}

// resolveFunctionReturnType finds a function declaration and returns its return type.
func resolveFunctionReturnType(name string, prog *parse.Program) string {
	if prog == nil {
		return "?"
	}
	return scanFunctionReturnType(name, prog.Statements)
}

// scanFunctionReturnType searches for a function declaration and returns its return type.
func scanFunctionReturnType(name string, stmts []parse.Statement) string {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parse.FunctionDeclaration:
			if s.Name == name {
				return funcReturnTypeString(s)
			}
			if t := scanFunctionReturnType(name, s.Body); t != "" {
				return t
			}
		case *parse.ImplBlock:
			for i := range s.Methods {
				if t := scanFunctionReturnType(name, []parse.Statement{&s.Methods[i]}); t != "" {
					return t
				}
			}
		case *parse.TraitImplementation:
			for i := range s.Methods {
				if t := scanFunctionReturnType(name, []parse.Statement{&s.Methods[i]}); t != "" {
					return t
				}
			}
		case *parse.TraitDefinition:
			for i := range s.Methods {
				if t := scanFunctionReturnType(name, []parse.Statement{&s.Methods[i]}); t != "" {
					return t
				}
			}
		}
	}
	return "?"
}

// funcReturnTypeString returns the return type string for a function declaration.
func funcReturnTypeString(fd *parse.FunctionDeclaration) string {
	if fd.ReturnType != nil {
		return typeDeclString(fd.ReturnType)
	}
	return "Void"
}

// inferBinaryExprType returns the type of a binary expression.
func inferBinaryExprType(e *parse.BinaryExpression, prog *parse.Program, filePath string) string {
	switch e.Operator {
	case parse.Equal, parse.NotEqual, parse.GreaterThan, parse.GreaterThanOrEqual,
		parse.LessThan, parse.LessThanOrEqual, parse.And, parse.Or:
		return "Bool"
	case parse.Plus:
		left := inferExprType(e.Left, prog, filePath)
		right := inferExprType(e.Right, prog, filePath)
		if left == right {
			return left
		}
		if left == "Str" || right == "Str" {
			return "Str"
		}
		if left == "Float" || right == "Float" {
			return "Float"
		}
		return "Int"
	case parse.Minus, parse.Divide, parse.Multiply, parse.Modulo:
		left := inferExprType(e.Left, prog, filePath)
		if left != "" && left != "?" {
			return left
		}
		return "Int"
	default:
		return inferExprType(e.Left, prog, filePath)
	}
}
