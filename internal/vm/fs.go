package vm

import (
	"fmt"
	"os"

	"github.com/akonwi/ard/internal/checker"
)

func (vm *VM) invokeFS(expr checker.Expression) *object {
	switch e := expr.(type) {
	case checker.FunctionCall:
		switch e.Name {
		case "exists":
			path := vm.evalExpression(e.Args[0]).raw.(string)
			_, err := os.Stat(path)
			return &object{os.IsNotExist(err) == false, checker.Bool{}}

		case "read":
			path := vm.evalExpression(e.Args[0]).raw.(string)
			contents, err := os.ReadFile(path)
			if err != nil {
				return &object{nil, checker.MakeOption(checker.Str{})}
			}
			return &object{string(contents), checker.MakeOption(checker.Str{})}
		case "create_file":
			path := vm.evalExpression(e.Args[0]).raw.(string)
			_, err := os.Create(path)
			return &object{err == nil, checker.Bool{}}
		case "delete":
			path := vm.evalExpression(e.Args[0]).raw.(string)
			err := os.Remove(path)
			return &object{err == nil, checker.Bool{}}
		case "write":
			path := vm.evalExpression(e.Args[0]).raw.(string)
			content := vm.evalExpression(e.Args[1]).raw.(string)
			/* file permissions:
			- `6` (owner): read (4) + write (2) = 6
			- `4` (group): read only
			- `4` (others): read only
			*/
			err := os.WriteFile(path, []byte(content), 0644)
			return &object{err == nil, checker.Bool{}}
		case "append":
			path := vm.evalExpression(e.Args[0]).raw.(string)
			content := vm.evalExpression(e.Args[1]).raw.(string)
			file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return &object{false, checker.Bool{}}
			}
			defer file.Close()
			_, err = file.Write([]byte("\n" + content))
			return &object{err == nil, checker.Bool{}}
		default:
			panic(fmt.Sprintf("Undefined fs.%s", e.Name))
		}
	default:
		panic(fmt.Sprintf("Unimplemented fs property: %s", e))
	}
}
