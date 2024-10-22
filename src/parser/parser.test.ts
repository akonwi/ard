import { describe, test, expect } from "bun:test";
import { Parser } from "./parser";
import { Lexer, TokenType } from "../lexer/lexer";

describe("Parser", () => {
	test("simple expression AST", () => {
		const input = "-123 * 45.67";
		const parser = new Parser(new Lexer(input).tokenize());
		const ast = parser.parse();
	});

	describe("generating ASTs", () => {
		test.skip("for a simple let declaration", () => {
			const input = "let x = 5";
			const parser = new Parser(new Lexer(input).tokenize());
			const ast = parser.parse();
			expect(ast).toEqual([
				{
					type: "Let",
					name: expect.objectContaining({
						type: TokenType.IDENTIFIER,
						value: "x",
					}),
					initializer: { type: "Literal", value: "5" },
				},
			]);
		});

		test.skip("for a simple mut declaration", () => {
			const input = "mut boolean = true";
			const parser = new Parser(new Lexer(input).tokenize());
			const ast = parser.parse();
			expect(ast).toEqual([
				{
					type: "Mut",
					name: expect.objectContaining({
						type: TokenType.IDENTIFIER,
						value: "boolean",
					}),
					initializer: { type: "Literal", value: false },
				},
			]);
		});

		test.skip("a simple function", () => {
			const input = `
			  func greet(name) {
					return name
				}
			`;
			const parser = new Parser(new Lexer(input).tokenize());
			const ast = parser.parse();
			expect(ast).toEqual([
				{
					type: "Function",
					name: expect.objectContaining({
						type: TokenType.IDENTIFIER,
						value: "greet",
					}),
					params: [
						expect.objectContaining({
							type: TokenType.IDENTIFIER,
						}),
					],
					body: [],
				},
			]);
		});
	});
});
