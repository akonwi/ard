package analysis

import (
	"context"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/platform"
)

// PlatformView is a file analyzed for a platform other than the engine's
// primary platform. It covers the declarations and `use` lines the primary
// platform does not select (ADR 0082).
type PlatformView struct {
	Platform platform.Platform
	// Regions are the parts of the file this view is authoritative for.
	Regions  []parse.Location
	Analysis *FileAnalysis
}

// Contains reports whether point falls inside one of the view's regions.
func (v PlatformView) Contains(point parse.Point) bool {
	for _, region := range v.Regions {
		if locationContains(region, point) {
			return true
		}
	}
	return false
}

func locationContains(location parse.Location, point parse.Point) bool {
	afterStart := point.Row > location.Start.Row || (point.Row == location.Start.Row && point.Col >= location.Start.Col)
	beforeEnd := point.Row < location.End.Row || (point.Row == location.End.Row && point.Col <= location.End.Col)
	return afterStart && beforeEnd
}

// inactiveRegions returns the file's regions the primary platform does not
// select, each with the platform an editor should analyze it for.
func (s *Snapshot) inactiveRegions(filePath string) ([]checker.PlatformRegion, error) {
	program, parseErrors, err := s.Parse(filePath)
	if err != nil || program == nil || len(parseErrors) > 0 {
		return nil, err
	}
	analysis := checker.AnalyzePlatforms(program)
	if analysis == nil {
		return nil, nil
	}
	primary := s.engine.PrimaryPlatform()
	return analysis.InactiveRegions(primary, platform.Preferred(primary)), nil
}

// PlatformViews analyzes the file once for each platform that selects
// declarations the primary platform does not. It returns nothing for files
// without `#platform` declarations or with parse errors. A platform whose
// analysis fails is left out, so it cannot hide the primary analysis; only
// cancellation is returned as an error.
func (s *Snapshot) PlatformViews(ctx context.Context, filePath string) ([]PlatformView, error) {
	regions, err := s.inactiveRegions(filePath)
	if err != nil || len(regions) == 0 {
		return nil, err
	}
	var views []PlatformView
	index := map[platform.Platform]int{}
	for _, region := range regions {
		if region.Platform.IsZero() {
			continue
		}
		position, ok := index[region.Platform]
		if !ok {
			position = len(views)
			index[region.Platform] = position
			views = append(views, PlatformView{Platform: region.Platform})
		}
		views[position].Regions = append(views[position].Regions, region.Location)
	}
	analyzed := views[:0]
	for _, view := range views {
		analysis, err := s.analyze(ctx, filePath, true, view.Platform)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			continue
		}
		view.Analysis = analysis
		analyzed = append(analyzed, view)
	}
	return analyzed, nil
}

// platformAt is the platform to analyze the file for at point: the platform
// that selects the declaration there, or zero for the primary platform.
func (s *Snapshot) platformAt(filePath string, point parse.Point) platform.Platform {
	regions, err := s.inactiveRegions(filePath)
	if err != nil {
		return platform.Platform{}
	}
	for _, region := range regions {
		if locationContains(region.Location, point) {
			return region.Platform
		}
	}
	return platform.Platform{}
}

// AnalyzeAtCtx is AnalyzeCtx for features that act at point: inside a
// declaration the primary platform does not select, the file is analyzed for
// a platform that selects it.
func (s *Snapshot) AnalyzeAtCtx(ctx context.Context, filePath string, point parse.Point) (*FileAnalysis, error) {
	return s.analyze(ctx, filePath, true, s.platformAt(filePath, point))
}

// AnalyzeEphemeralAt is AnalyzeEphemeral routed by point like AnalyzeAtCtx.
func (s *Snapshot) AnalyzeEphemeralAt(ctx context.Context, filePath string, point parse.Point) (*FileAnalysis, error) {
	return s.analyze(ctx, filePath, false, s.platformAt(filePath, point))
}
