import type { Point, Tree, TreeCursor } from "tree-sitter";
import {
	type NumberNode,
	type StringNode,
	SyntaxType,
	type BooleanNode,
	type ExpressionNode,
	type FunctionCallNode,
	type MemberAccessNode,
	type NamedNode,
	type StructDefinitionNode,
	type StructInstanceNode,
	type SyntaxNode,
	type TypeDeclarationNode,
	type TypedTreeCursor,
	type VariableDefinitionNode,
	type PrintStatementNode,
	type ReassignmentNode,
} from "../ast.ts";
import console from "node:console";
import {
	areCompatible,
	Bool,
	EmptyList,
	getStaticTypeForPrimitiveType,
	getStaticTypeForPrimitiveValue,
	ListType,
	MapType,
	Num,
	Str,
	StructType,
	Unknown,
} from "./kon-types.ts";

export type Diagnostic = {
	level: "error" | "warning";
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
	"print",
]);

interface StaticType {
	// identifier
	name: string;
	// used for display
	pretty: string;
}

class Variable implements StaticType {
	constructor(
		readonly name: string,
		readonly node: VariableDefinitionNode,
		readonly static_type: StaticType,
	) {}

	get is_mutable(): boolean {
		return this.node.bindingNode.text === "mut";
	}

	get pretty() {
		return this.static_type.pretty;
	}
}

// class StructDef implements StaticType {
// 	// Map<field_name, field_kon_type>
// 	readonly fields: Map<string, string> = new Map();

// 	constructor(readonly node: StructDefinitionNode) {
// 		for (const field of this.node.fieldNodes) {
// 			this.fields.set(field.nameNode.text, field.typeNode.text);
// 		}
// 	}

// 	get name(): string {
// 		return this.node.nameNode.text;
// 	}

// 	get static_type(): string {
// 		return this.name;
// 	}

// 	get pretty() {
// 		return this.name;
// 	}
// }

class LexScope {
	readonly variables: Map<string, Variable> = new Map();
	readonly structs: Map<string, StructType> = new Map();
}

export class Checker {
	cursor: TreeCursor;
	errors: Diagnostic[] = [];
	scopes: LexScope[] = [new LexScope()];

	constructor(readonly tree: Tree) {
		this.cursor = tree.walk();
	}

	private debug(message: any, ...extra: any[]) {
		if (Deno.env.get("NODE_ENV") === "test") {
			console.debug(message, ...extra);
		}
	}

	check(): Diagnostic[] {
		const cursor = this.cursor as unknown as TypedTreeCursor;
		this.visit(cursor.currentNode);
		return this.errors;
	}

	visit(node: SyntaxNode) {
		if (node === null) return;
		const methodName = `visit${node.type
			.split("_")
			.map((part) => part.charAt(0).toUpperCase() + part.slice(1))
			.join("")}`;
		this.debug("visiting ", node.type);

		// @ts-expect-error - dynamic method call
		const method = this[methodName]?.bind(this);
		if (method) {
			return method(node);
		}
		this.debug(`No visit method for ${node.type}, going through children`);

		for (const child of node.namedChildren) {
			this.visit(child);
		}
		return;
	}

	private error(error: Omit<Diagnostic, "level">) {
		this.errors.push({ ...error, level: "error" });
	}

	private warn(warning: Omit<Diagnostic, "level">) {
		this.errors.push({ ...warning, level: "warning" });
	}

	private scope(): LexScope {
		if (this.scopes.length === 0) throw new Error("No scope found");
		return this.scopes.at(0)!;
	}

	visitStructDefinition(node: StructDefinitionNode) {
		const def = StructType.from(node);
		this.scope().structs.set(def.name, def);
	}

