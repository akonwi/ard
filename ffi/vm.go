package ffi

import (
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// VM interface defines the methods needed by FFI functions
type VM interface {
	EvalStructMethod(obj *runtime.Object, call *checker.FunctionCall) *runtime.Object
	EvalEnumMethod(obj *runtime.Object, call *checker.FunctionCall, enum *checker.Enum) *runtime.Object
}
