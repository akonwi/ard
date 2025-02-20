package checker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/akonwi/ard/internal/ast"
)

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

type Program struct {
	Imports    map[string]Package
	Statements []Statement
}

// doubles as a symbol
type Package interface {
	symbol
	Type
	GetName() string
	GetPath() string
}

type ExternalPackage struct {
	path  string
	alias string
}

func newExternalPackage(path, alias string) Package {
	return ExternalPackage{
		path:  path,
		alias: alias,
	}
}

func (pkg ExternalPackage) GetName() string {
	if pkg.alias != "" {
		return pkg.alias
	}
	split := strings.Split(pkg.GetPath(), "/")
	return split[len(split)-1]
}
func (pkg ExternalPackage) GetPath() string {
	return pkg.path
}

// ExternalPackage impl symbol
func (p ExternalPackage) GetType() Type {
	return p
}
func (p ExternalPackage) asFunction() (function, bool) {
	return function{}, false
}

// ExternalPackage impl Type
func (p ExternalPackage) String() string {
	return p.path
}
func (p ExternalPackage) GetProperty(name string) Type {
	return nil
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

type IntLiteral struct {
	Value int
}

func (n IntLiteral) GetType() Type {
	return Int{}
}

type FloatLiteral struct {
	Value float64
}

func (f FloatLiteral) GetType() Type {
	return Float{}
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

type MapLiteral struct {
	Entries map[Expression]Expression
	_type   Map
}

func (m MapLiteral) GetType() Type {
	return m._type
}

type StructInstance struct {
	Name   string
	Fields map[string]Expression
	_type  *Struct
}

func (s StructInstance) GetType() Type {
	return s._type
}

type Negation struct {
	Value Expression
}

func (n Negation) GetType() Type {
	return n.Value.GetType()
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
		return b.Left.GetType()
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
func (i Identifier) GetType() Type {
	return i.symbol.GetType()
}

type InstanceProperty struct {
	Subject  Expression
	Property Identifier
}

func (i InstanceProperty) GetType() Type {
	return i.Property.GetType()
}

type InstanceMethod struct {
	Subject Expression
	Method  FunctionCall
}

func (i InstanceMethod) GetType() Type {
	return i.Method.GetType()
}

type StaticFunctionCall struct {
	Subject  Static
	Function FunctionCall
}

func (s StaticFunctionCall) GetType() Type {
	return s.Function.GetType()
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

type BoolMatch struct {
	Subject Expression
	True    Block
	False   Block
}

func (m BoolMatch) GetType() Type {
	return m.True.result
}

type EnumMatch struct {
	Subject  Expression
	Cases    []Block
	CatchAll MatchCase
}

func (m EnumMatch) GetType() Type {
	return m.Cases[0].result
}

type OptionMatch struct {
	Subject Expression
	None    Block
	Some    MatchCase
}

func (o OptionMatch) GetType() Type {
	return o.Some._type
}

type UnionMatch struct {
	Subject  Expression
	Cases    map[Type]Block
	CatchAll Block
}

func (u UnionMatch) GetType() Type {
	for _, block := range u.Cases {
		return block.result
	}
	panic("unreachable")
}

// Statements don't produce anything
type Statement interface{}

type VariableBinding struct {
	Name  string
	Value Expression
	mut   bool
}

type VariableAssignment struct {
	Target Expression
	Value  Expression
}

type Break struct{}

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

type ForLoop struct {
	Init      VariableBinding
	Condition Expression
	Step      Statement
	Body      Block
}

type WhileLoop struct {
	Condition Expression
	Body      []Statement
}

type Parameter struct {
	Name    string
	Type    Type
	Mutable bool
}

type FunctionDeclaration struct {
	Name       string
	Parameters []Parameter
	Body       []Statement
	Return     Type
	mutates    bool
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
		mutates:    f.mutates,
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

type Block struct {
	Body   []Statement
	result Type
}

func isMutable(expr Expression) bool {
	if id, ok := expr.(Identifier); ok {
		return id.symbol.(variable).mut
	}
	if prop, ok := expr.(InstanceProperty); ok {
		return isMutable(prop.Subject)
	}
	return true
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
		if _, ok := checker.imports[imp.Name]; ok {
			checker.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("%s Duplicate package name: %s", imp.GetLocation().Start, imp.Name),
				location: imp.GetLocation(),
			})
		} else {
			var pkg Package
			if strings.HasPrefix(imp.Path, "ard/") {
				pkg = findStdLib(imp.Path, imp.Name)
				if pkg == nil {
					checker.addDiagnostic(Diagnostic{
						Kind:     Error,
						location: imp.GetLocation(),
						Message:  fmt.Sprintf("Unknown package: %s", imp.Path),
					})
					continue
				}
			} else {
				pkg = newExternalPackage(imp.Path, imp.Name)
			}
			checker.imports[pkg.GetName()] = pkg
			checker.scope.declare(pkg)
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
		if s.Type == nil {
			var value Expression
			switch astVal := s.Value.(type) {
			case ast.ListLiteral:
				value = c.checkList(astVal, nil)
			default:
				value = c.checkExpression(astVal)
				if IsVoid(value.GetType()) {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						location: s.Value.GetLocation(),
						Message:  "Cannot assign a void value",
					})
					return nil
				}
			}

			c.scope.addVariable(variable{name: s.Name, mut: s.Mutable, _type: value.GetType()})
			return VariableBinding{Name: s.Name, Value: value, mut: s.Mutable}
		}

		expectedType := c.resolveDeclaredType(s.Type)
		if IsVoid(expectedType) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				location: s.Value.GetLocation(),
				Message:  "Cannot assign a void value",
			})
		}

		var value Expression
		switch literal := s.Value.(type) {
		case ast.ListLiteral:
			value = c.checkList(literal, expectedType)
		case ast.MapLiteral:
			value = c.checkMap(literal, expectedType)
		default:
			value = c.checkExpression(s.Value)
			if _, expectingFloat := expectedType.(Float); expectingFloat {
				if _, isInt := value.(IntLiteral); isInt {
					value = FloatLiteral{Value: float64(value.(IntLiteral).Value)}
				}
			}
			if !AreCoherent(expectedType, value.GetType()) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", expectedType, value.GetType()),
					location: s.Value.GetLocation(),
				})
				return nil
			}
		}

		c.scope.addVariable(variable{name: s.Name, mut: s.Mutable, _type: expectedType})
		return VariableBinding{Name: s.Name, Value: value, mut: s.Mutable}
	case ast.VariableAssignment:
		switch target := s.Target.(type) {
		case ast.Identifier:
			symbol := c.scope.find(target.Name)
			if symbol == nil {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Undefined: %s", target.Name),
					location: s.GetLocation(),
				})
				return nil
			}

			variable, ok := (*symbol).(variable)
			if !ok {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Undefined: %s", target.Name),
					location: s.GetLocation(),
				})
				return nil
			}
			if variable.isParam {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Cannot assign to parameter: %s", target.Name),
					location: s.GetLocation(),
				})
				return nil
			} else if !variable.mut {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Immutable variable: %s", target.Name),
					location: s.GetLocation(),
				})
				return nil
			}

			value := c.checkExpression(s.Value)
			// if value is nil, it means there was an error in the expression
			if value == nil {
				return nil
			}

			if s.Operator == ast.Increment || s.Operator == ast.Decrement {
				if !AreCoherent(variable._type, Int{}) || !AreCoherent(value.GetType(), Int{}) {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						Message:  "Increment and decrement operators can only be used on numbers",
						location: s.GetLocation(),
					})
					return nil
				}

				var operator BinaryOperator
				if s.Operator == ast.Increment {
					operator = Add
				} else {
					operator = Sub
				}

				value = BinaryExpr{
					Op:    operator,
					Left:  Identifier{Name: target.Name, symbol: variable},
					Right: c.checkExpression(s.Value),
				}
				return VariableAssignment{Target: Identifier{Name: target.Name}, Value: value}
			}

			if !AreCoherent(variable._type, value.GetType()) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", variable._type, value.GetType()),
					location: s.Value.GetLocation(),
				})
				return nil
			}
			return VariableAssignment{Target: Identifier{Name: target.Name}, Value: value}

		case ast.InstanceProperty:
			subject := c.checkExpression(target)
			if !isMutable(subject) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  "Cannot reassign in immutables",
					location: target.GetLocation(),
				})
				return nil
			}
			value := c.checkExpression(s.Value)

			if !AreCoherent(subject.GetType(), value.GetType()) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", subject.GetType(), value.GetType()),
					location: s.Value.GetLocation(),
				})
				return nil
			}
			return VariableAssignment{Target: subject, Value: value}
		default:
			panic(fmt.Errorf("Unsupported assignment subject: %s", target))
		}
	case ast.IfStatement:
		var condition Expression
		initialize := func() {
			if s.Condition != nil {
				condition = c.checkExpression(s.Condition)
				if condition.GetType() != (Bool{}) {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						Message:  "If conditions must be boolean expressions",
						location: s.Condition.GetLocation(),
					})
				}
			}
		}

		block := c.checkBlock(s.Body, initialize)

		var elseClause Statement = nil
		if s.Else != nil {
			elseClause = c.checkStatement(s.Else)
		}
		return IfStatement{Condition: condition, Body: block.Body, Else: elseClause}
	case ast.Comment:
		return nil
	case ast.Break:
		return Break{}
	case ast.RangeLoop:
		cursor := variable{name: s.Cursor.Name, mut: false, _type: Int{}}
		start := c.checkExpression(s.Start)
		end := c.checkExpression(s.End)

		startType := start.GetType()
		endType := end.GetType()
		if !AreCoherent(startType, Int{}) || !AreCoherent(startType, endType) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Invalid range: %s..%s", startType, endType),
				location: s.Start.GetLocation(),
			})
			return nil
		}
		block := c.checkBlock(s.Body, func() {
			c.scope.addVariable(cursor)
		})
		return ForRange{
			Cursor: Identifier{Name: s.Cursor.Name, symbol: cursor},
			Start:  start,
			End:    end,
			Body:   block.Body,
		}
	case ast.ForInLoop:
		iterable := c.checkExpression(s.Iterable)
		cursor := variable{name: s.Cursor.Name, mut: false, _type: iterable.GetType()}
		// getBody func allows lazy evaluation so that cursor can be updated within the switch below
		getBody := func() []Statement {
			return c.checkBlock(s.Body, func() {
				c.scope.addVariable(cursor)
			}).Body
		}

		switch iterable.GetType().(type) {
		case Int:
			return ForRange{
				Cursor: Identifier{Name: s.Cursor.Name, symbol: cursor},
				Start:  IntLiteral{Value: 0},
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
				Message:  "Cannot iterate over a Bool",
				location: s.Iterable.GetLocation(),
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

	case ast.ForLoop:
		var init VariableBinding
		var condition Expression
		var step Statement

		setup := func() {
			init = c.checkStatement(s.Init).(VariableBinding)
			condition = c.checkExpression(s.Condition)
			step = c.checkStatement(s.Incrementer)
		}

		block := c.checkBlock(s.Body, setup)

		return ForLoop{
			Init:      init,
			Condition: condition,
			Step:      step,
			Body:      block,
		}

	case ast.WhileLoop:
		condition := c.checkExpression(s.Condition)
		if condition.GetType() != (Bool{}) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  "While conditions must be boolean expressions",
				location: s.Condition.GetLocation(),
			})
		}

		block := c.checkBlock(s.Body, nil)

		return WhileLoop{
			Condition: condition,
			Body:      block.Body,
		}
	case ast.FunctionDeclaration:
		parameters := make([]Parameter, len(s.Parameters))
		blockVariables := make([]variable, len(s.Parameters))
		for i, p := range s.Parameters {
			parameters[i] = Parameter{
				Name:    p.Name,
				Type:    c.resolveDeclaredType(p.Type),
				Mutable: p.Mutable,
			}
			blockVariables[i] = variable{name: p.Name, mut: p.Mutable, isParam: true, _type: parameters[i].Type}
		}

		declaredReturnType := c.resolveDeclaredType(s.ReturnType)
		fn := function{
			name:       s.Name,
			parameters: blockVariables,
			returns:    declaredReturnType,
		}
		c.scope.declare(fn)

		block := c.checkBlock(s.Body, func() {
			for _, p := range blockVariables {
				c.scope.addVariable(p)
			}
		})
		if _, isVoid := declaredReturnType.(Void); !isVoid && !AreCoherent(declaredReturnType, block.result) {
			c.addDiagnostic(Diagnostic{
				Kind: Error,
				Message: fmt.Sprintf(
					"Type mismatch: Expected %s, got %s",
					declaredReturnType,
					block.result),
				location: s.ReturnType.GetLocation(),
			})
		}

		return FunctionDeclaration{
			Name:       s.Name,
			Parameters: parameters,
			Body:       block.Body,
			Return:     declaredReturnType,
		}
	case ast.EnumDefinition:
		if len(s.Variants) == 0 {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  "Enums must have at least one variant",
				location: s.GetLocation(),
			})
		}
		uniqueVariants := map[string]bool{}
		for _, variant := range s.Variants {
			if _, ok := uniqueVariants[variant]; ok {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Duplicate variant: %s", variant),
					location: s.GetLocation(),
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
	case ast.StructDefinition:
		fields := map[string]Type{}
		for _, field := range s.Fields {
			name := field.Name.Name
			if _, ok := fields[name]; ok {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Duplicate field: %s", name),
					location: field.Name.GetLocation(),
				})
			} else {
				fields[name] = c.resolveDeclaredType(field.Type)
			}
		}

		strct := &Struct{
			Name:    s.Name.Name,
			Fields:  fields,
			methods: map[string]FunctionDeclaration{},
		}
		if ok := c.scope.declareStruct(strct); !ok {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Duplicate struct declaration: %s", s.Name.Name),
				location: s.Name.GetLocation(),
			})
			return nil
		}
		return strct
	case ast.ImplBlock:
		_struct, ok := c.scope.getStruct(s.Self.Type.GetName())
		if !ok {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Undefined: %s", s.Self.Type.GetName()),
				location: s.GetLocation(),
			})
			return nil
		}

		new_scope := newScope(c.scope)
		c.scope = new_scope
		defer func() { c.scope = new_scope.parent }()
		new_scope.declare(variable{name: s.Self.Name, mut: s.Self.Mutable, _type: _struct})

		for _, method := range s.Methods {
			stmt := c.checkStatement(method)
			meth := stmt.(FunctionDeclaration)
			meth.mutates = s.Self.Mutable
			_struct.addMethod(s.Self.Name, meth)
		}
		return nil
	case ast.TypeDeclaration:
		types := make([]Type, len(s.Type))
		for i, t := range s.Type {
			types[i] = c.resolveDeclaredType(t)
		}
		union := Union{name: s.Name.Name, types: types}
		c.scope.declare(union)
		return nil
	default:
		return c.checkExpression(s)
	}
}

