import type { Expr } from "../ast";
import { Lexer } from "../lexer/lexer";
import { Parser } from "./parser";

export function print(expression: Expr): string {
	switch (expression.type) {
		case "Binary": {
			return `(${expression.operator.lexeme} ${print(expression.right)} ${print(
				expression.left,
			)})`;
		}
		case "Unary": {
			return `(${expression.operator.lexeme} ${print(expression.right)})`;
		}
		case "Grouping": {
			return `(group ${print(expression.expression)})`;
		}
		case "Literal": {
			return `${expression.token.lexeme}`;
		}
	}
}

// const input = "1 + 2 + (2 * 3) - 4";
// console.log("input =>", input);
// const ast = new Parser(new Lexer(input).tokenize()).parse();
// console.log("AST", ast);
// if (ast) console.log(print(ast));
