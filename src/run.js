// @ts-check
const { readFileSync } = require("node:fs");
const { makeParser } = require("./parser/tree-sitter-parser");
const { generateJavascript } = require("./generator/generate-javascript");

/** @type (input: string) => string **/
function compile(input) {
	return generateJavascript(makeParser().parse(input));
}

const kon = compile(readFileSync(process.argv[2], "utf8"));
eval(kon);
