import type { Point, Tree, TreeCursor } from "tree-sitter";

interface SyntaxNodeBase {
		tree: Tree;
		type: string;
		isNamed: boolean;
		text: string;
		startPosition: Point;
		endPosition: Point;
		startIndex: number;
		endIndex: number;
		parent: SyntaxNode | null;
		children: Array<SyntaxNode>;
		namedChildren: Array<SyntaxNode>;
		childCount: number;
		namedChildCount: number;
		firstChild: SyntaxNode | null;
		firstNamedChild: SyntaxNode | null;
		lastChild: SyntaxNode | null;
		lastNamedChild: SyntaxNode | null;
		nextSibling: SyntaxNode | null;
		nextNamedSibling: SyntaxNode | null;
		previousSibling: SyntaxNode | null;
		previousNamedSibling: SyntaxNode | null;

		hasChanges(): boolean;
		hasError(): boolean;
		isMissing(): boolean;
		toString(): string;
		child(index: number): SyntaxNode | null;
		namedChild(index: number): SyntaxNode | null;
		firstChildForIndex(index: number): SyntaxNode | null;
		firstNamedChildForIndex(index: number): SyntaxNode | null;

		descendantForIndex(index: number): SyntaxNode;
		descendantForIndex(startIndex: number, endIndex: number): SyntaxNode;
		namedDescendantForIndex(index: number): SyntaxNode;
		namedDescendantForIndex(startIndex: number, endIndex: number): SyntaxNode;
		descendantForPosition(position: Point): SyntaxNode;
		descendantForPosition(startPosition: Point, endPosition: Point): SyntaxNode;
		namedDescendantForPosition(position: Point): SyntaxNode;
		namedDescendantForPosition(
			startPosition: Point,
			endPosition: Point,
		): SyntaxNode;
		descendantsOfType<T extends TypeString>(
			types: T | readonly T[],
			startPosition?: Point,
			endPosition?: Point,
		): NodeOfType<T>[];

		closest<T extends SyntaxType>(types: T | readonly T[]): NamedNode<T> | null;
		walk(): TreeCursor;
}

interface NamedNodeBase extends SyntaxNodeBase {
    isNamed: true;
}

/** An unnamed node with the given type string. */
export interface UnnamedNode<T extends string = string> extends SyntaxNodeBase {
  type: T;
  isNamed: false;
}

type PickNamedType<Node, T extends string> = Node extends { type: T; isNamed: true } ? Node : never;

type PickType<Node, T extends string> = Node extends { type: T } ? Node : never;

/** A named node with the given `type` string. */
export type NamedNode<T extends SyntaxType = SyntaxType> = PickNamedType<SyntaxNode, T>;

/**
 * A node with the given `type` string.
 *
 * Note that this matches both named and unnamed nodes. Use `NamedNode<T>` to pick only named nodes.
 */
export type NodeOfType<T extends string> = PickType<SyntaxNode, T>;

interface TreeCursorOfType<S extends string, T extends SyntaxNodeBase> {
  nodeType: S;
  currentNode: T;
}

type TreeCursorRecord = { [K in TypeString]: TreeCursorOfType<K, NodeOfType<K>> };

/**
 * A tree cursor whose `nodeType` correlates with `currentNode`.
 *
 * The typing becomes invalid once the underlying cursor is mutated.
 *
 * The intention is to cast a `TreeCursor` to `TypedTreeCursor` before
 * switching on `nodeType`.
 *
 * For example:
 * ```ts
 * let cursor = root.walk();
 * while (cursor.gotoNextSibling()) {
 *   const c = cursor as TypedTreeCursor;
 *   switch (c.nodeType) {
 *     case SyntaxType.Foo: {
 *       let node = c.currentNode; // Typed as FooNode.
 *       break;
 *     }
 *   }
 * }
 * ```
 */
export type TypedTreeCursor = TreeCursorRecord[keyof TreeCursorRecord];

