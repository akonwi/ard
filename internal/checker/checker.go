package checker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/akonwi/ard/internal/ast"
	ts "github.com/tree-sitter/go-tree-sitter"
)

type DiagnosticKind string

const (
	Error DiagnosticKind = "error"
	Warn  DiagnosticKind = "warn"
)

type Diagnostic struct {
	Kind     DiagnosticKind
	Message  string
	location ts.Range
}

func (d Diagnostic) String() string {
	pos := d.location.StartPoint
	return fmt.Sprintf("[%d:%d] %s", pos.Row+1, pos.Column+1, d.Message)
}

type Program struct {
	Imports    map[string]Package
	Statements []Statement
}

// doubles as a symbol
type Package struct {
	name string
	Path string
}

// Package impl symbol
func (p Package) GetName() string {
	return p.name
}
func (p Package) GetType() Type {
	return p
}
func (p Package) asFunction() (function, bool) {
	return function{}, false
}

// Package impl Type
func (p Package) String() string {
	return "package " + p.name + " " + p.Path
}
func (p Package) GetProperty(name string) Type {
	if p.Path == "std/io" {
		switch name {
		case "print":
			return function{
				name:       name,
				parameters: []variable{{name: "string", mut: false, _type: Str{}}},
				returns:    Void{},
			}
		default:
			return nil
		}
	}
	return nil
}
func (p Package) Is(other Type) bool {
	return p.String() == other.String()
}

// Expressions produce something, therefore they have a Type
type Expression interface {
	GetType() Type
}

type StrLiteral struct {
	Value string
}

func (s StrLiteral) String() string {
	return `"` + s.Value + `"`
}
func (s StrLiteral) GetType() Type {
	return Str{}
}

type NumLiteral struct {
	Value int
}

func (n NumLiteral) GetType() Type {
	return Num{}
}

type BoolLiteral struct {
	Value bool
}

func (b BoolLiteral) GetType() Type {
	return Bool{}
}

type ListLiteral struct {
	Elements []Expression
	_type    List
}

func (l ListLiteral) GetType() Type {
	return l._type
}

type TupleLiteral struct {
	Elements []Expression
	// _type    List
}

func (l TupleLiteral) GetType() Type {
	return nil
}

type Negation struct {
	Value Expression
}

func (n Negation) GetType() Type {
	return Num{}
}

type Not struct {
	Value Expression
}

func (n Not) GetType() Type {
	return Bool{}
}

type BinaryOperator string

const (
	Add                BinaryOperator = "+"
	Sub                               = "-"
	Div                               = "/"
	Mul                               = "*"
	Mod                               = "%"
	Equal                             = "=="
	NotEqual                          = "!="
	GreaterThan                       = ">"
	GreaterThanOrEqual                = ">="
	LessThan                          = "<"
	LessThanOrEqual                   = "<="
	And                               = "and"
	Or                                = "or"
)

type BinaryExpr struct {
	Op    BinaryOperator
	Left  Expression
	Right Expression
}

func (b BinaryExpr) GetType() Type {
	switch b.Op {
	case Add, Sub, Div, Mul, Mod:
		return Num{}
	default:
		return Bool{}
	}
}

type Identifier struct {
	Name   string
	symbol symbol
}

func (i Identifier) String() string {
	return i.Name
}

type InstanceProperty struct {
	Subject  Expression
	Property Expression
}

func (i InstanceProperty) GetType() Type {
	return i.Property.GetType()
}

func (i Identifier) GetType() Type {
	return i.symbol.GetType()
}

type PackageAccess struct {
	Package  Package
	Property Expression
}

func (p PackageAccess) GetType() Type {
	return p.Property.GetType()
}

type InterpolatedStr struct {
	Parts []Expression
}

func (i InterpolatedStr) GetType() Type {
	return Str{}
}

type FunctionLiteral struct {
	Parameters []Parameter
	Return     Type
	Body       []Statement
}

func (f FunctionLiteral) GetType() Type {
	params := make([]variable, len(f.Parameters))
	for i, p := range f.Parameters {
		params[i] = variable{
			name:  p.Name,
			mut:   false,
			_type: p.Type,
		}
	}
	return function{
		name:       "",
		parameters: params,
		returns:    f.Return,
	}
}

