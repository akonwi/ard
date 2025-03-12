package ast

import "fmt"

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
	question_mark      = "question_mark"
	pipe               = "pipe"
	double_quote       = "double_quote"
	single_quote       = "single_quote"
	backtick           = "backtick"

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
	slash_slash        = "slash_slash"
	slash_star         = "slash_star"
	star_slash         = "star_slash"
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
	fn      = "fn"
	let     = "let"
	mut     = "mut"
	break_  = "break"
	match   = "match"
	while_  = "while"
	for_    = "for"
	use     = "use"
	as      = "as"
	in      = "in"
	if_     = "if"
	else_   = "else"
	type_   = "type"

	// Types
	int_  = "int"
	float = "float"
	bool_ = "bool"
	str   = "str"

	// Literals
	path           = "path"
	identifier     = "identifier"
	number         = "number"
	string_        = "string"
	complex_string = "complex_string"

	eof = "eof"
)

type token struct {
	kind   kind
	line   int
	column int
	text   string

	// for strings with interpolated expressions
	chunks []token
}

func toString(kind kind) string {
	switch kind {
	case space:
		return " "
	case new_line:
		return "\n"
	case left_paren:
		return "("
	case right_paren:
		return ")"
	case left_brace:
		return "{"
	case right_brace:
		return "}"
	case left_bracket:
		return "["
	case right_bracket:
		return "]"
	case colon:
		return ":"
	case semicolon:
		return ";"
	case comma:
		return ","
	case dot:
		return "."
	case dot_dot:
		return ".."
	case question_mark:
		return "?"
	case pipe:
		return "|"
	case double_quote:
		return "\""
	case single_quote:
		return "'"
	case backtick:
		return "`"
	case colon_colon:
		return "::"
	case bang:
		return "!"
	case greater_than:
		return ">"
	case less_than:
		return "<"
	case greater_than_equal:
		return ">="
	case less_than_equal:
		return "<="
	case equal:
		return "="
	case equal_equal:
		return "=="
	case bang_equal:
		return "!="
	case plus:
		return "+"
	case minus:
		return "-"
	case star:
		return "*"
	case slash:
		return "/"
	case slash_slash:
		return "//"
	case slash_star:
		return "/*"
	case star_slash:
		return "*/"
	case percent:
		return "%"
	case thin_arrow:
		return "->"
	case fat_arrow:
		return "=>"
	case increment:
		return "=+"
	case decrement:
		return "=-"
	case expr_open:
		return "{{"
	case expr_close:
		return "}}"
	case and:
		return "and"
	case not:
		return "not"
	case or:
		return "or"
	case true_:
		return "true"
	case false_:
		return "false"
	case struct_:
		return "struct"
	case enum:
		return "enum"
	case impl:
		return "impl"
	case fn:
		return "fn"
	case let:
		return "let"
	case mut:
		return "mut"
	case break_:
		return "break"
	case match:
		return "match"
	case while_:
		return "while"
	case for_:
		return "for"
	case use:
		return "use"
	case as:
		return "as"
	case in:
		return "in"
	case if_:
		return "if"
	case else_:
		return "else"
	case type_:
		return "type"
	case int_:
		return "int"
	case float:
		return "float"
	case bool_:
		return "bool"
	case str:
		return "str"
	case complex_string:
		return "complex_string"
	case eof:
		return "EOF"
	default:
		panic(fmt.Errorf("missing String() for token kind: %v", kind))
	}
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

type lexer struct {
	source []byte
	tokens []token
	// position in the source
	cursor int
	// position of the current token to take
	start int
	// position in the source
	line, column int
}

func newLexer(source []byte) lexer {
	return lexer{
		source: source,
		tokens: []token{},
		cursor: 0,
		line:   1,
		column: 1,
	}
}

func (l lexer) isAtEnd() bool {
	return l.cursor >= len(l.source)
}
func (l lexer) hasMore() bool {
	return !l.isAtEnd()
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
func (l *lexer) advance() *char {
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
		if l.matchNext('{') != nil {
			return currentChar.asToken(expr_open), true
		}
		return currentChar.asToken(left_brace), true
	case '}':
		if l.matchNext('}') != nil {
			return currentChar.asToken(expr_close), true
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
			return currentChar.asToken(dot_dot), true
		}
		return currentChar.asToken(dot), true
	case '?':
		return currentChar.asToken(question_mark), true
	case '|':
		return currentChar.asToken(pipe), true
	case '!':
		if l.hasMore() && l.matchNext('=') != nil {
			return currentChar.asToken(bang_equal), true
		}
		return currentChar.asToken(bang), true
	case '+':
		return currentChar.asToken(plus), true
	case '*':
		if l.matchNext('/') != nil {
			return currentChar.asToken(star_slash), true
		}
		return currentChar.asToken(star), true
	case '/':
		if l.matchNext('/') != nil {
			return currentChar.asToken(slash_slash), true
		}
		if l.matchNext('*') != nil {
			return currentChar.asToken(slash_star), true

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
		return l.takeString(currentChar), true
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

func (l *lexer) takeString(start *char) token {
	if l.isAtEnd() {
		// todo: this probably needs to be checked later too
		panic("unterminated string")
	}

	// if in a string, keep reading until the end of the string
	// when '`' is reached, finished. take latest chunk
	// when '{{' is reached, take subsequent tokens until '}}'
	// when '}}' is reached, continue taking string from there
	interpol := token{
		kind:   complex_string,
		line:   start.line,
		column: start.col,
		chunks: []token{},
	}

	// peek to skip the quote
	currentStringStart := l.peek()

	for l.hasMore() && l.matchNext('"') == nil {
		if l.peekMatch("{{") {
			// capture text so far as first chunk
			// todo: skip adding empty strings
			interpol.chunks = append(interpol.chunks,
				token{
					kind:   string_,
					line:   start.line,
					column: start.col,
					text:   string(l.source[currentStringStart.index:l.cursor]),
				},
			)

			// skip the {{
			l.advance()
			l.advance()

			for l.hasMore() && !l.peekMatch("}}") {
				if token, ok := l.take(); ok {
					interpol.chunks = append(interpol.chunks, token)
				}
			}

			l.advance()
			l.advance()
			// reset the start of the next string
			currentStringStart = l.advance()
		} else {
			l.advance()
		}
	}

	rawStart := currentStringStart.index
	quote := currentStringStart
	// if there was no interpolation, the start of this chunk is the quote
	if currentStringStart.index == start.index+1 {
		rawStart = start.index + 1
		quote = start
	}
	// todo: skip adding empty strings
	interpol.chunks = append(interpol.chunks, token{
		kind:   string_,
		line:   quote.line,
		column: quote.col,
		// skip closing backtick
		text: string(l.source[rawStart : l.cursor-1]),
	})

	return interpol
}

func (l *lexer) takePath(start *char) (token, bool) {
	// if the last token was a use, then this is a path
	if len(l.tokens) < 1 || l.tokens[len(l.tokens)-1].kind != use {
		return token{}, false
	}
	for l.hasMore() {
		peek := l.peek()
		if peek.isAlphaNumeric() || peek.raw == '/' || peek.raw == '.' || peek.raw == '-' {
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
		return makeKeyword(true_)
	case "false":
		return makeKeyword(false_)
	case "struct":
		return makeKeyword(struct_)
	case "enum":
		return makeKeyword(enum)
	case "impl":
		return makeKeyword(impl)
	case "fn":
		return makeKeyword(fn)
	case "let":
		return makeKeyword(let)
	case "mut":
		return makeKeyword(mut)
	case "break":
		return makeKeyword(break_)
	case "match":
		return makeKeyword(match)
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
	default:
		return makeIdentifier(identifier)
	}
}
func (l *lexer) takeNumber() token {
	// record the start column
	column := l.column - 1
	for l.hasMore() && l.peek().isDigit() {
		l.advance()
	}
	text := string(l.source[l.start:l.cursor])
	return token{kind: number, text: text, line: l.line, column: column}
}

func (l *lexer) scan() {
	for l.hasMore() {
		if token, ok := l.take(); ok {
			l.tokens = append(l.tokens, token)
		}
	}

	l.tokens = append(l.tokens, token{kind: eof})
}
