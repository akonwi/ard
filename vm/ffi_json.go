package vm

import (
	"encoding/json/v2"

	"github.com/akonwi/ard/checker"
)

// encode an Ard value into a JSON string
func encode(vm *VM, args []*object) (*object, any) {
	bytes, err := json.Marshal(args[0])
	if err != nil {
		return nil, err.Error()
	}
	return &object{string(bytes), checker.Str}, nil
}
