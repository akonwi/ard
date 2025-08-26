//go:build goexperiment.jsonv2

package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

type VM struct {
	scope          *scope
	result         runtime.Object
	imports        map[string]checker.Module
	moduleRegistry *ModuleRegistry
	ffiRegistry    *RuntimeFFIRegistry
	moduleScope    *scope // Captures the scope where extern functions are defined
}

func New(imports map[string]checker.Module) *VM {
	vm := &VM{
		scope:          newScope(nil),
		moduleRegistry: NewModuleRegistry(),
		ffiRegistry:    NewRuntimeFFIRegistry(),
		imports:        imports,
	}
	vm.initModuleRegistry()
	vm.initFFIRegistry()
	return vm
}

func (vm *VM) initModuleRegistry() {
	// <prelude>
	vm.moduleRegistry.Register(&ResultModule{})
	// </prelude>

	for path := range vm.imports {
		switch path {
		case "ard/io":
			vm.moduleRegistry.Register(&IOModule{})
		case "ard/fs":
			vm.moduleRegistry.Register(&FSModule{})
		case "ard/maybe":
			vm.moduleRegistry.Register(&MaybeModule{})
		case "ard/http":
			vm.moduleRegistry.Register(&HTTPModule{})
		case "ard/decode":
			vm.moduleRegistry.Register(&DecodeModule{})
		// "ard/sqlite" is now handled through FFI, not as a module
		case "ard/async":
			vm.moduleRegistry.Register(&AsyncModule{})
		}
	}
}

func (vm *VM) initFFIRegistry() {
	// Register builtin FFI functions
	if err := vm.ffiRegistry.RegisterBuiltinFFIFunctions(); err != nil {
		panic(fmt.Errorf("failed to initialize FFI registry: %w", err))
	}
}

func (vm *VM) pushScope() {
	vm.scope = newScope(vm.scope)
}

func (vm *VM) popScope() {
	vm.scope = vm.scope.parent
}

// Eval implements VMEvaluator interface
func (vm *VM) Eval(expr checker.Expression) *runtime.Object {
	return vm.eval(expr)
}

/*
 * fns that are bound to a particular execution scope (*VM)
 */
type Closure struct {
	vm            *VM
	expr          checker.FunctionDef
	builtinFn     func(*runtime.Object, *checker.Result) *runtime.Object // for built-in decoder functions
	capturedScope *scope                                                 // scope at closure creation time
}

type ExternalFunctionWrapper struct {
	vm      *VM
	binding string
	def     checker.ExternalFunctionDef
}

func (c Closure) eval(args ...*runtime.Object) *runtime.Object {
	// Handle built-in functions
	if c.builtinFn != nil {
		data := args[0]
		resultType := c.expr.ReturnType.(*checker.Result)
		return c.builtinFn(data, resultType)
	}

	// Handle regular Ard functions
	// Save current VM scope
	originalScope := c.vm.scope

	// Ensure scope is restored even if function panics
	defer func() {
		c.vm.scope = originalScope
	}()

	// Set VM scope to captured scope for lexical scoping
	if c.capturedScope != nil {
		c.vm.scope = c.capturedScope
	}

	// Execute function with captured scope as base
	res, _ := c.vm.evalBlock(c.expr.Body, func() {
		for i := range args {
			c.vm.scope.add(c.expr.Parameters[i].Name, args[i])
		}
	})
	return res
}

func (e ExternalFunctionWrapper) eval(args ...*runtime.Object) *runtime.Object {
	// Call the external function through the FFI registry
	result, err := e.vm.ffiRegistry.Call(e.vm, e.binding, args, e.def.ReturnType)
	if err != nil {
		panic(fmt.Errorf("FFI call failed for %s: %w", e.binding, err))
	}
	return result
}

func (c Closure) Type() checker.Type {
	return c.expr.Type()
}
