import { type Tree } from "tree-sitter";
import {
	type BinaryExpressionNode,
	type BlockNode,
	type CompoundAssignmentNode,
	type ExpressionNode,
	type SyntaxNode,
	SyntaxType,
	type VariableDefinitionNode,
} from "../ast.ts";
import console from "node:console";

export function generateJavascript(tree: Tree) {
	return generateNode(tree.rootNode as unknown as SyntaxNode);
}

const SEMICOLON_EXCLUSIONS = new Set([
	SyntaxType.WhileLoop,
	SyntaxType.ForLoop,
	SyntaxType.FunctionDefinition,
	SyntaxType.IfStatement,
	SyntaxType.StructDefinition,
]);

function generateNode(node: SyntaxNode): string {
	if (node == null) return "";
	switch (node.type) {
		case SyntaxType.Program: {
			return node.children.map(generateNode).join("\n");
		}
		case SyntaxType.Comment: {
			return `${node.text}`;
		}
		case SyntaxType.Statement: {
			if (node.firstNamedChild == null) return "";
			const needsSemicolon = !SEMICOLON_EXCLUSIONS.has(
				node!.firstNamedChild!.type,
			);
			return generateNode(node.firstNamedChild) + (needsSemicolon ? ";" : "");
		}
		case SyntaxType.Expression: {
			return generateNode(node.exprNode);
		}
		case SyntaxType.VariableDefinition: {
			return generateVariableDefinition(node, { statement: false });
		}
		case SyntaxType.FunctionDefinition: {
			const { nameNode, parametersNode, bodyNode } = node;
			return `function ${nameNode.text}${generateNode(
				parametersNode,
			)} ${generateBlock(bodyNode, true)}`;
		}
		case SyntaxType.Parameters: {
			return `(${node.namedChildren.map(generateNode).join(", ")})`;
		}
		case SyntaxType.ParamDef: {
			return `${node.nameNode.text}`;
		}
		case SyntaxType.FunctionCall: {
			const { targetNode, argumentsNode } = node;
			return `${generateNode(targetNode)}${generateNode(argumentsNode)}`;
		}
		case SyntaxType.ParenArguments: {
			return `(${node.namedChildren.map(generateNode).join(", ")})`;
		}
		case SyntaxType.WhileLoop: {
			const { doNode, conditionNode, bodyNode } = node;
			if (doNode) {
				return `do ${generateBlock(bodyNode)} while (${generateNode(
					conditionNode,
				)});`;
			}
			return `while (${generateNode(conditionNode)}) ${generateBlock(
				bodyNode,
			)}`;
		}
		case SyntaxType.ForLoop: {
			const { cursorNode, rangeNode, bodyNode } = node;
			const cursorName = generateNode(cursorNode);
			const range = getForLoopRange(rangeNode);
			if (typeof range === "string") {
				return `for (let ${cursorName} of ${range}) ${generateBlock(bodyNode)}`;
			}
			const [start, end] = range;
			return `for (let ${cursorName} = ${start}; ${cursorName} <= ${end}; ${cursorName}++) ${generateBlock(
				bodyNode,
			)}`;
		}
		case SyntaxType.UnaryExpression: {
			const { operatorNode, operandNode } = node;
			switch (operatorNode.type) {
				case SyntaxType.Bang:
					return `!${generateNode(operandNode)}`;
				case SyntaxType.Minus:
					return `-${generateNode(operandNode)}`;
				default:
					throw new Error("Unrecognized unary operator: " + operatorNode);
			}
		}
		case SyntaxType.BinaryExpression: {
			const { leftNode, operatorNode, rightNode } = node;
			return `${generateNode(leftNode)} ${generateBinaryOperator(
				operatorNode,
			)} ${generateNode(rightNode)}`;
		}
		case SyntaxType.Reassignment: {
			const { nameNode, valueNode } = node;
			return `${generateNode(nameNode)} = ${generateNode(valueNode)}`;
		}
		case SyntaxType.CompoundAssignment: {
			const { nameNode, operatorNode, valueNode } = node;
			return `${generateNode(nameNode)} ${generateCompoundAssignmentOperator(
				operatorNode,
			)} ${generateNode(valueNode)}`;
		}
		case SyntaxType.IfStatement: {
			const { conditionNode, bodyNode, elseNode } = node;
			return (
				`if (${generateNode(conditionNode)}) ${generateBlock(bodyNode)}` +
				(elseNode ? `${generateNode(elseNode)}` : "")
			);
		}
		case SyntaxType.ElseStatement: {
			if (node.ifNode != null) return ` else ${generateNode(node.ifNode)}`;

			if (node.bodyNode == null) return "";
			return ` else ${generateBlock(node.bodyNode)}`;
		}
		case SyntaxType.PrintStatement: {
			return `console.log(${node.argumentsNode.namedChildren
				.map(generateNode)
				.join(", ")})`;
		}
		case SyntaxType.MemberAccess: {
			return `${generateNode(node.targetNode)}.${generateNode(
				node.memberNode,
			)}`;
		}
		case SyntaxType.Identifier: {
			return node.text;
		}
		case SyntaxType.StructInstance: {
			const { fieldNodes } = node;
			if (fieldNodes.length === 0) return `{}`;
			return (
				"{\n" +
				fieldNodes.map((n) => `\t${generateNode(n)}`).join(",\n") +
				"\n" +
				"}"
			);
		}
		case SyntaxType.StructPropPair: {
			const { nameNode, valueNode } = node;
			return `${nameNode.text}: ${generateNode(valueNode)}`;
		}
		case SyntaxType.ListValue: {
			return `[${node.innerNodes.map(generateNode).join(", ")}]`;
		}
		case SyntaxType.PrimitiveValue: {
			return generateNode(node.primitiveNode);
		}
		case SyntaxType.Number:
		case SyntaxType.Boolean:
		case SyntaxType.String: {
			return node.text;
		}
		case SyntaxType.StructDefinition: {
			// could add struct literal as a comment block
			return "";
		}
		default: {
			console.debug(node.type, {
				text: node.text,
			});
			return `/* Unimplemented grammar - ${node.type} */`;
		}
	}
}