type MatchCase struct {
	Pattern Expression
	Body    []Statement
	_type   Type
}

type MatchExpr struct {
	Subject Expression
	Cases   []MatchCase
}

func (m MatchExpr) GetType() Type {
	return m.Cases[0]._type
}

// Statements don't produce anything
type Statement interface{}

type VariableBinding struct {
	Name  string
	Value Expression
	Mut   bool
}

type VariableAssignment struct {
	Name  string
	Value Expression
}

type IfStatement struct {
	Condition Expression
	Body      []Statement
	Else      Statement
}

type ForRange struct {
	Cursor Identifier
	Start  Expression
	End    Expression
	Body   []Statement
}

type ForIn struct {
	Cursor   Identifier
	Iterable Expression
	Body     []Statement
}

type WhileLoop struct {
	Condition Expression
	Body      []Statement
}

type Parameter struct {
	Name string
	Type Type
}

type FunctionDeclaration struct {
	Name       string
	Parameters []Parameter
	Body       []Statement
	Return     Type
}

func (f FunctionDeclaration) GetType() Type {
	params := make([]variable, len(f.Parameters))
	for i, p := range f.Parameters {
		params[i] = variable{
			name:  p.Name,
			mut:   false,
			_type: p.Type,
		}
	}
	return function{
		name:       f.Name,
		parameters: params,
		returns:    f.Return,
	}
}

func (e Enum) GetType() Type {
	return e
}

type FunctionCall struct {
	Name   string
	Args   []Expression
	symbol function
}

func (f FunctionCall) GetType() Type {
	return f.symbol.returns
}

// tree-sitter uses 0 based positioning
func startPointString(node *ts.Node) string {
	pos := node.StartPosition()
	return fmt.Sprintf("[%d:%d]", pos.Row+1, pos.Column+1)
}

type checker struct {
	diagnostics []Diagnostic
	imports     map[string]Package
	scope       *scope
}

func (c *checker) addDiagnostic(d Diagnostic) {
	c.diagnostics = append(c.diagnostics, d)
}

func Check(program ast.Program) (Program, []Diagnostic) {
	checker := checker{
		diagnostics: []Diagnostic{},
		imports:     map[string]Package{},
		scope:       newScope(nil),
	}
	statements := []Statement{}

	for _, imp := range program.Imports {
		if _, ok := checker.imports[imp.Name]; !ok {
			pkg := Package{Path: imp.Path, name: imp.Name}
			checker.imports[imp.Name] = pkg
			checker.scope.declare(pkg)
		} else {
			checker.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Duplicate package name: %s", startPointString(imp.TSNode), imp.Name),
				location: imp.TSNode.Range(),
			})
		}
	}

	for _, stmt := range program.Statements {
		statement := checker.checkStatement(stmt)
		if statement != nil {
			statements = append(statements, statement)
		}
	}

	return Program{
		Imports:    checker.imports,
		Statements: statements,
	}, checker.diagnostics
}

