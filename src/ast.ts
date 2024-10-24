import type { Token } from "./lexer/lexer";

export type Literal = { type: "Literal"; token: Token; value: any };
export type Variable = { type: "Variable"; token: Token };
export type Grouping = { type: "Grouping"; expression: Expr };
export type Unary = {
	type: "Unary";
	operator: Token;
	right: Expr;
};
export type Binary = {
	type: "Binary";
	left: Expr;
	operator: Token;
	right: Expr;
};

/*
 although assigning should technically be a statement,
 i'm treating it as an expression for a few reasons:
  * for precedence
  * it's right associative like most expressions
*/
export type Assign = { type: "Assign"; name: Token; value: Expr };

export type Expr = Literal | Grouping | Unary | Binary | Variable | Assign;

export type Print = { type: "Print"; expression: Expr };
export type ExprStmt = { type: "ExprStatement"; expression: Expr };
export type MutDecl = {
	type: "MutDecl";
	name: Token;
	initializer: Expr;
};
export type Block = { type: "Block"; statements: Stmt[] };
export type If = {
	type: "If";
	condition: Expr;
	thenBranch: Stmt;
	elseBranch: Stmt | null;
};
export type Stmt = Print | ExprStmt | MutDecl | Block | If;
// | { type: "Call"; callee: Expr; paren: Token; arguments: Expr[] };

// | { type: "Let"; name: Token; initializer: Expr | null }
// | { type: "Mut"; name: Token; initializer: Expr | null }
// | { type: "If"; condition: Expr; thenBranch: Stmt; elseBranch: Stmt | null }
// | { type: "While"; condition: Expr; body: Stmt }
// | { type: "Function"; name: Token; params: Token[]; body: Stmt[] }
// | { type: "Return"; keyword: Token; value: Expr | null };
