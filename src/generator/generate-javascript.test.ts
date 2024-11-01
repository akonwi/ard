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
