package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/frontend"
	"github.com/akonwi/ard/parse"
)

const defaultBenchmarkInput = "../../examples/vaxis-demo/main.ard"

func BenchmarkGoPipeline(b *testing.B) {
	input := os.Getenv("ARD_BENCH_INPUT")
	if input == "" {
		input = defaultBenchmarkInput
	}
	loaded, err := frontend.LoadModule(input)
	if err != nil {
		b.Fatal(err)
	}
	source, err := os.ReadFile(input)
	if err != nil {
		b.Fatal(err)
	}
	parsed := parse.Parse(source, input)
	if len(parsed.Errors) != 0 {
		b.Fatalf("parse errors: %v", parsed.Errors)
	}
	inputPath, err := filepath.Abs(input)
	if err != nil {
		b.Fatal(err)
	}
	moduleResolver, err := checker.NewModuleResolver(filepath.Dir(inputPath))
	if err != nil {
		b.Fatal(err)
	}
	projectInfo := moduleResolver.GetProjectInfo()
	modulePath, err := filepath.Rel(projectInfo.RootPath, inputPath)
	if err != nil {
		b.Fatal(err)
	}
	goResolver := checker.NewGoPackagesResolver(projectInfo.RootPath, projectInfo.Go.BuildTags)
	goResolver.DependencyModuleRoots = checker.DependencyGoModuleRoots(projectInfo)
	checkOptions := checker.CheckOptions{GoResolver: goResolver}
	warmChecker := checker.New(modulePath, parsed.Program, moduleResolver, checkOptions)
	warmChecker.Check()
	if warmChecker.HasErrors() {
		b.Fatalf("check errors: %v", warmChecker.Diagnostics())
	}
	renderProgram, err := air.Lower(loaded.Module)
	if err != nil {
		b.Fatal(err)
	}
	generated, err := lowerProgram(renderProgram, Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo})
	if err != nil {
		b.Fatal(err)
	}

	b.Run("frontend_load_and_check", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := frontend.LoadModule(input); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("warm_check", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			checked := checker.New(modulePath, parsed.Program, moduleResolver, checkOptions)
			checked.Check()
			if checked.HasErrors() {
				b.Fatal(checked.Diagnostics())
			}
		}
	})
	b.Run("project_check_cached_go", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			resolver, err := checker.NewModuleResolver(filepath.Dir(inputPath))
			if err != nil {
				b.Fatal(err)
			}
			checked := checker.New(modulePath, parsed.Program, resolver, checkOptions)
			checked.Check()
			if checked.HasErrors() {
				b.Fatal(checked.Diagnostics())
			}
		}
	})
	b.Run("air_lower", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := air.Lower(loaded.Module); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("go_ast_lower", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			b.StopTimer()
			program, err := air.Lower(loaded.Module)
			if err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
			if _, err := lowerProgram(program, Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("go_render", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for _, file := range generated {
				if _, err := renderFile(file); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("go_generate_sources", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			b.StopTimer()
			program, err := air.Lower(loaded.Module)
			if err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
			if _, err := GenerateSources(program, Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
