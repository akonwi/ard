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

// parseAndCache caches the parsed program for type resolution.
func parseAndCache(source string, filePath string) *parse.Program {
	result := parse.Parse([]byte(source), filePath)
	if result.Program != nil {
		lastParseSource = source
		lastParseProgram = result.Program
		lastParseFilepath = filePath
	}
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
	case *parse.List:
		s = "[" + typeDeclString(tt.Element) + "]"
	case *parse.Map:
		s = "[" + typeDeclString(tt.Key) + ":" + typeDeclString(tt.Value) + "]"
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
		if inner := walkExpr(e.Property, target); inner != nil {
			best = inner
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
		if id, ok := e.Property.(*parse.Identifier); ok {
			return simpleHover(fmt.Sprintf("%s::%s", e.Target, id.Name))
		}
		return simpleHover(fmt.Sprintf("%v::?", e.Target))
	case *parse.StaticFunction:
		return simpleHover(fmt.Sprintf("%s::%s(...)", e.Target, e.Function.Name))
	case *parse.VariableDeclaration:
		return describeVariableDecl(e, source, filePath, prog)
	case *parse.FunctionDeclaration:
		return describeFunctionDecl(e)
	case *parse.ListLiteral:
		if len(e.Items) == 0 {
			return simpleHover("[?]")
		}
		itemType := inferExprType(e.Items[0])
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
	info := scanForType(id.Name, prog.Statements)
	if info != nil {
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
		return checkerTypeString(sym.Type)
	}

	// Walk the checker's program tree for local variables
	checkerProg := c.Module().Program()
	if checkerProg == nil {
		return ""
	}
	return findTypeInCheckerProg(name, checkerProg.Statements)
}

// findTypeInCheckerProg walks checker statements to find a variable by name.
func findTypeInCheckerProg(name string, stmts []checker.Statement) string {
	for _, stmt := range stmts {
		// Check NonProducing variants
		if t := findTypeInNonProducing(name, stmt.Stmt); t != "" {
			return t
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
		for _, method := range s.Methods {
			if t := findTypeInExpr(name, method); t != "" {
				return t
			}
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
func scanForType(name string, stmts []parse.Statement) *hoverInfo {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parse.VariableDeclaration:
			if s.Name == name {
				return describeVariableDecl(s, "", "", nil)
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
			if info := scanForType(name, s.Body); info != nil {
				return info
			}
		case *parse.ImplBlock:
			if s.Target.Name == name || s.Receiver.Name == name {
				return simpleHover(s.Target.Name)
			}
			for i := range s.Methods {
				if info := scanForType(name, []parse.Statement{&s.Methods[i]}); info != nil {
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
				if info := scanForType(name, []parse.Statement{&s.Methods[i]}); info != nil {
					return info
				}
			}
		case *parse.TraitDefinition:
			if s.Name.Name == name {
				return simpleHover(s.Name.Name)
			}
			for i := range s.Methods {
				if info := scanForType(name, []parse.Statement{&s.Methods[i]}); info != nil {
					return info
				}
			}
		case *parse.IfStatement:
			if info := scanForType(name, s.Body); info != nil {
				return info
			}
			if s.Else != nil {
				if info := scanForType(name, []parse.Statement{s.Else}); info != nil {
					return info
				}
			}
		case *parse.WhileLoop:
			if info := scanForType(name, s.Body); info != nil {
				return info
			}
		case *parse.ForInLoop:
			if s.Cursor.Name == name {
				return simpleHover(inferLoopCursorType(s.Iterable, false))
			}
			if s.Cursor2.Name == name {
				return simpleHover(inferLoopCursorType(s.Iterable, true))
			}
			if info := scanForType(name, s.Body); info != nil {
				return info
			}
		case *parse.RangeLoop:
			if s.Cursor.Name == name || s.Cursor2.Name == name {
				return simpleHover("Int")
			}
			if info := scanForType(name, s.Body); info != nil {
				return info
			}
		case *parse.ForLoop:
			if info := scanForType(name, s.Body); info != nil {
				return info
			}
		case *parse.BlockExpression:
			if info := scanForType(name, s.Statements); info != nil {
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
		inferred := inferExprType(vd.Value)
		if inferred != "" && inferred != "?" {
			return simpleHover(inferred)
		}
	}
	return simpleHover("?")
}

// describeInstanceProperty returns hover info for an instance field access.
func describeInstanceProperty(ip *parse.InstanceProperty, source string, filePath string, prog *parse.Program) *hoverInfo {
	ownerType, fieldType := resolveInstancePropertyType(ip, prog)
	if ownerType != "" && fieldType != "" && fieldType != "?" {
		return simpleHover(fmt.Sprintf("%s.%s: %s", ownerType, ip.Property.Name, fieldType))
	}
	if fieldType != "" && fieldType != "?" {
		return simpleHover(fmt.Sprintf(".%s: %s", ip.Property.Name, fieldType))
	}
	return simpleHover(fmt.Sprintf(".%s", ip.Property.Name))
}

// resolveInstancePropertyType returns the receiver type and field type for a field access.
func resolveInstancePropertyType(ip *parse.InstanceProperty, prog *parse.Program) (string, string) {
	if ip == nil || prog == nil {
		return "", ""
	}

	ownerType := inferExprType(ip.Target)
	ownerType = strings.TrimPrefix(ownerType, "?")
	if ownerType == "" || ownerType == "?" {
		return "", ""
	}

	if fieldType := findStructFieldType(ownerType, ip.Property.Name, prog.Statements); fieldType != "" {
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

// describeInstanceMethod returns hover info for an instance method call.
func describeInstanceMethod(im *parse.InstanceMethod, source string, filePath string, prog *parse.Program) *hoverInfo {
	return simpleHover(fmt.Sprintf(".%s(...)", im.Method.Name))
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
func inferExprType(expr parse.Expression) string {
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
		itemType := inferExprType(e.Items[0])
		if itemType == "" || itemType == "?" {
			return "List"
		}
		return "[" + itemType + "]"
	case *parse.MapLiteral:
		return "Map"
	case *parse.Identifier:
		return resolveIdentType(e.Name)
	case *parse.FunctionCall:
		return resolveFunctionReturnType(e.Name)
	case *parse.InstanceProperty:
		_, fieldType := resolveInstancePropertyType(e, lastParseProgram)
		if fieldType != "" {
			return fieldType
		}
		return resolveIdentType(e.Property.Name)
	case *parse.InstanceMethod:
		// Infer from the method call chain — try the function call fallback
		return resolveFunctionReturnType(e.Method.Name)
	case *parse.StaticProperty:
		if id, ok := e.Property.(*parse.Identifier); ok {
			return resolveIdentType(id.Name)
		}
		return "?"
	case *parse.UnaryExpression:
		if e.Operator == parse.Bang || e.Operator == parse.Not {
			return "Bool"
		}
		return inferExprType(e.Operand)
	case *parse.BinaryExpression:
		return inferBinaryExprType(e)
	case *parse.IfStatement:
		if len(e.Body) > 0 {
			if last, ok := e.Body[len(e.Body)-1].(parse.Expression); ok {
				return inferExprType(last)
			}
		}
		if e.Else != nil {
			if last, ok := e.Else.(parse.Expression); ok {
				return inferExprType(last)
			}
		}
		return "Void"
	case *parse.StructInstance:
		return e.Name.Name
	}
	return "?"
}

// resolveIdentType resolves an identifier's type by scanning the parse tree.
var lastParseSource string
var lastParseFilepath string
var lastParseProgram *parse.Program

func resolveIdentType(name string) string {
	if lastParseProgram == nil {
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
	return scanIdentType(name, lastParseProgram.Statements)
}

// scanIdentType searches parse-tree statements for a declaration matching name.
func scanIdentType(name string, stmts []parse.Statement) string {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parse.VariableDeclaration:
			if s.Name == name {
				return typeFromDecl(s)
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
			if t := scanIdentType(name, s.Body); t != "" {
				return t
			}
		case *parse.ImplBlock:
			if s.Target.Name == name || s.Receiver.Name == name {
				return s.Target.Name
			}
			for i := range s.Methods {
				if t := scanIdentType(name, []parse.Statement{&s.Methods[i]}); t != "" {
					return t
				}
			}
		case *parse.TraitImplementation:
			if traitName := simpleExprName(s.Trait); traitName == name {
				return traitName
			}
			if s.ForType.Name == name || s.Receiver.Name == name {
				return s.ForType.Name
			}
			for i := range s.Methods {
				if t := scanIdentType(name, []parse.Statement{&s.Methods[i]}); t != "" {
					return t
				}
			}
		case *parse.TraitDefinition:
			if s.Name.Name == name {
				return s.Name.Name
			}
			for i := range s.Methods {
				if t := scanIdentType(name, []parse.Statement{&s.Methods[i]}); t != "" {
					return t
				}
			}
		case *parse.IfStatement:
			if t := scanIdentType(name, s.Body); t != "" {
				return t
			}
			if s.Else != nil {
				if t := scanIdentType(name, []parse.Statement{s.Else}); t != "" {
					return t
				}
			}
		case *parse.WhileLoop:
			if t := scanIdentType(name, s.Body); t != "" {
				return t
			}
		case *parse.ForInLoop:
			if s.Cursor.Name == name {
				return inferLoopCursorType(s.Iterable, false)
			}
			if s.Cursor2.Name == name {
				return inferLoopCursorType(s.Iterable, true)
			}
			if t := scanIdentType(name, s.Body); t != "" {
				return t
			}
		case *parse.RangeLoop:
			if s.Cursor.Name == name || s.Cursor2.Name == name {
				return "Int"
			}
			if t := scanIdentType(name, s.Body); t != "" {
				return t
			}
		case *parse.ForLoop:
			if t := scanIdentType(name, s.Body); t != "" {
				return t
			}
		case *parse.BlockExpression:
			if t := scanIdentType(name, s.Statements); t != "" {
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
func inferLoopCursorType(iterable parse.Expression, index bool) string {
	iterableType := inferExprType(iterable)
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
func typeFromDecl(vd *parse.VariableDeclaration) string {
	if vd.Type != nil {
		return typeDeclString(vd.Type)
	}
	if vd.Value != nil {
		return inferExprType(vd.Value)
	}
	return ""
}

// resolveFunctionReturnType finds a function declaration and returns its return type.
func resolveFunctionReturnType(name string) string {
	if lastParseProgram == nil {
		return "?"
	}
	return scanFunctionReturnType(name, lastParseProgram.Statements)
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
func inferBinaryExprType(e *parse.BinaryExpression) string {
	switch e.Operator {
	case parse.Equal, parse.NotEqual, parse.GreaterThan, parse.GreaterThanOrEqual,
		parse.LessThan, parse.LessThanOrEqual, parse.And, parse.Or:
		return "Bool"
	case parse.Plus:
		left := inferExprType(e.Left)
		right := inferExprType(e.Right)
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
		left := inferExprType(e.Left)
		if left != "" && left != "?" {
			return left
		}
		return "Int"
	default:
		return inferExprType(e.Left)
	}
}
