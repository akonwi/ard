package checker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestCheckerReportsPrimedGoImportCoverageMissOnce(t *testing.T) {
	resolver := checker.NewGoPackagesResolver(t.TempDir(), nil)
	if err := resolver.Prime(nil); err != nil {
		t.Fatal(err)
	}
	result := parse.Parse([]byte("use go:fmt\n"), "main.ard")
	c := checker.New("main.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
	c.Check()
	count := 0
	for _, diagnostic := range c.Diagnostics() {
		if diagnostic.Code == checker.DiagnosticCodeGoImportResolution {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("go import resolution diagnostics = %d; diagnostics: %#v", count, c.Diagnostics())
	}
}

func TestGoPackagesResolverResolvesStdlibWithoutGoMod(t *testing.T) {
	resolver := checker.NewGoPackagesResolver(t.TempDir(), nil)
	if err := resolver.Prime([]string{"fmt"}); err != nil {
		t.Fatalf("Prime(%s): %v", "fmt", err)
	}
	pkg, err := resolver.ResolveGoPackage("fmt")
	if err != nil {
		t.Fatalf("ResolveGoPackage(fmt): %v", err)
	}
	if pkg.Functions["Println"] == nil {
		t.Fatal("fmt.Println missing from resolved package")
	}
}

func TestGoPackagesResolverResolvesLocalModulePackage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte(`package ffi

func Greet(name string) string { return "hello " + name }
`), 0644); err != nil {
		t.Fatal(err)
	}
	resolver := checker.NewGoPackagesResolver(root, nil)
	if err := resolver.Prime([]string{"example.com/app/ffi"}); err != nil {
		t.Fatalf("Prime(%s): %v", "example.com/app/ffi", err)
	}
	pkg, err := resolver.ResolveGoPackage("example.com/app/ffi")
	if err != nil {
		t.Fatalf("ResolveGoPackage(local ffi): %v", err)
	}
	fn := pkg.Functions["Greet"]
	if fn == nil {
		t.Fatal("Greet missing from resolved package")
	}
	if got := fn.ReturnType.String(); got != "Str" {
		t.Fatalf("Greet return = %s, want Str", got)
	}
}

func TestGoPackagesResolverMapsChannelDirections(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte(`package ffi

func Both() chan int { return make(chan int) }
func In(ch chan<- int) {}
func Out() <-chan int { return make(chan int) }
`), 0644); err != nil {
		t.Fatal(err)
	}
	resolver := checker.NewGoPackagesResolver(root, nil)
	if err := resolver.Prime([]string{"example.com/app/ffi"}); err != nil {
		t.Fatalf("Prime(%s): %v", "example.com/app/ffi", err)
	}
	pkg, err := resolver.ResolveGoPackage("example.com/app/ffi")
	if err != nil {
		t.Fatalf("ResolveGoPackage(local ffi): %v", err)
	}
	if got := pkg.Functions["Both"].ReturnType.String(); got != "Chan<Int>" {
		t.Fatalf("Both return = %s, want Chan<Int>", got)
	}
	if got := pkg.Functions["In"].Parameters[0].Type.String(); got != "Sender<Int>" {
		t.Fatalf("In param = %s, want Sender<Int>", got)
	}
	if got := pkg.Functions["Out"].ReturnType.String(); got != "Receiver<Int>" {
		t.Fatalf("Out return = %s, want Receiver<Int>", got)
	}
}

func TestGoPackagesResolverRejectsNamedChannelTypes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte(`package ffi

type Ticks <-chan int

func NewTicks() Ticks { return make(chan int) }
`), 0644); err != nil {
		t.Fatal(err)
	}
	resolver := checker.NewGoPackagesResolver(root, nil)
	if err := resolver.Prime([]string{"example.com/app/ffi"}); err != nil {
		t.Fatalf("Prime(%s): %v", "example.com/app/ffi", err)
	}
	pkg, err := resolver.ResolveGoPackage("example.com/app/ffi")
	if err != nil {
		t.Fatalf("ResolveGoPackage(local ffi): %v", err)
	}
	if pkg.Functions["NewTicks"] != nil {
		t.Fatal("NewTicks should be unsupported while named channel types are deferred")
	}
	if got := pkg.UnsupportedFunctions["NewTicks"]; got != "named Go types with underlying <-chan int are not supported yet" {
		t.Fatalf("unsupported reason = %q", got)
	}
}