	visitStructInstance(node: StructInstanceNode): StructType | null {
		const struct_name = node.nameNode.text;
		const struct_def = this.scope().structs.get(struct_name);
		if (!struct_def) {
			this.error({
				message: `Missing definition for type '${struct_name}'.`,
				location: node.startPosition,
			});
			return null;
		}

		const expected_fields = struct_def.fields;
		const received_fields = new Set<string>();
		for (const inputFieldNode of node.fieldNodes) {
			const member_name = inputFieldNode.nameNode.text;
			if (expected_fields.has(member_name)) {
				const expected_type = expected_fields.get(member_name)!;
				const provided_type = this.getTypeFromExpressionNode(
					inputFieldNode.valueNode,
				);
				if (!areCompatible(expected_type, provided_type)) {
					this.error({
						location: inputFieldNode.valueNode.startPosition,

						message: `Expected '${expected_type.pretty}' and received '${provided_type.pretty}'.`,
					});
				}
				received_fields.add(member_name);
			}

			if (!expected_fields.has(member_name)) {
				this.error({
					message: `Struct '${struct_name}' does not have a field named ${member_name}.`,
					location: inputFieldNode.startPosition,
				});
				continue;
			}
		}

		const missing_field_names = new Set<string>();
		for (const field of expected_fields.keys()) {
			if (!received_fields.has(field)) {
				missing_field_names.add(field);
			}
		}
		if (missing_field_names.size > 0) {
			this.error({
				message: `Missing fields for struct '${struct_name}': ${Array.from(
					missing_field_names,
				).join(", ")}.`,
				location: node.startPosition,
			});
		}
		return struct_def;
	}

	visitVariableDefinition(node: VariableDefinitionNode) {
		const name = node.nameNode;
		this.validateIdentifier(node.nameNode);

		const typeNode = node.typeNode?.typeNode;
		const value = node.valueNodes.filter((n) => n.isNamed).at(0);
		if (value == null) {
			// can't really get here because tree-sitter captures a situation like this as an error
			this.error({
				message: "Variables must be initialized",
				location: node.startPosition,
			});
			return;
		}

		let declared_type = typeNode ? this.getTypeFromTypeDefNode(typeNode) : null;
		const provided_type = this.getTypeFromExpressionNode(value);
		if (declared_type === null) {
			// lazy-ish inference
			declared_type = provided_type;
		}
		const assigment_error = this.validateCompatibility(
			declared_type,
			provided_type,
		);
		if (assigment_error != null) {
			this.error({
				location: value.startPosition,

				message: assigment_error,
			});
		}

		const variable = new Variable(name.text, node, declared_type);
		this.scope().variables.set(name.text, variable);
		return;
	}

	visitReassignment(node: ReassignmentNode) {
		const target = node.nameNode;
		const variable = this.scope().variables.get(target.text);
		if (variable == null) {
			this.error({
				message: `Variable '${target.text}' is not defined.`,
				location: target.startPosition,
			});
			return;
		}

		if (!variable.is_mutable) {
			this.error({
				message: `Variable '${target.text}' is not mutable.`,
				// use location of = operator
				location: node.children.at(1)!.startPosition,
			});
		}
	}

	private validateCompatibility(
		expected: StaticType,
		received: StaticType,
	): string | null {
		const is_valid = areCompatible(expected, received);
		if (is_valid) {
			return null;
		}
		if (expected === Unknown || received === Unknown) return null;
		return `Expected '${expected.pretty}' and received '${received.pretty}'.`;
	}

	// return the kon type from the declaration
	private getTypeFromTypeDefNode(
		node: TypeDeclarationNode["typeNode"],
	): StaticType {
		switch (node.type) {
			case SyntaxType.PrimitiveType:
				return getStaticTypeForPrimitiveType(node);
			case SyntaxType.ListType: {
				switch (node.innerNode.type) {
					case SyntaxType.Identifier: {
						// check that the type exists
						const declaration =
							this.scope().variables.get(node.innerNode.text) ??
							this.scope().structs.get(node.innerNode.text);
						if (declaration == null) {
							this.error({
								location: node.innerNode.startPosition,
								message: `Missing definition for type '${node.innerNode.text}'.`,
							});
							return new ListType(Unknown);
						}
						return new ListType(declaration);
					}
					case SyntaxType.PrimitiveType: {
						return new ListType(getStaticTypeForPrimitiveType(node.innerNode));
					}
					default:
						return Unknown;
				}
			}
			default:
				return Unknown;
		}
	}