func (c *checker) checkBlock(block []ast.Statement, setup func()) Block {
	new_scope := newScope(c.scope)
	c.scope = new_scope
	defer func() { c.scope = new_scope.parent }()

	if setup != nil {
		setup()
	}

	var result Type = Void{}
	statements := []Statement{}
	for _, s := range block {
		stmt := c.checkStatement(s)
		if stmt != nil {
			statements = append(statements, stmt)
			if expr, ok := stmt.(Expression); ok {
				result = expr.GetType()
			}
		}
	}
	return Block{Body: statements, result: result}
}

func (c *checker) checkExpression(expr ast.Expression) Expression {
	switch e := expr.(type) {
	case ast.Identifier:
		sym := c.scope.find(e.Name)
		if sym == nil {
			panic(fmt.Sprintf("Undefined: %s", e.Name))
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Undefined: %s", e.Name),
				location: e.GetLocation(),
			})
			return nil
		}
		return Identifier{
			Name:   e.Name,
			symbol: *sym,
		}
	case ast.StrLiteral:
		return StrLiteral{Value: strings.Trim(e.Value, `"`)}
	case ast.NumLiteral:
		if strings.Contains(e.Value, ".") {
			value, err := strconv.ParseFloat(e.Value, 64)
			if err != nil {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Invalid float: %s", e.Value),
					location: e.GetLocation(),
				})
				return nil
			}
			return FloatLiteral{Value: value}
		}
		value, err := strconv.Atoi(e.Value)
		if err != nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Invalid int: %s", e.Value),
				location: e.GetLocation(),
			})
			return nil
		}
		return IntLiteral{Value: value}
	case ast.BoolLiteral:
		return BoolLiteral{Value: e.Value}
	case ast.InstanceProperty:
		return c.checkInstanceProperty(c.checkExpression(e.Target), e.Property)
	case ast.InstanceMethod:
		return c.checkInstanceMethod(c.checkExpression(e.Target), e.Method)
	case ast.StaticProperty:
		return c.checkStaticProperty(c.checkStaticExpression(e.Target), e.Property)
	case ast.StaticFunction:
		return c.checkStaticFunction(c.checkStaticExpression(e.Target), e.Function)
	case ast.UnaryExpression:
		expr := c.checkExpression(e.Operand)
		switch e.Operator {
		case ast.Minus:
			if !AreCoherent(Int{}, expr.GetType()) && !AreCoherent(Float{}, expr.GetType()) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  "The '-' operator can only be used on numbers",
					location: e.Operand.GetLocation(),
				})
				return nil
			}
			return Negation{Value: expr}
		case ast.Not:
			if !AreCoherent(expr.GetType(), Bool{}) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  "The 'not' keyword can only be used on booleans",
					location: e.Operand.GetLocation(),
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

		if left == nil || left.GetType() == nil {
			panic(fmt.Errorf("problem: %s", e.Left))
		}

		diagnostic := Diagnostic{
			Kind:     Error,
			location: e.GetLocation(),
			Message: fmt.Sprintf(
				"Invalid operation: %s %s %s",
				left.GetType(),
				operator,
				right.GetType()),
		}
		switch operator {
		case And, Or:
			if !AreCoherent(left.GetType(), Bool{}) || !AreCoherent(left.GetType(), right.GetType()) {
				c.addDiagnostic(diagnostic)
				return nil
			}
		case Equal, NotEqual:
			if !areComparable(left.GetType(), right.GetType()) {
				c.addDiagnostic(diagnostic)
				return nil
			}
		case GreaterThan, GreaterThanOrEqual, LessThan, LessThanOrEqual:
			notNum := !AreCoherent(Float{}, left.GetType()) && !AreCoherent(Int{}, left.GetType())
			if notNum || !AreCoherent(left.GetType(), right.GetType()) {
				c.addDiagnostic(diagnostic)
				return nil
			}
		case Mod:
			if AreCoherent(Float{}, left.GetType()) || AreCoherent(Float{}, right.GetType()) {
				diagnostic.Message = "% is not supported on Float"
				c.addDiagnostic(diagnostic)
				return nil
			}
			if !AreCoherent(Int{}, left.GetType()) || !AreCoherent(Int{}, right.GetType()) {
				c.addDiagnostic(diagnostic)
				return nil
			}
		default:
			notNum := !AreCoherent(Float{}, left.GetType()) && !AreCoherent(Int{}, left.GetType())
			if notNum || !AreCoherent(left.GetType(), right.GetType()) {
				c.addDiagnostic(diagnostic)
				return nil
			}
		}

		return BinaryExpr{Op: operator, Left: left, Right: right}
	case ast.InterpolatedStr:
		parts := make([]Expression, len(e.Chunks))
		for i, chunk := range e.Chunks {
			part := c.checkExpression(chunk)
			if !AreCoherent(part.GetType(), Str{}) && !AreCoherent(part.GetType(), Option{Str{}}) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Type mismatch: Expected Str, got %s", part.GetType()),
					location: chunk.GetLocation(),
				})
			} else {
				parts[i] = part
			}
		}
		return InterpolatedStr{Parts: parts}
	case ast.FunctionCall:
		sym := c.scope.find(e.Name)
		if sym == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Undefined: %s", e.Name),
				location: e.GetLocation(),
			})
			return nil
		}
		fn, ok := (*sym).asFunction()
		if !ok {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Not a function: %s", e.Name),
				location: e.GetLocation(),
			})
			return nil
		}

		args := make([]Expression, len(e.Args))
		if len(e.Args) != len(fn.parameters) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d", len(fn.parameters), len(e.Args)),
				location: e.GetLocation(),
			})
		} else {
			for i, arg := range e.Args {
				expr := c.checkExpression(arg)
				if !AreCoherent(fn.parameters[i]._type, expr.GetType()) {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", fn.parameters[i]._type, expr.GetType()),
						location: arg.GetLocation(),
					})
				} else {
					if fn.parameters[i].mut && !isMutable(expr) {
						c.addDiagnostic(Diagnostic{
							Kind:     Error,
							Message:  fmt.Sprintf("Type mismatch: Expected mutable %s, got %s", fn.parameters[i]._type, expr.GetType()),
							location: arg.GetLocation(),
						})
					} else {
						args[i] = expr
					}
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
		block := c.checkBlock(e.Body, func() {
			for _, p := range blockVariables {
				c.scope.addVariable(p)
			}
		})
		if !IsVoid(declaredReturnType) {
			if !AreCoherent(declaredReturnType, block.result) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					location: e.ReturnType.GetLocation(),
					Message: fmt.Sprintf(
						"Type mismatch: Expected %s, got %s",
						declaredReturnType,
						block.result),
				})
			}
		}
		return FunctionLiteral{
			Parameters: parameters,
			Return:     declaredReturnType,
			Body:       block.Body,
		}
	case ast.ListLiteral:
		return c.checkList(e, nil)
	case ast.MatchExpression:
		subject := c.checkExpression(e.Subject)
		switch sub := subject.GetType().(type) {
		case Enum:
			return c.checkEnumMatch(e, subject, sub)
		case Bool:
			return c.checkBoolMatch(e, subject)
		case Option:
			return c.checkOptionMatch(e, subject)
		case Union:
			return c.checkUnionMatch(e, subject)
		default:
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				location: e.GetLocation(),
				Message:  fmt.Sprintf("Cannot match on %s", sub),
			})
			return nil
		}

	case ast.StructInstance:
		_struct, ok := c.scope.getStruct(e.Name.Name)
		if !ok {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Undefined: %s", e.Name.Name),
				location: e.GetLocation(),
			})
			return nil
		}

		fields := map[string]Expression{}
		for _, field := range e.Properties {
			if _struct.GetProperty(field.Name.Name) == nil {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					location: field.Name.GetLocation(),
					Message:  fmt.Sprintf("Unknown field: %s", field.Name.Name),
				})
			} else {
				fields[field.Name.Name] = c.checkExpression(field.Value)
			}
		}

		for name := range _struct.Fields {
			if _, ok := fields[name]; !ok {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					location: e.GetLocation(),
					Message:  fmt.Sprintf("Missing field: %s", name),
				})
			}
		}

		instance := StructInstance{
			Name:   e.Name.Name,
			Fields: fields,
			_type:  _struct,
		}
		return instance
	case ast.MapLiteral:
		return c.checkMap(e, nil)
	default:
		panic(fmt.Sprintf("Unhandled expression: %T", e))
	}
}

