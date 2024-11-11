import { expect } from "jsr:@std/expect";
import { makeParser } from "../parser/parser.ts";
import { Checker } from "./check.ts";
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