func (c *checker) checkStatement(stmt ast.Statement) Statement {
	switch s := stmt.(type) {
	case ast.VariableDeclaration:
		value := c.checkExpression(s.Value)
		var _type Type = Void{}
		// get declared type if it exists
		if s.Type != nil {
			_type := c.resolveDeclaredType(s.Type)
			if _type.Is(value.GetType()) == false {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("%s Type mismatch: Expected %s, got %s", startPointString(s.Value.GetTSNode()), _type, value.GetType()),
					location: s.Value.GetTSNode().Range(),
				})
				return nil
			}
		} else if list, isList := value.GetType().(List); isList && list.element == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Empty lists need an explicit type", startPointString(s.Value.GetTSNode())),
				location: s.Value.GetTSNode().Range(),
			})
			return nil
		}
		// if no declared type, use the type of the value
		if _type == (Void{}) {
			_type = value.GetType()
		}

		c.scope.addVariable(variable{name: s.Name, mut: s.Mutable, _type: _type})
		return VariableBinding{Name: s.Name, Value: value, Mut: s.Mutable}
	case ast.VariableAssignment:
		symbol := c.scope.find(s.Name)
		if symbol == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Undefined: %s", startPointString(s.GetTSNode()), s.Name),
				location: s.TSNode.Range(),
			})
			return nil
		}
		variable, ok := symbol.(variable)
		if !ok {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Undefined: %s", startPointString(s.GetTSNode()), s.Name),
				location: s.TSNode.Range(),
			})
			return nil
		}
		if !variable.mut {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Immutable variable: %s", startPointString(s.GetTSNode()), s.Name),
				location: s.TSNode.Range(),
			})
			return nil
		}
		value := c.checkExpression(s.Value)
		if !variable._type.Is(value.GetType()) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Type mismatch: Expected %s, got %s", startPointString(s.Value.GetTSNode()), variable._type, value.GetType()),
				location: s.Value.GetTSNode().Range(),
			})
			return nil
		}
		return VariableAssignment{Name: s.Name, Value: value}
	case ast.IfStatement:
		var condition Expression
		if s.Condition != nil {
			condition = c.checkExpression(s.Condition)
			if condition.GetType() != (Bool{}) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("%s If conditions must be boolean expressions", startPointString(s.Condition.GetTSNode())),
					location: s.Condition.GetTSNode().Range(),
				})
			}
		}

		body := c.checkBlock(s.Body, []variable{})

		var elseClause Statement = nil
		if s.Else != nil {
			elseClause = c.checkStatement(s.Else)
		}
		return IfStatement{Condition: condition, Body: body, Else: elseClause}
	case ast.Comment:
		return nil
	case ast.RangeLoop:
		cursor := variable{name: s.Cursor.Name, mut: false, _type: Num{}}
		start := c.checkExpression(s.Start)
		end := c.checkExpression(s.End)

		startType := start.GetType()
		endType := end.GetType()
		if !startType.Is(Num{}) || !endType.Is(Num{}) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Invalid range: %s..%s", startPointString(s.Start.GetTSNode()), startType, endType),
				location: s.Start.GetTSNode().Range(),
			})
			return nil
		}
		body := c.checkBlock(s.Body, []variable{cursor})
		return ForRange{
			Cursor: Identifier{Name: s.Cursor.Name, symbol: cursor},
			Start:  start,
			End:    end,
			Body:   body,
		}
	case ast.ForLoop:
		iterable := c.checkExpression(s.Iterable)
		cursor := variable{name: s.Cursor.Name, mut: false, _type: iterable.GetType()}
		// getBody func allows lazy evaluation so that cursor can be updated within the switch below
		getBody := func() []Statement {
			return c.checkBlock(s.Body, []variable{cursor})
		}

		switch iterable.GetType().(type) {
		case Num:
			return ForRange{
				Cursor: Identifier{Name: s.Cursor.Name, symbol: cursor},
				Start:  NumLiteral{Value: 0},
				End:    iterable,
				Body:   getBody(),
			}
		case Str:
			return ForIn{
				Cursor:   Identifier{Name: s.Cursor.Name, symbol: cursor},
				Iterable: iterable,
				Body:     getBody(),
			}
		case Bool:
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Cannot iterate over a Bool", startPointString(s.Iterable.GetTSNode())),
				location: s.Iterable.GetTSNode().Range(),
			})
			return nil
		case List:
			listType := iterable.GetType().(List)
			cursor._type = listType.element
			return ForIn{
				Cursor:   Identifier{Name: s.Cursor.Name, symbol: cursor},
				Iterable: iterable,
				Body:     getBody(),
			}
		default:
			panic(fmt.Sprintf("Unhandled iterable type: %T", iterable.GetType()))
		}
	case ast.WhileLoop:
		condition := c.checkExpression(s.Condition)
		if condition.GetType() != (Bool{}) {
			c.addDiagnostic(Diagnostic{
				Kind: Error,
				Message: fmt.Sprintf(
					"%s While conditions must be boolean expressions",
					startPointString(s.Condition.GetTSNode()),
				),
				location: s.Condition.GetTSNode().Range(),
			})
		}

		body := c.checkBlock(s.Body, []variable{})
		return WhileLoop{
			Condition: condition,
			Body:      body,
		}
	case ast.FunctionDeclaration:
		parameters := make([]Parameter, len(s.Parameters))
		blockVariables := make([]variable, len(s.Parameters))
		for i, p := range s.Parameters {
			parameters[i] = Parameter{
				Name: p.Name,
				Type: c.resolveDeclaredType(p.Type),
			}
			blockVariables[i] = variable{name: p.Name, mut: false, _type: parameters[i].Type}
		}

		declaredReturnType := c.resolveDeclaredType(s.ReturnType)
		body := c.checkBlock(s.Body, blockVariables)
		var returnType Type = nil
		if len(body) > 0 {
			if expr, ok := body[len(body)-1].(Expression); ok {
				returnType = expr.GetType()
				// TODO: if declared type is Void, ignore actual return
				if declaredReturnType != nil && !declaredReturnType.Is(returnType) {
					c.addDiagnostic(Diagnostic{
						Kind: Error,
						Message: fmt.Sprintf(
							"%s Type mismatch: Expected %s, got %s",
							startPointString(s.ReturnType.GetTSNode()),
							declaredReturnType,
							returnType),
						location: s.ReturnType.GetTSNode().Range(),
					})
				}
			}
		}

		fn := function{
			name:       s.Name,
			parameters: blockVariables,
			returns:    returnType,
		}

		c.scope.declare(fn)

		return FunctionDeclaration{
			Name:       s.Name,
			Parameters: parameters,
			Body:       body,
			Return:     returnType,
		}
	case ast.EnumDefinition:
		if len(s.Variants) == 0 {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Enums must have at least one variant", startPointString(s.GetTSNode())),
				location: s.GetTSNode().Range(),
			})
		}
		uniqueVariants := map[string]bool{}
		for _, variant := range s.Variants {
			if _, ok := uniqueVariants[variant]; ok {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("%s Duplicate variant: %s", startPointString(s.GetTSNode()), variant),
					location: s.GetTSNode().Range(),
				})
			}
			uniqueVariants[variant] = true
		}
		enum := Enum{
			Name:     s.Name,
			Variants: s.Variants,
		}
		c.scope.declare(enum)
		return enum
	default:
		return c.checkExpression(s)
	}
}

