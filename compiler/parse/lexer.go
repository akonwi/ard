package parse

import (
	"strconv"
	"strings"
)

type kind string

const (
	space         kind = "space"
	new_line           = "new_line"
	left_paren    kind = "left_paren"
	right_paren        = "right_paren"
	left_brace         = "left_brace"
	right_brace        = "right_brace"
	left_bracket       = "left_bracket"
	right_bracket      = "right_bracket"
	colon              = "colon"
	semicolon          = "semicolon"
	comma              = "comma"
	dot                = "dot"
	dot_dot            = "dot_dot"
	ellipsis           = "ellipsis"
	question_mark      = "question_mark"
	pipe               = "pipe"
	double_quote       = "double_quote"
	single_quote       = "single_quote"
	backtick           = "backtick"
	dollar             = "dollar"
	at_sign            = "at_sign"

	colon_colon        = "colon_colon"
	bang               = "bang"
	greater_than       = "greater_than"
	less_than          = "less_than"
	greater_than_equal = "greater_than_equal"
	less_than_equal    = "less_than_equal"
	equal              = "equal"
	equal_equal        = "equal_equal"
	bang_equal         = "bang_equal"
	plus               = "plus"
	minus              = "minus"
	star               = "star"
	slash              = "slash"
	percent            = "percent"
	thin_arrow         = "thin_arrow"
	fat_arrow          = "fat_arrow"
	increment          = "increment"
	decrement          = "decrement"
	expr_open          = "expr_open"
	expr_close         = "expr_close"

	// Keywords
	and     = "and"
	not     = "not"
	or      = "or"
	true_   = "true"
	false_  = "false"
	struct_ = "struct"
	enum    = "enum"
	impl    = "impl"
	trait   = "trait"
	fn      = "fn"
	let     = "let"
	mut     = "mut"
	deref   = "deref"
	break_  = "break"
	match   = "match"
	select_ = "select"
	while_  = "while"
	for_    = "for"
	use     = "use"
	as      = "as"
	in      = "in"
	if_     = "if"
	else_   = "else"
	type_   = "type"
	private = "private"
	defer_  = "defer"

	// Types
	int_  = "int"
	float = "float"
	bool_ = "bool"
	str   = "str"

	// Literals
	path       = "path"
	identifier = "identifier"
	number     = "number"
	string_    = "string"
	rune_      = "rune"
	comment    = "comment"

	eof = "eof"
)

type token struct {
	kind                           kind
	line, column                   int
	text                           string
	stringForm                     StringForm
	sourceLength                   int
	sourceEndLine, sourceEndColumn int
	err                            string
}

func (t token) getLocation() Location {
	start := Point{Row: t.line, Col: t.column}
	if t.sourceEndLine > 0 && t.sourceEndColumn > 0 {
		return Location{Start: start, End: Point{Row: t.sourceEndLine, Col: t.sourceEndColumn}}
	}
	length := len(t.text)
	if t.sourceLength > 0 {
		length = t.sourceLength
	}
	return Location{Start: start, End: Point{Row: t.line, Col: t.column + length - 1}}
}

type char struct {
	raw   byte
	index int
	line  int
	col   int
}

func (c char) asToken(kind kind) token {
	return token{
		kind:   kind,
		line:   c.line,
		column: c.col,
	}
}

type stringInterpolation struct {
	form   StringForm
	depth  int
	resume char
	raw    *rawStringState
	open   Location
}

type rawStringChunk struct {
	tokenIndex          int
	start               int
	end                 int
	endsAtInterpolation bool
}

type rawStringState struct {
	form         StringForm
	opener       char
	contentStart int
	chunks       []rawStringChunk
	margin       string
	reportedLine map[int]bool
}

type lexer struct {
	source     []byte
	lineStarts []int
	tokens     []token
	errors     []ParseError
	// position in the source
	cursor int
	// position of the current token to take
	start int
	// position in the source
	line, column int
	strings      []stringInterpolation
}

func NewLexer(source []byte) *lexer {
	return &lexer{
		source:     source,
		lineStarts: sourceLineStarts(source),
		tokens:     []token{},
		cursor:     0,
		line:       1,
		column:     1,
	}
}

func (l *lexer) currentInterpolation() *stringInterpolation {
	if len(l.strings) == 0 {
		return nil
	}
	return &l.strings[len(l.strings)-1]
}

