import { expect } from "jsr:@std/expect";
import {
	BoolType,
	EmptyList,
	ListType,
	MapType,
	NumType,
	StrType,
	StructType,
	areCompatible,
} from "./kon-types.ts";

const str = new StrType();
const num = new NumType();
const bool = new BoolType();

Deno.test("Compatibility checks against Str", () => {
	expect(areCompatible(str, str)).toBe(true);
	expect(areCompatible(str, num)).toBe(false);
	expect(areCompatible(str, bool)).toBe(false);
	expect(areCompatible(str, new ListType(str))).toBe(false);
	expect(areCompatible(str, new MapType(num))).toBe(false);
});

Deno.test("Compatibility checks against Num", () => {
	expect(areCompatible(num, str)).toBe(false);
	expect(areCompatible(num, num)).toBe(true);
	expect(areCompatible(num, bool)).toBe(false);
	expect(areCompatible(num, new ListType(str))).toBe(false);
	expect(areCompatible(num, new MapType(num))).toBe(false);
});

Deno.test("Compatibility checks against Bool", () => {
	expect(areCompatible(bool, str)).toBe(false);
	expect(areCompatible(bool, num)).toBe(false);
	expect(areCompatible(bool, bool)).toBe(true);
	expect(areCompatible(bool, new ListType(str))).toBe(false);
	expect(areCompatible(bool, new MapType(num))).toBe(false);
});

Deno.test("Compatibility checks against [Bool]", () => {
	const bool_list = new ListType(bool);
	expect(areCompatible(bool_list, bool_list)).toBe(true);
	expect(areCompatible(bool_list, EmptyList)).toBe(true);
	expect(areCompatible(bool_list, new ListType(str))).toBe(false);
	expect(areCompatible(bool_list, new ListType(num))).toBe(false);
	expect(areCompatible(bool_list, new ListType(new MapType(bool)))).toBe(false);
	expect(areCompatible(bool_list, new ListType(new ListType(bool)))).toBe(
		false,
	);
});

Deno.test("Compatibility checks against [Str:Num]", () => {
	const map_to_num = new MapType(num);
	expect(areCompatible(map_to_num, new MapType(num))).toBe(true);
	expect(areCompatible(map_to_num, new MapType(str))).toBe(false);
	expect(areCompatible(map_to_num, new MapType(bool))).toBe(false);
	expect(areCompatible(map_to_num, new MapType(new MapType(bool)))).toBe(false);
	expect(areCompatible(map_to_num, new MapType(new ListType(bool)))).toBe(
		false,
	);
});

Deno.test("Compatibility checks against a struct", () => {
	const person_struct = new StructType("Person", new Map());
	expect(areCompatible(person_struct, person_struct)).toBe(true);
	expect(
		areCompatible(person_struct, new StructType("Animal", new Map())),
	).toBe(false);
	expect(areCompatible(person_struct, bool)).toBe(false);
	expect(areCompatible(person_struct, num)).toBe(false);
	expect(areCompatible(person_struct, str)).toBe(false);
	expect(areCompatible(person_struct, new MapType(str))).toBe(false);
	expect(areCompatible(person_struct, new ListType(bool))).toBe(false);
});
