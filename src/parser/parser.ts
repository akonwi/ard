import type {
	Block,
	Expr,
	ListLiteral,
	Literal,
	Print,
	RangeExpr,
	Stmt,
	Tangible,
	Variable,
} from "../ast";
import { TokenType, type Token } from "../lexer/lexer";

class ParseError extends Error {}

export class Parser {
	private current = 0;

	constructor(private tokens: Token[]) {}

	parse(): Stmt[] {
		const statements: Stmt[] = [];
		while (!this.isAtEnd()) {
			statements.push(this.declaration());
		}
		return statements;
	}

	private debug(label?: string) {
		console.log(label, {
			current: this.peek(),
			previous: this.tokens[this.current - 1],
			next: this.tokens[this.current + 1],
		});
	}

	private expression(): Expr {
		const expr = this.assignment();

		if (this.match(TokenType.LEFT_PAREN)) {
			return this.call(expr);
		}

		return expr;
	}

	private call(callee: Expr): Expr {
		if (this.match(TokenType.RIGHT_PAREN)) {
			return {
				type: "Call",
				callee,
				arguments: [],
			};
		}

		const args: Expr[] = [];
		do {
			args.push(this.expression());
		} while (this.match(TokenType.COMMA));
		this.consume(TokenType.RIGHT_PAREN, "Expect ')' to end function call.");

		return {
			type: "Call",
			callee,
			arguments: args,
		};
	}

	private assignment(): Expr {
		const expr = this.or();

		if (this.match(TokenType.INCREMENT)) {
			const plus = this.previous();
			const value = this.assignment();
			if (expr.type === "Variable") {
				return { type: "Increment", name: expr.token, value };
			}
			this.error(plus, "Invalid assignment target.");
		}
		if (this.match(TokenType.DECREMENT)) {
			const plus = this.previous();
			const value = this.assignment();
			if (expr.type === "Variable") {
				return { type: "Decrement", name: expr.token, value };
			}
			this.error(plus, "Invalid assignment target.");
		}

		if (this.match(TokenType.ASSIGN)) {
			const equals = this.previous();
			const value = this.assignment();
			if (expr.type === "Variable") {
				return { type: "Assign", name: expr.token, value };
			}
			this.error(equals, "Invalid assignment target.");
		}
		return expr;
	}

	private equality(): Expr {
		let expr = this.comparison();
		while (this.match(TokenType.NOT_EQUAL, TokenType.EQUAL)) {
			const operator = this.previous();
			const right = this.comparison();
			expr = { type: "Binary", left: expr, operator, right };
		}
		return expr;
	}

	private comparison(): Expr {
		let expr = this.term();
		while (
			this.match(
				TokenType.GREATER_THAN,
				TokenType.GREATER_EQUAL,
				TokenType.LESS_THAN,
				TokenType.LESS_EQUAL,
			)
		) {
			const operator = this.previous();
			const right = this.term();
			expr = { type: "Binary", left: expr, operator, right };
		}
		return expr;
	}

	private term(): Expr {
		let expr = this.factor();
		while (this.match(TokenType.MINUS, TokenType.PLUS)) {
			const operator = this.previous();
			const right = this.factor();
			expr = { type: "Binary", left: expr, operator, right };
		}
		return expr;
	}

	private factor(): Expr {
		let expr = this.unary();
		while (this.match(TokenType.SLASH, TokenType.STAR)) {
			const operator = this.previous();
			const right = this.unary();
			expr = { type: "Binary", left: expr, operator, right };
		}
		return expr;
	}

	private unary(): Expr {
		if (this.match(TokenType.BANG, TokenType.MINUS)) {
			const operator = this.previous();
			const right = this.unary();
			return { type: "Unary", operator, right };
		}
		return this.primary();
	}

	private declaration(): Stmt {
		try {
			if (this.match(TokenType.MUT)) return this.mutDeclaration();
			if (this.match(TokenType.LET)) return this.letDeclaration();
			if (this.match(TokenType.FUNC)) return this.function("function");
			if (this.match(TokenType.BLANK_LINE)) return { type: "BlankLine" };
			return this.statement();
		} catch (error) {
			this.synchronize();
			throw error;
		}
	}

	private statement(): Stmt {
		if (this.match(TokenType.PRINT)) return this.printStatement();
		if (this.match(TokenType.LEFT_BRACE)) return this.block();
		if (this.match(TokenType.IF)) return this.ifStatement();
		if (this.match(TokenType.WHILE)) return this.whileStatement();
		if (this.match(TokenType.FOR)) return this.forStatement();
		if (this.match(TokenType.RETURN)) return this.returnStatement();
		return this.expressionStatement();
		// TODO: immediately evaluated blocks
		// 	return { type: "Block", statements: this.block() };
	}

