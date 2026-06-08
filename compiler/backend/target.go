package backend

import "fmt"

const (
	TargetGo        = "go"
	TargetJSBrowser = "js-browser"
	TargetJSServer  = "js-server"
	TargetZig       = "zig"
	DefaultTarget   = TargetGo
)

func ParseTarget(raw string) (string, error) {
	switch raw {
	case "":
		return DefaultTarget, nil
	case TargetGo:
		return TargetGo, nil
	case TargetJSBrowser:
		return TargetJSBrowser, nil
	case TargetJSServer:
		return TargetJSServer, nil
	case TargetZig:
		return TargetZig, nil
	default:
		return "", fmt.Errorf("unknown target: %s", raw)
	}
}
