package checker

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/akonwi/ard/ast"
)

type Program struct {
	Imports    map[string]Module
	Statements []Statement
}

type Module interface {
	Path() string
	Get(name string) symbol
}

type DiagnosticKind string

const (
	Error DiagnosticKind = "error"
	Warn  DiagnosticKind = "warn"
)

type Diagnostic struct {
	Kind     DiagnosticKind
	Message  string
	filePath string
	location ast.Location
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s %s %s", d.filePath, d.location.Start, d.Message)
}

func isMutable(expr Expression) bool {
	if v, ok := expr.(*Variable); ok {
		return v.isMutable()
	}
	if prop, ok := expr.(*InstanceProperty); ok {
		return isMutable(prop.Subject)
	}
	return false
}

type checker struct {
	diagnostics []Diagnostic
	scope       *scope
	program     *Program
	filePath    string
}

func Check(input *ast.Program, moduleResolver *ModuleResolver, filePath string) (*Program, Module, []Diagnostic) {
	c := &checker{diagnostics: []Diagnostic{}, scope: newScope(nil), filePath: filePath}
	c.program = &Program{
		Imports:    map[string]Module{},
		Statements: []Statement{},
	}

	for _, imp := range input.Imports {
		if _, dup := c.program.Imports[imp.Name]; dup {
			c.addWarning(fmt.Sprintf("%s Duplicate import: %s", imp.GetStart(), imp.Name), imp.GetLocation())
			continue
		}
		if _, dup := c.program.Imports[imp.Name]; dup {
			c.addWarning(fmt.Sprintf("%s Duplicate import: %s", imp.GetStart(), imp.Name), imp.GetLocation())
			continue
		}

		if strings.HasPrefix(imp.Path, "ard/") {
			// Handle standard library imports
			if mod, ok := findInStdLib(imp.Path); ok {
				c.program.Imports[imp.Name] = mod
			} else {
				c.addError(fmt.Sprintf("Unknown module: %s", imp.Path), imp.GetLocation())
			}
		} else {
			// Handle user module imports
			if moduleResolver == nil {
				panic(fmt.Sprintf("No module resolver provided for user import: %s", imp.Path))
			}

			filePath, err := moduleResolver.ResolveImportPath(imp.Path)
			if err != nil {
				c.addError(fmt.Sprintf("Failed to resolve import '%s': %v", imp.Path, err), imp.GetLocation())
				continue
			}

			// Check if module is already cached
			if cachedModule, ok := moduleResolver.moduleCache[filePath]; ok {
				c.program.Imports[imp.Name] = cachedModule
				continue
			}

			// Load and parse the module file using import path
			ast, err := moduleResolver.LoadModule(imp.Path)
			if err != nil {
				c.addError(fmt.Sprintf("Failed to load module %s: %v", filePath, err), imp.GetLocation())
				continue
			}

			// Type-check the imported module
			_, userModule, diagnostics := Check(ast, moduleResolver, imp.Path+".ard")
			if len(diagnostics) > 0 {
				// Add all diagnostics from the imported module
				for _, diag := range diagnostics {
					c.diagnostics = append(c.diagnostics, diag)
				}
				continue
			}

			// Set the correct file path for the module
			if um, ok := userModule.(*UserModule); ok {
				um.setFilePath(imp.Path)
			}

			// Cache and add to imports
			moduleResolver.moduleCache[filePath] = userModule
			c.program.Imports[imp.Name] = userModule
		}
	}

	for i := range input.Statements {
		if stmt := c.checkStmt(&input.Statements[i]); stmt != nil {
			c.program.Statements = append(c.program.Statements, *stmt)
		}
	}

	// Create UserModule from the checked program
	userModule := NewUserModule("", c.program, c.scope)

	// now that we're done with the aliases, use module paths for the import keys
	for alias, mod := range c.program.Imports {
		delete(c.program.Imports, alias)
		c.program.Imports[mod.Path()] = mod
	}
	return c.program, userModule, c.diagnostics
}

func (c *checker) addError(msg string, location ast.Location) {
	c.diagnostics = append(c.diagnostics, Diagnostic{
		Kind:     Error,
		Message:  msg,
		filePath: c.filePath,
		location: location,
	})
}

func (c *checker) addWarning(msg string, location ast.Location) {
	c.diagnostics = append(c.diagnostics, Diagnostic{
		Kind:     Warn,
		Message:  msg,
		filePath: c.filePath,
		location: location,
	})
}

func (c *checker) resolveModule(name string) Module {
	if mod, ok := c.program.Imports[name]; ok {
		return mod
	}

	if mod, ok := prelude[name]; ok {
		return mod
	}

	return nil
}

func (c *checker) resolveType(t ast.DeclaredType) Type {
	var baseType Type
	switch ty := t.(type) {
	case *ast.StringType:
		baseType = Str
	case *ast.IntType:
		baseType = Int
	case *ast.FloatType:
		baseType = Float
	case *ast.BooleanType:
		baseType = Bool

	case *ast.FunctionType:
		// Convert each parameter type and return type
		params := make([]Parameter, len(ty.Params))
		for i, param := range ty.Params {
			params[i] = Parameter{
				Name: fmt.Sprintf("arg%d", i),
				Type: c.resolveType(param),
			}
		}
		returnType := c.resolveType(ty.Return)

		// Create a FunctionDef from the function type syntax
		baseType = &FunctionDef{
			Name:       "<function>",
			Parameters: params,
			ReturnType: returnType,
		}
	case *ast.List:
		of := c.resolveType(ty.Element)
		baseType = MakeList(of)
	case *ast.Map:
		key := c.resolveType(ty.Key)
		value := c.resolveType(ty.Value)
		baseType = MakeMap(key, value)
	case *ast.ResultType:
		val := c.resolveType(ty.Val)
		err := c.resolveType(ty.Err)
		baseType = MakeResult(val, err)
	case *ast.CustomType:
		if sym := c.scope.get(t.GetName()); sym != nil {
			// Check if it's an enum
			if enum, ok := sym.(*Enum); ok {
				baseType = enum
				break
			}

			// Check if it's a union type
			if union, ok := sym.(*Union); ok {
				baseType = union
				break
			}

			// Check if it's a struct type
			if structType, ok := sym.(*StructDef); ok {
				baseType = structType
				break
			}
		}
		if ty.Type.Target != nil {
			mod := c.resolveModule(ty.Type.Target.(*ast.Identifier).Name)
			if mod != nil {
				// at some point, this will need to unwrap the property down to root for nested paths: `mod::sym::more`
				sym := mod.Get(ty.Type.Property.(*ast.Identifier).Name)
				if sym != nil {
					if symType, ok := sym.(Type); ok {
						return symType
					}
				}
			}
		}
		c.addError(fmt.Sprintf("Unrecognized type: %s", t.GetName()), t.GetLocation())
		return nil
	case *ast.GenericType:
		return &Any{name: ty.Name}
	default:
		panic(fmt.Errorf("unrecognized type: %s", t.GetName()))
	}

	// If the type is nullable, wrap it in a Maybe
	if t.IsNullable() {
		return &Maybe{of: baseType}
	}

	return baseType
}

// resolveStaticPath resolves a nested static path to its final module and property name
func (c *checker) resolveStaticPath(expr ast.Expression) (Module, string) {
	switch e := expr.(type) {
	case *ast.Identifier:
		// Simple case: single identifier refers to a module
		if mod := c.resolveModule(e.Name); mod != nil {
			return mod, ""
		}
		return nil, ""
	case *ast.StaticProperty:
		// Nested case: recursively resolve the target
		targetModule, _ := c.resolveStaticPath(e.Target)
		if targetModule == nil {
			return nil, ""
		}

		// The property should be an identifier
		if propId, ok := e.Property.(*ast.Identifier); ok {
			return targetModule, propId.Name
		}
		return nil, ""
	}
	return nil, ""
}

func typeMismatch(expected, got Type) string {
	return fmt.Sprintf("Type mismatch: Expected %s, got %s", expected, got)
}

