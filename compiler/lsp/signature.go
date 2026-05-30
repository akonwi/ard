package lsp

import (
	"strings"

	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
)

type signatureCallInfo struct {
	label  string
	params []hoverParam
	args   []parse.Argument
	loc    parse.Location
}

func computeSignatureHelp(source string, filePath string, position protocol.Position) *protocol.SignatureHelp {
	target := lspPositionToParsePoint(position)
	parseSource := signatureParseSource(source, position)
	prog := parseAndCache(parseSource, filePath)
	if prog == nil {
		return nil
	}

	call := findSignatureCallInStmts(prog.Statements, target, prog, filePath)
	if call == nil || call.label == "" {
		return nil
	}

	activeParam := activeParameterIndex(source, target, call.loc, call.args, call.params)
	return &protocol.SignatureHelp{
		Signatures: []protocol.SignatureInformation{
			{
				Label:           call.label,
				Parameters:      signatureParameterInformation(call.params),
				ActiveParameter: activeParam,
			},
		},
		ActiveSignature: 0,
		ActiveParameter: activeParam,
	}
}

func signatureParseSource(source string, position protocol.Position) string {
	target := lspPositionToParsePoint(position)
	offset := parsePointToOffset(source, target)
	opens := openParensBefore(source, offset)
	if len(opens) == 0 {
		return source
	}

	insert := ""
	if needsSignaturePlaceholder(source, offset) {
		insert += "__ard_signature_arg__"
	}
	for _, open := range opens {
		if !hasMatchingParen(source, open) {
			insert += ")"
		}
	}
	if insert == "" {
		return source
	}
	return source[:offset] + insert + source[offset:]
}

func signatureParameterInformation(params []hoverParam) []protocol.ParameterInformation {
	out := make([]protocol.ParameterInformation, len(params))
	for i, p := range params {
		out[i] = protocol.ParameterInformation{Label: formatHoverParam(p)}
	}
	return out
}

func formatHoverParam(p hoverParam) string {
	mut := ""
	if p.Mutable {
		mut = "mut "
	}
	if p.Name == "" {
		return mut + normalizeDisplayType(p.Type)
	}
	return mut + p.Name + ": " + normalizeDisplayType(p.Type)
}

