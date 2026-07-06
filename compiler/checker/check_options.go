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
	// RecordSpans makes the checker record a position-indexed table of
	// resolved source spans for tooling (see SpanIndex). Off for normal
	// compilation.
	RecordSpans bool
}

func normalizeCheckOptions(options []CheckOptions) CheckOptions {
	if len(options) > 0 {
		return options[0]
	}
	return CheckOptions{}
}
