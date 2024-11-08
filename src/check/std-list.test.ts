import { expect } from "jsr:@std/expect";
import { makeParser } from "../parser/parser.ts";
import { Checker, type Diagnostic } from "./check.ts";

const code = `
let list: [Num] = [1, 2, 3]
list.lenth
list.map
`;
Deno.test("standard List interface", () => {
	const diagnostics = new Checker(makeParser().parse(code)).check();
	expect(diagnostics).toEqual([
		{
			level: "error",
			location: { row: 2, column: 5 },
			message: "Property 'lenth' does not exist on List.",
		},
	] as Diagnostic[]);
});