func openParensBefore(source string, offset int) []int {
	if offset > len(source) {
		offset = len(source)
	}
	stack := []int{}
	inString := false
	inLineComment := false
	escaped := false
	for i := 0; i < offset; i++ {
		ch := source[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '/' && i+1 < offset && source[i+1] == '/' {
			inLineComment = true
			i++
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '(':
			stack = append(stack, i)
		case ')':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return stack
}

func hasMatchingParen(source string, open int) bool {
	if open < 0 || open >= len(source) || source[open] != '(' {
		return false
	}
	depth := 0
	inString := false
	inLineComment := false
	escaped := false
	for i := open; i < len(source); i++ {
		ch := source[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '/' && i+1 < len(source) && source[i+1] == '/' {
			inLineComment = true
			i++
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

func needsSignaturePlaceholder(source string, offset int) bool {
	i := previousNonWhitespace(source, offset-1)
	if i < 0 {
		return false
	}
	if source[i] == ',' || source[i] == ':' {
		return true
	}
	if !isTypeIdentPart(source[i]) {
		return false
	}
	end := i + 1
	start := i
	for start > 0 && isTypeIdentPart(source[start-1]) {
		start--
	}
	if source[start:end] != "mut" {
		return false
	}
	prev := previousNonWhitespace(source, start-1)
	return prev >= 0 && (source[prev] == '(' || source[prev] == ',')
}

func previousNonWhitespace(source string, i int) int {
	for i >= 0 {
		switch source[i] {
		case ' ', '\t', '\n', '\r':
			i--
		default:
			return i
		}
	}
	return -1
}

func findSignatureCallInStmts(stmts []parse.Statement, target parse.Point, prog *parse.Program, filePath string) *signatureCallInfo {
	var best *signatureCallInfo
	for _, stmt := range stmts {
		if stmt == nil || !pointInRange(target, stmt.GetLocation()) {
			continue
		}

		switch s := stmt.(type) {
		case *parse.VariableDeclaration:
			best = betterSignatureCall(best, findSignatureCallInExpr(s.Value, target, prog, filePath))
		case *parse.VariableAssignment:
			best = betterSignatureCall(best, findSignatureCallInExpr(s.Target, target, prog, filePath))
			best = betterSignatureCall(best, findSignatureCallInExpr(s.Value, target, prog, filePath))
		case *parse.FunctionDeclaration:
			best = betterSignatureCall(best, findSignatureCallInStmts(s.Body, target, prog, filePath))
		case *parse.IfStatement:
			best = betterSignatureCall(best, findSignatureCallInExpr(s.Condition, target, prog, filePath))
			best = betterSignatureCall(best, findSignatureCallInStmts(s.Body, target, prog, filePath))
			if s.Else != nil {
				best = betterSignatureCall(best, findSignatureCallInStmts([]parse.Statement{s.Else}, target, prog, filePath))
			}
		case *parse.WhileLoop:
			best = betterSignatureCall(best, findSignatureCallInExpr(s.Condition, target, prog, filePath))
			best = betterSignatureCall(best, findSignatureCallInStmts(s.Body, target, prog, filePath))
		case *parse.ForInLoop:
			best = betterSignatureCall(best, findSignatureCallInExpr(s.Iterable, target, prog, filePath))
			best = betterSignatureCall(best, findSignatureCallInStmts(s.Body, target, prog, filePath))
		case *parse.RangeLoop:
			best = betterSignatureCall(best, findSignatureCallInExpr(s.Start, target, prog, filePath))
			best = betterSignatureCall(best, findSignatureCallInExpr(s.End, target, prog, filePath))
			best = betterSignatureCall(best, findSignatureCallInStmts(s.Body, target, prog, filePath))
		case *parse.ForLoop:
			if s.Init != nil {
				best = betterSignatureCall(best, findSignatureCallInStmts([]parse.Statement{s.Init}, target, prog, filePath))
			}
			best = betterSignatureCall(best, findSignatureCallInExpr(s.Condition, target, prog, filePath))
			if s.Incrementer != nil {
				best = betterSignatureCall(best, findSignatureCallInStmts([]parse.Statement{s.Incrementer}, target, prog, filePath))
			}
			best = betterSignatureCall(best, findSignatureCallInStmts(s.Body, target, prog, filePath))
		case *parse.MatchExpression:
			best = betterSignatureCall(best, findSignatureCallInExpr(s.Subject, target, prog, filePath))
			for _, matchCase := range s.Cases {
				best = betterSignatureCall(best, findSignatureCallInExpr(matchCase.Pattern, target, prog, filePath))
				best = betterSignatureCall(best, findSignatureCallInStmts(matchCase.Body, target, prog, filePath))
			}
		case *parse.ConditionalMatchExpression:
			for _, matchCase := range s.Cases {
				best = betterSignatureCall(best, findSignatureCallInExpr(matchCase.Condition, target, prog, filePath))
				best = betterSignatureCall(best, findSignatureCallInStmts(matchCase.Body, target, prog, filePath))
			}
		case *parse.Try:
			best = betterSignatureCall(best, findSignatureCallInExpr(s.Expression, target, prog, filePath))
			best = betterSignatureCall(best, findSignatureCallInStmts(s.CatchBlock, target, prog, filePath))
		case *parse.BlockExpression:
			best = betterSignatureCall(best, findSignatureCallInStmts(s.Statements, target, prog, filePath))
		case *parse.StructInstance:
			for _, prop := range s.Properties {
				best = betterSignatureCall(best, findSignatureCallInExpr(prop.Value, target, prog, filePath))
			}
		case *parse.ImplBlock:
			for i := range s.Methods {
				best = betterSignatureCall(best, findSignatureCallInStmts([]parse.Statement{&s.Methods[i]}, target, prog, filePath))
			}
		case *parse.TraitImplementation:
			for i := range s.Methods {
				best = betterSignatureCall(best, findSignatureCallInStmts([]parse.Statement{&s.Methods[i]}, target, prog, filePath))
			}
		case *parse.TraitDefinition:
			for i := range s.Methods {
				best = betterSignatureCall(best, findSignatureCallInStmts([]parse.Statement{&s.Methods[i]}, target, prog, filePath))
			}
		default:
			if expr, ok := stmt.(parse.Expression); ok {
				best = betterSignatureCall(best, findSignatureCallInExpr(expr, target, prog, filePath))
			}
		}
	}
	return best
}

func findSignatureCallInExpr(expr parse.Expression, target parse.Point, prog *parse.Program, filePath string) *signatureCallInfo {
	if expr == nil || !pointInRange(target, expr.GetLocation()) {
		return nil
	}

	var best *signatureCallInfo
	consider := func(call *signatureCallInfo) {
		best = betterSignatureCall(best, call)
	}
	recurse := func(child parse.Expression) {
		consider(findSignatureCallInExpr(child, target, prog, filePath))
	}

	switch e := expr.(type) {
	case *parse.FunctionCall:
		if pointInRange(target, e.Location) {
			consider(signatureForFunctionCall(e, prog, filePath))
		}
		for _, arg := range e.Args {
			recurse(arg.Value)
		}
	case *parse.InstanceMethod:
		if pointInRange(target, e.Method.Location) {
			consider(signatureForInstanceMethod(e, prog, filePath))
		}
		recurse(e.Target)
		for _, arg := range e.Method.Args {
			recurse(arg.Value)
		}
	case *parse.StaticFunction:
		if pointInRange(target, e.Function.Location) {
			consider(signatureForStaticFunction(e, prog, filePath))
		}
		recurse(e.Target)
		for _, arg := range e.Function.Args {
			recurse(arg.Value)
		}
	case *parse.BinaryExpression:
		recurse(e.Left)
		recurse(e.Right)
	case *parse.ChainedComparison:
		for _, operand := range e.Operands {
			recurse(operand)
		}
	case *parse.RangeExpression:
		recurse(e.Start)
		recurse(e.End)
	case *parse.UnaryExpression:
		recurse(e.Operand)
	case *parse.InstanceProperty:
		recurse(e.Target)
	case *parse.StaticProperty:
		recurse(e.Target)
		recurse(e.Property)
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
			consider(findSignatureCallInStmts(matchCase.Body, target, prog, filePath))
		}
	case *parse.ConditionalMatchExpression:
		for _, matchCase := range e.Cases {
			recurse(matchCase.Condition)
			consider(findSignatureCallInStmts(matchCase.Body, target, prog, filePath))
		}
	case *parse.Try:
		recurse(e.Expression)
		consider(findSignatureCallInStmts(e.CatchBlock, target, prog, filePath))
	case *parse.BlockExpression:
		consider(findSignatureCallInStmts(e.Statements, target, prog, filePath))
	case *parse.InterpolatedStr:
		for _, chunk := range e.Chunks {
			recurse(chunk)
		}
	case *parse.IfStatement:
		recurse(e.Condition)
		consider(findSignatureCallInStmts(e.Body, target, prog, filePath))
		if e.Else != nil {
			consider(findSignatureCallInStmts([]parse.Statement{e.Else}, target, prog, filePath))
		}
	case *parse.StructInstance:
		for _, prop := range e.Properties {
			recurse(prop.Value)
		}
	case *parse.AnonymousFunction:
		consider(findSignatureCallInStmts(e.Body, target, prog, filePath))
	}
	return best
}

func betterSignatureCall(current *signatureCallInfo, candidate *signatureCallInfo) *signatureCallInfo {
	if candidate == nil {
		return current
	}
	if current == nil {
		return candidate
	}
	if locationSize(candidate.loc) <= locationSize(current.loc) {
		return candidate
	}
	return current
}

func locationSize(loc parse.Location) int {
	return (loc.End.Row-loc.Start.Row)*100000 + (loc.End.Col - loc.Start.Col)
}

func signatureForFunctionCall(fc *parse.FunctionCall, prog *parse.Program, filePath string) *signatureCallInfo {
	sig := resolveFunctionCallSignature(fc, prog)
	if sig == nil {
		return nil
	}
	qualifyStaticFunctionSignature(sig, prog, filePath)
	return &signatureCallInfo{label: formatStaticFunctionSignature(sig), params: sig.Params, args: fc.Args, loc: fc.Location}
}

func signatureForStaticFunction(sf *parse.StaticFunction, prog *parse.Program, filePath string) *signatureCallInfo {
	sig := resolveStaticFunctionSignature(sf, prog, filePath)
	if sig == nil {
		return nil
	}
	qualifyStaticFunctionSignature(sig, prog, filePath)
	return &signatureCallInfo{label: formatStaticFunctionSignature(sig), params: sig.Params, args: sf.Function.Args, loc: sf.Function.Location}
}

func signatureForInstanceMethod(im *parse.InstanceMethod, prog *parse.Program, filePath string) *signatureCallInfo {
	sig := resolveInstanceMethodSignature(im, prog, filePath)
	if sig == nil {
		return nil
	}
	qualifyMethodSignature(sig, prog, filePath)
	return &signatureCallInfo{label: formatMethodSignature(sig), params: sig.Params, args: im.Method.Args, loc: im.Method.Location}
}

func resolveFunctionCallSignature(fc *parse.FunctionCall, prog *parse.Program) *hoverStaticFunctionSignature {
	if fc == nil || prog == nil {
		return nil
	}
	if sig := findFunctionSignature(fc.Name, prog.Statements); sig != nil {
		return sig
	}
	return builtinFunctionSignature(fc.Name)
}

func findFunctionSignature(name string, stmts []parse.Statement) *hoverStaticFunctionSignature {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parse.FunctionDeclaration:
			if s.Name == name {
				return parseStaticFunctionSignature("", s)
			}
			if sig := findFunctionSignature(name, s.Body); sig != nil {
				return sig
			}
		case *parse.ExternalFunction:
			if s.Name == name {
				return parseExternalFunctionSignature(s)
			}
		case *parse.ImplBlock:
			for i := range s.Methods {
				if sig := findFunctionSignature(name, []parse.Statement{&s.Methods[i]}); sig != nil {
					return sig
				}
			}
		case *parse.TraitImplementation:
			for i := range s.Methods {
				if sig := findFunctionSignature(name, []parse.Statement{&s.Methods[i]}); sig != nil {
					return sig
				}
			}
		case *parse.TraitDefinition:
			for i := range s.Methods {
				if sig := findFunctionSignature(name, []parse.Statement{&s.Methods[i]}); sig != nil {
					return sig
				}
			}
		}
	}
	return nil
}

func parseExternalFunctionSignature(fd *parse.ExternalFunction) *hoverStaticFunctionSignature {
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
		Name:       fd.Name,
		Params:     params,
		ReturnType: retType,
	}
}

func builtinFunctionSignature(name string) *hoverStaticFunctionSignature {
	switch name {
	case "print", "println":
		return &hoverStaticFunctionSignature{Name: name, Params: []hoverParam{{Type: "Str"}}, ReturnType: "Void"}
	case "panic":
		return &hoverStaticFunctionSignature{Name: name, Params: []hoverParam{{Type: "Str"}}, ReturnType: "Void"}
	}
	return nil
}

func activeParameterIndex(source string, target parse.Point, callLoc parse.Location, args []parse.Argument, params []hoverParam) uint32 {
	if len(params) == 0 {
		return 0
	}

	targetOffset := parsePointToOffset(source, target)
	for _, arg := range args {
		if arg.Name == "" {
			continue
		}
		argStart := parsePointToOffset(source, arg.Location.Start)
		argEnd := parsePointToOffset(source, arg.Location.End) + 1
		if !pointInRange(target, arg.Location) && (targetOffset < argStart || targetOffset > argEnd) {
			continue
		}
		for i, p := range params {
			if p.Name == arg.Name {
				return uint32(i)
			}
		}
	}

	openOffset := findCallOpenParen(source, callLoc, targetOffset)
	if openOffset < 0 || targetOffset <= openOffset {
		return 0
	}
	idx := countTopLevelCommas(source[openOffset+1 : targetOffset])
	if idx >= len(params) {
		idx = len(params) - 1
	}
	return uint32(idx)
}

func parsePointToOffset(source string, point parse.Point) int {
	if point.Row <= 1 {
		col := point.Col - 1
		if col < 0 {
			return 0
		}
		if col > len(source) {
			return len(source)
		}
		return col
	}
	offset := 0
	row := 1
	for offset < len(source) && row < point.Row {
		if source[offset] == '\n' {
			row++
		}
		offset++
	}
	col := point.Col - 1
	if col < 0 {
		col = 0
	}
	if offset+col > len(source) {
		return len(source)
	}
	return offset + col
}

func findCallOpenParen(source string, callLoc parse.Location, targetOffset int) int {
	start := parsePointToOffset(source, callLoc.Start)
	if start < 0 {
		start = 0
	}
	if targetOffset > len(source) {
		targetOffset = len(source)
	}
	if start > targetOffset {
		return -1
	}
	idx := strings.IndexByte(source[start:targetOffset], '(')
	if idx < 0 {
		// Cursor can be just before the first argument after the trigger character.
		if targetOffset < len(source) && source[targetOffset] == '(' {
			return targetOffset
		}
		return -1
	}
	return start + idx
}

func countTopLevelCommas(s string) int {
	count := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0
	inString := false
	escaped := false
	for _, r := range s {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case ',':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				count++
			}
		}
	}
	return count
}