func (c *checker) checkBlock(block []ast.Statement, variables []variable) []Statement {
	new_scope := newScope(c.scope)
	c.scope = new_scope
	defer func() { c.scope = new_scope.parent }()

	for _, variable := range variables {
		c.scope.addVariable(variable)
	}

	statements := make([]Statement, len(block))
	for i, stmt := range block {
		statements[i] = c.checkStatement(stmt)
	}
	return statements
}

func (c *checker) checkExpression(expr ast.Expression) Expression {
	switch e := expr.(type) {
	case ast.Identifier:
		sym := c.scope.find(e.Name)
		if sym == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Undefined: %s", startPointString(e.GetTSNode()), e.Name),
				location: e.GetTSNode().Range(),
			})
			return nil
		}
		return Identifier{Name: e.Name, symbol: sym}
	case ast.StrLiteral:
		return StrLiteral{Value: strings.Trim(e.Value, `"`)}
	case ast.NumLiteral:
		value, err := strconv.Atoi(e.Value)
		if err != nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Invalid number: %s", startPointString(e.TSNode), e.Value),
				location: e.TSNode.Range(),
			})
			return nil
		}
		return NumLiteral{Value: value}
	case ast.BoolLiteral:
		return BoolLiteral{Value: e.Value}
	case ast.MemberAccess:
		subject := c.checkExpression(e.Target)
		switch e.AccessType {
		case ast.Instance:
			return c.checkInstanceProperty(subject, e.Member)
		case ast.Static:
			return c.checkStaticProperty(subject, e.Member)
		default:
			panic("unreachable")
		}
	case ast.UnaryExpression:
		expr := c.checkExpression(e.Operand)
		switch e.Operator {
		case ast.Minus:
			if !expr.GetType().Is(Num{}) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("%s The '-' operator can only be used on numbers", startPointString(e.Operand.GetTSNode())),
					location: e.Operand.GetTSNode().Range(),
				})
				return nil
			}
			return Negation{Value: expr}
		case ast.Bang:
			if !expr.GetType().Is(Bool{}) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("%s The '!' operator can only be used on booleans", startPointString(e.Operand.GetTSNode())),
					location: e.Operand.GetTSNode().Range(),
				})
				return nil
			}
			return Not{Value: expr}
		}
		panic(fmt.Sprintf("Unhandled unary operator: %d", e.Operator))
	case ast.BinaryExpression:
		left := c.checkExpression(e.Left)
		right := c.checkExpression(e.Right)
		operator := c.resolveBinaryOperator(e.Operator)

		diagnostic := Diagnostic{
			Kind:     Error,
			location: e.TSNode.Range(),
			Message: fmt.Sprintf(
				"%s Invalid operation: %s %s %s",
				startPointString(e.GetTSNode()),
				left.GetType(),
				operator,
				right.GetType()),
		}
		switch operator {
		case And, Or:
			if !left.GetType().Is(Bool{}) || !right.GetType().Is(Bool{}) {
				c.addDiagnostic(diagnostic)
				return nil
			}
		case Equal, NotEqual:
			if (left.GetType() != Num{}) && (left.GetType() != Bool{}) && (left.GetType() != Str{}) {
				c.addDiagnostic(diagnostic)
				return nil
			}
		default:
			if !left.GetType().Is(Num{}) || !right.GetType().Is(Num{}) {
				c.addDiagnostic(diagnostic)
				return nil
			}
		}

		return BinaryExpr{Op: operator, Left: left, Right: right}
	case ast.InterpolatedStr:
		parts := make([]Expression, len(e.Chunks))
		for i, chunk := range e.Chunks {
			parts[i] = c.checkExpression(chunk)
		}
		return InterpolatedStr{Parts: parts}
	case ast.FunctionCall:
		sym := c.scope.find(e.Name)
		if sym == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Undefined: %s", startPointString(e.GetTSNode()), e.Name),
				location: e.TSNode.Range(),
			})
			return nil
		}
		fn, ok := sym.asFunction()
		if !ok {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Not a function: %s", startPointString(e.GetTSNode()), e.Name),
				location: e.TSNode.Range(),
			})
			return nil
		}

		args := make([]Expression, len(e.Args))
		if len(e.Args) != len(fn.parameters) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Incorrect number of arguments: Expected %d, got %d", startPointString(e.GetTSNode()), len(fn.parameters), len(e.Args)),
				location: e.TSNode.Range(),
			})
		} else {
			for i, arg := range e.Args {
				args[i] = c.checkExpression(arg)
				if !fn.parameters[i]._type.Is(args[i].GetType()) {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						Message:  fmt.Sprintf("%s Type mismatch: Expected %s, got %s", startPointString(arg.GetTSNode()), fn.parameters[i]._type, args[i].GetType()),
						location: arg.GetTSNode().Range(),
					})
				}
			}
		}

		return FunctionCall{Name: e.Name, Args: args, symbol: fn}
	case ast.AnonymousFunction:
		parameters := make([]Parameter, len(e.Parameters))
		blockVariables := make([]variable, len(e.Parameters))
		for i, p := range e.Parameters {
			parameters[i] = Parameter{
				Name: p.Name,
				Type: c.resolveDeclaredType(p.Type),
			}
			blockVariables[i] = variable{name: p.Name, mut: false, _type: parameters[i].Type}
		}
		declaredReturnType := c.resolveDeclaredType(e.ReturnType)
		body := c.checkBlock(e.Body, blockVariables)
		var returnType Type = nil
		if len(body) > 0 {
			if expr, ok := body[len(body)-1].(Expression); ok {
				returnType = expr.GetType()
				// TODO: if declared type is Void, ignore actual return
				if declaredReturnType != nil && !declaredReturnType.Is(returnType) {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						location: e.ReturnType.GetTSNode().Range(),
						Message: fmt.Sprintf(
							"%s Type mismatch: Expected %s, got %s",
							startPointString(e.ReturnType.GetTSNode()),
							declaredReturnType,
							returnType),
					})
				}
			}
		}
		return FunctionLiteral{
			Parameters: parameters,
			Return:     returnType,
			Body:       body,
		}
	case ast.ListLiteral:
		if len(e.Items) == 0 {
			return ListLiteral{}
		}
		var elementType Type
		elements := make([]Expression, len(e.Items))
		for i, item := range e.Items {
			elements[i] = c.checkExpression(item)
			_type := elements[i].GetType()
			if i == 0 {
				elementType = _type
			} else if !_type.Is(elementType) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					location: item.GetTSNode().Range(),
					Message:  fmt.Sprintf("%s Type mismatch: Expected %s, got %s", startPointString(item.GetTSNode()), elementType, _type),
				})
			}
		}
		return ListLiteral{
			Elements: elements,
			_type:    List{element: elementType},
		}
	case ast.MatchExpression:
		subject := c.checkExpression(e.Subject)
		cases := make([]MatchCase, len(e.Cases))

		sym := c.scope.find(subject.GetType().(EnumVariant).Enum)
		if sym == nil {
			c.addDiagnostic(Diagnostic{
				Kind: Error,
				Message: fmt.Sprintf(
					"%s Undefined: %s",
					startPointString(e.Subject.GetTSNode()),
					subject.GetType().(EnumVariant).Enum),
				location: e.Subject.GetTSNode().Range(),
			})
			return nil
		}
		enum := sym.(Enum)

		expectedCases := map[string]bool{}
		for _, variant := range enum.Variants {
			expectedCases[variant] = false
		}

		var _type Type = Void{}
		for i, arm := range e.Cases {
			pattern := c.checkExpression(arm.Pattern)
			variant := pattern.(EnumVariant)
			isDone, ok := expectedCases[variant.Variant]
			if !ok {
				panic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("%s Invalid variant: %s", startPointString(arm.Pattern.GetTSNode()), variant.Variant),
					location: arm.Pattern.GetTSNode().Range(),
				})
			}
			if isDone {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("%s Duplicate case: %s", startPointString(arm.Pattern.GetTSNode()), variant),
					location: arm.Pattern.GetTSNode().Range(),
				})
				return nil
			}
			expectedCases[variant.Variant] = true

			body := c.checkBlock(arm.Body, []variable{})
			if i == 0 {
				_type = body[len(body)-1].(Expression).GetType()
			} else if !body[len(body)-1].(Expression).GetType().Is(_type) {
				c.addDiagnostic(Diagnostic{
					Kind: Error,
					Message: fmt.Sprintf(
						"%s Type mismatch: Expected %s, got %s",
						startPointString(arm.Body[0].GetTSNode()), _type, body[len(body)-1].(Expression).GetType()),
					location: arm.Body[len(arm.Body)-1].GetTSNode().Range(),
				})
			}
			cases[i] = MatchCase{
				Pattern: pattern,
				Body:    body,
				_type:   _type,
			}
		}

		nonExhaustive := false
		for variant, isDone := range expectedCases {
			if !isDone {
				nonExhaustive = true
				c.addDiagnostic(Diagnostic{
					Kind: Error,
					Message: fmt.Sprintf(
						"%s Incomplete match: missing case for '%s'",
						startPointString(e.GetTSNode()), enum.Name+"::"+variant),
					location: e.GetTSNode().Range(),
				})
			}
		}

		if nonExhaustive {
			return nil
		}

		return MatchExpr{
			Subject: subject,
			Cases:   cases,
		}
	default:
		panic(fmt.Sprintf("Unhandled expression: %T", e))
	}
}