function generateVariableDefinition(
	node: VariableDefinitionNode,
	options?: { statement?: boolean },
): string {
	const { bindingNode, nameNode, valueNode } = node;
	const needsSemicolon = Boolean(options?.statement);
	const letConst = bindingNode.text === "mut" ? "let" : "const";
	const raw = `${letConst} ${nameNode.text} = ${generateNode(valueNode)}`;
	if (needsSemicolon) return raw + ";";
	return raw;
}

function generateBlock(node: BlockNode, isExpression = false): string {
	if (node == null) return "{}";
	if (node.namedChildCount === 0) return "{}";
	let raw = `{\n`;
	node.namedChildren.forEach((child, index) => {
		// return the result of the last statement
		const isLast = isExpression && index === node.namedChildren.length - 1;
		raw += `\t${isLast ? "return " : ""}${generateNode(child)}\n`;
	});
	raw += "}";
	return raw;
}

function generateCompoundAssignmentOperator(
	node: CompoundAssignmentNode["operatorNode"],
): string {
	switch (node.type) {
		case SyntaxType.Increment:
			return "+=";
		case SyntaxType.Decrement:
			return "-=";
		default:
			throw new Error("Unknown compound assignment operator: " + node);
	}
}

function generateBinaryOperator(
	node: BinaryExpressionNode["operatorNode"],
): string {
	switch (node.type) {
		case SyntaxType.Or:
			return "||";
		case SyntaxType.And:
			return "&&";
		case SyntaxType.Equal:
			return "===";
		case SyntaxType.NotEqual:
			return "!==";
		default:
			return node.text;
	}
}

function getForLoopRange(node: ExpressionNode): [string, string] | string {
	const exprNode = node.exprNode;
	switch (exprNode.type) {
		case SyntaxType.BinaryExpression:
			return [
				generateNode(exprNode.leftNode),
				generateNode(exprNode.rightNode),
			];
		default:
			return generateNode(exprNode);
	}
}
