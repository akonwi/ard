//go:build goexperiment.jsonv2

package vm

import (
	"errors"
	"fmt"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

type GlobalVM struct {
	modules map[string]*VM
	subject checker.Module

	// method closures lose their scope because they aren't evaluated in their module like regular functions
	// so keeping them track of them globally solves that
	// [needs-improvement]
	methodClosures map[string]runtime.Closure

	// hardcoded modules
	moduleRegistry *ModuleRegistry

	// Go functions for FFI
	ffiRegistry *RuntimeFFIRegistry
}

func NewRuntime(module checker.Module) *GlobalVM {
	g := &GlobalVM{
		subject:        module,
		methodClosures: map[string]runtime.Closure{},
		modules:        make(map[string]*VM),
		ffiRegistry:    NewRuntimeFFIRegistry(),
	}
	g.load(module.Program().Imports)
	g.moduleRegistry = NewModuleRegistry()
	g.initModuleRegistry()
	g.initFFIRegistry()
	return g
}

// go through the dependency tree and make sure a single instance of each module is ready
func (g *GlobalVM) load(imports map[string]checker.Module) error {
	for name, mod := range imports {
		if _, exists := g.modules[name]; !exists {
			program := mod.Program()
			if program != nil {
				vm := NewVM()
				vm.hq = g
				if _, err := vm.Interpret(program); err != nil {
					return fmt.Errorf("Failed to load module - %s: %w", name, err)
				}
				g.modules[name] = vm
				g.load(program.Imports)
			}
		}
	}
	return nil
}

// initialize the part of the standard library that is hardcoded into the compiler
func (g *GlobalVM) initModuleRegistry() {
	g.moduleRegistry.Register(&ResultModule{})
	g.moduleRegistry.Register(&MaybeModule{})
	g.moduleRegistry.Register(&HTTPModule{hq: g, vm: NewVM()})
	g.moduleRegistry.Register(&AsyncModule{hq: g})
}

func (vm *GlobalVM) initFFIRegistry() {
	// Register builtin FFI functions
	if err := vm.ffiRegistry.RegisterBuiltinFFIFunctions(); err != nil {
		panic(fmt.Errorf("failed to initialize FFI registry: %w", err))
	}
}

// call the program's main function
func (g *GlobalVM) Run() error {
	vm := NewVM()
	vm.hq = g
	program := g.subject.Program()

	hasMain := false
	for _, stmt := range program.Statements {
		if stmt.Expr == nil {
			continue
		}
		if fn, ok := stmt.Expr.Type().(*checker.FunctionDef); ok {
			if fn.Name == "main" && len(fn.Parameters) == 0 && fn.ReturnType == checker.Void {
				hasMain = true
				break
			}
		}
	}

	if !hasMain {
		return errors.New("No main function found")
	}

	// build up scope
	if _, err := vm.Interpret(program); err != nil {
		return err
	}
	return vm.callMain()
}

// evaluate the subject program as a script
func (g *GlobalVM) Interpret() (any, error) {
	vm := NewVM()
	vm.hq = g
	program := g.subject.Program()
	return vm.Interpret(program)
}

// call into another module
func (g *GlobalVM) callOn(moduleName string, call *checker.FunctionCall, getArgs func() []*runtime.Object) *runtime.Object {
	// check for Ard code
	if mvm, ok := g.modules[moduleName]; ok {
		return mvm.evalFunctionCall(call, getArgs()...)
	}
	// check in hardcoded std-lib
	if g.moduleRegistry.HasModule(moduleName) {
		module, ok := g.moduleRegistry.handlers[moduleName]
		if !ok {
			panic(fmt.Errorf("Unimplemented: %s::%s()", moduleName, call.Name))
		}

		args := []*runtime.Object{}
		if getArgs != nil {
			args = getArgs()
		}
		return module.Handle(call, args)
	}
	panic(fmt.Errorf("Unimplemented: %s::%s()", moduleName, call.Name))
}

func (g *GlobalVM) lookup(moduleName string, symbol checker.Symbol) *runtime.Object {
	module, ok := g.modules[moduleName]
	if !ok {
		if mod, ok := g.moduleRegistry.handlers[moduleName]; ok {
			return mod.get(symbol.Name)
		} else {
			panic(fmt.Errorf("Module not found: %s", moduleName))
		}
	}

	sym, _ := module.scope.get(symbol.Name)
	return sym
}

func (g *GlobalVM) addMethod(strct checker.Type, name string, closure runtime.Closure) {
	key := fmt.Sprintf("%p.%s", strct, name)
	g.methodClosures[key] = closure
}

func (g *GlobalVM) getMethod(strct checker.Type, name string) (runtime.Closure, bool) {
	key := fmt.Sprintf("%p.%s", strct, name)
	if closure, ok := g.methodClosures[key]; ok {
		return closure, true
	}
	return nil, false
}

type VM struct {
	hq     *GlobalVM
	scope  *scope
	result runtime.Object

	moduleScope *scope // Captures the scope where extern functions are defined
}

func NewVM() *VM {
	vm := &VM{
		scope: newScope(nil),
	}
	return vm
}

func (vm *VM) pushScope() {
	vm.scope = newScope(vm.scope)
}

func (vm *VM) popScope() {
	vm.scope = vm.scope.parent
}

func (vm *VM) Eval(expr checker.Expression) *runtime.Object {
	return vm.eval(expr)
}

/*
 * fns that are bound to a particular module VM
 */
type VMClosure struct {
	vm            *VM
	expr          *checker.FunctionDef
	builtinFn     func(*runtime.Object, *checker.Result) *runtime.Object // for built-in decoder functions
	capturedScope *scope                                                 // scope at closure creation time
}

func (c VMClosure) Eval(args ...*runtime.Object) *runtime.Object {
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

func (c VMClosure) IsolateEval(args ...*runtime.Object) *runtime.Object {
	// this feels like something i'll regret
	vm := NewVM()
	vm.hq = c.vm.hq
	c.vm = vm

	return c.Eval()
}

// returns a copy without a VM reference
func (c VMClosure) copy() *VMClosure {
	scope := *c.capturedScope
	return &VMClosure{
		vm:            nil,
		expr:          c.expr,
		capturedScope: &scope,
	}
}

// fns for FFI, which aren't bound to a vm and have no scope
type ExternClosure struct {
	hq      *GlobalVM
	binding string
	def     checker.ExternalFunctionDef
}

func (e ExternClosure) Eval(args ...*runtime.Object) *runtime.Object {
	// Call the external function through the FFI registry
	result, err := e.hq.ffiRegistry.Call(e.binding, args, e.def.ReturnType)
	if err != nil {
		panic(fmt.Errorf("FFI call failed for %s: %w", e.binding, err))
	}
	return result
}

func (e ExternClosure) IsolateEval(args ...*runtime.Object) *runtime.Object {
	return e.Eval(args...)
}

func (c VMClosure) Type() checker.Type {
	return c.expr.Type()
}

type BuiltInClosure struct {
	builtinFn func(*runtime.Object, *checker.Result) *runtime.Object
	retType   checker.Type
}

func (c BuiltInClosure) Eval(args ...*runtime.Object) *runtime.Object {
	data := args[0]
	resultType := c.retType.(*checker.Result)
	return c.builtinFn(data, resultType)
}

func (c BuiltInClosure) IsolateEval(args ...*runtime.Object) *runtime.Object {
	return c.Eval(args...)
}