func (l lexer) isAtEnd() bool {
	return l.cursor >= len(l.source)
}
func (l lexer) hasMore() bool {
	return !l.isAtEnd()
}

func (l *lexer) match(byte byte) bool {
	peek := l.peek()
	if peek != nil && peek.raw == byte {
		l.advance()
		return true
	}

	return false
}

func (l *lexer) previous() *char {
	if l.cursor == 0 {
		return nil
	}
	raw := l.source[l.cursor-1]
	return &char{
		raw:   raw,
		line:  l.line,
		col:   l.column,
		index: l.cursor - 1,
	}
}

func (l *lexer) matchNext(byte byte) *char {
	if l.isAtEnd() || l.peek().raw != byte {
		return nil
	}
	return l.advance()
}

func (l *lexer) peekMatch(str string) bool {
	if l.isAtEnd() {
		return false
	}

	for _, r := range str {
		if l.isAtEnd() || l.peek().raw != byte(r) {
			return false
		}
	}
	return true
}

func (l lexer) peek() *char {
	if l.isAtEnd() {
		return nil
	}
	return &char{
		raw:   l.source[l.cursor],
		index: l.cursor,
		line:  l.line,
		col:   l.column,
	}
}

func (l *lexer) check(string string) bool {
	for i, r := range string {
		if l.isAtEnd() || l.source[i+l.cursor] != byte(r) {
			return false
		}
	}
	return true
}

func (l *lexer) advance() *char {
	if l.cursor == len(l.source) {
		return nil
	}
	char := &char{
		raw:   l.source[l.cursor],
		line:  l.line,
		col:   l.column,
		index: l.cursor,
	}
	l.cursor++
	l.column++
	return char
}

func (l *lexer) advanceN(count int) {
	for i := 0; i < count; i++ {
		l.advance()
	}
}

func (l *lexer) takeEscape(quote byte) (rune, int, bool) {
	if l.isAtEnd() || l.source[l.cursor] != '\\' {
		return 0, 0, false
	}
	input := string(l.source[l.cursor:])
	value, _, tail, err := strconv.UnquoteChar(input, quote)
	if err != nil {
		return 0, 0, false
	}
	return value, len(input) - len(tail), true
}

func (c char) isDigit() bool {
	return c.raw >= '0' && c.raw <= '9'
}
func (c char) isAlpha() bool {
	return (c.raw >= 'a' && c.raw <= 'z') || (c.raw >= 'A' && c.raw <= 'Z') || c.raw == '_'
}

func (c char) isAlphaNumeric() bool {
	return c.isAlpha() || c.isDigit()
}

//	func isWhitespace(c byte) bool {
//		return c == ' ' || c == '\t' || c == '\n' || c == '\r'
//	}