export interface ErrorNode extends NamedNodeBase {
    type: SyntaxType.ERROR;
    hasError(): true;
}

// This is a helper because expression is a hidden rule in the grammar
export type ExpressionNode =
	| BinaryExpressionNode
	| UnaryExpressionNode
	| PrimitiveValueNode
	| FunctionCallNode
	| IdentifierNode
	| MemberAccessNode
	| StructInstanceNode;

export const enum SyntaxType {
  ERROR = "ERROR",
  BinaryExpression = "binary_expression",
  Block = "block",
  Boolean = "boolean",
  CompoundAssignment = "compound_assignment",
  ElseStatement = "else_statement",
  EnumDefinition = "enum_definition",
  EnumStructVariant = "enum_struct_variant",
  EnumVariant = "enum_variant",
  ForLoop = "for_loop",
  FunctionCall = "function_call",
  FunctionDefinition = "function_definition",
  IfStatement = "if_statement",
  ListType = "list_type",
  ListValue = "list_value",
  MapPair = "map_pair",
  MapType = "map_type",
  MapValue = "map_value",
  MemberAccess = "member_access",
  ParamDef = "param_def",
  Parameters = "parameters",
  ParenArguments = "paren_arguments",
  PrimitiveType = "primitive_type",
  PrimitiveValue = "primitive_value",
  Program = "program",
  Reassignment = "reassignment",
  ReturnType = "return_type",
  Statement = "statement",
  String = "string",
  StructDefinition = "struct_definition",
  StructInstance = "struct_instance",
  StructPropPair = "struct_prop_pair",
  StructProperty = "struct_property",
  TypeDeclaration = "type_declaration",
  UnaryExpression = "unary_expression",
  VariableBinding = "variable_binding",
  VariableDefinition = "variable_definition",
  WhileLoop = "while_loop",
  And = "and",
  Bang = "bang",
  Decrement = "decrement",
  Divide = "divide",
  Equal = "equal",
  GreaterThan = "greater_than",
  GreaterThanOrEqual = "greater_than_or_equal",
  Identifier = "identifier",
  InclusiveRange = "inclusive_range",
  Increment = "increment",
  LessThan = "less_than",
  LessThanOrEqual = "less_than_or_equal",
  Minus = "minus",
  Modulo = "modulo",
  Multiply = "multiply",
  NotEqual = "not_equal",
  Number = "number",
  Or = "or",
  Plus = "plus",
}

export type UnnamedType =
  | "\""
  | "("
  | ")"
  | ","
  | "."
  | ":"
  | "="
  | "Bool"
  | "Num"
  | "Str"
  | "["
  | "]"
  | "do"
  | "else"
  | "enum"
  | "false"
  | "fn"
  | "for"
  | "if"
  | "in"
  | "let"
  | "mut"
  | "struct"
  | "true"
  | "while"
  | "{"
  | "}"
  ;

export type TypeString = SyntaxType | UnnamedType;