func (c *checker) checkInstanceProperty(subject Expression, member ast.Expression) Expression {
	switch m := member.(type) {
	case ast.Identifier:
		sig := subject.GetType().GetProperty(m.Name)
		if sig == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Undefined: %s.%s", startPointString(m.GetTSNode()), subject, m.Name),
				location: m.TSNode.Range(),
			})
			return nil
		}
		return InstanceProperty{
			Subject: subject,
			Property: Identifier{
				Name: m.Name,
				symbol: variable{
					name:  m.Name,
					_type: sig,
				}},
		}
	case ast.FunctionCall:
		sig := subject.GetType().GetProperty(m.Name)
		if sig == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Undefined: %s.%s", startPointString(m.GetTSNode()), subject, m.Name),
				location: m.TSNode.Range(),
			})
			return nil
		}
		fn, ok := sig.(function)
		if !ok {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Not a function: %s", startPointString(m.GetTSNode()), m.Name),
				location: m.TSNode.Range(),
			})
			return nil
		}
		args := make([]Expression, len(m.Args))
		if len(m.Args) != len(fn.parameters) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Incorrect number of arguments: Expected %d, got %d", startPointString(m.GetTSNode()), len(fn.parameters), len(m.Args)),
				location: m.TSNode.Range(),
			})
		} else {
			for i, arg := range m.Args {
				args[i] = c.checkExpression(arg)
				if !fn.parameters[i]._type.Is(args[i].GetType()) {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						Message:  fmt.Sprintf("%s Type mismatch: Expected %s, got %s", startPointString(arg.GetTSNode()), fn.parameters[i]._type, args[i].GetType()),
						location: arg.GetTSNode().Range(),
					})
				}
			}
		}

		if pkg, ok := subject.GetType().(Package); ok {
			return PackageAccess{
				Package:  pkg,
				Property: FunctionCall{Name: m.Name, Args: args, symbol: fn},
			}
		}

		return InstanceProperty{
			Subject:  subject,
			Property: FunctionCall{Name: m.Name, Args: args, symbol: fn},
		}
	default:
		panic(fmt.Errorf("Unhandled instance access for %T", m))
	}
}

