import { expect } from "jsr:@std/expect";
import { makeParser } from "../parser/parser.ts";
import { Checker, type Diagnostic } from "./check.ts";

const parser = makeParser();

Deno.test("Matching on an enum must be exhaustive", () => {
	const tree = parser.parse(`
enum Color {
  Red,
  Green,
  Yellow
}

let light = Color::Red
match light {
  Red => print("Stop"),
  Green => print("Go")
}
`);

	const errors = new Checker(tree).check();
	expect(errors).toEqual([
		{
			level: "error",
			location: { row: 8, column: 0 },
			message: "Match must be exhaustive. Missing 'Yellow'",
		},
	] as Diagnostic[]);
});
