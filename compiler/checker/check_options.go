package checker

import "github.com/akonwi/ard/backend"

type CheckOptions struct {
	Target string
	// ModulePath overrides the checked module identity while keeping filePath for diagnostics.
	ModulePath string
}

func normalizeCheckOptions(moduleResolver *ModuleResolver, options []CheckOptions) CheckOptions {
	normalized := CheckOptions{}
	if len(options) > 0 {
		normalized = options[0]
	}

	if normalized.Target != "" {
		return normalized
	}

	if moduleResolver != nil {
		if project := moduleResolver.GetProjectInfo(); project != nil && project.Target != "" {
			normalized.Target = project.Target
			return normalized
		}
	}

	normalized.Target = backend.DefaultTarget
	return normalized
}
