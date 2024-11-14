import { compile } from "./compile.ts";

const path = Deno.args.at(0) as string;
const kon = compile(Deno.readTextFileSync(path));
eval(kon);
