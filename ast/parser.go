package ast

import (
	"fmt"
	"slices"
	"strings"
)

type node struct {
	text       string
	start, end int
}

type parser struct {
	tokens []token
	index  int
}

func Parse(source []byte) (*Program, error) {
	p := new(NewLexer(source).Scan())
	return p.parse()
}

func new(tokens []token) *parser {
	return &parser{
		tokens: tokens,
		index:  0,
	}
}

func (p *parser) parse() (*Program, error) {
	program := &Program{
		Imports:    []Import{},
		Statements: []Statement{},
	}

	// Parse imports first
	for p.check(use) || p.check(new_line) {
		if p.match(new_line) {
			continue
		}
		p.consume(use, "Expected 'use' keyword")
		imp, err := p.parseImport()
		if err != nil {
			return nil, err
		}
		program.Imports = append(program.Imports, *imp)
	}

	// Parse statements
	for !p.isAtEnd() {
		if p.match(new_line) {
			continue
		}
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
	}

	return program, nil
}

func (p *parser) parseImport() (*Import, error) {
	useToken := p.previous()
	pathToken := p.consume(path, "Expected a module path after 'use'")
	start := Point{Row: useToken.line, Col: useToken.column}

	var name string
	if p.match(as) {
		alias := p.consume(identifier, "Expected alias name after 'as'")
		name = alias.text
	} else {
		// Default alias is last part of path
		parts := strings.Split(pathToken.text, "/")
		if len(parts) == 1 {
			name = parts[0]
		} else {
			name = parts[len(parts)-1]
		}
		name = strings.ReplaceAll(name, "-", "_")
	}
	endCol := p.previous().column
	if p.match(new_line) {
		endCol = p.previous().column - 1
	}
	end := Point{Row: pathToken.line, Col: endCol}

	return &Import{
		Path: pathToken.text,
		Name: name,
		Location: Location{
			Start: start,
			End:   end,
		},
	}, nil
}

func (p *parser) parseStatement() (Statement, error) {
	if p.match(comment, block_comment) {
		tok := p.previous()
		return &Comment{
			Value: tok.text,
			Location: Location{
				Start: Point{Row: tok.line, Col: tok.column},
				End:   Point{Row: p.peek().line, Col: p.peek().column - 1},
			},
		}, nil
	}
	if p.match(new_line) {
		return nil, nil
	}
	if p.match(break_) {
		tok := p.previous()
		new_line := p.consume(new_line, "Expected new line")
		return &Break{
			Location: Location{
				Start: Point{Row: tok.line, Col: tok.column},
				End:   Point{Row: tok.line, Col: new_line.column - 1},
			},
		}, nil
	}
	if p.match(let, mut) {
		return p.parseVariableDef()
	}
	if p.match(if_) {
		return p.ifStatement()
	}
	if p.match(while_) {
		return p.whileLoop()
	}
	if p.match(for_) {
		return p.forLoop()
	}
	if p.match(type_) {
		return p.typeUnion()
	}
	if p.match(enum) {
		return p.enumDef()
	}
	if p.match(struct_) {
		return p.structDef()
	}
	if p.match(impl) {
		return p.implBlock()
	}
	return p.assignment()
}

func (p *parser) parseVariableDef() (Statement, error) {
	start := p.previous()
	kind := start.kind
	name := p.consume(identifier, fmt.Sprintf("Expected identifier after '%s'", string(kind)))
	var declaredType DeclaredType = nil
	if p.match(colon) {
		declaredType = p.parseType()
	}
	p.consume(equal, "Expected '=' after variable name")
	value, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	endCol := p.previous().column
	if p.match(new_line) {
		endCol = p.previous().column - 1
	}
	return &VariableDeclaration{
		Mutable: kind == mut,
		Name:    name.text,
		Value:   value,
		Type:    declaredType,
		Location: Location{
			Start: Point{Row: start.line, Col: start.column},
			End:   Point{Row: start.line, Col: endCol},
		},
	}, nil
}

