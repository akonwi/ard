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

class Variable {
	constructor(
		readonly name: string,
		readonly node: SyntaxNode,
	) {}
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
		if (node.type === "variable_definition") {
			const name = node.childForFieldName("name");
			this.validateIdentifier(name);
			const type = node.namedChildren
				.find((n) => n.grammarType === "type_declaration")
				?.childForFieldName("type");
			const value = node.childForFieldName("value");
			if (!(name && type && value)) {
				return;
			}

			const getValueLabel = (value: SyntaxNode) => {
				switch (value.type) {
					case "primitive_value": {
						const child = value.firstChild;
						switch (child?.grammarType) {
							case "number":
								return "Num";
							case "string":
								return "Str";
							case "boolean":
								return "Bool";
							default:
								return "unknown";
						}
					}
				}
			};

			// todo: require types until inference is implemented
			switch (type.text) {
				case "Str": {
					if (value.firstChild?.type !== "string") {
						this.error({
							location: value.startPosition,
							level: "error",
							message: `Expected a 'Str' but got '${getValueLabel(value)}'`,
						});
					}
					break;
				}
				case "Num": {
					if (value.firstChild?.type !== "number") {
						this.error({
							location: value.startPosition,
							level: "error",
							message: `Expected a 'Num' but got '${getValueLabel(value)}'`,
						});
					}
					break;
				}
				case "Bool": {
					if (value.firstChild?.type !== "boolean") {
						this.error({
							location: value.startPosition,
							level: "error",
							message: `Expected a 'Bool' but got '${getValueLabel(value)}'`,
						});
					}
				}
			}

			this.scope().definitions.set(name.text, new Variable(name.text, node));

			this.cursor.gotoParent();
			return;
		}

		if (node.type === "block") {
			// todo: create a new scope
		}

		if (node.type === "member_access") {
			this.visitMemberAccess(node);
			return;
		}

		this.check();
	}

	private visitMemberAccess(node: SyntaxNode) {
		const [target, member] = node.namedChildren;
		if (!target || !member) {
			this.error({
				level: "error",
				location: node.startPosition,
				message: "Invalid member access.",
			});
			return;
		}

		if (target.grammarType === "identifier") {
			const variable = this.scope().definitions.get(target.text);
			if (!variable) {
				this.error({
					level: "error",
					location: target.startPosition,
					message: `Missing declaration for '${target.text}'.`,
				});
				return;
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
