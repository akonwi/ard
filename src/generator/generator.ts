import type { Binary, Expr, Literal, Stmt, Unary } from "../ast";

export class Generator {
	private statements: Stmt[] = [];

	set input(statements: Stmt[]) {
		this.statements = statements;
	}

	generate(): string {
		if (this.statements === null) throw new Error("No AST provided.");
		return this.statements.map((stmt) => this.generateStmt(stmt)).join("\n");
	}

	private generateStmt(stmt: Stmt): string {
		switch (stmt.type) {
			case "ExprStatement":
				return this.generateExpr(stmt.expression) + ";";
			// Add more cases for other statement types as needed
			case "Print":
				return `console.log(${this.generateExpr(stmt.expression)});`;
			default:
				// @ts-expect-error - This should never happen
				throw new Error("Unknown statement type: " + stmt.type);
		}
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
