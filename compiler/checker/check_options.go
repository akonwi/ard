package checker

type CheckOptions struct {
	// ModulePath overrides the checked module identity while keeping filePath for diagnostics.
	ModulePath string

	// GoResolver loads Go package metadata for direct `use go:` imports.
	GoResolver GoPackageResolver
}

func normalizeCheckOptions(options []CheckOptions) CheckOptions {
	if len(options) > 0 {
		return options[0]
	}
	return CheckOptions{}
}
