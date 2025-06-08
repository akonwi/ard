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

func (m *FSModule) Handle(vm VMEvaluator, call *checker.FunctionCall) *object {
	switch call.Name {
	case "append":
		path := vm.Eval(call.Args[0]).raw.(string)
		content := vm.Eval(call.Args[1]).raw.(string)
		res := &object{false, call.Type()}
		if file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644); err == nil {
			if _, err := file.WriteString(content); err == nil {
				res.raw = true
			}
			file.Close()
		}
		return res
	case "create_file":
		path := vm.Eval(call.Args[0]).raw.(string)
		res := &object{false, call.Type()}
		if file, err := os.Create(path); err == nil {
			file.Close()
			res.raw = true
		}
		return res
	case "delete":
		path := vm.Eval(call.Args[0]).raw.(string)
		res := &object{false, call.Type()}
		if err := os.Remove(path); err == nil {
			res.raw = true
		}
		return res
	case "exists":
		path := vm.Eval(call.Args[0]).raw.(string)
		res := &object{false, call.Type()}
		if _, err := os.Stat(path); err == nil {
			res.raw = true
		}
		return res
	case "read":
		path := vm.Eval(call.Args[0]).raw.(string)
		res := &object{nil, call.Type()}
		if content, err := os.ReadFile(path); err == nil {
			res.raw = string(content)
		}
		return res
	case "write":
		path := vm.Eval(call.Args[0]).raw.(string)
		content := vm.Eval(call.Args[1]).raw.(string)
		res := &object{false, call.Type()}
		/* file permissions:
		- `6` (owner): read (4) + write (2) = 6
		- `4` (group): read only
		- `4` (others): read only
		*/
		if err := os.WriteFile(path, []byte(content), 0644); err == nil {
			res.raw = true
		}
		return res
	default:
		panic(fmt.Errorf("Unimplemented: fs::%s()", call.Name))
	}
}
