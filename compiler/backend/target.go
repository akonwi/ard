package backend

import "fmt"

const (
	TargetVMNext    = "vm_next"
	TargetGo        = "go"
	TargetJSBrowser = "js-browser"
	TargetJSServer  = "js-server"
	DefaultTarget   = TargetVMNext
)

func ParseTarget(raw string) (string, error) {
	switch raw {
	case "", DefaultTarget:
		return DefaultTarget, nil
	case TargetGo:
		return TargetGo, nil
	case TargetJSBrowser:
		return TargetJSBrowser, nil
	case TargetJSServer:
		return TargetJSServer, nil
	default:
		return "", fmt.Errorf("unknown target: %s", raw)
	}
}
