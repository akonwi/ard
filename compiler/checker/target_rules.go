package checker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/akonwi/ard/backend"
)

var StdlibAllowedTargets = map[string]map[string]bool{
	"ard/fs": {
		backend.TargetBytecode: true,
		backend.TargetGo:       true,
	},
	"ard/sql": {
		backend.TargetBytecode: true,
		backend.TargetGo:       true,
	},
	"ard/env": {
		backend.TargetBytecode: true,
		backend.TargetGo:       true,
		backend.TargetJSServer: true,
	},
	"ard/argv": {
		backend.TargetBytecode: true,
		backend.TargetGo:       true,
		backend.TargetJSServer: true,
	},
}

var targetDisplayOrder = []string{
	backend.TargetBytecode,
	backend.TargetGo,
	backend.TargetJSBrowser,
	backend.TargetJSServer,
}

func ValidateStdlibImportTarget(path string, target string) error {
	if target == "" {
		target = backend.DefaultTarget
	}

	allowedTargets, restricted := StdlibAllowedTargets[path]
	if !restricted || allowedTargets[target] {
		return nil
	}

	return fmt.Errorf(
		"Cannot import %s when targeting %s; allowed targets: %s",
		path,
		target,
		strings.Join(orderedAllowedTargets(allowedTargets), ", "),
	)
}

func orderedAllowedTargets(allowedTargets map[string]bool) []string {
	ordered := make([]string, 0, len(allowedTargets))
	seen := make(map[string]bool, len(allowedTargets))
	for _, target := range targetDisplayOrder {
		if allowedTargets[target] {
			ordered = append(ordered, target)
			seen[target] = true
		}
	}

	extra := make([]string, 0)
	for target := range allowedTargets {
		if !seen[target] {
			extra = append(extra, target)
		}
	}
	sort.Strings(extra)
	return append(ordered, extra...)
}