export type SyntaxNode = 
  | BinaryExpressionNode
  | BlockNode
  | BooleanNode
  | CompoundAssignmentNode
  | ElseStatementNode
  | EnumDefinitionNode
  | EnumStructVariantNode
  | EnumVariantNode
  | ForLoopNode
  | FunctionCallNode
  | FunctionDefinitionNode
  | IfStatementNode
  | ListTypeNode
  | ListValueNode
  | MapPairNode
  | MapTypeNode
  | MapValueNode
  | MemberAccessNode
  | ParamDefNode
  | ParametersNode
  | ParenArgumentsNode
  | PrimitiveTypeNode
  | PrimitiveValueNode
  | ProgramNode
  | ReassignmentNode
  | ReturnTypeNode
  | StatementNode
  | StringNode
  | StructDefinitionNode
  | StructInstanceNode
  | StructPropPairNode
  | StructPropertyNode
  | TypeDeclarationNode
  | UnaryExpressionNode
  | VariableBindingNode
  | VariableDefinitionNode
  | WhileLoopNode
  | UnnamedNode<"\"">
  | UnnamedNode<"(">
  | UnnamedNode<")">
  | UnnamedNode<",">
  | UnnamedNode<".">
  | UnnamedNode<":">
  | UnnamedNode<"=">
  | UnnamedNode<"Bool">
  | UnnamedNode<"Num">
  | UnnamedNode<"Str">
  | UnnamedNode<"[">
  | UnnamedNode<"]">
  | AndNode
  | BangNode
  | DecrementNode
  | DivideNode
  | UnnamedNode<"do">
  | UnnamedNode<"else">
  | UnnamedNode<"enum">
  | EqualNode
  | UnnamedNode<"false">
  | UnnamedNode<"fn">
  | UnnamedNode<"for">
  | GreaterThanNode
  | GreaterThanOrEqualNode
  | IdentifierNode
  | UnnamedNode<"if">
  | UnnamedNode<"in">
  | InclusiveRangeNode
  | IncrementNode
  | LessThanNode
  | LessThanOrEqualNode
  | UnnamedNode<"let">
  | MinusNode
  | ModuloNode
  | MultiplyNode
  | UnnamedNode<"mut">
  | NotEqualNode
  | NumberNode
  | OrNode
  | PlusNode
  | UnnamedNode<"struct">
  | UnnamedNode<"true">
  | UnnamedNode<"while">
  | UnnamedNode<"{">
  | UnnamedNode<"}">
  | ErrorNode
  ;

export interface BinaryExpressionNode extends NamedNodeBase {
  type: SyntaxType.BinaryExpression;
  leftNodes: (UnnamedNode<"("> | UnnamedNode<")"> | BinaryExpressionNode | FunctionCallNode | IdentifierNode | MemberAccessNode | PrimitiveValueNode | StructInstanceNode | UnaryExpressionNode)[];
  operatorNode: AndNode | BangNode | DivideNode | EqualNode | GreaterThanNode | GreaterThanOrEqualNode | InclusiveRangeNode | LessThanNode | LessThanOrEqualNode | MinusNode | ModuloNode | MultiplyNode | NotEqualNode | OrNode | PlusNode;
  rightNodes: (UnnamedNode<"("> | UnnamedNode<")"> | BinaryExpressionNode | FunctionCallNode | IdentifierNode | MemberAccessNode | PrimitiveValueNode | StructInstanceNode | UnaryExpressionNode)[];
}

export interface BlockNode extends NamedNodeBase {
  type: SyntaxType.Block;
}

export interface BooleanNode extends NamedNodeBase {
  type: SyntaxType.Boolean;
}

export interface CompoundAssignmentNode extends NamedNodeBase {
  type: SyntaxType.CompoundAssignment;
  nameNode: IdentifierNode;
  operatorNode: DecrementNode | IncrementNode;
  valueNodes: (UnnamedNode<"("> | UnnamedNode<")"> | BinaryExpressionNode | FunctionCallNode | IdentifierNode | MemberAccessNode | PrimitiveValueNode | StructInstanceNode | UnaryExpressionNode)[];
}

export interface ElseStatementNode extends NamedNodeBase {
  type: SyntaxType.ElseStatement;
  bodyNode?: BlockNode;
}

export interface EnumDefinitionNode extends NamedNodeBase {
  type: SyntaxType.EnumDefinition;
  nameNode: IdentifierNode;
}

export interface EnumStructVariantNode extends NamedNodeBase {
  type: SyntaxType.EnumStructVariant;
  nameNode: IdentifierNode;
}

export interface EnumVariantNode extends NamedNodeBase {
  type: SyntaxType.EnumVariant;
}

