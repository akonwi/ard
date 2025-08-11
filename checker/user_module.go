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

// NewUserModule creates a UserModule from a checked program, extracting only public symbols
func NewUserModule(filePath string, program *Program, globalScope *SymbolTable) *UserModule {
	publicSymbols := make(map[string]Symbol)

	// Extract public symbols from the global scope
	for _, sym := range globalScope.symbols {
		switch s := sym.Type.(type) {
		case *FunctionDef:
			if !s.Private {
				publicSymbols[s.Name] = *sym
			}
		case *ExternalFunctionDef:
			if !s.Private {
				publicSymbols[s.Name] = *sym
			}
		case *StructDef:
			if !s.Private {
				publicSymbols[s.Name] = *sym
			}
		case *Trait:
			if !s.private {
				publicSymbols[s.Name] = *sym
			}
		case *Enum:
			if !s.private {
				publicSymbols[s.Name] = *sym
			}
		}
	}

	return &UserModule{
		filePath:      filePath,
		publicSymbols: publicSymbols,
		program:       program,
	}
}
