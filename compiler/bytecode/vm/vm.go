package vm

import (
	"fmt"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/runtime"
)

type Frame struct {
	Fn       bytecode.Function
	IP       int
	Locals   []*runtime.Object
	Stack    []*runtime.Object
	MaxStack int
}

type VM struct {
	Program bytecode.Program
	Frames  []*Frame
}

func New(program bytecode.Program) *VM {
	return &VM{Program: program, Frames: []*Frame{}}
}

func (vm *VM) Run(functionName string) (*runtime.Object, error) {
	fn, ok := vm.lookupFunction(functionName)
	if !ok {
		return nil, fmt.Errorf("function not found: %s", functionName)
	}

	frame := &Frame{
		Fn:       fn,
		IP:       0,
		Locals:   make([]*runtime.Object, fn.Locals),
		Stack:    []*runtime.Object{},
		MaxStack: fn.MaxStack,
	}
	vm.Frames = append(vm.Frames, frame)

	return nil, fmt.Errorf("bytecode VM not implemented")
}

func (vm *VM) lookupFunction(name string) (bytecode.Function, bool) {
	for _, fn := range vm.Program.Functions {
		if fn.Name == name {
			return fn, true
		}
	}
	return bytecode.Function{}, false
}
