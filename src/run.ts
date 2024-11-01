import { makeParser } from "./parser/parser.ts";
import { generateJavascript } from "./generator/generate-javascript.ts";

function compile(input: string): string {
	return generateJavascript(makeParser().parse(input));
}

const path = Deno.args.at(0) as string;
const kon = compile(Deno.readTextFileSync(path));
eval(kon);
