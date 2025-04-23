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
	return fmt.Sprintf(`"%s"`, s.Value)
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
	sym  symbol
}

func (i *Identifier) Type() Type {
	return i.sym._type()
}

type Variable struct {
	sym symbol
}

func (v Variable) String() string {
	return v.Name()
}
func (v Variable) Name() string {
	return v.sym.name()
}
func (v *Variable) Type() Type {
	return v.sym._type()
}

type InstanceProperty struct {
	Subject  Expression
	Property string
	_type    Type
}

func (i *InstanceProperty) Type() Type {
	return i._type
}

type Negation struct {
	Value Expression
}

func (n *Negation) String() string {
	return fmt.Sprintf("-%s", n.Value)
}
func (n *Negation) Type() Type {
	return n.Value.Type()
}

type Not struct {
	Value Expression
}

func (n *Not) String() string {
	return fmt.Sprintf("not %s", n.Value)
}
func (n *Not) Type() Type {
	return Bool
}

type IntAddition struct {
	Left  Expression
	Right Expression
}

func (n *IntAddition) Type() Type {
	return Int
}

type IntSubtraction struct {
	Left  Expression
	Right Expression
}

func (n *IntSubtraction) Type() Type {
	return Int
}

type IntMultiplication struct {
	Left  Expression
	Right Expression
}

func (n *IntMultiplication) Type() Type {
	return Int
}

type IntDivision struct {
	Left  Expression
	Right Expression
}

func (n *IntDivision) Type() Type {
	return Int
}

type IntModulo struct {
	Left  Expression
	Right Expression
}

func (n *IntModulo) Type() Type {
	return Int
}

type IntGreater struct {
	Left  Expression
	Right Expression
}

func (n *IntGreater) Type() Type {
	return Bool
}

type IntGreaterEqual struct {
	Left  Expression
	Right Expression
}

func (n *IntGreaterEqual) Type() Type {
	return Bool
}

type IntLess struct {
	Left  Expression
	Right Expression
}

func (n *IntLess) Type() Type {
	return Bool
}

type IntLessEqual struct {
	Left  Expression
	Right Expression
}

func (n *IntLessEqual) Type() Type {
	return Bool
}

type FloatAddition struct {
	Left  Expression
	Right Expression
}

func (n *FloatAddition) Type() Type {
	return Float
}

type FloatSubtraction struct {
	Left  Expression
	Right Expression
}

func (n *FloatSubtraction) Type() Type {
	return Float
}

type FloatMultiplication struct {
	Left  Expression
	Right Expression
}

func (n *FloatMultiplication) Type() Type {
	return Float
}

type FloatDivision struct {
	Left  Expression
	Right Expression
}

func (n *FloatDivision) Type() Type {
	return Float
}

type FloatGreater struct {
	Left  Expression
	Right Expression
}

func (n *FloatGreater) Type() Type {
	return Bool
}

type FloatGreaterEqual struct {
	Left  Expression
	Right Expression
}

func (n *FloatGreaterEqual) Type() Type {
	return Bool
}

type FloatLess struct {
	Left  Expression
	Right Expression
}

func (n *FloatLess) Type() Type {
	return Bool
}

type FloatLessEqual struct {
	Left  Expression
	Right Expression
}

func (n *FloatLessEqual) Type() Type {
	return Bool
}

type StrAddition struct {
	Left  Expression
	Right Expression
}

func (n *StrAddition) Type() Type {
	return Str
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
						Stmt: &Reassignment{Target: &Variable{target}, Value: value},
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
	case *ast.Identifier:
		if sym := c.scope.getVar(s.Name); sym != nil {
			return &Variable{sym}
		}
		panic(fmt.Errorf("Undefined variable: %s", s.Name))
	case *ast.InstanceProperty:
		{
			subj := c.checkExpr(s.Target)
			if subj == nil {
				panic(fmt.Errorf("Cannot access %s on nil", s.Property))
			}

			propType := subj.Type().get(s.Property.Name)
			if propType == nil {
				c.addError(fmt.Sprintf("Undefined: %s.%s", subj, s.Property.Name), s.Property.GetLocation())
				return nil
			}
			return &InstanceProperty{
				Subject:  subj,
				Property: s.Property.Name,
				_type:    propType,
			}
		}
	case *ast.UnaryExpression:
		{
			value := c.checkExpr(s.Operand)
			if value == nil {
				return nil
			}
			if s.Operator == ast.Minus {
				if value.Type() != Int && value.Type() != Float {
					c.addError("Only numbers can be negated with '-'", s.GetLocation())
					return nil
				}
				return &Negation{value}
			}

			if value.Type() != Bool {
				c.addError("Only booleans can be negated with 'not'", s.GetLocation())
				return nil
			}
			return &Not{value}
		}
	case *ast.BinaryExpression:
		{
			switch s.Operator {
			case ast.Plus:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot add different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntAddition{left, right}
					}
					if left.Type() == Float {
						return &FloatAddition{left, right}
					}
					if left.Type() == Str {
						return &StrAddition{left, right}
					}
					c.addError("The '-' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case ast.Minus:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot subtract different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntSubtraction{left, right}
					}
					if left.Type() == Float {
						return &FloatSubtraction{left, right}
					}
					c.addError("The '+' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case ast.Multiply:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot multiply different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntMultiplication{left, right}
					}
					if left.Type() == Float {
						return &FloatMultiplication{left, right}
					}
					c.addError("The '*' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case ast.Divide:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot divide different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntDivision{left, right}
					}
					if left.Type() == Float {
						return &FloatDivision{left, right}
					}
					c.addError("The '/' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case ast.Modulo:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot modulo different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntModulo{left, right}
					}
					c.addError("The '%' operator can only be used for Int", s.GetLocation())
					return nil
				}
			case ast.GreaterThan:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot compare different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntGreater{left, right}
					}
					if left.Type() == Float {
						return &FloatGreater{left, right}
					}
					c.addError("The '>' operator can only be used for Int", s.GetLocation())
					return nil
				}
			case ast.GreaterThanOrEqual:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot compare different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntGreaterEqual{left, right}
					}
					if left.Type() == Float {
						return &FloatGreaterEqual{left, right}
					}
					c.addError("The '>=' operator can only be used for Int", s.GetLocation())
					return nil
				}
			case ast.LessThan:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot compare different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntLess{left, right}
					}
					if left.Type() == Float {
						return &FloatLess{left, right}
					}
					c.addError("The '<' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			case ast.LessThanOrEqual:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError("Cannot compare different types", s.GetLocation())
						return nil
					}
					if left.Type() == Int {
						return &IntLessEqual{left, right}
					}
					if left.Type() == Float {
						return &FloatLessEqual{left, right}
					}
					c.addError("The '<=' operator can only be used for Int or Float", s.GetLocation())
					return nil
				}
			default:
				panic(fmt.Errorf("Unexpected operator: %v", s.Operator))
			}
		}
	default:
		panic(fmt.Errorf("Unexpected expression: %s", reflect.TypeOf(s)))
	}
}
