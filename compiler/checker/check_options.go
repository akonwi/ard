package checker

import "github.com/akonwi/ard/backend"

type CheckOptions struct {
	Target string
}

func normalizeCheckOptions(moduleResolver *ModuleResolver, options []CheckOptions) CheckOptions {
	if len(options) > 0 && options[0].Target != "" {
		return options[0]
	}

	if moduleResolver != nil {
		if project := moduleResolver.GetProjectInfo(); project != nil && project.Target != "" {
			return CheckOptions{Target: project.Target}
		}
	}

	return CheckOptions{Target: backend.DefaultTarget}
}