func (c *checker) checkStmt(stmt *ast.Statement) *Statement {
	switch s := (*stmt).(type) {
	case *ast.Comment:
		return nil
	case *ast.Break:
		return &Statement{Break: true}
	case *ast.TraitDefinition:
		{
			methods := make([]FunctionDef, len(s.Methods))
			for i, method := range s.Methods {
				params := make([]Parameter, len(method.Parameters))
				for j, param := range method.Parameters {
					paramType := c.resolveType(param.Type)
					if paramType == nil {
						c.addError(fmt.Sprintf("Unrecognized type: %s", param.Type.GetName()), param.Type.GetLocation())
						continue
					}
					params[j] = Parameter{Name: param.Name, Type: paramType}
				}

				var returnType Type = Void
				if method.ReturnType != nil {
					returnType = c.resolveType(method.ReturnType)
					if returnType == nil {
						c.addError(fmt.Sprintf("Unrecognized return type: %s", method.ReturnType.GetName()), method.ReturnType.GetLocation())
						continue
					}
				}

				methods[i] = FunctionDef{
					Public:     true,
					Name:       method.Name,
					Parameters: params,
					ReturnType: returnType,
				}
			}

			trait := &Trait{
				public:  s.Public,
				Name:    s.Name.Name,
				methods: methods,
			}

			c.scope.add(trait)
			return nil
		}
	case *ast.TraitImplementation:
		{
			var sym symbol = nil
			switch name := s.Trait.(type) {
			case ast.Identifier:
				sym = c.scope.get(name.Name)
			case ast.StaticProperty:
				mod := c.resolveModule(name.Target.(*ast.Identifier).Name)
				if mod != nil {
					if propId, ok := name.Property.(*ast.Identifier); ok {
						sym = mod.Get(propId.Name)
					} else {
						c.addError(fmt.Sprintf("Bad path: %s", name), name.Property.GetLocation())
						return nil
					}
				}
			default:
				panic(fmt.Errorf("Unsupported trait node: %s", name))
			}

			if sym == nil {
				c.addError(fmt.Sprintf("Undefined trait: %s", s.Trait), s.Trait.GetLocation())
				return nil
			}

			trait, ok := sym.(*Trait)
			if !ok {
				c.addError(fmt.Sprintf("%s is not a trait", s.Trait), s.Trait.GetLocation())
				return nil
			}

			// Check that the type exists
			typeSym := c.scope.get(s.ForType.Name)
			if typeSym == nil {
				c.addError(fmt.Sprintf("Undefined type: %s", s.ForType.Name), s.ForType.GetLocation())
				return nil
			}

			structType, ok := typeSym.(*StructDef)
			if !ok {
				c.addError(fmt.Sprintf("%s is not a struct type", s.ForType.Name), s.ForType.GetLocation())
				return nil
			}

			// Verify that all required methods are implemented
			traitMethods := trait.GetMethods()
			implementedMethods := make(map[string]bool)

			// Check each method in the implementation
			for _, method := range s.Methods {
				implementedMethods[method.Name] = true

				// Find the corresponding trait method
				var traitMethod *FunctionDef
				for _, m := range traitMethods {
					if m.Name == method.Name {
						traitMethod = &m
						break
					}
				}

				if traitMethod == nil {
					c.addError(fmt.Sprintf("Method %s is not required by trait %s", method.Name, trait.name()), method.GetLocation())
					continue
				}

				// Check parameter count
				if len(method.Parameters) != len(traitMethod.Parameters) {
					c.addError(fmt.Sprintf("Method %s has wrong number of parameters", method.Name), method.GetLocation())
					continue
				}

				params := make([]Parameter, len(method.Parameters))
				for i, param := range method.Parameters {
					paramType := c.resolveType(param.Type)
					expectedType := traitMethod.Parameters[i].Type
					if !paramType.equal(expectedType) {
						c.addError(typeMismatch(expectedType, paramType), param.GetLocation())
					}

					params[i] = Parameter{Name: param.Name, Type: paramType}
				}

				// Check return type
				var returnType Type = Void
				if method.ReturnType != nil {
					returnType = c.resolveType(method.ReturnType)
				}
				if !traitMethod.ReturnType.equal(returnType) {
					c.addError(fmt.Sprintf("Trait method '%s' has return type of %s", method.Name, traitMethod.ReturnType), method.GetLocation())
					continue
				}

				// if we made it this far, it's a valid implementation
				fnDef := c.checkFunction(&method, func() {
					c.scope.add(&VariableDef{
						Name:    "@",
						__type:  structType,
						Mutable: method.Mutates,
					})
				})
				fnDef.Mutates = method.Mutates
				// add the method to the struct
				structType.Fields[method.Name] = fnDef
			}

			// Check if all required methods are implemented
			for _, method := range traitMethods {
				if !implementedMethods[method.Name] {
					c.addError(fmt.Sprintf("Missing method '%s' in trait '%s'", method.Name, trait.name()), s.GetLocation())
				}
			}

			// Add the trait to the struct type's traits list
			structType.Traits = append(structType.Traits, trait)

			return nil
		}
	case *ast.TypeDeclaration:
		{
			// Handle type declaration (type unions/aliases)
			types := make([]Type, len(s.Type))
			for i, declType := range s.Type {
				resolvedType := c.resolveType(declType)
				if resolvedType == nil {
					c.addError(fmt.Sprintf("Unrecognized type: %s", declType.GetName()), declType.GetLocation())
					return nil
				}
				types[i] = resolvedType
			}

			// Create a union type (even if it only contains one type)
			unionType := &Union{
				Name:  s.Name.Name,
				Types: types,
			}

			// Register the type in the scope with the given name
			c.scope.add(unionType)
			return nil
		}
	case *ast.VariableDeclaration:
		{
			var val Expression
			if s.Type == nil {
				switch literal := s.Value.(type) {
				case *ast.ListLiteral:
					if expr := c.checkList(nil, literal); expr != nil {
						val = expr
					}
				case *ast.MapLiteral:
					if expr := c.checkMap(nil, literal); expr != nil {
						val = expr
					}
				default:
					val = c.checkExpr(s.Value)
				}
			} else {
				expected := c.resolveType(s.Type)
				if expected == Void {
					c.addError("Cannot assign a void value", s.Value.GetLocation())
					return nil
				}

				switch literal := s.Value.(type) {
				case *ast.ListLiteral:
					if expr := c.checkList(expected, literal); expr != nil {
						val = expr
					}
				case *ast.MapLiteral:
					if expr := c.checkMap(expected, literal); expr != nil {
						val = expr
					}
				default:
					val = c.checkExprAs(s.Value, expected)
				}
			}

			if val == nil {
				return nil
			}

			__type := val.Type()

			if s.Type != nil {
				if expected := c.resolveType(s.Type); expected != nil {
					if !expected.equal(val.Type()) {
						c.addError(typeMismatch(expected, val.Type()), s.Value.GetLocation())
						return nil
					}
					__type = expected
				}
			}

			v := &VariableDef{
				Mutable: s.Mutable,
				Name:    s.Name,
				Value:   val,
				__type:  __type,
			}
			c.scope.add(v)
			return &Statement{
				Stmt: v,
			}
		}
	case *ast.VariableAssignment:
		{
			if id, ok := s.Target.(*ast.Identifier); ok {
				target := c.scope.get(id.Name)
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
					if !target._type().equal(value.Type()) {
						c.addError(typeMismatch(target._type(), value.Type()), s.Value.GetLocation())
						return nil
					}

					return &Statement{
						Stmt: &Reassignment{Target: &Variable{target}, Value: value},
					}
				}
			}

			if ip, ok := s.Target.(*ast.InstanceProperty); ok {
				subject := c.checkExpr(ip)
				if subject == nil {
					return nil
				}
				value := c.checkExpr(s.Value)
				if value == nil {
					return nil
				}

				if !isMutable(subject) {
					c.addError(fmt.Sprintf("Immutable: %s", ip), s.Target.GetLocation())
					return nil
				}

				return &Statement{
					Stmt: &Reassignment{Target: subject, Value: value},
				}
			}

			panic(fmt.Sprintf("Unsupported reassignment target: %T", s.Target))
		}
	case *ast.WhileLoop:
		{
			// Check the condition expression
			var condition Expression
			if s.Condition == nil {
				condition = &BoolLiteral{true}
			} else {
				condition = c.checkExpr(s.Condition)
			}

			// Condition must be a boolean expression
			if condition.Type() != Bool {
				c.addError("While loop condition must be a boolean expression", s.Condition.GetLocation())
				return nil
			}

			// Check the body of the loop
			body := c.checkBlock(s.Body, nil)

			// Create and return the while loop
			loop := &WhileLoop{
				Condition: condition,
				Body:      body,
			}

			return &Statement{Stmt: loop}
		}
	case *ast.ForLoop:
		{
			// Create a new scope for the loop body and initialization
			scope := newScope(c.scope)
			c.scope = scope
			defer func() {
				c.scope = c.scope.parent
			}()

			// Check the initialization statement - handle it as a variable declaration
			initDeclStmt := ast.Statement(s.Init)
			initStmt := c.checkStmt(&initDeclStmt)
			if initStmt == nil || initStmt.Stmt == nil {
				c.addError("Invalid for loop initialization", s.Init.GetLocation())
				return nil
			}
			initVarDef, ok := initStmt.Stmt.(*VariableDef)
			if !ok {
				c.addError("For loop initialization must be a variable declaration", s.Init.GetLocation())
				return nil
			}

			// Check the condition expression
			condition := c.checkExpr(s.Condition)
			if condition == nil {
				return nil
			}

			// Condition must be a boolean expression
			if condition.Type() != Bool {
				c.addError("For loop condition must be a boolean expression", s.Condition.GetLocation())
				return nil
			}

			// Check the update statement - handle it as a variable assignment
			incrStmt := ast.Statement(s.Incrementer)
			updateStmt := c.checkStmt(&incrStmt)
			if updateStmt == nil || updateStmt.Stmt == nil {
				c.addError("Invalid for loop update expression", s.Incrementer.GetLocation())
				return nil
			}
			update, ok := updateStmt.Stmt.(*Reassignment)
			if !ok {
				c.addError("For loop update must be a reassignment", s.Incrementer.GetLocation())
				return nil
			}

			// Check the body of the loop
			body := c.checkBlock(s.Body, nil)

			// Create and return the for loop
			loop := &ForLoop{
				Init:      initVarDef,
				Condition: condition,
				Update:    update,
				Body:      body,
			}

			return &Statement{Stmt: loop}
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
					Index:  s.Cursor2.Name,
					Start:  start,
					End:    end,
				}
				body := c.checkBlock(s.Body, func() {
					c.scope.add(&VariableDef{
						Mutable: false,
						Name:    s.Cursor.Name,
						__type:  start.Type(),
					})
					if loop.Index != "" {
						c.scope.add(&VariableDef{
							Mutable: false,
							Name:    loop.Index,
							__type:  Int,
						})
					}
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
					Index:  s.Cursor2.Name,
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
					if loop.Index != "" {
						c.scope.add(&VariableDef{
							Mutable: false,
							Name:    loop.Index,
							__type:  Int,
						})
					}
				})

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			// Handle integer iteration (for i in n - sugar for 0..n)
			if iterValue.Type() == Int {
				// This is syntax sugar for a range from 0 to n
				loop := &ForIntRange{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					Start:  &IntLiteral{0}, // Start from 0
					End:    iterValue,      // End at the specified number
				}

				// Create a new scope for the loop body where the cursor is defined
				body := c.checkBlock(s.Body, func() {
					// Add the cursor variable to the scope
					c.scope.add(&VariableDef{
						Mutable: false,
						Name:    s.Cursor.Name,
						__type:  Int,
					})
					if loop.Index != "" {
						c.scope.add(&VariableDef{
							Mutable: false,
							Name:    loop.Index,
							__type:  Int,
						})
					}
				})

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			if listType, ok := iterValue.Type().(*List); ok {
				// This is syntax sugar for a range from 0 to n
				loop := &ForInList{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					List:   iterValue,
				}

				body := c.checkBlock(s.Body, func() {
					// Add the cursor variable to the scope
					c.scope.add(&VariableDef{
						Mutable: false,
						Name:    s.Cursor.Name,
						__type:  listType.of,
					})
					if loop.Index != "" {
						c.scope.add(&VariableDef{
							Mutable: false,
							Name:    loop.Index,
							__type:  Int,
						})
					}
				})

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			if mapType, ok := iterValue.Type().(*Map); ok {
				iterable := c.checkExpr(s.Iterable)
				if iterable == nil {
					return nil
				}

				loop := &ForInMap{
					Key: s.Cursor.Name,
					Val: s.Cursor2.Name,
					Map: iterable,
				}

				body := c.checkBlock(s.Body, func() {
					// Add the cursors to the scope
					c.scope.add(&VariableDef{
						Mutable: false,
						Name:    s.Cursor.Name,
						__type:  mapType.key,
					})
					c.scope.add(&VariableDef{
						Mutable: false,
						Name:    s.Cursor2.Name,
						__type:  mapType.value,
					})
				})

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			// Currently we only support string, integer, and List iteration
			c.addError(fmt.Sprintf("Cannot iterate over a %s", iterValue.Type()), s.Iterable.GetLocation())
			return nil
		}
	case *ast.EnumDefinition:
		{
			if len(s.Variants) == 0 {
				c.addError("Enums must have at least one variant", s.GetLocation())
				return nil
			}

			// Check for duplicate variant names
			seenVariants := make(map[string]bool)
			for _, variant := range s.Variants {
				if seenVariants[variant] {
					c.addError(fmt.Sprintf("Duplicate variant: %s", variant), s.GetLocation())
					return nil
				}
				seenVariants[variant] = true
			}

			enum := &Enum{
				public:   s.Public,
				Name:     s.Name,
				Variants: s.Variants,
			}
			c.scope.add(enum)
			return nil
		}
	case *ast.StructDefinition:
		{
			def := &StructDef{
				Name:    s.Name.Name,
				Fields:  make(map[string]Type),
				Public:  s.Public,
				Statics: map[string]*FunctionDef{},
			}
			for _, field := range s.Fields {
				fieldType := c.resolveType(field.Type)
				if fieldType == nil {
					return nil
				}

				if _, dup := def.Fields[field.Name.Name]; dup {
					c.addError(fmt.Sprintf("Duplicate field: %s", field.Name.Name), field.Name.GetLocation())
					return nil
				}
				def.Fields[field.Name.Name] = fieldType
			}
			c.scope.add(def)
			return nil
		}
	case *ast.ImplBlock:
		{
			sym := c.scope.get(s.Target.Name)
			if sym == nil {
				c.addError(fmt.Sprintf("Undefined: %s", s.Target), s.Target.GetLocation())
				return nil
			}

			structDef, ok := sym.(*StructDef)
			if !ok {
				c.addError(fmt.Sprintf("Expected struct type, got %s", sym), s.Target.GetLocation())
				return nil
			}

			for _, method := range s.Methods {
				fnDef := c.checkFunction(&method, func() {
					c.scope.add(&VariableDef{
						Name:    "@",
						__type:  structDef,
						Mutable: method.Mutates,
					})
				})
				fnDef.Mutates = method.Mutates
				structDef.Fields[method.Name] = fnDef
			}
			return nil
		}
	case nil:
		return nil
	default:
		expr := c.checkExpr((ast.Expression)(*stmt))
		if expr == nil {
			return nil
		}
		return &Statement{Expr: expr}
	}
}

func (c *checker) checkList(declaredType Type, expr *ast.ListLiteral) *ListLiteral {
	if declaredType != nil {
		expectedElementType := declaredType.(*List).of
		elements := make([]Expression, len(expr.Items))
		for i := range expr.Items {
			item := expr.Items[i]
			element := c.checkExpr(item)
			if !expectedElementType.equal(element.Type()) {
				c.addError(typeMismatch(expectedElementType, element.Type()), item.GetLocation())
				return nil
			}
			elements[i] = element
		}

		return &ListLiteral{
			Elements: elements,
			_type:    declaredType.(*List),
		}
	}

	if len(expr.Items) == 0 {
		c.addError("Empty lists need an explicit type", expr.GetLocation())
		return nil
	}

	hasError := false
	var elementType Type
	elements := make([]Expression, len(expr.Items))
	for i := range expr.Items {
		item := expr.Items[i]
		element := c.checkExpr(item)
		if element == nil {
			continue
		}

		if i == 0 {
			elementType = element.Type()
		} else if elementType != element.Type() {
			c.addError("Type mismatch: A list can only contain values of single type", item.GetLocation())
			hasError = true
			continue
		}

		elements[i] = element
	}

	if hasError {
		return nil
	}

	return &ListLiteral{
		Elements: elements,
		_type:    MakeList(elementType),
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

func (c *checker) checkMap(declaredType Type, expr *ast.MapLiteral) *MapLiteral {
	// Handle empty map with declared type
	if len(expr.Entries) == 0 {
		if declaredType != nil {
			mapType, ok := declaredType.(*Map)
			if !ok {
				c.addError(fmt.Sprintf("Expected map type but got %s", declaredType), expr.GetLocation())
				return nil
			}

			// Return empty map with the declared type
			return &MapLiteral{
				Keys:   []Expression{},
				Values: []Expression{},
				_type:  mapType,
			}
		} else {
			// Empty map without a declared type is an error
			c.addError("Empty maps need an explicit type", expr.GetLocation())
			return nil
		}
	}

	// Handle non-empty map
	if declaredType != nil {
		mapType, ok := declaredType.(*Map)
		if !ok {
			c.addError(fmt.Sprintf("Expected map type but got %s", declaredType), expr.GetLocation())
			return nil
		}

		expectedKeyType := mapType.key
		expectedValueType := mapType.value

		keys := make([]Expression, len(expr.Entries))
		values := make([]Expression, len(expr.Entries))

		hasError := false
		for i, entry := range expr.Entries {
			// Type check the key
			key := c.checkExpr(entry.Key)
			if key == nil {
				hasError = true
				continue
			}
			if !expectedKeyType.equal(key.Type()) {
				c.addError(typeMismatch(expectedKeyType, key.Type()), entry.Key.GetLocation())
				hasError = true
				continue
			}
			keys[i] = key

			// Type check the value
			value := c.checkExpr(entry.Value)
			if value == nil {
				hasError = true
				continue
			}
			if !expectedValueType.equal(value.Type()) {
				c.addError(typeMismatch(expectedValueType, value.Type()), entry.Value.GetLocation())
				hasError = true
				continue
			}
			values[i] = value
		}

		if hasError {
			return nil
		}

		return &MapLiteral{
			Keys:   keys,
			Values: values,
			_type:  mapType,
		}
	}

	// Type inference for non-empty maps without declared type
	keys := make([]Expression, len(expr.Entries))
	values := make([]Expression, len(expr.Entries))

	// Check the first entry to determine key and value types
	firstKey := c.checkExpr(expr.Entries[0].Key)
	firstValue := c.checkExpr(expr.Entries[0].Value)

	if firstKey == nil || firstValue == nil {
		return nil
	}

	keyType := firstKey.Type()
	valueType := firstValue.Type()
	keys[0] = firstKey
	values[0] = firstValue

	// Check that all entries have consistent types
	hasError := false
	for i := 1; i < len(expr.Entries); i++ {
		key := c.checkExpr(expr.Entries[i].Key)
		if key == nil {
			hasError = true
			continue
		}
		if !keyType.equal(key.Type()) {
			c.addError(fmt.Sprintf("Map key type mismatch: Expected %s, got %s", keyType, key.Type()), expr.Entries[i].Key.GetLocation())
			hasError = true
			continue
		}
		keys[i] = key

		value := c.checkExpr(expr.Entries[i].Value)
		if value == nil {
			hasError = true
			continue
		}
		if !valueType.equal(value.Type()) {
			c.addError(fmt.Sprintf("Map value type mismatch: Expected %s, got %s", valueType, value.Type()), expr.Entries[i].Value.GetLocation())
			hasError = true
			continue
		}
		values[i] = value
	}

	if hasError {
		return nil
	}

	// Create and return the map
	return &MapLiteral{
		Keys:   keys,
		Values: values,
		_type:  MakeMap(keyType, valueType),
	}
}

// validateStructInstance validates struct instantiation and returns the instance or nil if errors
func (c *checker) validateStructInstance(structType *StructDef, properties []ast.StructValue, structName string, loc ast.Location) *StructInstance {
	instance := &StructInstance{Name: structName, _type: structType}
	fields := make(map[string]Expression)

	// Check all provided properties
	for _, property := range properties {
		if field, ok := structType.Fields[property.Name.Name]; !ok {
			c.addError(fmt.Sprintf("Unknown field: %s", property.Name.Name), property.GetLocation())
		} else {
			fields[property.Name.Name] = c.checkExprAs(property.Value, field)
		}
	}

	// Check for missing required fields
	missing := []string{}
	for name, t := range structType.Fields {
		if _, isMethod := t.(*FunctionDef); !isMethod {
			if _, exists := fields[name]; !exists {
				if _, isMaybe := t.(*Maybe); !isMaybe {
					missing = append(missing, name)
				}
			}
		}
	}
	if len(missing) > 0 {
		c.addError(fmt.Sprintf("Missing field: %s", strings.Join(missing, ", ")), loc)
		return nil
	}

	instance.Fields = fields
	return instance
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
	case *ast.InterpolatedStr:
		{
			chunks := make([]Expression, len(s.Chunks))
			for i := range s.Chunks {
				cx := c.checkExpr(s.Chunks[i])
				if cx == nil {
					return nil
				}
				if !cx.Type().hasTrait(strMod.Get("ToString").(*Trait)) {
					c.addError(typeMismatch(Str, cx.Type()), s.Chunks[i].GetLocation())
					return nil
				}
				chunks[i] = cx
			}
			return &TemplateStr{chunks}
		}
	case *ast.Identifier:
		if sym := c.scope.get(s.Name); sym != nil {
			return &Variable{sym}
		}
		c.addError(fmt.Sprintf("Undefined variable: %s", s.Name), s.GetLocation())
		return nil
	case *ast.FunctionCall:
		{
			if s.Name == "panic" {
				if len(s.Args) != 1 {
					c.addError("Incorrect number of arguments: 'panic' requires a message", s.GetLocation())
					return nil
				}
				message := c.checkExpr(s.Args[0])
				if message == nil {
					return nil
				}

				return &Panic{
					Message: message,
					node:    s,
				}
			}

			// Find the function in the scope
			fnSym := c.scope.get(s.Name)
			if fnSym == nil {
				c.addError(fmt.Sprintf("Undefined function: %s", s.Name), s.GetLocation())
				return nil
			}

			// Cast to FunctionDef
			var fnDef *FunctionDef
			var ok bool

			// Try different types for the function symbol
			fnDef, ok = fnSym.(*FunctionDef)
			if !ok {
				// Check if it's a variable that holds a function
				if varDef, ok := fnSym.(*VariableDef); ok {
					// Try to get a FunctionDef directly
					if anon, ok := varDef.Value.(*FunctionDef); ok {
						fnDef = anon
					} else if existingFnDef, ok := varDef._type().(*FunctionDef); ok {
						// FunctionDef can be used directly
						// This handles the case where a variable holds a function
						fnDef = existingFnDef
					} else {
						c.addError(fmt.Sprintf("Not a function: %s", s.Name), s.GetLocation())
						return nil
					}
				} else {
					c.addError(fmt.Sprintf("Not a function: %s", s.Name), s.GetLocation())
					return nil
				}
			}

			// Check argument count
			if len(s.Args) != len(fnDef.Parameters) {
				c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d",
					len(fnDef.Parameters), len(s.Args)), s.GetLocation())
				return nil
			}

			// Check and process arguments
			args := make([]Expression, len(s.Args))
			for i, arg := range s.Args {
				checkedArg := c.checkExpr(arg)
				if checkedArg == nil {
					return nil
				}

				// Type check the argument against the parameter type
				paramType := fnDef.Parameters[i].Type
				if !areCompatible(paramType, checkedArg.Type()) {
					c.addError(typeMismatch(paramType, checkedArg.Type()), arg.GetLocation())
					return nil
				}

				// Check mutability constraints if needed
				if fnDef.Parameters[i].Mutable && !isMutable(checkedArg) {
					c.addError(fmt.Sprintf("Type mismatch: Expected a mutable %s", fnDef.Parameters[i].Type.String()), arg.GetLocation())
				}

				args[i] = checkedArg
			}

			if fnDef.hasGenerics() {
				if len(s.TypeArgs) > 0 {
					// collect generics
					generics := []Type{}
					for _, param := range fnDef.Parameters {
						generics = append(generics, getGenerics(param.Type)...)
					}
					generics = append(generics, getGenerics(fnDef.ReturnType)...)

					if len(s.TypeArgs) != len(generics) {
						c.addError(fmt.Sprintf("Expected %d type arguments", len(generics)), s.GetLocation())
						return nil
					}

					for i, any := range generics {
						actual := c.resolveType(s.TypeArgs[i])
						if actual == nil {
							return nil
						}
						typeMap := make(map[string]Type)
						typeMap[any.(*Any).name] = actual
						substituteType(any, typeMap)
					}
				}

				// Create a mapping of generic parameters to concrete types
				typeMap := make(map[string]Type)
				// Infer types from arguments
				for i, param := range fnDef.Parameters {
					if anyType, ok := param.Type.(*Any); ok {
						if existing, exists := typeMap[anyType.name]; exists {
							// Ensure consistent types for the same generic parameter
							if !existing.equal(args[i].Type()) {
								c.addError(fmt.Sprintf("Type mismatch for $%s: Expected %s, got %s", anyType.name, anyType.actual, args[i].Type()), s.Args[i].GetLocation())
								return nil
							}
						} else {
							// Bind the generic parameter to the argument type
							typeMap[anyType.name] = args[i].Type()
						}
					}
				}

				// Create specialized function with generic parameters substituted
				specialized := &FunctionDef{
					Name:       fnDef.Name,
					Parameters: make([]Parameter, len(fnDef.Parameters)),
					ReturnType: substituteType(fnDef.ReturnType, typeMap),
					Body:       fnDef.Body,
				}

				// Substitute types in parameters
				for i, param := range fnDef.Parameters {
					specialized.Parameters[i] = Parameter{
						Name:    param.Name,
						Type:    substituteType(param.Type, typeMap),
						Mutable: param.Mutable,
					}
				}

				// Return function call with specialized function
				return &FunctionCall{
					Name: s.Name,
					Args: args,
					fn:   specialized,
				}
			}

			// Create and return the function call node
			return &FunctionCall{
				Name: s.Name,
				Args: args,
				fn:   fnDef,
			}
		}
	case *ast.InstanceProperty:
		{
			subj := c.checkExpr(s.Target)
			if subj == nil {
				// panic(fmt.Errorf("Cannot access %s on nil", s.Property))
				return nil
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
	case *ast.InstanceMethod:
		{
			subj := c.checkExpr(s.Target)
			if subj == nil {
				c.addError(fmt.Sprintf("Cannot access %s on Void", s.Method.Name), s.Method.GetLocation())
				return nil
			}

			sig := subj.Type().get(s.Method.Name)
			if sig == nil {
				c.addError(fmt.Sprintf("Undefined: %s.%s", subj, s.Method.Name), s.Method.GetLocation())
				return nil
			}

			fnDef, ok := sig.(*FunctionDef)
			if !ok {
				c.addError(fmt.Sprintf("%s.%s is not a function", subj, s.Method.Name), s.Method.GetLocation())
				return nil
			}

			if fnDef.Mutates && !isMutable(subj) {
				c.addError(fmt.Sprintf("Cannot mutate immutable '%s' with '.%s()'", subj, s.Method.Name), s.Method.GetLocation())
				return nil
			}

			if len(s.Method.Args) != len(fnDef.Parameters) {
				c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d",
					len(fnDef.Parameters), len(s.Method.Args)), s.GetLocation())
				return nil
			}

			// Check and process arguments
			args := make([]Expression, len(s.Method.Args))
			for i, arg := range s.Method.Args {
				checkedArg := c.checkExpr(arg)
				if checkedArg == nil {
					return nil
				}

				// Type check the argument against the parameter type
				paramType := fnDef.Parameters[i].Type
				if !areCompatible(paramType, checkedArg.Type()) {
					c.addError(typeMismatch(paramType, checkedArg.Type()), arg.GetLocation())
					return nil
				}

				// Check mutability constraints if needed
				if fnDef.Parameters[i].Mutable && !isMutable(checkedArg) {
					c.addError(fmt.Sprintf("Type mismatch: Expected a mutable %s", fnDef.Parameters[i].Type.String()), arg.GetLocation())
				}

				args[i] = checkedArg
			}

			if fnDef.hasGenerics() {
				if len(s.Method.TypeArgs) > 0 {
					// collect generics
					generics := []Type{}
					for _, param := range fnDef.Parameters {
						generics = append(generics, getGenerics(param.Type)...)
					}
					generics = append(generics, getGenerics(fnDef.ReturnType)...)

					if len(generics) == 0 && len(s.Method.TypeArgs) > 0 {
						// c.addWarning("Unnecessary type arguments", s.Method.GetLocation())
					} else {
						if len(s.Method.TypeArgs) != len(generics) {
							c.addError(fmt.Sprintf("Expected %d type arguments", len(generics)), s.Method.GetLocation())
							return nil
						}
					}

					for i, any := range generics {
						actual := c.resolveType(s.Method.TypeArgs[i])
						if actual == nil {
							return nil
						}
						typeMap := make(map[string]Type)
						typeMap[any.(*Any).name] = actual
						substituteType(any, typeMap)
					}
				}

				// Create a mapping of generic parameters to concrete types
				typeMap := make(map[string]Type)
				// Infer types from arguments
				for i, param := range fnDef.Parameters {
					if anyType, ok := param.Type.(*Any); ok {
						if existing, exists := typeMap[anyType.name]; exists {
							// Ensure consistent types for the same generic parameter
							if !existing.equal(args[i].Type()) {
								c.addError(fmt.Sprintf("Type mismatch for $%s: Expected %s, got %s", anyType.name, anyType.actual, args[i].Type()), s.Method.Args[i].GetLocation())
								return nil
							}
						} else {
							// Bind the generic parameter to the argument type
							typeMap[anyType.name] = args[i].Type()
						}
					}
				}

				// Create specialized function with generic parameters substituted
				specialized := &FunctionDef{
					Name:       fnDef.Name,
					Parameters: make([]Parameter, len(fnDef.Parameters)),
					ReturnType: substituteType(fnDef.ReturnType, typeMap),
					Body:       fnDef.Body,
				}

				// Substitute types in parameters
				for i, param := range fnDef.Parameters {
					specialized.Parameters[i] = Parameter{
						Name:    param.Name,
						Type:    substituteType(param.Type, typeMap),
						Mutable: param.Mutable,
					}
				}

				// Return function call with specialized function
				return &InstanceMethod{
					Subject: subj,
					Method: &FunctionCall{
						Name: s.Method.Name,
						Args: args,
						fn:   specialized,
					},
				}
			}

			// Create function call
			call := &FunctionCall{
				Name: s.Method.Name,
				Args: args,
				fn:   fnDef,
			}

			return &InstanceMethod{
				Subject: subj,
				Method:  call,
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

					if !left.Type().equal(right.Type()) {
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

					if !left.Type().equal(right.Type()) {
						c.addError(fmt.Sprintf("Invalid: %s == %s", left.Type(), right.Type()), s.GetLocation())
						return nil
					}

					isMaybe := func(val Type) bool {
						_, ok := val.(*Maybe)
						return ok
					}
					if isMaybe(left.Type()) {
						return &Equality{left, right}
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
	case *ast.StaticFunction:
		{
			// Process module function calls like io::print() or http::Response::new()
			mod, structName := c.resolveStaticPath(s.Target)

			if mod != nil {
				var fnDef *FunctionDef

				if structName != "" {
					// We have a nested path like http::Response::new, resolve the struct first
					structSymbol := mod.Get(structName)
					if structSymbol == nil {
						c.addError(fmt.Sprintf("Undefined: %s", structName), s.GetLocation())
						return nil
					}

					structDef, ok := structSymbol.(*StructDef)
					if !ok {
						c.addError(fmt.Sprintf("%s is not a struct", structName), s.GetLocation())
						return nil
					}

					// Look for the static function in the struct
					staticFn, exists := structDef.Statics[s.Function.Name]
					if !exists {
						targetName := s.Target.String()
						c.addError(fmt.Sprintf("Undefined static function: %s::%s", targetName, s.Function.Name), s.GetLocation())
						return nil
					}
					fnDef = staticFn
				} else {
					// Simple case like io::print(), look for function directly in module
					sym := mod.Get(s.Function.Name)
					if sym == nil {
						targetName := s.Target.String()
						c.addError(fmt.Sprintf("Undefined: %s::%s", targetName, s.Function.Name), s.GetLocation())
						return nil
					}

					var ok bool
					fnDef, ok = sym.(*FunctionDef)
					if !ok {
						targetName := s.Target.String()
						c.addError(fmt.Sprintf("%s::%s is not a function", targetName, s.Function.Name), s.GetLocation())
						return nil
					}
				}

				// Check argument count
				if len(s.Function.Args) != len(fnDef.Parameters) {
					c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d",
						len(fnDef.Parameters), len(s.Function.Args)), s.GetLocation())
					return nil
				}

				// Check and process arguments
				args := make([]Expression, len(s.Function.Args))
				for i, arg := range s.Function.Args {
					checkedArg := c.checkExpr(arg)
					if checkedArg == nil {
						return nil
					}

					// Type check the argument against the parameter type
					paramType := fnDef.Parameters[i].Type
					if !areCompatible(paramType, checkedArg.Type()) {
						c.addError(typeMismatch(paramType, checkedArg.Type()), arg.GetLocation())
						return nil
					}

					// Check mutability constraints if needed
					if fnDef.Parameters[i].Mutable && !isMutable(checkedArg) {
						c.addError(fmt.Sprintf("Type mismatch: Expected a mutable %s", fnDef.Parameters[i].Type.String()), arg.GetLocation())
					}

					args[i] = checkedArg
				}

				if fnDef.hasGenerics() {
					if len(s.Function.TypeArgs) > 0 {
						// collect generics
						generics := []Type{}
						for _, param := range fnDef.Parameters {
							generics = append(generics, getGenerics(param.Type)...)
						}
						generics = append(generics, getGenerics(fnDef.ReturnType)...)

						if len(s.Function.TypeArgs) != len(generics) {
							c.addError(fmt.Sprintf("Expected %d type arguments", len(generics)), s.Function.GetLocation())
							return nil
						}

						for i, any := range generics {
							actual := c.resolveType(s.Function.TypeArgs[i])
							if actual == nil {
								return nil
							}
							typeMap := make(map[string]Type)
							typeMap[any.(*Any).name] = actual
							substituteType(any, typeMap)
						}
					}
					// technically could be an else block

					// Create a mapping of generic parameters to concrete types
					typeMap := make(map[string]Type)
					// Infer types from arguments
					for i, param := range fnDef.Parameters {
						if anyType, ok := param.Type.(*Any); ok {
							if existing, exists := typeMap[anyType.name]; exists {
								// Ensure consistent types for the same generic parameter
								if !existing.equal(args[i].Type()) {
									c.addError(fmt.Sprintf("Type mismatch for $%s: Expected %s, got %s", anyType.name, anyType.actual, args[i].Type()), s.Function.Args[i].GetLocation())
									return nil
								}
							} else {
								// Bind the generic parameter to the argument type
								typeMap[anyType.name] = args[i].Type()
							}
						}
					}

					// Create specialized function with generic parameters substituted
					specialized := &FunctionDef{
						Name:       fnDef.Name,
						Parameters: make([]Parameter, len(fnDef.Parameters)),
						ReturnType: substituteType(fnDef.ReturnType, typeMap),
						Body:       fnDef.Body,
					}

					// Substitute types in parameters
					for i, param := range fnDef.Parameters {
						specialized.Parameters[i] = Parameter{
							Name:    param.Name,
							Type:    substituteType(param.Type, typeMap),
							Mutable: param.Mutable,
						}
					}

					// Return function call with specialized function
					return &ModuleFunctionCall{
						Module: mod.Path(),
						Call: &FunctionCall{
							Name: s.Function.Name,
							Args: args,
							fn:   specialized,
						},
					}
				}

				// Create function call
				call := &FunctionCall{
					Name: s.Function.Name,
					Args: args,
					fn:   fnDef,
				}

				// Create module static function call if struct is involved, otherwise regular module function call
				if structName != "" {
					return &ModuleStaticFunctionCall{
						Module: mod.Path(),
						Struct: structName,
						Call:   call,
					}
				} else {
					return &ModuleFunctionCall{
						Module: mod.Path(),
						Call:   call,
					}
				}
			} else {
				// Handle local static functions (non-module case)
				// For now, we only support simple identifiers for local static functions
				var fnDef *FunctionDef
				if id, ok := s.Target.(*ast.Identifier); ok {
					sym := c.scope.get(id.Name)

					if sym == nil {
						c.addError(fmt.Sprintf("Undefined: %s", id.Name), s.Target.GetLocation())
						return nil
					}

					strct, ok := sym.(*StructDef)
					if !ok {
						c.addError(fmt.Sprintf("Undefined: %s", id.Name), s.GetLocation())
						return nil
					}

					var exists bool
					fnDef, exists = strct.Statics[s.Function.Name]
					if !exists {
						c.addError(fmt.Sprintf("Undefined: %s::%s", id.Name, s.Function.Name), s.GetLocation())
						return nil
					}
				} else {
					c.addError("Unsupported static function target", s.Target.GetLocation())
					return nil
				}

				// Check argument count
				if len(s.Function.Args) != len(fnDef.Parameters) {
					c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d",
						len(fnDef.Parameters), len(s.Function.Args)), s.GetLocation())
					return nil
				}

				// Check and process arguments
				args := make([]Expression, len(s.Function.Args))
				for i, arg := range s.Function.Args {
					checkedArg := c.checkExpr(arg)
					if checkedArg == nil {
						return nil
					}

					// Type check the argument against the parameter type
					paramType := fnDef.Parameters[i].Type
					if !areCompatible(paramType, checkedArg.Type()) {
						c.addError(typeMismatch(paramType, checkedArg.Type()), arg.GetLocation())
						return nil
					}

					// Check mutability constraints if needed
					if fnDef.Parameters[i].Mutable && !isMutable(checkedArg) {
						c.addError(fmt.Sprintf("Type mismatch: Expected a mutable %s", fnDef.Parameters[i].Type.String()), arg.GetLocation())
					}

					args[i] = checkedArg
				}

				if fnDef.hasGenerics() {
					if len(s.Function.TypeArgs) > 0 {
						// collect generics
						generics := []Type{}
						for _, param := range fnDef.Parameters {
							generics = append(generics, getGenerics(param.Type)...)
						}
						generics = append(generics, getGenerics(fnDef.ReturnType)...)

						if len(s.Function.TypeArgs) != len(generics) {
							c.addError(fmt.Sprintf("Expected %d type arguments", len(generics)), s.Function.GetLocation())
							return nil
						}

						for i, any := range generics {
							actual := c.resolveType(s.Function.TypeArgs[i])
							if actual == nil {
								return nil
							}
							typeMap := make(map[string]Type)
							typeMap[any.(*Any).name] = actual
							substituteType(any, typeMap)
						}
					}
					// technically could be an else block

					// Create a mapping of generic parameters to concrete types
					typeMap := make(map[string]Type)
					// Infer types from arguments
					for i, param := range fnDef.Parameters {
						if anyType, ok := param.Type.(*Any); ok {
							if existing, exists := typeMap[anyType.name]; exists {
								// Ensure consistent types for the same generic parameter
								if !existing.equal(args[i].Type()) {
									c.addError(fmt.Sprintf("Type mismatch for $%s: Expected %s, got %s", anyType.name, anyType.actual, args[i].Type()), s.Function.Args[i].GetLocation())
									return nil
								}
							} else {
								// Bind the generic parameter to the argument type
								typeMap[anyType.name] = args[i].Type()
							}
						}
					}

					// Create specialized function with generic parameters substituted
					specialized := &FunctionDef{
						Name:       fnDef.Name,
						Parameters: make([]Parameter, len(fnDef.Parameters)),
						ReturnType: substituteType(fnDef.ReturnType, typeMap),
						Body:       fnDef.Body,
					}

					// Substitute types in parameters
					for i, param := range fnDef.Parameters {
						specialized.Parameters[i] = Parameter{
							Name:    param.Name,
							Type:    substituteType(param.Type, typeMap),
							Mutable: param.Mutable,
						}
					}

					// Return function call with specialized function
					return &ModuleFunctionCall{
						Module: mod.Path(),
						Call: &FunctionCall{
							Name: s.Function.Name,
							Args: args,
							fn:   specialized,
						},
					}
				}

				// Create function call
				call := &FunctionCall{
					Name: s.Function.Name,
					Args: args,
					fn:   fnDef,
				}
				// Get the struct definition for the scope
				var scopeStruct *StructDef
				if id, ok := s.Target.(*ast.Identifier); ok {
					if sym := c.scope.get(id.Name); sym != nil {
						scopeStruct, _ = sym.(*StructDef)
					}
				}

				return &StaticFunctionCall{
					Scope: scopeStruct,
					Call:  call,
				}
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
	case *ast.FunctionDeclaration:
		return c.checkFunction(s, nil)
	case *ast.AnonymousFunction:
		{
			// Process parameters
			params := make([]Parameter, len(s.Parameters))
			for i, param := range s.Parameters {
				var paramType Type = Void
				if param.Type != nil {
					paramType = c.resolveType(param.Type)
				}

				params[i] = Parameter{
					Name:    param.Name,
					Type:    paramType,
					Mutable: param.Mutable,
				}
			}

			// Determine return type
			var returnType Type = Void
			if s.ReturnType != nil {
				returnType = c.resolveType(s.ReturnType)
			}

			// Check function body with a setup function that adds parameters to scope
			body := c.checkBlock(s.Body, func() {
				for _, param := range params {
					c.scope.add(&VariableDef{
						Mutable: param.Mutable,
						Name:    param.Name,
						__type:  param.Type,
					})
				}
			})

			// Check that the function's return type matches its body's type
			if returnType != Void && !returnType.equal(body.Type()) {
				c.addError(typeMismatch(returnType, body.Type()), s.GetLocation())
				return nil
			}

			// Create function definition
			// Generate a unique name for the anonymous function
			uniqueName := fmt.Sprintf("anon_func_%p", s)

			fn := &FunctionDef{
				Name:       uniqueName,
				Parameters: params,
				ReturnType: returnType,
				Body:       body,
			}

			// Add function to scope
			c.scope.add(fn)

			return fn
		}
	case *ast.StaticFunctionDeclaration:
		qualifier := s.Path.Target.(*ast.Identifier)
		sym := c.scope.get(qualifier.Name)
		if sym == nil {
			c.addError(fmt.Sprintf("Undefined: %s", qualifier), qualifier.GetLocation())
			return nil
		}

		strct, ok := sym.(*StructDef)
		if !ok {
			c.addError(fmt.Sprintf("Not a struct: %s", sym), s.GetLocation())
			return nil
		}

		segment := s.Path.Property.(*ast.Identifier)
		if _, ok := strct.Statics[segment.Name]; ok {
			c.addError(fmt.Sprintf("Duplicate declaration: %s", s.Path), s.Path.GetLocation())
			return nil
		}

		fn := c.checkFunction(&s.FunctionDeclaration, nil)
		if fn != nil {
			fn.Name = s.Path.String()
			strct.Statics[segment.Name] = fn
		}

		return nil
	case *ast.ListLiteral:
		return c.checkList(nil, s)
	case *ast.MapLiteral:
		return c.checkMap(nil, s)
	case *ast.MatchExpression:
		// Check the subject
		subject := c.checkExpr(s.Subject)
		if subject == nil {
			return nil
		}

		// For Maybe types, generate an OptionMatch
		if maybeType, ok := subject.Type().(*Maybe); ok {
			var patternIdent *Identifier
			var someBody *Block
			var noneBody *Block

			// Process the cases
			for _, matchCase := range s.Cases {
				// Check if it's the default case (_)
				if id, ok := matchCase.Pattern.(*ast.Identifier); ok {
					if id.Name == "_" {
						// This is the None case
						noneBody = c.checkBlock(matchCase.Body, nil)
					} else {
						// This is the Some case with a variable binding
						// Create a new scope for the body with the pattern bound to the unwrapped value
						someBody = c.checkBlock(matchCase.Body, func() {
							// Add the pattern name as a variable in the scope with the inner type
							// For example, if the Maybe is Str?, the pattern should be a Str
							c.scope.add(&VariableDef{
								Mutable: false,
								Name:    id.Name,
								__type:  maybeType.of,
							})
						})

						// Create an identifier to use in the Match struct
						patternIdent = &Identifier{Name: id.Name}
					}
				} else {
					c.addError("Pattern in Maybe match must be an identifier", matchCase.Pattern.GetLocation())
					return nil
				}
			}

			// Ensure we have both some and none cases
			if someBody == nil {
				c.addError("Match on a Maybe type must include a binding case", s.GetLocation())
				return nil
			}

			if noneBody == nil {
				c.addError("Match on a Maybe type must include a wildcard (_) case", s.GetLocation())
				return nil
			}

			// Create the OptionMatch
			return &OptionMatch{
				Subject: subject,
				Some: &Match{
					Pattern: patternIdent,
					Body:    someBody,
				},
				None: noneBody,
			}
		}

		// For Enum types, generate an EnumMatch
		if enumType, ok := subject.Type().(*Enum); ok {
			// Map to track which variants we've seen
			seenVariants := make(map[string]bool)
			// Track whether we've seen a catch-all case
			hasCatchAll := false
			// Cases in the match statement mapped to enum variants
			cases := make([]*Block, len(enumType.Variants))
			var catchAllBody *Block

			// Process the cases
			for _, matchCase := range s.Cases {
				if id, ok := matchCase.Pattern.(*ast.Identifier); ok {
					if id.Name == "_" {
						// This is a catch-all case
						if hasCatchAll {
							c.addError("Duplicate catch-all case", matchCase.Pattern.GetLocation())
							return nil
						}

						hasCatchAll = true
						catchAllBody = c.checkBlock(matchCase.Body, nil)
						continue
					}
				}

				// Handle enum variant case - the pattern should be a static property reference like Enum::Variant
				if staticProp, ok := matchCase.Pattern.(*ast.StaticProperty); ok {
					// Get the enum name from the target
					if enumId, ok := staticProp.Target.(*ast.Identifier); ok {
						// Verify the enum name matches
						if enumId.Name != enumType.Name {
							c.addError(fmt.Sprintf("Expected %s::<variant>, got %s::%s",
								enumType.Name, enumId.Name, staticProp.Property), staticProp.GetLocation())
							return nil
						}

						// Find the variant in the enum
						variantName := staticProp.Property.(*ast.Identifier).Name
						variantIndex := enumType.variant(variantName)
						if variantIndex == -1 {
							c.addError(fmt.Sprintf("Undefined: %s::%s", enumType.Name, variantName), staticProp.GetLocation())
							return nil
						}

						// Check for duplicate cases
						if seenVariants[variantName] {
							c.addError(fmt.Sprintf("Duplicate case: %s::%s", enumType.Name, variantName), staticProp.GetLocation())
							return nil
						}
						seenVariants[variantName] = true

						// Check the body for this case
						body := c.checkBlock(matchCase.Body, nil)
						cases[variantIndex] = body
					} else {
						c.addError("Invalid pattern in enum match", matchCase.Pattern.GetLocation())
						return nil
					}
				} else {
					c.addError("Pattern in enum match must be an enum variant or wildcard", matchCase.Pattern.GetLocation())
					return nil
				}
			}

			// Check if the match is exhaustive
			if !hasCatchAll {
				for i, variant := range enumType.Variants {
					if cases[i] == nil {
						c.addError(fmt.Sprintf("Incomplete match: missing case for '%s::%s'", enumType.Name, variant), s.GetLocation())
					}
				}
			}

			// Ensure all cases return the same type
			if len(cases) > 0 {
				// Find the first non-nil case to use as reference type
				var referenceType Type
				for _, caseBody := range cases {
					if caseBody != nil {
						referenceType = caseBody.Type()
						break
					}
				}

				if referenceType != nil {
					if catchAllBody != nil && !referenceType.equal(catchAllBody.Type()) {
						c.addError(typeMismatch(referenceType, catchAllBody.Type()), s.GetLocation())
						return nil
					}

					for _, caseBody := range cases {
						if caseBody != nil && !referenceType.equal(caseBody.Type()) {
							c.addError(typeMismatch(referenceType, caseBody.Type()), s.GetLocation())
							return nil
						}
					}
				}
			}

			// Create the EnumMatch
			enumMatch := &EnumMatch{
				Subject:  subject,
				Cases:    cases,
				CatchAll: catchAllBody,
			}

			return enumMatch
		}

		// For Bool types, generate a BoolMatch
		if subject.Type() == Bool {
			var trueBody, falseBody *Block
			// Track which cases we've seen
			seenTrue, seenFalse := false, false

			// Process the cases
			for _, matchCase := range s.Cases {
				if id, ok := matchCase.Pattern.(*ast.Identifier); ok {
					if id.Name == "_" {
						// Catch-all cases aren't allowed for boolean matches
						c.addError("Catch-all case is not allowed for boolean matches", matchCase.Pattern.GetLocation())
						return nil
					}
				}

				// Handle boolean literal case
				if boolLit, ok := matchCase.Pattern.(*ast.BoolLiteral); ok {
					// Check for duplicates
					if boolLit.Value && seenTrue {
						c.addError("Duplicate case: 'true'", matchCase.Pattern.GetLocation())
						return nil
					}
					if !boolLit.Value && seenFalse {
						c.addError("Duplicate case: 'false'", matchCase.Pattern.GetLocation())
						return nil
					}

					// Process the body
					body := c.checkBlock(matchCase.Body, nil)

					// Store the body in the appropriate field
					if boolLit.Value {
						seenTrue = true
						trueBody = body
					} else {
						seenFalse = true
						falseBody = body
					}
				} else {
					c.addError("Pattern in boolean match must be a boolean literal (true or false)", matchCase.Pattern.GetLocation())
					return nil
				}
			}

			// Check exhaustiveness
			if !seenTrue || !seenFalse {
				if !seenTrue {
					c.addError("Incomplete match: Missing case for 'true'", s.GetLocation())
				} else {
					c.addError("Incomplete match: Missing case for 'false'", s.GetLocation())
				}
				return nil
			}

			// Ensure both branches return the same type
			if !trueBody.Type().equal(falseBody.Type()) {
				c.addError(typeMismatch(trueBody.Type(), falseBody.Type()), s.GetLocation())
				return nil
			}

			// Create and return the BoolMatch
			return &BoolMatch{
				Subject: subject,
				True:    trueBody,
				False:   falseBody,
			}
		}

		// For other types, handle according to their type...
		// For Union types, generate a UnionMatch
		if unionType, ok := subject.Type().(*Union); ok {
			// Track which union types we've seen and their corresponding bodies
			typeCases := make(map[string]*Block)
			var catchAllBody *Block

			// Record all types in the union
			unionTypeSet := make(map[string]Type)
			for _, t := range unionType.Types {
				unionTypeSet[t.String()] = t
			}

			// Process the cases
			for _, matchCase := range s.Cases {
				// Check for catch-all case (_)
				if id, ok := matchCase.Pattern.(*ast.Identifier); ok {
					if id.Name == "_" {
						if catchAllBody != nil {
							c.addError("Duplicate catch-all case", matchCase.Pattern.GetLocation())
							return nil
						}
						catchAllBody = c.checkBlock(matchCase.Body, nil)
						continue
					}
				}

				// Handle type pattern - should be an identifier matching a type in the union
				if typeId, ok := matchCase.Pattern.(*ast.Identifier); ok {
					typeName := typeId.Name

					// Check if the type exists in the union
					_, found := unionTypeSet[typeName]
					if !found {
						c.addError(fmt.Sprintf("Type %s is not part of union %s", typeName, unionType),
							matchCase.Pattern.GetLocation())
						return nil
					}

					// Check for duplicates
					if _, exists := typeCases[typeName]; exists {
						c.addError(fmt.Sprintf("Duplicate case: %s", typeName), matchCase.Pattern.GetLocation())
						return nil
					}

					// Get the actual type object
					matchedType := unionTypeSet[typeName]

					// Process the body with the matched type binding
					body := c.checkBlock(matchCase.Body, func() {
						// Add "it" variable to the scope with the union element's type
						c.scope.add(&VariableDef{
							Mutable: false,
							Name:    "it",
							__type:  matchedType,
							Value:   nil, // Will be set at runtime
						})
					})
					typeCases[typeName] = body
				} else {
					c.addError("Pattern in union match must be a type name or catch-all (_)",
						matchCase.Pattern.GetLocation())
					return nil
				}
			}

			// Check exhaustiveness if no catch-all is provided
			if catchAllBody == nil {
				for typeName := range unionTypeSet {
					if _, covered := typeCases[typeName]; !covered {
						c.addError(fmt.Sprintf("Incomplete match: missing case for '%s'", typeName),
							s.GetLocation())
						return nil
					}
				}
			}

			// Ensure all cases return the same type
			var referenceType Type
			for _, caseBody := range typeCases {
				if caseBody != nil {
					referenceType = caseBody.Type()
					break
				}
			}

			if referenceType != nil {
				if catchAllBody != nil && !referenceType.equal(catchAllBody.Type()) {
					c.addError(typeMismatch(referenceType, catchAllBody.Type()), s.GetLocation())
					return nil
				}

				for _, caseBody := range typeCases {
					if caseBody != nil && !referenceType.equal(caseBody.Type()) {
						c.addError(typeMismatch(referenceType, caseBody.Type()), s.GetLocation())
						return nil
					}
				}
			}

			// Create and return the UnionMatch
			return &UnionMatch{
				Subject:   subject,
				TypeCases: typeCases,
				CatchAll:  catchAllBody,
			}
		}

		if resultType, ok := subject.Type().(*Result); ok {
			if len(s.Cases) > 2 {
				c.addError("Too many cases in match", s.GetLocation())
				return nil
			}

			var okCase *Match
			var errCase *Match
			for _, node := range s.Cases {
				switch p := node.Pattern.(type) {
				case *ast.Identifier:
					{
						if p.Name == "ok" {
							okCase = &Match{
								Pattern: &Identifier{Name: "ok"},
								Body: c.checkBlock(node.Body, func() {
									c.scope.add(&VariableDef{
										Name:   "ok",
										__type: resultType.Val(),
									})
								}),
							}
						} else if p.Name == "err" {
							errCase = &Match{
								Pattern: &Identifier{Name: "err"},
								Body: c.checkBlock(node.Body, func() {
									c.scope.add(&VariableDef{
										Name:   "err",
										__type: resultType.Err(),
									})
								}),
							}
						} else {
							c.addWarning("Ignored pattern", p.GetLocation())
						}
					}
				case *ast.FunctionCall: // use FunctionCall node as aliasing variable
					{
						varName := p.Args[0].(*ast.Identifier).Name
						if p.Name == "ok" {
							varName := p.Args[0].(*ast.Identifier).Name
							okCase = &Match{
								Pattern: &Identifier{Name: varName},
								Body: c.checkBlock(node.Body, func() {
									c.scope.add(&VariableDef{
										Name:   varName,
										__type: resultType.Val(),
									})
								}),
							}
						} else if p.Name == "err" {
							errCase = &Match{
								Pattern: &Identifier{Name: varName},
								Body: c.checkBlock(node.Body, func() {
									c.scope.add(&VariableDef{
										Name:   varName,
										__type: resultType.Err(),
									})
								}),
							}
						} else {
							c.addWarning("Ignored pattern", p.GetLocation())
						}
					}
				}
			}

			if okCase == nil {
				c.addError("Missing ok case", s.GetLocation())
				return nil
			}
			if errCase == nil {
				c.addError("Missing err case", s.GetLocation())
				return nil
			}

			return &ResultMatch{
				Subject: subject,
				Ok:      okCase,
				Err:     errCase,
			}
		}

		// Check for Int matching
		if subject.Type() == Int {
			intCases := make(map[int]*Block)
			rangeCases := make(map[IntRange]*Block)
			var catchAll *Block

			for _, matchCase := range s.Cases {
				// Check if it's the default case (_)
				if id, ok := matchCase.Pattern.(*ast.Identifier); ok && id.Name == "_" {
					catchAll = c.checkBlock(matchCase.Body, nil)
				} else if literal, ok := matchCase.Pattern.(*ast.NumLiteral); ok {
					// Convert string to int
					value, err := strconv.Atoi(literal.Value)
					if err != nil {
						c.addError(fmt.Sprintf("Invalid integer literal: %s", literal.Value), matchCase.Pattern.GetLocation())
						return nil
					}
					caseBlock := c.checkBlock(matchCase.Body, nil)
					intCases[value] = caseBlock
				} else if rangeExpr, ok := matchCase.Pattern.(*ast.RangeExpression); ok {
					// Handle range pattern like 1..10
					startLiteral, startOk := rangeExpr.Start.(*ast.NumLiteral)
					endLiteral, endOk := rangeExpr.End.(*ast.NumLiteral)

					if !startOk || !endOk {
						c.addError("Range patterns must use integer literals", matchCase.Pattern.GetLocation())
						return nil
					}

					startValue, err := strconv.Atoi(startLiteral.Value)
					if err != nil {
						c.addError(fmt.Sprintf("Invalid start value in range: %s", startLiteral.Value), rangeExpr.Start.GetLocation())
						return nil
					}

					endValue, err := strconv.Atoi(endLiteral.Value)
					if err != nil {
						c.addError(fmt.Sprintf("Invalid end value in range: %s", endLiteral.Value), rangeExpr.End.GetLocation())
						return nil
					}

					if startValue > endValue {
						c.addError("Range start must be less than or equal to end", matchCase.Pattern.GetLocation())
						return nil
					}

					caseBlock := c.checkBlock(matchCase.Body, nil)
					rangeCases[IntRange{Start: startValue, End: endValue}] = caseBlock
				} else {
					c.addError(fmt.Sprintf("Invalid pattern for Int match: %T", matchCase.Pattern), matchCase.Pattern.GetLocation())
					return nil
				}
			}

			return &IntMatch{
				Subject:    subject,
				IntCases:   intCases,
				RangeCases: rangeCases,
				CatchAll:   catchAll,
			}
		}

		c.addError(fmt.Sprintf("Cannot match on %s", subject.Type()), s.GetLocation())
		return nil
	case *ast.StaticProperty:
		{
			if id, ok := s.Target.(*ast.Identifier); ok {
				// Check if this is accessing a module
				if mod := c.resolveModule(id.Name); mod != nil {
					switch prop := s.Property.(type) {
					case *ast.StructInstance:
						// Look up the struct symbol directly from the module
						sym := mod.Get(prop.Name.Name)
						if sym == nil {
							c.addError(fmt.Sprintf("Undefined: %s::%s", id.Name, prop.Name.Name), prop.Name.GetLocation())
							return nil
						}

						structType, ok := sym.(*StructDef)
						if !ok {
							c.addError(fmt.Sprintf("%s::%s is not a struct", id.Name, prop.Name.Name), prop.Name.GetLocation())
							return nil
						}

						// Use helper function for validation
						instance := c.validateStructInstance(structType, prop.Properties, prop.Name.Name, prop.GetLocation())
						if instance == nil {
							return nil
						}

						return &ModuleStructInstance{
							Module:   mod.Path(),
							Property: instance,
						}
					case *ast.Identifier:
						// Look up other symbols (like enum variants, etc.)
						sym := mod.Get(prop.Name)
						if sym == nil {
							c.addError(fmt.Sprintf("Undefined: %s::%s", id.Name, prop.Name), prop.GetLocation())
							return nil
						}
						// For now, we don't handle other module symbols besides structs
						// This could be extended for constants, etc.
						c.addError(fmt.Sprintf("Cannot access %s::%s in this context", id.Name, prop.Name), prop.GetLocation())
						return nil
					default:
						c.addError(fmt.Sprintf("Unsupported property type in %s::%s", id.Name, prop), s.Property.GetLocation())
						return nil
					}
				}

				// Handle local enum variants (not from modules)
				sym := c.scope.get(id.Name)
				if sym == nil {
					c.addError(fmt.Sprintf("Undefined: %s", id.Name), id.GetLocation())
					return nil
				}
				enum, ok := sym.(*Enum)
				if !ok {
					c.addError(fmt.Sprintf("Undefined: %s::%s", sym, s.Property), id.GetLocation())
					return nil
				}

				var variant int8 = -1
				for i := range enum.Variants {
					if enum.Variants[i] == s.Property.(*ast.Identifier).Name {
						variant = int8(i)
						break
					}
				}
				if variant == -1 {
					c.addError(fmt.Sprintf("Undefined: %s::%s", sym, s.Property.(*ast.Identifier).Name), id.GetLocation())
					return nil
				}

				return &EnumVariant{enum: enum, Variant: variant}
			}
			panic(fmt.Errorf("Unexpected static property target: %T", s.Target))
		}
	case *ast.StructInstance:
		name := s.Name.Name
		sym := c.scope.get(name)
		if sym == nil {
			c.addError(fmt.Sprintf("Undefined: %s", name), s.GetLocation())
			return nil
		}

		structType, ok := sym.(*StructDef)
		if !ok {
			c.addError(fmt.Sprintf("Undefined: %s", name), s.GetLocation())
			return nil
		}

		// Use helper function for validation
		return c.validateStructInstance(structType, s.Properties, name, s.GetLocation())
	case *ast.Try:
		{
			expr := c.checkExpr(s.Expression)
			if expr == nil {
				return nil
			}

			if c.scope.returnType == nil {
				c.addError("The `try` keyword can only be used in a function body", s.GetLocation())
				return nil
			}

			switch _type := expr.Type().(type) {
			case *Result:
				if !_type.equal(c.scope.returnType) {
					c.addError(typeMismatch(c.scope.returnType, _type), s.Expression.GetLocation())
				}

				return &TryOp{
					expr: expr,
					ok:   _type.val,
				}
			case *Maybe:
				return &TryOp{
					expr: expr,
					ok:   _type.of,
				}
			default:
				c.addError("todo: Unsupported try expression type: "+expr.Type().String(), s.Expression.GetLocation())
				return nil
			}
		}
	default:
		panic(fmt.Errorf("Unexpected expression: %s", reflect.TypeOf(s)))
	}
}

// use this when we know what the expr's Type should be
func (c *checker) checkExprAs(expr ast.Expression, expectedType Type) Expression {
	switch s := (expr).(type) {
	case *ast.ListLiteral:
		return c.checkList(expectedType, s)
	case *ast.MapLiteral:
		return c.checkMap(expectedType, s)
	case *ast.StaticFunction:
		{
			resultType, expectResult := expectedType.(*Result)
			if !expectResult &&
				s.Target.(*ast.Identifier).Name != "Result" &&
				(s.Function.Name != "ok" && s.Function.Name != "err") {
				return c.checkExpr(s)
			}

			moduleName := s.Target.(*ast.Identifier).Name
			mod := c.resolveModule(moduleName)
			if mod == nil {
				c.addError(fmt.Sprintf("Undefined: %s", moduleName), s.GetLocation())
				return nil
			}

			sym := mod.Get(s.Function.Name)
			if sym == nil {
				c.addError(fmt.Sprintf("Undefined: %s::%s", moduleName, s.Function.Name), s.GetLocation())
				return nil
			}

			fnDef, isFunc := sym.(*FunctionDef)
			if !isFunc {
				c.addError(fmt.Sprintf("%s::%s is not a function", moduleName, s.Function.Name), s.GetLocation())
				return nil
			}

			if len(s.Function.Args) != len(fnDef.Parameters) {
				c.addError(fmt.Sprintf("Incorrect number of arguments: Expected %d, got %d",
					len(fnDef.Parameters), len(s.Function.Args)), s.GetLocation())
				return nil
			}

			var arg Expression = nil
			if fnDef.name() == "ok" {
				arg = c.checkExpr(s.Function.Args[0])
				if !resultType.Val().equal(arg.Type()) {
					c.addError(typeMismatch(resultType.Val(), arg.Type()), s.Function.Args[0].GetLocation())
					return nil
				}
			}
			if fnDef.name() == "err" {
				arg = c.checkExpr(s.Function.Args[0])
				if !resultType.Err().equal(arg.Type()) {
					c.addError(typeMismatch(resultType.Err(), arg.Type()), s.Function.Args[0].GetLocation())
					return nil
				}
			}

			fnDef.ReturnType = resultType
			return &ModuleFunctionCall{
				Module: mod.Path(),
				Call: &FunctionCall{
					Name: fnDef.name(),
					Args: []Expression{arg},
					fn:   fnDef,
				},
			}
		}
	}

	checked := c.checkExpr(expr)
	if checked == nil {
		return nil
	}

	if !expectedType.equal(checked.Type()) {
		c.addError(typeMismatch(expectedType, checked.Type()), expr.GetLocation())
		return nil
	}

	return checked
}

func (c *checker) checkFunction(def *ast.FunctionDeclaration, init func()) *FunctionDef {
	if init != nil {
		init()
	}

	// Process parameters
	params := make([]Parameter, len(def.Parameters))
	for i, param := range def.Parameters {
		var paramType Type = Void
		if param.Type != nil {
			paramType = c.resolveType(param.Type)
		}

		params[i] = Parameter{
			Name:    param.Name,
			Type:    paramType,
			Mutable: param.Mutable,
		}
	}

	// Determine return type
	var returnType Type = Void
	if def.ReturnType != nil {
		returnType = c.resolveType(def.ReturnType)
	}

	// Create function definition early (before checking body)
	fn := &FunctionDef{
		Name:       def.Name,
		Parameters: params,
		ReturnType: returnType,
		Body:       nil, // Will be set after checking
		Public:     def.Public,
	}

	// Add function to scope BEFORE checking body to support recursion
	c.scope.add(fn)

	// Check function body
	body := c.checkBlock(def.Body, func() {
		// set the expected return type to the scope
		c.scope.returnType = returnType
		// add parameters to scope
		for _, param := range params {
			c.scope.add(&VariableDef{
				Mutable: param.Mutable,
				Name:    param.Name,
				__type:  param.Type,
			})
		}
	})

	// Check that the function's return type matches its body's type
	if returnType != Void && !areCompatible(returnType, body.Type()) {
		c.addError(typeMismatch(returnType, body.Type()), def.GetLocation())
		return nil
	}

	// Set the body now that it's been checked
	fn.Body = body

	return fn
}

// Substitute generic parameters in a type
func substituteType(t Type, typeMap map[string]Type) Type {
	switch typ := t.(type) {
	case *Any:
		if concrete, exists := typeMap[typ.name]; exists {
			typ.actual = concrete
		}
		return typ
	case *Maybe:
		return &Maybe{of: substituteType(typ.of, typeMap)}
	// Handle other compound types
	default:
		return t
	}
}
