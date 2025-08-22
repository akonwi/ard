package vm

import (
	"bufio"
	"fmt"
	"os"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// IOModule handles ard/io module functions
type IOModule struct{}

func (m *IOModule) Path() string {
	return "ard/io"
}

func (m *IOModule) Program() *checker.Program {
	return nil
}

func (m *IOModule) Handle(vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "print":
		toPrint := vm.evalInstanceMethod(args[0], &checker.InstanceMethod{
			Subject: call.Args[0],
			Method: &checker.FunctionCall{
				Name: "to_str",
				Args: []checker.Expression{},
			},
		}).Raw().(string)

		fmt.Println(toPrint)
		return runtime.Void()
	case "read_line":
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		if err := scanner.Err(); err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}
		return runtime.MakeOk(runtime.MakeStr(scanner.Text()))
	default:
		panic(fmt.Errorf("Unimplemented: io::%s()", call.Name))
	}
}

func (m *IOModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	panic(fmt.Errorf("Unimplemented: io::%s::%s()", structName, call.Name))
}
