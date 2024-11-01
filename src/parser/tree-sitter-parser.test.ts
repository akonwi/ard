import { expect } from "jsr:@std/expect";
import { makeParser } from "./tree-sitter-parser.ts";

Deno.test("walking the tree", () => {
	const tree = makeParser().parse("let x: Num = 5");
	expect(tree.rootNode.type).toEqual("program");
});
