package vm

import (
	"bufio"
	"fmt"
	"os"

	"github.com/akonwi/ard/internal/checker"
)

// TODO: use this for 3rd party packages
// iiio := reflect.TypeFor[IO]()
// if print, ok := iiio.MethodByName("print"); ok {
// 	print.Func.Call([]reflect.Value{reflect.ValueOf("Hello, World)")})
// }

func (vm *VM) invokeIO(expr checker.Expression) *object {
	switch e := expr.(type) {
	case checker.FunctionCall:
		switch e.Name {
		case "print":
			arg := vm.evalExpression(e.Args[0])
			string := arg.raw.(string)
			fmt.Println(string)

		case "read_line":
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Scan()
			return &object{scanner.Text(), checker.Str{}}
		default:
			panic(fmt.Sprintf("Undefined io.%s", e.Name))
		}
	default:
		panic(fmt.Sprintf("Unimplemented io property: %s", e))
	}
	return &object{nil, checker.Void{}}
}
