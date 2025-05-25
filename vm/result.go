package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

/*
 * runtime wrapper for results
 * raw is the Val|Err
 * ok is boolean flag for whether it is ok or not
 * for immutability, _result will always be referenced by value
 */
type _result struct {
	ok  bool
	raw *object
}

func makeOk(raw *object, resultType *checker.Result) *object {
	return &object{
		raw:   _result{ok: true, raw: raw},
		_type: resultType,
	}
}

func makeErr(raw *object, resultType *checker.Result) *object {
	return &object{
		raw:   _result{ok: false, raw: raw},
		_type: resultType,
	}
}

func evalInResult(vm *VM, call *checker.FunctionCall) *object {
	switch call.Name {
	case "ok", "err":
		resultType := call.Type().(*checker.Result)
		res := vm.eval(call.Args[0])
		if call.Name == "ok" {
			return makeOk(res, resultType)
		}
		return makeErr(res, resultType)
	default:
		panic(fmt.Errorf("unimplemented: Result::%s", call.Name))
	}
}

func (vm *VM) evalResultMethod(subj *object, call *checker.FunctionCall) *object {
	switch call.Name {
	case "or":
		rawObj := subj.raw.(_result)
		if rawObj.ok {
			return rawObj.raw
		}
		return vm.eval(call.Args[0])
	}

	panic(fmt.Errorf("unimplemented: %s.%s", subj._type, call.Name))
}
