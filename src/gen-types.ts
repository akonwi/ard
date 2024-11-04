import console from "node:console";

interface NodeTypeRef {
	type: string;
	named: boolean;
	isError?: boolean;
}
interface NodeTypeEntry extends NodeTypeRef {
	subtypes?: NodeTypeRef[];
	fields?: Record<string, NodeTypeChildren>;
	children?: NodeTypeChildren;
}
interface NodeTypeChildren {
	multiple: boolean;
	required: boolean;
	types: NodeTypeRef[];
}

class Printer {
	private indentation = "";

	indent(): this {
		this.indentation += "  ";
		return this;
	}
	deindent(): this {
		this.indentation = this.indentation.substring(
			0,
			this.indentation.length - 2,
		);
		return this;
	}
	println(str?: string): this {
		if (str == null) {
			console.log();
		} else {
			console.log(this.indentation + str);
		}
		return this;
	}
	printEach(items: (string | void)[] | void): this {
		if (items == null) return this;
		for (let item of items) {
			if (item == null) continue;
			this.println(item);
		}
		return this;
	}
	forEach<U>(
		items: U[] | void,
		fn: (item: NonNullable<U>, printer: Printer) => void,
	): this {
		if (items == null) return this;
		for (let item of items) {
			if (item == null) continue;
			fn(item!, this);
		}
		return this;
	}
	forEachInRecord<U>(
		items: Record<string, U> | void,
		fn: (key: string, item: NonNullable<U>, printer: Printer) => void,
	): this {
		if (items == null) return this;
		for (let key of Object.keys(items)) {
			let item = items[key];
			if (item == null) continue;
			fn(key, item!, this);
		}
		return this;
	}
}

function isIdentifier(str: string) {
	return /^[a-z$_][a-z0-9$_]*$/i.test(str);
}

function mangleNameToIdentifier(str: string) {
	let sb = "$";
	for (let i = 0; i < str.length; ++i) {
		let char = str.charAt(i);
		if (/[a-z0-9_]/i.test(char)) {
			sb += char;
		} else {
			sb += "$" + str.charCodeAt(i) + "$";
		}
	}
	return sb;
}

function toCapitalCase(str: string) {
	return str
		.replace(/^[a-z]/, (t) => t.toUpperCase())
		.replace(/_[a-zA-Z]/g, (t) => t.substring(1).toUpperCase());
}

function getTypePrefixFromString(str: string) {
	return isIdentifier(str) ? toCapitalCase(str) : mangleNameToIdentifier(str);
}

function getTypeNameFromString(str: string) {
	return getTypePrefixFromString(str) + "Node";
}

function getSyntaxKindFromString(str: string) {
	return getTypePrefixFromString(str);
}

function getTypeExprFromRef(ref: NodeTypeRef, index: IndexedData) {
	if (ref.isError) {
		return "ErrorNode";
	}
	if (!ref.named) {
		let name = index.typeNames.get(ref.type);
		let arg = name != null ? `SyntaxType.${name}` : JSON.stringify(ref.type);
		return `UnnamedNode<${arg}>`;
	}
	return getTypeNameFromString(ref.type);
}

interface IndexedData {
	typeNames: Map<string, string>;
}

function buildIndex(json: NodeTypeEntry[]): IndexedData {
	let typeNames = new Map<string, string>();
	for (let entry of json) {
		if (entry.named) {
			let name = getSyntaxKindFromString(entry.type);
			typeNames.set(entry.type, name);
		}
	}
	return { typeNames };
}