func (c *checker) checkStaticExpression(expr ast.Expression) Static {
	switch e := expr.(type) {
	case ast.Identifier:
		if e.Name == "Int" {
			return Int{}
		}
		if e.Name == "Float" {
			return Float{}
		}

		sym := c.scope.find(e.Name)
		if sym == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Undefined: %s", e.Name),
				location: e.GetLocation(),
			})
			return nil
		}

		if enum, ok := (*sym).(Enum); ok {
			return enum
		}
		return nil
	default:
		panic(fmt.Sprintf("Unhandled static expression: %T", e))
	}
}

func (c *checker) checkList(expr ast.ListLiteral, declaredType Type) Expression {
	if declaredType != nil {
		elements := make([]Expression, len(expr.Items))
		for i, item := range expr.Items {
			element := c.checkExpression(item)
			if !AreCoherent(declaredType.(List).element, element.GetType()) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", declaredType, element.GetType()),
					location: item.GetLocation(),
				})
			}
			elements[i] = element
		}

		return ListLiteral{
			Elements: elements,
			_type:    declaredType.(List),
		}
	}

	if len(expr.Items) == 0 {
		c.addDiagnostic(Diagnostic{
			Kind:     Error,
			Message:  "Empty lists need an explicit type",
			location: expr.GetLocation(),
		})
		return ListLiteral{}
	}

	var elementType Type
	elements := make([]Expression, len(expr.Items))
	for i, item := range expr.Items {
		elements[i] = c.checkExpression(item)
		_type := elements[i].GetType()
		if i == 0 {
			elementType = _type
		} else if !AreCoherent(_type, elementType) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				location: item.GetLocation(),
				Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", elementType, _type),
			})
		}
	}
	return ListLiteral{
		Elements: elements,
		_type:    List{element: elementType},
	}
}

