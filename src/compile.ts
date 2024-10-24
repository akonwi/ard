import fs from "fs";
import { Generator } from "./generator/generator";
import { Lexer } from "./lexer/lexer";
import { Parser } from "./parser/parser";

function compile(input: string): string {
	const tokens = new Lexer(input).tokenize();
	const ast = new Parser(tokens).parse();
	const generator = new Generator();
	generator.input = ast;
	return generator.generate();
}

const sample = fs.readFileSync(require.resolve("./sample.stone"), "utf-8");
console.log(compile(sample));
