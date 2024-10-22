// AST.ts

import type { Token } from "./lexer/lexer";

type Literal = { type: "Literal"; token: Token; value: any };
type Grouping = { type: "Grouping"; expression: Expr };
type Unary = {
	type: "Unary";
	operator: Token;
	right: Expr;
};
type Binary = {
	type: "Binary";
	left: Expr;
	operator: Token;
	right: Expr;
};

export type Expr = Literal | Grouping | Unary | Binary;
// | { type: "Variable"; name: Token }
// | { type: "Assign"; name: Token; value: Expr }
// | { type: "Call"; callee: Expr; paren: Token; arguments: Expr[] };

export type Stmt =
	| { type: "Expression"; expression: Expr }
	| { type: "Let"; name: Token; initializer: Expr | null }
	| { type: "Mut"; name: Token; initializer: Expr | null }
	| { type: "Block"; statements: Stmt[] }
	| { type: "If"; condition: Expr; thenBranch: Stmt; elseBranch: Stmt | null }
	| { type: "While"; condition: Expr; body: Stmt }
	| { type: "Function"; name: Token; params: Token[]; body: Stmt[] }
	| { type: "Return"; keyword: Token; value: Expr | null };
