import { expect } from "jsr:@std/expect";
import { makeParser } from "../parser/parser.ts";
import { Checker, type Diagnostic } from "./check.ts";

const list_code = `
let list: [Num] = [1, 2, 3]
list.lenth
list.map
`;
Deno.test("standard List interface", () => {
	const diagnostics = new Checker(makeParser().parse(list_code)).check();
	expect(diagnostics).toEqual([
		{
			level: "error",
			location: { row: 2, column: 5 },
			message: "Property 'lenth' does not exist on List.",
		},
	] as Diagnostic[]);
});

const str_code = `
let name = "test"
name.length
name.map
`;
Deno.test("standard Str interface", () => {
	const diagnostics = new Checker(makeParser().parse(str_code)).check();
	expect(diagnostics).toEqual([
		{
			level: "error",
			location: { row: 2, column: 5 },
			message: "Property 'map' does not exist on Str.",
		},
	] as Diagnostic[]);
});
