import { expect } from "jsr:@std/expect";
import { makeParser } from "../parser/parser.ts";
import { generateJavascript } from "./generate-javascript.ts";

const parser = makeParser();

Deno.test("generating enums", () => {
	const tree = parser.parse(`
enum Color {
  Red,
  Green,
  Yellow
}`);

	expect(generateJavascript(tree)).toEqual(`const Color = Object.freeze({
  Red: Object.freeze({ index: 0 }),
  Green: Object.freeze({ index: 1 }),
  Yellow: Object.freeze({ index: 2 })
});`);
});
