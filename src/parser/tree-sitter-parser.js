import Parser from "tree-sitter";
import Kon from "../../../tree-sitter-kon/bindings/node/index.cjs";

export const makeParser = () => {
	const parser = new Parser();
	parser.setLanguage(Kon);
	return parser;
};
