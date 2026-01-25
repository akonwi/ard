//go:build goexperiment.jsonv2

package vm

import (
	"errors"
	"fmt"
	"sync"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

const SUBJECT = "__subject__"

type GlobalVM struct {
	// modules and moduleScopes are loaded once during initialization and never modified
	// multiple fibers can safely read them concurrently
	modules      map[string]*VM
	moduleScopes map[string]*scope
	subject      checker.Module

	// method closures lose their scope because they aren't evaluated in their module like regular functions
	// so keeping them track of them globally solves that
	// [needs-improvement]
	// uses sync.Map for concurrent read-heavy access by multiple fibers
	methodClosures sync.Map // map[string]runtime.Closure

	// hardcoded modules
	moduleRegistry *ModuleRegistry

	// Go functions for FFI
	ffiRegistry *RuntimeFFIRegistry
}

func NewRuntime(module checker.Module, scripting ...bool) *GlobalVM {
	g := &GlobalVM{
		subject:      module,
		modules:      map[string]*VM{},
		moduleScopes: map[string]*scope{},
		ffiRegistry: NewRuntimeFFIRegistry(),
		moduleRegistry: NewModuleRegistry(),
	}
	g.initFFIRegistry()
	g.initModuleRegistry()

	g.loadImports(module.Program().Imports)
	g.loadModule(SUBJECT, module.Program(), len(scripting) > 0 && scripting[0] == true)
	return g
}

func NewScriptRuntime(module checker.Module) *GlobalVM {
	return NewRuntime(module, true)
}

// go through the dependency tree and make sure a single instance of each module is ready
func (g *GlobalVM) loadImports(imports map[string]checker.Module) {
	for name, mod := range imports {
		if _, exists := g.modules[name]; !exists {
			program := mod.Program()
			if program != nil {
				g.loadImports(program.Imports)
				g.loadModule(name, program, false)
			}
		}
	}
}

func (g *GlobalVM) loadModule(name string, program *checker.Program, scripting bool) (*VM, *scope) {
	s := newScope(nil)
	vm := NewVM(g)
	if !scripting {
		vm.init(program, s)
	}
	g.modules[name] = vm
	g.moduleScopes[name] = s

	return vm, s
}

func (g *GlobalVM) unloadModule(name string) {
	delete(g.modules, name)
	delete(g.moduleScopes, name)
}

// initialize the part of the standard library that is hardcoded into the compiler
func (g *GlobalVM) initModuleRegistry() {
	g.moduleRegistry.Register(&ResultModule{})
	g.moduleRegistry.Register(&MaybeModule{})
}

func (vm *GlobalVM) initFFIRegistry() {
	// Register builtin FFI functions
	if err := vm.ffiRegistry.RegisterBuiltinFFIFunctions(); err != nil {
		panic(fmt.Errorf("failed to initialize FFI registry: %w", err))
	}
}

func (g *GlobalVM) getModule(name string) (*VM, *scope, bool) {
	vm, hasMod := g.modules[name]
	scope, hasScope := g.moduleScopes[name]
	return vm, scope, hasMod && hasScope
}

// call the program's main function
func (g *GlobalVM) Run(fnName string) error {
	vm, scope, ok := g.getModule(SUBJECT)
	if !ok {
		return errors.New("Module not initialized")
	}

	vm.callMain(fnName, scope)
	return nil
}

// evaluate the subject program as a script
func (g *GlobalVM) Interpret() (any, error) {
	program := g.subject.Program()
	mvm, scope, ok := g.getModule(SUBJECT)
	if !ok {
		return nil, errors.New("Module not initialized")
	}
	return mvm.Interpret(program, scope)
}

// call into another module
func (g *GlobalVM) callOn(moduleName string, call *checker.FunctionCall, getArgs func() []*runtime.Object) *runtime.Object {
	// check for Ard code
	if mvm, scope, ok := g.getModule(moduleName); ok {
		return mvm.evalFunctionCall(scope, call, getArgs()...)
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
	// [todo] maestro-api randomly reaches this point on startup
	panic(fmt.Errorf("Unimplemented: %s::%s()", moduleName, call.Name))
}

func (g *GlobalVM) lookup(moduleName string, symbol checker.Symbol) *runtime.Object {
	_, scope, ok := g.getModule(moduleName)
	if !ok {
		if mod, ok := g.moduleRegistry.handlers[moduleName]; ok {
			return mod.get(symbol.Name)
		} else {
			panic(fmt.Errorf("Module not found: %s", moduleName))
		}
	}

	sym, _ := scope.get(symbol.Name)
	return sym
}

func (g *GlobalVM) addMethod(strct checker.Type, name string, closure runtime.Closure) {
	key := fmt.Sprintf("%s.%s", strct.String(), name)
	g.methodClosures.Store(key, closure)
}

func (g *GlobalVM) getMethod(strct checker.Type, name string) (runtime.Closure, bool) {
	key := fmt.Sprintf("%s.%s", strct.String(), name)
	if val, ok := g.methodClosures.Load(key); ok {
		return val.(runtime.Closure), true
	}
	return nil, false
}

type VM struct {
	hq     *GlobalVM
	result runtime.Object
}

func NewVM(hq *GlobalVM) *VM {
	vm := &VM{
		hq: hq,
	}
	return vm
}

// evaluate all top-level statements to build up the module scope
func (vm *VM) init(program *checker.Program, scope *scope) {
	for _, statement := range program.Statements {
		vm.do(statement, scope)
	}

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

// [refactor] i think there's room for simplification here
func (c VMClosure) Eval(args ...*runtime.Object) *runtime.Object {
	// Handle built-in functions
	if c.builtinFn != nil {
		data := args[0]
		resultType := c.expr.ReturnType.(*checker.Result)
		return c.builtinFn(data, resultType)
	}

	// Execute function with captured scope as base
	res, _ := c.vm.evalBlock(c.capturedScope, c.expr.Body, func(sc *scope) {
		for i := range args {
			sc.add(c.expr.Parameters[i].Name, args[i])
		}
	})
	return res
}

func (c VMClosure) EvalIsolated(args ...*runtime.Object) *runtime.Object {
	// Create a shallow copy of the VM with a new scope
	isolatedVM := &VM{
		hq: c.vm.hq, // Share GlobalVM (read-only)
	}

	// Create a new closure pointing to isolated VM
	isolatedClosure := VMClosure{
		vm:            isolatedVM,
		expr:          c.expr,
		capturedScope: c.capturedScope,
	}

	// Execute normally - no race condition because scope is isolated
	return isolatedClosure.Eval(args...)
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

func (e ExternClosure) EvalIsolated(args ...*runtime.Object) *runtime.Object {
	return e.Eval(args...)
}

func (c ExternClosure) GetParams() []checker.Parameter {
	return c.def.Parameters
}

func (c VMClosure) Type() checker.Type {
	return c.expr.Type()
}

func (c VMClosure) GetParams() []checker.Parameter {
	return c.expr.Parameters
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

func (c BuiltInClosure) EvalIsolated(args ...*runtime.Object) *runtime.Object {
	return c.Eval(args...)
}

func (c BuiltInClosure) GetParams() []checker.Parameter {
	return []checker.Parameter{}
}
