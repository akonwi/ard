import { expect } from "jsr:@std/expect";
import { makeParser } from "../parser/parser.ts";
import { Checker, type Diagnostic } from "./check.ts";

const parser = makeParser();

Deno.test("Enums are recognized types", () => {
	const tree = parser.parse(`
enum Color {
  Red,
  Green,
  Yellow
}`);

	const checker = new Checker(tree);
	const errors = checker.check();
	expect(errors).toEqual([]);

	const color_enum = checker.scope().getEnum("Color");
	expect(color_enum!.name).toBe("Color");
});

Deno.test.ignore("Enum as a variable type", () => {
	const tree = parser.parse(`
enum Color {
  Red,
  Green,
  Yellow
}

let favorite = Color::Red
mut traffic_light: Color = Color.Green
let invalid: Color = "foo"`);

	const errors = new Checker(tree).check();
	expect(errors).toEqual([
		{
			level: "error",
			location: { row: 9, column: 21 },
			message: "Expected 'Color' and received 'Str'.",
		},
	] as Diagnostic[]);
});
