package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/formatter"
	"github.com/akonwi/ard/frontend"
	gotarget "github.com/akonwi/ard/go"
	"github.com/akonwi/ard/javascript"
	"github.com/akonwi/ard/version"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide a command")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Println(version.Get())
		os.Exit(0)
	case "check":
		{
			if len(os.Args) < 3 {
				fmt.Println("Expected filepath argument")
				os.Exit(1)
			}

			inputPath := os.Args[2]
			if !check(inputPath) {
				os.Exit(1)
			}

			fmt.Println("✅ No errors found")
			os.Exit(0)
		}
	case "run":
		{
			inputPath, requestedTarget, err := parseRunArgs(os.Args[2:])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			target, err := resolveTarget(inputPath, requestedTarget)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			switch target {
			case backend.TargetGo:
				profile := newPipelineProfile("run go")
				defer profile.Print()
				var loaded *frontend.LoadResult
				if err := profile.Time("frontend.load_module", func() error {
					var loadErr error
					loaded, loadErr = frontend.LoadModule(inputPath, target)
					return loadErr
				}); err != nil {
					os.Exit(1)
				}
				var program *air.Program
				if err := profile.Time("air.lower", func() error {
					var lowerErr error
					program, lowerErr = air.Lower(loaded.Module)
					return lowerErr
				}); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				if err := validateEntrypointSignature(profile, program); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				if err := gotarget.RunProgram(program, os.Args, loaded.ProjectInfo); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			case backend.TargetJSBrowser, backend.TargetJSServer:
				if err := runJSProgram(inputPath, target, os.Args); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			default:
				fmt.Printf("unknown target: %s\n", target)
				os.Exit(1)
			}
		}
	case "build":
		{
			inputPath, outputPath, requestedTarget, err := parseBuildArgs(os.Args[2:])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			target, err := resolveTarget(inputPath, requestedTarget)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			var builtPath string
			switch target {
			case backend.TargetGo:
				builtPath, err = buildGoBinary(inputPath, outputPath, target)
			case backend.TargetJSBrowser, backend.TargetJSServer:
				builtPath, err = buildJSProgram(inputPath, outputPath, target)
			default:
				err = fmt.Errorf("unknown target: %s", target)
			}
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Printf("Built %s\n", builtPath)
		}
	case "test":
		{
			inputPath, filter, failFast, target, err := parseTestArgs(os.Args[2:])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if !runTests(inputPath, filter, failFast, target) {
				os.Exit(1)
			}
			os.Exit(0)
		}
	case "format":
		{
			inputPath, checkOnly, err := parseFormatArgs(os.Args[2:])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			changedPaths, err := formatPath(inputPath, checkOnly)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if checkOnly {
				if len(changedPaths) > 0 {
					fmt.Println("files with format errors:")
					for _, changedPath := range changedPaths {
						fmt.Println(changedPath)
					}
					os.Exit(1)
				}
				os.Exit(0)
			}
			if len(changedPaths) > 0 {
				os.Exit(0)
			}
			os.Exit(0)
		}
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func check(inputPath string) bool {
	_, err := loadModule(inputPath, "")
	return err == nil
}

func loadModule(inputPath string, target string) (checker.Module, error) {
	result, err := frontend.LoadModule(inputPath, target)
	if err != nil {
		return nil, err
	}
	return result.Module, nil
}

func parseRunArgs(args []string) (string, string, error) {
	inputPath := ""
	target := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--target" {
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--target requires a value")
			}
			parsedTarget, err := backend.ParseTarget(args[i+1])
			if err != nil {
				return "", "", err
			}
			target = parsedTarget
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return "", "", fmt.Errorf("unknown flag: %s", arg)
		}
		if inputPath == "" {
			inputPath = arg
			continue
		}
	}
	if inputPath == "" {
		return "", "", fmt.Errorf("expected filepath argument")
	}
	return inputPath, target, nil
}

