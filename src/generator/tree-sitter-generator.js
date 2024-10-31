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
			return node.children.map(generateNode).join("\n");
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
			return `${letConst} ${name.text} = ${generateNode(value)};`;
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
