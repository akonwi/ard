import { expect } from "jsr:@std/expect";
import {
	Bool,
	EmptyList,
	ListType,
	MapType,
	Num,
	Str,
	StructType,
	areCompatible,
} from "./kon-types.ts";

Deno.test("Compatibility checks against Str", () => {
	expect(areCompatible(Str, Str)).toBe(true);
	expect(areCompatible(Str, Num)).toBe(false);
	expect(areCompatible(Str, Bool)).toBe(false);
	expect(areCompatible(Str, new ListType(Str))).toBe(false);
	expect(areCompatible(Str, new MapType(Num))).toBe(false);
});

Deno.test("Compatibility checks against Num", () => {
	expect(areCompatible(Num, Str)).toBe(false);
	expect(areCompatible(Num, Num)).toBe(true);
	expect(areCompatible(Num, Bool)).toBe(false);
	expect(areCompatible(Num, new ListType(Str))).toBe(false);
	expect(areCompatible(Num, new MapType(Num))).toBe(false);
});

Deno.test("Compatibility checks against Bool", () => {
	expect(areCompatible(Bool, Str)).toBe(false);
	expect(areCompatible(Bool, Num)).toBe(false);
	expect(areCompatible(Bool, Bool)).toBe(true);
	expect(areCompatible(Bool, new ListType(Str))).toBe(false);
	expect(areCompatible(Bool, new MapType(Num))).toBe(false);
});

Deno.test("Compatibility checks against [Bool]", () => {
	const bool_list = new ListType(Bool);
	expect(areCompatible(bool_list, bool_list)).toBe(true);
	expect(areCompatible(bool_list, EmptyList)).toBe(true);
	expect(areCompatible(bool_list, new ListType(Str))).toBe(false);
	expect(areCompatible(bool_list, new ListType(Num))).toBe(false);
	expect(areCompatible(bool_list, new ListType(new MapType(Bool)))).toBe(false);
	expect(areCompatible(bool_list, new ListType(new ListType(Bool)))).toBe(
		false,
	);
});

Deno.test("Compatibility checks against [Str:Num]", () => {
	const map_to_num = new MapType(Num);
	expect(areCompatible(map_to_num, new MapType(Num))).toBe(true);
	expect(areCompatible(map_to_num, new MapType(Str))).toBe(false);
	expect(areCompatible(map_to_num, new MapType(Bool))).toBe(false);
	expect(areCompatible(map_to_num, new MapType(new MapType(Bool)))).toBe(false);
	expect(areCompatible(map_to_num, new MapType(new ListType(Bool)))).toBe(
		false,
	);
});

Deno.test("Compatibility checks against a struct", () => {
	const person_struct = new StructType("Person", new Map());
	expect(areCompatible(person_struct, person_struct)).toBe(true);
	expect(
		areCompatible(person_struct, new StructType("Animal", new Map())),
	).toBe(false);
	expect(areCompatible(person_struct, Bool)).toBe(false);
	expect(areCompatible(person_struct, Num)).toBe(false);
	expect(areCompatible(person_struct, Str)).toBe(false);
	expect(areCompatible(person_struct, new MapType(Str))).toBe(false);
	expect(areCompatible(person_struct, new ListType(Bool))).toBe(false);
});
