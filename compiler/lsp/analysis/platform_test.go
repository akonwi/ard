package analysis

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/compiler/parse"
	"github.com/akonwi/ard/compiler/platform"
)

var (
	linuxAMD64   = platform.Platform{OS: "linux", Arch: "amd64"}
	windowsAMD64 = platform.Platform{OS: "windows", Arch: "amd64"}
)

const platformSource = `fn value() Int { 1 }

#platform(windows)
fn value() Int { "windows" }

fn main() {
  let v = value()
}
`

func TestPlatformViewsCheckInactiveDeclarations(t *testing.T) {
	root := writeProject(t, map[string]string{"main.ard": platformSource})
	engine := NewEngine(root)
	engine.SetPrimaryPlatform(linuxAMD64)
	snap := NewWorkspace(engine).Snapshot()
	path := filepath.Join(root, "main.ard")
	ctx := context.Background()

	primary, err := snap.Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if primary.Platform != linuxAMD64 || len(primary.Diagnostics) != 0 {
		t.Fatalf("primary = %s with %d diagnostics", primary.Platform, len(primary.Diagnostics))
	}

	views, err := snap.PlatformViews(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Platform != windowsAMD64 {
		t.Fatalf("views = %+v, want one windows/amd64 view", views)
	}
	view := views[0]
	if len(view.Regions) != 1 || view.Regions[0].Start.Row != 3 || view.Regions[0].End.Row != 4 {
		t.Fatalf("regions = %+v, want rows 3-4", view.Regions)
	}
	if view.Analysis.Platform != windowsAMD64 || len(view.Analysis.Diagnostics) != 1 || view.Analysis.Diagnostics[0].Location().Start.Row != 4 {
		t.Fatalf("windows analysis = %s with %v", view.Analysis.Platform, view.Analysis.Diagnostics)
	}

	inside, err := snap.AnalyzeAtCtx(ctx, path, parse.Point{Row: 4, Col: 20})
	if err != nil {
		t.Fatal(err)
	}
	if inside != view.Analysis {
		t.Fatalf("analysis inside the windows declaration = %s, want the windows view", inside.Platform)
	}
	outside, err := snap.AnalyzeAtCtx(ctx, path, parse.Point{Row: 7, Col: 11})
	if err != nil {
		t.Fatal(err)
	}
	if outside != primary {
		t.Fatalf("analysis outside = %s, want the primary analysis", outside.Platform)
	}
	ephemeral, err := snap.AnalyzeEphemeralAt(ctx, path, parse.Point{Row: 3, Col: 1})
	if err != nil {
		t.Fatal(err)
	}
	if ephemeral.Platform != windowsAMD64 {
		t.Fatalf("ephemeral analysis on the attribute = %s, want windows/amd64", ephemeral.Platform)
	}
}

func TestPlatformViewsAreEmptyWithoutAttributes(t *testing.T) {
	root := writeProject(t, map[string]string{"main.ard": "fn main() {}\n"})
	engine := NewEngine(root)
	snap := NewWorkspace(engine).Snapshot()
	views, err := snap.PlatformViews(context.Background(), filepath.Join(root, "main.ard"))
	if err != nil || len(views) != 0 {
		t.Fatalf("views = %v, %v", views, err)
	}
}