func (l *lexer) take() (token, bool) {
	currentChar := l.advance()
	switch currentChar.raw {
	case '\n':
		t := currentChar.asToken(new_line)
		l.line++
		l.column = 1
		return t, true
	case ' ', '\t', '\r':
		return token{}, false
	case '(':
		return currentChar.asToken(left_paren), true
	case ')':
		return currentChar.asToken(right_paren), true
	case '{':
		if interpolation := l.currentInterpolation(); interpolation != nil {
			interpolation.depth++
		}
		return currentChar.asToken(left_brace), true
	case '}':
		if interpolation := l.currentInterpolation(); interpolation != nil {
			if interpolation.depth > 0 {
				interpolation.depth--
				return currentChar.asToken(right_brace), true
			}

			closingToken := currentChar.asToken(expr_close)
			closingToken.sourceLength = 1
			l.tokens = append(l.tokens, closingToken)
			context := *interpolation
			l.strings = l.strings[:len(l.strings)-1]

			resume := char{
				line:  l.line,
				col:   l.column,
				index: l.cursor,
			}
			if context.form == StringFormQuoted {
				return l.takeString(resume)
			}
			l.takeRawStringSegment(context.raw, resume)
			return token{}, false
		}
		return currentChar.asToken(right_brace), true
	case '[':
		return currentChar.asToken(left_bracket), true
	case ']':
		return currentChar.asToken(right_bracket), true
	case ';':
		return currentChar.asToken(semicolon), true
	case ',':
		return currentChar.asToken(comma), true
	case '.':
		if l.matchNext('.') != nil {
			if l.matchNext('.') != nil {
				tok := currentChar.asToken(ellipsis)
				tok.sourceLength = 3
				return tok, true
			}
			return currentChar.asToken(dot_dot), true
		}
		return currentChar.asToken(dot), true
	case '?':
		return currentChar.asToken(question_mark), true
	case '|':
		return currentChar.asToken(pipe), true
	case '@':
		// Simply return the at_sign token
		return currentChar.asToken(at_sign), true
	case '$':
		if l.hasMore() && l.peek().isAlpha() {
			l.start = l.cursor - 1
			return l.takeIdentifier(), true
		}
		return currentChar.asToken(dollar), true
	case '!':
		if l.hasMore() && l.matchNext('=') != nil {
			return currentChar.asToken(bang_equal), true
		}
		return currentChar.asToken(bang), true
	case '+':
		return currentChar.asToken(plus), true
	case '*':
		return currentChar.asToken(star), true
	case '/':
		if l.matchNext('/') != nil {
			return l.comment(currentChar), true
		}
		return currentChar.asToken(slash), true
	case '%':
		return currentChar.asToken(percent), true
	case ':':
		if l.matchNext(':') != nil {
			return currentChar.asToken(colon_colon), true
		}
		return currentChar.asToken(colon), true
	case '>':
		if l.hasMore() && l.matchNext('=') != nil {
			return currentChar.asToken(greater_than_equal), true
		}
		return currentChar.asToken(greater_than), true
	case '<':
		if l.matchNext('=') != nil {
			return currentChar.asToken(less_than_equal), true
		}
		return currentChar.asToken(less_than), true
	case '-':
		if l.hasMore() && l.matchNext('>') != nil {
			return currentChar.asToken(thin_arrow), true
		}
		return currentChar.asToken(minus), true
	case '=':
		if l.matchNext('>') != nil {
			return currentChar.asToken(fat_arrow), true
		}
		if l.matchNext('=') != nil {
			return currentChar.asToken(equal_equal), true
		}
		if l.matchNext('+') != nil {
			return currentChar.asToken(increment), true
		}
		if l.matchNext('-') != nil {
			return currentChar.asToken(decrement), true
		}
		return currentChar.asToken(equal), true
	case '"':
		return l.takeString(*currentChar)
	case '`':
		l.takeRawString(*currentChar)
		return token{}, false
	case '\'':
		return l.takeRune(*currentChar)
	case '\\':
		if interpolation := l.currentInterpolation(); interpolation != nil && interpolation.form == StringFormQuoted && l.hasMore() && l.peek().raw == '"' {
			return l.takeEscapedTemplateString(*currentChar)
		}
		return token{}, false
	default:
		if currentChar.isAlpha() {
			if path, ok := l.takePath(currentChar); ok {
				return path, true
			}
			l.start = l.cursor - 1
			return l.takeIdentifier(), true
		}
		if currentChar.isDigit() {
			l.start = l.cursor - 1
			return l.takeNumber(), true
		}
		return token{}, false
	}
}

func (l *lexer) comment(start *char) token {
	var text strings.Builder
	text.WriteString("//")
	for l.hasMore() && !l.peekMatch(string('\n')) {
		text.WriteByte(l.peek().raw)
		l.advance()
	}
	return token{kind: comment, line: start.line, column: start.col, text: text.String()}
}

