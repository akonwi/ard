package vm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

func bytecodeToStrKind(kind int) checker.StrMethodKind {
	return checker.StrMethodKind(kind)
}

func bytecodeToIntKind(kind int) checker.IntMethodKind {
	return checker.IntMethodKind(kind)
}

func bytecodeToFloatKind(kind int) checker.FloatMethodKind {
	return checker.FloatMethodKind(kind)
}

func bytecodeToBoolKind(kind int) checker.BoolMethodKind {
	return checker.BoolMethodKind(kind)
}

func bytecodeToMaybeKind(kind int) checker.MaybeMethodKind {
	return checker.MaybeMethodKind(kind)
}

func bytecodeToResultKind(kind int) checker.ResultMethodKind {
	return checker.ResultMethodKind(kind)
}

func (vm *VM) evalStrMethod(kind checker.StrMethodKind, subj *runtime.Object, args []*runtime.Object) (*runtime.Object, error) {
	raw := subj.AsString()
	switch kind {
	case checker.StrSize:
		return runtime.MakeInt(len(raw)), nil
	case checker.StrIsEmpty:
		return runtime.MakeBool(len(raw) == 0), nil
	case checker.StrContains:
		return runtime.MakeBool(strings.Contains(raw, args[0].AsString())), nil
	case checker.StrReplace:
		old := args[0].AsString()
		newStr := args[1].AsString()
		return runtime.MakeStr(strings.Replace(raw, old, newStr, 1)), nil
	case checker.StrReplaceAll:
		old := args[0].AsString()
		newStr := args[1].AsString()
		return runtime.MakeStr(strings.ReplaceAll(raw, old, newStr)), nil
	case checker.StrSplit:
		sep := args[0].AsString()
		split := strings.Split(raw, sep)
		values := make([]*runtime.Object, len(split))
		for i, str := range split {
			values[i] = runtime.MakeStr(str)
		}
		return runtime.MakeList(checker.Str, values...), nil
	case checker.StrStartsWith:
		prefix := args[0].AsString()
		return runtime.MakeBool(strings.HasPrefix(raw, prefix)), nil
	case checker.StrToStr:
		return subj, nil
	case checker.StrToDyn:
		return runtime.MakeDynamic(raw), nil
	case checker.StrTrim:
		return runtime.MakeStr(strings.Trim(raw, " ")), nil
	default:
		return nil, fmt.Errorf("Unknown StrMethodKind: %d", kind)
	}
}

func (vm *VM) evalIntMethod(kind checker.IntMethodKind, subj *runtime.Object) (*runtime.Object, error) {
	switch kind {
	case checker.IntToStr:
		return runtime.MakeStr(strconv.Itoa(subj.AsInt())), nil
	case checker.IntToDyn:
		return runtime.MakeDynamic(subj.AsInt()), nil
	default:
		return nil, fmt.Errorf("Unknown IntMethodKind: %d", kind)
	}
}

func (vm *VM) evalFloatMethod(kind checker.FloatMethodKind, subj *runtime.Object) (*runtime.Object, error) {
	switch kind {
	case checker.FloatToStr:
		return runtime.MakeStr(strconv.FormatFloat(subj.AsFloat(), 'f', 2, 64)), nil
	case checker.FloatToInt:
		floatVal := subj.AsFloat()
		intVal := int(floatVal)
		return runtime.MakeInt(intVal), nil
	case checker.FloatToDyn:
		return runtime.MakeDynamic(subj.AsFloat()), nil
	default:
		return nil, fmt.Errorf("Unknown FloatMethodKind: %d", kind)
	}
}

func (vm *VM) evalBoolMethod(kind checker.BoolMethodKind, subj *runtime.Object) (*runtime.Object, error) {
	switch kind {
	case checker.BoolToStr:
		return runtime.MakeStr(strconv.FormatBool(subj.AsBool())), nil
	case checker.BoolToDyn:
		return runtime.MakeDynamic(subj.AsBool()), nil
	default:
		return nil, fmt.Errorf("Unknown BoolMethodKind: %d", kind)
	}
}

func (vm *VM) evalMaybeMethod(kind checker.MaybeMethodKind, subj *runtime.Object, args []*runtime.Object, returnType bytecode.TypeID) (*runtime.Object, error) {
	switch kind {
	case checker.MaybeExpect:
		if subj.Raw() == nil {
			return nil, fmt.Errorf("%s", args[0].AsString())
		}
		return vm.makeValueWithType(subj.Raw(), returnType)
	case checker.MaybeIsNone:
		return runtime.MakeBool(subj.Raw() == nil), nil
	case checker.MaybeIsSome:
		return runtime.MakeBool(subj.Raw() != nil), nil
	case checker.MaybeOr:
		if subj.Raw() == nil {
			return args[0], nil
		}
		return vm.makeValueWithType(subj.Raw(), returnType)
	default:
		return nil, fmt.Errorf("Unknown MaybeMethodKind: %d", kind)
	}
}

func (vm *VM) evalResultMethod(kind checker.ResultMethodKind, subj *runtime.Object, args []*runtime.Object) (*runtime.Object, error) {
	switch kind {
	case checker.ResultExpect:
		if subj.IsErr() {
			actual := ""
			if str, ok := subj.IsStr(); ok {
				actual = str
			} else {
				actual = fmt.Sprintf("%v", subj.GoValue())
			}
			return nil, fmt.Errorf("%s: %s", args[0].AsString(), actual)
		}
		unwrapped := subj.UnwrapResult()
		return unwrapped, nil
	case checker.ResultOr:
		if subj.IsErr() {
			return args[0], nil
		}
		unwrapped := subj.UnwrapResult()
		return unwrapped, nil
	case checker.ResultIsOk:
		return runtime.MakeBool(!subj.IsErr()), nil
	case checker.ResultIsErr:
		return runtime.MakeBool(subj.IsErr()), nil
	default:
		return nil, fmt.Errorf("Unknown ResultMethodKind: %d", kind)
	}
}

func (vm *VM) evalTraitMethodByName(subj *runtime.Object, name string, args []*runtime.Object) (*runtime.Object, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("trait method %s expects no args", name)
	}
	switch subj.Kind() {
	case runtime.KindStr:
		switch name {
		case "to_str":
			return vm.evalStrMethod(checker.StrToStr, subj, nil)
		case "to_dyn":
			return vm.evalStrMethod(checker.StrToDyn, subj, nil)
		}
	case runtime.KindInt:
		switch name {
		case "to_str":
			return vm.evalIntMethod(checker.IntToStr, subj)
		case "to_dyn":
			return vm.evalIntMethod(checker.IntToDyn, subj)
		}
	case runtime.KindFloat:
		switch name {
		case "to_str":
			return vm.evalFloatMethod(checker.FloatToStr, subj)
		case "to_dyn":
			return vm.evalFloatMethod(checker.FloatToDyn, subj)
		}
	case runtime.KindBool:
		switch name {
		case "to_str":
			return vm.evalBoolMethod(checker.BoolToStr, subj)
		case "to_dyn":
			return vm.evalBoolMethod(checker.BoolToDyn, subj)
		}
	}
	return nil, fmt.Errorf("unsupported trait method: %s", name)
}

func (vm *VM) makeValueWithType(raw any, typeID bytecode.TypeID) (*runtime.Object, error) {
	if typeID == 0 {
		return runtime.MakeDynamic(raw), nil
	}
	resolved, err := vm.typeFor(typeID)
	if err != nil {
		return nil, err
	}
	return runtime.Make(raw, resolved), nil
}
