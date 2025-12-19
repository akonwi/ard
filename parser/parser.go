package parser

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

type ParseError struct {
	Location Location
	Message  string
}

type ParseResult struct {
	fileName string
	Program  *Program
	Errors   []ParseError
}

func (pr ParseResult) PrintErrors() {
	fmt.Fprintf(os.Stderr, "Parse errors:\n")
	for _, err := range pr.Errors {
		fmt.Fprintf(os.Stderr, "  %s %s %s\n", pr.fileName, err.Location.Start, err.Message)
	}
}

type parser struct {
	tokens   []token
	index    int
	fileName string
	errors   []ParseError
}

func Parse(source []byte, fileName string) ParseResult {
	p := new(NewLexer(source).Scan(), fileName)
	program, err := p.parse()

	result := ParseResult{
		fileName: fileName,
		Program:  program,
		Errors:   p.errors,
	}

	// If there was a panic-based error, add it to errors
	if err != nil && len(p.errors) == 0 {
		result.Errors = append(result.Errors, ParseError{
			Message: err.Error(),
		})
	}

	return result
}

func new(tokens []token, fileName string) *parser {
	return &parser{
		tokens:   tokens,
		index:    0,
		fileName: fileName,
		errors:   []ParseError{},
	}
}

func (p *parser) addError(at *token, msg string) {
	location := Location{}
	if at != nil {
		location = at.getLocation()
	}

	p.errors = append(p.errors, ParseError{
		Location: location,
		Message:  msg,
	})
}

func (p *parser) skipNewlines() {
	for p.match(new_line) {
		// continue
	}
}

// synchronize skips tokens until reaching a statement boundary or EOF
func (p *parser) synchronize() {
	for !p.check(new_line) && !p.isAtEnd() {
		p.advance()
	}
}

// synchronizeToBlockEnd skips tokens until reaching the matching closing brace, accounting for nested blocks
func (p *parser) synchronizeToBlockEnd() {
	braceCount := 1 // We're already inside a block
	for braceCount > 0 && !p.isAtEnd() {
		if p.check(left_brace) {
			braceCount++
		} else if p.check(right_brace) {
			braceCount--
		}
		p.advance()
	}
}

// synchronizeToTokens skips tokens until reaching one of the specified target tokens.
// Automatically handles nesting when targeting closing brackets (}, ], ), >).
func (p *parser) synchronizeToTokens(tokens ...kind) {
	nestingLevel := 0

	// Auto-detect if nesting is needed by checking for closing brackets
	needsNesting := false
	for _, token := range tokens {
		if token == right_paren || token == right_bracket ||
			token == right_brace || token == greater_than {
			needsNesting = true
			break
		}
	}

	for !p.isAtEnd() {
		current := p.peek().kind

		if needsNesting {
			// Track nesting for all bracket types
			switch current {
			case left_paren, left_bracket, left_brace, less_than:
				nestingLevel++
			case right_paren, right_bracket, right_brace, greater_than:
				nestingLevel--
			}
		}

		// Only stop at target tokens if nesting is balanced (or nesting not needed)
		if !needsNesting || nestingLevel == 0 {
			for _, token := range tokens {
				if current == token {
					return // Found target token, stop here (don't consume it)
				}
			}
		}

		p.advance()
	}
}

func (p *parser) parseComment() *Comment {
	// If not a comment, return nil
	if !p.check(comment) {
		return nil
	}

	tok := p.advance()
	line_break := p.advance()
	return &Comment{
		Value: tok.text,
		Location: Location{
			Start: tok.getLocation().Start,
			End:   line_break.getLocation().Start,
		},
	}
}

// parseInlineComment parses a comment token and returns a cleaned Comment object.
// Returns nil if the current token is not a comment.
func (p *parser) parseInlineComment() *Comment {
	if !p.check(comment) {
		return nil
	}

	commentToken := p.advance()
	// Strip "//" and leading whitespace
	commentText := commentToken.text[2:] // Remove "//"
	for len(commentText) > 0 && (commentText[0] == ' ' || commentText[0] == '\t') {
		commentText = commentText[1:]
	}
	// Strip trailing whitespace
	for len(commentText) > 0 && (commentText[len(commentText)-1] == ' ' || commentText[len(commentText)-1] == '\t') {
		commentText = commentText[:len(commentText)-1]
	}

	return &Comment{
		Location: commentToken.getLocation(),
		Value:    commentText,
	}
}