func parseBuildArgs(args []string) (string, string, string, error) {
	inputPath := ""
	outputPath := ""
	target := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--out" {
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("--out requires a path")
			}
			outputPath = args[i+1]
			i++
			continue
		}
		if arg == "--target" {
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("--target requires a value")
			}
			parsedTarget, err := backend.ParseTarget(args[i+1])
			if err != nil {
				return "", "", "", err
			}
			target = parsedTarget
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return "", "", "", fmt.Errorf("unknown flag: %s", arg)
		}
		if inputPath == "" {
			inputPath = arg
			continue
		}
	}
	if inputPath == "" {
		return "", "", "", fmt.Errorf("expected filepath argument")
	}
	if outputPath == "" {
		outputPath = filepath.Base(strings.TrimSuffix(inputPath, filepath.Ext(inputPath)))
		if outputPath == "" || outputPath == "." || outputPath == string(filepath.Separator) {
			outputPath = "main"
		}
	}
	return inputPath, outputPath, target, nil
}

func resolveTarget(inputPath, requestedTarget string) (string, error) {
	return frontend.ResolveTarget(inputPath, requestedTarget)
}

func parseFormatArgs(args []string) (string, bool, error) {
	inputPath := ""
	checkOnly := false
	for i := range args {
		arg := args[i]
		if arg == "--check" {
			checkOnly = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return "", false, fmt.Errorf("unknown flag: %s", arg)
		}
		if inputPath == "" {
			inputPath = arg
			continue
		}
		return "", false, fmt.Errorf("unexpected argument: %s", arg)
	}
	if inputPath == "" {
		return "", false, fmt.Errorf("expected filepath argument")
	}
	return inputPath, checkOnly, nil
}

func parseTestArgs(args []string) (string, string, bool, string, error) {
	inputPath := "."
	filter := ""
	failFast := false
	target := backend.DefaultTarget
	seenPath := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--fail-fast":
			failFast = true
		case "--filter":
			if i+1 >= len(args) {
				return "", "", false, "", fmt.Errorf("--filter requires a value")
			}
			filter = args[i+1]
			i++
		case "--target":
			if i+1 >= len(args) {
				return "", "", false, "", fmt.Errorf("--target requires a value")
			}
			parsedTarget, err := backend.ParseTarget(args[i+1])
			if err != nil {
				return "", "", false, "", err
			}
			target = parsedTarget
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", false, "", fmt.Errorf("unknown flag: %s", arg)
			}
			if seenPath {
				return "", "", false, "", fmt.Errorf("unexpected argument: %s", arg)
			}
			inputPath = arg
			seenPath = true
		}
	}

	return inputPath, filter, failFast, target, nil
}

