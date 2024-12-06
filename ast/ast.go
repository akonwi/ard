package ast

import (
	"fmt"

	checker "github.com/akonwi/kon/checker"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// statements do not produce values
type Statement interface {
	String() string
	GetTSNode() *tree_sitter.Node
}

// expressions produce values
type Expression interface {
	Statement
	GetType() checker.Type
}

// the base struct for all AST nodes
type BaseNode struct {
	TSNode *tree_sitter.Node
}

func (b BaseNode) GetTSNode() *tree_sitter.Node {
	return b.TSNode
}

type Program struct {
	BaseNode
	Statements []Statement
}

type VariableDeclaration struct {
	BaseNode
	Name    string
	Mutable bool
	Value   Expression
	Type    checker.Type // the declared type
}

func (v VariableDeclaration) String() string {
	binding := "let"
	if v.Mutable {
		binding = "mut"
	}
	return fmt.Sprintf("%s %s: %s", binding, v.Name, v.Type)
}

type VariableAssignment struct {
	BaseNode
	Name     string
	Operator Operator
	Value    Expression
}

// impl interfaces
func (v VariableAssignment) String() string {
	return fmt.Sprintf("%v = %s", v.Name, v.Value)
}

type Parameter struct {
	BaseNode
	Name string
	Type checker.Type
}

func (p Parameter) String() string {
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

func (f FunctionDeclaration) String() string {
	return fmt.Sprintf("%s(%v) %s", f.Name, f.Parameters, f.ReturnType)
}

type AnonymousFunction struct {
	BaseNode
	Parameters []Parameter
	ReturnType checker.Type
	Body       []Statement
}

func (a AnonymousFunction) String() string {
	return fmt.Sprintf("AnonymousFunction(%s)", a.GetType())
}
func (a AnonymousFunction) GetType() checker.Type {
	parameterTypes := make([]checker.Type, len(a.Parameters))
	for i, param := range a.Parameters {
		parameterTypes[i] = param.Type
	}
	return checker.FunctionType{
		Mutates:    false,
		Parameters: parameterTypes,
		ReturnType: a.ReturnType,
	}
}

type StructDefinition struct {
	BaseNode
	Type checker.StructType
}

func (s StructDefinition) String() string {
	return fmt.Sprintf("StructDefinition(%s)", s.Type.Name)
}

type StructValue struct {
	Name  string
	Value Expression
}

type StructInstance struct {
	BaseNode
	Type       checker.StructType
	Properties []StructValue
}

func (s StructInstance) String() string {
	return fmt.Sprintf("StructInstance(%s)", s.Type.Name)
}
func (s StructInstance) GetType() checker.Type {
	return s.Type
}

type EnumDefinition struct {
	BaseNode
	Type checker.EnumType
}

func (e EnumDefinition) String() string {
	return fmt.Sprintf("EnumDefinition(%s)", e.Type.Name)
}

type WhileLoop struct {
	BaseNode
	Condition Expression
	Body      []Statement
}

func (w WhileLoop) String() string {
	return "while"
}

type ForLoop struct {
	BaseNode
	Cursor   Identifier
	Iterable Expression
	Body     []Statement
}

func (f ForLoop) String() string {
	return "ForLoop"
}

type IfStatement struct {
	BaseNode
	Condition Expression
	Body      []Statement
	Else      Statement
}

func (i IfStatement) String() string {
	return "IfStatement"
}

type FunctionCall struct {
	BaseNode
	Name string
	Args []Expression
	Type checker.FunctionType
}

func (f FunctionCall) String() string {
	return fmt.Sprintf("FunctionCall(%s)", f.Name)
}
func (f FunctionCall) GetType() checker.Type {
	return f.Type.ReturnType
}

type MemberAccessType string

const (
	Instance = "instance"
	Static   = "static"
)

type MemberAccess struct {
	BaseNode
	Target     Expression
	AccessType MemberAccessType
	Member     Expression
}

func (m MemberAccess) String() string {
	operator := "."
	if m.AccessType == Static {
		operator = "::"
	}
	return fmt.Sprintf("MemberAccess(%s%s%s)", m.Target, operator, m.Member)
}
func (m MemberAccess) GetType() checker.Type {
	return m.Member.GetType()
}

type Operator int

const (
	InvalidOp Operator = iota
	Bang
	Minus
	Decrement
	Plus
	Increment
	Divide
	Multiply
	Modulo
	GreaterThan
	GreaterThanOrEqual
	LessThan
	LessThanOrEqual
	Equal
	NotEqual
	And
	Or
	Range
	Assign
)

type UnaryExpression struct {
	BaseNode
	Operator Operator
	Operand  Expression
}

// impl interfaces
func (u UnaryExpression) String() string {
	return fmt.Sprintf("(%v %v)", u.Operator, u.Operand)
}
func (u UnaryExpression) GetType() checker.Type {
	return u.Operand.GetType()
}

type BinaryExpression struct {
	BaseNode
	Operator      Operator
	Left, Right   Expression
	HasPrecedence bool
}

func (b BinaryExpression) String() string {
	return fmt.Sprintf("%v %v %v", b.Left, b.Operator, b.Right)
}
func (b BinaryExpression) GetType() checker.Type {
	switch b.Operator {
	case Plus, Minus, Multiply, Divide, Modulo:
		return checker.NumType
	case GreaterThan, GreaterThanOrEqual, LessThan, LessThanOrEqual, Equal, NotEqual, And, Or:
		return checker.BoolType
	case Range:
		return checker.NumType
	default:
		return nil
	}
}

type RangeExpression struct {
	BaseNode
	Start, End Expression
}

func (b RangeExpression) String() string {
	return "RangeExpression"
}
func (b RangeExpression) GetType() checker.Type {
	return checker.NumType
}

type Identifier struct {
	BaseNode
	Name string
	Type checker.Type
}

func (i Identifier) String() string {
	return fmt.Sprintf("Identifier(%s)", i.Name)
}
func (i Identifier) GetType() checker.Type {
	return i.Type
}

type StrLiteral struct {
	BaseNode
	Value string
	Type  checker.Type
}

func (s StrLiteral) String() string {
	return s.Value
}
func (s StrLiteral) GetType() checker.Type {
	return checker.StrType
}

type InterpolatedStr struct {
	BaseNode
	Chunks []Expression
}

func (i InterpolatedStr) String() string {
	return "InterpolatedStr"
}
func (i InterpolatedStr) GetType() checker.Type {
	return checker.StrType
}

type NumLiteral struct {
	BaseNode
	Value string
	Type  checker.Type
}

func (n NumLiteral) String() string {
	return n.Value
}
func (n NumLiteral) GetType() checker.Type {
	return checker.NumType
}

type BoolLiteral struct {
	BaseNode
	Value bool
	Type  checker.Type
}

// impl interfaces
func (b BoolLiteral) String() string {
	return fmt.Sprintf("%t", b.Value)
}
func (b BoolLiteral) GetType() checker.Type {
	return checker.BoolType
}

type ListLiteral struct {
	BaseNode
	Type  checker.Type
	Items []Expression
}

func (l ListLiteral) String() string {
	return "ListLiteral"
}
func (l ListLiteral) GetType() checker.Type {
	return l.Type
}

type MapEntry struct {
	Key   string
	Value Expression
}

type MapLiteral struct {
	BaseNode
	Entries []MapEntry
	Type    checker.Type
}

func (m MapLiteral) String() string {
	return fmt.Sprintf("MapLiteral { %v }", m.Entries)
}
func (m MapLiteral) GetType() checker.Type {
	return m.Type
}

type MatchExpression struct {
	BaseNode
	Subject Expression
	Cases   []MatchCase
}

func (m MatchExpression) String() string {
	return fmt.Sprintf("MatchExpression(%s)", m.Subject)
}
func (m MatchExpression) GetType() checker.Type {
	return m.Cases[0].GetType()
}

type MatchCase struct {
	BaseNode
	Pattern Expression
	Body    []Statement
	Type    checker.Type
}

func (m MatchCase) String() string {
	return fmt.Sprintf("MatchCase(%s)", m.Pattern)
}
func (m MatchCase) GetType() checker.Type {
	return m.Type
}

type Parser struct {
	sourceCode []byte
	tree       *tree_sitter.Tree
	scope      *checker.Scope
	typeErrors []checker.Diagnostic
}

func (p *Parser) GetDiagnostics() []checker.Diagnostic {
	return p.typeErrors
}

func NewParser(sourceCode []byte, tree *tree_sitter.Tree) *Parser {
	scope := checker.NewScope(nil, checker.ScopeOptions{IsTop: true})
	return &Parser{sourceCode: sourceCode, tree: tree, scope: &scope}
}

func (p *Parser) text(node *tree_sitter.Node) string {
	return string(p.sourceCode[node.StartByte():node.EndByte()])
}

func (p *Parser) mustChild(node *tree_sitter.Node, name string) *tree_sitter.Node {
	child := node.ChildByFieldName(name)
	// if node.HasError() {
	// 	panic(fmt.Errorf("Parsing error encountered: %s", p.text(node)))
	// }
	if child == nil {
		panic(fmt.Errorf("Missing child: %s in `%s`", name, p.text(node)))
	}
	return child
}

func (p *Parser) mustChildren(node *tree_sitter.Node, name string) []tree_sitter.Node {
	children := node.ChildrenByFieldName(name, p.tree.Walk())
	// if node.HasError() {
	// 	panic(fmt.Errorf("Parsing error encountered: %s", p.text(node)))
	// }
	if len(children) == 0 {
		panic(fmt.Errorf("Missing children: %s in `%s`", name, p.text(node)))
	}
	return children
}

func (p *Parser) pushScope() *checker.Scope {
	new := checker.NewScope(p.scope, checker.ScopeOptions{})
	p.scope = &new
	return p.scope
}

func (p *Parser) popScope() *checker.Scope {
	p.scope = p.scope.GetParent()
	return p.scope
}

func (p *Parser) typeMismatchError(node *tree_sitter.Node, expected, actual checker.Type) {
	msg := fmt.Sprintf("Type mismatch: expected %s, got %s", expected, actual)
	p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
}

func (p *Parser) unaryOperatorError(node *tree_sitter.Node, expected checker.Type) {
	msg := fmt.Sprintf("The '%v' operator can only be used on '%v'", p.text(node), expected)
	p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
}

func (p *Parser) binaryOperatorError(node *tree_sitter.Node, operator string, expected checker.Type) {
	msg := fmt.Sprintf("The '%v' operator can only be used between instances of '%v'", operator, expected)
	p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
}

func (p *Parser) equalityOperatorError(node *tree_sitter.Node, operator string) {
	msg := fmt.Sprintf("The '%v' operator can only be used between instances of 'Num', 'Str', or 'Bool'", operator)
	p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
}

func (p *Parser) logicalOperatorError(node *tree_sitter.Node, operator string) {
	msg := fmt.Sprintf("The '%v' operator can only be used between instances of 'Bool'", operator)
	p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
}

func (p *Parser) Parse() (Program, error) {
	rootNode := p.tree.RootNode()
	program := Program{
		BaseNode:   BaseNode{TSNode: rootNode},
		Statements: []Statement{}}

	for i := range rootNode.NamedChildCount() {
		stmt, err := p.parseStatement(rootNode.NamedChild(i))
		if err != nil {
			return Program{}, err
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
	case "reassignment":
		return p.parseVariableReassignment(child)
	case "function_definition":
		return p.parseFunctionDecl(child)
	case "while_loop":
		return p.parseWhileLoop(child)
	case "for_loop":
		return p.parseForLoop(child)
	case "if_statement":
		return p.parseIfStatement(child)
	case "struct_definition":
		return p.parseStructDefinition(child)
	case "enum_definition":
		return p.parseEnumDefinition(child)
	case "expression":
		expr, err := p.parseExpression(child)
		if err != nil {
			return nil, err
		}
		return expr, nil
	default:
		return nil, fmt.Errorf("Unhandled statement: %s", child.GrammarName())
	}
}

func (p *Parser) parseVariableDecl(node *tree_sitter.Node) (VariableDeclaration, error) {
	isMutable := p.text(node.NamedChild(0)) == "mut"
	name := p.text(node.NamedChild(1))
	declaredType := p.resolveType(node.ChildByFieldName("type"))
	value, err := p.parseExpression(node.ChildByFieldName("value"))
	if err != nil {
		return VariableDeclaration{}, err
	}

	inferredType := value.GetType()

	if declaredType != nil {
		if !declaredType.Equals(inferredType) {
			p.typeMismatchError(node.ChildByFieldName("value"), declaredType, inferredType)
		}
	} else if inferredType == nil {
		panic(fmt.Errorf("variable inferred type and declared type are nil"))
	} else {
		if lt, ok := inferredType.(checker.ListType); ok {
			if lt.ItemType == nil {
				msg := fmt.Sprintf("Empty lists need a declared type")
				p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
			}
		}

		if mt, ok := inferredType.(checker.MapType); ok {
			if mt.KeyType == nil || mt.ValueType == nil {
				msg := fmt.Sprintf("Empty maps need a declared type")
				p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
			}
		}
	}

	symbolType := declaredType
	if declaredType == nil {
		symbolType = inferredType
	}
	p.scope.Declare(checker.Variable{
		Mutable: isMutable,
		Name:    name,
		Type:    symbolType,
	})

	return VariableDeclaration{
		BaseNode: BaseNode{TSNode: node},
		Mutable:  isMutable,
		Name:     name,
		Value:    value,
		Type:     symbolType,
	}, nil
}

// use for resolving explicit type declarations
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
	case "list_type":
		element_typeNode := child.ChildByFieldName("element_type")
		return &checker.ListType{ItemType: p.resolveType(element_typeNode)}
	case "map_type":
		valueNode := child.ChildByFieldName("value")
		return checker.MapType{
			KeyType:   checker.StrType,
			ValueType: p.resolveType(valueNode),
		}
	case "void":
		return checker.VoidType
	case "identifier":
		identifier := p.text(child)
		symbol := p.scope.Lookup(identifier)
		if symbol == nil {
			panic(fmt.Sprintf("Undefined: '%s'", identifier))
		}
		return symbol.GetType()
	default:
		panic(fmt.Errorf("Unresolved type: %v", child.GrammarName()))
	}
}

func (p *Parser) parseVariableReassignment(node *tree_sitter.Node) (VariableAssignment, error) {
	nameNode := node.ChildByFieldName("name")
	operatorNode := node.ChildByFieldName("operator")
	valueNode := node.ChildByFieldName("value")

	name := p.text(nameNode)
	operator := resolveOperator(operatorNode)
	symbol := p.scope.Lookup(name)

	value, err := p.parseExpression(valueNode)
	if err != nil {
		return VariableAssignment{}, err
	}

	if symbol == nil {
		msg := fmt.Sprintf("Undefined: '%s'", name)
		p.typeErrors = append(p.typeErrors, checker.Diagnostic{Msg: msg, Range: nameNode.Range()})
		return VariableAssignment{Name: name, Operator: operator, Value: value}, nil
	}

	variable, ok := symbol.(checker.Variable)
	if !ok {
		msg := fmt.Sprintf("'%s' is not a variable", name)
		p.typeErrors = append(p.typeErrors, checker.Diagnostic{Msg: msg, Range: nameNode.Range()})
		return VariableAssignment{}, fmt.Errorf(msg)
	}

	if variable.Mutable == false {
		msg := fmt.Sprintf("'%s' is not mutable", name)
		p.typeErrors = append(p.typeErrors, checker.Diagnostic{Msg: msg, Range: nameNode.Range()})
	}

	switch operator {
	case Assign:
		if !variable.GetType().Equals(value.GetType()) {
			msg := fmt.Sprintf("Expected a '%s' and received '%v'", variable.GetType(), value.GetType())
			p.typeErrors = append(p.typeErrors, checker.Diagnostic{Msg: msg, Range: valueNode.Range()})
		}
	case Increment, Decrement:
		if variable.GetType() != checker.NumType || value.GetType() != checker.NumType {
			msg := fmt.Sprintf("'%s' can only be used with 'Num'", p.text(operatorNode))
			p.typeErrors = append(p.typeErrors, checker.Diagnostic{Msg: msg, Range: valueNode.Range()})
		}
	}

	return VariableAssignment{
		Name:     name,
		Operator: operator,
		Value:    value,
	}, nil
}

func (p *Parser) parseFunctionDecl(node *tree_sitter.Node) (FunctionDeclaration, error) {
	name := p.text(node.ChildByFieldName("name"))
	parameters := p.parseParameters(node.ChildByFieldName("parameters"))
	returnType := p.resolveType(node.ChildByFieldName("return"))

	scope := p.pushScope()
	parameterTypes := make([]checker.Type, len(parameters))
	for i, param := range parameters {
		parameterTypes[i] = param.Type
		scope.Declare(checker.Variable{
			Mutable: false,
			Name:    param.Name,
			Type:    param.Type,
		})
	}

	body, err := p.parseBlock(node.ChildByFieldName("body"))

	p.popScope()

	if err != nil {
		return FunctionDeclaration{}, err
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

	fnType := checker.FunctionType{
		Name:       name,
		Mutates:    false,
		Parameters: parameterTypes,
		ReturnType: returnType,
	}
	p.scope.Declare(fnType)

	return FunctionDeclaration{
		BaseNode:   BaseNode{TSNode: node},
		Name:       name,
		Parameters: parameters,
		ReturnType: returnType,
		Body:       body,
	}, nil
}

func (p *Parser) parseParameters(node *tree_sitter.Node) []Parameter {
	if node.HasError() {
		panic(fmt.Errorf("Error parsing function parameters: %s", p.text(node)))
	}
	parameterNodes := node.ChildrenByFieldName("parameter", p.tree.Walk())
	parameters := []Parameter{}

	for _, node := range parameterNodes {
		parameters = append(parameters, Parameter{
			BaseNode: BaseNode{TSNode: &node},
			Name:     p.text(node.ChildByFieldName("name")),
			Type:     p.resolveType(node.ChildByFieldName("type")),
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

func (p *Parser) parseWhileLoop(node *tree_sitter.Node) (Statement, error) {
	conditionNode := node.ChildByFieldName("condition")
	bodyNode := node.ChildByFieldName("body")

	condition, err := p.parseExpression(conditionNode)
	if err != nil {
		return nil, err
	}

	if condition.GetType() != checker.BoolType {
		msg := fmt.Sprintf("A while loop condition must be a 'Bool' expression")
		p.typeErrors = append(p.typeErrors, checker.Diagnostic{Msg: msg, Range: conditionNode.Range()})
	}

	body, err := p.parseBlock(bodyNode)
	if err != nil {
		return nil, err
	}

	return WhileLoop{
		Condition: condition,
		Body:      body,
	}, nil
}

func (p *Parser) parseForLoop(node *tree_sitter.Node) (Statement, error) {
	cursorNode := node.ChildByFieldName("cursor")
	rangeNode := node.ChildByFieldName("range")
	bodyNode := node.ChildByFieldName("body")

	iterable, err := p.parseExpression(rangeNode)
	if err != nil {
		return nil, err
	}

	iterableType := iterable.GetType()

	if iterableType == checker.NumType || iterableType == checker.StrType {
		_cursor := Identifier{Name: p.text(cursorNode), Type: iterableType}
		newScope := p.pushScope()
		newScope.Declare(checker.Variable{Mutable: false, Name: _cursor.Name, Type: _cursor.Type})
		body, err := p.parseBlock(bodyNode)
		p.popScope()
		if err != nil {
			return nil, err
		}
		return ForLoop{
			Cursor:   _cursor,
			Iterable: iterable,
			Body:     body,
		}, nil
	}

	if _listType, ok := iterableType.(checker.ListType); ok {
		_cursor := Identifier{Name: p.text(cursorNode), Type: _listType.ItemType}
		newScope := p.pushScope()
		newScope.Declare(checker.Variable{Mutable: false, Name: _cursor.Name, Type: _cursor.Type})
		body, err := p.parseBlock(bodyNode)
		p.popScope()
		if err != nil {
			return nil, err
		}
		return ForLoop{
			Cursor:   _cursor,
			Iterable: iterable,
			Body:     body,
		}, nil
	}

	msg := fmt.Sprintf("Cannot iterate over a '%s'", iterableType)
	p.typeErrors = append(p.typeErrors, checker.Diagnostic{Msg: msg, Range: rangeNode.Range()})
	return nil, fmt.Errorf(msg)
}

func (p *Parser) parseIfStatement(node *tree_sitter.Node) (Statement, error) {
	conditionNode := node.ChildByFieldName("condition")
	bodyNode := node.ChildByFieldName("body")
	elseNode := node.ChildByFieldName("else")

	condition, err := p.parseExpression(conditionNode)
	if err != nil {
		return nil, err
	}

	if condition.GetType() != checker.BoolType {
		msg := fmt.Sprintf("An if condition must be a 'Bool' expression")
		p.typeErrors = append(p.typeErrors, checker.Diagnostic{Msg: msg, Range: conditionNode.Range()})
	}

	body, err := p.parseBlock(bodyNode)
	if err != nil {
		return nil, err
	}

	if elseNode != nil {
		clause, err := p.parseElseClause(elseNode)
		if err != nil {
			return nil, err
		}
		return IfStatement{
			BaseNode:  BaseNode{TSNode: node},
			Condition: condition,
			Body:      body,
			Else:      clause,
		}, nil
	}

	return IfStatement{
		BaseNode:  BaseNode{TSNode: node},
		Condition: condition,
		Body:      body,
	}, nil
}

func (p *Parser) parseElseClause(node *tree_sitter.Node) (Statement, error) {
	ifNode := node.ChildByFieldName("if")
	if ifNode != nil {
		return p.parseIfStatement(ifNode)
	}

	bodyNode := node.ChildByFieldName("body")
	body, err := p.parseBlock(bodyNode)
	if err != nil {
		return nil, err
	}
	return IfStatement{
		BaseNode: BaseNode{TSNode: node},
		Body:     body,
	}, nil
}

func (p *Parser) parseStructDefinition(node *tree_sitter.Node) (Statement, error) {
	nameNode := node.ChildByFieldName("name")
	fieldNodes := node.ChildrenByFieldName("field", p.tree.Walk())

	fields := make(map[string]checker.Type)
	for _, fieldNode := range fieldNodes {
		nameNode := fieldNode.ChildByFieldName("name")
		name := p.text(nameNode)
		typeNode := fieldNode.ChildByFieldName("type")
		fieldType := p.resolveType(typeNode)
		fields[name] = fieldType
	}

	_type := checker.StructType{Name: p.text(nameNode), Fields: fields}
	p.scope.Declare(_type)

	strct := StructDefinition{
		Type: _type,
	}
	return strct, nil
}

func (p *Parser) parseStructInstance(node *tree_sitter.Node) (Expression, error) {
	nameNode := node.ChildByFieldName("name")
	fieldNodes := node.ChildrenByFieldName("field", p.tree.Walk())

	name := p.text(nameNode)
	symbol := p.scope.Lookup(name)
	if symbol == nil {
		return nil, p.undefinedSymbolError(nameNode)
	}

	structType, ok := symbol.GetType().(checker.StructType)
	if !ok {
		msg := fmt.Sprintf("'%s' is not a struct", name)
		p.typeErrors = append(p.typeErrors, checker.MakeError(msg, nameNode))
		return nil, fmt.Errorf(msg)
	}

	receivedNames := make(map[string]int8)
	properties := make([]StructValue, len(fieldNodes))
	for i, propertyNode := range fieldNodes {
		nameNode := propertyNode.ChildByFieldName("name")
		name := p.text(nameNode)

		valueNode := propertyNode.ChildByFieldName("value")
		value, err := p.parsePrimitiveValue(valueNode)
		if err != nil {
			return nil, err
		}

		expectedType, ok := structType.Fields[name]
		if !ok {
			msg := fmt.Sprintf("'%s' is not a field of '%s'", name, structType.Name)
			p.typeErrors = append(p.typeErrors, checker.MakeError(msg, nameNode))
			continue
		}

		if !expectedType.Equals(value.GetType()) {
			p.typeMismatchError(&propertyNode, expectedType, value.GetType())
		}

		if _, ok := receivedNames[name]; ok {
			p.typeErrors = append(p.typeErrors, checker.MakeError(fmt.Sprintf("Duplicate field '%s' in struct '%s'", name, structType.Name), nameNode))
		} else {
			receivedNames[name] = 0
		}
		properties[i] = StructValue{Name: name, Value: value}
	}

	for name := range structType.Fields {
		if _, ok := receivedNames[name]; !ok {
			msg := fmt.Sprintf("Missing field '%s' in struct '%s'", name, structType.Name)
			p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
		}
	}

	return StructInstance{
		BaseNode:   BaseNode{TSNode: node},
		Type:       structType,
		Properties: properties,
	}, nil
}

func (p *Parser) parseEnumDefinition(node *tree_sitter.Node) (Statement, error) {
	nameNode := node.ChildByFieldName("name")
	variantNodes := node.ChildrenByFieldName("variant", p.tree.Walk())

	variants := make([]string, len(variantNodes))
	names := make(map[string]int8)
	for i, variantNode := range variantNodes {
		nameNode := variantNode.NamedChild(0)
		name := p.text(nameNode)
		if _, ok := names[name]; ok {
			msg := fmt.Sprintf("Duplicate variant '%s'", name)
			p.typeErrors = append(p.typeErrors, checker.MakeError(msg, nameNode))
		} else {
			names[name] = 0
		}
		variants[i] = name
	}

	_type := checker.EnumType{Name: p.text(nameNode), Variants: variants}

	enum := EnumDefinition{
		BaseNode: BaseNode{TSNode: node},
		Type:     _type,
	}
	p.scope.Declare(_type)
	return enum, nil
}

func (p *Parser) parseExpression(node *tree_sitter.Node) (Expression, error) {
	child := node.Child(0)
	switch child.GrammarName() {
	case "paren_expression":
		expr, err := p.parseExpression(child.ChildByFieldName("expr"))
		if err != nil {
			return nil, err
		}
		if binary, ok := expr.(BinaryExpression); ok {
			binary.HasPrecedence = true
			return binary, nil
		}
		return expr, nil
	case "primitive_value":
		return p.parsePrimitiveValue(child)
	case "list_value":
		return p.parseListValue(child)
	case "map_value":
		return p.parseMapLiteral(child)
	case "identifier":
		return p.parseIdentifier(child)
	case "unary_expression":
		return p.parseUnaryExpression(child)
	case "binary_expression":
		return p.parseBinaryExpression(child)
	case "member_access":
		return p.parseMemberAccess(child)
	case "function_call":
		return p.parseFunctionCall(child, nil)
	case "struct_instance":
		return p.parseStructInstance(child)
	case "match_expression":
		return p.parseMatchExpression(child)
	case "anonymous_function":
		return p.parseAnonymousFunction(child)
	default:
		return nil, fmt.Errorf("Unhandled expression: %s", child.GrammarName())
	}
}

func (p *Parser) parseIdentifier(node *tree_sitter.Node) (Identifier, error) {
	name := p.text(node)
	symbol := p.scope.Lookup(name)
	if symbol == nil {
		return Identifier{}, p.undefinedSymbolError(node)
	}

	return Identifier{Name: name, Type: symbol.GetType()}, nil
}

func (p *Parser) undefinedSymbolError(node *tree_sitter.Node) error {
	msg := fmt.Sprintf("Undefined: '%s'", p.text(node))
	p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
	return fmt.Errorf(msg)
}

func (p *Parser) parsePrimitiveValue(node *tree_sitter.Node) (Expression, error) {
	child := node.Child(0)
	switch child.GrammarName() {
	case "string":
		chunkNodes := p.mustChildren(child, "chunk")
		if len(chunkNodes) == 1 && chunkNodes[0].GrammarName() == "string_content" {
			return StrLiteral{
				BaseNode: BaseNode{TSNode: node},
				Value:    p.text(node)}, nil
		}

		chunks := make([]Expression, len(chunkNodes))
		for i, chunkNode := range chunkNodes {
			if chunkNode.GrammarName() == "string_content" {
				chunks[i] = StrLiteral{BaseNode: BaseNode{TSNode: &chunkNode}, Value: p.text(&chunkNode)}
			} else {
				chunk, err := p.parseExpression(p.mustChild(&chunkNode, "expression"))
				if err != nil {
					return nil, err
				}
				chunks[i] = chunk
			}
		}
		return InterpolatedStr{
			BaseNode: BaseNode{TSNode: node},
			Chunks:   chunks,
		}, nil
	case "number":
		return NumLiteral{
			BaseNode: BaseNode{TSNode: node},
			Value:    p.text(child)}, nil
	case "boolean":
		return BoolLiteral{
			BaseNode: BaseNode{TSNode: node},
			Value:    p.text(child) == "true"}, nil
	default:
		return nil, fmt.Errorf("Unhandled primitive node: %s", child.GrammarName())
	}
}

func (p *Parser) parseListValue(node *tree_sitter.Node) (Expression, error) {
	elementNodes := node.ChildrenByFieldName("element", p.tree.Walk())
	items := make([]Expression, len(elementNodes))

	var itemType checker.Type

	for i, innerNode := range elementNodes {
		item, err := p.parseListElement(&innerNode)
		if err != nil {
			return nil, err
		}
		items[i] = item
		if i == 0 {
			itemType = item.GetType()
		} else if itemType != item.GetType() {
			msg := fmt.Sprintf("List elements must be of the same type")
			p.typeErrors = append(p.typeErrors, checker.MakeError(msg, &innerNode))
			break
		}
	}
	listType := checker.ListType{ItemType: itemType}

	return ListLiteral{
		BaseNode: BaseNode{TSNode: node},
		Type:     listType,
		Items:    items,
	}, nil
}

func (p *Parser) parseListElement(node *tree_sitter.Node) (Expression, error) {
	switch node.GrammarName() {
	case "string":
		return StrLiteral{
			BaseNode: BaseNode{TSNode: node},
			Value:    p.text(node)}, nil
	case "number":
		return NumLiteral{
			BaseNode: BaseNode{TSNode: node},
			Value:    p.text(node)}, nil
	case "boolean":
		return BoolLiteral{
			BaseNode: BaseNode{TSNode: node},
			Value:    p.text(node) == "true"}, nil
	default:
		return nil, fmt.Errorf("Unhandled list element: %s", node.GrammarName())
	}
}

func (p *Parser) parseMapLiteral(node *tree_sitter.Node) (Expression, error) {
	entryNodes := node.ChildrenByFieldName("entry", p.tree.Walk())
	entries := make([]MapEntry, len(entryNodes))

	var valueType checker.Type

	receivedKeys := make(map[string]int, len(entryNodes))
	for i, entryNode := range entryNodes {
		key, value, err := p.parseMapEntry(&entryNode)
		if err != nil {
			return nil, err
		}
		if _, ok := receivedKeys[key]; ok {
			msg := fmt.Sprintf("Duplicate key '%s' in map", key)
			p.typeErrors = append(p.typeErrors, checker.MakeError(msg, &entryNode))
		} else {
			receivedKeys[key] = 0
		}

		if i == 0 {
			valueType = value.GetType()
		} else if valueType != value.GetType() {
			// msg := fmt.Sprintf("List elements must be of the same type")
			// p.typeErrors = append(p.typeErrors, checker.MakeError(msg, &entryNode))
			break
		}
		entries[i] = MapEntry{Key: key, Value: value}
	}
	mapType := checker.MapType{KeyType: checker.StrType, ValueType: valueType}

	return MapLiteral{
		BaseNode: BaseNode{TSNode: node},
		Type:     mapType,
		Entries:  entries,
	}, nil
}

func (p *Parser) parseMapEntry(node *tree_sitter.Node) (string, Expression, error) {
	keyNode := node.ChildByFieldName("key")
	key := p.text(keyNode)
	valueNode := node.ChildByFieldName("value")
	value, err := p.parsePrimitiveValue(valueNode)
	if err != nil {
		return key, nil, err
	}
	return key, value, nil
}

func (p *Parser) parseUnaryExpression(node *tree_sitter.Node) (Expression, error) {
	operatorNode := node.ChildByFieldName("operator")
	operandNode := node.ChildByFieldName("operand")

	operator := resolveOperator(operatorNode)
	if operator != Minus && operator != Bang {
		return nil, fmt.Errorf("Unsupported unary operator: %v", operatorNode.GrammarName())
	}

	operand, err := p.parseExpression(operandNode)
	if err != nil {
		return nil, err
	}

	switch operator {
	case Minus:
		if operand.GetType() != checker.NumType {
			p.unaryOperatorError(operatorNode, checker.NumType)
		}
	case Bang:
		if operand.GetType() != checker.BoolType {
			p.unaryOperatorError(operatorNode, checker.BoolType)
		}
	}

	return UnaryExpression{
		BaseNode: BaseNode{TSNode: node},
		Operator: operator,
		Operand:  operand,
	}, nil
}

func resolveOperator(node *tree_sitter.Node) Operator {
	switch node.GrammarName() {
	case "assign":
		return Assign
	case "minus":
		return Minus
	case "decrement":
		return Decrement
	case "plus":
		return Plus
	case "increment":
		return Increment
	case "divide":
		return Divide
	case "multiply":
		return Multiply
	case "modulo":
		return Modulo
	case "bang":
		return Bang
	case "greater_than":
		return GreaterThan
	case "greater_than_or_equal":
		return GreaterThanOrEqual
	case "less_than":
		return LessThan
	case "less_than_or_equal":
		return LessThanOrEqual
	case "equal":
		return Equal
	case "not_equal":
		return NotEqual
	case "or":
		return Or
	case "and":
		return And
	case "inclusive_range":
		return Range
	default:
		return InvalidOp
	}
}

func (p *Parser) parseBinaryExpression(node *tree_sitter.Node) (Expression, error) {
	leftNode := node.ChildByFieldName("left")
	operatorNode := node.ChildByFieldName("operator")
	rightNode := node.ChildByFieldName("right")

	left, err := p.parseExpression(leftNode)
	if err != nil {
		return nil, err
	}

	operator := resolveOperator(operatorNode)
	if operator == InvalidOp || operator == Bang {
		return nil, fmt.Errorf("Unsupported operator: %v", operator)
	}

	right, err := p.parseExpression(rightNode)
	if err != nil {
		return nil, err
	}

	switch operator {
	case Plus, Minus, Multiply, Divide, Modulo, GreaterThan, GreaterThanOrEqual, LessThan, LessThanOrEqual:
		if left.GetType() != checker.NumType || right.GetType() != checker.NumType {
			p.binaryOperatorError(node, p.text(operatorNode), checker.NumType)
		}
	case Equal, NotEqual:
		if left.GetType() != right.GetType() {
			p.equalityOperatorError(node, p.text(operatorNode))
		}
	case And, Or:
		if left.GetType() != checker.BoolType || right.GetType() != checker.BoolType {
			p.logicalOperatorError(node, p.text(operatorNode))
		}
	case Range:
		if left.GetType() != checker.NumType || right.GetType() != checker.NumType {
			msg := "A range must be between two Num"
			p.typeErrors = append(p.typeErrors, checker.MakeError(msg, operatorNode))
		}
	}

	if operator == Range {
		return RangeExpression{
			BaseNode: BaseNode{TSNode: node},
			Start:    left,
			End:      right,
		}, nil
	}

	return BinaryExpression{
		BaseNode: BaseNode{TSNode: node},
		Left:     left,
		Operator: operator,
		Right:    right,
	}, nil
}

func (p *Parser) parseMemberAccess(node *tree_sitter.Node) (Expression, error) {
	targetNode := p.mustChild(node, "target")
	operatorNode := node.ChildByFieldName("operator")
	memberNode := node.ChildByFieldName("member")

	target, err := p.parseExpression(targetNode)
	if err != nil {
		return nil, err
	}

	var accessType MemberAccessType
	switch operatorNode.GrammarName() {
	case "period":
		accessType = Instance
	case "double_colon":
		accessType = Static
	default:
		panic(fmt.Errorf("Unexpected member access operator: %s", operatorNode.GrammarName()))
	}

	switch target.GetType().(type) {
	case checker.EnumType:
		enum := target.GetType().(checker.EnumType)
		switch memberNode.GrammarName() {
		case "identifier":
			name := p.text(memberNode)
			if accessType == Static {
				if ok := enum.HasVariant(name); ok {
					return MemberAccess{
						Target:     target,
						AccessType: accessType,
						Member:     Identifier{Name: name, Type: target.GetType()},
					}, nil
				}
				msg := fmt.Sprintf("'%s' is not a variant of '%s' enum", name, enum.Name)
				p.typeErrors = append(p.typeErrors, checker.MakeError(msg, memberNode))
				return nil, fmt.Errorf(msg)
			}
			return nil, fmt.Errorf("Unsupported: instance members on enums")
		default:
			panic(fmt.Errorf("Unhandled member type on enum: %s", memberNode.GrammarName()))
		}
	case checker.StructType:
		structDef := target.GetType().(checker.StructType)
		switch memberNode.GrammarName() {
		case "identifier":
			name := p.text(memberNode)
			if accessType == Instance {
				if fieldType, ok := structDef.Fields[name]; ok {
					return MemberAccess{
						Target:     target,
						AccessType: accessType,
						Member:     Identifier{Name: name, Type: fieldType},
					}, nil
				} else {
					msg := fmt.Sprintf("No field '%s' in '%s' struct", name, structDef.Name)
					p.typeErrors = append(p.typeErrors, checker.MakeError(msg, memberNode))
					return nil, fmt.Errorf(msg)
				}
			}
			panic("Unimplemented: static members on structs")
		default:
			panic(fmt.Errorf("Unhandled member type on struct: %s", memberNode.GrammarName()))
		}
	case checker.ListType:
		listType := target.GetType().(checker.ListType)
		switch memberNode.GrammarName() {
		case "identifier":
			{
				name := p.text(memberNode)
				if accessType == Instance {
					property := listType.GetProperty(name)
					if property == nil {
						msg := fmt.Sprintf("No property '%s' on List", name)
						p.typeErrors = append(p.typeErrors, checker.MakeError(msg, memberNode))
						return nil, fmt.Errorf(msg)
					}

					return MemberAccess{
						Target:     target,
						AccessType: accessType,
						Member:     Identifier{Name: name, Type: property},
					}, nil
				} else {
					panic("Unimplemented: static members on List")
				}
			}
		case "function_call":
			call, err := p.parseFunctionCall(memberNode, &target)
			if err != nil {
				return nil, err
			}

			return MemberAccess{
				Target:     target,
				AccessType: accessType,
				Member:     call,
			}, nil
		default:
			panic(fmt.Errorf("Unhandled member type on list: %s", memberNode.GrammarName()))
		}
	case checker.PrimitiveType:
		prim := target.GetType().(checker.PrimitiveType)
		if prim.Name != "Str" {
			return MemberAccess{
				Target:     target,
				AccessType: accessType,
			}, nil
		}

		switch memberNode.GrammarName() {
		case "identifier":
			name := p.text(memberNode)
			if accessType == Instance {
				property := prim.GetProperty(name)
				if property == nil {
					msg := fmt.Sprintf("No property '%s' on %s", name, prim.Name)
					p.typeErrors = append(p.typeErrors, checker.MakeError(msg, memberNode))
					return nil, fmt.Errorf(msg)
				}

				return MemberAccess{
					Target:     target,
					AccessType: accessType,
					Member:     Identifier{Name: name, Type: property},
				}, nil
			} else {
				panic("Unimplemented: static members on Str")
			}
		default:
			panic(fmt.Errorf("Unhandled member type on Str: %s", memberNode.GrammarName()))
		}
	default:
		panic(fmt.Errorf("Unhandled target type for MemberAccess: %s", target.GetType()))
	}
}

/* look for a function in scope */
func (p *Parser) findFunction(name string) *checker.FunctionType {
	symbol := p.scope.Lookup(name)
	if symbol == nil {
		return nil
	}
	fnType, ok := symbol.GetType().(checker.FunctionType)
	if !ok {
		return nil
	}
	return &fnType
}

/* look for a method on a type */
func (p *Parser) findMethod(subject checker.Type, name string) *checker.FunctionType {
	switch subject.(type) {
	case checker.ListType:
		{
			method := subject.(checker.ListType).GetProperty(name)
			signature, ok := method.(checker.FunctionType)
			if !ok {
				return nil
			}
			return &signature
		}
	default:
		panic(fmt.Errorf("Unhandled method call on %s", subject))
	}
}

/*
@target - when parsing a method call
*/
func (p *Parser) parseFunctionCall(node *tree_sitter.Node, target *Expression) (FunctionCall, error) {
	targetNode := p.mustChild(node, "target")
	var signature checker.FunctionType
	if target == nil {
		if fn := p.findFunction(p.text(targetNode)); fn != nil {
			signature = *fn
		} else {
			return FunctionCall{}, p.undefinedSymbolError(node)
		}
	} else {
		if method := p.findMethod((*target).GetType(), p.text(targetNode)); method != nil {
			signature = *method
		} else {
			msg := fmt.Sprintf("Method '%s' not found on %s", p.text(targetNode), (*target).GetType())
			p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
			return FunctionCall{}, fmt.Errorf(msg)
		}
	}

	argsNode := node.ChildByFieldName("arguments")
	argNodes := argsNode.ChildrenByFieldName("argument", p.tree.Walk())

	if len(argNodes) != len(signature.Parameters) {
		msg := fmt.Sprintf("Expected %d arguments, got %d", len(signature.Parameters), len(argNodes))
		p.typeErrors = append(p.typeErrors, checker.MakeError(msg, argsNode))
		return FunctionCall{}, fmt.Errorf(msg)
	}

	args := make([]Expression, len(argNodes))
	for i, argNode := range argNodes {
		arg, err := p.parseExpression(&argNode)
		if err != nil {
			return FunctionCall{}, err
		}
		expectedType := signature.Parameters[i]
		resolvedArg := coerceArgIfNecessary(arg, expectedType)

		if !expectedType.Equals(resolvedArg) {
			p.typeMismatchError(&argNode, expectedType, resolvedArg)
		}
		args[i] = arg
	}

	if signature.Mutates {
		if identifier, is_identifier := (*target).(Identifier); is_identifier {
			symbol := p.scope.Lookup(identifier.Name)
			if v, ok := symbol.(checker.Variable); ok {
				if v.Mutable == false {
					msg := fmt.Sprintf("Cannot mutate an immutable list")
					p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
				}
			}
		}
	}

	return FunctionCall{
		BaseNode: BaseNode{TSNode: node},
		Name:     signature.GetName(),
		Args:     args,
		Type:     signature,
	}, nil
}

// if @arg is an anonymous function and @expectedType is a function
// it returns the generics coerced with the expected type.
//
// otherwise it returns the type of the argument
func coerceArgIfNecessary(arg Expression, expectedType checker.Type) checker.Type {
	anon, ok := arg.(AnonymousFunction)
	if !ok {
		return arg.GetType()
	}

	anonSignature := anon.GetType().(checker.FunctionType)

	signature, ok := expectedType.(checker.FunctionType)
	if !ok {
		return arg.GetType()
	}

	params := make([]checker.Type, len(anon.Parameters))
	for i, param := range anonSignature.Parameters {
		if _, isGeneric := param.(checker.GenericType); isGeneric {
			params[i] = signature.Parameters[i]
		} else {
			params[i] = param
		}
	}

	returnType := anon.ReturnType
	if _, isGeneric := returnType.(checker.GenericType); isGeneric {
		returnType = signature.ReturnType
	}

	return checker.FunctionType{
		Mutates:    false,
		Name:       anonSignature.Name,
		Parameters: params,
		ReturnType: returnType,
	}
}

func (p *Parser) parseMatchExpression(node *tree_sitter.Node) (Expression, error) {
	expressionNode := p.mustChild(node, "expr")
	caseNodes := p.mustChildren(node, "case")

	expression, err := p.parseExpression(expressionNode)
	if err != nil {
		return nil, err
	}

	switch expression.GetType().(type) {
	case checker.EnumType:
		enum := expression.GetType().(checker.EnumType)

		providedCases := make(map[string]int)
		cases := make([]MatchCase, 0)
		var resultType checker.Type = checker.VoidType
		for i, caseNode := range caseNodes {
			_case, err := p.parseMemberAccess(p.mustChild(&caseNode, "pattern"))
			if err != nil {
				return nil, err
			}
			var returnType checker.Type = checker.VoidType
			var body = make([]Statement, 0)
			bodyNode := p.mustChild(&caseNode, "body")
			if bodyNode.GrammarName() == "block" {
				_body, err := p.parseBlock(bodyNode)
				if err != nil {
					return nil, err
				}
				body = _body

				last := body[len(body)-1]
				if expr, ok := last.(Expression); ok {
					returnType = expr.GetType()
				}
			} else if bodyNode.GrammarName() == "expression" {
				_body, err := p.parseExpression(bodyNode)
				if err != nil {
					return nil, err
				}
				body = append(body, _body)
				returnType = _body.GetType()
			}

			memberAccess := _case.(MemberAccess)
			cases = append(cases, MatchCase{
				Pattern: memberAccess,
				Body:    body,
				Type:    returnType,
			})
			providedCases[memberAccess.Member.(Identifier).Name] = 0

			if i == 0 {
				resultType = returnType
			} else if resultType.Equals(returnType) == false {
				p.typeMismatchError(&caseNode, resultType, returnType)
			}
		}
		for _, variant := range enum.Variants {
			if _, ok := providedCases[variant]; !ok {
				msg := fmt.Sprintf("Missing case for '%s'", enum.FormatVariant(variant))
				p.typeErrors = append(p.typeErrors, checker.MakeError(msg, node))
			}
		}

		return MatchExpression{
			BaseNode: BaseNode{TSNode: node},
			Subject:  expression,
			Cases:    cases,
		}, nil
	default:
		panic(fmt.Sprintf("Unsupported subject type for match expression: %v", expression.GetType()))
	}
}

func (p *Parser) parseAnonymousFunction(node *tree_sitter.Node) (AnonymousFunction, error) {
	parameterNodes := node.ChildrenByFieldName("parameter", p.tree.Walk())
	parameters := make([]Parameter, len(parameterNodes))
	for i, paramNode := range parameterNodes {
		name := p.text(p.mustChild(&paramNode, "name"))
		var _type checker.Type
		typeNode := paramNode.ChildByFieldName("type")
		if typeNode == nil {
			_type = checker.GenericType{}
		} else {
			_type = p.resolveType(typeNode)
		}
		parameters[i] = Parameter{
			BaseNode: BaseNode{TSNode: &paramNode},
			Name:     name,
			Type:     _type,
		}
	}

	scope := p.pushScope()
	for _, param := range parameters {
		scope.Declare(checker.Variable{
			Mutable: false,
			Name:    param.Name,
			Type:    param.Type,
		})
	}
	body, err := p.parseBlock(p.mustChild(node, "body"))
	if err != nil {
		return AnonymousFunction{}, err
	}
	p.popScope()

	var returnType checker.Type = checker.VoidType
	if len(body) > 0 {
		last := body[len(body)-1]
		if expr, ok := last.(Expression); ok {
			returnType = expr.GetType()
		}
	}

	return AnonymousFunction{
		BaseNode:   BaseNode{TSNode: node},
		Parameters: parameters,
		Body:       body,
		ReturnType: returnType,
	}, nil
}
