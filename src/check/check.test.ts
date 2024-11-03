import { expect } from "jsr:@std/expect";
import { makeParser } from "../parser/parser.ts";
import { Checker, type Diagnostic } from "./check.ts";

const parser = makeParser();

Deno.test("Incorrect primitive initializers when expecting a Str", () => {
	const tree = parser.parse(`
let x: Str = 5
let y: Str = false
let valid1: Str = "foo"
let valid2 = "bar"`);
	const errors = new Checker(tree).check();
	expect(errors).toEqual([
		{
			location: { row: 1, column: 13 },
			level: "error",
			message: "Expected a 'Str' but got 'Num'",
		},
		{
			location: { row: 2, column: 13 },
			level: "error",
			message: "Expected a 'Str' but got 'Bool'",
		},
	] satisfies Diagnostic[]);
});

Deno.test("Incorrect primitive initializers when expecting a Num", () => {
	const tree = parser.parse(`
let x: Num = "foo"
let y: Num = false
let five: Num = 5
let 6 = 6`);
	const errors = new Checker(tree).check();
	expect(errors).toEqual([
		{
			location: { row: 1, column: 13 },
			level: "error",
			message: "Expected a 'Num' but got 'Str'",
		},
		{
			location: { row: 2, column: 13 },
			level: "error",
			message: "Expected a 'Num' but got 'Bool'",
		},
	] satisfies Diagnostic[]);
});

Deno.test("Incorrect primitive initializers when expecting a Bool", () => {
	const tree = parser.parse(`
let x: Bool = "foo"
let y: Bool = 0
let valid: Bool = true
let also_valid = false`);
	const errors = new Checker(tree).check();
	expect(errors).toEqual([
		{
			location: { row: 1, column: 14 },
			level: "error",
			message: "Expected a 'Bool' but got 'Str'",
		},
		{
			location: { row: 2, column: 14 },
			level: "error",
			message: "Expected a 'Bool' but got 'Num'",
		},
	] satisfies Diagnostic[]);
});

const of = () => {};

Deno.test("reserved keywords cannot be used as variable names", () => {
	const tree = parser.parse(`let let = 5`);
	const errors = new Checker(tree).check();
	expect(errors).toEqual([
		{
			level: "error",
			message:
				"'let' is a reserved keyword and cannot be used as a variable name",
			location: { row: 0, column: 4 },
		} satisfies Diagnostic,
	]);
});

// todo: provide a std List implementation
// until then, use checker to add syntactic sugar for ideal API
Deno.test.ignore(
	"mutable array methods aren't allowed on immutable arrays",
	() => {},
);
