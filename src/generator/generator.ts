import type { Binary, Expr, Literal, Unary } from "../ast";

export class Generator {
	private ast: Expr | null = null;

	set input(expr: Expr) {
		this.ast = expr;
	}

	generate(): string {
		if (this.ast === null) throw new Error("No AST provided.");
		return this.generateExpr(this.ast) + ";";
	}

	private generateExpr(expr: Expr): string {
		switch (expr.type) {
			case "Binary":
				return this.generateBinary(expr);
			case "Unary":
				return this.generateUnary(expr);
			case "Grouping":
				return `(${this.generateExpr(expr.expression)})`;
			case "Literal":
				return this.generateLiteral(expr);
		}
		// @ts-expect-error - This should never happen
		throw new Error("Unknown expression type: " + expr.type);
	}

	private generateBinary(expr: Binary): string {
		return `${this.generateExpr(expr.left)} ${
			expr.operator.lexeme
		} ${this.generateExpr(expr.right)}`;
	}

	private generateUnary(expr: Unary): string {
		return `${expr.operator.lexeme}${this.generateExpr(expr.right)}`;
	}

	private generateLiteral(expr: Literal): string {
		switch (expr.token.type) {
			case "FALSE":
				return "false";
			case "TRUE":
				return "true";
			case "INTEGER":
			case "DOUBLE":
				return expr.token.lexeme;
			case "STRING":
				return `"${expr.token.lexeme}"`;
			default:
				throw new Error("Unknown literal type: " + expr.token.type);
		}
	}
}