func (p *parser) ifStatement() (Statement, error) {
	ifToken := p.previous()
	condition, err := p.or()
	if err != nil {
		return nil, err
	}

	statements, err := p.block()
	p.match(new_line)

	stmt := &IfStatement{
		Condition: condition,
		Body:      statements,
		Location: Location{
			Start: Point{Row: ifToken.line, Col: ifToken.column},
		},
	}

	if p.match(else_) {
		if p.match(if_) {
			elseIf, err := p.ifStatement()
			if err != nil {
				return nil, err
			}
			stmt.Else = elseIf
		} else {
			elseBlock, err := p.block()
			if err != nil {
				return nil, err
			}
			stmt.Else = &IfStatement{Body: elseBlock}
		}
	}

	return stmt, nil
}

func (p *parser) whileLoop() (Statement, error) {
	condition, err := p.or()
	if err != nil {
		return nil, err
	}
	p.consume(left_brace, "Expected '{' after while condition")

	statements := []Statement{}
	for !p.check(right_brace) {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			statements = append(statements, stmt)
		}
	}
	p.consume(right_brace, "Unclosed while loop")
	p.match(new_line)

	return &WhileLoop{
		Condition: condition,
		Body:      statements,
	}, nil
}

func (p *parser) forLoop() (Statement, error) {
	if p.match(identifier) {
		cursor := Identifier{Name: p.previous().text}
		p.consume(in, "Expected 'in' after cursor name")
		seq, err := p.iterRange()
		if err != nil {
			return nil, err
		}
		body, err := p.block()
		if err != nil {
			return nil, err
		}
		if seq, ok := seq.(*RangeExpression); ok {
			return &RangeLoop{
				Cursor: cursor,
				Start:  seq.Start,
				End:    seq.End,
				Body:   body,
			}, nil
		}

		return &ForInLoop{
			Cursor:   cursor,
			Iterable: seq,
			Body:     body,
		}, nil
	}

	p.match(let, mut)
	initial, err := p.parseVariableDef()
	if err != nil {
		return nil, err
	}
	p.consume(semicolon, "Expected ';' after loop cursor")
	condition, err := p.or()
	if err != nil {
		return nil, err
	}
	p.consume(semicolon, "Expected ';' after loop condition")
	incrementer, err := p.parseStatement()
	if err != nil {
		return nil, err
	}
	p.match(semicolon)
	body, err := p.block()
	if err != nil {
		return nil, err
	}
	return &ForLoop{
		Init:        initial.(*VariableDeclaration),
		Condition:   condition,
		Incrementer: incrementer,
		Body:        body,
	}, nil
}

func (p *parser) typeUnion() (Statement, error) {
	decl := &TypeDeclaration{Type: []DeclaredType{}}
	nameToken := p.consume(identifier, "Expected name after 'type'")
	decl.Name = Identifier{Name: nameToken.text}
	p.consume(equal, "Expected '=' after type name")

	if p.check(new_line) {
		return nil, fmt.Errorf("Expected type definition after '='")
	}

	hasMore := true
	for hasMore {
		declType := p.parseType()
		decl.Type = append(decl.Type, declType)
		hasMore = p.match(pipe)
	}

	return decl, nil
}

func (p *parser) enumDef() (Statement, error) {
	nameToken := p.consume(identifier, "Expected name after 'enum'")
	enum := &EnumDefinition{Name: nameToken.text}
	p.consume(left_brace, "Expected '{'")
	p.match(new_line)
	for !p.match(right_brace) {
		variantToken := p.consume(identifier, "Expected variant name")
		enum.Variants = append(enum.Variants, variantToken.text)
		p.match(comma)
		p.match(new_line)
	}

	return enum, nil
}

func (p *parser) structDef() (Statement, error) {
	nameToken := p.consume(identifier, "Expected name")
	structDef := &StructDefinition{
		Name:   Identifier{Name: nameToken.text},
		Fields: []StructField{},
	}
	p.consume(left_brace, "Expected '{'")
	p.match(new_line)
	for !p.match(right_brace) {
		fieldName := p.consume(identifier, "Expected field name")
		p.consume(colon, "Expected ':'")
		fieldType := p.parseType()
		structDef.Fields = append(structDef.Fields, StructField{
			Name: Identifier{Name: fieldName.text},
			Type: fieldType,
		})
		p.match(comma)
		p.match(new_line)
	}

	return structDef, nil
}

