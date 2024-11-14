import type { Point, Tree, TreeCursor } from "tree-sitter";
import {
	SyntaxType,
	type ExpressionNode,
	type FunctionCallNode,
	type MemberAccessNode,
	type NamedNode,
	type StructDefinitionNode,
	type StructInstanceNode,
	type SyntaxNode,
	type TypedTreeCursor,
	type VariableDefinitionNode,
	type ReassignmentNode,
	type BinaryExpressionNode,
	type StructPropPairNode,
	type ForLoopNode,
	type UnaryExpressionNode,
	type StringNode,
	type StringContentNode,
	type StringInterpolationNode,
	type IdentifierNode,
	type WhileLoopNode,
	type FunctionDefinitionNode,
	type TypeDeclarationNode,
	type CompoundAssignmentNode,
	EnumDefinitionNode,
} from "../ast.ts";
import console from "node:console";
import {
	areCompatible,
	Bool,
	EmptyList,
	EnumType,
	FunctionType,
	getStaticTypeForPrimitiveType,
	getStaticTypeForPrimitiveValue,
	ListType,
	MapType,
	Num,
	type Signature,
	type StaticType,
	Str,
	STR_MEMBERS,
	StructType,
	Unknown,
} from "./kon-types.ts";

/*
todo: create interface for different types of diagnostics

each diagnostic can generate its message
*/
export type Diagnostic = {
	level: "error" | "warning";
	location: Point;
	message: string;
};

const RESERVED_KEYWORDS = new Set([
	"let",
	"mut",
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
	"while",
	"do",
	"Void",
	"Str",
	"Num",
	"Bool",
]);

class Variable implements StaticType {
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

	get pretty() {
		return this.static_type.pretty;
	}

	get is_iterable() {
		return this.static_type.is_iterable;
	}
}

class LexScope {
	private variables: Map<string, Variable> = new Map();
	private structs: Map<string, StructType> = new Map();
	private enums: Map<string, EnumType> = new Map();
	private functions: Map<string, FunctionType> = new Map();

	constructor(readonly parent: LexScope | null = null) {}

	addEnum(e: EnumType) {
		this.enums.set(e.name, e);
	}

	addStruct(struct: StructType) {
		this.structs.set(struct.name, struct);
	}

	addVariable(variable: Variable) {
		this.variables.set(variable.name, variable);
	}

	addFunction(fn: FunctionType) {
		this.functions.set(fn.name, fn);
	}

	getEnum(name: string): EnumType | null {
		const e = this.enums.get(name);
		if (e) return e;
		if (this.parent) return this.parent.getEnum(name);
		return null;
	}

	getStruct(name: string): StructType | null {
		const struct = this.structs.get(name);
		if (struct) return struct;
		if (this.parent) return this.parent.getStruct(name);
		return null;
	}

	getVariable(name: string): Variable | null {
		const variable = this.variables.get(name);
		if (variable) return variable;
		if (this.parent) return this.parent.getVariable(name);
		return null;
	}

	getFunction(name: string): FunctionType | null {
		return this.functions.get(name) ?? null;
	}
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

	visit(node: SyntaxNode): StaticType {
		// todo: eliminate nulls from the SyntaxNode union
		if (node === null) return Unknown;
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
		return Unknown;
	}

	private error(error: Omit<Diagnostic, "level">) {
		this.errors.push({ ...error, level: "error" });
	}

	private warn(warning: Omit<Diagnostic, "level">) {
		this.errors.push({ ...warning, level: "warning" });
	}

	scope(): LexScope {
		if (this.scopes.length === 0) {
			this.scopes.push(new LexScope());
		}
		return this.scopes.at(0)!;
	}

	visitEnumDefinition(node: EnumDefinitionNode) {
		const { nameNode, variantNodes } = node;

		const variants = variantNodes.map((n) => n.variantNode.text);
		const e_num = new EnumType(nameNode.text, variants);
		this.scope().addEnum(e_num);
	}

	visitStructDefinition(node: StructDefinitionNode) {
		const def = StructType.from(node);
		this.scope().addStruct(def);
	}