export interface ForLoopNode extends NamedNodeBase {
  type: SyntaxType.ForLoop;
  cursorNode: IdentifierNode;
  rangeNodes: (UnnamedNode<"("> | UnnamedNode<")"> | BinaryExpressionNode | FunctionCallNode | IdentifierNode | MemberAccessNode | PrimitiveValueNode | StructInstanceNode | UnaryExpressionNode)[];
  statement_blockNode: BlockNode;
}

export interface FunctionCallNode extends NamedNodeBase {
  type: SyntaxType.FunctionCall;
  argumentsNode: ParenArgumentsNode;
  targetNode: IdentifierNode;
}

export interface FunctionDefinitionNode extends NamedNodeBase {
  type: SyntaxType.FunctionDefinition;
  bodyNode: BlockNode;
  nameNode: IdentifierNode;
  parametersNode: ParametersNode;
  return_typeNode?: ReturnTypeNode;
}

export interface IfStatementNode extends NamedNodeBase {
  type: SyntaxType.IfStatement;
  bodyNode: BlockNode;
  conditionNodes: (UnnamedNode<"("> | UnnamedNode<")"> | BinaryExpressionNode | FunctionCallNode | IdentifierNode | MemberAccessNode | PrimitiveValueNode | StructInstanceNode | UnaryExpressionNode)[];
  elseNode?: ElseStatementNode;
}

export interface ListTypeNode extends NamedNodeBase {
  type: SyntaxType.ListType;
  innerNode: IdentifierNode | PrimitiveTypeNode;
}

export interface ListValueNode extends NamedNodeBase {
  type: SyntaxType.ListValue;
}

export interface MapPairNode extends NamedNodeBase {
  type: SyntaxType.MapPair;
  keyNode: NumberNode | StringNode;
  valueNode: BooleanNode | NumberNode | StringNode;
}

export interface MapTypeNode extends NamedNodeBase {
  type: SyntaxType.MapType;
  keyNode: PrimitiveTypeNode;
  valueNode: PrimitiveTypeNode;
}

export interface MapValueNode extends NamedNodeBase {
  type: SyntaxType.MapValue;
}

export interface MemberAccessNode extends NamedNodeBase {
  type: SyntaxType.MemberAccess;
  memberNode: FunctionCallNode | IdentifierNode;
  targetNodes: (UnnamedNode<"("> | UnnamedNode<")"> | BinaryExpressionNode | FunctionCallNode | IdentifierNode | MemberAccessNode | PrimitiveValueNode | StructInstanceNode | UnaryExpressionNode)[];
}

export interface ParamDefNode extends NamedNodeBase {
  type: SyntaxType.ParamDef;
  nameNode: IdentifierNode;
  typeNode: PrimitiveTypeNode;
}

export interface ParametersNode extends NamedNodeBase {
  type: SyntaxType.Parameters;
}

export interface ParenArgumentsNode extends NamedNodeBase {
  type: SyntaxType.ParenArguments;
}

export interface PrimitiveTypeNode extends NamedNodeBase {
  type: SyntaxType.PrimitiveType;
}

export interface PrimitiveValueNode extends NamedNodeBase {
  type: SyntaxType.PrimitiveValue;
  primitiveNode: BooleanNode | ListValueNode | MapValueNode | NumberNode | StringNode;
}

export interface ProgramNode extends NamedNodeBase {
  type: SyntaxType.Program;
}

export interface ReassignmentNode extends NamedNodeBase {
  type: SyntaxType.Reassignment;
  nameNode: IdentifierNode;
  valueNodes: (UnnamedNode<"("> | UnnamedNode<")"> | BinaryExpressionNode | FunctionCallNode | IdentifierNode | MemberAccessNode | PrimitiveValueNode | StructInstanceNode | UnaryExpressionNode)[];
}

export interface ReturnTypeNode extends NamedNodeBase {
  type: SyntaxType.ReturnType;
}

export interface StatementNode extends NamedNodeBase {
  type: SyntaxType.Statement;
}

export interface StringNode extends NamedNodeBase {
  type: SyntaxType.String;
}

