// @ts-check
const assert = require("node:assert");
const { describe, test } = require("node:test");
const { makeParser } = require("./tree-sitter-parser");

describe("tree-sitter parser", () => {
	test("walking the tree", () => {
		const tree = makeParser().parse("let x: Num = 5");
		assert(tree != null, "tree is null");
	});
});
