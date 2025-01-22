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
	if p.Path == "ard/io" {
		switch name {
		case "print":
			return function{
				name:       name,
				parameters: []variable{{name: "string", mut: false, _type: Str{}}},
				returns:    Void{},
			}

		case "read_line":
			return function{
				name:       name,
				parameters: []variable{},
				returns:    Str{},
			}

		default:
			return nil
		}
	}
	if p.Path == "ard/option" {
		switch name {
		case "make":
			return function{
				name:       name,
				parameters: []variable{},
				returns:    Option{},
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

type StructInstance struct {
	Name   string
	Fields map[string]Expression
	_type  *Struct
}

func (s StructInstance) GetType() Type {
	return s._type
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

type Block struct {
	Body   []Statement
	result Type
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
				Message:  fmt.Sprintf("%s Duplicate package name: %s", imp.GetLocation().Start, imp.Name),
				location: imp.GetLocation(),
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
		var value Expression
		var _type Type = Void{}
		if literal, isList := s.Value.(ast.ListLiteral); isList && s.Type != nil {
			_type = c.resolveDeclaredType(s.Type)
			value = c.checkList(literal, _type)
		} else {
			value = c.checkExpression(s.Value)
			// get declared type if it exists
			if s.Type != nil {
				_type = c.resolveDeclaredType(s.Type)
				if _type.Is(value.GetType()) == false {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", _type, value.GetType()),
						location: s.Value.GetLocation(),
					})
					return nil
				}
			}

			// TODO: we've already checked for type mismatches at this point,
			// if this is not declared as option, we can safely use the value's type
			// but for now, we'll just use the value's type when inference is necessary
			if _type.Is(Void{}) {
				_type = value.GetType()
			}
			if _type.Is(Void{}) || _type == nil {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					location: s.Value.GetLocation(),
					Message:  "Cannot assign a void value",
				})
			}
		}

		c.scope.addVariable(variable{name: s.Name, mut: s.Mutable, _type: _type})
		return VariableBinding{Name: s.Name, Value: value, Mut: s.Mutable}
	case ast.VariableAssignment:
		symbol := c.scope.find(s.Name)
		if symbol == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Undefined: %s", s.Name),
				location: s.GetLocation(),
			})
			return nil
		}
		variable, ok := (*symbol).(variable)
		if !ok {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Undefined: %s", s.Name),
				location: s.GetLocation(),
			})
			return nil
		}
		if !variable.mut {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Immutable variable: %s", s.Name),
				location: s.GetLocation(),
			})
			return nil
		}
		value := c.checkExpression(s.Value)
		if !variable._type.Is(value.GetType()) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", variable._type, value.GetType()),
				location: s.Value.GetLocation(),
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
					Message:  "If conditions must be boolean expressions",
					location: s.Condition.GetLocation(),
				})
			}
		}

		block := c.checkBlock(s.Body, []variable{})

		var elseClause Statement = nil
		if s.Else != nil {
			elseClause = c.checkStatement(s.Else)
		}
		return IfStatement{Condition: condition, Body: block.Body, Else: elseClause}
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
				Message:  fmt.Sprintf("Invalid range: %s..%s", startType, endType),
				location: s.Start.GetLocation(),
			})
			return nil
		}
		block := c.checkBlock(s.Body, []variable{cursor})
		return ForRange{
			Cursor: Identifier{Name: s.Cursor.Name, symbol: cursor},
			Start:  start,
			End:    end,
			Body:   block.Body,
		}
	case ast.ForLoop:
		iterable := c.checkExpression(s.Iterable)
		cursor := variable{name: s.Cursor.Name, mut: false, _type: iterable.GetType()}
		// getBody func allows lazy evaluation so that cursor can be updated within the switch below
		getBody := func() []Statement {
			return c.checkBlock(s.Body, []variable{cursor}).Body
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
	case ast.WhileLoop:
		condition := c.checkExpression(s.Condition)
		if condition.GetType() != (Bool{}) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  "While conditions must be boolean expressions",
				location: s.Condition.GetLocation(),
			})
		}

		block := c.checkBlock(s.Body, []variable{})
		return WhileLoop{
			Condition: condition,
			Body:      block.Body,
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
		fn := function{
			name:       s.Name,
			parameters: blockVariables,
			returns:    declaredReturnType,
		}
		c.scope.declare(fn)

		block := c.checkBlock(s.Body, blockVariables)
		if !declaredReturnType.Is(Void{}) && !declaredReturnType.Is(block.result) {
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
		new_scope.declare(variable{name: s.Self.Name, mut: false, _type: _struct})

		for _, method := range s.Methods {
			stmt := c.checkStatement(method)
			meth := stmt.(FunctionDeclaration)
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

func (c *checker) checkBlock(block []ast.Statement, variables []variable) Block {
	new_scope := newScope(c.scope)
	c.scope = new_scope
	defer func() { c.scope = new_scope.parent }()

	for _, variable := range variables {
		c.scope.addVariable(variable)
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
		value, err := strconv.Atoi(e.Value)
		if err != nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Invalid number: %s", e.Value),
				location: e.GetLocation(),
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
					Message:  "The '-' operator can only be used on numbers",
					location: e.Operand.GetLocation(),
				})
				return nil
			}
			return Negation{Value: expr}
		case ast.Bang:
			if !expr.GetType().Is(Bool{}) {
				c.addDiagnostic(Diagnostic{
					Kind:     Error,
					Message:  "The '!' operator can only be used on booleans",
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
			if !left.GetType().Is(Bool{}) || !right.GetType().Is(Bool{}) {
				c.addDiagnostic(diagnostic)
				return nil
			}
		case Equal, NotEqual:
			if (left.GetType() != Num{}) && (left.GetType() != Bool{}) && (left.GetType() != Str{}) {
				c.addDiagnostic(diagnostic)
				return nil
			}
		case GreaterThan, GreaterThanOrEqual, LessThan, LessThanOrEqual:
			if !left.GetType().Is(Num{}) || !right.GetType().Is(Num{}) {
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
			part := c.checkExpression(chunk)
			// todo: check if part has as_str
			if part.GetType() != (Str{}) {
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
				if expr != nil && !fn.parameters[i]._type.Is(expr.GetType()) {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", fn.parameters[i]._type, expr.GetType()),
						location: arg.GetLocation(),
					})
				} else {
					args[i] = expr
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
		block := c.checkBlock(e.Body, blockVariables)
		if !declaredReturnType.Is(Void{}) {
			if !declaredReturnType.Is(block.result) {
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
	default:
		panic(fmt.Sprintf("Unhandled expression: %T", e))
	}
}

func (c *checker) checkList(expr ast.ListLiteral, declaredType Type) Expression {
	if declaredType != nil {
		elements := make([]Expression, len(expr.Items))
		for i, item := range expr.Items {
			element := c.checkExpression(item)
			if !declaredType.(List).element.Is(element.GetType()) {
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
		} else if !_type.Is(elementType) {
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

		block := c.checkBlock(arm.Body, variables)
		if i == 0 {
			_type = block.result
		} else if !block.result.Is(_type) {
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

		block := c.checkBlock(arm.Body, []variable{})

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
			if !block.result.Is(result) {
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
			noneCase = c.checkBlock(arm.Body, []variable{})
			if result == nil {
				result = noneCase.result
			} else {
				if !noneCase.result.Is(result) {
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
				block := c.checkBlock(arm.Body, []variable{{name: id.Name, mut: false, _type: subject.GetType().(Option).inner}})
				someCase = MatchCase{
					Pattern: Identifier{Name: id.Name},
					Body:    block.Body,
					_type:   block.result,
				}

				if result == nil {
					result = block.result
				} else {
					if !block.result.Is(result) {
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
			catchAll = c.checkBlock(arm.Body, []variable{})
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
			block := c.checkBlock(
				arm.Body,
				[]variable{
					{name: "it", mut: false, _type: it_type},
				},
			)
			cases[it_type] = block
			if result == nil {
				result = block.result
			} else {
				if !result.Is(block.result) {
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

func (c *checker) checkInstanceProperty(subject Expression, member ast.Expression) Expression {
	switch m := member.(type) {
	case ast.Identifier:
		sig := subject.GetType().GetProperty(m.Name)
		if sig == nil {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Undefined: %s.%s", subject, m.Name),
				location: m.GetLocation(),
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
				Message:  fmt.Sprintf("Undefined: %s.%s", subject, m.Name),
				location: m.GetLocation(),
			})
			return nil
		}
		fn, ok := sig.(function)
		if !ok {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Not a function: %s", m.Name),
				location: m.GetLocation(),
			})
			return nil
		}
		args := make([]Expression, len(m.Args))
		if len(m.Args) != len(fn.parameters) {
			c.addDiagnostic(Diagnostic{
				Kind:     Error,
				Message:  fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d", len(fn.parameters), len(m.Args)),
				location: m.GetLocation(),
			})
		} else {
			for i, arg := range m.Args {
				args[i] = c.checkExpression(arg)
				if !fn.parameters[i]._type.Is(args[i].GetType()) {
					c.addDiagnostic(Diagnostic{
						Kind:     Error,
						Message:  fmt.Sprintf("Type mismatch: Expected %s, got %s", fn.parameters[i]._type, args[i].GetType()),
						location: arg.GetLocation(),
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
		if variant, ok := s.GetVariant(member.(ast.Identifier).Name); ok {
			return variant
		}

		c.addDiagnostic(Diagnostic{
			Kind: Error,
			Message: fmt.Sprintf(
				"Undefined: %s::%s",
				subject,
				member.(ast.Identifier).Name),
			location: member.GetLocation(),
		})
		return nil
	default:
		panic(fmt.Sprintf("Unsupported static access for %T", s))
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
	case ast.NumberType:
		_type = Num{}
	case ast.BooleanType:
		_type = Bool{}
	case ast.Void:
		_type = Void{}
	case ast.List:
		_type = List{
			element: c.resolveDeclaredType(tt.Element),
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
