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

func (vm *VM) evalResultMethod(subj *object, call *checker.FunctionCall) *object {
	self := subj.raw.(_result)
	switch call.Name {
	case "expect":
		if !self.ok {
			actual := ""
			if self.raw._type == checker.Str {
				actual = self.raw.raw.(string)
			}
			_msg := vm.eval(call.Args[0]).raw.(string)
			panic(_msg + ": " + actual)
		}
		return self.raw
	case "or":
		if self.ok {
			return self.raw
		}
		return vm.eval(call.Args[0])
	}

	panic(fmt.Errorf("unimplemented: %s.%s", subj._type, call.Name))
}
