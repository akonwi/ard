import {
	SyntaxType,
	type PrimitiveTypeNode,
	type PrimitiveValueNode,
	type StructDefinitionNode,
} from "../ast.ts";

export const LIST_MEMBERS = new Map<
	string,
	{ callable: boolean; mutates: boolean }
>([
	["at", { mutates: false, callable: true }],
	["concat", { mutates: false, callable: true }],
	["copyWithin", { mutates: true, callable: true }],
	["length", { mutates: false, callable: false }],
	["size", { mutates: false, callable: false }], // todo: alias for length
	["map", { mutates: false, callable: true }],
	["pop", { mutates: true, callable: true }],
	["push", { mutates: true, callable: true }],
	["reverse", { mutates: true, callable: true }],
	["shift", { mutates: true, callable: true }],
	["slice", { mutates: false, callable: true }],
	["sort", { mutates: true, callable: true }],
	["splice", { mutates: true, callable: true }],
]);

export const MAP_MEMBERS = new Map<
	string,
	{ callable: boolean; mutates: boolean }
>([
	["concat", { mutates: false, callable: true }],
	["entries", { mutates: false, callable: true }],
	["get", { mutates: false, callable: true }],
	["keys", { mutates: false, callable: true }],
	["length", { mutates: false, callable: false }],
	["set", { mutates: true, callable: true }],
	["size", { mutates: false, callable: false }], // todo: alias for length
]);

export const STR_MEMBERS = new Map<
	string,
	{ callable: boolean; mutates: boolean }
>([
	["at", { mutates: false, callable: true }],
	["concat", { mutates: false, callable: true }],
	["length", { mutates: false, callable: false }],
	["size", { mutates: false, callable: false }], // todo: alias for length
]);

export interface StaticType {
	// identifier
	name: string;
	// used for display
	pretty: string;
	is_iterable: boolean;
}

class NumType implements StaticType {
	get name() {
		return this.pretty;
	}
	get pretty() {
		return "Num";
	}

	get is_iterable() {
		return true;
	}
}

class StrType implements StaticType {
	get name() {
		return this.pretty;
	}
	get pretty() {
		return "Str";
	}

	get is_iterable() {
		return true;
	}
}

class BoolType implements StaticType {
	constructor() {}

	get name() {
		return this.pretty;
	}
	get pretty() {
		return "Bool";
	}

	get is_iterable() {
		return false;
	}
}

export class ListType implements StaticType {
	constructor(readonly inner: StaticType) {}

	get name() {
		return this.pretty;
	}
	get pretty() {
		return `[${this.inner.pretty}]`;
	}

	get is_iterable() {
		return true;
	}
}

export const EmptyList: StaticType = {
	name: "EmptyList",
	pretty: "[]",
	is_iterable: true,
};

export class MapType implements StaticType {
	constructor(readonly value: StaticType) {}

	get name() {
		return this.pretty;
	}
	get pretty() {
		return `[Str:${this.value.pretty}]`;
	}

	readonly is_iterable = true;
}

export class StructType implements StaticType {
	constructor(
		readonly name: string,
		readonly fields: Map<string, StaticType> = new Map(),
	) {}

	static from(node: StructDefinitionNode): StructType {
		const fields = new Map<string, StaticType>();
		for (const field of node.fieldNodes) {
			fields.set(
				field.nameNode.text,
				getStaticTypeForPrimitiveType(field.typeNode),
			);
		}
		return new StructType(node.nameNode.text, fields);
	}

	get static_type(): string {
		return `${this.name}`;
	}

	get pretty() {
		return this.name;
	}

	readonly is_iterable = false;
}

// could theoretically work for struct fields
export type ParameterType = { name: string; type: StaticType };
export class FunctionType implements StaticType {
	readonly name: string;
	readonly parameters: Array<ParameterType>;
	readonly return_type: StaticType;

	constructor(input: {
		name: string;
		params: ParameterType[];
		return_type: StaticType;
	}) {
		this.name = input.name;
		this.parameters = input.params;
		this.return_type = input.return_type;
	}

	get pretty() {
		return `() ${this.return_type.pretty}`;
	}

	get is_iterable() {
		return false;
	}
}

export const Num = new NumType();
export const Bool = new BoolType();
export const Str = new StrType();
export const Unknown: StaticType = {
	name: "unknown",
	pretty: "unknown",
	is_iterable: false,
};

export function getStaticTypeForPrimitiveType(
	node: PrimitiveTypeNode,
): StaticType {
	switch (node.text) {
		case "Num":
			return Num;
		case "Str":
			return Str;
		case "Bool":
			return Bool;
		default:
			return Unknown;
	}
}

export function getStaticTypeForPrimitiveValue(
	node: Pick<PrimitiveValueNode, "primitiveNode">,
): StaticType {
	switch (node.primitiveNode.type) {
		case SyntaxType.Number:
			return Num;
		case SyntaxType.String:
			return Str;
		case SyntaxType.Boolean:
			return Bool;
		default:
			return Unknown;
	}
}

export function areCompatible(a: StaticType, b: StaticType): boolean {
	if (a === EmptyList && b instanceof ListType) return true;
	if (a instanceof ListType && b === EmptyList) return true;
	return a.pretty === b.pretty;
}

export function isIterable(type: StaticType): boolean {
	return type.is_iterable === true;
}
