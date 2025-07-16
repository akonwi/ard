package checker

// UserModule represents a user-defined module that implements the Module interface
type UserModule struct {
	filePath      string
	publicSymbols map[string]symbol // only public symbols from the checked program
	program       *Program
}

// Path returns the file path for this module
func (m *UserModule) Path() string {
	return m.filePath
}

// Get returns a public symbol by name, or nil if not found or private
func (m *UserModule) Get(name string) symbol {
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
	publicSymbols := make(map[string]symbol)

	// Extract public symbols from the global scope
	for _, sym := range globalScope.symbols {
		switch s := sym.Type.(type) {
		case *FunctionDef:
			if !s.Private {
				publicSymbols[s.Name] = s
			}
		case *StructDef:
			if !s.Private {
				publicSymbols[s.Name] = s
			}
		case *Trait:
			if !s.private {
				publicSymbols[s.Name] = s
			}
		case *Enum:
			if !s.private {
				publicSymbols[s.Name] = s
			}
		}
	}

	return &UserModule{
		filePath:      filePath,
		publicSymbols: publicSymbols,
		program:       program,
	}
}