	private getTypeFromExpressionNode(
		node: ExpressionNode | BooleanNode | NumberNode | StringNode,
	): StaticType {
		switch (node.type) {
			case SyntaxType.Boolean:
				return Bool;
			case SyntaxType.Number:
				return Num;
			case SyntaxType.String:
				return Str;
			case SyntaxType.PrimitiveValue: {
				return getStaticTypeForPrimitiveValue(node);
			}
			case SyntaxType.ListValue: {
				if (node.innerNodes.length === 0) {
					return EmptyList;
				}
				const first = node.innerNodes.at(0)!;
				switch (first.type) {
					case SyntaxType.Boolean:
						return new ListType(Bool);
					case SyntaxType.String:
						return new ListType(Str);
					case SyntaxType.Number:
						return new ListType(Num);
					// case SyntaxType.ListValue:
					// 	return new ListType(this.getTypeFromExpressionNode(first));
					// case SyntaxType.MapValue:
					// 	return new ListType(this.getTypeFromExpressionNode(first));
					case SyntaxType.StructInstance: {
						const struct = this.visitStructInstance(first) ?? Unknown;
						if (struct === Unknown) {
							this.error({
								location: first.startPosition,
								message: `Unknown struct`,
							});
						}
						return new ListType(struct);
					}
					default: {
						this.warn({
							location: first.startPosition,
							message: `Unknown type in list`,
						});
						return new ListType(Unknown);
					}
				}
			}
			case SyntaxType.StructInstance: {
				return this.visitStructInstance(node) ?? Unknown;
			}
			default: {
				return Unknown;
			}
		}
	}

	visitPrintStatement(_: PrintStatementNode) {
		// todo: validate that arguments are printable
	}

	visitFunctionCall(node: FunctionCallNode) {
		const { targetNode, argumentsNode } = node;
		const targetVariable = (() => {
			switch (targetNode.type) {
				case SyntaxType.Identifier: {
					const variable = this.scope().variables.get(targetNode.text);
					if (!variable) {
						this.error({
							location: targetNode.startPosition,
							message: `Missing declaration for '${targetNode.text}'.`,
						});
						return;
					}
					return variable;
				}
				default:
					return null;
			}
		})();

		if (targetVariable?.static_type instanceof ListType) {
			if (LIST_MEMBERS.has(argumentsNode.text)) {
				const signature = LIST_MEMBERS.get(argumentsNode.text)!;
				if (signature.mutates && !targetVariable.is_mutable) {
					this.error({
						location: argumentsNode.startPosition,
						message: `Cannot mutate an immutable list. Use 'mut' to make it mutable.`,
					});
				}
			} else {
				this.error({
					location: argumentsNode.startPosition,
					message: `Unknown member '${argumentsNode.text}' for list type.`,
				});
			}
			return;
		}

		switch (targetVariable?.static_type) {
			case undefined:
			case null: {
				this.warn({
					location: targetNode.startPosition,
					message: `The type of '${targetNode.text}' is unknown.`,
				});
				break;
			}
			default: {
				this.debug(`Unknown type: ${targetVariable?.static_type}`);
				break;
			}
		}
	}

	visitMemberAccess(node: MemberAccessNode) {
		const { memberNode } = node;
		const targetNode = node.targetNodes.filter((n) => n.isNamed).at(0);
		if (targetNode == null) throw new Error("Invalid member access");

		switch (targetNode.type) {
			case SyntaxType.Identifier: {
				const variable = this.scope().variables.get(targetNode.text);
				if (!variable) {
					this.error({
						location: targetNode.startPosition,
						message: `Missing declaration for '${targetNode.text}'.`,
					});
					return;
				}

				if (variable.static_type instanceof ListType) {
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
										location: memberNode.startPosition,
										message: `Cannot mutate an immutable list. Use 'mut' to make it mutable.`,
									});
								}
								if (!signature.callable) {
									this.error({
										location: memberNode.startPosition,
										message: `${variable.name}.${member} is not a callable function.`,
									});
								}
							} else {
								this.error({
									location: memberNode.startPosition,
									message: `Unknown member '${member}' for list type.`,
								});
							}
						}
					}
					// 	break;
					// }
					// case null: {
					// 	this.error({
					// 		location: targetNode.startPosition,
					// 		message: `The type of '${targetNode.text}' is unknown.`,
					// 	});
					// 	break;
					// }
					// default: {
					// 	this.debug(`Unknown type: ${variable.static_type}`);
					// 	break;
					// }
				}
			}
		}
	}

	validateIdentifier(node: NamedNode) {
		if (RESERVED_KEYWORDS.has(node.text)) {
			this.error({
				location: node.startPosition,

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
