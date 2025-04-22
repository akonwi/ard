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
	__type  Type
	Value   Expression
}

func (v *VariableDef) NonProducing() {}
func (v *VariableDef) name() string {
	return v.Name
}
func (v *VariableDef) _type() Type {
	return v.__type
}

type Reassignment struct {
	Target Expression
	Value  Expression
}

func (r *Reassignment) NonProducing() {}

type Identifier struct {
	Name string
}

func (i *Identifier) Type() Type {
	return nil
}

type checker struct {
	diagnostics []Diagnostic
	scope       *scope
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
	c := checker{diagnostics: []Diagnostic{}, scope: newScope(nil)}
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

func typeMismatch(expected, got Type) string {
	return fmt.Sprintf("Type mismatch: Expected %s, got %s", expected, got)
}

func (c *checker) checkStmt(stmt *ast.Statement) *Statement {
	switch s := (*stmt).(type) {
	case *ast.VariableDeclaration:
		{
			val := c.checkExpr(s.Value)
			if s.Type != nil {
				if expected := c.resolveType(s.Type); expected != nil {
					if expected != val.Type() {
						c.addError(typeMismatch(expected, val.Type()), s.Value.GetLocation())
						return nil
					}
				}
			}
			v := &VariableDef{
				Mutable: s.Mutable,
				Name:    s.Name,
				Value:   val,
				__type:  val.Type(),
			}
			c.scope.add(v)
			return &Statement{
				Stmt: v,
			}
		}
	case *ast.VariableAssignment:
		{
			// todo: not always a variable
			if id, ok := s.Target.(*ast.Identifier); ok {
				target := c.scope.getVar(id.Name)
				if target == nil {
					c.addError(fmt.Sprintf("Undefined: %s", id.Name), s.Target.GetLocation())
					return nil
				}
				value := c.checkExpr(s.Value)
				if value == nil {
					return nil
				}

				if binding, ok := target.(*VariableDef); ok {
					if !binding.Mutable {
						c.addError(fmt.Sprintf("Immutable variable: %s", binding.Name), s.Target.GetLocation())
						return nil
					}
					if target._type() != value.Type() {
						c.addError(typeMismatch(target._type(), value.Type()), s.Value.GetLocation())
						return nil
					}

					return &Statement{
						Stmt: &Reassignment{Target: &Identifier{target.name()}, Value: value},
					}
				}

			}
			return nil
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