func (p *parser) implBlock() (*ImplBlock, error) {
	impl := &ImplBlock{}

	isMutable := p.match(mut) // Check for "impl mut Type {}" syntax
	typeToken := p.consume(identifier, "Expected type name after 'impl'")
	typeDecl := &CustomType{
		Name: typeToken.text,
	}

	impl.Self = Parameter{
		Mutable: isMutable,
		Name:    "@", // Use @ as the self name
		Type:    typeDecl,
	}

	p.consume(left_brace, "Expected '{'")
	p.consume(new_line, "Expected new line")

	for !p.match(right_brace) {
		// not using p.parseStatement() in order to be precise
		if p.match(new_line) {
			continue
		}
		stmt, err := p.functionDef()
		if err != nil {
			return nil, err
		}
		fn, ok := stmt.(*FunctionDeclaration)
		if !ok {
			return nil, fmt.Errorf("Expected function declaration in impl block")
		}
		impl.Methods = append(impl.Methods, *fn)
	}

	return impl, nil
}

func (p *parser) block() ([]Statement, error) {
	p.consume(left_brace, "Expected block")
	p.match(new_line)
	statements := []Statement{}
	for !p.check(right_brace) {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		statements = append(statements, stmt)
	}
	p.consume(right_brace, "Unclosed block")
	return statements, nil
}

func (p *parser) assignment() (Statement, error) {
	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	if p.match(equal, increment, decrement) {
		opToken := p.previous()
		value, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		p.match(new_line)

		switch opToken.kind {
		case increment:
			return &VariableAssignment{
				Operator: Assign,
				Target:   expr,
				Value: &BinaryExpression{
					Operator: Plus,
					Left:     expr,
					Right:    value,
				},
			}, nil
		case decrement:
			return &VariableAssignment{
				Operator: Assign,
				Target:   expr,
				Value: &BinaryExpression{
					Operator: Minus,
					Left:     expr,
					Right:    value,
				},
			}, nil
		default:
			return &VariableAssignment{
				Operator: Assign,
				Target:   expr,
				Value:    value,
			}, nil
		}
	}
	p.match(new_line)

	return expr, nil
}

func (p *parser) parseType() DeclaredType {
	if p.match(identifier) {
		id := p.previous()
		nullable := p.match(question_mark)

		// Check if this is a generic type parameter (starts with $)
		if len(id.text) > 0 && id.text[0] == '$' {
			return &GenericType{
				Name:     id.text[1:], // Remove the leading '$'
				nullable: nullable,
			}
		}

		switch id.text {
		case "Int":
			return &IntType{nullable: nullable}
		case "Float":
			return &FloatType{nullable: nullable}
		case "Str":
			return &StringType{nullable: nullable}
		case "Bool":
			return &BooleanType{nullable: nullable}
		default:
			return &CustomType{
				Name:     id.text,
				nullable: nullable,
			}
		}
	}
	if p.match(left_bracket) {
		elementType := p.parseType()
		if p.match(colon) {
			valElementType := p.parseType()
			p.consume(right_bracket, "Expected ']'")
			return &Map{
				Key:      elementType,
				Value:    valElementType,
				nullable: p.match(question_mark),
			}
		}
		p.consume(right_bracket, "Expected ']'")

		return &List{
			Element:  elementType,
			nullable: p.match(question_mark),
		}
	}
	return nil
}

func (p *parser) parseExpression() (Expression, error) {
	return p.matchExpr()
}