func TestGoPackagesResolverUsesBuildTags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte("package ffi\n\nfunc Always() string { return \"always\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "tagged.go"), []byte("//go:build special\n\npackage ffi\n\nfunc Tagged() string { return \"tagged\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	withoutTag := checker.NewGoPackagesResolver(root, nil)
	if err := withoutTag.Prime([]string{"example.com/app/ffi"}); err != nil {
		t.Fatalf("Prime(%s): %v", "example.com/app/ffi", err)
	}
	pkg, err := withoutTag.ResolveGoPackage("example.com/app/ffi")
	if err != nil {
		t.Fatalf("ResolveGoPackage without tags: %v", err)
	}
	if pkg.Functions["Tagged"] != nil {
		t.Fatal("Tagged should not be visible without build tag")
	}
	withTag := checker.NewGoPackagesResolver(root, []string{"special"})
	if err := withTag.Prime([]string{"example.com/app/ffi"}); err != nil {
		t.Fatalf("Prime(%s): %v", "example.com/app/ffi", err)
	}
	pkg, err = withTag.ResolveGoPackage("example.com/app/ffi")
	if err != nil {
		t.Fatalf("ResolveGoPackage with tags: %v", err)
	}
	if pkg.Functions["Tagged"] == nil {
		t.Fatal("Tagged should be visible with build tag")
	}
}

func TestGoPackagesResolverPrimeSharesOneLoad(t *testing.T) {
	resolver := checker.NewGoPackagesResolver(t.TempDir(), nil)
	if err := resolver.Prime([]string{"fmt", "strings", "fmt", ""}); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	// Re-priming with covered paths is an idempotent no-op.
	if err := resolver.Prime([]string{"fmt"}); err != nil {
		t.Fatalf("re-Prime with covered paths: %v", err)
	}
	pkg, err := resolver.ResolveGoPackage("fmt")
	if err != nil {
		t.Fatalf("ResolveGoPackage(fmt): %v", err)
	}
	if pkg.Functions["Println"] == nil {
		t.Fatal("fmt.Println missing from primed package")
	}
	if _, err := resolver.ResolveGoPackage("strings"); err != nil {
		t.Fatalf("ResolveGoPackage(strings): %v", err)
	}
}

func TestGoPackagesResolverPrimedMissIsInternalError(t *testing.T) {
	resolver := checker.NewGoPackagesResolver(t.TempDir(), nil)
	if err := resolver.Prime([]string{"fmt"}); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	// Re-priming with an uncovered path would create a second universe, so
	// it reports the internal pre-scan error instead of loading.
	if err := resolver.Prime([]string{"strings"}); err == nil {
		t.Fatal("expected an internal error for re-priming with a new path")
	} else if !strings.Contains(err.Error(), "internal compiler bug") || !strings.Contains(err.Error(), "pre-scan") {
		t.Fatalf("re-prime error = %q, want internal pre-scan bug report", err)
	}
	_, err := resolver.ResolveGoPackage("strings")
	if err == nil {
		t.Fatal("expected an internal error for a post-prime miss")
	}
	if !strings.Contains(err.Error(), "internal compiler bug") || !strings.Contains(err.Error(), "pre-scan") {
		t.Fatalf("post-prime miss error = %q, want internal pre-scan bug report", err)
	}
}

func TestGoPackagesResolverPrimeRecordsPerPathErrors(t *testing.T) {
	resolver := checker.NewGoPackagesResolver(t.TempDir(), nil)
	if err := resolver.Prime([]string{"fmt", "example.com/definitely/missing"}); err != nil {
		t.Fatalf("Prime should not fail for per-path errors: %v", err)
	}
	if _, err := resolver.ResolveGoPackage("fmt"); err != nil {
		t.Fatalf("ResolveGoPackage(fmt): %v", err)
	}
	if _, err := resolver.ResolveGoPackage("example.com/definitely/missing"); err == nil {
		t.Fatal("expected an error for the missing package")
	}
}