function generatePreamble(json: NodeTypeEntry[], printer: Printer) {
	printer.println(`import type { Point, Tree, TreeCursor } from "tree-sitter";

interface SyntaxNodeBase {
		tree: Tree;
		type: string;
		isNamed: boolean;
		text: string;
		startPosition: Point;
		endPosition: Point;
		startIndex: number;
		endIndex: number;
		parent: SyntaxNode | null;
		children: Array<SyntaxNode>;
		namedChildren: Array<SyntaxNode>;
		childCount: number;
		namedChildCount: number;
		firstChild: SyntaxNode | null;
		firstNamedChild: SyntaxNode | null;
		lastChild: SyntaxNode | null;
		lastNamedChild: SyntaxNode | null;
		nextSibling: SyntaxNode | null;
		nextNamedSibling: SyntaxNode | null;
		previousSibling: SyntaxNode | null;
		previousNamedSibling: SyntaxNode | null;

		hasChanges(): boolean;
		hasError(): boolean;
		isMissing(): boolean;
		toString(): string;
		child(index: number): SyntaxNode | null;
		namedChild(index: number): SyntaxNode | null;
		firstChildForIndex(index: number): SyntaxNode | null;
		firstNamedChildForIndex(index: number): SyntaxNode | null;

		descendantForIndex(index: number): SyntaxNode;
		descendantForIndex(startIndex: number, endIndex: number): SyntaxNode;
		namedDescendantForIndex(index: number): SyntaxNode;
		namedDescendantForIndex(startIndex: number, endIndex: number): SyntaxNode;
		descendantForPosition(position: Point): SyntaxNode;
		descendantForPosition(startPosition: Point, endPosition: Point): SyntaxNode;
		namedDescendantForPosition(position: Point): SyntaxNode;
		namedDescendantForPosition(
			startPosition: Point,
			endPosition: Point,
		): SyntaxNode;
		descendantsOfType<T extends TypeString>(
			types: T | readonly T[],
			startPosition?: Point,
			endPosition?: Point,
		): NodeOfType<T>[];

		closest<T extends SyntaxType>(types: T | readonly T[]): NamedNode<T> | null;
		walk(): TreeCursor;
}

interface NamedNodeBase extends SyntaxNodeBase {
    isNamed: true;
}

/** An unnamed node with the given type string. */
export interface UnnamedNode<T extends string = string> extends SyntaxNodeBase {
  type: T;
  isNamed: false;
}

type PickNamedType<Node, T extends string> = Node extends { type: T; isNamed: true } ? Node : never;

type PickType<Node, T extends string> = Node extends { type: T } ? Node : never;

/** A named node with the given \`type\` string. */
export type NamedNode<T extends SyntaxType = SyntaxType> = PickNamedType<SyntaxNode, T>;

/**
 * A node with the given \`type\` string.
 *
 * Note that this matches both named and unnamed nodes. Use \`NamedNode<T>\` to pick only named nodes.
 */
export type NodeOfType<T extends string> = PickType<SyntaxNode, T>;

interface TreeCursorOfType<S extends string, T extends SyntaxNodeBase> {
  nodeType: S;
  currentNode: T;
}

type TreeCursorRecord = { [K in TypeString]: TreeCursorOfType<K, NodeOfType<K>> };

/**
 * A tree cursor whose \`nodeType\` correlates with \`currentNode\`.
 *
 * The typing becomes invalid once the underlying cursor is mutated.
 *
 * The intention is to cast a \`TreeCursor\` to \`TypedTreeCursor\` before
 * switching on \`nodeType\`.
 *
 * For example:
 * \`\`\`ts
 * let cursor = root.walk();
 * while (cursor.gotoNextSibling()) {
 *   const c = cursor as TypedTreeCursor;
 *   switch (c.nodeType) {
 *     case SyntaxType.Foo: {
 *       let node = c.currentNode; // Typed as FooNode.
 *       break;
 *     }
 *   }
 * }
 * \`\`\`
 */
export type TypedTreeCursor = TreeCursorRecord[keyof TreeCursorRecord];

export interface ErrorNode extends NamedNodeBase {
    type: SyntaxType.ERROR;
    hasError(): true;
}
`);
}