func (p *parser) matchExpr() (Expression, error) {
	if p.match(match) {
		keyword := p.previous()
		matchExpr := &MatchExpression{
			Location: Location{
				Start: Point{Row: keyword.line, Col: keyword.column},
			},
		}
		expr, err := p.or()
		if err != nil {
			return nil, err
		}
		p.consume(left_brace, "Expected '{'")
		p.consume(new_line, "Expected new line")
		for !p.match(right_brace) {
			if p.match(new_line) {
				continue
			}
			pattern, err := p.or()
			if err != nil {
				return nil, err
			}
			p.consume(fat_arrow, "Expected '=>'")
			body := []Statement{}
			if p.check(left_brace) {
				b, err := p.block()
				if err != nil {
					return nil, err
				}
				body = b
			} else {
				stmt, err := p.parseExpression()
				if err != nil {
					return nil, err
				}
				body = append(body, stmt)
			}

			matchExpr.Cases = append(matchExpr.Cases, MatchCase{
				Pattern: pattern,
				Body:    body,
			})
			p.match(comma)
		}

		matchExpr.Location.End = Point{Row: p.previous().line, Col: p.previous().column}
		matchExpr.Subject = expr
		return matchExpr, nil
	}

	return p.functionDef()
}

func (p *parser) functionDef() (Statement, error) {
	if p.match(fn) {
		keyword := p.previous()
		name := ""
		if p.check(identifier) {
			name = p.consume("identifier", "Expected function name after 'fn'").text
		}
		p.consume(left_paren, "Expected parameters list")
		params := []Parameter{}
		for !p.match(right_paren) {
			isMutable := p.match(mut)
			nameToken := p.consume(identifier, "Expected parameter name")
			p.consume(colon, "Expected ':' after parameter name")
			paramType := p.parseType()
			params = append(params, Parameter{
				Mutable: isMutable,
				Name:    nameToken.text,
				Type:    paramType,
			})
			p.match(comma)
		}
		returnType := p.parseType()

		statements, err := p.block()
		if err != nil {
			return nil, err
		}

		if name == "" {
			return &AnonymousFunction{
				Parameters: params,
				ReturnType: returnType,
				Body:       statements,
				Location: Location{
					Start: Point{Row: keyword.line, Col: keyword.column},
					End:   Point{Row: p.previous().line, Col: p.previous().column},
				},
			}, nil
		}

		return &FunctionDeclaration{
			Name:       name,
			Parameters: params,
			ReturnType: returnType,
			Body:       statements,
			Location: Location{
				Start: Point{Row: keyword.line, Col: keyword.column},
				End:   Point{Row: p.previous().line, Col: p.previous().column},
			},
		}, nil
	}

	return p.structInstance()
}

func (p *parser) structInstance() (Expression, error) {
	if p.check(identifier, left_brace) {
		nameToken := p.consume(identifier, "Expected struct name")
		p.consume(left_brace, "Expected '{'")
		instance := &StructInstance{
			Name:       Identifier{Name: nameToken.text},
			Properties: []StructValue{},
			Location: Location{
				Start: Point{Row: nameToken.line, Col: nameToken.column},
			},
		}

		for !p.match(right_brace) {
			propToken := p.consume(identifier, "Expected name")
			p.consume(colon, "Expected ':'")
			val, err := p.or()
			if err != nil {
				return nil, err
			}
			instance.Properties = append(instance.Properties, StructValue{
				Name:  Identifier{Name: propToken.text},
				Value: val,
			})
			p.match(comma)
		}
		instance.Location.End = Point{Row: p.previous().line, Col: p.previous().column}
		return instance, nil
	}

	return p.iterRange()
}

func (p *parser) iterRange() (Expression, error) {
	start, err := p.or()
	if err != nil {
		return nil, err
	}

	if p.match(dot_dot) {
		end, err := p.or()
		if err != nil {
			return nil, err
		}

		return &RangeExpression{
			Start: start,
			End:   end,
		}, nil
	}

	return start, nil
}

func (p *parser) or() (Expression, error) {
	left, err := p.and()
	if err != nil {
		return nil, err
	}

	if p.match(or) {
		right, err := p.or()
		if err != nil {
			return nil, err
		}
		return &BinaryExpression{
			Operator: Or,
			Left:     left,
			Right:    right,
		}, nil
	}
	return left, nil
}

func (p *parser) and() (Expression, error) {
	left, err := p.comparison()
	if err != nil {
		return nil, err
	}
	if p.match(and) {
		right, err := p.and()
		if err != nil {
			return nil, err
		}
		return &BinaryExpression{
			Operator: And,
			Left:     left,
			Right:    right,
		}, nil
	}
	return left, nil
}