func (c *checker) checkStaticProperty(subject Expression, member ast.Expression) Expression {
	switch s := subject.GetType().(type) {
	case Enum:
		for i, variant := range s.Variants {
			if variant == member.(ast.Identifier).Name {
				return EnumVariant{
					Enum:    s.Name,
					Variant: variant,
					Value:   i,
				}
			}
		}

		c.addDiagnostic(Diagnostic{
			Kind: Error,
			Message: fmt.Sprintf(
				"%s Undefined: %s::%s",
				startPointString(member.GetTSNode()),
				subject,
				member.(ast.Identifier).Name),
			location: member.GetTSNode().Range(),
		})
		return nil
	default:
		panic(fmt.Sprintf("Unsupported static access for %T", s))
	}
}

func (c checker) resolveDeclaredType(t ast.DeclaredType) Type {
	if t == nil {
		return nil
	}

	switch tt := t.(type) {
	case ast.StringType:
		return Str{}
	case ast.NumberType:
		return Num{}
	case ast.BooleanType:
		return Bool{}
	case ast.Void:
		return Void{}
	case ast.List:
		return List{
			element: c.resolveDeclaredType(tt.Element),
		}
	case ast.CustomType:
		name := c.scope.find(tt.GetName())
		custom, isType := name.(Type)
		if !isType {
			c.addDiagnostic(Diagnostic{
				Kind:    Error,
				Message: fmt.Sprintf(`%s Undefined: %s`, startPointString(tt.GetTSNode()), name),
			})
			return nil
		}
		return custom
	default:
		panic(fmt.Sprintf("Unhandled declared type: %T", t))
	}
}

func (c *checker) resolveBinaryOperator(op ast.Operator) BinaryOperator {
	switch op {
	case ast.Plus:
		return Add
	case ast.Minus:
		return Sub
	case ast.Multiply:
		return Mul
	case ast.Divide:
		return Div
	case ast.Modulo:
		return Mod
	case ast.Equal:
		return Equal
	case ast.NotEqual:
		return NotEqual
	case ast.GreaterThan:
		return GreaterThan
	case ast.GreaterThanOrEqual:
		return GreaterThanOrEqual
	case ast.LessThan:
		return LessThan
	case ast.LessThanOrEqual:
		return LessThanOrEqual
	case ast.And:
		return And
	case ast.Or:
		return Or
	default:
		panic(fmt.Sprintf("Unsupported binary operator: %d", op))
	}
}