	visitStructInstance(node: StructInstanceNode): StructType | null {
		const struct_name = node.nameNode.text;

		const struct_def = this.scope().getStruct(struct_name);
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
				const provided_type =
					this.getTypeFromStructPropPairNode(inputFieldNode);
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
		const { bindingNode, nameNode, typeNode, valueNode } = node;
		this.validateIdentifier(nameNode);

		if (valueNode == null) {
			// can't really get here because tree-sitter captures a situation like this as an error
			this.error({
				message: "Variables must be initialized",
				location: node.startPosition,
			});
			return;
		}

		let declared_type = typeNode ? this.getTypeFromTypeDefNode(typeNode) : null;
		const provided_type = this.getTypeFromExpressionNode(valueNode);
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
				location: valueNode.startPosition,

				message: assigment_error,
			});
		}

		const variable = new Variable({
			name: nameNode.text,
			type: declared_type,
			is_mutable: bindingNode.text === "mut",
		});

		this.scope().addVariable(variable);
		return;
	}

	visitFunctionDefinition(node: FunctionDefinitionNode) {
		const { nameNode, parametersNode, bodyNode, returnNode } = node;
		const name = nameNode.text;
		const params = parametersNode.parameterNodes.map((n) => {
			const { nameNode, typeNode } = n;
			return {
				name: nameNode.text,
				type: this.getTypeFromTypeDefNode(typeNode),
			};
		});

		const new_scope = new LexScope(this.scope());
		for (const param of params) {
			new_scope.addVariable(
				new Variable({
					name: param.name,
					type: param.type,
				}),
			);
		}
		this.scopes.unshift(new_scope);
		this.visit(bodyNode);
		this.scopes.shift();

		const def = new FunctionType({
			name,
			params,
			return_type: this.getTypeFromTypeDefNode(returnNode),
		});
		this.scope().addFunction(def);
	}

	visitReassignment(node: ReassignmentNode) {
		const target = node.nameNode;

		const variable = this.scope().getVariable(target.text);
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

	visitCompoundAssignment(node: CompoundAssignmentNode) {
		const target = node.nameNode;

		const variable = this.scope().getVariable(target.text);
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
				location: node.operatorNode.startPosition,
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
	private getTypeFromTypeDefNode(_node: TypeDeclarationNode): StaticType {
		const node = _node.typeNode;
		switch (node.type) {
			case SyntaxType.PrimitiveType:
				return getStaticTypeForPrimitiveType(node);
			case SyntaxType.ListType: {
				switch (node.innerNode.type) {
					case SyntaxType.Identifier: {
						// check that the type exists
						const declaration = this.scope().getStruct(node.innerNode.text);
						if (declaration == null) {
							this.error({
								location: node.innerNode.startPosition,
								message: `Missing definition for type '${node.innerNode.text}'.`,
							});
							return new ListType(Unknown);
						}

						const list = new ListType(declaration);
						this.debug(`got ${list.inner.pretty}`);
						return list;
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

	visitExpressionNode(node: ExpressionNode): StaticType {
		const expr = node.exprNode;
		switch (expr.type) {
			case SyntaxType.ParenExpression: {
				return this.visitExpressionNode(expr.exprNode);
			}
			case SyntaxType.PrimitiveValue:
				return getStaticTypeForPrimitiveValue(expr);
			case SyntaxType.ListValue: {
				if (expr.innerNodes.length === 0) {
					return EmptyList;
				}
				const first = expr.innerNodes.at(0)!;
				switch (first.type) {
					case SyntaxType.Boolean:
						return new ListType(Bool);
					case SyntaxType.String:
						return new ListType(Str);
					case SyntaxType.Number:
						return new ListType(Num);
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
				return this.visitStructInstance(expr) ?? Unknown;
			}
			case SyntaxType.UnaryExpression: {
				return this.visitUnaryExpression(expr);
			}
			case SyntaxType.BinaryExpression: {
				return this.visitBinaryExpression(expr);
			}
			case SyntaxType.Identifier: {
				const variable = this.scope().getVariable(expr.text);
				if (variable == null) {
					this.error({
						message: `Cannot find name '${expr.text}'.`,
						location: expr.startPosition,
					});
					return Unknown;
				}
				return variable.static_type;
			}
			case SyntaxType.MemberAccess: {
				return this.visitMemberAccess(expr);
			}
			default: {
				return Unknown;
			}
		}
	}

	visitForLoop(node: ForLoopNode) {
		const { cursorNode, rangeNode, bodyNode } = node;
		const range = this.visitExpressionNode(rangeNode);
		if (!range.is_iterable) {
			this.error({
				message: `Cannot iterate over a '${range.pretty}'.`,
				location: rangeNode.startPosition,
			});
		}
		// infer the type of the cursor from the range
		const cursor = new Variable({
			name: cursorNode.text,
			type: range instanceof ListType ? range.inner : range,
		});
		const new_scope = new LexScope(this.scope());
		new_scope.addVariable(cursor);
		this.scopes.unshift(new_scope);
		this.visit(bodyNode);
		this.scopes.shift();
	}

	visitWhileLoop(node: WhileLoopNode) {
		const { bodyNode } = node;
		const new_scope = new LexScope(this.scope());
		this.scopes.unshift(new_scope);
		this.visit(bodyNode);
		this.scopes.shift();
	}

	private getTypeFromExpressionNode(node: ExpressionNode): StaticType {
		switch (node.exprNode.type) {
			case SyntaxType.PrimitiveValue:
				return getStaticTypeForPrimitiveValue(node.exprNode);
			case SyntaxType.ListValue: {
				if (node.exprNode.innerNodes.length === 0) {
					return EmptyList;
				}
				const first = node.exprNode.innerNodes.at(0)!;
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
				return this.visitStructInstance(node.exprNode) ?? Unknown;
			}
			default: {
				return Unknown;
			}
		}
	}

	private getTypeFromStructPropPairNode(node: StructPropPairNode): StaticType {
		switch (node.valueNode.type) {
			case SyntaxType.Boolean:
				return Bool;
			case SyntaxType.String:
				return Str;
			case SyntaxType.Number:
				return Num;
		}
	}

	visitFunctionCall(
		node: FunctionCallNode,
		parent: Variable | null = null,
	): StaticType {
		const { targetNode, argumentsNode } = node;
		const name = targetNode.text;
		if (parent === null) {
			const signature = this.scope().getFunction(name);
			if (signature == null) {
				this.error({
					location: node.startPosition,
					message: `Cannot find name '${name}'`,
				});
				return Unknown;
			}

			const args = argumentsNode.argumentNodes.map((n) =>
				this.getTypeFromExpressionNode(n),
			);
			if (args.length !== signature.parameters.length) {
				this.error({
					location: argumentsNode.startPosition,
					message: `Expected ${signature.parameters.length} arguments and got ${args.length}`,
				});
				return signature.return_type;
			}

			signature.parameters.forEach((param, index) => {
				const arg = args[index];
				if (arg && !areCompatible(param.type, arg)) {
					this.error({
						location: argumentsNode.argumentNodes[index]!.startPosition,
						message: `Argument of type '${arg.pretty}' is not assignable to parameter of type '${param.type.pretty}'`,
					});
				}
			});

			return signature.return_type;
		}

		if (parent.static_type === Str) {
			const signature = STR_MEMBERS.get(name);
			return this.checkArgumentsForCall({
				name,
				parent,
				signature,
				node,
			});
		}
		if (parent.static_type instanceof ListType) {
			const signature = parent.static_type.properties.get(name);
			return this.checkArgumentsForCall({
				name,
				parent,
				signature,
				node,
			});
		}
		// todo: map
		this.error({
			location: targetNode.startPosition,
			message: `Type '${parent.static_type.pretty} has no method ${name}`,
		});

		return Unknown;
	}

	checkArgumentsForCall(input: {
		node: FunctionCallNode;
		parent: Variable;
		name: string;
		signature?: Signature;
	}) {
		const { name, signature, parent, node } = input;
		if (signature == null) {
			this.error({
				location: node.startPosition,
				message: `Property '${name}' does not exist on type '${parent.static_type.pretty}'`,
			});
			return Unknown;
		}
		if (!signature.callable) {
			this.error({
				location: node.startPosition,
				message: `${parent.static_type.pretty}.${name} is not a callable function`,
			});
			return Unknown;
		}
		if (signature.mutates && !parent.is_mutable) {
			this.error({
				location: node.startPosition,
				message: "Cannot mutate an immutable List",
			});
			return signature.return_type;
		}

		const args = node.argumentsNode.argumentNodes.map((n) =>
			this.getTypeFromExpressionNode(n),
		);
		if (signature.parameters) {
			if (args.length !== signature.parameters.length) {
				this.error({
					location: node.argumentsNode.startPosition,
					message: `Expected ${signature.parameters.length} arguments and got ${args.length}`,
				});
				return signature.return_type ?? Unknown;
			}

			signature.parameters.forEach((param, index) => {
				const arg = args[index];
				if (arg && !areCompatible(param.type, arg)) {
					this.error({
						location: node.argumentsNode.argumentNodes[index]!.startPosition,
						message: `Argument of type '${arg.pretty}' is not assignable to parameter of type '${param.type.pretty}'`,
					});
				}
			});
		}
		return signature.return_type ?? Unknown;
	}

	visitIdentifier(node: IdentifierNode): Variable | null {
		const name = node.text;

		const variable = this.scope().getVariable(name);
		if (!variable) {
			this.error({
				location: node.startPosition,
				message: `Cannot find name '${name}'.`,
			});
			return null;
		}

		return variable;
	}

	visitMemberAccess(node: MemberAccessNode): StaticType {
		const { targetNode, memberNode } = node;
		const target = this.scope().getVariable(targetNode.text);
		if (!target) {
			this.error({
				location: targetNode.startPosition,
				message: `Cannot find name '${targetNode.text}'.`,
			});
			return Unknown;
		}
		this.debug(`target is ${target.name} of type ${target?.pretty}`);

		if (target.static_type instanceof ListType) {
			const signature = target.static_type.properties.get(memberNode.text);
			switch (memberNode.type) {
				case SyntaxType.FunctionCall: {
					return this.visitFunctionCall(memberNode, target);
				}
				case SyntaxType.Identifier: {
					if (!signature) {
						this.error({
							location: memberNode.startPosition,
							message: `Property '${memberNode.text}' does not exist on List.`,
						});
					}
					// handle signatures
					return Unknown;
				}
			}
		}
		if (target.static_type instanceof MapType) {
			const signature = target.static_type.properties.get(memberNode.text);
			switch (memberNode.type) {
				case SyntaxType.FunctionCall: {
					const member = memberNode.targetNode.text;
					if (!member) {
						return Unknown;
					}
					if (signature) {
						if (!signature.callable) {
							this.error({
								location: memberNode.startPosition,
								message: `${target.name}.${member} is not a callable function.`,
							});
							return Unknown;
						}
						if (signature.mutates && !target.is_mutable) {
							this.error({
								location: memberNode.startPosition,
								message: `Cannot mutate an immutable Map. Use 'mut' to make it mutable.`,
							});
						}
						// todo: signatures for list functions
						return Unknown;
					} else {
						this.error({
							location: memberNode.startPosition,
							message: `Unsupported member '${member}' for Map.`,
						});
						return Unknown;
					}
				}
				case SyntaxType.Identifier: {
					if (signature == null) {
						this.error({
							location: memberNode.startPosition,
							message: `Property '${memberNode.text}' does not exist on Map.`,
						});
						return Unknown;
					}
					// handle signatures
					return signature.return_type;
				}
			}
		}
		if (target.static_type === Str) {
			switch (memberNode.type) {
				case SyntaxType.FunctionCall: {
					return this.visitFunctionCall(memberNode, target);
				}
				case SyntaxType.Identifier: {
					const str_member = STR_MEMBERS.get(memberNode.text);
					if (str_member == null) {
						this.error({
							location: memberNode.startPosition,
							message: `Property '${memberNode.text}' does not exist on Str.`,
						});
					}
					// handle signatures
					return Unknown;
				}
			}
		}
		if (target.static_type === Num) {
			this.error({
				location: memberNode.startPosition,
				message: "Num has no supported properties",
			});
			return Unknown;
		}
		if (target.static_type === Bool) {
			this.error({
				location: memberNode.startPosition,
				message: "Bool has no supported properties",
			});
			return Unknown;
		}
		if (target.static_type instanceof StructType) {
			switch (memberNode.type) {
				case SyntaxType.Identifier: {
					const member_name = memberNode.text;
					const member_signature = target.static_type.fields.get(member_name);
					if (member_signature == null) {
						this.error({
							location: memberNode.startPosition,
							message: `Property '${member_name}' does not exist on type '${target.static_type.pretty}'`,
						});
						return Unknown;
					}
					return member_signature;
				}
			}
		}
		return Unknown;
	}

	visitUnaryExpression(node: UnaryExpressionNode): StaticType {
		return this.visitExpressionNode(node.operandNode);
	}

	visitBinaryExpression(node: BinaryExpressionNode): StaticType {
		this.debug("visiting left of binary expression", node.leftNode.type);
		const left = this.visitExpressionNode(node.leftNode);
		const right = this.visitExpressionNode(node.rightNode);
		this.debug(
			`BinaryExpressionNode: ${left.pretty} ${node.operatorNode.type} ${right.pretty}`,
		);
		switch (node.operatorNode.type) {
			case SyntaxType.InclusiveRange: {
				const invalidStart = left !== Num;
				const invalidEnd = right !== Num;
				if (invalidStart || invalidEnd) {
					this.error({
						location: invalidStart
							? node.leftNode.startPosition
							: node.rightNode.startPosition,
						message: "A range must be between two numbers.",
					});
				}
				return Num;
			}
			case SyntaxType.Plus: {
				const validLeft = left === Num;
				const validRight = right === Num;
				if (!(validLeft && validRight)) {
					this.error({
						location: validLeft
							? node.rightNode.startPosition
							: node.leftNode.startPosition,
						message: "Addition is only supported between numbers.",
					});
				}
				return Num;
			}
		}
		return left;
	}

	visitString(node: StringNode): StaticType {
		for (const chunkNode of node.chunkNodes) {
			this.visit(chunkNode);
		}
		return Str;
	}

	visitStringContent(_: StringContentNode): StaticType {
		return Str;
	}

	visitStringInterpolation(node: StringInterpolationNode): StaticType {
		return this.visit(node.expressionNode);
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