func (l *lexer) takeString(start char) (token, bool) {
	sb := strings.Builder{}
	lastConsumed := start
	consumedAny := start.raw != 0
	advance := func() *char {
		consumed := l.advance()
		if consumed != nil {
			lastConsumed = *consumed
			consumedAny = true
		}
		return consumed
	}
	advanceN := func(count int) {
		for i := 0; i < count; i++ {
			advance()
		}
	}

	// Start a new state to track the string contents
	inString := true

	for inString && l.hasMore() {
		currChar := l.peek()
		if currChar == nil {
			break
		}

		// A doubled brace represents one literal brace. Check this before
		// interpolation so `{{` is not mistaken for an expression opening.
		if (currChar.raw == '{' || currChar.raw == '}') && l.cursor+1 < len(l.source) && l.source[l.cursor+1] == currChar.raw {
			sb.WriteByte(currChar.raw)
			advanceN(2)
			continue
		}

		// Handle escape sequences
		if currChar.raw == '\\' {
			if escaped, consumed, ok := l.takeEscape('"'); ok {
				sb.WriteRune(escaped)
				advanceN(consumed)
				continue
			}
			advance() // Consume the backslash
			if l.hasMore() {
				escChar := advance() // Get the escaped character
				if escChar.raw == '{' || escChar.raw == '}' {
					// Continue accepting legacy brace escapes. The formatter
					// canonicalizes these as doubled braces.
					sb.WriteByte(escChar.raw)
				} else {
					// For unrecognized escapes, output both chars.
					sb.WriteByte('\\')
					sb.WriteByte(escChar.raw)
				}
			}
			continue
		}

		// Check for interpolation start
		if currChar.raw == '{' {
			// Use the last consumed source byte rather than deriving the endpoint
			// from the delimiter. This remains correct when `{` starts a new line.
			str := token{kind: string_, line: start.line, column: start.col, text: sb.String()}
			if consumedAny {
				str.sourceEndLine = lastConsumed.line
				str.sourceEndColumn = lastConsumed.col
			}

			// Add the string content token
			l.tokens = append(l.tokens, str)

			// Add the expression open token
			exprChar := l.advance() // Consume the '{'
			l.tokens = append(l.tokens, exprChar.asToken(expr_open))

			// Pause this string until the matching interpolation brace closes.
			l.strings = append(l.strings, stringInterpolation{
				form:   StringFormQuoted,
				resume: start,
				open:   exprChar.asToken(expr_open).getLocation(),
			})
			return token{}, false
		}

		// Check for end of string
		if currChar.raw == '"' {
			advance() // Consume the closing quote
			inString = false
			break
		}

		// Handle newlines properly
		if currChar.raw == '\n' {
			sb.WriteByte(currChar.raw)
			advance()
			l.line++
			l.column = 1
		} else {
			// Regular character
			sb.WriteByte(currChar.raw)
			advance()
		}
	}

	str := token{kind: string_, line: start.line, column: start.col, text: sb.String()}
	if consumedAny {
		str.sourceEndLine = lastConsumed.line
		str.sourceEndColumn = lastConsumed.col
	}
	return str, true
}

func (l *lexer) takeRawString(start char) {
	state := &rawStringState{
		form:         StringFormRawSingleLine,
		opener:       start,
		contentStart: l.cursor,
		reportedLine: map[int]bool{},
	}
	if l.hasMore() && (l.peek().raw == '\n' || l.peek().raw == '\r') {
		state.form = StringFormRawMultiline
		l.advanceRawNewline()
		state.contentStart = l.cursor
	}
	l.takeRawStringSegment(state, start)
}

func (l *lexer) takeRawStringSegment(state *rawStringState, tokenStart char) {
	rawStart := l.cursor
	for l.hasMore() {
		current := l.peek()
		if current == nil {
			break
		}

		if current.raw == '`' {
			contentEnd := current.index
			if state.form == StringFormRawMultiline {
				lineStart := l.physicalLineStart(current.index)
				prefix := l.source[lineStart:current.index]
				if rawIndentation(prefix) {
					state.margin = string(prefix)
					contentEnd = trimBoundaryNewline(l.source, lineStart, state.contentStart)
				} else {
					l.addLexError(current.getLocation(), "Closing backtick for a multiline raw string must be preceded only by indentation")
				}
			}

			closing := l.advance()
			l.appendRawStringChunk(state, tokenStart, rawStart, contentEnd, false, Point{Row: closing.line, Col: closing.col})
			l.finalizeRawString(state)
			return
		}

		if (current.raw == '{' || current.raw == '}') && l.cursor+1 < len(l.source) && l.source[l.cursor+1] == current.raw {
			l.advanceN(2)
			continue
		}

		if current.raw == '{' {
			end := l.pointBefore(current.index, tokenStart)
			l.appendRawStringChunk(state, tokenStart, rawStart, current.index, true, end)
			opening := l.advance()
			openingToken := opening.asToken(expr_open)
			openingToken.sourceLength = 1
			l.tokens = append(l.tokens, openingToken)
			l.strings = append(l.strings, stringInterpolation{
				form: state.form,
				raw:  state,
				open: openingToken.getLocation(),
			})
			return
		}

		if current.raw == '\n' || current.raw == '\r' {
			if state.form == StringFormRawSingleLine && !state.reportedLine[current.line] {
				state.reportedLine[current.line] = true
				l.addLexError(current.getLocation(), "Single-line raw strings cannot contain physical newlines; put a newline immediately after the opening backtick")
			}
			l.advanceRawNewline()
			continue
		}

		l.advance()
	}

	end := l.pointBefore(l.cursor, tokenStart)
	l.appendRawStringChunk(state, tokenStart, rawStart, l.cursor, false, end)
	l.finalizeRawString(state)
	endLocation := Location{Start: state.opener.getLocation().Start, End: end}
	l.addLexError(endLocation, "Unterminated raw string literal")
}

