package vm

import (
	"fmt"
	"sync"
	"testing"

	"github.com/akonwi/ard/checker"
)

// TestConcurrentMethodAccess verifies that GlobalVM is safe for concurrent fiber execution.
// This test simulates multiple goroutines (fibers) accessing methods concurrently,
// which is what happens when spawning 20-30 fibers with async::eval().
func TestConcurrentMethodAccess(t *testing.T) {
	g := &GlobalVM{
		modules:        make(map[string]*VM),
		moduleScopes:   make(map[string]*scope),
		moduleRegistry: NewModuleRegistry(),
		ffiRegistry:    NewRuntimeFFIRegistry(),
	}

	// Create a sample struct type and method closure
	structType := &checker.StructDef{Name: "TestStruct"}
	closure := &VMClosure{}

	// Track completions and errors
	var wg sync.WaitGroup
	numGoroutines := 50

	// Half the goroutines write (addMethod), half read (getMethod)
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			if id%2 == 0 {
				// Write access
				methodName := "method"
				g.addMethod(structType, methodName, closure)
			} else {
				// Read access
				methodName := "method"
				_, _ = g.getMethod(structType, methodName)
			}
		}(i)
	}

	wg.Wait()
	// If we get here without panic, the fix works
}

// TestConcurrentMethodAccessWithMultipleMethods verifies safety with different method names
func TestConcurrentMethodAccessWithMultipleMethods(t *testing.T) {
	g := &GlobalVM{
		modules:        make(map[string]*VM),
		moduleScopes:   make(map[string]*scope),
		moduleRegistry: NewModuleRegistry(),
		ffiRegistry:    NewRuntimeFFIRegistry(),
	}

	structType := &checker.StructDef{Name: "TestStruct"}
	closure := &VMClosure{}

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			methodName := "method"

			if id%3 == 0 {
				g.addMethod(structType, methodName, closure)
			} else if id%3 == 1 {
				g.getMethod(structType, methodName)
			} else {
				g.addMethod(structType, methodName, closure)
				g.getMethod(structType, methodName)
			}
		}(i)
	}

	wg.Wait()
	// Success = no panic from concurrent map access
}

// TestConcurrentModuleAccess verifies that GlobalVM is safe for concurrent module lookups
func TestConcurrentModuleAccess(t *testing.T) {
	g := &GlobalVM{
		modules:        make(map[string]*VM),
		moduleScopes:   make(map[string]*scope),
		moduleRegistry: NewModuleRegistry(),
		ffiRegistry:    NewRuntimeFFIRegistry(),
	}

	// Pre-populate some modules
	for i := range 5 {
		name := fmt.Sprintf("module_%d", i)
		s := newScope(nil)
		vm := &VM{hq: g}
		g.modules[name] = vm
		g.moduleScopes[name] = s
	}

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Concurrent reads of different modules
			moduleName := fmt.Sprintf("module_%d", id%5)
			_, _, _ = g.getModule(moduleName)
		}(i)
	}

	wg.Wait()
	// Success = no panic from concurrent module map access
}
