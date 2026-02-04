package vm

import (
	"fmt"
	"strings"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/checker"
)

func (vm *VM) typeFor(id bytecode.TypeID) (checker.Type, error) {
	if id == 0 {
		return nil, nil
	}
	if vm.typeCache == nil {
		vm.typeCache = map[bytecode.TypeID]checker.Type{}
	}
	if t, ok := vm.typeCache[id]; ok {
		return t, nil
	}

	var entry *bytecode.TypeEntry
	for i := range vm.Program.Types {
		if vm.Program.Types[i].ID == id {
			entry = &vm.Program.Types[i]
			break
		}
	}
	if entry == nil {
		return nil, fmt.Errorf("unknown type id: %d", id)
	}
	parsed, err := parseTypeName(entry.Name)
	if err != nil {
		return nil, err
	}
	vm.typeCache[id] = parsed
	return parsed, nil
}

func (vm *VM) typeNameFor(id bytecode.TypeID) (string, error) {
	if id == 0 {
		return "", nil
	}
	for i := range vm.Program.Types {
		if vm.Program.Types[i].ID == id {
			return vm.Program.Types[i].Name, nil
		}
	}
	return "", fmt.Errorf("unknown type id: %d", id)
}

func (vm *VM) structTypeFor(id bytecode.TypeID) (*checker.StructDef, error) {
	if t, ok := vm.typeCache[id]; ok {
		if s, ok := t.(*checker.StructDef); ok {
			return s, nil
		}
	}
	name, err := vm.typeNameFor(id)
	if err != nil {
		return nil, err
	}
	strct := &checker.StructDef{Name: name, Fields: map[string]checker.Type{}, Methods: map[string]*checker.FunctionDef{}}
	vm.typeCache[id] = strct
	return strct, nil
}

func (vm *VM) enumTypeFor(id bytecode.TypeID) (*checker.Enum, error) {
	if t, ok := vm.typeCache[id]; ok {
		if e, ok := t.(*checker.Enum); ok {
			return e, nil
		}
	}
	name, err := vm.typeNameFor(id)
	if err != nil {
		return nil, err
	}
	enum := &checker.Enum{Name: name, Values: []checker.EnumValue{}, Methods: map[string]*checker.FunctionDef{}}
	vm.typeCache[id] = enum
	return enum, nil
}

func parseTypeName(name string) (checker.Type, error) {
	trimmed := strings.TrimSpace(name)
	if strings.Contains(trimmed, "$") {
		return checker.Dynamic, nil
	}
	switch trimmed {
	case checker.Int.String():
		return checker.Int, nil
	case checker.Float.String():
		return checker.Float, nil
	case checker.Str.String():
		return checker.Str, nil
	case checker.Bool.String():
		return checker.Bool, nil
	case checker.Void.String():
		return checker.Void, nil
	case checker.Dynamic.String():
		return checker.Dynamic, nil
	}
	if strings.HasSuffix(trimmed, "?") {
		inner := strings.TrimSuffix(trimmed, "?")
		of, err := parseTypeName(inner)
		if err != nil {
			return nil, err
		}
		return checker.MakeMaybe(of), nil
	}
	leftBang, rightBang := splitTopLevel(trimmed, '!')
	if rightBang != "" {
		leftType, err := parseTypeName(leftBang)
		if err != nil {
			return nil, err
		}
		rightType, err := parseTypeName(rightBang)
		if err != nil {
			return nil, err
		}
		return checker.MakeResult(leftType, rightType), nil
	}

	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		inner := strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
		left, right := splitTopLevel(inner, ':')
		if right == "" {
			of, err := parseTypeName(left)
			if err != nil {
				return nil, err
			}
			return checker.MakeList(of), nil
		}
		keyType, err := parseTypeName(left)
		if err != nil {
			return nil, err
		}
		valType, err := parseTypeName(right)
		if err != nil {
			return nil, err
		}
		return checker.MakeMap(keyType, valType), nil
	}

	return &checker.StructDef{Name: trimmed, Fields: map[string]checker.Type{}, Methods: map[string]*checker.FunctionDef{}}, nil
}

func splitTopLevel(s string, sep rune) (string, string) {
	depth := 0
	for i, r := range s {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		default:
			if r == sep && depth == 0 {
				return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
			}
		}
	}
	return strings.TrimSpace(s), ""
}
