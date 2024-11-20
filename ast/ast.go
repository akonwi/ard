package ast

import (
	"fmt"

	checker "github.com/akonwi/kon/checker"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type BaseNode struct {
	TSNode *tree_sitter.Node
}

func (b *BaseNode) GetTSNode() *tree_sitter.Node {
	return b.TSNode
}

type TypedNode interface {
	Node
	GetType() checker.Type
}

type Node interface {
	String() string
	GetTSNode() *tree_sitter.Node
}

type Program struct {
	BaseNode
	Statements []Statement
}

type Statement interface {
	Node
	StatementNode()
}

type Expression interface {
	Node
	ExpressionNode()
	GetType() checker.Type
}

type VariableDeclaration struct {
	BaseNode
	Name         string
	Mutable      bool
	Value        Expression
	Type         checker.Type
	InferredType checker.Type
}

func (v *VariableDeclaration) StatementNode() {}
func (v *VariableDeclaration) String() string {
	return fmt.Sprintf("TODO")
}

type Parameter struct {
	BaseNode
	Name string
	Type checker.Type
}

func (p *Parameter) String() string {
	return p.Name
}

type FunctionDeclaration struct {
	BaseNode
	Name       string
	Parameters []Parameter
	ReturnType checker.Type
	Body       []Statement
	Type       checker.FunctionType
}

func (f *FunctionDeclaration) StatementNode() {}
func (f *FunctionDeclaration) String() string {
	return fmt.Sprintf("(%s) ?", f.Name)
}

type Operator int

const (
	InvalidOp Operator = iota
	Minus
	Plus
	Slash
	Star
	Bang
)

type UnaryExpression struct {
	BaseNode
	Operator Operator
	Operand  Expression
}

// impl interfaces
func (u *UnaryExpression) ExpressionNode() {}
func (u *UnaryExpression) StatementNode()  {}
func (u *UnaryExpression) String() string {
	return fmt.Sprintf("(%v %v)", u.Operator, u.Operand)
}
func (u *UnaryExpression) GetType() checker.Type {
	return u.Operand.GetType()
}

type StrLiteral struct {
	BaseNode
	Value string
	Type  checker.Type
}

// impl interfaces
func (s *StrLiteral) ExpressionNode() {}
func (s *StrLiteral) StatementNode()  {}
func (s *StrLiteral) String() string {
	return s.Value
}
func (s *StrLiteral) GetType() checker.Type {
	return checker.StrType
}

type NumLiteral struct {
	BaseNode
	Value string
	Type  checker.Type
}

// impl interfaces
func (n *NumLiteral) ExpressionNode() {}
func (n *NumLiteral) StatementNode()  {}
func (n *NumLiteral) String() string {
	return n.Value
}
func (n *NumLiteral) GetType() checker.Type {
	return checker.NumType
}

type BoolLiteral struct {
	BaseNode
	Value bool
	Type  checker.Type
}

// impl interfaces
func (b *BoolLiteral) ExpressionNode() {}
func (b *BoolLiteral) StatementNode()  {}
func (b *BoolLiteral) String() string {
	return fmt.Sprintf("%t", b.Value)
}
func (b *BoolLiteral) GetType() checker.Type {
	return checker.BoolType
}

type Parser struct {
	sourceCode []byte
	tree       *tree_sitter.Tree
	scope      *checker.Scope
	typeErrors []checker.Error
}

func NewParser(sourceCode []byte, tree *tree_sitter.Tree) *Parser {
	return &Parser{sourceCode: sourceCode, tree: tree, scope: checker.NewScope(nil)}
}

func (p *Parser) text(node *tree_sitter.Node) string {
	return string(p.sourceCode[node.StartByte():node.EndByte()])
}

func (p *Parser) typeMismatchError(node *tree_sitter.Node, expected, actual checker.Type) {
	msg := fmt.Sprintf("Type mismatch: expected %s, got %s", expected, actual)
	p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
}

func (p *Parser) operatorError(node *tree_sitter.Node, expected checker.Type) {
	msg := fmt.Sprintf("The '%v' operator can only be used on '%v'", p.text(node), expected)
	p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
}

func (p *Parser) Parse() (*Program, error) {
	rootNode := p.tree.RootNode()
	program := &Program{
		BaseNode:   BaseNode{TSNode: rootNode},
		Statements: []Statement{}}

	for i := range rootNode.NamedChildCount() {
		stmt, err := p.parseStatement(rootNode.NamedChild(i))
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
	}

	return program, nil
}

func (p *Parser) parseStatement(node *tree_sitter.Node) (Statement, error) {
	child := node.NamedChild(0)
	switch child.GrammarName() {
	case "variable_definition":
		return p.parseVariableDecl(child)
	case "function_definition":
		return p.parseFunctionDecl(child)
	case "expression":
		expr, err := p.parseExpression(child)
		if err != nil {
			return nil, err
		}
		return expr.(Statement), nil
	default:
		return nil, fmt.Errorf("Unhandled statement: %s", child.GrammarName())
	}
}

func (p *Parser) parseVariableDecl(node *tree_sitter.Node) (*VariableDeclaration, error) {
	isMutable := p.text(node.NamedChild(0)) == "mut"
	name := p.text(node.NamedChild(1))
	declaredType := p.resolveType(node.ChildByFieldName("type"))
	value, err := p.parseExpression(node.ChildByFieldName("value"))
	if err != nil {
		return nil, err
	}

	inferredType := value.GetType()

	if declaredType != nil && inferredType != declaredType {
		p.typeMismatchError(node.ChildByFieldName("value"), declaredType, inferredType)
	}

	p.scope.Declare(checker.Symbol{
		Mutable:  isMutable,
		Name:     name,
		Type:     declaredType,
		Declared: true,
	})

	return &VariableDeclaration{
		BaseNode:     BaseNode{TSNode: node},
		Mutable:      isMutable,
		Name:         name,
		Value:        value,
		Type:         declaredType,
		InferredType: inferredType,
	}, nil
}

func (p *Parser) resolveType(node *tree_sitter.Node) checker.Type {
	if node == nil {
		return nil
	}

	child := node.NamedChild(0)
	switch child.GrammarName() {
	case "primitive_type":
		{
			text := p.text(child)
			switch text {
			case "Str":
				return checker.StrType
			case "Num":
				return checker.NumType
			case "Bool":
				return checker.BoolType
			default:
				panic(fmt.Errorf("Unresolved primitive type: %s", text))
			}
		}
	case "void":
		return checker.VoidType
	default:
		panic(fmt.Errorf("Unresolved type: %v", child.GrammarName()))
	}
}

func (p *Parser) parseFunctionDecl(node *tree_sitter.Node) (*FunctionDeclaration, error) {
	name := p.text(node.ChildByFieldName("name"))
	parameters := p.parseParameters(
		node.ChildByFieldName("parameters"))
	returnType := p.resolveType(node.ChildByFieldName("return"))
	body, err := p.parseBlock(node.ChildByFieldName("body"))

	if err != nil {
		return nil, err
	}

	var inferredType checker.Type = checker.VoidType
	var lastStatement Statement
	if len(body) > 0 {
		lastStatement = body[len(body)-1]
		if expr, ok := lastStatement.(Expression); ok {
			inferredType = expr.GetType()
		}
	}

	if returnType == nil {
		returnType = inferredType
	} else if returnType != inferredType {
		if lastStatement != nil {
			p.typeMismatchError(lastStatement.GetTSNode(), returnType, inferredType)
		} else {
			p.typeMismatchError(node.ChildByFieldName("body"), returnType, inferredType)
		}
	}

	return &FunctionDeclaration{
		BaseNode:   BaseNode{TSNode: node},
		Name:       name,
		Parameters: parameters,
		ReturnType: returnType,
		Body:       body,
	}, nil
}

func (p *Parser) parseParameters(node *tree_sitter.Node) []Parameter {
	parameterNodes := node.ChildrenByFieldName("parameter", p.tree.Walk())
	parameters := []Parameter{}

	for _, node := range parameterNodes {
		parameters = append(parameters, Parameter{
			BaseNode: BaseNode{TSNode: &node},
			Name:     p.text(node.ChildByFieldName("name")),
		})
	}

	return parameters
}

func (p *Parser) parseBlock(node *tree_sitter.Node) ([]Statement, error) {
	statements := []Statement{}
	for i := range node.NamedChildCount() {
		stmt, err := p.parseStatement(node.NamedChild(i))
		if err != nil {
			return statements, err
		}
		if stmt != nil {
			statements = append(statements, stmt)
		}
	}
	return statements, nil
}

func (p *Parser) parseExpression(node *tree_sitter.Node) (Expression, error) {
	child := node.Child(0)
	switch child.GrammarName() {
	case "primitive_value":
		return p.parsePrimitiveValue(child)
	case "unary_expression":
		return p.parseUnaryExpression(child)
	default:
		return nil, fmt.Errorf("Unhandled expression: %s", child.GrammarName())
	}
}

func (p *Parser) parsePrimitiveValue(node *tree_sitter.Node) (Expression, error) {
	child := node.Child(0)
	switch child.GrammarName() {
	case "string":
		return &StrLiteral{
			BaseNode: BaseNode{TSNode: node},
			Value:    p.text(child)}, nil
	case "number":
		return &NumLiteral{
			BaseNode: BaseNode{TSNode: node},
			Value:    p.text(child)}, nil
	case "boolean":
		return &BoolLiteral{
			BaseNode: BaseNode{TSNode: node},
			Value:    p.text(child) == "true"}, nil
	default:
		return nil, fmt.Errorf("Unhandled expression: %s", child.GrammarName())
	}
}

func (p *Parser) parseUnaryExpression(node *tree_sitter.Node) (Expression, error) {
	operatorNode := node.ChildByFieldName("operator")
	operandNode := node.ChildByFieldName("operand")

	operator := InvalidOp
	switch operatorNode.GrammarName() {
	case "minus":
		operator = Minus
	case "bang":
		operator = Bang
	default:
		return nil, fmt.Errorf("Unsupported unary operator: %v", operatorNode.GrammarName())
	}

	operand, err := p.parseExpression(operandNode)
	if err != nil {
		return nil, err
	}

	switch operator {
	case Minus:
		if operand.GetType() != checker.NumType {
			p.operatorError(operatorNode, checker.NumType)
		}
	case Bang:
		if operand.GetType() != checker.BoolType {
			p.operatorError(operatorNode, checker.BoolType)
		}
	}

	return &UnaryExpression{
		BaseNode: BaseNode{TSNode: node},
		Operator: operator,
		Operand:  operand,
	}, nil
}
