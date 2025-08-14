//go:build goexperiment.jsonv2

package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

type VM struct {
	scope          *scope
	result         object
	imports        map[string]checker.Module
	moduleRegistry *ModuleRegistry
	ffiRegistry    *RuntimeFFIRegistry
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
	vm.moduleRegistry.Register(&IntModule{})
	vm.moduleRegistry.Register(&FloatModule{})
	vm.moduleRegistry.Register(&ResultModule{})
	vm.moduleRegistry.Register(&ListModule{})
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
		case "ard/json":
			vm.moduleRegistry.Register(&JSONModule{})
		case "ard/decode":
			vm.moduleRegistry.Register(&DecodeModule{})
		case "ard/sqlite":
			vm.moduleRegistry.Register(&SQLiteModule{})
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
func (vm *VM) Eval(expr checker.Expression) *object {
	return vm.eval(expr)
}

/*
 * fns that are bound to a particular execution scope (*VM)
 */
type Closure struct {
	vm            *VM
	expr          checker.FunctionDef
	builtinFn     func(*object, *checker.Result) *object // for built-in decoder functions
	capturedScope *scope                                 // scope at closure creation time
}

type ExternalFunctionWrapper struct {
	vm      *VM
	binding string
	def     checker.ExternalFunctionDef
}

func (c Closure) eval(args ...*object) *object {
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

func (e ExternalFunctionWrapper) eval(args ...*object) *object {
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

type object struct {
	raw   any
	_type checker.Type
}

func (o object) String() string {
	return fmt.Sprintf("%v:%s", o.raw, o._type)
}

func (o *object) premarshal() any {
	if o._type == checker.Void || o._type == nil {
		return nil
	}
	if o._type == checker.Str || o._type == checker.Int || o._type == checker.Float || o._type == checker.Bool {
		return o.raw
	}
	if o._type == checker.Dynamic {
		return o.raw
	}

	switch o._type.(type) {
	case *checker.FunctionDef:
		return o._type.String()
	case *checker.Enum:
		return o.raw
	case *checker.Maybe, *checker.Any:
		if o.raw == nil {
			return nil
		}
		if inner, ok := o.raw.(*object); ok {
			return inner.premarshal()
		}
		return o.raw
	case *checker.List:
		raw := o.raw.([]*object)
		_array := make([]any, len(raw))
		for i, item := range raw {
			_array[i] = item.premarshal()
		}
		return _array
	case *checker.Result:
		return o.raw.(*object).premarshal()
	}

	if _, isStruct := o._type.(*checker.StructDef); isStruct {
		m := o.raw.(map[string]*object)
		rawMap := make(map[string]any)
		for key, value := range m {
			rawMap[key] = value.premarshal()
		}
		return rawMap
	}

	if _, isMap := o._type.(*checker.Map); isMap {
		m := o.raw.(map[string]*object)
		rawMap := make(map[string]any)
		for key, value := range m {
			rawMap[key] = value.premarshal()
		}
		return rawMap
	}

	panic(fmt.Sprintf("Cannot marshall type: %T", o._type))
}