func (p *parser) parse() (*Program, error) {
	program := &Program{
		Imports:    []Import{},
		Statements: []Statement{},
	}

	// Parse imports first
	importing := true
	for importing {
		if imp := p.parseImport(); imp != nil {
			program.Imports = append(program.Imports, *imp)
		} else {
			if c := p.parseComment(); c != nil {
				program.Statements = append(program.Statements, c)
			}
			// Continue import phase while the current token is an empty line, 'use', or comment
			importing = p.check(new_line) || p.check(use) || p.check(comment)
		}
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

func (p *parser) parseImport() *Import {
	// Skip any leading newlines
	p.skipNewlines()

	// If not 'use', return nil (end of import section)
	if !p.check(use) {
		return nil
	}

	// We have 'use' - consume it
	useToken := p.advance()
	start := useToken.getLocation().Start

	// Check for missing path
	if !p.check(path) {
		p.addError(p.peek(), "Expected module path after 'use'")
		p.synchronize()
		return nil
	}

	pathToken := p.advance()

	// Parse optional alias
	var name string
	if p.match(as) {
		if !p.check(identifier) {
			p.addError(p.peek(), "Expected alias name after 'as'")
			return nil
		} else {
			alias := p.advance()
			name = alias.text
		}
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
	}
}

func (p *parser) parseStatement() (Statement, error) {
	if c := p.parseComment(); c != nil {
		return c, nil
	}
	if p.match(new_line) {
		return nil, nil
	}
	if p.match(break_) {
		tok := p.previous()

		// Check for expected newline
		if !p.check(new_line) {
			p.addError(p.peek(), "Expected new line")
			// Recovery: Create Break without consuming newline, use current position
			return &Break{
				Location: Location{
					Start: tok.getLocation().Start,
					End:   Point{Row: tok.line, Col: tok.column + len("break")},
				},
			}, nil
		}

		new_line := p.advance()
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

	if p.check(private, type_) {
		p.match(private)
		p.match(type_)
		return p.typeUnion(true)
	}
	if p.match(type_) {
		return p.typeUnion(false)
	}

	if p.check(private, enum) {
		p.match(private)
		p.match(enum)
		return p.enumDef(true), nil
	}
	if p.match(enum) {
		return p.enumDef(false), nil
	}

	if p.check(private, struct_) {
		p.match(private)
		p.match(struct_)
		return p.structDef(true), nil
	}
	if p.match(struct_) {
		return p.structDef(false), nil
	}

	if p.check(private, trait) {
		p.match(private)
		p.match(trait)
		return p.traitDef(true), nil
	}
	if p.match(trait) {
		return p.traitDef(false), nil
	}
	if p.match(impl) {
		// if implementing a static reference, it's a trait
		if p.check(identifier, colon_colon) {
			return p.traitImpl()
		}
		// if there's a "for" keyword directly after impl (missing trait name)
		if p.check(for_) {
			return p.traitImpl()
		}
		// if implementing a local reference, could be a regular impl or trait impl
		if p.check(identifier) {
			// Look ahead to see if there's a "for" keyword after the identifier
			if p.peek2().kind == for_ {
				return p.traitImpl()
			}
			// If there's an identifier after the first identifier, treat as malformed trait impl
			// e.g., "impl Display Person" -> should be "impl Display for Person"
			if p.peek2().kind == identifier {
				return p.traitImpl()
			}
		}
		return p.implBlock(), nil
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
	if !p.check(equal) {
		p.addError(p.peek(), "Expected '=' after variable name")
		// Recovery: Skip to next statement boundary - missing '=' makes declaration invalid
		return nil, nil
	}

	p.advance() // consume the '='
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
		if !p.check(left_brace) {
			p.addError(p.peek(), "Expected '{' after while condition")
			// Recovery: Skip malformed while loop
			return nil, nil
		}
		p.advance() // consume the '{'
	}

	statements := []Statement{}
	for !p.check(right_brace) && !p.isAtEnd() {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			statements = append(statements, stmt)
		}
	}
	if !p.check(right_brace) {
		p.addError(p.peek(), "Unclosed while loop")
		// Recovery: Create while loop with statements parsed so far
		return &WhileLoop{
			Condition: condition,
			Body:      statements,
		}, nil
	}
	p.advance() // consume the '}'
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
			if !p.check(in) {
				p.addError(p.peek(), "Expected 'in' after cursor name")
				// Recovery: Skip malformed for-in loop
				return nil, nil
			}
			p.advance() // consume 'in'
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
	if !p.check(semicolon) {
		p.addError(p.peek(), "Expected ';' after loop cursor")
		// Recovery: Skip malformed C-style for loop
		return nil, nil
	}
	p.advance() // consume first ';'

	condition, err := p.or()
	if err != nil {
		return nil, err
	}

	if !p.check(semicolon) {
		p.addError(p.peek(), "Expected ';' after loop condition")
		// Recovery: Skip malformed C-style for loop
		return nil, nil
	}
	p.advance() // consume second ';'
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

func (p *parser) typeUnion(private bool) (Statement, error) {
	decl := &TypeDeclaration{Private: private, Type: []DeclaredType{}}

	if !p.check(identifier) {
		p.addError(p.peek(), "Expected name after 'type'")
		p.synchronize()
		return nil, nil
	}
	nameToken := p.advance()
	decl.Name = Identifier{Name: nameToken.text}

	if !p.check(equal) {
		p.addError(p.peek(), "Expected '=' after type name")
		p.synchronize()
		return nil, nil
	}
	p.advance()

	if p.check(new_line) {
		return nil, p.makeError(p.peek(), "Expected type definition after '='")
	}

	hasMore := true
	for hasMore {
		declType := p.parseType()
		decl.Type = append(decl.Type, declType)
		hasMore = p.match(pipe)
	}

	return decl, nil
}

func (p *parser) enumDef(private bool) Statement {
	if !p.check(identifier) {
		p.addError(p.peek(), "Expected name after 'enum'")
		p.synchronize()
		return nil
	}
	nameToken := p.advance()
	enum := &EnumDefinition{Name: nameToken.text, Private: private}

	if !p.check(left_brace) {
		p.addError(p.peek(), "Expected '{'")
		p.synchronize()
		return nil
	}
	p.advance()

	p.match(new_line)
	for !p.match(right_brace) {
		// Parse and collect comments
		if c := p.parseInlineComment(); c != nil {
			enum.Comments = append(enum.Comments, *c)
			p.match(new_line) // consume newline after comment
			continue
		}

		// Skip standalone newlines
		if p.match(new_line) {
			continue
		}

		if !p.check(identifier) {
			// Skip empty variant (graceful recovery)
			if p.match(comma) {
				p.match(new_line)
				continue
			}
			// If not a comma, we might be at end or have other issues
			break
		}
		variantToken := p.advance()
		enum.Variants = append(enum.Variants, variantToken.text)
		p.match(comma)
		p.match(new_line)
	}

	return enum
}

func (p *parser) structDef(private bool) Statement {
	if !p.check(identifier) {
		p.addError(p.peek(), "Expected name after 'struct'")
		p.synchronize()
		return nil
	}
	nameToken := p.advance()
	structDef := &StructDefinition{
		Private: private,
		Name:    Identifier{Name: nameToken.text},
		Fields:  []StructField{},
	}

	if !p.check(left_brace) {
		p.addError(p.peek(), "Expected '{'")
		p.synchronize()
		return nil
	}
	p.advance()

	p.match(new_line)
	for !p.check(right_brace) {
		// Parse and collect comments
		if c := p.parseInlineComment(); c != nil {
			structDef.Comments = append(structDef.Comments, *c)
			p.match(new_line) // consume newline after comment
			continue
		}

		// Skip standalone newlines
		if p.match(new_line) {
			continue
		}

		// Check for field name (identifier or allowed keywords)
		current := p.peek()
		if !(current.kind == identifier || p.isAllowedIdentifierKeyword(current.kind)) {
			// If we can't parse a field, skip to end of line and continue
			p.addError(p.peek(), "Expected field name")
			p.synchronize()
			break
		}

		fieldName := p.advance()
		// For keywords, we need to set the text to be the keyword string
		if fieldName.text == "" {
			fieldName.text = string(fieldName.kind)
		}

		if !p.check(colon) {
			p.addError(p.peek(), "Expected ':' after field name")
			// Skip this malformed field and continue with next
			p.synchronize()
			break
		}
		p.advance()
		fieldType := p.parseType()
		structDef.Fields = append(structDef.Fields, StructField{
			Name: Identifier{Name: fieldName.text},
			Type: fieldType,
		})

		// Check for inline comment after field type
		if c := p.parseInlineComment(); c != nil {
			structDef.Comments = append(structDef.Comments, *c)
		}

		// After parsing field type, we expect either a comma (more fields) or closing brace (end of struct)
		if p.check(comma) {
			p.advance()
			p.match(new_line)
		} else if p.check(right_brace) {
			// End of struct - will be handled by loop condition
			break
		} else if p.check(new_line) {
			// Allow newline without comma for last field
			p.advance()
		} else {
			p.addError(p.peek(), "Expected ',' or '}' after field type")
			p.synchronize()
			break
		}
	}

	if !p.check(right_brace) {
		p.addError(p.peek(), "Expected '}'")
		return structDef
	}
	p.advance()

	return structDef
}

func (p *parser) implBlock() *ImplBlock {
	impl := &ImplBlock{}
	implToken := p.previous()

	if !p.check(identifier) {
		p.addError(p.peek(), "Expected type name after 'impl'")
		p.synchronize()
		return nil
	}
	nameToken := p.advance()
	impl.Target = Identifier{
		Name: nameToken.text,
		Location: Location{
			Start: Point{nameToken.line, nameToken.column},
			End:   Point{nameToken.line, nameToken.column + len(nameToken.text)},
		},
	}

	if !p.check(left_brace) {
		p.addError(p.peek(), "Expected '{'")
		p.synchronize()
		return nil
	}
	p.advance()

	if !p.check(new_line) {
		p.addError(p.peek(), "Expected new line after '{'")
		// Continue parsing - this is not a critical error
	} else {
		p.advance()
	}

	for !p.match(right_brace) {
		// Parse and collect comments
		if c := p.parseInlineComment(); c != nil {
			impl.Comments = append(impl.Comments, *c)
			p.match(new_line) // consume newline after comment
			continue
		}

		// Skip standalone newlines
		if p.match(new_line) {
			continue
		}

		stmt, err := p.functionDef(true)
		if err != nil {
			// For now, keep the old error handling until functionDef is converted
			p.addError(p.peek(), err.Error())
			p.synchronizeToBlockEnd()
			break
		}
		fn, ok := stmt.(*FunctionDeclaration)
		if !ok {
			p.addError(p.peek(), "Expected function declaration in impl block")
			p.synchronizeToBlockEnd()
			break
		}
		impl.Methods = append(impl.Methods, *fn)
	}

	// Set location
	impl.Location = Location{
		Start: Point{implToken.line, implToken.column},
		End:   Point{p.previous().line, p.previous().column},
	}

	return impl
}

func (p *parser) traitDef(private bool) *TraitDefinition {
	traitToken := p.previous()
	traitDef := &TraitDefinition{Private: private}

	if !p.check(identifier) {
		p.addError(p.peek(), "Expected trait name after 'trait'")
		p.synchronize()
		return nil
	}
	nameToken := p.advance()
	traitDef.Name = Identifier{
		Name: nameToken.text,
		Location: Location{
			Start: Point{nameToken.line, nameToken.column},
			End:   Point{nameToken.line, nameToken.column + len(nameToken.text)},
		},
	}

	if !p.check(left_brace) {
		p.addError(p.peek(), "Expected '{'")
		p.synchronize()
		return nil
	}
	p.advance()

	if !p.check(new_line) {
		p.addError(p.peek(), "Expected new line after '{'")
		// Continue parsing - non-critical error
	} else {
		p.advance()
	}

	for !p.match(right_brace) {
		// Parse and collect comments
		if c := p.parseInlineComment(); c != nil {
			traitDef.Comments = append(traitDef.Comments, *c)
			p.match(new_line) // consume newline after comment
			continue
		}

		// Skip standalone newlines
		if p.match(new_line) {
			continue
		}

		// Parse function declaration without body (signature only)
		if !p.check(fn) {
			p.addError(p.peek(), "Expected function declaration in trait block")
			p.synchronizeToBlockEnd()
			break
		}
		fnToken := p.advance()

		if !p.check(identifier) {
			p.addError(p.peek(), "Expected function name")
			p.synchronizeToBlockEnd()
			break
		}
		name := p.advance()

		if !p.check(left_paren) {
			p.addError(p.peek(), "Expected '(' after function name")
			p.synchronizeToBlockEnd()
			break
		}
		p.advance()

		// Parse parameters
		params := []Parameter{}
		for !p.check(right_paren) {
			if len(params) > 0 {
				if !p.check(comma) {
					p.addError(p.peek(), "Expected ',' between parameters")
					break
				}
				p.advance()
			}

			// Use same logic as struct fields for parameter name parsing
			current := p.peek()
			if !(current.kind == identifier || p.isAllowedIdentifierKeyword(current.kind)) {
				p.addError(p.peek(), "Expected parameter name")
				break
			}
			paramName := p.advance()
			if paramName.text == "" {
				paramName.text = string(paramName.kind)
			}

			if !p.check(colon) {
				p.addError(p.peek(), "Expected ':' after parameter name")
				break
			}
			p.advance()

			paramType := p.parseType()
			params = append(params, Parameter{
				Name: paramName.text,
				Type: paramType,
			})
		}

		if !p.check(right_paren) {
			p.addError(p.peek(), "Expected ')' after parameters")
			p.synchronizeToBlockEnd()
			break
		}
		p.advance()

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

		// Check for inline comment after method signature
		if c := p.parseInlineComment(); c != nil {
			traitDef.Comments = append(traitDef.Comments, *c)
		}

		p.match(new_line)
	}

	// Set location
	traitDef.Location = Location{
		Start: Point{traitToken.line, traitToken.column},
		End:   Point{p.previous().line, p.previous().column},
	}

	return traitDef
}

func (p *parser) traitImpl() (*TraitImplementation, error) {
	implToken := p.previous()
	traitImpl := &TraitImplementation{}

	// Parse trait name (already consumed 'impl' token)
	if path := p.parseStaticPath(); path != nil {
		traitImpl.Trait = *path
	} else {
		if p.check(identifier) {
			traitToken := p.advance()
			traitImpl.Trait = Identifier{
				Name: traitToken.text,
				Location: Location{
					Start: Point{traitToken.line, traitToken.column},
					End:   Point{traitToken.line, traitToken.column + len(traitToken.text)},
				},
			}
		} else {
			p.addError(p.peek(), "Expected trait name after 'impl'")
			// Create placeholder identifier to maintain AST structure
			current := p.peek()
			traitImpl.Trait = Identifier{
				Name: "",
				Location: Location{
					Start: Point{current.line, current.column},
					End:   Point{current.line, current.column},
				},
			}
			p.synchronizeToBlockEnd()
			return traitImpl, nil
		}
	}

	// Parse 'for'
	if !p.match(for_) {
		p.addError(p.peek(), "Expected 'for' after trait name")
		p.synchronizeToBlockEnd()
		return traitImpl, nil
	}

	// Parse type name
	if p.check(identifier) {
		typeToken := p.advance()
		traitImpl.ForType = Identifier{
			Name: typeToken.text,
			Location: Location{
				Start: Point{typeToken.line, typeToken.column},
				End:   Point{typeToken.line, typeToken.column + len(typeToken.text)},
			},
		}
	} else {
		p.addError(p.peek(), "Expected type name after 'for'")
		// Create placeholder identifier to maintain AST structure
		current := p.peek()
		traitImpl.ForType = Identifier{
			Name: "",
			Location: Location{
				Start: Point{current.line, current.column},
				End:   Point{current.line, current.column},
			},
		}
		p.synchronizeToBlockEnd()
		return traitImpl, nil
	}

	if !p.match(left_brace) {
		p.addError(p.peek(), "Expected '{' after type name")
		p.synchronizeToBlockEnd()
		return traitImpl, nil
	}

	if !p.match(new_line) {
		p.addError(p.peek(), "Expected new line")
		// Continue parsing block contents even without newline
	}

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
			return nil, p.makeError(p.peek(), "Expected function declaration in trait implementation block")
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
	if !p.check(left_brace) {
		p.addError(p.peek(), "Expected '{'")
		p.synchronizeToTokens(left_brace)
		if !p.check(left_brace) {
			// Could not find opening brace, return empty block
			return []Statement{}, nil
		}
	}
	p.advance() // consume the '{'

	p.match(new_line)
	statements := []Statement{}
	for !p.check(right_brace) {
		// Prevent infinite loops when closing brace is missing
		if p.isAtEnd() {
			p.addError(p.peek(), "Expected '}' to close block")
			break
		}

		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		statements = append(statements, stmt)
	}

	if !p.check(right_brace) {
		p.addError(p.peek(), "Expected '}' to close block")
		// Return statements we've parsed so far
		return statements, nil
	}
	p.advance() // consume the '}'

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
				Location: Location{
					Start: expr.GetLocation().Start,
					End:   value.GetLocation().End,
				},
				Operator: Assign,
				Target:   expr,
				Value: &BinaryExpression{
					Location: Location{
						Start: expr.GetLocation().Start,
						End:   value.GetLocation().End,
					},
					Operator: Plus,
					Left:     expr,
					Right:    value,
				},
			}, nil
		case decrement:
			return &VariableAssignment{
				Location: Location{
					Start: expr.GetLocation().Start,
					End:   value.GetLocation().End,
				},
				Operator: Assign,
				Target:   expr,
				Value: &BinaryExpression{
					Location: Location{
						Start: expr.GetLocation().Start,
						End:   value.GetLocation().End,
					},
					Operator: Minus,
					Left:     expr,
					Right:    value,
				},
			}, nil
		default:
			return &VariableAssignment{
				Location: Location{
					Start: expr.GetLocation().Start,
					End:   value.GetLocation().End,
				},
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
		// Check for Result sugar syntax
		if p.match(bang) {
			// Parse the error type
			errType := p.parseType()
			// Check for nullable
			nullable := p.match(question_mark)
			// Create the value type as a CustomType with StaticProperty
			valType := &CustomType{
				Name:     static.String(),
				Location: static.Location,
				Type:     *static,
				nullable: false,
			}
			// Return ResultType using sugar syntax
			return &ResultType{
				Val:      valType,
				Err:      errType,
				nullable: nullable,
				Location: Location{
					Start: static.Location.Start,
					End:   Point{Row: p.previous().line, Col: p.previous().column},
				},
			}
		}
		return &CustomType{
			Name:     static.String(),
			Location: static.Location,
			Type:     *static,
			nullable: p.match(question_mark),
		}
	}

	// Check for function type: fn(ParamType) ReturnType
	if p.match(fn) {
		fnToken := p.previous()

		// Expect opening paren
		hasLeftParen := p.match(left_paren)
		if !hasLeftParen {
			p.addError(p.peek(), "Expected '(' after 'fn' in function type")
			// Skip until we find type boundaries
			p.synchronizeToTokens(equal, new_line)
		}

		// Parse parameter types
		paramTypes := []DeclaredType{}
		if hasLeftParen && !p.check(right_paren) {
			for {
				paramType := p.parseType()
				paramTypes = append(paramTypes, paramType)
				if !p.match(comma) {
					break
				}
			}
		}

		// Expect closing paren (only if we had opening paren)
		hasRightParen := false
		if hasLeftParen {
			hasRightParen = p.match(right_paren)
			if !hasRightParen {
				p.addError(p.peek(), "Expected ')' after function parameters")
				// Skip until we find type boundaries
				p.synchronizeToTokens(equal, new_line)
			}
		}

		// Parse return type directly (no arrow in Ard syntax)
		returnType := p.parseType()

		// Check for nullable
		nullable := p.match(question_mark)

		return &FunctionType{
			Params:   paramTypes,
			Return:   returnType,
			Nullable: nullable,
			Location: Location{
				Start: Point{Row: fnToken.line, Col: fnToken.column},
				End:   Point{Row: p.previous().line, Col: p.previous().column},
			},
		}
	}

	if _type := p.parseNamedType(); _type != nil {
		if p.match(bang) {
			valType := _type
			errType := p.parseType()

			if p.match(question_mark) {
				p.addError(p.previous(), "Unexpected '?': Result can't be nullable")
			}

			return &ResultType{
				Val:      valType,
				Err:      errType,
				nullable: false,
				Location: Location{
					Start: valType.GetLocation().Start,
					End:   Point{Row: p.previous().line, Col: p.previous().column},
				},
			}
		}

		return _type
	}

	if p.match(left_bracket) {
		bracket := p.previous()
		elementType := p.parseType()
		if p.match(colon) {
			valElementType := p.parseType()
			if !p.match(right_bracket) {
				p.addError(p.peek(), "Expected ']'")
				p.synchronizeToTokens(equal, new_line, comma, right_paren)
			}
			endBracket := p.previous()

			// Check for Result sugar syntax: [Key:Value]!ErrorType
			if p.match(bang) {
				errType := p.parseType()
				nullable := p.match(question_mark)
				mapType := &Map{
					Location: Location{
						Start: bracket.getLocation().Start,
						End:   endBracket.getLocation().Start,
					},
					Key:      elementType,
					Value:    valElementType,
					nullable: false,
				}
				return &ResultType{
					Val:      mapType,
					Err:      errType,
					nullable: nullable,
					Location: Location{
						Start: bracket.getLocation().Start,
						End:   Point{Row: p.previous().line, Col: p.previous().column},
					},
				}
			}

			return &Map{
				Key:      elementType,
				Value:    valElementType,
				nullable: p.match(question_mark),
				Location: Location{
					Start: bracket.getLocation().Start,
					End:   endBracket.getLocation().Start,
				},
			}
		}
		if !p.match(right_bracket) {
			p.addError(p.peek(), "Expected ']'")
			p.synchronizeToTokens(equal, new_line, comma, right_paren)
		}

		// Check for Result sugar syntax: [Type]!ErrorType
		if p.match(bang) {
			errType := p.parseType()
			nullable := p.match(question_mark)
			listType := &List{
				Location: bracket.getLocation(),
				Element:  elementType,
				nullable: false,
			}
			return &ResultType{
				Val:      listType,
				Err:      errType,
				nullable: nullable,
				Location: Location{
					Start: bracket.getLocation().Start,
					End:   Point{Row: p.previous().line, Col: p.previous().column},
				},
			}
		}

		return &List{
			Location: bracket.getLocation(),
			Element:  elementType,
			nullable: p.match(question_mark),
		}
	}

	return nil
}

func (p *parser) parseNamedType() DeclaredType {
	if !p.match(identifier) {
		return nil
	}

	id := p.previous()

	// Check for generic arguments
	var typeArgs []DeclaredType
	if p.match(less_than) {
		// Loop to parse comma-separated types until '>'
		for !p.check(greater_than) && !p.isAtEnd() {
			typeArgs = append(typeArgs, p.parseType())
			if !p.match(comma) {
				break // No comma, so expect '>' next
			}
		}
		if !p.match(greater_than) {
			p.addError(p.peek(), "Expected '>' to close generic type arguments")
		}
	}

	nullable := p.match(question_mark)

	// Check if this is a generic (starts with $)
	if len(id.text) > 0 && id.text[0] == '$' {
		return &GenericType{
			Location: id.getLocation(),
			Name:     id.text[1:], // Remove the leading '$'
			nullable: nullable,
		}
	}

	switch id.text {
	case "Int":
		return &IntType{
			Location: id.getLocation(),
			nullable: nullable,
		}
	case "Float":
		return &FloatType{
			Location: id.getLocation(),
			nullable: nullable,
		}
	case "Str":
		return &StringType{
			Location: id.getLocation(),
			nullable: nullable,
		}
	case "Bool":
		return &BooleanType{
			Location: id.getLocation(),
			nullable: nullable,
		}
	case "Void":
		return &VoidType{
			Location: id.getLocation(),
			nullable: nullable,
		}
	default:
		return &CustomType{
			Location: id.getLocation(),
			Name:     id.text,
			nullable: nullable,
			TypeArgs: typeArgs,
		}
	}
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
		if p.check(identifier) {
			propName := p.advance()
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
		} else {
			p.addError(p.peek(), "Expected an identifier after '::'")
			// Just break - don't create placeholder, let higher level handle it
			break
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

		// Check if this is a conditional match (no subject expression)
		if p.check(left_brace) {
			return p.parseConditionalMatch(*keyword)
		}

		// Regular match with subject expression
		matchExpr := &MatchExpression{
			Location: Location{
				Start: Point{Row: keyword.line, Col: keyword.column},
			},
		}
		expr, err := p.or()
		if err != nil {
			return nil, err
		}

		if !p.check(left_brace) {
			p.addError(p.peek(), "Expected '{'")
			p.synchronizeToTokens(left_brace, new_line)
			if !p.check(left_brace) {
				// Could not find opening brace, return incomplete match expression
				matchExpr.Subject = expr
				return matchExpr, nil
			}
		}
		p.advance() // consume the '{'

		if !p.check(new_line) {
			p.addError(p.peek(), "Expected new line after '{'")
			// Continue parsing - this is not a critical error
		} else {
			p.advance()
		}

		for !p.match(right_brace) {
			// Parse and collect comments
			if c := p.parseInlineComment(); c != nil {
				matchExpr.Comments = append(matchExpr.Comments, *c)
				p.match(new_line) // consume newline after comment
				continue
			}

			// Skip standalone newlines
			if p.match(new_line) {
				continue
			}
			pattern, err := p.iterRange()
			if err != nil {
				return nil, err
			}

			if !p.check(fat_arrow) {
				p.addError(p.peek(), "Expected '=>' after pattern")
				p.synchronizeToTokens(fat_arrow, new_line, right_brace)
				if !p.check(fat_arrow) {
					// Could not find fat arrow, skip to next case or end
					continue
				}
			}
			p.advance() // consume the '=>'
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

func (p *parser) parseConditionalMatch(keyword token) (Expression, error) {
	conditionalMatch := &ConditionalMatchExpression{
		Location: Location{
			Start: Point{Row: keyword.line, Col: keyword.column},
		},
	}

	if !p.check(left_brace) {
		p.addError(p.peek(), "Expected '{'")
		return conditionalMatch, nil
	}
	p.advance() // consume the '{'

	if !p.check(new_line) {
		p.addError(p.peek(), "Expected new line after '{'")
		// Continue parsing - this is not a critical error
	} else {
		p.advance()
	}

	for !p.match(right_brace) {
		// Parse and collect comments
		if c := p.parseInlineComment(); c != nil {
			conditionalMatch.Comments = append(conditionalMatch.Comments, *c)
			p.match(new_line) // consume newline after comment
			continue
		}

		// Skip standalone newlines
		if p.match(new_line) {
			continue
		}

		var condition Expression
		var err error

		// Check for catch-all case (_)
		if p.check(identifier) && p.peek().text == "_" {
			p.advance()     // consume '_'
			condition = nil // nil condition indicates catch-all
		} else {
			// Parse the condition expression
			condition, err = p.or()
			if err != nil {
				return nil, err
			}
		}

		if !p.check(fat_arrow) {
			p.addError(p.peek(), "Expected '=>' after condition")
			p.synchronizeToTokens(fat_arrow, new_line, right_brace)
			if !p.check(fat_arrow) {
				// Could not find fat arrow, skip to next case or end
				continue
			}
		}
		p.advance() // consume the '=>'

		// Parse the body
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

		conditionalMatch.Cases = append(conditionalMatch.Cases, ConditionalMatchCase{
			Condition: condition,
			Body:      body,
		})
		p.match(comma)
	}

	conditionalMatch.Location.End = Point{Row: p.previous().line, Col: p.previous().column}
	return conditionalMatch, nil
}

func (p *parser) try() (Expression, error) {
	// Handle the explicit `try` keyword first
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

		var catchVar *Identifier
		var catchBlock []Statement

		// Check for catch clause: -> varname { ... } or -> function_name
		if p.match(thin_arrow) {
			if !p.check(identifier) {
				return nil, p.makeError(p.peek(), "Expected identifier after '->' in try-catch")
			}
			idToken := p.advance()

			// Check if this is a function reference (no block) or variable binding (with block)
			if p.check(left_brace) {
				// Block syntax: -> var { ... }
				catchVar = &Identifier{
					Name:     idToken.text,
					Location: idToken.getLocation(),
				}

				block, err := p.block()
				if err != nil {
					return nil, err
				}
				catchBlock = block
			} else {
				// Function syntax: -> function_name
				// Desugar to: -> err { function_name(err) }
				catchVar = &Identifier{
					Name:     "err",
					Location: idToken.getLocation(),
				}

				// Create function call: function_name(err)
				funcCall := &FunctionCall{
					Location: idToken.getLocation(),
					Name:     idToken.text,
					Args: []Argument{
						{
							Location: idToken.getLocation(),
							Name:     "",
							Value: &Identifier{
								Name:     "err",
								Location: idToken.getLocation(),
							},
						},
					},
				}

				// Wrap in a statement
				catchBlock = []Statement{
					funcCall,
				}
			}
		}

		// Calculate the end location based on what we parsed
		endLoc := expr.GetLocation().End
		if len(catchBlock) > 0 {
			// If we have a catch block, use the last statement's location
			lastStmt := catchBlock[len(catchBlock)-1]
			endLoc = lastStmt.GetLocation().End
		} else if catchVar != nil {
			// If we only have a catch variable, use its location
			endLoc = catchVar.Location.End
		}

		return &Try{
			Location: Location{
				Start: keyword.Location.Start,
				End:   endLoc,
			},
			keyword:    keyword,
			Expression: expr,
			CatchVar:   catchVar,
			CatchBlock: catchBlock,
		}, nil
	}

	// Not a `try` expression: parse the underlying expression as usual.
	expr, err := p.functionDef(false)
	if err != nil {
		return nil, err
	}

	// If we now see a `->` after a non-`try` expression, this is
	// syntactically invalid: the catch block will never be evaluated.
	if p.check(thin_arrow) {
		p.addError(p.peek(), "Missing 'try' keyword: the catch block after '->' will never be evaluated")

		// Best-effort recovery: consume the arrow, optional identifier,
		// and an optional `{ ... }` block so parsing can continue.
		p.advance() // consume '->'

		if p.check(identifier) {
			p.advance()
		}

		if p.check(left_brace) {
			if _, err := p.block(); err != nil {
				return nil, err
			}
		}
	}

	return expr, nil
}

func (p *parser) functionDef(asMethod bool) (Statement, error) {
	private := p.match(private)
	isExtern := p.match(extern)
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
			nameToken := p.advance()
			name = nameToken.text
		} else if p.check(left_paren) {
			// This is a valid anonymous function fn(...) pattern
			name = "" // Anonymous function
		} else {
			// Missing function name and no immediate parameters - add error but continue as anonymous
			p.addError(p.peek(), "Expected function name or '(' after 'fn'")
			name = "" // Treat as anonymous function
		}

		if !p.check(left_paren) {
			p.addError(p.peek(), "Expected '(' for parameters list")
			p.synchronizeToTokens(left_paren, left_brace)
			if !p.check(left_paren) {
				// Could not find opening paren, assume empty parameters and continue
				params := []Parameter{}
				// Try to parse function body if we found '{'
				if p.check(left_brace) {
					statements, err := p.block()
					if err != nil {
						return nil, err
					}
					return &AnonymousFunction{
						Parameters: params,
						ReturnType: &StringType{}, // Default type
						Body:       statements,
						Location: Location{
							Start: Point{Row: keyword.line, Col: keyword.column},
							End:   Point{Row: p.previous().line, Col: p.previous().column},
						},
					}, nil
				}
				// No body found either, return minimal function
				return &AnonymousFunction{
					Parameters: params,
					ReturnType: &StringType{}, // Default type
					Body:       []Statement{},
					Location: Location{
						Start: Point{Row: keyword.line, Col: keyword.column},
						End:   Point{Row: keyword.line, Col: keyword.column},
					},
				}, nil
			}
		}
		p.advance() // consume the '('
		params := []Parameter{}
		var functionComments []Comment
		for !p.match(right_paren) {
			// Prevent infinite loops when closing paren is missing
			if p.isAtEnd() {
				p.addError(p.peek(), "Expected ')' to close parameter list")
				break
			}

			// Parse and collect comments between parameters
			if c := p.parseInlineComment(); c != nil {
				functionComments = append(functionComments, *c)
				p.match(new_line)
				continue
			}

			// Skip standalone newlines
			if p.match(new_line) {
				continue
			}
			isMutable := p.match(mut)
			nameToken := p.consumeVariableName("Expected parameter name")

			// Check if this is a simple parameter list in an anonymous function
			// In case of anonymous functions with unnamed params, we don't need types
			var paramType DeclaredType
			if p.check(colon) {
				p.advance() // consume ':'
				paramType = p.parseType()
			} else if name == "" { // Anonymous function with untyped params
				// For anonymous functions, allow simple parameter names without types
				paramType = &StringType{} // Default to string type for now
			} else {
				// Replace makeError with addError and recovery
				p.addError(p.peek(), "Expected ':' after parameter name")
				p.synchronizeToTokens(colon, comma, right_paren, left_brace)
				if p.check(colon) {
					p.advance() // consume ':'
					paramType = p.parseType()
				} else {
					// No colon found, use default type and continue
					paramType = &StringType{}
				}
			}

			params = append(params, Parameter{
				Mutable: isMutable,
				Name:    nameToken.text,
				Type:    paramType,
			})

			// Check for inline comment after parameter
			if c := p.parseInlineComment(); c != nil {
				functionComments = append(functionComments, *c)
			}

			p.match(comma)
		}

		// Return type is required for all functions - except for simple anonymous functions
		var returnType DeclaredType = nil
		if (name == "" && !p.check(left_brace)) || name != "" {
			// Function with explicit return type
			returnType = p.parseType()
		}

		// Handle extern functions
		if isExtern {
			// Extern functions must have a name
			if name == "" {
				return nil, p.makeError(p.peek(), "Extern functions must have a name")
			}

			// Expect "= external_binding"
			if !p.match(equal) {
				return nil, p.makeError(p.peek(), "Expected '=' after extern function signature")
			}

			// Parse the external binding string
			if !p.check(string_) {
				return nil, p.makeError(p.peek(), "Expected string literal for external binding")
			}
			bindingToken := p.advance()
			externalBinding := bindingToken.text
			// Remove quotes from string literal
			if len(externalBinding) >= 2 && externalBinding[0] == '"' && externalBinding[len(externalBinding)-1] == '"' {
				externalBinding = externalBinding[1 : len(externalBinding)-1]
			}

			extFn := &ExternalFunction{
				Private:         private,
				Name:            name.(string), // External functions must have string names
				Parameters:      params,
				ReturnType:      returnType,
				ExternalBinding: externalBinding,
				Location: Location{
					Start: Point{Row: keyword.line, Col: keyword.column},
					End:   Point{Row: p.previous().line, Col: p.previous().column},
				},
			}

			return extFn, nil
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
			Private:    private,
			Mutates:    asMethod && mutates,
			Parameters: params,
			ReturnType: returnType,
			Body:       statements,
			Comments:   functionComments, // Add collected comments
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
		if !p.check(identifier) {
			p.addError(p.peek(), "Expected struct name")
			// No anonymous structs - skip this struct instantiation attempt
			p.index = index // Reset to original position
			return nil, nil
		}
		nameToken := p.advance()

		if !p.check(left_brace) {
			p.addError(p.peek(), "Expected '{'")
			// Missing brace means this isn't a struct instantiation - skip
			p.index = index // Reset to original position
			return nil, nil
		}
		p.advance() // consume the '{'
		instance := &StructInstance{
			Name:       Identifier{Name: nameToken.text},
			Properties: []StructValue{},
			Location: Location{
				Start: Point{Row: nameToken.line, Col: nameToken.column},
			},
		}

		p.match(new_line)

		for !p.match(right_brace) {
			// Parse and collect comments between properties
			if c := p.parseInlineComment(); c != nil {
				instance.Comments = append(instance.Comments, *c)
				p.match(new_line)
				continue
			}

			// Skip standalone newlines
			if p.match(new_line) {
				continue
			}

			propToken := p.consumeVariableName("Expected name")

			if !p.check(colon) {
				p.addError(p.peek(), "Expected ':' after field name - assuming it")
				// Continue parsing without consuming colon - assume it was meant to be there
			} else {
				p.advance() // consume the ':'
			}

			val, err := p.or()
			if err != nil {
				return nil, err
			}
			instance.Properties = append(instance.Properties, StructValue{
				Name:  Identifier{Name: propToken.text},
				Value: val,
			})

			// Check for inline comment after property
			if c := p.parseInlineComment(); c != nil {
				instance.Comments = append(instance.Comments, *c)
			}

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
			Location: Location{
				Start: left.GetLocation().Start,
				End:   right.GetLocation().End,
			},
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
			Location: Location{
				Start: left.GetLocation().Start,
				End:   right.GetLocation().End,
			},
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
			Location: Location{
				Start: left.GetLocation().Start,
				End:   right.GetLocation().End,
			},
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
			Location: Location{
				Start: left.GetLocation().Start,
				End:   right.GetLocation().End,
			},
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
			Location: Location{
				Start: left.GetLocation().Start,
				End:   right.GetLocation().End,
			},
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
			Location: Location{
				Start: left.GetLocation().Start,
				End:   right.GetLocation().End,
			},
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

	// Handle @property and @method() syntax (no dot)
	if id, ok := expr.(*Identifier); ok && id.Name == "@" {
		if p.check(identifier) {
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
				// match line break for multi-line chaining
				p.match(new_line)
			}
		} else {
			// Check for type arguments in static function calls
			if p.check(identifier, less_than) {
				// This is a static function call with type arguments
				if !p.match(identifier) {
					p.addError(p.peek(), "Expected function name")
					p.synchronizeToTokens(left_paren)
					return nil, nil
				}

				funcName := p.previous()

				// Parse type arguments
				if !p.check(less_than) {
					p.addError(p.peek(), "Expected '<'")
					p.synchronizeToTokens(less_than, left_paren)
					if !p.check(less_than) {
						// Could not find '<', skip type arguments and try regular function call
						return nil, nil
					}
				}
				p.advance() // consume the '<'

				typeArgs := []DeclaredType{}

				typeArg := p.parseType()
				typeArgs = append(typeArgs, typeArg)

				for p.match(comma) {
					typeArg = p.parseType()
					typeArgs = append(typeArgs, typeArg)
				}

				if !p.check(greater_than) {
					p.addError(p.peek(), "Expected '>' after type arguments")
					p.synchronizeToTokens(greater_than, left_paren)
					if !p.check(greater_than) {
						// Could not find '>', skip to function arguments or bail
						if !p.check(left_paren) {
							return nil, nil
						}
					} else {
						p.advance() // consume the '>'
					}
				} else {
					p.advance() // consume the '>'
				}

				// Parse arguments
				if !p.check(left_paren) {
					p.addError(p.peek(), "Expected '(' after type arguments")
					p.synchronizeToTokens(left_paren)
					if !p.check(left_paren) {
						// Could not find '(', skip this function call
						return nil, nil
					}
				}
				p.advance() // consume the '('

				p.match(new_line)
				args, argComments, err := p.parseFunctionArguments()
				if err != nil {
					return nil, err
				}

				if !p.check(right_paren) {
					p.addError(p.peek(), "Expected ')' to close function call")
					p.synchronizeToTokens(right_paren)
					if !p.check(right_paren) {
						// Could not find ')', return partial function call
						return &StaticFunction{
							Target: expr,
							Function: FunctionCall{
								Name:     funcName.text,
								TypeArgs: typeArgs,
								Args:     args,
								Comments: argComments,
								Location: Location{
									Start: expr.GetLocation().Start,
									End:   Point{Row: funcName.line, Col: funcName.column},
								},
							},
						}, nil
					}
				}
				p.advance() // consume the ')'

				// Create the StaticFunction with type arguments
				expr = &StaticFunction{
					Target: expr,
					Function: FunctionCall{
						Name:     funcName.text,
						TypeArgs: typeArgs,
						Args:     args,
						Comments: argComments,
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
				if !p.check(left_paren) {
					p.addError(p.peek(), "Expected '(' after type arguments")
					p.synchronizeToTokens(left_paren)
					if !p.check(left_paren) {
						// Could not find '(', skip this function call
						return nil, nil
					}
				}
				p.advance() // consume the '('

				args, argComments, err := p.parseFunctionArguments()
				if err != nil {
					return nil, err
				}

				if !p.check(right_paren) {
					p.addError(p.peek(), "Expected ')' to close function call")
					p.synchronizeToTokens(right_paren)
					if !p.check(right_paren) {
						// Could not find ')', return partial function call
						return &FunctionCall{
							Name:     expr.(*Identifier).Name,
							TypeArgs: typeArgs,
							Args:     args,
							Comments: argComments,
							Location: Location{
								Start: expr.GetLocation().Start,
								End:   Point{Row: p.previous().line, Col: p.previous().column},
							},
						}, nil
					}
				}
				p.advance() // consume the ')'

				return &FunctionCall{
					Name:     expr.(*Identifier).Name,
					TypeArgs: typeArgs,
					Args:     args,
					Comments: argComments,
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
		p.match(new_line)
		// Regular function call without type arguments
		args, argComments, err := p.parseFunctionArguments()
		if err != nil {
			return nil, err
		}

		if !p.check(right_paren) {
			p.addError(p.peek(), "Expected ')' to close function call")
			p.synchronizeToTokens(right_paren)
			if !p.check(right_paren) {
				// Could not find ')', return partial function call
				return &FunctionCall{
					Name:     expr.(*Identifier).Name,
					Args:     args,
					Comments: argComments,
					Location: Location{
						Start: expr.GetLocation().Start,
						End:   Point{Row: p.previous().line, Col: p.previous().column},
					},
				}, nil
			}
		}
		p.advance() // consume the ')'

		return &FunctionCall{
			Name:     expr.(*Identifier).Name,
			Args:     args,
			Comments: argComments,
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
		tok := p.previous()
		return &BoolLiteral{
			Value:    tok.text == "true",
			Location: tok.getLocation(),
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
		// Check for empty parentheses (void literal)
		if p.check(right_paren) {
			location := p.previous().getLocation()
			p.advance() // consume the ')'
			return &VoidLiteral{
				Location: location,
			}, nil
		}

		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}

		if !p.check(right_paren) {
			p.addError(p.peek(), "Expected ')' after expression")
			p.synchronizeToTokens(right_paren)
			if !p.check(right_paren) {
				// Could not find closing paren, return the expression anyway
				return expr, nil
			}
		}
		p.advance() // consume the ')'
		return expr, nil
	}
	if p.match(left_bracket) {
		return p.list()
	}
	switch tok := p.peek(); tok.kind {
	// Handle keywords as identifiers when used as variables
	case and, not, or, true_, false_, struct_, enum, impl, trait, fn, let, mut,
		break_, match, while_, for_, use, as, in, if_, else_, type_, private:
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
		p.addError(peek, fmt.Sprintf("unmatched primary expression: %s", peek.kind))
		// Return a dummy identifier to allow parsing to continue
		return &Identifier{
			Name:     peek.text,
			Location: peek.getLocation(),
		}, nil
	}
}

func (p *parser) list() (Expression, error) {
	startToken := p.previous()
	if p.check(colon) {
		return p.map_()
	}

	p.match(new_line)

	start := p.index
	items := []Expression{}
	comments := []Comment{}
	for !p.match(right_bracket) {
		// Parse and collect comments
		if c := p.parseInlineComment(); c != nil {
			comments = append(comments, *c)
			p.match(new_line) // consume newline after comment
			continue
		}

		// Skip standalone newlines
		if p.match(new_line) {
			continue
		}

		item, err := p.functionDef(false)
		if err != nil {
			return nil, err
		}
		if p.check(colon) {
			p.index = start
			return p.map_()
		}

		items = append(items, item)

		// Check for inline comment after list element
		if c := p.parseInlineComment(); c != nil {
			comments = append(comments, *c)
		}

		p.match(comma)
		p.match(new_line)
	}
	result := &ListLiteral{
		Items:    items,
		Location: startToken.getLocation(),
	}
	if len(comments) > 0 {
		result.Comments = comments
	}
	return result, nil
}

func (p *parser) map_() (Expression, error) {
	startToken := p.previous()
	node := &MapLiteral{
		Location: startToken.getLocation(),
		Entries:  []MapEntry{},
	}
	if p.match(colon) {
		if !p.match(right_bracket) {
			p.addError(p.peek(), "Expected ']' after ':' in empty map")
			p.synchronizeToTokens(equal, new_line, comma, right_paren)
		}
		return node, nil
	}

	for !p.match(right_bracket) {
		// Parse and collect comments
		if c := p.parseInlineComment(); c != nil {
			node.Comments = append(node.Comments, *c)
			p.match(new_line) // consume newline after comment
			continue
		}

		// Skip standalone newlines
		if p.match(new_line) {
			continue
		}

		key, err := p.primary()
		if err != nil {
			return nil, err
		}
		if !p.check(colon) {
			p.addError(p.peek(), "Expected ':' after map key")
			p.synchronizeToTokens(colon, comma, right_bracket)
			if !p.check(colon) {
				// Could not find ':', skip this map entry
				continue
			}
		}
		p.advance() // consume the ':'
		val, err := p.functionDef(false)
		if err != nil {
			return nil, err
		}
		node.Entries = append(node.Entries, MapEntry{
			Key:   key,
			Value: val,
		})

		// Check for inline comment after map entry
		if c := p.parseInlineComment(); c != nil {
			node.Comments = append(node.Comments, *c)
		}

		p.match(comma)
		p.match(new_line)
	}

	if len(node.Comments) == 0 {
		node.Comments = nil // Keep nil for backward compatibility
	}
	return node, nil
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
		p.addError(nil, "Unexpected end of input")
		// Return a dummy token to allow parsing to continue
		return token{
			kind:   identifier,
			text:   "missing_name",
			line:   0,
			column: 0,
		}
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

	p.addError(current, message)
	p.advance() // CRITICAL: Must advance past the invalid token to prevent infinite loops
	// Return a dummy token to allow parsing to continue
	return token{
		kind:   identifier,
		text:   "invalid_name",
		line:   current.line,
		column: current.column,
	}
}

/* check if a token kind is an allowed keyword */
/* `match` is excluded because it can be confusing in assignments `let foo = match.thing` */
func (p *parser) isAllowedIdentifierKeyword(k kind) bool {
	keywords := []kind{
		and, not, or, true_, false_, struct_, enum, impl, trait, fn, let, mut,
		break_, while_, for_, use, as, in, if_, else_, type_, private,
	}
	return slices.Contains(keywords, k)
}

/* Error creation helpers */
func (p *parser) makeError(at *token, msg string) error {
	return fmt.Errorf("%s:%d:%d: %s", p.fileName, at.line, at.column, msg)
}

/* conditionally advance if the current token is one of those provided */
func (p *parser) match(kinds ...kind) bool {
	if slices.Contains(kinds, p.peek().kind) {
		p.advance()
		return true
	}
	return false
}

func (p *parser) match2(kinds ...kind) bool {
	if slices.Contains(kinds, p.peek().kind) && slices.Contains(kinds, p.peek2().kind) {
		p.advance()
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

func (p *parser) parseFunctionArguments() ([]Argument, []Comment, error) {
	args := []Argument{}
	comments := []Comment{}
	hasNamedArgs := false

	for !p.check(right_paren) {
		// Parse and collect comments between arguments
		if c := p.parseInlineComment(); c != nil {
			comments = append(comments, *c)
			p.match(new_line)
			continue
		}

		// Skip standalone newlines
		if p.match(new_line) {
			continue
		}

		// Check for mut keyword first
		start := p.peek().getLocation().Start
		isMutable := p.match(mut)

		// Check if this is a named argument (identifier followed by colon)
		if p.check(identifier) && p.peek2() != nil && p.peek2().kind == colon {
			hasNamedArgs = true
			name := p.advance().text

			if !p.check(colon) {
				p.addError(p.peek(), "Expected ':' after parameter name")
				p.synchronizeToTokens(colon, comma, right_paren)
				if !p.check(colon) {
					// Could not find ':', skip this parameter
					continue
				}
			}
			p.advance() // consume the ':'

			value, err := p.parseExpression()
			if err != nil {
				return nil, nil, err
			}

			args = append(args, Argument{
				Location: Location{
					Start: start,
					End:   value.GetLocation().End,
				},
				Name:    name,
				Value:   value,
				Mutable: isMutable,
			})
		} else {
			// Check if we've already seen named arguments
			if hasNamedArgs {
				return nil, nil, fmt.Errorf("positional arguments cannot follow named arguments")
			}

			arg, err := p.parseExpression()
			if err != nil {
				return nil, nil, err
			}

			// Add as Argument with empty name (positional)
			args = append(args, Argument{
				Location: Location{
					Start: start,
					End:   arg.GetLocation().End,
				},
				Name:    "",
				Value:   arg,
				Mutable: isMutable,
			})
		}

		// Check for inline comment after argument
		if c := p.parseInlineComment(); c != nil {
			comments = append(comments, *c)
		}

		p.match(comma)
		p.match(new_line)
	}

	return args, comments, nil
}
