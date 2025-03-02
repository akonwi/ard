package ast

type kind string

const (
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

	// Types
	int_  = "int"
	float = "float"
	bool_ = "bool"
	str   = "str"

	// Literals
	identifier = "identifier"
	number     = "number"
	string_    = "string"

	eof = "eof"
)

type token struct {
	kind   kind
	line   int
	column int
	text   string
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

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
func (l *lexer) take() (token, bool) {
	switch currentChar := l.advance(); currentChar.raw {
	case '\n':
		l.line++
		l.column = 1
		return token{}, false
	case '(':
		return currentChar.asToken(left_paren), true
	case ')':
		return currentChar.asToken(right_paren), true
	case '{':
		return currentChar.asToken(left_brace), true
	case '}':
		return currentChar.asToken(right_brace), true
	case '[':
		return token{kind: left_bracket}, true
	case ']':
		return token{kind: right_bracket}, true
	case ';':
		return token{kind: semicolon}, true
	case ',':
		return currentChar.asToken(comma), true
	case '.':
		if l.matchNext('.') != nil {
			return currentChar.asToken(dot_dot), true
		}
		return currentChar.asToken(dot), true
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
		return token{kind: percent}, true
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
		if l.matchNext('>') != nil {
			return currentChar.asToken(fat_arrow), true
		}
		if l.matchNext('=') != nil {
			return currentChar.asToken(equal_equal), true
		}
		return currentChar.asToken(equal), true
	case '"':
		start := currentChar.index
		for l.hasMore() && l.advance().raw != '"' {
		}
		return token{
			kind:   string_,
			text:   string(l.source[start:l.cursor]),
			line:   currentChar.line,
			column: currentChar.col,
		}, true
	default:
		if currentChar.isAlpha() {
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
	}
	return makeIdentifier(identifier)
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
