export enum TokenType {
	// Keywords
	LET = "LET",
	MUT = "MUT",
	FUNC = "FUNC",
	RETURN = "RETURN",
	IF = "IF",
	ELSE = "ELSE",
	FOR = "FOR",
	IN = "IN",
	WHILE = "WHILE",
	STRUCT = "STRUCT",
	ENUM = "ENUM",
	CASE = "CASE",
	THROWS = "THROWS",
	THROW = "THROW",
	TRY = "TRY",
	ASYNC = "ASYNC",
	AWAIT = "AWAIT",
	MATCH = "MATCH",
	INTERNAL = "INTERNAL",
	IMPORT = "IMPORT",
	FROM = "FROM",

	// Literals
	IDENTIFIER = "IDENTIFIER",
	INTEGER = "INTEGER",
	DOUBLE = "DOUBLE",
	STRING = "STRING",
	BOOLEAN = "BOOLEAN",

	// Operators
	PLUS = "PLUS",
	MINUS = "MINUS",
	MULTIPLY = "MULTIPLY",
	SLASH = "DIVIDE",
	ASSIGN = "ASSIGN",
	EQUAL = "EQUAL",
	NOT_EQUAL = "NOT_EQUAL",
	GREATER_THAN = "GREATER_THAN",
	LESS_THAN = "LESS_THAN",
	GREATER_EQUAL = "GREATER_EQUAL",
	LESS_EQUAL = "LESS_EQUAL",
	AND = "AND",
	OR = "OR",
	NOT = "NOT",

	// Delimiters
	LEFT_PAREN = "LEFT_PAREN",
	RIGHT_PAREN = "RIGHT_PAREN",
	LEFT_BRACE = "LEFT_BRACE",
	RIGHT_BRACE = "RIGHT_BRACE",
	LEFT_BRACKET = "LEFT_BRACKET",
	RIGHT_BRACKET = "RIGHT_BRACKET",
	COMMA = "COMMA",
	DOT = "DOT",
	COLON = "COLON",
	ARROW = "ARROW",

	// Special tokens
	EOF = "EOF",
	ERROR = "ERROR",
}

export class Token {
	constructor(
		public readonly type: TokenType,
		public readonly lexeme: string,
		public readonly line: number,
		public readonly column: number,
	) {}

	static init(params: {
		type: TokenType;
		value: string;
		line: number;
		column: number;
	}): Token {
		return new Token(params.type, params.value, params.line, params.column);
	}
}

// TODO: support resetting the source
export class Lexer {
	private source: string;
	private tokens: Token[] = [];
	private start: number = 0;
	private current: number = 0;
	private line: number = 1;
	private column: number = 1;

	private keywords: { [key: string]: TokenType } = {
		let: TokenType.LET,
		mut: TokenType.MUT,
		func: TokenType.FUNC,
		return: TokenType.RETURN,
		if: TokenType.IF,
		else: TokenType.ELSE,
		for: TokenType.FOR,
		in: TokenType.IN,
		while: TokenType.WHILE,
		struct: TokenType.STRUCT,
		enum: TokenType.ENUM,
		case: TokenType.CASE,
		throws: TokenType.THROWS,
		throw: TokenType.THROW,
		try: TokenType.TRY,
		async: TokenType.ASYNC,
		await: TokenType.AWAIT,
		match: TokenType.MATCH,
		internal: TokenType.INTERNAL,
		import: TokenType.IMPORT,
		from: TokenType.FROM,
		true: TokenType.BOOLEAN,
		false: TokenType.BOOLEAN,
	};

	constructor(source: string) {
		this.source = source;
	}

	tokenize(): Token[] {
		while (!this.isAtEnd()) {
			this.start = this.current;
			this.scanToken();
		}

		this.tokens.push(
			Token.init({
				type: TokenType.EOF,
				value: "",
				line: this.line,
				column: this.column,
			}),
		);
		return this.tokens;
	}

