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

	return nil, fmt.Errorf("unsupported type name: %s", name)
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