func (c *checker) checkMap(expr ast.MapLiteral, declaredType Type) Expression {
	entries := map[Expression]Expression{}
	if declaredType != nil {
		for _, entry := range expr.Entries {
			key := c.checkExpression(entry.Key)
			value := c.checkExpression(entry.Value)
			declaredKeyType := declaredType.(Map).key
			declaredValType := declaredType.(Map).value
			if !AreCoherent(declaredKeyType, key.GetType()) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", declaredKeyType, key.GetType()),
					location: entry.Key.GetLocation(),
				})
			}
			if !AreCoherent(declaredValType, value.GetType()) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", declaredValType, value.GetType()),
					location: entry.Value.GetLocation(),
				})
			}
			entries[key] = value
		}

		return MapLiteral{
			Entries: entries,
			_type:   declaredType.(Map),
		}
	}

	if len(expr.Entries) == 0 {
		c.addDiagnostic(Diagnostic{
			Kind:     Error,
			Message:  "Empty maps need an explicit type",
			location: expr.GetLocation(),
		})
		return MapLiteral{}
	}

	var keyType Type = Void{}
	var valType Type = Void{}
	for i, entry := range expr.Entries {
		key := c.checkExpression(entry.Key)
		value := c.checkExpression(entry.Value)
		if i == 0 {
			keyType = key.GetType()
			valType = value.GetType()
		} else {
			if !AreCoherent(keyType, key.GetType()) || !AreCoherent(valType, value.GetType()) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  "Map error: All entries must have the same type",
					location: expr.GetLocation(),
				})
				return MapLiteral{}
			}
		}

		entries[key] = value
	}

	return MapLiteral{
		Entries: entries,
		_type:   Map{key: keyType, value: valType},
	}
}

