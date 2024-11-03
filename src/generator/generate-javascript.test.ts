import { expect } from "jsr:@std/expect";
import path from "node:path";
import { generateJavascript } from "./generate-javascript.ts";
import { makeParser } from "../parser/parser.ts";

const parser = makeParser();

const fixtures: Array<{ fileName: string; label: string }> = [
	{ fileName: "variables", label: "variable definitions" },
	{ fileName: "functions", label: "function definitions and calls" },
	{ fileName: "loops", label: "while loops" },
	{ fileName: "control-flow", label: "if/else blocks" },
	{ fileName: "expressions", label: "various expressions" },
];

fixtures.forEach(({ fileName, label }) => {
	Deno.test(
		{
			name: `generates JavaScript for ${label}`,
		},
		() => {
			const kon = Deno.readTextFileSync(`./src/samples/${fileName}.kon`);
			const js = Deno.readTextFileSync(
				path.resolve(`./src/samples/${fileName}.js`),
			).trimEnd();
			const tree = parser.parse(kon);
			const result = generateJavascript(tree);
			expect(result).toEqual(js);
		},
	);
});

Deno.test("struct definitions are stripped", () => {
	const tree = parser.parse(`struct Point { x: Num, y: Num }`);
	expect(generateJavascript(tree)).toEqual("");
});

Deno.test("list declarations", () => {
	const emptyList = parser.parse(`let x: [Num] = []`);
	expect(generateJavascript(emptyList)).toEqual("const x = [];");

	const numbers = parser.parse(`let x: [Num] = [1,2,3]`);
	expect(generateJavascript(numbers)).toEqual("const x = [1, 2, 3];");

	const strings = parser.parse(`let x: [Num] = ["a", "b", "c"]`);
	expect(generateJavascript(strings)).toEqual('const x = ["a", "b", "c"];');

	const variables = parser.parse(`let x: [Num] = [a, b]`);
	expect(generateJavascript(variables)).toEqual("const x = [a, b];");
});