function generateTypeEnum(
	json: NodeTypeEntry[],
	{ typeNames }: IndexedData,
	printer: Printer,
) {
	printer
		.println("export const enum SyntaxType {")
		.indent()
		.println('ERROR = "ERROR",')
		.forEach(json, (entry) => {
			if (
				entry.named &&
				(entry.subtypes == null || entry.subtypes.length === 0)
			) {
				let name = getSyntaxKindFromString(entry.type);
				printer.println(`${name} = ${JSON.stringify(entry.type)},`);
			}
		})
		.deindent()
		.println("}")
		.println()
		.println("export type UnnamedType =")
		.indent()
		.forEach(json, (entry) => {
			if (!entry.named) {
				let name = typeNames.get(entry.type);
				if (name != null) {
					printer.println(`| SyntaxType.${name} // both named and unnamed`);
				} else {
					printer.println(`| ${JSON.stringify(entry.type)}`);
				}
			}
		})
		.println(";")
		.deindent()
		.println()
		.println("export type TypeString = SyntaxType | UnnamedType;")
		.println();
}

function generateNamedDeclaration(
	entry: NodeTypeEntry,
	index: IndexedData,
	printer: Printer,
) {
	if (!entry.named) return;
	if (entry.subtypes != null && entry.subtypes.length > 0) {
		generateUnionFromEntry(entry, index, printer);
	} else {
		generateInterfaceFromEntry(entry, index, printer);
	}
}

function generateInterfaceFromEntry(
	entry: NodeTypeEntry,
	index: IndexedData,
	printer: Printer,
) {
	let kind = getSyntaxKindFromString(entry.type);
	let name = getTypeNameFromString(entry.type);
	printer
		.println(`export interface ${name} extends NamedNodeBase {`)
		.indent()
		.println(`type: SyntaxType.${kind};`)
		.forEachInRecord(entry.fields, (field, children) => {
			let fieldName = field + "Node";
			let type = children.types
				.map((t) => getTypeExprFromRef(t, index))
				.join(" | ");
			if (type === "") {
				type = "UnnamedNode";
			}
			if (children.multiple) {
				if (children.types.length > 1) {
					type = "(" + type + ")";
				}
				type += "[]";
				fieldName += "s";
			}
			let opt = children.required || children.multiple ? "" : "?";
			printer.println(`${fieldName}${opt}: ${type};`);
		})
		.deindent()
		.println("}")
		.println();
}

function generateUnionFromEntry(
	entry: NodeTypeEntry,
	index: IndexedData,
	printer: Printer,
) {
	generateUnion(
		getTypeNameFromString(entry.type),
		entry.subtypes!,
		index,
		printer,
	);
}

function generateRootUnion(
	json: NodeTypeEntry[],
	index: IndexedData,
	printer: Printer,
) {
	let errorType: NodeTypeRef = { type: "ERROR", named: true, isError: true };
	generateUnion("SyntaxNode", [...json, errorType], index, printer);
}

function generateUnion(
	name: string,
	members: NodeTypeRef[],
	index: IndexedData,
	printer: Printer,
) {
	printer
		.println(`export type ${name} = `)
		.indent()
		.forEach(members, (ref) => {
			printer.println("| " + getTypeExprFromRef(ref, index));
		})
		.println(";")
		.deindent()
		.println();
}

const usageText = `
  Usage: dts-tree-sitter INPUT > OUTPUT.d.ts

  Generates a .d.ts file to stdout.
`;

function main() {
	if (Deno.args.length !== 1) {
		console.error(usageText);
		Deno.exit(1);
	}
	const filename = Deno.args.at(0)!;
	const json = JSON.parse(Deno.readTextFileSync(filename!)) as NodeTypeEntry[];
	const index = buildIndex(json);
	const printer = new Printer();
	generatePreamble(json, printer);
	generateTypeEnum(json, index, printer);
	generateRootUnion(json, index, printer);
	printer.forEach(json, (t) => generateNamedDeclaration(t, index, printer));
}

main();
