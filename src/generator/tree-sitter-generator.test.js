// @ts-check
const assert = require("node:assert");
const { describe, test } = require("node:test");
const fs = require("node:fs");
const { generateJavascript } = require("./tree-sitter-generator");
const { makeParser } = require("../parser/tree-sitter-parser");

const parser = makeParser();

const variablesKon = fs.readFileSync(
	require.resolve("../samples/variables.kon"),
	"utf8",
);
const variablesJS = fs
	.readFileSync(require.resolve("../samples/variables.js"), "utf8")
	.trimEnd();
const functionsKon = fs.readFileSync(
	require.resolve("../samples/functions.kon"),
	"utf8",
);
const functionsJS = fs
	.readFileSync(require.resolve("../samples/functions.js"), "utf8")
	.trimEnd();

describe("generating javascript", () => {
	test("variable definitions", () => {
		const tree = parser.parse(variablesKon);
		const result = generateJavascript(tree);
		assert.equal(result, variablesJS, "generated code does not match");
	});

	test("function definitions and calls", () => {
		const tree = parser.parse(functionsKon);
		const result = generateJavascript(tree);
		assert.equal(result, functionsJS, "generated code does not match");
	});
});
