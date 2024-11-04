import type { Point, Tree, TreeCursor } from "tree-sitter";
import {
	SyntaxType,
	type ExpressionNode,
	type FunctionCallNode,
	type MemberAccessNode,
	type NamedNode,
	type TypeDeclarationNode,
	type TypedTreeCursor,
	type VariableDefinitionNode,
} from "../ast.ts";

export type Diagnostic = {
	level: "error";
	location: Point;
	message: string;
};

const RESERVED_KEYWORDS = new Set([
	"let",
	"of",
	"in",
	"if",
	"else",
	"true",
	"false",
	"or",
	"and",
	"struct",
	"enum",
]);

const tokenTypeToText: Record<
	SyntaxType.Boolean | SyntaxType.Number | SyntaxType.String,
	string
> = {
	string: "Str",
	number: "Num",
	boolean: "Bool",
};

class Variable {
	constructor(
		readonly name: string,
		readonly node: VariableDefinitionNode,
	) {}

	get is_mutable(): boolean {
		return this.node.bindingNode.text === "mut";
	}

	get static_type(): string | null {
		const type_declaration = this.node.typeNode?.typeNode;
		if (!type_declaration) {
			return null;
		}
		switch (type_declaration.type) {
			case SyntaxType.PrimitiveType:
				return type_declaration.text;
			case SyntaxType.ListType:
				return SyntaxType.ListType;
			// return `[${type_declaration.innerNode.text}]`;
			case SyntaxType.MapType:
				return `{${type_declaration.keyNode.text}: ${type_declaration.valueNode.text}}`;
			default:
				return null;
		}
	}
}

class LexScope {
	readonly definitions: Map<string, Variable> = new Map();
}

export class Checker {
	cursor: TreeCursor;
	errors: Diagnostic[] = [];
	scopes: LexScope[] = [new LexScope()];

	constructor(readonly tree: Tree) {
		this.cursor = tree.walk();
	}

	check(): Diagnostic[] {
		// go through children
		if (this.cursor.gotoFirstChild()) {
			do {
				console.debug(this.cursor.currentNode.type, {
					text: this.cursor.currentNode.text,
				});
				const cursor = this.cursor as unknown as TypedTreeCursor;
				switch (cursor.nodeType) {
					case SyntaxType.Statement: {
						this.check();
						break;
					}
					case SyntaxType.VariableDefinition: {
						this.visitVariableDefinition(cursor.currentNode);
						break;
					}
					case SyntaxType.FunctionCall: {
						this.visitFunctionCall(cursor.currentNode);
						break;
					}
					case SyntaxType.MemberAccess: {
						this.visitMemberAccess(cursor.currentNode);
						break;
					}
					default: {
						this.cursor.gotoParent();
					}
				}
			} while (this.cursor.gotoNextSibling());
		}

		return this.errors;
	}

	private error(error: Diagnostic) {
		this.errors.push(error);
	}

	private scope(): LexScope {
		if (this.scopes.length === 0) throw new Error("No scope found");
		return this.scopes.at(0)!;
	}

	private visitVariableDefinition(node: VariableDefinitionNode) {
		const name = node.nameNode;
		this.validateIdentifier(node.nameNode);

		const typeNode = node.typeNode?.typeNode;
		// if (!type)
		// 	this.error({
		// 		level: "error",
		// 		message: `Missing type declaration for variable ${name?.text ?? ""}.`,
		// 		location: node.startPosition,
		// 	});
		const value = node.valueNodes.filter((n) => n.isNamed).at(0);
		if (!(typeNode && value)) {
			return;
		}

		const declared_type = this.getTypeFromTypeDefNode(typeNode);
		const provided_type = this.getTypeFromExpressionNode(value);
		if (
			declared_type !== provided_type &&
			provided_type !== "unknown" &&
			declared_type !== "unknown"
		) {
			this.error({
				location: value.startPosition,
				level: "error",
				message: `Expected a '${declared_type}' but got '${provided_type}'`,
			});
		}

		const variable = new Variable(name.text, node);
		this.scope().definitions.set(name.text, variable);

		this.cursor.gotoParent();
		return;
	}

	private getTypeFromTypeDefNode(
		node: TypeDeclarationNode["typeNode"],
	): string {
		switch (node.type) {
			case SyntaxType.PrimitiveType:
				return node.text;
			case SyntaxType.ListType: {
				switch (node.innerNode.type) {
					case "identifier": {
						// check that the type exists
						if (!this.scope().definitions.has(node.innerNode.text)) {
							this.error({
								level: "error",
								location: node.innerNode.startPosition,
								message: `Missing definition for type '${node.innerNode.text}'.`,
							});
						}
						return node.text;
					}
					case "primitive_type": {
						return node.text;
					}
					default:
						return "unknown";
				}
			}
			default:
				return "unknown";
		}
	}

