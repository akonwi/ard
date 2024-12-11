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
	Kind    DiagnosticKind
	Message string
}

type Program struct {
	Imports    map[string]Package
	Statements []Statement
}
type Package struct {
	Path string
}

// Expressions produce something, therefore they have a Type
type Expression interface {
	GetType() Type
}

type StrLiteral struct {
	Value string
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
	return b.Left.GetType()
}

// Statements don't produce anything
type Statement interface{}

type VariableBinding struct {
	Name  string
	Value Expression
}

type VariableAssignment struct {
	Name  string
	Value Expression
}

// tree-sitter uses 0 based positioning
func startPointString(node *ts.Node) string {
	pos := node.StartPosition()
	return fmt.Sprintf("[%d:%d]", pos.Row+1, pos.Column+1)
}

type checker struct {
	diagnostics []Diagnostic
	imports     map[string]Package
	scope       scope
}

func (c *checker) addDiagnostic(d Diagnostic) {
	c.diagnostics = append(c.diagnostics, d)
}

func Check(program ast.Program) (Program, []Diagnostic) {
	checker := checker{
		diagnostics: []Diagnostic{},
		imports:     map[string]Package{},
		scope:       NewScope(),
	}
	statements := []Statement{}

	for _, imp := range program.Imports {
		if _, ok := checker.imports[imp.Name]; !ok {
			checker.imports[imp.Name] = Package{Path: imp.Path}
		} else {
			checker.addDiagnostic(Diagnostic{
				Kind:    Error,
				Message: fmt.Sprintf("%s Duplicate package name: %s", startPointString(imp.TSNode), imp.Name),
			})
		}
	}

	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case ast.StrLiteral:
			expr := checker.checkExpression(s)
			statements = append(statements, expr)
		case ast.NumLiteral:
			expr := checker.checkExpression(s)
			statements = append(statements, expr)
		case ast.BoolLiteral:
			expr := checker.checkExpression(s)
			statements = append(statements, expr)
		case ast.UnaryExpression:
			expr := checker.checkExpression(s)
			statements = append(statements, expr)
		case ast.VariableDeclaration:
			value := checker.checkExpression(s.Value)
			if s.Type != nil {
				declared := resolveDeclaredType(s.Type)
				if !declared.Is(value.GetType()) {
					checker.addDiagnostic(Diagnostic{
						Kind:    Error,
						Message: fmt.Sprintf("%s Type mismatch: Expected %s, got %s", startPointString(s.Value.GetTSNode()), declared, value.GetType()),
					})
				}
			}
			statements = append(statements, VariableBinding{Name: s.Name, Value: value})
			checker.scope.addVariable(variable{name: s.Name, mut: s.Mutable, _type: value.GetType()})
		case ast.VariableAssignment:
			variable, ok := checker.scope.findVariable(s.Name)
			if !ok {
				checker.addDiagnostic(Diagnostic{
					Kind:    Error,
					Message: fmt.Sprintf("%s Undefined: %s", startPointString(s.GetTSNode()), s.Name),
				})
				continue
			}
			if !variable.mut {
				checker.addDiagnostic(Diagnostic{
					Kind:    Error,
					Message: fmt.Sprintf("%s Immutable variable: %s", startPointString(s.GetTSNode()), s.Name),
				})
				continue
			}
			value := checker.checkExpression(s.Value)
			if !variable._type.Is(value.GetType()) {
				checker.addDiagnostic(Diagnostic{
					Kind:    Error,
					Message: fmt.Sprintf("%s Type mismatch: Expected %s, got %s", startPointString(s.Value.GetTSNode()), variable._type, s.Value.GetType()),
				})
				continue
			}
			statements = append(statements, VariableAssignment{Name: s.Name, Value: value})
		case ast.BinaryExpression:
			expr := checker.checkExpression(s)
			statements = append(statements, expr)
		default:
			panic(fmt.Sprintf("Unhandled statement: %T", s))
		}
	}

	return Program{
		Imports:    checker.imports,
		Statements: statements,
	}, checker.diagnostics
}

func (c *checker) checkExpression(expr ast.Expression) Expression {
	switch e := expr.(type) {
	case ast.StrLiteral:
		return StrLiteral{Value: strings.Trim(e.Value, `"`)}
	case ast.NumLiteral:
		value, err := strconv.Atoi(e.Value)
		if err != nil {
			c.addDiagnostic(Diagnostic{
				Kind:    Error,
				Message: fmt.Sprintf("%s Invalid number: %s", startPointString(e.TSNode), e.Value),
			})
			return nil
		}
		return NumLiteral{Value: value}
	case ast.BoolLiteral:
		return BoolLiteral{Value: e.Value}
	case ast.UnaryExpression:
		expr := c.checkExpression(e.Operand)
		switch e.Operator {
		case ast.Minus:
			if !expr.GetType().Is(Num{}) {
				c.addDiagnostic(Diagnostic{
					Kind:    Error,
					Message: fmt.Sprintf("%s The '-' operator can only be used on numbers", startPointString(e.Operand.GetTSNode())),
				})
				return nil
			}
			return Negation{Value: expr}
		case ast.Bang:
			if !expr.GetType().Is(Bool{}) {
				c.addDiagnostic(Diagnostic{
					Kind:    Error,
					Message: fmt.Sprintf("%s The '!' operator can only be used on booleans", startPointString(e.Operand.GetTSNode())),
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
			Kind: Error,
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
	default:
		panic(fmt.Sprintf("Unhandled expression: %T", e))
	}
}

func resolveDeclaredType(t ast.DeclaredType) Type {
	switch t.(type) {
	case ast.StringType:
		return Str{}
	case ast.NumberType:
		return Num{}
	case ast.BooleanType:
		return Bool{}
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
