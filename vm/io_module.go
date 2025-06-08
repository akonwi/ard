package vm

import (
	"bufio"
	"fmt"
	"os"

	"github.com/akonwi/ard/checker"
)

// IOModule handles ard/io module functions
type IOModule struct{}

func (m *IOModule) Path() string {
	return "ard/io"
}

func (m *IOModule) Handle(vm VMEvaluator, call *checker.FunctionCall) *object {
	switch call.Name {
	case "print":
		toPrint := vm.Eval(&checker.InstanceMethod{
			Subject: call.Args[0],
			Method: &checker.FunctionCall{
				Name: "to_str",
				Args: []checker.Expression{},
			},
		}).raw.(string)

		fmt.Println(toPrint)
		return void
	case "read_line":
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		resultType := call.Type().(*checker.Result)
		if err := scanner.Err(); err != nil {
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}
		return makeOk(&object{scanner.Text(), resultType.Val()}, resultType)
	default:
		panic(fmt.Errorf("Unimplemented: io::%s()", call.Name))
	}
}