func (c *checker) checkEnumMatch(expr ast.MatchExpression, subject Expression, enum Enum) EnumMatch {
	expectedCases := make([]bool, len(enum.Variants))
	for i := range enum.Variants {
		expectedCases[i] = false
	}

	var pattern Expression
	var catchAll MatchCase
	cases := []Block{}

	var _type Type = Void{}
	for i, arm := range expr.Cases {
		variables := []variable{}
		var isCatchAll bool = false

		if id, ok := arm.Pattern.(ast.Identifier); ok {
			if i != len(expr.Cases)-1 {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  "Catch-all case must be last",
					location: arm.Pattern.GetLocation(),
				})
				return EnumMatch{}
			}
			pattern = nil
			if id.Name != "_" {
				variables = append(variables, variable{
					name:  id.Name,
					mut:   false,
					_type: enum,
				})
				pattern = Identifier{Name: id.Name, symbol: variables[0]}
			}
			isCatchAll = true
		} else {
			pattern = c.checkExpression(arm.Pattern)
			variant := pattern.(EnumVariant)
			if expectedCases[variant.Value] {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Duplicate case: %s", variant),
					location: arm.Pattern.GetLocation(),
				})
				return EnumMatch{}
			}
			expectedCases[variant.Value] = true
		}

		block := c.checkBlock(arm.Body, func() {
			for _, v := range variables {
				c.scope.addVariable(v)
			}
		})
		if i == 0 {
			_type = block.result
		} else if !AreCoherent(block.result, _type) {
			c.addDiagnostic(Diagnostic{
				Kind: Error,
				Message: fmt.Sprintf(
					"Type mismatch: Expected %s, got %s",
					_type,
					block.result),
				location: arm.Body[len(arm.Body)-1].GetLocation(),
			})
		}
		if isCatchAll {
			catchAll = MatchCase{
				Pattern: pattern,
				Body:    block.Body,
			}
		} else {
			cases = append(cases, block)
		}
	}

	nonExhaustive := false
	if catchAll.Body == nil {
		for value, name := range enum.Variants {
			if !expectedCases[value] {
				nonExhaustive = true
				c.addDiagnostic(Diagnostic{
					Kind: Error,
					Message: fmt.Sprintf(
						"Incomplete match: missing case for '%s'",
						enum.Name+"::"+name),
					location: expr.GetLocation(),
				})
			}
		}
	}

	if nonExhaustive {
		return EnumMatch{}
	}

	return EnumMatch{
		Subject:  subject,
		Cases:    cases,
		CatchAll: catchAll,
	}
}