func formatPath(inputPath string, checkOnly bool) ([]string, error) {
	fileInfo, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("error reading path %s - %w", inputPath, err)
	}

	if !fileInfo.IsDir() {
		changed, err := formatFile(inputPath, checkOnly)
		if err != nil {
			return nil, err
		}
		if changed {
			return []string{inputPath}, nil
		}
		return nil, nil
	}

	ardFiles := make([]string, 0)
	err = filepath.WalkDir(inputPath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".ard" {
			ardFiles = append(ardFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error walking directory %s - %w", inputPath, err)
	}

	changedPaths := make([]string, 0)
	for _, filePath := range ardFiles {
		changed, fileErr := formatFile(filePath, checkOnly)
		if fileErr != nil {
			return nil, fileErr
		}
		if changed {
			changedPaths = append(changedPaths, filePath)
		}
	}
	return changedPaths, nil
}

func formatFile(inputPath string, checkOnly bool) (bool, error) {
	sourceCode, err := os.ReadFile(inputPath)
	if err != nil {
		return false, fmt.Errorf("error reading file %s - %w", inputPath, err)
	}

	formatted, err := formatter.Format(sourceCode, inputPath)
	if err != nil {
		return false, fmt.Errorf("error formatting file %s - %w", inputPath, err)
	}
	changed := !bytes.Equal(sourceCode, formatted)
	if !changed || checkOnly {
		return changed, nil
	}

	fileInfo, err := os.Stat(inputPath)
	if err != nil {
		return false, fmt.Errorf("error reading file info %s - %w", inputPath, err)
	}

	if err := os.WriteFile(inputPath, formatted, fileInfo.Mode()); err != nil {
		return false, fmt.Errorf("error writing file %s - %w", inputPath, err)
	}

	return true, nil
}

type discoveredTest struct {
	filePath    string
	displayPath string
	name        string
}

type testStatus string

const (
	testPass  testStatus = "pass"
	testFail  testStatus = "fail"
	testPanic testStatus = "panic"
)

func (s testStatus) symbol() string {
	switch s {
	case testPass:
		return "✓"
	case testFail:
		return "✗"
	case testPanic:
		return "💥"
	default:
		return "?"
	}
}

type testOutcome struct {
	test    discoveredTest
	status  testStatus
	message string
}

func runTests(inputPath, filter string, failFast bool, target ...string) bool {
	testTarget := backend.DefaultTarget
	if len(target) > 0 && target[0] != "" {
		testTarget = target[0]
	}
	if testTarget != backend.TargetGo {
		fmt.Printf("unsupported test target: %s\n", testTarget)
		return false
	}
	return runGoTests(inputPath, filter, failFast)
}

func runGoTests(inputPath, filter string, failFast bool) bool {
	files, err := discoverTestFiles(inputPath)
	if err != nil {
		fmt.Println(err)
		return false
	}

	outcomes := make([]testOutcome, 0)
	for _, path := range files {
		loaded, err := frontend.LoadModule(path, backend.TargetGo)
		if err != nil {
			return false
		}
		tests := collectTests(loaded.Module, path, filter)
		if len(tests) == 0 {
			continue
		}

		program, err := air.LowerWithTests(loaded.Module)
		if err != nil {
			fmt.Println(err)
			return false
		}
		if err := air.Validate(program); err != nil {
			fmt.Println(err)
			return false
		}
		goTests := make([]gotarget.TestCase, 0, len(tests))
		for _, test := range tests {
			fnID := air.NoFunction
			for _, airTest := range program.Tests {
				if airTest.Name == test.name {
					fnID = airTest.Function
					break
				}
			}
			if fnID == air.NoFunction {
				fmt.Printf("test not found: %s\n", test.displayName())
				return false
			}
			goTests = append(goTests, gotarget.TestCase{Name: test.name, DisplayName: test.displayName(), Function: fnID})
		}
		goOutcomes, err := gotarget.RunTests(program, []string{"ard", "test", path}, goTests, failFast, loaded.ProjectInfo)
		if err != nil {
			fmt.Println(err)
			return false
		}
		byName := map[string]discoveredTest{}
		for _, test := range tests {
			byName[test.displayName()] = test
		}
		for _, goOutcome := range goOutcomes {
			test, ok := byName[goOutcome.DisplayName]
			if !ok {
				test = discoveredTest{filePath: path, displayPath: strings.TrimSuffix(filepath.Clean(path), filepath.Ext(path)), name: goOutcome.Name}
			}
			outcome := testOutcome{test: test, message: goOutcome.Message}
			switch goOutcome.Status {
			case "pass":
				outcome.status = testPass
			case "fail":
				outcome.status = testFail
			default:
				outcome.status = testPanic
			}
			outcomes = append(outcomes, outcome)
			reportTestOutcome(outcome)
			if failFast && outcome.status != testPass {
				reportTestSummary(outcomes)
				return false
			}
		}
	}

	if len(outcomes) == 0 {
		fmt.Println("No tests found")
		return true
	}
	reportTestSummary(outcomes)
	for _, outcome := range outcomes {
		if outcome.status != testPass {
			return false
		}
	}
	return true
}

func stdlibModulePathForTestFile(root, filePath string) (string, bool) {
	root = filepath.Clean(root)
	filePath = filepath.Clean(filePath)
	if filepath.Base(root) != "std_lib" || filepath.Ext(filePath) != ".ard" {
		return "", false
	}
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(strings.TrimSuffix(rel, ".ard"))
	if rel == "" || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return "ard/" + rel, true
}

func discoverTestFiles(inputPath string) ([]string, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("error reading path %s - %w", inputPath, err)
	}

	if !info.IsDir() {
		if filepath.Ext(inputPath) != ".ard" {
			return nil, fmt.Errorf("expected an .ard file or directory: %s", inputPath)
		}
		return []string{filepath.Clean(inputPath)}, nil
	}

	files := make([]string, 0)
	seen := make(map[string]struct{})
	err = filepath.WalkDir(inputPath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && path != inputPath {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".ard" {
			return nil
		}
		if modulePath, ok := stdlibModulePathForTestFile(inputPath, path); ok {
			if err := checker.ValidateStdlibImportTarget(modulePath, backend.DefaultTarget); err != nil {
				return nil
			}
		}
		cleaned := filepath.Clean(path)
		if _, ok := seen[cleaned]; ok {
			return nil
		}
		seen[cleaned] = struct{}{}
		files = append(files, cleaned)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func collectTests(module checker.Module, filePath string, filter string) []discoveredTest {
	program := module.Program()
	if program == nil {
		return nil
	}

	displayPath := strings.TrimSuffix(filepath.Clean(filePath), filepath.Ext(filePath))
	tests := make([]discoveredTest, 0)
	for _, stmt := range program.Statements {
		fn, ok := stmt.Expr.(*checker.FunctionDef)
		if !ok || !fn.IsTest {
			continue
		}
		test := discoveredTest{
			filePath:    filePath,
			displayPath: displayPath,
			name:        fn.Name,
		}
		if filter != "" && !strings.Contains(test.displayName(), filter) {
			continue
		}
		tests = append(tests, test)
	}
	return tests
}

func (t discoveredTest) displayName() string {
	return fmt.Sprintf("%s::%s", t.displayPath, t.name)
}

func reportTestOutcome(outcome testOutcome) {
	fmt.Printf("%s  %s\n", outcome.status.symbol(), outcome.test.displayName())
	if outcome.message != "" && outcome.status != testPass {
		fmt.Printf("  %s\n", outcome.message)
	}
}

func reportTestSummary(outcomes []testOutcome) {
	passed := 0
	failed := 0
	panicked := 0
	for _, outcome := range outcomes {
		switch outcome.status {
		case testPass:
			passed++
		case testFail:
			failed++
		case testPanic:
			panicked++
		}
	}
	fmt.Printf("\n%d passed; %d failed; %d panicked\n", passed, failed, panicked)
}

func runJSProgram(inputPath string, target string, args []string) error {
	profile := newPipelineProfile("run javascript")
	defer profile.Print()
	var loaded *frontend.LoadResult
	if err := profile.Time("frontend.load_module", func() error {
		var loadErr error
		loaded, loadErr = frontend.LoadModule(inputPath, target)
		return loadErr
	}); err != nil {
		return err
	}
	var program *air.Program
	if err := profile.Time("air.lower", func() error {
		var lowerErr error
		program, lowerErr = air.Lower(loaded.Module)
		return lowerErr
	}); err != nil {
		return err
	}
	if err := validateEntrypointSignature(profile, program); err != nil {
		return err
	}
	if err := profile.Time("javascript.run", func() error {
		return javascript.RunProgram(program, target, args, loaded.ProjectInfo)
	}); err != nil {
		return err
	}
	return nil
}

func buildJSProgram(inputPath string, outputPath string, target string) (string, error) {
	profile := newPipelineProfile("build javascript")
	defer profile.Print()
	var loaded *frontend.LoadResult
	if err := profile.Time("frontend.load_module", func() error {
		var loadErr error
		loaded, loadErr = frontend.LoadModule(inputPath, target)
		return loadErr
	}); err != nil {
		return "", err
	}
	var program *air.Program
	if err := profile.Time("air.lower", func() error {
		var lowerErr error
		program, lowerErr = air.Lower(loaded.Module)
		return lowerErr
	}); err != nil {
		return "", err
	}
	if err := validateEntrypointSignature(profile, program); err != nil {
		return "", err
	}
	outputPath = resolveJSBuildOutputPath(inputPath, outputPath, target, loaded.ProjectInfo)
	var builtPath string
	if err := profile.Time("javascript.build", func() error {
		var buildErr error
		builtPath, buildErr = javascript.BuildProgram(program, outputPath, target, loaded.ProjectInfo)
		return buildErr
	}); err != nil {
		return "", err
	}
	return builtPath, nil
}

func resolveJSBuildOutputPath(inputPath string, outputPath string, target string, projectInfo *checker.ProjectInfo) string {
	defaultOutput := filepath.Base(strings.TrimSuffix(inputPath, filepath.Ext(inputPath)))
	if defaultOutput == "" || defaultOutput == "." || defaultOutput == string(filepath.Separator) {
		defaultOutput = "main"
	}
	if outputPath != defaultOutput {
		return outputPath
	}
	rootDir := ""
	if projectInfo != nil {
		rootDir = strings.TrimSpace(projectInfo.RootPath)
	}
	if rootDir == "" {
		rootDir = filepath.Dir(inputPath)
		if rootDir == "" || rootDir == "." {
			rootDir = "."
		}
	}
	return filepath.Join(rootDir, "ard-out", target, defaultOutput+".mjs")
}

func buildGoBinary(inputPath string, outputPath string, target string) (string, error) {
	profile := newPipelineProfile("build go")
	defer profile.Print()
	var loaded *frontend.LoadResult
	if err := profile.Time("frontend.load_module", func() error {
		var loadErr error
		loaded, loadErr = frontend.LoadModule(inputPath, target)
		return loadErr
	}); err != nil {
		return "", err
	}
	var program *air.Program
	if err := profile.Time("air.lower", func() error {
		var lowerErr error
		program, lowerErr = air.Lower(loaded.Module)
		return lowerErr
	}); err != nil {
		return "", err
	}
	if err := profile.Time("air.validate", func() error {
		return air.Validate(program)
	}); err != nil {
		return "", err
	}
	if err := validateEntrypointSignature(profile, program); err != nil {
		return "", err
	}
	if outputPath == "" {
		outputPath = filepath.Base(strings.TrimSuffix(inputPath, filepath.Ext(inputPath)))
		if outputPath == "" || outputPath == "." || outputPath == string(filepath.Separator) {
			outputPath = "main"
		}
	}
	var builtPath string
	if err := profile.Time("go.build", func() error {
		var buildErr error
		builtPath, buildErr = gotarget.BuildProgram(program, outputPath, loaded.ProjectInfo)
		return buildErr
	}); err != nil {
		return "", err
	}
	return builtPath, nil
}

func validateEntrypointSignature(profile *pipelineProfile, program *air.Program) error {
	return profile.Time("air.validate_entrypoint", func() error {
		return air.ValidateEntrypointSignature(program)
	})
}

const pipelineProfileEnvVar = "ARD_PIPELINE_PROFILE"

type pipelineProfile struct {
	scope   string
	started time.Time
	stages  []pipelineProfileStage
}

type pipelineProfileStage struct {
	name string
	dur  time.Duration
}

func newPipelineProfile(scope string) *pipelineProfile {
	if !pipelineProfilingEnabled() {
		return nil
	}
	return &pipelineProfile{scope: scope, started: time.Now()}
}

func pipelineProfilingEnabled() bool {
	raw, ok := os.LookupEnv(pipelineProfileEnvVar)
	if !ok {
		return false
	}
	raw = strings.TrimSpace(strings.ToLower(raw))
	return raw != "" && raw != "0" && raw != "false" && raw != "off"
}

func (p *pipelineProfile) Time(name string, fn func() error) error {
	if p == nil {
		return fn()
	}
	started := time.Now()
	err := fn()
	p.stages = append(p.stages, pipelineProfileStage{name: name, dur: time.Since(started)})
	return err
}

func (p *pipelineProfile) Print() {
	if p == nil {
		return
	}
	fmt.Fprintln(os.Stderr, p.Report())
}

func (p *pipelineProfile) Report() string {
	if p == nil {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "[ard pipeline profile: %s]\n", p.scope)
	fmt.Fprintf(&out, "total=%s\n", time.Since(p.started).Round(time.Microsecond))
	for _, stage := range p.stages {
		fmt.Fprintf(&out, "%s=%s\n", stage.name, stage.dur.Round(time.Microsecond))
	}
	return strings.TrimRight(out.String(), "\n")
}
