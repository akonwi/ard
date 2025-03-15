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

func newParser(tokens []token) *parser {
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
	for p.match(use) {
		imp, err := p.parseImport()
		if err != nil {
			return nil, err
		}
		program.Imports = append(program.Imports, *imp)
	}

	// Parse statements
	for !p.isAtEnd() {
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
	pathToken := p.consume(path, "Expected a module path after 'use'")

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
	p.match(new_line)

	return &Import{
		Path: pathToken.text,
		Name: name,
	}, nil
}

func (p *parser) parseStatement() (Statement, error) {
	if p.match(let, mut) {
		return p.parseVariableDef()
	}
	return p.expressionStatement()
}

func (p *parser) parseVariableDef() (Statement, error) {
	kind := p.tokens[p.index-1].kind
	name := p.consume(identifier, "Expected identifier after variable declaration")
	var declaredType DeclaredType
	if p.match(colon) {
		typeToken := p.consume(identifier, "Expected a type name after ':'")
		switch typeToken.text {
		case "Int":
			declaredType = IntType{}
		case "Float":
			declaredType = FloatType{}
		case "Str":
			declaredType = StringType{}
		case "Bool":
			declaredType = BooleanType{}
		default:
			declaredType = CustomType{
				Name: typeToken.text,
			}
		}
	}
	p.consume(equal, "Expected '=' after variable name")
	value, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	p.match(new_line)
	return &VariableDeclaration{
		Mutable: kind == mut,
		Name:    name.text,
		Value:   value,
		Type:    declaredType,
	}, nil
}

func (p *parser) expressionStatement() (Statement, error) {
	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	p.match(new_line)
	return expr, nil
}

func (p *parser) parseExpression() (Expression, error) {
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
	expr, err := p.primary()
	if err != nil {
		return nil, err
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
	return expr, nil
}

func (p *parser) call() (Expression, error) {
	name := p.consume(identifier, "Expected identifier after '.'")
	if p.match(left_paren) {
		args := []Expression{}
		for !p.check(right_paren) {
			arg, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
		p.consume(right_paren, "Unclosed function call")
		return &FunctionCall{
			Name: name.text,
			Args: args,
		}, nil
	}

	return &Identifier{
		Name: name.text,
	}, nil
}

func (p *parser) primary() (Expression, error) {
	if p.match(number) {
		return &NumLiteral{
			Value: p.previous().text,
		}, nil
	}
	if p.match(complex_string) {
		return p.interpolatedString()
	}
	if p.match(string_) {
		return &StrLiteral{
			Value: p.previous().text,
		}, nil
	}
	if p.match(true_, false_) {
		return &BoolLiteral{
			Value: p.previous().text == "true",
		}, nil
	}
	if p.match(identifier) {
		return &Identifier{
			Name: p.previous().text,
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
	panic(fmt.Errorf("unmatched primary expression: %s", p.peek().kind))
}

func (p *parser) interpolatedString() (Expression, error) {
	chunks := []Expression{}
	tok := p.previous()
	for i := range tok.chunks {
		chunk := tok.chunks[i]
		if chunk.kind == string_ {
			chunks = append(chunks, &StrLiteral{
				Value: chunk.text,
			})
		} else {
			expr, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, expr)
		}
	}

	if len(chunks) == 1 {
		return chunks[0], nil
	}
	return &InterpolatedStr{
		Chunks: chunks,
	}, nil
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

/* conditionally advance if the current token is the provided kind */
func (p *parser) match(kinds ...kind) bool {
	if slices.Contains(kinds, p.peek().kind) {
		p.advance()
		return true
	}
	return false
}

func (p *parser) check(kind kind) bool {
	return p.peek().kind == kind
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
