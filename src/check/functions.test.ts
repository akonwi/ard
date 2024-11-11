import { expect } from "jsr:@std/expect";
import { makeParser } from "../parser/parser.ts";
import { Checker, type Diagnostic } from "./check.ts";
import { Num, type FunctionType } from "./kon-types.ts";

const parser = makeParser();

Deno.test("Function declarations", () => {
	const tree = parser.parse(`
fn add(x: Num, y: Num) Num {
  x + y
}
`);
	const checker = new Checker(tree);
	const diagnostics = checker.check();
	expect(diagnostics).toEqual([]);

	const add_def = checker.scope().getFunction("add");
	expect(add_def).toEqual({
		name: "add",
		parameters: [
			{ name: "x", type: Num },
			{ name: "y", type: Num },
		],
		return_type: Num,
	} satisfies Pick<FunctionType, "name" | "parameters" | "return_type">);
});

Deno.test("Cannot call undeclared functions", () => {
	const tree = parser.parse(`add()`);
	const checker = new Checker(tree);
	const diagnostics = checker.check();
	expect(diagnostics).toContainEqual({
		location: { row: 0, column: 0 },
		level: "error",
		message: "Cannot find name 'add'",
	} as Diagnostic);
});

Deno.test("Function calls should match the signature", () => {
	const tree = parser.parse(`
fn add(x: Num, y: Num) Num {}
add(20)
`);
	const checker = new Checker(tree);
	const diagnostics = checker.check();
	expect(diagnostics).toEqual([
		{
			location: { row: 2, column: 3 },
			level: "error",
			message: "Expected 2 arguments and got 1",
		},
	] as Diagnostic[]);
});
