import {
	SyntaxType,
	type ListValueNode,
	type PrimitiveTypeNode,
	type PrimitiveValueNode,
	type StructDefinitionNode,
} from "../ast.ts";

export interface StaticType {
	// identifier
	name: string;
	// used for display
	pretty: string;
}

export class NumType implements StaticType {
	get name() {
		return this.pretty;
	}
	get pretty() {
		return "Num";
	}
}

export class StrType implements StaticType {
	get name() {
		return this.pretty;
	}
	get pretty() {
		return "Str";
	}
}

export class BoolType implements StaticType {
	constructor() {}

	get name() {
		return this.pretty;
	}
	get pretty() {
		return "Bool";
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
}

export const EmptyList: StaticType = { name: "EmptyList", pretty: "[]" };

export class MapType implements StaticType {
	constructor(readonly value: StaticType) {}

	get name() {
		return this.pretty;
	}
	get pretty() {
		return `[Str:${this.value.pretty}]`;
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
}

export const Num = new NumType();
export const Bool = new BoolType();
export const Str = new StrType();
export const Unknown: StaticType = { name: "unknown", pretty: "unknown" };
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
