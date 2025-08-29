package vm

import (
	"fmt"
	"os"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

// FSModule handles ard/fs module functions
type FSModule struct{}

func (m *FSModule) Path() string {
	return "ard/fs"
}

func (m *FSModule) Program() *checker.Program {
	return nil
}

func (m *FSModule) Handle(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "append":
		path := args[0].Raw().(string)
		content := args[1].Raw().(string)
		if file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644); err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		} else {
			if _, err := file.WriteString(content); err != nil {
				file.Close()
				return runtime.MakeErr(runtime.MakeStr(err.Error()))
			}
			file.Close()
		}
		return runtime.MakeOk(runtime.Void())
	case "create_file":
		path := args[0].Raw().(string)
		if file, err := os.Create(path); err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		} else {
			file.Close()
		}
		return runtime.MakeOk(runtime.Void())
	case "delete":
		path := args[0].Raw().(string)
		if err := os.Remove(path); err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}
		return runtime.MakeOk(runtime.Void())
	case "exists":
		path := args[0].Raw().(string)
		if _, err := os.Stat(path); err == nil {
			return runtime.MakeBool(true)
		}
		return runtime.MakeBool(false)
	case "read":
		path := args[0].Raw().(string)
		if content, err := os.ReadFile(path); err == nil {
			return runtime.MakeStr(string(content)).ToSome()
		}
		return runtime.Make(nil, call.Type())
	case "write":
		path := args[0].Raw().(string)
		content := args[0].String()
		/* file permissions:
		- `6` (owner): read (4) + write (2) = 6
		- `4` (group): read only
		- `4` (others): read only
		*/
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}
		return runtime.MakeOk(runtime.Void())
	default:
		panic(fmt.Errorf("Unimplemented: fs::%s()", call.Name))
	}
}

func (m *FSModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	panic(fmt.Errorf("Unimplemented: fs::%s::%s()", structName, call.Name))
}
