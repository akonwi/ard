const assert = require("node:assert");
const { describe, test } = require("node:test");
const { parser } = require("./tree-sitter-parser");
// @ts-check

describe("tree-sitter parser", () => {
	test("walking the tree", () => {
		const tree = parser.parse("let x: Num = 5");
		assert(tree != null, "tree is null");
	});
});
