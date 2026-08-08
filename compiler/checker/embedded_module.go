package checker

import (
	"sync"

	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/std_lib"
)

type embeddedModuleCacheEntry struct {
	once   sync.Once
	module Module
	ok     bool
}

var embeddedModuleCache sync.Map

// EmbeddedModule represents a standard library module loaded from embedded .ard files
type EmbeddedModule struct {
	path          string
	program       *Program
	publicSymbols map[string]Symbol
}

func (m EmbeddedModule) Path() string {
	return m.path
}

func (m EmbeddedModule) Program() *Program {
	return m.program
}

func (m EmbeddedModule) Get(name string) Symbol {
	return m.publicSymbols[name]
}

// FindEmbeddedModule loads a .ard standard library module from embedded files.
// Embedded source is immutable for the process lifetime, so parsed and checked
// modules are shared across checker invocations.
func FindEmbeddedModule(path string) (Module, bool) {
	loaded, ok := embeddedModuleCache.Load(path)
	if !ok {
		loaded, _ = embeddedModuleCache.LoadOrStore(path, &embeddedModuleCacheEntry{})
	}
	entry := loaded.(*embeddedModuleCacheEntry)
	entry.once.Do(func() {
		entry.module, entry.ok = loadEmbeddedModule(path)
	})
	return entry.module, entry.ok
}

func loadEmbeddedModule(path string) (Module, bool) {
	// Read the embedded file using std_lib.Find
	content, err := std_lib.Find(path)
	if err != nil {
		return nil, false
	}

	// Parse the .ard file
	result := parse.Parse(content, path)
	if len(result.Errors) > 0 {
		return nil, false
	}
	program := result.Program

	// Type check the program to create a Program with symbols.
	//
	// Load-time diagnostics are deliberately not surfaced here (issue #350):
	// the embedded standard library ships with the compiler and is validated
	// at compiler build time by TestEmbeddedStdLibModulesCheckCleanly, so
	// importers trust it rather than re-reporting its internals. Every other
	// imported Ard module — project-owned and dependencies alike — is
	// re-validated at check time with diagnostics attributed to the module
	// (see the user-module import path in checker.go).
	module, _ := check(program, nil, path, path, CheckOptions{})
	return module, true
}

// Symbols returns the module's public symbols. Read-only.
func (m EmbeddedModule) Symbols() map[string]Symbol {
	return m.publicSymbols
}
