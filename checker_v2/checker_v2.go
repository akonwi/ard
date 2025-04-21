package checker_v2

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/akonwi/ard/ast"
)

type Program struct {
	StdImports map[string]StdPackage
	Imports    map[string]ExtPackage
	Statements []Statement
}

type StdPackage struct {
	Name string
	Path string
}

type ExtPackage struct {
	Name string
	Path string
}

type DiagnosticKind string

const (
	Error DiagnosticKind = "error"
	Warn  DiagnosticKind = "warn"
)

type Diagnostic struct {
	Kind     DiagnosticKind
	Message  string
	location ast.Location
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s %s", d.location.Start, d.Message)
}

/* can either produce a value or not */
type Statement struct {
	Expr Expression
	Stmt NonProducing
}

type NonProducing interface {
	NonProducing()
}

type Expression interface {
	String() string
	Type() Type
}

type StrLiteral struct {
	Value string
}

func (s *StrLiteral) String() string {
	return s.Value
}
func (s *StrLiteral) Type() Type {
	return Str
}

type BoolLiteral struct {
	Value bool
}

func (b *BoolLiteral) String() string {
	return strconv.FormatBool(b.Value)
}

func (b *BoolLiteral) Type() Type {
	return Bool
}

type IntLiteral struct {
	Value int
}

func (i *IntLiteral) String() string {
	return strconv.Itoa(i.Value)
}

func (i *IntLiteral) Type() Type {
	return Int
}

type FloatLiteral struct {
	Value float64
}

func (f *FloatLiteral) String() string {
	return strconv.FormatFloat(f.Value, 'g', 10, 64)
}

func (f *FloatLiteral) Type() Type {
	return Float
}

type VariableDef struct {
	Mutable bool
	Name    string
	Type    Type
	Value   Expression
}

func (v *VariableDef) NonProducing() {}

type checker struct {
	diagnostics []Diagnostic
}

func (c *checker) addError(msg string, location ast.Location) {
	c.diagnostics = append(c.diagnostics, Diagnostic{
		Kind:     Error,
		Message:  msg,
		location: location,
	})
}

func (c *checker) addWarning(msg string, location ast.Location) {
	c.diagnostics = append(c.diagnostics, Diagnostic{
		Kind:     Warn,
		Message:  msg,
		location: location,
	})
}

func Check(input *ast.Program) (*Program, []Diagnostic) {
	c := checker{diagnostics: []Diagnostic{}}
	program := &Program{
		StdImports: map[string]StdPackage{},
		Imports:    map[string]ExtPackage{},
		Statements: []Statement{},
	}

	for _, imp := range input.Imports {
		if _, dup := program.StdImports[imp.Name]; dup {
			c.addWarning(fmt.Sprintf("%s Duplicate import: %s", imp.GetStart(), imp.Name), imp.GetLocation())
			continue
		}
		if _, dup := program.Imports[imp.Name]; dup {
			c.addWarning(fmt.Sprintf("%s Duplicate import: %s", imp.GetStart(), imp.Name), imp.GetLocation())
			continue
		}

		if strings.HasPrefix(imp.Path, "ard/") {
			if pkg, ok := findInStdLib(imp.Path, imp.Name); ok {
				program.StdImports[imp.Name] = pkg
			} else {
				c.addError(fmt.Sprintf("Unknown package: %s", imp.Path), imp.GetLocation())
			}
		} else {
			program.Imports[imp.Name] = ExtPackage{Path: imp.Path, Name: imp.Name}
		}
	}

	for i := range input.Statements {
		if stmt := c.checkStmt(&input.Statements[i]); stmt != nil {
			program.Statements = append(program.Statements, *stmt)
		}
	}

	return program, c.diagnostics
}

func findInStdLib(path, name string) (StdPackage, bool) {
	switch path {
	case "ard/io", "ard/json", "ard/maybe", "ard/fs":
		return StdPackage{path, name}, true
	}
	return StdPackage{}, false
}

func (c *checker) resolveType(t ast.DeclaredType) Type {
	switch t.GetName() {
	case "String":
		return Str
	case Int.String():
		return Int
	case Float.String():
		return Float
	case "Boolean":
		return Bool
	default:
		panic(fmt.Errorf("unrecognized type: %s", t.GetName()))
	}
}

func (c *checker) checkStmt(stmt *ast.Statement) *Statement {
	switch s := (*stmt).(type) {
	case *ast.VariableDeclaration:
		return &Statement{
			Stmt: &VariableDef{
				Mutable: s.Mutable,
				Name:    s.Name,
				Type:    c.resolveType(s.Type),
				Value:   c.checkExpr(s.Value),
			},
		}
	default:
		expr := c.checkExpr((ast.Expression)(*stmt))
		return &Statement{Expr: expr}
	}
}

func (c *checker) checkExpr(expr ast.Expression) Expression {
	switch s := (expr).(type) {
	case *ast.StrLiteral:
		return &StrLiteral{s.Value}
	case *ast.BoolLiteral:
		return &BoolLiteral{s.Value}
	case *ast.NumLiteral:
		{
			if strings.Contains(s.Value, ".") {
				value, err := strconv.ParseFloat(s.Value, 64)
				if err != nil {
					c.addError(fmt.Sprintf("Invalid float: %s", s.Value), s.GetLocation())
					return nil
				}
				return &FloatLiteral{Value: value}
			}
			value, err := strconv.Atoi(s.Value)
			if err != nil {
				c.addError(fmt.Sprintf("Invalid int: %s", s.Value), s.GetLocation())
			}
			return &IntLiteral{value}
		}
	default:
		panic(fmt.Errorf("Unexpected expression: %s", reflect.TypeOf(s)))
	}
}