export interface StructDefinitionNode extends NamedNodeBase {
  type: SyntaxType.StructDefinition;
}

export interface StructInstanceNode extends NamedNodeBase {
  type: SyntaxType.StructInstance;
  nameNode: IdentifierNode;
}

export interface StructPropPairNode extends NamedNodeBase {
  type: SyntaxType.StructPropPair;
  nameNode: IdentifierNode;
  valueNode: BooleanNode | NumberNode | StringNode;
}

export interface StructPropertyNode extends NamedNodeBase {
  type: SyntaxType.StructProperty;
}

export interface TypeDeclarationNode extends NamedNodeBase {
  type: SyntaxType.TypeDeclaration;
  typeNode: ListTypeNode | MapTypeNode | PrimitiveTypeNode;
}

export interface UnaryExpressionNode extends NamedNodeBase {
  type: SyntaxType.UnaryExpression;
}

export interface VariableBindingNode extends NamedNodeBase {
  type: SyntaxType.VariableBinding;
}

export interface VariableDefinitionNode extends NamedNodeBase {
  type: SyntaxType.VariableDefinition;
  bindingNode: VariableBindingNode;
  nameNode: IdentifierNode;
  typeNode?: TypeDeclarationNode;
  valueNodes: (UnnamedNode<"("> | UnnamedNode<")"> | BinaryExpressionNode | FunctionCallNode | IdentifierNode | MemberAccessNode | PrimitiveValueNode | StructInstanceNode | UnaryExpressionNode)[];
}

export interface WhileLoopNode extends NamedNodeBase {
  type: SyntaxType.WhileLoop;
  conditionNodes: (UnnamedNode<"("> | UnnamedNode<")"> | BinaryExpressionNode | FunctionCallNode | IdentifierNode | MemberAccessNode | PrimitiveValueNode | StructInstanceNode | UnaryExpressionNode)[];
  doNode?: UnnamedNode<"do">;
  statement_blockNode: BlockNode;
}

export interface AndNode extends NamedNodeBase {
  type: SyntaxType.And;
}

export interface BangNode extends NamedNodeBase {
  type: SyntaxType.Bang;
}

export interface DecrementNode extends NamedNodeBase {
  type: SyntaxType.Decrement;
}

export interface DivideNode extends NamedNodeBase {
  type: SyntaxType.Divide;
}

export interface EqualNode extends NamedNodeBase {
  type: SyntaxType.Equal;
}

export interface GreaterThanNode extends NamedNodeBase {
  type: SyntaxType.GreaterThan;
}

export interface GreaterThanOrEqualNode extends NamedNodeBase {
  type: SyntaxType.GreaterThanOrEqual;
}

export interface IdentifierNode extends NamedNodeBase {
  type: SyntaxType.Identifier;
}

export interface InclusiveRangeNode extends NamedNodeBase {
  type: SyntaxType.InclusiveRange;
}

export interface IncrementNode extends NamedNodeBase {
  type: SyntaxType.Increment;
}

export interface LessThanNode extends NamedNodeBase {
  type: SyntaxType.LessThan;
}

export interface LessThanOrEqualNode extends NamedNodeBase {
  type: SyntaxType.LessThanOrEqual;
}

export interface MinusNode extends NamedNodeBase {
  type: SyntaxType.Minus;
}

export interface ModuloNode extends NamedNodeBase {
  type: SyntaxType.Modulo;
}

export interface MultiplyNode extends NamedNodeBase {
  type: SyntaxType.Multiply;
}

export interface NotEqualNode extends NamedNodeBase {
  type: SyntaxType.NotEqual;
}

export interface NumberNode extends NamedNodeBase {
  type: SyntaxType.Number;
}

export interface OrNode extends NamedNodeBase {
  type: SyntaxType.Or;
}

export interface PlusNode extends NamedNodeBase {
  type: SyntaxType.Plus;
}

