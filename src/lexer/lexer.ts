enum TokenType {
	LET,
	VAR,
	IDENTIFIER,
	EQUALS,
	BOOLEAN,
	NUMBER,
	STRING,
	PLUS,
	MINUS,
	ASTERISK,
	SLASH,
	LPAREN,
	RPAREN,
	EOF,
}

interface Token {
	type: TokenType;
	value: string;
	line: number;
	column: number;
}

class Lexer {
	private source: string;
	private position: number = 0;
	private line: number = 1;
	private column: number = 1;

	constructor(source: string) {
		this.source = source;
	}

	nextToken(): Token {
		this.skipWhitespace();

		if (this.position >= this.source.length) {
			return this.createToken(TokenType.EOF, "");
		}

		const char = this.currentChar;

		if (this.isAlpha(char)) {
			return this.readIdentifier();
		}

		if (this.isDigit(char)) {
			return this.readNumber();
		}

		switch (char) {
			case "=":
				return this.createToken(TokenType.EQUALS, this.advance());
			case "+":
				return this.createToken(TokenType.PLUS, this.advance());
			case "-":
				return this.createToken(TokenType.MINUS, this.advance());
			case "*":
				return this.createToken(TokenType.ASTERISK, this.advance());
			case "/":
				return this.createToken(TokenType.SLASH, this.advance());
			case "(":
				return this.createToken(TokenType.LPAREN, this.advance());
			case ")":
				return this.createToken(TokenType.RPAREN, this.advance());
			case '"':
				return this.readString();
			default:
				throw new Error(`Unexpected character: ${char}`);
		}
	}

	private createToken(type: TokenType, value: string): Token {
		return { type, value, line: this.line, column: this.column - value.length };
	}

	private get currentChar(): string {
		return this.source[this.position];
	}

	private advance(): string {
		const char = this.currentChar;
		this.position++;
		this.column++;
		return char;
	}

	private isAlpha(char: string): boolean {
		return /[a-zA-Z_]/.test(char);
	}

	private isDigit(char: string): boolean {
		return /[0-9]/.test(char);
	}

	private isAlphanumeric(char: string): boolean {
		return this.isAlpha(char) || this.isDigit(char);
	}

	private readIdentifier(): Token {
		let value = "";
		while (
			this.position < this.source.length &&
			this.isAlphanumeric(this.currentChar)
		) {
			value += this.advance();
		}

		let type: TokenType;
		switch (value) {
			case "let":
				type = TokenType.LET;
				break;
			case "var":
				type = TokenType.VAR;
				break;
			case "true":
			case "false":
				type = TokenType.BOOLEAN;
				break;
			default:
				type = TokenType.IDENTIFIER;
		}

		return this.createToken(type, value);
	}

	private readNumber(): Token {
		let value = "";
		while (
			this.position < this.source.length &&
			this.isDigit(this.currentChar)
		) {
			value += this.advance();
		}
		return this.createToken(TokenType.NUMBER, value);
	}

	private readString(): Token {
		const token = this.createToken(TokenType.STRING, "");
		this.advance(); // Skip opening quote
		while (this.position < this.source.length && this.currentChar !== '"') {
			token.value += this.advance();
		}
		if (this.currentChar === '"') {
			this.advance(); // Skip closing quote
		} else {
			throw new Error("Unterminated string");
		}
		return token;
	}

	private skipWhitespace(): void {
		while (this.position < this.source.length) {
			const char = this.currentChar;
			if (char === " " || char === "\t" || char === "\r") {
				this.advance();
			} else if (char === "\n") {
				this.advance();
				this.line++;
				this.column = 1;
			} else {
				break;
			}
		}
	}
}

export { Lexer, Token, TokenType };
