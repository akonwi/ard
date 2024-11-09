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
			message: "Cannot find name 'foo'.",
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
	expect(checker.scope().getVariable("string")?.static_type).toBe(Str);
	expect(checker.scope().getVariable("number")?.static_type).toBe(Num);
	expect(checker.scope().getVariable("boolean")?.static_type).toBe(Bool);
	expect(checker.scope().getVariable("list")?.static_type).toEqual(
		new ListType(Num),
	);
});

Deno.test("numeric ranges", () => {
	const valid_asc_range = parser.parse(`for i in 0...3 {}`);
	expect(new Checker(valid_asc_range).check()).toEqual([]);
	const invalid_range_end = parser.parse(`for i in 3...bar {}`);
	expect(new Checker(invalid_range_end).check()).toEqual([
		{
			level: "error",
			location: { row: 0, column: 13 },
			message: "Cannot find name 'bar'.",
		},
		{
			level: "error",
			location: { row: 0, column: 13 },
			message: "A range must be between two numbers.",
		},
	] as Diagnostic[]);

	const invalid_range_start = parser.parse(`for i in false...10 {}`);
	expect(new Checker(invalid_range_start).check()).toEqual([
		{
			level: "error",
			location: { row: 0, column: 9 },
			message: "A range must be between two numbers.",
		},
	] as Diagnostic[]);

	const valid_desc_range = parser.parse(`
let x = 5
for i in (x + 3)...0 {}
`);
	expect(new Checker(valid_desc_range).check()).toEqual([]);
});

Deno.test("iterating over lists", () => {
	const over_literal_list = parser.parse(`for i in [1, 2, 3] {}`);
	expect(new Checker(over_literal_list).check()).toEqual([] as Diagnostic[]);

	const over_list_variable = parser.parse(`
let list = [1, 2, 3]
for i in list {}
`);
	expect(new Checker(over_list_variable).check()).toEqual([] as Diagnostic[]);

	const over_bool = parser.parse(`
let not_iterable = true
for i in not_iterable {}
`);
	expect(new Checker(over_bool).check()).toEqual([
		{
			level: "error",
			location: { row: 2, column: 9 },
			message: "Cannot iterate over a 'Bool'.",
		},
	] as Diagnostic[]);
});

Deno.test("iterating over strings", () => {
	const over_string_literal = parser.parse(`for char in "fizzbuzz" {}`);
	expect(new Checker(over_string_literal).check()).toEqual([] as Diagnostic[]);

	const over_string_variable = parser.parse(`
let string = "fizzbuzz"
for char in string {}
`);
	expect(new Checker(over_string_variable).check()).toEqual([]);
});

Deno.test("using a number as the range", () => {
	const positive = parser.parse(`for i in 100 {}`);
	expect(new Checker(positive).check()).toEqual([]);

	const negative_ascending = parser.parse(`for i in -20 {}`);
	expect(new Checker(negative_ascending).check()).toEqual([]);
});

Deno.test("string interpolation", () => {
	const str_literal = parser.parse('"Hello, ${"world"}!"');
	expect(new Checker(str_literal).check()).toEqual([]);

	const num_literal = parser.parse('"Hello, ${100}!"');
	expect(new Checker(num_literal).check()).toEqual([]);

	const bool_literal = parser.parse('"Hello, ${true}!"');
	expect(new Checker(bool_literal).check()).toEqual([]);

	const undeclared_variable = parser.parse('"Hello, ${name}!"');
	expect(new Checker(undeclared_variable).check()).toEqual([
		{
			level: "error",
			location: { row: 0, column: 10 },
			message: "Cannot find name 'name'.",
		},
	] satisfies Diagnostic[]);
});