func (p *parser) comparison() (Expression, error) {
	left, err := p.modulo()
	if err != nil {
		return nil, err
	}
	for p.match(greater_than, greater_than_equal, less_than, less_than_equal, equal_equal) {
		opToken := p.previous()
		var operator Operator
		switch opToken.kind {
		case greater_than:
			operator = GreaterThan
		case greater_than_equal:
			operator = GreaterThanOrEqual
		case less_than:
			operator = LessThan
		case less_than_equal:
			operator = LessThanOrEqual
		case equal_equal:
			operator = Equal
		}
		right, err := p.modulo()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpression{
			Operator: operator,
			Left:     left,
			Right:    right,
		}
	}
	return left, nil
}

func (p *parser) modulo() (Expression, error) {
	left, err := p.addition()
	if err != nil {
		return nil, err
	}
	if p.match(percent) {
		right, err := p.addition()
		if err != nil {
			return nil, err
		}
		return &BinaryExpression{
			Operator: Modulo,
			Left:     left,
			Right:    right,
		}, nil
	}
	return left, nil
}

func (p *parser) addition() (Expression, error) {
	left, err := p.multiplication()
	if err != nil {
		return nil, err
	}

	for p.match(plus, minus) {
		opToken := p.previous()
		operator := Plus
		if opToken.kind == minus {
			operator = Minus
		}
		right, err := p.multiplication()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpression{
			Operator: operator,
			Left:     left,
			Right:    right,
		}
	}
	return left, nil
}

func (p *parser) multiplication() (Expression, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for p.match(star, slash) {
		opToken := p.previous()
		operator := Multiply
		if opToken.kind == slash {
			operator = Divide
		}

		right, err := p.unary()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpression{
			Operator: operator,
			Left:     left,
			Right:    right,
		}
	}
	return left, nil
}

func (p *parser) unary() (Expression, error) {
	if p.match(minus, not) {
		opToken := p.previous()
		if opToken.kind == not {
			operand, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			return &UnaryExpression{
				Operator: Not,
				Operand:  operand,
			}, nil
		} else {
			operand, err := p.unary()
			if err != nil {
				return nil, err
			}
			return &UnaryExpression{
				Operator: Minus,
				Operand:  operand,
			}, nil
		}
	}
	return p.memberAccess()
}

func (p *parser) memberAccess() (Expression, error) {
	expr, err := p.call()
	if err != nil {
		return nil, err
	}

	if id, ok := expr.(*Identifier); ok && id.Name == "@" {
		call, err := p.call()
		if err != nil {
			return nil, err
		}

		switch prop := call.(type) {
		case *Identifier:
			expr = &InstanceProperty{
				Target:   expr,
				Property: *prop,
			}
		case *FunctionCall:
			expr = &InstanceMethod{
				Target: expr,
				Method: *prop,
			}
		}
	}
	for p.match(dot, colon_colon) {
		if p.previous().kind == dot {
			call, err := p.call()
			if err != nil {
				return nil, err
			}

			switch prop := call.(type) {
			case *Identifier:
				expr = &InstanceProperty{
					Target:   expr,
					Property: *prop,
				}
			case *FunctionCall:
				expr = &InstanceMethod{
					Target: expr,
					Method: *prop,
				}
			}
		} else {
			// Check for type arguments in static function calls
			if p.check(identifier, less_than) {
				// This is a static function call with type arguments
				funcName := p.consume(identifier, "Expected function name")

				// Parse type arguments
				p.consume(less_than, "Expected '<'")
				typeArgs := []DeclaredType{}

				typeArg := p.parseType()
				typeArgs = append(typeArgs, typeArg)

				for p.match(comma) {
					typeArg = p.parseType()
					typeArgs = append(typeArgs, typeArg)
				}

				p.consume(greater_than, "Expected '>' after type arguments")

				// Parse arguments
				p.consume(left_paren, "Expected '(' after type arguments")
				args := []Expression{}

				for !p.check(right_paren) {
					arg, err := p.parseExpression()
					if err != nil {
						return nil, err
					}
					args = append(args, arg)
					p.match(comma)
				}

				p.consume(right_paren, "Expected ')' to close function call")

				// Create the StaticFunction with type arguments
				expr = &StaticFunction{
					Target: expr,
					Function: FunctionCall{
						Name:     funcName.text,
						TypeArgs: typeArgs,
						Args:     args,
						Location: Location{
							Start: Point{Row: funcName.line, Col: funcName.column},
							End:   Point{Row: p.previous().line, Col: p.previous().column},
						},
					},
				}
			} else {
				call, err := p.call()
				if err != nil {
					return nil, err
				}

				switch prop := call.(type) {
				case *Identifier:
					expr = &StaticProperty{
						Target:   expr,
						Property: *prop,
					}
				case *FunctionCall:
					expr = &StaticFunction{
						Target:   expr,
						Function: *prop,
					}
				}
			}
		}
	}
	return expr, nil
}

