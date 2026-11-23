package checker

// UserModule represents a user-defined module that implements the Module interface
type UserModule struct {
	filePath      string
	publicSymbols map[string]Symbol // only public symbols from the checked program
	program       *Program
}

// Path returns the file path for this module
func (m *UserModule) Path() string {
	return m.filePath
}

// Get returns a public symbol by name, or nil if not found or private
func (m *UserModule) Get(name string) Symbol {
	return m.publicSymbols[name] // returns nil if not found
}

// Program returns the checked program for this module
func (m *UserModule) Program() *Program {
	return m.program
}

// setFilePath sets the file path for this module
func (m *UserModule) setFilePath(path string) {
	m.filePath = path
}

// NewUserModule creates a UserModule from a checked program, extracting only
// public symbols. typeAliases maps each top-level type alias name to whether
// its declaration is private.
func NewUserModule(filePath string, program *Program, globalScope *SymbolTable, typeAliases map[string]bool) *UserModule {
	publicSymbols := make(map[string]Symbol)

	// Extract public symbols from the global scope
	for name, sym := range globalScope.symbols {
		// A type alias's scope symbol carries its target type, which can be any
		// type (`&mut Box<Int>`, `[Str]`, a Go type) and has its own visibility.
		// The alias declaration alone decides whether the alias is exported.
		if private, alias := typeAliases[name]; alias && sym.typeDeclaration {
			if !private {
				publicSymbols[name] = *sym
			}
			continue
		}
		switch s := sym.Type.(type) {
		case *FunctionDef:
			if !s.Private {
				publicSymbols[name] = *sym
			}
		case *StructDef:
			if !s.Private {
				publicSymbols[name] = *sym
			}
		case *Trait:
			if !s.private {
				publicSymbols[name] = *sym
			}
		case *Enum:
			if !s.Private {
				publicSymbols[name] = *sym
			}
		case *Union:
			if !s.Private {
				publicSymbols[name] = *sym
			}
		}
	}

	// Extract public variables from program statements
	// Immutable variables are public, mutable variables are private
	for _, stmt := range program.Statements {
		if s, ok := stmt.Stmt.(*VariableDef); ok {
			if !s.Mutable { // Only immutable variables are public
				// Create a symbol for the public variable
				publicSymbols[s.Name] = Symbol{
					Name:    s.Name,
					Type:    s.__type,
					mutable: s.Mutable,
				}
			}
		}
	}

	return &UserModule{
		filePath:      filePath,
		publicSymbols: publicSymbols,
		program:       program,
	}
}

// typeAliasVisibility maps each top-level type alias this module declares to
// whether it is private.
func (c *Checker) typeAliasVisibility() map[string]bool {
	aliases := make(map[string]bool, len(c.topLevelTypeAliases))
	for name, declaration := range c.topLevelTypeAliases {
		aliases[name] = declaration.Private
	}
	return aliases
}

// Symbols returns the module's public symbols. Read-only.
func (m *UserModule) Symbols() map[string]Symbol {
	return m.publicSymbols
}
