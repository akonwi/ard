package ast

type kind int

const (
	left_paren kind = iota
	right_paren
	left_brace
	right_brace
	left_bracket
	right_bracket
	colon
	semicolon
	comma
	dot
	question_mark
	pipe

	colon_colon
	bang
	greater_than
	less_than
	greater_than_equal
	less_than_equal
	equal
	equal_equal
	bang_equal
	plus
	minus
	star
	slash
	slash_slash
	slash_star
	star_slash
	percent
	thin_arrow
	fat_arrow
	increment
	decrement

	// Keywords
	and
	not
	or
	true_
	false_
	struct_
	enum
	impl
	fn
	let
	mut
	break_
	match
	while_
	for_
	use
	as
	in

	// Types
	int_
	float
	bool_
	str

	// Literals
	identifier
	number
	string_

	eof
)

type token struct {
	kind   kind
	line   uint
	column uint
	text   string
}

type char struct {
	raw   byte
	index uint
	line  uint
	col   uint
}

type lexer struct {
	source []byte
	tokens []token
	// position in the source
	cursor uint
	// position of the current token to take
	start uint
	// position in the source
	line, column uint
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
	return l.cursor >= uint(len(l.source))
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
		raw:  l.source[l.cursor],
		line: l.line,
		col:  l.column,
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

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
func (l *lexer) take() (token, bool) {
	switch c := l.advance(); c.raw {
	case '\n':
		l.line++
		l.column = 1
		return token{}, false
	case '(':
		return token{kind: left_paren}, true
	case ')':
		return token{kind: right_paren}, true
	case '{':
		return token{kind: left_brace}, true
	case '}':
		return token{kind: right_brace}, true
	case '[':
		return token{kind: left_bracket}, true
	case ']':
		return token{kind: right_bracket}, true
	case ';':
		return token{kind: semicolon}, true
	case ',':
		return token{kind: comma}, true
	case '.':
		return token{kind: dot}, true
	case '?':
		return token{kind: question_mark}, true
	case '|':
		return token{kind: pipe}, true
	case '!':
		if l.hasMore() && l.matchNext('=') != nil {
			return token{kind: bang_equal}, true
		}
		return token{kind: bang}, true
	case '+':
		return token{kind: plus}, true
	case '*':
		if l.matchNext('/') != nil {
			return token{
				kind:   star_slash,
				line:   c.line,
				column: c.col,
			}, true
		}
		return token{kind: star, line: c.line, column: c.col}, true
	case '/':
		if l.matchNext('/') != nil {
			return token{
				kind:   slash_slash,
				line:   uint(c.line),
				column: uint(c.col),
			}, true
		}
		if l.matchNext('*') != nil {
			return token{
				kind:   slash_star,
				line:   uint(c.line),
				column: uint(c.col),
			}, true
		}
		return token{kind: slash, line: uint(c.line), column: uint(c.col)}, true
	case '%':
		return token{kind: percent}, true
	case ':':
		col := uint(l.column - 1)
		if l.matchNext(':') != nil {
			return token{kind: colon_colon, line: uint(l.line), column: uint(c.col)}, true
		}
		return token{kind: colon, line: uint(l.line), column: col}, true
	case '>':
		if l.hasMore() && l.matchNext('=') != nil {
			return token{kind: greater_than_equal}, true
		}
		return token{kind: greater_than}, true
	case '<':
		if l.hasMore() && l.matchNext('=') != nil {
			return token{kind: less_than_equal}, true
		}
		return token{kind: less_than}, true
	case '-':
		if l.hasMore() && l.matchNext('>') != nil {
			return token{kind: thin_arrow}, true
		}
		return token{kind: minus}, true
	case '=':
		column := uint(l.column - 1)
		if l.matchNext('>') != nil {
			return token{kind: fat_arrow, line: uint(l.line), column: column, text: "=>"}, true
		}
		if l.matchNext('=') != nil {
			return token{kind: equal_equal, line: uint(l.line), column: column, text: "=="}, true
		}
		return token{kind: equal, line: uint(l.line), column: column, text: "="}, true
	case '"':
		start := l.cursor - 1
		col := uint(l.column - 1)
		for l.hasMore() && l.advance().raw != '"' {
		}
		return token{
			kind:   string_,
			text:   string(l.source[start:l.cursor]),
			line:   uint(l.line),
			column: col,
		}, true
	default:
		if c.isAlpha() {
			l.start = l.cursor - 1
			return l.takeIdentifier(), true
		}
		if c.isDigit() {
			l.start = l.cursor - 1
			return l.takeNumber(), true
		}
		return token{}, false
	}
}

func (l *lexer) takeIdentifier() token {
	// record the start column
	column := l.column - 1
	for l.hasMore() && l.peek().isAlphaNumeric() {
		l.advance()
	}
	text := string(l.source[l.start:l.cursor])

	makeToken := func(kind kind) token {
		return token{kind: kind, text: text, line: uint(l.line), column: uint(column)}
	}

	switch text {
	case "and":
		return makeToken(and)
	case "not":
		return makeToken(not)
	case "or":
		return makeToken(or)
	case "true":
		return makeToken(true_)
	case "false":
		return makeToken(false_)
	case "struct":
		return makeToken(struct_)
	case "enum":
		return makeToken(enum)
	case "impl":
		return makeToken(impl)
	case "fn":
		return makeToken(fn)
	case "let":
		return makeToken(let)
	case "mut":
		return makeToken(mut)
	case "break":
		return makeToken(break_)
	case "match":
		return makeToken(match)
	case "while":
		return makeToken(while_)
	case "for":
		return makeToken(for_)
	case "use":
		return makeToken(use)
	case "as":
		return makeToken(as)
	case "in":
		return makeToken(in)
	}
	return makeToken(identifier)
}
func (l *lexer) takeNumber() token {
	// record the start column
	column := l.column - 1
	for l.hasMore() && l.peek().isDigit() {
		l.advance()
	}
	text := string(l.source[l.start:l.cursor])
	return token{kind: number, text: text, line: uint(l.line), column: uint(column)}
}

func (l *lexer) scan() {
	for l.hasMore() {
		if token, ok := l.take(); ok {
			l.tokens = append(l.tokens, token)
		}
	}

	l.tokens = append(l.tokens, token{kind: eof})
}