func (p *parser) call() (Expression, error) {
	expr, err := p.primary()
	if err != nil {
		return nil, err
	}

	// todo: to support chaining, wrap in for loop
	// ex: foo()()()

	// Check if it's a function call with potential type arguments
	// Only parse as type arguments if we have an identifier followed by <
	_, isIdentifier := expr.(*Identifier)

	if isIdentifier && p.check(less_than) && !p.check(less_than, number) && !p.check(less_than, identifier, less_than) {
		// Look ahead to see if this is a type argument or a comparison
		p.advance() // consume the '<'

		// Save position so we can rewind if this isn't a type argument
		savedIndex := p.index

		// Try to parse as a type
		typeArg := p.parseType()

		// If we have a '>' after parsing the type, this is probably a type argument
		if p.check(greater_than) {
			p.advance() // consume the '>'

			// If a left parenthesis follows, this is definitely a generic function call
			if p.check(left_paren) {
				// This is a function call with generic type arguments
				typeArgs := []DeclaredType{typeArg}

				// Parse arguments
				p.consume(left_paren, "Expected '(' after type arguments")
				args := []Expression{}
				for !p.check(right_paren) {
					arg, err := p.parseExpression()
					if err != nil {
						return nil, err
					}
					args = append(args, arg)
					p.match(comma)
				}
				p.consume(right_paren, "Unclosed function call")

				return &FunctionCall{
					Name:     expr.(*Identifier).Name,
					TypeArgs: typeArgs,
					Args:     args,
					Location: Location{
						Start: expr.GetLocation().Start,
						End:   Point{Row: p.previous().line, Col: p.previous().column},
					},
				}, nil
			}
		}

		// Rewind if this wasn't a type argument
		p.index = savedIndex - 1 // go back to before the '<'
	}

	if p.match(left_paren) {
		// Regular function call without type arguments
		args := []Expression{}
		for !p.check(right_paren) {
			arg, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			p.match(comma)
		}
		p.consume(right_paren, "Unclosed function call")
		return &FunctionCall{
			Name: expr.(*Identifier).Name,
			Args: args,
			Location: Location{
				Start: expr.GetLocation().Start,
				End:   Point{Row: p.previous().line, Col: p.previous().column},
			},
		}, nil
	}

	return expr, nil
}

func (p *parser) primary() (Expression, error) {
	if p.match(number) {
		return &NumLiteral{
			Value: p.previous().text,
			Location: Location{
				Start: Point{Row: p.previous().line, Col: p.previous().column},
				End:   Point{Row: p.previous().line, Col: p.previous().column + len(p.previous().text) - 1},
			},
		}, nil
	}
	if p.match(string_) {
		return p.string()
	}
	if p.match(true_, false_) {
		return &BoolLiteral{
			Value: p.previous().text == "true",
		}, nil
	}
	if p.match(at_sign) {
		// Handle @ token as a special identifier
		tok := p.previous()
		return &Identifier{
			Name: "@",
			Location: Location{
				Start: Point{Row: tok.line, Col: tok.column},
				End:   Point{Row: tok.line, Col: tok.column},
			},
		}, nil
	}
	if p.match(identifier) {
		tok := p.previous()
		return &Identifier{
			Name: tok.text,
			Location: Location{
				Start: Point{Row: tok.line, Col: tok.column},
				End:   Point{Row: tok.line, Col: tok.column + len(tok.text) - 1},
			},
		}, nil
	}
	if p.match(left_paren) {
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		p.consume(right_paren, "Expected ')' after expression")
		return expr, nil
	}
	if p.match(left_bracket) {
		return p.list()
	}
	switch tok := p.peek(); tok.kind {
	// will i regret this?
	case and, or, fn, match:
		name := string(p.advance().kind)
		return &Identifier{Name: name}, nil
	default:
		peek := p.peek()
		panic(fmt.Errorf("unmatched primary expression at %d,%d: %s", peek.line, peek.column, peek.kind))
	}
}

