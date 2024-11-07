import { expect } from "jsr:@std/expect";
import { makeParser } from "../parser/parser.ts";
import { Checker, type Diagnostic } from "./check.ts";
import { Bool, ListType, Num, Str } from "./kon-types.ts";

const parser = makeParser();

const expectErrors = (
	actual: Diagnostic[],
	expected: Partial<Diagnostic>[],
) => {
	expect(actual).toEqual(expected);
};

Deno.test("Incorrect primitive initializers when expecting a Str", () => {
	const tree = parser.parse(`
let x: Str = 5
let y: Str = false
let valid1: Str = "foo"
let valid2 = "bar"`);
	const errors = new Checker(tree).check();
	expectErrors(errors, [
		{
			location: { row: 1, column: 13 },
			level: "error",
			message: "Expected 'Str' and received 'Num'.",
		},
		{
			location: { row: 2, column: 13 },
			level: "error",
			message: "Expected 'Str' and received 'Bool'.",
		},
	]);
});

Deno.test("Incorrect primitive initializers when expecting a Num", () => {
	const tree = parser.parse(`
let x: Num = "foo"
let y: Num = false
let five: Num = 5
let 6 = 6`);
	const errors = new Checker(tree).check();
	expectErrors(errors, [
		{
			location: { row: 1, column: 13 },
			level: "error",
			message: "Expected 'Num' and received 'Str'.",
		},
		{
			location: { row: 2, column: 13 },
			level: "error",
			message: "Expected 'Num' and received 'Bool'.",
		},
	]);
});

Deno.test("Incorrect primitive initializers when expecting a Bool", () => {
	const tree = parser.parse(`
let x: Bool = "foo"
let y: Bool = 0
let valid: Bool = true
let also_valid = false`);
	const errors = new Checker(tree).check();
	expectErrors(errors, [
		{
			location: { row: 1, column: 14 },
			level: "error",
			message: "Expected 'Bool' and received 'Str'.",
		},
		{
			location: { row: 2, column: 14 },
			level: "error",
			message: "Expected 'Bool' and received 'Num'.",
		},
	]);
});

Deno.test("reserved keywords cannot be used as variable names", () => {
	const tree = parser.parse(`let let = 5`);
	const errors = new Checker(tree).check();
	expectErrors(errors, [
		{
			level: "error",
			message:
				"'let' is a reserved keyword and cannot be used as a variable name",
			location: { row: 0, column: 4 },
		},
	]);
});

Deno.test("referencing undeclared variables", () => {
	const tree = parser.parse(`foo.bar`);
	const errors = new Checker(tree).check();
	expect(errors).toEqual([
		{
			level: "error",
			message: "Missing declaration for 'foo'.",
			location: { row: 0, column: 0 },
		} satisfies Diagnostic,
	]);
});

// todo: provide a std List implementation
// until then, use checker to add syntactic sugar for ideal API
Deno.test("mutable Array methods aren't allowed on immutable arrays", () => {
	const tree = parser.parse(`
let list: [Num] = [5]
list.push(6)
`);
	const errors = new Checker(tree).check();
	expect(errors).toEqual([
		{
			level: "error",
			message: "Cannot mutate an immutable list. Use 'mut' to make it mutable.",
			location: { row: 2, column: 5 },
		} satisfies Diagnostic,
	]);
});

Deno.test("cannot reference undeclared types", () => {
	const message = "Missing definition for type 'Todo'.";
	const list = parser.parse(`let x: [Todo] = []`);
	const errors = new Checker(list).check();
	expect(errors).toEqual([
		{
			level: "error",
			location: { row: 0, column: 8 },
			message,
		} satisfies Diagnostic,
	]);

	const struct = parser.parse(`Todo {}`);
	expect(new Checker(struct).check()).toEqual([
		{
			level: "error",
			location: { row: 0, column: 0 },
			message,
		},
	]);
});

const STRUCT_DEF = `
struct Todo {
  title: Str,
  completed: Bool
}`;

Deno.test("struct instantiation", () => {
	const tree = parser.parse(`${STRUCT_DEF}
Todo {}
Todo { title: "foo" }
Todo { title: "foo", completed: true }
Todo { title: 404, completed: "yes" }
`);
	const errors = new Checker(tree).check();
	expectErrors(errors, [
		{
			level: "error",
			location: { row: 5, column: 0 },
			message: "Missing fields for struct 'Todo': title, completed.",
		},
		{
			level: "error",
			location: { row: 6, column: 0 },
			message: "Missing fields for struct 'Todo': completed.",
		},
		{
			level: "error",
			location: { row: 8, column: 14 },
			message: "Expected 'Str' and received 'Num'.",
		},
		{
			level: "error",
			location: { row: 8, column: 30 },
			message: "Expected 'Bool' and received 'Str'.",
		},
	]);
});

Deno.test("assigning a struct to a variable", () => {
	const tree = parser.parse(`${STRUCT_DEF}
let invalid: Str = Todo { title: "foo", completed: true }
let valid: Todo = Todo { title: "foo", completed: true }
`);
	const errors = new Checker(tree).check();
	expect(errors).toEqual([
		{
			level: "error",
			location: { row: 5, column: 19 },
			message: "Expected 'Str' and received 'Todo'.",
		},
	] satisfies Diagnostic[]);
});

Deno.test("assigning a list of structs", () => {
	const tree = parser.parse(`${STRUCT_DEF}
let empty_valid: [Todo] = []
let valid: [Todo] = [500, "try this"]
let valid: [Todo] = [Todo { title: "foo", completed: true }]
`);

	const errors = new Checker(tree).check();
	expect(errors).toEqual([
		{
			level: "error",
			location: { row: 6, column: 20 },
			message: "Expected '[Todo]' and received '[Num]'.",
		},
	]);
});

Deno.test("variable reassignment", () => {
	const tree = parser.parse(`
let immutable: Num = 5
mut mutable: Str = "foo"

immutable = immutable * 2
`);
	expect(new Checker(tree).check()).toEqual([
		{
			level: "error",
			location: { row: 4, column: 10 },
			message: "Variable 'immutable' is not mutable.",
		},
	]);
});

Deno.test("basic type inference", () => {
	const tree = parser.parse(`
let string = "foo"
let number = 5
let boolean = true
let list = [1, 2, 3]
`);
	const checker = new Checker(tree);
	expect(checker.check().length).toBe(0);
	expect(checker.scopes[0]!.variables.get("string")?.static_type).toBe(Str);
	expect(checker.scopes[0]!.variables.get("number")?.static_type).toBe(Num);
	expect(checker.scopes[0]!.variables.get("boolean")?.static_type).toBe(Bool);
	expect(checker.scopes[0]!.variables.get("list")?.static_type).toEqual(
		new ListType(Num),
	);
});
