import {
	SyntaxType,
	type PrimitiveTypeNode,
	type PrimitiveValueNode,
	type StructDefinitionNode,
} from "../ast.ts";

export interface StaticType {
	// identifier
	name: string;
	// used for display
	pretty: string;
	is_iterable: boolean;
}

export class ListType implements StaticType {
	readonly properties: Map<string, Signature>;

	constructor(readonly inner: StaticType) {
		this.properties = this._build();
	}

	get name() {
		return this.pretty;
	}
	get pretty() {
		return `[${this.inner.pretty}]`;
	}

	get is_iterable() {
		return true;
	}

	can_hold(other: StaticType): boolean {
		return areCompatible(this.inner, other);
	}

	_build() {
		return new Map<string, Signature>([
			[
				"at",
				{
					mutates: false,
					callable: true,
					parameters: [{ name: "index", type: Num }],
					return_type: this.inner,
				},
			],
			[
				"concat",
				{
					mutates: false,
					callable: true,
					parameters: [{ name: "list", type: this }],
					return_type: this,
				},
			],
			[
				"length",
				{ mutates: false, callable: false, parameters: [], return_type: Num },
			],
			// ["map", { mutates: false, callable: true }],
			[
				"pop",
				{
					mutates: true,
					callable: true,
					parameters: [],
					return_type: this.inner,
				},
			],
			[
				"push",
				{
					mutates: true,
					callable: true,
					parameters: [{ name: "item", type: this.inner }],
					return_type: Num,
				},
			],
			["size", { mutates: false, callable: false, return_type: Num }], // todo: alias for length
			[
				"shift",
				{
					mutates: true,
					callable: true,
					parameters: [],
					return_type: this.inner,
				},
			],
			[
				"unshift",
				{
					mutates: true,
					callable: true,
					parameters: [{ name: "item", type: Str }],
					return_type: Num,
				},
			],
		]);
	}
}

export const EmptyList: StaticType = {
	name: "EmptyList",
	pretty: "[]",
	is_iterable: true,
};

export class MapType implements StaticType {
	readonly properties: Map<string, Signature>;
	constructor(readonly value: StaticType) {
		this.properties = this._build();
	}

	get name() {
		return this.pretty;
	}
	get pretty() {
		return `[Str:${this.value.pretty}]`;
	}

	readonly is_iterable = true;

	_build() {
		return new Map<string, Signature>([
			// ["entries", { mutates: false, callable: true }],
			[
				"get",
				{
					mutates: false,
					callable: true,
					parameters: [{ name: "key", type: Str }],
					return_type: this.value,
				},
			],
			[
				"keys",
				{
					mutates: false,
					callable: true,
					parameters: [],
					return_type: new ListType(Str),
				},
			],
			["length", { mutates: false, callable: false, return_type: Num }],
			[
				"set",
				{
					mutates: true,
					callable: true,
					parameters: [
						{ name: "key", type: Str },
						{ name: "value", type: this.value },
					],
					return_type: Void,
				},
			],
			["size", { mutates: false, callable: false, return_type: Num }], // todo: alias for length
		]);
	}
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
export const Void: StaticType = {
	name: "void",
	pretty: "Void",
	is_iterable: false,
};
export const Num: StaticType = {
	name: "number",
	pretty: "Num",
	is_iterable: true,
} as const;
export const Str: StaticType = {
	name: "string",
	pretty: "Str",
	is_iterable: true,
} as const;
export const Bool: StaticType = {
	name: "boolean",
	pretty: "Bool",
	is_iterable: false,
} as const;
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

export type Signature = {
	mutates: boolean;
	callable: boolean;
	parameters?: ParameterType[];
	return_type: StaticType;
};

export const STR_MEMBERS = new Map<string, Signature>([
	[
		"at",
		{
			mutates: false,
			callable: true,
			parameters: [{ name: "index", type: Num }],
			return_type: Str,
		},
	],
	[
		"concat",
		{
			mutates: false,
			callable: true,
			parameters: [{ name: "string", type: Str }],
			return_type: Str,
		},
	],
	[
		"length",
		{ mutates: false, callable: false, parameters: [], return_type: Num },
	],
	[
		"size",
		{ mutates: false, callable: false, parameters: [], return_type: Num },
	], // todo: alias for length
]);

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
		return `(${this.parameters.map((p) => p.name).join(", ")}) ${
			this.return_type.pretty
		}`;
	}

	get is_iterable() {
		return false;
	}
}
