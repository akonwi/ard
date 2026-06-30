package checker

type CheckOptions struct {
	// ModulePath overrides the checked module identity while keeping filePath for diagnostics.
	ModulePath string
}

func normalizeCheckOptions(options []CheckOptions) CheckOptions {
	if len(options) > 0 {
		return options[0]
	}
	return CheckOptions{}
}
