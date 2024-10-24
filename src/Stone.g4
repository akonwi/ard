grammar Stone;

program -> declaration* EOF;

declaration -> mutDecl | statement;
mutDecl -> "mut" IDENTIFIER ( "=" expression )? ";";
statement -> exprStatement | printStatement | block ;
block -> "{" declaration* "}" ;
exprStatement -> expression ";" ;
printStatement -> "print" expression ";" ;
ifStatement -> "if" "(" expression ")" statement ( "else" statement )? ;

expression → assignment ;
assignment -> IDENTIFIER "=" expression
    | logic_or ;
logic_or -> logic_and ( "or" logic_and )* ;
logic_and -> equality ( "and" equality )* ;
equality -> comparison (( "==" | "!=" ) comparison)* ;
comparison -> term (( ">" | ">=" | "<" | "<=" ) term)* ;
term → factor (( "+" | "-") factor)* ;
factor: unary (( "*" | "/" ) unary)* ;
unary → ( "-" | "!" ) unary
    | primary ;
primary -> "true" | "false" |
    | NUMBER | STRING |
    "(" expression ")"
    | IDENTIFIER ;


// Parser Rules FROM claude
program: statement* EOF;

statement:
	letDeclaration
	| mutDeclaration
	| functionDeclaration
	| expressionStatement
	| ifStatement
	| whileStatement
	| forStatement
	| returnStatement
	| block;

letDeclaration: 'let' IDENTIFIER ('=' expression)?;
mutDeclaration: 'var' IDENTIFIER ('=' expression)?;

functionDeclaration:
	'func' IDENTIFIER '(' parameterList? ')' ('->' type)? block;

parameterList: parameter (',' parameter)*;
parameter: IDENTIFIER ':' type;

type: 'Int' | 'Double' | 'String' | 'Boolean' | 'Void';

expressionStatement: expression;

ifStatement:
	'if' expression block ('else' (ifStatement | block))?;

whileStatement: 'while' expression block;

forStatement:
	'for' '(' (
		letDeclaration
		| mutDeclaration
		| expressionStatement
	)? ';' expression? ';' expression? ')' block;

returnStatement: 'return' expression?;

block: '{' statement* '}';

expression: assignmentExpression;

assignmentExpression:
	logicalOrExpression ('=' assignmentExpression)?;

logicalOrExpression:
	logicalAndExpression ('||' logicalAndExpression)*;

logicalAndExpression:
	equalityExpression ('&&' equalityExpression)*;

equalityExpression:
	comparisonExpression (('==' | '!=') comparisonExpression)*;

comparisonExpression:
	additiveExpression (
		('<' | '>' | '<=' | '>=') additiveExpression
	)*;

additiveExpression:
	multiplicativeExpression (
		('+' | '-') multiplicativeExpression
	)*;

multiplicativeExpression:
	unaryExpression (('*' | '/') unaryExpression)*;

unaryExpression: ('-' | '!') unaryExpression | callExpression;

callExpression: primaryExpression ('(' argumentList? ')')*;

argumentList: expression (',' expression)*;

primaryExpression:
	IDENTIFIER
	| INTEGER
	| DOUBLE
	| STRING
	| 'true'
	| 'false'
	| '(' expression ')';

// Lexer Rules
IDENTIFIER: [a-zA-Z_][a-zA-Z0-9_]*;
INTEGER: [0-9]+;
DOUBLE: [0-9]+ '.' [0-9]+;
STRING: '"' .*? '"';

WS: [ \t\r\n]+ -> skip;
COMMENT: '//' .*? '\n' -> skip;
