import { Lexer, TokenType } from "./lexer";

describe("Lexer", () => {
	it("should tokenize a simple let declaration", () => {
		const input = "let x = 5";
		const lexer = new Lexer(input);

		expect(lexer.nextToken()).toEqual({
			type: TokenType.LET,
			value: "let",
			line: 1,
			column: 1,
		});
		expect(lexer.nextToken()).toEqual({
			type: TokenType.IDENTIFIER,
			value: "x",
			line: 1,
			column: 5,
		});
		expect(lexer.nextToken()).toEqual({
			type: TokenType.EQUALS,
			value: "=",
			line: 1,
			column: 7,
		});
		expect(lexer.nextToken()).toEqual({
			type: TokenType.NUMBER,
			value: "5",
			line: 1,
			column: 9,
		});
		expect(lexer.nextToken()).toEqual({
			type: TokenType.EOF,
			value: "",
			line: 1,
			column: 10,
		});
	});

	it("should tokenize a simple var declaration", () => {
		const input = "var x = true";
		const lexer = new Lexer(input);

		expect(lexer.nextToken()).toEqual({
			type: TokenType.VAR,
			value: "var",
			line: 1,
			column: 1,
		});
		expect(lexer.nextToken()).toEqual({
			type: TokenType.IDENTIFIER,
			value: "x",
			line: 1,
			column: 5,
		});
		expect(lexer.nextToken()).toEqual({
			type: TokenType.EQUALS,
			value: "=",
			line: 1,
			column: 7,
		});
		expect(lexer.nextToken()).toEqual({
			type: TokenType.BOOLEAN,
			value: "true",
			line: 1,
			column: 9,
		});
		expect(lexer.nextToken()).toEqual({
			type: TokenType.EOF,
			value: "",
			line: 1,
			column: 13,
		});
	});

	it("should tokenize arithmetic expressions", () => {
		const input = "x + 5 * (10 - 3) / 2";
		const lexer = new Lexer(input);

		const expectedTokens = [
			{ type: TokenType.IDENTIFIER, value: "x" },
			{ type: TokenType.PLUS, value: "+" },
			{ type: TokenType.NUMBER, value: "5" },
			{ type: TokenType.ASTERISK, value: "*" },
			{ type: TokenType.LPAREN, value: "(" },
			{ type: TokenType.NUMBER, value: "10" },
			{ type: TokenType.MINUS, value: "-" },
			{ type: TokenType.NUMBER, value: "3" },
			{ type: TokenType.RPAREN, value: ")" },
			{ type: TokenType.SLASH, value: "/" },
			{ type: TokenType.NUMBER, value: "2" },
			{ type: TokenType.EOF, value: "" },
		];

		expectedTokens.forEach((expected) => {
			const token = lexer.nextToken();
			expect(token.type).toBe(expected.type);
			expect(token.value).toBe(expected.value);
		});
	});

	it("should tokenize strings", () => {
		const input = 'let message = "Hello, world!"';
		const lexer = new Lexer(input);

		expect(lexer.nextToken()).toEqual({
			type: TokenType.LET,
			value: "let",
			line: 1,
			column: 1,
		});
		expect(lexer.nextToken()).toEqual({
			type: TokenType.IDENTIFIER,
			value: "message",
			line: 1,
			column: 5,
		});
		expect(lexer.nextToken()).toEqual({
			type: TokenType.EQUALS,
			value: "=",
			line: 1,
			column: 13,
		});
		expect(lexer.nextToken()).toEqual({
			type: TokenType.STRING,
			value: "Hello, world!",
			line: 1,
			column: 15,
		});
		expect(lexer.nextToken()).toEqual({
			type: TokenType.EOF,
			value: "",
			line: 1,
			column: 30,
		});
	});

	it("should handle multiline input", () => {
		const input = `
      let x = 5
      var y = 10
      x + y
    `;
		const lexer = new Lexer(input);

		const expectedTokens = [
			{ type: TokenType.LET, value: "let", line: 2, column: 7 },
			{ type: TokenType.IDENTIFIER, value: "x", line: 2, column: 11 },
			{ type: TokenType.EQUALS, value: "=", line: 2, column: 13 },
			{ type: TokenType.NUMBER, value: "5", line: 2, column: 15 },
			{ type: TokenType.VAR, value: "var", line: 3, column: 7 },
			{ type: TokenType.IDENTIFIER, value: "y", line: 3, column: 11 },
			{ type: TokenType.EQUALS, value: "=", line: 3, column: 13 },
			{ type: TokenType.NUMBER, value: "10", line: 3, column: 15 },
			{ type: TokenType.IDENTIFIER, value: "x", line: 4, column: 7 },
			{ type: TokenType.PLUS, value: "+", line: 4, column: 9 },
			{ type: TokenType.IDENTIFIER, value: "y", line: 4, column: 11 },
			{ type: TokenType.EOF, value: "", line: 5, column: 5 },
		];

		expectedTokens.forEach((expected) => {
			const token = lexer.nextToken();
			expect(token).toEqual(expected);
		});
	});
});