func (c *checker) checkBoolMatch(expr ast.MatchExpression, subject Expression) Expression {
	var trueCase Block
	var falseCase Block

	var result Type = Void{}
	for i, arm := range expr.Cases {
		if _, isIdentifier := arm.Pattern.(ast.Identifier); isIdentifier {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				location: arm.Pattern.GetLocation(),
				Message:  "Catch-all case is not allowed for boolean matches",
			})
			return nil
		}
		pattern := c.checkExpression(arm.Pattern)

		if _, isLiteral := pattern.(BoolLiteral); !isLiteral {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  "Expected either `true` or `false`",
				location: arm.Pattern.GetLocation(),
			})
			return nil
		}

		block := c.checkBlock(arm.Body, nil)

		if pattern.(BoolLiteral).Value {
			if trueCase.Body != nil {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  "Duplicate case: 'true'",
					location: arm.Pattern.GetLocation(),
				})
			} else {
				trueCase = block
			}
		} else {
			if falseCase.Body != nil {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  "Duplicate case: 'false'",
					location: arm.Pattern.GetLocation(),
				})
			} else {
				falseCase = block
			}
		}

		if i == 0 {
			result = block.result
		} else {
			if !AreCoherent(block.result, result) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					location: arm.GetLocation(),
					Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", result, block.result),
				})
			}
		}
	}

	var missingCase string
	if trueCase.Body == nil {
		missingCase = "true"
	} else if falseCase.Body == nil {
		missingCase = "false"
	}

	if missingCase != "" {
		c.addDiagnostic(Diagnostic{
			Kind:     Error,
			location: expr.GetLocation(),
			Message:  fmt.Sprintf("Incomplete match: Missing case for '%s'", missingCase),
		})
	}

	return BoolMatch{
		Subject: subject,
		True:    trueCase,
		False:   falseCase,
	}
}

