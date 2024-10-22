import { Lexer, Token, TokenType } from "./lexer";
import { describe, it, expect, test } from "bun:test";

describe("Lexer", () => {
	it("should tokenize a simple let declaration", () => {
		const input = "let x = 5";
		const lexer = new Lexer(input);
		const tokens = lexer.tokenize();

		expect(tokens).toEqual([
			Token.init({
				type: TokenType.LET,
				lexeme: "let",
				line: 1,
				column: 1,
			}),
			Token.init({
				type: TokenType.IDENTIFIER,
				lexeme: "x",
				line: 1,
				column: 5,
			}),
			Token.init({
				type: TokenType.ASSIGN,
				lexeme: "=",
				line: 1,
				column: 7,
			}),
			Token.init({
				type: TokenType.INTEGER,
				lexeme: "5",
				line: 1,
				column: 9,
			}),
			Token.init({
				type: TokenType.EOF,
				lexeme: "",
				line: 1,
				column: 10,
			}),
		]);
	});

	it("should tokenize a simple mut declaration", () => {
		const input = "mut x = true";
		const lexer = new Lexer(input);
		const tokens = lexer.tokenize();

		expect(tokens).toEqual([
			Token.init({
				type: TokenType.MUT,
				lexeme: "mut",
				line: 1,
				column: 1,
			}),
			Token.init({
				type: TokenType.IDENTIFIER,
				lexeme: "x",
				line: 1,
				column: 5,
			}),
			Token.init({
				type: TokenType.ASSIGN,
				lexeme: "=",
				line: 1,
				column: 7,
			}),
			Token.init({
				type: TokenType.TRUE,
				lexeme: "true",
				line: 1,
				column: 9,
			}),
			Token.init({
				type: TokenType.EOF,
				lexeme: "",
				line: 1,
				column: 13,
			}),
		]);
	});

	it("should tokenize arithmetic expressions", () => {
		const input = "x + 5 * (10 - 3) / 2";
		const lexer = new Lexer(input);
		const tokens = lexer.tokenize();

		expect(tokens).toEqual([
			Token.init({
				type: TokenType.IDENTIFIER,
				lexeme: "x",
				line: 1,
				column: 1,
			}),
			Token.init({
				type: TokenType.PLUS,
				lexeme: "+",
				line: 1,
				column: 3,
			}),
			Token.init({
				type: TokenType.INTEGER,
				lexeme: "5",
				line: 1,
				column: 5,
			}),
			Token.init({
				type: TokenType.STAR,
				lexeme: "*",
				line: 1,
				column: 7,
			}),
			Token.init({
				type: TokenType.LEFT_PAREN,
				lexeme: "(",
				line: 1,
				column: 9,
			}),
			Token.init({
				type: TokenType.INTEGER,
				lexeme: "10",
				line: 1,
				column: 10,
			}),
			Token.init({
				type: TokenType.MINUS,
				lexeme: "-",
				line: 1,
				column: 13,
			}),
			Token.init({
				type: TokenType.INTEGER,
				lexeme: "3",
				line: 1,
				column: 15,
			}),
			Token.init({
				type: TokenType.RIGHT_PAREN,
				lexeme: ")",
				line: 1,
				column: 16,
			}),
			Token.init({
				type: TokenType.SLASH,
				lexeme: "/",
				line: 1,
				column: 18,
			}),
			Token.init({
				type: TokenType.INTEGER,
				lexeme: "2",
				line: 1,
				column: 20,
			}),
			Token.init({
				type: TokenType.EOF,
				lexeme: "",
				line: 1,
				column: 21,
			}),
		]);
	});

	it("should tokenize strings", () => {
		const input = 'let message = "Hello, world!"';
		const lexer = new Lexer(input);
		const tokens = lexer.tokenize();

		expect(tokens).toEqual([
			Token.init({
				type: TokenType.LET,
				lexeme: "let",
				line: 1,
				column: 1,
			}),
			Token.init({
				type: TokenType.IDENTIFIER,
				lexeme: "message",
				line: 1,
				column: 5,
			}),
			Token.init({
				type: TokenType.ASSIGN,
				lexeme: "=",
				line: 1,
				column: 13,
			}),
			Token.init({
				type: TokenType.STRING,
				lexeme: "Hello, world!",
				line: 1,
				column: 15,
			}),
			Token.init({
				type: TokenType.EOF,
				lexeme: "",
				line: 1,
				column: 30,
			}),
		]);
	});

	it("should handle multiline input", () => {
		const input = `
						let x = 5
						mut y = 10
						x + y
				`;
		const lexer = new Lexer(input);
		const tokens = lexer.tokenize();

		expect(tokens).toEqual([
			Token.init({
				type: TokenType.LET,
				lexeme: "let",
				line: 2,
				column: 7,
			}),
			Token.init({
				type: TokenType.IDENTIFIER,
				lexeme: "x",
				line: 2,
				column: 11,
			}),
			Token.init({
				type: TokenType.ASSIGN,
				lexeme: "=",
				line: 2,
				column: 13,
			}),
			Token.init({
				type: TokenType.INTEGER,
				lexeme: "5",
				line: 2,
				column: 15,
			}),
			Token.init({
				type: TokenType.MUT,
				lexeme: "mut",
				line: 3,
				column: 7,
			}),
			Token.init({
				type: TokenType.IDENTIFIER,
				lexeme: "y",
				line: 3,
				column: 11,
			}),
			Token.init({
				type: TokenType.ASSIGN,
				lexeme: "=",
				line: 3,
				column: 13,
			}),
			Token.init({
				type: TokenType.INTEGER,
				lexeme: "10",
				line: 3,
				column: 15,
			}),
			Token.init({
				type: TokenType.IDENTIFIER,
				lexeme: "x",
				line: 4,
				column: 7,
			}),
			Token.init({
				type: TokenType.PLUS,
				lexeme: "+",
				line: 4,
				column: 9,
			}),
			Token.init({
				type: TokenType.IDENTIFIER,
				lexeme: "y",
				line: 4,
				column: 11,
			}),
			Token.init({
				type: TokenType.EOF,
				lexeme: "",
				line: 5,
				column: 5,
			}),
		]);
	});

	describe("functions", () => {
		test("a simple function", () => {});
		const input = `func greet(name: String) -> String {
  return "Hello, " + name + "!"
}`;
		const tokens = new Lexer(input).tokenize();

		expect(tokens).toEqual([
			Token.init({
				type: TokenType.FUNC,
				line: 1,
				column: 1,
				lexeme: "func",
			}),
			Token.init({
				type: TokenType.IDENTIFIER,
				line: 1,
				column: 6,
				lexeme: "greet",
			}),
			Token.init({
				type: TokenType.LEFT_PAREN,
				line: 1,
				column: 11,
				lexeme: "(",
			}),
			Token.init({
				type: TokenType.IDENTIFIER,
				line: 1,
				column: 12,
				lexeme: "name",
			}),
			Token.init({
				type: TokenType.COLON,
				line: 1,
				column: 16,
				lexeme: ":",
			}),
			Token.init({
				type: TokenType.IDENTIFIER,
				line: 1,
				column: 18,
				lexeme: "String",
			}),
			Token.init({
				type: TokenType.RIGHT_PAREN,
				line: 1,
				column: 24,
				lexeme: ")",
			}),
			Token.init({
				type: TokenType.ARROW,
				line: 1,
				column: 26,
				lexeme: "->",
			}),
			Token.init({
				type: TokenType.IDENTIFIER,
				line: 1,
				column: 29,
				lexeme: "String",
			}),
			Token.init({
				type: TokenType.LEFT_BRACE,
				column: 36,
				line: 1,
				lexeme: "{",
			}),
			Token.init({
				type: TokenType.RETURN,
				line: 2,
				column: 3,
				lexeme: "return",
			}),
			Token.init({
				column: 10,
				line: 2,
				type: TokenType.STRING,
				lexeme: "Hello, ",
			}),
			Token.init({
				column: 20,
				line: 2,
				type: TokenType.PLUS,
				lexeme: "+",
			}),
			Token.init({
				column: 22,
				line: 2,
				type: TokenType.IDENTIFIER,
				lexeme: "name",
			}),
			Token.init({
				column: 27,
				line: 2,
				type: TokenType.PLUS,
				lexeme: "+",
			}),
			Token.init({
				column: 29,
				line: 2,
				type: TokenType.STRING,
				lexeme: "!",
			}),
			Token.init({
				column: 1,
				line: 3,
				type: TokenType.RIGHT_BRACE,
				lexeme: "}",
			}),
			Token.init({
				column: 2,
				line: 3,
				type: TokenType.EOF,
				lexeme: "",
			}),
		]);
	});
});
