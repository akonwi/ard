package checker_v2

import (
	"fmt"
	"reflect"
	"slices"
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

type Equality struct {
	Left, Right Expression
}

func (n *Equality) Type() Type {
	return Bool
}

type And struct {
	Left, Right Expression
}

func (a *And) Type() Type {
	return Bool
}

type Or struct {
	Left, Right Expression
}

func (o *Or) Type() Type {
	return Bool
}

type Block struct {
	Stmts []Statement
}

func (b *Block) Type() Type {
	if len(b.Stmts) == 0 {
		return Void
	}
	last := b.Stmts[len(b.Stmts)-1]
	if last.Expr != nil {
		return last.Expr.Type()
	}
	return Void
}

type If struct {
	Condition Expression
	Body      *Block
	ElseIf    *If
	Else      *Block
}

func (i *If) Type() Type {
	return i.Body.Type()
}

type ForIntRange struct {
	Cursor string
	Start  Expression
	End    Expression
	Body   *Block
}

func (f ForIntRange) NonProducing() {}

type ForInStr struct {
	Cursor string
	Value  Expression
	Body   *Block
}

func (f ForInStr) NonProducing() {}

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
			if val == nil {
				return nil
			}

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
	case *ast.RangeLoop:
		{
			start, end := c.checkExpr(s.Start), c.checkExpr(s.End)
			if start == nil || end == nil {
				return nil
			}
			if start.Type() != end.Type() {
				c.addError(fmt.Sprintf("Invalid range: %s..%s", start.Type(), end.Type()), s.Start.GetLocation())
				return nil
			}

			if start.Type() == Int {
				loop := &ForIntRange{
					Cursor: s.Cursor.Name,
					Start:  start,
					End:    end,
				}
				body := c.checkBlock(s.Body, func() {
					c.scope.add(&VariableDef{
						Mutable: false,
						Name:    s.Cursor.Name,
						__type:  start.Type(),
					})
				})
				loop.Body = body
				return &Statement{Stmt: loop}
			}

			panic(fmt.Errorf("Cannot create range of %s", start.Type()))
		}
	case *ast.ForInLoop:
		{
			iterValue := c.checkExpr(s.Iterable)
			if iterValue == nil {
				return nil
			}
			
			// Handle strings specifically
			if iterValue.Type() == Str {
				loop := &ForInStr{
					Cursor: s.Cursor.Name,
					Value:  iterValue,
				}
				
				// Create a new scope for the loop body where the cursor is defined
				body := c.checkBlock(s.Body, func() {
					// Add the cursor variable to the scope as a string
					// Each character in a string is also a string
					c.scope.add(&VariableDef{
						Mutable: false,
						Name:    s.Cursor.Name,
						__type:  Str,
					})
				})
				
				loop.Body = body
				return &Statement{Stmt: loop}
			}
			
			// Handle integer iteration (for i in n - sugar for 0..n)
			if iterValue.Type() == Int {
				// This is syntax sugar for a range from 0 to n
				loop := &ForIntRange{
					Cursor: s.Cursor.Name,
					Start:  &IntLiteral{0},      // Start from 0
					End:    iterValue,           // End at the specified number
				}
				
				// Create a new scope for the loop body where the cursor is defined
				body := c.checkBlock(s.Body, func() {
					// Add the cursor variable to the scope
					c.scope.add(&VariableDef{
						Mutable: false,
						Name:    s.Cursor.Name,
						__type:  Int,
					})
				})
				
				loop.Body = body
				return &Statement{Stmt: loop}
			}
			
			// Here we would handle other iterable types (like lists, etc.)
			// Currently we only support string and integer iteration
			c.addError(fmt.Sprintf("Cannot iterate over a %s", iterValue.Type()), s.Iterable.GetLocation())
			return nil
		}
	default:
		expr := c.checkExpr((ast.Expression)(*stmt))
		if expr == nil {
			return nil
		}
		return &Statement{Expr: expr}
	}
}

func (c *checker) checkBlock(stmts []ast.Statement, setup func()) *Block {
	if len(stmts) == 0 {
		return &Block{Stmts: []Statement{}}
	}

	scope := newScope(c.scope)
	c.scope = scope
	defer func() {
		c.scope = c.scope.parent
	}()

	if setup != nil {
		setup()
	}

	block := &Block{Stmts: make([]Statement, len(stmts))}
	for i := range stmts {
		if stmt := c.checkStmt(&stmts[i]); stmt != nil {
			block.Stmts[i] = *stmt
		}
	}
	return block
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
			case ast.Equal:
				{
					left, right := c.checkExpr(s.Left), c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != right.Type() {
						c.addError(fmt.Sprintf("Invalid: %s == %s", left.Type(), right.Type()), s.GetLocation())
						return nil
					}

					allowedTypes := []Type{Int, Float, Str, Bool}
					if !slices.Contains(allowedTypes, left.Type()) || !slices.Contains(allowedTypes, right.Type()) {
						c.addError(fmt.Sprintf("Invalid: %s == %s", left.Type(), right.Type()), s.GetLocation())
						return nil
					}

					return &Equality{left, right}
				}
			case ast.And:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != Bool || right.Type() != Bool {
						c.addError("The 'and' operator can only be used between Bools", s.GetLocation())
						return nil
					}

					return &And{left, right}
				}
			case ast.Or:
				{
					left := c.checkExpr(s.Left)
					right := c.checkExpr(s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != Bool || right.Type() != Bool {
						c.addError("The 'or' operator can only be used with Boolean values", s.GetLocation())
						return nil
					}

					return &Or{left, right}
				}
			default:
				panic(fmt.Errorf("Unexpected operator: %v", s.Operator))
			}
		}
	case *ast.IfStatement:
		{
			cond := c.checkExpr(s.Condition)
			if cond == nil {
				return nil
			}
			if cond.Type() != Bool {
				c.addError("If conditions must be boolean expressions", s.GetLocation())
				return nil
			}

			body := c.checkBlock(s.Body, nil)

			var elseIf *If
			var elseBody *Block

			// does not recurse. reach into AST for each level since it's fixed
			if s.Else != nil {
				next := s.Else.(*ast.IfStatement)
				if next.Condition != nil {
					cond := c.checkExpr(next.Condition)
					if cond == nil {
						return nil
					}
					if cond.Type() != Bool {
						c.addError("If conditions must be boolean expressions", next.GetLocation())
						return nil
					}

					elseIfBody := c.checkBlock(next.Body, nil)
					if elseIfBody.Type() != body.Type() {
						c.addError("All branches must have the same result type", next.GetLocation())
						return nil
					}

					elseIf = &If{
						Condition: cond,
						Body:      elseIfBody,
					}

					if next, ok := next.Else.(*ast.IfStatement); ok {
						elseBody = c.checkBlock(next.Body, nil)
					}
				} else {
					b := c.checkBlock(next.Body, nil)
					if b.Type() != body.Type() {
						c.addError("All branches must have the same result type", next.GetLocation())
						return nil
					}
					elseBody = b
				}
			}

			return &If{
				Condition: cond,
				Body:      body,
				ElseIf:    elseIf,
				Else:      elseBody,
			}
		}
	default:
		panic(fmt.Errorf("Unexpected expression: %s", reflect.TypeOf(s)))
	}
}
