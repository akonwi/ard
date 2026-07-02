package checker

type TargetInfo struct {
	IntBits     int
	UintBits    int
	UintptrBits int
}

type CheckOptions struct {
	// ModulePath overrides the checked module identity while keeping filePath for diagnostics.
	ModulePath string
	Target     TargetInfo
	GoResolver GoPackageResolver
}

func normalizeCheckOptions(options []CheckOptions) CheckOptions {
	if len(options) > 0 {
		return options[0]
	}
	return CheckOptions{}
}
