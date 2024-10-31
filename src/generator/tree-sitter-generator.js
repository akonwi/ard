// @ts-check
const Parser = require("tree-sitter");

/** @type {(tree: Parser.Tree) => string} */
function generateJavascript(tree) {
	return generateNode(tree.rootNode);
}

/** @type {(node: Parser.SyntaxNode) => string} */
function generateNode(node) {
	switch (node.type) {
		case "program": {
			return node.children.map(generateNode).join("\n");
		}
		case "statement": {
			if (node.childCount > 1)
				throw new Error(
					"Multiple statements encountered at " + node.startPosition,
				);
			if (node.firstChild == null)
				throw new Error("Empty statement at " + node.startPosition);
			let raw = generateNode(node.firstChild);
			if (
				node.firstChild.type !== "function_definition" &&
				node.firstChild.type !== "while_loop" &&
				node.firstChild.type !== "if_statement"
			)
				raw += ";";
			return raw;
		}
		case "variable_definition": {
			const binding = node.childForFieldName("binding");
			if (!binding) {
				throw new Error("Missing variable binding at " + node.startPosition);
			}
			const name = node.childForFieldName("name");
			if (!name) {
				throw new Error("Missing variable name at " + node.startPosition);
			}
			const value = node.childForFieldName("value");
			if (!value) {
				throw new Error(
					"Variable definition is missing its value at: " + node.startPosition,
				);
			}
			const letConst = binding.text === "mut" ? "let" : "const";
			return `${letConst} ${name.text} = ${generateNode(value)}`;
		}
		case "function_definition": {
			const nameNode = node.childForFieldName("name");
			const parameters = node.childForFieldName("parameters");
			const body = node.childForFieldName("body");
			if (nameNode == null)
				throw new Error("Missing function name at " + node.startPosition);
			if (parameters == null)
				throw new Error("Missing function parameters at " + node.startPosition);
			if (body == null)
				throw new Error("Missing function body at " + node.startPosition);
			return `function ${nameNode.text}${generateNode(parameters)} ${generateBlock(body, true)}`;
		}
		case "parameters": {
			return `(${node.namedChildren.map(generateNode).join(", ")})`;
		}
		case "param_def": {
			const name = node.childForFieldName("name");
			if (name == null)
				throw new Error("Missing parameter name at " + node.startPosition);
			return `${name.text}`;
		}
		case "function_call": {
			const target = node.childForFieldName("target");
			const args = node.childForFieldName("arguments");
			if (target == null)
				throw new Error(
					"Missing function name to call at" + node.startPosition,
				);
			if (args == null)
				throw new Error(
					"Missing function arguments for call at " + node.startPosition,
				);

			return `${generateNode(target)}${generateNode(args)}`;
		}
		case "paren_arguments": {
			return `(${node.namedChildren.map(generateNode).join(", ")})`;
		}
		case "while_loop": {
			const isDo = node.childForFieldName("do") != null;
			const condition = node.childForFieldName("condition");
			if (condition == null)
				throw new Error("Missing condition at " + node.startPosition);
			const body = node.childForFieldName("statement_block");
			if (body == null)
				throw new Error("Missing loop block at " + node.startPosition);
			if (isDo) {
				return `do ${generateBlock(body)} while (${generateNode(condition)});`;
			}
			return `while (${generateNode(condition)}) ${generateBlock(body)}`;
		}
		case "binary_expression": {
			const left = node.childForFieldName("left");
			const right = node.childForFieldName("right");
			const operator = node.childForFieldName("operator");
			if (left == null)
				throw new Error("Missing left operand at " + node.startPosition);
			if (right == null)
				throw new Error("Missing right operand at " + node.startPosition);
			if (operator == null)
				throw new Error("Missing operator at " + node.startPosition);
			return `${generateNode(left)} ${generateBinaryOperator(operator)} ${generateNode(right)}`;
		}
		case "compound_assignment": {
			const name = node.childForFieldName("name");
			const operator = node.childForFieldName("operator");
			const value = node.childForFieldName("value");
			if (name == null)
				throw new Error("Missing target at " + node.startPosition);
			if (operator == null)
				throw new Error("Missing operator at " + node.startPosition);
			if (value == null)
				throw new Error("Missing value at " + node.startPosition);
			return `${generateNode(name)} ${generateCompoundAssignment(operator)} ${generateNode(value)}`;
		}
		case "if_statement": {
			const condition = node.childForFieldName("condition");
			if (condition == null)
				throw new Error(
					"Missing condition for if statement at " + node.startPosition,
				);
			const body = node.childForFieldName("body");
			if (body == null)
				throw new Error(
					"Missing block for if statement at " + node.startPosition,
				);
			const elseNode = node.childForFieldName("else");
			return (
				`if (${generateNode(condition)}) ${generateBlock(body)}` +
				(elseNode ? `${generateNode(elseNode)}` : "")
			);
		}
		case "else_statement": {
			const ifStatement = node.namedChildren.find(
				(n) => n.type === "if_statement",
			);
			if (ifStatement != null) return ` else ${generateNode(ifStatement)}`;

			const body = node.childForFieldName("body");
			if (body == null)
				throw new Error(
					"Missing block for else statement at " + node.startPosition,
				);
			return ` else ${generateBlock(body)}`;
		}
		case "identifier": {
			if (node.text === "print") {
				return `console.log`;
			}
			return node.text;
		}
		case "primitive_value": {
			const child = node.firstChild;
			if (!child) {
				throw new Error(
					"Primitive value is missing its child at: " + node.startPosition,
				);
			}
			return generateNode(child);
		}
		case "number":
		case "boolean":
		case "string": {
			return node.text;
		}
		default: {
			console.log(node.type);
			return `/* Unimplemented syntax - ${node.type} */`;
		}
	}
}

/** @type {(node: Parser.SyntaxNode, isExpression?: boolean) => string} */
function generateBlock(node, isExpression = false) {
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

/** @type {(node: Parser.SyntaxNode, isExpression?: boolean) => string} */
function generateCompoundAssignment(node) {
	switch (node.grammarType) {
		case "increment":
			return "+=";
		case "decrement":
			return "-=";
		default:
			throw new Error(
				"Unknown compound assignment operator: " + node.grammarType,
			);
	}
}

/** @type {(node: Parser.SyntaxNode) => string} */
function generateBinaryOperator(node) {
	switch (node.type) {
		case "or":
			return "||";
		case "and":
			return "&&";
		default:
			return node.text;
	}
}

module.exports = { generateJavascript };
