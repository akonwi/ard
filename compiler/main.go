package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/bytecode"
	bytecodevm "github.com/akonwi/ard/bytecode/vm"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/formatter"
	"github.com/akonwi/ard/frontend"
	"github.com/akonwi/ard/javascript"
	"github.com/akonwi/ard/runtime"
	"github.com/akonwi/ard/version"
	vm_next "github.com/akonwi/ard/vm_next"
)

const (
	bytecodeFooterMarker = "ARDBYTECODEv1"
	vmNextFooterMarker   = "ARDVMNEXTv001"
)

func main() {
	if maybeRunEmbedded() {
		return
	}
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
			case backend.TargetBytecode:
				module, err := loadModule(inputPath, target)
				if err != nil {
					os.Exit(1)
				}
				program, err := bytecode.NewEmitter().EmitProgram(module)
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				if err := bytecode.VerifyProgram(program); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				if runErr := runBytecodeProgram(program, os.Args); runErr != nil {
					fmt.Println(runErr)
					os.Exit(1)
				}
			case backend.TargetVMNext:
				profile := newPipelineProfile("run vm_next")
				defer profile.Print()
				var module checker.Module
				if err := profile.Time("frontend.load_module", func() error {
					var loadErr error
					module, loadErr = loadModule(inputPath, target)
					return loadErr
				}); err != nil {
					os.Exit(1)
				}
				var program *air.Program
				if err := profile.Time("air.lower", func() error {
					var lowerErr error
					program, lowerErr = air.Lower(module)
					return lowerErr
				}); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				if err := runVMNextProgramProfile(program, os.Args, profile); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			case backend.TargetGo:
				fmt.Println(goTargetRewriteError())
				os.Exit(1)
			case backend.TargetJSBrowser, backend.TargetJSServer:
				if err := javascript.Run(inputPath, target, os.Args); err != nil {
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
			case backend.TargetBytecode:
				builtPath, err = buildBytecodeBinary(inputPath, outputPath, target)
			case backend.TargetVMNext:
				builtPath, err = buildVMNextBinary(inputPath, outputPath)
			case backend.TargetGo:
				err = goTargetRewriteError()
			case backend.TargetJSBrowser, backend.TargetJSServer:
				builtPath, err = javascript.Build(inputPath, outputPath, target)
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
			inputPath, filter, failFast, err := parseTestArgs(os.Args[2:])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if !runTests(inputPath, filter, failFast) {
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

func goTargetRewriteError() error {
	return fmt.Errorf("go target rewrite in progress")
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
		// Try to use the project name from ard.toml
		inputDir := filepath.Dir(inputPath)
		if inputDir == "" {
			inputDir = "."
		}
		if project, err := checker.FindProjectRoot(inputDir); err == nil && project.ProjectName != "" {
			// Check if the project name came from ard.toml (not just directory fallback)
			tomlPath := filepath.Join(project.RootPath, "ard.toml")
			if _, statErr := os.Stat(tomlPath); statErr == nil {
				outputPath = project.ProjectName
			}
		}
		if outputPath == "" {
			outputPath = strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
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

func parseTestArgs(args []string) (string, string, bool, error) {
	inputPath := "."
	filter := ""
	failFast := false
	seenPath := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--fail-fast":
			failFast = true
		case "--filter":
			if i+1 >= len(args) {
				return "", "", false, fmt.Errorf("--filter requires a value")
			}
			filter = args[i+1]
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", false, fmt.Errorf("unknown flag: %s", arg)
			}
			if seenPath {
				return "", "", false, fmt.Errorf("unexpected argument: %s", arg)
			}
			inputPath = arg
			seenPath = true
		}
	}

	return inputPath, filter, failFast, nil
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

func runTests(inputPath, filter string, failFast bool) bool {
	files, err := discoverTestFiles(inputPath)
	if err != nil {
		fmt.Println(err)
		return false
	}

	outcomes := make([]testOutcome, 0)
	for _, path := range files {
		module, err := loadModule(path, "")
		if err != nil {
			return false
		}
		tests := collectTests(module, path, filter)
		if len(tests) == 0 {
			continue
		}

		program, err := bytecode.NewTestEmitter().EmitProgram(module)
		if err != nil {
			fmt.Println(err)
			return false
		}
		if err := bytecode.VerifyProgram(program); err != nil {
			fmt.Println(err)
			return false
		}

		for _, test := range tests {
			outcome := runCompiledTest(program, test)
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

func runCompiledTest(program bytecode.Program, test discoveredTest) testOutcome {
	runtime.SetOSArgs(os.Args)
	defer runtime.SetOSArgs(nil)

	res, err := bytecodevm.New(program).Run(test.name)
	if err != nil {
		return testOutcome{test: test, status: testPanic, message: err.Error()}
	}
	if res == nil {
		return testOutcome{test: test, status: testPanic, message: "test returned no result"}
	}
	if !res.IsResult() {
		return testOutcome{test: test, status: testPanic, message: "test did not return a Result"}
	}
	if res.IsErr() {
		return testOutcome{test: test, status: testFail, message: resultMessage(res)}
	}
	return testOutcome{test: test, status: testPass}
}

func resultMessage(res *runtime.Object) string {
	if res == nil {
		return ""
	}
	unwrapped := res.UnwrapResult()
	if msg, ok := unwrapped.IsStr(); ok {
		return msg
	}
	return fmt.Sprintf("%v", unwrapped.GoValue())
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

func buildBytecodeBinary(inputPath string, outputPath string, target string) (string, error) {
	module, err := loadModule(inputPath, target)
	if err != nil {
		return "", err
	}
	program, err := bytecode.NewEmitter().EmitProgram(module)
	if err != nil {
		return "", err
	}
	if err := bytecode.VerifyProgram(program); err != nil {
		return "", err
	}
	data, err := bytecode.SerializeProgram(program)
	if err != nil {
		return "", err
	}
	selfPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := writeEmbeddedBinary(selfPath, outputPath, bytecodeFooterMarker, data); err != nil {
		return "", err
	}
	return outputPath, nil
}

func buildVMNextBinary(inputPath string, outputPath string) (string, error) {
	profile := newPipelineProfile("build vm_next")
	defer profile.Print()
	var module checker.Module
	if err := profile.Time("frontend.load_module", func() error {
		var loadErr error
		module, loadErr = loadModule(inputPath, backend.TargetVMNext)
		return loadErr
	}); err != nil {
		return "", err
	}
	var program *air.Program
	if err := profile.Time("air.lower", func() error {
		var lowerErr error
		program, lowerErr = air.Lower(module)
		return lowerErr
	}); err != nil {
		return "", err
	}
	if err := profile.Time("air.validate", func() error {
		return air.Validate(program)
	}); err != nil {
		return "", err
	}
	if program.Entry == air.NoFunction {
		return "", fmt.Errorf("vm_next builds require fn main()")
	}
	var data []byte
	if err := profile.Time("air.serialize", func() error {
		var serializeErr error
		data, serializeErr = air.SerializeProgram(program)
		return serializeErr
	}); err != nil {
		return "", err
	}
	selfPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := profile.Time("embed.write_binary", func() error {
		return writeEmbeddedBinary(selfPath, outputPath, vmNextFooterMarker, data)
	}); err != nil {
		return "", err
	}
	return outputPath, nil
}

func argsForEmbeddedProgram(args []string) []string {
	if len(args) > 1 && args[1] == "run-embedded" {
		out := make([]string, 0, len(args)-1)
		out = append(out, args[0])
		out = append(out, args[2:]...)
		return out
	}
	return append([]string(nil), args...)
}

func maybeRunEmbedded() bool {
	marker, data, err := readEmbeddedPayload()
	if err != nil || data == nil {
		return false
	}
	switch marker {
	case bytecodeFooterMarker:
		program, err := bytecode.DeserializeProgram(data)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to deserialize bytecode:", err)
			os.Exit(1)
		}
		if err := bytecode.VerifyProgram(program); err != nil {
			fmt.Fprintln(os.Stderr, "Invalid bytecode:", err)
			os.Exit(1)
		}
		if runErr := runBytecodeProgram(program, argsForEmbeddedProgram(os.Args)); runErr != nil {
			fmt.Fprintln(os.Stderr, runErr)
			os.Exit(1)
		}
	case vmNextFooterMarker:
		profile := newPipelineProfile("embedded vm_next")
		defer profile.Print()
		var program *air.Program
		if err := profile.Time("air.deserialize", func() error {
			var deserializeErr error
			program, deserializeErr = air.DeserializeProgram(data)
			return deserializeErr
		}); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to deserialize vm_next AIR:", err)
			os.Exit(1)
		}
		if err := profile.Time("air.validate", func() error {
			return air.Validate(program)
		}); err != nil {
			fmt.Fprintln(os.Stderr, "Invalid vm_next AIR:", err)
			os.Exit(1)
		}
		if runErr := runVMNextProgramProfile(program, argsForEmbeddedProgram(os.Args), profile); runErr != nil {
			fmt.Fprintln(os.Stderr, runErr)
			os.Exit(1)
		}
	default:
		return false
	}
	return true
}

func runBytecodeProgram(program bytecode.Program, args []string) error {
	profiling := bytecodevm.ProfilingEnabled()
	if profiling {
		runtime.EnableObjectProfiling(true)
		runtime.ResetObjectProfile()
	} else {
		runtime.EnableObjectProfiling(false)
	}
	defer runtime.EnableObjectProfiling(false)

	runtime.SetOSArgs(args)
	defer runtime.SetOSArgs(nil)

	vm := bytecodevm.New(program)
	_, runErr := vm.Run("main")
	if profiling {
		if report := vm.ProfileReport(); report != "" {
			fmt.Fprintln(os.Stderr, report)
		}
		if report := runtime.ObjectProfileReport(); report != "" {
			fmt.Fprintln(os.Stderr, report)
		}
	}
	return runErr
}

func runVMNextProgram(program *air.Program, args []string) error {
	return runVMNextProgramProfile(program, args, nil)
}

func runVMNextProgramProfile(program *air.Program, args []string, profile *pipelineProfile) error {
	var vm *vm_next.VM
	if err := profile.Time("vm_next.init", func() error {
		var initErr error
		vm, initErr = vm_next.NewWithOptions(program, vm_next.Options{Args: args})
		return initErr
	}); err != nil {
		return err
	}
	var err error
	runErr := profile.Time("vm_next.run", func() error {
		if program.Entry != air.NoFunction {
			_, err = vm.RunEntry()
			return err
		}
		_, err = vm.RunScript()
		return err
	})
	if report := vm.ProfileReport(); report != "" {
		fmt.Fprintln(os.Stderr, report)
	}
	return runErr
}

func writeEmbeddedBinary(srcPath string, dstPath string, marker string, data []byte) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	if _, err := dst.Write(data); err != nil {
		return err
	}
	if err := writeFooter(dst, marker, uint64(len(data))); err != nil {
		return err
	}
	return dst.Chmod(0o755)
}

func writeFooter(w io.Writer, marker string, length uint64) error {
	if _, err := w.Write([]byte(marker)); err != nil {
		return err
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, length)
	_, err := w.Write(buf)
	return err
}

func readEmbeddedPayload() (string, []byte, error) {
	path, err := os.Executable()
	if err != nil {
		return "", nil, err
	}
	return readEmbeddedPayloadFromPath(path)
}

func readEmbeddedPayloadFromPath(path string) (string, []byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", nil, err
	}
	footerSize := int64(len(bytecodeFooterMarker) + 8)
	if info.Size() < footerSize {
		return "", nil, nil
	}
	_, err = file.Seek(info.Size()-footerSize, io.SeekStart)
	if err != nil {
		return "", nil, err
	}
	footer := make([]byte, footerSize)
	if _, err := io.ReadFull(file, footer); err != nil {
		return "", nil, err
	}
	marker := string(footer[:len(bytecodeFooterMarker)])
	if marker != bytecodeFooterMarker && marker != vmNextFooterMarker {
		return "", nil, nil
	}
	length := binary.LittleEndian.Uint64(footer[len(bytecodeFooterMarker):])
	dataOffset := info.Size() - footerSize - int64(length)
	if dataOffset < 0 {
		return "", nil, fmt.Errorf("invalid embedded payload length")
	}
	if _, err := file.Seek(dataOffset, io.SeekStart); err != nil {
		return "", nil, err
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(file, data); err != nil {
		return "", nil, err
	}
	return marker, data, nil
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
