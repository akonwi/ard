package vm

import (
	"fmt"
	"os"

	"github.com/akonwi/ard/checker"
)

// FSModule handles ard/fs module functions
type FSModule struct{}

func (m *FSModule) Path() string {
	return "ard/fs"
}

func (m *FSModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "append":
		path := args[0].raw.(string)
		content := args[1].raw.(string)
		resultType := call.Type().(*checker.Result)
		res := makeOk(void, resultType)
		if file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644); err != nil {
			res = makeErr(&object{err.Error(), resultType.Err()}, resultType)
		} else {
			if _, err := file.WriteString(content); err != nil {
				res = makeErr(&object{err.Error(), resultType.Err()}, resultType)
			}
			file.Close()
		}
		return res
	case "create_file":
		path := args[0].raw.(string)
		resultType := call.Type().(*checker.Result)
		res := makeOk(void, resultType)
		if file, err := os.Create(path); err != nil {
			res = makeErr(&object{err.Error(), resultType.Err()}, resultType)
		} else {
			file.Close()
		}
		return res
	case "delete":
		path := args[0].raw.(string)
		resultType := call.Type().(*checker.Result)
		res := makeOk(void, resultType)
		if err := os.Remove(path); err != nil {
			res = makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}
		return res
	case "exists":
		path := args[0].raw.(string)
		res := &object{false, call.Type()}
		if _, err := os.Stat(path); err == nil {
			res.raw = true
		}
		return res
	case "read":
		path := args[0].raw.(string)
		res := &object{nil, call.Type()}
		if content, err := os.ReadFile(path); err == nil {
			res.raw = string(content)
		}
		return res
	case "write":
		path := args[0].raw.(string)
		content := vm.Eval(call.Args[1]).raw.(string)
		resultType := call.Type().(*checker.Result)
		res := makeOk(void, resultType)
		/* file permissions:
		- `6` (owner): read (4) + write (2) = 6
		- `4` (group): read only
		- `4` (others): read only
		*/
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			res = makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}
		return res
	default:
		panic(fmt.Errorf("Unimplemented: fs::%s()", call.Name))
	}
}

func (m *FSModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	panic(fmt.Errorf("Unimplemented: fs::%s::%s()", structName, call.Name))
}
