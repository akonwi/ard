package ast

import (
	"fmt"
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
	// for p.index < len(p.tokens) && p.tokens[p.index].kind != eof {
	// 	stmt, err := p.parseStatement()
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	if stmt != nil {
	// 		program.Statements = append(program.Statements, stmt)
	// 	}
	// }

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
	token := p.tokens[p.index]

	switch token.kind {
	// case let, mut:
	// 	return p.parseVariableDeclaration()
	// case if_:
	// 	return p.parseIfStatement()
	// case while_:
	// 	return p.parseWhileLoop()
	// case for_:
	// 	return p.parseForLoop()
	// case fn:
	// 	return p.parseFunctionDeclaration()
	// case struct_:
	// 	return p.parseStructDefinition()
	// case enum:
	// 	return p.parseEnumDefinition()
	// case impl:
	// 	return p.parseImplBlock()
	// case type_:
	// 	return p.parseTypeDeclaration()
	// case break_:
	// 	p.index++
	// 	return Break{}, nil
	// case slash_slash, slash_star:
	// 	return p.parseComment()
	// case identifier:
	// 	// Could be a variable assignment or function call
	// 	if p.peekNext().kind == equal || p.peekNext().kind == increment || p.peekNext().kind == decrement {
	// 		return p.parseAssignment()
	// 	}
	// 	// Otherwise treat as expression statement
	// 	expr, err := p.parseExpression()
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	return expr, nil
	default:
		// Try parsing as expression statement
		// expr, err := p.parseExpression()
		// if err != nil {
		// 	return nil, err
		// }
		// return expr, nil
		return nil, nil
	}
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
func (p *parser) match(kind kind) bool {
	if c := p.peek(); c.kind != kind {
		return false
	}
	p.advance()
	return true
}

func (p *parser) peek() *token {
	if p.index >= len(p.tokens) {
		return nil
	}
	return &p.tokens[p.index]
}

func (p *parser) isAtEnd() bool {
	return p.tokens[p.index].kind == eof
}
