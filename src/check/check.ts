import type { Point, SyntaxNode, Tree, TreeCursor } from "tree-sitter";

/*
 * Lists
 * length: Num
 *
 */

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

const textToTokenType: Record<string, string> = {
	Str: "string",
	Num: "number",
	Bool: "boolean",
};

const tokenTypeToText: Record<string, string> = {
	string: "Str",
	number: "Num",
	boolean: "Bool",
};

class Variable {
	constructor(
		readonly name: string,
		readonly node: SyntaxNode,
	) {}

	get is_mutable(): boolean {
		return this.node.childForFieldName("binding")?.text === "mut";
	}

	get static_type(): string | null {
		const type_declaration = this.node.namedChildren.find(
			(n) => n.grammarType === "type_declaration",
		);
		if (!type_declaration) {
			return null;
		}
		const type_node = type_declaration.childForFieldName("type");
		if (!type_node) return null;
		return type_node.grammarType;
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
				this.visitNode(this.cursor.currentNode);
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

	// if a check can be done, do it and go back to the parent node
	// otherwise, continue to the next child
	private visitNode(node: SyntaxNode) {
		switch (node.type) {
			case "statement": {
				this.check();
				return;
			}
			case "variable_definition": {
				this.visitVariableDefinition(node);
				return;
			}
			case "member_access": {
				this.visitMemberAccess(node);
				return;
			}
			case "function_call": {
				return this.visitFunctionCall(node);
			}
			default: {
				console.debug("skipping node", { type: node.type });
			}
		}

		this.cursor.gotoParent();
	}

	private visitVariableDefinition(node: SyntaxNode) {
		const name = node.childForFieldName("name");
		this.validateIdentifier(name);

		const typeNode = node.namedChildren
			.find((n) => n.grammarType === "type_declaration")
			?.childForFieldName("type");
		// if (!type)
		// 	this.error({
		// 		level: "error",
		// 		message: `Missing type declaration for variable ${name?.text ?? ""}.`,
		// 		location: node.startPosition,
		// 	});
		const value = node.childForFieldName("value");
		if (!(name && typeNode && value)) {
			return;
		}

		const declared_type = this.getTypeFromTypeDefNode(typeNode);
		const provided_type = this.getTypeFromValueNode(value);
		if (declared_type !== provided_type) {
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

	private visitVariableTypeDeclaration(node: SyntaxNode): string {
		switch (node.grammarType) {
			case "primitive_type":
				return node.text;
			case "list_type": {
				const type = node.childForFieldName("inner")?.grammarType;
				if (!type) throw new Error("Invalid list type");
				switch (type) {
					case "identifier": {
						// todo: check that the type exists
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

	private getTypeFromTypeDefNode(node: SyntaxNode): string {
		switch (node.grammarType) {
			case "primitive_type":
				return node.text;
			// case "list_type": {
			// 	const type = node.childForFieldName("inner")?.grammarType;
			// 	if (!type) throw new Error("Invalid list type");
			// 	switch (type) {
			// 		case "identifier": {
			// 			// todo: check that the type exists
			// 			return node.text;
			// 		}
			// 		case "primitive_type": {
			// 			return node.text;
			// 		}
			// 		default:
			// 			return "unknown";
			// 	}
			// }
			default:
				return "unknown";
		}
	}

	private getTypeFromValueNode(node: SyntaxNode): string {
		if (node.firstNamedChild?.type == null) return "unknown";
		const tokenType = node.firstNamedChild!.type!;
		const konType = tokenTypeToText[tokenType];
		if (konType == null) {
			console.debug("Unknown kon type for ", tokenType);
			return "unknown";
		}
		return konType;
	}

	private visitFunctionCall(node: SyntaxNode) {
		const [targetNode, args] = node.namedChildren;
		if (!targetNode || !args) {
			this.error({
				level: "error",
				location: node.startPosition,
				message: "Invalid function call.",
			});
			return;
		}

		let targetVariable = (() => {
			switch (targetNode.grammarType) {
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
				case "member_access": {
					const [target, member] = targetNode.namedChildren;
				}
				default:
					return null;
			}
		})();

		switch (targetVariable?.static_type) {
			case "list_type": {
				console.log("checking list member access");
				if (LIST_MEMBERS.has(args.text)) {
					const signature = LIST_MEMBERS.get(args.text)!;
					if (signature.mutates && !targetVariable.is_mutable) {
						this.error({
							level: "error",
							location: args.startPosition,
							message: `Cannot mutate an immutable list. Use 'mut' to make it mutable.`,
						});
					}
				} else {
					this.error({
						level: "error",
						location: args.startPosition,
						message: `Unknown member '${args.text}' for list type.`,
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

	private visitMemberAccess(node: SyntaxNode) {
		const [targetNode, memberNode] = node.namedChildren;
		if (!targetNode || !memberNode) {
			this.error({
				level: "error",
				location: node.startPosition,
				message: "Invalid member access.",
			});
			return;
		}

		if (targetNode.grammarType === "identifier") {
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
						case "function_call": {
							const member = memberNode.childForFieldName("target")?.text!;
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

	private validateIdentifier(node: SyntaxNode | null) {
		if (!node) {
			return;
		}

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
