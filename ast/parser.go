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
	start := useToken.getLocation().Start

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
				Start: tok.getLocation().Start,
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
				Start: tok.getLocation().Start,
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

	if p.check(pub, type_) {
		p.match(pub)
		p.match(type_)
		return p.typeUnion(true)
	}
	if p.match(type_) {
		return p.typeUnion(false)
	}

	if p.check(pub, enum) {
		p.match(pub)
		p.match(enum)
		return p.enumDef(true)
	}
	if p.match(enum) {
		return p.enumDef(false)
	}

	if p.check(pub, struct_) {
		p.match(pub)
		p.match(struct_)
		return p.structDef(true)
	}
	if p.match(struct_) {
		return p.structDef(false)
	}

	if p.check(pub, trait) {
		p.match(pub)
		p.match(trait)
		return p.traitDef(true)
	}
	if p.match(trait) {
		return p.traitDef(false)
	}
	if p.match(impl) {
		// if implementing a static reference, it's a trait
		if p.check(identifier, colon_colon) {
			return p.traitImpl()
		}
		// if implementing a local reference, could be a regular impl or trait impl
		if p.check(identifier) {
			// Look ahead to see if there's a "for" keyword after the identifier
			if p.peek2().kind == for_ {
				return p.traitImpl()
			}
		}
		return p.implBlock()
	}
	return p.assignment()
}

func (p *parser) parseVariableDef() (Statement, error) {
	start := p.previous()
	kind := start.kind
	name := p.consumeVariableName(fmt.Sprintf("Expected identifier after '%s'", string(kind)))
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
	var condition Expression

	// skip condition for infinite loop - `while { foo() }`
	if !p.match(left_brace) {
		or, err := p.or()
		if err != nil {
			return nil, err
		}
		condition = or
		p.consume(left_brace, "Expected '{' after while condition")
	}

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
	// Try to parse a for-in loop by looking ahead
	current := p.peek()
	if current.kind == identifier || p.isAllowedIdentifierKeyword(current.kind) {
		// Look ahead to see if this is a for-in loop (variable followed by ',' or 'in')
		savedIndex := p.index
		_ = p.advance() // consume the potential variable name

		// Check for for-in pattern: variable [, variable] in ...
		isForIn := p.check(in) || p.check(comma)

		// Restore position
		p.index = savedIndex

		if isForIn {
			id := p.consumeVariableName("Expected variable name")
			cursor := Identifier{
				Name:     id.text,
				Location: id.getLocation(),
			}
			cursor2 := Identifier{}
			if p.match(comma) {
				id := p.consumeVariableName("Expected a name after ','")
				cursor2.Name = id.text
				cursor2.Location = id.getLocation()
			}
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
					Cursor:  cursor,
					Cursor2: cursor2,
					Start:   seq.Start,
					End:     seq.End,
					Body:    body,
				}, nil
			}

			return &ForInLoop{
				Cursor:   cursor,
				Cursor2:  cursor2,
				Iterable: seq,
				Body:     body,
			}, nil
		}
	}

	// Parse C-style for loop: for [let|mut] variable = ...; condition; update
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

