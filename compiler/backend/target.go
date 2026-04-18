package backend

import "fmt"

const (
	TargetBytecode = "bytecode"
	TargetGo       = "go"
	DefaultTarget  = TargetBytecode
)

func ParseTarget(raw string) (string, error) {
	switch raw {
	case "", DefaultTarget:
		return DefaultTarget, nil
	case TargetGo:
		return TargetGo, nil
	default:
		return "", fmt.Errorf("unknown target: %s", raw)
	}
}