func (l *lexer) appendRawStringChunk(state *rawStringState, tokenStart char, start, end int, endsAtInterpolation bool, sourceEnd Point) {
	if end < start {
		end = start
	}
	entry := token{
		kind:            string_,
		line:            tokenStart.line,
		column:          tokenStart.col,
		stringForm:      state.form,
		sourceEndLine:   sourceEnd.Row,
		sourceEndColumn: sourceEnd.Col,
	}
	index := len(l.tokens)
	l.tokens = append(l.tokens, entry)
	state.chunks = append(state.chunks, rawStringChunk{
		tokenIndex:          index,
		start:               start,
		end:                 end,
		endsAtInterpolation: endsAtInterpolation,
	})
}

func (l *lexer) finalizeRawString(state *rawStringState) {
	for _, chunk := range state.chunks {
		l.tokens[chunk.tokenIndex].text = l.decodeRawStringChunk(state, chunk)
	}
}

func (l *lexer) decodeRawStringChunk(state *rawStringState, chunk rawStringChunk) string {
	var value strings.Builder
	for index := chunk.start; index < chunk.end; {
		if state.form == StringFormRawMultiline && l.isPhysicalLineStart(index) {
			lineEnd := index
			for lineEnd < chunk.end && l.source[lineEnd] != '\n' && l.source[lineEnd] != '\r' {
				lineEnd++
			}
			lineText := l.source[index:lineEnd]
			endsBeforeInterpolation := lineEnd == chunk.end && chunk.endsAtInterpolation
			if lineEnd > index && len(strings.Trim(string(lineText), " \t")) == 0 && !endsBeforeInterpolation {
				index = lineEnd
				continue
			}
			if state.margin != "" {
				if len(lineText) >= len(state.margin) && string(lineText[:len(state.margin)]) == state.margin {
					index += len(state.margin)
				} else if len(strings.Trim(string(lineText), " \t")) > 0 || endsBeforeInterpolation {
					point := l.pointAt(index)
					if !state.reportedLine[point.Row] {
						state.reportedLine[point.Row] = true
						end := point
						if len(lineText) > 0 {
							end.Col += len(lineText) - 1
						}
						l.addLexError(Location{Start: point, End: end}, "Raw string content must begin with the closing delimiter margin")
					}
				}
			}
		}

		if index >= chunk.end {
			break
		}
		current := l.source[index]
		if current == '\r' {
			if index+1 < chunk.end && l.source[index+1] == '\n' {
				index++
			}
			value.WriteByte('\n')
			index++
			continue
		}
		if current == '\n' {
			value.WriteByte('\n')
			index++
			continue
		}
		if (current == '{' || current == '}') && index+1 < chunk.end && l.source[index+1] == current {
			value.WriteByte(current)
			index += 2
			continue
		}
		value.WriteByte(current)
		index++
	}
	return value.String()
}

func (l *lexer) advanceRawNewline() {
	if !l.hasMore() {
		return
	}
	if l.peek().raw == '\r' {
		l.advance()
		if l.hasMore() && l.peek().raw == '\n' {
			l.advance()
		}
	} else {
		l.advance()
	}
	l.line++
	l.column = 1
}

func (l *lexer) physicalLineStart(index int) int {
	for index > 0 {
		previous := l.source[index-1]
		if previous == '\n' || previous == '\r' {
			break
		}
		index--
	}
	return index
}

