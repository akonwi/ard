// @ts-check
const assert = require("node:assert");
const { describe, test } = require("node:test");
const fs = require("node:fs");
const { generateJavascript } = require("./tree-sitter-generator");
const { makeParser } = require("../parser/tree-sitter-parser");

const variablesKon = fs.readFileSync(
	require.resolve("../samples/variables.kon"),
	"utf8",
);
const variablesJS = fs
	.readFileSync(require.resolve("../samples/variables.js"), "utf8")
	.trimEnd();

describe("generating javascript", () => {
	test("variable definitions", () => {
		const tree = makeParser().parse(variablesKon);
		const result = generateJavascript(tree);
		assert.equal(result, variablesJS, "generated code does not match");
	});
});
