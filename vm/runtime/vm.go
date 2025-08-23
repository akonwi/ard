package runtime

import (
	"github.com/akonwi/ard/checker"
)

// VM interface defines the methods needed by FFI functions
type VM interface {
	EvalStructMethod(obj *Object, call *checker.FunctionCall) *Object
	EvalEnumMethod(obj *Object, call *checker.FunctionCall, enum *checker.Enum) *Object
}
