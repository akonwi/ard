import { makeParser } from "./parser/parser.ts";
import { generateJavascript } from "./generator/generate-javascript.ts";
import { Checker } from "./check/check.ts";
import console from "node:console";

function compile(input: string): string {
	const tree = makeParser().parse(input);
	const diagnostics = new Checker(tree).check();
	if (diagnostics.length > 0) {
		for (const diagnostic of diagnostics) {
			console.error(
				`${diagnostic.level}: [${diagnostic.location.row}:${diagnostic.location.column}] ${diagnostic.message}`,
			);
		}
		Deno.exit(1);
	}
	return generateJavascript(tree);
}

const path = Deno.args.at(0) as string;
const kon = compile(Deno.readTextFileSync(path));
eval(kon);