func (c *checker) checkOptionMatch(expr ast.MatchExpression, subject Expression) OptionMatch {
	var someCase MatchCase
	var noneCase Block

	var result Type = nil
	for _, arm := range expr.Cases {
		id, isIdentifier := arm.Pattern.(ast.Identifier)
		if !isIdentifier {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				location: arm.Pattern.GetLocation(),
				Message:  "Pattern must be either a variable name for the value or _ for the empty case",
			})
			return OptionMatch{}
		}

		if id.Name == "_" {
			if noneCase.Body != nil {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  "Duplicate case: empty path",
					location: arm.Pattern.GetLocation(),
				})
				continue
			}
			noneCase = c.checkBlock(arm.Body, nil)
			if result == nil {
				result = noneCase.result
			} else {
				if !AreCoherent(noneCase.result, result) {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						location: arm.GetLocation(),
						Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", result, noneCase.result),
					})
				}
			}
		} else {
			if someCase.Body != nil {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  "Duplicate case: happy path",
					location: arm.Pattern.GetLocation(),
				})
			} else {
				block := c.checkBlock(arm.Body, func() {
					c.scope.addVariable(variable{
						name:  id.Name,
						mut:   false,
						_type: subject.GetType().(Option).inner,
					})
				})
				someCase = MatchCase{
					Pattern: Identifier{Name: id.Name},
					Body:    block.Body,
					_type:   block.result,
				}

				if result == nil {
					result = block.result
				} else {
					if !AreCoherent(block.result, result) {
						c.addDiagnostic(Diagnostic{
							Kind:     Error,
							location: arm.GetLocation(),
							Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", result, noneCase.result),
						})
					}
				}
			}
		}
	}

	var missingCase string
	if someCase.Body == nil {
		missingCase = "happy path"
	} else if noneCase.Body == nil {
		missingCase = "empty path"
	}

	if missingCase != "" {
		c.addDiagnostic(Diagnostic{
			Kind:     Error,
			location: expr.GetLocation(),
			Message:  fmt.Sprintf("Incomplete match: Missing case for '%s'", missingCase),
		})
	}

	return OptionMatch{
		Subject: subject,
		None:    noneCase,
		Some:    someCase,
	}
}

func (c *checker) checkUnionMatch(expr ast.MatchExpression, subject Expression) UnionMatch {
	cases := map[Type]Block{}
	var catchAll Block

	var result Type = nil
	for i, arm := range expr.Cases {
		id, isIdentifier := arm.Pattern.(ast.Identifier)
		if !isIdentifier {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				location: arm.Pattern.GetLocation(),
				Message:  "Pattern must be a type",
			})
			return UnionMatch{}
		}

		if id.Name == "_" {
			if i != len(expr.Cases)-1 {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  "Catch-all case must be last",
					location: arm.Pattern.GetLocation(),
				})
				return UnionMatch{}
			}
			catchAll = c.checkBlock(arm.Body, nil)
			continue
		}

		union := subject.GetType().(Union)
		if it_type := union.getFor(id.Name); it_type == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Unexpected: %s is not in %s", id.Name, union),
				location: arm.Pattern.GetLocation(),
			})
			return UnionMatch{}
		} else {
			block := c.checkBlock(arm.Body, func() {
				c.scope.addVariable(variable{name: "it", mut: false, _type: it_type})
			})
			cases[it_type] = block
			if result == nil {
				result = block.result
			} else {
				if !AreCoherent(result, block.result) {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						location: arm.GetLocation(),
						Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", result, block.result),
					})
				}
			}
		}

	}

	return UnionMatch{
		Subject:  subject,
		Cases:    cases,
		CatchAll: catchAll,
	}
}

func (c *checker) checkInstanceProperty(subject Expression, member ast.Identifier) Expression {
	sig := subject.GetType().GetProperty(member.Name)
	if sig == nil {
		c.addDiagnostic(Diagnostic{
			Kind:     Error,
			Message:  fmt.Sprintf("Undefined: %s.%s", subject, member.Name),
			location: member.GetLocation(),
		})
		return nil
	}
	return InstanceProperty{
		Subject: subject,
		Property: Identifier{
			Name: member.Name,
			symbol: variable{
				name:  member.Name,
				_type: sig,
			}},
	}
}

