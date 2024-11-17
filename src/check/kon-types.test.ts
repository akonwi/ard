import { expect } from "jsr:@std/expect";
import {
	Bool,
	EmptyList,
	EnumType,
	GenericType,
	ListType,
	MapType,
	Num,
	Str,
	StructType,
	Void,
	areCompatible,
	type Signature,
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

Deno.test("Checking if a List can hold a type", () => {
	const bool_list = new ListType(Bool);
	expect(bool_list.can_hold(Bool)).toBe(true);
	expect(bool_list.can_hold(Num)).toBe(false);
	expect(bool_list.can_hold(Str)).toBe(false);
	expect(bool_list.can_hold(Void)).toBe(false);
	expect(bool_list.can_hold(EmptyList)).toBe(false);
	expect(bool_list.can_hold(new ListType(Bool))).toBe(false);
});

Deno.test("List built-in API", () => {
	const str_list = new ListType(Str);
	expect(str_list.properties.get("at")).toEqual({
		mutates: false,
		callable: true,
		parameters: [{ name: "index", type: Num }],
		return_type: Str,
	} as Signature);

	const concat = str_list.properties.get("concat");
	expect(concat?.mutates).toBe(false);
	expect(concat?.parameters).toEqual([{ name: "list", type: str_list }]);
	expect(concat?.return_type).toBe(str_list);

	expect(str_list.properties.get("length")).toEqual({
		mutates: false,
		callable: false,
		parameters: [],
		return_type: Num,
	} as Signature);
	expect(str_list.properties.get("pop")).toEqual({
		mutates: true,
		callable: true,
		parameters: [],
		return_type: Str,
	} as Signature);
	expect(str_list.properties.get("push")).toEqual({
		mutates: true,
		callable: true,
		parameters: [{ name: "item", type: Str }],
		return_type: Num,
	} as Signature);
	expect(str_list.properties.get("shift")).toEqual({
		mutates: true,
		callable: true,
		parameters: [],
		return_type: Str,
	} as Signature);
	expect(str_list.properties.get("unshift")).toEqual({
		mutates: true,
		callable: true,
		parameters: [{ name: "item", type: Str }],
		return_type: Num,
	} as Signature);

	expect(new ListType(Bool).properties.get("at")?.return_type).toBe(Bool);
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

Deno.test("Compatibility checks for enums", () => {
	const Color = new EnumType("Color", ["Red", "Green", "Blue"]);
	const Level = new EnumType("Level", ["Junior", "Senior", "Lead"]);

	expect(areCompatible(Color, Color)).toBe(true);
	expect(areCompatible(Color, Level)).toBe(false);

	expect(areCompatible(Color, Color.variant("Red")!)).toBe(true);
});

Deno.test("Generics", () => {
	const T = new GenericType("T");
	expect(areCompatible(T, Num)).toBe(true);

	T.fill(Str);
	expect(areCompatible(T, Str)).toBe(true);
	expect(areCompatible(T, Num)).toBe(false);
});

Deno.test("Generics in collections", () => {
	const T = new GenericType("T");
	const TList = new ListType(T);
	const NumList = new ListType(Num);

	expect(TList.can_hold(Num)).toBe(true);
	expect(TList.can_hold(Str)).toBe(true);
	expect(TList.can_hold(Bool)).toBe(true);
	expect(areCompatible(TList, NumList)).toBe(true);
	expect(areCompatible(TList, TList)).toBe(true);
	expect(areCompatible(NumList, NumList)).toBe(true);

	T.fill(Str);
	expect(areCompatible(TList, NumList)).toBe(false);
	expect(TList.can_hold(Str)).toBe(true);
	expect(TList.can_hold(Num)).toBe(false);
	expect(TList.can_hold(Bool)).toBe(false);
});
