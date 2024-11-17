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

Deno.test("Enum variables must be initialized correctly", () => {
	const tree = parser.parse(`
enum Color {
  Red,
  Green,
  Yellow
}

let favorite = Color::Red
mut traffic_light: Color = Color::Green
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

Deno.test("Cannot reassign incorrectly to a bound variable", () => {
	const tree = parser.parse(`
enum Color {
  Red,
  Green,
  Yellow
}
enum Place {
  Home,
  Office,
  Beach
}

let favorite = Color::Red
favorite = Color::Green

mut traffic_light = Color::Green
traffic_light = Place::Beach`);

	const errors = new Checker(tree).check();
	expect(errors).toEqual([
		{
			level: "error",
			location: { row: 13, column: 9 },
			message: "Variable 'favorite' is not mutable.",
		},
		{
			level: "error",
			location: { row: 16, column: 16 },
			message: "Expected 'Color' and received 'Place::Beach'.",
		},
	] as Diagnostic[]);
});

Deno.test("Enums can be types of struct fields", () => {
	const tree = parser.parse(`
 enum Color {
   Red,
   Green,
   Yellow
 }

struct Person {
  favorite_color: Color
}
let joe = Person { favorite_color: Color::Red }
`);

	const checker = new Checker(tree);
	const errors = checker.check();
	expect(errors).toEqual([]);
	expect(
		checker.scope().getStruct("Person")?.fields.get("favorite_color")
			?.return_type.name,
	).toBe("Color");
});
