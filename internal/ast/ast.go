package ast

import (
	"fmt"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// statements do not produce values
type Statement interface {
	String() string
	GetLocation() Location
}

// expressions produce values
type Expression interface {
	Statement
}

// the base struct for all AST nodes
type BaseNode struct {
	tsNode *tree_sitter.Node
}

func makeBaseNode(node *tree_sitter.Node) BaseNode {
	return BaseNode{tsNode: node}
}

type Point struct {
	Row uint
	Col uint
}

func (p Point) String() string {
	return fmt.Sprintf("[%d:%d]", p.Row, p.Col)
}

type Location struct {
	Start Point
	End   Point
}

func (l Location) String() string {
	return l.Start.String() + "-" + l.End.String()
}

func (b BaseNode) GetLocation() Location {
	_range := b.tsNode.Range()
	return Location{
		// tree-sitter start positions are 0-indexed
		Start: Point{Row: _range.StartPoint.Row + 1, Col: _range.StartPoint.Column + 1},
		End:   Point{Row: _range.EndPoint.Row, Col: _range.EndPoint.Column},
	}
}

type Import struct {
	BaseNode
	Path string
	Name string
}

func (p Import) String() string {
	return p.Name
}

type Program struct {
	BaseNode
	Imports    []Import
	Statements []Statement
}

type Break struct{ BaseNode }

func (b Break) String() string {
	return "break"
}

type Comment struct {
	BaseNode
	Value string
}

func (c Comment) String() string {
	return fmt.Sprintf("Comment(%s)", c.Value)
}

type VariableDeclaration struct {
	BaseNode
	Name    string
	Mutable bool
	Value   Expression
	Type    DeclaredType
}

type DeclaredType interface {
	GetName() string
	IsOptional() bool
	GetLocation() Location
}

type StringType struct {
	BaseNode
	optional bool
}

func (v StringType) IsOptional() bool {
	return v.optional
}

func (s StringType) GetName() string {
	return "String"
}

type IntType struct {
	BaseNode
	optional bool
}

func (s IntType) GetName() string {
	return "Int"
}

func (v IntType) IsOptional() bool {
	return v.optional
}

type FloatType struct {
	BaseNode
	optional bool
}

func (f FloatType) GetName() string {
	return "Float"
}
func (f FloatType) IsOptional() bool {
	return f.optional
}

type BooleanType struct {
	BaseNode
	optional bool
}

func (s BooleanType) GetName() string {
	return "Boolean"
}

func (v BooleanType) IsOptional() bool {
	return v.optional
}

type TypeDeclaration struct {
	BaseNode
	Name Identifier
	Type []DeclaredType
}

func (t TypeDeclaration) String() string {
	return fmt.Sprintf("TypeDeclaration(%s)", t.Name)
}

type List struct {
	BaseNode
	Element  DeclaredType
	optional bool
}

func (s List) GetName() string {
	return "List"
}

func (v List) IsOptional() bool {
	return v.optional
}

type Map struct {
	BaseNode
	Key      DeclaredType
	Value    DeclaredType
	optional bool
}

func (s Map) GetName() string {
	return "Map"
}

func (v Map) IsOptional() bool {
	return v.optional
}

type CustomType struct {
	BaseNode
	Name     string
	optional bool
}

func (u CustomType) GetName() string {
	return u.Name
}

func (u CustomType) IsOptional() bool {
	return u.optional
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
	Target   Expression
	Operator Operator
	Value    Expression
}

// impl interfaces
func (v VariableAssignment) String() string {
	return fmt.Sprintf("%v = %s", v.Target, v.Value)
}

type Parameter struct {
	BaseNode
	Name    string
	Type    DeclaredType
	Mutable bool
}

func (p Parameter) String() string {
	return p.Name
}

type FunctionDeclaration struct {
	BaseNode
	Name       string
	Parameters []Parameter
	ReturnType DeclaredType
	Body       []Statement
}

func (f FunctionDeclaration) String() string {
	return fmt.Sprintf("%s(%v) %s", f.Name, f.Parameters, f.ReturnType)
}

type AnonymousFunction struct {
	BaseNode
	Parameters []Parameter
	ReturnType DeclaredType
	Body       []Statement
}

func (a AnonymousFunction) String() string {
	return fmt.Sprintf("AnonymousFunction")
}

type StructDefinition struct {
	BaseNode
	Name   Identifier
	Fields []StructField
}

type StructField struct {
	Name Identifier
	Type DeclaredType
}

func (s StructDefinition) String() string {
	return fmt.Sprintf("StructDefinition(%s)", s.Name)
}

type ImplBlock struct {
	BaseNode
	Self    Parameter
	Methods []FunctionDeclaration
}

func (i ImplBlock) String() string {
	return fmt.Sprintf("ImplBlock(%s)", i.Self)
}

type StructValue struct {
	BaseNode
	Name  Identifier
	Value Expression
}

type StructInstance struct {
	BaseNode
	Name       Identifier
	Properties []StructValue
}

func (s StructInstance) String() string {
	return fmt.Sprintf("StructInstance(%s)", s.Name)
}

type EnumDefinition struct {
	BaseNode
	Name     string
	Variants []string
}

func (e EnumDefinition) String() string {
	return fmt.Sprintf("EnumDefinition(%s)", e.Name)
}

type WhileLoop struct {
	BaseNode
	Condition Expression
	Body      []Statement
}

func (w WhileLoop) String() string {
	return "while"
}

type RangeLoop struct {
	BaseNode
	Cursor Identifier
	Start  Expression
	End    Expression
	Body   []Statement
}

func (r RangeLoop) String() string {
	return fmt.Sprintf("for range %s..%s", r.Start, r.End)
}

type ForInLoop struct {
	BaseNode
	Cursor   Identifier
	Iterable Expression
	Body     []Statement
}

func (f ForInLoop) String() string {
	return fmt.Sprintf("for %s", f.Iterable)
}

type ForLoop struct {
	BaseNode
	Init        VariableDeclaration
	Condition   Expression
	Incrementer Statement
	Body        []Statement
}

func (f ForLoop) String() string {
	return fmt.Sprintf("for %s", f.Init)
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
}

func (f FunctionCall) String() string {
	return fmt.Sprintf("FunctionCall(%s)", f.Name)
}

type InstanceProperty struct {
	BaseNode
	Target   Expression
	Property Identifier
}

func (ip InstanceProperty) String() string {
	return fmt.Sprintf("%s.%s", ip.Target, ip.Property)
}

type InstanceMethod struct {
	BaseNode
	Target Expression
	Method FunctionCall
}

func (im InstanceMethod) String() string {
	return fmt.Sprintf("%s.%s", im.Target, im.Method)
}

type StaticProperty struct {
	BaseNode
	Target   Expression
	Property Identifier
}

func (s StaticProperty) String() string {
	return fmt.Sprintf("%s::%s", s.Target, s.Property)
}

type StaticFunction struct {
	BaseNode
	Target   Expression
	Function FunctionCall
}

func (s StaticFunction) String() string {
	return fmt.Sprintf("%s::%s", s.Target, s.Function)
}

type EnumAccess struct {
	BaseNode
	Enum    Identifier
	Variant Identifier
}

func (m EnumAccess) String() string {
	return fmt.Sprintf("EnumAccess(%s::%s)", m.Enum, m.Variant)
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
	Not
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

type BinaryExpression struct {
	BaseNode
	Operator      Operator
	Left, Right   Expression
	HasPrecedence bool
}

func (b BinaryExpression) String() string {
	return fmt.Sprintf("%v %v %v", b.Left, b.Operator, b.Right)
}

type RangeExpression struct {
	BaseNode
	Start, End Expression
}

func (b RangeExpression) String() string {
	return "RangeExpression"
}

type Identifier struct {
	BaseNode
	Name string
}

func (i Identifier) String() string {
	return fmt.Sprintf("Identifier(%s)", i.Name)
}

type StrLiteral struct {
	BaseNode
	Value string
}

func (s StrLiteral) String() string {
	return s.Value
}

type InterpolatedStr struct {
	BaseNode
	Chunks []Expression
}

func (i InterpolatedStr) String() string {
	return "InterpolatedStr"
}

type NumLiteral struct {
	BaseNode
	Value string
}

func (n NumLiteral) String() string {
	return n.Value
}

type BoolLiteral struct {
	BaseNode
	Value bool
}

// impl interfaces
func (b BoolLiteral) String() string {
	return fmt.Sprintf("%t", b.Value)
}

type ListLiteral struct {
	BaseNode
	Items []Expression
}

func (l ListLiteral) String() string {
	return "ListLiteral"
}

type MapEntry struct {
	Key   Expression
	Value Expression
}

type MapLiteral struct {
	BaseNode
	Entries []MapEntry
}

func (m MapLiteral) String() string {
	return fmt.Sprintf("MapLiteral { %v }", m.Entries)
}

type MatchExpression struct {
	BaseNode
	Subject Expression
	Cases   []MatchCase
}

func (m MatchExpression) String() string {
	return fmt.Sprintf("MatchExpression(%s)", m.Subject)
}

type MatchCase struct {
	BaseNode
	Pattern Expression
	Body    []Statement
}

func (m MatchCase) String() string {
	return fmt.Sprintf("MatchCase(%s)", m.Pattern)
}

type Parser struct {
	sourceCode []byte
	tree       *tree_sitter.Tree
}

func NewParser(sourceCode []byte, tree *tree_sitter.Tree) *Parser {
	return &Parser{sourceCode: sourceCode, tree: tree}
}

func (p *Parser) text(node *tree_sitter.Node) string {
	return string(p.sourceCode[node.StartByte():node.EndByte()])
}

func (p *Parser) mustChild(node *tree_sitter.Node, name string) *tree_sitter.Node {
	child := node.ChildByFieldName(name)
	if child == nil {
		panic(fmt.Errorf("Missing child: %s in `%s`", name, p.text(node)))
	}
	return child
}

func (p *Parser) mustChildren(node *tree_sitter.Node, name string) []tree_sitter.Node {
	children := node.ChildrenByFieldName(name, p.tree.Walk())
	if len(children) == 0 {
		panic(fmt.Errorf("Missing children: %s in `%s`", name, p.text(node)))
	}
	return children
}

func (p *Parser) sweepForError(node *tree_sitter.Node, minChildren int) error {
	if int(node.ChildCount()) != minChildren {
		for _, child := range node.Children(p.tree.Walk()) {
			if child.IsError() {
				point := child.Range().StartPoint
				return fmt.Errorf(
					"[%d, %d] Unexpected character: '%s'",
					point.Row,
					point.Column,
					p.text(&child))
			}
		}
	}
	return nil
}

func (p *Parser) Parse() (Program, error) {
	rootNode := p.tree.RootNode()
	program := &Program{
		BaseNode:   BaseNode{tsNode: rootNode},
		Imports:    []Import{},
		Statements: []Statement{},
	}

	for i := range rootNode.NamedChildCount() {
		switch rootNode.NamedChild(i).GrammarName() {
		case "statement":
			stmt, err := p.parseStatement(rootNode.NamedChild(i))
			if err != nil {
				return *program, err
			}
			if stmt != nil {
				program.Statements = append(program.Statements, stmt)
			}
		case "import":
			imp, err := p.parseImport(rootNode.NamedChild(i))
			if err != nil {
				return *program, err
			}
			program.Imports = append(program.Imports, imp)
		default:
			panic(fmt.Errorf("Unhandled root node: %s", rootNode.NamedChild(i).GrammarName()))
		}
	}

	return *program, nil
}

func (p *Parser) parseImport(node *tree_sitter.Node) (Import, error) {
	err := p.sweepForError(node, 2)
	if err != nil {
		return Import{}, err
	}

	pathNode := p.mustChild(node, "path")
	aliasNode := node.ChildByFieldName("alias")

	path := p.text(pathNode)
	var name string
	if aliasNode != nil {
		name = p.text(aliasNode)
	} else {
		parts := strings.Split(path, "/")
		if len(parts) == 1 {
			name = parts[0]
		} else {
			name = parts[len(parts)-1]
		}
		name = strings.ReplaceAll(name, "-", "_")
	}

	return Import{
		BaseNode: BaseNode{tsNode: node},
		Name:     name,
		Path:     path,
	}, nil
}

func (p *Parser) parseStatement(node *tree_sitter.Node) (Statement, error) {
	p.sweepForError(node, 1)
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
	case "for_in_loop":
		return p.parseForInLoop(child)
	case "for_loop":
		return p.parseForLoop(child)
	case "if_statement":
		return p.parseIfStatement(child)
	case "struct_definition":
		return p.parseStructDefinition(child)
	case "implements_definition":
		return p.parseImplBlock(child)
	case "enum_definition":
		return p.parseEnumDefinition(child)
	case "expression":
		expr, err := p.parseExpression(child)
		if err != nil {
			return nil, err
		}
		return expr, nil
	case "comment":
		return Comment{
			BaseNode: BaseNode{tsNode: node},
			Value:    p.text(node),
		}, nil
	case "break":
		return Break{BaseNode: BaseNode{tsNode: node}}, nil
	case "type_declaration":
		nameNode := p.mustChild(child, "name")
		name := p.text(nameNode)
		types := []DeclaredType{}
		for _, n := range child.NamedChild(2).NamedChildren(p.tree.Walk()) {
			types = append(types, p.resolveType(&n))
		}

		return TypeDeclaration{
			BaseNode: makeBaseNode(child),
			Name:     Identifier{BaseNode: makeBaseNode(nameNode), Name: name},
			Type:     types,
		}, nil
	default:
		return nil, fmt.Errorf("Unhandled statement: %s", child.GrammarName())
	}
}

func (p *Parser) parseVariableDecl(node *tree_sitter.Node) (VariableDeclaration, error) {
	p.sweepForError(node, 3)

	isMutable := p.text(node.NamedChild(0)) == "mut"
	name := p.text(node.NamedChild(1))
	declaredType := p.resolveType(node.ChildByFieldName("type"))
	value, err := p.parseExpression(node.ChildByFieldName("value"))
	if err != nil {
		return VariableDeclaration{}, err
	}

	return VariableDeclaration{
		BaseNode: BaseNode{tsNode: node},
		Mutable:  isMutable,
		Name:     name,
		Value:    value,
		Type:     declaredType,
	}, nil
}

// use for resolving explicit type declarations
func (p *Parser) resolveType(node *tree_sitter.Node) DeclaredType {
	if node == nil {
		return nil
	}
	child := node.NamedChild(0)
	optional := node.ChildByFieldName("optional") != nil
	switch child.GrammarName() {
	case "primitive_type":
		{
			text := p.text(child)
			switch text {
			case "Str":
				return StringType{makeBaseNode(child), optional}
			case "Int":
				return IntType{makeBaseNode(child), optional}
			case "Float":
				return FloatType{makeBaseNode(child), optional}
			case "Bool":
				return BooleanType{makeBaseNode(child), optional}
			default:
				panic(fmt.Errorf("Unresolved primitive type: %s", text))
			}
		}
	case "list_type":
		element_typeNode := p.mustChild(child, "element_type")
		return List{BaseNode: makeBaseNode(child), Element: p.resolveType(element_typeNode), optional: optional}
	case "map_type":
		keyNode := p.mustChild(child, "key")
		valueNode := p.mustChild(child, "value")
		return Map{
			Key:      p.resolveType(keyNode),
			Value:    p.resolveType(valueNode),
			optional: optional,
		}
	case "identifier":
		return CustomType{makeBaseNode(child), p.text(child), optional}
	default:
		panic(fmt.Errorf("Couldn't resolve type for grammar name: %v", child.GrammarName()))
	}
}

func (p *Parser) parseVariableReassignment(node *tree_sitter.Node) (VariableAssignment, error) {
	p.sweepForError(node, 3)
	targetNode := p.mustChild(node, "target")
	operatorNode := node.ChildByFieldName("operator")
	valueNode := node.ChildByFieldName("value")

	var target Expression
	if targetNode.GrammarName() == "identifier" {
		target = Identifier{BaseNode: makeBaseNode(targetNode), Name: p.text(targetNode)}
	} else {
		member, err := p.parseMemberAccess(targetNode)
		if err != nil {
			return VariableAssignment{}, err
		}
		target = member
	}
	operator := resolveOperator(operatorNode)

	value, err := p.parseExpression(valueNode)
	if err != nil {
		return VariableAssignment{}, err
	}

	return VariableAssignment{
		BaseNode: makeBaseNode(node),
		Target:   target,
		Operator: operator,
		Value:    value,
	}, nil
}

func (p *Parser) parseFunctionDecl(node *tree_sitter.Node) (FunctionDeclaration, error) {
	p.sweepForError(node, 2)
	name := p.text(p.mustChild(node, "name"))
	parameters := p.parseParameters(p.mustChild(node, "parameters"))
	returnType := p.resolveType(node.ChildByFieldName("return"))

	body, err := p.parseBlock(node.ChildByFieldName("body"))

	if err != nil {
		return FunctionDeclaration{}, err
	}

	return FunctionDeclaration{
		BaseNode:   BaseNode{tsNode: node},
		Name:       name,
		Parameters: parameters,
		ReturnType: returnType,
		Body:       body,
	}, nil
}

func (p *Parser) parseParameters(node *tree_sitter.Node) []Parameter {
	p.sweepForError(node, 0)
	parameterNodes := node.ChildrenByFieldName("parameter", p.tree.Walk())
	parameters := []Parameter{}

	for _, node := range parameterNodes {
		bindingNode := node.ChildByFieldName("binding")
		parameters = append(parameters, Parameter{
			BaseNode: BaseNode{tsNode: &node},
			Name:     p.text(node.ChildByFieldName("name")),
			Type:     p.resolveType(node.ChildByFieldName("type")),
			Mutable:  bindingNode != nil,
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
	p.sweepForError(node, 2)
	conditionNode := node.ChildByFieldName("condition")
	bodyNode := node.ChildByFieldName("body")

	condition, err := p.parseExpression(conditionNode)
	if err != nil {
		return nil, err
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

func (p *Parser) parseForInLoop(node *tree_sitter.Node) (Statement, error) {
	p.sweepForError(node, 3)
	cursorNode := p.mustChild(node, "cursor")

	rangeNode := node.ChildByFieldName("range")
	iterable, err := p.parseExpression(rangeNode)
	if err != nil {
		return nil, err
	}

	bodyNode := node.ChildByFieldName("body")
	body, err := p.parseBlock(bodyNode)

	if r, ok := iterable.(RangeExpression); ok {
		if err != nil {
			return nil, err
		}

		return RangeLoop{
			BaseNode: BaseNode{tsNode: node},
			Cursor:   Identifier{BaseNode: makeBaseNode(cursorNode), Name: p.text(cursorNode)},
			Start:    r.Start,
			End:      r.End,
			Body:     body,
		}, nil
	}

	return ForInLoop{
		BaseNode: BaseNode{tsNode: node},
		Cursor:   Identifier{BaseNode: makeBaseNode(cursorNode), Name: p.text(cursorNode)},
		Iterable: iterable,
		Body:     body,
	}, nil
}

func (p *Parser) parseForLoop(node *tree_sitter.Node) (Statement, error) {
	p.sweepForError(node, 4)

	cursorNode := p.mustChild(node, "cursor")
	cursor, err := p.parseVariableDecl(cursorNode)
	if err != nil {
		return nil, err
	}

	conditionNode := p.mustChild(node, "condition")
	condition, err := p.parseExpression(conditionNode)
	if err != nil {
		return nil, err
	}

	incrementerNode := p.mustChild(node, "step")
	incrementer, err := p.parseStatement(incrementerNode)
	if err != nil {
		return nil, err
	}

	bodyNode := p.mustChild(node, "body")
	body, err := p.parseBlock(bodyNode)
	if err != nil {
		return nil, err
	}

	return ForLoop{
		BaseNode:    BaseNode{tsNode: node},
		Init:        cursor,
		Condition:   condition,
		Incrementer: incrementer,
		Body:        body,
	}, nil
}

func (p *Parser) parseIfStatement(node *tree_sitter.Node) (Statement, error) {
	p.sweepForError(node, 2)
	conditionNode := node.ChildByFieldName("condition")
	bodyNode := node.ChildByFieldName("body")
	elseNode := node.ChildByFieldName("else")

	condition, err := p.parseExpression(conditionNode)
	if err != nil {
		return nil, err
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
			BaseNode:  BaseNode{tsNode: node},
			Condition: condition,
			Body:      body,
			Else:      clause,
		}, nil
	}

	return IfStatement{
		BaseNode:  BaseNode{tsNode: node},
		Condition: condition,
		Body:      body,
	}, nil
}

func (p *Parser) parseElseClause(node *tree_sitter.Node) (Statement, error) {
	p.sweepForError(node, 1)

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
		Condition: nil,
		BaseNode:  BaseNode{tsNode: node},
		Body:      body,
	}, nil
}

func (p *Parser) parseStructDefinition(node *tree_sitter.Node) (Statement, error) {
	p.sweepForError(node, 1)
	nameNode := p.mustChild(node, "name")
	fieldNodes := node.ChildrenByFieldName("field", p.tree.Walk())

	fields := make([]StructField, len(fieldNodes))
	for i, fieldNode := range fieldNodes {
		nameNode := fieldNode.ChildByFieldName("name")
		name := p.text(nameNode)
		typeNode := fieldNode.ChildByFieldName("type")
		fields[i] = StructField{
			Name: Identifier{BaseNode: makeBaseNode(nameNode), Name: name},
			Type: p.resolveType(typeNode),
		}
	}

	strct := StructDefinition{
		BaseNode: BaseNode{node},
		Name:     Identifier{BaseNode: makeBaseNode(nameNode), Name: p.text(nameNode)},
		Fields:   fields,
	}
	return strct, nil
}

func (p *Parser) parseImplBlock(node *tree_sitter.Node) (Statement, error) {
	p.sweepForError(node, 2)

	params := p.parseParameters(node.NamedChild(0))
	if len(params) != 1 {
		return nil, fmt.Errorf("Expected 1 parameter in impl block, got: %d", len(params))
	}
	bodyNode := node.NamedChild(1)
	defNodes := bodyNode.NamedChildren(node.Walk())
	methods := make([]FunctionDeclaration, len(defNodes))
	for i := range defNodes {
		stmt, err := p.parseStatement(&defNodes[i])
		if err != nil {
			return nil, err
		}
		def, ok := stmt.(FunctionDeclaration)
		if !ok {
			return nil, fmt.Errorf("Expected function declaration, got: %s", stmt)
		}
		methods[i] = def
	}

	return ImplBlock{
		BaseNode: makeBaseNode(node),
		Self:     params[0],
		Methods:  methods,
	}, nil
}

func (p *Parser) parseStructInstance(node *tree_sitter.Node) (Expression, error) {
	p.sweepForError(node, 2)
	nameNode := node.ChildByFieldName("name")
	fieldNodes := node.ChildrenByFieldName("field", p.tree.Walk())

	properties := make([]StructValue, len(fieldNodes))
	for i, propertyNode := range fieldNodes {
		nameNode := propertyNode.ChildByFieldName("name")
		name := p.text(nameNode)

		valueNode := propertyNode.ChildByFieldName("value")
		value, err := p.parseExpression(valueNode)
		if err != nil {
			return nil, err
		}

		properties[i] = StructValue{
			BaseNode: BaseNode{tsNode: &propertyNode},
			Name:     Identifier{BaseNode: makeBaseNode(nameNode), Name: name},
			Value:    value,
		}
	}

	return StructInstance{
		BaseNode:   BaseNode{tsNode: node},
		Name:       Identifier{BaseNode: makeBaseNode(nameNode), Name: p.text(nameNode)},
		Properties: properties,
	}, nil
}

func (p *Parser) parseEnumDefinition(node *tree_sitter.Node) (Statement, error) {
	p.sweepForError(node, 2)
	nameNode := p.mustChild(node, "name")
	variantNodes := node.ChildrenByFieldName("variant", p.tree.Walk())

	if node.HasError() {
		if len(p.text(&variantNodes[0])) == 0 {
			return EnumDefinition{
				BaseNode: BaseNode{tsNode: node},
				Name:     p.text(nameNode),
			}, nil
		}
		panic(fmt.Errorf("Parsing error encountered: %s", p.text(node)))
	}

	variants := make([]string, len(variantNodes))
	names := make(map[string]int8)
	for i, variantNode := range variantNodes {
		nameNode := variantNode.NamedChild(0)
		name := p.text(nameNode)
		names[name] = 0
		variants[i] = name
	}

	enum := EnumDefinition{
		BaseNode: BaseNode{tsNode: node},
		Name:     p.text(nameNode),
		Variants: variants,
	}
	return enum, nil
}

func (p *Parser) parseExpression(node *tree_sitter.Node) (Expression, error) {
	p.sweepForError(node, 1)

	child := node.Child(0)
	switch child.GrammarName() {
	case "expression":
		return p.parseExpression(child)
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
		return p.parseFunctionCall(child)
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

	return Identifier{
		BaseNode: makeBaseNode(node),
		Name:     name,
	}, nil
}

func (p *Parser) parsePrimitiveValue(node *tree_sitter.Node) (Expression, error) {
	p.sweepForError(node, 1)
	child := node.Child(0)
	switch child.GrammarName() {
	case "string":
		chunkNodes := child.ChildrenByFieldName("chunk", p.tree.Walk())
		if len(chunkNodes) == 1 && chunkNodes[0].GrammarName() == "string_content" {
			return StrLiteral{
				BaseNode: makeBaseNode(node),
				Value:    p.text(node)}, nil
		}

		chunks := make([]Expression, len(chunkNodes))
		for i, chunkNode := range chunkNodes {
			if chunkNode.GrammarName() == "string_content" {
				chunks[i] = StrLiteral{BaseNode: makeBaseNode(&chunkNode), Value: p.text(&chunkNode)}
			} else {
				chunk, err := p.parseExpression(p.mustChild(&chunkNode, "expression"))
				if err != nil {
					return nil, err
				}
				chunks[i] = chunk
			}
		}
		return InterpolatedStr{
			BaseNode: makeBaseNode(node),
			Chunks:   chunks,
		}, nil
	case "number":
		return NumLiteral{
			BaseNode: makeBaseNode(node),
			Value:    p.text(child)}, nil
	case "boolean":
		return BoolLiteral{
			BaseNode: makeBaseNode(node),
			Value:    p.text(child) == "true"}, nil
	default:
		return nil, fmt.Errorf("Unhandled primitive node: %s", child.GrammarName())
	}
}

func (p *Parser) parseListValue(node *tree_sitter.Node) (Expression, error) {
	elementNodes := node.ChildrenByFieldName("element", p.tree.Walk())
	items := make([]Expression, len(elementNodes))

	for i, innerNode := range elementNodes {
		item, err := p.parseListElement(&innerNode)
		if err != nil {
			return nil, err
		}
		items[i] = item
	}

	return ListLiteral{
		BaseNode: BaseNode{tsNode: node},
		Items:    items,
	}, nil
}

func (p *Parser) parseListElement(node *tree_sitter.Node) (Expression, error) {
	switch node.GrammarName() {
	case "string":
		return StrLiteral{
			BaseNode: BaseNode{tsNode: node},
			Value:    p.text(node)}, nil
	case "number":
		return NumLiteral{
			BaseNode: BaseNode{tsNode: node},
			Value:    p.text(node)}, nil
	case "boolean":
		return BoolLiteral{
			BaseNode: BaseNode{tsNode: node},
			Value:    p.text(node) == "true"}, nil
	case "struct_instance":
		return p.parseStructInstance(node)
	case "identifier":
		return Identifier{
			BaseNode: makeBaseNode(node),
			Name:     p.text(node),
		}, nil
	default:
		return nil, fmt.Errorf("Unhandled list element: %s", node.GrammarName())
	}
}

func (p *Parser) parseMapLiteral(node *tree_sitter.Node) (Expression, error) {
	entryNodes := node.ChildrenByFieldName("entry", p.tree.Walk())
	entries := make([]MapEntry, len(entryNodes))

	receivedKeys := make(map[Expression]int, len(entryNodes))
	for i, entryNode := range entryNodes {
		key, value, err := p.parseMapEntry(&entryNode)
		if err != nil {
			return nil, err
		}
		receivedKeys[key] = 0
		entries[i] = MapEntry{Key: key, Value: value}
	}

	return MapLiteral{
		BaseNode: BaseNode{tsNode: node},
		Entries:  entries,
	}, nil
}

func (p *Parser) parseMapEntry(node *tree_sitter.Node) (Expression, Expression, error) {
	p.sweepForError(node, 2)
	keyNode := node.ChildByFieldName("key")
	key, err := p.parsePrimitiveValue(keyNode)
	if err != nil {
		return nil, nil, err
	}
	valueNode := node.ChildByFieldName("value")
	value, err := p.parsePrimitiveValue(valueNode)
	if err != nil {
		return key, nil, err
	}
	return key, value, nil
}

func (p *Parser) parseUnaryExpression(node *tree_sitter.Node) (Expression, error) {
	p.sweepForError(node, 2)
	operatorNode := node.ChildByFieldName("operator")
	operandNode := node.ChildByFieldName("operand")

	operator := resolveOperator(operatorNode)
	if operator != Minus && operator != Not {
		return nil, fmt.Errorf("Unsupported unary operator: %v", operatorNode.GrammarName())
	}

	operand, err := p.parseExpression(operandNode)
	if err != nil {
		return nil, err
	}

	return UnaryExpression{
		BaseNode: BaseNode{tsNode: node},
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
	case "not":
		return Not
	case "inclusive_range":
		return Range
	default:
		return InvalidOp
	}
}

func (p *Parser) parseBinaryExpression(node *tree_sitter.Node) (Expression, error) {
	p.sweepForError(node, 3)
	leftNode := node.ChildByFieldName("left")
	operatorNode := node.ChildByFieldName("operator")
	rightNode := node.ChildByFieldName("right")

	left, err := p.parseExpression(leftNode)
	if err != nil {
		return nil, err
	}

	operator := resolveOperator(operatorNode)
	if operator == InvalidOp || operator == Not {
		return nil, fmt.Errorf("Unsupported operator: %v", operator)
	}

	right, err := p.parseExpression(rightNode)
	if err != nil {
		return nil, err
	}

	if operator == Range {
		return RangeExpression{
			BaseNode: BaseNode{tsNode: node},
			Start:    left,
			End:      right,
		}, nil
	}

	return BinaryExpression{
		BaseNode: BaseNode{tsNode: node},
		Left:     left,
		Operator: operator,
		Right:    right,
	}, nil
}

func (p *Parser) parseMemberAccess(node *tree_sitter.Node) (Expression, error) {
	p.sweepForError(node, 3)
	targetNode := p.mustChild(node, "target")
	target, err := p.parseExpression(targetNode)
	if err != nil {
		return nil, err
	}

	operatorNode := p.mustChild(node, "operator")
	memberNode := p.mustChild(node, "member")

	switch operatorNode.GrammarName() {
	case "period":
		switch memberNode.GrammarName() {
		case "identifier":
			return InstanceProperty{
				BaseNode: makeBaseNode(node),
				Target:   target,
				Property: Identifier{BaseNode: makeBaseNode(memberNode), Name: p.text(memberNode)},
			}, nil
		case "function_call":
			method, err := p.parseFunctionCall(memberNode)
			if err != nil {
				return nil, err
			}
			return InstanceMethod{
				BaseNode: makeBaseNode(node),
				Target:   target,
				Method:   method,
			}, nil
		default:
			panic(fmt.Errorf("Unexpected member node: %s", memberNode.GrammarName()))
		}

	case "double_colon":
		switch memberNode.GrammarName() {
		case "identifier":
			return StaticProperty{
				BaseNode: makeBaseNode(node),
				Target:   target,
				Property: Identifier{BaseNode: makeBaseNode(memberNode), Name: p.text(memberNode)},
			}, nil
		case "function_call":
			function, err := p.parseFunctionCall(memberNode)
			if err != nil {
				return nil, err
			}
			return StaticFunction{
				BaseNode: makeBaseNode(node),
				Target:   target,
				Function: function,
			}, nil
		default:
			panic(fmt.Errorf("Unexpected member node: %s", memberNode.GrammarName()))
		}
	default:
		panic(fmt.Errorf("Unexpected member access operator: %s", operatorNode.GrammarName()))
	}
}

/*
@target - when parsing a method call
*/
func (p *Parser) parseFunctionCall(node *tree_sitter.Node) (FunctionCall, error) {
	p.sweepForError(node, 2)
	targetNode := p.mustChild(node, "target")

	argsNode := node.ChildByFieldName("arguments")
	argNodes := argsNode.ChildrenByFieldName("argument", p.tree.Walk())

	args := make([]Expression, len(argNodes))
	for i, argNode := range argNodes {
		arg, err := p.parseExpression(&argNode)
		if err != nil {
			return FunctionCall{}, err
		}

		args[i] = arg
	}

	return FunctionCall{
		BaseNode: BaseNode{tsNode: node},
		Name:     p.text(targetNode),
		Args:     args,
	}, nil
}

func (p *Parser) parseMatchExpression(node *tree_sitter.Node) (Expression, error) {
	p.sweepForError(node, 2)
	expressionNode := p.mustChild(node, "expr")
	caseNodes := p.mustChildren(node, "case")

	expression, err := p.parseExpression(expressionNode)
	if err != nil {
		return nil, err
	}
	cases := make([]MatchCase, len(caseNodes))

	for i, caseNode := range caseNodes {
		c, err := p.parseMatchCase(&caseNode)
		if err != nil {
			return nil, err
		}
		cases[i] = c
	}

	return MatchExpression{
		BaseNode: BaseNode{tsNode: node},
		Subject:  expression,
		Cases:    cases,
	}, nil
}

func (p *Parser) parseMatchCase(node *tree_sitter.Node) (MatchCase, error) {
	patternNode := p.mustChild(node, "pattern")
	var pattern Expression
	switch patternNode.GrammarName() {
	case "member_access":
		p, err := p.parseMemberAccess(patternNode)
		if err != nil {
			return MatchCase{}, err
		}
		pattern = p
	case "primitive_value":
		p, err := p.parsePrimitiveValue(patternNode)
		if err != nil {
			return MatchCase{}, err
		}
		pattern = p
	case "identifier":
		p, err := p.parseIdentifier(patternNode)
		if err != nil {
			return MatchCase{}, err
		}
		pattern = p
	default:
		panic(fmt.Errorf("Unhandled match pattern grammar: %s", patternNode.GrammarName()))
	}

	bodyNode := p.mustChild(node, "body")
	body := []Statement{}
	if bodyNode.GrammarName() == "block" {
		b, err := p.parseBlock(bodyNode)
		if err != nil {
			return MatchCase{}, err
		}
		body = b
	} else {
		exp, err := p.parseExpression(bodyNode)
		if err != nil {
			return MatchCase{}, err
		}
		body = []Statement{exp}
	}

	return MatchCase{
		BaseNode: makeBaseNode(node),
		Pattern:  pattern,
		Body:     body,
	}, nil
}

// todo: still necessary?
func (p *Parser) parseAnonymousFunction(node *tree_sitter.Node) (AnonymousFunction, error) {
	parameterNodes := node.ChildrenByFieldName("parameter", p.tree.Walk())
	returnNode := node.ChildByFieldName("return")
	returnType := p.resolveType(returnNode)
	parameters := make([]Parameter, len(parameterNodes))
	for i, paramNode := range parameterNodes {
		name := p.text(p.mustChild(&paramNode, "name"))
		parameters[i] = Parameter{
			BaseNode: BaseNode{tsNode: &paramNode},
			Name:     name,
			Type:     p.resolveType(p.mustChild(&paramNode, "type")),
		}
	}

	body, err := p.parseBlock(p.mustChild(node, "body"))
	if err != nil {
		return AnonymousFunction{}, err
	}

	return AnonymousFunction{
		BaseNode:   BaseNode{tsNode: node},
		Parameters: parameters,
		Body:       body,
		ReturnType: returnType,
	}, nil
}
