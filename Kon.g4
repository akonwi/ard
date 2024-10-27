program: declaration* EOF;

declaration: mutDecl | letDecl | statement ;
mutDecl: "mut" IDENTIFIER (":" type)? "=" expression ? (";")? ;
letDecl: "let" IDENTIFIER (":" type)? "=" expression ? (";")? ;
fnDecl: "fn" IDENTIFIER "(" ( primary "," )? ")" block ;
type ->
    | "Int"
    | "Float"
    | "Str"
    | "Bool"
    | "Void"
    | IDENTIFIER ;


statement:
    | exprStatement
    | printStatement
    | returnStatement
    | ifStatement
    | whileStatement
    | forStatement
    | block ;

block: "{" declaration* "}" ;
exprStatement: expression ";" ;
printStatement: "print" expression (";")? ;
returnStatement: "return" expression (";")? ;
ifStatement: "if" expression block ( "else" block )? ;
whileStatement: "while" expression block ;
forStatement: "for" IDENTIFIER "in" rangeExpression block ;
rangeExpression: INTEGER "..." INTEGER ;

expression → assignment | functionCall ;
functionCall: IDENTIFIER "(" ")" | IDENTIFIER "(" ( expression "," )* ")" | IDENTIFIER (expression ",")* ;
assignment: increment | decrement | IDENTIFIER "=" expression
    | logic_or ;
increment: IDENTIFIER "=+" expression ;
decrement: IDENTIFIER "=-" expression ;
logic_or: logic_and ( "or" logic_and )* ;
logic_and: equality ( "and" equality )* ;
equality: comparison (( "==" | "!=" ) comparison)* ;
comparison: term (( ">" | ">=" | "<" | "<=" ) term)* ;
term → factor (( "+" | "-") factor)* ;
factor: unary (( "*" | "/" ) unary)* ;
unary → ( "-" | "!" ) unary
    | primary ;
primary: "(" expression ")" | atom ;
atom:
		| "true" | "false"
    | INTEGER | DOUBLE | STRING
    | IDENTIFIER | list | object ;
list: "[" ( atom "," )* "]" ;
object: "{" ( IDENTIFIER ":" expression "," )* "}" ;

// Tokens
RANGE_DOTS: "." "." ".";
IDENTIFIER: [a-zA-Z_][a-zA-Z0-9_]*;
INTEGER: [0-9]+;
DOUBLE: [0-9]+ '.' [0-9]+;
STRING: '"' .*? '"';

WS: [ \t\r\n]+ -> skip;
COMMENT: '//' .*? '\n' -> skip;
