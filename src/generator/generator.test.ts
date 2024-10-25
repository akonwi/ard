import { describe, expect, test } from "bun:test";
import { Generator } from "./generator";
import { Lexer } from "../lexer/lexer";
import { Parser } from "../parser/parser";
import controlFlowKon from "../samples/control-flow.kon.txt";
import controlFlowJs from "../samples/control-flow.js.txt";
import variablesKon from "../samples/variables.kon.txt";
import variablesJs from "../samples/variables.js.txt";
import mathKon from "../samples/math.kon.txt";
import mathJs from "../samples/math.js.txt";
import loopsKon from "../samples/loops.kon.txt";
import loopsJs from "../samples/loops.js.txt";
import functionsKon from "../samples/functions.kon.txt";
import functionsJs from "../samples/functions.js.txt";

const generator = new Generator();

function ast(input: string) {
	const tree = new Parser(new Lexer(input).tokenize()).parse();
	expect(tree).toBeDefined();
	return tree;
}

describe("javascript generator", () => {
	test.each([
		["variable declarations", variablesKon, variablesJs],
		["control flow", controlFlowKon, controlFlowJs],
		["arithmatic", mathKon, mathJs],
		["loops", loopsKon, loopsJs],
		["functions", functionsKon, functionsJs],
	])(`%s`, (_, kon, js) => {
		generator.input = ast(kon);
		expect(generator.generate()).toEqual(js.trim());
	});
});