	private getTypeFromExpressionNode(node: ExpressionNode): string {
		switch (node.type) {
			case SyntaxType.PrimitiveValue: {
				// @ts-expect-error not supporting everything yet
				return tokenTypeToText[node.primitiveNode.type] ?? "unknown";
			}
			default: {
				return "unknown";
			}
		}
	}

	private visitFunctionCall(node: FunctionCallNode) {
		const { targetNode, argumentsNode } = node;
		const targetVariable = (() => {
			switch (targetNode.type) {
				case "identifier": {
					const variable = this.scope().definitions.get(targetNode.text);
					if (!variable) {
						this.error({
							level: "error",
							location: targetNode.startPosition,
							message: `Missing declaration for '${targetNode.text}'.`,
						});
						return;
					}
					return variable;
				}
				case SyntaxType.Identifier: {
					// TODO
					return null;
				}
				default:
					return null;
			}
		})();

		switch (targetVariable?.static_type) {
			case "list_type": {
				if (LIST_MEMBERS.has(argumentsNode.text)) {
					const signature = LIST_MEMBERS.get(argumentsNode.text)!;
					if (signature.mutates && !targetVariable.is_mutable) {
						this.error({
							level: "error",
							location: argumentsNode.startPosition,
							message: `Cannot mutate an immutable list. Use 'mut' to make it mutable.`,
						});
					}
				} else {
					this.error({
						level: "error",
						location: argumentsNode.startPosition,
						message: `Unknown member '${argumentsNode.text}' for list type.`,
					});
				}
				break;
			}
			case undefined:
			case null: {
				this.error({
					level: "error",
					location: targetNode.startPosition,
					message: `The type of '${targetNode.text}' is unknown.`,
				});
				break;
			}
			default: {
				console.log(`Unknown type: ${targetVariable?.static_type}`);
				break;
			}
		}
	}

	private visitMemberAccess(node: MemberAccessNode) {
		const { memberNode } = node;
		const targetNode = node.targetNodes.filter((n) => n.isNamed).at(0);
		if (targetNode == null) throw new Error("Invalid member access");

		switch (targetNode.type) {
			case SyntaxType.Identifier: {
				const variable = this.scope().definitions.get(targetNode.text);
				if (!variable) {
					this.error({
						level: "error",
						location: targetNode.startPosition,
						message: `Missing declaration for '${targetNode.text}'.`,
					});
					return;
				}

				switch (variable.static_type) {
					case "list_type": {
						switch (memberNode.type) {
							case SyntaxType.FunctionCall: {
								const member = memberNode.targetNode.text;
								if (!member) {
									return;
								}
								if (LIST_MEMBERS.has(member)) {
									const signature = LIST_MEMBERS.get(member)!;
									if (signature.mutates && !variable.is_mutable) {
										this.error({
											level: "error",
											location: memberNode.startPosition,
											message: `Cannot mutate an immutable list. Use 'mut' to make it mutable.`,
										});
									}
									if (!signature.callable) {
										this.error({
											level: "error",
											location: memberNode.startPosition,
											message: `${variable.name}.${member} is not a callable function.`,
										});
									}
								} else {
									this.error({
										level: "error",
										location: memberNode.startPosition,
										message: `Unknown member '${member}' for list type.`,
									});
								}
							}
						}
						break;
					}
					case null: {
						this.error({
							level: "error",
							location: targetNode.startPosition,
							message: `The type of '${targetNode.text}' is unknown.`,
						});
						break;
					}
					default: {
						console.log(`Unknown type: ${variable.static_type}`);
						break;
					}
				}
			}
		}
	}

	private validateIdentifier(node: NamedNode) {
		if (RESERVED_KEYWORDS.has(node.text)) {
			this.error({
				location: node.startPosition,
				level: "error",
				message: `'${node.text}' is a reserved keyword and cannot be used as a variable name`,
			});
		}
	}
}

const LIST_MEMBERS = new Map<string, { callable: boolean; mutates: boolean }>([
	["at", { mutates: false, callable: true }],
	["concat", { mutates: false, callable: true }],
	["copyWithin", { mutates: true, callable: true }],
	["length", { mutates: false, callable: false }],
	["pop", { mutates: true, callable: true }],
	["push", { mutates: true, callable: true }],
	["reverse", { mutates: true, callable: true }],
	["shift", { mutates: true, callable: true }],
	["slice", { mutates: false, callable: true }],
	["sort", { mutates: true, callable: true }],
	["splice", { mutates: true, callable: true }],
]);
