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
			if (node.firstChild.type !== "function_definition") raw += ";";
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
			return `function ${nameNode.text}${generateNode(parameters)} ${generateNode(body)}`;
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
		case "block": {
			if (node.namedChildCount === 0) return "{}";
			let raw = `{\n`;
			node.namedChildren.forEach((child, index) => {
				// return the result of the last statement
				const isLast = index === node.namedChildren.length - 1;
				raw += `\t${isLast ? "return " : ""}${generateNode(child)}\n`;
			});
			raw += "}";
			return raw;
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
		case "identifier": {
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

module.exports = { generateJavascript };
