import { readFileSync } from "node:fs";
import { makeParser } from "./parser/tree-sitter-parser.js";
import { generateJavascript } from "./generator/generate-javascript.ts";

function compile(input: string): string {
	return generateJavascript(makeParser().parse(input));
}

const path = Deno.args.at(0) as string;
const kon = compile(readFileSync(path, "utf8"));
eval(kon);