func (p *parser) typeUnion(public bool) (Statement, error) {
	decl := &TypeDeclaration{Public: true, Type: []DeclaredType{}}
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

func (p *parser) enumDef(public bool) (Statement, error) {
	nameToken := p.consume(identifier, "Expected name after 'enum'")
	enum := &EnumDefinition{Name: nameToken.text, Public: public}
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

func (p *parser) structDef(public bool) (Statement, error) {
	nameToken := p.consume(identifier, "Expected name")
	structDef := &StructDefinition{
		Public: public,
		Name:   Identifier{Name: nameToken.text},
		Fields: []StructField{},
	}
	p.consume(left_brace, "Expected '{'")
	p.match(new_line)
	for !p.match(right_brace) {
		fieldName := p.consumeVariableName("Expected field name")
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
	implToken := p.previous()

	nameToken := p.consume(identifier, "Expected type name after 'impl'")
	impl.Target = Identifier{
		Name: nameToken.text,
		Location: Location{
			Start: Point{nameToken.line, nameToken.column},
			End:   Point{nameToken.line, nameToken.column + len(nameToken.text)},
		},
	}

	p.consume(left_brace, "Expected '{'")
	p.consume(new_line, "Expected new line")

	for !p.match(right_brace) {
		// not using p.parseStatement() in order to be precise
		if p.match(new_line) {
			continue
		}
		stmt, err := p.functionDef(true)
		if err != nil {
			return nil, err
		}
		fn, ok := stmt.(*FunctionDeclaration)
		if !ok {
			return nil, fmt.Errorf("Expected function declaration in impl block")
		}
		impl.Methods = append(impl.Methods, *fn)
	}

	// Set location
	impl.Location = Location{
		Start: Point{implToken.line, implToken.column},
		End:   Point{p.previous().line, p.previous().column},
	}

	return impl, nil
}

func (p *parser) traitDef(public bool) (*TraitDefinition, error) {
	traitToken := p.previous()
	traitDef := &TraitDefinition{Public: public}

	nameToken := p.consume(identifier, "Expected trait name after 'trait'")
	traitDef.Name = Identifier{
		Name: nameToken.text,
		Location: Location{
			Start: Point{nameToken.line, nameToken.column},
			End:   Point{nameToken.line, nameToken.column + len(nameToken.text)},
		},
	}

	p.consume(left_brace, "Expected '{'")
	p.consume(new_line, "Expected new line")

	for !p.match(right_brace) {
		if p.match(new_line) {
			continue
		}
		// Parse function declaration without body (signature only)
		fnToken := p.consume(fn, "Expected function declaration in trait block")
		name := p.consume(identifier, "Expected function name")
		p.consume(left_paren, "Expected '(' after function name")

		// Parse parameters
		params := []Parameter{}
		for !p.check(right_paren) {
			if len(params) > 0 {
				p.consume(comma, "Expected ',' between parameters")
			}
			paramName := p.consumeVariableName("Expected parameter name")
			p.consume(colon, "Expected ':' after parameter name")
			paramType := p.parseType()
			params = append(params, Parameter{
				Name: paramName.text,
				Type: paramType,
			})
		}
		p.consume(right_paren, "Expected ')' after parameters")

		// Parse return type
		var returnType DeclaredType = nil
		if !p.check(new_line) {
			returnType = p.parseType()
		}

		fnLocation := Location{
			Start: Point{fnToken.line, fnToken.column},
			End:   Point{p.previous().line, p.previous().column},
		}

		// Add method to trait definition (without body since it's just a signature)
		traitDef.Methods = append(traitDef.Methods, FunctionDeclaration{
			Location:   fnLocation,
			Name:       name.text,
			Parameters: params,
			ReturnType: returnType,
			Body:       nil, // No body for trait method signatures
		})

		p.match(new_line)
	}

	// Set location
	traitDef.Location = Location{
		Start: Point{traitToken.line, traitToken.column},
		End:   Point{p.previous().line, p.previous().column},
	}

	return traitDef, nil
}

func (p *parser) traitImpl() (*TraitImplementation, error) {
	implToken := p.previous()
	traitImpl := &TraitImplementation{}

	// Parse trait name (already consumed 'impl' token)
	if path := p.parseStaticPath(); path != nil {
		traitImpl.Trait = *path
	} else {
		traitToken := p.consume(identifier, "Expected trait name after 'impl'")
		traitImpl.Trait = Identifier{
			Name: traitToken.text,
			Location: Location{
				Start: Point{traitToken.line, traitToken.column},
				End:   Point{traitToken.line, traitToken.column + len(traitToken.text)},
			},
		}
	}

	// Parse 'for'
	p.consume(for_, "Expected 'for' after trait name")

	// Parse type name
	typeToken := p.consume(identifier, "Expected type name after 'for'")
	traitImpl.ForType = Identifier{
		Name: typeToken.text,
		Location: Location{
			Start: Point{typeToken.line, typeToken.column},
			End:   Point{typeToken.line, typeToken.column + len(typeToken.text)},
		},
	}

	p.consume(left_brace, "Expected '{' after type name")
	p.consume(new_line, "Expected new line")

	for !p.match(right_brace) {
		if p.match(new_line) {
			continue
		}
		stmt, err := p.functionDef(true)
		if err != nil {
			return nil, err
		}
		fn, ok := stmt.(*FunctionDeclaration)
		if !ok {
			return nil, fmt.Errorf("Expected function declaration in trait implementation block")
		}
		traitImpl.Methods = append(traitImpl.Methods, *fn)
	}

	// Set location
	traitImpl.Location = Location{
		Start: Point{implToken.line, implToken.column},
		End:   Point{p.previous().line, p.previous().column},
	}

	return traitImpl, nil
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
	static := p.parseStaticPath()
	if static != nil {
		return &CustomType{
			Location: static.Location,
			Type:     *static,
			nullable: p.match(question_mark),
		}
	}

	// Check for function type: fn(ParamType) ReturnType
	if p.match(fn) {
		fnToken := p.previous()
		p.consume(left_paren, "Expected '(' after 'fn' in function type")

		// Parse parameter types
		paramTypes := []DeclaredType{}
		if !p.check(right_paren) {
			for {
				paramType := p.parseType()
				paramTypes = append(paramTypes, paramType)
				if !p.match(comma) {
					break
				}
			}
		}
		p.consume(right_paren, "Expected ')' after function parameters")

		// Parse return type
		returnType := p.parseType()

		// Check for nullable
		nullable := p.match(question_mark)

		return &FunctionType{
			Params:   paramTypes,
			Return:   returnType,
			nullable: nullable,
			Location: Location{
				Start: Point{Row: fnToken.line, Col: fnToken.column},
				End:   Point{Row: p.previous().line, Col: p.previous().column},
			},
		}
	}

	if p.match(identifier) {
		id := p.previous()
		nullable := false

		// Check for Result<T, E> type
		if id.text == "Result" && p.match(less_than) {
			// Parse the value type
			valType := p.parseType()
			p.consume(comma, "Expected comma after value type in Result")

			// Parse the error type
			errType := p.parseType()
			p.consume(greater_than, "Expected '>' after Result type parameters")

			// Check for nullable
			nullable = p.match(question_mark)

			// Return ResultType
			return &ResultType{
				Val:      valType,
				Err:      errType,
				nullable: nullable,
				Location: Location{
					Start: Point{Row: id.line, Col: id.column},
					End:   Point{Row: p.previous().line, Col: p.previous().column},
				},
			}
		} else {
			nullable = p.match(question_mark)

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

func (p *parser) parseStaticPath() *StaticProperty {
	if !p.check(identifier, colon_colon, identifier) {
		return nil
	}

	namespace := p.advance()
	joint := p.advance()
	propName := p.advance()

	prop := &StaticProperty{}
	prop.Target = &Identifier{
		Location: Location{
			Start: Point{namespace.line, namespace.column},
			End:   Point{joint.line, joint.column - 1},
		},
		Name: namespace.text,
	}
	prop.Property = &Identifier{
		Location: Location{
			Start: Point{propName.line, propName.column},
			End:   Point{propName.line, propName.column + len(propName.text)},
		},
		Name: propName.text,
	}

	for p.match(colon_colon) {
		propName := p.consume(identifier, "Expected an identifier after '::'")
		prop = &StaticProperty{
			Target: prop,
			Property: &Identifier{
				Location: Location{
					Start: Point{propName.line, propName.column},
					End:   Point{propName.line, propName.column + len(propName.text)},
				},
				Name: propName.text,
			},
		}
	}

	return prop
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
			pattern, err := p.iterRange()
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

	return p.try()
}

func (p *parser) try() (Expression, error) {
	if p.check(identifier) && p.peek().text == "try" {
		idToken := p.advance()
		keyword := Identifier{
			Name:     idToken.text,
			Location: idToken.getLocation(),
		}
		expr, err := p.functionDef(false)
		if err != nil {
			return nil, err
		}

		return &Try{
			keyword:    keyword,
			Expression: expr,
		}, nil
	}
	return p.functionDef(false)
}

func (p *parser) functionDef(asMethod bool) (Statement, error) {
	public := p.match(pub)
	if p.match(fn) {
		keyword := p.previous()
		var name any = ""
		mutates := p.match(mut)
		if !asMethod {
			// should this signal warning of unnecessary `mut`?
		}

		if path := p.parseStaticPath(); path != nil {
			name = path
		} else if p.check(identifier) {
			name = p.consume("identifier", "Expected function name after 'fn'").text
		}

		p.consume(left_paren, "Expected parameters list")
		params := []Parameter{}
		for !p.match(right_paren) {
			isMutable := p.match(mut)
			nameToken := p.consumeVariableName("Expected parameter name")

			// Check if this is a simple parameter list in an anonymous function
			// In case of anonymous functions with unnamed params, we don't need types
			var paramType DeclaredType
			if p.check(colon) {
				p.consume(colon, "Expected ':' after parameter name")
				paramType = p.parseType()
			} else if name == "" { // Anonymous function with untyped params
				// For anonymous functions, allow simple parameter names without types
				paramType = &StringType{} // Default to string type for now
			} else {
				return nil, fmt.Errorf("Expected ':' after parameter name")
			}

			params = append(params, Parameter{
				Mutable: isMutable,
				Name:    nameToken.text,
				Type:    paramType,
			})
			p.match(comma)
		}

		// Return type is required for all functions - except for simple anonymous functions
		var returnType DeclaredType = nil
		if (name == "" && !p.check(left_brace)) || name != "" {
			// Function with explicit return type
			returnType = p.parseType()
		}

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

		fnDef := &FunctionDeclaration{
			Public:     public,
			Mutates:    asMethod && mutates,
			Parameters: params,
			ReturnType: returnType,
			Body:       statements,
			Location: Location{
				Start: Point{Row: keyword.line, Col: keyword.column},
				End:   Point{Row: p.previous().line, Col: p.previous().column},
			},
		}

		switch name := name.(type) {
		case string:
			fnDef.Name = name
			return fnDef, nil
		case *StaticProperty:
			return &StaticFunctionDeclaration{
				FunctionDeclaration: *fnDef,
				Path:                *name,
			}, nil
		}
	}

	return p.structInstance()
}

func (p *parser) structInstance() (Expression, error) {
	index := p.index
	static := p.parseStaticPath()
	if static != nil {
		// go back 1 so the last identifier can be checked as starting the below expression
		p.index = p.index - 1
	}

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

		p.match(new_line)

		for !p.match(right_brace) {
			propToken := p.consumeVariableName("Expected name")
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
			p.match(new_line)
		}
		instance.Location.End = Point{Row: p.previous().line, Col: p.previous().column}

		if static != nil {
			static.Property = instance
			return static, nil
		}

		return instance, nil
	} else {
		// rewind after static parsing
		p.index = index
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
						Property: prop,
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
		tok := p.previous()
		return &NumLiteral{
			Value:    tok.text,
			Location: tok.getLocation(),
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
			Name:     "@",
			Location: tok.getLocation(),
		}, nil
	}
	if p.match(identifier) {
		tok := p.previous()
		return &Identifier{
			Name:     tok.text,
			Location: tok.getLocation(),
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
	// Handle keywords as identifiers when used as variables
	case and, not, or, true_, false_, struct_, enum, impl, trait, fn, let, mut,
		break_, match, while_, for_, use, as, in, if_, else_, type_, pub:
		tok := p.advance()
		name := tok.text
		if name == "" {
			name = string(tok.kind)
		}
		return &Identifier{
			Name:     name,
			Location: tok.getLocation(),
		}, nil
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
		item, err := p.functionDef(false)
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
		val, err := p.functionDef(false)
		if err != nil {
			return nil, err
		}
		entries = append(entries, MapEntry{
			Key:   key,
			Value: val,
		})
		p.match(comma)
		p.match(new_line)
	}
	return &MapLiteral{
		Entries: entries,
	}, nil
}

func (p *parser) string() (Expression, error) {
	tok := p.previous()
	str := &StrLiteral{
		Value:    tok.text,
		Location: tok.getLocation(),
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

/* consume a token that can be used as a variable name (identifier or keyword) */
func (p *parser) consumeVariableName(message string) token {
	if p.isAtEnd() {
		panic(fmt.Errorf("Unexpected end of input"))
	}

	current := p.peek()
	// Allow both identifiers and keywords as variable names
	if current.kind == identifier || p.isAllowedIdentifierKeyword(current.kind) {
		token := p.advance()
		// For keywords, we need to set the text to be the keyword string
		if token.text == "" {
			token.text = string(token.kind)
		}
		return token
	}

	panic(fmt.Errorf("%s at line %d, column %d. (Actual: %s)",
		message, p.tokens[p.index].line, p.tokens[p.index].column, current.kind))
}

/* check if a token kind is an allowed keyword */
/* `match` is excluded because it can be confusing in assignments `let foo = match.thing` */
func (p *parser) isAllowedIdentifierKeyword(k kind) bool {
	keywords := []kind{
		and, not, or, true_, false_, struct_, enum, impl, trait, fn, let, mut,
		break_, while_, for_, use, as, in, if_, else_, type_, pub,
	}
	return slices.Contains(keywords, k)
}

/* assert that the current token is the provided kind and return it */
func (p *parser) consume(kind kind, message string) token {
	if p.isAtEnd() {
		panic(fmt.Errorf("Unexpected end of input"))
	}
	if p.peek().kind == kind {
		return p.advance()
	}

	panic(fmt.Errorf("%s at line %d, column %d. (Actual: %s)",
		message, p.tokens[p.index].line, p.tokens[p.index].column, p.peek().kind))
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

func (p *parser) peek2() *token {
	if p.index+1 >= len(p.tokens) {
		return nil
	}
	return &p.tokens[p.index+1]
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