	private block(): Block {
		const statements: Stmt[] = [];
		while (!this.check(TokenType.RIGHT_BRACE) && !this.isAtEnd()) {
			statements.push(this.declaration());
		}
		this.consume(TokenType.RIGHT_BRACE, "Expect '}' after block.");
		return { type: "Block", statements };
	}

	private printStatement(): Print {
		const expression = this.expression();
		this.match(TokenType.SEMICOLON);
		return { type: "Print", expression };
	}

	private expressionStatement(): Stmt {
		const expr = this.expression();
		this.match(TokenType.SEMICOLON);
		return { type: "ExprStatement", expression: expr };
	}

	private letDeclaration(): Stmt {
		const name = this.consume(TokenType.IDENTIFIER, "Expect variable name.");
		let initializer: Expr | null = null;
		let variableType: Token | null = null;
		if (this.match(TokenType.COLON)) {
			variableType = this.consume(
				TokenType.IDENTIFIER,
				"Expect type after ':'.",
			);
		}
		if (this.match(TokenType.ASSIGN)) {
			initializer = this.expression();
			// semi-colon is optional
			if (this.match(TokenType.SEMICOLON)) {
			}
			return { type: "LetDecl", name, initializer, _staticType: variableType };
		}
		throw this.error(this.peek(), "Expect variable initializer.");
	}

	private mutDeclaration(): Stmt {
		const name = this.consume(TokenType.IDENTIFIER, "Expect variable name.");
		let initializer: Expr | null = null;
		let variableType: Token | null = null;
		if (this.match(TokenType.COLON)) {
			variableType = this.consume(
				TokenType.IDENTIFIER,
				"Expect type after ':'.",
			);
		}
		if (this.match(TokenType.ASSIGN)) {
			initializer = this.expression();
			// semi-colon is optional
			if (this.match(TokenType.SEMICOLON)) {
			}
			return { type: "MutDecl", name, initializer, _staticType: variableType };
		} else {
			throw this.error(this.peek(), "Expect variable initializer.");
		}
	}

	private ifStatement(): Stmt {
		const condition = this.expression();
		this.consume(TokenType.LEFT_BRACE, "Expect '{' after if condition.");
		const thenBranch: Stmt = this.block();
		let elseBranch: Stmt | null = null;
		if (this.match(TokenType.ELSE)) {
			this.consume(TokenType.LEFT_BRACE, "Expect '{' after else condition.");
			elseBranch = this.block();
		}
		return { type: "If", condition, thenBranch, elseBranch };
	}

	private whileStatement(): Stmt {
		const condition = this.expression();
		this.consume(TokenType.LEFT_BRACE, "Expect '{' after while condition.");
		const body = this.block();
		return { type: "While", condition, body };
	}

	private forStatement(): Stmt {
		const cursor = this.consume(
			TokenType.IDENTIFIER,
			"Expect cursor name after 'for'.",
		);
		this.consume(TokenType.IN, "Expect 'in' after cursor name.");
		const range = this.rangeExpression();
		this.consume(TokenType.LEFT_BRACE, "Expect '{' after 'for range'.");
		const body = this.block();
		return {
			type: "ForIn",
			cursor,
			range,
			body,
		};
	}

	private rangeExpression(): RangeExpr {
		const start = this.consume(
			TokenType.INTEGER,
			"Expect integer range after 'for'.",
		);
		this.consume(TokenType.RANGE_DOTS, "Expect integer range after 'for'.");
		const end = this.consume(
			TokenType.INTEGER,
			"Expect integer range after 'for'.",
		);
		return { type: "RangeExpr", start, end };
	}

	private function(kind: string): Stmt {
		const name = this.consume(TokenType.IDENTIFIER, `Expect ${kind} name.`);
		this.consume(TokenType.LEFT_PAREN, `Expect '(' after ${kind} name.`);
		const parameters: Token[] = [];
		if (!this.check(TokenType.RIGHT_PAREN)) {
			do {
				if (parameters.length >= 255) {
					this.error(this.peek(), "Can't have more than 255 parameters.");
				}
				parameters.push(
					this.consume(TokenType.IDENTIFIER, "Expect parameter name."),
				);
			} while (this.match(TokenType.COMMA));
		}
		this.consume(TokenType.RIGHT_PAREN, "Expect ')' after parameters.");
		this.consume(TokenType.LEFT_BRACE, `Expect '{' before ${kind} body.`);
		const body = this.block();
		return { type: "Function", name, params: parameters, body };
	}

	private returnStatement(): Stmt {
		const keyword = this.previous();
		let value = null;
		if (
			!this.check(TokenType.SEMICOLON) &&
			!this.check(TokenType.RIGHT_BRACE)
		) {
			value = this.expression();
		}
		return { type: "Return", keyword, value };
	}

