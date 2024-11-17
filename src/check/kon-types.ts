import {
	SyntaxType,
	type PrimitiveTypeNode,
	type PrimitiveValueNode,
} from "../ast.ts";

export interface StaticType {
	// identifier
	name: string;
	// used for display
	pretty: string;
	is_iterable: boolean;
}

export class EnumType implements StaticType {
	readonly name: string;
	readonly variants = new Map<string, EnumVariant>();
	readonly is_iterable = false;

	constructor(name: string, variants: string[]) {
		this.name = name;
		for (const variant of variants) {
			this.variants.set(variant, new EnumVariant(variant, this));
		}
	}

	get pretty() {
		return this.name;
	}

	variant(name: string) {
		return this.variants.get(name);
	}

	hasVariant(variant: EnumVariant): boolean {
		return this.variants.has(variant.name);
	}
}

export class EnumVariant implements StaticType {
	readonly name: string;
	readonly parent: EnumType;
	readonly is_iterable = false;

	constructor(name: string, parent: EnumType) {
		this.name = name;
		this.parent = parent;
	}

	get pretty() {
		return `${this.parent.pretty}::${this.name}`;
	}
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

	compatible_with(other: StaticType): boolean {
		if (!(other instanceof ListType)) return false;
		return areCompatible(this.inner, other.inner);
	}

	_build() {
		const is_unrefined =
			this.inner instanceof GenericType && this.inner.is_open;

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
			[
				"map",
				{
					mutates: false,
					callable: true,
					parameters: [
						{
							name: "callback",
							type: new FunctionType({
								name: "callback",
								params: [{ name: "item", type: this.inner }],
								return_type: this.inner,
							}),
						},
					],
					return_type: EmptyList,
				},
			],
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

class StrType implements StaticType {
	readonly name = "string";
	readonly pretty = "Str";
	readonly is_iterable = true;
	readonly properties: Map<string, Signature>;

	constructor() {
		this.properties = this._build();
	}

	_build() {
		return new Map<string, Signature>([
			[
				"at",
				{
					mutates: false,
					callable: true,
					parameters: [{ name: "index", type: Num }],
					return_type: this,
				},
			],
			[
				"concat",
				{
					mutates: false,
					callable: true,
					parameters: [{ name: "string", type: this }],
					return_type: this,
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
	}
}

export const Str = new StrType();
export const Bool: StaticType = {
	name: "boolean",
	pretty: "Bool",
	is_iterable: false,
} as const;

export class GenericType implements StaticType {
	readonly name: string;
	_inner: StaticType | null = null;

	constructor(name: string) {
		this.name = name;
	}

	fill(type: StaticType) {
		this._inner = type;
	}

	get is_open() {
		return this._inner === null;
	}

	get pretty() {
		if (this._inner) return this._inner.pretty;
		return "?";
	}

	get is_iterable() {
		if (this._inner) return this._inner.is_iterable;
		return false;
	}
}
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
	if (a instanceof ListType) {
		return a.compatible_with(b);
	}
	if (a instanceof EnumType && b instanceof EnumVariant) return a === b.parent;
	if (a instanceof EnumVariant && b instanceof EnumType) return a.parent === b;
	if (a instanceof GenericType) {
		if (a.is_open) return true;
	}
	return a.pretty === b.pretty;
}

export type Signature = {
	mutates: boolean;
	callable: boolean;
	parameters?: ParameterType[];
	return_type: StaticType; // | (() => StaticType);
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
		return `(${this.parameters.map((p) => p.type.pretty).join(", ")}) ${
			this.return_type.pretty
		}`;
	}

	get is_iterable() {
		return false;
	}
}

export class Variable {
	readonly name: string;
	readonly static_type: StaticType;
	readonly is_mutable: boolean;

	constructor(input: {
		name: string;
		type: StaticType;
		is_mutable?: boolean;
	}) {
		this.name = input.name;
		this.static_type = input.type;
		this.is_mutable = input.is_mutable ?? false;
	}
}

// needed because Variable is a StaticType
// TODO: variables should not implement StaticType
export function get_cursor_type(type: StaticType): StaticType | null {
	if (type.is_iterable === false) return null;

	if (type instanceof ListType) {
		return type.inner;
	}
	if (type instanceof Variable) {
		return get_cursor_type(type.static_type);
	}
	return type;
}