func (l *lexer) isPhysicalLineStart(index int) bool {
	return index == 0 || l.source[index-1] == '\n' || l.source[index-1] == '\r'
}

func (l *lexer) pointAt(index int) Point {
	low, high := 0, len(l.lineStarts)
	for low+1 < high {
		middle := low + (high-low)/2
		if l.lineStarts[middle] <= index {
			low = middle
		} else {
			high = middle
		}
	}
	return Point{Row: low + 1, Col: index - l.lineStarts[low] + 1}
}

func (l *lexer) pointBefore(index int, fallback char) Point {
	if index <= 0 {
		return Point{Row: fallback.line, Col: fallback.col}
	}
	return l.pointAt(index - 1)
}

func sourceLineStarts(source []byte) []int {
	starts := []int{0}
	for index := 0; index < len(source); index++ {
		if source[index] == '\r' {
			if index+1 < len(source) && source[index+1] == '\n' {
				index++
			}
			starts = append(starts, index+1)
		} else if source[index] == '\n' {
			starts = append(starts, index+1)
		}
	}
	return starts
}

func (l *lexer) addLexError(location Location, message string) {
	l.errors = append(l.errors, ParseError{Location: location, Message: message})
}

func (c char) getLocation() Location {
	return Location{
		Start: Point{Row: c.line, Col: c.col},
		End:   Point{Row: c.line, Col: c.col},
	}
}

func rawIndentation(value []byte) bool {
	for _, char := range value {
		if char != ' ' && char != '\t' {
			return false
		}
	}
	return true
}

func trimBoundaryNewline(source []byte, lineStart, contentStart int) int {
	end := lineStart
	if end <= contentStart {
		return contentStart
	}
	if source[end-1] == '\n' {
		end--
		if end > contentStart && source[end-1] == '\r' {
			end--
		}
	} else if source[end-1] == '\r' {
		end--
	}
	if end < contentStart {
		return contentStart
	}
	return end
}

func (l *lexer) takeRune(start char) (token, bool) {
	var sb strings.Builder

	for l.hasMore() {
		currChar := l.peek()
		if currChar == nil {
			break
		}

		if currChar.raw == '\\' {
			if escaped, consumed, ok := l.takeEscape('\''); ok {
				sb.WriteRune(escaped)
				l.advanceN(consumed)
				continue
			}
			l.advance() // Consume the backslash.
			if !l.hasMore() {
				sb.WriteByte('\\')
				break
			}
			escChar := l.advance()
			if escChar.raw == '"' {
				sb.WriteByte('"')
			} else {
				sb.WriteByte('\\')
				sb.WriteByte(escChar.raw)
			}
			continue
		}

		if currChar.raw == '\'' {
			l.advance() // Consume the closing quote.
			return token{kind: rune_, line: start.line, column: start.col, text: sb.String(), sourceLength: l.column - start.col}, true
		}

		if currChar.raw == '\n' {
			return token{kind: rune_, line: start.line, column: start.col, text: sb.String(), sourceLength: l.column - start.col, err: "Unterminated rune literal"}, true
		}

		sb.WriteByte(currChar.raw)
		l.advance()
	}

	return token{kind: rune_, line: start.line, column: start.col, text: sb.String(), sourceLength: l.column - start.col, err: "Unterminated rune literal"}, true
}

func (l *lexer) takeEscapedTemplateString(start char) (token, bool) {
	// String literals inside interpolation are written with escaped quotes so
	// they do not terminate the outer string, e.g. "{wrap(\"arg\")}".
	// In template mode, treat \" as the delimiter for the nested string.
	lastConsumed := start
	advance := func() *char {
		consumed := l.advance()
		if consumed != nil {
			lastConsumed = *consumed
		}
		return consumed
	}
	advanceN := func(count int) {
		for i := 0; i < count; i++ {
			advance()
		}
	}
	advance() // consume the opening quote after the backslash

	sb := strings.Builder{}
	for l.hasMore() {
		currChar := l.peek()
		if currChar == nil {
			break
		}

		if currChar.raw == '\\' {
			if l.cursor+1 < len(l.source) && l.source[l.cursor+1] == '"' {
				advanceN(2)
				return token{kind: string_, line: start.line, column: start.col, text: sb.String(), sourceEndLine: lastConsumed.line, sourceEndColumn: lastConsumed.col}, true
			}
			if escaped, consumed, ok := l.takeEscape('"'); ok {
				sb.WriteRune(escaped)
				advanceN(consumed)
				continue
			}
			advance() // consume backslash
			if !l.hasMore() {
				sb.WriteByte('\\')
				break
			}
			escChar := advance()
			sb.WriteByte('\\')
			sb.WriteByte(escChar.raw)
			continue
		}

		if currChar.raw == '\n' {
			sb.WriteByte(currChar.raw)
			advance()
			l.line++
			l.column = 1
			continue
		}

		sb.WriteByte(currChar.raw)
		advance()
	}

	return token{kind: string_, line: start.line, column: start.col, text: sb.String(), sourceEndLine: lastConsumed.line, sourceEndColumn: lastConsumed.col}, true
}

