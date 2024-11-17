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

Deno.test("Function return type must match implementation", () => {
	const tree = parser.parse(`
fn add(x: Num, y: Num) Str {
  x + y
}
`);
	const checker = new Checker(tree);
	const diagnostics = checker.check();
	expect(diagnostics).toEqual([
		{
			level: "error",
			location: { row: 2, column: 2 },
			message: "Expected 'Str' and received 'Num'.",
		},
	] as Diagnostic[]);
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
fn add(x: Num, y: Num) Num { x + y }
add(20)
add(true, 5)
`);
	const checker = new Checker(tree);
	const diagnostics = checker.check();
	expect(diagnostics).toEqual([
		{
			location: { row: 2, column: 3 },
			level: "error",
			message: "Expected 2 arguments and got 1",
		},
		{
			location: { row: 3, column: 4 },
			level: "error",
			message:
				"Argument of type 'Bool' is not assignable to parameter of type 'Num'",
		},
	] as Diagnostic[]);
});

Deno.test("calling member functions should match the signature", () => {
	const tree = parser.parse(`
let string = "foobar"
string.at()
string.at("0")
string.at(3)
string.foo()
`);
	const checker = new Checker(tree);
	const diagnostics = checker.check();
	expect(diagnostics).toEqual([
		{
			location: { row: 2, column: 9 },
			level: "error",
			message: "Expected 1 arguments and got 0",
		},
		{
			location: { row: 3, column: 10 },
			level: "error",
			message:
				"Argument of type 'Str' is not assignable to parameter of type 'Num'",
		},
		{
			location: { row: 5, column: 7 },
			level: "error",
			message: "Property 'foo' does not exist on type 'Str'",
		},
	] as Diagnostic[]);
});

Deno.test("Anonymous functions as arguments", () => {
	const tree = parser.parse(`
let names = ["joe", "nick", "kevin"]
names.map((name) Str { name.length })
 `);
	const checker = new Checker(tree);
	const diagnostics = checker.check();
	expect(diagnostics).toEqual([
		{
			level: "error",
			location: { row: 2, column: 23 },
			message: "Expected 'Str' and received 'Num'.",
		},
	] as Diagnostic[]);
});