func (p *parser) list() (Expression, error) {
	if p.check(colon) {
		return p.map_()
	}

	p.match(new_line)

	start := p.index
	items := []Expression{}
	for !p.match(right_bracket) {
		item, err := p.structInstance()
		if err != nil {
			return nil, err
		}
		if p.check(colon) {
			p.index = start
			return p.map_()
		}

		items = append(items, item)
		p.match(comma)
		p.match(new_line)
	}
	return &ListLiteral{Items: items}, nil
}

func (p *parser) map_() (Expression, error) {
	if p.match(colon) {
		p.consume(right_bracket, "Expected ']' after ':' in empty map")
		return &MapLiteral{Entries: []MapEntry{}}, nil
	}

	entries := []MapEntry{}
	for !p.match(right_bracket) {
		key, err := p.primary()
		if err != nil {
			return nil, err
		}
		p.consume(colon, "Expected ':' after map key")
		val, err := p.or()
		if err != nil {
			return nil, err
		}
		entries = append(entries, MapEntry{
			Key:   key,
			Value: val,
		})
		p.match(comma)
	}
	return &MapLiteral{
		Entries: entries,
	}, nil
}

func (p *parser) string() (Expression, error) {
	str := &StrLiteral{
		Value: p.previous().text,
		Location: Location{
			Start: Point{Row: p.previous().line, Col: p.previous().column},
			End:   Point{Row: p.previous().line, Col: p.previous().column + len(p.previous().text) - 1},
		},
	}
	if p.match(expr_open) {
		chunks := []Expression{str}
		for !p.match(expr_close) {
			expr, err := p.or()
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, expr)
		}
		for p.match(string_) {
			more, err := p.string()
			if err != nil {
				return nil, err
			}
			if more.String() != "" {
				chunks = append(chunks, more)
			}
		}
		return &InterpolatedStr{
			Chunks: chunks,
			Location: Location{
				Start: str.GetLocation().Start,
				End:   Point{Row: p.peek().line, Col: p.peek().column},
			},
		}, nil
	}

	return str, nil
}

func (p *parser) advance() token {
	if !p.isAtEnd() {
		p.index++
	}
	return p.tokens[p.index-1]
}

/* assert that the current token is the provided kind and return it */
func (p *parser) consume(kind kind, message string) token {
	if p.isAtEnd() {
		panic(fmt.Errorf("Unexpected end of input"))
	}
	if p.peek().kind == kind {
		return p.advance()
	}

	panic(fmt.Errorf("%s at line %d, column %d",
		message, p.tokens[p.index].line, p.tokens[p.index].column))
}

/* conditionally advance if the current token is one of those provided */
func (p *parser) match(kinds ...kind) bool {
	if slices.Contains(kinds, p.peek().kind) {
		p.advance()
		return true
	}
	return false
}

// check if the provided sequence of tokens is next
func (p *parser) check(kind ...kind) bool {
	for i, k := range kind {
		if p.index+i >= len(p.tokens) {
			return false
		}
		if p.tokens[p.index+i].kind != k {
			return false
		}
	}
	return true
}

func (p *parser) peek() *token {
	if p.index >= len(p.tokens) {
		return nil
	}
	return &p.tokens[p.index]
}

func (p *parser) previous() *token {
	if p.index == 0 {
		return nil
	}
	return &p.tokens[p.index-1]
}

func (p *parser) isAtEnd() bool {
	return p.tokens[p.index].kind == eof
}
