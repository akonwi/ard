import { expect } from "jsr:@std/expect";
import { makeParser } from "../parser/parser.ts";
import { Checker, type CheckError } from "./check.ts";

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
			point: { row: 1, column: 13 },
			type: "variable_type",
			message: "Expected a 'Str' but got 'Num'",
		},
		{
			point: { row: 2, column: 13 },
			type: "variable_type",
			message: "Expected a 'Str' but got 'Bool'",
		},
	] satisfies CheckError[]);
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
			point: { row: 1, column: 13 },
			type: "variable_type",
			message: "Expected a 'Num' but got 'Str'",
		},
		{
			point: { row: 2, column: 13 },
			type: "variable_type",
			message: "Expected a 'Num' but got 'Bool'",
		},
	] satisfies CheckError[]);
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
			point: { row: 1, column: 14 },
			type: "variable_type",
			message: "Expected a 'Bool' but got 'Str'",
		},
		{
			point: { row: 2, column: 14 },
			type: "variable_type",
			message: "Expected a 'Bool' but got 'Num'",
		},
	] satisfies CheckError[]);
});

Deno.test.ignore(
	"reserved keywords cannot be used as variable and method names",
	() => {},
);
