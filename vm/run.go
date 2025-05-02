package vm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/akonwi/ard/checker_v2"
)

var void = &object{nil, checker_v2.Void}

func Run2(program *checker_v2.Program) (any, error) {
	vm := New()
	for _, statement := range program.Statements {
		vm.result = *vm.do(statement)
	}
	return vm.result.raw, nil
}

func (vm *VM) do(stmt checker_v2.Statement) *object {
	if stmt.Expr != nil {
		return vm.eval(stmt.Expr)
	}

	switch s := stmt.Stmt.(type) {
	case *checker_v2.VariableDef:
		val := vm.eval(s.Value)
		if !s.Mutable {
			original := val.raw
			var copy any = new(any)
			copy = original
			val.raw = copy
		}
		vm.scope.add(s.Name, val)
		return void
	case *checker_v2.Reassignment:
		target := vm.eval(s.Target)
		val := vm.eval(s.Value)
		target.raw = val.raw
		return void
	default:
		panic(fmt.Errorf("Unimplemented statement: %T", s))
	}
}

func (vm *VM) eval(expr checker_v2.Expression) *object {
	switch e := expr.(type) {
	case *checker_v2.StrLiteral:
		return &object{e.Value, e.Type()}
	case *checker_v2.BoolLiteral:
		return &object{e.Value, e.Type()}
	case *checker_v2.IntLiteral:
		return &object{e.Value, e.Type()}
	case *checker_v2.FloatLiteral:
		return &object{e.Value, e.Type()}
	case *checker_v2.TemplateStr:
		sb := strings.Builder{}
		for i := range e.Chunks {
			chunk := vm.eval(e.Chunks[i])
			sb.WriteString(chunk.raw.(string))
		}
		return &object{sb.String(), checker_v2.Str}
	case *checker_v2.Variable:
		val, ok := vm.scope.get(e.Name())
		if !ok {
			panic(fmt.Errorf("variable not found: %s", e.Name()))
		}
		return val
	case *checker_v2.Not:
		val := vm.eval(e.Value)
		return &object{!val.raw.(bool), val._type}

	case *checker_v2.Negation:
		val := vm.eval(e.Value)
		if num, isInt := val.raw.(int); isInt {
			return &object{-num, val._type}
		}
		return &object{-val.raw.(float64), val._type}
	case *checker_v2.IntAddition:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) + right.raw.(int),
			left._type,
		}
	case *checker_v2.IntSubtraction:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) - right.raw.(int),
			left._type,
		}
	case *checker_v2.IntMultiplication:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) * right.raw.(int),
			left._type,
		}
	case *checker_v2.IntDivision:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) / right.raw.(int),
			left._type,
		}
	case *checker_v2.IntModulo:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) % right.raw.(int),
			left._type,
		}
	case *checker_v2.IntGreater:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) > right.raw.(int),
			checker_v2.Bool,
		}
	case *checker_v2.IntGreaterEqual:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) >= right.raw.(int),
			checker_v2.Bool,
		}
	case *checker_v2.IntLess:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) < right.raw.(int),
			checker_v2.Bool,
		}
	case *checker_v2.IntLessEqual:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) <= right.raw.(int),
			checker_v2.Bool,
		}
	case *checker_v2.FloatDivision:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) / right.raw.(float64),
			left._type,
		}
	case *checker_v2.FloatMultiplication:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) * right.raw.(float64),
			left._type,
		}
	case *checker_v2.FloatSubtraction:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) - right.raw.(float64),
			left._type,
		}
	case *checker_v2.FloatAddition:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) + right.raw.(float64),
			left._type,
		}
	case *checker_v2.Equality:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{left.raw == right.raw, checker_v2.Bool}
	case *checker_v2.And:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{left.raw.(bool) && right.raw.(bool), checker_v2.Bool}
	case *checker_v2.Or:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{left.raw.(bool) || right.raw.(bool), checker_v2.Bool}
	case *checker_v2.If:
		if cond := vm.eval(e.Condition); cond.raw.(bool) {
			return vm.evalBlock2(e.Body, nil)
		} else if e.ElseIf != nil {
			if cond := vm.eval(e.ElseIf.Condition); cond.raw.(bool) {
				return vm.evalBlock2(e.ElseIf.Body, nil)
			}
		} else if e.Else != nil {
			return vm.evalBlock2(e.Else, nil)
		}
		return void
	case *checker_v2.FunctionDef:
		raw := func(args ...*object) *object {
			return vm.evalBlock2(e.Body, func() {
				for i := range args {
					vm.scope.add(e.Parameters[i].Name, args[i])
				}
			})
		}
		obj := &object{raw, e.Type()}
		vm.scope.add(e.Name, obj)
		return obj
	case *checker_v2.FunctionCall:
		sig, ok := vm.scope.get(e.Name)
		if !ok {
			panic(fmt.Errorf("Undefined: %s", e.Name))
		}
		fn, ok := sig.raw.(func(args ...*object) *object)
		if !ok {
			panic(fmt.Errorf("Not a function: %s: %s", e.Name, sig._type))
		}

		args := make([]*object, len(e.Args))
		for i := range e.Args {
			args[i] = vm.eval(e.Args[i])
		}

		return fn(args...)
	case *checker_v2.InstanceProperty:
		{
			subj := vm.eval(e.Subject)
			switch subj._type {
			case checker_v2.Str:
				return vm.evalStrProperty(subj, e.Property)
			default:
				return void
			}
		}
	case *checker_v2.InstanceMethod:
		{
			subj := vm.eval(e.Subject)
			if subj._type == checker_v2.Int {
				return vm.evalIntMethod(subj, e)
			}
			if _, ok := subj._type.(*checker_v2.List); ok {
				return vm.evalListMethod(subj, e)
			}

			panic(fmt.Errorf("Unimplemented: %s.%s() on %T", e.Subject.Type(), e.Method.Name, e.Subject.Type()))
		}
	case *checker_v2.PackageFunctionCall:
		{
			if e.Package == "ard/ints" {
				switch e.Call.Name {
				case "from_str":
					input := vm.eval(e.Call.Args[0]).raw.(string)

					// todo: this type should be a Maybe
					res := &object{nil, checker_v2.Int}
					if num, err := strconv.Atoi(input); err == nil {
						res.raw = num
					}
					return res
				default:
					panic(fmt.Errorf("Unimplemented: Int::%s()", e.Call.Name))
				}
			}

			if e.Package == "ard/io" {
				switch e.Call.Name {
				case "print":
					arg := vm.eval(e.Call.Args[0])

					string, ok := arg.raw.(string)
					if !ok {
						panic(fmt.Errorf("Unprintable arg to print: %s", arg))
					}
					fmt.Println(string)
					return void
				default:
					panic(fmt.Errorf("Unimplemented: io::%s()", e.Call.Name))
				}
			}
			panic(fmt.Errorf("Unimplemented: %s::%s()", e.Package, e.Call.Name))
		}
	case *checker_v2.ListLiteral:
		{
			raw := make([]*object, len(e.Elements))
			for i, el := range e.Elements {
				raw[i] = vm.eval(el)
			}
			return &object{raw, e.Type()}
		}
	default:
		panic(fmt.Errorf("Unimplemented expression: %T", e))
	}
}

