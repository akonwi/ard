// @ts-check
const assert = require("node:assert");
const { describe, test } = require("node:test");
const fs = require("node:fs");
const { generateJavascript } = require("./generate-javascript");
const { makeParser } = require("../parser/tree-sitter-parser");

const parser = makeParser();

const fixtures = [
	{ fileName: "variables", label: "variable definitions" },
	{ fileName: "functions", label: "function definitions and calls" },
	{ fileName: "loops", label: "while loops" },
	{ fileName: "control-flow", label: "if/else blocks" },
];

describe("generating javascript", () => {
	fixtures.forEach(({ fileName, label }) => {
		test(label, () => {
			const kon = fs.readFileSync(
				require.resolve(`../samples/${fileName}.kon`),
				"utf8",
			);
			const js = fs
				.readFileSync(require.resolve(`../samples/${fileName}.js`), "utf8")
				.trimEnd();
			const tree = parser.parse(kon);
			const result = generateJavascript(tree);
			assert.equal(result, js, "generated code does not match");
		});
	});
});