func (c *checker) checkInstanceMethod(subject Expression, member ast.FunctionCall) Expression {
	if subject == nil || subject.GetType() == nil {
		panic(fmt.Sprintf("problem: %s", subject))
	}
	sig := subject.GetType().GetProperty(member.Name)
	if sig == nil {
		c.addDiagnostic(Diagnostic{
			Kind:     Error,
			Message:  fmt.Sprintf("Undefined: %s.%s", subject, member.Name),
			location: member.GetLocation(),
		})
		return nil
	}
	fn, ok := sig.(function)
	if !ok {
		c.addDiagnostic(Diagnostic{
			Kind:     Error,
			Message:  fmt.Sprintf("Not a function: %s.%s", subject, member.Name),
			location: member.GetLocation(),
		})
		return nil
	}

	if fn.mutates && !isMutable(subject) {
		c.addDiagnostic(Diagnostic{
			Kind:     Error,
			Message:  fmt.Sprintf("Cannot mutate immutable '%s' with '.%s()'", subject, member.Name),
			location: member.GetLocation(),
		})
		return nil
	}

	args := make([]Expression, len(member.Args))
	if len(member.Args) != len(fn.parameters) {
		c.addDiagnostic(Diagnostic{
			Kind:     Error,
			Message:  fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d", len(fn.parameters), len(member.Args)),
			location: member.GetLocation(),
		})
	} else {
		for i, arg := range member.Args {
			args[i] = c.checkExpression(arg)
			if !AreCoherent(fn.parameters[i]._type, args[i].GetType()) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", fn.parameters[i]._type, args[i].GetType()),
					location: arg.GetLocation(),
				})
			} else {
				if fn.parameters[i].mut && !isMutable(args[i]) {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						Message:  fmt.Sprintf("Type mismatch: Expected mutable %s, got %s", fn.parameters[i]._type, args[i].GetType()),
						location: arg.GetLocation(),
					})
				}
			}
		}
	}

	if pkg, ok := subject.GetType().(Package); ok {
		return PackageAccess{
			Package:  pkg,
			Property: FunctionCall{Name: member.Name, Args: args, symbol: fn},
		}
	}

	return InstanceMethod{
		Subject: subject,
		Method:  FunctionCall{Name: member.Name, Args: args, symbol: fn},
	}
}

func (c *checker) checkStaticProperty(subject Static, member ast.Identifier) Expression {
	switch s := subject.(type) {
	case Enum:
		if variant, ok := s.GetVariant(member.Name); ok {
			return variant
		}

		c.addDiagnostic(Diagnostic{
			Kind: Error,
			Message: fmt.Sprintf(
				"Undefined: %s::%s",
				subject,
				member.Name),
			location: member.GetLocation(),
		})
		return nil
	default:
		panic(fmt.Sprintf("Undefined %s::%s", s, member))
	}
}

func (c *checker) checkStaticFunction(subject Static, member ast.FunctionCall) Expression {
	switch s := subject.(type) {
	case Int:
		prop := s.GetStaticProperty(member.Name)
		if prop == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Undefined: Int::%s", member.Name),
				location: member.GetLocation(),
			})
			return nil
		}
		fn, ok := prop.(function)
		if !ok {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Not a function: Int::%s", member.Name),
				location: member.GetLocation(),
			})
			return nil
		}

		if len(member.Args) != len(fn.parameters) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d", len(fn.parameters), len(member.Args)),
				location: member.GetLocation(),
			})
			return nil
		}

		args := make([]Expression, len(member.Args))
		for i, param := range fn.parameters {
			arg := c.checkExpression(member.Args[i])
			if !AreCoherent(param._type, arg.GetType()) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", param._type, arg.GetType()),
					location: member.Args[i].GetLocation(),
				})
				return nil
			} else {
				args[i] = arg
			}
		}

		return StaticFunctionCall{
			Subject:  s,
			Function: FunctionCall{Name: member.Name, Args: args, symbol: fn},
		}

	case Float:
		prop := s.GetStaticProperty(member.Name)
		if prop == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Undefined: Float::%s", member.Name),
				location: member.GetLocation(),
			})
			return nil
		}
		fn, ok := prop.(function)
		if !ok {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Not a function: Float::%s", member.Name),
				location: member.GetLocation(),
			})
			return nil
		}

		if len(member.Args) != len(fn.parameters) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d", len(fn.parameters), len(member.Args)),
				location: member.GetLocation(),
			})
			return nil
		}

		args := make([]Expression, len(member.Args))
		for i, param := range fn.parameters {
			arg := c.checkExpression(member.Args[i])
			if !AreCoherent(param._type, arg.GetType()) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", param._type, arg.GetType()),
					location: member.Args[i].GetLocation(),
				})
				return nil
			} else {
				args[i] = arg
			}
		}

		return StaticFunctionCall{
			Subject:  s,
			Function: FunctionCall{Name: member.Name, Args: args, symbol: fn},
		}
	default:
		panic(fmt.Sprintf("Undefined %s::%s", s, member))
	}
}

func (c checker) resolveDeclaredType(t ast.DeclaredType) Type {
	if t == nil {
		return Void{}
	}

	var _type Type
	switch tt := t.(type) {
	case ast.StringType:
		_type = Str{}
	case ast.IntType:
		_type = Int{}
	case ast.FloatType:
		_type = Float{}
	case ast.BooleanType:
		_type = Bool{}
	case ast.List:
		_type = List{
			element: c.resolveDeclaredType(tt.Element),
		}
	case ast.Map:
		_type = Map{
			key:   c.resolveDeclaredType(tt.Key),
			value: c.resolveDeclaredType(tt.Value),
		}
	case ast.CustomType:
		if name := c.scope.find(tt.GetName()); name != nil {
			if custom, isType := (*name).(Type); isType {
				_type = custom
				break
			}
		}
		if _struct, ok := c.scope.getStruct(tt.GetName()); ok {
			_type = _struct
			break
		}
		c.addDiagnostic(Diagnostic{
			Kind:    Error,
			Message: fmt.Sprintf(`Undefined: %s`, tt.GetName()),
		})
	default:
		panic(fmt.Sprintf("Unhandled declared type: %T", t))
	}

	if t.IsOptional() {
		return Option{_type}
	}
	return _type
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
