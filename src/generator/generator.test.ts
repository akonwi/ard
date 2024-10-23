import { describe, expect, test } from "bun:test";
import { Generator } from "./generator";
import { Lexer } from "../lexer/lexer";
import { Parser } from "../parser/parser";

describe("javascript generator", () => {
	test("generating a JS expression", () => {
		const generator = new Generator();
		generator.input = new Parser(
			new Lexer("1 + 2 + (2 * 3) - 4;").tokenize(),
		).parse()!;
		expect(generator.generate()).toEqual("1 + 2 + (2 * 3) - 4;");
	});

	test("generating a print expression", () => {
		const generator = new Generator();
		generator.input = new Parser(new Lexer("print 2 + 2;").tokenize()).parse()!;
		expect(generator.generate()).toEqual("console.log(2 + 2);");
	});
});