	private scanToken() {
		const c = this.advance();
		switch (c) {
			case "(":
				this.addToken(TokenType.LEFT_PAREN);
				break;
			case ")":
				this.addToken(TokenType.RIGHT_PAREN);
				break;
			case "{":
				this.addToken(TokenType.LEFT_BRACE);
				break;
			case "}":
				this.addToken(TokenType.RIGHT_BRACE);
				break;
			case "[":
				this.addToken(TokenType.LEFT_BRACKET);
				break;
			case "]":
				this.addToken(TokenType.RIGHT_BRACKET);
				break;
			case ",":
				this.addToken(TokenType.COMMA);
				break;
			case ".":
				this.addToken(TokenType.DOT);
				break;
			case ":":
				this.addToken(TokenType.COLON);
				break;
			case "-":
				if (this.match(">")) {
					this.addToken(TokenType.ARROW);
				} else {
					this.addToken(TokenType.MINUS);
				}
				break;
			case "+":
				this.addToken(TokenType.PLUS);
				break;
			case "*":
				this.addToken(TokenType.MULTIPLY);
				break;
			case "/":
				if (this.match("/")) {
					// A comment goes until the end of the line.
					while (this.peek() != "\n" && !this.isAtEnd()) this.advance();
				} else {
					this.addToken(TokenType.SLASH);
				}
				break;
			case "=":
				this.addToken(this.match("=") ? TokenType.EQUAL : TokenType.ASSIGN);
				break;
			case "!":
				this.addToken(this.match("=") ? TokenType.NOT_EQUAL : TokenType.NOT);
				break;
			case "<":
				this.addToken(
					this.match("=") ? TokenType.LESS_EQUAL : TokenType.LESS_THAN,
				);
				break;
			case ">":
				this.addToken(
					this.match("=") ? TokenType.GREATER_EQUAL : TokenType.GREATER_THAN,
				);
				break;
			case "&":
				if (this.match("&")) this.addToken(TokenType.AND);
				else this.addToken(TokenType.ERROR, "Unexpected character");
				break;
			case "|":
				if (this.match("|")) this.addToken(TokenType.OR);
				else this.addToken(TokenType.ERROR, "Unexpected character");
				break;
			case " ":
			case "\r":
			case "\t":
				// Ignore whitespace.
				break;
			case "\n":
				this.line++;
				this.column = 1;
				break;
			case '"':
				this.string();
				break;
			default:
				if (this.isDigit(c)) {
					this.number();
				} else if (this.isAlpha(c)) {
					this.identifier();
				} else {
					this.addToken(TokenType.ERROR, "Unexpected character");
				}
				break;
		}
	}

	private string() {
		while (this.peek() != '"' && !this.isAtEnd()) {
			if (this.peek() == "\n") this.line++;
			this.advance();
		}

		if (this.isAtEnd()) {
			this.addToken(TokenType.ERROR, "Unterminated string");
			return;
		}

		// The closing ".
		this.advance();

		// Trim the surrounding quotes.
		const value = this.source.substring(this.start + 1, this.current - 1);
		this.addToken(TokenType.STRING, value);
	}

	private number() {
		while (this.isDigit(this.peek())) this.advance();

		// Look for a fractional part.
		if (this.peek() == "." && this.isDigit(this.peekNext())) {
			// Consume the "."
			this.advance();

			while (this.isDigit(this.peek())) this.advance();
			this.addToken(TokenType.DOUBLE);
		} else {
			this.addToken(TokenType.INTEGER);
		}
	}

	private identifier() {
		while (this.isAlphaNumeric(this.peek())) this.advance();

		const text = this.source.substring(this.start, this.current);
		let type = this.keywords[text];
		if (type == null) type = TokenType.IDENTIFIER;
		this.addToken(type);
	}

	private match(expected: string): boolean {
		if (this.isAtEnd()) return false;
		if (this.source.charAt(this.current) != expected) return false;

		this.current++;
		this.column++;
		return true;
	}

	private peek(): string {
		if (this.isAtEnd()) return "\0";
		return this.source.charAt(this.current);
	}

	private peekNext(): string {
		if (this.current + 1 >= this.source.length) return "\0";
		return this.source.charAt(this.current + 1);
	}

	private isAlpha(c: string): boolean {
		return (c >= "a" && c <= "z") || (c >= "A" && c <= "Z") || c == "_";
	}

	private isAlphaNumeric(c: string): boolean {
		return this.isAlpha(c) || this.isDigit(c);
	}

	private isDigit(c: string): boolean {
		return c >= "0" && c <= "9";
	}

	private isAtEnd(): boolean {
		return this.current >= this.source.length;
	}

	private advance(): string {
		this.current++;
		this.column++;
		return this.source.charAt(this.current - 1);
	}

	private addToken(type: TokenType, literal: string = "") {
		const text = this.source.substring(this.start, this.current);
		this.tokens.push(
			Token.init({
				type,
				value: text,
				line: this.line,
				column: this.column - (this.current - this.start),
			}),
		);
	}
}
