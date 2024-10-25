import type { Binary, Expr, Literal, Stmt, Unary } from "../ast";

export class Generator {
	private statements: Stmt[] = [];
	private indent = "";

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
				return this.indent + this.generateExpr(stmt.expression) + ";";
			case "Print":
				return (
					this.indent + `console.log(${this.generateExpr(stmt.expression)});`
				);
			case "MutDecl":
				return (
					this.indent +
					`let ${stmt.name.lexeme} = ${this.generateExpr(stmt.initializer)};`
				);
			case "LetDecl":
				return (
					this.indent +
					`const ${stmt.name.lexeme} = ${this.generateExpr(stmt.initializer)};`
				);
			case "Block":
				const prevIndent = this.indent;
				this.indent += "\t";
				const block = stmt.statements
					.map((s) => this.generateStmt(s))
					.join("\n");
				this.indent = prevIndent;
				return `{\n${this.indent}${block}\n}`;
			case "If":
				const ifStmt = `${this.indent}if (${this.generateExpr(
					stmt.condition,
				)}) ${this.generateStmt(stmt.thenBranch)}`;

				if (stmt.elseBranch) {
					return ifStmt + ` else ${this.generateStmt(stmt.elseBranch)}`;
				}
				return ifStmt;
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
			case "Assign":
				return `${expr.name.lexeme} = ${this.generateExpr(expr.value)}`;
			case "Variable":
				return expr.token.lexeme;
			case "Logical":
				const operator = expr.operator.lexeme === "or" ? "||" : "&&";
				return `${this.generateExpr(expr.left)} ${operator} ${this.generateExpr(
					expr.right,
				)}`;
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
