import { describe, test, expect } from "bun:test";
import { compile } from "./compile";

describe.skip("compiling stone", () => {
	test("a simple script", () => {
		const input = `
		let name = "John"
		mut age = 23
		mut isStudent = true
		`;
		expect(compile(input)).toEqual(
			`
		const name = "John";
		let age = 23;
		let isStudent = true;
		`.trim(),
		);
	});
});
