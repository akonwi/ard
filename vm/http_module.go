package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

// HTTPModule handles ard/http module functions
type HTTPModule struct{}

func (m *HTTPModule) Path() string {
	return "ard/http"
}

func (m *HTTPModule) Functions() []string {
	return []string{"send"}
}

func (m *HTTPModule) Handle(vm VMEvaluator, call *checker.FunctionCall) *object {
	switch call.Name {
	case "send":
		// Cast back to *VM to access the original evalHttpSend function
		// This preserves the existing complex HTTP logic
		if vmInstance, ok := vm.(*VM); ok {
			return evalHttpSend(vmInstance, call)
		}
		panic(fmt.Errorf("HTTP module requires full VM instance"))
	default:
		panic(fmt.Errorf("Unimplemented: http::%s()", call.Name))
	}
}