func (vm *VM) evalBlock2(block *checker_v2.Block, init func()) *object {
	vm.pushScope()
	defer vm.popScope()

	if init != nil {
		init()
	}

	res := void
	for i := range block.Stmts {
		res = vm.do(block.Stmts[i])
	}

	return res
}

func (vm *VM) evalStrProperty(subj *object, name string) *object {
	switch name {
	case "size":
		return &object{len(subj.raw.(string)), checker_v2.Int}
	default:
		return void
	}
}

func (vm *VM) evalIntMethod(subj *object, m *checker_v2.InstanceMethod) *object {
	switch m.Method.Name {
	case "to_str":
		return &object{strconv.Itoa(subj.raw.(int)), checker_v2.Str}
	default:
		return void
	}
}

func (vm *VM) evalListMethod(subj *object, m *checker_v2.InstanceMethod) *object {
	raw := subj.raw.([]*object)
	switch m.Method.Name {
	case "at":
		index := vm.eval(m.Method.Args[0]).raw.(int)
		return &object{raw[index].raw, m.Type()}
	case "set":
		index := vm.eval(m.Method.Args[0]).raw.(int)
		value := vm.eval(m.Method.Args[1])
		result := &object{false, checker_v2.Bool}
		if index <= len(raw) {
			raw[index] = value
			result.raw = true
		}
		return result
	case "size":
		return &object{len(raw), checker_v2.Int}
	case "push":
		subj.raw = append(raw, vm.eval(m.Method.Args[0]))
		return subj
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", subj._type, m.Method.Name))
	}
}
