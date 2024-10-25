import { describe, test, expect } from "bun:test";
import { Parser } from "./parser";
import { Lexer, Token, TokenType } from "../lexer/lexer";

describe("Parser", () => {
	test("simple expression AST", () => {
		const input = "-123 * 45.67;";
		const parser = new Parser(new Lexer(input).tokenize());
		const ast = parser.parse();
		expect(ast).toEqual([
			{
				type: "ExprStatement",
				expression: {
					type: "Binary",
					left: {
						type: "Unary",
						operator: Token.init({
							type: TokenType.MINUS,
							lexeme: "-",
							column: 1,
							line: 1,
						}),
						right: {
							type: "Literal",
							token: Token.init({
								type: TokenType.INTEGER,
								lexeme: "123",
								line: 1,
								column: 2,
							}),
							value: 123,
						},
					},
					operator: Token.init({
						type: TokenType.STAR,
						lexeme: "*",
						column: 6,
						line: 1,
					}),
					right: {
						type: "Literal",
						value: 45.67,
						token: Token.init({
							type: TokenType.DOUBLE,
							lexeme: "45.67",
							column: 8,
							line: 1,
						}),
					},
				},
			},
		]);
	});

	test("mut declarations", () => {
		const input = 'mut name = "John";';
		const parser = new Parser(new Lexer(input).tokenize());
		const ast = parser.parse();
		expect(ast).toEqual([
			{
				type: "MutDecl",
				name: Token.init({
					type: TokenType.IDENTIFIER,
					lexeme: "name",
					column: 5,
					line: 1,
				}),
				_staticType: null,
				initializer: {
					type: "Literal",
					token: Token.init({
						type: TokenType.STRING,
						lexeme: "John",
						column: 12,
						line: 1,
					}),
					value: "John",
				},
			},
		]);
	});

	// describe.skip("generating ASTs", () => {
	// 	test.skip("for a simple let declaration", () => {
	// 		const input = "let x = 5";
	// 		const parser = new Parser(new Lexer(input).tokenize());
	// 		const ast = parser.parse();
	// 		expect(ast).toEqual([
	// 			{
	// 				type: "Let",
	// 				name: expect.objectContaining({
	// 					type: TokenType.IDENTIFIER,
	// 					value: "x",
	// 				}),
	// 				initializer: { type: "Literal", value: "5" },
	// 			},
	// 		]);
	// 	});

	// 	test.skip("for a simple mut declaration", () => {
	// 		const input = "mut boolean = true";
	// 		const parser = new Parser(new Lexer(input).tokenize());
	// 		const ast = parser.parse();
	// 		expect(ast).toEqual([
	// 			{
	// 				type: "Mut",
	// 				name: expect.objectContaining({
	// 					type: TokenType.IDENTIFIER,
	// 					value: "boolean",
	// 				}),
	// 				initializer: { type: "Literal", value: false },
	// 			},
	// 		]);
	// 	});

	// 	test.skip("a simple function", () => {
	// 		const input = `
	// 		  func greet(name) {
	// 				return name
	// 			}
	// 		`;
	// 		const parser = new Parser(new Lexer(input).tokenize());
	// 		const ast = parser.parse();
	// 		expect(ast).toEqual([
	// 			{
	// 				type: "Function",
	// 				name: expect.objectContaining({
	// 					type: TokenType.IDENTIFIER,
	// 					value: "greet",
	// 				}),
	// 				params: [
	// 					expect.objectContaining({
	// 						type: TokenType.IDENTIFIER,
	// 					}),
	// 				],
	// 				body: [],
	// 			},
	// 		]);
	// 	});
	// });
});
