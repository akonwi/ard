package lsp

import (
	"fmt"
	"strings"

	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
)

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
	// Reject zero-value locations — the parser didn't set them.
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
	s := t.GetName()
	if t.IsNullable() {
		s = "?" + s
	}
	return s
}

// bindingStrDecl returns "let" or "mut".
func bindingStrDecl(mutable bool) string {
	if mutable {
		return "mut"
	}
	return "let"
}

// hoverInfo holds the content to display in the hover popup.
type hoverInfo struct {
	content string // Markdown content
}

// computeHover finds the expression at the given position and returns hover info.
func computeHover(source string, position protocol.Position) *hoverInfo {
	target := lspPositionToParsePoint(position)

	// Walk the top-level program to find the expression at the cursor
	expr := findTopLevelExpr(source, target)
	if expr == nil {
		return nil
	}

	return describeExpr(expr, source)
}

// findTopLevelExpr parses the source and walks the AST to find the deepest
// expression at the target point. It also persists the parsed program for type
// resolution in inferExprType.
func findTopLevelExpr(source string, target parse.Point) parse.Expression {
	result := parse.Parse([]byte(source), "")
	if result.Program == nil {
		return nil
	}
	lastParseSource = source
	lastParseProgram = result.Program
	return findInStmts(result.Program.Statements, target)
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
			// If cursor is on the variable name (which spans from after "let " to before ":" or "="),
			// show the variable declaration as the match.
			if s.Value != nil {
				if inner := walkExpr(s.Value, target); inner != nil {
					best = inner
					continue
				}
			}
			// Cursor is not on the value, so it's on the declaration itself (name/type)
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
			// Only show the function signature when cursor is on its first line
			// (the fn keyword / name line), not when somewhere in the body.
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
			if inner := walkExpr(s.Iterable, target); inner != nil {
				best = inner
			}
			if inner := findInStmts(s.Body, target); inner != nil {
				best = inner
			}
			continue
		case *parse.RangeLoop:
			if inner := walkExpr(s.Start, target); inner != nil {
				best = inner
			}
			if inner := walkExpr(s.End, target); inner != nil {
				best = inner
			}
			continue
		case *parse.ForLoop:
			if s.Condition != nil {
				if inner := walkExpr(s.Condition, target); inner != nil {
					best = inner
				}
			}
			continue
		case *parse.MatchExpression:
			if s.Subject != nil {
				if inner := walkExpr(s.Subject, target); inner != nil {
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
		case *parse.StructDefinition, *parse.ImplBlock, *parse.EnumDefinition,
			*parse.TraitDefinition, *parse.TraitImplementation, *parse.ExternTypeDeclaration,
			*parse.ExternalFunction, *parse.TypeDeclaration, *parse.Comment,
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
		// If cursor is on the property name, show that
		if pointInRange(target, e.Property.GetLocation()) {
			return &e.Property
		}
	case *parse.InstanceMethod:
		recurse(e.Target)
	case *parse.StaticProperty:
		recurse(e.Target)
		if inner := walkExpr(e.Property, target); inner != nil {
			best = inner
		}
	case *parse.StaticFunction:
		recurse(e.Target)
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
func describeExpr(expr parse.Expression, source string) *hoverInfo {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *parse.Identifier:
		return describeIdentifier(e, source)
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
			return simpleHover(fmt.Sprintf("%s: %s", e.Name, typeDeclString(e.Type)))
		}
		return simpleHover(fmt.Sprintf("%s: ?", e.Name))
	case *parse.FunctionCall:
		return describeFunctionCall(e, source)
	case *parse.InstanceProperty:
		return simpleHover(fmt.Sprintf(".%s", e.Property.Name))
	case *parse.InstanceMethod:
		return simpleHover(fmt.Sprintf(".%s(...)", e.Method.Name))
	case *parse.StaticProperty:
		if id, ok := e.Property.(*parse.Identifier); ok {
			return simpleHover(fmt.Sprintf("%s::%s", e.Target, id.Name))
		}
		return simpleHover(fmt.Sprintf("%v::?", e.Target))
	case *parse.StaticFunction:
		return simpleHover(fmt.Sprintf("%s::%s(...)", e.Target, e.Function.Name))
	case *parse.VariableDeclaration:
		return describeVariableDecl(e)
	case *parse.FunctionDeclaration:
		return describeFunctionDecl(e)
	case *parse.ListLiteral:
		return simpleHover("List")
	case *parse.MapLiteral:
		return simpleHover("Map")
	case *parse.MatchExpression:
		return simpleHover("match")
	case *parse.Try:
		return simpleHover("try")
	case *parse.BinaryExpression:
		return simpleHover("Bool") // comparisons produce Bool
	case *parse.UnaryExpression:
		if e.Operator == parse.Bang || e.Operator == parse.Not {
			return simpleHover("Bool")
		}
		return simpleHover("Int") // negation preserves numeric type
	case *parse.AnonymousFunction:
		return describeAnonFunction(e)
	case *parse.StructInstance:
		return simpleHover(e.Name.Name)
	case *parse.IfStatement:
		if len(e.Body) > 0 {
			if expr, ok := e.Body[len(e.Body)-1].(parse.Expression); ok {
				return describeExpr(expr, source)
			}
		}
		return simpleHover("Void")
	}

	return nil
}

// describeIdentifier looks up an identifier in the parse tree to find its type.
func describeIdentifier(id *parse.Identifier, source string) *hoverInfo {
	// Re-parse to scan variable declarations
	result := parse.Parse([]byte(source), "")
	if result.Program == nil {
		return nil
	}

	// Scan top-level statements for declarations
	info := scanForType(id.Name, result.Program.Statements)
	if info != nil {
		return info
	}

	// Check if it's a known built-in
	if builtinInfo := builtinType(id.Name); builtinInfo != nil {
		return builtinInfo
	}

	return simpleHover("?")
}

// scanForType searches statements for a declaration matching name.
func scanForType(name string, stmts []parse.Statement) *hoverInfo {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *parse.VariableDeclaration:
			if s.Name == name {
				return describeVariableDecl(s)
			}
		case *parse.FunctionDeclaration:
			if s.Name == name {
				return describeFunctionDecl(s)
			}
			// Check parameters
			for _, p := range s.Parameters {
				if p.Name == name {
					return describeParam(&p)
				}
			}
			// Check body
			if info := scanForType(name, s.Body); info != nil {
				return info
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
			if info := scanForType(name, s.Body); info != nil {
				return info
			}
		case *parse.RangeLoop:
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
				return simpleHover("?") // catch variable, type unknown from parse alone
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
func describeVariableDecl(vd *parse.VariableDeclaration) *hoverInfo {
	if vd.Type != nil {
		return simpleHover(typeDeclString(vd.Type))
	}
	// Try to infer from the value using the full expression resolver
	if vd.Value != nil {
		inferred := inferExprType(vd.Value)
		if inferred != "" && inferred != "?" {
			return simpleHover(inferred)
		}
	}
	return simpleHover("?")
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
	return simpleHover(fmt.Sprintf("%s: %s", p.Name, t))
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
func describeFunctionCall(fc *parse.FunctionCall, source string) *hoverInfo {
	// Try to find the function declaration in the AST
	result := parse.Parse([]byte(source), "")
	if result.Program != nil {
		if info := findFunctionDecl(fc.Name, result.Program.Statements); info != nil {
			return info
		}
	}
	return simpleHover(fmt.Sprintf("fn %s(...)", fc.Name))
}

// findFunctionDecl searches for a function declaration with the given name.
func findFunctionDecl(name string, stmts []parse.Statement) *hoverInfo {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *parse.FunctionDeclaration:
			if s.Name == name {
				return describeFunctionDecl(s)
			}
			// Check nested functions
			if info := findFunctionDecl(name, s.Body); info != nil {
				return info
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
		// Infer element type from the first item
		itemType := inferExprType(e.Items[0])
		if itemType == "" || itemType == "?" {
			return "List"
		}
		return "[" + itemType + "]"
	case *parse.MapLiteral:
		return "Map"
	case *parse.StructInstance:
		return e.Name.Name
	case *parse.Identifier:
		// Look up identifiers — they might reference a variable or function
		return resolveIdentType(e.Name)
	case *parse.FunctionCall:
		// Try to find the function declaration's return type
		return resolveFunctionReturnType(e.Name)
	case *parse.InstanceProperty:
		// Try property access type: infer from the .Name
		return resolveIdentType(e.Property.Name)
	case *parse.StaticProperty:
		if id, ok := e.Property.(*parse.Identifier); ok {
			return resolveIdentType(id.Name)
		}
		return "?"
	case *parse.UnaryExpression:
		if e.Operator == parse.Bang || e.Operator == parse.Not {
			return "Bool"
		}
		// Negation/minus — same type as operand
		return inferExprType(e.Operand)
	case *parse.BinaryExpression:
		return inferBinaryExprType(e)
	case *parse.IfStatement:
		// Return type of if is type of last expression in body/else
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
	}
	return "?"
}

// resolveIdentType resolves an identifier's type by scanning the parse tree.
// Uses a persisted parse from the last call to findTopLevelExpr.
var lastParseSource string
var lastParseProgram *parse.Program

func resolveIdentType(name string) string {
	if lastParseProgram == nil {
		// Scan from the last-known top-level source — caller must set this.
		return "?"
	}
	// Check builtins
	switch name {
	case "true", "false":
		return "Bool"
	case "print", "println":
		return "Void"
	case "panic":
		return "?"
	}
	// Scan top-level declarations
	return scanIdentType(name, lastParseProgram.Statements)
}

// scanIdentType searches statements for a declaration matching name.
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
			// Check parameters
			for _, p := range s.Parameters {
				if p.Name == name && p.Type != nil {
					return typeDeclString(p.Type)
				}
			}
			// Check body
			if t := scanIdentType(name, s.Body); t != "" {
				return t
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
			if t := scanIdentType(name, s.Body); t != "" {
				return t
			}
		case *parse.BlockExpression:
			if t := scanIdentType(name, s.Statements); t != "" {
				return t
			}
		case *parse.Try:
			if s.CatchVar != nil && s.CatchVar.Name == name {
				return "?" // catch variable type unknown from parse
			}
		}
	}
	return ""
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
			// Check nested functions
			if t := scanFunctionReturnType(name, s.Body); t != "" {
				return t
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
		// Str + Str = Str, Int + Int = Int, Float + Float = Float
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
		// Arithmetic: infer from left operand
		left := inferExprType(e.Left)
		if left != "" && left != "?" {
			return left
		}
		return "Int"
	default:
		return inferExprType(e.Left)
	}
}

// simpleHover builds a hoverInfo from a label string.
func simpleHover(label string) *hoverInfo {
	return &hoverInfo{content: fmt.Sprintf("```ard\n%s\n```", label)}
}
