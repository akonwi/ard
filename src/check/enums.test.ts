import { expect } from "jsr:@std/expect";
import { makeParser } from "../parser/parser.ts";
import { Checker, type Diagnostic } from "./check.ts";
import { EnumType } from "./kon-types.ts";

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
	expect(color_enum!.variants.size).toEqual(3);
	expect(color_enum!.variant("Red")).toBeTruthy();
	expect(color_enum!.variant("Green")).toBeTruthy();
	expect(color_enum!.variant("Yellow")).toBeTruthy();
});

Deno.test("Enums can be inferred", () => {
	const tree = parser.parse(`
enum Color {
  Red,
  Green,
  Yellow
}

let favorite = Color::Red`);

	const checker = new Checker(tree);
	const errors = checker.check();
	expect(errors).toEqual([]);

	const favorite = checker.scope().getVariable("favorite");
	expect(favorite?.static_type.name).toBe("Color");
});

Deno.test.ignore("Enum variables must be initialized correctly", () => {
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