	private or(): Expr {
		let expr = this.and();
		while (this.match(TokenType.OR)) {
			const operator = this.previous();
			const right = this.and();
			expr = { type: "Logical", left: expr, operator, right };
		}
		return expr;
	}

	private and(): Expr {
		let expr = this.equality();
		while (this.match(TokenType.AND)) {
			const operator = this.previous();
			const right = this.equality();
			expr = { type: "Logical", left: expr, operator, right };
		}
		return expr;
	}

	private primary(): Expr {
		if (this.match(TokenType.LEFT_PAREN)) {
			const expr = this.expression();
			this.consume(TokenType.RIGHT_PAREN, "Expect ')' after expression.");
			return { type: "Grouping", expression: expr };
		}
		return this.atom();
	}

	private atom(): Tangible {
		if (this.match(TokenType.FALSE)) {
			return { type: "Literal", value: false, token: this.previous() };
		}
		if (this.match(TokenType.TRUE)) {
			return { type: "Literal", value: true, token: this.previous() };
		}
		if (this.match(TokenType.INTEGER)) {
			const token = this.previous();
			return { type: "Literal", value: Number(token.lexeme), token };
		}
		// memo: this distinction probably doesn't matter in a JS runtime
		if (this.match(TokenType.DOUBLE)) {
			const token = this.previous();
			return { type: "Literal", value: parseFloat(token.lexeme), token };
		}
		if (this.match(TokenType.STRING)) {
			const token = this.previous();
			return {
				type: "Literal",
				value: token.lexeme,
				token,
			};
		}
		if (this.match(TokenType.IDENTIFIER)) {
			return { type: "Variable", token: this.previous() };
		}
		return this.list();
	}

	private list(): ListLiteral {
		this.consume(TokenType.LEFT_BRACKET, "Expect a '['.");
		let items: Tangible[] = [];
		if (!this.check(TokenType.RIGHT_BRACKET)) {
			do {
				items.push(this.atom());
			} while (this.match(TokenType.COMMA));
		}
		this.consume(TokenType.RIGHT_BRACKET, "Expect a ']' to close the list.");
		return { type: "List", items };
	}

	private match(...types: TokenType[]): boolean {
		for (const type of types) {
			if (this.check(type)) {
				this.advance();
				return true;
			}
		}
		return false;
	}

	private consume(type: TokenType, message: string): Token {
		if (this.check(type)) return this.advance();
		throw this.error(this.peek(), message);
	}

	private check(type: TokenType): boolean {
		if (this.isAtEnd()) return false;
		return this.peek().type === type;
	}

	private advance(): Token {
		if (!this.isAtEnd()) this.current++;
		return this.previous();
	}

	private isAtEnd(): boolean {
		return this.peek().type === TokenType.EOF;
	}

	private peek(): Token {
		const token = this.tokens[this.current];
		if (!token) throw new ParseError();
		return token;
	}

	private previous(): Token {
		const token = this.tokens[this.current - 1];
		if (!token) throw new ParseError();
		return token;
	}

	private error(token: Token, message: string): ParseError {
		// if (token.type == TokenType.EOF) {
		// 	console.log(`Syntax error on line ${token.line}, at end: ${message}`);
		// } else {
		// 	console.log(
		// 		`Syntax error on line ${token.line}, at ${token.lexeme}: ${message}`,
		// 	);
		// }
		return new ParseError(
			message + " at " + token.line + ":" + token.column + ".",
		);
	}

	private synchronize() {
		this.advance();
		while (!this.isAtEnd()) {
			// explicit semicolons may be an explicit statement boundary
			if (this.previous().type === TokenType.SEMICOLON) return;
			switch (this.peek().type) {
				case TokenType.FUNC:
				case TokenType.LET:
				case TokenType.MUT:
				case TokenType.FOR:
				case TokenType.IF:
				case TokenType.WHILE:
				case TokenType.RETURN:
					return;
			}
			this.advance();
		}
	}

	// private finishCall(callee: Expr): Expr {
	// 	const args: Expr[] = [];
	// 	if (!this.check(TokenType.RIGHT_PAREN)) {
	// 		do {
	// 			if (args.length >= 255) {
	// 				this.error(this.peek(), "Can't have more than 255 arguments.");
	// 			}
	// 			args.push(this.expression());
	// 		} while (this.match(TokenType.COMMA));
	// 	}
	// 	const paren = this.consume(
	// 		TokenType.RIGHT_PAREN,
	// 		"Expect ')' after arguments.",
	// 	);
	// 	return { type: "Call", callee, paren, arguments: args };
	// }
}