func (l *lexer) takePath(start *char) (token, bool) {
	// if the last token was a use, then this is a path
	if len(l.tokens) < 1 || l.tokens[len(l.tokens)-1].kind != use {
		return token{}, false
	}
	for l.hasMore() {
		peek := l.peek()
		if peek.isAlphaNumeric() || peek.raw == '/' || peek.raw == '.' || peek.raw == '-' || peek.raw == ':' || peek.raw == '~' {
			l.advance()
		} else {
			break
		}
	}

	text := string(l.source[start.index:l.cursor])

	if text == "" {
		l.cursor = start.index - 1
		return token{}, false
	}

	return token{
		kind:   path,
		line:   start.line,
		column: start.col,
		text:   text,
	}, true
}

func (l *lexer) takeIdentifier() token {
	// record the start column
	column := l.column - 1
	for l.hasMore() && l.peek().isAlphaNumeric() {
		l.advance()
	}
	text := string(l.source[l.start:l.cursor])

	makeKeyword := func(kind kind) token {
		return token{kind: kind, line: l.line, column: column}
	}
	makeIdentifier := func(kind kind) token {
		return token{kind: kind, text: text, line: l.line, column: column}
	}

	switch text {
	case "and":
		return makeKeyword(and)
	case "not":
		return makeKeyword(not)
	case "or":
		return makeKeyword(or)
	case "true":
		k := makeKeyword(true_)
		k.text = text
		return k
	case "false":
		k := makeKeyword(false_)
		k.text = text
		return k
	case "struct":
		return makeKeyword(struct_)
	case "enum":
		return makeKeyword(enum)
	case "impl":
		return makeKeyword(impl)
	case "trait":
		return makeKeyword(trait)
	case "fn":
		return makeKeyword(fn)
	case "let":
		return makeKeyword(let)
	case "mut":
		return makeKeyword(mut)
	case "deref":
		return makeKeyword(deref)
	case "break":
		return makeKeyword(break_)
	case "match":
		return makeKeyword(match)
	case "select":
		return makeKeyword(select_)
	case "while":
		return makeKeyword(while_)
	case "for":
		return makeKeyword(for_)
	case "use":
		return makeKeyword(use)
	case "as":
		return makeKeyword(as)
	case "in":
		return makeKeyword(in)
	case "if":
		return makeKeyword(if_)
	case "else":
		return makeKeyword(else_)
	case "type":
		return makeKeyword(type_)
	case "private":
		return makeKeyword(private)
	case "defer":
		return makeKeyword(defer_)
	default:
		return makeIdentifier(identifier)
	}
}

func (l *lexer) at(i int) *char {
	if i < 0 || i >= len(l.source) {
		return nil
	}
	return &char{
		raw:   l.source[i],
		index: i,
	}
}

func (l *lexer) takeNumber() token {
	// record the start column
	column := l.column - 1
	for l.hasMore() && (l.peek().isDigit() || l.check("_") || (l.check(".") && !l.check(".."))) {
		if l.check(".") && !l.at(l.cursor+1).isDigit() {
			break
		}
		l.advance()
	}
	text := string(l.source[l.start:l.cursor])
	return token{kind: number, text: text, line: l.line, column: column}
}

func (l *lexer) Scan() []token {
	for l.hasMore() {
		if token, ok := l.take(); ok {
			l.tokens = append(l.tokens, token)
		}
	}

	l.tokens = append(l.tokens, token{kind: eof})
	return l.tokens
}
