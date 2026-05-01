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
		backend.TargetJSServer: true,
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
	"ard/io": {
		backend.TargetBytecode: true,
		backend.TargetGo:       true,
		backend.TargetJSServer: true,
	},
	"ard/argv": {
		backend.TargetBytecode: true,
		backend.TargetGo:       true,
		backend.TargetJSServer: true,
	},
	"ard/js/promise": {
		backend.TargetJSServer:  true,
		backend.TargetJSBrowser: true,
	},
	"ard/js/fetch": {
		backend.TargetJSServer:  true,
		backend.TargetJSBrowser: true,
	},
}

var targetDisplayOrder = []string{
	backend.TargetBytecode,
	backend.TargetVMNext,
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

func ValidateUnionMatchTarget(target string, unionType *Union, typeCases map[string]*Match) error {
	if target != backend.TargetJSBrowser && target != backend.TargetJSServer {
		return nil
	}
	if unionType == nil || !unionContainsIntAndFloat(unionType) {
		return nil
	}
	if typeCases["Int"] == nil && typeCases["Float"] == nil {
		return nil
	}

	return fmt.Errorf(
		"Cannot discriminate Int from Float in union matches when targeting %s; JavaScript represents both as number",
		target,
	)
}

func unionContainsIntAndFloat(unionType *Union) bool {
	hasInt := false
	hasFloat := false
	for _, t := range unionType.Types {
		switch t {
		case Int:
			hasInt = true
		case Float:
			hasFloat = true
		}
	}
	return hasInt && hasFloat
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
