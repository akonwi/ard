const Parser = require("tree-sitter");
const Kon = require("../../../tree-sitter-kon/bindings/node");

module.exports = {
	makeParser: () => {
		const parser = new Parser();
		parser.setLanguage(Kon);
		return parser;
	},
};
