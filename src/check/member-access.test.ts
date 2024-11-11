import { expect } from "jsr:@std/expect";
import { makeParser } from "../parser/parser.ts";
import { Checker, type Diagnostic } from "./check.ts";

const parser = makeParser();

const list_code = `
let list: [Num] = [1, 2, 3]
list.lenth
list.map
`;
Deno.test("standard List interface", () => {
	const diagnostics = new Checker(parser.parse(list_code)).check();
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
	const diagnostics = new Checker(parser.parse(str_code)).check();
	expect(diagnostics).toEqual([
		{
			level: "error",
			location: { row: 3, column: 5 },
			message: "Property 'map' does not exist on Str.",
		},
	] as Diagnostic[]);
});

Deno.test("property access on structs", () => {
	const tree = parser.parse(`
struct Person {
  name: Str,
  age: Num
}

let peeps: [Person] = [
  Person { name: "John Doe", age: 33 },
  Person { name: "Jane Doe", age: 33 },
]

for person in peeps {
  print("\${person.title} is \${person.age} years old")
}
`);

	expect(new Checker(tree).check()).toEqual([
		{
			level: "error",
			location: {
				row: 12,
				column: 18,
			},
			message: "Property 'title' does not exist on type 'Person'",
		},
	] satisfies Diagnostic[]);
});
